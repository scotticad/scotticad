package app

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/scotticad/scotticad/pkg/window"
)

// typeAll types s a character at a time, as WM_CHAR would.
func typeAll(e *codeEdit, s string) {
	for _, r := range s {
		e.typeText(string(r))
	}
}

func keyOf(e *codeEdit, vk int, m window.Mods, clip *string) {
	e.key(vk, m, 3, func() string { return *clip }, func(s string) { *clip = s })
}

// Typing over a selection starts an undo step of its own: after Ctrl+A,
// undo brings back what was selected, not what was there before it.
func TestCodeEditTypingOverASelection(t *testing.T) {
	clip := ""
	e := newCodeEdit()
	typeAll(e, "abc")
	keyOf(e, 'A', window.Ctrl, &clip)
	typeAll(e, "xy")
	if e.text != "xy" {
		t.Fatalf("text %q", e.text)
	}
	keyOf(e, 'Z', window.Ctrl, &clip)
	if e.text != "abc" {
		t.Fatalf("undo gave %q, want the selected text back", e.text)
	}
	keyOf(e, 'Z', window.Ctrl, &clip)
	if e.text != "" {
		t.Fatalf("second undo gave %q", e.text)
	}
	keyOf(e, 'Y', window.Ctrl, &clip)
	keyOf(e, 'Y', window.Ctrl, &clip)
	if e.text != "xy" {
		t.Fatalf("redo gave %q", e.text)
	}
}

// Tab and Shift+Tab indent and dedent every line the selection touches;
// Tab without a line break in the selection goes to the next indent stop
// from where the spaces go in.
func TestCodeEditIndent(t *testing.T) {
	clip := ""
	e := newCodeEdit()
	e.set("a\n  b\n\nc")
	e.anchor, e.cur = 0, len(e.text)
	keyOf(e, vkTab, 0, &clip)
	if e.text != "    a\n      b\n\n    c" {
		t.Fatalf("indent: %q", e.text)
	}
	keyOf(e, vkTab, window.Shift, &clip)
	keyOf(e, vkTab, window.Shift, &clip)
	if e.text != "a\nb\n\nc" {
		t.Fatalf("dedent twice: %q", e.text)
	}
	// a selection ending at the start of a line leaves that line alone
	e.set("x\ny\n")
	e.anchor, e.cur = 0, 2
	keyOf(e, vkTab, 0, &clip)
	if e.text != "x\ny\n" && e.text != "    x\ny\n" {
		t.Fatalf("indent of one line by its break: %q", e.text)
	}
	e.set("ab cd")
	e.anchor, e.cur = 1, 4 // "b c"
	keyOf(e, vkTab, 0, &clip)
	if e.text != "a   d" {
		t.Fatalf("tab over a selection: %q (to column 4 from column 1)", e.text)
	}
}

// Ctrl+arrows jump by words, Home goes to the indentation then the line
// start, and Up/Down keep the column across shorter lines.
func TestCodeEditMoves(t *testing.T) {
	clip := ""
	e := newCodeEdit()
	e.set("    foo_bar(baz, 12)")
	keyOf(e, vkLeft, window.Ctrl, &clip)
	if e.cur != 17 {
		t.Fatalf("ctrl+left to %d, want 17 (the 12)", e.cur)
	}
	keyOf(e, vkLeft, window.Ctrl, &clip)
	keyOf(e, vkLeft, window.Ctrl, &clip)
	if e.cur != 4 {
		t.Fatalf("ctrl+left twice more to %d, want 4 (foo_bar)", e.cur)
	}
	keyOf(e, vkRight, window.Ctrl, &clip)
	if e.cur != 11 {
		t.Fatalf("ctrl+right to %d, want 11 (after foo_bar)", e.cur)
	}
	keyOf(e, vkHome, 0, &clip)
	if e.cur != 4 {
		t.Fatalf("home to %d, want the indentation", e.cur)
	}
	keyOf(e, vkHome, 0, &clip)
	if e.cur != 0 {
		t.Fatalf("home again to %d, want the line start", e.cur)
	}
	e.set("long line here\nab\nanother long line")
	e.cur, e.anchor = 10, 10
	keyOf(e, vkDown, 0, &clip)
	keyOf(e, vkDown, 0, &clip)
	if line, s := e.lineOf(e.cur); line != 2 || e.cur-s != 10 {
		t.Fatalf("down twice: line %d col %d, want line 2 col 10", line, e.cur-s)
	}
	keyOf(e, vkUp, window.Shift, &clip)
	if !e.hasSel() {
		t.Fatal("shift+up made no selection")
	}
	keyOf(e, vkEnd, window.Ctrl, &clip)
	if e.cur != len(e.text) || e.hasSel() {
		t.Fatalf("ctrl+end: %d, selection %v", e.cur, e.hasSel())
	}
}

// Backspace in the indentation goes back to the previous indent stop;
// Ctrl+Backspace and Ctrl+Delete take a word.
func TestCodeEditDeletes(t *testing.T) {
	clip := ""
	e := newCodeEdit()
	e.set("      x")
	e.cur, e.anchor = 6, 6
	keyOf(e, vkBack, 0, &clip)
	if e.text != "    x" {
		t.Fatalf("backspace in the indentation: %q", e.text)
	}
	e.set("alpha beta")
	keyOf(e, vkBack, window.Ctrl, &clip)
	if e.text != "alpha " {
		t.Fatalf("ctrl+backspace: %q", e.text)
	}
	e.cur, e.anchor = 0, 0
	keyOf(e, vkDelete, window.Ctrl, &clip)
	if e.text != " " {
		t.Fatalf("ctrl+delete: %q", e.text)
	}
}

// Random keystrokes never put the cursor outside the text or inside a
// character, and undo and redo walk the history exactly.
func TestCodeEditRandomKeys(t *testing.T) {
	keys := []struct {
		vk int
		m  window.Mods
	}{
		{vkLeft, 0}, {vkRight, 0}, {vkUp, 0}, {vkDown, 0}, {vkLeft, window.Shift}, {vkRight, window.Shift},
		{vkUp, window.Shift}, {vkDown, window.Shift}, {vkLeft, window.Ctrl}, {vkRight, window.Ctrl | window.Shift},
		{vkHome, 0}, {vkEnd, 0}, {vkHome, window.Ctrl}, {vkEnd, window.Ctrl | window.Shift}, {vkPgUp, 0}, {vkPgDn, window.Shift},
		{vkBack, 0}, {vkBack, window.Ctrl}, {vkDelete, 0}, {vkDelete, window.Ctrl}, {vkTab, 0}, {vkTab, window.Shift},
		{'A', window.Ctrl}, {'C', window.Ctrl}, {'X', window.Ctrl}, {'V', window.Ctrl},
	}
	pieces := []string{"a", "b", " ", "_", "é", "🎉", "×", "1", ":", "\n", "def f():", "\t", "x\r\ny", "    "}
	for seed := int64(1); seed <= 40; seed++ {
		r := rand.New(rand.NewSource(seed))
		e := newCodeEdit()
		clip := "pasted\ttext\n"
		var log []string
		check := func(what string) {
			bad := ""
			switch {
			case e.cur < 0 || e.cur > len(e.text) || e.anchor < 0 || e.anchor > len(e.text):
				bad = fmt.Sprintf("cursor %d, anchor %d outside %d bytes", e.cur, e.anchor, len(e.text))
			case !utf8.ValidString(e.text):
				bad = "text isn't valid UTF-8"
			case e.cur < len(e.text) && !utf8.RuneStart(e.text[e.cur]):
				bad = fmt.Sprintf("cursor %d inside a character", e.cur)
			case e.anchor < len(e.text) && !utf8.RuneStart(e.text[e.anchor]):
				bad = fmt.Sprintf("anchor %d inside a character", e.anchor)
			case strings.ContainsAny(e.text, "\t\r"):
				bad = "a tab or carriage return got in"
			}
			if bad != "" {
				t.Fatalf("seed %d after %s: %s\ntext %q\nsteps: %s", seed, what, bad, e.text, strings.Join(log, " "))
			}
		}
		capped := false // the undo history reached its limit (the oldest steps went)
		for i := 0; i < 300; i++ {
			if r.Intn(3) == 0 {
				p := pieces[r.Intn(len(pieces))]
				if len(p) == 1 || p == "é" || p == "🎉" || p == "×" {
					e.typeText(p)
				} else {
					e.paste(p)
				}
				log = append(log, fmt.Sprintf("type%q", p))
			} else if r.Intn(8) == 0 {
				e.newline()
				log = append(log, "enter")
			} else {
				k := keys[r.Intn(len(keys))]
				keyOf(e, k.vk, k.m, &clip)
				log = append(log, fmt.Sprintf("key(%d,%d)", k.vk, k.m))
			}
			check(log[len(log)-1])
			capped = capped || len(e.undo) >= 300
		}
		// undo all the way back gives the empty text, and redo the final
		final := e.text
		for n := 0; len(e.undo) > 0; n++ {
			keyOf(e, 'Z', window.Ctrl, &clip)
			check("undo")
			if n > 400 {
				t.Fatalf("seed %d: undo never ends", seed)
			}
		}
		if !capped && e.text != "" {
			t.Fatalf("seed %d: undoing everything left %q", seed, e.text)
		}
		for len(e.redo) > 0 {
			keyOf(e, 'Y', window.Ctrl, &clip)
			check("redo")
		}
		if e.text != final {
			t.Fatalf("seed %d: redoing everything gave %q, not %q", seed, e.text, final)
		}
	}
}
