//go:build windows

package app

import "testing"

// Pasted text loses Windows line breaks' carriage returns; a note keeps
// its lines, a one-line field gets spaces.
func TestPastable(t *testing.T) {
	for _, c := range []struct {
		in        string
		multiline bool
		want      string
	}{
		{"12.5\r\n", false, "12.5 "},
		{"one\r\ntwo\rthree\nfour", true, "one\ntwo\nthree\nfour"},
		{"one\r\ntwo", false, "one two"},
		{"a\tb", false, "a b"},
		{"plain", true, "plain"},
	} {
		if got := pastable(c.in, c.multiline); got != c.want {
			t.Errorf("pastable(%q, %v) = %q, want %q", c.in, c.multiline, got, c.want)
		}
	}
}
