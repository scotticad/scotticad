package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// The rounded box, the crescent, the port opening and the finishing washer
// (registered in zz_added.go).

// --- finishing washer ------------------------------------------------------------------

// finishingWasherModel is a finishing washer (a screw cup): a domed ring a
// countersunk screw's head sits in, for a tidy look on a panel. Flat
// underneath; a low upright rim, then a quarter-ellipse dome rising inward
// to the lip of the seat; the seat a cone of Seat Angle from the lip down
// Seat Depth to the hole; the hole straight through. Each measure is kept
// within what the others leave: the hole inside the rim, the seat inside
// the height with a ring under it.
func finishingWasherModel(v Values) *geom.Mesh {
	outer := math.Max(0.05, v.Get("diameter", 13.4)/2)
	h := math.Max(0.05, v.Get("height", 3.95))
	shaft := math.Max(math.Min(0.1, outer/4), math.Min(v.Get("hole", 4.5)/2, outer-math.Min(0.2, outer/4)))
	slope := math.Tan(math.Max(30, math.Min(v.Get("angle", 90), 150)) / 2 * math.Pi / 180)
	deepest := math.Min(h-math.Min(0.2, h/4), (outer-math.Min(0.2, (outer-shaft)/2)-shaft)/slope)
	seatDepth := math.Max(0, math.Min(v.Get("seat", 2.45), math.Max(0, deepest)))
	seat := shaft + seatDepth*slope // the lip's radius
	rim := 0.25 * (h - seatDepth)   // the rim's upright part
	sides := max(8, int(v.Get("sides", 48)))
	steps := max(8, int(math.Round(float64(sides)/3)))
	prof := [][2]float64{{shaft, 0}, {outer, 0}}
	for i := 0; i <= steps; i++ {
		a := math.Pi / 2 * float64(i) / float64(steps)
		if i == steps {
			prof = append(prof, [2]float64{seat, h})
			break
		}
		prof = append(prof, [2]float64{seat + (outer-seat)*math.Cos(a), rim + (h-rim)*math.Sin(a)})
	}
	if seatDepth > 1e-6 {
		prof = append(prof, [2]float64{shaft, h - seatDepth})
	}
	return revolve(prof, sides, true)
}

// --- rounded box -----------------------------------------------------------------------

// roundedBoxMesh is a box with its four upright edges rounded by Corner
// Radius and its top and bottom edges by Top Edge and Bottom Edge Radius,
// all in millimetres whatever the box's size (resizing doesn't stretch
// them). Each corner is an eighth of an ellipsoid Corner Radius across and
// the edge's radius up, so where the two radii differ a top or bottom
// edge's section is a quarter ellipse, and with no corner radius the top
// and bottom edges stay square. Steps is the segments in each quarter
// circle.
func roundedBoxMesh(s geom.Vec3, v Values) *geom.Mesh {
	h := s.Mul(0.5)
	rc := math.Max(0, math.Min(v.Get("radius", 5), math.Min(h.X, h.Y)))
	rb := math.Max(0, math.Min(v.Get("bottom", 0), s.Z))
	rt := math.Max(0, math.Min(v.Get("top", 0), s.Z-rb))
	n := max(1, int(v.Get("steps", 8)))
	if rc < 1e-9 {
		return extrude([]Pt{{-0.5, -0.5}, {0.5, -0.5}, {0.5, 0.5}, {-0.5, 0.5}}).FitTo(s)
	}
	cx, cy := h.X-rc, h.Y-rc // the corners' centres, seen from above
	// the surface as a grid: round the outline (four corners of n+1
	// points each), and from the bottom's middle to the top's (the bottom
	// edge's n+1 rows, then the top edge's)
	nu, nv := 4*(n+1), 2*(n+1)
	// a quarter circle's cosine and sine at step k of n, exact at its ends:
	// neighbouring corners and the flat middles meet there, and a sine of π
	// that isn't quite 0 leaves them apart where a coordinate is 0
	quarter := func(k int) (float64, float64) {
		switch k {
		case 0:
			return 1, 0
		case n:
			return 0, 1
		}
		a := float64(k) / float64(n) * math.Pi / 2
		return math.Cos(a), math.Sin(a)
	}
	pt := func(iu, iv int) geom.Vec3 {
		q, k := iu/(n+1), iu%(n+1)
		ca, sa := quarter(k)
		ca, sa = []float64{ca, -sa, -ca, sa}[q], []float64{sa, ca, -sa, -ca}[q] // turned q quarters
		half, l := iv/(n+1), iv%(n+1)
		cb, sb := quarter(n - l) // the bottom edge, from below
		sb = -sb
		zc, rz := -h.Z+rb, rb
		if half == 1 { // the top edge, up to the top
			cb, sb = quarter(l)
			zc, rz = h.Z-rt, rt
		}
		sx := []float64{1, -1, -1, 1}[q]
		sy := []float64{1, 1, -1, -1}[q]
		return geom.V(sx*cx+rc*cb*ca, sy*cy+rc*cb*sa, zc+rz*sb)
	}
	m := &geom.Mesh{}
	for iv := 0; iv < nv-1; iv++ {
		for iu := 0; iu < nu; iu++ {
			a, b := pt(iu, iv), pt((iu+1)%nu, iv)
			c, d := pt((iu+1)%nu, iv+1), pt(iu, iv+1)
			m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{a, b, c}}, geom.Tri{P: [3]geom.Vec3{a, c, d}})
		}
	}
	// the flat middles of the top and bottom, left open by the grid
	if cx > 1e-9 && cy > 1e-9 {
		for _, z := range []float64{-h.Z, h.Z} {
			p := []geom.Vec3{geom.V(cx, cy, z), geom.V(-cx, cy, z), geom.V(-cx, -cy, z), geom.V(cx, -cy, z)}
			if z < 0 {
				p[1], p[3] = p[3], p[1]
			}
			m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{p[0], p[1], p[2]}}, geom.Tri{P: [3]geom.Vec3{p[0], p[2], p[3]}})
		}
	}
	return m.Clean()
}

// --- crescent --------------------------------------------------------------------------

// crescentOutline is a crescent filling w × d, counter-clockwise: its back
// on -X, its horns' tips at the +X corners. The outer edge is the circle
// through both tips and the back's middle, the inner edge the circle
// through both tips and a point Thickness in from the back; the whole is
// then scaled along Y to fit d (the outer circle bulges past the tips
// unless the crescent is more than twice as deep as it is wide).
// Thickness, in mm, is kept from 1 mm to 85% of the width. Tip Radius
// rounds the horns. Steps is the number of points along the outline, half
// on each edge (at least 8 each). Rounded tips fall short of the box's
// corners, so the rounded outline is stretched to fill it again.
func crescentOutline(w, d float64, v Values) []Pt {
	w, d = math.Max(w, 1e-3), math.Max(d, 1e-3)
	t := math.Max(1, math.Min(v.Get("thickness", 14), math.Max(1, 0.85*w)))
	t = math.Min(t, 0.999*w)
	steps := max(16, int(v.Get("steps", 32)))
	hw, hd := w/2, d/2
	xo := d * d / (8 * w) // the outer circle's centre: equally far from the back and the tips
	ro := hw + xo
	xin := -hw + t // where the inner edge crosses the middle
	xi := (hw*hw + hd*hd - xin*xin) / (2 * (hw - xin))
	ri := xi - xin
	seg := max(8, int(math.Round(float64(steps)/2)))
	// from the top tip round the back to the bottom tip on the outer
	// circle, then back up the inner circle; both arcs pass through angle π
	ao := math.Atan2(hd, hw-xo)
	ai := math.Atan2(hd, hw-xi)
	var out []Pt
	for i := 0; i < seg; i++ {
		a := ao + (2*math.Pi-2*ao)*float64(i)/float64(seg)
		out = append(out, Pt{xo + ro*math.Cos(a), ro * math.Sin(a)})
	}
	out = append(out, Pt{hw, -hd})
	for i := 1; i < seg; i++ {
		a := -ai - (2*math.Pi-2*ai)*float64(i)/float64(seg)
		out = append(out, Pt{xi + ri*math.Cos(a), ri * math.Sin(a)})
	}
	// fit to the box (across, only an odd number of points on the outer
	// edge, missing the back's middle, leaves anything to fit), round the
	// tips, and fit it again: rounding sharp tips takes them in
	out = fitOutline(out, w, d)
	r := math.Max(0, math.Min(v.Get("tip", 0.5), 8))
	if r > 0.01 {
		arc := max(3, int(math.Round(float64(steps)/6)))
		out = roundCorner(out, seg, r, arc) // the bottom tip first: the top's index stays put
		out = roundCorner(out, 0, r, arc)
		out = fitOutline(out, w, d)
	}
	return out
}

// fitOutline scales and moves c to fill w × d about the origin.
func fitOutline(c []Pt, w, d float64) []Pt {
	lo, hi := Pt{math.Inf(1), math.Inf(1)}, Pt{math.Inf(-1), math.Inf(-1)}
	for _, p := range c {
		lo = Pt{math.Min(lo.X, p.X), math.Min(lo.Y, p.Y)}
		hi = Pt{math.Max(hi.X, p.X), math.Max(hi.Y, p.Y)}
	}
	out := make([]Pt, len(c))
	for i, p := range c {
		out[i] = Pt{(p.X - (lo.X+hi.X)/2) * w / (hi.X - lo.X), (p.Y - (lo.Y+hi.Y)/2) * d / (hi.Y - lo.Y)}
	}
	return out
}

// roundCorner replaces corner i of a closed outline with an arc of radius
// r (n segments) tangent to its two sides. The arc takes at most 40% of
// either side, its radius shrinking to fit.
func roundCorner(c []Pt, i int, r float64, n int) []Pt {
	p, prev, next := c[i], c[(i+len(c)-1)%len(c)], c[(i+1)%len(c)]
	l1, l2 := math.Hypot(prev.X-p.X, prev.Y-p.Y), math.Hypot(next.X-p.X, next.Y-p.Y)
	if l1 < 1e-12 || l2 < 1e-12 {
		return c
	}
	u1 := Pt{(prev.X - p.X) / l1, (prev.Y - p.Y) / l1}
	u2 := Pt{(next.X - p.X) / l2, (next.Y - p.Y) / l2}
	half := math.Acos(math.Max(-1, math.Min(1, u1.X*u2.X+u1.Y*u2.Y))) / 2 // half the corner's angle
	if half < 1e-6 || half > math.Pi/2-1e-6 {
		return c
	}
	t := math.Min(0.4*math.Min(l1, l2), r/math.Tan(half)) // from the corner to where the arc starts
	r = t * math.Tan(half)
	if t <= 0.01 || r <= 0.01 {
		return c
	}
	bis := Pt{u1.X + u2.X, u1.Y + u2.Y}
	bl := math.Hypot(bis.X, bis.Y)
	ctr := Pt{p.X + bis.X/bl*r/math.Sin(half), p.Y + bis.Y/bl*r/math.Sin(half)}
	a1 := math.Atan2(p.Y+t*u1.Y-ctr.Y, p.X+t*u1.X-ctr.X)
	a2 := math.Atan2(p.Y+t*u2.Y-ctr.Y, p.X+t*u2.X-ctr.X)
	da := math.Remainder(a2-a1, 2*math.Pi) // the short way round
	arc := make([]Pt, 0, n+1)
	for k := 0; k <= n; k++ {
		a := a1 + da*float64(k)/float64(n)
		arc = append(arc, Pt{ctr.X + r*math.Cos(a), ctr.Y + r*math.Sin(a)})
	}
	out := make([]Pt, 0, len(c)+n)
	out = append(out, c[:i]...)
	out = append(out, arc...)
	return append(out, c[i+1:]...)
}

func crescentMesh(s geom.Vec3, v Values) *geom.Mesh {
	return ExtrudeContours([][]Pt{crescentOutline(s.X, s.Y, v)}, -s.Z/2, s.Z/2)
}

// --- port opening ----------------------------------------------------------------------

// A port opening is the cut-out a connector needs in an enclosure's wall:
// seen from the front, the connector's outline, Width × Height, pushed
// Depth through the wall (along Y). Connector picks a ready-made opening;
// Custom uses the Custom settings instead. Clearance adds room on every
// side of either.

// portOutlines are the Custom Outline choices.
var portOutlines = []string{"Rounded", "HDMI", "Mini HDMI", "Micro HDMI", "DisplayPort", "Mini DisplayPort",
	"XT60", "XT30", "MT60", "D-Sub"}

// portCuts are the outlines with their lower corners cut off at a slant,
// so the plug goes in one way up: each cut's width and height as shares
// of the opening's (so it grows with the opening), and whether only the
// left corner is cut.
var portCuts = map[int]struct {
	x, y     float64
	leftOnly bool
}{
	1: {2.45 / 15.7, 1.2 / 6.2, false}, // HDMI: panel cut-out 15.70 × 6.20, bottom edge 10.80, slants 1.20 high
	2: {1 / 11.2, 0.7 / 3.1, false},    // Mini HDMI: 10.5 × 2.5 shell plus about 0.3 mm round it
	3: {1 / 6.8, 0.9 / 3.6, false},     // Micro HDMI: 6.2 × 3.0 shell plus about 0.3 mm (Raspberry Pi 4 and 5)
	4: {1.8 / 17.1, 1.8 / 5.76, true},  // DisplayPort (VESA panel cut-out 17.10 × 5.76): one corner, 1.80 × 1.80
	5: {1.4 / 8.8, 1.4 / 5.9, false},   // Mini DisplayPort: 45° cuts on an 8.30-wide shell plus 0.25 mm
}

// portPolygons are the outlines that are rounded polygons: their corners,
// counter-clockwise, in the box inside the rounding (x across the middle,
// y from 0 up), from that box's width and height.
var portPolygons = map[int]func(w, h float64) [][2]float64{
	// XT60 (16.00 × 8.10): one end narrowed by two cuts 3.25 along and 2.95 across
	6: func(w, h float64) [][2]float64 {
		a, b := 3.25/16, 2.95/8.1
		return [][2]float64{{(-0.5 + a) * w, 0}, {0.5 * w, 0}, {0.5 * w, h}, {(-0.5 + a) * w, h}, {-0.5 * w, (1 - b) * h}, {-0.5 * w, b * h}}
	},
	// XT30 (10.20 × 5.20): its end cut as the XT60's
	7: func(w, h float64) [][2]float64 {
		a, b := 0.254, 0.275
		return [][2]float64{{(-0.5 + a) * w, 0}, {0.5 * w, 0}, {0.5 * w, h}, {(-0.5 + a) * w, h}, {-0.5 * w, (1 - b) * h}, {-0.5 * w, b * h}}
	},
	// MT60: a rounded triangle, 12.00 from its flat side to its tip, 12.50 along the flat side
	8: func(w, h float64) [][2]float64 {
		return [][2]float64{{-0.5 * w, 0}, {0.5 * w, 0.5 * h}, {-0.5 * w, h}}
	},
	// D-Sub (19.74 × 11.18 cut-out): the wide side up, the sides 10° from upright
	9: func(w, h float64) [][2]float64 {
		in := math.Min(w/2-1e-3, h*math.Tan(10*math.Pi/180))
		return [][2]float64{{-w/2 + in, 0}, {w/2 - in, 0}, {w / 2, h}, {-w / 2, h}}
	},
}

// portConnector is a ready-made opening: its size in mm (with the room the
// connector needs already in it), corner radius, outline (an index into
// portOutlines), and the screw holes beside it, if any (centre to centre,
// and their diameter).
type portConnector struct {
	name                 string
	width, height, round float64
	outline              int
	holeSpacing, holeDia float64
}

// portConnectors are the Connector choices after Custom: the openings
// from the connectors' published sizes and panel cut-outs.
var portConnectors = []portConnector{
	{name: "USB-C", width: 9.4, height: 3.7, round: 1.85},            // receptacle 8.94 × 3.26 plus 0.25 mm: round ends
	{name: "USB-C Plug", width: 12.8, height: 7, round: 3.5},         // room for a plug's overmold, the socket behind the wall
	{name: "Micro-USB", width: 8, height: 3.1, round: 0.6},           // receptacle about 7.5 × 2.6
	{name: "USB-A", width: 13, height: 5.6, round: 0.5},              // receptacle shell 12.5 × 5.12
	{name: "HDMI", width: 15.7, height: 6.2, round: 0.3, outline: 1}, // the receptacle maker's panel cut-out
	{name: "Mini HDMI", width: 11.2, height: 3.1, round: 0.3, outline: 2},
	{name: "Micro HDMI", width: 6.8, height: 3.6, round: 0.3, outline: 3},
	{name: "DisplayPort", width: 17.1, height: 5.76, round: 0.8, outline: 4},
	{name: "Mini DisplayPort", width: 8.8, height: 5.9, round: 0.4, outline: 5},
	{name: "RJ45", width: 16.7, height: 14.4, round: 1},               // covers shielded and unshielded jacks' cut-outs
	{name: "XT30", width: 10.7, height: 5.7, round: 0.55, outline: 7}, // the plug (10.20 × 5.20) with 0.25 mm round it
	{name: "XT60", width: 16.5, height: 8.6, round: 0.65, outline: 6}, // 16.00 × 8.10 with 0.25 mm round it
	{name: "MT60", width: 12.5, height: 13, round: 2.25, outline: 8},
	{name: "MPX", width: 16.5, height: 6.9, round: 0.5}, // six-pole, 16 × 6.4 with 0.25 mm round it
	{name: "D-Sub 9", width: 19.74, height: 11.18, round: 0.5, outline: 9, holeSpacing: 24.99, holeDia: 3.18},
	{name: "Powerpole ×1", width: 8.9, height: 8.9, round: 0.5}, // 7.90 a pole and its tongues, with 0.25 mm round it
	{name: "Powerpole ×2", width: 16.8, height: 8.9, round: 0.5},
	{name: "Powerpole ×3", width: 24.7, height: 8.9, round: 0.5},
	{name: "Powerpole ×4", width: 32.6, height: 8.9, round: 0.5},
}

func portConnectorNames() []string {
	out := []string{"Custom"}
	for _, c := range portConnectors {
		out = append(out, c.name)
	}
	return out
}

// portOpening is the opening portModel builds for v: size, corner radius
// and outline, with the clearance added, and its screw holes.
func portOpening(v Values) (w, h, r float64, outline int, holeSpacing, holeDia float64) {
	w, h, r = v.Get("width", 9.4), v.Get("height", 3.7), v.Get("corner", 1.85)
	outline = int(v.Get("outline", 0))
	if i := int(v.Get("connector", 1)) - 1; i >= 0 && i < len(portConnectors) {
		c := portConnectors[i]
		w, h, r, outline, holeSpacing, holeDia = c.width, c.height, c.round, c.outline, c.holeSpacing, c.holeDia
	}
	c := v.Get("clearance", 0)
	w, h, r = w+2*c, h+2*c, r+c
	if holeDia > 0 {
		holeDia += 2 * c
	}
	return w, h, math.Min(r, portMaxCorner(w, h, outline)), outline, holeSpacing, holeDia
}

// portMaxCorner is the largest corner radius that fits: half the smaller
// side, less where a cut leaves a side shorter. A polygon's rounding is
// outside its corners, so it may take nearly half the smaller side.
func portMaxCorner(w, h float64, outline int) float64 {
	if _, ok := portPolygons[outline]; ok {
		return math.Max(0, math.Min(w, h)/2-0.01)
	}
	if cut, ok := portCuts[outline]; ok {
		return math.Max(0, math.Min((w-cut.x*w)/2, (h-cut.y*h)/2))
	}
	return math.Max(0, math.Min(w, h)/2)
}

// portOutline is the opening seen from the front, counter-clockwise, x
// across its middle, y from 0 up to h; sides is the segments a full
// circle's worth of rounding gets.
func portOutline(w, h, r float64, outline, sides int) []Pt {
	var out []Pt
	add := func(p Pt) {
		if n := len(out); n == 0 || math.Hypot(out[n-1].X-p.X, out[n-1].Y-p.Y) > 1e-9 {
			out = append(out, p) // where two arcs meet (a round end) the point is there already
		}
	}
	arc := func(cx, cy, a0, a1 float64, n int) {
		for i := 0; i <= n; i++ {
			a := a0 + (a1-a0)*float64(i)/float64(n)
			add(Pt{cx + r*math.Cos(a), cy + r*math.Sin(a)})
		}
	}
	if poly, ok := portPolygons[outline]; ok {
		ps := poly(math.Max(1e-6, w-2*r), math.Max(1e-6, h-2*r))
		for i, p := range ps {
			p[1] += r
			if r <= 1e-4 {
				add(Pt{p[0], p[1]})
				continue
			}
			// round the corner from the side before's outward normal to the side after's
			prev, next := ps[(i+len(ps)-1)%len(ps)], ps[(i+1)%len(ps)]
			a0 := math.Atan2(-(p[0] - prev[0]), p[1]-r-prev[1])
			a1 := math.Atan2(-(next[0] - p[0]), next[1]-(p[1]-r))
			for a1 <= a0+1e-12 {
				a1 += 2 * math.Pi
			}
			// as many segments as the turn's share of a full circle, the arc
			// broken where it faces straight along an axis, so its widest
			// and tallest points are corners and the opening is its full size
			n := max(1, int(math.Ceil(float64(sides)*(a1-a0)/(2*math.Pi))))
			from := a0
			for k := math.Floor(a0/(math.Pi/2)) + 1; k*math.Pi/2 < a1-1e-9; k++ {
				to := k * math.Pi / 2
				arc(p[0], p[1], from, to, max(1, int(math.Round(float64(n)*(to-from)/(a1-a0)))))
				from = to
			}
			arc(p[0], p[1], from, a1, max(1, int(math.Round(float64(n)*(a1-from)/(a1-a0)))))
		}
	} else {
		hw := w / 2
		q := max(2, (max(4, sides)+3)/4) // segments in a quarter circle
		corner := func(x, y, cx, cy, a0 float64) {
			if r <= 1e-4 {
				add(Pt{x, y})
			} else {
				arc(cx, cy, a0, a0+math.Pi/2, q)
			}
		}
		cut, isCut := portCuts[outline]
		cx, cy := cut.x*w, cut.y*h
		if isCut && !cut.leftOnly { // lower right
			add(Pt{hw - cx, 0})
			add(Pt{hw, cy})
		} else {
			corner(hw, 0, hw-r, r, -math.Pi/2)
		}
		corner(hw, h, hw-r, h-r, 0)
		corner(-hw, h, -hw+r, h-r, math.Pi/2)
		if isCut { // lower left
			add(Pt{-hw, cy})
			add(Pt{-hw + cx, 0})
		} else {
			corner(-hw, 0, -hw+r, r, math.Pi)
		}
	}
	if n := len(out); n > 1 && math.Hypot(out[0].X-out[n-1].X, out[0].Y-out[n-1].Y) <= 1e-9 {
		out = out[:n-1]
	}
	return out
}

// portModel is the opening, Depth long along Y, with its screw holes.
func portModel(v Values) *geom.Mesh {
	w, h, r, outline, spacing, dia := portOpening(v)
	sides := int(v.Get("sides", 32))
	d := v.Get("depth", 10)
	open := portOutline(w, h, r, outline, sides)
	cs := [][]Pt{open}
	var joined []*geom.Mesh
	if dia > 0 {
		for _, x := range []float64{-spacing / 2, spacing / 2} {
			c := circlePts(x, h/2, dia/2, max(12, sides))
			// a large clearance grows a screw hole into the opening: joined
			// to it, not two shapes overlapping
			if outlinesMeet(open, c) || PointInPolygon(c[0], open) {
				joined = append(joined, ExtrudeContours([][]Pt{c}, -d/2, d/2))
				continue
			}
			cs = append(cs, c)
		}
	}
	m := ExtrudeContours(cs, -d/2, d/2)
	for _, j := range joined {
		m = geom.Union(m, j).FixTJunctions()
	}
	return m.Transform(outlineToXZ)
}
