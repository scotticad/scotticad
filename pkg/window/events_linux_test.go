//go:build linux

package window

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

// The X event handling, fed synthetic events (no X server): the two Xlib
// key lookups it calls are replaced by a small keyboard map, and a
// recording handler shows what the app would see.

// recorder is a Handler that writes down every call.
type recorder struct{ calls []string }

func (r *recorder) add(f string, a ...any) { r.calls = append(r.calls, fmt.Sprintf(f, a...)) }
func (r *recorder) take() []string         { c := r.calls; r.calls = nil; return c }

func (r *recorder) MouseMove(x, y float64, m Mods) { r.add("move %v,%v %d", x, y, m) }
func (r *recorder) MouseDown(b int, x, y float64, m Mods, double bool) {
	r.add("down %d %v,%v %d double=%v", b, x, y, m, double)
}
func (r *recorder) MouseUp(b int, x, y float64, m Mods)        { r.add("up %d %v,%v %d", b, x, y, m) }
func (r *recorder) MouseWheel(d float64, x, y float64, m Mods) { r.add("wheel %v", d) }
func (r *recorder) MouseLeave()                                { r.add("leave") }
func (r *recorder) KeyDown(vk int, m Mods, repeat bool) {
	r.add("keydown %#x %d repeat=%v", vk, m, repeat)
}
func (r *recorder) KeyUp(vk int, m Mods)               { r.add("keyup %#x %d", vk, m) }
func (r *recorder) Char(c rune)                        { r.add("char %q", c) }
func (r *recorder) Resize(w, h int)                    { r.add("resize %dx%d", w, h) }
func (r *recorder) DropFiles(p []string, x, y float64) {}
func (r *recorder) Frame()                             {}
func (r *recorder) Idle()                              {}
func (r *recorder) Close() bool                        { return true }
func (r *recorder) CaptureLost()                       { r.add("capturelost") }

// keycode: the key's unshifted keysym and the one Shift gives
var keyboard = map[uint32][2]uint64{
	38: {'a', 'A'}, 53: {'x', 'X'}, 54: {'c', 'C'}, 10: {'1', '!'},
	50: {0xffe1, 0xffe1}, // Shift_L
	37: {0xffe3, 0xffe3}, // Control_L
	64: {0xffe9, 0xffe9}, // Alt_L
	36: {0xff0d, 0xff0d}, // Return
}

// fakeKeyboard stands in for XLookupKeysym and XLookupString.
func fakeKeyboard(t *testing.T) {
	sym, str := xLookupKeysym, xLookupString
	t.Cleanup(func() { xLookupKeysym, xLookupString = sym, str })
	xLookupKeysym = func(ev unsafe.Pointer, index int32) uint64 {
		return keyboard[(*xEvent)(ev).detail()][0]
	}
	xLookupString = func(ev unsafe.Pointer, buf *byte, n int32, keysym *uint64, status unsafe.Pointer) int32 {
		e := (*xEvent)(ev)
		k := keyboard[e.detail()]
		*keysym = k[0]
		if e.state()&stateShift != 0 {
			*keysym = k[1]
		}
		return 0
	}
}

func newTestWindow() (*Window, *recorder) {
	r := &recorder{}
	return &Window{h: r, keysDown: map[uint32]bool{}}, r
}

// event builds an X event: key, button and motion events share their
// layout (time 56, x and y 64 and 68, state 80, keycode or button 84),
// and a crossing event keeps its mode where they keep the state.
func event(typ int32, detail uint32, state uint32, x, y int32, time uint64) *xEvent {
	var e xEvent
	p := unsafe.Pointer(&e)
	*(*int32)(p) = typ
	*(*uint64)(unsafe.Add(p, 56)) = time
	*(*int32)(unsafe.Add(p, 64)) = x
	*(*int32)(unsafe.Add(p, 68)) = y
	*(*uint32)(unsafe.Add(p, 80)) = state
	*(*uint32)(unsafe.Add(p, 84)) = detail
	return &e
}

func key(down bool, code, state uint32) *xEvent {
	t := int32(evKeyRelease)
	if down {
		t = evKeyPress
	}
	return event(t, code, state, 0, 0, 0)
}

func expect(t *testing.T, what string, r *recorder, want ...string) {
	t.Helper()
	if got := r.take(); !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got  %q\n want %q", what, got, want)
	}
}

// Keys: the app gets the key (unshifted) and the character it types; a
// held key repeats as presses marked repeat; Ctrl and Alt make shortcuts
// that type nothing; Shift counts from its press until its release.
func TestKeyEvents(t *testing.T) {
	fakeKeyboard(t)
	w, r := newTestWindow()

	w.handle(key(true, 38, 0))
	expect(t, "a", r, "keydown 0x41 0 repeat=false", "char 'a'")
	w.handle(key(true, 38, 0))
	expect(t, "a held", r, "keydown 0x41 0 repeat=true", "char 'a'")
	w.handle(key(false, 38, 0))
	expect(t, "a released", r, "keyup 0x41 0")
	w.handle(key(true, 38, 0))
	expect(t, "a again: not a repeat", r, "keydown 0x41 0 repeat=false", "char 'a'")
	w.handle(key(false, 38, 0))
	r.take()

	// Shift: held from its own press (X reports the state before it), and
	// gone at its release
	w.handle(key(true, 50, 0))
	expect(t, "Shift down", r, "keydown 0x10 1 repeat=false")
	w.handle(key(true, 10, stateShift))
	expect(t, "Shift+1", r, "keydown 0x31 1 repeat=false", "char '!'")
	w.handle(key(false, 10, stateShift))
	w.handle(key(false, 50, stateShift))
	expect(t, "Shift up", r, "keyup 0x31 1", "keyup 0x10 0")

	// Ctrl+C and Alt+X are shortcuts: no character
	w.handle(key(true, 54, stateControl))
	expect(t, "Ctrl+C", r, "keydown 0x43 2 repeat=false")
	w.handle(key(true, 53, stateMod1))
	expect(t, "Alt+X", r, "keydown 0x58 4 repeat=false")

	// Return is a key, not a character
	w.handle(key(true, 36, 0))
	expect(t, "Return", r, "keydown 0xd 0 repeat=false")
}

// Losing the focus forgets which keys are down: a key held when the
// window lost it isn't taken for a repeat when it comes back.
func TestFocusOutForgetsHeldKeys(t *testing.T) {
	fakeKeyboard(t)
	w, r := newTestWindow()
	w.handle(key(true, 38, 0))
	w.handle(event(evFocusOut, 0, 0, 0, 0, 0))
	r.take()
	w.handle(key(true, 38, 0))
	expect(t, "a after a focus change", r, "keydown 0x41 0 repeat=false", "char 'a'")
}

// Buttons: X's 1, 2, 3 are left, middle and right; 4 and 5 are the wheel,
// one notch each, and never a press. Two left presses close in time and
// place are a double click, and a third starts over.
func TestButtonEvents(t *testing.T) {
	w, r := newTestWindow()
	press := func(b uint32, x, y int32, ms uint64) { w.handle(event(evButtonPress, b, 0, x, y, ms)) }
	release := func(b uint32, x, y int32) { w.handle(event(evButtonRelease, b, 0, x, y, 0)) }

	press(1, 10, 20, 1000)
	release(1, 10, 20)
	press(1, 12, 21, 1300)
	release(1, 12, 21)
	press(1, 12, 21, 1400)
	release(1, 12, 21)
	expect(t, "three clicks", r,
		"down 0 10,20 0 double=false", "up 0 10,20 0",
		"down 0 12,21 0 double=true", "up 0 12,21 0",
		"down 0 12,21 0 double=false", "up 0 12,21 0")

	press(1, 100, 100, 5000)
	press(1, 100, 100, 5600) // too slow
	press(1, 130, 100, 5700) // too far
	expect(t, "slow and distant clicks", r,
		"down 0 100,100 0 double=false", "down 0 100,100 0 double=false", "down 0 130,100 0 double=false")
	release(1, 130, 100)
	r.take()

	press(2, 5, 5, 9000)
	press(3, 5, 5, 9010)
	release(2, 5, 5)
	release(3, 5, 5)
	expect(t, "middle and right", r,
		fmt.Sprintf("down %d 5,5 0 double=false", Middle), fmt.Sprintf("down %d 5,5 0 double=false", Right),
		fmt.Sprintf("up %d 5,5 0", Middle), fmt.Sprintf("up %d 5,5 0", Right))
	if w.buttons != 0 {
		t.Errorf("buttons still held after their releases: %b", w.buttons)
	}

	press(4, 5, 5, 9100)
	press(5, 5, 5, 9200)
	release(4, 5, 5)
	release(5, 5, 5)
	expect(t, "wheel", r, "wheel 1", "wheel -1")
	if w.buttons != 0 {
		t.Errorf("the wheel counted as a held button: %b", w.buttons)
	}
}

// Leaving the window: an ordinary leave with no button held is a
// MouseLeave; during a drag X keeps the pointer (an implicit grab), so it
// isn't one. Another program taking the pointer mid-drag (a grab), or the
// window losing the focus, ends the drag with CaptureLost.
func TestLeaveAndLostCapture(t *testing.T) {
	w, r := newTestWindow()
	leave := func(mode int32) { w.handle(event(evLeaveNotify, 0, uint32(mode), 0, 0, 0)) }

	leave(notifyNormal)
	expect(t, "leave, no drag", r, "leave")

	w.handle(event(evButtonPress, 1, 0, 1, 1, 1))
	r.take()
	leave(notifyNormal)
	expect(t, "leave mid-drag", r)
	leave(notifyGrab)
	expect(t, "grabbed mid-drag", r, "capturelost")
	if w.buttons != 0 {
		t.Errorf("buttons after a lost capture: %b", w.buttons)
	}
	leave(notifyGrab)
	expect(t, "a grab with no drag", r)

	w.handle(event(evButtonPress, 3, 0, 1, 1, 2))
	r.take()
	w.handle(event(evFocusOut, 0, 0, 0, 0, 0))
	expect(t, "focus lost mid-drag", r, "capturelost")
	w.handle(event(evFocusOut, 0, 0, 0, 0, 0))
	expect(t, "focus lost, no drag", r)
}

// A configure event resizes only when the size changes (a move sends
// them too), and never to nothing.
func TestConfigureResizes(t *testing.T) {
	w, r := newTestWindow()
	w.W, w.H = 800, 600
	configure := func(width, height int32) {
		e := event(evConfigureNotify, 0, 0, 0, 0, 0)
		*(*int32)(unsafe.Add(unsafe.Pointer(e), 56)) = width
		*(*int32)(unsafe.Add(unsafe.Pointer(e), 60)) = height
		w.handle(e)
	}
	configure(800, 600)
	configure(1024, 700)
	configure(0, 700)
	expect(t, "configure", r, "resize 1024x700")
	if w.W != 1024 || w.H != 700 || !w.dirty {
		t.Errorf("size %dx%d, dirty %v", w.W, w.H, w.dirty)
	}
}

// IgnoreInput (headless runs) drops the real mouse and keyboard, but not
// what the window itself needs, such as resizes.
func TestIgnoreInput(t *testing.T) {
	fakeKeyboard(t)
	w, r := newTestWindow()
	w.IgnoreInput = true
	w.handle(key(true, 38, 0))
	w.handle(event(evButtonPress, 1, 0, 1, 1, 1))
	w.handle(event(evMotionNotify, 0, 0, 3, 4, 0))
	w.handle(event(evLeaveNotify, 0, notifyNormal, 0, 0, 0))
	e := event(evConfigureNotify, 0, 0, 0, 0, 0)
	*(*int32)(unsafe.Add(unsafe.Pointer(e), 56)) = 640
	*(*int32)(unsafe.Add(unsafe.Pointer(e), 60)) = 480
	w.handle(e)
	if got := strings.Join(r.take(), "; "); got != "resize 640x480" {
		t.Errorf("with input ignored: %s", got)
	}
}
