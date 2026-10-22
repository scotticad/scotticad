//go:build windows

package app

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/scotticad/scotticad"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/win"
)

// --- recent designs ---------------------------------------------------------------

// The recent designs are the last few opened or saved, newest first, kept
// beside the user's other preferences. Automated (headless) runs keep
// theirs in memory only, so tests never touch the user's list.

const maxRecent = 8

type recentPrefs struct {
	Files []string `json:"files"`
}

func readRecent(p string) []string {
	var rp recentPrefs
	if b, err := os.ReadFile(p); err == nil {
		json.Unmarshal(b, &rp)
	}
	if len(rp.Files) > maxRecent {
		rp.Files = rp.Files[:maxRecent]
	}
	return rp.Files
}

func writeRecent(p string, files []string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(recentPrefs{Files: files}, "", " ")
	return os.WriteFile(p, b, 0o644)
}

// addRecent is files with path first and only once (paths compare as
// Windows does, ignoring case), at most maxRecent long.
func addRecent(files []string, path string) []string {
	out := []string{path}
	for _, f := range files {
		if !strings.EqualFold(f, path) && len(out) < maxRecent {
			out = append(out, f)
		}
	}
	return out
}

// removeRecent is files without path.
func removeRecent(files []string, path string) []string {
	var out []string
	for _, f := range files {
		if !strings.EqualFold(f, path) {
			out = append(out, f)
		}
	}
	return out
}

func (a *App) loadRecent() {
	if a.headless {
		return
	}
	if p := prefsPath("recent.json"); p != "" {
		a.recent = readRecent(p)
	}
}

// setRecent replaces the list and saves it (best effort: the list is a
// convenience, and failing to save it shouldn't interrupt anything).
func (a *App) setRecent(files []string) {
	a.recent = files
	if a.headless {
		return
	}
	if p := prefsPath("recent.json"); p != "" {
		writeRecent(p, files)
	}
}

// noteRecent puts the design at path at the top of the recent list.
func (a *App) noteRecent(path string) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	a.setRecent(addRecent(a.recent, path))
}

// saveChanges asks to save the design's unsaved changes before it is
// replaced; false if the user cancelled (or the save failed).
func (a *App) saveChanges() bool {
	if !a.ed.Dirty() {
		return true
	}
	switch a.ask("Save changes to " + a.designName() + "?") {
	case win.IDCancel:
		return false
	case win.IDYes:
		return a.save(false)
	}
	return true
}

// openPath opens the design at p in place of the current one, which it
// first offers to save; false if it wasn't opened.
func (a *App) openPath(p string) bool {
	if !a.saveChanges() {
		return false
	}
	if err := a.ed.Open(p); err != nil {
		a.showToast(err.Error())
		return false
	}
	a.loading = a.designName()
	a.restoreCamera()
	a.noteRecent(p)
	return true
}

// openRecent opens a recent design; one that has been moved or deleted
// leaves the list.
func (a *App) openRecent(p string) {
	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
		a.setRecent(removeRecent(a.recent, p))
		a.showToast(filepath.Base(p) + " was moved or deleted")
		return
	}
	if a.openPath(p) {
		a.closePopup()
	}
}

// --- the start screen ---------------------------------------------------------------

// drawStart is the start screen, shown when ScottiCAD starts without a
// design to open (and from the File menu): the logo, name and version,
// New and Open, and the recent designs.
func (a *App) drawStart() {
	b := a.batch
	r := a.modal(720, 440, "")

	// the product, on the left
	lx := r.X + 40
	drawLogo(b, render.Rect{X: lx, Y: r.Y + 44, W: 88, H: 88})
	b.Text("ScottiCAD", lx, r.Y+148, fontUI+17, true, colText)
	b.Text("Version "+scotticad.Version(), lx, r.Y+194, fontUI, false, colTextDim)
	b.Text("3D modelling for makers", lx, r.Y+216, fontUI, false, colTextDim)
	a.primaryButton("start-new", render.Rect{X: lx, Y: r.Y + r.H - 128, W: 200, H: 36}, "New design", func() {
		if a.saveChanges() {
			a.closePopup()
			a.ed.NewDesign()
			a.homeCamera(false)
		}
	})
	a.textButton("start-open", render.Rect{X: lx, Y: r.Y + r.H - 80, W: 200, H: 36}, "Open…", false, func() {
		a.closePopup()
		a.open()
	})

	// the recent designs, on the right
	dx := r.X + 290
	b.Fill(render.Rect{X: dx, Y: r.Y + 40, W: 1, H: r.H - 80}, colBorder)
	rx, rw := dx+30, r.X+r.W-40-(dx+30)
	b.Text("Recent designs", rx, r.Y+44, fontUI+1, true, colText)
	if len(a.recent) == 0 {
		b.Text("Designs you open or save will appear here.", rx, r.Y+80, fontUI, false, colTextDim)
		return
	}
	const rowH = 42.0
	y := r.Y + 74
	for i, p := range a.recent {
		if y+rowH > r.Y+r.H-30 {
			break
		}
		row := render.Rect{X: rx - 8, Y: y, W: rw + 16, H: rowH - 4}
		id := "start-recent-" + string(rune('0'+i))
		if a.isHot(id) {
			b.RoundRect(row, 4, colHoverBG)
		}
		name := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		b.Text(elideStart(b, name, rw, fontUI, true), rx, y+4, fontUI, true, colText)
		b.Text(elideStart(b, filepath.Dir(p), rw, fontSmall, false), rx, y+21, fontSmall, false, colTextDim)
		a.button(id, row, p, "", tipBelow, func() { a.openRecent(p) })
		y += rowH
	}
}

// elideEnd is s, cut short with an ellipsis until it fits in w.
func elideEnd(b *render.Batch, s string, w, size float64, bold bool) string {
	if b.TextWidth(s, size, bold) <= w {
		return s
	}
	rs := []rune(s)
	for i := len(rs) - 1; i > 0; i-- {
		t := string(rs[:i]) + "…"
		if b.TextWidth(t, size, bold) <= w {
			return t
		}
	}
	return "…"
}

// elideStart is s, cut from the front with an ellipsis until it fits in w
// (a folder path's end is the telling part).
func elideStart(b *render.Batch, s string, w, size float64, bold bool) string {
	if b.TextWidth(s, size, bold) <= w {
		return s
	}
	rs := []rune(s)
	for i := 1; i < len(rs); i++ {
		t := "…" + string(rs[i:])
		if b.TextWidth(t, size, bold) <= w {
			return t
		}
	}
	return "…"
}
