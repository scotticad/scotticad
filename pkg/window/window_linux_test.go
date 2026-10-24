//go:build linux

package window

import (
	"reflect"
	"testing"
)

// X keysyms become the key codes the app knows (the Win32 numbers),
// whatever the shift state: the unshifted keysym is what's translated.
func TestKeysymToKey(t *testing.T) {
	for sym, want := range map[uint64]int{
		'a': 'A', 'z': 'Z', 'Q': 'Q', '7': '7',
		0xff09: KeyTab, 0xfe20: KeyTab, // Tab, Shift+Tab
		0xff0d: KeyReturn, 0xff8d: KeyReturn, // Return, keypad Enter
		0xff1b: KeyEscape, 0xff08: KeyBack, 0xffff: KeyDelete, 0xff9f: KeyDelete,
		0xff51: KeyLeft, 0xff96: KeyLeft, 0xff52: KeyUp, 0xff53: KeyRight, 0xff54: KeyDown,
		0xff50: KeyHome, 0xff57: KeyEnd, 0xff55: KeyPgUp, 0xff56: KeyPgDn,
		0xffbe: KeyF1, 0xffc2: KeyF1 + 4, // F1, F5
		'=': KeyPlus, '+': KeyPlus, '-': KeyMinus, 0xffab: KeyAdd, 0xffad: KeySubtract,
		0xffe1: KeyShift, 0xffe3: KeyControl, 0xffe9: KeyAlt, ' ': KeySpace,
		0xfe03: 0, // ISO_Level3_Shift (AltGr): no key code
	} {
		if got := keysymToKey(sym); got != want {
			t.Errorf("keysym %#x: key %#x, want %#x", sym, got, want)
		}
	}
}

// A modifier key counts as held from its press and not after its release
// (X reports the state before the event), and other keys keep the state.
func TestKeyMods(t *testing.T) {
	for _, c := range []struct {
		vk    int
		state uint32
		down  bool
		want  Mods
	}{
		{KeyShift, 0, true, Shift},
		{KeyShift, stateShift, false, 0},
		{KeyControl, stateShift, true, Shift | Ctrl},
		{KeyControl, stateShift | stateControl, false, Shift},
		{KeyAlt, stateMod1, false, 0},
		{'A', stateShift | stateControl, true, Shift | Ctrl},
		{'A', stateShift, false, Shift},
	} {
		if got := keyMods(c.vk, c.state, c.down); got != c.want {
			t.Errorf("key %#x, state %#x, down %v: %v, want %v", c.vk, c.state, c.down, got, c.want)
		}
	}
}

// The characters keysyms type: Latin-1 directly, the rest as Unicode.
func TestKeysymToRune(t *testing.T) {
	for sym, want := range map[uint64]rune{
		'a': 'a', 'Z': 'Z', ' ': ' ', 0xe9: 'é', 0x01000394: 'Δ', 0x0100263a: '☺',
		0xffb7: '7', 0xffae: '.', 0x20ac: '€',
		0xff0d: 0, 0xff08: 0, // Return and Backspace type nothing
	} {
		if got := keysymToRune(sym); got != want {
			t.Errorf("keysym %#x: %q, want %q", sym, got, want)
		}
	}
}

// The desktop's scale: Xft.dpi first, then GDK_SCALE, in quarter steps.
func TestScaleFrom(t *testing.T) {
	for _, c := range []struct {
		res, gdk string
		want     float64
	}{
		{"", "", 1},
		{"Xft.dpi:\t96\nXft.antialias:\t1\n", "", 1},
		{"Xft.antialias:\t1\nXft.dpi:\t192\n", "", 2},
		{"Xft.dpi:\t144", "", 1.5},
		{"Xft.dpi: 120", "", 1.25},
		{"Xft.dpi:\t101", "", 1}, // a quarter step: 1.05 rounds to 1
		{"", "2", 2},
		{"Xft.dpi:\t192", "3", 2}, // Xft.dpi wins
		{"Xft.dpi:\t9999", "", 4}, // clamped
		{"Xft.dpi:\tnonsense", "x", 1},
	} {
		if got := scaleFrom(c.res, c.gdk); got != c.want {
			t.Errorf("scaleFrom(%q, %q) = %v, want %v", c.res, c.gdk, got, c.want)
		}
	}
}

// The Windows-style filter pairs become filters with their patterns.
func TestParseFilters(t *testing.T) {
	got := parseFilters([]string{"ScottiCAD designs (*.scotticad)", "*.scotticad", "Meshes", "*.stl; *.obj;*.3mf", "odd"})
	want := []filterSpec{
		{"ScottiCAD designs (*.scotticad)", []string{"*.scotticad"}},
		{"Meshes", []string{"*.stl", "*.obj", "*.3mf"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseFilters: %+v", got)
	}
}
