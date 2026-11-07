package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// rootParams are the parameters that name files, by method. With
// Options.Root set, each must be inside the root; relative ones resolve
// against it. Scripts' calls come through the same dispatcher, so a
// script's cad.save or cad.export is held to the root too.
var rootParams = map[string]string{
	"open": "path", "save": "path", "import": "path", "export": "path",
	"screenshot": "path", "run_script": "cwd",
}

// describeMethods is describe's reply. With a root, the methods that take
// files say where they must be, so a caller (an AI assistant reading the
// MCP tools, which come from this) uses the root from the start instead of
// learning it from a refusal.
func (a *App) describeMethods() []apiMethod {
	if a.root == "" {
		return apiMethods
	}
	ms := slices.Clone(apiMethods)
	for i, m := range ms {
		if _, ok := rootParams[m.Name]; ok {
			ms[i].Doc += fmt.Sprintf(" Files are limited to %s: a relative path resolves there, and a path outside it is refused.", a.root)
		}
	}
	return ms
}

// setRoot sets the folder file paths are limited to (Options.Root).
func (a *App) setRoot(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a folder", dir)
	}
	a.root = filepath.Clean(abs)
	return nil
}

// checkRoot refuses a call whose file is outside the root, and makes a
// relative path absolute against it. Without a root it does nothing.
func (a *App) checkRoot(method string, p args) error {
	if a.root == "" {
		return nil
	}
	switch method {
	case "answer_dialogs", "drop_files":
		return fmt.Errorf("%s isn't available while files are limited to %s", method, a.root)
	}
	k, ok := rootParams[method]
	if !ok {
		return nil
	}
	path := p.s(k, "")
	if path == "" {
		switch method {
		case "run_script":
			p[k] = a.root // a script's relative paths resolve against the root
		case "save":
			// saving over the design's own file: it must be inside too
			if a.ed.Path != "" && !a.inRoot(a.ed.Path) {
				return fmt.Errorf("the design's file %s is outside %s: save it with a path inside", a.ed.Path, a.root)
			}
		}
		return nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.root, path)
	}
	path = filepath.Clean(path)
	if !a.inRoot(path) {
		return fmt.Errorf("%s is outside %s, the only folder ScottiCAD may use", p.s(k, ""), a.root)
	}
	p[k] = path
	return nil
}

// inRoot reports whether path, absolute and clean, is the root or inside it.
func (a *App) inRoot(path string) bool {
	rel, err := filepath.Rel(a.root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
