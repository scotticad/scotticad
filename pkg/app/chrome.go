//go:build windows

package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/win"
)

func (a *App) drawChrome() {
	a.drawTitleBar()
	a.drawToolbar()
	a.drawConsole()
	if a.panelOpen {
		a.drawShapePanel()
	}
	a.drawInspector()
	a.drawFilament()
}

// --- title bar -------------------------------------------------------------------

func (a *App) drawTitleBar() {
	b := a.batch
	r := render.Rect{W: a.W, H: titleH}
	b.Fill(r, colChrome)
	b.Fill(render.Rect{Y: titleH - 1, W: a.W, H: 1}, colBorder)

	// logo (also opens the File menu)
	lr := render.Rect{X: 12, Y: 8, W: 32, H: 32}
	if a.isHot("logo") {
		b.RoundRect(lr.Inset(-3), 5, colHoverBG)
	}
	drawLogo(b, lr)
	a.button("logo", lr, "File menu", "Alt + F", tipBelow, func() { a.openFileMenu(lr.X) })

	// File menu button
	tw := b.TextWidth("File", fontUI+1, false)
	fr := render.Rect{X: lr.X + lr.W + 10, Y: 12, W: tw + 34, H: 24}
	a.fileMenuX = fr.X
	open := a.popup == popDesignMenu
	if a.isHot("filemenu") || open {
		b.RoundRect(fr, 4, colHoverBG)
	}
	fc := colText
	if open {
		fc = colBlue
	}
	b.Text("File", fr.X+10, fr.CY()-b.LineHeight(fontUI+1, false)/2, fontUI+1, false, fc)
	drawIcon(b, icChevronDown, render.Rect{X: fr.X + 14 + tw, Y: fr.CY() - 6, W: 12, H: 12}, colIcon)
	a.button("filemenu", fr, "", "", 0, func() { a.openFileMenu(fr.X) })

	// separator, then the design's name: its file's (Save as renames it),
	// the full path in the tooltip
	b.Fill(render.Rect{X: fr.X + fr.W + 8, Y: 14, W: 1, H: 20}, colBorder)
	nr := render.Rect{X: fr.X + fr.W + 16, Y: 10, W: 220, H: 28}
	name := elideEnd(b, a.designName(), nr.W-8, fontUI+1, false)
	b.Text(name, nr.X+6, nr.CY()-b.LineHeight(fontUI+1, false)/2, fontUI+1, false, colText)
	tip := a.ed.Path
	if tip == "" {
		tip = "Not saved yet"
	}
	a.reg(region{r: nr, id: "docname", tip: tip, tipSide: tipBelow})

	// on a narrow window, the clipboard and undo buttons, after the name
	if a.leftInTitle {
		x := nr.X + nr.W + 8
		b.Fill(render.Rect{X: x, Y: 14, W: 1, H: 20}, colBorder)
		a.drawLeftTools(x+8, (titleH-toolbarH)/2)
	}
}

// titleToolsX is where the clipboard and undo buttons start when they sit
// in the title bar: after the File button and the design name.
func (a *App) titleToolsX() float64 {
	fileW := a.batch.TextWidth("File", fontUI+1, false) + 34
	return 12 + 32 + 10 + fileW + 16 + 220 + 16
}

// --- toolbar -------------------------------------------------------------------------

type tool struct {
	id, tip, key string
	icon         int
	enabled      bool
	active       bool
	click        func()
	caret        bool // narrow dropdown arrow
}

// leftToolsW is the width the left group (clipboard and undo) needs.
const leftToolsW = 10 + 6*toolBtnW + 10 + 12

// toolsEnd is where the right group of tools ends: at the shape panel.
func (a *App) toolsEnd() float64 {
	if !a.panelOpen {
		return a.W - 250
	}
	return a.W - panelW - 6
}

// leftGroupW is the clipboard and undo buttons' width.
const leftGroupW = 6*toolBtnW + 10

// barH is the toolbar's height: one row, or two when the window is too
// narrow for the tools even with the clipboard and undo buttons moved up
// to the title bar.
func (a *App) barH() float64 { return toolbarH * float64(max(1, a.tbRows)) }

// placeLeftTools decides where the clipboard and undo buttons go: in the
// toolbar beside the tools when they fit; else up in the title bar; else
// (a very narrow window) on a toolbar row of their own above the tools.
func (a *App) placeLeftTools() {
	a.leftInTitle, a.tbRows = false, a.toolbarRows()
	if a.tbRows > 1 && a.titleToolsX()+leftGroupW+12 <= a.W {
		a.leftInTitle, a.tbRows = true, 1
	}
}

// leftTools are the clipboard and undo buttons.
func (a *App) leftTools() []tool {
	ed := a.ed
	sel := len(ed.Sel) > 0
	return []tool{
		{id: "copy", tip: "Copy", key: "Ctrl + C", icon: icCopy, enabled: sel, click: func() { ed.Copy() }},
		{id: "paste", tip: "Paste", key: "Ctrl + V", icon: icPaste, enabled: true, click: func() { a.paste(false) }},
		{id: "duplicate", tip: "Duplicate", key: "Ctrl + D", icon: icDuplicate, enabled: sel, click: func() { ed.Duplicate() }},
		{id: "delete", tip: "Delete", key: "Delete", icon: icDelete, enabled: sel, click: func() { ed.Delete(nil) }},
		{id: "undo", tip: "Undo", key: "Ctrl + Z", icon: icUndo, enabled: ed.Hist.CanUndo(), click: func() { a.undo() }},
		{id: "redo", tip: "Redo", key: "Ctrl + Y", icon: icRedo, enabled: ed.Hist.CanRedo(), click: func() { a.redo() }},
	}
}

// drawLeftTools draws the clipboard and undo buttons from x, in a row
// whose top is y.
func (a *App) drawLeftTools(x, y float64) {
	for i, t := range a.leftTools() {
		if i == 4 {
			x += 10
		}
		a.toolButton(t, x, y)
		x += toolBtnW
	}
}

// rightTools are the toolbar's tools, from Show all to the filament
// estimate.
func (a *App) rightTools() []tool {
	ed := a.ed
	sel := len(ed.Sel) > 0
	multi := len(ed.Editable()) > 1
	hasGroup := false
	for _, n := range ed.Editable() {
		hasGroup = hasGroup || n.IsGroup()
	}
	return []tool{
		{id: "showall", tip: "Show all", key: "Ctrl + Shift + H", icon: icEye, enabled: true, active: ed.AnyHidden(), click: func() { ed.ShowAll() }},
		{id: "visibility", tip: "Visibility", icon: icCaret, enabled: true, caret: true, click: func() { a.openPopup(popVisibility) }},
		{id: "bundle", tip: "Bundle Group", key: "Ctrl + B", icon: icBundle, enabled: multi, click: func() { a.group(doc.ModeBundle) }},
		{id: "union", tip: "Union Group", key: "Ctrl + G", icon: icUnion, enabled: multi, click: func() { a.group(doc.ModeUnion) }},
		{id: "ungroup", tip: "Ungroup", key: "Ctrl + Shift + G", icon: icUngroup, enabled: hasGroup, click: func() { ed.Ungroup() }},
		{id: "split", tip: "Split", key: "K", icon: icSplit, enabled: len(ed.Editable()) > 0, active: a.mode == modeSplit, click: func() { a.setMode(modeSplit) }},
		{id: "section", tip: "Section view", key: "Shift + K", icon: icSection, enabled: len(ed.Doc.Nodes) > 0 || a.section.on, active: a.section.on, click: func() { a.setSection(!a.section.on) }},
		{id: "align", tip: "Align", key: "L", icon: icAlign, enabled: len(ed.Editable()) > 0, active: a.mode == modeAlign, click: func() { a.setMode(modeAlign) }},
		{id: "mirror", tip: "Mirror", key: "M", icon: icMirror, enabled: len(ed.Editable()) > 0, active: a.mode == modeMirror, click: func() { a.setMode(modeMirror) }},
		{id: "cruise", tip: "Place on surface", key: "C", icon: icCruise, enabled: sel || a.cruise, active: a.cruise, click: func() { a.cruise = !a.cruise }},
		{id: "measure", tip: "Measure", key: "", icon: icMeasure, enabled: true, active: a.mode == modeMeasure, click: func() { a.setMode(modeMeasure) }},
		{id: "shapewp", tip: "Construction plane on shape", icon: icShapeWP, enabled: sel || ed.Workplane != nil, active: ed.Workplane != nil, click: func() {
			if err := ed.ToggleShapeWorkplane(); err != nil {
				a.showToast(err.Error())
			}
		}},
		{id: "drop", tip: "Drop down", key: "D", icon: icDrop, enabled: sel, click: func() { ed.Drop(nil) }},
		{id: "layflat", tip: "Lay flat", key: "Shift + D", icon: icLayFlat, enabled: len(ed.Doc.Nodes) > 0, active: a.mode == modeLayFlat, click: func() { a.setMode(modeLayFlat) }},
		{id: "filament", tip: "Filament estimate", icon: icSpool, enabled: len(ed.Doc.Nodes) > 0, active: a.fil.shown, click: a.toggleFilament},
	}
}

func toolsWidth(ts []tool) float64 {
	w := 0.0
	for _, t := range ts {
		if t.caret {
			w += 18
		} else {
			w += toolBtnW
		}
	}
	return w
}

// toolbarRows is 2 when the left group and the tools don't fit side by
// side.
func (a *App) toolbarRows() int {
	if leftToolsW > a.toolsEnd()-toolsWidth(a.rightTools()) {
		return 2
	}
	return 1
}

func (a *App) drawToolbar() {
	b := a.batch
	y := titleH
	r := render.Rect{Y: y, W: a.W, H: a.barH()}
	b.Fill(r, colChrome)
	b.Fill(render.Rect{Y: y + a.barH() - 1, W: a.W, H: 1}, colBorder)
	if !a.leftInTitle {
		a.drawLeftTools(10, y)
	}

	// the tools: beside the left group, or on a second row under it
	if a.tbRows > 1 {
		y += toolbarH
	}
	right := a.rightTools()
	end := a.toolsEnd()
	x := end - toolsWidth(right)
	for _, t := range right {
		a.toolButton(t, x, y)
		if t.caret {
			x += 18
		} else {
			x += toolBtnW
		}
	}

	// Import / Export / Scripting
	bx := end + 8
	for _, t := range []struct {
		id, label string
		active    bool
		click     func()
	}{
		{"import", "Import", false, func() { a.openPopup(popImport) }},
		{"export", "Export", false, func() { a.exportSel = len(a.ed.Sel) > 0; a.openPopup(popExport) }},
		{"scripting", "Scripting", a.con.open, a.toggleConsole},
	} {
		br := render.Rect{X: bx, Y: y + 8, W: 74, H: toolbarH - 16}
		bg := colChrome
		if a.isHot(t.id) || t.active {
			bg = colHoverBG
		}
		b.RoundRect(br, 4, bg)
		c := colText
		if a.isHot(t.id) || t.active {
			c = colIconHover
		}
		b.TextCentered(t.label, br, fontUI, false, c)
		if t.id == "scripting" && a.con.unread && !a.con.open {
			b.Circle(br.X+br.W-8, br.Y+7, 3.5, colBlue) // new output
		}
		a.button(t.id, br, "", "", 0, t.click)
		bx += 78
	}
}

func (a *App) toolButton(t tool, x, y float64) {
	b := a.batch
	w := toolBtnW
	if t.caret {
		w = 18
	}
	r := render.Rect{X: x, Y: y + (toolbarH-34)/2, W: w, H: 34}
	hot := a.isHot(t.id) && t.enabled
	c, accent := colIcon, colIconAccent
	switch {
	case !t.enabled:
		c, accent = colIconOff, colIconOff
	case t.active || hot:
		c, accent = colIconHover, colIconHover
	}
	if t.active {
		b.RoundRect(r.Inset(1), 4, colBlueLight)
	}
	is := iconSize
	if t.caret {
		is = 14
	}
	drawIconAccent(b, t.icon, render.Rect{X: r.CX() - is/2, Y: r.CY() - is/2, W: is, H: is}, c, accent)
	click := t.click
	if !t.enabled {
		click = func() {}
	}
	a.reg(region{r: r, id: t.id, cursor: win.CursorHand, tip: t.tip, tipKey: t.key, tipSide: tipBelow, click: click})
}

// --- right shape panel -------------------------------------------------------------------

// tileOrder is the Basic Shapes library: the three hole shapes first, then
// the solids.
var tileOrder = []struct {
	kind string
	hole bool
}{
	{"box", true}, {"cylinder", true}, {"sphere", true},
	{"box", false}, {"cylinder", false}, {"sphere", false},
	{"cone", false}, {"pyramid", false}, {"roof", false},
	{"text", false}, {"roundroof", false}, {"halfsphere", false},
	{"torus", false}, {"tube", false}, {"ring", false},
	{"wedge", false}, {"polygon", false}, {"icosahedron", false},
	{"star3d", false}, {"star", false}, {"heart", false},
	{"dice", false}, {"diamond", false}, {"paraboloid", false},
}

var libraries = []string{"Basic Shapes", "Parametric", "Hardware & Enclosures", "Mechanical", "Computers & Networking", "All Shapes"}

func (a *App) libraryTiles() []struct {
	kind string
	hole bool
} {
	if a.searchOn {
		return searchTiles(a.searchQuery)
	}
	return tilesFor(a.library)
}

// shapeKeywords are extra words a search matches besides a shape's name.
var shapeKeywords = map[string]string{
	"box":          "cube square block rectangle",
	"cylinder":     "round rod post",
	"sphere":       "ball globe round",
	"cone":         "point",
	"roof":         "gable triangle prism",
	"roundroof":    "arch tunnel",
	"halfsphere":   "dome hemisphere",
	"torus":        "donut doughnut ring",
	"tube":         "pipe hollow",
	"ring":         "washer",
	"wedge":        "ramp slope",
	"polygon":      "hexagon octagon prism",
	"icosahedron":  "dice d20 polyhedron",
	"text":         "letters numbers words font",
	"thread":       "screw bolt metric iso helix",
	"puffy":        "rounded box soft pillow",
	"grid":         "squares vent grille array",
	"star3d":       "pointed faceted 3d",
	"dice":         "die cube rounded",
	"diamond":      "gem jewel crystal brilliant",
	"hyperboloid":  "hourglass waist cooling tower spool",
	"sphereshell":  "hollow dome bowl hemisphere cup",
	"gear":         "cog sprocket wheel teeth",
	"projectbox":   "enclosure case box lid hollow container",
	"extrudedbox":  "enclosure case aluminium aluminum extrusion tube end panels",
	"standoff":     "pcb spacer boss mount screw",
	"plate":        "base panel mounting holes",
	"vent":         "grille grill honeycomb hex slots airflow",
	"hexnut":       "nut hardware metric screw",
	"hexbolt":      "bolt screw hardware metric thread",
	"nuttrap":      "nut pocket captive hole",
	"countersink":  "countersunk screw hole flat head",
	"counterbore":  "counterbored socket cap screw hole",
	"spurgear":     "gear involute cog wheel teeth",
	"gearrack":     "rack linear gear teeth",
	"elbow":        "pipe tube bend curved",
	"spring":       "coil helix",
	"knob":         "knurled potentiometer dial handle",
	"hinge":        "print in place joint",
	"laptop":       "computer notebook pc",
	"monitor":      "computer screen display",
	"keyboard":     "computer keys typing",
	"mouse":        "computer pointer",
	"desktop":      "computer tower pc case",
	"server":       "computer rack 1u 2u network",
	"rack":         "server cabinet network data center",
	"router":       "network wireless antenna",
	"switch":       "network ethernet ports hub",
	"cloud":        "network internet sky weather",
	"database":     "server storage data disk",
	"wifi":         "wireless network signal icon",
	"laserprinter": "printer paper office computer",
	"printer3d":    "printer fdm maker computer",
}

// searchTiles returns every library tile (once) whose name or keywords
// contain all the words of q; "hole" and "solid" pick the tile type.
func searchTiles(q string) []struct {
	kind string
	hole bool
} {
	words := strings.Fields(strings.ToLower(q))
	seen := map[string]bool{}
	var out []struct {
		kind string
		hole bool
	}
	for _, lib := range libraries {
		for _, t := range tilesFor(lib) {
			key := thumbKey(t.kind, t.hole)
			if seen[key] {
				continue
			}
			seen[key] = true
			hay := strings.ToLower(kindLabel(t.kind, t.hole) + " " + t.kind + " " + shapeKeywords[t.kind])
			if t.hole {
				hay += " hole"
			} else {
				hay += " solid"
			}
			match := true
			for _, w := range words {
				if !strings.Contains(hay, w) {
					match = false
					break
				}
			}
			if match {
				out = append(out, t)
			}
		}
	}
	return out
}

// openSearch turns the library picker into a search box.
func (a *App) openSearch() {
	a.searchOn = true
	a.panelScroll = 0
	a.openField(&textField{id: "shape-search", text: a.searchQuery,
		live:   func(s string) { a.searchQuery = s; a.panelScroll = 0 },
		commit: func(s string) bool { a.searchQuery = s; return true },
		cancel: a.closeSearch,
	})
	if a.field != nil {
		a.field.selAll = false
	}
}

func (a *App) closeSearch() {
	if a.field != nil && a.field.id == "shape-search" {
		a.field = nil
	}
	a.searchOn, a.searchQuery, a.panelScroll = false, "", 0
	a.w.Redraw()
}

// tilesFor lists a library's tiles.
func tilesFor(library string) []struct {
	kind string
	hole bool
} {
	switch library {
	case "Parametric":
		// shape generators (solid tiles only: holes are made with the
		// Hole button)
		return []struct {
			kind string
			hole bool
		}{{"gear", false}, {"hyperboloid", false}, {"sphereshell", false}, {"thread", false}, {"puffy", false}, {"grid", false}}
	case "Hardware & Enclosures", "Mechanical":
		ids := shapes.HardwareKinds
		if library == "Mechanical" {
			ids = shapes.MechanicalKinds
		}
		var out []struct {
			kind string
			hole bool
		}
		for _, id := range ids {
			out = append(out, struct {
				kind string
				hole bool
			}{id, shapes.HoleKinds[id]})
		}
		return out
	case "Computers & Networking":
		var out []struct {
			kind string
			hole bool
		}
		for _, id := range shapes.TechKinds {
			out = append(out, struct {
				kind string
				hole bool
			}{id, false})
		}
		return out
	case "All Shapes":
		var out []struct {
			kind string
			hole bool
		}
		for _, k := range shapes.All() {
			if k.Hidden {
				continue
			}
			out = append(out, struct {
				kind string
				hole bool
			}{k.ID, shapes.HoleKinds[k.ID]})
		}
		return out
	}
	return tileOrder
}

func kindColor(kind string) string {
	if k, ok := shapes.Lookup(kind); ok {
		return k.Color
	}
	return ""
}

func kindLabel(kind string, hole bool) string {
	l := kind
	if k, ok := shapes.Lookup(kind); ok {
		l = k.Label
	}
	if hole {
		l += " Hole"
	}
	return l
}

func (a *App) panelRect() render.Rect {
	return render.Rect{X: a.W - panelW, Y: titleH + a.barH(), W: panelW, H: a.H - titleH - a.barH()}
}

func (a *App) drawPanelToggle(x float64) {
	b := a.batch
	r := render.Rect{X: x - 2, Y: a.vp.CY() - 18, W: 14, H: 36}
	b.RoundRect(r, 3, colChrome)
	b.RoundRectStroke(r, 3, 1, colBorder)
	ic := icChevronRight
	tip := "Hide shapes panel"
	if !a.panelOpen {
		ic = icChevronLeft
		tip = "Show shapes panel"
	}
	drawIcon(b, ic, render.Rect{X: r.X, Y: r.CY() - 7, W: 14, H: 14}, colIcon)
	a.button("paneltoggle", r, tip, "", tipBelow, func() { a.panelOpen = !a.panelOpen; a.w.Redraw() })
}

func (a *App) drawShapePanel() {
	b := a.batch
	pr := a.panelRect()
	b.Fill(pr, colChrome)
	b.Fill(render.Rect{X: pr.X, Y: pr.Y, W: 1, H: pr.H}, colBorder)
	a.reg(region{r: pr, id: "panel-area"})
	a.drawPanelToggle(pr.X - 12)

	// tool tabs
	tabs := []struct {
		id, tip, key string
		icon         int
		click        func()
		active       bool
	}{
		{"tab-construction-plane", "Construction plane tool", "W", icGridTab, func() { a.setMode(modeWorkplane) }, a.mode == modeWorkplane},
		{"tab-ruler", "Ruler tool", "R", icRulerTab, func() { a.setMode(modeRulerPlace) }, a.mode == modeRulerPlace || a.ed.Ruler != nil},
		{"tab-notes", "Notes tool", "N", icNotesTab, func() { a.setMode(modeNotes) }, a.mode == modeNotes},
	}
	tw := pr.W / 3
	for i, t := range tabs {
		r := render.Rect{X: pr.X + float64(i)*tw, Y: pr.Y + 6, W: tw, H: 44}
		c := colTabIcon
		if a.isHot(t.id) || t.active {
			b.RoundRect(r.Inset(6), 4, colBlueLight)
		}
		drawIcon(b, t.icon, render.Rect{X: r.CX() - 14, Y: r.CY() - 14, W: 28, H: 28}, c)
		a.button(t.id, r, t.tip, t.key, tipBelow, t.click)
	}
	b.Fill(render.Rect{X: pr.X + 10, Y: pr.Y + 56, W: pr.W - 20, H: 1}, colBorder)

	// library dropdown and search
	lr := render.Rect{X: pr.X + 10, Y: pr.Y + 66, W: pr.W - 56, H: 30}
	tilesTop := lr.Y + lr.H + 10
	tiles := a.libraryTiles()
	if a.searchOn {
		// a search box across the panel: magnifier, query, close
		sb := render.Rect{X: pr.X + 10, Y: lr.Y, W: pr.W - 20, H: 30}
		editing := a.field != nil && a.field.id == "shape-search"
		b.RoundRect(sb, 4, colChrome)
		bc := colFieldBorder
		if editing {
			bc = colBlue
		}
		b.RoundRectStroke(sb, 4, 1.2, bc)
		drawIcon(b, icSearch, render.Rect{X: sb.X + 6, Y: sb.CY() - 9, W: 18, H: 18}, colIcon)
		tr := render.Rect{X: sb.X + 30, Y: sb.Y, W: sb.W - 60, H: sb.H}
		switch {
		case editing:
			a.drawFieldText("shape-search", tr, a.field.text, fontUI, false, colText)
		case a.searchQuery == "":
			b.Text("Search shapes", tr.X, tr.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, colTextDim)
		default:
			b.Text(a.searchQuery, tr.X, tr.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
		}
		a.reg(region{r: tr, id: "search-field", cursor: win.CursorIBeam, press: func(float64, float64, win.Mods) func(float64, float64, bool) {
			if a.field == nil || a.field.id != "shape-search" {
				a.openSearch()
			}
			return nil
		}})
		xr := render.Rect{X: sb.X + sb.W - 28, Y: sb.CY() - 11, W: 22, H: 22}
		if a.isHot("search-close") {
			b.RoundRect(xr, 4, colHoverBG)
		}
		drawIcon(b, icClose, xr.Inset(5), colIcon)
		a.button("search-close", xr, "Close search", "Esc", tipBelow, a.closeSearch)
		// result count
		msg := fmt.Sprintf("Found %d", len(tiles))
		if q := strings.TrimSpace(a.searchQuery); q != "" {
			msg += ": " + q
		}
		b.Text(msg, pr.X+12, sb.Y+sb.H+8, fontSmall, false, colTextDim)
		tilesTop = sb.Y + sb.H + 30
	} else {
		bg := colChrome
		if a.isHot("library") || a.popup == popLibrary {
			bg = colHoverBG
		}
		b.RoundRect(lr, 4, bg)
		b.RoundRectStroke(lr, 4, 1, colFieldBorder)
		b.Text(a.library, lr.X+10, lr.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
		drawIcon(b, icChevronDown, render.Rect{X: lr.X + lr.W - 22, Y: lr.CY() - 7, W: 14, H: 14}, colIcon)
		a.button("library", lr, "", "", 0, func() { a.openPopup(popLibrary) })
		sr := render.Rect{X: lr.X + lr.W + 6, Y: lr.Y, W: 30, H: 30}
		if a.isHot("search") {
			b.RoundRect(sr, 4, colHoverBG)
		}
		drawIcon(b, icSearch, sr.Inset(6), colIcon)
		a.button("search", sr, "Search shapes", "", tipBelow, a.openSearch)
	}

	// tiles
	area := render.Rect{X: pr.X, Y: tilesTop, W: pr.W, H: pr.Y + pr.H - tilesTop - 10}
	cols := 3
	cell := (area.W - 20) / float64(cols)
	rows := (len(tiles) + cols - 1) / cols
	contentH := float64(rows)*cell + 10
	maxScroll := math.Max(0, contentH-area.H)
	a.panelScroll = math.Max(0, math.Min(maxScroll, a.panelScroll))
	b.PushClip(area)
	a.reg(region{r: area, id: "tiles", wheel: func(d float64) bool {
		a.panelScroll = math.Max(0, math.Min(maxScroll, a.panelScroll-d*40))
		return true
	}})
	for i, t := range tiles {
		col, row := i%cols, i/cols
		r := render.Rect{X: area.X + 10 + float64(col)*cell, Y: area.Y + float64(row)*cell - a.panelScroll, W: cell, H: cell}
		if r.Y+r.H < area.Y || r.Y > area.Y+area.H {
			continue
		}
		id := "tile-" + t.kind
		if t.hole {
			id += "-hole"
		}
		inner := r.Inset(3)
		if a.isHot(id) {
			b.RoundRect(inner, 4, colChrome)
			b.RoundRectStroke(inner, 4, 1, colFieldBorder)
		}
		// the atlas can be rebuilt part way through a frame: its old sprites
		// point at nothing now (the next frame rebuilds the tiles)
		if sp, ok := a.thumbs[thumbKey(t.kind, t.hole)]; ok && a.thumbGen == a.batch.Generation {
			b.Image(sp, inner.Inset(2), render.Color{1, 1, 1, 1})
		}
		kind, hole := t.kind, t.hole
		visible := r
		if visible.Y < area.Y {
			visible.H -= area.Y - visible.Y
			visible.Y = area.Y
		}
		if bottom := area.Y + area.H; visible.Y+visible.H > bottom {
			visible.H = bottom - visible.Y // the part below the list is clipped away, and can't be clicked
		}
		if visible.H <= 0 {
			continue
		}
		a.reg(region{r: visible, id: id, cursor: win.CursorHand, tip: kindLabel(kind, hole), tipSide: tipBelow,
			press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
				return a.tilePress(kind, hole, x, y)
			}})
	}
	b.PopClip()
	// scrollbar
	if maxScroll > 0 {
		h := area.H * area.H / contentH
		y := area.Y + (area.H-h)*a.panelScroll/maxScroll
		b.RoundRect(render.Rect{X: pr.X + pr.W - 7, Y: y, W: 4, H: h}, 2, colDivider)
	}
}

// tilePress starts dragging a tile; a click without drag arms placement
// mode instead (click the workplane to drop).
func (a *App) tilePress(kind string, hole bool, x, y float64) func(float64, float64, bool) {
	sx, sy := x, y
	started := false
	return func(x, y float64, up bool) {
		if !started && math.Hypot(x-sx, y-sy) > 4 {
			started = true
			a.place = newPlace(kind, hole)
		}
		if started {
			a.updatePlace(x, y)
			a.mx, a.my = x, y
			if up {
				if a.vp.Contains(x, y) && !a.panelRect().Contains(x, y) {
					a.commitPlace()
				} else {
					a.place = nil
				}
			}
			return
		}
		if up {
			a.place = newPlace(kind, hole)
		}
	}
}
