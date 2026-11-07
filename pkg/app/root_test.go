package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/editor"
)

func TestRootLimitsFiles(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	a := &App{ed: editor.New()}
	if err := a.setRoot(dir); err != nil {
		t.Fatal(err)
	}
	in := func(p string) string { return filepath.Join(a.root, p) }
	for _, c := range []struct {
		method, key, path, want string // want "" for refused
	}{
		{"save", "path", "part.scotticad", in("part.scotticad")},
		{"export", "path", "sub/part.stl", in(filepath.Join("sub", "part.stl"))},
		{"open", "path", in("x.scotticad"), in("x.scotticad")},
		{"export", "path", "../part.stl", ""},
		{"import", "path", "sub/../../x.stl", ""},
		{"screenshot", "path", filepath.Join(filepath.Dir(a.root), "elsewhere.png"), ""},
		{"run_script", "cwd", "sub", in("sub")},
		{"run_script", "cwd", "..", ""},
	} {
		p := args{c.key: c.path}
		err := a.checkRoot(c.method, p)
		switch {
		case c.want == "" && err == nil:
			t.Errorf("%s %s: allowed outside the root", c.method, c.path)
		case c.want != "" && err != nil:
			t.Errorf("%s %s: %v", c.method, c.path, err)
		case c.want != "" && p.s(c.key, "") != c.want:
			t.Errorf("%s %s: resolved to %s, not %s", c.method, c.path, p.s(c.key, ""), c.want)
		}
	}

	// a script with no folder works in the root
	p := args{}
	if err := a.checkRoot("run_script", p); err != nil || p.s("cwd", "") != a.root {
		t.Errorf("run_script without cwd: %v %v", p, err)
	}
	// saving over a file outside the root is refused; inside it isn't
	a.ed.Path = filepath.Join(filepath.Dir(a.root), "theirs.scotticad")
	if err := a.checkRoot("save", args{}); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("saving over a file outside: %v", err)
	}
	a.ed.Path = in("mine.scotticad")
	if err := a.checkRoot("save", args{}); err != nil {
		t.Errorf("saving over a file inside: %v", err)
	}
	for _, m := range []string{"answer_dialogs", "drop_files"} {
		if err := a.checkRoot(m, args{}); err == nil {
			t.Errorf("%s with a root", m)
		}
	}
	// methods without files, and no root at all, pass
	if err := a.checkRoot("add_shape", args{"kind": "box"}); err != nil {
		t.Error(err)
	}
	b := &App{ed: editor.New()}
	if err := b.checkRoot("export", args{"path": "../anywhere.stl"}); err != nil {
		t.Error(err)
	}
	if err := (&App{}).setRoot(filepath.Join(dir, "missing")); err == nil {
		t.Error("a root that isn't a folder")
	}
}

// With a root, describe tells callers where files go, on the methods that
// take them, and only on those; without one it is the plain table.
func TestDescribeNamesTheRoot(t *testing.T) {
	a := &App{ed: editor.New()}
	if err := a.setRoot(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, m := range a.describeMethods() {
		_, takesFiles := rootParams[m.Name]
		if strings.Contains(m.Doc, a.root) != takesFiles {
			t.Errorf("%s: %q", m.Name, m.Doc)
		}
	}
	for _, m := range apiMethods {
		if strings.Contains(m.Doc, a.root) {
			t.Errorf("the shared table changed: %s", m.Name)
		}
	}
	if b := (&App{}).describeMethods(); &b[0] != &apiMethods[0] {
		t.Error("no root: describe is the method table")
	}
}
