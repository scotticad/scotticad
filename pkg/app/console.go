//go:build windows

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/script"
	"github.com/scotticad/scotticad/pkg/win"
)

// The Scripting console: a panel below the viewport with a transcript
// (print output, values, errors), a prompt, and an Editor tab for longer
// scripts. Scripts are Starlark (pkg/script); they reach the design
// through the same API as the Python client, and each run is one undo
// step.

const (
	conMaxLines   = 10000
	conHeaderH    = 34.0
	conMinH       = 150.0
	conDefaultH   = 280.0
	conInputLines = 8 // the prompt grows to this many lines
)

type conLine struct {
	kind  script.Kind
	text  string
	count int // repeats of the same line, collapsed
	file  string
	line  int
}

type console struct {
	open   bool
	script bool // the Editor tab is showing
	height float64

	lines  []conLine
	scroll int // transcript rows scrolled up from the bottom
	// a selection in the transcript, and whether the transcript has the
	// keyboard (for Ctrl+C)
	selA, selB tPos
	tfocus     bool
	view       transcriptView

	input   *codeEdit
	editor  *codeEdit
	focus   *codeEdit // the editor taking keys, or nil
	history []string
	queued  []string // prompt entries typed while a script ran, run next
	histPos int      // index into history while browsing; len(history) when not
	draft   string   // the input before browsing history

	path  string // the Editor tab's file
	saved string // its text as last opened or saved

	sess     *script.Session
	running  bool
	runStart time.Time
	runName  string
	nodes0   int

	unread    bool // output arrived while the panel was closed
	inputView codeView
	editView  codeView

	mu      sync.Mutex
	pending []script.Line
	dropped int            // output lines dropped before they were shown
	capture *[]script.Line // an API run's output, also collected here
}

func (a *App) initConsole() {
	c := &a.con
	c.height = conDefaultH
	c.input, c.editor = newCodeEdit(), newCodeEdit()
	c.sess = script.NewSession(a.scriptCall, a.scriptOut)
	c.sess.Doc = func(api string) string {
		for _, m := range apiMethods {
			if m.Name == api {
				return m.Doc
			}
		}
		return ""
	}
	c.addLine(conLine{kind: script.Info, text: "ScottiCAD scripting — Starlark, a dialect of Python. Type help() for the cad methods."})
}

// scriptCall is the scripts' route to the API: JSON in, JSON out, through
// the same dispatcher as the API's socket, on the UI thread.
func (a *App) scriptCall(method string, params []byte) ([]byte, error) {
	switch method {
	case "quit":
		return nil, errors.New("a script can't close ScottiCAD")
	case "run_script":
		return nil, errors.New("a script can't start another script")
	}
	p := args{}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	reply := make(chan apiResult, 1)
	a.calls <- apiCall{method: method, params: p, reply: reply}
	a.w.Wake()
	r := <-reply
	if r.err != nil {
		return nil, r.err
	}
	if r.result == nil {
		r.result = map[string]any{}
	}
	return json.Marshal(r.result)
}

// scriptOut receives output from the running script's goroutine.
func (a *App) scriptOut(l script.Line) {
	c := &a.con
	c.mu.Lock()
	c.pending = append(c.pending, l)
	if len(c.pending) > 2*conMaxLines {
		// more than the transcript keeps is waiting to be shown: drop the
		// oldest (they would be trimmed off anyway)
		n := len(c.pending) - conMaxLines
		c.pending = append(c.pending[:0], c.pending[n:]...)
		c.dropped += n
	}
	if c.capture != nil {
		*c.capture = append(*c.capture, l)
	}
	c.mu.Unlock()
	a.wakePending.Store(true)
	a.w.Wake()
}

// post runs f on the UI thread.
func (a *App) post(f func()) {
	a.calls <- apiCall{do: f}
	a.w.Wake()
}

// drainConsole moves pending output into the transcript (UI thread).
func (a *App) drainConsole() {
	c := &a.con
	c.mu.Lock()
	ls, dropped := c.pending, c.dropped
	c.pending, c.dropped = nil, 0
	c.mu.Unlock()
	for _, l := range ls {
		for _, t := range strings.Split(l.Text, "\n") {
			c.addLine(conLine{kind: l.Kind, text: t, file: l.File, line: l.Line})
		}
		if !c.open {
			c.unread = true
			if l.Kind == script.Error {
				c.open = true // an error never goes unseen
			}
		}
	}
	if dropped > 0 {
		// the note goes after the kept lines: before them, trimming could
		// drop it at once
		c.info("(%d earlier lines of output weren't kept)", dropped)
	}
	if len(ls) > 0 {
		a.w.Redraw()
	}
}

// addLine appends to the transcript, collapsing repeats and keeping the
// last conMaxLines lines.
func (c *console) addLine(l conLine) {
	l.count = 1
	if n := len(c.lines); n > 0 {
		if p := &c.lines[n-1]; p.kind == l.kind && p.text == l.text && l.kind != script.Input {
			p.count++
			return
		}
	}
	c.lines = append(c.lines, l)
	// trimmed a tenth at a time, not a line at a time: copying the whole
	// transcript for every line printed would stall the UI
	if len(c.lines) > conMaxLines+conMaxLines/10 {
		drop := len(c.lines) - conMaxLines
		c.lines = append([]conLine(nil), c.lines[drop:]...)
		// the selection moves with its lines (or goes with them)
		c.selA.line -= drop
		c.selB.line -= drop
		if c.selA.line < 0 || c.selB.line < 0 {
			c.selA, c.selB = tPos{}, tPos{}
		}
	}
}

func (c *console) info(f string, v ...any) {
	c.addLine(conLine{kind: script.Info, text: fmt.Sprintf(f, v...)})
}

func (c *console) scriptName() string {
	if c.path != "" {
		return filepath.Base(c.path)
	}
	return "untitled.star"
}

// baseDir is where a script's relative paths point: its own folder, else
// the design's, else Documents.
func (a *App) baseDir(scriptPath string) string {
	switch {
	case scriptPath != "":
		return filepath.Dir(scriptPath)
	case a.ed.Path != "":
		return filepath.Dir(a.ed.Path)
	}
	if h, err := os.UserHomeDir(); err == nil {
		if d := filepath.Join(h, "Documents"); dirExists(d) {
			return d
		}
		return h
	}
	return ""
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// beginRun starts a run on the UI thread: one at a time, as one undo
// step.
func (a *App) beginRun(name string) bool {
	c := &a.con
	if c.running {
		return false
	}
	c.running, c.runStart, c.runName = true, time.Now(), name
	c.nodes0 = len(a.ed.Doc.Nodes)
	a.ed.Hist.Begin()
	a.w.Redraw()
	return true
}

// endRun finishes a run on the UI thread.
func (a *App) endRun(prompt bool, err error) {
	c := &a.con
	a.drainConsole()
	c.running = false
	a.ed.Hist.End()
	a.onChanged()
	secs := time.Since(c.runStart).Seconds()
	switch {
	case err != nil && strings.Contains(err.Error(), "computation cancelled: stopped"): // Stop, not a message that says "stopped"
		c.info("Stopped after %.1f s", secs)
	case err != nil:
	case !prompt:
		msg := fmt.Sprintf("Done in %.2f s", secs)
		if d := len(a.ed.Doc.Nodes) - c.nodes0; d != 0 {
			msg += fmt.Sprintf(" (%+d shapes)", d)
		}
		c.info("%s", msg)
	}
	if len(c.queued) > 0 {
		next := c.queued[0]
		c.queued = c.queued[1:]
		a.startPrompt(next)
	}
	a.w.Redraw()
}

// runPrompt runs what is typed at the prompt; while a script runs, the
// entry waits its turn, as typing ahead at a Python prompt does.
func (a *App) runPrompt() {
	c := &a.con
	src := strings.TrimRight(c.input.text, " \n")
	if strings.TrimSpace(src) == "" {
		return
	}
	if n := len(c.history); n == 0 || c.history[n-1] != src {
		c.history = append(c.history, src)
		if len(c.history) > 200 {
			c.history = c.history[1:]
		}
	}
	c.histPos = len(c.history)
	c.input.set("")
	c.scroll = 0
	if c.running {
		c.queued = append(c.queued, src)
		return
	}
	a.startPrompt(src)
}

func (a *App) startPrompt(src string) {
	c := &a.con
	if !a.beginRun("console") {
		c.queued = append([]string{src}, c.queued...)
		return
	}
	for i, l := range strings.Split(src, "\n") {
		p := ">>> "
		if i > 0 {
			p = "... "
		}
		c.addLine(conLine{kind: script.Input, text: p + l})
	}
	base := a.baseDir(c.path)
	go func() {
		_, err := c.sess.Prompt(src, base)
		a.post(func() { a.endRun(true, err) })
	}()
}

// runEditor runs the Editor tab's script.
func (a *App) runEditor() {
	c := &a.con
	name := c.scriptName()
	if !a.beginRun(name) {
		return
	}
	c.info("Running %s", name)
	c.scroll = 0
	src, base := c.editor.text, a.baseDir(c.path)
	go func() {
		_, err := c.sess.Run(name, src, base)
		a.post(func() { a.endRun(false, err) })
	}()
}

// runScriptFile is File → Run script…: open a script into the Editor tab
// and run it.
func (a *App) runScriptFile() {
	if !a.openScript() {
		return
	}
	a.runEditor()
}

// runScriptAPI is the API's run_script: it runs on the caller's
// connection, not the UI thread (the script's cad calls need the UI
// thread), and returns the output.
func (a *App) runScriptAPI(p args) (any, error) {
	c := &a.con
	name := p.s("name", "<api>")
	started := make(chan bool)
	a.post(func() { started <- a.beginRun(name) })
	if !<-started {
		return nil, script.ErrBusy
	}
	var out []script.Line
	c.mu.Lock()
	c.capture = &out
	c.mu.Unlock()
	_, err := c.sess.Run(name, p.s("code", ""), p.s("cwd", ""))
	c.mu.Lock()
	c.capture = nil
	c.mu.Unlock()
	done := make(chan bool)
	a.post(func() { a.endRun(false, err); done <- true })
	<-done
	var lines []string
	for _, l := range out {
		if l.Kind == script.Print || l.Kind == script.Result {
			lines = append(lines, l.Text)
		}
	}
	res := map[string]any{"output": strings.Join(lines, "\n"), "error": nil}
	if err != nil {
		var errs []string
		for _, l := range out {
			if l.Kind == script.Error {
				errs = append(errs, l.Text)
			}
		}
		res["error"] = strings.Join(errs, "\n")
	}
	return res, nil
}

func (a *App) stopScript() {
	a.con.queued = nil
	a.con.sess.Stop()
}

// --- the Editor tab's file ---------------------------------------------------------

const scriptFilter = "Starlark scripts (*.star)"

func (a *App) openScript() bool {
	c := &a.con
	if !a.keepScriptChanges() {
		return false
	}
	p, ok := a.openFile("Open script", scriptFilter, "*.star", "Python files (*.py)", "*.py", "All files", "*.*")
	if !ok {
		return false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		a.showToast(err.Error())
		return false
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	c.editor.set(text)
	c.editor.cur, c.editor.anchor, c.editor.top = 0, 0, 0
	c.path, c.saved = p, text
	c.open, c.script = true, true
	c.focus = c.editor
	return true
}

func (a *App) saveScript(as bool) bool {
	c := &a.con
	p := c.path
	if p == "" || as {
		var ok bool
		p, ok = a.saveFile("Save script", c.scriptName(), "star", scriptFilter, "*.star", "All files", "*.*")
		if !ok {
			return false
		}
	}
	if err := editor.WriteFileAtomic(p, []byte(c.editor.text)); err != nil {
		a.showToast(err.Error())
		return false
	}
	c.path, c.saved = p, c.editor.text
	a.showToast("Saved " + filepath.Base(p))
	return true
}

func (a *App) newScript() {
	c := &a.con
	if !a.keepScriptChanges() {
		return
	}
	c.editor.set("")
	c.path, c.saved = "", ""
	c.focus = c.editor
}

// keepScriptChanges asks before discarding an edited script; false
// cancels.
func (a *App) keepScriptChanges() bool {
	c := &a.con
	if c.editor.text == c.saved {
		return true
	}
	switch a.ask("Save changes to " + c.scriptName() + "?") {
	case win.IDCancel:
		return false
	case win.IDYes:
		return a.saveScript(false)
	}
	return true
}

// --- keys ----------------------------------------------------------------------------

// consoleKey handles a key while one of the console's editors has focus.
func (a *App) consoleKey(vk int, m win.Mods) bool {
	c := &a.con
	e := c.focus
	ctrl, shift := m&win.Ctrl != 0, m&win.Shift != 0
	a.w.Redraw()
	switch {
	case vk == vkEscape:
		c.focus = nil
		return true
	case vk == vkF5 || (ctrl && vk == vkReturn):
		if e == c.editor {
			a.runEditor()
		} else {
			a.runPrompt()
		}
		return true
	case ctrl && vk == 'C' && c.running && !e.hasSel():
		a.stopScript()
		return true
	case e == c.editor && ctrl && vk == 'S':
		a.saveScript(shift)
		return true
	case e == c.editor && ctrl && vk == 'O':
		a.openScript()
		return true
	case vk == vkReturn:
		if shift || e == c.editor {
			e.newline()
			return true
		}
		// at the prompt Enter runs a complete entry, as at a Python prompt
		if e.cur == len(e.text) && script.Complete(e.text) {
			a.runPrompt()
		} else if e.cur == len(e.text) && strings.TrimSpace(e.text) == "" {
			// nothing to run
		} else {
			e.newline()
		}
		return true
	case e == c.input && !shift && (vk == vkUp || vk == vkDown) && a.browseHistory(vk == vkUp):
		return true
	}
	page := 10
	if e == c.editor {
		page = max(1, int(c.editView.r.H/max(1, c.editView.lh))-1)
	}
	if e.key(vk, m, page, a.w.Clipboard, a.w.SetClipboard) {
		return true
	}
	return !passesThrough(vk, m)
}

// passesThrough reports whether a key the console doesn't use goes on to
// the app: Ctrl and Alt combinations and function keys do; anything else
// (a plain letter) stays with the console, so typing never triggers the
// design's one-key shortcuts.
func passesThrough(vk int, m win.Mods) bool {
	return m&(win.Ctrl|win.Alt) != 0 || (vk >= vkF1 && vk <= vkF1+11)
}

// browseHistory steps through earlier entries with Up and Down when the
// cursor is on the first (Up) or last (Down) line of the prompt.
func (a *App) browseHistory(up bool) bool {
	c := &a.con
	e := c.input
	line, _ := e.lineOf(e.cur)
	if (up && line != 0) || (!up && line != e.lineCount()-1) || len(c.history) == 0 {
		return false
	}
	if c.histPos == len(c.history) {
		c.draft = e.text
	}
	switch {
	case up && c.histPos > 0:
		c.histPos--
	case !up && c.histPos < len(c.history):
		c.histPos++
	default:
		return true
	}
	text := c.draft
	if c.histPos < len(c.history) {
		text = c.history[c.histPos]
	}
	e.set(text)
	return true
}

// --- drawing -------------------------------------------------------------------------

// consoleRect is where the panel goes: below the viewport.
func (a *App) consoleRect() render.Rect {
	return render.Rect{X: a.vp.X, Y: a.vp.Y + a.vp.H, W: a.vp.W, H: a.con.height}
}

var (
	colConPrint  = render.RGB(0x24, 0x29, 0x2e)
	colConResult = render.RGB(0x2f, 0x6f, 0xc6)
	colConError  = render.RGB(0xc6, 0x28, 0x28)
	colConInfo   = render.RGB(0x8a, 0x94, 0x9e)
	colConInput  = render.RGB(0x5b, 0x66, 0x72)
)

func (a *App) drawConsole() {
	c := &a.con
	if !c.open {
		return
	}
	a.drainConsole()
	b := a.batch
	r := a.consoleRect()
	b.Fill(r, colChrome)
	b.Fill(render.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, colBorder)
	// the top edge resizes the panel
	split := render.Rect{X: r.X, Y: r.Y - 3, W: r.W, H: 7}
	a.reg(region{r: split, id: "con-split", cursor: win.CursorSizeNS, press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
		y0, h0 := y, c.height
		return func(x, y float64, up bool) {
			c.height = clampConsole(h0+(y0-y), a.H-titleH-a.barH())
			a.w.Redraw()
		}
	}})
	a.drawConsoleHeader(render.Rect{X: r.X, Y: r.Y + 1, W: r.W, H: conHeaderH})
	body := render.Rect{X: r.X, Y: r.Y + conHeaderH + 1, W: r.W, H: r.H - conHeaderH - 1}
	b.Fill(render.Rect{X: body.X, Y: body.Y - 1, W: body.W, H: 1}, colBorder)
	if c.script {
		ew := float64(int(body.W * 0.58))
		er := render.Rect{X: body.X, Y: body.Y, W: ew, H: body.H}
		b.Fill(er, colCodeBG)
		c.editView = a.drawCode(c.editor, er.Inset(6), c.focus == c.editor, true, "")
		a.editorRegion("con-editor", er, c.editor, &c.editView)
		b.Fill(render.Rect{X: er.X + er.W, Y: body.Y, W: 1, H: body.H}, colBorder)
		a.drawTranscript(render.Rect{X: er.X + er.W + 1, Y: body.Y, W: body.W - ew - 1, H: body.H})
		return
	}
	// the prompt, growing with its text, under the transcript
	lh := b.MonoLineHeight(codeSize)
	n := min(conInputLines, c.input.lineCount())
	ih := float64(n)*lh + 14
	ir := render.Rect{X: body.X, Y: body.Y + body.H - ih, W: body.W, H: ih}
	a.drawTranscript(render.Rect{X: body.X, Y: body.Y, W: body.W, H: body.H - ih})
	b.Fill(ir, colCodeBG)
	b.Fill(render.Rect{X: ir.X, Y: ir.Y, W: ir.W, H: 1}, colBorder)
	prompt := ">>> "
	if c.running {
		prompt = "    "
	}
	c.inputView = a.drawCode(c.input, render.Rect{X: ir.X + 10, Y: ir.Y + 7, W: ir.W - 20, H: ir.H - 14}, c.focus == c.input, false, prompt)
	a.editorRegion("con-input", ir, c.input, &c.inputView)
}

func clampConsole(h, max float64) float64 {
	if h > max-120 {
		h = max - 120
	}
	if h < conMinH {
		h = conMinH
	}
	return h
}

// editorRegion makes an editor clickable (place the cursor, drag to
// select, Shift-click to extend) and scrollable.
func (a *App) editorRegion(id string, r render.Rect, e *codeEdit, v *codeView) {
	c := &a.con
	a.reg(region{r: r, id: id, cursor: win.CursorIBeam,
		press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
			c.focus, c.tfocus = e, false
			e.move(v.at(e, x, y), m&win.Shift != 0)
			e.wantCol = -1
			a.w.Redraw()
			return func(x, y float64, up bool) {
				e.move(v.at(e, x, y), true)
				a.w.Redraw()
			}
		},
		wheel: func(d float64) bool {
			e.top = max(0, min(e.lineCount()-1, e.top-int(d*3)))
			return true
		}})
}

func (a *App) drawConsoleHeader(r render.Rect) {
	c := &a.con
	b := a.batch
	// tabs
	x := r.X + 10
	for i, name := range []string{"Console", "Editor"} {
		isScript := i == 1
		label := name
		if isScript {
			label = "Editor: " + c.scriptName()
			if c.editor.text != c.saved {
				label += " •"
			}
		}
		tw := b.TextWidth(label, fontUI, false)
		tr := render.Rect{X: x, Y: r.Y, W: tw + 20, H: r.H}
		col := colText
		if c.script == isScript {
			col = colBlue
			b.Fill(render.Rect{X: tr.X + 6, Y: r.Y + r.H - 3, W: tr.W - 12, H: 2}, colBlue)
		} else if a.isHot("con-tab-" + name) {
			col = colIconHover
		}
		b.Text(label, tr.X+10, tr.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, col)
		a.button("con-tab-"+name, tr, "", "", 0, func() {
			c.script = isScript
			if isScript {
				c.focus = c.editor
			} else {
				c.focus = c.input
			}
		})
		x += tr.W + 4
	}
	// buttons, from the right
	type cbtn struct {
		id, label, tip string
		click          func()
		primary        bool
	}
	btns := []cbtn{{id: "con-close", tip: "Close the console", click: func() { c.open, c.focus = false, nil }}}
	btns = append(btns,
		cbtn{id: "con-savelog", label: "Save log", tip: "Save the transcript to a text file", click: a.saveLog},
		cbtn{id: "con-copy", label: "Copy", tip: "Copy the transcript", click: func() { a.w.SetClipboard(c.transcript()) }},
		cbtn{id: "con-clear", label: "Clear", tip: "Clear the transcript", click: func() { c.lines, c.scroll, c.selA, c.selB = nil, 0, tPos{}, tPos{} }},
	)
	if c.script {
		btns = append(btns,
			cbtn{id: "con-saveas", label: "Save as…", click: func() { a.saveScript(true) }},
			cbtn{id: "con-save", label: "Save", tip: "Ctrl + S", click: func() { a.saveScript(false) }},
			cbtn{id: "con-open", label: "Open…", tip: "Ctrl + O", click: func() { a.openScript() }},
			cbtn{id: "con-new", label: "New", click: a.newScript},
		)
	}
	if c.running {
		btns = append(btns, cbtn{id: "con-stop", label: "Stop", tip: "Stop the script (Ctrl + C)", click: a.stopScript, primary: true})
	} else if c.script {
		btns = append(btns, cbtn{id: "con-run", label: "Run", tip: "Run the script (F5 or Ctrl + Enter)", click: a.runEditor, primary: true})
	}
	bx := r.X + r.W - 8
	for _, t := range btns {
		w := b.TextWidth(t.label, fontUI, true) + 20
		switch {
		case t.id == "con-close":
			w = 28
		case t.primary:
			w += 16 // the play / stop mark
		}
		bx -= w
		br := render.Rect{X: bx, Y: r.Y + 5, W: w, H: r.H - 10}
		switch {
		case t.id == "con-close":
			col := colIcon
			if a.isHot(t.id) {
				b.RoundRect(br, 4, colHoverBG)
				col = colIconHover
			}
			drawIcon(b, icClose, render.Rect{X: br.CX() - 7, Y: br.CY() - 7, W: 14, H: 14}, col)
		case t.primary:
			bg := colBlue
			if t.id == "con-stop" {
				bg = colConError
			} else if a.isHot(t.id) {
				bg = colBlueDark
			}
			b.RoundRect(br, 4, bg)
			mx, my := br.X+14, br.CY()
			if t.id == "con-stop" {
				b.Fill(render.Rect{X: mx - 4, Y: my - 4, W: 8, H: 8}, colOnColor)
			} else {
				b.Tri(mx-4, my-5, mx-4, my+5, mx+5, my, colOnColor)
			}
			b.Text(t.label, br.X+24, br.CY()-b.LineHeight(fontUI, true)/2, fontUI, true, colOnColor)
		default:
			if a.isHot(t.id) {
				b.RoundRect(br, 4, colHoverBG)
			}
			col := colText
			if a.isHot(t.id) {
				col = colIconHover
			}
			b.TextCentered(t.label, br, fontUI, false, col)
		}
		a.button(t.id, br, t.tip, "", tipAbove, t.click)
		bx -= 4
	}
	if c.running {
		el := time.Since(c.runStart).Seconds()
		s := fmt.Sprintf("Running %s… %.0f s", c.runName, el)
		b.Text(s, x+12, r.CY()-b.LineHeight(fontSmall, false)/2, fontSmall, false, colTextDim)
		a.wakeAfter(time.Second)
	}
}

// transcript is the transcript as plain text.
func (c *console) transcript() string {
	var sb strings.Builder
	for _, l := range c.lines {
		sb.WriteString(l.text)
		if l.count > 1 {
			fmt.Fprintf(&sb, "  [×%d]", l.count)
		}
		sb.WriteString("\r\n")
	}
	return sb.String()
}

func (a *App) saveLog() {
	p, ok := a.saveFile("Save log", "scotticad-log.txt", "txt", "Text files (*.txt)", "*.txt", "All files", "*.*")
	if !ok {
		return
	}
	if err := editor.WriteFileAtomic(p, []byte(a.con.transcript())); err != nil {
		a.showToast(err.Error())
	}
}

// drawTranscript draws the transcript's rows bottom up, long lines
// wrapped. An error row from the Editor tab's script jumps to its line
// when clicked.
func (a *App) drawTranscript(r render.Rect) {
	c := &a.con
	b := a.batch
	in := render.Rect{X: r.X + 10, Y: r.Y + 6, W: r.W - 20, H: r.H - 12}
	cw, lh := b.MonoAdvance(codeSize), b.MonoLineHeight(codeSize)
	cols := max(10, int(in.W/cw))
	visible := max(1, int(in.H/lh))
	// rows from the bottom: wrap each line into pieces of cols characters
	type row struct {
		l        *conLine
		text     string
		last     bool // the line's last piece (where its repeat count goes)
		idx, off int  // the line's index, and this piece's first character
	}
	need := visible + c.scroll
	var rows []row // collected backwards
	total := 0
	for i := len(c.lines) - 1; i >= 0; i-- {
		l := &c.lines[i]
		rs := []rune(l.text)
		var pieces []string
		for len(rs) > cols {
			pieces = append(pieces, string(rs[:cols]))
			rs = rs[cols:]
		}
		pieces = append(pieces, string(rs))
		for j := len(pieces) - 1; j >= 0; j-- {
			if len(rows) < need {
				rows = append(rows, row{l, pieces[j], j == len(pieces)-1, i, j * cols})
			}
			total++
		}
		if len(rows) >= need && total > need+visible*4 {
			break // enough rows to draw and to judge the scroll range
		}
	}
	c.scroll = max(0, min(c.scroll, total-visible))
	// click and drag selects text (registered first: the error links on
	// top of it select too)
	a.reg(region{r: r, id: "con-transcript", cursor: win.CursorIBeam, press: a.transcriptPress(nil),
		wheel: func(d float64) bool {
			c.scroll = max(0, c.scroll+int(d*3))
			return true
		}})
	c.view = transcriptView{in: in, cw: cw, lh: lh}
	sa, sb := c.selRange()
	b.PushClip(r)
	for k := c.scroll; k < len(rows) && k < c.scroll+visible; k++ {
		rw := rows[k]
		y := in.Y + in.H - float64(k-c.scroll+1)*lh
		n := utf8.RuneCountInString(rw.text)
		c.view.rows = append([]tRow{{idx: rw.idx, off: rw.off, n: n, y: y}}, c.view.rows...)
		// the selection's part of this row
		if c.hasSel() && rw.idx >= sa.line && rw.idx <= sb.line {
			from, to := rw.off, rw.off+n
			if rw.idx == sa.line {
				from = max(from, sa.col)
			}
			if rw.idx == sb.line {
				to = min(to, sb.col)
			}
			if to > from || (rw.idx < sb.line && rw.last) {
				w := float64(to-from) * cw
				if rw.idx < sb.line && rw.last {
					w += cw / 2 // the selected line break
				}
				b.Fill(render.Rect{X: in.X + float64(from-rw.off)*cw, Y: y, W: w, H: lh}, colCodeSel)
			}
		}
		col := colConPrint
		switch rw.l.kind {
		case script.Result:
			col = colConResult
		case script.Error:
			col = colConError
		case script.Info:
			col = colConInfo
		case script.Input:
			col = colConInput
		}
		w := b.Mono(rw.text, in.X, y, codeSize, col)
		if rw.last && rw.l.count > 1 {
			b.Mono(fmt.Sprintf("  [×%d]", rw.l.count), in.X+w, y, codeSize, colConInfo)
		}
		if rw.l.kind == script.Error && rw.l.line > 0 && rw.l.file == c.scriptName() {
			// a click (not a drag) goes to the line
			id := fmt.Sprintf("con-err-%d", k)
			lr := render.Rect{X: in.X, Y: y, W: w, H: lh}
			line := rw.l.line
			if a.isHot(id) {
				b.Fill(render.Rect{X: lr.X, Y: lr.Y + lh - 1.5, W: lr.W, H: 1}, colConError)
			}
			a.reg(region{r: lr, id: id, cursor: win.CursorHand, tip: "Go to line " + fmt.Sprint(line), tipSide: tipAbove,
				press: a.transcriptPress(func() { a.gotoLine(line) })})
		}
	}
	b.PopClip()
}

// --- selecting in the transcript ----------------------------------------------------

// tPos is a place in the transcript: a line and a character in it.
type tPos struct{ line, col int }

func (p tPos) before(q tPos) bool { return p.line < q.line || (p.line == q.line && p.col < q.col) }

// tRow is a drawn row: a piece of line idx starting at character off.
type tRow struct {
	idx, off, n int
	y           float64
}

// transcriptView is where the transcript was last drawn, for mouse hits.
type transcriptView struct {
	in     render.Rect
	cw, lh float64
	rows   []tRow // top to bottom
}

func (c *console) hasSel() bool { return c.selA != c.selB }

func (c *console) selRange() (tPos, tPos) {
	if c.selB.before(c.selA) {
		return c.selB, c.selA
	}
	return c.selA, c.selB
}

// hit is the transcript position under (x, y).
func (c *console) hit(x, y float64) tPos {
	v := c.view
	if len(v.rows) == 0 {
		return tPos{}
	}
	if y < v.rows[0].y {
		r := v.rows[0]
		return tPos{r.idx, r.off}
	}
	for _, r := range v.rows {
		if y < r.y+v.lh {
			col := int((x - v.in.X + v.cw/2) / v.cw)
			return tPos{r.idx, r.off + max(0, min(r.n, col))}
		}
	}
	r := v.rows[len(v.rows)-1]
	return tPos{r.idx, r.off + r.n}
}

// transcriptPress starts a selection; click, if set, runs when the mouse
// is released without having dragged.
func (a *App) transcriptPress(click func()) func(x, y float64, m win.Mods) func(float64, float64, bool) {
	c := &a.con
	return func(x, y float64, m win.Mods) func(float64, float64, bool) {
		c.focus, c.tfocus = nil, true
		p := c.hit(x, y)
		if m&win.Shift == 0 {
			c.selA = p
		}
		c.selB = p
		a.w.Redraw()
		x0, y0 := x, y
		dragged := false
		return func(x, y float64, up bool) {
			if math.Abs(x-x0) > 3 || math.Abs(y-y0) > 3 {
				dragged = true
			}
			c.selB = c.hit(x, y)
			if up && !dragged && click != nil {
				c.selB = c.selA
				click()
			}
			a.w.Redraw()
		}
	}
}

// selectedText is the selected transcript text, lines joined with CRLF.
func (c *console) selectedText() string {
	a, b := c.selRange()
	var out []string
	for i := a.line; i <= b.line && i < len(c.lines); i++ {
		rs := []rune(c.lines[i].text)
		from, to := 0, len(rs)
		if i == a.line {
			from = min(a.col, len(rs))
		}
		if i == b.line {
			to = min(b.col, len(rs))
		}
		s := string(rs[from:max(from, to)])
		if c.lines[i].kind == script.Input {
			// what was typed, without the prompts, so it pastes as code
			s = strings.TrimPrefix(strings.TrimPrefix(s, ">>> "), "... ")
		}
		out = append(out, s)
	}
	return strings.Join(out, "\r\n")
}

// transcriptKey handles keys while the transcript has focus: Ctrl+C
// copies the selection, Ctrl+A selects everything, Esc clears.
func (a *App) transcriptKey(vk int, m win.Mods) bool {
	c := &a.con
	ctrl := m&win.Ctrl != 0
	switch {
	case ctrl && vk == 'C':
		if c.hasSel() {
			a.w.SetClipboard(c.selectedText())
		} else if c.running {
			a.stopScript()
		}
	case ctrl && vk == 'A':
		if n := len(c.lines); n > 0 {
			c.selA, c.selB = tPos{}, tPos{n - 1, utf8.RuneCountInString(c.lines[n-1].text)}
		}
	case vk == vkEscape:
		c.selA, c.selB, c.tfocus = tPos{}, tPos{}, false
	case passesThrough(vk, m):
		return false
	default:
		// typing goes to the prompt (or the Editor tab), as in a terminal
		c.tfocus = false
		c.focus = c.input
		if c.script {
			c.focus = c.editor
		}
		return a.consoleKey(vk, m)
	}
	a.w.Redraw()
	return true
}

// gotoLine shows a line of the Editor tab's script.
func (a *App) gotoLine(n int) {
	c := &a.con
	e := c.editor
	c.script = true
	c.focus = e
	i := e.lineStartAt(n - 1)
	e.move(i, false)
	_, s := e.lineOf(i)
	e.anchor, e.cur = s, e.lineEnd(s) // select the line
	a.w.Redraw()
}

// toggleConsole is the toolbar's Scripting button.
func (a *App) toggleConsole() {
	c := &a.con
	c.open = !c.open
	c.unread = false
	if c.open {
		if c.script {
			c.focus = c.editor
		} else {
			c.focus = c.input
		}
	} else {
		c.focus = nil
	}
	a.w.Redraw()
}

// consoleJSON is the console's state for the API.
func (a *App) consoleJSON() map[string]any {
	c := &a.con
	a.drainConsole()
	kinds := []string{"print", "result", "error", "info", "input"}
	var ls []any
	for _, l := range c.lines {
		m := map[string]any{"kind": kinds[l.kind], "text": l.text}
		if l.count > 1 {
			m["count"] = l.count
		}
		if l.line > 0 {
			m["file"], m["line"] = l.file, l.line
		}
		ls = append(ls, m)
	}
	tab := "console"
	if c.script {
		tab = "editor"
	}
	focus := ""
	switch c.focus {
	case c.input:
		focus = "input"
	case c.editor:
		focus = "editor"
	}
	selected := ""
	if c.hasSel() {
		selected = c.selectedText()
	}
	return map[string]any{"open": c.open, "tab": tab, "running": c.running, "lines": ls, "input": c.input.text, "selected": selected,
		"script": c.editor.text, "script_path": c.path, "focus": focus, "unread": c.unread}
}
