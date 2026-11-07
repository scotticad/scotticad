//go:build linux

// The Linux implementation: X11 and GLX through purego (no cgo). It runs on
// X11 desktops and, through XWayland, on Wayland ones.

package window

import (
	"errors"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Window is the application window.
type Window struct {
	d      uintptr // the X display
	screen int32
	xwin   uintptr
	ctx    uintptr
	h      Handler
	W, H   int
	DPI    float64 // scale factor (1.0 = 96 dpi)
	// FixedDPI keeps DPI as set (a --scale override).
	FixedDPI bool
	// MinW, MinH are the smallest client area (logical pixels) the window
	// can be resized to; 0: no limit.
	MinW, MinH int
	dirty      bool
	Animate    bool
	// OnPanic, if set, is called once with what a handler panicked with,
	// before the panic goes on and ends the program: a last chance to save
	// the user's work.
	OnPanic func(r any)
	// IgnoreInput drops real mouse/keyboard input (headless test runs,
	// where simulated input is fed to the handler directly).
	IgnoreInput bool
	quit        bool

	wake     [2]int // a pipe: Wake writes, the loop wakes on it
	buttons  uint32 // mouse buttons held (bit per Left/Right/Middle)
	keysDown map[uint32]bool
	lastDown struct { // the last left press, for double clicks
		t    uint64
		x, y int32
	}
	cursorID int
	cursors  map[int]uintptr
	clip     string // the text we own the CLIPBOARD selection with
	icons    map[int][]byte
	minSet   [2]int // the minimum size last given to the window manager
	atoms    map[string]uintptr
}

var current *Window

// New creates the window and an OpenGL context. Call from the main
// goroutine; it locks the OS thread.
// scale, if above 0, fixes the interface scale (else the desktop's); w and
// h are the client area in logical pixels either way.
func New(title string, w, h int, visible bool, handler Handler, scale float64) (*Window, error) {
	runtime.LockOSThread()
	if err := loadLibs(); err != nil {
		return nil, err
	}
	d := xOpenDisplay(nil)
	if d == 0 {
		return nil, errNoDisplay
	}
	// X errors (a window gone, a property refused) are reported to us
	// rather than ending the program, as Xlib's default handler would
	if cb := xErrorCallback(); cb != 0 {
		xSetErrorHandler(cb)
	}
	win := &Window{d: d, h: handler, DPI: 1, keysDown: map[uint32]bool{}, cursors: map[int]uintptr{},
		icons: map[int][]byte{}, atoms: map[string]uintptr{}}
	current = win
	win.screen = xDefaultScreen(d)
	root := xRootWindow(d, win.screen)
	win.DPI = desktopScale(d)
	if scale > 0 {
		win.DPI, win.FixedDPI = scale, true
	}

	// a double-buffered RGBA visual with depth and stencil, as the Windows
	// pixel format asks for
	attrs := []int32{
		0x8012, 1, // GLX_X_RENDERABLE
		0x8010, 1, // GLX_DRAWABLE_TYPE: GLX_WINDOW_BIT
		0x8011, 1, // GLX_RENDER_TYPE: GLX_RGBA_BIT
		5, 1, // GLX_DOUBLEBUFFER
		8, 8, 9, 8, 10, 8, 11, 8, // GLX_RED/GREEN/BLUE/ALPHA_SIZE
		12, 24, 13, 8, // GLX_DEPTH_SIZE, GLX_STENCIL_SIZE
		0,
	}
	var n int32
	cfgs := glXChooseFBConfig(d, win.screen, &attrs[0], &n)
	if cfgs == 0 || n == 0 {
		return nil, errors.New("no OpenGL-capable visual (glXChooseFBConfig found none)")
	}
	// the first config with an opaque (24-bit) visual: a 32-bit one is
	// ARGB, and a compositing desktop would show the window through
	// wherever the frame's alpha is below 1
	var cfg, visual uintptr
	var depth int32
	for i := range int(n) {
		c := *(*uintptr)(unsafe.Add(foreign(cfgs), i*8))
		vi := glXGetVisualFromFBConfig(d, c)
		if vi == 0 {
			continue
		}
		v, dp := *(*uintptr)(foreign(vi)), *(*int32)(foreign(vi + 20)) // XVisualInfo.visual, .depth
		xFree(vi)
		if cfg == 0 || (depth == 32 && dp != 32) {
			cfg, visual, depth = c, v, dp
		}
		if dp != 32 {
			break
		}
	}
	xFree(cfgs)
	if cfg == 0 {
		return nil, errors.New("glXGetVisualFromFBConfig found no visual")
	}

	// w×h is the client area in logical pixels
	pw, ph := int(float64(w)*win.DPI), int(float64(h)*win.DPI)
	if visible {
		sw, sh := int(xDisplayWidth(d, win.screen)), int(xDisplayHeight(d, win.screen))
		pw, ph = min(pw, sw), min(ph, sh-40)
	}
	var swa [112]byte // XSetWindowAttributes
	*(*uintptr)(unsafe.Pointer(&swa[96])) = xCreateColormap(d, root, visual, 0)
	*(*int64)(unsafe.Pointer(&swa[72])) = maskKeyPress | maskKeyRelease | maskButtonPress | maskButtonRelease |
		maskEnterWindow | maskLeaveWindow | maskPointerMotion | maskExposure | maskStructure | maskFocusChange | maskPropertyChange
	const cwBorderPixel, cwEventMask, cwColormap = 1 << 3, 1 << 11, 1 << 13
	win.xwin = xCreateWindow(d, root, 0, 0, uint32(pw), uint32(ph), 0, depth, 1 /*InputOutput*/, visual,
		cwBorderPixel|cwEventMask|cwColormap, unsafe.Pointer(&swa[0]))
	if win.xwin == 0 {
		return nil, errors.New("XCreateWindow failed")
	}
	win.W, win.H = pw, ph

	// a legacy context, as wglCreateContext makes on Windows: the
	// compatibility profile at the highest version the driver has
	win.ctx = glXCreateNewContext(d, cfg, 0x8014 /*GLX_RGBA_TYPE*/, 0, 1)
	if win.ctx == 0 {
		return nil, errors.New("glXCreateNewContext failed")
	}
	if glXMakeCurrent(d, win.xwin, win.ctx) == 0 {
		return nil, errors.New("glXMakeCurrent failed")
	}

	// the window manager's side: closing, the class (for desktop files and
	// the dock), the title
	del := win.atom("WM_DELETE_WINDOW")
	xSetWMProtocols(d, win.xwin, &del, 1)
	cls := [2]*byte{cstr("scotticad"), cstr("ScottiCAD")}
	xSetClassHint(d, win.xwin, unsafe.Pointer(&cls))
	win.SetTitle(title)
	var supported int32
	xkbSetDetectableAutoRepeat(d, 1, &supported) // held keys repeat as presses only

	if err := syscall.Pipe2(win.wake[:], syscall.O_NONBLOCK|syscall.O_CLOEXEC); err != nil {
		return nil, err
	}
	win.SetCursor(CursorArrow)
	if visible {
		xMapWindow(d, win.xwin)
	}
	xFlush(d)
	return win, nil
}

// desktopScale is the interface scale the desktop asks for: Xft.dpi (which
// GNOME and KDE set for X programs), else GDK_SCALE, else 1. It is rounded
// to a quarter, as Windows' scale settings are.
func desktopScale(d uintptr) float64 {
	return scaleFrom(gostr(xResourceManagerString(d)), os.Getenv("GDK_SCALE"))
}

// scaleFrom is desktopScale from the X resources text and GDK_SCALE.
func scaleFrom(resources, gdkScale string) float64 {
	s := 1.0
	for _, line := range strings.Split(resources, "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "Xft.dpi" {
			if dpi, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && dpi > 0 {
				s = dpi / 96
			}
		}
	}
	if s == 1 {
		if g, err := strconv.ParseFloat(gdkScale, 64); err == nil && g > 0 {
			s = g
		}
	}
	return math.Max(0.5, math.Min(4, math.Round(s*4)/4))
}

var errorCallback uintptr

// xErrorCallback is an Xlib error handler that ignores the error (a C
// callback through purego; 0 if callbacks are unavailable).
func xErrorCallback() (cb uintptr) {
	if errorCallback != 0 {
		return errorCallback
	}
	defer func() {
		if recover() != nil {
			cb = 0
		}
	}()
	errorCallback = purego.NewCallback(func(d, ev uintptr) uintptr { return 0 })
	return errorCallback
}

func (w *Window) atom(name string) uintptr {
	if a, ok := w.atoms[name]; ok {
		return a
	}
	a := xInternAtom(w.d, cstr(name), 0)
	w.atoms[name] = a
	return a
}

// GetProcAddress resolves an OpenGL function.
func GetProcAddress(name string) uintptr {
	if loadLibs() != nil {
		return 0
	}
	if p, err := purego.Dlsym(libGL, name); err == nil && p != 0 {
		return p
	}
	return glXGetProcAddressARB(cstr(name))
}

// SwapInterval sets how many vertical blanks a buffer swap waits for, with
// whichever GLX extension the driver has.
func SwapInterval(n int) {
	w := current
	if w == nil {
		return
	}
	ext := " " + gostr(glXQueryExtensionsString(w.d, w.screen)) + " "
	bind := func(name string, f any) bool {
		p := glXGetProcAddressARB(cstr(name))
		if p == 0 {
			return false
		}
		purego.RegisterFunc(f, p)
		return true
	}
	switch {
	case strings.Contains(ext, " GLX_EXT_swap_control ") && bind("glXSwapIntervalEXT", &glXSwapIntervalEXT):
		glXSwapIntervalEXT(w.d, w.xwin, int32(n))
	case strings.Contains(ext, " GLX_MESA_swap_control ") && bind("glXSwapIntervalMESA", &glXSwapIntervalMESA):
		glXSwapIntervalMESA(uint32(n))
	case strings.Contains(ext, " GLX_SGI_swap_control ") && bind("glXSwapIntervalSGI", &glXSwapIntervalSGI):
		glXSwapIntervalSGI(int32(n))
	}
}

func (w *Window) Handler(h Handler) { w.h = h }

// Redraw schedules a frame.
func (w *Window) Redraw() { w.dirty = true }

// Wake can be called from any goroutine to make the loop run Idle.
func (w *Window) Wake() { syscall.Write(w.wake[1], []byte{1}) }

func (w *Window) SetTitle(s string) {
	b := []byte(s)
	xStoreName(w.d, w.xwin, cstr(s))
	if len(b) > 0 {
		xChangeProperty(w.d, w.xwin, w.atom("_NET_WM_NAME"), w.atom("UTF8_STRING"), 8, propModeReplace, unsafe.Pointer(&b[0]), int32(len(b)))
	}
	xFlush(w.d)
}

func (w *Window) Show() { xMapWindow(w.d, w.xwin); xFlush(w.d) }

func (w *Window) Quit() { w.quit = true; w.Wake() }

// cursorNames are each cursor's names in the cursor theme (the CSS name,
// then the older X one) and its X cursor-font shape, the last resort.
var cursorNames = map[int]struct {
	names []string
	shape uint32
}{
	CursorArrow:    {[]string{"default", "left_ptr"}, 68},
	CursorIBeam:    {[]string{"text", "xterm"}, 152},
	CursorWait:     {[]string{"wait", "watch"}, 150},
	CursorCross:    {[]string{"crosshair", "cross"}, 34},
	CursorSizeNWSE: {[]string{"nwse-resize", "bottom_right_corner"}, 14},
	CursorSizeNESW: {[]string{"nesw-resize", "bottom_left_corner"}, 12},
	CursorSizeWE:   {[]string{"ew-resize", "sb_h_double_arrow"}, 108},
	CursorSizeNS:   {[]string{"ns-resize", "sb_v_double_arrow"}, 116},
	CursorSizeAll:  {[]string{"move", "fleur"}, 52},
	CursorNo:       {[]string{"not-allowed", "circle"}, 0},
	CursorHand:     {[]string{"pointer", "hand2"}, 60},
}

func (w *Window) SetCursor(id int) {
	if id == w.cursorID {
		return
	}
	c, ok := w.cursors[id]
	if !ok {
		spec, known := cursorNames[id]
		if !known {
			spec = cursorNames[CursorArrow]
		}
		if xcursorLibraryLoadCursor != nil {
			for _, n := range spec.names {
				if c = xcursorLibraryLoadCursor(w.d, cstr(n)); c != 0 {
					break
				}
			}
		}
		if c == 0 {
			c = xCreateFontCursor(w.d, spec.shape)
		}
		w.cursors[id] = c
	}
	w.cursorID = id
	xDefineCursor(w.d, w.xwin, c)
	xFlush(w.d)
}

// Capture keeps mouse events coming while a button is held outside the
// window. X does that by itself (a press grabs the pointer until the
// release), so there is nothing to do here.
func (w *Window) Capture(on bool) {}

func (w *Window) Swap() { glXSwapBuffers(w.d, w.xwin) }

// Run pumps events until the window closes.
func (w *Window) Run() {
	defer w.guard()
	w.applyMinSize()
	var ev xEvent
	for !w.quit {
		for xPending(w.d) > 0 {
			xNextEvent(w.d, ev.ptr())
			w.handle(&ev)
			if w.quit {
				return
			}
		}
		w.drainWake()
		w.h.Idle()
		w.applyMinSize()
		if w.quit {
			return
		}
		if w.dirty || w.Animate {
			w.dirty = false
			w.h.Frame()
			w.Swap()
		} else if xPending(w.d) == 0 {
			w.waitEvents()
		}
	}
}

// waitEvents blocks until the X connection has something to read or Wake
// is called.
func (w *Window) waitEvents() {
	xFlush(w.d)
	fds := [2]struct {
		fd             int32
		events, revent int16
	}{{fd: xConnectionNumber(w.d), events: 1}, {fd: int32(w.wake[0]), events: 1}} // POLLIN
	for {
		_, _, e := syscall.Syscall6(syscall.SYS_PPOLL, uintptr(unsafe.Pointer(&fds[0])), 2, 0, 0, 0, 0)
		if e != syscall.EINTR {
			return
		}
	}
}

func (w *Window) drainWake() {
	var b [64]byte
	for {
		if n, err := syscall.Read(w.wake[0], b[:]); n <= 0 || err != nil {
			return
		}
	}
}

// applyMinSize tells the window manager the smallest size, when it changes.
func (w *Window) applyMinSize() {
	mw, mh := int(float64(w.MinW)*w.DPI), int(float64(w.MinH)*w.DPI)
	if [2]int{mw, mh} == w.minSet {
		return
	}
	w.minSet = [2]int{mw, mh}
	var hints [80]byte                            // XSizeHints
	*(*int64)(unsafe.Pointer(&hints[0])) = 1 << 4 // PMinSize
	*(*int32)(unsafe.Pointer(&hints[24])) = int32(mw)
	*(*int32)(unsafe.Pointer(&hints[28])) = int32(mh)
	xSetWMNormalHints(w.d, w.xwin, unsafe.Pointer(&hints[0]))
	xFlush(w.d)
}

func modsOf(state uint32) Mods {
	var m Mods
	if state&stateShift != 0 {
		m |= Shift
	}
	if state&stateControl != 0 {
		m |= Ctrl
	}
	if state&stateMod1 != 0 {
		m |= Alt
	}
	return m
}

func (w *Window) handle(ev *xEvent) {
	t := ev.typ()
	if w.IgnoreInput {
		switch t {
		case evKeyPress, evKeyRelease, evButtonPress, evButtonRelease, evMotionNotify, evEnterNotify, evLeaveNotify:
			return
		}
	}
	switch t {
	case evClientMessage:
		if uintptr(ev.u64(56)) == w.atom("WM_DELETE_WINDOW") && w.h.Close() {
			w.quit = true
			xDestroyWindow(w.d, w.xwin)
			xFlush(w.d)
		}
	case evExpose:
		w.dirty = true
	case evConfigureNotify:
		pw, ph := int(ev.i32(56)), int(ev.i32(60))
		if (pw != w.W || ph != w.H) && pw > 0 && ph > 0 {
			w.W, w.H = pw, ph
			w.h.Resize(w.W, w.H)
			w.dirty = true
		}
	case evMotionNotify:
		x, y := ev.xy()
		w.h.MouseMove(float64(x), float64(y), modsOf(ev.state()))
	case evLeaveNotify:
		// an ordinary leave, with no drag under way; a grab by another
		// program (a menu, Alt+Tab) during a drag takes the mouse away
		switch mode := ev.i32(80); {
		case mode == notifyNormal && w.buttons == 0:
			w.h.MouseLeave()
		case mode == notifyGrab && w.buttons != 0:
			w.lostCapture()
		}
	case evFocusOut:
		w.keysDown = map[uint32]bool{}
		if w.buttons != 0 {
			w.lostCapture()
		}
	case evButtonPress:
		x, y := ev.xy()
		m := modsOf(ev.state())
		switch b := ev.detail(); b {
		case 1, 2, 3:
			btn := xButtons[b]
			double := false
			if btn == Left {
				// two presses within half a second and a few pixels, as
				// Windows' double-click defaults
				dt := ev.time() - w.lastDown.t
				if w.lastDown.t != 0 && dt <= 500 && abs32(x-w.lastDown.x) <= 4 && abs32(y-w.lastDown.y) <= 4 {
					double = true
					w.lastDown.t = 0
				} else {
					w.lastDown.t, w.lastDown.x, w.lastDown.y = ev.time(), x, y
				}
			}
			w.buttons |= 1 << btn
			w.h.MouseDown(btn, float64(x), float64(y), m, double)
		case 4, 5: // the wheel: one notch per press
			d := 1.0
			if b == 5 {
				d = -1
			}
			w.h.MouseWheel(d, float64(x), float64(y), m)
		}
	case evButtonRelease:
		x, y := ev.xy()
		if b := ev.detail(); b >= 1 && b <= 3 {
			btn := xButtons[b]
			w.buttons &^= 1 << btn
			w.h.MouseUp(btn, float64(x), float64(y), modsOf(ev.state()))
		}
	case evKeyPress:
		w.key(ev, true)
	case evKeyRelease:
		w.key(ev, false)
	case evSelectionRequest:
		w.serveSelection(ev)
	case evSelectionClear:
		if uintptr(ev.u64(40)) == w.atom("CLIPBOARD") {
			w.clip = ""
		}
	}
}

// xButtons maps X's pointer button numbers to ours.
var xButtons = map[uint32]int{1: Left, 2: Middle, 3: Right}

func (w *Window) lostCapture() {
	w.buttons = 0
	w.h.CaptureLost()
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func (w *Window) key(ev *xEvent, down bool) {
	code := ev.detail()
	vk := keysymToKey(xLookupKeysym(ev.ptr(), 0)) // the key itself, unshifted
	m := keyMods(vk, ev.state(), down)
	if !down {
		delete(w.keysDown, code)
		if vk != 0 {
			w.h.KeyUp(vk, m)
		}
		return
	}
	repeat := w.keysDown[code]
	w.keysDown[code] = true
	if vk != 0 {
		w.h.KeyDown(vk, m, repeat)
	}
	// the character it types, unless Ctrl or Alt makes it a shortcut (as
	// on Windows, where Alt+letter is WM_SYSCHAR and types nothing; AltGr
	// is X's Mod5, not Alt, so AltGr characters still come through)
	if m&(Ctrl|Alt) == 0 {
		var buf [32]byte
		var sym uint64
		xLookupString(ev.ptr(), &buf[0], int32(len(buf)), &sym, nil)
		if r := keysymToRune(sym); r >= 32 && r != 127 {
			w.h.Char(r)
		}
	}
}

// keyMods is the modifiers held as key vk goes down or up. X gives the
// state as it was before the event, so a modifier being pressed counts
// and one being released doesn't, as GetKeyState sees them on Windows
// (else releasing Shift mid-drag would keep the drag constrained until
// the mouse next moved).
func keyMods(vk int, state uint32, down bool) Mods {
	m := modsOf(state)
	var bit Mods
	switch vk {
	case KeyShift:
		bit = Shift
	case KeyControl:
		bit = Ctrl
	case KeyAlt:
		bit = Alt
	}
	if down {
		return m | bit
	}
	return m &^ bit
}

// keysymToKey maps an X keysym to ScottiCAD's key code (see Key*); 0 for
// keys it has none for.
func keysymToKey(s uint64) int {
	switch {
	case s >= 'a' && s <= 'z':
		return int(s - 'a' + 'A')
	case s >= 'A' && s <= 'Z', s >= '0' && s <= '9':
		return int(s)
	case s >= 0xffbe && s <= 0xffc9: // F1..F12
		return KeyF1 + int(s-0xffbe)
	}
	switch s {
	case 0xff08:
		return KeyBack
	case 0xff09, 0xfe20: // Tab, Shift+Tab (ISO_Left_Tab)
		return KeyTab
	case 0xff0d, 0xff8d: // Return, keypad Enter
		return KeyReturn
	case 0xff1b:
		return KeyEscape
	case 0x20:
		return KeySpace
	case 0xffe1, 0xffe2:
		return KeyShift
	case 0xffe3, 0xffe4:
		return KeyControl
	case 0xffe9, 0xffea, 0xffe7, 0xffe8: // Alt, Meta
		return KeyAlt
	case 0xff55, 0xff9a:
		return KeyPgUp
	case 0xff56, 0xff9b:
		return KeyPgDn
	case 0xff57, 0xff9c:
		return KeyEnd
	case 0xff50, 0xff95:
		return KeyHome
	case 0xff51, 0xff96:
		return KeyLeft
	case 0xff52, 0xff97:
		return KeyUp
	case 0xff53, 0xff98:
		return KeyRight
	case 0xff54, 0xff99:
		return KeyDown
	case 0xff63, 0xff9e:
		return KeyInsert
	case 0xffff, 0xff9f:
		return KeyDelete
	case 0xffab:
		return KeyAdd
	case 0xffad:
		return KeySubtract
	case '=', '+':
		return KeyPlus
	case '-', '_':
		return KeyMinus
	case ',':
		return KeyComma
	case '.':
		return KeyPeriod
	case '/':
		return KeySlash
	}
	return 0
}

// keysymToRune is the character a keysym types: Latin-1 keysyms are their
// own code, and keysyms from 0x01000000 carry the Unicode code point.
func keysymToRune(s uint64) rune {
	switch {
	case s >= 0x20 && s <= 0x7e, s >= 0xa0 && s <= 0xff:
		return rune(s)
	case s&0xff000000 == 0x01000000:
		return rune(s & 0x00ffffff)
	case s >= 0xffb0 && s <= 0xffb9: // keypad digits
		return rune('0' + s - 0xffb0)
	}
	switch s {
	case 0xffaa:
		return '*'
	case 0xffab:
		return '+'
	case 0xffad:
		return '-'
	case 0xffae:
		return '.'
	case 0xffaf:
		return '/'
	case 0x20ac:
		return '€'
	}
	return 0
}

// --- clipboard -----------------------------------------------------------

func (w *Window) SetClipboard(s string) {
	w.clip = strings.ReplaceAll(s, "\x00", "")
	xSetSelectionOwner(w.d, w.atom("CLIPBOARD"), w.xwin, 0)
	xFlush(w.d)
}

// Clipboard is the CLIPBOARD selection's text: ours if we own it, else
// asked of the program that does (waiting up to two seconds for it).
func (w *Window) Clipboard() string {
	sel := w.atom("CLIPBOARD")
	owner := xGetSelectionOwner(w.d, sel)
	switch owner {
	case 0:
		return ""
	case w.xwin:
		return w.clip
	}
	for _, target := range []uintptr{w.atom("UTF8_STRING"), atomString} {
		prop := w.atom("SCOTTICAD_CLIPBOARD")
		xDeleteProperty(w.d, w.xwin, prop)
		xConvertSelection(w.d, sel, target, prop, w.xwin, 0)
		xFlush(w.d)
		var ev xEvent
		if !w.waitFor(evSelectionNotify, &ev, 2*time.Second) {
			return ""
		}
		if uintptr(ev.u64(56)) == 0 { // property None: this target was refused
			continue
		}
		data, typ := w.readProperty(prop)
		if typ == w.atom("INCR") {
			return w.readIncremental(prop)
		}
		return string(data)
	}
	return ""
}

// waitFor waits for an event of type t on our window, leaving other events
// queued.
func (w *Window) waitFor(t int32, ev *xEvent, timeout time.Duration) bool {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		if xCheckTypedWindowEvent(w.d, w.xwin, t, ev.ptr()) != 0 {
			return true
		}
		fds := [1]struct {
			fd             int32
			events, revent int16
		}{{fd: xConnectionNumber(w.d), events: 1}}
		ts := syscall.NsecToTimespec(int64(20 * time.Millisecond))
		syscall.Syscall6(syscall.SYS_PPOLL, uintptr(unsafe.Pointer(&fds[0])), 1, uintptr(unsafe.Pointer(&ts)), 0, 0, 0)
		xPending(w.d) // reads what arrived into Xlib's queue
	}
	return false
}

// readProperty reads (and deletes) a property of our window.
func (w *Window) readProperty(prop uintptr) ([]byte, uintptr) {
	var typ uintptr
	var format int32
	var n, after uint64
	var p uintptr
	if xGetWindowProperty(w.d, w.xwin, prop, 0, 1<<28, 1, 0, &typ, &format, &n, &after, &p) != 0 || p == 0 {
		return nil, typ
	}
	defer xFree(p)
	size := int(n) * int(format) / 8
	if format == 32 {
		size = int(n) * 8 // format 32 comes as C longs
	}
	return append([]byte(nil), unsafe.Slice((*byte)(foreign(p)), size)...), typ
}

// readIncremental receives a large selection in pieces (the INCR protocol):
// each new value of the property is the next piece; an empty one ends it.
func (w *Window) readIncremental(prop uintptr) string {
	var out []byte
	for {
		var ev xEvent
		got := false
		end := time.Now().Add(2 * time.Second)
		for time.Now().Before(end) && !got {
			if !w.waitFor(evPropertyNotify, &ev, time.Until(end)) {
				return string(out)
			}
			got = uintptr(ev.u64(40)) == prop && ev.i32(56) == 0 // PropertyNewValue
		}
		if !got {
			return string(out)
		}
		data, _ := w.readProperty(prop)
		if len(data) == 0 {
			return string(out)
		}
		out = append(out, data...)
	}
}

// serveSelection answers another program asking for our clipboard text.
func (w *Window) serveSelection(ev *xEvent) {
	requestor, sel, target, prop, tm := uintptr(ev.u64(40)), uintptr(ev.u64(48)), uintptr(ev.u64(56)), uintptr(ev.u64(64)), ev.u64(72)
	if prop == 0 {
		prop = target // an obsolete client
	}
	utf8 := w.atom("UTF8_STRING")
	plain := w.atom("text/plain;charset=utf-8")
	switch {
	case sel != w.atom("CLIPBOARD"):
		prop = 0
	case target == w.atom("TARGETS"):
		targets := []uint64{uint64(w.atom("TARGETS")), uint64(utf8), uint64(plain), uint64(w.atom("TEXT")), atomString}
		xChangeProperty(w.d, requestor, prop, atomAtom, 32, propModeReplace, unsafe.Pointer(&targets[0]), int32(len(targets)))
	case target == utf8 || target == plain || target == w.atom("TEXT") || target == atomString:
		b := []byte(w.clip)
		typ := utf8
		if target == atomString {
			b, typ = latin1(w.clip), atomString
		}
		var p unsafe.Pointer
		if len(b) > 0 {
			p = unsafe.Pointer(&b[0])
		}
		xChangeProperty(w.d, requestor, prop, typ, 8, propModeReplace, p, int32(len(b)))
	default:
		prop = 0 // refused
	}
	var reply xEvent // XSelectionEvent
	*(*int32)(unsafe.Pointer(&reply[0])) = evSelectionNotify
	reply[4], reply[5], reply[6], reply[7], reply[8] = uint64(requestor), uint64(sel), uint64(target), uint64(prop), tm
	xSendEvent(w.d, requestor, 0, 0, reply.ptr())
	xFlush(w.d)
}

func latin1(s string) []byte {
	var b []byte
	for _, r := range s {
		if r > 0xff {
			r = '?'
		}
		b = append(b, byte(r))
	}
	return b
}

// SetIcon sets the window's icon from size×size RGBA pixels (top row
// first, straight alpha). The icon sizes given (big and small) are all
// offered to the desktop, which picks.
func (w *Window) SetIcon(rgba []byte, size int, big bool) {
	if len(rgba) != size*size*4 {
		return
	}
	w.icons[size] = append([]byte(nil), rgba...)
	// _NET_WM_ICON: width, height, then ARGB pixels, each a C long
	var data []uint64
	for s, px := range w.icons {
		data = append(data, uint64(s), uint64(s))
		for i := 0; i < len(px); i += 4 {
			data = append(data, uint64(px[i+3])<<24|uint64(px[i])<<16|uint64(px[i+1])<<8|uint64(px[i+2]))
		}
	}
	xChangeProperty(w.d, w.xwin, w.atom("_NET_WM_ICON"), atomCardinal, 32, propModeReplace, unsafe.Pointer(&data[0]), int32(len(data)))
	xFlush(w.d)
}
