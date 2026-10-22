//go:build windows

package app

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/win"
)

// codeEdit is a multi-line code editor (the console's input and the
// Editor tab): a cursor and a selection, keyboard editing with
// Python-style indentation, its own undo, and drawing in the monospaced
// font with simple syntax colours.
type codeEdit struct {
	text        string
	cur, anchor int     // byte offsets; the selection lies between them
	top         int     // first visible line
	left        float64 // horizontal scroll, px
	undo, redo  []editState
	typing      bool // the last edit was typing (consecutive typing undoes as one)
	wantCol     int  // column kept while moving up and down; -1 if none
	// seen is the cursor offset last scrolled into view: the view follows
	// the cursor when it moves, and otherwise stays where the wheel put it
	seen int
}

type editState struct {
	text        string
	cur, anchor int
}

const (
	codeSize   = 12.5 // font size
	indentStep = 4
)

func newCodeEdit() *codeEdit { return &codeEdit{wantCol: -1, seen: -1} }

func (e *codeEdit) set(text string) {
	e.text, e.cur, e.anchor = text, len(text), len(text)
	e.undo, e.redo, e.typing, e.wantCol, e.seen = nil, nil, false, -1, -1
}

func (e *codeEdit) hasSel() bool { return e.cur != e.anchor }

func (e *codeEdit) sel() (int, int) {
	if e.cur < e.anchor {
		return e.cur, e.anchor
	}
	return e.anchor, e.cur
}

// lineOf returns the line index holding offset i and that line's start.
func (e *codeEdit) lineOf(i int) (line, start int) {
	line = strings.Count(e.text[:i], "\n")
	start = strings.LastIndexByte(e.text[:i], '\n') + 1
	return
}

func (e *codeEdit) lineEnd(start int) int {
	if j := strings.IndexByte(e.text[start:], '\n'); j >= 0 {
		return start + j
	}
	return len(e.text)
}

// lineStartAt returns the start offset of line n (clamped).
func (e *codeEdit) lineStartAt(n int) int {
	i := 0
	for ; n > 0; n-- {
		j := strings.IndexByte(e.text[i:], '\n')
		if j < 0 {
			return i
		}
		i += j + 1
	}
	return i
}

func (e *codeEdit) lineCount() int { return strings.Count(e.text, "\n") + 1 }

// col is the column (in characters) of offset i.
func (e *codeEdit) col(i int) int {
	_, s := e.lineOf(i)
	return utf8.RuneCountInString(e.text[s:i])
}

// atCol is the offset at column c of the line starting at start.
func (e *codeEdit) atCol(start, c int) int {
	end := e.lineEnd(start)
	i := start
	for n := 0; n < c && i < end; n++ {
		_, w := utf8.DecodeRuneInString(e.text[i:])
		i += w
	}
	return i
}

func (e *codeEdit) save(typing bool) {
	// consecutive typing undoes as one step, but typing over a selection
	// (after Ctrl+A, say) starts a new one: undo brings the selection back
	if typing && e.typing && !e.hasSel() {
		return
	}
	e.undo = append(e.undo, editState{e.text, e.cur, e.anchor})
	if len(e.undo) > 300 {
		e.undo = e.undo[1:]
	}
	e.redo = nil
	e.typing = typing
}

// replace puts s in place of the selection.
func (e *codeEdit) replace(s string, typing bool) {
	e.save(typing)
	a, b := e.sel()
	e.text = e.text[:a] + s + e.text[b:]
	e.cur = a + len(s)
	e.anchor = e.cur
	e.wantCol = -1
}

func (e *codeEdit) typeText(s string) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	s = strings.ReplaceAll(s, "\t", strings.Repeat(" ", indentStep))
	e.replace(s, len(s) == 1 && s != "\n")
}

// paste inserts s as one undo step of its own (even a single character,
// which typing would join to the typing around it).
func (e *codeEdit) paste(s string) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	s = strings.ReplaceAll(s, "\t", strings.Repeat(" ", indentStep))
	e.replace(s, false)
}

// newline breaks the line, keeping its indentation and indenting one
// more step after a line ending in ":".
func (e *codeEdit) newline() {
	_, s := e.lineOf(e.cur)
	line := e.text[s:e.cur]
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	if strings.HasSuffix(strings.TrimRight(line, " "), ":") {
		indent += strings.Repeat(" ", indentStep)
	}
	e.replace("\n"+indent, false)
}

func (e *codeEdit) move(to int, extend bool) {
	e.cur = max(0, min(len(e.text), to))
	if !extend {
		e.anchor = e.cur
	}
	e.typing = false
}

func (e *codeEdit) undoEdit() {
	if len(e.undo) == 0 {
		return
	}
	e.redo = append(e.redo, editState{e.text, e.cur, e.anchor})
	s := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.text, e.cur, e.anchor, e.typing = s.text, s.cur, s.anchor, false
}

func (e *codeEdit) redoEdit() {
	if len(e.redo) == 0 {
		return
	}
	e.undo = append(e.undo, editState{e.text, e.cur, e.anchor})
	s := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.text, e.cur, e.anchor, e.typing = s.text, s.cur, s.anchor, false
}

// indentLines indents (or, with out, dedents) every line the selection
// touches.
func (e *codeEdit) indentLines(out bool) {
	a, b := e.sel()
	_, s := e.lineOf(a)
	if b > a && b > 0 && e.text[b-1] == '\n' {
		b--
	}
	end := e.lineEnd(b)
	lines := strings.Split(e.text[s:end], "\n")
	for i, l := range lines {
		if out {
			n := 0
			for n < indentStep && n < len(l) && l[n] == ' ' {
				n++
			}
			lines[i] = l[n:]
		} else if strings.TrimSpace(l) != "" {
			lines[i] = strings.Repeat(" ", indentStep) + l
		}
	}
	e.save(false)
	repl := strings.Join(lines, "\n")
	e.text = e.text[:s] + repl + e.text[end:]
	e.anchor, e.cur = s, s+len(repl)
}

func isWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// wordLeft/wordRight find the next word boundary, as Ctrl+arrows do.
func (e *codeEdit) wordLeft(i int) int {
	for i > 0 {
		r, w := utf8.DecodeLastRuneInString(e.text[:i])
		if isWord(r) {
			break
		}
		i -= w
	}
	for i > 0 {
		r, w := utf8.DecodeLastRuneInString(e.text[:i])
		if !isWord(r) {
			break
		}
		i -= w
	}
	return i
}

func (e *codeEdit) wordRight(i int) int {
	for i < len(e.text) {
		r, w := utf8.DecodeRuneInString(e.text[i:])
		if isWord(r) {
			break
		}
		i += w
	}
	for i < len(e.text) {
		r, w := utf8.DecodeRuneInString(e.text[i:])
		if !isWord(r) {
			break
		}
		i += w
	}
	return i
}

// key handles editing keys; Enter is left to the caller (which decides
// between a new line and running). It reports whether it used the key.
func (e *codeEdit) key(vk int, m win.Mods, page int, clip func() string, setClip func(string)) bool {
	ctrl, shift := m&win.Ctrl != 0, m&win.Shift != 0
	upDown := func(d int) {
		line, _ := e.lineOf(e.cur)
		if e.wantCol < 0 {
			e.wantCol = e.col(e.cur)
		}
		want := e.wantCol
		target := max(0, min(e.lineCount()-1, line+d))
		if target == line {
			if d < 0 {
				e.move(0, shift)
			} else {
				e.move(len(e.text), shift)
			}
			e.wantCol = want
			return
		}
		e.move(e.atCol(e.lineStartAt(target), want), shift)
		e.wantCol = want
	}
	switch {
	case vk == vkLeft:
		e.wantCol = -1
		switch {
		case e.hasSel() && !shift:
			a, _ := e.sel()
			e.move(a, false)
		case ctrl:
			e.move(e.wordLeft(e.cur), shift)
		case e.cur > 0:
			_, w := utf8.DecodeLastRuneInString(e.text[:e.cur])
			e.move(e.cur-w, shift)
		}
	case vk == vkRight:
		e.wantCol = -1
		switch {
		case e.hasSel() && !shift:
			_, b := e.sel()
			e.move(b, false)
		case ctrl:
			e.move(e.wordRight(e.cur), shift)
		case e.cur < len(e.text):
			_, w := utf8.DecodeRuneInString(e.text[e.cur:])
			e.move(e.cur+w, shift)
		}
	case vk == vkUp:
		upDown(-1)
	case vk == vkDown:
		upDown(1)
	case vk == vkPgUp:
		upDown(-page)
	case vk == vkPgDn:
		upDown(page)
	case vk == vkHome:
		e.wantCol = -1
		if ctrl {
			e.move(0, shift)
			break
		}
		// first to the indentation, then to the line start
		_, s := e.lineOf(e.cur)
		ind := s + len(e.text[s:e.lineEnd(s)]) - len(strings.TrimLeft(e.text[s:e.lineEnd(s)], " \t"))
		if e.cur == ind {
			ind = s
		}
		e.move(ind, shift)
	case vk == vkEnd:
		e.wantCol = -1
		if ctrl {
			e.move(len(e.text), shift)
			break
		}
		_, s := e.lineOf(e.cur)
		e.move(e.lineEnd(s), shift)
	case vk == vkBack:
		if !e.hasSel() && e.cur > 0 {
			// in the indentation, back to the previous indent stop
			_, s := e.lineOf(e.cur)
			before := e.text[s:e.cur]
			if before != "" && strings.TrimLeft(before, " ") == "" {
				n := (len(before)-1)%indentStep + 1
				e.anchor = e.cur - n
			} else if ctrl {
				e.anchor = e.wordLeft(e.cur)
			} else {
				_, w := utf8.DecodeLastRuneInString(e.text[:e.cur])
				e.anchor = e.cur - w
			}
		}
		if e.hasSel() {
			e.replace("", false)
		}
	case vk == vkDelete:
		if !e.hasSel() && e.cur < len(e.text) {
			if ctrl {
				e.anchor = e.wordRight(e.cur)
			} else {
				_, w := utf8.DecodeRuneInString(e.text[e.cur:])
				e.anchor = e.cur + w
			}
		}
		if e.hasSel() {
			e.replace("", false)
		}
	case vk == vkTab:
		a, b := e.sel()
		switch {
		case shift:
			e.indentLines(true)
		case strings.Contains(e.text[a:b], "\n"):
			e.indentLines(false)
		default:
			// to the next indent stop from where the spaces go in (the
			// selection's start)
			n := indentStep - e.col(a)%indentStep
			e.replace(strings.Repeat(" ", n), false)
		}
	case ctrl && vk == 'A':
		e.anchor, e.cur = 0, len(e.text)
		e.typing, e.wantCol = false, -1
	case ctrl && (vk == 'C' || vk == 'X'):
		a, b := e.sel()
		text := ""
		if a == b { // no selection: the whole line, as code editors do
			_, s := e.lineOf(e.cur)
			a, b = s, min(len(e.text), e.lineEnd(s)+1)
			text = e.text[a:b]
			if !strings.HasSuffix(text, "\n") {
				text += "\n" // a line, to paste as one
				if vk == 'X' && a > 0 {
					a-- // the last line: the break before it goes, not leaving a blank line
				}
			}
		} else {
			text = e.text[a:b]
		}
		setClip(text)
		if vk == 'X' {
			e.anchor, e.cur = a, b
			e.replace("", false)
		}
	case ctrl && vk == 'V':
		e.paste(clip())
	case ctrl && vk == 'Z' && shift, ctrl && vk == 'Y':
		e.redoEdit()
	case ctrl && vk == 'Z':
		e.undoEdit()
	default:
		return false
	}
	return true
}

// --- drawing ----------------------------------------------------------------------

var (
	colCodeBG      = render.RGB(0xfb, 0xfc, 0xfd)
	colCodeText    = render.RGB(0x24, 0x29, 0x2e)
	colCodeKeyword = render.RGB(0xa6, 0x26, 0xa4)
	colCodeString  = render.RGB(0x50, 0x8a, 0x2e)
	colCodeNumber  = render.RGB(0xb7, 0x6b, 0x01)
	colCodeComment = render.RGB(0x8a, 0x94, 0x9e)
	colCodeName    = render.RGB(0x2f, 0x6f, 0xc6) // cad and its methods
	colCodeGutter  = render.RGB(0xa8, 0xb0, 0xb8)
)

var codeKeywords = map[string]bool{
	"and": true, "break": true, "continue": true, "def": true, "elif": true, "else": true, "for": true,
	"if": true, "in": true, "lambda": true, "load": true, "not": true, "or": true, "pass": true,
	"return": true, "while": true, "True": true, "False": true, "None": true,
}

// codeSpans splits one line into coloured runs (a line-by-line
// highlighter: strings don't continue across lines).
func codeSpans(line string) []struct {
	s string
	c render.Color
} {
	var out []struct {
		s string
		c render.Color
	}
	add := func(s string, c render.Color) {
		if s != "" {
			out = append(out, struct {
				s string
				c render.Color
			}{s, c})
		}
	}
	i := 0
	for i < len(line) {
		c := line[i]
		switch {
		case c == '#':
			add(line[i:], colCodeComment)
			return out
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(line) && line[j] != c {
				if line[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(line))
			add(line[i:j], colCodeString)
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(line) && (isWord(rune(line[j])) || line[j] == '.') {
				j++
			}
			add(line[i:j], colCodeNumber)
			i = j
		case isWord(rune(c)) || c >= 0x80:
			j := i
			for j < len(line) {
				r, w := utf8.DecodeRuneInString(line[j:])
				if !isWord(r) {
					break
				}
				j += w
			}
			if j == i {
				// a character that isn't part of a word (×, “, an emoji,
				// invalid UTF-8): plain text, one character
				_, w := utf8.DecodeRuneInString(line[i:])
				add(line[i:i+w], colCodeText)
				i += w
				continue
			}
			w := line[i:j]
			col := colCodeText
			switch {
			case codeKeywords[w]:
				col = colCodeKeyword
			case w == "cad" || (i > 0 && line[i-1] == '.' && strings.HasSuffix(line[:i-1], "cad") &&
				(i-1 == 3 || !isWord(rune(line[i-5])))):
				col = colCodeName
			}
			add(w, col)
			i = j
		default:
			j := i + 1
			for j < len(line) && !isWord(rune(line[j])) && line[j] != '#' && line[j] != '"' && line[j] != '\'' && line[j] < 0x80 {
				j++
			}
			add(line[i:j], colCodeText)
			i = j
		}
	}
	return out
}

// codeView is where an editor was last drawn, for mouse hits.
type codeView struct {
	r      render.Rect // the text area (after the gutter)
	cw, lh float64
}

// drawCode renders the editor into r; prompt, if set, is drawn before the
// first line (and "... " before the others). It returns where the text
// went, for mouse hits.
func (a *App) drawCode(e *codeEdit, r render.Rect, focused, gutter bool, prompt string) codeView {
	b := a.batch
	cw, lh := b.MonoAdvance(codeSize), math.Round(b.MonoLineHeight(codeSize))
	text := r
	gw := 0.0
	if gutter {
		gw = cw*float64(len(strconv.Itoa(e.lineCount()))+1) + 10
	} else if prompt != "" {
		gw = cw * float64(utf8.RuneCountInString(prompt))
	}
	text.X += gw
	text.W -= gw
	rows := max(1, int(r.H/lh))
	// keep the cursor in view when it moves (the wheel may scroll away
	// from it meanwhile)
	line, _ := e.lineOf(e.cur)
	cx := float64(e.col(e.cur)) * cw
	if e.cur != e.seen {
		e.seen = e.cur
		if line < e.top {
			e.top = line
		}
		if line >= e.top+rows {
			e.top = line - rows + 1
		}
		if cx-e.left > text.W-cw {
			e.left = cx - text.W + cw*4
		}
		if cx < e.left {
			e.left = max(0, cx-cw*4)
		}
	}
	e.top = max(0, min(e.top, e.lineCount()-1))
	sa, sb := e.sel()
	b.PushClip(r)
	lines := strings.Split(e.text, "\n")
	off := 0
	for i := 0; i < e.top && i < len(lines); i++ {
		off += len(lines[i]) + 1
	}
	for i := e.top; i < len(lines) && i < e.top+rows+1; i++ {
		y := r.Y + float64(i-e.top)*lh
		l := lines[i]
		if gutter {
			n := strconv.Itoa(i + 1)
			b.Mono(n, r.X+gw-10-cw*float64(len(n)), y, codeSize, colCodeGutter)
		} else if prompt != "" {
			p := prompt
			if i > 0 {
				p = "... "
			}
			b.Mono(p, r.X, y, codeSize, colCodeGutter)
		}
		// selection
		ls, le := off, off+len(l)
		if sa < sb && sb >= ls && sa <= le {
			x0 := float64(utf8.RuneCountInString(l[:max(0, sa-ls)])) * cw
			x1 := float64(utf8.RuneCountInString(l[:min(len(l), sb-ls)])) * cw
			if sb > le {
				x1 += cw * 0.5 // the selected line break
			}
			b.PushClip(text)
			b.Fill(render.Rect{X: text.X + x0 - e.left, Y: y, W: x1 - x0, H: lh}, colCodeSel)
			b.PopClip()
		}
		b.PushClip(text)
		x := text.X - e.left
		for _, sp := range codeSpans(l) {
			x += b.Mono(sp.s, x, y, codeSize, sp.c)
		}
		b.PopClip()
		off += len(l) + 1
	}
	if focused {
		if (time.Now().UnixMilli()/530)%2 == 0 {
			b.Fill(render.Rect{X: text.X + cx - e.left, Y: r.Y + float64(line-e.top)*lh + 1, W: 1.4, H: lh - 2}, colText)
		}
		a.blink = true
	}
	b.PopClip()
	return codeView{r: text, cw: cw, lh: lh}
}

// at returns the offset under (x, y) in a drawn editor.
func (v codeView) at(e *codeEdit, x, y float64) int {
	line := e.top + int((y-v.r.Y)/v.lh)
	line = max(0, min(e.lineCount()-1, line))
	c := int((x - v.r.X + e.left + v.cw/2) / v.cw)
	return e.atCol(e.lineStartAt(line), max(0, c))
}
