// Package window is a minimal platform host: one top-level window with an
// OpenGL context, input events, clipboard, cursors and file dialogs. The
// Windows implementation is written against the Win32 API directly, the
// Linux one against X11 and GLX through purego; neither uses cgo.
package window

import (
	"fmt"
	"os"
	"runtime/debug"
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

// Cursor identifiers (the Win32 IDC_ numbers, which the Windows
// implementation uses directly).
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

// Message box kinds and results (the Win32 MessageBox numbers).
const (
	MsgOK          = 0
	MsgYesNoCancel = 3
	MsgYesNo       = 4
	IDOK           = 1
	IDCancel       = 2
	IDYes          = 6
	IDNo           = 7
)

// Key codes passed to KeyDown and KeyUp are the Win32 virtual-key numbers
// on every platform: letters and digits are their upper-case ASCII codes,
// and the rest are the values below (the Linux implementation translates
// X keysyms to them).
const (
	KeyBack     = 0x08
	KeyTab      = 0x09
	KeyReturn   = 0x0D
	KeyShift    = 0x10
	KeyControl  = 0x11
	KeyAlt      = 0x12
	KeyEscape   = 0x1B
	KeySpace    = 0x20
	KeyPgUp     = 0x21
	KeyPgDn     = 0x22
	KeyEnd      = 0x23
	KeyHome     = 0x24
	KeyLeft     = 0x25
	KeyUp       = 0x26
	KeyRight    = 0x27
	KeyDown     = 0x28
	KeyInsert   = 0x2D
	KeyDelete   = 0x2E
	KeyAdd      = 0x6B // numeric keypad +
	KeySubtract = 0x6D // numeric keypad -
	KeyF1       = 0x70 // F1 to F12 are KeyF1 to KeyF1+11
	KeyPlus     = 0xBB // the = / + key
	KeyComma    = 0xBC
	KeyMinus    = 0xBD // the - / _ key
	KeyPeriod   = 0xBE
	KeySlash    = 0xBF
)

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
