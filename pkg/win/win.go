//go:build windows

// Package win is a minimal Win32 host: one top-level window with an OpenGL
// context, input events, clipboard, cursors and file dialogs, written
// against the Win32 API directly (no cgo).
package win

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	opengl32 = syscall.NewLazyDLL("opengl32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")

	pRegisterClassExW   = user32.NewProc("RegisterClassExW")
	pCreateWindowExW    = user32.NewProc("CreateWindowExW")
	pDefWindowProcW     = user32.NewProc("DefWindowProcW")
	pPeekMessageW       = user32.NewProc("PeekMessageW")
	pWaitMessage        = user32.NewProc("WaitMessage")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessageW   = user32.NewProc("DispatchMessageW")
	pShowWindow         = user32.NewProc("ShowWindow")
	pGetDC              = user32.NewProc("GetDC")
	pLoadCursorW        = user32.NewProc("LoadCursorW")
	pSetCursor          = user32.NewProc("SetCursor")
	pGetClientRect      = user32.NewProc("GetClientRect")
	pSetCapture         = user32.NewProc("SetCapture")
	pReleaseCapture     = user32.NewProc("ReleaseCapture")
	pSetWindowTextW     = user32.NewProc("SetWindowTextW")
	pGetKeyState        = user32.NewProc("GetKeyState")
	pPostQuitMessage    = user32.NewProc("PostQuitMessage")
	pPostMessageW       = user32.NewProc("PostMessageW")
	pValidateRect       = user32.NewProc("ValidateRect")
	pDestroyWindow      = user32.NewProc("DestroyWindow")
	pSendMessageW       = user32.NewProc("SendMessageW")
	pDestroyIcon        = user32.NewProc("DestroyIcon")
	pCreateIconIndirect = user32.NewProc("CreateIconIndirect")
	pCreateBitmap       = gdi32.NewProc("CreateBitmap")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pSetProcessDpiAware = user32.NewProc("SetProcessDpiAwarenessContext")
	pGetDpiForWindow    = user32.NewProc("GetDpiForWindow")
	pOpenClipboard      = user32.NewProc("OpenClipboard")
	pCloseClipboard     = user32.NewProc("CloseClipboard")
	pEmptyClipboard     = user32.NewProc("EmptyClipboard")
	pGetClipboardData   = user32.NewProc("GetClipboardData")
	pSetClipboardData   = user32.NewProc("SetClipboardData")
	pMessageBoxW        = user32.NewProc("MessageBoxW")
	pTrackMouseEvent    = user32.NewProc("TrackMouseEvent")
	pSetWindowPos       = user32.NewProc("SetWindowPos")
	pAdjustWindowRectEx = user32.NewProc("AdjustWindowRectEx")
	pGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	pChoosePixelFormat  = gdi32.NewProc("ChoosePixelFormat")
	pSetPixelFormat     = gdi32.NewProc("SetPixelFormat")
	pSwapBuffers        = gdi32.NewProc("SwapBuffers")
	pGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	pGlobalAlloc        = kernel32.NewProc("GlobalAlloc")
	pGlobalLock         = kernel32.NewProc("GlobalLock")
	pGlobalUnlock       = kernel32.NewProc("GlobalUnlock")
	pGlobalSize         = kernel32.NewProc("GlobalSize")
	pGlobalFree         = kernel32.NewProc("GlobalFree")
	pWglCreateContext   = opengl32.NewProc("wglCreateContext")
	pWglMakeCurrent     = opengl32.NewProc("wglMakeCurrent")
	pWglGetProcAddress  = opengl32.NewProc("wglGetProcAddress")
	pDragAcceptFiles    = shell32.NewProc("DragAcceptFiles")
	pDragQueryFileW     = shell32.NewProc("DragQueryFileW")
	pDragFinish         = shell32.NewProc("DragFinish")
	pShellExecuteW      = shell32.NewProc("ShellExecuteW")
	pGetOpenFileNameW   = comdlg32.NewProc("GetOpenFileNameW")
	pGetSaveFileNameW   = comdlg32.NewProc("GetSaveFileNameW")
)

const (
	wmDestroy        = 0x0002
	wmSize           = 0x0005
	wmPaint          = 0x000F
	wmClose          = 0x0010
	wmEraseBkgnd     = 0x0014
	wmSetCursor      = 0x0020
	wmKeyDown        = 0x0100
	wmKeyUp          = 0x0101
	wmChar           = 0x0102
	wmSysKeyDown     = 0x0104
	wmSysKeyUp       = 0x0105
	wmTimer          = 0x0113
	wmMouseMove      = 0x0200
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmLButtonDbl     = 0x0203
	wmRButtonDown    = 0x0204
	wmRButtonUp      = 0x0205
	wmMButtonDown    = 0x0207
	wmMButtonUp      = 0x0208
	wmMouseWheel     = 0x020A
	wmMouseLeave     = 0x02A3
	wmDropFiles      = 0x0233
	wmDpiChanged     = 0x02E0
	wmGetMinMaxInfo  = 0x0024
	wmCaptureChanged = 0x0215
	wmApp            = 0x8000
	wmQuit           = 0x0012
)

// Mouse buttons.
const (
	Left = iota
	Right
	Middle
)

// Mods is a modifier key bitmask.
type Mods int

const (
	Shift Mods = 1 << iota
	Ctrl
	Alt
)

// Handler receives window events. All calls happen on the window thread
// with the GL context current.
type Handler interface {
	MouseMove(x, y float64, m Mods)
	MouseDown(btn int, x, y float64, m Mods, double bool)
	MouseUp(btn int, x, y float64, m Mods)
	MouseWheel(delta float64, x, y float64, m Mods)
	MouseLeave()
	KeyDown(vk int, m Mods, repeat bool)
	KeyUp(vk int, m Mods)
	Char(r rune)
	Resize(w, h int)
	DropFiles(paths []string, x, y float64)
	Frame()
	Idle() // called every loop iteration
	Close() bool
	// CaptureLost: the mouse was taken away mid-drag (another window,
	// Alt+Tab, a dialog): the button-up that would end the drag won't come.
	CaptureLost()
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      [2]int32
	private uint32
}

type pixelFormatDescriptor struct {
	nSize, nVersion                                             uint16
	dwFlags                                                     uint32
	iPixelType, cColorBits, cRedBits, cRedShift, cGreenBits     byte
	cGreenShift, cBlueBits, cBlueShift, cAlphaBits, cAlphaShift byte
	cAccumBits, cAccumRedBits, cAccumGreenBits, cAccumBlueBits  byte
	cAccumAlphaBits, cDepthBits, cStencilBits, cAuxBuffers      byte
	iLayerType, bReserved                                       byte
	dwLayerMask, dwVisibleMask, dwDamageMask                    uint32
}

type rect struct{ left, top, right, bottom int32 }

// Window is the application window.
type Window struct {
	hwnd  uintptr
	hdc   uintptr
	hglrc uintptr
	h     Handler
	W, H  int
	DPI   float64 // scale factor (1.0 = 96 dpi)
	// FixedDPI keeps DPI as set (a --scale override) when the window moves
	// to a display with another scale.
	FixedDPI bool
	// MinW, MinH are the smallest client area (logical pixels) the window
	// can be resized to; 0: no limit.
	MinW, MinH int
	hiSurr     rune // the high half of a surrogate pair, waiting for its low half
	releasing  bool // inside Capture(false): its release isn't a lost capture
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
	cursor      uintptr
	tracking    bool
}

var current *Window

// Cursor identifiers.
const (
	CursorArrow    = 32512
	CursorIBeam    = 32513
	CursorWait     = 32514
	CursorCross    = 32515
	CursorSizeNWSE = 32642
	CursorSizeNESW = 32643
	CursorSizeWE   = 32644
	CursorSizeNS   = 32645
	CursorSizeAll  = 32646
	CursorNo       = 32648
	CursorHand     = 32649
)

func utf16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

// New creates the window and an OpenGL context. Call from the main
// goroutine; it locks the OS thread.
// scale, if above 0, fixes the interface scale (else the display's); w and h
// are the client area in logical pixels either way.
func New(title string, w, h int, visible bool, handler Handler, scale float64) (*Window, error) {
	runtime.LockOSThread()
	pSetProcessDpiAware.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	inst, _, _ := pGetModuleHandleW.Call(0)
	win := &Window{h: handler, DPI: 1}
	current = win
	cls := wndClassEx{
		style:         0x0020 | 0x0008 | 0x0001 | 0x0002, // OWNDC|DBLCLKS|VREDRAW|HREDRAW
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     inst,
		lpszClassName: utf16("ScottiCAD"),
	}
	cls.cbSize = uint32(unsafe.Sizeof(cls))
	cls.hCursor, _, _ = pLoadCursorW.Call(0, CursorArrow)
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&cls))); r == 0 {
		return nil, err
	}
	style := uintptr(0x00CF0000) // WS_OVERLAPPEDWINDOW
	hwnd, _, err := pCreateWindowExW.Call(0x10 /*WS_EX_ACCEPTFILES*/, uintptr(unsafe.Pointer(utf16("ScottiCAD"))), uintptr(unsafe.Pointer(utf16(title))),
		style, 0x80000000, 0x80000000, uintptr(w), uintptr(h), 0, 0, inst, 0)
	if hwnd == 0 {
		return nil, err
	}
	win.hwnd = hwnd
	if d, _, _ := pGetDpiForWindow.Call(hwnd); d != 0 {
		win.DPI = float64(d) / 96
	}
	if scale > 0 {
		win.DPI, win.FixedDPI = scale, true
	}
	// w×h is the client area in logical pixels
	{
		rc := rect{0, 0, int32(float64(w) * win.DPI), int32(float64(h) * win.DPI)}
		pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&rc)), style, 0, 0x10)
		nw, nh := int(rc.right-rc.left), int(rc.bottom-rc.top)
		if visible {
			sw, _, _ := pGetSystemMetrics.Call(0)
			sh, _, _ := pGetSystemMetrics.Call(1)
			if nw > int(sw) {
				nw = int(sw)
			}
			if nh > int(sh)-40 {
				nh = int(sh) - 40
			}
		}
		pSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(nw), uintptr(nh), 0x0002|0x0004) // NOMOVE|NOZORDER
	}
	win.hdc, _, _ = pGetDC.Call(hwnd)
	pfd := pixelFormatDescriptor{nVersion: 1, dwFlags: 0x4 | 0x20 | 0x1, cColorBits: 32, cAlphaBits: 8, cDepthBits: 24, cStencilBits: 8}
	pfd.nSize = uint16(unsafe.Sizeof(pfd))
	pf, _, _ := pChoosePixelFormat.Call(win.hdc, uintptr(unsafe.Pointer(&pfd)))
	if pf == 0 {
		return nil, errors.New("ChoosePixelFormat failed")
	}
	if r, _, _ := pSetPixelFormat.Call(win.hdc, pf, uintptr(unsafe.Pointer(&pfd))); r == 0 {
		return nil, errors.New("SetPixelFormat failed")
	}
	win.hglrc, _, _ = pWglCreateContext.Call(win.hdc)
	if win.hglrc == 0 {
		return nil, errors.New("wglCreateContext failed")
	}
	if r, _, _ := pWglMakeCurrent.Call(win.hdc, win.hglrc); r == 0 {
		return nil, errors.New("wglMakeCurrent failed")
	}
	pDragAcceptFiles.Call(hwnd, 1)
	var rc rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	win.W, win.H = int(rc.right-rc.left), int(rc.bottom-rc.top)
	win.cursor, _, _ = pLoadCursorW.Call(0, CursorArrow)
	if visible {
		pShowWindow.Call(hwnd, 1)
	}
	return win, nil
}

// GetProcAddress resolves an OpenGL function.
func GetProcAddress(name string) uintptr {
	n, _ := syscall.BytePtrFromString(name)
	p, _, _ := pWglGetProcAddress.Call(uintptr(unsafe.Pointer(n)))
	if p == 0 || p == 1 || p == 2 || p == 3 || p == ^uintptr(0) {
		if proc := opengl32.NewProc(name); proc.Find() == nil {
			return proc.Addr()
		}
		return 0
	}
	return p
}

func (w *Window) Handler(h Handler) { w.h = h }

// Redraw schedules a frame.
func (w *Window) Redraw() { w.dirty = true }

// Wake can be called from any goroutine to make the loop run Idle.
func (w *Window) Wake() { pPostMessageW.Call(w.hwnd, wmApp, 0, 0) }

func (w *Window) SetTitle(s string) { pSetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(utf16(s)))) }

func (w *Window) Show() { pShowWindow.Call(w.hwnd, 1) }

func (w *Window) Quit() { w.quit = true; pPostQuitMessage.Call(0) }

func (w *Window) SetCursor(id int) {
	c, _, _ := pLoadCursorW.Call(0, uintptr(id))
	if c != w.cursor {
		w.cursor = c
		pSetCursor.Call(c)
	}
}

func (w *Window) Capture(on bool) {
	if w.IgnoreInput {
		return // headless: real mouse input is ignored, so there is nothing to capture
	}
	if on {
		pSetCapture.Call(w.hwnd)
	} else {
		w.releasing = true // our own release isn't a lost capture
		pReleaseCapture.Call()
		w.releasing = false
	}
}

func (w *Window) Swap() { pSwapBuffers.Call(w.hdc) }

// guard, deferred around the handler's calls, gives OnPanic its chance
// when one panics, printing the panic's own stack first (re-panicking
// from here would show only this frame's).
func (w *Window) guard() {
	r := recover()
	if r == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "panic: %v\n\n%s\n", r, debug.Stack())
	if f := w.OnPanic; f != nil {
		w.OnPanic = nil // once: saving must not loop if it panics too
		func() {
			defer func() { recover() }()
			f(r)
		}()
	}
	panic(r)
}

// Run pumps messages until the window closes.
func (w *Window) Run() {
	defer w.guard()
	var m msg
	for !w.quit {
		for {
			r, _, _ := pPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
			if r == 0 {
				break
			}
			if m.message == wmQuit {
				return
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		if w.quit {
			return
		}
		w.h.Idle()
		if w.dirty || w.Animate {
			w.dirty = false
			w.h.Frame()
			w.Swap()
		} else {
			pWaitMessage.Call()
		}
	}
}

func mods() Mods {
	var m Mods
	ks := func(vk uintptr) bool { r, _, _ := pGetKeyState.Call(vk); return int16(r) < 0 }
	if ks(0x10) {
		m |= Shift
	}
	if ks(0x11) {
		m |= Ctrl
	}
	if ks(0x12) {
		m |= Alt
	}
	return m
}

func xy(l uintptr) (float64, float64) {
	return float64(int16(l & 0xffff)), float64(int16(l >> 16 & 0xffff))
}

func wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	w := current
	if w == nil || w.h == nil || w.hwnd == 0 {
		r, _, _ := pDefWindowProcW.Call(hwnd, message, wParam, lParam)
		return r
	}
	defer w.guard()
	if w.IgnoreInput {
		switch message {
		case wmMouseMove, wmMouseLeave, wmLButtonDown, wmLButtonDbl, wmRButtonDown, wmMButtonDown,
			wmLButtonUp, wmRButtonUp, wmMButtonUp, wmMouseWheel, wmKeyDown, wmSysKeyDown,
			wmKeyUp, wmSysKeyUp, wmChar, wmDropFiles, wmCaptureChanged: // (headless: the mouse is simulated, never captured)
			r, _, _ := pDefWindowProcW.Call(hwnd, message, wParam, lParam)
			return r
		}
	}
	switch message {
	case wmClose:
		if w.h.Close() {
			w.quit = true
			pDestroyWindow.Call(hwnd)
		}
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		w.h.Frame()
		w.Swap()
		pValidateRect.Call(hwnd, 0)
		return 0
	case wmSize:
		w.W, w.H = int(lParam&0xffff), int(lParam>>16&0xffff)
		if w.W > 0 && w.H > 0 {
			w.h.Resize(w.W, w.H)
			w.dirty = true
		}
		return 0
	case wmCaptureChanged:
		if !w.releasing && lParam != hwnd {
			w.h.CaptureLost()
		}
		return 0
	case wmGetMinMaxInfo:
		if w.MinW > 0 || w.MinH > 0 {
			rc := rect{0, 0, int32(float64(w.MinW) * w.DPI), int32(float64(w.MinH) * w.DPI)}
			pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&rc)), 0x00CF0000, 0, 0x10)
			mmi := (*[10]int32)(unsafe.Pointer(lParam)) // MINMAXINFO: ptMinTrackSize is the 4th point
			mmi[6], mmi[7] = rc.right-rc.left, rc.bottom-rc.top
			return 0
		}
	case wmDpiChanged:
		if !w.FixedDPI {
			// the suggested rectangle keeps the window's size in logical
			// pixels at the new scale; with a fixed scale its pixel size
			// already does
			w.DPI = float64(wParam&0xffff) / 96
			rc := (*rect)(unsafe.Pointer(lParam))
			pSetWindowPos.Call(hwnd, 0, uintptr(rc.left), uintptr(rc.top), uintptr(rc.right-rc.left), uintptr(rc.bottom-rc.top), 0x0004)
		}
		w.dirty = true
		return 0
	case wmSetCursor:
		if lParam&0xffff == 1 { // HTCLIENT
			pSetCursor.Call(w.cursor)
			return 1
		}
	case wmMouseMove:
		if !w.tracking {
			tme := struct {
				size, flags uint32
				hwnd        uintptr
				hover       uint32
			}{flags: 0x2, hwnd: hwnd}
			tme.size = uint32(unsafe.Sizeof(tme))
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			w.tracking = true
		}
		x, y := xy(lParam)
		w.h.MouseMove(x, y, mods())
		return 0
	case wmMouseLeave:
		w.tracking = false
		w.h.MouseLeave()
		return 0
	case wmLButtonDown, wmLButtonDbl, wmRButtonDown, wmMButtonDown:
		x, y := xy(lParam)
		btn := map[uintptr]int{wmLButtonDown: Left, wmLButtonDbl: Left, wmRButtonDown: Right, wmMButtonDown: Middle}[message]
		w.Capture(true)
		w.h.MouseDown(btn, x, y, mods(), message == wmLButtonDbl)
		return 0
	case wmLButtonUp, wmRButtonUp, wmMButtonUp:
		x, y := xy(lParam)
		btn := map[uintptr]int{wmLButtonUp: Left, wmRButtonUp: Right, wmMButtonUp: Middle}[message]
		if wParam&(0x1|0x2|0x10) == 0 { // MK_LBUTTON, MK_RBUTTON, MK_MBUTTON: no button still down
			w.Capture(false)
		}
		w.h.MouseUp(btn, x, y, mods())
		return 0
	case wmMouseWheel:
		d := float64(int16(wParam>>16&0xffff)) / 120
		x, y := xy(lParam)
		// wheel coordinates are screen coordinates; convert to client
		pt := struct{ x, y int32 }{int32(x), int32(y)}
		user32.NewProc("ScreenToClient").Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		w.h.MouseWheel(d, float64(pt.x), float64(pt.y), mods())
		return 0
	case wmKeyDown, wmSysKeyDown:
		w.h.KeyDown(int(wParam), mods(), lParam&(1<<30) != 0)
		if message == wmSysKeyDown && wParam != 0x73 { // keep Alt+F4
			return 0
		}
		if message == wmKeyDown {
			return 0
		}
	case wmKeyUp, wmSysKeyUp:
		w.h.KeyUp(int(wParam), mods())
		return 0
	case 0x0106: // WM_SYSCHAR: Alt+letter shortcuts are handled in KeyDown
		return 0
	case wmChar:
		// a character outside the basic plane (an emoji) comes as a
		// surrogate pair, one half per message
		r := rune(wParam)
		switch {
		case r >= 0xD800 && r < 0xDC00:
			w.hiSurr = r
			return 0
		case r >= 0xDC00 && r < 0xE000:
			if w.hiSurr == 0 {
				return 0
			}
			r = (w.hiSurr-0xD800)<<10 + (r - 0xDC00) + 0x10000
		}
		w.hiSurr = 0
		w.h.Char(r)
		return 0
	case wmDropFiles:
		n, _, _ := pDragQueryFileW.Call(wParam, 0xFFFFFFFF, 0, 0)
		var paths []string
		for i := uintptr(0); i < n; i++ {
			l, _, _ := pDragQueryFileW.Call(wParam, i, 0, 0) // its length, without the terminator
			buf := make([]uint16, l+1)
			pDragQueryFileW.Call(wParam, i, uintptr(unsafe.Pointer(&buf[0])), l+1)
			paths = append(paths, syscall.UTF16ToString(buf))
		}
		pDragFinish.Call(wParam)
		w.h.DropFiles(paths, -1, -1)
		return 0
	case wmApp, wmTimer:
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

// --- clipboard -----------------------------------------------------------

func (w *Window) SetClipboard(s string) {
	if r, _, _ := pOpenClipboard.Call(w.hwnd); r == 0 {
		return
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	// a NUL would end the text early (and UTF16FromString refuses it)
	u, _ := syscall.UTF16FromString(strings.ReplaceAll(s, "\x00", ""))
	h, _, _ := pGlobalAlloc.Call(0x0002, uintptr(len(u)*2))
	if h == 0 {
		return
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		pGlobalFree.Call(h)
		return
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u)), u)
	pGlobalUnlock.Call(h)
	if r, _, _ := pSetClipboardData.Call(13, h); r == 0 {
		pGlobalFree.Call(h) // the clipboard didn't take it
	}
}

func (w *Window) Clipboard() string {
	if r, _, _ := pOpenClipboard.Call(w.hwnd); r == 0 {
		return ""
	}
	defer pCloseClipboard.Call()
	h, _, _ := pGetClipboardData.Call(13)
	if h == 0 {
		return ""
	}
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer pGlobalUnlock.Call(h)
	// up to the terminator, and never past the block (another program's
	// text may lack one)
	size, _, _ := pGlobalSize.Call(h)
	limit := int(size / 2)
	var n int
	for n < limit && *(*uint16)(unsafe.Pointer(p + uintptr(n*2))) != 0 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(p)), n))
}

// --- dialogs -------------------------------------------------------------

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	Flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	FlagsEx           uint32
}

// filter pairs: "Description", "*.ext;*.ext2", ...
func fileDialog(w *Window, save bool, title, initial, defExt string, filter ...string) (string, bool) {
	var f []uint16
	for _, s := range filter {
		u, _ := syscall.UTF16FromString(s)
		f = append(f, u...)
	}
	f = append(f, 0)
	buf := make([]uint16, 4096)
	if initial != "" {
		u, _ := syscall.UTF16FromString(initial)
		copy(buf, u)
	}
	ofn := openFileName{hwndOwner: w.hwnd, lpstrFilter: &f[0], lpstrFile: &buf[0], nMaxFile: uint32(len(buf)), lpstrTitle: utf16(title), nFilterIndex: 1}
	if defExt != "" {
		ofn.lpstrDefExt = utf16(defExt)
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	var r uintptr
	if save {
		ofn.Flags = 0x2 | 0x800 // OVERWRITEPROMPT|PATHMUSTEXIST
		r, _, _ = pGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	} else {
		ofn.Flags = 0x1000 | 0x800 // FILEMUSTEXIST|PATHMUSTEXIST
		r, _, _ = pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	}
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

func (w *Window) OpenFile(title string, filter ...string) (string, bool) {
	return fileDialog(w, false, title, "", "", filter...)
}

func (w *Window) SaveFile(title, initial, defExt string, filter ...string) (string, bool) {
	return fileDialog(w, true, title, initial, defExt, filter...)
}

// Message shows a message box. kind: 0 OK, 4 Yes/No, 3 Yes/No/Cancel.
// Returns 1 OK, 6 yes, 7 no, 2 cancel.
func (w *Window) Message(title, text string, kind int) int {
	r, _, _ := pMessageBoxW.Call(w.hwnd, uintptr(unsafe.Pointer(utf16(text))), uintptr(unsafe.Pointer(utf16(title))), uintptr(kind))
	return int(r)
}

// Message box kinds and results.
const (
	MsgOK          = 0
	MsgYesNoCancel = 3
	MsgYesNo       = 4
	IDOK           = 1
	IDCancel       = 2
	IDYes          = 6
	IDNo           = 7
)

// ErrorBox shows a message box without a parent window.
func ErrorBox(title, text string) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(utf16(text))), uintptr(unsafe.Pointer(utf16(title))), 0x10)
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

// SetIcon sets the window's icon from size×size RGBA pixels (top row
// first, straight alpha). big selects the taskbar/Alt-Tab icon; otherwise
// the small title-bar icon.
func (w *Window) SetIcon(rgba []byte, size int, big bool) {
	if len(rgba) != size*size*4 {
		return
	}
	bgra := make([]byte, len(rgba))
	for i := 0; i < len(rgba); i += 4 {
		bgra[i], bgra[i+1], bgra[i+2], bgra[i+3] = rgba[i+2], rgba[i+1], rgba[i], rgba[i+3]
	}
	color, _, _ := pCreateBitmap.Call(uintptr(size), uintptr(size), 1, 32, uintptr(unsafe.Pointer(&bgra[0])))
	mask := make([]byte, ((size+15)/16*2)*size) // all zero: alpha decides
	maskBmp, _, _ := pCreateBitmap.Call(uintptr(size), uintptr(size), 1, 1, uintptr(unsafe.Pointer(&mask[0])))
	if color == 0 || maskBmp == 0 {
		return
	}
	ii := iconInfo{fIcon: 1, hbmMask: maskBmp, hbmColor: color}
	icon, _, _ := pCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	pDeleteObject.Call(color)
	pDeleteObject.Call(maskBmp)
	if icon == 0 {
		return
	}
	which := uintptr(0) // ICON_SMALL
	if big {
		which = 1 // ICON_BIG
	}
	if old, _, _ := pSendMessageW.Call(w.hwnd, 0x0080 /* WM_SETICON */, which, icon); old != 0 {
		pDestroyIcon.Call(old) // the one it replaces (ours, made here)
	}
}

// OpenURL opens a web address (or file) with the user's default handler.
func OpenURL(url string) {
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(utf16("open"))), uintptr(unsafe.Pointer(utf16(url))), 0, 0, 1)
}
