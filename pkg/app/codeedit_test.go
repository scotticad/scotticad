//go:build windows

package app

import (
	"strings"
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/win"
)

// The highlighter covers every byte of a line and always finishes, whatever
// the line holds: punctuation outside ASCII, emoji, invalid UTF-8.
func TestCodeSpansCoverEveryLine(t *testing.T) {
	for _, line := range []string{
		`print(“hi”)`, `x = 3 × 4`, `total = 5 € # cost`, `s = "naïve"`, "\xff\xfe = 1", `emoji = 🎉 + 1`,
		`cad.add_shape("box", x=10)`, `def f(n): return n * 2  # done`, ``, `"unterminated`,
	} {
		done := make(chan string)
		go func() {
			var sb strings.Builder
			for _, s := range codeSpans(line) {
				sb.WriteString(s.s)
			}
			done <- sb.String()
		}()
		select {
		case got := <-done:
			if got != line {
				t.Errorf("spans of %q join to %q", line, got)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("highlighting %q never finished", line)
		}
	}
}

// Plain keys stay with the console; Ctrl/Alt combinations and function
// keys go on to the app.
func TestConsoleKeysPassThrough(t *testing.T) {
	for _, c := range []struct {
		vk   int
		m    win.Mods
		want bool
	}{
		{'H', 0, false}, {'L', win.Shift, false}, {'K', 0, false}, {vkDelete, 0, false},
		{'S', win.Ctrl, true}, {'F', win.Alt, true}, {vkF1, 0, true}, {vkF5 + 7, 0, true},
	} {
		if got := passesThrough(c.vk, c.m); got != c.want {
			t.Errorf("passesThrough(%q, %v) = %v", rune(c.vk), c.m, got)
		}
	}
}

// A new line keeps tab indentation, Ctrl+X of the last line leaves no
// blank line, a pasted character undoes on its own, and cad's methods are
// coloured only on cad itself.
func TestCodeEditDetails(t *testing.T) {
	clip := ""
	get, set := func() string { return clip }, func(s string) { clip = s }
	e := newCodeEdit()
	e.set("if x:\n\ty = 1")
	e.newline()
	if e.text != "if x:\n\ty = 1\n\t" {
		t.Fatalf("tab indent lost: %q", e.text)
	}
	e.set("one\ntwo")
	e.key('X', win.Ctrl, 10, get, set)
	if e.text != "one" || clip != "two\n" {
		t.Fatalf("cut last line: text %q, clip %q", e.text, clip)
	}
	e.set("ab")
	e.typeText("c")
	clip = "d"
	e.key('V', win.Ctrl, 10, get, set)
	e.key('Z', win.Ctrl, 10, get, set)
	if e.text != "abc" {
		t.Fatalf("undoing the paste gave %q", e.text)
	}
	for line, want := range map[string]bool{"cad.add_shape(1)": true, "mycad.add_shape(1)": false, "x = cad.node(1)": true} {
		got := false
		for _, sp := range codeSpans(line) {
			if sp.s == "add_shape" || sp.s == "node" {
				got = sp.c == colCodeName
			}
		}
		if got != want {
			t.Errorf("%q: coloured as a cad method %v, want %v", line, got, want)
		}
	}
}

func TestCodeEditBasics(t *testing.T) {
	e := newCodeEdit()
	e.typeText("for i in range(3):")
	e.newline()
	if e.text != "for i in range(3):\n    " {
		t.Fatalf("auto-indent: %q", e.text)
	}
	e.typeText("print(i)\r\n\tx = 1")
	if !strings.HasSuffix(e.text, "print(i)\n    x = 1") {
		t.Fatalf("CRLF and tab: %q", e.text)
	}
	e.undoEdit()
	if strings.Contains(e.text, "x = 1") {
		t.Fatalf("undo: %q", e.text)
	}
}
