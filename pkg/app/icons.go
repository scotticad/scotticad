package app

import (
	"math"

	"github.com/scotticad/scotticad/pkg/render"
)

// pen draws in a 24×24 design grid mapped onto a screen rectangle.
type pen struct {
	b *render.Batch
	r render.Rect
	c render.Color
	w float64
}

func newPen(b *render.Batch, r render.Rect, c render.Color) pen {
	return pen{b, r, c, math.Max(1.2, r.W/15)}
}

func (p pen) x(v float64) float64 { return p.r.X + v/24*p.r.W }
func (p pen) y(v float64) float64 { return p.r.Y + v/24*p.r.H }
func (p pen) s(v float64) float64 { return v / 24 * p.r.W }

func (p pen) line(x0, y0, x1, y1 float64) {
	p.b.Line(p.x(x0), p.y(y0), p.x(x1), p.y(y1), p.w, p.c)
}

func (p pen) poly(closed bool, pts ...float64) {
	var q [][2]float64
	for i := 0; i+1 < len(pts); i += 2 {
		q = append(q, [2]float64{p.x(pts[i]), p.y(pts[i+1])})
	}
	if closed {
		q = append(q, q[0])
	}
	p.b.Polyline(q, p.w, p.c)
}

func (p pen) fill(pts ...float64) {
	var q [][2]float64
	for i := 0; i+1 < len(pts); i += 2 {
		q = append(q, [2]float64{p.x(pts[i]), p.y(pts[i+1])})
	}
	p.b.Poly(q, p.c)
}

func (p pen) rect(x, y, w, h, rad float64) {
	p.b.RoundRectStroke(render.Rect{X: p.x(x), Y: p.y(y), W: p.s(w), H: p.s(h)}, p.s(rad), p.w, p.c)
}

func (p pen) fillRect(x, y, w, h, rad float64) {
	p.b.RoundRect(render.Rect{X: p.x(x), Y: p.y(y), W: p.s(w), H: p.s(h)}, p.s(rad), p.c)
}

func (p pen) circle(cx, cy, r float64) { p.b.Ring(p.x(cx), p.y(cy), p.s(r), p.w, p.c) }
func (p pen) disc(cx, cy, r float64)   { p.b.Circle(p.x(cx), p.y(cy), p.s(r), p.c) }

func (p pen) arc(cx, cy, r, a0, a1 float64) {
	n := 16
	var q [][2]float64
	for i := 0; i <= n; i++ {
		a := (a0 + (a1-a0)*float64(i)/float64(n)) * math.Pi / 180
		q = append(q, [2]float64{p.x(cx + r*math.Cos(a)), p.y(cy + r*math.Sin(a))})
	}
	p.b.Polyline(q, p.w, p.c)
}

func (p pen) dashed(x0, y0, x1, y1 float64) {
	p.b.Dashed(p.x(x0), p.y(y0), p.x(x1), p.y(y1), p.w*0.8, p.s(2), p.c)
}

// path draws a polyline given in design units.
func (p pen) path(pts [][2]float64) {
	q := make([][2]float64, len(pts))
	for i, v := range pts {
		q[i] = [2]float64{p.x(v[0]), p.y(v[1])}
	}
	p.b.Polyline(q, p.w, p.c)
}

// spline is a Catmull-Rom curve through pts, n steps per span, for the
// icons' hand-drawn curves.
func spline(pts [][2]float64, n int) [][2]float64 {
	at := func(i int) [2]float64 { return pts[max(0, min(len(pts)-1, i))] }
	var out [][2]float64
	for i := 0; i+1 < len(pts); i++ {
		p0, p1, p2, p3 := at(i-1), at(i), at(i+1), at(i+2)
		for j := range n {
			t := float64(j) / float64(n)
			t2, t3 := t*t, t*t*t
			var q [2]float64
			for k := range 2 {
				q[k] = 0.5 * (2*p1[k] + (p2[k]-p0[k])*t + (2*p0[k]-5*p1[k]+4*p2[k]-p3[k])*t2 + (3*p1[k]-p0[k]-3*p2[k]+p3[k])*t3)
			}
			out = append(out, q)
		}
	}
	return append(out, pts[len(pts)-1])
}

// Icon names.
const (
	icCopy = iota
	icPaste
	icDuplicate
	icDelete
	icUndo
	icRedo
	icCaret
	icBundle
	icUnion
	icUngroup
	icSplit
	icSection
	icLayFlat
	icSpool
	icAlign
	icMirror
	icCruise
	icMeasure
	icShapeWP
	icDrop
	icHome
	icFit
	icZoomIn
	icZoomOut
	icPersp
	icOrtho
	icOrbitCenter
	icGridTab
	icRulerTab
	icNotesTab
	icSearch
	icLock
	icUnlock
	icEye
	icEyeOff
	icChevronUp
	icChevronDown
	icChevronLeft
	icChevronRight
	icClose
	icInfo
	icWarning
	icCheck
	icMenu
	icHide
	icToggleVis
	icNotes
	icUnionGroupType
	icBundleGroupType
	icIntersectGroupType
	icHullGroupType
	icPattern
	icStretch
	icSkSelect
	icSkLine
	icSkBezier
	icSkSmooth
	icSkRefine
	icSkPen
	icSkEdit
	icSkDims
	icSkClose
	icSkStroke
	icSkCorner
	icSkSmoothPt
	icSkSplit
	icSkClear
	icSkImage
	icSkHole
	icSkMirrorTurn
	icToolSketch
	icToolRevolve
	icToolCoil
)

// drawIcon draws an icon in one colour.
func drawIcon(b *render.Batch, id int, r render.Rect, c render.Color) {
	drawIconAccent(b, id, r, c, c)
}

// drawIconAccent draws an icon in c with its key element (the shape the
// tool acts on, the arrowhead, the clip) in accent: a touch of colour makes
// the tools easier to tell apart.
func drawIconAccent(b *render.Batch, id int, r render.Rect, c, accent render.Color) {
	p := newPen(b, r, c)
	a := p // the accent pen
	a.c = accent
	switch id {
	case icCopy:
		a.rect(8, 3, 12, 14, 1.5)
		p.poly(false, 5, 7, 4, 7, 4, 21, 16, 21, 16, 20)
	case icPaste:
		p.rect(5, 5, 14, 16, 1.5)
		a.fillRect(9, 3, 6, 4, 1)
		p.line(8.5, 11, 15.5, 11)
		p.line(8.5, 15, 13.5, 15)
	case icDuplicate:
		p.rect(3, 3, 12, 12, 1.5)
		a.rect(9, 9, 12, 12, 1.5)
	case icDelete:
		p.line(4, 6, 20, 6)
		p.poly(false, 9, 6, 9, 3.5, 15, 3.5, 15, 6)
		p.poly(false, 6, 6, 7, 21, 17, 21, 18, 6)
		a.line(10, 10, 10, 17)
		a.line(14, 10, 14, 17)
	case icUndo:
		p.arc(13, 13, 7, 200, 360+60)
		a.fill(3, 8, 9, 6.5, 5.5, 13)
	case icRedo:
		p.arc(11, 13, 7, 120, 340)
		a.fill(21, 8, 15, 6.5, 18.5, 13)
	case icCaret:
		p.fill(7, 10, 17, 10, 12, 15)
	case icBundle:
		p.rect(2, 2, 20, 20, 0)
		p.w *= 0.7
		a.w = p.w
		a.fillRect(5, 7, 7, 7, 0.5)
		p.rect(11, 10, 8, 8, 0.5)
	case icUnion:
		p.fillRect(3, 8, 11, 11, 1)
		a.fillRect(9, 4, 12, 11, 1)
	case icUngroup:
		a.fillRect(3, 9, 9, 9, 1)
		p.rect(13, 4, 8, 8, 1)
		p.dashed(12, 12, 14, 12)
	case icSplit:
		// a block cut in two, the halves apart, the cut dashed between
		p.rect(4, 3, 16, 7, 1)
		a.fillRect(4, 14, 16, 7, 1)
		p.dashed(1, 12, 23, 12)
	case icSection:
		// a block cut open: the near half gone, the cut face filled
		p.rect(4, 4, 16, 16, 1)
		a.fillRect(4, 12, 16, 8, 1)
		p.dashed(1, 12, 23, 12)
	case icLayFlat:
		// a tilted block coming down onto its face on the workplane
		p.line(3, 21, 21, 21)
		p.poly(true, 6, 9, 13, 4, 18, 11, 11, 16)
		a.fillRect(7, 17, 10, 3, 0.5)
	case icSpool:
		// a filament spool: rim, hub, and the strand leaving it
		p.circle(11, 12, 8)
		a.circle(11, 12, 3)
		a.line(11, 4, 21, 4)
		a.line(21, 4, 21, 9)
	case icAlign:
		p.line(4, 3, 4, 21)
		a.fillRect(6.5, 6, 13, 4.5, 0.5)
		a.fillRect(6.5, 13.5, 8, 4.5, 0.5)
	case icMirror:
		p.dashed(12, 2, 12, 22)
		a.fill(10, 5, 10, 19, 2, 19)
		p.poly(true, 14, 5, 14, 19, 22, 19)
	case icCruise:
		p.arc(12, 13, 8, 180, 360)
		p.line(4, 13, 20, 13)
		a.fill(9, 13, 15, 13, 12, 8.5)
		p.line(12, 13, 12, 21)
	case icMeasure:
		p.circle(12, 11, 7)
		a.disc(12, 11, 2)
		p.line(12, 18, 12, 22)
		p.line(5, 22, 19, 22)
	case icShapeWP:
		p.poly(true, 2, 16, 12, 11, 22, 16, 12, 21)
		a.fillRect(9, 4, 6, 7, 1)
	case icDrop:
		p.line(3, 21, 21, 21)
		a.fillRect(7, 3, 10, 8, 1)
		p.line(12, 12, 12, 19)
		p.fill(8.5, 15.5, 15.5, 15.5, 12, 19.5)
	case icHome:
		p.poly(false, 3, 12, 12, 4, 21, 12)
		p.poly(false, 6, 10, 6, 20, 18, 20, 18, 10)
		p.fillRect(10, 14, 4, 6, 0.5)
	case icFit:
		p.poly(false, 3, 8, 3, 3, 8, 3)
		p.poly(false, 16, 3, 21, 3, 21, 8)
		p.poly(false, 21, 16, 21, 21, 16, 21)
		p.poly(false, 8, 21, 3, 21, 3, 16)
		p.rect(8, 8, 8, 8, 1)
	case icZoomIn:
		p.line(4, 12, 20, 12)
		p.line(12, 4, 12, 20)
	case icZoomOut:
		p.line(4, 12, 20, 12)
	case icPersp, icOrtho:
		p.poly(true, 12, 3, 20, 7.5, 20, 16.5, 12, 21, 4, 16.5, 4, 7.5)
		p.poly(false, 4, 7.5, 12, 12, 20, 7.5)
		p.line(12, 12, 12, 21)
		if id == icOrtho {
			p.fill(12, 3, 20, 7.5, 12, 12, 4, 7.5)
		}
	case icOrbitCenter:
		// a point with an arrow turning around it
		a.disc(12, 12, 2.2)
		p.arc(12, 12, 8, 0, 300)
		p.fill(18.4, 6.4, 17.2, 3.0, 14.8, 7.2)
	case icGridTab:
		p.rect(3, 3, 18, 18, 0)
		for _, v := range []float64{7.5, 12, 16.5} {
			p.line(v, 3, v, 21)
			p.line(3, v, 21, v)
		}
	case icRulerTab:
		p.poly(true, 3, 2, 8, 2, 8, 16, 22, 16, 22, 21, 3, 21)
		for _, v := range []float64{5, 8, 11} {
			p.line(5.5, v, 8, v)
		}
		for _, v := range []float64{12, 15, 18} {
			p.line(v, 16, v, 18.5)
		}
	case icNotesTab, icNotes:
		p.poly(true, 3, 3, 21, 3, 21, 16, 12, 16, 7, 21, 7, 16, 3, 16)
		p.line(7, 8, 17, 8)
		p.line(7, 11.5, 14, 11.5)
	case icToolSketch:
		// an outline drawn by hand, the pencil finishing it
		out := [][2]float64{{3, 21}}
		out = append(out, spline([][2]float64{{3, 11}, {8, 7.5}, {11.5, 11}, {16.5, 12}, {19, 16.5}, {17, 21}}, 6)...)
		a.path(append(out, [2]float64{3, 21}))
		p.poly(true, 22.6, 3.1, 17.1, 8.6, 14.9, 6.4, 20.4, 0.9)
		p.poly(false, 17.1, 8.6, 13.9, 9.6, 14.9, 6.4)
	case icToolRevolve:
		// a vase: the half you draw solid, the half the turn makes thin,
		// on its dashed axis
		half := spline([][2]float64{{12, 4}, {16.5, 4}, {15, 9}, {15.5, 13}, {19, 17}, {18.5, 20.5}, {12, 20.5}}, 5)
		a.path(half)
		mirror := make([][2]float64, len(half))
		for i, v := range half {
			mirror[i] = [2]float64{24 - v[0], v[1]}
		}
		thin := p
		thin.w *= 0.5
		thin.path(mirror)
		p.dashed(12, 1.5, 12, 23)
	case icToolCoil:
		// a spring from the side: the front of each turn solid, the back thin
		thin := a
		thin.w *= 0.5
		const turns, top, bot, n = 3, 5.0, 19.0, 72
		var run [][2]float64
		front := true
		for i := 0; i <= n; i++ {
			t := turns * 2 * math.Pi * float64(i) / n
			q := [2]float64{12 - 8*math.Cos(t), top + (bot-top)*float64(i)/n + 2.6*math.Sin(t)}
			run = append(run, q)
			if f := math.Sin(t+math.Pi/n) >= 0; f != front || i == n {
				switch {
				case len(run) < 2:
				case front:
					a.path(run)
				default:
					thin.path(run)
				}
				run, front = [][2]float64{q}, f
			}
		}
	case icSearch:
		p.circle(10, 10, 6)
		p.line(14.5, 14.5, 20.5, 20.5)
	case icLock, icUnlock:
		p.rect(5, 11, 14, 10, 1.5)
		if id == icLock {
			p.arc(12, 11, 4.5, 180, 360)
		} else {
			p.arc(16, 9, 4.5, 180, 360)
		}
		p.disc(12, 16, 1.4)
	case icEye, icEyeOff:
		p.arc(12, 18, 10, 215, 325)
		p.arc(12, 6, 10, 35, 145)
		a.circle(12, 12, 3)
		if id == icEyeOff {
			p.line(4, 4, 20, 20)
		}
	case icHide:
		drawIcon(b, icEyeOff, r, c)
	case icToggleVis:
		drawIcon(b, icEye, r, c)
	case icChevronUp:
		p.poly(false, 6, 15, 12, 9, 18, 15)
	case icChevronDown:
		p.poly(false, 6, 9, 12, 15, 18, 9)
	case icChevronLeft:
		p.poly(false, 15, 6, 9, 12, 15, 18)
	case icChevronRight:
		p.poly(false, 9, 6, 15, 12, 9, 18)
	case icClose:
		p.line(6, 6, 18, 18)
		p.line(18, 6, 6, 18)
	case icInfo:
		p.circle(12, 12, 9)
		p.line(12, 11, 12, 17)
		p.disc(12, 7.5, 1.3)
	case icWarning:
		p.poly(true, 12, 3, 22, 20, 2, 20)
		p.line(12, 9, 12, 14)
		p.disc(12, 17, 1.3)
	case icCheck:
		p.poly(false, 5, 12.5, 10, 17.5, 19, 7)
	case icMenu:
		p.line(5, 7, 19, 7)
		p.line(5, 12, 19, 12)
		p.line(5, 17, 19, 17)
	case icUnionGroupType:
		p.fillRect(3, 9, 10, 10, 1)
		p.fillRect(9, 4, 11, 10, 1)
	case icBundleGroupType:
		p.w *= 0.8
		p.dashed(2, 2, 22, 2)
		p.dashed(22, 2, 22, 22)
		p.dashed(22, 22, 2, 22)
		p.dashed(2, 22, 2, 2)
		p.fillRect(5, 6, 7, 7, 0.5)
		p.rect(11, 11, 8, 8, 0.5)
	case icIntersectGroupType:
		// two overlapping outlines with only the common part filled
		p.w *= 0.8
		p.rect(3, 8, 12, 12, 1)
		p.rect(9, 4, 12, 12, 1)
		p.fillRect(9, 8, 6, 8, 0.5)
	case icHullGroupType:
		// two round shapes and the skin stretched between them
		p.w *= 0.8
		p.disc(8, 15, 5)
		p.disc(17, 7, 3)
		p.line(11.9, 18.1, 19.3, 8.9)
		p.line(5.3, 10.8, 15.4, 4.5)
	case icPattern:
		// one shape, and its copies in rows and columns
		a.fillRect(3, 3, 7, 7, 1)
		p.rect(14, 3, 7, 7, 1)
		p.rect(3, 14, 7, 7, 1)
		p.rect(14, 14, 7, 7, 1)
	case icStretch:
		// a block parted at a dashed plane, arrows pushing its ends apart
		p.rect(3, 7, 6, 10, 1)
		a.fillRect(15, 7, 6, 10, 1)
		p.dashed(12, 3, 12, 21)
		p.line(1, 12, 3, 12)
		p.poly(false, 2.5, 10, 0.5, 12, 2.5, 14)
		p.line(21, 12, 23, 12)
		p.poly(false, 21.5, 10, 23.5, 12, 21.5, 14)
	case icSkSelect:
		// a pointer arrow
		a.fill(6, 3, 6, 19, 10, 15, 13, 21, 15.5, 20, 12.5, 14, 18, 14)
	case icSkLine:
		// straight lines between square points
		p.poly(false, 4, 19, 10, 6, 20, 13)
		for _, q := range [][2]float64{{4, 19}, {10, 6}, {20, 13}} {
			a.fillRect(q[0]-2, q[1]-2, 4, 4, 0.5)
		}
	case icSkBezier:
		// a curve, and the handle line that bends it
		var pts []float64
		for i := 0; i <= 16; i++ {
			t := float64(i) / 16
			u := 1 - t
			x := u*u*u*3 + 3*u*u*t*6 + 3*u*t*t*18 + t*t*t*21
			y := u*u*u*20 + 3*u*u*t*4 + 3*u*t*t*4 + t*t*t*20
			pts = append(pts, x, y)
		}
		p.poly(false, pts...)
		a.line(6, 4, 14, 4)
		a.disc(6, 4, 1.8)
		a.disc(14, 4, 1.8)
		a.fillRect(10-1.8, 4-1.8, 3.6, 3.6, 0.5)
	case icSkSmooth:
		// a wave through round points
		var pts []float64
		for i := 0; i <= 20; i++ {
			x := 3 + float64(i)*0.9
			pts = append(pts, x, 12-6*math.Sin((x-3)/18*2*math.Pi))
		}
		p.poly(false, pts...)
		for _, x := range []float64{3, 7.5, 16.5, 21} {
			a.disc(x, 12-6*math.Sin((x-3)/18*2*math.Pi), 2)
		}
	case icSkRefine:
		// a line with a point being added on it
		p.line(3, 17, 21, 17)
		a.fillRect(10, 15, 4, 4, 0.5)
		a.line(12, 4, 12, 11)
		a.line(8.5, 7.5, 15.5, 7.5)
	case icSkPen:
		// a pen nib drawing a line
		a.fill(12, 3, 18, 11, 14, 19, 10, 19, 6, 11)
		p.line(12, 9, 12, 14)
		p.line(3, 21, 21, 21)
	case icSkEdit:
		// a pointer on a curve
		p.arc(12, 13, 8, 150, 330)
		a.fill(11, 8, 11, 21, 14, 18, 16, 22, 18, 21, 16, 17, 20, 17)
		p.disc(5, 17, 1.8)
	case icSkDims:
		// a dimension: a line with arrowheads between two ticks
		p.line(3, 6, 3, 18)
		p.line(21, 6, 21, 18)
		a.line(4, 12, 20, 12)
		a.poly(false, 7, 9, 4, 12, 7, 15)
		a.poly(false, 17, 9, 20, 12, 17, 15)
	case icSkClose:
		// a path round three sides, its last side the one that closes it
		p.poly(false, 5, 19, 4, 6, 19, 4, 20, 18)
		a.dashed(20, 18, 5, 19)
		p.disc(5, 19, 1.8)
		p.disc(20, 18, 1.8)
	case icSkStroke:
		// a broad line: two edges and its hatching
		p.line(4, 15, 15, 4)
		p.line(9, 20, 20, 9)
		for _, k := range []float64{0, 3.5, 7, 10.5} {
			a.line(6.5+k, 16.5-k, 8.5+k, 18.5-k)
		}
	case icSkHole:
		// an area with a dashed outline cut out of it
		p.rect(3, 3, 18, 18, 2)
		a.dashed(8, 8, 16, 8)
		a.dashed(16, 8, 16, 16)
		a.dashed(16, 16, 8, 16)
		a.dashed(8, 16, 8, 8)
	case icSkMirrorTurn:
		// a shape and its mirror image about a dashed line, a turn arrow
		// over them
		p.dashed(12, 9, 12, 22)
		a.fill(10, 11, 10, 21, 3, 21)
		p.poly(true, 14, 11, 14, 21, 21, 21)
		p.arc(12, 9, 6, 200, 335)
		p.fill(19.2, 5.1, 18.7, 9.2, 15.2, 6.9)
	case icSkCorner:
		// two straight sides meeting at a sharp point
		p.poly(false, 3, 19, 12, 5, 21, 19)
		a.fillRect(10, 3, 4, 4, 0.5)
	case icSkSmoothPt:
		// a curve through a point, its handles in one line
		p.arc(12, 20, 10, 200, 340)
		a.line(4, 10, 20, 10)
		a.disc(4, 10, 1.6)
		a.disc(20, 10, 1.6)
		a.disc(12, 10, 2.2)
	case icSkImage:
		// a picture: its frame, a hill and the sun
		p.rect(3, 5, 18, 14, 1.5)
		a.poly(false, 4, 17, 9, 11, 13, 15, 16, 12, 20, 16)
		a.disc(16, 9, 1.8)
	case icSkClear:
		// an eraser on a line
		a.fill(10, 4, 20, 14, 13, 21, 3, 11)
		p.line(8, 9, 15, 16)
		p.line(13, 21, 21, 21)
	case icSkSplit:
		// a point whose two handles go their own ways
		p.poly(false, 3, 20, 12, 12, 21, 20)
		a.line(12, 12, 6, 4)
		a.line(12, 12, 20, 6)
		a.disc(6, 4, 1.6)
		a.disc(20, 6, 1.6)
		a.disc(12, 12, 2.2)
	}
}

// Logo colours: three shades of one blue for the cube's faces, and a deep
// navy for the bored hole.
var (
	logoTop   = render.RGB(0x6c, 0xc4, 0xf0)
	logoLeft  = render.RGB(0x2e, 0x86, 0xc8)
	logoRight = render.RGB(0x1b, 0x55, 0x8c)
	logoHole  = render.RGB(0x0e, 0x2a, 0x47)
	logoBore  = render.RGB(0x16, 0x3d, 0x63)
)

// drawLogo draws ScottiCAD's mark into r: an isometric cube, shaded in
// three blues, with a round hole bored into its top face: a solid with a
// hole cut from it.
func drawLogo(b *render.Batch, r render.Rect) {
	p := newPen(b, r, logoTop)
	// hexagon outline of the cube (24-unit design grid)
	top, ur, lr := [2]float64{12, 1}, [2]float64{21.5, 6.5}, [2]float64{21.5, 17.5}
	bot, ll, ul := [2]float64{12, 23}, [2]float64{2.5, 17.5}, [2]float64{2.5, 6.5}
	mid := [2]float64{12, 12}
	face := func(c render.Color, pts ...[2]float64) {
		p.c = c
		var f []float64
		for _, q := range pts {
			f = append(f, q[0], q[1])
		}
		p.fill(f...)
	}
	face(logoTop, top, ur, mid, ul)
	face(logoLeft, ul, mid, bot, ll)
	face(logoRight, mid, ur, lr, bot)

	// the hole: an isometric ellipse on the top face, with the far wall of
	// the bore visible inside it
	ellipse := func(cx, cy, rx, ry float64, c render.Color) {
		var pts [][2]float64
		for i := 0; i < 40; i++ {
			a := 2 * math.Pi * float64(i) / 40
			pts = append(pts, [2]float64{p.x(cx + rx*math.Cos(a)), p.y(cy + ry*math.Sin(a))})
		}
		b.Poly(pts, c)
	}
	cx, cy, rx, ry := 12.0, 6.5, 4.6, 2.65
	ellipse(cx, cy, rx, ry, logoBore)
	ellipse(cx, cy+0.9, rx*0.92, ry*0.72, logoHole)

	// crisp light edges where the faces meet
	p.c = render.Color{1, 1, 1, 0.55}
	p.w = math.Max(0.8, r.W/40)
	p.line(mid[0], mid[1], ul[0], ul[1])
	p.line(mid[0], mid[1], ur[0], ur[1])
	p.line(mid[0], mid[1], bot[0], bot[1])
}
