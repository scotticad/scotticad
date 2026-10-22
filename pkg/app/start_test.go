//go:build windows

package app

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

// A design goes to the top of the recent list once, paths matching as
// Windows matches them, and the list stays short.
func TestAddRecent(t *testing.T) {
	l := addRecent(nil, `C:\a.scotticad`)
	l = addRecent(l, `C:\b.scotticad`)
	l = addRecent(l, `c:\A.scotticad`) // the same file again
	if want := []string{`c:\A.scotticad`, `C:\b.scotticad`}; !reflect.DeepEqual(l, want) {
		t.Errorf("got %q, want %q", l, want)
	}
	for i := 0; i < 20; i++ {
		l = addRecent(l, fmt.Sprintf(`C:\%d.scotticad`, i))
	}
	if len(l) != maxRecent || l[0] != `C:\19.scotticad` {
		t.Errorf("after 20 more: %d entries, first %q", len(l), l[0])
	}
	if l = removeRecent(l, `c:\19.SCOTTICAD`); len(l) != maxRecent-1 || l[0] != `C:\18.scotticad` {
		t.Errorf("after removing the first: %q", l)
	}
}

func TestRecentPrefs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "recent.json")
	if got := readRecent(p); len(got) != 0 {
		t.Errorf("no file: %q", got)
	}
	want := []string{`C:\x.scotticad`, `D:\designs\y.scotticad`}
	if err := writeRecent(p, want); err != nil {
		t.Fatal(err)
	}
	if got := readRecent(p); !reflect.DeepEqual(got, want) {
		t.Errorf("read back %q", got)
	}
}
