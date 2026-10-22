//go:build windows

package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/win"
)

// emergencySave is the window's OnPanic: an internal error is about to end
// the program, so the design is saved to a recovery file first and the
// user told where. The program still ends: the design may be half way
// through an edit, and carrying on could save that over good work later.
// (Headless runs don't set it: tests must see the crash.)
func (a *App) emergencySave(r any) {
	msg := fmt.Sprintf("ScottiCAD hit an internal error and has to close.\n\n%v", r)
	if a.ed != nil && a.ed.Doc != nil && (len(a.ed.Doc.Nodes) > 0 || len(a.ed.Doc.Notes) > 0) {
		if p, err := writeRecovery(a.baseDir(""), a.ed.Doc, time.Now()); err == nil {
			msg += "\n\nYour design was saved to:\n" + p
		} else {
			msg += "\n\nThe design couldn't be saved: " + err.Error()
		}
	}
	win.ErrorBox("ScottiCAD", msg)
}

// writeRecovery saves d in dir as "<name> (recovered <time>).scotticad"
// and returns the path.
func writeRecovery(dir string, d *doc.Document, now time.Time) (string, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	b, err := d.Marshal()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, fmt.Sprintf("%s (recovered %s).scotticad", sanitize(d.Name), now.Format("2006-01-02 15-04-05")))
	if err := editor.WriteFileAtomic(p, b); err != nil {
		return "", err
	}
	return p, nil
}
