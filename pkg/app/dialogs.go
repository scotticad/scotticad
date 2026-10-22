//go:build windows

package app

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/scotticad/scotticad"
	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/win"
)

type menuItem struct {
	id, label, key string
	icon           int // -1 none
	enabled        bool
	active         bool
	click          func()
}

// menu draws a list menu at (x, y); upward opens it above y.
func (a *App) menu(x, y, w float64, items []menuItem, rowH float64, upward bool) render.Rect {
	b := a.batch
	h := float64(len(items))*rowH + 8
	if upward {
		y -= h
	}
	if y+h > a.H-4 {
		y = a.H - 4 - h
	}
	r := render.Rect{X: x, Y: y, W: w, H: h}
	a.catcher(false)
	a.panel(r)
	for i, it := range items {
		ir := render.Rect{X: r.X + 4, Y: r.Y + 4 + float64(i)*rowH, W: r.W - 8, H: rowH}
		if it.enabled && a.isHot(it.id) {
			b.RoundRect(ir, 3, colHoverBG)
		}
		c := colText
		if !it.enabled {
			c = colIconOff
		} else if it.active {
			c = colBlue
		}
		tx := ir.X + 10
		if it.icon >= 0 {
			drawIcon(b, it.icon, render.Rect{X: ir.X + 10, Y: ir.CY() - 10, W: 20, H: 20}, c)
			tx = ir.X + 40
		}
		if it.key != "" && rowH >= 44 {
			// label with the shortcut in a blue badge underneath
			b.Text(it.label, tx, ir.Y+6, fontUI, false, c)
			kw := b.TextWidth(it.key, fontSmall-1, false) + 10
			kr := render.Rect{X: tx, Y: ir.Y + 25, W: kw, H: 16}
			kc := colBlue
			if !it.enabled {
				kc = colIconOff
			}
			b.RoundRect(kr, 3, kc.A(0.12))
			b.TextCentered(it.key, kr, fontSmall-1, false, kc)
		} else {
			b.Text(it.label, tx, ir.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, c)
			if it.key != "" {
				kw := b.TextWidth(it.key, fontSmall, false)
				b.Text(it.key, ir.X+ir.W-kw-10, ir.CY()-b.LineHeight(fontSmall, false)/2, fontSmall, false, colTextDim)
			}
		}
		if it.enabled {
			click := it.click
			a.button(it.id, ir, "", "", 0, func() { a.closePopup(); click() })
		}
	}
	return r
}

func (a *App) drawPopups() {
	ed := a.ed
	switch a.popup {
	case popVisibility:
		var r *region
		for i := range a.prevRegions {
			if a.prevRegions[i].id == "visibility" {
				r = &a.prevRegions[i]
			}
		}
		x, y := a.W-panelW-400, titleH+a.barH()
		if r != nil {
			x, y = r.r.X-200, r.r.Y+r.r.H+4
		}
		notesLabel := "Hide notes"
		if a.notesHidden {
			notesLabel = "Show notes"
		}
		a.menu(x, y, 230, []menuItem{
			{id: "m-hide", label: "Hide selected shapes", key: "Ctrl + H", icon: icHide, enabled: len(ed.Sel) > 0, click: ed.HideSelected},
			{id: "m-togglevis", label: "Toggle shape visibility", key: "V", icon: icToggleVis, enabled: true, click: func() { a.setMode(modeVisibility) }},
			{id: "m-notes", label: notesLabel, key: "Shift + N", icon: icNotes, enabled: len(ed.Notes()) > 0 || a.notesHidden, click: a.toggleNotes},
		}, 48, false)
	case popSnap:
		sr := a.snapRect()
		var items []menuItem
		for _, o := range editor.SnapOptions(a.units()) {
			mm := o.MM
			items = append(items, menuItem{id: "snap-" + o.Label, label: o.Label, icon: -1, enabled: true,
				active: math.Abs(mm-a.snap()) < 1e-6, click: func() { ed.SetSnap(mm) }})
		}
		a.menu(sr.X, sr.Y-4, sr.W+10, items, 26, true)
	case popFont:
		ns := ed.Selected()
		if len(ns) != 1 {
			a.popup = popNone
			return
		}
		n := ns[0]
		var items []menuItem
		for _, f := range shapes.FontNames() {
			items = append(items, menuItem{id: "font-" + f, label: f, icon: -1, enabled: true, active: f == n.Font,
				click: func() { ed.SetText(n.ID, n.Text, f) }})
		}
		r := a.fontMenuAt
		maxRows := int(math.Max(4, (a.H-r.Y-r.H-20)/26))
		if len(items) > maxRows {
			// scroll the list so the current font is visible
			start := 0
			for i, it := range items {
				if it.active {
					start = i - maxRows/2
				}
			}
			start = int(math.Max(0, math.Min(float64(len(items)-maxRows), float64(start)+a.fontScroll)))
			items = items[start : start+maxRows]
		}
		mr := a.menu(r.X, r.Y+r.H+2, r.W, items, 26, false)
		a.reg(region{r: mr, id: "fontlist", wheel: func(d float64) bool {
			a.fontScroll -= d * 3
			return true
		}})
	case popLibrary:
		pr := a.panelRect()
		var items []menuItem
		for _, l := range libraries {
			items = append(items, menuItem{id: "lib-" + l, label: l, icon: -1, enabled: true, active: l == a.library,
				click: func() { a.library = l; a.panelScroll = 0 }})
		}
		a.menu(pr.X+10, pr.Y+98, pr.W-56, items, 30, false)
	case popDesignMenu:
		a.menu(a.menuX, titleH-6, 250, []menuItem{
			{id: "dm-new", label: "New design", key: "Ctrl+N", icon: -1, enabled: true, click: a.newDesign},
			{id: "dm-open", label: "Open…", key: "Ctrl+O", icon: -1, enabled: true, click: a.open},
			{id: "dm-save", label: "Save", key: "Ctrl+S", icon: -1, enabled: true, click: func() { a.save(false) }},
			{id: "dm-saveas", label: "Save as…", key: "Ctrl+Shift+S", icon: -1, enabled: true, click: func() { a.save(true) }},
			{id: "dm-import", label: "Import…", key: "Ctrl+I", icon: -1, enabled: true, click: func() { a.popup = popImport }},
			{id: "dm-export", label: "Export…", key: "Ctrl+E", icon: -1, enabled: true, click: func() { a.exportSel = len(a.ed.Sel) > 0; a.popup = popExport }},
			{id: "dm-runscript", label: "Run script…", icon: -1, enabled: !a.con.running, click: func() { a.popup = popNone; a.runScriptFile() }},
			{id: "dm-help", label: "Keyboard shortcuts", key: "?", icon: -1, enabled: true, click: func() { a.popup = popHelp }},
			{id: "dm-start", label: "Start screen", icon: -1, enabled: true, click: func() { a.popup = popStart }},
			{id: "dm-about", label: "About ScottiCAD", icon: -1, enabled: true, click: func() { a.popup = popAbout }},
			{id: "dm-exit", label: "Exit", key: "Alt+F4", icon: -1, enabled: true, click: func() {
				if a.Close() {
					a.w.Quit()
				}
			}},
		}, 32, false)
	case popSettings:
		a.drawSettings()
	case popImport:
		a.drawImport()
	case popExport:
		a.drawExport()
	case popHelp:
		a.drawHelp()
	case popAbout:
		a.drawAbout()
	case popStart:
		a.drawStart()
	}
	// an open dropdown's list goes over everything else in the dialog
	if f := a.dropList; f != nil {
		a.dropList = nil
		f()
	}
}

// modal draws a centred dialog with a title and close button.
func (a *App) modal(w, h float64, title string) render.Rect {
	a.catcher(true)
	// centred, but never above the window's top (where its close button
	// would be out of reach)
	r := render.Rect{X: math.Round(math.Max(4, a.W/2-w/2)), Y: math.Round(math.Max(4, a.H/2-h/2)), W: w, H: h}
	a.panel(r)
	b := a.batch
	b.Text(title, r.X+24, r.Y+20, fontUI+4, true, colText)
	cr := render.Rect{X: r.X + r.W - 40, Y: r.Y + 14, W: 26, H: 26}
	c := colIcon
	if a.isHot("modal-close") {
		c = colIconHover
	}
	drawIcon(b, icClose, cr.Inset(5), c)
	a.button("modal-close", cr, "", "", 0, a.closePopup)
	return r
}

// dropdown draws a select box showing label; a click opens the list of
// options beneath it (each the control id-option), and choosing one calls
// pick with its index. A click outside the list, or Esc, closes it and
// leaves the dialog open.
func (a *App) dropdown(id string, r render.Rect, label string, options []string, pick func(i int)) {
	b := a.batch
	open := a.openDrop == id
	bg := colChrome
	if a.isHot(id) || open {
		bg = colHoverBG
	}
	b.RoundRect(r, 3, bg)
	bc := colFieldBorder
	if open {
		bc = colBlue
	}
	b.RoundRectStroke(r, 3, 1, bc)
	b.Text(label, r.X+10, r.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
	drawIcon(b, icChevronDown, render.Rect{X: r.X + r.W - 22, Y: r.CY() - 7, W: 14, H: 14}, colIcon)
	a.button(id, r, "", "", 0, func() {
		if open {
			a.openDrop = ""
		} else {
			a.openDrop = id
		}
	})
	if open {
		a.dropList = func() { a.drawDropList(id, r, label, options, pick) }
	}
}

// drawDropList draws dropdown id's open list under its box r (above it,
// if there's no room below), the current option marked.
func (a *App) drawDropList(id string, r render.Rect, label string, options []string, pick func(i int)) {
	b := a.batch
	const rowH = 28.0
	w := r.W
	for _, o := range options {
		w = math.Max(w, b.TextWidth(o, fontUI, false)+40)
	}
	h := float64(len(options))*rowH + 8
	lr := render.Rect{X: r.X, Y: r.Y + r.H + 2, W: w, H: h}
	if lr.Y+h > a.H-4 {
		lr.Y = r.Y - 2 - h
	}
	// a click anywhere else closes the list (and does nothing more)
	a.reg(region{r: render.Rect{W: a.W, H: a.H}, id: "drop-catcher", press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
		a.openDrop = ""
		a.w.Redraw()
		return nil
	}})
	a.panel(lr)
	for i, o := range options {
		ir := render.Rect{X: lr.X + 4, Y: lr.Y + 4 + float64(i)*rowH, W: lr.W - 8, H: rowH}
		if a.isHot(id + "-" + o) {
			b.RoundRect(ir, 3, colHoverBG)
		}
		c := colText
		if o == label {
			c = colBlue
			p := newPen(b, render.Rect{X: ir.X + ir.W - 22, Y: ir.CY() - 7, W: 14, H: 14}, c)
			p.w = 1.8
			p.poly(false, 4, 12, 9, 17, 20, 6)
		}
		b.Text(o, ir.X+10, ir.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, c)
		a.button(id+"-"+o, ir, "", "", 0, func() { a.openDrop = ""; pick(i) })
	}
}

type bedPreset struct {
	name string
	w, l float64
}

var bedPresets = []bedPreset{
	{"Default (200 × 200)", 200, 200},
	{"Bambu Lab X1 / P1 (256 × 256)", 256, 256},
	{"Bambu Lab A1 mini (180 × 180)", 180, 180},
	{"Prusa MK4 (250 × 210)", 250, 210},
	{"Prusa MINI (180 × 180)", 180, 180},
	{"Creality Ender 3 (220 × 220)", 220, 220},
}

// drawBgPicker draws the background colour picker beside the Settings
// dialog r (over it, if the window is too narrow): the full picker of
// the shape colours' Custom tab, its colour going to the background.
func (a *App) drawBgPicker(r render.Rect) {
	if !a.bgPicker {
		a.pickTarget = nil
		return
	}
	b := a.batch
	const w, h = 512.0, 340.0
	pr := render.Rect{X: math.Min(r.X+r.W+12, a.W-w-8), Y: r.Y, W: w, H: h}
	pr.X = math.Max(8, pr.X)
	a.panel(pr)
	b.Text("Background color", pr.X+16, pr.Y+14, fontUI, true, colText)
	cr := render.Rect{X: pr.X + pr.W - 34, Y: pr.Y + 10, W: 24, H: 24}
	c := colIcon
	if a.isHot("bg-close") {
		c = colIconHover
	}
	drawIcon(b, icClose, cr.Inset(5), c)
	a.button("bg-close", cr, "", "", 0, func() { a.bgPicker, a.pickTarget = false, nil })
	b.Fill(render.Rect{X: pr.X, Y: pr.Y + 44, W: pr.W, H: 1}, colBorder)
	a.pickTarget = func(hex string, live bool) { a.bgColor = colorOf(hex) }
	bg := a.bgColor
	a.drawCustomColor(render.Rect{X: pr.X + 16, Y: pr.Y + 58, W: pr.W - 32, H: 270},
		rgbHex(float64(bg[0]), float64(bg[1]), float64(bg[2])))
}

func (a *App) drawSettings() {
	if a.settingsDraft == nil {
		s := a.ed.Doc.Settings
		a.settingsDraft, a.settingsBase = &s, s
	}
	s := a.settingsDraft
	b := a.batch
	r := a.modal(480, 476, "Settings")
	x := r.X + 24
	vx := r.X + 230
	y := r.Y + 68
	row := func(label string) float64 {
		b.Text(label, x, y+13-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
		yy := y
		y += 44
		return yy
	}
	// background colour: the swatch opens the full colour picker
	yy := row("Background color")
	sw := render.Rect{X: vx, Y: yy, W: 26, H: 26}
	b.RoundRect(sw, 3, a.bgColor)
	b.RoundRectStroke(sw, 3, 1, colFieldBorder)
	a.button("set-bg", sw, "", "", 0, func() { a.bgPicker = !a.bgPicker })
	defer a.drawBgPicker(r)
	// the app's colour theme: a preference of the user's, not the
	// design's, so it applies at once (Cancel doesn't undo it)
	yy = row("Theme")
	var themeNames []string
	for _, t := range themes {
		themeNames = append(themeNames, t.Name)
	}
	a.dropdown("set-theme", render.Rect{X: vx, Y: yy, W: 200, H: 28}, currentTheme.Name, themeNames,
		func(i int) { a.setTheme(themes[i]) })
	yy = row("Show shadows")
	a.checkbox("set-shadows", vx, yy+5, "", s.ShowShadows, func(v bool) { s.ShowShadows = v })
	yy = row("Show grid")
	a.checkbox("set-grid", vx, yy+5, "", s.ShowGrid, func(v bool) { s.ShowGrid = v })
	yy = row("Zoom speed")
	sr := render.Rect{X: vx + 36, Y: yy + 13, W: 140, H: 0}
	b.Text("Slow", vx, yy+13-b.LineHeight(fontSmall, false)/2, fontSmall, false, colTextDim)
	b.Text("Fast", sr.X+sr.W+8, yy+13-b.LineHeight(fontSmall, false)/2, fontSmall, false, colTextDim)
	t := (a.zoomSpeed - 0.4) / 1.6
	b.RoundRect(render.Rect{X: sr.X, Y: sr.Y - 1.5, W: sr.W, H: 3}, 1.5, colDivider)
	b.RoundRect(render.Rect{X: sr.X, Y: sr.Y - 1.5, W: sr.W * t, H: 3}, 1.5, colBlue)
	b.Circle(sr.X+sr.W*t, sr.Y, 7, colOnColor)
	b.Ring(sr.X+sr.W*t, sr.Y, 7, 1, colFieldBorder)
	a.reg(region{r: render.Rect{X: sr.X - 8, Y: sr.Y - 10, W: sr.W + 16, H: 20}, id: "set-zoom", cursor: win.CursorHand,
		press: func(px, py float64, m win.Mods) func(float64, float64, bool) {
			set := func(px float64) { a.zoomSpeed = 0.4 + 1.6*math.Max(0, math.Min(1, (px-sr.X)/sr.W)) }
			set(px)
			return func(px, py float64, up bool) { set(px) }
		}})
	yy = row("Units")
	unitLabel := map[string]string{"mm": "Metric", "in": "Imperial"}[s.Units]
	a.dropdown("set-units", render.Rect{X: vx, Y: yy, W: 200, H: 28}, unitLabel, []string{"Metric", "Imperial"}, func(i int) {
		u := []string{"mm", "in"}[i]
		if u == s.Units {
			return
		}
		s.Units = u
		if u == "in" {
			s.Snap = 25.4 / 16
		} else {
			s.Snap = 1
		}
	})
	yy = row("Build plate")
	preset := "Custom"
	for _, p := range bedPresets {
		if math.Abs(p.w-s.WorkplaneW) < 1e-6 && math.Abs(p.l-s.WorkplaneL) < 1e-6 {
			preset = p.name
			break
		}
	}
	var presetNames []string
	for _, p := range bedPresets {
		presetNames = append(presetNames, p.name)
	}
	a.dropdown("set-preset", render.Rect{X: vx, Y: yy, W: 220, H: 28}, preset, presetNames, func(i int) {
		s.WorkplaneW, s.WorkplaneL = bedPresets[i].w, bedPresets[i].l
	})
	// width / length
	units := s.Units
	for i, f := range []struct {
		label string
		v     *float64
	}{{"Width", &s.WorkplaneW}, {"Length", &s.WorkplaneL}} {
		fx := vx + float64(i)*110
		b.Text(f.label, fx, y-6, fontSmall, false, colTextDim)
		fr := render.Rect{X: fx, Y: y + 12, W: 100, H: 26}
		ptr := f.v
		a.numberField("set-"+f.label, fr, editor.Format(*f.v, units, true), func(str string) bool {
			v, err := editor.Parse(str, units)
			if err == nil && v > 0 && v <= 10000 {
				*ptr = v
			}
			return true
		}, func(dir float64) {
			step := 10.0
			if units == "in" {
				step = 25.4
			}
			*ptr = math.Max(step, math.Min(10000, *ptr+dir*step))
		})
	}
	// buttons
	a.textButton("set-reset", render.Rect{X: r.X + 24, Y: r.Y + r.H - 52, W: 120, H: 32}, "Reset default", false, func() {
		d := doc.DefaultSettings()
		*s = d
		a.bgColor = colBG
		a.zoomSpeed = 1
	})
	a.primaryButton("set-close", render.Rect{X: r.X + r.W - 164, Y: r.Y + r.H - 52, W: 140, H: 32}, "Close settings", func() {
		a.commitField()
		// only what was changed here: anything else changed meanwhile
		// (snap from its menu, a script) stays
		if m := mergeSettings(a.settingsBase, *s, a.ed.Doc.Settings); m != a.ed.Doc.Settings {
			a.ed.SetSettings(m)
		}
		a.settingsDraft = nil
		a.closePopup()
	})
}

// mergeSettings is cur with the fields that draft changed from base.
func mergeSettings(base, draft, cur doc.Settings) doc.Settings {
	if draft.Units != base.Units {
		cur.Units = draft.Units
	}
	if draft.WorkplaneW != base.WorkplaneW {
		cur.WorkplaneW = draft.WorkplaneW
	}
	if draft.WorkplaneL != base.WorkplaneL {
		cur.WorkplaneL = draft.WorkplaneL
	}
	if draft.Snap != base.Snap {
		cur.Snap = draft.Snap
	}
	if draft.ShowGrid != base.ShowGrid {
		cur.ShowGrid = draft.ShowGrid
	}
	if draft.ShowShadows != base.ShowShadows {
		cur.ShowShadows = draft.ShowShadows
	}
	return cur
}

func (a *App) drawImport() {
	b := a.batch
	r := a.modal(460, 330, "Import Shapes")
	x := r.X + 24
	y := r.Y + 66
	a.textButton("imp-choose", render.Rect{X: x, Y: y, W: 130, H: 32}, "Choose a file", false, func() {
		if p, ok := a.openFile("Import", "3D and 2D files (*.stl;*.obj;*.svg)", "*.stl;*.obj;*.svg", "All files", "*.*"); ok {
			a.importPath = p
		}
	})
	name := "No file chosen"
	if a.importPath != "" {
		name = filepath.Base(a.importPath)
	}
	b.PushClip(render.Rect{X: x + 140, Y: y, W: r.W - 190, H: 32})
	b.Text(name, x+142, y+16-b.LineHeight(fontUI, false)/2, fontUI, false, colTextDim)
	b.PopClip()
	y += 44
	b.Text("Supported formats: .STL, .OBJ, .SVG", x, y, fontSmall, false, colTextDim)
	y += 34
	o := &a.importOpts
	b.Text("Scale (%)", x, y+13-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
	a.numberField("imp-scale", render.Rect{X: x + 150, Y: y, W: 100, H: 26}, fmt.Sprintf("%g", o.Scale*100), func(s string) bool {
		var v float64
		if _, err := fmt.Sscan(strings.TrimSuffix(strings.TrimSpace(s), "%"), &v); err == nil && v > 0 && !math.IsInf(v, 0) {
			o.Scale = v / 100
		}
		return true
	}, func(dir float64) { o.Scale = math.Max(0.01, o.Scale+dir*0.1) })
	y += 40
	b.Text("Units (STL/OBJ)", x, y+13-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
	ul := map[string]string{"mm": "Millimeters", "in": "Inches"}[o.Units]
	a.dropdown("imp-units", render.Rect{X: x + 150, Y: y, W: 150, H: 28}, ul, []string{"Millimeters", "Inches"},
		func(i int) { o.Units = []string{"mm", "in"}[i] })
	y += 40
	b.Text("SVG height (mm)", x, y+13-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
	a.numberField("imp-height", render.Rect{X: x + 150, Y: y, W: 100, H: 26}, fmt.Sprintf("%g", o.Height), func(s string) bool {
		var v float64
		if _, err := fmt.Sscan(strings.TrimSpace(s), &v); err == nil && v > 0 && !math.IsInf(v, 0) {
			o.Height = v
		}
		return true
	}, func(dir float64) { o.Height = math.Max(0.5, o.Height+dir) })

	a.textButton("imp-cancel", render.Rect{X: r.X + r.W - 260, Y: r.Y + r.H - 52, W: 100, H: 32}, "Cancel", false, a.closePopup)
	a.primaryButton("imp-go", render.Rect{X: r.X + r.W - 150, Y: r.Y + r.H - 52, W: 126, H: 32}, "Import", func() {
		a.commitField()
		if a.importPath == "" {
			a.showToast("Choose a file first")
			return
		}
		a.closePopup()
		a.importFile(a.importPath)
	})
}

func (a *App) drawExport() {
	b := a.batch
	r := a.modal(520, 450, "")
	x := r.X + 24
	b.Text("Export", x, r.Y+18+(30-b.LineHeight(fontUI+1, true))/2, fontUI+1, true, colText)
	b.Fill(render.Rect{X: r.X, Y: r.Y + 50, W: r.W, H: 1}, colBorder)
	y := r.Y + 70
	b.Text("Include", x, y, fontUI, true, colText)
	y += 28
	sel := a.exportSel && len(a.ed.Sel) > 0
	radio := func(id string, yy float64, label string, on, enabled bool, click func()) {
		cx, cy := x+8, yy+8
		c := colFieldBorder
		if on {
			c = colBlue
		}
		b.Ring(cx, cy, 7, 1.5, c)
		if on {
			b.Circle(cx, cy, 3.5, colBlue)
		}
		tc := colText
		if !enabled {
			tc = colTextDim
		}
		tw := b.Text(label, x+24, cy-b.LineHeight(fontUI, false)/2, fontUI, false, tc)
		if enabled {
			a.button(id, render.Rect{X: x, Y: yy - 4, W: tw + 30, H: 24}, "", "", 0, click)
		}
	}
	radio("exp-all", y, "Everything in the design.", !sel, true, func() { a.exportSel = false })
	y += 28
	label := "Selected shapes"
	if len(a.ed.Sel) == 0 {
		label += " (you need to select something first.)"
	}
	radio("exp-sel", y, label, sel, len(a.ed.Sel) > 0, func() { a.exportSel = true })
	y += 44
	b.Text("Format", x, y, fontUI, true, colText)
	y += 24
	// one row per format, the most useful first: its name and what it's for
	for _, f := range exportFormats {
		br := render.Rect{X: x, Y: y, W: r.W - 48, H: 42}
		hot := a.isHot(f.id)
		bg := colChrome
		if hot {
			bg = colHoverBG
		}
		b.RoundRect(br, 4, bg)
		b.RoundRectStroke(br, 4, 1, colBorder)
		nc := colText
		if hot {
			nc = colBlue
		}
		b.Text(f.name, br.X+12, br.Y+5, fontUI, true, nc)
		b.Text(f.what, br.X+12, br.Y+23, fontSmall, false, colTextDim)
		ff := f.ext
		a.button(f.id, br, "", "", 0, func() { a.export(ff) })
		y += 46
	}
}

// exportFormats are the Export dialog's formats, in its order.
var exportFormats = []struct{ id, name, ext, what string }{
	{"exp-3mf", "3MF (.3mf)", "3mf", "3D printing: one file with the colours, read by PrusaSlicer, Bambu Studio and Cura"},
	{"exp-stl", "STL (.stl)", "stl", "3D printing: read by every slicer; no colours"},
	{"exp-obj", "OBJ (.obj)", "obj", "3D printing and other 3D programs: the colours in a .mtl file beside it"},
	{"exp-glb", "glTF (.glb)", "glb", "Viewing in 3D apps and on the web, with the colours"},
	{"exp-svg", "SVG (.svg)", "svg", "Laser cutting: the outline at the design's base"},
}

func (a *App) drawHelp() {
	b := a.batch
	rows := [][2]string{
		{"Copy / Cut / Paste", "Ctrl+C / Ctrl+X / Ctrl+V"}, {"Paste in place", "Ctrl+Shift+V"}, {"Duplicate", "Ctrl+D"},
		{"Delete", "Delete"}, {"Undo / Redo", "Ctrl+Z / Ctrl+Y"}, {"Select all", "Ctrl+A"},
		{"Union / Bundle group", "Ctrl+G / Ctrl+B"}, {"Ungroup", "Ctrl+Shift+G"}, {"Hole / Solid", "H / S"},
		{"Hide selected / Show all", "Ctrl+H / Ctrl+Shift+H"}, {"Hide / show notes", "Shift+N"}, {"Toggle visibility mode", "V"}, {"Lock", "Ctrl+L"},
		{"Transparent", "Ctrl+T"}, {"Nudge", "Arrows (Shift ×10)"}, {"Raise / lower", "Ctrl+Up / Ctrl+Down"},
		{"Drop down", "D"}, {"Fit view", "F"}, {"Align / Mirror", "L / M"}, {"Split (then X / Y / Z, Enter)", "K"}, {"Section view", "Shift+K"}, {"Lay flat", "Shift+D"}, {"Place on surface", "C"},
		{"Ruler / Construction plane", "R / W"}, {"Save / Open / New", "Ctrl+S / Ctrl+O / Ctrl+N"},
		{"Orbit", "Right drag"}, {"Pan", "Middle or Shift+Right drag"}, {"Zoom", "Wheel"},
	}
	// one column, or two side by side when a column is taller than the
	// window
	cols := 1
	if float64(len(rows))*24+90 > a.H-16 {
		cols = 2
	}
	per := (len(rows) + cols - 1) / cols
	// each column as wide as its longest label and key side by side
	keyAt, keyW := 0.0, 0.0
	for _, row := range rows {
		keyAt = math.Max(keyAt, b.TextWidth(row[0], fontUI, false)+18)
		keyW = math.Max(keyW, b.TextWidth(row[1], fontUI, false))
	}
	colW := math.Min(keyAt+keyW+28, (a.W-40)/float64(cols))
	r := a.modal(colW*float64(cols)+40, float64(per)*24+90, "Keyboard shortcuts")
	for i, row := range rows {
		x := r.X + 24 + float64(i/per)*colW
		y := r.Y + 62 + float64(i%per)*24
		b.Text(row[0], x, y, fontUI, false, colText)
		b.Text(row[1], x+keyAt, y, fontUI, false, colBlue)
	}
}

// projectURL is where ScottiCAD lives online.
const projectURL = "https://github.com/scotticad/scotticad"

// drawAbout is the About dialog: the logo, name and version (from the
// VERSION file, built in), licence and project link.
func (a *App) drawAbout() {
	b := a.batch
	r := a.modal(420, 330, "")
	cx := r.CX()
	drawLogo(b, render.Rect{X: cx - 32, Y: r.Y + 30, W: 64, H: 64})
	b.TextCentered("ScottiCAD", render.Rect{X: r.X, Y: r.Y + 104, W: r.W, H: 32}, fontUI+10, true, colText)
	b.TextCentered("Version "+scotticad.Version(), render.Rect{X: r.X, Y: r.Y + 140, W: r.W, H: 22}, fontUI+1, false, colTextDim)
	b.TextCentered("3D modelling for makers — runs locally, no cloud required", render.Rect{X: r.X, Y: r.Y + 176, W: r.W, H: 20}, fontUI, false, colText)
	b.TextCentered("Copyright 2026 scotticad.dev. Licensed under the Apache License 2.0.", render.Rect{X: r.X, Y: r.Y + 204, W: r.W, H: 20}, fontSmall, false, colTextDim)
	// the project link
	lr := render.Rect{X: cx - 140, Y: r.Y + 236, W: 280, H: 22}
	lc := colBlue
	if a.isHot("about-link") {
		lc = colIconHover
	}
	b.TextCentered(strings.TrimPrefix(projectURL, "https://"), lr, fontUI, false, lc)
	a.button("about-link", lr, "Open in your browser", "", tipBelow, func() { win.OpenURL(projectURL) })
	a.primaryButton("about-ok", render.Rect{X: cx - 50, Y: r.Y + r.H - 52, W: 100, H: 32}, "OK", a.closePopup)
}
