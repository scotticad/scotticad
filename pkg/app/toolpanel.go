//go:build windows

package app

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/win"
)

// A tool panel is the small floating bar of options a tool shows over
// the viewport (Split, Section view).

type panelItemKind int

const (
	panelCaption panelItemKind = iota // dim text
	panelToggle                       // a button, highlighted when on
	panelButton                       // a plain button
	panelPrimary                      // the blue action button
	panelCustom                       // drawn by draw, w wide
)

type panelItem struct {
	kind  panelItemKind
	id    string
	label string
	tip   string
	on    bool
	click func()
	w     float64
	draw  func(r render.Rect) // panelCustom
}

// drawToolPanel lays the items out in a row in a panel centred on the
// viewport, its top at y, and returns the panel's rectangle.
func (a *App) drawToolPanel(y float64, items []panelItem) render.Rect {
	b := a.batch
	const h = 40.0
	total := 16.0
	for i := range items {
		it := &items[i]
		switch it.kind {
		case panelCaption:
			it.w = b.TextWidth(it.label, fontUI, false) + 8
		case panelCustom:
		default:
			it.w = b.TextWidth(it.label, fontUI, it.kind == panelPrimary) + 20
		}
		total += it.w + 6
	}
	// centred in the viewport, or in the room left of the properties
	// panel when it is showing, so its buttons are never underneath it
	lo, hi := a.vp.X+64, a.vp.X+a.vp.W-8
	if len(a.ed.Sel) > 0 {
		hi = a.inspectorRect(0).X - 8
	}
	cx := (lo + hi) / 2
	if hi-lo < total { // not enough room: start at the left edge
		cx = lo + total/2
	}
	r := render.Rect{X: math.Round(cx - total/2), Y: y, W: total, H: h}
	a.panel(r)
	x := r.X + 10
	for _, it := range items {
		br := render.Rect{X: x, Y: r.Y + 7, W: it.w, H: h - 14}
		switch it.kind {
		case panelCaption:
			b.Text(it.label, x, br.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, colTextDim)
		case panelCustom:
			it.draw(br)
		case panelPrimary:
			a.primaryButton(it.id, br, it.label, it.click)
		default:
			a.textButton(it.id, br, it.label, it.on, it.click)
		}
		if it.tip != "" && it.kind != panelCustom && it.kind != panelCaption {
			// the button above registered the region; give it its tooltip
			if n := len(a.regions); n > 0 && a.regions[n-1].id == it.id {
				a.regions[n-1].tip, a.regions[n-1].tipSide = it.tip, tipBelow
			}
		}
		x += it.w + 6
	}
	return r
}

// axisItems are X / Y / Z toggles for a tool's plane.
func axisItems(prefix string, axis int, set func(int)) []panelItem {
	var out []panelItem
	for i, name := range []string{"X", "Y", "Z"} {
		i := i
		out = append(out, panelItem{kind: panelToggle, id: prefix + string(rune('x'+i)), label: name, on: axis == i, click: func() { set(i) }})
	}
	return out
}

// --- a plane square to a workplane axis (Split, Section view) ---------------------

// planeWorld is the plane square to the workplane's axis at position at:
// its unit normal (the + direction of the axis) and a point on it, in
// world space.
func (a *App) planeWorld(axis int, at float64) (n, p geom.Vec3) {
	f := a.ed.Work()
	u := geom.V(0, 0, 0).WithAxis(axis, 1)
	return f.Dir(u).Norm(), f.Point(u.Mul(at))
}

// clipPlane is the render clip for keeping the side of the plane through
// p that normal n points to.
func clipPlane(n, p geom.Vec3) [4]float32 {
	return [4]float32{float32(n.X), float32(n.Y), float32(n.Z), float32(-n.Dot(p))}
}

// planeCorners are the corners (workplane frame) of the plane square to
// axis at position at, spanning box b plus a margin.
func planeCorners(b geom.Box, axis int, at float64) [4]geom.Vec3 {
	u, v := (axis+1)%3, (axis+2)%3
	s := b.Size()
	m := 2 + 0.08*math.Max(s.Axis(u), s.Axis(v))
	corner := func(cu, cv float64) geom.Vec3 {
		return geom.V(0, 0, 0).WithAxis(axis, at).WithAxis(u, cu).WithAxis(v, cv)
	}
	u0, u1 := b.Min.Axis(u)-m, b.Max.Axis(u)+m
	v0, v1 := b.Min.Axis(v)-m, b.Max.Axis(v)+m
	return [4]geom.Vec3{corner(u0, v0), corner(u1, v0), corner(u1, v1), corner(u0, v1)}
}

// drawPlaneInScene draws a tool's plane: its outline, and a light fill if
// fill (in the scene's translucent pass).
func (a *App) drawPlaneInScene(cs [4]geom.Vec3, fill bool) {
	sc := a.scene
	if fill {
		sc.DrawTrisM(appendQuad(nil, cs[0], cs[1], cs[2], cs[3], colSplitPlane), 1, a.ed.Work())
	}
	var l render.Lines
	col := [4]float32{colSplitEdge[0], colSplitEdge[1], colSplitEdge[2], 0.95}
	for i := range cs {
		l.Add(cs[i], cs[(i+1)%4], col)
	}
	sc.DrawLinesM(&l, 1, a.ed.Work())
}

// planeDrag moves a plane (*at, along axis) with the pointer, snapped to
// the grid and kept within bounds() (a step inside it if inset: a cut on
// a face cuts nothing).
func (a *App) planeDrag(axis int, at *float64, bounds func() geom.Box, inset bool) func(x, y float64, m win.Mods) func(float64, float64, bool) {
	return func(x, y float64, m win.Mods) func(float64, float64, bool) {
		// along the axis line through the plane's middle over the shapes,
		// where the grip is (through the workplane's origin, far off, a
		// pixel on screen is a different length): the plane follows the
		// cursor
		b := bounds()
		n, q := a.planeWorld(axis, *at)
		p := a.ed.Work().Point(b.Center().WithAxis(axis, *at))
		if tt, ok := a.ray(x, y).Plane(q, n); ok {
			p = a.ray(x, y).At(tt) // where the press meets the plane: the grip
		}
		t0, ok := lineParam(p, n, a.ray(x, y))
		if !ok {
			return nil
		}
		at0 := *at
		return func(x, y float64, up bool) {
			t, ok := lineParam(p, n, a.ray(x, y))
			if !ok {
				return
			}
			v := at0 + t - t0
			s := a.snap()
			if s > 0 && a.mods&win.Alt == 0 {
				v = math.Round(v/s) * s
			}
			in := 0.0
			if inset {
				in = s
				if s <= 0 || 2*s >= b.Size().Axis(axis) {
					in = b.Size().Axis(axis) / 100
				}
			}
			*at = math.Max(b.Min.Axis(axis)+in, math.Min(b.Max.Axis(axis)-in, v))
			a.w.Redraw()
		}
	}
}

// lineParam is where along the line p + t·n (n a unit vector) the ray
// passes closest.
func lineParam(p, n geom.Vec3, r geom.Ray) (float64, bool) {
	w := p.Sub(r.O)
	b, c := n.Dot(r.D), r.D.Dot(r.D)
	den := c - b*b
	if math.Abs(den) < 1e-9 {
		return 0, false
	}
	return (b*r.D.Dot(w) - c*n.Dot(w)) / den, true
}

// drawPlaneGrip draws the grip that drags a tool's plane along its axis
// (beside the plane's edge nearest the camera, with arrows along the
// axis) and registers it; it returns the grip's screen position.
func (a *App) drawPlaneGrip(id, tip string, corners [4]geom.Vec3, axis int, at float64, press func(x, y float64, m win.Mods) func(float64, float64, bool)) (float64, float64, bool) {
	eye := a.eyeW()
	gp := corners[0].Add(corners[3]).Mul(0.5)
	if alt := corners[1].Add(corners[2]).Mul(0.5); alt.Sub(eye).Len() < gp.Sub(eye).Len() {
		gp = alt
	}
	gx, gy, ok := a.projectW(gp)
	if !ok {
		return 0, 0, false
	}
	step := 14 * a.pixelSizeW(gp)
	ux, uy, ok1 := a.projectW(gp.WithAxis(axis, at+step))
	dx, dy, ok2 := a.projectW(gp.WithAxis(axis, at-step))
	b := a.batch
	c := colSplitEdge
	if a.isHot(id) || a.pressed == id {
		c = colHandleHot
	}
	if ok1 && ok2 {
		b.Line(dx, dy, ux, uy, 2, c)
		arrowHead(b, [2]float64{gx, gy}, [2]float64{ux, uy}, c)
		arrowHead(b, [2]float64{gx, gy}, [2]float64{dx, dy}, c)
	}
	b.Circle(gx, gy, 7.5, colOnColor)
	b.Circle(gx, gy, 6, c)
	a.reg(region{r: render.Rect{X: gx - 11, Y: gy - 11, W: 22, H: 22}, id: id, cursor: win.CursorSizeAll, tip: tip, tipSide: tipAbove, press: press})
	return gx, gy, true
}
