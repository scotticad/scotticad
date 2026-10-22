//go:build windows

// Package app is ScottiCAD's editor window: the layout, viewport
// interaction and dialogs on top of pkg/editor (all edits) and pkg/render.
package app

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/gl"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/win"
)

// virtual key codes
const (
	vkBack   = 0x08
	vkTab    = 0x09
	vkReturn = 0x0D
	vkEscape = 0x1B
	vkPgUp   = 0x21
	vkPgDn   = 0x22
	vkEnd    = 0x23
	vkHome   = 0x24
	vkLeft   = 0x25
	vkUp     = 0x26
	vkRight  = 0x27
	vkDown   = 0x28
	vkDelete = 0x2E
	vkF1     = 0x70
	vkF5     = 0x74
	vkPlus   = 0xBB
	vkMinus  = 0xBD
	vkAdd    = 0x6B
	vkSub    = 0x6D
)

// tool modes
type toolMode int

const (
	modeNone toolMode = iota
	modeVisibility
	modeMeasure
	modeRulerPlace
	modeAlign
	modeMirror
	modePlace     // a tile was clicked; next viewport click places it
	modeNotes     // the Notes tool: click a shape to pin a note
	modeWorkplane // the Workplane tool: click a shape to put the workplane on it
	modeSplit     // the Split tool: cut the selection with a plane
	modeLayFlat   // the Lay flat tool: click a face to put it on the workplane
)

// App is the editor window.
type App struct {
	w      *win.Window
	ed     *editor.Editor
	scene  *render.Scene
	batch  *render.Batch
	target *render.Target
	cam    render.Camera
	anim   *camAnim

	W, H   float64     // window size in logical pixels
	vp     render.Rect // 3D viewport
	mx, my float64
	mods   win.Mods

	regions, prevRegions []region
	hot, pressed         string
	pressRegion          *region
	pressDrag            func(x, y float64, up bool)
	hoverSince           time.Time
	wakePending          atomic.Bool
	blink                bool

	field *textField
	popup popupKind

	mode   toolMode
	cruise bool // Cruise tool active
	drag   *dragState
	// viewport hover
	hoverNode   int
	hoverHandle int // index into handles, -1 none
	handles     []handle
	alignHover  *alignDot
	alignKey    int // key object for align (0 = combined bounds)
	mirrorHover int // axis+1, 0 none
	labels      []dimLabel

	place      *placeState // tile being dragged or click-placed
	measure    measureState
	thumbs     map[string]render.Sprite
	thumbsDone bool
	thumbGen   int     // the atlas generation the tiles were drawn into
	thumbDPI   float64 // and the scale they were drawn at
	// pendingOpen opens a field on the next frame (Tab to the next label)
	pendingOpen func()
	dragBtn     int // the mouse button that started the current drag

	toast      string
	toastUntil time.Time

	inspectorOpen bool
	propsOpen     bool
	// inspScroll is how far the inspector's body is scrolled up when the
	// window is too short to show it all
	inspScroll  float64
	panelOpen   bool
	panelScroll float64
	library     string
	// shape search: the panel shows matching tiles from every library
	searchOn     bool
	searchQuery  string
	colorTab     int
	colorMode    int // custom colour sliders: 0 RGB, 1 HSB
	pickHex      string
	pickH, pickS float64
	pickV        float64
	// pickTarget, when set, takes the picker's colour (live: mid-drag)
	// instead of the selected shapes: the Settings background colour
	pickTarget func(hex string, live bool)
	bgPicker   bool     // the background colour picker is open in Settings
	recent     []string // recent designs, newest first (see start.go)
	// openDrop is the dialog dropdown whose list is open; dropList draws
	// that list once the rest of the dialog is drawn
	openDrop string
	dropList func()
	// notesHidden hides every note from view (Shift+N): the view's, not
	// the design's, like the grid
	notesHidden   bool
	settingsDraft *doc.Settings
	settingsBase  doc.Settings   // the settings when the dialog opened
	answers       []dialogAnswer // queued answers to prompts and file dialogs (see prompt.go)
	importOpts    editor.ImportOptions
	importPath    string
	exportSel     bool
	// the last rotation, kept on screen after the mouse is released so its
	// angle can be typed (see keptRotation)
	rotKeep *keptRot
	// sizes pinned by clicking a handle, and the distance bars left by a
	// move, both shown until the selection or tool changes
	dimPin   *dimPin
	moveKeep *keptMove
	noteOpen *noteID // the note whose text box is showing
	// loading names the design being opened while its shapes are still
	// being combined in the background (for the "Loading…" banner)
	loading string

	bgColor    render.Color
	fontMenuAt render.Rect
	fileMenuX  float64 // where the File button is drawn
	menuX      float64 // left edge of the open File menu
	fontScroll float64
	titleDirty bool
	zoomSpeed  float64

	keysDown map[int]bool

	// the Scripting console
	con console
	// the Split tool's plane and options
	split splitState
	// Section view (on or off, independent of the tool mode)
	section sectionState
	// the face the Lay flat tool would lay down
	layHover *layFace
	// the filament estimate's settings and last result
	fil filamentState
	// rev counts changes to the design (for caches)
	rev int
	// tbRows is the toolbar's rows: 2 only on a very narrow window
	tbRows int
	// leftInTitle: the clipboard and undo buttons are in the title bar (a
	// narrow window)
	leftInTitle bool

	// API
	calls    chan apiCall
	headless bool
	frames   int

	// cached on-plane text
	watermark    []float32
	watermarkKey string
	cubeLabels   []float32
}

// Options configure New.
type Options struct {
	File     string
	Headless bool
	Width    int
	Height   int
	Scale    float64 // interface scale; 0: the display's
}

// New creates the window and the editor.
func New(o Options) (*App, error) {
	if o.Width == 0 {
		o.Width, o.Height = 1600, 1000
	}
	a := &App{
		ed:            editor.New(),
		thumbs:        map[string]render.Sprite{},
		inspectorOpen: true,
		propsOpen:     true,
		panelOpen:     true,
		library:       "Basic Shapes",
		hoverHandle:   -1,
		bgColor:       colBG,
		zoomSpeed:     1,
		keysDown:      map[int]bool{},
		calls:         make(chan apiCall, 64),
		headless:      o.Headless,
		importOpts:    editor.DefaultImportOptions(),
	}
	w, err := win.New("ScottiCAD", o.Width, o.Height, !o.Headless, a, o.Scale)
	if err != nil {
		return nil, err
	}
	a.w = w
	w.IgnoreInput = o.Headless
	if !o.Headless {
		w.OnPanic = a.emergencySave // a crash saves the design first
	}
	if err := gl.Init(); err != nil {
		return nil, err
	}
	gl.SwapInterval(1)
	if a.scene, err = render.NewScene(); err != nil {
		return nil, err
	}
	if a.batch, err = render.NewBatch(); err != nil {
		return nil, err
	}
	samples := int32(8)
	if max := gl.GetInteger(gl.MAX_SAMPLES); max < samples {
		samples = max
	}
	a.target = render.NewTarget(samples)
	a.ed.Changed = a.onChanged
	a.ed.Replaced = a.designReplaced
	// no narrower than the toolbar's tools beside the shape panel (the
	// clipboard and undo buttons go up to the title bar first)
	w.MinW, w.MinH = int(math.Ceil(toolsWidth(a.rightTools())+panelW+24)), 480
	// big booleans are computed in the background (previews show
	// meanwhile); redraw when each finishes
	doc.SetAsync(true)
	doc.OnReady = func() {
		a.wakePending.Store(true)
		a.w.Wake()
	}
	a.ed.SetClipboard = w.SetClipboard
	a.ed.GetClipboard = w.Clipboard
	a.initConsole()
	a.loadFilament()
	a.loadTheme()
	a.loadRecent()
	a.homeCamera(false)
	if o.File != "" {
		if err := a.ed.Open(o.File); err != nil {
			a.showToast(err.Error())
		} else {
			a.loading = a.designName()
			a.restoreCamera()
			a.noteRecent(o.File)
		}
	} else if !o.Headless {
		a.popup = popStart // started on its own: the start screen
	}
	a.updateTitle()
	return a, nil
}

// Run enters the message loop.
func (a *App) Run() { a.w.Run() }

func (a *App) onChanged() {
	a.rev++
	a.titleDirty = true
	a.w.Redraw()
}

// designName is what the design is called (see editor.Name), as shown.
func (a *App) designName() string {
	if n := a.ed.Name(); n != "" {
		return n
	}
	return "Unsaved new design"
}

func (a *App) updateTitle() {
	name := a.designName()
	if a.ed.Dirty() {
		name = "• " + name
	}
	a.w.SetTitle(name + " — ScottiCAD")
}

func (a *App) showToast(s string) {
	a.toast = s
	a.toastUntil = time.Now().Add(3 * time.Second)
	a.wakeAfter(3 * time.Second)
	a.w.Redraw()
}

func (a *App) units() string { return a.ed.Doc.Settings.Units }

func (a *App) fmtLen(mm float64) string { return editor.Format(mm, a.units(), false) }

// --- win.Handler ------------------------------------------------------------

func (a *App) Resize(w, h int) {
	if a.w != nil {
		a.w.Redraw()
	}
}

func (a *App) Close() bool {
	if a.ed.Dirty() {
		switch a.ask("Save changes to " + a.designName() + "?") {
		case win.IDCancel:
			return false
		case win.IDYes:
			if !a.save(false) {
				return false
			}
		}
	}
	return true
}

func (a *App) Idle() {
	for {
		select {
		case c := <-a.calls:
			if c.do != nil {
				c.do()
				a.w.Redraw()
			} else {
				a.runCall(c)
			}
			continue
		default:
		}
		break
	}
	if a.wakePending.Swap(false) {
		a.drainConsole()
		a.w.Redraw()
	}
	if a.blink && a.field != nil {
		a.w.Animate = true
	} else if a.anim == nil {
		a.w.Animate = false
	}
}

func (a *App) MouseMove(x, y float64, m win.Mods) {
	x, y = x/a.w.DPI, y/a.w.DPI
	a.mx, a.my, a.mods = x, y, m
	if a.pressDrag != nil {
		a.pressDrag(x, y, false)
		a.w.Redraw()
		return
	}
	if a.drag != nil {
		a.dragMove(x, y, m)
		a.w.Redraw()
		return
	}
	a.updateHot()
	if a.hot == "" || a.hot == "viewport" {
		a.viewHover(x, y)
	} else if a.hoverNode != 0 || a.hoverHandle >= 0 {
		a.hoverNode, a.hoverHandle = 0, -1
		a.w.Redraw()
	}
	if a.place != nil || a.mode == modeMeasure || a.mode == modeRulerPlace || a.mode == modeNotes {
		a.w.Redraw()
	}
}

func (a *App) MouseLeave() {
	if a.pressDrag == nil && a.drag == nil {
		a.hot = ""
		a.hoverNode, a.hoverHandle = 0, -1
		a.w.Redraw()
	}
}

func (a *App) MouseDown(btn int, x, y float64, m win.Mods, double bool) {
	x, y = x/a.w.DPI, y/a.w.DPI
	a.mx, a.my, a.mods = x, y, m
	a.w.Capture(true)
	a.w.Redraw()
	if a.drag != nil || a.pressDrag != nil {
		return
	}
	a.dragBtn = btn // only this button's release ends what it starts
	if btn != win.Left {
		if a.vp.Contains(x, y) && a.popup == popNone {
			a.commitField()
			a.startCameraDrag(btn, x, y, m)
		}
		return
	}
	r := a.regionAt(x, y)
	if r != nil && r.passThrough {
		r.press(x, y, m)
		top := 0
		for i := range a.prevRegions {
			if &a.prevRegions[i] == r {
				top = i
			}
		}
		r = a.regionBelow(x, y, top)
	}
	if a.field != nil && (r == nil || r.id != a.field.id) {
		if !a.commitField() {
			return
		}
	}
	if (a.con.focus != nil || a.con.tfocus) && (r == nil || !strings.HasPrefix(r.id, "con-")) && (r == nil || r.id != "scripting") {
		a.con.focus, a.con.tfocus = nil, false // clicking elsewhere leaves the console
	}
	if r == nil {
		return
	}
	a.pressed = r.id
	rc := *r
	a.pressRegion = &rc
	if r.press != nil {
		a.pressDrag = r.press(x, y, m)
	}
}

func (a *App) MouseUp(btn int, x, y float64, m win.Mods) {
	x, y = x/a.w.DPI, y/a.w.DPI
	a.mx, a.my, a.mods = x, y, m
	if (a.drag != nil || a.pressDrag != nil) && btn != a.dragBtn {
		return // another button, pressed and released during the drag
	}
	a.w.Capture(false)
	a.w.Redraw()
	if a.drag != nil {
		a.dragEnd(x, y, m)
		a.pressed, a.pressRegion, a.pressDrag = "", nil, nil
		a.updateHot()
		return
	}
	if a.pressDrag != nil {
		d := a.pressDrag
		a.pressDrag = nil
		d(x, y, true)
	}
	if a.pressed != "" && a.pressRegion != nil && a.pressRegion.click != nil {
		if r := a.regionAt(x, y); r != nil && r.id == a.pressed {
			a.pressRegion.click()
		}
	}
	a.pressed, a.pressRegion = "", nil
	a.updateHot()
}

// CaptureLost ends a drag whose button-up won't come (the mouse was taken
// by another window): a viewport drag is cancelled, a control's drag
// (a slider, the colour square) finishes where it is.
func (a *App) CaptureLost() {
	if a.drag != nil {
		a.dragCancel()
	}
	if d := a.pressDrag; d != nil {
		a.pressDrag = nil
		d(a.mx, a.my, true)
	}
	a.pressed, a.pressRegion = "", nil
	a.w.Redraw()
}

func (a *App) MouseWheel(delta, x, y float64, m win.Mods) {
	x, y = x/a.w.DPI, y/a.w.DPI
	// the topmost scrollable region under the cursor gets the wheel;
	// nothing below an open popup's backdrop scrolls
	for i := len(a.prevRegions) - 1; i >= 0; i-- {
		r := &a.prevRegions[i]
		if !r.r.Contains(x, y) {
			continue
		}
		if r.wheel != nil {
			if r.wheel(delta) {
				a.w.Redraw()
				return
			}
			break
		}
		if r.id == "catcher" {
			return
		}
	}
	if a.vp.Contains(x, y) && a.popup == popNone {
		f := math.Pow(0.87, delta*a.zoomSpeed)
		a.zoomAt(x, y, f)
	}
}

func (a *App) DropFiles(paths []string, x, y float64) {
	for _, p := range paths {
		a.importFile(p)
	}
}

func (a *App) Char(r rune) {
	if a.fieldChar(r) {
		return
	}
	if a.con.tfocus && a.con.focus == nil && a.popup == popNone && !a.con.script {
		a.con.focus, a.con.tfocus = a.con.input, false // typing goes to the prompt
	}
	if a.con.focus != nil && a.popup == popNone {
		if r >= 32 && r != 127 {
			a.con.focus.typeText(string(r))
			a.w.Redraw()
		}
		return
	}
	if r == '?' {
		a.openPopup(popHelp)
	}
}

func (a *App) KeyUp(vk int, m win.Mods) {
	delete(a.keysDown, vk)
	a.mods = m
	if a.drag != nil {
		a.dragMove(a.mx, a.my, m)
	}
	a.w.Redraw()
}

func (a *App) KeyDown(vk int, m win.Mods, repeat bool) {
	a.keysDown[vk] = true
	a.mods = m
	if a.field != nil {
		a.fieldKey(vk, m)
		return
	}
	if a.pressDrag != nil {
		// a slider or colour drag is under way: Ctrl+Z now would end its
		// edit and the rest of the drag would change the design outside
		// any undo step
		return
	}
	if a.con.focus != nil && a.drag == nil && a.popup == popNone && a.consoleKey(vk, m) {
		return
	}
	if a.con.tfocus && a.drag == nil && a.popup == popNone && a.transcriptKey(vk, m) {
		return
	}
	if a.drag != nil {
		if vk == vkEscape {
			a.dragCancel()
		} else {
			a.dragMove(a.mx, a.my, m)
		}
		a.w.Redraw()
		return
	}
	ctrl, shift := m&win.Ctrl != 0, m&win.Shift != 0
	if m&win.Alt != 0 && !ctrl && vk == 'F' {
		a.openFileMenu(a.fileMenuX)
		return
	}
	if a.openDrop != "" && vk == vkEscape { // a dropdown's list, not its dialog
		a.openDrop = ""
		a.w.Redraw()
		return
	}
	if a.popup != popNone && vk == vkEscape {
		a.closePopup()
		return
	}
	if a.popup == popSettings || a.popup == popImport || a.popup == popExport || a.popup == popStart {
		return
	}
	a.w.Redraw()
	ed := a.ed
	switch {
	case a.mode == modeSplit && vk == vkReturn:
		a.applySplit()
	case a.mode == modeSplit && !ctrl && (vk == 'X' || vk == 'Y' || vk == 'Z'):
		a.split.axis = vk - 'X'
		a.centreSplit()
	case ctrl && vk == 'Z' && shift, ctrl && vk == 'Y':
		a.redo()
	case ctrl && vk == 'Z':
		a.undo()
	case ctrl && vk == 'C':
		ed.Copy()
	case ctrl && vk == 'X':
		ed.Cut()
	case ctrl && vk == 'V':
		a.paste(shift)
	case ctrl && vk == 'D':
		ed.Duplicate()
	case ctrl && vk == 'A':
		ed.SelectAll()
	case ctrl && vk == 'G' && shift:
		ed.Ungroup()
	case ctrl && vk == 'G':
		a.group(doc.ModeUnion)
	case ctrl && vk == 'B':
		a.group(doc.ModeBundle)
	case ctrl && vk == 'H' && shift:
		ed.ShowAll()
	case ctrl && vk == 'H':
		ed.HideSelected()
	case ctrl && vk == 'L':
		a.toggleLock()
	case ctrl && vk == 'T':
		a.toggleTransparent()
	case ctrl && vk == 'S':
		a.save(shift)
	case ctrl && vk == 'O':
		a.open()
	case ctrl && vk == 'N':
		a.newDesign()
	case ctrl && vk == 'E':
		a.exportSel = len(ed.Sel) > 0
		a.openPopup(popExport)
	case ctrl && vk == 'I':
		a.openPopup(popImport)
	case ctrl && (vk == vkUp || vk == vkDown):
		s := 1.0
		if vk == vkDown {
			s = -1
		}
		if shift {
			s *= 10
		}
		ed.Nudge(2, s)
	case vk == vkLeft || vk == vkRight || vk == vkUp || vk == vkDown:
		a.nudgeView(vk, shift)
	case vk == vkDelete || vk == vkBack:
		ed.Delete(nil)
	case vk == vkEscape:
		a.escape()
	case ctrl:
		return
	case vk == 'H' && !shift:
		ed.SetHole(true)
	case vk == 'S' && !shift:
		a.makeSolid()
	case vk == 'D' && shift:
		a.setMode(modeLayFlat)
	case vk == 'D':
		ed.Drop(nil)
	case vk == 'F':
		a.fitView()
	case vk == 'L':
		a.setMode(modeAlign)
	case vk == 'M':
		a.setMode(modeMirror)
	case vk == 'C' && !repeat:
		if a.place == nil {
			a.cruise = !a.cruise
		}
	case vk == 'V':
		a.setMode(modeVisibility)
	case vk == 'R':
		a.setMode(modeRulerPlace)
	case vk == 'N' && shift:
		a.toggleNotes()
	case vk == 'N':
		a.setMode(modeNotes)
	case vk == 'W':
		a.setMode(modeWorkplane)
	case vk == 'K' && shift:
		a.setSection(!a.section.on)
	case vk == 'K':
		a.setMode(modeSplit)
	case vk == vkPlus || vk == vkAdd:
		a.zoomAt(a.vp.CX(), a.vp.CY(), 0.8)
	case vk == vkMinus || vk == vkSub:
		a.zoomAt(a.vp.CX(), a.vp.CY(), 1.25)
	case vk == vkF1:
		a.openPopup(popHelp)
	}
}

// nudgeView moves the selection along the world axis closest to the
// arrow's screen direction, so nudging follows the view.
func (a *App) nudgeView(vk int, shift bool) {
	if len(a.ed.Sel) == 0 {
		return
	}
	dx, dy := 0.0, 0.0
	switch vk {
	case vkLeft:
		dx = -1
	case vkRight:
		dx = 1
	case vkUp:
		dy = 1
	case vkDown:
		dy = -1
	}
	// camera right/forward projected on XY
	yaw := a.cam.Yaw
	right := geom.V(-math.Sin(yaw), math.Cos(yaw), 0)
	fwd := geom.V(-math.Cos(yaw), -math.Sin(yaw), 0)
	// in the workplane's frame
	if inv, ok := a.ed.Work().Inverse(); ok {
		right, fwd = inv.Dir(right), inv.Dir(fwd)
	}
	dir := right.Mul(dx).Add(fwd.Mul(dy))
	axis, sign := 0, 1.0
	if math.Abs(dir.Y) > math.Abs(dir.X) {
		axis = 1
		if dir.Y < 0 {
			sign = -1
		}
	} else if dir.X < 0 {
		sign = -1
	}
	if shift {
		sign *= 10
	}
	a.ed.Nudge(axis, sign)
}

// openFileMenu toggles the File menu, dropping down from x.
func (a *App) openFileMenu(x float64) {
	a.menuX = x
	a.openPopup(popDesignMenu)
}

func (a *App) escape() {
	switch {
	case a.place != nil:
		a.place = nil
	case a.noteOpen != nil:
		a.closeNote()
	case a.mode != modeNone:
		a.setMode(modeNone)
	case a.section.on:
		a.setSection(false)
	case a.fil.shown:
		a.toggleFilament()
	case a.cruise:
		a.cruise = false
	case a.ed.Workplane != nil:
		a.ed.AttachWorkplane(0)
	default:
		a.ed.ClearSelection()
	}
}

func (a *App) setMode(m toolMode) {
	if a.mode == m {
		m = modeNone
	}
	if (m == modeAlign || m == modeMirror || m == modeSplit) && len(a.ed.Editable()) == 0 {
		return
	}
	a.mode = m
	if m == modeNotes {
		a.notesHidden = false // a note added unseen would be lost to view
	}
	a.place = nil // a tile armed before the switch would take the tool's next click
	a.rotKeep, a.dimPin, a.moveKeep = nil, nil, nil
	a.measure = measureState{}
	a.alignHover = nil
	a.alignKey = 0
	a.mirrorHover = 0
	a.layHover = nil
	if m == modeSplit {
		a.initSplit()
	}
	a.w.Redraw()
}

// designReplaced drops everything held for a design that New or Open (from
// a menu, the API or a script) has just replaced: a field being typed in
// would commit into whatever shape now has its id, a drag or kept overlay
// points at nodes no longer in the design, a pinned section cuts by old ids.
func (a *App) designReplaced() {
	a.field, a.pendingOpen = nil, nil
	a.drag, a.pressDrag, a.pressed, a.pressRegion = nil, nil, "", nil
	a.noteOpen = nil
	a.section = sectionState{}
	a.mode = modeNone
	a.setMode(modeNone) // clears the tools' state
	a.closePopup()
}

// --- commands shared by keys, buttons and the API ------------------------------

// undo and redo replace every node with a copy from the history, so a
// field still open (which edits a node) is committed first.
func (a *App) undo() bool {
	a.place = nil
	a.finishPress()
	return a.ed.Undo()
}

func (a *App) redo() bool {
	a.finishPress()
	return a.ed.Redo()
}

// finishPress ends what is being typed or dragged in a control (a field,
// a slider, the colour square) where it is, as its own step, before an
// undo or redo from elsewhere (the API, a script) replaces the design
// under it. A drag on the canvas that edits shapes ends the same way: it
// holds the nodes it moves, and cancelling an Alt-drag would otherwise
// undo whatever step came before it.
func (a *App) finishPress() {
	a.commitField()
	if d := a.pressDrag; d != nil {
		a.pressDrag = nil
		d(a.mx, a.my, true)
	}
	if d := a.drag; d != nil {
		switch d.kind {
		case dragOrbit, dragPan, dragCube, dragMarquee:
			// the view or a selection rectangle: carries on
		case dragTile:
			a.drag, a.place = nil, nil
			a.w.Capture(false)
		default:
			a.dragEnd(a.mx, a.my, a.mods)
			a.w.Capture(false)
		}
	}
}

func (a *App) paste(inPlace bool) {
	if _, err := a.ed.Paste(inPlace); err != nil {
		a.showToast(err.Error())
	}
}

func (a *App) group(mode string) {
	if _, err := a.ed.Group(mode); err != nil {
		a.showToast(err.Error())
	}
}

func (a *App) makeSolid() {
	ns := a.ed.Editable()
	a.ed.SetHole(false)
	// a hole turned solid gets its kind's default colour back
	for _, n := range ns {
		if n.Color == editor.HoleColor && !n.IsGroup() {
			if k := kindColor(n.Kind); k != "" {
				doc.SetColor(n, k)
			}
		}
	}
}

func (a *App) toggleLock() {
	all := true
	for _, n := range a.ed.Selected() {
		all = all && n.Locked
	}
	a.ed.SetLocked(!all)
}

func (a *App) toggleTransparent() {
	all := true
	for _, n := range a.ed.Editable() {
		all = all && n.Transparent
	}
	a.ed.SetTransparent(!all)
}

func (a *App) newDesign() {
	if !a.saveChanges() {
		return
	}
	a.ed.NewDesign()
	a.homeCamera(false)
}

func (a *App) open() {
	if !a.saveChanges() {
		return
	}
	p, ok := a.openFile("Open design", "ScottiCAD designs (*.scotticad)", "*.scotticad", "All files", "*.*")
	if !ok {
		return
	}
	a.openPath(p)
}

// save writes the design; saveAs or an unnamed design asks for a path.
func (a *App) save(saveAs bool) bool {
	a.storeCamera()
	p := a.ed.Path
	if p == "" || saveAs {
		name := sanitize(a.ed.Name()) // "design" when unsaved
		var ok bool
		p, ok = a.saveFile("Save design", name+".scotticad", "scotticad", "ScottiCAD designs (*.scotticad)", "*.scotticad")
		if !ok {
			return false
		}
	}
	if err := a.ed.Save(p); err != nil {
		a.showToast(err.Error())
		return false
	}
	a.noteRecent(a.ed.Path)
	a.showToast("Saved " + filepath.Base(a.ed.Path))
	return true
}

func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, s)
	if s == "" {
		s = "design"
	}
	return s
}

func (a *App) storeCamera() {
	c := a.cam
	a.ed.Doc.Camera = &doc.Camera{Target: c.Target, Yaw: c.Yaw, Pitch: c.Pitch, Dist: c.Dist, Ortho: c.Ortho}
}

func (a *App) restoreCamera() {
	if c := a.ed.Doc.Camera; c != nil && c.Dist > 0 {
		a.cam.Target, a.cam.Yaw, a.cam.Pitch, a.cam.Dist, a.cam.Ortho = c.Target, c.Yaw, c.Pitch, c.Dist, c.Ortho
		a.anim = nil
	} else {
		a.homeCamera(false)
	}
}

func (a *App) importFile(p string) {
	// where the view's centre meets the workplane
	pos := geom.Vec3{}
	if a.vp.W > 0 {
		if hit, ok := a.planeHitW(a.vp.CX(), a.vp.CY(), 0); ok {
			pos = editorSnapXY(hit, a.ed.Doc.Settings.Snap)
		}
	}
	if _, err := a.ed.ImportOn(p, pos, a.importOpts); err != nil {
		a.showToast(err.Error())
		return
	}
	a.showToast("Imported " + filepath.Base(p))
}

func editorSnapXY(p geom.Vec3, step float64) geom.Vec3 {
	return geom.V(editor.Snap(p.X, step), editor.Snap(p.Y, step), p.Z)
}

func (a *App) export(format string) {
	sel := a.exportSel && len(a.ed.Sel) > 0
	name := sanitize(a.ed.Name()) // "design" when unsaved
	desc := map[string]string{"stl": "STL files (*.stl)", "obj": "OBJ files (*.obj)", "glb": "glTF binary files (*.glb)", "3mf": "3MF files (*.3mf)", "svg": "SVG files (*.svg)"}[format]
	p, ok := a.saveFile("Export", name+"."+format, format, desc, "*."+format)
	if !ok {
		return
	}
	if err := a.ed.Export(p, sel); err != nil {
		a.showToast(err.Error())
		return
	}
	a.closePopup()
	a.showToast("Exported " + filepath.Base(p))
}

// --- frame ---------------------------------------------------------------------

func (a *App) layout() {
	dpi := a.w.DPI
	a.batch.DPI = dpi
	a.W, a.H = float64(a.w.W)/dpi, float64(a.w.H)/dpi
	a.placeLeftTools()
	top := titleH + a.barH()
	pw := panelW
	if !a.panelOpen {
		pw = 0
	}
	a.vp = render.Rect{X: 0, Y: top, W: math.Max(50, a.W-pw), H: math.Max(50, a.H-top)}
	if a.con.open {
		a.con.height = clampConsole(a.con.height, a.H-top)
		a.vp.H = math.Max(50, a.vp.H-a.con.height)
	}
}

func (a *App) Frame() {
	if a.scene == nil || a.batch == nil {
		return
	}
	a.frames++
	a.layout()
	if a.thumbGen != a.batch.Generation || a.thumbDPI != a.w.DPI {
		a.thumbsDone = false // the atlas was rebuilt (the tiles' images are gone), or the scale changed
	}
	if !a.thumbsDone {
		// again if the atlas was rebuilt while they were added (the ones
		// before the rebuild point at stale places); it grows, so this ends
		for i := 0; i < 4; i++ {
			if i > 0 {
				// the atlas filled part way through the last pass: this one
				// starts with it empty (and as large as it has grown)
				a.batch.Reset()
			}
			gen := a.batch.Generation
			a.buildThumbs()
			if a.batch.Generation == gen {
				break
			}
		}
		a.thumbGen, a.thumbDPI = a.batch.Generation, a.w.DPI
		if !a.headless {
			a.setWindowIcons()
		}
	}
	a.stepAnim()
	a.followSelection()
	if a.titleDirty && a.drag == nil && a.pressDrag == nil {
		a.titleDirty = false
		a.updateTitle()
	}
	a.regions = a.regions[:0]
	a.blink = false

	dw, dh := int32(a.w.W), int32(a.w.H)
	a.target.Resize(dw, dh)
	a.target.Bind()
	gl.ClearColor(a.bgColor[0], a.bgColor[1], a.bgColor[2], 1)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)

	a.drawScene()

	gl.Viewport(0, 0, dw, dh)
	a.batch.Begin(a.W, a.H)
	a.drawViewportOverlay()
	if f := a.pendingOpen; f != nil {
		a.pendingOpen = nil
		f()
	}
	a.drawChrome()
	a.drawPopups()
	a.drawTooltip()
	a.drawToast()
	a.batch.Flush()
	if a.thumbGen != a.batch.Generation {
		a.w.Redraw() // the tiles were left out: draw them again, rebuilt
	}

	if !a.headless {
		a.target.BlitToWindow()
	}
	a.scene.Evict()
	a.updateHotSilently()
}

// updateHotSilently makes this frame's regions the hit-test list and
// refreshes hover state without forcing another frame.
func (a *App) updateHotSilently() {
	a.prevRegions, a.regions = a.regions, a.prevRegions
	id := ""
	if r := a.regionAt(a.mx, a.my); r != nil {
		id = r.id
	}
	if id != a.hot && a.pressDrag == nil {
		a.hot = id
		a.hoverSince = time.Now()
	}
}

func (a *App) drawToast() {
	if a.toast == "" || time.Now().After(a.toastUntil) {
		a.toast = ""
		return
	}
	b := a.batch
	tw := b.TextWidth(a.toast, fontUI, false)
	r := render.Rect{X: a.vp.CX() - tw/2 - 16, Y: a.vp.Y + a.vp.H - 64, W: tw + 32, H: 34}
	b.RoundRect(r, 6, colTooltip)
	b.TextCentered(a.toast, r, fontUI, false, colOnColor)
}

// screenshotPNG renders a frame and writes it to path as a PNG.
func (a *App) screenshotPNG(path string) error {
	a.Frame()
	img := a.target.Read(0, 0, a.target.W, a.target.H)
	// the framebuffer's alpha is meaningless after blending; screenshots
	// are opaque (image.RGBA is premultiplied, so alpha must be 255)
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	var buf bytes.Buffer
	if err := encodePNG(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644) // its error reported, close included
}
