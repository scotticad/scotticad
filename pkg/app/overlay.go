//go:build windows

package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/win"
)

// dimLabel is an on-canvas number that can be clicked and typed into.
type dimLabel struct {
	id     string
	x, y   float64
	value  float64 // mm
	text   string  // overrides the formatted value
	commit func(v float64)
	signed bool
	angle  bool // degrees, not a length
}

func (a *App) drawViewportOverlay() {
	b := a.batch
	// the viewport itself (lowest region)
	a.reg(region{r: a.vp, id: "viewport", press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
		a.viewPress(x, y, m)
		return nil
	}})

	b.PushClip(a.vp)
	a.handles = a.computeHandles()
	a.labels = a.labels[:0]

	a.drawRuler()
	a.drawRulerGhost()
	a.drawWorkplaneGhost()
	a.drawProtractor()
	a.drawDims()
	a.drawMoveBars()
	a.drawHandles()
	a.drawNotes() // over the size lines: the Z rotation handle sits on one
	a.drawAlignMirror()
	a.drawSplit()
	a.drawSection()
	a.drawMeasure()
	a.drawMarquee()
	a.drawLabels()
	a.drawGlow()
	a.drawBanner()
	a.drawWorking()
	b.PopClip()

	a.drawLeftControls()
	a.drawSnapControls()
	if !a.panelOpen {
		a.drawPanelToggle(a.vp.X + a.vp.W - 14)
	}
}

// --- handles ----------------------------------------------------------------------

func (a *App) drawHandles() {
	if a.drag != nil && a.drag.kind == dragRotate {
		return // only the protractor shows while rotating
	}
	b := a.batch
	// multi-selections and bundles get a dashed footprint box
	if len(a.handles) >= 4 {
		ns := a.ed.Selected()
		if len(ns) > 1 || (len(ns) == 1 && ns[0].IsBundle()) {
			for i := 0; i < 4; i++ {
				p, q := a.handles[i], a.handles[(i+1)%4]
				if p.ok && q.ok {
					b.Dashed(p.x, p.y, q.x, q.y, 1, 5, colHandleDark.A(0.7))
				}
			}
		}
	}
	for i, h := range a.handles {
		if !h.ok {
			continue
		}
		hot := i == a.hoverHandle || (a.drag != nil && a.drag.h.kind == h.kind && a.drag.h.idx == h.idx && a.drag.orig != nil)
		if p := a.pinnedDims(); p != nil && a.drag == nil && p.h.kind == h.kind && p.h.idx == h.idx {
			hot = true
		}
		s := handleSize
		if hot {
			s += 2
		}
		r := rectAround(h.x, h.y, s/2)
		switch h.kind {
		case hCorner:
			fill := colHandleDark
			if hot {
				fill = colHandleHot
			}
			b.Fill(r.Inset(-1), colHandleLight)
			b.Fill(r, fill)
		case hEdge, hTop:
			b.Fill(r.Inset(-1), colHandleDark)
			c := colHandleLight
			if hot {
				c = colHandleHot
			}
			b.Fill(r, c)
		case hLift:
			// a drop: pointed top, round bottom
			c := colHandleDark
			if hot {
				c = colHandleHot
			}
			r := 5.5
			b.Circle(h.x, h.y+3, r, c)
			b.Tri(h.x, h.y-9, h.x+r*0.93, h.y+1, h.x-r*0.93, h.y+1, c)
		case hRot:
			c := colProtractor
			if hot {
				c = colHandleHot
			}
			if len(h.arc) > 1 {
				b.Polyline(h.arc, 2.2, c)
				arrowHeadSize(b, h.arc[1], h.arc[0], 8, c)
				arrowHeadSize(b, h.arc[len(h.arc)-2], h.arc[len(h.arc)-1], 8, c)
			}
		}
	}
}

func arrowHead(b *render.Batch, from, to [2]float64, c render.Color) {
	arrowHeadSize(b, from, to, 6, c)
}

// arrowHeadSize draws an arrowhead of length s whose tip is at to.
func arrowHeadSize(b *render.Batch, from, to [2]float64, s float64, c render.Color) {
	dx, dy := to[0]-from[0], to[1]-from[1]
	l := math.Hypot(dx, dy)
	if l < 1e-6 {
		return
	}
	dx, dy = dx/l, dy/l
	b.Tri(to[0]+dx*s*0.35, to[1]+dy*s*0.35, to[0]-dx*s*0.65-dy*s*0.45, to[1]-dy*s*0.65+dx*s*0.45, to[0]-dx*s*0.65+dy*s*0.45, to[1]-dy*s*0.65-dx*s*0.45, c)
}

// screenDir is the on-screen unit direction of the 3D direction d at p.
func (a *App) screenDir(p, d geom.Vec3) ([2]float64, bool) {
	x0, y0, ok0 := a.projectW(p)
	x1, y1, ok1 := a.projectW(p.Add(d.Mul(a.pixelSizeW(p))))
	l := math.Hypot(x1-x0, y1-y0)
	if !ok0 || !ok1 || l < 1e-9 {
		return [2]float64{}, false
	}
	return [2]float64{(x1 - x0) / l, (y1 - y0) / l}, true
}

// band fills the strip beside the screen segment s0-s1 between offsets w0
// and w1 along n.
func band(b *render.Batch, s0, s1, n [2]float64, w0, w1 float64, c render.Color) {
	b.Quad(s0[0]+n[0]*w0, s0[1]+n[1]*w0, s1[0]+n[0]*w0, s1[1]+n[1]*w0,
		s1[0]+n[0]*w1, s1[1]+n[1]*w1, s0[0]+n[0]*w1, s0[1]+n[1]*w1, c)
}

// labelBeyond places a label of text txt just past dist pixels from (x, y)
// along n, so the label's box clears that distance.
func (a *App) labelBeyond(x, y float64, n [2]float64, dist float64, txt string) (float64, float64) {
	w := math.Max(38, a.batch.TextWidth(txt, fontLabel, false)+2*labelPadX)
	reach := math.Abs(n[0])*w/2 + math.Abs(n[1])*10
	return x + n[0]*(dist+reach+4), y + n[1]*(dist+reach+4)
}

// --- protractor -------------------------------------------------------------------

func (a *App) drawProtractor() {
	var axis int
	var c geom.Vec3
	var R float64
	var hot *handle
	// while dragging, or after a rotation until something else happens
	// (then the angle can be typed), the protractor shows the sweep
	var rd *dragState
	kept := a.keptRotation()
	switch {
	case a.drag != nil && a.drag.kind == dragRotate:
		rd = a.drag
	case kept != nil && a.drag == nil:
		rd = kept.d
	case a.hoverHandle >= 0 && a.hoverHandle < len(a.handles) && a.handles[a.hoverHandle].kind == hRot && a.drag == nil:
		hot = &a.handles[a.hoverHandle]
		box := a.ed.SelectionBoundsIn()
		axis, c, R = hot.idx, box.Center(), a.protractorRadius(box, hot.idx)
	default:
		return
	}
	dragging := rd != nil
	if dragging {
		axis, c, R = rd.axis, rd.center, rd.ringR
	}
	b := a.batch
	u, v := rotBasis(axis)
	pt := func(ang, r float64) ([2]float64, bool) {
		p := c.Add(u.Mul(r * math.Cos(ang))).Add(v.Mul(r * math.Sin(ang)))
		x, y, ok := a.projectW(p)
		return [2]float64{x, y}, ok
	}
	cx, cy, okc := a.projectW(c)
	if !okc {
		return
	}
	R2, R3 := R*protBandOut, R*protOuter
	// 0° points from the centre towards the handle, squared to the axes
	var a0 float64
	if dragging {
		a0 = rd.a0
	} else {
		dir := hot.p.Sub(c)
		a0 = math.Round(math.Atan2(dir.Dot(v), dir.Dot(u))/(math.Pi/2)) * (math.Pi / 2)
	}
	const n = 96
	ring := func(r float64, w float64, col render.Color) {
		var pts [][2]float64
		for i := 0; i <= n; i++ {
			p, ok := pt(2*math.Pi*float64(i)/n, r)
			if !ok {
				return
			}
			pts = append(pts, p)
		}
		b.Polyline(pts, w, col)
	}
	// the swept sector, out to the band's outer edge
	if dragging && rd.angle != 0 {
		a1 := a0 + rd.angle*math.Pi/180
		m := int(math.Max(4, math.Abs(a1-a0)/(math.Pi/48)))
		prev, _ := pt(a0, R2)
		for i := 1; i <= m; i++ {
			p, _ := pt(a0+(a1-a0)*float64(i)/float64(m), R2)
			b.Tri(cx, cy, prev[0], prev[1], p[0], p[1], colSweep)
			prev = p
		}
	}
	// the band, divided every 22.5°
	for i := 0; i < n; i++ {
		t0, t1 := 2*math.Pi*float64(i)/n, 2*math.Pi*float64(i+1)/n
		p0, ok0 := pt(t0, R)
		p1, ok1 := pt(t1, R)
		q0, ok2 := pt(t0, R2)
		q1, ok3 := pt(t1, R2)
		if ok0 && ok1 && ok2 && ok3 {
			b.Quad(p0[0], p0[1], p1[0], p1[1], q1[0], q1[1], q0[0], q0[1], colProtBand)
		}
	}
	ring(R, 1.2, colProtLine)
	ring(R2, 1.2, colProtLine)
	ring(R3, 1.2, colProtLine)
	for i := 0; i < 16; i++ {
		t := a0 + 2*math.Pi*float64(i)/16
		p0, ok0 := pt(t, R)
		p1, ok1 := pt(t, R2)
		if ok0 && ok1 {
			b.Line(p0[0], p0[1], p1[0], p1[1], 1, colProtLine)
		}
	}
	// out on the outer circle: degree ticks for 1° steps
	if dragging && rd == a.drag && rd.onRing {
		for i := 0; i < 360; i++ {
			t := a0 + math.Pi*float64(i)/180
			l := 0.025
			if i%5 == 0 {
				l = 0.05
			}
			if math.Mod(float64(i), 22.5) == 0 {
				l = 0.08
			}
			p0, ok0 := pt(t, R3*(1-l))
			p1, ok1 := pt(t, R3)
			if ok0 && ok1 {
				b.Line(p0[0], p0[1], p1[0], p1[1], 1, colProtLine)
			}
		}
	}
	if !dragging {
		// hovering: an orange reference line to the handle and an arrow
		// across the band at 0°
		b.Line(cx, cy, hot.x, hot.y, 1.2, colProtRef)
		p0, ok0 := pt(a0, R)
		p1, ok1 := pt(a0, R2)
		if ok0 && ok1 {
			b.Line(p0[0], p0[1], p1[0], p1[1], 1.5, colDimLine)
			arrowHeadSize(b, p0, p1, 10, colDimLine)
		}
	} else {
		p1, _ := pt(a0+rd.angle*math.Pi/180, R2)
		b.Line(cx, cy, p1[0], p1[1], 1.2, colProtRef)
	}
	// the angle sits outside the outer circle at 0°; counterclockwise seen
	// from above shows as negative
	txt := "0°"
	if dragging {
		txt = fmtAngle(-rd.angle) + "°"
	}
	if p, ok := pt(a0, R3); ok {
		dx, dy := p[0]-cx, p[1]-cy
		if l := math.Hypot(dx, dy); l > 1e-6 {
			lx, ly := a.labelBeyond(p[0], p[1], [2]float64{dx / l, dy / l}, 2, txt)
			lb := dimLabel{id: "angle", x: lx, y: ly, text: txt}
			if kept != nil && rd == kept.d {
				// after the drag the angle can be typed in
				lb.value, lb.angle = -rd.angle, true
				lb.commit = func(v float64) { a.setKeptAngle(-v) }
			}
			a.labels = append(a.labels, lb)
		}
	}
}

func fmtAngle(d float64) string {
	v := math.Round(d*10) / 10
	if v == 0 {
		v = 0 // not "-0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// --- dimensions --------------------------------------------------------------------

// dimsShown says which size lines to draw (x, y, z, elevation): all of
// them with the ruler, otherwise those of the handle being hovered,
// dragged or clicked: a corner shows length and width, a side handle its
// side, the top handle the height.
func (a *App) dimsShown() (show [4]bool) {
	if !a.showHandles() || (a.drag != nil && a.drag.kind == dragRotate) {
		return
	}
	all := [4]bool{true, true, true, true}
	if a.ed.Ruler != nil {
		return all
	}
	var h *handle
	switch {
	case a.drag != nil && (a.drag.kind == dragResize || a.drag.kind == dragLift):
		h = &a.drag.h
	case a.drag == nil && a.hoverHandle >= 0 && a.hoverHandle < len(a.handles) && a.handles[a.hoverHandle].kind != hRot:
		h = &a.handles[a.hoverHandle]
	case a.drag == nil && a.pinnedDims() != nil:
		h = &a.dimPin.h
	}
	if h == nil {
		if (a.field != nil && strings.HasPrefix(a.field.id, "dim-")) || strings.HasPrefix(a.hot, "dim-") {
			return all
		}
		return
	}
	switch h.kind {
	case hCorner:
		show[0], show[1] = true, true
	case hEdge:
		show[1-h.idx%2] = true // edges 0, 2 sit on the Y faces
	case hTop:
		show[2] = true
	case hLift:
		show[3] = true
	}
	return
}

func (a *App) drawDims() {
	show := a.dimsShown()
	if show == ([4]bool{}) {
		return
	}
	ed := a.ed
	box := ed.SelectionBoundsIn()
	if box.IsEmpty() {
		return // nothing to measure (shapes that combine to nothing): no NaN labels
	}
	c := box.Center()
	z0 := box.Min.Z
	ps := a.pixelSizeW(c)
	off := dimOffset * ps
	b := a.batch
	const half = 7.0 // half the highlight band, px
	// a size line from p0 to p1 (3D), offset from the box along out: a
	// light-blue band with the arrowed line in it, short end caps, and the
	// label beyond the band
	sizeBar := func(p0, p1, out geom.Vec3, value float64) (float64, float64, bool) {
		s0x, s0y, ok0 := a.projectW(p0)
		s1x, s1y, ok1 := a.projectW(p1)
		n, okn := a.screenDir(p0.Lerp(p1, 0.5), out)
		if !ok0 || !ok1 || !okn {
			return 0, 0, false
		}
		s0, s1 := [2]float64{s0x, s0y}, [2]float64{s1x, s1y}
		dx, dy := s1x-s0x, s1y-s0y
		l := math.Hypot(dx, dy)
		if l < 1e-6 {
			return 0, 0, false
		}
		perp := [2]float64{-dy / l, dx / l} // across the line
		if perp[0]*n[0]+perp[1]*n[1] < 0 {
			perp = [2]float64{-perp[0], -perp[1]}
		}
		band(b, s0, s1, perp, -half, half, colDimBand)
		for _, e := range [][2]float64{s0, s1} {
			b.Line(e[0]-perp[0]*(half+2), e[1]-perp[1]*(half+2), e[0]+perp[0]*(half+2), e[1]+perp[1]*(half+2), 1, colDimLine)
		}
		b.Line(s0x, s0y, s1x, s1y, 1.2, colDimLine)
		if l > 18 {
			arrowHeadSize(b, s1, s0, 8, colDimLine)
			arrowHeadSize(b, s0, s1, 8, colDimLine)
		}
		lx, ly := a.labelBeyond((s0x+s1x)/2, (s0y+s1y)/2, perp, half, a.fmtLen(value))
		return lx, ly, true
	}
	ext := func(p0, p1 geom.Vec3) {
		x0, y0, ok0 := a.projectW(p0)
		x1, y1, ok1 := a.projectW(p1)
		if ok0 && ok1 {
			b.Line(x0, y0, x1, y1, 0.8, colDimLine.A(0.75))
		}
	}
	size := box.Size()
	// a label's commit works from the shapes as they are when it commits
	// (another label's commit, an undo or a click may have changed them
	// since this frame), and on the shapes it was drawn for
	ids := append([]int(nil), ed.Sel...)
	boxNow := func() geom.Box { return doc.BoundsIn(ed.Doc.FindAll(ids), ed.Work()) }
	resizeTo := func(axis int) func(v float64) {
		return func(v float64) {
			if v <= 0 {
				a.showToast("a size must be more than 0")
				return
			}
			nb := boxNow()
			if nb.IsEmpty() {
				return
			}
			c := nb.Center()
			// with the ruler measuring to endpoints, the measured edge stays
			// put and the size grows from it; otherwise about the middle.
			// A height grows from the bottom, the elevation it measures,
			// or about the middle when the ruler measures to midpoints
			anchor, mid := 0, false
			if ed.Ruler != nil {
				anchor, mid = ed.Ruler.Anchor(nb, axis), ed.Ruler.Mid
			}
			switch {
			case axis == 2 && !mid:
				nb.Max.Z = nb.Min.Z + v
			case axis == 2:
				nb.Min.Z, nb.Max.Z = c.Z-v/2, c.Z+v/2
			case anchor < 0:
				nb.Max = nb.Max.WithAxis(axis, nb.Min.Axis(axis)+v)
			case anchor > 0:
				nb.Min = nb.Min.WithAxis(axis, nb.Max.Axis(axis)-v)
			default:
				nb.Min = nb.Min.WithAxis(axis, c.Axis(axis)-v/2)
				nb.Max = nb.Max.WithAxis(axis, c.Axis(axis)+v/2)
			}
			if err := ed.SetBoundsIn(ids, nb); err != nil {
				a.showToast(err.Error())
			}
		}
	}
	// size lines go on the sides facing the camera, so the object never
	// hides them; the height goes at the corner where those sides' far
	// ends meet the near ones (for a front-left view: width in front, depth
	// on the left, height on the right)
	eye := a.eyeW()
	xn, sx := box.Min.X, -1.0
	if eye.X > c.X {
		xn, sx = box.Max.X, 1
	}
	yn, sy := box.Min.Y, -1.0
	if eye.Y > c.Y {
		yn, sy = box.Max.Y, 1
	}
	xf := box.Min.X + box.Max.X - xn
	outX, outY := geom.V(sx, 0, 0), geom.V(0, sy, 0)
	// with the ruler measuring to midpoints, heights are measured to the
	// middle of the shape too
	mid := ed.Ruler != nil && ed.Ruler.Mid
	sizeLine := func(axis int, p0, p1, out geom.Vec3) {
		id := "dim-" + "xy"[axis:axis+1]
		if x, y, ok := sizeBar(p0, p1, out, size.Axis(axis)); ok {
			a.labels = append(a.labels, dimLabel{id: id, x: x, y: y, value: size.Axis(axis), commit: resizeTo(axis)})
		}
	}
	// X along the near Y side
	yl := yn + sy*off
	if show[0] {
		ext(geom.V(box.Min.X, yn, z0), geom.V(box.Min.X, yl+sy*off*0.35, z0))
		ext(geom.V(box.Max.X, yn, z0), geom.V(box.Max.X, yl+sy*off*0.35, z0))
		sizeLine(0, geom.V(box.Min.X, yl, z0), geom.V(box.Max.X, yl, z0), outY)
	}
	// Y along the near X side
	xl := xn + sx*off
	if show[1] {
		ext(geom.V(xn, box.Min.Y, z0), geom.V(xl+sx*off*0.35, box.Min.Y, z0))
		ext(geom.V(xn, box.Max.Y, z0), geom.V(xl+sx*off*0.35, box.Max.Y, z0))
		sizeLine(1, geom.V(xl, box.Min.Y, z0), geom.V(xl, box.Max.Y, z0), outX)
	}
	// Z beside the far-X, near-Y corner
	zx := xf - sx*off
	if show[2] {
		ext(geom.V(xf, yn, z0), geom.V(zx-sx*off*0.35, yn, z0))
		ext(geom.V(xf, yn, box.Max.Z), geom.V(zx-sx*off*0.35, yn, box.Max.Z))
		if x, y, ok := sizeBar(geom.V(zx, yn, z0), geom.V(zx, yn, box.Max.Z), outX.Neg(), size.Z); ok {
			a.labels = append(a.labels, dimLabel{id: "dim-z", x: x, y: y, value: size.Z, commit: resizeTo(2)})
		}
	}
	// elevation above the workplane: a green bar continuing the height
	// line down to the workplane
	ze := z0 // the height measured: the bottom, or the middle
	if mid {
		ze = (z0 + box.Max.Z) / 2
	}
	elev := ze
	if show[3] {
		// beside the height line, further out, so the two don't overlap
		ex := zx - sx*off*1.6
		p0 := geom.V(ex, yn, 0)
		p1 := geom.V(ex, yn, ze)
		x0, y0, ok0 := a.projectW(p0)
		x1, y1, ok1 := a.projectW(p1)
		n, okn := a.screenDir(p0, outX.Neg())
		if ok0 && ok1 && okn {
			s0, s1 := [2]float64{x0, y0}, [2]float64{x1, y1}
			if math.Hypot(x1-x0, y1-y0) > 1 {
				band(b, s0, s1, n, -half, half, colRulerBand)
				b.Line(x0, y0, x1, y1, 1.2, colRulerArrow)
				arrowHeadSize(b, s0, s1, 8, colRulerArrow)
			}
			lx, ly := a.labelBeyond((x0+x1)/2, (y0+y1)/2, n, half, a.fmtLen(elev))
			a.labels = append(a.labels, dimLabel{id: "dim-elev", x: lx, y: ly + 0.01, value: elev, signed: true, commit: func(v float64) {
				nb := boxNow()
				if nb.IsEmpty() {
					return
				}
				z := nb.Min.Z // measured as the label measures: the bottom, or the middle
				if mid {
					z = (nb.Min.Z + nb.Max.Z) / 2
				}
				ed.MoveIn(ids, geom.V(0, 0, v-z))
			}})
		}
	}
}

// drawMoveBars shows how far a move (in progress, or just finished and
// still selected) has gone when there is no ruler: an L of arrows from the
// starting corner along X and Y, with the distances editable afterwards.
func (a *App) drawMoveBars() {
	if a.ed.Ruler != nil || !a.showHandles() {
		return
	}
	var from geom.Box
	editable := false
	switch {
	case a.drag != nil && a.drag.kind == dragMove && a.drag.orig != nil:
		from = a.drag.box
	case a.drag == nil && a.keptMove() != nil:
		from, editable = a.moveKeep.from, true
	default:
		return
	}
	ed := a.ed
	b := a.batch
	// measured at the corner the height bar uses: far X, near Y
	c := from.Center()
	eye := a.eyeW()
	maxX, maxY := eye.X < c.X, eye.Y > c.Y
	corner := func(bx geom.Box) geom.Vec3 {
		p := geom.V(bx.Min.X, bx.Min.Y, from.Min.Z)
		if maxX {
			p.X = bx.Max.X
		}
		if maxY {
			p.Y = bx.Max.Y
		}
		return p
	}
	to := ed.SelectionBoundsIn()
	if from.IsEmpty() || to.IsEmpty() {
		return
	}
	p0, p1 := corner(from), corner(to)
	outX, outY := geom.V(-1, 0, 0), geom.V(0, -1, 0)
	if maxX {
		outX = geom.V(1, 0, 0)
	}
	if maxY {
		outY = geom.V(0, 1, 0)
	}
	seg := func(q0, q1 geom.Vec3, w float64, arrow bool) ([2]float64, [2]float64, bool) {
		x0, y0, ok0 := a.projectW(q0)
		x1, y1, ok1 := a.projectW(q1)
		if !ok0 || !ok1 {
			return [2]float64{}, [2]float64{}, false
		}
		s0, s1 := [2]float64{x0, y0}, [2]float64{x1, y1}
		b.Line(x0, y0, x1, y1, w, colDimLine)
		if arrow && math.Hypot(x1-x0, y1-y0) > 10 {
			arrowHeadSize(b, s0, s1, 9, colDimLine)
		}
		return s0, s1, true
	}
	for axis, out := range []geom.Vec3{outY, outX} {
		d := p1.Axis(axis) - p0.Axis(axis)
		if math.Abs(d) < 1e-6 {
			continue
		}
		end := p0.WithAxis(axis, p1.Axis(axis))
		seg(p1, end, 0.8, false) // from the shape's corner to the arrow's tip
		s0, s1, ok := seg(p0, end, 1.2, true)
		if !ok {
			continue
		}
		n, ok := a.screenDir(p0.Lerp(end, 0.5), out)
		if !ok {
			continue
		}
		lx, ly := a.labelBeyond((s0[0]+s1[0])/2, (s0[1]+s1[1])/2, n, 6, a.fmtLen(d))
		l := dimLabel{id: "move-" + "xy"[axis:axis+1], x: lx, y: ly, value: d, signed: true}
		if editable {
			ax := axis
			l.commit = func(v float64) {
				now := corner(ed.SelectionBoundsIn()).Axis(ax) - p0.Axis(ax)
				ed.MoveIn(nil, geom.V(0, 0, 0).WithAxis(ax, v-now))
				if k := a.moveKeep; k != nil {
					k.rev = a.ed.Version // this edit keeps the bars up
				}
			}
		}
		a.labels = append(a.labels, l)
	}
}

// --- ruler ---------------------------------------------------------------------------

// drawWorkplaneGhost previews the Workplane tool: the plane the hovered
// shape would carry, on its own bottom.
func (a *App) drawWorkplaneGhost() {
	if a.mode != modeWorkplane || a.drag != nil || a.hot != "viewport" {
		return
	}
	h, ok := a.pick(a.mx, a.my, false, nil)
	if !ok {
		return
	}
	f := editor.ShapeFrame(h.node)
	bx := doc.BoundsIn([]*doc.Node{h.node}, f)
	if bx.IsEmpty() {
		return
	}
	m := 0.25 * math.Max(bx.Size().X, bx.Size().Y)
	cs := []geom.Vec3{geom.V(bx.Min.X-m, bx.Min.Y-m, 0), geom.V(bx.Max.X+m, bx.Min.Y-m, 0), geom.V(bx.Max.X+m, bx.Max.Y+m, 0), geom.V(bx.Min.X-m, bx.Max.Y+m, 0)}
	var s [4][2]float64
	for i, c := range cs {
		x, y, ok := a.project(f.Point(c))
		if !ok {
			return
		}
		s[i] = [2]float64{x, y}
	}
	b := a.batch
	b.Quad(s[0][0], s[0][1], s[1][0], s[1][1], s[2][0], s[2][1], s[3][0], s[3][1], colShapeWP)
	for i := 0; i < 4; i++ {
		j := (i + 1) % 4
		b.Line(s[i][0], s[i][1], s[j][0], s[j][1], 2, render.Color{colShapeWP[0], colShapeWP[1], colShapeWP[2], 0.9})
	}
}

// drawRulerGhost shows the ruler tool following the pointer until it is
// placed: a small L lying on the workplane with its arms on their outer
// sides and a red ring at the (snapped) origin.
func (a *App) drawRulerGhost() {
	if a.mode != modeRulerPlace || a.drag != nil || a.hot != "viewport" {
		return
	}
	p, ok := a.planeHitW(a.mx, a.my, 0)
	if !ok {
		return
	}
	o := editorSnapXY(p, a.snap())
	o.Z = 0
	ps := a.pixelSizeW(o)
	L, w := 48*ps, 12*ps
	b := a.batch
	quad := func(c0, c1, c2, c3 geom.Vec3) {
		var s [4][2]float64
		for i, c := range []geom.Vec3{c0, c1, c2, c3} {
			x, y, ok := a.projectW(c)
			if !ok {
				return
			}
			s[i] = [2]float64{x, y}
		}
		b.Quad(s[0][0], s[0][1], s[1][0], s[1][1], s[2][0], s[2][1], s[3][0], s[3][1], colDimBand)
		for i := 0; i < 4; i++ {
			j := (i + 1) % 4
			b.Line(s[i][0], s[i][1], s[j][0], s[j][1], 1, colRulerTick.A(0.8))
		}
	}
	// X arm below its line, Y arm left of its line (the ticks' sides)
	quad(o, o.Add(geom.V(L, 0, 0)), o.Add(geom.V(L, -w, 0)), o.Add(geom.V(0, -w, 0)))
	quad(o.Add(geom.V(-w, 0, 0)), o, o.Add(geom.V(0, L*0.8, 0)), o.Add(geom.V(-w, L*0.8, 0)))
	var ring [][2]float64
	for i := 0; i <= 32; i++ {
		t := 2 * math.Pi * float64(i) / 32
		x, y, ok := a.projectW(o.Add(geom.V(math.Cos(t), math.Sin(t), 0).Mul(6 * ps)))
		if !ok {
			return
		}
		ring = append(ring, [2]float64{x, y})
	}
	b.Polyline(ring, 2, colMarquee)
}

func (a *App) drawRuler() {
	r := a.ed.Ruler
	if r == nil {
		return
	}
	b := a.batch
	o := r.Origin
	o.Z = 0
	s := a.ed.Doc.Settings
	L := math.Max(s.WorkplaneW, s.WorkplaneL) * 0.5
	step, mid, major := 1.0, 5.0, 10.0
	if s.Units == "in" {
		step, mid, major = 25.4/16, 25.4/2, 25.4
	}
	ps := a.pixelSizeW(o)
	if step/ps < 4 {
		step = mid
	}
	// the arms, with ticks on their outer sides: long major and shorter mid
	// ticks, fine ones only when zoomed in
	for k := 0; k < 2; k++ {
		dir, out := r.Arm(k), r.Arm(1-k).Neg()
		x0, y0, ok0 := a.projectW(o.Sub(dir.Mul(12 * ps)))
		x1, y1, ok1 := a.projectW(o.Add(dir.Mul(L)))
		if ok0 && ok1 {
			b.Line(x0, y0, x1, y1, 1.5, colRulerTick)
		}
		for i := 1; float64(i)*step <= L+1e-9; i++ {
			v := float64(i) * step
			tl := 6.0
			if math.Abs(math.Mod(v+1e-9, major)) < 1e-6 {
				tl = 20
			} else if math.Abs(math.Mod(v+1e-9, mid)) < 1e-6 {
				tl = 12
			}
			p := o.Add(dir.Mul(v))
			q := p.Add(out.Mul(tl * ps))
			px, py, ok := a.projectW(p)
			qx, qy, ok2 := a.projectW(q)
			if ok && ok2 {
				b.Line(px, py, qx, qy, 1.2, colRulerTick)
			}
		}
	}
	ox, oy, ok := a.projectW(o)
	if !ok {
		return
	}
	oc := colHandleDark
	if a.isHot("ruler-origin") || a.pressed == "ruler-origin" {
		oc = colHandleHot
	}
	b.Circle(ox, oy, 7, colHandleLight)
	b.Ring(ox, oy, 7, 2, oc)
	// drag the origin to move the ruler; click it to turn the ruler a
	// quarter turn counter-clockwise
	a.reg(region{r: rectAround(ox, oy, 10), id: "ruler-origin", cursor: win.CursorSizeAll, tip: "Drag to move the ruler, click to rotate it", tipSide: tipBelow,
		press: func(x0, y0 float64, m win.Mods) func(float64, float64, bool) {
			moved := false
			return func(x, y float64, up bool) {
				if math.Hypot(x-x0, y-y0) > 3 {
					moved = true
				}
				if moved {
					if p, ok := a.planeHitW(x, y, 0); ok {
						r.Origin = editorSnapXY(p, a.snap())
					}
				}
				if up && !moved {
					a.ed.TurnRuler()
					a.w.Redraw()
				}
			}
		}})
	// remove (×) and midpoint (☰) buttons below the origin
	col := colRulerBlue
	xr := render.Rect{X: ox - 22, Y: oy + 12, W: 16, H: 16}
	b.Circle(xr.CX(), xr.CY(), 8, colHandleLight)
	b.Ring(xr.CX(), xr.CY(), 8, 1, col)
	drawIcon(b, icClose, xr.Inset(3), col)
	a.button("ruler-x-btn", xr, "Remove ruler", "", tipBelow, a.ed.RemoveRuler)
	mr := render.Rect{X: ox + 6, Y: oy + 12, W: 16, H: 16}
	b.Circle(mr.CX(), mr.CY(), 8, colHandleLight)
	mc := col
	if r.Mid {
		mc = colBlue
	}
	b.Ring(mr.CX(), mr.CY(), 8, 1, mc)
	drawIcon(b, icMenu, mr.Inset(3), mc)
	tip := "Measure to midpoints"
	if r.Mid {
		tip = "Measure to edges"
	}
	a.button("ruler-mid-btn", mr, tip, "", tipBelow, func() { r.Mid = !r.Mid; a.w.Redraw() })

	// green distance bars to the selection, on the arms' outer sides with
	// an arrow to the measured edge and the value beyond the bar
	if len(a.ed.Sel) == 0 || !a.showHandles() {
		return
	}
	box := a.ed.SelectionBoundsIn()
	if box.IsEmpty() {
		return
	}
	ids := append([]int(nil), a.ed.Sel...) // a commit moves these, as they are then
	ed := a.ed
	const bw = 16.0 // bar width, px
	for k := 0; k < 2; k++ {
		dir, out := r.Arm(k), r.Arm(1-k).Neg()
		cur := r.Measure(box, k)
		x0, y0, ok0 := a.projectW(o)
		x1, y1, ok1 := a.projectW(o.Add(dir.Mul(cur)))
		n, okn := a.screenDir(o, out)
		if !ok0 || !ok1 || !okn {
			continue
		}
		s0, s1 := [2]float64{x0, y0}, [2]float64{x1, y1}
		if math.Hypot(x1-x0, y1-y0) > 1 {
			band(b, s0, s1, n, 0, bw, colRulerBand)
			m0 := [2]float64{x0 + n[0]*bw/2, y0 + n[1]*bw/2}
			m1 := [2]float64{x1 + n[0]*bw/2, y1 + n[1]*bw/2}
			b.Line(m0[0], m0[1], m1[0], m1[1], 1.2, colRulerArrow)
			arrowHeadSize(b, m0, m1, 8, colRulerArrow)
		}
		// a thin line from the arm across to the edge (or, measuring to
		// midpoints, through the middle) of the selection
		w := r.Arm(1 - k)
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, cx := range []float64{box.Min.X, box.Max.X} {
			for _, cy := range []float64{box.Min.Y, box.Max.Y} {
				v := geom.V(cx-o.X, cy-o.Y, 0).Dot(w)
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
		}
		reach := hi // across to the far side of the selection
		if hi < 0 {
			reach = lo
		}
		if !r.Mid {
			reach = lo // up to the near side
			if hi < 0 {
				reach = hi
			}
		}
		if lx0, ly0, ok := a.projectW(o.Add(dir.Mul(cur))); ok {
			if lx1, ly1, ok := a.projectW(o.Add(dir.Mul(cur)).Add(w.Mul(reach))); ok {
				b.Line(lx0, ly0, lx1, ly1, 0.9, colDimLine.A(0.8))
			}
		}
		lx, ly := a.labelBeyond((x0+x1)/2, (y0+y1)/2, n, bw, a.fmtLen(cur))
		kk := k
		a.labels = append(a.labels, dimLabel{id: "ruler-" + "xy"[k:k+1], x: lx, y: ly, value: cur, signed: true, commit: func(v float64) {
			now := doc.BoundsIn(ed.Doc.FindAll(ids), ed.Work())
			if !now.IsEmpty() {
				ed.MoveIn(ids, r.Arm(kk).Mul(r.MoveFor(now, kk, v)))
			}
		}})
	}
}

// --- labels --------------------------------------------------------------------------

func (a *App) labelText(l dimLabel) string {
	if l.text != "" {
		return l.text
	}
	return a.fmtLen(l.value)
}

func (a *App) drawLabels() {
	b := a.batch
	var order []string
	for _, l := range a.labels {
		if l.commit != nil {
			order = append(order, l.id)
		}
	}
	for _, l := range a.labels {
		txt := a.labelText(l)
		editing := a.field != nil && a.field.id == l.id
		if editing {
			txt = a.field.text
		}
		w := math.Max(38, b.TextWidth(txt, fontLabel, false)+2*labelPadX)
		r := render.Rect{X: l.x - w/2, Y: l.y - 10, W: w, H: 20}
		bg := colChrome
		border := colFieldBorder
		if editing {
			border = colBlue
		} else if a.isHot(l.id) && l.commit != nil {
			bg = colBlueLight
		}
		b.RoundRect(r, 3, bg)
		b.RoundRectStroke(r, 3, 1, border)
		a.drawFieldText(l.id, r, a.labelText(l), fontLabel, true, colText)
		if l.commit == nil {
			continue
		}
		ll := l
		a.reg(region{r: r, id: l.id, cursor: win.CursorIBeam, press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
			a.openLabel(ll.id, order)
			return nil
		}})
	}
}

// openLabel turns a dimension label into a text field.
func (a *App) openLabel(id string, order []string) {
	var l *dimLabel
	for i := range a.labels {
		if a.labels[i].id == id {
			l = &a.labels[i]
		}
	}
	if l == nil {
		return
	}
	commit := l.commit
	units := a.units()
	text := strings.TrimSpace(editor.Format(l.value, units, true))
	if l.angle {
		text = fmtAngle(l.value)
	}
	isAngle := l.angle
	a.openField(&textField{
		id:       id,
		text:     text,
		tabOrder: order,
		openNext: func(next string) { a.openLabel(next, order) },
		commit: func(s string) bool {
			if strings.TrimSpace(s) == text {
				return true // untouched: the rounded value shown isn't written back
			}
			if isAngle {
				v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "°")), 64)
				if err != nil {
					a.showToast("not an angle: " + s)
					return true
				}
				commit(v)
				return true
			}
			v, err := editor.Parse(s, units)
			if err != nil {
				a.showToast(err.Error())
				return true
			}
			commit(v)
			return true
		},
	})
}

// --- align / mirror ---------------------------------------------------------------------

func (a *App) drawAlignMirror() {
	b := a.batch
	if a.mode == modeAlign {
		ns := a.ed.Selected()
		if len(ns) == 0 {
			return
		}
		// handles, each joined to the box by a short line; grey where the
		// shapes already line up
		aligned := map[alignDot]bool{}
		for _, h := range a.alignDots() {
			if _, seen := aligned[h.d]; seen {
				continue
			}
			done := true
			for _, d := range a.alignDeltas(ns, h.d.axis, h.d.where) {
				if d.Len() > 1e-6 {
					done = false
				}
			}
			aligned[h.d] = done
		}
		for _, h := range a.alignDots() {
			hot := a.alignHover != nil && *a.alignHover == h.d
			c := colHandleDark
			if aligned[h.d] {
				c = render.RGB(0x9a, 0x9f, 0xa5)
			}
			r := 5.0
			if hot {
				r, c = 6.5, colHandleHot
			}
			b.Line(h.ax, h.ay, h.x, h.y, 1, c)
			b.Circle(h.x, h.y, r+1.2, colHandleLight)
			b.Circle(h.x, h.y, r, c)
		}
	}
	if a.mode == modeMirror {
		for _, m := range a.mirrorArrows() {
			c := colHandleDark
			if a.mirrorHover == m.axis+1 {
				c = colMirrorHot
			}
			b.Line(m.x0, m.y0, m.x1, m.y1, 3, c)
			mx, my := (m.x0+m.x1)/2, (m.y0+m.y1)/2
			arrowHead(b, [2]float64{mx, my}, [2]float64{m.x0, m.y0}, c)
			arrowHead(b, [2]float64{mx, my}, [2]float64{m.x1, m.y1}, c)
			arrowHead(b, [2]float64{mx, my}, [2]float64{m.x0 + (m.x0-mx)*0.25, m.y0 + (m.y0-my)*0.25}, c)
			arrowHead(b, [2]float64{mx, my}, [2]float64{m.x1 + (m.x1-mx)*0.25, m.y1 + (m.y1-my)*0.25}, c)
		}
	}
}

// --- measure ---------------------------------------------------------------------------

func (a *App) drawMeasure() {
	if a.mode != modeMeasure {
		return
	}
	b := a.batch
	ms := &a.measure
	pin := func(p geom.Vec3, c render.Color) {
		x, y, ok := a.project(p)
		if !ok {
			return
		}
		b.Circle(x, y-15, 7, c)
		b.Tri(x-6, y-12, x+6, y-12, x, y, c)
		b.Circle(x, y-15, 2.5, colOnColor)
	}
	if ms.stage >= 1 {
		pin(ms.a, colBlue)
	}
	var end geom.Vec3
	have := false
	if ms.stage == 2 {
		end, have = ms.b, true
	} else if a.vp.Contains(a.mx, a.my) && (a.hot == "viewport" || a.hot == "") { // over the view, not a panel
		if p, ok := a.pickPoint(a.mx, a.my); ok {
			pin(p, colBlue.A(0.6))
			if ms.stage == 1 {
				end, have = p, true
			}
		}
	}
	if ms.stage >= 1 && have {
		x0, y0, ok0 := a.project(ms.a)
		x1, y1, ok1 := a.project(end)
		if ok0 && ok1 {
			b.Dashed(x0, y0, x1, y1, 1.5, 6, colText)
			dist := end.Sub(ms.a).Len()
			l := dimLabel{id: "measure", x: (x0 + x1) / 2, y: (y0+y1)/2 - 14, value: dist}
			if ms.stage == 2 {
				pin(ms.b, colBlue)
				aPt, bPt, nodeID := ms.a, ms.b, ms.bNode
				l.commit = func(v float64) {
					if nodeID == 0 || v < 0 {
						return
					}
					n := a.ed.Doc.Find(nodeID)
					if n == nil {
						a.showToast("That shape is no longer in the design")
						return
					}
					if n.Locked {
						a.showToast("That shape is locked")
						return
					}
					dir := bPt.Sub(aPt)
					if dir.Len() < 1e-9 {
						return
					}
					delta := dir.Norm().Mul(v - dir.Len())
					a.ed.Move([]int{nodeID}, delta)
					a.measure.b = bPt.Add(delta)
				}
			}
			a.labels = append(a.labels, l)
		}
	}
}

func (a *App) drawMarquee() {
	d := a.drag
	if d == nil || d.kind != dragMarquee || !d.moved {
		return
	}
	b := a.batch
	x0, x1 := math.Min(d.sx, d.lx), math.Max(d.sx, d.lx)
	y0, y1 := math.Min(d.sy, d.ly), math.Max(d.sy, d.ly)
	b.Fill(render.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}, colMarquee.A(0.05))
	b.Dashed(x0, y0, x1, y0, 1, 5, colMarquee)
	b.Dashed(x1, y0, x1, y1, 1, 5, colMarquee)
	b.Dashed(x1, y1, x0, y1, 1, 5, colMarquee)
	b.Dashed(x0, y1, x0, y0, 1, 5, colMarquee)
}

// --- banners and glow ---------------------------------------------------------------------

func (a *App) bannerText() string {
	d := a.drag
	switch {
	case d != nil && d.kind == dragRotate:
		return "Hold [Shift] to snap 45° angles"
	case d != nil && d.kind == dragMove && d.orig != nil:
		if a.cruise {
			return "Place on surface: drop shapes onto other shapes. Press [C] to exit"
		}
		return "Hold [Shift] to move in a straight line"
	case d != nil && d.kind == dragResize:
		return "Hold [Shift] to scale uniformly"
	case a.mode == modeNotes:
		return "Click on a shape to add a note. Press [Esc] to exit"
	case a.mode == modeWorkplane:
		return "Click on a shape to place a construction plane on it, or on the build plate to go back to it"
	case a.mode == modeLayFlat:
		return "Click a face to lay it flat. Press [Esc] to exit"
	case a.place != nil:
		return "Hold [C] to ignore other shapes' surfaces"
	case a.mode == modeVisibility:
		return "Select shapes to hide or show them. Press [Esc] to exit"
	case a.mode == modeMeasure:
		return "Place start and end points to see the distance between. Type a new distance to move the shapes."
	case a.mode == modeRulerPlace:
		return "Click to place the Ruler. Press [Esc] to cancel"
	case a.cruise:
		return "Place on surface: drag shapes onto other shapes' surfaces. Press [C] to exit"
	}
	return ""
}

// drawWorking shows a banner while shapes are being combined in the
// background: "Loading <design>…" just after opening a design, "Combining
// shapes…" after an edit. It sits at the top of the viewport, below a tool
// hint if one is showing.
func (a *App) drawWorking() {
	n := doc.Pending()
	if n == 0 {
		a.loading = ""
		return
	}
	b := a.batch
	t := "Combining shapes\u2026"
	if a.loading != "" {
		t = "Loading " + a.loading + "\u2026"
	}
	if n > 1 {
		t += fmt.Sprintf("  (%d left)", n)
	}
	w := b.TextWidth(t, fontUI, true) + 52
	y := a.vp.Y + 12
	if a.bannerText() != "" {
		y += 40
	}
	r := render.Rect{X: a.vp.CX() - w/2, Y: y, W: w, H: 32}
	b.RoundRect(r, 16, colBusy)
	// a spinner: a ring with a gap that turns
	cx, cy := r.X+20, r.CY()
	ph := float64(time.Now().UnixMilli()%1000) / 1000 * 2 * math.Pi
	var pts [][2]float64
	for i := 0; i <= 16; i++ {
		ang := ph + 1.5*math.Pi*float64(i)/16
		pts = append(pts, [2]float64{cx + 7*math.Cos(ang), cy + 7*math.Sin(ang)})
	}
	b.Polyline(pts, 2.2, colOnColor)
	b.Text(t, r.X+36, r.CY()-b.LineHeight(fontUI, true)/2, fontUI, true, colOnColor)
	a.wakeAfter(50 * time.Millisecond) // keep it turning
}

func (a *App) drawBanner() {
	t := a.bannerText()
	if t == "" {
		return
	}
	b := a.batch
	type seg struct {
		s   string
		key bool
	}
	var segs []seg
	for t != "" {
		i := strings.Index(t, "[")
		if i < 0 {
			segs = append(segs, seg{t, false})
			break
		}
		j := strings.Index(t[i:], "]")
		if i > 0 {
			segs = append(segs, seg{t[:i], false})
		}
		segs = append(segs, seg{t[i+1 : i+j], true})
		t = t[i+j+1:]
	}
	w := 0.0
	for _, s := range segs {
		w += b.TextWidth(s.s, fontUI, false)
		if s.key {
			w += 14
		}
	}
	r := render.Rect{X: a.vp.CX() - w/2 - 16, Y: a.vp.Y + 12, W: w + 32, H: 32}
	b.RoundRect(r, 16, colBanner)
	x := r.X + 16
	ty := r.CY() - b.LineHeight(fontUI, false)/2
	for _, s := range segs {
		if s.key {
			kw := b.TextWidth(s.s, fontUI, false) + 10
			kr := render.Rect{X: x + 2, Y: r.CY() - 11, W: kw, H: 22}
			b.RoundRect(kr, 4, colChrome)
			b.RoundRectStroke(kr, 4, 1, colFieldBorder)
			b.TextCentered(s.s, kr, fontUI, false, colText)
			x += kw + 4
			continue
		}
		x += b.Text(s.s, x, ty, fontUI, false, colText)
	}
}

func (a *App) drawGlow() {
	if a.mode != modeVisibility && a.mode != modeMeasure {
		return
	}
	b := a.batch
	r := a.vp
	for i := 0; i < 10; i++ {
		al := float32(0.22 * (1 - float64(i)/10))
		c := colGlow.A(al)
		d := float64(i) * 1.5
		b.Fill(render.Rect{X: r.X + d, Y: r.Y + d, W: r.W - 2*d, H: 1.5}, c)
		b.Fill(render.Rect{X: r.X + d, Y: r.Y + r.H - d - 1.5, W: r.W - 2*d, H: 1.5}, c)
		b.Fill(render.Rect{X: r.X + d, Y: r.Y + d, W: 1.5, H: r.H - 2*d}, c)
		b.Fill(render.Rect{X: r.X + r.W - d - 1.5, Y: r.Y + d, W: 1.5, H: r.H - 2*d}, c)
	}
}

// --- left view controls -------------------------------------------------------------------

func (a *App) drawLeftControls() {
	b := a.batch
	cr := a.cubeRect()
	a.reg(region{r: cr.Inset(6), id: "viewcube", cursor: win.CursorHand, press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
		a.drag = &dragState{kind: dragCube, sx: x, sy: y, lx: x, ly: y}
		return nil
	}})
	type btn struct {
		id, tip, key string
		icon         int
		click        func()
	}
	orthoTip := "Switch to flat view (Orthographic)"
	orthoIcon := icPersp
	if a.cam.Ortho {
		orthoTip = "Switch to perspective view"
		orthoIcon = icOrtho
	}
	btns := []btn{
		{"home", "Home view", "", icHome, func() { a.homeCamera(true) }},
		{"fit", "Fit all in view", "F", icFit, a.fitView},
		{"zoomin", "Zoom in", "+", icZoomIn, func() { a.zoomAt(a.vp.CX(), a.vp.CY(), 0.8) }},
		{"zoomout", "Zoom out", "-", icZoomOut, func() { a.zoomAt(a.vp.CX(), a.vp.CY(), 1.25) }},
		{"ortho", orthoTip, "", orthoIcon, func() { a.setOrtho(!a.cam.Ortho) }},
	}
	y := cr.Y + cr.H + 12
	for _, bt := range btns {
		r := render.Rect{X: a.vp.X + leftColX, Y: y, W: leftBtn, H: leftBtn}
		hot := a.isHot(bt.id)
		bg := colChrome
		if hot {
			bg = colHoverBG
		}
		b.Circle(r.CX(), r.CY(), leftBtn/2+1, colBorder)
		b.Circle(r.CX(), r.CY(), leftBtn/2, bg)
		c := colIcon
		if hot {
			c = colIconHover
		}
		drawIcon(b, bt.icon, r.Inset(9), c)
		a.button(bt.id, r, bt.tip, bt.key, tipRight, bt.click)
		y += leftBtn + 12
	}
}

// --- snap grid and settings -----------------------------------------------------------

func (a *App) snapRect() render.Rect {
	return render.Rect{X: a.vp.X + a.vp.W - 96, Y: a.vp.Y + a.vp.H - 32, W: 80, H: 22}
}

func (a *App) drawSnapControls() {
	b := a.batch
	sr := a.snapRect()
	lbl := "Snap Grid"
	lw := b.TextWidth(lbl, fontSmall, false)
	b.Text(lbl, sr.X-lw-8, sr.CY()-b.LineHeight(fontSmall, false)/2, fontSmall, false, colText)
	bg := colChrome
	if a.isHot("snap") || a.popup == popSnap {
		bg = colHoverBG
	}
	b.RoundRect(sr, 3, bg)
	b.RoundRectStroke(sr, 3, 1, colFieldBorder)
	b.Text(editor.SnapLabel(a.snap(), a.units()), sr.X+8, sr.CY()-b.LineHeight(fontSmall, false)/2, fontSmall, false, colText)
	drawIcon(b, icChevronUp, render.Rect{X: sr.X + sr.W - 18, Y: sr.CY() - 7, W: 14, H: 14}, colIcon)
	a.button("snap", sr, "", "", 0, func() { a.openPopup(popSnap) })

	st := render.Rect{X: sr.X + sr.W - 70, Y: sr.Y - 30, W: 70, H: 22}
	bg = colChrome
	if a.isHot("settings") {
		bg = colHoverBG
	}
	b.RoundRect(st, 3, bg)
	b.RoundRectStroke(st, 3, 1, colFieldBorder)
	b.TextCentered("Settings", st, fontSmall, false, colText)
	a.button("settings", st, "", "", 0, func() {
		s := a.ed.Doc.Settings
		a.settingsDraft, a.settingsBase = &s, s
		a.openPopup(popSettings)
	})
}
