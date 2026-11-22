package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/window"
)

// The sketch workspace's controls: a top bar (copy, paste, delete, undo,
// redo; the dimensions switch, Cancel, Finish sketch), view buttons at the board's top left, the Pen and Edit modes
// down its left side, and the actions for the selection along its foot.

const skBtnR = 19.0 // a round button's radius

// skRound draws a round icon button centred at (cx, cy) and registers it.
func (a *App) skRound(id, tip, key string, icon int, cx, cy float64, active, enabled bool, side int, click func()) {
	b := a.batch
	hot := a.isHot(id) && enabled
	fill, edge, ic := colChrome, colFieldBorder, colIcon
	switch {
	case !enabled:
		ic, edge = colIconOff, colBorder
	case active:
		fill, edge, ic = colBlue, colBlue, colOnColor
	case hot:
		edge, ic = colIconHover, colIconHover
	}
	b.Circle(cx, cy, skBtnR, edge)
	b.Circle(cx, cy, skBtnR-1.5, fill)
	is := 22.0
	drawIconAccent(b, icon, render.Rect{X: cx - is/2, Y: cy - is/2, W: is, H: is}, ic, ic)
	if !enabled {
		click = func() {}
	}
	a.reg(region{r: render.Rect{X: cx - skBtnR, Y: cy - skBtnR, W: 2 * skBtnR, H: 2 * skBtnR}, id: id, cursor: window.CursorHand,
		tip: tip, tipKey: key, tipSide: side, click: click})
}

// skDrawToolbar draws the workspace's top bar where the main toolbar goes.
func (a *App) skDrawToolbar() {
	s := a.sk
	b := a.batch
	y := titleH
	b.Fill(render.Rect{Y: y, W: a.W, H: toolbarH}, colChrome)
	b.Fill(render.Rect{Y: y + toolbarH - 1, W: a.W, H: 1}, colBorder)
	any := len(s.sel) > 0
	x := 10.0
	for i, t := range []tool{
		{id: "sk-copy", tip: "Copy", key: "Ctrl + C", icon: icCopy, enabled: any, click: a.skCopy},
		{id: "sk-paste", tip: "Paste", key: "Ctrl + V", icon: icPaste, enabled: len(a.skClip) > 0, click: a.skPaste},
		{id: "sk-delete", tip: "Delete", key: "Delete", icon: icDelete, enabled: any, click: func() {
			a.skPush()
			a.skDeleteSelected()
		}},
		{id: "sk-undo", tip: "Undo", key: "Ctrl + Z", icon: icUndo, enabled: len(s.undo) > 0, click: a.skUndo},
		{id: "sk-redo", tip: "Redo", key: "Ctrl + Y", icon: icRedo, enabled: len(s.redo) > 0, click: a.skRedo},
	} {
		if i == 3 {
			b.Fill(render.Rect{X: x + 4, Y: y + 12, W: 1, H: toolbarH - 24}, colBorder)
			x += 10
		}
		a.toolButton(t, x, y)
		x += toolBtnW
	}
	a.skDrawKindHeader(y)

	done := render.Rect{X: a.W - 132, Y: y + 8, W: 120, H: toolbarH - 16}
	cancel := render.Rect{X: done.X - 90, Y: done.Y, W: 82, H: done.H}
	a.toolButton(tool{id: "sk-dims", tip: "Show dimensions", key: "D", icon: icSkDims, enabled: true, active: s.dims,
		click: func() { s.dims = !s.dims }}, cancel.X-toolBtnW-10, y)
	a.toolButton(tool{id: "sk-image", tip: "Picture to trace", icon: icSkImage, enabled: true, active: s.img.panel,
		click: func() {
			if s.img.panel {
				a.skCloseImagePanel()
			} else {
				a.skOpenImagePanel()
			}
		}}, cancel.X-2*toolBtnW-14, y)
	a.textButton("sk-cancel", cancel, "Cancel", false, a.closeSketch)
	a.primaryButton("sk-done", done, "Finish sketch", a.sketchDone)
}

// skKindIcons are the sketch kinds' icons, as on the shape panel's tools.
var skKindIcons = map[string]int{"sketch": icToolSketch, "revolve": icToolRevolve, "coil": icToolCoil}

// skDrawKindHeader marks the top bar (top at y) with the kind of sketch
// being drawn: a strip of its color along the bar's foot, and in the
// middle its tool's icon and name on a tint of it.
func (a *App) skDrawKindHeader(y float64) {
	s := a.sk
	b := a.batch
	col := geom.ParseColor(kindColor(s.kind))
	kc := render.Color{col.R, col.G, col.B, 1}
	b.Fill(render.Rect{Y: y + toolbarH - 3, W: a.W, H: 3}, kc)
	title := map[string]string{"sketch": "Extrude sketch", "revolve": "Revolve sketch", "coil": "Coil sketch"}[s.kind]
	tw := b.TextWidth(title, fontUI, true)
	const is, pad = 20.0, 12.0
	w := pad + is + 8 + tw + pad
	r := render.Rect{X: math.Round(a.W/2 - w/2), Y: y + 9, W: w, H: toolbarH - 21}
	b.RoundRect(r, r.H/2, render.Color{col.R, col.G, col.B, 0.13})
	b.RoundRectStroke(r, r.H/2, 1, render.Color{col.R, col.G, col.B, 0.5})
	drawIconAccent(b, skKindIcons[s.kind], render.Rect{X: r.X + pad, Y: r.CY() - is/2, W: is, H: is}, colTabIcon, kc)
	b.Text(title, r.X+pad+is+8, r.CY()-b.LineHeight(fontUI, true)/2, fontUI, true, colText)
}

// skDrawViewButtons draws home, zoom in, zoom out and fit at the board's
// top left.
func (a *App) skDrawViewButtons() {
	s := a.sk
	r := s.board
	cy := r.Y + 14 + skBtnR
	for i, t := range []struct {
		id, tip, key string
		icon         int
		click        func()
	}{
		{"sk-home", "Home view", "", icHome, a.skHome}, // (the home scale, the origin in the middle)
		{"sk-zoom-in", "Zoom in", "+", icZoomIn, func() { a.skZoomAt(r.CX(), r.CY(), 1.25) }},
		{"sk-zoom-out", "Zoom out", "-", icZoomOut, func() { a.skZoomAt(r.CX(), r.CY(), 0.8) }},
		{"sk-fit", "Fit the drawing in view", "F", icFit, a.fitSketch},
	} {
		a.skRound(t.id, t.tip, t.key, t.icon, r.X+14+skBtnR+float64(i)*(2*skBtnR+8), cy, false, true, tipBelow, t.click)
	}
}

// The mode rail's sizes: its width, a tool's cell, the header naming the
// mode.
const (
	skRailW     = 70.0
	skRailCellH = 54.0
	skRailHeadH = 32.0
)

// skRailItem is a tool on the mode rail.
type skRailItem struct {
	id, tip, key, label string
	icon                int
	active, enabled     bool
	click               func()
}

// skDrawModeBar draws the mode rail down the board's left side: a card
// with Pen and Edit, or, inside one, its name (click to go back), its
// tools with their names, and the Pen's Clear. The tool in use is marked
// in the sketch kind's color.
func (a *App) skDrawModeBar() {
	s := a.sk
	b := a.batch
	var head, back string
	var items []skRailItem
	var foot *skRailItem
	if s.tool == skShape {
		items = []skRailItem{
			{id: "sk-pen", tip: "Pen, create new shapes", key: "P", label: "Pen", icon: icSkPen, enabled: true, click: func() { a.skSetTool(s.lastPen) }},
			{id: "sk-edit", tip: "Edit, modify selected shape", key: "A", label: "Edit", icon: icSkEdit, enabled: len(s.sk.Paths) > 0, click: func() { a.skSetTool(s.lastEdit) }},
		}
	} else {
		head, back = "Pen", "Close the Pen tool"
		tools := []int{skLine, skBezier, skSmooth}
		if skEditTool(s.tool) {
			head, back = "Edit", "Close the Edit tool"
			tools = []int{skSelect, skRefine}
		}
		for _, t := range tools {
			info := skToolInfo[t]
			items = append(items, skRailItem{id: info.id, tip: info.tip, key: info.key, label: info.label, icon: info.icon,
				active: s.tool == t, enabled: true, click: func() { a.skSetTool(t) }})
		}
		if skPenTool(s.tool) {
			foot = &skRailItem{id: "sk-clear", tip: "Clear the path being drawn", label: "Clear", icon: icSkClear, enabled: s.drawing >= 0, click: a.skClear}
		}
	}
	h := 16 + float64(len(items))*skRailCellH
	if head != "" {
		h += skRailHeadH
	}
	if foot != nil {
		h += 9 + skRailCellH
	}
	card := render.Rect{X: s.board.X + 8, Y: math.Round(s.board.CY() - h/2), W: skRailW, H: h}
	a.panel(card)
	y := card.Y + 8
	if head != "" {
		hr := render.Rect{X: card.X + 4, Y: y, W: card.W - 8, H: skRailHeadH - 6}
		if a.isHot("sk-back") {
			b.RoundRect(hr, 6, colHoverBG)
		}
		drawIcon(b, icChevronLeft, render.Rect{X: hr.X + 4, Y: hr.CY() - 6, W: 12, H: 12}, colIcon)
		b.Text(head, hr.X+20, hr.CY()-b.LineHeight(fontLabel, true)/2, fontLabel, true, colText)
		a.reg(region{r: hr, id: "sk-back", cursor: window.CursorHand, tip: back, tipKey: "Esc", tipSide: tipRight, click: a.skBack})
		b.Fill(render.Rect{X: card.X + 8, Y: y + skRailHeadH - 3, W: card.W - 16, H: 1}, colBorder)
		y += skRailHeadH
	}
	for _, it := range items {
		a.skRailCell(it, card.X, y)
		y += skRailCellH
	}
	if foot != nil {
		b.Fill(render.Rect{X: card.X + 8, Y: y + 4, W: card.W - 16, H: 1}, colBorder)
		a.skRailCell(*foot, card.X, y+9)
	}
}

// skRailCell draws one tool of the rail, the card's left edge at x and the
// cell's top at y, and registers it.
func (a *App) skRailCell(it skRailItem, x, y float64) {
	s := a.sk
	b := a.batch
	col := geom.ParseColor(kindColor(s.kind))
	kc := render.Color{col.R, col.G, col.B, 1}
	cell := render.Rect{X: x + 5, Y: y, W: skRailW - 10, H: skRailCellH - 4}
	ic, text := colTabIcon, colTextDim
	switch {
	case !it.enabled:
		ic, text = colIconOff, colIconOff
	case it.active:
		ic, text = kc, kc
		b.RoundRect(cell, 8, render.Color{col.R, col.G, col.B, 0.15})
		b.RoundRect(render.Rect{X: x + 1, Y: cell.Y + 8, W: 3, H: cell.H - 16}, 1.5, kc)
	case a.isHot(it.id):
		b.RoundRect(cell, 8, colHoverBG)
	}
	const is = 24.0
	drawIcon(b, it.icon, render.Rect{X: cell.CX() - is/2, Y: cell.Y + 5, W: is, H: is}, ic)
	const fs = 11.0
	tw := b.TextWidth(it.label, fs, it.active)
	b.Text(it.label, cell.CX()-tw/2, cell.Y+cell.H-b.LineHeight(fs, it.active)-3, fs, it.active, text)
	click := it.click
	if !it.enabled {
		click = func() {}
	}
	a.reg(region{r: cell, id: it.id, cursor: window.CursorHand, tip: it.tip, tipKey: it.key, tipSide: tipRight, click: click})
}

// skDrawActionBar draws the actions along the board's foot: open or close
// the selected paths, their stroke settings, and the selected points' kind
// (Make corner, Make smooth, Split handles). While an open path is being
// drawn, a check ends it there.
func (a *App) skDrawActionBar() {
	s := a.sk
	b := a.batch
	r := s.board
	cy := r.Y + r.H - 16 - skBtnR
	step := 2*skBtnR + 8
	ps := s.selPaths()
	allOpen := len(ps) > 0
	for _, i := range ps {
		allOpen = allOpen && !s.sk.Paths[i].Closed
	}
	pts := len(s.sel) > 0 && s.drawing < 0
	paths := len(ps) > 0 && s.drawing < 0
	cx := r.CX() - 3*step
	if s.drawing >= 0 && len(s.sk.Paths[s.drawing].Points) >= 2 {
		// the green check: end the line here, open
		gx := cx - step
		id := "sk-finish-open"
		col := render.RGB(0x2e, 0xa0, 0x44)
		if a.isHot(id) {
			col = render.RGB(0x24, 0x86, 0x38)
		}
		b.Circle(gx, cy, skBtnR, col)
		drawIcon(b, icCheck, render.Rect{X: gx - 11, Y: cy - 11, W: 22, H: 22}, colOnColor)
		a.reg(region{r: render.Rect{X: gx - skBtnR, Y: cy - skBtnR, W: 2 * skBtnR, H: 2 * skBtnR}, id: id, cursor: window.CursorHand,
			tip: "Finish the path without closing it", tipKey: "Enter", tipSide: tipAbove, click: a.skFinish})
	}
	tip := "Close path"
	if len(ps) > 0 && !allOpen {
		tip = "Open path"
	}
	a.skRound("sk-close", tip, "O", icSkClose, cx, cy, false, paths, tipAbove, a.skToggleClosed)
	a.skRound("sk-stroke", "Stroke settings", "", icSkStroke, cx+step, cy, s.strokeOpen, paths, tipAbove, func() {
		s.strokeOpen, s.xformOpen = !s.strokeOpen, false
	})
	holes := s.skAllHoles()
	holeTip := "Make hole: cut this out of the rest"
	if holes {
		holeTip = "Make solid again"
	}
	a.skRound("sk-hole", holeTip, "H", icSkHole, cx+2*step, cy, paths && holes, paths, tipAbove, func() { a.skSetHole(!holes) })
	a.skRound("sk-xform", "Mirror and turn", "M", icSkMirrorTurn, cx+3*step, cy, s.xformOpen, paths, tipAbove, func() {
		s.xformOpen, s.strokeOpen = !s.xformOpen, false
	})
	b.Fill(render.Rect{X: cx + 3.5*step - 0.5, Y: cy - 14, W: 1, H: 28}, colBorder)
	sharp, smooth, split := s.pointKinds()
	a.skRound("sk-sharp", "Make corner", "", icSkCorner, cx+4*step, cy, pts && sharp, pts, tipAbove, func() { a.skSetPointKind(shapes.PointCorner, false) })
	a.skRound("sk-make-smooth", "Make smooth", "", icSkSmoothPt, cx+5*step, cy, pts && smooth, pts, tipAbove, func() { a.skSetPointKind(shapes.PointSmooth, false) })
	a.skRound("sk-split", "Split handles", "", icSkSplit, cx+6*step, cy, pts && split, pts, tipAbove, func() { a.skSetPointKind(shapes.PointCorner, true) })
	switch {
	case !paths:
	case s.strokeOpen:
		a.skDrawStrokePanel(cy - skBtnR - 54)
	case s.xformOpen:
		a.skDrawXformPanel(cy - skBtnR - 54)
	}
}

// pointKinds says whether the selected points are all sharp corners, all
// smooth, or all corners with split handles.
func (s *sketchSession) pointKinds() (sharp, smooth, split bool) {
	kinds := map[int]bool{}
	anySplit, allSplit := false, true
	for r := range s.sel {
		q := s.sk.Paths[r.path].Points[r.pt]
		kinds[q.Kind] = true
		sp := q.Kind == shapes.PointCorner && (q.In != [2]float64{} || q.Out != [2]float64{})
		anySplit = anySplit || sp
		allSplit = allSplit && sp
	}
	sharp = len(kinds) == 1 && kinds[shapes.PointCorner] && !anySplit
	smooth = len(kinds) == 1 && (kinds[shapes.PointSmooth] || kinds[shapes.PointAuto])
	split = len(kinds) == 1 && allSplit && len(s.sel) > 0
	return
}

// skDrawStrokePanel draws the selected paths' stroke settings: on or off
// (an open path always has one), its width, where it lies on a closed path,
// its corners, and an open path's ends.
func (a *App) skDrawStrokePanel(y float64) {
	s := a.sk
	ps := s.selPaths()
	allClosed, anyLine, anyOpen := true, false, false
	join, capKind, align := -1, -1, -1
	for i, k := range ps {
		p := s.sk.Paths[k]
		allClosed = allClosed && p.Closed
		anyLine = anyLine || p.IsLine()
		anyOpen = anyOpen || !p.Closed
		if i == 0 {
			join, capKind, align = p.Join, p.Cap, p.Align
		}
		if p.Join != join {
			join = -1
		}
		if p.Cap != capKind {
			capKind = -1
		}
		if p.Align != align {
			align = -1
		}
	}
	on := anyLine
	items := []panelItem{}
	if allClosed {
		label := "Stroke off"
		if on {
			label = "Stroke on"
		}
		items = append(items, panelItem{kind: panelToggle, id: "sk-as-line", label: label, on: on,
			tip: "Build the path as a line of a width instead of the area it encloses", click: func() {
				// every selected path the same way, as the toggle shows
				// it (flipping each would swap a mixed selection)
				a.skSetPaths(func(p *shapes.SketchPath) {
					if on {
						p.Stroke = 0
					} else if p.Stroke <= 0 {
						p.Stroke = shapes.DefaultStroke
					}
				})
			}})
	}
	if on {
		items = append(items,
			panelItem{kind: panelCaption, label: "Width"},
			panelItem{kind: panelCustom, id: "sk-width", w: 64, draw: a.skDrawWidthField})
		if allClosed {
			items = append(items, panelItem{kind: panelCaption, label: "  Alignment"})
			for _, al := range []struct {
				v    int
				name string
			}{{shapes.AlignInside, "Inside"}, {shapes.AlignOutside, "Outside"}, {shapes.AlignCenter, "Center"}} {
				v := al.v
				items = append(items, panelItem{kind: panelToggle, id: "sk-align-" + al.name, label: al.name, on: align == v, click: func() {
					a.skSetPaths(func(p *shapes.SketchPath) { p.Align = v })
				}})
			}
		}
		items = append(items, panelItem{kind: panelCaption, label: "  Join"})
		for v, name := range []string{"Sharp", "Round", "Bevel"} {
			v := v
			items = append(items, panelItem{kind: panelToggle, id: "sk-join-" + name, label: name, on: join == v, click: func() {
				a.skSetPaths(func(p *shapes.SketchPath) { p.Join = v })
			}})
		}
		if anyOpen {
			items = append(items, panelItem{kind: panelCaption, label: "  Ends"})
			for v, name := range []string{"Flat", "Round", "Square"} {
				v := v
				items = append(items, panelItem{kind: panelToggle, id: "sk-cap-" + name, label: name, on: capKind == v, click: func() {
					a.skSetPaths(func(p *shapes.SketchPath) { p.Cap = v })
				}})
			}
		}
	}
	a.drawToolPanel(y, items)
}

// skDrawPointPanel shows a single selected point's position, to type.
func (a *App) skDrawPointPanel() {
	s := a.sk
	if len(s.sel) != 1 || s.drawing >= 0 || !skEditTool(s.tool) {
		return
	}
	var r skRef
	for k := range s.sel {
		r = k
	}
	var items []panelItem
	for axis, name := range []string{"X", "Y"} {
		axis := axis
		items = append(items,
			panelItem{kind: panelCaption, label: name},
			panelItem{kind: panelCustom, w: 70, draw: func(fr render.Rect) { a.skDrawPosField(fr, r, axis) }})
	}
	a.drawToolPanel(s.board.Y+14+2*skBtnR+12, items)
}

// --- dimensions ------------------------------------------------------------------

// skDimLabel draws a size's label centred at (cx, cy); with set it can be
// clicked to type the size.
func (a *App) skDimLabel(id string, v, cx, cy float64, set func(float64)) {
	b := a.batch
	text := strings.TrimSpace(editor.Format(v, a.units(), true))
	w := b.TextWidth(text, fontSmall, false) + 14
	r := render.Rect{X: math.Round(cx - w/2), Y: math.Round(cy - 10), W: w, H: 20}
	if a.field != nil && a.field.id == id {
		r.X, r.W = math.Round(cx-35), 70
	}
	if set == nil {
		b.RoundRect(r, 4, colChrome)
		b.RoundRectStroke(r, 4, 1, colFieldBorder)
		b.TextCentered(text, r, fontSmall, false, colText)
		return
	}
	a.skField(id, r, v, set)
}

// skDimLine draws a dimension line from (x0, y0) to (x1, y1) with arrow
// heads.
func (a *App) skDimLine(x0, y0, x1, y1 float64) {
	b := a.batch
	b.Line(x0, y0, x1, y1, 1, colDimLine)
	arrowHead(b, [2]float64{x1, y1}, [2]float64{x0, y0}, colDimLine)
	arrowHead(b, [2]float64{x0, y0}, [2]float64{x1, y1}, colDimLine)
}

// skDrawDims shows the sizes of what is selected: a whole path's (or
// paths') width and height, a single stretch's length, and a handle's
// offsets while it is dragged. All but the handle's can be typed.
func (a *App) skDrawDims() {
	s := a.sk
	if !s.dims || s.drawing >= 0 {
		return
	}
	b := a.batch
	if h := s.dragHandle; h != nil && h.path < len(s.sk.Paths) && h.pt < len(s.sk.Paths[h.path].Points) {
		q := s.sk.Paths[h.path].Points[h.pt]
		d := q.Out
		if s.dragWhich == 0 {
			d = q.In
		}
		px, py := s.toScreen(q.P)
		hx, hy := s.toScreen([2]float64{q.P[0] + d[0], q.P[1] + d[1]})
		b.Dashed(px, py, hx, py, 1, 4, colDimLine)
		b.Dashed(hx, py, hx, hy, 1, 4, colDimLine)
		a.skDimLabel("sk-dim-hx", math.Abs(d[0]), (px+hx)/2, py+(map[bool]float64{true: 14, false: -14})[hy < py], nil)
		a.skDimLabel("sk-dim-hy", math.Abs(d[1]), hx+(map[bool]float64{true: 28, false: -28})[hx >= px], (py+hy)/2, nil)
		return
	}
	if path, seg, ok := s.skSegment(); ok && (skEditTool(s.tool) || s.tool == skShape) {
		poly := s.sk.Paths[path].Segment(seg)
		mid := poly[len(poly)/2]
		if len(poly) == 2 {
			mid = shapes.Pt{X: (poly[0].X + poly[1].X) / 2, Y: (poly[0].Y + poly[1].Y) / 2}
		}
		p := s.sk.Paths[path]
		a0, a1 := p.Points[seg].P, p.Points[(seg+1)%len(p.Points)].P
		l := math.Hypot(a1[0]-a0[0], a1[1]-a0[1])
		mx, my := s.ptScreen(mid)
		a.skDimLabel("sk-dim-len", l, mx, my-18, a.skSetSegmentLength)
		return
	}
	// whole paths: every point of each path with one selected
	ps := s.selPaths()
	if len(ps) == 0 {
		return
	}
	for _, i := range ps {
		for j := range s.sk.Paths[i].Points {
			if !s.sel[skRef{i, j}] {
				return
			}
		}
	}
	x0, y0, x1, y1, ok := s.skSelBounds()
	if !ok {
		return
	}
	sx0, sy1 := s.toScreen([2]float64{x0, y0})
	sx1, sy0 := s.toScreen([2]float64{x1, y1})
	box := render.Rect{X: sx0, Y: sy0, W: sx1 - sx0, H: sy1 - sy0}
	b.Fill(box, render.Color{colBlue[0], colBlue[1], colBlue[2], 0.05})
	b.Dashed(sx0, sy0, sx1, sy0, 1, 6, colBlue)
	b.Dashed(sx1, sy0, sx1, sy1, 1, 6, colBlue)
	b.Dashed(sx1, sy1, sx0, sy1, 1, 6, colBlue)
	b.Dashed(sx0, sy1, sx0, sy0, 1, 6, colBlue)
	// the width below, the height to the right
	wy := sy1 + 36
	a.skDimLine(sx0, wy, sx1, wy)
	b.Line(sx0, sy1+4, sx0, wy+8, 1, colDimLine)
	b.Line(sx1, sy1+4, sx1, wy+8, 1, colDimLine)
	hx := sx1 + 36
	a.skDimLine(hx, sy0, hx, sy1)
	b.Line(sx1+4, sy0, hx+8, sy0, 1, colDimLine)
	b.Line(sx1+4, sy1, hx+8, sy1, 1, colDimLine)
	a.skDimLabel("sk-dim-w", x1-x0, (sx0+sx1)/2, wy, func(v float64) { a.skSetSize(0, v) })
	a.skDimLabel("sk-dim-h", y1-y0, hx, (sy0+sy1)/2, func(v float64) { a.skSetSize(1, v) })
}

// skLengthLabel is the label on the stretch being drawn: its length (bold,
// and Tab types it) and angle.
func (a *App) skLengthLabel(cx, cy, l, ang float64) {
	b := a.batch
	if a.field != nil && a.field.id == "sk-len" {
		r := render.Rect{X: cx + 14, Y: cy + 10, W: 76, H: 22}
		b.RoundRect(r, 3, colChrome)
		b.RoundRectStroke(r, 3, 1, colBlue)
		a.drawFieldText("sk-len", r, a.field.text, fontUI, true, colText)
		return
	}
	t := strings.TrimSpace(a.fmtLen(l))
	at := fmt.Sprintf("  %.0f°   Tab: type", ang)
	tw := b.TextWidth(t, fontSmall, true) + b.TextWidth(at, fontSmall, false)
	lr := render.Rect{X: cx + 14, Y: cy + 10, W: tw + 14, H: 20}
	b.RoundRect(lr, 4, colTooltip)
	x := lr.X + 7
	ty := lr.CY() - b.LineHeight(fontSmall, true)/2
	x += b.Text(t, x, ty, fontSmall, true, colOnColor)
	b.Text(at, x, ty, fontSmall, false, colTooltipKey)
}
