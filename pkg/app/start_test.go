package app

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// A design goes to the top of the recent list once, and the list stays
// short.
func TestAddRecent(t *testing.T) {
	l := addRecent(nil, `C:\a.scotticad`)
	l = addRecent(l, `C:\b.scotticad`)
	l = addRecent(l, `C:\a.scotticad`) // the same file again
	if want := []string{`C:\a.scotticad`, `C:\b.scotticad`}; !reflect.DeepEqual(l, want) {
		t.Errorf("got %q, want %q", l, want)
	}
	for i := 0; i < 20; i++ {
		l = addRecent(l, fmt.Sprintf(`C:\%d.scotticad`, i))
	}
	if len(l) != maxRecent || l[0] != `C:\19.scotticad` {
		t.Errorf("after 20 more: %d entries, first %q", len(l), l[0])
	}
	if l = removeRecent(l, `C:\19.scotticad`); len(l) != maxRecent-1 || l[0] != `C:\18.scotticad` {
		t.Errorf("after removing the first: %q", l)
	}
}

// Paths match as the platform's file names do: Windows ignores case,
// Linux doesn't.
func TestSamePath(t *testing.T) {
	if !samePath(`C:\a.scotticad`, `C:\a.scotticad`) || samePath("/a.scotticad", "/b.scotticad") {
		t.Error("equal or different paths misjudged")
	}
	if got := samePath(`C:\a.scotticad`, `c:\A.SCOTTICAD`); got != (runtime.GOOS == "windows") {
		t.Errorf("paths differing in case: %v on %s", got, runtime.GOOS)
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

// The center of rotation's marker is on unless the saved choice says off.
func TestViewPrefs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "view.json")
	if got := readViewPrefs(p); got.OrbitMarker != nil {
		t.Errorf("no file: %v", *got.OrbitMarker)
	}
	off := false
	if err := writeViewPrefs(p, viewPrefs{OrbitMarker: &off}); err != nil {
		t.Fatal(err)
	}
	if got := readViewPrefs(p); got.OrbitMarker == nil || *got.OrbitMarker {
		t.Errorf("read back %v, want off", got.OrbitMarker)
	}
}
