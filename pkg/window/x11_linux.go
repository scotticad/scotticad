//go:build linux

package window

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The X11 and GLX entry points, called through purego (no cgo). The
// libraries are opened at run time: libX11 and libGL are on every desktop
// Linux; libXcursor (themed cursors) is optional.

var (
	xOpenDisplay               func(name *byte) uintptr
	xDefaultScreen             func(d uintptr) int32
	xRootWindow                func(d uintptr, screen int32) uintptr
	xDisplayWidth              func(d uintptr, screen int32) int32
	xDisplayHeight             func(d uintptr, screen int32) int32
	xCreateColormap            func(d, w, visual uintptr, alloc int32) uintptr
	xCreateWindow              func(d, parent uintptr, x, y int32, w, h, border uint32, depth int32, class uint32, visual uintptr, mask uint64, attrs unsafe.Pointer) uintptr
	xMapWindow                 func(d, w uintptr) int32
	xDestroyWindow             func(d, w uintptr) int32
	xStoreName                 func(d, w uintptr, name *byte) int32
	xChangeProperty            func(d, w, property, typ uintptr, format, mode int32, data unsafe.Pointer, n int32) int32
	xDeleteProperty            func(d, w, property uintptr) int32
	xGetWindowProperty         func(d, w, property uintptr, offset, length int64, del int32, reqType uintptr, actualType *uintptr, actualFormat *int32, nitems, bytesAfter *uint64, prop *uintptr) int32
	xInternAtom                func(d uintptr, name *byte, onlyIfExists int32) uintptr
	xSetWMProtocols            func(d, w uintptr, protocols *uintptr, n int32) int32
	xSetWMNormalHints          func(d, w uintptr, hints unsafe.Pointer)
	xSetClassHint              func(d, w uintptr, hint unsafe.Pointer) int32
	xPending                   func(d uintptr) int32
	xNextEvent                 func(d uintptr, ev unsafe.Pointer) int32
	xCheckTypedWindowEvent     func(d, w uintptr, typ int32, ev unsafe.Pointer) int32
	xFlush                     func(d uintptr) int32
	xConnectionNumber          func(d uintptr) int32
	xLookupString              func(ev unsafe.Pointer, buf *byte, n int32, keysym *uint64, status unsafe.Pointer) int32
	xLookupKeysym              func(ev unsafe.Pointer, index int32) uint64
	xkbSetDetectableAutoRepeat func(d uintptr, detectable int32, supported *int32) int32
	xCreateFontCursor          func(d uintptr, shape uint32) uintptr
	xDefineCursor              func(d, w, cursor uintptr) int32
	xSetSelectionOwner         func(d, selection, owner uintptr, time uint64) int32
	xGetSelectionOwner         func(d, selection uintptr) uintptr
	xConvertSelection          func(d, selection, target, property, requestor uintptr, time uint64) int32
	xSendEvent                 func(d, w uintptr, propagate int32, mask int64, ev unsafe.Pointer) int32
	xFree                      func(p uintptr) int32
	xResourceManagerString     func(d uintptr) uintptr
	xSetErrorHandler           func(handler uintptr) uintptr

	xcursorLibraryLoadCursor func(d uintptr, name *byte) uintptr // optional (libXcursor)

	glXChooseFBConfig        func(d uintptr, screen int32, attribs *int32, n *int32) uintptr
	glXGetVisualFromFBConfig func(d, cfg uintptr) uintptr
	glXCreateNewContext      func(d, cfg uintptr, renderType int32, share uintptr, direct int32) uintptr
	glXMakeCurrent           func(d, drawable, ctx uintptr) int32
	glXSwapBuffers           func(d, drawable uintptr)
	glXGetProcAddressARB     func(name *byte) uintptr
	glXQueryExtensionsString func(d uintptr, screen int32) uintptr
	glXSwapIntervalEXT       func(d, drawable uintptr, interval int32)
	glXSwapIntervalMESA      func(interval uint32) int32
	glXSwapIntervalSGI       func(interval int32) int32
)

var (
	libsLoaded bool
	libsErr    error
	libGL      uintptr
)

// loadLibs opens libX11 and libGL and binds every entry point; once.
func loadLibs() error {
	if libsLoaded {
		return libsErr
	}
	libsLoaded = true
	open := func(names ...string) (uintptr, error) {
		var err error
		for _, n := range names {
			var h uintptr
			if h, err = purego.Dlopen(n, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
				return h, nil
			}
		}
		return 0, err
	}
	x11, err := open("libX11.so.6", "libX11.so")
	if err != nil {
		libsErr = fmt.Errorf("the X11 library (libX11) is not installed: %v", err)
		return libsErr
	}
	gl, err := open("libGL.so.1", "libGL.so")
	if err != nil {
		libsErr = fmt.Errorf("the OpenGL library (libGL) is not installed: %v", err)
		return libsErr
	}
	libGL = gl
	bind := func(lib uintptr, fns map[string]any) error {
		for name, p := range fns {
			sym, err := purego.Dlsym(lib, name)
			if err != nil {
				return fmt.Errorf("%s: %v", name, err)
			}
			purego.RegisterFunc(p, sym)
		}
		return nil
	}
	if err := bind(x11, map[string]any{
		"XOpenDisplay": &xOpenDisplay, "XDefaultScreen": &xDefaultScreen,
		"XRootWindow": &xRootWindow, "XDisplayWidth": &xDisplayWidth, "XDisplayHeight": &xDisplayHeight,
		"XCreateColormap": &xCreateColormap, "XCreateWindow": &xCreateWindow, "XMapWindow": &xMapWindow,
		"XDestroyWindow": &xDestroyWindow, "XStoreName": &xStoreName, "XChangeProperty": &xChangeProperty,
		"XDeleteProperty": &xDeleteProperty, "XGetWindowProperty": &xGetWindowProperty, "XInternAtom": &xInternAtom,
		"XSetWMProtocols": &xSetWMProtocols, "XSetWMNormalHints": &xSetWMNormalHints, "XSetClassHint": &xSetClassHint,
		"XPending": &xPending, "XNextEvent": &xNextEvent, "XCheckTypedWindowEvent": &xCheckTypedWindowEvent,
		"XFlush": &xFlush, "XConnectionNumber": &xConnectionNumber, "XLookupString": &xLookupString,
		"XLookupKeysym": &xLookupKeysym, "XkbSetDetectableAutoRepeat": &xkbSetDetectableAutoRepeat,
		"XCreateFontCursor": &xCreateFontCursor, "XDefineCursor": &xDefineCursor,
		"XSetSelectionOwner": &xSetSelectionOwner, "XGetSelectionOwner": &xGetSelectionOwner,
		"XConvertSelection": &xConvertSelection, "XSendEvent": &xSendEvent, "XFree": &xFree,
		"XResourceManagerString": &xResourceManagerString, "XSetErrorHandler": &xSetErrorHandler,
	}); err != nil {
		libsErr = fmt.Errorf("libX11: %v", err)
		return libsErr
	}
	if err := bind(gl, map[string]any{
		"glXChooseFBConfig": &glXChooseFBConfig, "glXGetVisualFromFBConfig": &glXGetVisualFromFBConfig,
		"glXCreateNewContext": &glXCreateNewContext, "glXMakeCurrent": &glXMakeCurrent,
		"glXSwapBuffers": &glXSwapBuffers, "glXGetProcAddressARB": &glXGetProcAddressARB,
		"glXQueryExtensionsString": &glXQueryExtensionsString,
	}); err != nil {
		libsErr = fmt.Errorf("libGL (GLX): %v", err)
		return libsErr
	}
	if xc, err := open("libXcursor.so.1", "libXcursor.so"); err == nil {
		if sym, err := purego.Dlsym(xc, "XcursorLibraryLoadCursor"); err == nil {
			purego.RegisterFunc(&xcursorLibraryLoadCursor, sym)
		}
	}
	return nil
}

var errNoDisplay = errors.New("cannot open the X display: ScottiCAD needs an X11 desktop " +
	"(or XWayland on a Wayland desktop); is DISPLAY set?")

// cstr is s as a NUL-terminated C string (valid while the result is reachable).
func cstr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// gostr copies a NUL-terminated C string.
func gostr(p uintptr) string {
	if p == 0 {
		return ""
	}
	var b []byte
	for i := uintptr(0); ; i++ {
		c := *(*byte)(foreign(p + i))
		if c == 0 {
			return string(b)
		}
		b = append(b, c)
	}
}

// X protocol and Xlib constants used here.
const (
	evKeyPress         = 2
	evKeyRelease       = 3
	evButtonPress      = 4
	evButtonRelease    = 5
	evMotionNotify     = 6
	evEnterNotify      = 7
	evLeaveNotify      = 8
	evFocusOut         = 10
	evExpose           = 12
	evConfigureNotify  = 22
	evPropertyNotify   = 28
	evSelectionClear   = 29
	evSelectionRequest = 30
	evSelectionNotify  = 31
	evClientMessage    = 33

	maskKeyPress       = 1 << 0
	maskKeyRelease     = 1 << 1
	maskButtonPress    = 1 << 2
	maskButtonRelease  = 1 << 3
	maskEnterWindow    = 1 << 4
	maskLeaveWindow    = 1 << 5
	maskPointerMotion  = 1 << 6
	maskExposure       = 1 << 15
	maskStructure      = 1 << 17
	maskFocusChange    = 1 << 21
	maskPropertyChange = 1 << 22

	stateShift   = 1 << 0
	stateControl = 1 << 2
	stateMod1    = 1 << 3 // Alt

	notifyNormal = 0
	notifyGrab   = 1

	propModeReplace = 0
	atomAtom        = 4
	atomCardinal    = 6
	atomString      = 31
)

// xEvent is Xlib's XEvent: a union of 24 longs. The field offsets used
// below are those of the LP64 layout (amd64 and arm64 alike).
type xEvent [24]uint64

func (e *xEvent) typ() int32             { return e.i32(0) }
func (e *xEvent) i32(off uintptr) int32  { return *(*int32)(unsafe.Add(unsafe.Pointer(e), off)) }
func (e *xEvent) u32(off uintptr) uint32 { return *(*uint32)(unsafe.Add(unsafe.Pointer(e), off)) }
func (e *xEvent) u64(off uintptr) uint64 { return *(*uint64)(unsafe.Add(unsafe.Pointer(e), off)) }
func (e *xEvent) ptr() unsafe.Pointer    { return unsafe.Pointer(e) }

// Key, button and motion events share their first fields: the time at 56,
// x and y at 64 and 68, the modifier state at 80, and the keycode or
// button at 84.
func (e *xEvent) time() uint64       { return e.u64(56) }
func (e *xEvent) xy() (int32, int32) { return e.i32(64), e.i32(68) }
func (e *xEvent) state() uint32      { return e.u32(80) }
func (e *xEvent) detail() uint32     { return e.u32(84) }
