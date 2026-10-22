//go:build windows

package app

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/filament"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/win"
)

// The filament estimate: a panel, opened by the toolbar's spool button,
// with grams, metres and cost for the selection (or the whole design), a
// line per colour, and the slicer settings it uses, which are kept between
// sessions. It sits beside the properties panel and stays open while the
// design is edited, so the numbers can be watched as parts change; its
// button, its ✕ or Esc close it.

type filamentState struct {
	settings filament.Settings
	shown    bool // the panel is open
	open     bool // its Settings section is expanded
	// the last result, reused until the design, selection or settings
	// change
	key    string
	result filamentResult
}

type filamentResult struct {
	total   filament.Estimate
	colors  []colorEstimate
	shapes  int
	pending bool // some groups are still being combined
}

type colorEstimate struct {
	color string
	est   filament.Estimate
}

// prefsPath is where per-user settings live.
func prefsPath(name string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ScottiCAD", name)
}

func (a *App) loadFilament() {
	a.fil.settings = filament.Defaults()
	if a.headless {
		return // automated runs use the defaults
	}
	if p := prefsPath("filament.json"); p != "" {
		a.fil.settings = readFilamentPrefs(p)
	}
}

func (a *App) saveFilament() {
	a.fil.key = ""
	if a.headless {
		return // automated runs don't touch the user's settings
	}
	p := prefsPath("filament.json")
	if p == "" {
		return
	}
	if err := writeFilamentPrefs(p, a.fil.settings); err != nil {
		a.showToast("Couldn't save the filament settings: " + err.Error())
	}
}

// readFilamentPrefs reads saved filament settings: the defaults for any
// setting the file leaves out, and the defaults altogether if it is
// missing or damaged; values out of range are brought into it.
func readFilamentPrefs(p string) filament.Settings {
	s := filament.Defaults()
	b, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	if json.Unmarshal(b, &s) != nil {
		return filament.Defaults()
	}
	return s.Normalized()
}

// writeFilamentPrefs saves filament settings, making the folder if need
// be; written beside the file and renamed over it, so a failure never
// leaves the saved settings empty.
func writeFilamentPrefs(p string, s filament.Settings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return editor.WriteFileAtomic(p, b)
}

// filamentNodes are what is estimated: the selection, or every visible
// shape.
func (a *App) filamentNodes(ids []int) []*doc.Node {
	if ids != nil {
		return a.ed.Doc.FindAll(ids)
	}
	if len(a.ed.Sel) > 0 {
		return a.ed.Selected()
	}
	var ns []*doc.Node
	for _, n := range a.ed.Doc.Nodes {
		if !n.Hidden {
			ns = append(ns, n)
		}
	}
	return ns
}

// estimateFilament estimates nodes part by part (holes don't print),
// with a total per colour.
func (a *App) estimateFilament(ns []*doc.Node) filamentResult {
	s := a.fil.settings
	r := filamentResult{}
	byColor := map[string]filament.Estimate{}
	var order []string
	add := func(c string, e filament.Estimate) {
		c = strings.ToLower(c)
		if _, ok := byColor[c]; !ok {
			order = append(order, c)
		}
		byColor[c] = byColor[c].Add(e)
	}
	for _, n := range ns {
		if n.Hole {
			continue // holes don't print
		}
		r.shapes++
		parts, exact := n.PartsExact()
		r.pending = r.pending || !exact
		for _, p := range parts {
			if p.Hole {
				continue
			}
			e := filament.Of(p.Mesh, s)
			r.total = r.total.Add(e)
			if !(p.Owner.IsGroup() && p.Owner.Multicolor) {
				add(p.Owner.Color, e)
				continue
			}
			// a multicolour part keeps its shapes' colours: shared out by
			// how much of its surface each colour covers
			area, total := map[string]float64{}, 0.0
			var cs []string
			for _, t := range p.Mesh.Tris {
				c := t.C.Hex()
				if _, ok := area[c]; !ok {
					cs = append(cs, c)
				}
				a := t.Area()
				area[c] += a
				total += a
			}
			for _, c := range cs {
				if total > 0 {
					add(c, e.Scale(area[c]/total))
				}
			}
		}
	}
	for _, c := range order {
		r.colors = append(r.colors, colorEstimate{c, byColor[c]})
	}
	return r
}

// filamentNow is the estimate for the panel, recomputed only when the
// design, the selection or the settings change.
func (a *App) filamentNow() filamentResult {
	f := &a.fil
	key := fmt.Sprint(a.rev, a.ed.Sel, f.settings, doc.Pending())
	if key != f.key {
		f.key, f.result = key, a.estimateFilament(a.filamentNodes(nil))
	}
	return f.result
}

func fmtGrams(g float64) string {
	switch {
	case g >= 1000:
		return fmt.Sprintf("%.2f kg", g/1000)
	case g >= 100:
		return fmt.Sprintf("%.0f g", g)
	}
	return fmt.Sprintf("%.1f g", g)
}

func fmtMeters(m float64) string {
	if m < 1 {
		return fmt.Sprintf("%.0f cm", m*100)
	}
	return fmt.Sprintf("%.2f m", m)
}

// filamentRow is a setting the panel edits.
type filamentRow struct {
	key, label, unit string
	get              func(s filament.Settings) float64
	set              func(s *filament.Settings, v float64)
	integer          bool
}

var filamentRows = []filamentRow{
	{"density", "Density", "g/cm³", func(s filament.Settings) float64 { return s.Density }, func(s *filament.Settings, v float64) { s.Density, s.Material = v, "Custom" }, false},
	{"diameter", "Filament diameter", "mm", func(s filament.Settings) float64 { return s.Diameter }, func(s *filament.Settings, v float64) { s.Diameter = v }, false},
	{"layer_height", "Layer height", "mm", func(s filament.Settings) float64 { return s.LayerHeight }, func(s *filament.Settings, v float64) { s.LayerHeight = v }, false},
	{"line_width", "Line width", "mm", func(s filament.Settings) float64 { return s.LineWidth }, func(s *filament.Settings, v float64) { s.LineWidth = v }, false},
	{"walls", "Walls", "", func(s filament.Settings) float64 { return float64(s.Walls) }, func(s *filament.Settings, v float64) { s.Walls = int(v) }, true},
	{"top_layers", "Top layers", "", func(s filament.Settings) float64 { return float64(s.TopLayers) }, func(s *filament.Settings, v float64) { s.TopLayers = int(v) }, true},
	{"bottom_layers", "Bottom layers", "", func(s filament.Settings) float64 { return float64(s.BottomLayers) }, func(s *filament.Settings, v float64) { s.BottomLayers = int(v) }, true},
	{"infill", "Infill", "%", func(s filament.Settings) float64 { return s.Infill }, func(s *filament.Settings, v float64) { s.Infill = v }, false},
	{"price_per_kg", "Price per kg", "", func(s filament.Settings) float64 { return s.PricePerKg }, func(s *filament.Settings, v float64) { s.PricePerKg = v }, false},
	{"calibration", "Calibration", "%", func(s filament.Settings) float64 { return s.Calibration }, func(s *filament.Settings, v float64) { s.Calibration = v }, false},
}

func fmtSetting(v float64, integer bool) string {
	if integer {
		return strconv.Itoa(int(v))
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// nextMaterial steps the Material button through the presets.
func (a *App) nextMaterial() {
	s := &a.fil.settings
	i := slices.IndexFunc(filament.Materials, func(m filament.Material) bool { return m.Name == s.Material })
	m := filament.Materials[(i+1)%len(filament.Materials)]
	s.Material, s.Density = m.Name, m.Density
	a.saveFilament()
}

// toggleFilament is the spool button.
func (a *App) toggleFilament() {
	a.fil.shown = !a.fil.shown
	a.w.Redraw()
}

// filamentRect is where the panel goes: left of the properties panel (or
// in its place when nothing is selected), at the top of the viewport.
func (a *App) filamentRect(w, h float64) render.Rect {
	x := a.vp.X + a.vp.W - w - 12
	if len(a.ed.Sel) > 0 {
		x = a.inspectorRect(0).X - w - 10
	}
	return render.Rect{X: max(a.vp.X+64, x), Y: a.vp.Y + 10, W: w, H: h}
}

// drawFilament draws the panel.
func (a *App) drawFilament() {
	if !a.fil.shown {
		return
	}
	b := a.batch
	f := &a.fil
	r := a.filamentNow()
	const w = 300.0
	rowH := 26.0
	h := 150.0
	if len(r.colors) > 1 {
		h += float64(len(r.colors))*20 + 8
	}
	if f.open {
		h += float64(len(filamentRows)+1)*rowH + 6
	}
	pr := a.filamentRect(w, h)
	a.panel(pr)
	px := pr.X + 16
	cy := pr.Y + 12
	scope := "Whole design"
	if len(a.ed.Sel) > 0 {
		scope = fmt.Sprintf("%d selected", len(a.ed.Sel))
		if len(a.ed.Sel) == 1 {
			scope = "Selection"
		}
	}
	b.Text("Filament estimate", px, cy, fontUI+1, true, colText)
	cr := render.Rect{X: pr.X + pr.W - 32, Y: pr.Y + 8, W: 24, H: 24}
	c := colIcon
	if a.isHot("fil-close") {
		c = colIconHover
	}
	drawIcon(b, icClose, cr.Inset(6), c)
	a.button("fil-close", cr, "Close", "Esc", tipBelow, a.toggleFilament)
	b.Text(scope, cr.X-6-b.TextWidth(scope, fontSmall, false), cy+2, fontSmall, false, colTextDim)
	cy += 30
	if r.pending {
		b.Text("Calculating…", px, cy+6, fontUI+6, true, colTextDim)
	} else {
		b.Text("≈ "+fmtGrams(r.total.Grams), px, cy, fontUI+10, true, colText)
		line := fmtMeters(r.total.Meters)
		if f.settings.PricePerKg > 0 {
			line += fmt.Sprintf("   ·   cost %.2f", r.total.Cost)
		}
		b.Text(line, px, cy+34, fontUI, false, colText)
	}
	cy += 62
	if len(r.colors) > 1 && !r.pending {
		for _, c := range r.colors {
			b.Circle(px+6, cy+8, 6, colorOf(c.color))
			b.RoundRectStroke(render.Rect{X: px, Y: cy + 2, W: 12, H: 12}, 6, 1, colBorder)
			b.Text(fmt.Sprintf("%s  ·  %s", fmtGrams(c.est.Grams), fmtMeters(c.est.Meters)), px+20, cy+1, fontSmall+0.5, false, colText)
			cy += 20
		}
		cy += 8
	}
	b.Text("Supports, brims and purge aren't included.", px, cy, fontSmall, false, colTextDim)
	cy += 24
	// settings
	sr := render.Rect{X: pr.X + 8, Y: cy - 4, W: pr.W - 16, H: 24}
	if a.isHot("fil-settings") {
		b.RoundRect(sr, 3, colHoverBG)
	}
	ic := icChevronDown
	if f.open {
		ic = icChevronUp
	}
	b.Text("Settings", px, sr.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
	label := f.settings.Material + fmt.Sprintf(", %g%% infill", f.settings.Infill)
	b.Text(label, sr.X+sr.W-34-b.TextWidth(label, fontSmall, false), sr.CY()-b.LineHeight(fontSmall, false)/2, fontSmall, false, colTextDim)
	drawIcon(b, ic, render.Rect{X: sr.X + sr.W - 24, Y: sr.CY() - 7, W: 14, H: 14}, colIcon)
	a.button("fil-settings", sr, "", "", 0, func() { f.open = !f.open })
	cy += 26
	if !f.open {
		return
	}
	fieldX := pr.X + pr.W - 16 - 110
	b.Text("Material", px, cy+4, fontUI, false, colText)
	a.textButton("fil-material", render.Rect{X: fieldX, Y: cy, W: 110, H: rowH - 4}, f.settings.Material, false, a.nextMaterial)
	cy += rowH
	for _, row := range filamentRows {
		id := "fil-" + row.key
		b.Text(row.label, px, cy+4, fontUI, false, colText)
		fr := render.Rect{X: fieldX, Y: cy, W: 70, H: rowH - 4}
		b.RoundRect(fr, 3, colChrome)
		border := colFieldBorder
		if a.field != nil && a.field.id == id {
			border = colBlue
		}
		b.RoundRectStroke(fr, 3, 1, border)
		a.drawFieldText(id, fr, fmtSetting(row.get(f.settings), row.integer), fontUI, false, colText)
		if row.unit != "" {
			b.Text(row.unit, fr.X+fr.W+6, cy+4, fontSmall, false, colTextDim)
		}
		a.reg(region{r: fr, id: id, cursor: win.CursorIBeam, press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
			a.openField(&textField{id: id, text: fmtSetting(row.get(f.settings), row.integer),
				commit: func(t string) bool {
					v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(t), "%")), 64)
					if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
						a.showToast("not a number: " + t)
						return true
					}
					row.set(&f.settings, v)
					f.settings = f.settings.Normalized()
					a.saveFilament()
					return true
				}})
			return nil
		}})
		cy += rowH
	}
}

// filamentJSON is an estimate for the API.
func filamentJSON(r filamentResult, s filament.Settings) map[string]any {
	var cs []any
	for _, c := range r.colors {
		cs = append(cs, map[string]any{"color": c.color, "grams": c.est.Grams, "meters": c.est.Meters, "cost": c.est.Cost, "volume": c.est.Volume})
	}
	return map[string]any{"grams": r.total.Grams, "meters": r.total.Meters, "cost": r.total.Cost, "volume": r.total.Volume,
		"shell": r.total.Shell, "shapes": r.shapes, "by_color": cs, "settings": s}
}
