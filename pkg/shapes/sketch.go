package shapes

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Sketch shapes are drawn in the sketch editor: paths of points joined by
// straight lines and curves. An extruded sketch stands the drawing straight
// up; a revolved sketch spins the part of it left of its centre line (x < 0)
// round that line, which becomes the shape's vertical axis.
//
// A closed path is a filled area. A path can instead be built as a line of
// a given width (a stroke); open paths always are. Areas and lines join,
// one inside another included (SVG import, unlike this, makes holes of
// outlines inside outlines).

// Point kinds: how a point's curve handles behave.
const (
	// PointCorner: the handles are independent (none: a sharp corner).
	PointCorner = 0
	// PointSmooth: the handles stay in line, so the curve has no kink.
	PointSmooth = 1
	// PointAuto: the handles are worked out from the neighbouring points
	// (the Smooth Curve tool), so the curve flows through the points: in
	// line with them, each a quarter of the distance between them.
	PointAuto = 2
)

// Stroke joins and ends.
const (
	JoinSharp = 0
	JoinRound = 1
	JoinBevel = 2

	CapFlat   = 0
	CapRound  = 1
	CapSquare = 2
)

// Where a closed path's line lies: centred on the path, or all inside or
// all outside it (an outline drawn round a shape keeps the shape's size).
const (
	AlignCenter  = 0
	AlignInside  = 1
	AlignOutside = 2
)

// DefaultStroke is the width of a line drawn without one (mm).
const DefaultStroke = 2.0

// SketchPoint is a point of a path. In and Out are its curve handles,
// relative to P: the curve arrives along In (from the previous point) and
// leaves along Out. Both zero is a sharp corner joined by straight lines.
type SketchPoint struct {
	P    [2]float64 `json:"p"`
	In   [2]float64 `json:"in,omitempty"`
	Out  [2]float64 `json:"out,omitempty"`
	Kind int        `json:"kind,omitempty"`
}

// SketchPath is one path: an outline (closed) or a line (open).
type SketchPath struct {
	Points []SketchPoint `json:"points"`
	Closed bool          `json:"closed,omitempty"`
	// Stroke, above 0, builds the path as a line this wide (mm) instead of
	// the area it encloses. An open path is always a line (DefaultStroke
	// wide when Stroke is 0).
	Stroke float64 `json:"stroke,omitempty"`
	Join   int     `json:"join,omitempty"`  // a line's corners: JoinSharp, JoinRound, JoinBevel
	Cap    int     `json:"cap,omitempty"`   // an open line's ends: CapFlat, CapRound, CapSquare
	Align  int     `json:"align,omitempty"` // a closed line's place: AlignCenter, AlignInside, AlignOutside
	// Hole cuts what the path covers (its area, or its line) out of the
	// rest of the drawing, instead of adding it.
	Hole bool `json:"hole,omitempty"`
}

// IsLine is whether the path is built as a line rather than an area.
func (p SketchPath) IsLine() bool { return !p.Closed || p.Stroke > 0 }

// Width is the width the path is built with as a line.
func (p SketchPath) Width() float64 {
	if p.Stroke > 0 {
		return p.Stroke
	}
	return DefaultStroke
}

// Sketch is a sketch shape's drawing, in millimetres in the shape's own
// frame. Like an outline's sections it is replaced whole, never changed in
// place, so copies of a node may share it.
type Sketch struct {
	Paths []SketchPath `json:"paths"`
	// Image is a picture under the drawing, to trace (nil: none). It is
	// never part of the shape: SketchKey leaves it out.
	Image *SketchImage `json:"image,omitempty"`
}

// SketchImage is a picture placed under a drawing in the sketch
// workspace, in the drawing's millimeters.
type SketchImage struct {
	// Data is the picture, a JPEG, base64 (as loaded: turned upright and
	// no more than MaxSketchImagePx on a side).
	Data string `json:"data"`
	// PW and PH are its size in pixels.
	PW int `json:"pw"`
	PH int `json:"ph"`
	// X and Y are its center, Width how wide it is across (its height
	// follows), Angle how far it is turned, in degrees anticlockwise.
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Width float64 `json:"width"`
	Angle float64 `json:"angle"`
	// Opacity is how strongly it shows, 0 to 1.
	Opacity float64 `json:"opacity"`
}

// MaxSketchImagePx is the most pixels a sketch's picture has on a side:
// plenty to trace by, and a design file stays a reasonable size.
const MaxSketchImagePx = 2048

// Height is how tall the picture is, in millimeters.
func (im *SketchImage) Height() float64 {
	if im.PW <= 0 {
		return 0
	}
	return im.Width * float64(im.PH) / float64(im.PW)
}

// Corners are the picture's corners in the drawing's millimeters: top
// left, top right, bottom right, bottom left (as the picture is seen).
func (im *SketchImage) Corners() [4][2]float64 {
	a := im.Angle * math.Pi / 180
	cs, sn := math.Cos(a), math.Sin(a)
	hw, hh := im.Width/2, im.Height()/2
	var out [4][2]float64
	for i, c := range [4][2]float64{{-hw, hh}, {hw, hh}, {hw, -hh}, {-hw, -hh}} {
		out[i] = [2]float64{im.X + c[0]*cs - c[1]*sn, im.Y + c[0]*sn + c[1]*cs}
	}
	return out
}

// CopySketch returns a copy of sk that shares nothing with it (but the
// picture's data, a string, which never changes).
func CopySketch(sk *Sketch) *Sketch {
	if sk == nil {
		return nil
	}
	c := &Sketch{Paths: make([]SketchPath, len(sk.Paths))}
	for i, p := range sk.Paths {
		p.Points = append([]SketchPoint(nil), p.Points...)
		c.Paths[i] = p
	}
	if sk.Image != nil {
		im := *sk.Image
		c.Image = &im
	}
	return c
}

// SameImage reports whether two sketches have the same picture, placed the
// same way.
func SameImage(a, b *Sketch) bool {
	var ia, ib *SketchImage
	if a != nil {
		ia = a.Image
	}
	if b != nil {
		ib = b.Image
	}
	if ia == nil || ib == nil {
		return ia == ib
	}
	return *ia == *ib
}

// IsSketch reports whether a kind is drawn in the sketch editor.
func IsSketch(kind string) bool { return kind == "sketch" || kind == "revolve" || kind == "coil" }

// Spins reports whether a sketch kind spins what is left of its centre
// line (a revolve, or a coil that rises as it turns) rather than standing
// the whole drawing up.
func Spins(kind string) bool { return kind == "revolve" || kind == "coil" }

// Limits on a sketch.
const (
	maxSketchPoints = 20000
	maxSketchCoord  = 1e5 // mm
	maxStroke       = 1000
)

// CheckSketch reports what is wrong with a sketch, if anything, in words fit
// to show a script's author. A sketch with nothing buildable in it is fine
// here (the editor keeps a half-drawn one); Buildable says whether it makes
// a shape.
func CheckSketch(sk *Sketch) error {
	if sk == nil {
		return errors.New("no sketch")
	}
	total := 0
	for i, p := range sk.Paths {
		if len(p.Points) == 0 {
			return fmt.Errorf("path %d has no points", i+1)
		}
		total += len(p.Points)
		if total > maxSketchPoints {
			return fmt.Errorf("too many points (more than %d)", maxSketchPoints)
		}
		for _, q := range p.Points {
			for _, v := range [][2]float64{q.P, q.In, q.Out} {
				if !finite(v[0]) || !finite(v[1]) || math.Abs(v[0]) > maxSketchCoord || math.Abs(v[1]) > maxSketchCoord {
					return fmt.Errorf("path %d: point (%v, %v) is out of range", i+1, v[0], v[1])
				}
			}
			if q.Kind < PointCorner || q.Kind > PointAuto {
				return fmt.Errorf("path %d: unknown point kind %d", i+1, q.Kind)
			}
		}
		if !finite(p.Stroke) || p.Stroke < 0 || p.Stroke > maxStroke {
			return fmt.Errorf("path %d: line width %v is out of range (0 to %v mm)", i+1, p.Stroke, maxStroke)
		}
		if p.Join < JoinSharp || p.Join > JoinBevel {
			return fmt.Errorf("path %d: unknown corner style %d", i+1, p.Join)
		}
		if p.Cap < CapFlat || p.Cap > CapSquare {
			return fmt.Errorf("path %d: unknown end style %d", i+1, p.Cap)
		}
		if p.Align < AlignCenter || p.Align > AlignOutside {
			return fmt.Errorf("path %d: unknown line alignment %d", i+1, p.Align)
		}
	}
	if im := sk.Image; im != nil {
		switch {
		case im.Data == "" || len(im.Data) > 16<<20:
			return errors.New("the sketch's picture has no data, or too much")
		case im.PW <= 0 || im.PH <= 0 || im.PW > MaxSketchImagePx || im.PH > MaxSketchImagePx:
			return fmt.Errorf("the sketch's picture is %d × %d pixels (1 to %d a side)", im.PW, im.PH, MaxSketchImagePx)
		case !finite(im.X) || !finite(im.Y) || math.Abs(im.X) > maxSketchCoord || math.Abs(im.Y) > maxSketchCoord:
			return errors.New("the sketch's picture is out of range")
		case !finite(im.Width) || im.Width <= 0 || im.Width > 2*maxSketchCoord:
			return fmt.Errorf("the sketch's picture's width %v is out of range", im.Width)
		case !finite(im.Angle) || !finite(im.Opacity) || im.Opacity < 0 || im.Opacity > 1:
			return errors.New("the sketch's picture's angle or opacity is out of range")
		}
	}
	return nil
}

// SketchKey is a short hash of a sketch, for cache keys.
func SketchKey(sk *Sketch) string {
	if sk == nil {
		return ""
	}
	h := sha256.New()
	var b [8]byte
	put := func(f float64) {
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(f))
		h.Write(b[:])
	}
	for _, p := range sk.Paths {
		c := 0.0
		if p.Closed {
			c = 1
		}
		if p.Hole {
			c += 2 // (added, so a drawing without holes has the key it would without this)
		}
		put(float64(len(p.Points)))
		put(c)
		put(p.Stroke)
		put(float64(p.Join))
		put(float64(p.Cap))
		put(float64(p.Align))
		for _, q := range p.Points {
			put(q.P[0])
			put(q.P[1])
			put(q.In[0])
			put(q.In[1])
			put(q.Out[0])
			put(q.Out[1])
			put(float64(q.Kind))
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// Handles returns point i's handles as they are used (worked out for an
// automatic point), relative to the point.
func (p SketchPath) Handles(i int) (in, out Pt) {
	q := p.Points[i]
	if q.Kind != PointAuto {
		return Pt{q.In[0], q.In[1]}, Pt{q.Out[0], q.Out[1]}
	}
	n := len(p.Points)
	var prev, next *Pt
	if i > 0 || p.Closed {
		v := p.Points[(i+n-1)%n].P
		prev = &Pt{v[0], v[1]}
	}
	if i < n-1 || p.Closed {
		v := p.Points[(i+1)%n].P
		next = &Pt{v[0], v[1]}
	}
	if prev != nil && next != nil {
		// the handles in line with the neighbours, each a quarter of the
		// way between them (four points on a square make a near-circle)
		d := Pt{(next.X - prev.X) / 4, (next.Y - prev.Y) / 4}
		return Pt{-d.X, -d.Y}, d
	}
	// an open line's end: straight on to its neighbour
	return Pt{}, Pt{}
}

// curveTol is how far (mm) a flattened curve may stray from the true one.
const curveTol = 0.01

// segment returns the points of the stretch from point i to the next,
// without its start: the end alone for a straight line.
func (p SketchPath) segment(i int) []Pt { return p.segmentN(i, 0) }

// segmentN is segment with a curve cut into steps straight pieces (0: as
// many as keep it within curveTol of the true curve).
func (p SketchPath) segmentN(i, steps int) []Pt {
	n := len(p.Points)
	j := (i + 1) % n
	a, b := p.Points[i].P, p.Points[j].P
	_, out := p.Handles(i)
	in, _ := p.Handles(j)
	c0, c3 := Pt{a[0], a[1]}, Pt{b[0], b[1]}
	if out == (Pt{}) && in == (Pt{}) {
		return []Pt{c3}
	}
	c1 := Pt{c0.X + out.X, c0.Y + out.Y}
	c2 := Pt{c3.X + in.X, c3.Y + in.Y}
	// enough steps that the chords stay within curveTol of the curve: a
	// cubic's distance from its chords falls with the square of the steps
	if steps <= 0 {
		dd := math.Max(math.Hypot(c0.X-2*c1.X+c2.X, c0.Y-2*c1.Y+c2.Y), math.Hypot(c1.X-2*c2.X+c3.X, c1.Y-2*c2.Y+c3.Y))
		steps = int(math.Ceil(math.Sqrt(0.75 * dd / curveTol)))
	}
	steps = max(1, min(steps, 100))
	out2 := make([]Pt, 0, steps)
	for s := 1; s <= steps; s++ {
		t := float64(s) / float64(steps)
		u := 1 - t
		w0, w1, w2, w3 := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
		out2 = append(out2, Pt{w0*c0.X + w1*c1.X + w2*c2.X + w3*c3.X, w0*c0.Y + w1*c1.Y + w2*c2.Y + w3*c3.Y})
	}
	out2[len(out2)-1] = c3 // exactly
	return out2
}

// Segment returns the stretch of the path from point i to the next as a
// polyline, its start included: its points are at evenly spaced values of
// the curve's parameter, from 0 to 1.
func (p SketchPath) Segment(i int) []Pt {
	q := p.Points[i].P
	return append([]Pt{{q[0], q[1]}}, p.segment(i)...)
}

// Bezier returns the control points of the curve from point i to the next
// (a straight line has its ends as its middle two).
func (p SketchPath) Bezier(i int) [4]Pt {
	n := len(p.Points)
	j := (i + 1) % n
	a, b := p.Points[i].P, p.Points[j].P
	_, out := p.Handles(i)
	in, _ := p.Handles(j)
	c0, c3 := Pt{a[0], a[1]}, Pt{b[0], b[1]}
	return [4]Pt{c0, ptAdd(c0, out), ptAdd(c3, in), c3}
}

// Flatten returns the path as a polyline: straight lines through its points,
// curves as many short lines. A closed path's last point is not repeated.
// Points that repeat the one before are dropped.
func (p SketchPath) Flatten() []Pt { return p.flattenN(0) }

// flattenN is Flatten with each curve cut into steps pieces (0: fine
// enough to follow it within curveTol).
func (p SketchPath) flattenN(steps int) []Pt {
	n := len(p.Points)
	if n == 0 {
		return nil
	}
	out := []Pt{{p.Points[0].P[0], p.Points[0].P[1]}}
	last := n - 1
	if p.Closed {
		last = n
	}
	for i := 0; i < last; i++ {
		out = append(out, p.segmentN(i, steps)...)
	}
	if p.Closed && len(out) > 1 {
		out = out[:len(out)-1] // back at the start
	}
	var d []Pt
	for _, q := range out {
		if len(d) > 0 && same(d[len(d)-1], q) {
			continue
		}
		d = append(d, q)
	}
	for p.Closed && len(d) > 1 && same(d[0], d[len(d)-1]) {
		d = d[:len(d)-1]
	}
	return d
}

// --- lines -------------------------------------------------------------------

// arcSteps is the number of sides a full circle of a round corner or end
// has.
const arcSteps = 32

func ptSub(a, b Pt) Pt           { return Pt{a.X - b.X, a.Y - b.Y} }
func ptAdd(a, b Pt) Pt           { return Pt{a.X + b.X, a.Y + b.Y} }
func ptScale(a Pt, f float64) Pt { return Pt{a.X * f, a.Y * f} }
func ptDot(a, b Pt) float64      { return a.X*b.X + a.Y*b.Y }
func ptPerp(a Pt) Pt             { return Pt{-a.Y, a.X} } // a turned left
func ptUnit(a Pt) Pt             { l := math.Hypot(a.X, a.Y); return Pt{a.X / l, a.Y / l} }
func ptCross(a, b Pt) float64    { return a.X*b.Y - a.Y*b.X }
func ptAngle(a Pt) float64       { return math.Atan2(a.Y, a.X) }
func ptPolar(r, a float64) Pt    { return Pt{r * math.Cos(a), r * math.Sin(a)} }
func lineCircle(c Pt, r float64) []Pt {
	out := make([]Pt, arcSteps)
	for i := range out {
		out[i] = ptAdd(c, ptPolar(r, 2*math.Pi*float64(i)/arcSteps))
	}
	return out
}

// lineArc returns the points strictly between from and to on the circle of
// radius r round c, turning clockwise (cw) or anticlockwise by the smaller
// way round in that direction.
func lineArc(c Pt, r float64, from, to Pt, cw bool) []Pt {
	a0, a1 := ptAngle(from), ptAngle(to)
	d := a1 - a0
	if cw {
		for d > 0 {
			d -= 2 * math.Pi
		}
	} else {
		for d < 0 {
			d += 2 * math.Pi
		}
	}
	n := int(math.Ceil(math.Abs(d) / (2 * math.Pi) * arcSteps))
	var out []Pt
	for i := 1; i < n; i++ {
		out = append(out, ptAdd(c, ptPolar(r, a0+d*float64(i)/float64(n))))
	}
	return out
}

// miterLimit is how far, in half-widths, a sharp corner may reach before it
// is cut off (bevelled), as in SVG.
const miterLimit = 4

// leftSide returns the points of the line's left edge (h from the centre
// line pts), with corners joined as join says. closed joins the end to the
// start. (Where the line doubles straight back, the inner corner is the
// two edges' ends.)
func leftSide(pts []Pt, closed bool, h float64, join int) []Pt {
	n := len(pts)
	segs := n - 1
	if closed {
		segs = n
	}
	dir := make([]Pt, segs)
	nrm := make([]Pt, segs)
	for i := 0; i < segs; i++ {
		dir[i] = ptUnit(ptSub(pts[(i+1)%n], pts[i]))
		nrm[i] = ptPerp(dir[i])
	}
	var out []Pt
	corner := func(k, s0, s1 int) {
		p := pts[k]
		n0, n1 := nrm[s0], nrm[s1]
		c := ptCross(dir[s0], dir[s1])
		d := ptDot(n0, n1)
		if math.Abs(c) < 1e-12 && d > 0 {
			out = append(out, ptAdd(p, ptScale(n0, h)))
			return
		}
		inner := c > 0 // turning left: the left edge is inside the turn
		miterOK := 1+d > 1e-9
		if inner {
			if miterOK {
				out = append(out, ptAdd(p, ptScale(ptAdd(n0, n1), h/(1+d))))
			} else {
				out = append(out, ptAdd(p, ptScale(n0, h)), ptAdd(p, ptScale(n1, h)))
			}
			return
		}
		switch {
		case join == JoinRound:
			out = append(out, ptAdd(p, ptScale(n0, h)))
			out = append(out, lineArc(p, h, n0, n1, true)...)
			out = append(out, ptAdd(p, ptScale(n1, h)))
		case join == JoinSharp && miterOK && math.Sqrt(2/(1+d)) <= miterLimit:
			out = append(out, ptAdd(p, ptScale(ptAdd(n0, n1), h/(1+d))))
		case join == JoinBevel:
			a, b := bevelEnds(p, dir[s0], dir[s1], n0, n1, h)
			out = append(out, a, b)
		default:
			out = append(out, ptAdd(p, ptScale(n0, h)), ptAdd(p, ptScale(n1, h)))
		}
	}
	if closed {
		for k := 0; k < n; k++ {
			corner(k, (k+n-1)%n, k)
		}
		return out
	}
	out = append(out, ptAdd(pts[0], ptScale(nrm[0], h)))
	for k := 1; k < n-1; k++ {
		corner(k, k-1, k)
	}
	return append(out, ptAdd(pts[n-1], ptScale(nrm[segs-1], h)))
}

// bevelEnds returns the ends of a bevelled outer corner at p, between the
// edge arriving along d0 and the one leaving along d1, with n0 and n1 their
// normals on the outside. The cut is square to the corner's bisector, a
// full h from p (so a bevel trims less than joining the two edges' ends
// would: a square join, as polygon offsetting libraries make it).
func bevelEnds(p, d0, d1, n0, n1 Pt, h float64) (Pt, Pt) {
	bis := ptAdd(n0, n1)
	if math.Hypot(bis.X, bis.Y) < 1e-9 {
		bis = d0 // doubling straight back: the cut faces on along the line
	} else {
		bis = ptUnit(bis)
	}
	e0, e1 := ptAdd(p, ptScale(n0, h)), ptAdd(p, ptScale(n1, h))
	t0, t1 := 0.0, 0.0
	if c := ptDot(d0, bis); c > 1e-12 {
		t0 = h * (1 - ptDot(n0, bis)) / c
	}
	if c := -ptDot(d1, bis); c > 1e-12 {
		t1 = h * (1 - ptDot(n1, bis)) / c
	}
	return ptAdd(e0, ptScale(d0, t0)), ptSub(e1, ptScale(d1, t1))
}

func reversedPts(c []Pt) []Pt {
	out := make([]Pt, len(c))
	for i, p := range c {
		out[len(c)-1-i] = p
	}
	return out
}

// lineOutline returns the outline of an open line through pts: its left
// edge, the end, its right edge back, and the start.
func lineOutline(pts []Pt, h float64, join, capKind int) []Pt {
	left := leftSide(pts, false, h, join)
	right := leftSide(reversedPts(pts), false, h, join)
	n := len(pts)
	d0, d1 := ptUnit(ptSub(pts[1], pts[0])), ptUnit(ptSub(pts[n-1], pts[n-2]))
	if capKind == CapSquare {
		left[0] = ptSub(left[0], ptScale(d0, h))
		right[len(right)-1] = ptSub(right[len(right)-1], ptScale(d0, h))
		left[len(left)-1] = ptAdd(left[len(left)-1], ptScale(d1, h))
		right[0] = ptAdd(right[0], ptScale(d1, h))
	}
	out := append([]Pt(nil), left...)
	if capKind == CapRound {
		out = append(out, lineArc(pts[n-1], h, ptPerp(d1), ptScale(ptPerp(d1), -1), true)...)
	}
	out = append(out, right...)
	if capKind == CapRound {
		out = append(out, lineArc(pts[0], h, ptScale(ptPerp(d0), -1), ptPerp(d0), true)...)
	}
	return out
}

// simpleRing reports whether c is a usable outline: no crossings, and
// some area.
func simpleRing(c []Pt) bool {
	c = dedupe(c)
	if len(c) < 3 || math.Abs(SignedArea(c)) < 1e-12 {
		return false
	}
	_, _, crosses := selfCrossing(c)
	return !crosses
}

// clearOf reports whether every point of c is inside the closed center
// line pts and at least h from it, as the inside edge of a line h wide on
// each side is. A line wider than its shape (a narrow slot, a small loop,
// drawn thick) has an inside edge turned inside out: still a simple
// outline, but it would cut a hole in what should be solid. Turned a
// little, it runs closer to the center line than h; turned a lot, it lies
// outside it. (Very long outlines are trusted: the check is every point
// against every segment.)
func clearOf(c, pts []Pt, h float64) bool {
	if len(c)*len(pts) > 20_000_000 {
		return true
	}
	lim := h * (1 - 1e-6)
	lim *= lim
	n := len(pts)
	for _, p := range c {
		if !PointInPolygon(p, pts) {
			return false
		}
		for i := range n {
			a, b := pts[i], pts[(i+1)%n]
			ab, ap := ptSub(b, a), ptSub(p, a)
			t := 0.0
			if l := ptDot(ab, ab); l > 0 {
				t = math.Max(0, math.Min(1, ptDot(ap, ab)/l))
			}
			dx, dy := ap.X-t*ab.X, ap.Y-t*ab.Y
			if dx*dx+dy*dy < lim {
				return false
			}
		}
	}
	return true
}

// insetsFit reports whether a line h wide on each side along pts has its
// inside edge on every side: at each turn the inside edges meet h/tan(α/2)
// from the corner along both sides (α the angle between them), and a side
// must be longer than the cut-backs at its two ends. A line doubling back
// sharply on a short side fails it: built from its outline alone,
// (1,0)->(0,0)->(10,1) at width 1 would be 6 mm² where its second side alone
// is 10.
func insetsFit(pts []Pt, closed bool, h float64) bool {
	n := len(pts)
	inset := make([]float64, n) // the cut-back at each corner
	for i := range pts {
		if !closed && (i == 0 || i == n-1) {
			continue
		}
		v := pts[i]
		u, w := ptSub(pts[(i+n-1)%n], v), ptSub(pts[(i+1)%n], v)
		lu, lw := math.Hypot(u.X, u.Y), math.Hypot(w.X, w.Y)
		if lu == 0 || lw == 0 {
			continue
		}
		cos := math.Max(-1, math.Min(1, ptDot(u, w)/(lu*lw)))
		half := math.Acos(cos) / 2 // half the angle between the sides
		if t := math.Tan(half); t < 1e-12 {
			inset[i] = math.Inf(1)
		} else {
			inset[i] = h / t
		}
	}
	segs := n - 1
	if closed {
		segs = n
	}
	for i := 0; i < segs; i++ {
		a, b := pts[i], pts[(i+1)%n]
		if inset[i]+inset[(i+1)%n] > math.Hypot(b.X-a.X, b.Y-a.Y)*(1+1e-9)+1e-12 {
			return false
		}
	}
	return true
}

// linePieces returns a path built as a line: pieces whose union is the line.
// A line whose outline is simple is one piece (a closed line, a ring with a
// hole); one that crosses itself, or whose edge folds inside a tight bend,
// is runs of segments whose outlines are simple (as long as they can be),
// the corners between the runs, and the ends.
func linePieces(p SketchPath, steps int) []Region {
	pts := p.flattenN(steps)
	h := p.Width() / 2
	closed := p.Closed && len(pts) >= 3
	if len(pts) < 2 || h <= 0 {
		return nil
	}
	if !insetsFit(pts, closed, h) {
		// (a sharp turn whose inside edge runs past a side's other end:
		// the outline is still simple, but leaves part of the line out)
	} else if closed {
		l, r := leftSide(pts, true, h, p.Join), leftSide(reversedPts(pts), true, h, p.Join)
		if simpleRing(l) && simpleRing(r) {
			in, out := l, r
			if math.Abs(SignedArea(l)) > math.Abs(SignedArea(r)) {
				in, out = r, l
			}
			if within(in, out) && !outlinesMeet(in, out) && clearOf(in, pts, h) {
				return []Region{{Outer: out, Holes: [][]Pt{in}}}
			}
		}
	} else if o := lineOutline(pts, h, p.Join, p.Cap); simpleRing(o) {
		return []Region{{Outer: o}}
	}
	join, capKind := p.Join, p.Cap
	n := len(pts)
	segs := n - 1
	if closed {
		segs = n
	}
	at := func(i int) Pt { return pts[i%n] }
	run := func(a, b int) []Pt { // the centre line from segment a to segment b-1
		out := make([]Pt, 0, b-a+1)
		for i := a; i <= b; i++ {
			out = append(out, at(i))
		}
		return out
	}
	okRun := func(a, b int) bool {
		r := run(a, b)
		return insetsFit(r, false, h) && simpleRing(lineOutline(r, h, join, CapFlat))
	}
	var pieces [][]Pt
	var corners []int
	for a := 0; a < segs; {
		// the longest run from a that is simple: doubling, then halving
		lo := a + 1
		top := segs + 1
		for step := 1; ; step *= 2 {
			b := min(segs, a+2*step)
			if b <= lo {
				break
			}
			if !okRun(a, b) {
				top = b
				break
			}
			lo = b
			if b == segs {
				break
			}
		}
		for top-lo > 1 {
			mid := (lo + top) / 2
			if mid > segs {
				break
			}
			if okRun(a, mid) {
				lo = mid
			} else {
				top = mid
			}
		}
		if o := lineOutline(run(a, lo), h, join, CapFlat); simpleRing(o) && insetsFit(run(a, lo), false, h) {
			pieces = append(pieces, o)
		} else {
			// a single segment's: a rectangle
			d := ptSub(at(lo), at(a))
			if l := math.Hypot(d.X, d.Y); l > 0 {
				q := ptScale(ptPerp(ptScale(d, 1/l)), h)
				pieces = append(pieces, []Pt{ptAdd(at(a), q), ptSub(at(a), q), ptSub(at(lo), q), ptAdd(at(lo), q)})
			}
		}
		if lo < segs || closed {
			corners = append(corners, lo%n)
		}
		a = lo
	}
	// the corners between runs
	for _, k := range corners {
		p := pts[k]
		var prev, next Pt
		if closed {
			prev, next = at(k+n-1), at(k+1)
		} else {
			if k <= 0 || k >= n-1 {
				continue
			}
			prev, next = pts[k-1], pts[k+1]
		}
		d0, d1 := ptUnit(ptSub(p, prev)), ptUnit(ptSub(next, p))
		c := ptCross(d0, d1)
		if math.Abs(c) < 1e-12 && ptDot(d0, d1) > 0 {
			continue
		}
		n0, n1 := ptPerp(d0), ptPerp(d1)
		if c > 0 {
			// turning left: the outside is on the right
			n0, n1 = ptScale(n0, -1), ptScale(n1, -1)
		}
		d := ptDot(n0, n1)
		gentle := d > math.Cos(15*math.Pi/180) // within a curve: a bevel is as good
		switch {
		case join == JoinRound && !gentle:
			pieces = append(pieces, lineCircle(p, h))
		case join == JoinSharp && 1+d > 1e-9 && math.Sqrt(2/(1+d)) <= miterLimit:
			pieces = append(pieces, []Pt{p, ptAdd(p, ptScale(n0, h)), ptAdd(p, ptScale(ptAdd(n0, n1), h/(1+d))), ptAdd(p, ptScale(n1, h))})
		case join == JoinBevel && !gentle:
			e0, e1 := bevelEnds(p, d0, d1, n0, n1, h)
			pieces = append(pieces, []Pt{p, ptAdd(p, ptScale(n0, h)), e0, e1, ptAdd(p, ptScale(n1, h))})
		default:
			pieces = append(pieces, []Pt{p, ptAdd(p, ptScale(n0, h)), ptAdd(p, ptScale(n1, h))})
		}
	}
	if !closed {
		for _, e := range [][2]Pt{{pts[0], pts[1]}, {pts[n-1], pts[n-2]}} {
			p, d := e[0], ptUnit(ptSub(e[0], e[1])) // pointing out of the line
			switch capKind {
			case CapRound:
				pieces = append(pieces, lineCircle(p, h))
			case CapSquare:
				q := ptPerp(d)
				pieces = append(pieces, []Pt{ptAdd(p, ptScale(q, h)), ptSub(p, ptScale(q, h)), ptAdd(ptSub(p, ptScale(q, h)), ptScale(d, h)), ptAdd(ptAdd(p, ptScale(q, h)), ptScale(d, h))})
			}
		}
	}
	var out []Region
	for _, c := range pieces {
		if simpleRing(c) {
			out = append(out, Region{Outer: c})
		}
	}
	return out
}

// --- the drawing's outlines --------------------------------------------------

// area is a set of edges and a test for what is inside: the outline of what
// is inside runs along the edges.
type area struct {
	edges  [][2]Pt
	inside func(Pt) bool
}

type boxedPoly struct {
	c []Pt
	b bounds2
	x *ringIndex // for a long outline
}

func boxed(c []Pt) boxedPoly {
	p := boxedPoly{c: c, b: boundsOf([][]Pt{c})}
	if len(c) >= ringIndexMin {
		p.x = newRingIndex(c)
	}
	return p
}

func (p boxedPoly) has(q Pt) bool {
	if q.X < p.b.x0 || q.X > p.b.x1 || q.Y < p.b.y0 || q.Y > p.b.y1 {
		return false
	}
	if p.x != nil {
		return p.x.contains(q)
	}
	return PointInPolygon(q, p.c)
}

// winding is windingNumber(q, p.c).
func (p boxedPoly) winding(q Pt) int {
	if q.X < p.b.x0 || q.X > p.b.x1 || q.Y < p.b.y0 || q.Y > p.b.y1 {
		return 0
	}
	if p.x != nil {
		return p.x.winding(q)
	}
	return windingNumber(q, p.c)
}

func ringEdges(c []Pt) [][2]Pt {
	out := make([][2]Pt, 0, len(c))
	for i := range c {
		out = append(out, [2]Pt{c[i], c[(i+1)%len(c)]})
	}
	return out
}

// sketchArea is what a sketch covers: its areas and its lines, all joined,
// less what its hole paths cover. Separate paths join, one inside another
// included (a shape drawn inside a shape leaves just the outside one,
// unless it is a hole). A path that crosses itself fills by the nonzero
// rule: a five-pointed star drawn in one stroke is solid, its middle (gone
// round twice) included.
func sketchArea(sk *Sketch, steps int) area {
	var solid, cut sketchCover
	var areaEdges, lineEdges [][2]Pt
	for _, p := range sk.Paths {
		cv := &solid
		if p.Hole {
			cv = &cut
		}
		cv.add(&areaEdges, &lineEdges, p, steps)
	}
	for _, cv := range []*sketchCover{&solid, &cut} {
		if len(cv.lines) >= coverGridMin {
			cv.grid = newCoverGrid(cv.lines)
		}
	}
	return area{
		edges:  append(areaEdges, lineEdges...),
		inside: func(q Pt) bool { return solid.has(q) && !cut.has(q) },
	}
}

// sketchCover is what a set of paths covers: their areas and their lines.
type sketchCover struct {
	fills []boxedPoly // each closed area path's outline
	lines []coverLine
	grid  *coverGrid // the lines by where they are, once there are many
}

// coverGrid holds which line pieces' boxes reach each cell of a grid
// over them all, in order.
type coverGrid struct {
	x0, y0, cw, ch float64
	nx, ny         int
	cells          [][]int32
}

// coverGridMin is how many line pieces a cover has before it keeps them
// in a grid: below it, looking at every box is as quick.
const coverGridMin = 64

// near is the line pieces that may hold q, in order; all says to look at
// every one.
func (cv *sketchCover) near(q Pt) (cands []int32, all bool) {
	g := cv.grid
	if g == nil {
		return nil, true
	}
	cx, cy := math.Floor((q.X-g.x0)/g.cw), math.Floor((q.Y-g.y0)/g.ch)
	if !(cx >= 0 && cy >= 0 && cx < float64(g.nx) && cy < float64(g.ny)) {
		return nil, false // outside them all
	}
	return g.cells[int(cy)*g.nx+int(cx)], false
}

func newCoverGrid(ls []coverLine) *coverGrid {
	b := bounds2{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for i := range ls {
		o := ls[i].outer.b
		b = bounds2{math.Min(b.x0, o.x0), math.Min(b.y0, o.y0), math.Max(b.x1, o.x1), math.Max(b.y1, o.y1)}
	}
	n := max(1, min(128, int(math.Sqrt(float64(len(ls))))))
	g := &coverGrid{x0: b.x0, y0: b.y0, nx: n, ny: n, cells: make([][]int32, n*n)}
	// (a hair wider than the box, so its far edge falls in the last cell)
	g.cw = math.Max((b.x1-b.x0)*(1+1e-9), 1e-12) / float64(n)
	g.ch = math.Max((b.y1-b.y0)*(1+1e-9), 1e-12) / float64(n)
	cell := func(v, v0, w float64) int { return max(0, min(n-1, int(math.Floor((v-v0)/w)))) }
	for i := range ls {
		o := ls[i].outer.b
		for y := cell(o.y0, g.y0, g.ch); y <= cell(o.y1, g.y0, g.ch); y++ {
			for x := cell(o.x0, g.x0, g.cw); x <= cell(o.x1, g.x0, g.cw); x++ {
				g.cells[y*n+x] = append(g.cells[y*n+x], int32(i))
			}
		}
	}
	return g
}

// coverLine is a piece of a line, and for a line inside or outside its
// path, the path it keeps to and which side.
type coverLine struct {
	outer  boxedPoly
	holes  []boxedPoly
	clip   *boxedPoly
	inside bool
}

// add adds what p covers, an area's edges to areaEdges and a line's to
// lineEdges.
func (cv *sketchCover) add(areaEdges, lineEdges *[][2]Pt, p SketchPath, steps int) {
	if !p.IsLine() {
		if c := cleanContour(p.flattenN(steps)); len(c) >= 3 {
			cv.fills = append(cv.fills, boxed(c))
			*areaEdges = append(*areaEdges, ringEdges(c)...)
		}
		return
	}
	var pieces []Region
	var clip *boxedPoly
	inside := false
	if c := p.flattenN(steps); p.Closed && p.Align != AlignCenter && len(c) >= 3 {
		// twice as wide, centred, and cut back to its path
		wide := p
		wide.Stroke, wide.Align = 2*p.Width(), AlignCenter
		pieces = linePieces(wide, steps)
		b := boxed(c)
		clip, inside = &b, p.Align == AlignInside
		*lineEdges = append(*lineEdges, ringEdges(c)...)
	} else {
		pieces = linePieces(p, steps)
	}
	for _, l := range pieces {
		cl := coverLine{outer: boxed(l.Outer), clip: clip, inside: inside}
		*lineEdges = append(*lineEdges, ringEdges(l.Outer)...)
		for _, h := range l.Holes {
			cl.holes = append(cl.holes, boxed(h))
			*lineEdges = append(*lineEdges, ringEdges(h)...)
		}
		cv.lines = append(cv.lines, cl)
	}
}

// has is whether the cover holds q.
func (cv *sketchCover) has(q Pt) bool {
	// (by index throughout: ranging by value copies each entry)
	for i := range cv.fills {
		if cv.fills[i].winding(q) != 0 {
			return true
		}
	}
	// A line's pieces are together and share its clip, so the clip is
	// asked once for them all: a wide stroke on a fine curve is thousands
	// of pieces.
	var memo clipMemo
	cands, all := cv.near(q)
	if all {
		for i := range cv.lines {
			if cv.lines[i].has(q, &memo) {
				return true
			}
		}
		return false
	}
	for _, i := range cands {
		if cv.lines[i].has(q, &memo) {
			return true
		}
	}
	return false
}

// clipMemo is the last clip asked about a point, and its answer.
type clipMemo struct {
	clip *boxedPoly
	in   bool
}

// has is whether the piece holds q. Its clip is asked first: remembered in
// memo it costs nothing, and it rules out most pieces of a line inside or
// outside its path.
func (l *coverLine) has(q Pt, memo *clipMemo) bool {
	if l.clip != nil {
		if l.clip != memo.clip {
			memo.clip, memo.in = l.clip, l.clip.has(q)
		}
		if memo.in != l.inside {
			return false
		}
	}
	if !l.outer.has(q) {
		return false
	}
	for j := range l.holes {
		if l.holes[j].has(q) {
			return false
		}
	}
	return true
}

// outlinesOf returns the outlines of an area: its edges, split where they
// cross or touch, that have the inside on one side only, joined into loops
// (outer outlines anticlockwise, holes clockwise).
func outlinesOf(r area) [][]Pt {
	type seg struct {
		a, b  Pt
		bx    bounds2
		cuts  []Pt
		lenSq float64
	}
	var segs []*seg
	all := bounds2{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, e := range r.edges {
		if same(e[0], e[1]) {
			continue
		}
		s := &seg{a: e[0], b: e[1], bx: boundsOf([][]Pt{{e[0], e[1]}})}
		d := ptSub(e[1], e[0])
		s.lenSq = ptDot(d, d)
		segs = append(segs, s)
		all = bounds2{math.Min(all.x0, s.bx.x0), math.Min(all.y0, s.bx.y0), math.Max(all.x1, s.bx.x1), math.Max(all.y1, s.bx.y1)}
	}
	if len(segs) == 0 {
		return nil
	}
	size := math.Max(1, math.Max(all.x1-all.x0, all.y1-all.y0))
	eps := 1e-9 * size
	sort.Slice(segs, func(a, b int) bool { return segs[a].bx.x0 < segs[b].bx.x0 })
	// whether q lies on s (within eps)
	onSeg := func(s *seg, q Pt) bool {
		d := ptSub(s.b, s.a)
		t := ptDot(ptSub(q, s.a), d) / s.lenSq
		if t < 0 || t > 1 {
			return false
		}
		f := ptAdd(s.a, ptScale(d, t))
		return math.Hypot(f.X-q.X, f.Y-q.Y) <= eps
	}
	cut := func(s *seg, q Pt) {
		if !same(q, s.a) && !same(q, s.b) {
			s.cuts = append(s.cuts, q)
		}
	}
	for x, s := range segs {
		for _, u := range segs[x+1:] {
			if u.bx.x0 > s.bx.x1+eps {
				break
			}
			if u.bx.y0 > s.bx.y1+eps || s.bx.y0 > u.bx.y1+eps {
				continue
			}
			// an end of one on the other (touching, or lying along it)
			ends := false
			for _, e := range []struct {
				on *seg
				q  Pt
			}{{s, u.a}, {s, u.b}, {u, s.a}, {u, s.b}} {
				if onSeg(e.on, e.q) {
					cut(e.on, e.q)
					ends = true
				}
			}
			if ends {
				continue
			}
			p, d1 := s.a, ptSub(s.b, s.a)
			q, d2 := u.a, ptSub(u.b, u.a)
			den := ptCross(d1, d2)
			if den == 0 {
				continue
			}
			t := ptCross(ptSub(q, p), d2) / den
			v := ptCross(ptSub(q, p), d1) / den
			if t > 0 && t < 1 && v > 0 && v < 1 {
				x := ptAdd(p, ptScale(d1, t))
				s.cuts = append(s.cuts, x)
				u.cuts = append(u.cuts, x)
			}
		}
	}
	// the pieces with the inside on one side only, turned to have it on
	// their left; points closer than a few eps are one point
	key := func(p Pt) [2]int64 {
		return [2]int64{int64(math.Round(p.X / (8 * eps))), int64(math.Round(p.Y / (8 * eps)))}
	}
	type piece struct {
		a, b   Pt
		ka, kb [2]int64
	}
	var pieces []piece
	seen := map[[2][2]int64]bool{}
	delta := 1e-7 * size
	for _, s := range segs {
		ps := make([]Pt, 0, len(s.cuts)+2)
		ps = append(append(append(ps, s.a), s.cuts...), s.b)
		d := ptSub(s.b, s.a)
		sort.Slice(ps, func(i, j int) bool { return ptDot(ptSub(ps[i], s.a), d) < ptDot(ptSub(ps[j], s.a), d) })
		for k := 0; k+1 < len(ps); k++ {
			a, b := ps[k], ps[k+1]
			ka, kb := key(a), key(b)
			if ka == kb {
				continue
			}
			m := Pt{(a.X + b.X) / 2, (a.Y + b.Y) / 2}
			nrm := ptScale(ptPerp(ptUnit(ptSub(b, a))), delta)
			l, rgt := r.inside(ptAdd(m, nrm)), r.inside(ptSub(m, nrm))
			if l == rgt {
				// a sliver thinner than delta (two shapes overlapping by a
				// hair) reads the same both sides; asked again closer, its
				// edge shows (left out, it would break its loop, and the
				// outline it belongs to would be dropped whole)
				nrm = ptScale(nrm, 1e-3)
				l, rgt = r.inside(ptAdd(m, nrm)), r.inside(ptSub(m, nrm))
			}
			if l == rgt {
				continue
			}
			if rgt {
				a, b, ka, kb = b, a, kb, ka
			}
			if seen[[2][2]int64{ka, kb}] {
				continue
			}
			seen[[2][2]int64{ka, kb}] = true
			pieces = append(pieces, piece{a, b, ka, kb})
		}
	}
	// joined into loops: where outlines touch at a point, the sharpest turn
	// to the left is taken, so each loop goes round one area
	from := map[[2]int64][]int{}
	for i, p := range pieces {
		from[p.ka] = append(from[p.ka], i)
	}
	used := make([]bool, len(pieces))
	var loops [][]Pt
	for i := range pieces {
		if used[i] {
			continue
		}
		var loop []Pt
		cur := i
		for {
			used[cur] = true
			p := pieces[cur]
			loop = append(loop, p.a)
			if p.kb == pieces[i].ka {
				break
			}
			din := ptUnit(ptSub(p.b, p.a))
			next, best := -1, math.Inf(-1)
			for _, j := range from[p.kb] {
				if used[j] {
					continue
				}
				dout := ptUnit(ptSub(pieces[j].b, pieces[j].a))
				if turn := math.Atan2(ptCross(din, dout), ptDot(din, dout)); turn > best {
					next, best = j, turn
				}
			}
			if next < 0 {
				// Two cuts that are one point can land a hair further
				// apart than the keys merge (two edges crossing at a
				// shallow angle, as in a wide stroke at Quality 64, can
				// put them 27 eps apart). Rather than lose the whole outline, carry
				// on from the nearest start within gap, or close the loop.
				gap := 1e-6 * size
				if q := pieces[i].a; math.Hypot(q.X-p.b.X, q.Y-p.b.Y) <= gap {
					break
				}
				for j, q := range pieces {
					if used[j] || math.Hypot(q.a.X-p.b.X, q.a.Y-p.b.Y) > gap {
						continue
					}
					dout := ptUnit(ptSub(q.b, q.a))
					if turn := math.Atan2(ptCross(din, dout), ptDot(din, dout)); turn > best {
						next, best = j, turn
					}
				}
			}
			if next < 0 {
				loop = nil // broken: left out
				break
			}
			cur = next
		}
		if c := straighten(cleanContour(loop), eps); len(c) >= 3 && math.Abs(SignedArea(c)) > eps*size {
			loops = append(loops, c)
		}
	}
	return loops
}

// straighten drops the corners of outline c that lie within eps of the
// line through their two neighbours, between them. cleanContour drops
// those exactly in line; one a hair off (4e-12 mm, from rounding where
// stroke pieces join) stays, and the triangle it makes with its neighbours
// has next to no area: fitting the solid into another size loses it, and
// the gap doesn't always repair closed. Two corners next to each other
// never go in the same pass, so a fine curve isn't flattened step by
// step.
func straighten(c []Pt, eps float64) []Pt {
	for len(c) > 3 {
		out := make([]Pt, 0, len(c))
		keepNext, firstDropped := false, false
		for i, b := range c {
			if keepNext || (i == len(c)-1 && firstDropped) {
				out, keepNext = append(out, b), false
				continue
			}
			a, d := c[(i+len(c)-1)%len(c)], c[(i+1)%len(c)]
			ad, ab := ptSub(d, a), ptSub(b, a)
			l := math.Hypot(ad.X, ad.Y)
			if along := ptDot(ab, ad); l > eps && along > 0 && along < l*l && math.Abs(ptCross(ad, ab))/l <= eps {
				keepNext, firstDropped = true, firstDropped || i == 0
				continue
			}
			out = append(out, b)
		}
		if len(out) == len(c) {
			break
		}
		c = out
	}
	return c
}

// sketchCache keeps sketches' outlines and solids
var sketchCache = geom.NewCache(500_000, func(v any) int {
	switch x := v.(type) {
	case *geom.Mesh:
		return geom.MeshWeight(x)
	case [][]Pt:
		n := 1
		for _, c := range x {
			n += len(c) / 4
		}
		return n
	}
	return 1
})

// SketchOutlines returns the outlines of what a sketch builds (areas and
// lines joined; outer outlines anticlockwise, holes clockwise).
func SketchOutlines(sk *Sketch) [][]Pt { return SketchOutlinesN(sk, 0) }

// SketchOutlinesN is SketchOutlines with each curve cut into steps straight
// pieces (an extrusion's Quality; 0: fine enough to follow the curve).
func SketchOutlinesN(sk *Sketch, steps int) [][]Pt {
	if sk == nil || CheckSketch(sk) != nil {
		return nil
	}
	key := fmt.Sprintf("outline|%s|%d", SketchKey(sk), steps)
	if c, ok := sketchCache.Load(key); ok {
		return c.([][]Pt)
	}
	out := outlinesOf(sketchArea(sk, steps))
	sketchCache.Store(key, out)
	return out
}

// Extrude is how an extruded sketch rises: its curves cut into Quality
// pieces, the top and the base scaled (percent, round the drawing's
// middle), and the top turned Twist degrees in TwistSteps + 1 layers (at 0
// the top is joined straight to the bottom).
type Extrude struct {
	Quality             int
	TopScale, BaseScale float64
	Twist               float64
	TwistSteps          int
	// Smooth twists in many fine layers, each side cut into pieces too,
	// whatever TwistSteps says (Twist Mode Smooth)
	Smooth bool
}

// A Smooth twist: 96 layers, and each side of the outline cut into 8.
const (
	smoothTwistLayers = 96
	smoothTwistCuts   = 8
)

// extrudeValues reads an extruded sketch's settings.
func extrudeValues(v Values) Extrude {
	return Extrude{
		Quality:    int(math.Round(v.Get("quality", 12))),
		TopScale:   v.Get("topscale", 100),
		BaseScale:  v.Get("basescale", 100),
		Twist:      v.Get("twist", 0),
		TwistSteps: int(math.Round(v.Get("twiststeps", 0))),
		Smooth:     v.Get("twistmode", 0) == 1,
	}
}

// SketchSolid is the sketch built straight up between z0 and z1: its areas
// and lines, joined. Empty for a sketch with nothing to build.
func SketchSolid(sk *Sketch, z0, z1 float64) *geom.Mesh {
	return SketchExtrude(sk, z0, z1, Extrude{TopScale: 100, BaseScale: 100})
}

// SketchExtrude is the sketch built up between z0 and z1 as e says.
func SketchExtrude(sk *Sketch, z0, z1 float64, e Extrude) *geom.Mesh {
	cs := SketchOutlinesN(sk, e.Quality)
	if len(cs) == 0 {
		return &geom.Mesh{}
	}
	key := fmt.Sprintf("solid|%s|%v|%v|%+v", SketchKey(sk), z0, z1, e)
	if m, ok := sketchCache.Load(key); ok {
		return m.(*geom.Mesh)
	}
	var m *geom.Mesh
	if e.TopScale == e.BaseScale && e.Twist == 0 {
		s := e.TopScale / 100
		if s <= 0 {
			m = &geom.Mesh{}
		} else {
			if s != 1 {
				cs = scaleContours(cs, boundsOf(cs), s, 0)
			}
			m = ExtrudeContours(cs, z0, z1)
		}
	} else {
		m = loftContours(cs, z0, z1, e)
	}
	sketchCache.Store(key, m)
	return m
}

// scaleContours scales contours by s and turns them by ang radians, both
// round the middle of b.
func scaleContours(cs [][]Pt, b bounds2, s, ang float64) [][]Pt {
	c := Pt{(b.x0 + b.x1) / 2, (b.y0 + b.y1) / 2}
	cos, sin := math.Cos(ang)*s, math.Sin(ang)*s
	out := make([][]Pt, len(cs))
	for i, ring := range cs {
		r := make([]Pt, len(ring))
		for j, p := range ring {
			d := ptSub(p, c)
			r[j] = Pt{c.X + d.X*cos - d.Y*sin, c.Y + d.X*sin + d.Y*cos}
		}
		out[i] = r
	}
	return out
}

// loftContours extrudes contours from z0 to z1 changing as it rises: scaled
// from BaseScale to TopScale and turned through Twist, in layers. A scale
// of 0 brings that end to a point.
func loftContours(cs [][]Pt, z0, z1 float64, e Extrude) *geom.Mesh {
	// Twist Steps N is N + 1 layers (the top joined straight to the bottom
	// at 0, 6 layers at 5); Smooth is many layers with each side cut into
	// pieces
	layers, cuts := 1, 1
	switch {
	case e.Twist != 0 && e.Smooth:
		layers, cuts = smoothTwistLayers, smoothTwistCuts
	case e.Twist != 0:
		layers = max(1, min(e.TwistSteps+1, 721))
	}
	// a long outline gives way to the triangle budget: fewer cuts a side
	// first, then fewer layers (each side is two triangles a cut a layer)
	sides := 0
	for _, ring := range cs {
		sides += len(ring)
	}
	for cuts > 1 && 2*sides*cuts*layers > sketchTriBudget {
		cuts /= 2
	}
	layers = max(1, min(layers, sketchTriBudget/(2*sides*cuts)))
	b := boundsOf(cs)
	c := Pt{(b.x0 + b.x1) / 2, (b.y0 + b.y1) / 2}
	at := func(p Pt, k int) geom.Vec3 {
		t := float64(k) / float64(layers)
		s := (e.BaseScale + (e.TopScale-e.BaseScale)*t) / 100
		a := e.Twist * t * math.Pi / 180
		d := ptSub(p, c)
		cos, sin := math.Cos(a)*s, math.Sin(a)*s
		return geom.V(c.X+d.X*cos-d.Y*sin, c.Y+d.X*sin+d.Y*cos, z0+(z1-z0)*t)
	}
	m := &geom.Mesh{}
	type edge struct{ a, b Pt }
	count := map[edge]int{}
	var order []edge
	var caps [][3]Pt
	for _, r := range Regions(cs) {
		for _, t := range Triangulate(r) {
			if math.Abs(cross2(t[0], t[1], t[2]))/2 <= 1e-12 {
				continue
			}
			caps = append(caps, t)
			for k := 0; k < 3; k++ {
				ed := edge{t[k], t[(k+1)%3]}
				if ed.a == ed.b {
					continue
				}
				if _, ok := count[ed]; !ok {
					order = append(order, ed)
				}
				count[ed]++
			}
		}
	}
	boundary := func(ed edge) bool { return count[ed]-count[edge{ed.b, ed.a}] > 0 }
	// the walls follow the caps' sides: a cap cut in slabs (see
	// triangulate) has corners part way along the outlines' sides, and a
	// smooth twist's cuts and layers would multiply those past the budget
	// (a million triangles): the budget again, for the sides there are
	bs := 0
	for _, ed := range order {
		if boundary(ed) {
			bs++
		}
	}
	if bs > sides {
		for cuts > 1 && 2*bs*cuts*layers > sketchTriBudget {
			cuts /= 2
		}
		layers = max(1, min(layers, sketchTriBudget/(2*bs*cuts)))
	}
	// a side's points, its start included and its end not: cut into
	// pieces, as the walls are
	sidePts := func(ed edge) []Pt {
		out := make([]Pt, cuts)
		for k := range out {
			t := float64(k) / float64(cuts)
			out[k] = Pt{ed.a.X + (ed.b.X-ed.a.X)*t, ed.a.Y + (ed.b.Y-ed.a.Y)*t}
		}
		return out
	}
	for _, t := range caps {
		// a cap triangle on a cut side is a fan round its middle, through
		// the cut points, so the caps meet the walls point for point
		ring := []Pt{t[0], t[1], t[2]}
		if cuts > 1 {
			ring = ring[:0]
			for k := 0; k < 3; k++ {
				ed := edge{t[k], t[(k+1)%3]}
				if boundary(ed) {
					ring = append(ring, sidePts(ed)...)
				} else {
					ring = append(ring, ed.a)
				}
			}
		}
		emit := func(a, b, d Pt) {
			if e.TopScale > 0 {
				m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{at(a, layers), at(b, layers), at(d, layers)}})
			}
			if e.BaseScale > 0 {
				m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{at(a, 0), at(d, 0), at(b, 0)}})
			}
		}
		if len(ring) == 3 {
			emit(ring[0], ring[1], ring[2])
			continue
		}
		mid := Pt{(t[0].X + t[1].X + t[2].X) / 3, (t[0].Y + t[1].Y + t[2].Y) / 3}
		for k := range ring {
			emit(ring[k], ring[(k+1)%len(ring)], mid)
		}
	}
	for _, ed := range order {
		for n := count[ed] - count[edge{ed.b, ed.a}]; n > 0; n-- {
			pts := append(sidePts(ed), ed.b)
			for i := 0; i+1 < len(pts); i++ {
				a, bb := pts[i], pts[i+1]
				for k := 0; k < layers; k++ {
					p0, q0 := at(a, k), at(bb, k)
					p1, q1 := at(a, k+1), at(bb, k+1)
					if e.Twist == 0 {
						m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{p0, q0, q1}}, geom.Tri{P: [3]geom.Vec3{p0, q1, p1}})
						continue
					}
					// a twisted side isn't flat: folded along the diagonal
					// from the piece's end at the bottom to its start at the
					// top
					m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{p0, q0, p1}}, geom.Tri{P: [3]geom.Vec3{q0, q1, p1}})
				}
			}
		}
	}
	return cleanClosed(m)
}

// cleanClosed is m.Clean(), kept closed: where Clean drops a sliver (three
// of an outline's points in line to within rounding make one in a cap),
// the neighbours' sides it leaves meeting part way along are split there.
func cleanClosed(m *geom.Mesh) *geom.Mesh { return cleanClosedFix(m, false) }

// cleanClosedFix is cleanClosed, splitting sides at corners part way along
// them anyway when fix says so (a cap cut in slabs: see triangulate).
func cleanClosedFix(m *geom.Mesh, fix bool) *geom.Mesh {
	c := m.Clean()
	if fix || len(c.Tris) < len(m.Tris) {
		c = c.FixTJunctions()
	}
	return c
}

// Buildable reports whether a sketch makes a shape of kind: something to
// extrude, or for a revolve something left of the centre line.
func Buildable(kind string, sk *Sketch) bool {
	switch kind {
	case "revolve":
		return len(revolveProfile(sk, 0)) > 0
	case "coil":
		return len(coilProfile(sk, 0)) > 0
	}
	return len(SketchOutlines(sk)) > 0
}

// extrudeHeight is the height an extruded sketch has at scale 1 (mm).
const extrudeHeight = 10

// RevolveOutlines returns the outlines of the part of a sketch that a
// revolve spins: what is left of the centre line (x < 0).
func RevolveOutlines(sk *Sketch) [][]Pt { return RevolveOutlinesN(sk, 0) }

// RevolveOutlinesN is RevolveOutlines with each curve cut into steps
// pieces (0: fine enough to follow it).
func RevolveOutlinesN(sk *Sketch, steps int) [][]Pt { return spunOutlines(sk, steps, 0) }

// CoilOutlines returns the outlines of the part of a sketch that a coil
// spins: what is left of the centre line, kept coilAxisGap clear of it.
func CoilOutlines(sk *Sketch) [][]Pt { return spunOutlines(sk, 0, -coilAxisGap) }

// coilAxisGap is how far (mm) a coil's profile is kept from its axis: on
// the axis, one turn's middle would meet the next's.
const coilAxisGap = 0.05

// spunOutlines is what of the drawing is left of x = x0.
func spunOutlines(sk *Sketch, steps int, x0 float64) [][]Pt {
	cs := SketchOutlinesN(sk, steps)
	if len(cs) == 0 {
		return nil
	}
	b := boundsOf(cs)
	if b.x0 >= x0 {
		return nil
	}
	if b.x1 <= x0 {
		return cs
	}
	key := fmt.Sprintf("revolve|%s|%d|%v", SketchKey(sk), steps, x0)
	if c, ok := sketchCache.Load(key); ok {
		return c.([][]Pt)
	}
	// clipped at the centre line
	var r area
	var polys []boxedPoly
	for _, c := range cs {
		polys = append(polys, boxed(c))
		r.edges = append(r.edges, ringEdges(c)...)
	}
	r.edges = append(r.edges, [2]Pt{{x0, b.y0 - 1}, {x0, b.y1 + 1}})
	r.inside = func(q Pt) bool {
		if q.X >= x0 {
			return false
		}
		odd := false
		for _, p := range polys {
			if p.has(q) {
				odd = !odd
			}
		}
		return odd
	}
	out := outlinesOf(r)
	sketchCache.Store(key, out)
	return out
}

// revolveProfile returns the regions a revolve spins, in (radius, height)
// coordinates.
func revolveProfile(sk *Sketch, steps int) []Region { return spunProfile(RevolveOutlinesN(sk, steps)) }

// coilProfile is a coil's profile, in (radius, height) coordinates.
func coilProfile(sk *Sketch, steps int) []Region {
	return spunProfile(spunOutlines(sk, steps, -coilAxisGap))
}

func spunProfile(outlines [][]Pt) []Region {
	var out [][]Pt
	for _, c := range outlines {
		rc := make([]Pt, len(c))
		for i, p := range c {
			// x mirrored to a radius (the outline turns the other way)
			r := -p.X
			if r < 0 {
				r = 0 // on the axis, exactly
			}
			rc[len(c)-1-i] = Pt{r, p.Y}
		}
		out = append(out, rc)
	}
	return splitTouching(Regions(out))
}

// splitTouching gives every side of a region's outlines a point where a
// corner of that region's outlines lies on it part way along (a hole's
// corner touching the outline, say). The caps' triangles end at such a
// corner, and the walls, which follow the sides, must too: without the
// point the two meet in a T and the solid is open. (Cleaning an outline
// drops the point, in line with its neighbours as it is.)
func splitTouching(regs []Region) []Region {
	for ri, r := range regs {
		rings := append([][]Pt{r.Outer}, r.Holes...)
		var pts []Pt
		for _, c := range rings {
			pts = append(pts, c...)
		}
		sort.Slice(pts, func(i, j int) bool { return pts[i].X < pts[j].X })
		b := boundsOf(rings)
		eps := 1e-9 * math.Max(1, math.Max(b.x1-b.x0, b.y1-b.y0))
		for ci, c := range rings {
			var out []Pt // the outline with its points added; nil until one is
			for i, p := range c {
				q := c[(i+1)%len(c)]
				d := ptSub(q, p)
				l := math.Hypot(d.X, d.Y)
				var on []Pt
				if l > eps {
					lo := sort.Search(len(pts), func(k int) bool { return pts[k].X >= math.Min(p.X, q.X)-eps })
					for _, v := range pts[lo:] {
						if v.X > math.Max(p.X, q.X)+eps {
							break
						}
						w := ptSub(v, p)
						if along := ptDot(w, d) / l; along > eps && along < l-eps && math.Abs(ptCross(d, w))/l <= eps {
							on = append(on, v)
						}
					}
				}
				if len(on) > 0 && out == nil {
					out = append(make([]Pt, 0, len(c)+len(on)), c[:i]...)
				}
				if out == nil {
					continue
				}
				out = append(out, p)
				sort.Slice(on, func(a, b int) bool { return ptDot(ptSub(on[a], p), d) < ptDot(ptSub(on[b], p), d) })
				for _, v := range on {
					if !same(v, out[len(out)-1]) {
						out = append(out, v)
					}
				}
			}
			if out == nil {
				continue
			}
			if ci == 0 {
				regs[ri].Outer = out
			} else {
				regs[ri].Holes[ci-1] = out
			}
		}
	}
	return regs
}

// revolveMesh spins the profile round the Z axis through sweep degrees in
// sides steps, anticlockwise from start degrees past +Y, its curves cut
// into quality pieces. Start 0, Sweep 90 runs from 90° to 180°, and the
// sides divide the sweep, not a full turn.
func revolveMesh(sk *Sketch, start, sweep float64, sides, quality int) *geom.Mesh {
	regs := revolveProfile(sk, quality)
	if len(regs) == 0 {
		return nil
	}
	full := sweep >= 360-1e-9
	if full {
		sweep = 360
	}
	steps := spinSteps(regs, max(1, sides), 3)
	cs, sn := make([]float64, steps+1), make([]float64, steps+1)
	for j := 0; j <= steps; j++ {
		a := (90 + start + sweep*float64(j)/float64(steps)) * math.Pi / 180
		cs[j], sn[j] = math.Cos(a), math.Sin(a)
	}
	if full {
		cs[steps], sn[steps] = cs[0], sn[0] // closed exactly
	}
	at := func(p Pt, j int) geom.Vec3 { return geom.V(p.X*cs[j], p.X*sn[j], p.Y) }
	m := &geom.Mesh{}
	fell := false // a cap cut in slabs (see triangulate): its corners need the walls split
	tri := func(a, b, c geom.Vec3) { m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{a, b, c}}) }
	wall := func(c []Pt) {
		for i := range c {
			p, q := c[i], c[(i+1)%len(c)]
			if p.X == 0 && q.X == 0 {
				continue // along the axis: no surface
			}
			for j := 0; j < steps; j++ {
				tri(at(p, j), at(p, j+1), at(q, j+1))
				tri(at(p, j), at(q, j+1), at(q, j))
			}
		}
	}
	for _, r := range regs {
		wall(r.Outer)
		for _, h := range r.Holes {
			wall(h)
		}
		if !full {
			ts, slabs := triangulate(r)
			fell = fell || slabs
			for _, t := range ts {
				tri(at(t[0], 0), at(t[1], 0), at(t[2], 0))
				tri(at(t[0], steps), at(t[2], steps), at(t[1], steps))
			}
		}
	}
	// (a sweep of a degree or so makes walls thin enough for Clean to drop
	// as slivers)
	return cleanClosedFix(m, fell)
}

// coilMesh spins the profile round the Z axis through turns full turns,
// rising pitch mm a turn, in sides steps a turn, from start degrees past
// +Y as a revolve does. The pitch is never less than the profile's height
// (plus a hair), so the turns can't run into each other.
func coilMesh(sk *Sketch, turns, pitch, start float64, sides, quality int) *geom.Mesh {
	return coilMeshAs(sk, turns, pitch, start, sides, quality, coilQuick)
}

// coilHow is a way to build a coil. They make the same solid, but differ
// in how well it survives being fitted into a much smaller size (fifty
// 200 mm turns into 20 mm), which flattens its thinnest triangles to
// nothing: see fitCoil, which chooses between them.
type coilHow int

const (
	coilQuick coilHow = iota // the caps from the cut profile (see coilCap)
	// the profile's sides not cut at its corners' radii, so there are no
	// thin strips or cap pieces to lose (and far fewer triangles); walls
	// facing each other at a tight pitch may then cross by up to a step's
	// rise
	coilUncut
	coilFan // cut, the caps the sure way: slow to repair, the last resort
)

// coilMeshAs is coilMesh built the way how says.
func coilMeshAs(sk *Sketch, turns, pitch, start float64, sides, quality int, how coilHow) *geom.Mesh {
	regs := coilProfile(sk, quality)
	if len(regs) == 0 || !(turns > 0) {
		return nil
	}
	y0, y1 := math.Inf(1), math.Inf(-1)
	for _, r := range regs {
		for _, p := range r.Outer {
			y0, y1 = math.Min(y0, p.Y), math.Max(y1, p.Y)
		}
	}
	pitch = math.Max(pitch, CoilMinPitch(y1-y0))
	// (never fewer than 3 sides a turn, whatever the budget; a profile too
	// detailed for even that, thousands of corners coiled 50 times, isn't
	// built: millions of triangles)
	// the walls' profile: its sides cut at its corners' radii (see wall
	// below), which the steps' budget counts
	radii, near, cut := coilRadii(regs)
	cut = cut && how != coilUncut
	least := int(math.Ceil(3 * turns))
	want := max(1, int(math.Ceil(float64(max(3, sides))*turns-1e-9)))
	wallRegs := regs
	if cut {
		wallRegs = make([]Region, len(regs))
		for i, r := range regs {
			wallRegs[i].Outer = cutAtRadii(r.Outer, radii, near)
			for _, h := range r.Holes {
				wallRegs[i].Holes = append(wallRegs[i].Holes, cutAtRadii(h, radii, near))
			}
		}
	}
	if 2*regionSides(wallRegs)*least > 8*sketchTriBudget {
		return nil
	}
	steps := spinSteps(wallRegs, want, least)
	// The steps are the same angles on every turn (so walls facing each
	// other match: see wall below).
	// (A coil ending part way round a turn has a step at that angle on
	// every turn, so its last turn's last step has one below it too.)
	perTurn := max(3, int(math.Floor(float64(steps)/turns+1e-9)))
	tol := 1e-3 / float64(perTurn)
	var round []float64 // the steps' places in a turn, as fractions of it
	for k := 0; k < perTurn; k++ {
		round = append(round, float64(k)/float64(perTurn))
	}
	if f := turns - math.Floor(turns); f > tol && f < 1-tol {
		k := int(f * float64(perTurn))
		if lo, hi := float64(k)/float64(perTurn), float64(k+1)/float64(perTurn); f-lo > tol && hi-f > tol {
			round = append(round[:k+1], append([]float64{f}, round[k+1:]...)...)
		}
	}
	var ts []float64 // where each step is, in turns
	for n := 0; ; n++ {
		for _, f := range round {
			if t := float64(n) + f; t < turns-tol {
				ts = append(ts, t)
			}
		}
		if float64(n+1) >= turns-tol {
			break
		}
	}
	ts = append(ts, turns)
	steps = len(ts) - 1
	cs, sn, dz := make([]float64, steps+1), make([]float64, steps+1), make([]float64, steps+1)
	for j, t := range ts {
		a := (90 + start + 360*t) * math.Pi / 180
		cs[j], sn[j], dz[j] = math.Cos(a), math.Sin(a), pitch*t
	}
	at := func(p Pt, j int) geom.Vec3 { return geom.V(p.X*cs[j], p.X*sn[j], p.Y+dz[j]) }
	m := &geom.Mesh{}
	fell := false // a cap cut in slabs (see triangulate): its corners need the walls split
	tri := func(a, b, c geom.Vec3) { m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{a, b, c}}) }
	// Every profile side is cut at each corner's radius (radii: see
	// coilRadii), so any two walls facing each other across a gap (one
	// turn's top and the next's bottom, or a thin part's two sides) span
	// the same radii at the same steps: folded the same way in plan (from
	// the outer corner at one step to the inner at the next), they are the
	// same triangles in plan, and their heights apart there are an average
	// of their corners' (all positive), so they can't cross. Cut otherwise,
	// a sloped profile's turns would run into each other, or a thin part
	// turn inside out, by up to a step's rise.
	wall := func(c []Pt) {
		for i := range c {
			p, q := c[i], c[(i+1)%len(c)]
			for j := 0; j < steps; j++ {
				a, b, cc, d := at(p, j), at(p, j+1), at(q, j+1), at(q, j)
				if math.Abs(p.X) >= math.Abs(q.X) {
					tri(a, b, cc)
					tri(a, cc, d)
				} else {
					tri(a, b, d)
					tri(b, cc, d)
				}
			}
		}
	}
	for i := range regs {
		wall(wallRegs[i].Outer)
		for _, h := range wallRegs[i].Holes {
			wall(h)
		}
		var ts [][3]Pt
		var slabs bool
		if cut {
			ts, slabs = coilCap(regs[i], wallRegs[i], radii, near, how == coilFan)
		} else {
			ts, slabs = triangulate(regs[i])
		}
		fell = fell || slabs
		for _, t := range ts {
			tri(at(t[0], 0), at(t[1], 0), at(t[2], 0))
			tri(at(t[0], steps), at(t[2], steps), at(t[1], steps))
		}
	}
	if how == coilFan && !fell {
		// As built: the fan's thinnest pieces are for the coil's build to
		// drop and repair after the fit, with those the fit flattens, in
		// one pass. (Cleaned here, the repair would run twice, seconds
		// each time.)
		return m
	}
	return cleanClosedFix(m, fell)
}

// coilCap is a coil's end cap: triangles covering region reg whose corners
// include every point of cutReg, which is reg with its sides cut at radii
// (the walls' profile), so the cap meets the walls point for point. fell
// is as triangulate's.
//
// There are two ways to make it. The cut profile triangulated whole is
// quick to build and is used when it fits (capFits: points in a line on
// a side can defeat the ear clipping, leaving one out or covering a place
// twice). The sure way, taken when it doesn't or when fan says so, is
// reg's own triangles, each taking its sides' cut points as a fan round
// its middle.
//
// They differ when a coil is fitted into a much smaller size (fifty
// 200 mm turns into 20 mm), which flattens the thinnest cap triangles to
// nothing. The gaps they leave are repaired: quickly but not always
// cleanly for the whole profile's, slowly and more often cleanly for the
// fan's. fitCoil chooses between them, the fan last.
func coilCap(reg, cutReg Region, radii []float64, tol float64, fan bool) (tris [][3]Pt, fell bool) {
	if !fan {
		if ts, slabs := triangulate(cutReg); slabs || capFits(ts, cutReg) {
			return ts, slabs
		}
	}
	ts, fell := triangulate(reg)
	for _, t := range ts {
		ring := make([]Pt, 0, 8)
		for k := range 3 {
			ring = append(ring, t[k])
			ring = append(ring, sideCuts(t[k], t[(k+1)%3], radii, tol)...)
		}
		if len(ring) == 3 {
			tris = append(tris, t)
			continue
		}
		mid := Pt{(t[0].X + t[1].X + t[2].X) / 3, (t[0].Y + t[1].Y + t[2].Y) / 3}
		for k := range ring {
			tris = append(tris, [3]Pt{mid, ring[k], ring[(k+1)%len(ring)]})
		}
	}
	return tris, fell
}

// capFits is whether triangles ts cover region r as a cap must: every side
// of r is one triangle's edge, once, and every other edge is shared by
// exactly two triangles.
func capFits(ts [][3]Pt, r Region) bool {
	type edge [2]Pt
	key := func(a, b Pt) edge {
		if b.X < a.X || (b.X == a.X && b.Y < a.Y) {
			a, b = b, a
		}
		return edge{a, b}
	}
	n := map[edge]int{}
	for _, t := range ts {
		for k := range 3 {
			n[key(t[k], t[(k+1)%3])]++
		}
	}
	sides := 0
	for _, c := range append([][]Pt{r.Outer}, r.Holes...) {
		for i := range c {
			if n[key(c[i], c[(i+1)%len(c)])] != 1 {
				return false
			}
			sides++
		}
	}
	inner := 0
	for _, k := range n {
		if k == 2 {
			inner++
		} else if k != 1 {
			return false
		}
	}
	return len(n) == sides+inner
}

// coilRadii are the radii (|x|) of a coil profile's corners, sorted, and
// whether to cut the profile's sides at them: not when that would more than
// triple the sides the walls have (a profile of many corners at many radii:
// slow to build, for a guarantee only tight pitches need).
func coilRadii(regs []Region) (radii []float64, tol float64, cut bool) {
	var rs []float64
	var all [][]Pt
	sides := 0
	for _, r := range regs {
		for _, c := range append([][]Pt{r.Outer}, r.Holes...) {
			sides += len(c)
			all = append(all, c)
			for _, p := range c {
				rs = append(rs, math.Abs(p.X))
			}
		}
	}
	tol = cutTol(all)
	sort.Float64s(rs)
	out := rs[:0]
	for _, r := range rs {
		if len(out) == 0 || r-out[len(out)-1] > tol {
			out = append(out, r)
		}
	}
	// the cuts each side gets: the radii strictly within its span
	cuts := 0
	for _, c := range all {
		for i := range c {
			lo, hi := math.Abs(c[i].X), math.Abs(c[(i+1)%len(c)].X)
			if lo > hi {
				lo, hi = hi, lo
			}
			cuts += max(0, sort.SearchFloat64s(out, hi-tol)-sort.SearchFloat64s(out, lo+tol))
		}
	}
	return out, tol, cuts > 0 && cuts <= 2*sides+64
}

// cutTol is how near two radii are taken as one in a coil's cuts, and how
// near a corner a side is left uncut: a millionth of the profile's size. A
// cut closer than that makes a sliver, which a coil squashed small (fifty
// turns fitted into 20 mm) flattens to nothing, and the gap left would
// need the slow T-junction repair.
func cutTol(cs [][]Pt) float64 {
	b := boundsOf(cs)
	return 1e-6 * math.Max(1, math.Max(b.x1-b.x0, b.y1-b.y0))
}

// cutAtRadii is outline c with a point added on each side wherever it
// crosses one of radii (|x|), in order along the side.
func cutAtRadii(c []Pt, radii []float64, tol float64) []Pt {
	out := make([]Pt, 0, len(c)*2)
	for i := range c {
		out = append(out, c[i])
		out = append(out, sideCuts(c[i], c[(i+1)%len(c)], radii, tol)...)
	}
	return out
}

// sideCuts are the points where the side from p to q crosses radii (|x|),
// more than tol from its ends' radii, in order from p. Both directions
// give the same points bit for bit, so a wall and a cap triangle that
// share the side meet exactly.
func sideCuts(p, q Pt, radii []float64, tol float64) []Pt {
	if q.X < p.X || (q.X == p.X && q.Y < p.Y) {
		pts := sideCuts(q, p, radii, tol)
		slices.Reverse(pts)
		return pts
	}
	rp, rq := math.Abs(p.X), math.Abs(q.X)
	lo, hi := math.Min(rp, rq), math.Max(rp, rq)
	a, b := sort.SearchFloat64s(radii, lo+tol), sort.SearchFloat64s(radii, hi-tol)
	if a >= b || rp == rq {
		return nil
	}
	pts := make([]Pt, 0, b-a)
	for k := a; k < b; k++ {
		t := (radii[k] - rp) / (rq - rp)
		pts = append(pts, Pt{p.X + t*(q.X-p.X), p.Y + t*(q.Y-p.Y)})
	}
	if rq < rp {
		for l, r := 0, len(pts)-1; l < r; l, r = l+1, r-1 {
			pts[l], pts[r] = pts[r], pts[l]
		}
	}
	return pts
}

// spinSteps is how many steps a profile is spun in: steps, or as many
// fewer as keep its walls (two triangles a profile side a step) within the
// triangle budget, but at least least.
func spinSteps(regs []Region, steps, least int) int {
	return max(1, least, min(steps, sketchTriBudget/(2*max(1, regionSides(regs)))))
}

// regionSides is how many sides regions' outlines have in all.
func regionSides(regs []Region) int {
	sides := 0
	for _, r := range regs {
		sides += len(r.Outer)
		for _, h := range r.Holes {
			sides += len(h)
		}
	}
	return sides
}

// CoilMinPitch is the least pitch a coil whose profile is h tall can have
// without one turn touching the next.
func CoilMinPitch(h float64) float64 { return h + math.Max(0.01, h*1e-3) }

// coilValues reads a coil's settings.
func coilValues(v Values) (turns, pitch, start float64, sides, quality int) {
	return v.Get("turns", 3), v.Get("pitch", 6), v.Get("start", 0), int(math.Round(v.Get("sides", 48))), int(math.Round(v.Get("quality", 12)))
}

// revolveValues reads a revolve's settings.
func revolveValues(v Values) (start, sweep float64, sides, quality int) {
	return v.Get("start", 0), v.Get("sweep", 360), int(math.Round(v.Get("sides", 48))), int(math.Round(v.Get("quality", 12)))
}

// sketchModel is a sketch shape in its own frame (empty for a sketch with
// nothing to build).
func sketchModel(kind string, v Values) *geom.Mesh { return sketchModelAs(kind, v, coilQuick) }

// sketchModelAs is sketchModel, a coil built the way how says.
func sketchModelAs(kind string, v Values, how coilHow) *geom.Mesh {
	m := sketchModelAt(kind, v, how)
	// A drawing whose only bends are in its curves can come to nothing at
	// a low Quality (two points joined by two curves are, at one straight
	// piece each, a line there and back). It is built at the lowest
	// quality above that makes something of it.
	for q := v.Get("quality", 12); len(m.Tris) == 0 && q < 64 && Buildable(kind, v.Sketch); {
		q = math.Min(64, math.Max(2, 2*q))
		w := v
		w.Num = map[string]float64{}
		for key, x := range v.Num {
			w.Num[key] = x
		}
		w.Num["quality"] = q
		m = sketchModelAt(kind, w, how)
	}
	return m
}

// maxOutlinePoints is about the most points a drawing's outlines are
// flattened to: Quality gives way past it. (20 000 curved points at
// Quality 64 would be 1.28 million, which take many minutes to
// triangulate.)
const maxOutlinePoints = 64_000

// sketchModelAt is sketchModel with the settings as they stand.
func sketchModelAt(kind string, v Values, how coilHow) *geom.Mesh {
	if v.Sketch != nil {
		pts := 0
		for _, p := range v.Sketch.Paths {
			pts += len(p.Points)
		}
		if most := max(1, maxOutlinePoints/max(1, pts)); v.Get("quality", 12) > float64(most) {
			w := v
			w.Num = map[string]float64{}
			for key, x := range v.Num {
				w.Num[key] = x
			}
			w.Num["quality"] = float64(most)
			v = w
		}
	}
	switch kind {
	case "revolve":
		start, sweep, sides, quality := revolveValues(v)
		if m := revolveMesh(v.Sketch, start, sweep, sides, quality); m != nil {
			return m
		}
		return &geom.Mesh{}
	case "coil":
		turns, pitch, start, sides, quality := coilValues(v)
		if m := coilMeshAs(v.Sketch, turns, pitch, start, sides, quality, how); m != nil {
			return m
		}
		return &geom.Mesh{}
	}
	return SketchExtrude(v.Sketch, 0, extrudeHeight, extrudeValues(v))
}

// SketchBounds is the box a sketch shape fills in its own frame (empty for
// a sketch with nothing to build).
func SketchBounds(kind string, v Values) geom.Box {
	// as Build uses the settings: a file can hold anything
	if k, ok := catalogue[kind]; ok {
		v = k.clamp(v)
	}
	start, sweep, sides, quality := revolveValues(v)
	turns, pitch, _, _, _ := coilValues(v)
	key := fmt.Sprintf("bounds|%s|%s|%v|%v|%d|%d|%v|%v|%+v", kind, SketchKey(v.Sketch), start, sweep, sides, quality, turns, pitch, extrudeValues(v))
	if b, ok := sketchCache.Load(key); ok {
		return b.(geom.Box)
	}
	b := sketchModel(kind, v).Bounds()
	sketchCache.Store(key, b)
	return b
}

// the default sketches, for placing from the API without a drawing (and the
// tiles): a freehand blob for the extrusion (drawn-looking, not a basic
// shape), a goblet's half for the revolve, a round wire for the coil (a
// spring)
var (
	defaultSketch = &Sketch{Paths: []SketchPath{{Closed: true, Points: []SketchPoint{
		{P: [2]float64{-12, -6}, Kind: PointAuto},
		{P: [2]float64{-2, -10}, Kind: PointAuto},
		{P: [2]float64{11, -8}, Kind: PointAuto},
		{P: [2]float64{9, 1}, Kind: PointAuto},
		{P: [2]float64{12, 9}, Kind: PointAuto},
		{P: [2]float64{2, 10}, Kind: PointAuto},
		{P: [2]float64{-3, 4}, Kind: PointAuto},
		{P: [2]float64{-11, 6}, Kind: PointAuto},
	}}}}
	defaultCoil = &Sketch{Paths: []SketchPath{{Closed: true, Points: []SketchPoint{
		{P: [2]float64{-11.5, 1.5}, Kind: PointAuto},
		{P: [2]float64{-10, 0}, Kind: PointAuto},
		{P: [2]float64{-8.5, 1.5}, Kind: PointAuto},
		{P: [2]float64{-10, 3}, Kind: PointAuto},
	}}}}
	defaultRevolve = &Sketch{Paths: []SketchPath{{Closed: true, Points: []SketchPoint{
		{P: [2]float64{0, 0}},
		{P: [2]float64{-9, 0}},
		{P: [2]float64{-9, 2}},
		{P: [2]float64{-2, 3}, Kind: PointAuto},
		{P: [2]float64{-2, 10}, Kind: PointAuto},
		{P: [2]float64{-10, 18}, Kind: PointAuto},
		{P: [2]float64{-10, 22}},
		{P: [2]float64{0, 22}},
	}}}}
)

// fitCoil is a coil fitted into its size, for Build: quick is the quick
// build's mesh fitted, and build makes the others (see coilHow).
//
// Fitting a coil into a much smaller size flattens some of its triangles
// to nothing. Build drops those and repairs the gaps they leave, which
// takes seconds on a large coil and doesn't always come out closed. So a
// build that loses nothing is looked for first (the quick one, as nearly
// every coil is; then the one without cuts, far smaller), and only then
// is each tried with its repair (mendFitted), the first closed taken.
func fitCoil(quick *geom.Mesh, build func(coilHow) *geom.Mesh) *geom.Mesh {
	whole := func(m *geom.Mesh) bool { return len(m.Clean().Tris) == len(m.Tris) }
	if whole(quick) {
		return quick
	}
	uncut := build(coilUncut)
	if whole(uncut) {
		return uncut
	}
	for how := coilQuick; how <= coilFan; how++ {
		var fit *geom.Mesh
		switch how {
		case coilQuick:
			fit = quick
		case coilUncut:
			fit = uncut
		default:
			fit = build(how)
		}
		if mended, ok := mendFitted(fit); ok {
			return mended
		}
	}
	return quick
}

// mendFitted readies a coil's mesh, fitted into its size, for Build. Build
// drops the triangles the fit flattened to nothing and repairs the gaps
// they leave, once; ok is whether that comes out closed. The repair is
// done here to see, and its result returned in a state Build will leave
// alone (nothing for its Clean to drop), so the repair, seconds on a
// large coil, isn't done twice. Where no such state is closed, m itself
// is returned for Build to repair the same way: a second repair of the
// repaired mesh has turned a closed one into one with wrong edges. A mesh
// that loses nothing is returned as it is, unchecked: it is as it was
// built.
func mendFitted(m *geom.Mesh) (out *geom.Mesh, ok bool) {
	closed := func(m *geom.Mesh) bool {
		open, nonManifold := m.EdgeCheck(1e-7)
		return open == 0 && nonManifold == 0
	}
	kept := m.Clean()
	if len(kept.Tris) == len(m.Tris) {
		return m, true
	}
	mended := kept.FixTJunctions()
	if !closed(mended) {
		return m, false
	}
	settled := mended.Clean()
	if len(settled.Tris) == len(mended.Tris) || closed(settled) {
		return settled, true
	}
	return m, true
}

func registerSketchKinds() {
	for _, d := range []struct {
		id, label, color string
		params           []Param
		sketch           *Sketch
	}{
		{"sketch", "Sketch", "#e6582e", []Param{
			slider("quality", "Quality", 1, 64, 1, 12, true),
			slider("topscale", "Top Scale (%)", 0, 200, 1, 100, false),
			slider("basescale", "Base Scale (%)", 0, 200, 1, 100, false),
			slider("twist", "Twist", -360, 360, 1, 0, false),
			choice("twistmode", "Twist Mode", []string{"Steps", "Smooth"}, 0),
			slider("twiststeps", "Twist Steps", 0, 360, 1, 0, true),
		}, defaultSketch},
		{"revolve", "Revolve Sketch", "#7b4fd6", []Param{
			slider("sides", "Sides", 3, 128, 1, 48, true),
			slider("start", "Start", 0, 360, 1, 0, false),
			slider("sweep", "Sweep", 1, 360, 1, 360, false),
			slider("quality", "Quality", 1, 64, 1, 12, true),
		}, defaultRevolve},
		{"coil", "Coil Sketch", "#1f9e8f", []Param{
			slider("turns", "Turns", 0.25, 50, 0.25, 3, false),
			lslider("pitch", "Pitch (rise a turn)", 0.1, 200, 0.1, 6),
			slider("sides", "Sides a Turn", 3, 128, 1, 48, true),
			slider("start", "Start", 0, 360, 1, 0, false),
			slider("quality", "Quality", 1, 64, 1, 12, true),
		}, defaultCoil},
	} {
		id := d.id
		k := &Kind{ID: id, Label: d.label, Color: d.color, Params: d.params, sketch: d.sketch,
			build: func(size geom.Vec3, v Values) *geom.Mesh {
				m := sketchModel(id, v)
				if len(m.Tris) == 0 {
					return roundedBox(size, 0, 1)
				}
				fit := m.FitTo(size)
				if id != "coil" {
					return fit
				}
				return fitCoil(fit, func(how coilHow) *geom.Mesh { return sketchModelAs(id, v, how).FitTo(size) })
			}}
		k.natural = func(v Values) geom.Vec3 {
			b := SketchBounds(id, v)
			if b.IsEmpty() {
				return geom.V(1, 1, 1)
			}
			return b.Size()
		}
		k.Size = k.natural(k.Defaults())
		register(k)
	}
}

// sketchTriBudget is about how many triangles a sketch shape may have: at
// the ends of the sliders (a coil's 50 turns of 128 sides, a smooth twist
// of a long outline) the sides, layers and cuts give way to keep within it.
const sketchTriBudget = 400_000
