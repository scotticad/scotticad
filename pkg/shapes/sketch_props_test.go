package shapes

// Property tests of the sketch shapes (extrude, revolve, coil) against exact
// math, over many seeded random drawings and settings. Each built mesh is
// checked against an independent model of what it should be:
//
//   - the drawing's area (and first moment, for spun shapes) is integrated
//     exactly by vertical slabs (propSlab), with inside-ness from this file's
//     own winding rule, not the product's;
//   - the mesh's generalized winding number (windingAt) at sample points is
//     compared with the model: 1 inside, 0 outside, never 2 (covered twice);
//   - volumes are compared with closed-form formulas for the faceted solids
//     (prism, frustum, faceted Pappus) and with exact symmetries (scaling,
//     translation, mirroring, a coil's turns all congruent).

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// SKETCH_PROPS_TIMES=k runs k times the cases, SKETCH_PROPS_SEED=s moves
// every test to other seeds: a longer hunt than the default quick run.
func propN(n int) int {
	if k, err := strconv.Atoi(os.Getenv("SKETCH_PROPS_TIMES")); err == nil && k > 0 {
		return n * k
	}
	return n
}

func propSeed(base int64) int64 {
	s, _ := strconv.ParseInt(os.Getenv("SKETCH_PROPS_SEED"), 10, 64)
	return base + 1_000_003*s
}

// --- the model ---------------------------------------------------------------

// propWind is how many times the closed polygon c goes round q, by summing
// the angles its sides turn through as seen from q (independent of the
// crossing count windingNumber uses).
func propWind(q Pt, c []Pt) int {
	s := 0.0
	for i := range c {
		a, b := ptSub(c[i], q), ptSub(c[(i+1)%len(c)], q)
		s += math.Atan2(ptCross(a, b), ptDot(a, b))
	}
	return int(math.Round(s / (2 * math.Pi)))
}

// propSegDist is the distance from q to the nearest of segs.
func propSegDist(q Pt, segs [][2]Pt) float64 {
	best := math.Inf(1)
	for _, s := range segs {
		a, b := s[0], s[1]
		ab, aq := ptSub(b, a), ptSub(q, a)
		t := 0.0
		if l := ptDot(ab, ab); l > 0 {
			t = math.Max(0, math.Min(1, ptDot(aq, ab)/l))
		}
		best = math.Min(best, math.Hypot(aq.X-t*ab.X, aq.Y-t*ab.Y))
	}
	return best
}

// propSlab integrates the set {q : inside(q)}, whose boundary runs along
// segs, between x = xlo and x = xhi: its area ∫∫ dA and first moment
// ∫∫ x dA. It is exact (to rounding): the plane is cut into vertical slabs
// at every endpoint and every crossing of two segments, so inside a slab no
// two segments cross, the covered length L(x) at each x is linear (a sum of
// differences of the bounding segments' heights), and Simpson's rule
// (w/6)(f(a) + 4f(m) + f(b)) integrates L and x·L exactly. Which gaps between
// neighboring segments are covered is asked once, in the middle of the slab.
func propSlab(segs [][2]Pt, inside func(Pt) bool, xlo, xhi float64) (area, mx float64) {
	xs := []float64{xlo, xhi}
	for _, s := range segs {
		for _, p := range s {
			if p.X > xlo && p.X < xhi {
				xs = append(xs, p.X)
			}
		}
	}
	for i := range segs {
		a, b := segs[i][0], segs[i][1]
		for j := i + 1; j < len(segs); j++ {
			c, d := segs[j][0], segs[j][1]
			if math.Max(a.X, b.X) < math.Min(c.X, d.X) || math.Max(c.X, d.X) < math.Min(a.X, b.X) ||
				math.Max(a.Y, b.Y) < math.Min(c.Y, d.Y) || math.Max(c.Y, d.Y) < math.Min(a.Y, b.Y) {
				continue
			}
			r, s := ptSub(b, a), ptSub(d, c)
			den := ptCross(r, s)
			if den == 0 {
				continue // parallel: collinear overlaps meet at endpoints already listed
			}
			t := ptCross(ptSub(c, a), s) / den
			u := ptCross(ptSub(c, a), r) / den
			if t >= -1e-12 && t <= 1+1e-12 && u >= -1e-12 && u <= 1+1e-12 {
				if x := a.X + t*r.X; x > xlo && x < xhi {
					xs = append(xs, x)
				}
			}
		}
	}
	sort.Float64s(xs)
	yAt := func(s [2]Pt, x float64) float64 {
		return s[0].Y + (x-s[0].X)*(s[1].Y-s[0].Y)/(s[1].X-s[0].X)
	}
	var span [][2]Pt
	for k := 0; k+1 < len(xs); k++ {
		x0, x1 := xs[k], xs[k+1]
		if x1-x0 < 1e-11 {
			continue
		}
		xm := (x0 + x1) / 2
		span = span[:0]
		for _, s := range segs {
			if math.Min(s[0].X, s[1].X) <= x0+1e-12 && math.Max(s[0].X, s[1].X) >= x1-1e-12 && s[0].X != s[1].X {
				span = append(span, s)
			}
		}
		sort.Slice(span, func(i, j int) bool { return yAt(span[i], xm) < yAt(span[j], xm) })
		var la, lm, lb float64
		for i := 0; i+1 < len(span); i++ {
			lo, hi := span[i], span[i+1]
			ylo, yhi := yAt(lo, xm), yAt(hi, xm)
			if yhi-ylo < 1e-12 || !inside(Pt{xm, (ylo + yhi) / 2}) {
				continue
			}
			la += yAt(hi, x0) - yAt(lo, x0)
			lm += yhi - ylo
			lb += yAt(hi, x1) - yAt(lo, x1)
		}
		w := x1 - x0
		area += w * (la + 4*lm + lb) / 6
		mx += w * (x0*la + 4*xm*lm + x1*lb) / 6
	}
	return area, mx
}

// propModel is what a drawing of filled closed paths covers, by the spec:
// the union of the paths, each filled by the nonzero rule (inside where it
// winds round a point at all), its curves cut as the build cuts them.
type propModel struct {
	polys [][]Pt
	segs  [][2]Pt
}

func newPropModel(sk *Sketch, steps int) propModel {
	var m propModel
	for _, p := range sk.Paths {
		if p.IsLine() {
			panic("propModel models filled paths only")
		}
		c := p.flattenN(steps)
		if len(c) < 3 {
			continue
		}
		m.polys = append(m.polys, c)
		m.segs = append(m.segs, ringEdges(c)...)
	}
	return m
}

func (m propModel) inside(q Pt) bool {
	for _, c := range m.polys {
		if propWind(q, c) != 0 {
			return true
		}
	}
	return false
}

func (m propModel) bounds() (x0, y0, x1, y1 float64) {
	x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, c := range m.polys {
		for _, p := range c {
			x0, y0, x1, y1 = math.Min(x0, p.X), math.Min(y0, p.Y), math.Max(x1, p.X), math.Max(y1, p.Y)
		}
	}
	return
}

// filledBounds is the box of what the model fills: its paths' corners that
// touch something filled (a point a hair away in some direction inside).
// The box's extremes are always corners, but a corner can stand alone: the
// tip of a spike gone out and straight back along itself fills nothing.
func (m propModel) filledBounds() (x0, y0, x1, y1 float64) {
	x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	const eps = 1e-6
	for _, c := range m.polys {
		for k, p := range c {
			u1 := ptUnit(ptSub(c[(k+1)%len(c)], p))
			u2 := ptUnit(ptSub(c[(k+len(c)-1)%len(c)], p))
			// the bisectors, either side of each side (for a tip too sharp
			// for the 64 directions round), and all round
			dirs := []Pt{ptAdd(u1, u2), ptScale(ptAdd(u1, u2), -1)}
			for _, u := range []Pt{u1, u2} {
				for _, a := range []float64{-1e-3, 1e-3} {
					dirs = append(dirs, Pt{u.X*math.Cos(a) - u.Y*math.Sin(a), u.X*math.Sin(a) + u.Y*math.Cos(a)})
				}
			}
			for a := 0; a < 64; a++ {
				dirs = append(dirs, ptPolar(1, float64(a)*math.Pi/32))
			}
			for _, d := range dirs {
				l := math.Hypot(d.X, d.Y)
				if l < 1e-9 {
					continue
				}
				// (asked off the sides: on one, the angles' sum is ambiguous)
				if q := ptAdd(p, ptScale(d, eps/l)); propSegDist(q, m.segs) > eps*1e-6 && m.inside(q) {
					x0, y0, x1, y1 = math.Min(x0, p.X), math.Min(y0, p.Y), math.Max(x1, p.X), math.Max(y1, p.Y)
					break
				}
			}
		}
	}
	return
}

// area is the model's exact area and first moment ∫∫ x dA, left of xhi.
func (m propModel) measure(xhi float64) (area, mx float64) {
	x0, _, x1, _ := m.bounds()
	if x0 >= xhi {
		return 0, 0
	}
	return propSlab(m.segs, m.inside, x0, math.Min(x1, xhi))
}

// --- random drawings ---------------------------------------------------------

// propPath is a random closed path round (cx, cy), about size across, of one
// of several kinds: a star-shaped simple polygon, points anywhere (crossing
// itself), a grid-aligned rectangle (sides that coincide and touch), or a
// smooth curve through a star's points.
func propPath(r *rand.Rand, cx, cy, size float64, curves bool) SketchPath {
	snap := r.Intn(2) == 0 // on a quarter grid: coincidences
	at := func(x, y float64) SketchPoint {
		if snap {
			x, y = math.Round(x*4)/4, math.Round(y*4)/4
		}
		return SketchPoint{P: [2]float64{x, y}}
	}
	kinds := 3
	if curves {
		kinds = 5
	}
	p := SketchPath{Closed: true}
	switch k := r.Intn(kinds); k {
	case 0, 3: // star-shaped
		n := 3 + r.Intn(7)
		var angs []float64
		for i := 0; i < n; i++ {
			angs = append(angs, r.Float64()*2*math.Pi)
		}
		sort.Float64s(angs)
		for _, a := range angs {
			rad := size / 2 * (0.3 + 0.7*r.Float64())
			p.Points = append(p.Points, at(cx+rad*math.Cos(a), cy+rad*math.Sin(a)))
		}
		if k == 3 {
			for i := range p.Points {
				p.Points[i].Kind = PointAuto
			}
		}
	case 1: // anywhere: crossing itself
		for i := 0; i < 3+r.Intn(5); i++ {
			p.Points = append(p.Points, at(cx+(r.Float64()-0.5)*size, cy+(r.Float64()-0.5)*size))
		}
	case 2: // a rectangle on the whole-millimeter grid
		x0, y0 := math.Round(cx-size/2*r.Float64()), math.Round(cy-size/2*r.Float64())
		x1, y1 := x0+1+math.Round(size/2*r.Float64()), y0+1+math.Round(size/2*r.Float64())
		p.Points = skPts(x0, y0, x1, y0, x1, y1, x0, y1)
	case 4: // curves from random handles
		for i := 0; i < 2+r.Intn(4); i++ {
			q := at(cx+(r.Float64()-0.5)*size, cy+(r.Float64()-0.5)*size)
			q.Kind = PointSmooth
			h := [2]float64{(r.Float64() - 0.5) * size / 2, (r.Float64() - 0.5) * size / 2}
			q.Out, q.In = h, [2]float64{-h[0], -h[1]}
			p.Points = append(p.Points, q)
		}
	}
	if r.Intn(2) == 0 { // either way round
		for i, j := 0, len(p.Points)-1; i < j; i, j = i+1, j-1 {
			p.Points[i], p.Points[j] = p.Points[j], p.Points[i]
		}
		for i := range p.Points {
			p.Points[i].In, p.Points[i].Out = p.Points[i].Out, p.Points[i].In
		}
	}
	return p
}

// propDrawing is 1 to 4 random filled paths with their middles in the box
// x0…x1, y0…y1: apart, overlapping, one inside another.
func propDrawing(r *rand.Rand, x0, x1, y0, y1 float64, curves bool) *Sketch {
	sk := &Sketch{}
	for i := 0; i < 1+r.Intn(4); i++ {
		cx, cy := x0+r.Float64()*(x1-x0), y0+r.Float64()*(y1-y0)
		sk.Paths = append(sk.Paths, propPath(r, cx, cy, 6+r.Float64()*18, curves))
	}
	return sk
}

func propTransform(sk *Sketch, f func(x, y float64) (float64, float64), linear func(x, y float64) (float64, float64)) *Sketch {
	c := CopySketch(sk)
	for i := range c.Paths {
		for j := range c.Paths[i].Points {
			q := &c.Paths[i].Points[j]
			q.P[0], q.P[1] = f(q.P[0], q.P[1])
			q.In[0], q.In[1] = linear(q.In[0], q.In[1])
			q.Out[0], q.Out[1] = linear(q.Out[0], q.Out[1])
		}
	}
	return c
}

// --- mesh checks -------------------------------------------------------------

// propSolid checks a built mesh is a closed, finite, outward-facing solid
// and returns its volume (as built: not reoriented).
func propSolid(t *testing.T, name string, m *geom.Mesh) float64 {
	t.Helper()
	if m == nil || len(m.Tris) == 0 {
		t.Errorf("%s: empty", name)
		return 0
	}
	if o, nm := m.EdgeCheck(1e-7); o != 0 || nm != 0 {
		t.Errorf("%s: %d open, %d non-manifold edges", name, o, nm)
	}
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			if math.IsNaN(p.X+p.Y+p.Z) || math.IsInf(p.X+p.Y+p.Z, 0) {
				t.Errorf("%s: a point %v isn't finite", name, p)
				return 0
			}
		}
	}
	v := m.Volume()
	if !(v > 0) {
		t.Errorf("%s: volume %v (inside out, or flat)", name, v)
	}
	return v
}

// propWinder compares a mesh's winding number at points with what the
// model says, reporting at most a few disagreements a mesh.
type propWinder struct {
	t      *testing.T
	name   string
	m      *geom.Mesh
	bad    int
	tested int
}

func (w *propWinder) check(q geom.Vec3, want int, why string) {
	w.t.Helper()
	w.tested++
	got := windingAt(w.m, q)
	if math.Abs(got-float64(want)) < 0.25 {
		return
	}
	w.bad++
	if w.bad <= 3 {
		w.t.Errorf("%s: winding %.3f at %.4f,%.4f,%.4f, want %d (%s)", w.name, got, q.X, q.Y, q.Z, want, why)
	}
}

// propCapArea is the area of the mesh's triangles lying flat at height z,
// counted up for those facing up and down for those facing down.
func propCapArea(m *geom.Mesh, z float64) float64 {
	a := 0.0
	for _, tr := range m.Tris {
		if tr.P[0].Z == z && tr.P[1].Z == z && tr.P[2].Z == z {
			a += tr.P[1].Sub(tr.P[0]).Cross(tr.P[2].Sub(tr.P[0])).Z / 2
		}
	}
	return a
}

func propRel(a, b float64) float64 { return math.Abs(a-b) / math.Max(1, math.Abs(b)) }

// --- extrusion ---------------------------------------------------------------

// A straight extrusion of a random drawing (separate, overlapping, nested,
// self-crossing paths; straight and curved) is the drawing's exact area
// times its height; its outlines' signed areas add up to that area; its
// bounds are the drawing's; and at random points its winding number is the
// model's: 1 over the drawing, 0 off it, never 2.
func TestSketchPropExtrude(t *testing.T) {
	r := rand.New(rand.NewSource(propSeed(101)))
	const h = 7.5
	for i := 0; i < propN(80); i++ {
		sk := propDrawing(r, -15, 15, -15, 15, i%2 == 1)
		q := []int{0, 3, 12}[r.Intn(3)]
		name := fmt.Sprintf("case %d (quality %d) %+v", i, q, sk.Paths)
		mod := newPropModel(sk, q)
		area, _ := mod.measure(math.Inf(1))
		if area < 1e-3 {
			continue
		}
		outl := 0.0
		for _, c := range SketchOutlinesN(sk, q) {
			outl += SignedArea(c)
		}
		if propRel(outl, area) > 1e-7 {
			t.Errorf("%s: outlines cover %.6f mm², the drawing %.6f", name, outl, area)
		}
		m := SketchExtrude(sk, 0, h, Extrude{Quality: q, TopScale: 100, BaseScale: 100})
		v := propSolid(t, name, m)
		if propRel(v, area*h) > 1e-7 {
			t.Errorf("%s: volume %.6f, want area × height %.6f", name, v, area*h)
		}
		x0, y0, x1, y1 := mod.filledBounds()
		if b := m.Bounds(); math.Abs(b.Min.X-x0) > 1e-9 || math.Abs(b.Max.X-x1) > 1e-9 || math.Abs(b.Min.Y-y0) > 1e-9 ||
			math.Abs(b.Max.Y-y1) > 1e-9 || b.Min.Z != 0 || b.Max.Z != h {
			t.Errorf("%s: bounds %v, want %v,%v…%v,%v × 0…%v", name, b, x0, y0, x1, y1, h)
		}
		w := propWinder{t: t, name: name, m: m}
		for k := 0; k < 60; k++ {
			p := Pt{x0 - 1 + r.Float64()*(x1-x0+2), y0 - 1 + r.Float64()*(y1-y0+2)}
			if propSegDist(p, mod.segs) < 1e-4 {
				continue
			}
			want := 0
			if mod.inside(p) {
				want = 1
			}
			w.check(geom.V(p.X, p.Y, 0.01+r.Float64()*(h-0.02)), want, "over the drawing")
		}
	}
}

// Tapered: each height's section is the drawing scaled about its bounds'
// middle by s(t) = s0 + (s1 − s0)·t, so the volume is the prismatoid's
// ∫ A·s(t)² dz = h·A·(s0² + s0·s1 + s1²)/3, the caps are A·s0² and A·s1²,
// and the winding number at height t is the model's at the point scaled
// back by 1/s(t).
func TestSketchPropTaper(t *testing.T) {
	r := rand.New(rand.NewSource(propSeed(202)))
	const h = 10.0
	for i := 0; i < propN(50); i++ {
		sk := propDrawing(r, -12, 12, -12, 12, i%3 == 0)
		mod := newPropModel(sk, 6)
		area, _ := mod.measure(math.Inf(1))
		if area < 1e-3 {
			continue
		}
		e := Extrude{Quality: 6, BaseScale: float64(r.Intn(201)), TopScale: float64(r.Intn(201))}
		if i%7 == 0 {
			e.TopScale = 0 // to a point
		}
		if e.TopScale == e.BaseScale {
			e.TopScale++
		}
		s0, s1 := e.BaseScale/100, e.TopScale/100
		name := fmt.Sprintf("case %d %+v %+v", i, e, sk.Paths)
		m := SketchExtrude(sk, 0, h, e)
		v := propSolid(t, name, m)
		if want := h * area * (s0*s0 + s0*s1 + s1*s1) / 3; propRel(v, want) > 1e-7 {
			t.Errorf("%s: volume %.6f, want h·A·(s0²+s0·s1+s1²)/3 = %.6f", name, v, want)
		}
		if top, base := propCapArea(m, h), -propCapArea(m, 0); propRel(top, area*s1*s1) > 1e-7 || propRel(base, area*s0*s0) > 1e-7 {
			t.Errorf("%s: top %.6f and base %.6f, want %.6f and %.6f", name, top, base, area*s1*s1, area*s0*s0)
		}
		x0, y0, x1, y1 := mod.filledBounds()
		c := Pt{(x0 + x1) / 2, (y0 + y1) / 2}
		w := propWinder{t: t, name: name, m: m}
		for k := 0; k < 40; k++ {
			tt := 0.01 + 0.98*r.Float64()
			s := s0 + (s1-s0)*tt
			if s < 0.05 {
				continue
			}
			p := Pt{x0 - 1 + r.Float64()*(x1-x0+2), y0 - 1 + r.Float64()*(y1-y0+2)}
			if propSegDist(p, mod.segs)*s < 1e-4 {
				continue
			}
			want := 0
			if mod.inside(p) {
				want = 1
			}
			q := ptAdd(c, ptScale(ptSub(p, c), s))
			w.check(geom.V(q.X, q.Y, tt*h), want, "scaled section")
		}
	}
}

// Twisted: at each layer's height (Twist Steps + 1 layers, or Smooth's 96)
// the section is the drawing turned about its bounds' middle by that
// layer's share of the twist, so the winding number there is the model's at
// the point turned back; the top and base keep the drawing's area. (Between
// layers the sides are folded strips, so the volume is only near A·h.)
func TestSketchPropTwist(t *testing.T) {
	r := rand.New(rand.NewSource(propSeed(303)))
	const h = 10.0
	for i := 0; i < propN(40); i++ {
		sk := propDrawing(r, -10, 10, -10, 10, false)
		mod := newPropModel(sk, 12)
		area, _ := mod.measure(math.Inf(1))
		if area < 1e-3 {
			continue
		}
		e := Extrude{Quality: 12, BaseScale: 100, TopScale: 100, Twist: math.Round(r.Float64()*360 - 180), TwistSteps: r.Intn(6), Smooth: i%4 == 0}
		if e.Twist == 0 {
			e.Twist = 45
		}
		layers := e.TwistSteps + 1
		if e.Smooth {
			layers = smoothTwistLayers
		}
		name := fmt.Sprintf("case %d %+v %+v", i, e, sk.Paths)
		m := SketchExtrude(sk, 0, h, e)
		x0, y0, x1, y1 := mod.filledBounds()
		c := Pt{(x0 + x1) / 2, (y0 + y1) / 2}
		a := e.Twist / float64(layers) * math.Pi / 180 // the turn a layer
		watertight(t, name, m)
		v := m.Volume()
		if top, base := propCapArea(m, h), -propCapArea(m, 0); propRel(top, area) > 1e-7 || propRel(base, area) > 1e-7 {
			t.Errorf("%s: top %.6f and base %.6f, want both %.6f", name, top, base, area)
		}
		// A layer's side strip from P->Q at the bottom to RP->RQ at the top (R
		// the layer's turn) is folded from Q at the bottom to RP at the top:
		// triangles (P, Q, RP) and (Q, RQ, RP). At a fraction s up the layer
		// they cut the plane in (1−s)P + s·RP, (1−s)Q + s·RP, (1−s)Q + s·RQ, so
		// each section is the polygon of the points (1−s)P_i + s·RP_i with
		// (1−s)P_{i+1} + s·RP_i between each two: its area is quadratic in s,
		// and Simpson's rule gives the layer's volume exactly, (h/L)·(A +
		// 4·A(½) + A)/6. The layers are congruent, so V = h·(2A + 4·A(½))/6.
		// (Smooth cuts each side into smoothTwistCuts pieces first.)
		cuts := 1
		if e.Smooth {
			cuts = smoothTwistCuts
		}
		half := 0.0
		for _, ring := range SketchOutlinesN(sk, e.Quality) {
			var pts []Pt
			for k := range ring {
				p, q := ring[k], ring[(k+1)%len(ring)]
				for cut := 0; cut < cuts; cut++ {
					f := float64(cut) / float64(cuts)
					pts = append(pts, Pt{p.X + (q.X-p.X)*f, p.Y + (q.Y-p.Y)*f})
				}
			}
			turn := func(p Pt) Pt {
				d := ptSub(p, c)
				return Pt{c.X + d.X*math.Cos(a) - d.Y*math.Sin(a), c.Y + d.X*math.Sin(a) + d.Y*math.Cos(a)}
			}
			var sec []Pt
			for k := range pts {
				p, q := pts[k], pts[(k+1)%len(pts)]
				rp := turn(p)
				sec = append(sec, ptScale(ptAdd(p, rp), 0.5), ptScale(ptAdd(q, rp), 0.5))
			}
			half += SignedArea(sec)
		}
		// (A turn a layer large for the drawing's size, or a thin sliver far
		// from the middle, folds the strips through each other, as the
		// reference does: still closed, but A(½), and the volume, can then
		// come out small or negative. Only the formula is checked.)
		if want := h * (2*area + 4*half) / 6; propRel(v, want) > 1e-7 {
			t.Errorf("%s: volume %.6f, want h·(2A + 4·A(½))/6 = %.6f (A·h %.6f)", name, v, want, area*h)
		}
		w := propWinder{t: t, name: name, m: m}
		for k := 0; k < 40 && layers > 1; k++ {
			layer := 1 + r.Intn(layers-1)
			tt := float64(layer) / float64(layers)
			ang := e.Twist * tt * math.Pi / 180
			p := Pt{x0 - 1 + r.Float64()*(x1-x0+2), y0 - 1 + r.Float64()*(y1-y0+2)}
			if propSegDist(p, mod.segs) < 1e-4 {
				continue
			}
			want := 0
			if mod.inside(p) {
				want = 1
			}
			d := ptSub(p, c)
			q := Pt{c.X + d.X*math.Cos(ang) - d.Y*math.Sin(ang), c.Y + d.X*math.Sin(ang) + d.Y*math.Cos(ang)}
			w.check(geom.V(q.X, q.Y, tt*h), want, fmt.Sprintf("layer %d of %d", layer, layers))
		}
	}
}

// --- strokes -----------------------------------------------------------------

// A closed rectangle W × H drawn with a line w wide, turned and moved
// anywhere, has an exact area for every alignment and corner. The line's
// outer edge is offset d from the path (w/2 centered, w outside, 0 inside)
// and its inner edge e (w/2, 0, w):
//
//	outer: sharp (W+2d)(H+2d); bevel the same less 4·(d(√2−1))² (a square
//	join, cut a full d from the corner); round the same less 4d² − πd², the
//	arcs being polygons inscribed in the circle with at most 2π/32 a side,
//	so between πd²·cos²(π/32) and πd²;
//	inner: (W−2e)(H−2e), or nothing when the line is wider than the shape
//	(the shape is then solid, no false hole).
func TestSketchPropStrokeRectangles(t *testing.T) {
	r := rand.New(rand.NewSource(propSeed(404)))
	for i := 0; i < propN(120); i++ {
		W, H := 2+r.Float64()*20, 2+r.Float64()*20
		w := 0.3 + r.Float64()*6
		if i%4 == 0 {
			w = math.Max(W, H) * (0.6 + r.Float64()) // often wider than the shape
		}
		align, join := r.Intn(3), r.Intn(3)
		d, e := w/2, w/2
		switch align {
		case AlignInside:
			d, e = 0, w
		case AlignOutside:
			d, e = w, 0
		}
		if math.Abs(W-2*e) < 0.05 || math.Abs(H-2*e) < 0.05 {
			continue // the inside edge just closing up: either answer is near
		}
		ang, cx, cy := r.Float64()*2*math.Pi, r.Float64()*20-10, r.Float64()*20-10
		var pts []SketchPoint
		for _, c := range [][2]float64{{-W / 2, -H / 2}, {W / 2, -H / 2}, {W / 2, H / 2}, {-W / 2, H / 2}} {
			pts = append(pts, SketchPoint{P: [2]float64{cx + c[0]*math.Cos(ang) - c[1]*math.Sin(ang), cy + c[0]*math.Sin(ang) + c[1]*math.Cos(ang)}})
		}
		path := SketchPath{Closed: true, Points: pts, Stroke: w, Align: align, Join: join}
		sk := &Sketch{Paths: []SketchPath{path}}
		outer := (W + 2*d) * (H + 2*d)
		tol := 1e-7 * outer
		switch join {
		case JoinBevel:
			outer -= 4 * math.Pow(d*(math.Sqrt2-1), 2)
		case JoinRound:
			outer -= 4*d*d - math.Pi*d*d*(1+math.Pow(math.Cos(math.Pi/32), 2))/2
			tol += math.Pi * d * d * (1 - math.Pow(math.Cos(math.Pi/32), 2)) / 2
		}
		inner := 0.0
		if W > 2*e && H > 2*e {
			inner = (W - 2*e) * (H - 2*e)
		}
		if align == AlignOutside {
			inner = W * H
		}
		want := outer - inner
		name := fmt.Sprintf("case %d: %.3f × %.3f, line %.3f, align %d, join %d", i, W, H, w, align, join)
		got := outlineArea(sk)
		if math.Abs(got-want) > tol {
			t.Errorf("%s: area %.6f, want %.6f ± %.2g", name, got, want, tol)
		}
		m := SketchSolid(sk, 0, 2)
		if v := propSolid(t, name, m); math.Abs(v-2*got) > 1e-7*v {
			t.Errorf("%s: volume %.6f, want 2 × its outlines' area %.6f", name, v, 2*got)
		}
		// sharp corners: the line is exactly the outer rectangle less the inner
		if join != JoinSharp {
			continue
		}
		pw := propWinder{t: t, name: name, m: m}
		for k := 0; k < 40; k++ {
			lx, ly := (r.Float64()-0.5)*(W+2*d+2), (r.Float64()-0.5)*(H+2*d+2)
			ax, ay := math.Abs(lx), math.Abs(ly)
			in := ax < W/2+d && ay < H/2+d && !(ax < W/2-e && ay < H/2-e)
			near := func(a, b float64) bool { return math.Abs(a-b) < 1e-4 }
			if near(ax, W/2+d) || near(ay, H/2+d) || near(ax, W/2-e) || near(ay, H/2-e) {
				continue
			}
			want := 0
			if in {
				want = 1
			}
			x, y := cx+lx*math.Cos(ang)-ly*math.Sin(ang), cy+lx*math.Sin(ang)+ly*math.Cos(ang)
			pw.check(geom.V(x, y, 1), want, "frame")
		}
	}
}

// An open straight line L long and w wide has an exact area for each end:
// flat L·w, square (L+w)·w, round L·w plus a polygon circle of radius w/2
// inscribed in the circle (between π(w/2)²·cos²(π/32) and π(w/2)²).
func TestSketchPropOpenLineCaps(t *testing.T) {
	r := rand.New(rand.NewSource(propSeed(505)))
	for i := 0; i < propN(60); i++ {
		L, w := 0.5+r.Float64()*30, 0.2+r.Float64()*8
		ang, cx, cy := r.Float64()*2*math.Pi, r.Float64()*20-10, r.Float64()*20-10
		dx, dy := L/2*math.Cos(ang), L/2*math.Sin(ang)
		capKind := r.Intn(3)
		path := SketchPath{Points: skPts(cx-dx, cy-dy, cx+dx, cy+dy), Stroke: w, Cap: capKind, Join: r.Intn(3)}
		want, tol := L*w, 1e-7*L*w
		rr := w / 2
		switch capKind {
		case CapSquare:
			want += w * w
		case CapRound:
			c2 := math.Pow(math.Cos(math.Pi/32), 2)
			want += math.Pi * rr * rr * (1 + c2) / 2
			tol += math.Pi * rr * rr * (1 - c2) / 2
		}
		sk := &Sketch{Paths: []SketchPath{path}}
		name := fmt.Sprintf("case %d: %.3f long, %.3f wide, cap %d", i, L, w, capKind)
		got := outlineArea(sk)
		if math.Abs(got-want) > tol+1e-7*want {
			t.Errorf("%s: area %.6f, want %.6f ± %.2g", name, got, want, tol)
		}
		if v := propSolid(t, name, SketchSolid(sk, 0, 3)); math.Abs(v-3*got) > 1e-7*v {
			t.Errorf("%s: volume %.6f, want 3 × %.6f", name, v, got)
		}
	}
}

// Random lines (open and closed, every corner, round or square ends, wider
// than their bends included), checked point by point through the mesh: a
// line covers every point nearer its center line than its half-width h
// (less the inscribed arcs' shortfall, h·cos(π/32), for round corners and
// ends), and nothing farther than h for round corners and ends, or than
// miterLimit·h for sharp and bevelled ones (a miter reaches at most that
// far, a bevel less). Its outlines' area is the exact area of the set the
// build itself says is covered (sketchArea's own inside test, integrated by
// slabs): a check on how the outlines are joined up and triangulated.
func TestSketchPropStrokes(t *testing.T) {
	r := rand.New(rand.NewSource(propSeed(606)))
	for i := 0; i < propN(50); i++ {
		path := SketchPath{Closed: r.Intn(2) == 0, Stroke: 0.5 + r.Float64()*6, Join: r.Intn(3), Cap: CapRound + r.Intn(2), Align: AlignCenter}
		for k := 0; k < 2+r.Intn(3); k++ {
			path.Points = append(path.Points, SketchPoint{P: [2]float64{math.Round(r.Float64()*80) / 4, math.Round(r.Float64()*80) / 4}})
		}
		if path.Closed && len(path.Points) < 3 {
			path.Closed = false
		}
		sk := &Sketch{Paths: []SketchPath{path}}
		// and sometimes a filled area under it, to join
		fill := propModel{}
		if i%3 == 0 {
			fp := propPath(r, 10, 10, 14, false)
			sk.Paths = append(sk.Paths, fp)
			fill = newPropModel(&Sketch{Paths: []SketchPath{fp}}, 0)
		}
		name := fmt.Sprintf("case %d %+v", i, sk.Paths)
		cl := path.Flatten()
		var cline [][2]Pt
		for k := 0; k+1 < len(cl); k++ {
			cline = append(cline, [2]Pt{cl[k], cl[k+1]})
		}
		if path.Closed {
			cline = append(cline, [2]Pt{cl[len(cl)-1], cl[0]})
		}
		if len(cline) == 0 {
			continue
		}
		h := path.Width() / 2
		inscribed := h * math.Cos(math.Pi/32)
		// surely covered: within a segment's rectangle (the foot of the
		// perpendicular on the segment), within the inscribed radius of a
		// round corner or end, within a square end's square
		covered := func(p Pt) bool {
			for k, s := range cline {
				ab, ap := ptSub(s[1], s[0]), ptSub(p, s[0])
				l := math.Sqrt(ptDot(ab, ab))
				if l == 0 {
					continue
				}
				along, off := ptDot(ap, ab)/l, math.Abs(ptCross(ab, ap))/l
				lo, hi := 0.0, l
				if !path.Closed && path.Cap == CapSquare {
					if k == 0 {
						lo = -h
					}
					if k == len(cline)-1 {
						hi = l + h
					}
				}
				if along > lo+1e-3 && along < hi-1e-3 && off < h-1e-3 {
					return true
				}
			}
			ends := path.Join == JoinRound
			for k, q := range cl {
				end := !path.Closed && (k == 0 || k == len(cl)-1)
				if (ends && !end) || (end && path.Cap == CapRound) {
					if math.Hypot(p.X-q.X, p.Y-q.Y) < inscribed-1e-3 {
						return true
					}
				}
			}
			return false
		}
		// the farthest any of it reaches: a miter miterLimit·h, a bevel (cut
		// a full h out along the bisector, its ends on the edges' offsets)
		// h/cos(a quarter of the turn) ≤ √2·h, a square end's corners √2·h
		hi := h
		switch path.Join {
		case JoinSharp:
			hi = miterLimit * h
		case JoinBevel:
			hi = math.Sqrt2 * h
		}
		if !path.Closed && path.Cap == CapSquare {
			hi = math.Max(hi, math.Sqrt2*h)
		}
		ar := sketchArea(sk, 0)
		bx0, by0, bx1, by1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, e := range ar.edges {
			for _, p := range e {
				bx0, by0, bx1, by1 = math.Min(bx0, p.X), math.Min(by0, p.Y), math.Max(bx1, p.X), math.Max(by1, p.Y)
			}
		}
		exact, _ := propSlab(ar.edges, ar.inside, bx0, bx1)
		got := outlineArea(sk)
		if propRel(got, exact) > 1e-7 {
			t.Errorf("%s: outlines cover %.6f mm², the line %.6f", name, got, exact)
		}
		m := SketchSolid(sk, 0, 2)
		if v := propSolid(t, name, m); propRel(v, 2*exact) > 1e-7 {
			t.Errorf("%s: volume %.6f, want 2 × %.6f", name, v, exact)
		}
		pw := propWinder{t: t, name: name, m: m}
		for k := 0; k < 80; k++ {
			p := Pt{bx0 - 1 + r.Float64()*(bx1-bx0+2), by0 - 1 + r.Float64()*(by1-by0+2)}
			dist := propSegDist(p, cline)
			inFill := false
			if fill.polys != nil {
				if propSegDist(p, fill.segs) < 1e-4 {
					continue
				}
				inFill = fill.inside(p)
			}
			switch {
			case covered(p) || inFill:
				pw.check(geom.V(p.X, p.Y, 1), 1, fmt.Sprintf("%.3f from the center line", dist))
			case dist > hi+1e-3:
				pw.check(geom.V(p.X, p.Y, 1), 0, fmt.Sprintf("%.3f from the center line", dist))
			}
		}
	}
}

// --- revolve -----------------------------------------------------------------

// A revolve in N steps through a sweep of N·Δ is the profile (the drawing
// left of x = 0, radius r = −x) spun in planar trapezoid strips: in the
// half-plane at angle α from the middle of a step, the section is the
// profile with its radii scaled by k(α) = cos(Δ/2)/cos(α). Its volume is
// then exactly Σ_steps ∫ k² dα · ∫∫ r dA = N·sin(Δ)·M, M = ∫∫ r dA (the
// faceted Pappus: N·sin Δ -> the sweep in radians as N grows, giving
// A·x̄·sweep), and at any point the winding number is the profile's at the
// radius scaled back. Profiles crossing the axis are clipped there; the
// start angle turns the solid without changing its volume.
func TestSketchPropRevolve(t *testing.T) {
	r := rand.New(rand.NewSource(propSeed(707)))
	for i := 0; i < propN(50); i++ {
		sk := propDrawing(r, -20, 4, -10, 10, i%3 == 0)
		quality := 6
		mod := newPropModel(sk, quality)
		area, mx := mod.measure(0)
		M := -mx
		if area < 1e-2 || M < 1e-2 {
			continue
		}
		sides := 3 + r.Intn(62)
		sweep := 360.0
		if i%2 == 1 {
			sweep = 1 + math.Round(r.Float64()*358)
		}
		start := math.Round(r.Float64() * 360)
		regs := revolveProfile(sk, quality)
		n := spinSteps(regs, sides, 3)
		dRad := sweep / float64(n) * math.Pi / 180
		name := fmt.Sprintf("case %d (sides %d, sweep %v, start %v) %+v", i, sides, sweep, start, sk.Paths)
		m := revolveMesh(sk, start, sweep, sides, quality)
		v := propSolid(t, name, m)
		if want := float64(n) * math.Sin(dRad) * M; propRel(v, want) > 1e-7 {
			t.Errorf("%s: volume %.6f, want N·sin(Δ)·∫∫r dA = %.6f (Pappus %.6f)", name, v, want, sweep*math.Pi/180*M)
		}
		// the start only turns it: the same volume, and the bounds of the
		// start-0 solid's corners turned through the start
		m0 := revolveMesh(sk, 0, sweep, sides, quality)
		if v0 := m0.Volume(); propRel(v, v0) > 1e-9 {
			t.Errorf("%s: volume %.9f, at start 0 %.9f", name, v, v0)
		}
		cs, sn := math.Cos(start*math.Pi/180), math.Sin(start*math.Pi/180)
		rb := geom.EmptyBox()
		for _, tr := range m0.Tris {
			for _, p := range tr.P {
				rb = rb.Extend(geom.V(p.X*cs-p.Y*sn, p.X*sn+p.Y*cs, p.Z))
			}
		}
		if b := m.Bounds(); b.Min.Sub(rb.Min).Len() > 1e-9 || b.Max.Sub(rb.Max).Len() > 1e-9 {
			t.Errorf("%s: bounds %v, want start 0's turned %v", name, b, rb)
		}
		x0, y0, _, y1 := mod.bounds()
		rmax := -x0
		w := propWinder{t: t, name: name, m: m}
		for k := 0; k < 60; k++ {
			rho := r.Float64() * (rmax + 1)
			z := y0 - 1 + r.Float64()*(y1-y0+2)
			phi := r.Float64() * 360
			u := math.Mod(phi-(90+start)+720, 360) // degrees into the sweep
			q := geom.V(rho*math.Cos(phi*math.Pi/180), rho*math.Sin(phi*math.Pi/180), z)
			if rho < 1e-3 {
				continue
			}
			if sweep < 360 && u > sweep {
				// beyond the ends: skip points near the end caps' planes
				if off := math.Min(u-sweep, 360-u); rho*math.Sin(math.Min(off, 90)*math.Pi/180) < 1e-4 {
					continue
				}
				w.check(q, 0, "outside the sweep")
				continue
			}
			if sweep < 360 && rho*math.Sin(math.Min(math.Min(u, sweep-u), 90)*math.Pi/180) < 1e-4 {
				continue
			}
			dDeg := sweep / float64(n)
			j := math.Min(math.Floor(u/dDeg), float64(n-1))
			alpha := (u - (j+0.5)*dDeg) * math.Pi / 180
			kf := math.Cos(dRad/2) / math.Cos(alpha)
			p := Pt{-rho / kf, z}
			if propSegDist(p, mod.segs) < 1e-4 || p.X > -1e-4 {
				continue
			}
			want := 0
			if mod.inside(p) {
				want = 1
			}
			w.check(q, want, fmt.Sprintf("radius scaled by %.5f", kf))
		}
	}
}

// --- coil --------------------------------------------------------------------

// A coil's steps are the same angles on every turn, so its turns are
// congruent (each the last turned 360° and raised a pitch): n whole turns
// are exactly n times one. In the half-plane at any step's angle the coil's
// section is exactly the profile (the drawing left of x = −coilAxisGap)
// once for each turn through it, raised by the pitch times the turns so
// far, so the winding number there is the count of those profiles round
// the point: never 2, even at the least pitch. Its height is the profile's
// plus pitch × turns, the pitch raised to CoilMinPitch(the profile's
// height). Its volume is exactly linear in the pitch, and at pitch 0 is the
// faceted revolve's, step by step (see below).
func TestSketchPropCoil(t *testing.T) {
	r := rand.New(rand.NewSource(propSeed(808)))
	for i := 0; i < propN(40); i++ {
		sk := propDrawing(r, -18, 2, -6, 6, i%3 == 0)
		quality := 6
		mod := newPropModel(sk, quality)
		area, mx := mod.measure(-coilAxisGap)
		M := -mx
		if area < 1e-2 {
			continue
		}
		regs := coilProfile(sk, quality)
		if len(regs) == 0 {
			t.Errorf("case %d: no profile for %.4f mm² left of the axis", i, area)
			continue
		}
		py0, py1 := math.Inf(1), math.Inf(-1)
		for _, rg := range regs {
			for _, p := range rg.Outer {
				py0, py1 = math.Min(py0, p.Y), math.Max(py1, p.Y)
			}
		}
		sides := 3 + r.Intn(30)
		pitch := 0.1 // raised to the least
		if i%2 == 1 {
			pitch = (py1 - py0) * (1.1 + r.Float64())
		}
		turns := float64(1 + r.Intn(3))
		if i%4 == 3 {
			turns += 0.25 * float64(1+r.Intn(3))
		}
		start := math.Round(r.Float64() * 360)
		pitchUsed := math.Max(pitch, CoilMinPitch(py1-py0))
		name := fmt.Sprintf("case %d (turns %v, pitch %.3f, sides %d, start %v) %+v", i, turns, pitchUsed, sides, start, sk.Paths)
		m := coilMesh(sk, turns, pitch, start, sides, quality)
		v := propSolid(t, name, m)
		if b := m.Bounds(); math.Abs(b.Min.Z-py0) > 1e-9 || math.Abs(b.Max.Z-(py1+pitchUsed*turns)) > 1e-9 {
			t.Errorf("%s: height %v…%v, want %v…%v", name, b.Min.Z, b.Max.Z, py0, py1+pitchUsed*turns)
		}
		// the steps a turn, as coilMesh works them out (no budget at these sizes)
		steps := spinSteps(regs, max(1, int(math.Ceil(float64(max(3, sides))*turns-1e-9))), int(math.Ceil(3*turns)))
		perTurn := max(3, int(math.Floor(float64(steps)/turns+1e-9)))
		if turns == math.Floor(turns) {
			one := coilMesh(sk, 1, pitch, start, sides, quality)
			if v1 := one.Volume(); propRel(v, turns*v1) > 1e-9 {
				t.Errorf("%s: volume %.9f, want %v × one turn's %.9f", name, v, turns, v1)
			}
		}
		// The volume Σ P0·(P1 × P2)/6 is linear in each point's z, and only
		// z depends on the pitch (z = y + pitch·t), so the volume is exactly
		// linear in the pitch: V(p) = V(0) + c·p. At pitch 0 every wall quad
		// is a revolve's planar trapezoid and the caps lie in planes through
		// the axis (no volume), so V(0) = M·Σ sin(Δ_j) over the steps
		// (the faceted Pappus, per step). The steps: perTurn a turn at the
		// same angles on every turn, plus a partial turn's end angle on every
		// turn (as coilMesh lays them out).
		stol := 1e-3 / float64(perTurn)
		var round []float64
		for k := 0; k < perTurn; k++ {
			round = append(round, float64(k)/float64(perTurn))
		}
		if f := turns - math.Floor(turns); f > stol && f < 1-stol {
			k := int(f * float64(perTurn))
			if lo, hi := float64(k)/float64(perTurn), float64(k+1)/float64(perTurn); f-lo > stol && hi-f > stol {
				round = append(round[:k+1], append([]float64{f}, round[k+1:]...)...)
			}
		}
		var ts []float64
		for n := 0; float64(n) < turns-stol; n++ {
			for _, f := range round {
				if tt := float64(n) + f; tt < turns-stol {
					ts = append(ts, tt)
				}
			}
		}
		ts = append(ts, turns)
		sumSin := 0.0
		for j := 0; j+1 < len(ts); j++ {
			sumSin += math.Sin(2 * math.Pi * (ts[j+1] - ts[j]))
		}
		v2 := coilMesh(sk, turns, 2*pitchUsed, start, sides, quality).Volume()
		v3 := coilMesh(sk, turns, 3*pitchUsed, start, sides, quality).Volume()
		if math.Abs((v3-v2)-(v2-v)) > 1e-7*v {
			t.Errorf("%s: volume not linear in the pitch: %.9f, %.9f, %.9f at 1, 2, 3 × %.4f", name, v, v2, v3, pitchUsed)
		}
		if v0 := 2*v - v2; propRel(v0, M*sumSin) > 1e-7 {
			t.Errorf("%s: volume at pitch 0 (by the line through pitches %.4f and twice it) %.6f, want M·Σ sin Δ = %.6f", name, pitchUsed, v0, M*sumSin)
		}
		if testing.Verbose() {
			t.Logf("case %d: %d steps a turn; the pitch adds %+.3f%% to the pitch-0 volume (%+.3f%% to Pappus × turns)", i, perTurn,
				100*(v-M*sumSin)/(M*sumSin), 100*(v-2*math.Pi*turns*M)/(2*math.Pi*turns*M))
		}
		// the step half-planes
		w := propWinder{t: t, name: name, m: m}
		tol := 1e-3 / float64(perTurn)
		rmax := -mod.polys[0][0].X
		for _, c := range mod.polys {
			for _, p := range c {
				rmax = math.Max(rmax, -p.X)
			}
		}
		for k := 0; k < 80; k++ {
			step := r.Intn(perTurn)
			f := float64(step) / float64(perTurn)
			rho := coilAxisGap + r.Float64()*(rmax+1)
			z := py0 - 1 + r.Float64()*(py1-py0+pitchUsed*turns+2)
			a := (90 + start + 360*f) * math.Pi / 180
			q := geom.V(rho*math.Cos(a), rho*math.Sin(a), z)
			want, skip := 0, false
			for n := 0; float64(n)+f <= turns+tol; n++ {
				tt := float64(n) + f
				p := Pt{-rho, z - pitchUsed*tt}
				if propSegDist(p, mod.segs) < 1e-4 || math.Abs(p.X+coilAxisGap) < 1e-4 {
					skip = true
					break
				}
				if p.X < -coilAxisGap && mod.inside(p) {
					if tt < tol || math.Abs(tt-turns) < tol {
						skip = true // on an end cap
						break
					}
					want++
				}
			}
			if !skip {
				w.check(q, want, fmt.Sprintf("step %d of %d", step, perTurn))
			}
		}
		// anywhere: never inside twice
		for k := 0; k < 60; k++ {
			rho := r.Float64() * (rmax + 1)
			a := r.Float64() * 2 * math.Pi
			q := geom.V(rho*math.Cos(a), rho*math.Sin(a), py0-1+r.Float64()*(py1-py0+pitchUsed*turns+2))
			if wn := windingAt(m, q); wn > 1.5 || wn < -0.5 {
				t.Errorf("%s: winding %.3f at %v: covered twice (or inside out)", name, wn, q)
				break
			}
		}
	}
}

func propRegionsBounds(regs []Region) (x0, y0, x1, y1 float64) {
	x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, rg := range regs {
		for _, p := range rg.Outer {
			x0, y0, x1, y1 = math.Min(x0, p.X), math.Min(y0, p.Y), math.Max(x1, p.X), math.Max(y1, p.Y)
		}
	}
	return
}

// --- invariances -------------------------------------------------------------

// Moving a drawing doesn't change its extrusion's volume; scaling it by k
// scales an extrusion's volume by k² (the height is fixed) and a revolve's
// and a coil's (pitch scaled too) by k³; mirroring keeps every volume; so
// does drawing each path the other way round. All exactly, to rounding.
func TestSketchPropInvariance(t *testing.T) {
	r := rand.New(rand.NewSource(propSeed(909)))
	id := func(x, y float64) (float64, float64) { return x, y }
	for i := 0; i < propN(40); i++ {
		sk := propDrawing(r, -18, -6, -8, 8, i%2 == 0)
		quality := 8
		if _, _, x1, _ := newPropModel(sk, quality).bounds(); x1 > -3 {
			// moved left of x = −3 (curves bulge past their points a little)
			sh := -3 - x1 - 1
			sk = propTransform(sk, func(x, y float64) (float64, float64) { return x + sh, y }, id)
		}
		ext := func(s *Sketch) float64 {
			return SketchExtrude(s, 0, 5, Extrude{Quality: quality, TopScale: 100, BaseScale: 100}).Volume()
		}
		rev := func(s *Sketch) float64 {
			m := revolveMesh(s, 30, 360, 24, quality)
			if m == nil {
				return 0
			}
			return m.Volume()
		}
		base := ext(sk)
		if base <= 0 {
			continue
		}
		name := fmt.Sprintf("case %d %+v", i, sk.Paths)
		dx, dy := r.Float64()*40-20, r.Float64()*40-20
		if v := ext(propTransform(sk, func(x, y float64) (float64, float64) { return x + dx, y + dy }, id)); propRel(v, base) > 1e-9 {
			t.Errorf("%s: moved by %.4f,%.4f: volume %.9f, was %.9f", name, dx, dy, v, base)
		}
		k := 0.37 + r.Float64()*2.5
		scale := func(x, y float64) (float64, float64) { return x * k, y * k }
		sc := propTransform(sk, scale, scale)
		if v := ext(sc); propRel(v, base*k*k) > 1e-9 {
			t.Errorf("%s: scaled by %.4f: volume %.9f, want k² × %.9f = %.9f", name, k, v, base, base*k*k)
		}
		flipX := func(x, y float64) (float64, float64) { return -x, y }
		if v := ext(propTransform(sk, flipX, flipX)); propRel(v, base) > 1e-9 {
			t.Errorf("%s: mirrored: volume %.9f, was %.9f", name, v, base)
		}
		rv := propTransform(sk, id, id)
		for j := range rv.Paths {
			p := rv.Paths[j].Points
			for a, b := 0, len(p)-1; a < b; a, b = a+1, b-1 {
				p[a], p[b] = p[b], p[a]
			}
			for a := range p {
				p[a].In, p[a].Out = p[a].Out, p[a].In
			}
		}
		if v := ext(rv); propRel(v, base) > 1e-9 {
			t.Errorf("%s: drawn the other way round: volume %.9f, was %.9f", name, v, base)
		}
		// spun: the drawing is left of x = −1 (k ≥ 0.37 keeps it clear of
		// the coil's axis gap), so nothing is clipped
		rb := rev(sk)
		if v := rev(sc); propRel(v, rb*k*k*k) > 1e-9 {
			t.Errorf("%s: revolve scaled by %.4f: volume %.9f, want k³ × %.9f = %.9f", name, k, v, rb, rb*k*k*k)
		}
		flipY := func(x, y float64) (float64, float64) { return x, -y }
		if v := rev(propTransform(sk, flipY, flipY)); propRel(v, rb) > 1e-9 {
			t.Errorf("%s: revolve mirrored: volume %.9f, was %.9f", name, v, rb)
		}
		_, py0, _, py1 := propRegionsBounds(coilProfile(sk, quality))
		pitch := 1.5*(py1-py0) + 1
		cb := coilMesh(sk, 2, pitch, 0, 16, quality).Volume()
		if v := coilMesh(sc, 2, pitch*k, 0, 16, quality).Volume(); propRel(v, cb*k*k*k) > 1e-9 {
			t.Errorf("%s: coil scaled by %.4f: volume %.9f, want k³ × %.9f = %.9f", name, k, v, cb, cb*k*k*k)
		}
		if v := coilMesh(sk, 2, pitch, 77, 16, quality).Volume(); propRel(v, cb) > 1e-9 {
			t.Errorf("%s: coil started at 77°: volume %.9f, at 0° %.9f", name, v, cb)
		}
		// (a coil of the profile mirrored top to bottom is the coil's mirror
		// image, but its wall quads fold along the other diagonals, so its
		// volume differs: not compared)
	}
}

// The Quality of a curved drawing converges on the curve: a smooth closed
// curve's area is ½∮(x dy − y dx), exact for cubic Béziers by 3-point
// Gauss-Legendre (the integrand is a degree-5 polynomial), and the
// flattened outline, its corners on the curve at even steps of each
// curve's parameter, falls short by a sum of chord segments that shrinks as
// 1/steps²: doubling Quality divides the error by about 4.
func TestSketchPropQualityConverges(t *testing.T) {
	r := rand.New(rand.NewSource(propSeed(1010)))
	gx := []float64{-math.Sqrt(0.6), 0, math.Sqrt(0.6)}
	gw := []float64{5.0 / 9, 8.0 / 9, 5.0 / 9}
	for i := 0; i < propN(20); i++ {
		n := 4 + r.Intn(4)
		p := SketchPath{Closed: true}
		for k := 0; k < n; k++ {
			a := (float64(k) + 0.3*r.Float64()) / float64(n) * 2 * math.Pi
			rad := 8 + 3*r.Float64()
			p.Points = append(p.Points, SketchPoint{P: [2]float64{rad * math.Cos(a), rad * math.Sin(a)}, Kind: PointAuto})
		}
		exact := 0.0
		for k := range p.Points {
			c := p.Bezier(k)
			for g := range gx {
				tt := (gx[g] + 1) / 2
				u := 1 - tt
				x := u*u*u*c[0].X + 3*u*u*tt*c[1].X + 3*u*tt*tt*c[2].X + tt*tt*tt*c[3].X
				y := u*u*u*c[0].Y + 3*u*u*tt*c[1].Y + 3*u*tt*tt*c[2].Y + tt*tt*tt*c[3].Y
				dx := 3 * (u*u*(c[1].X-c[0].X) + 2*u*tt*(c[2].X-c[1].X) + tt*tt*(c[3].X-c[2].X))
				dy := 3 * (u*u*(c[1].Y-c[0].Y) + 2*u*tt*(c[2].Y-c[1].Y) + tt*tt*(c[3].Y-c[2].Y))
				exact += gw[g] / 2 * (x*dy - y*dx) / 2
			}
		}
		sk := &Sketch{Paths: []SketchPath{p}}
		prev := 0.0
		for _, q := range []int{8, 16, 32, 64} {
			v := propSolid(t, fmt.Sprintf("case %d quality %d", i, q), SketchExtrude(sk, 0, 1, Extrude{Quality: q, TopScale: 100, BaseScale: 100}))
			err := exact - v
			if err <= 0 {
				t.Errorf("case %d quality %d: area %.9f isn't short of the curve's %.9f", i, q, v, exact)
			}
			if prev > 0 {
				if ratio := err / prev; ratio < 0.2 || ratio > 0.3 {
					t.Errorf("case %d quality %d: error %.3g, %.3f of the last (want about 1/4)", i, q, err, ratio)
				}
			}
			prev = err
		}
	}
}

// --- regions -----------------------------------------------------------------

// For the regions of random drawings' outlines (lines included, crossing
// and touching), each region's triangles run anticlockwise, have their
// middles inside the region (even-odd over its outlines), and cover the
// region's even-odd area exactly: its outer outline less its holes. And all
// the regions together cover the drawing's exact area (for filled
// drawings, this file's model's).
func TestSketchPropTriangulation(t *testing.T) {
	r := rand.New(rand.NewSource(propSeed(1111)))
	for i := 0; i < propN(100); i++ {
		var sk *Sketch
		filled := i%3 != 0
		if filled {
			sk = propDrawing(r, -12, 12, -12, 12, i%2 == 0)
		} else {
			sk = &Sketch{}
			for k := 0; k < 1+r.Intn(2); k++ {
				p := SketchPath{Closed: r.Intn(2) == 0, Stroke: 0.5 + r.Float64()*4, Join: r.Intn(3), Cap: r.Intn(3), Align: r.Intn(3)}
				for j := 0; j < 2+r.Intn(3); j++ {
					p.Points = append(p.Points, SketchPoint{P: [2]float64{math.Round(r.Float64()*80)/4 - 10, math.Round(r.Float64()*80)/4 - 10}})
				}
				sk.Paths = append(sk.Paths, p)
			}
		}
		q := 6
		outl := SketchOutlinesN(sk, q)
		total := 0.0
		for ri, rg := range Regions(outl) {
			rings := append([][]Pt{rg.Outer}, rg.Holes...)
			want := 0.0
			for _, c := range rings {
				want += SignedArea(c)
			}
			got := 0.0
			for _, tr := range Triangulate(rg) {
				a := SignedArea(tr[:])
				got += a
				if a < -1e-12 {
					t.Errorf("case %d region %d: a triangle runs clockwise: %v", i, ri, tr)
				}
				if a > 1e-6 {
					mid := Pt{(tr[0].X + tr[1].X + tr[2].X) / 3, (tr[0].Y + tr[1].Y + tr[2].Y) / 3}
					in := false
					for _, c := range rings {
						if propWind(mid, c) != 0 {
							in = !in
						}
					}
					if !in {
						t.Errorf("case %d region %d: triangle %v is outside the region", i, ri, tr)
					}
				}
			}
			if math.Abs(got-want) > 1e-9*math.Max(1, math.Abs(want)) {
				t.Errorf("case %d region %d: triangles cover %.9f, the region %.9f %+v", i, ri, got, want, sk.Paths)
			}
			total += got
		}
		var exact float64
		if filled {
			exact, _ = newPropModel(sk, q).measure(math.Inf(1))
		} else {
			ar := sketchArea(sk, q)
			x0, x1 := math.Inf(1), math.Inf(-1)
			for _, e := range ar.edges {
				x0, x1 = math.Min(x0, math.Min(e[0].X, e[1].X)), math.Max(x1, math.Max(e[0].X, e[1].X))
			}
			if len(ar.edges) > 0 {
				exact, _ = propSlab(ar.edges, ar.inside, x0, x1)
			}
		}
		if propRel(total, exact) > 1e-7 {
			t.Errorf("case %d: the regions cover %.6f, the drawing %.6f %+v", i, total, exact, sk.Paths)
		}
	}
}

// --- found by the properties ---------------------------------------------------

// A line covers each segment's rectangle (its width about its center
// line), whatever its corners and ends, even one that doubles back
// sharply, (1,0) -> (0,0) -> (10,1), 1 wide. There the inner corner's miter
// point (where the two segments' inside offsets meet, h/sin(half the angle
// between them) from the corner) lies beyond the shorter segment's far
// end, yet the outline it makes is still simple; used as it is, it cuts a
// strip along the second segment out of the line (6.05 mm² in all, less
// than the second segment's 1 × √101 rectangle alone). The first segment
// needn't be short, only shorter than the miter: (11.25,6.75) -> (2,8.25)
// -> (20,8.75); and closed lines do it too. (Regression:
// TestSketchPropStrokes, SKETCH_PROPS_TIMES=10.)
func TestSketchPropLineDoublingBackKeepsItsWidth(t *testing.T) {
	for _, c := range []struct {
		name    string
		path    SketchPath
		atLeast float64
	}{
		{"short then back", SketchPath{Points: skPts(1, 0, 0, 0, 10, 1), Stroke: 1}, math.Sqrt(101)},
		{"long then back", SketchPath{Points: skPts(11.25, 6.75, 2, 8.25, 20, 8.75), Stroke: 2.46}, 0},
		{"closed", SketchPath{Closed: true, Points: skPts(7.75, 14.75, 16.75, 18.75, 2.25, 7, 11.25, 12.25), Stroke: 2.51}, 0},
	} {
		for _, join := range []int{JoinSharp, JoinRound, JoinBevel} {
			for _, capKind := range []int{CapFlat, CapRound, CapSquare} {
				if c.path.Closed && capKind != CapFlat {
					continue // no ends
				}
				path := c.path
				path.Join, path.Cap = join, capKind
				sk := &Sketch{Paths: []SketchPath{path}}
				name := fmt.Sprintf("%s, join %d, cap %d", c.name, join, capKind)
				if got := outlineArea(sk); got < c.atLeast-1e-9 {
					t.Errorf("%s: area %.4f, less than the second segment's rectangle %.4f", name, got, c.atLeast)
				}
				m := SketchSolid(sk, 0, 1)
				solidOK(t, name, m)
				pts := path.Flatten()
				n := len(pts) - 1
				if path.Closed {
					n++
				}
				missed, tried := 0, 0
				h := path.Width() / 2
				for k := 0; k < n; k++ {
					a, b := pts[k], pts[(k+1)%len(pts)]
					nrm := ptScale(ptPerp(ptUnit(ptSub(b, a))), h)
					for f := 0.05; f < 0.96; f += 0.05 {
						for _, off := range []float64{-0.9, -0.5, 0, 0.5, 0.9} {
							tried++
							p := ptAdd(Pt{a.X + (b.X-a.X)*f, a.Y + (b.Y-a.Y)*f}, ptScale(nrm, off))
							if windingAt(m, geom.V(p.X, p.Y, 0.5)) < 0.5 {
								missed++
							}
						}
					}
				}
				if missed > 0 {
					t.Errorf("%s: %d of %d points within its segments' rectangles are outside the line", name, missed, tried)
				}
			}
		}
	}
}

// A coil's turns must not run into each other anywhere, not only at its
// steps' angles: here a profile (−0.05,0) (−0.4,0) (−4,5) (−0.05,10) whose
// top edge runs down to the axis gap in one long side while its bottom
// edge there is short. Each wall quad is folded along a diagonal; if the
// top's long quad and the next turn's bottom quads were folded across
// different corners, near the axis the long quad's fold would rise by much
// of a step's rise (pitch/sides) between the steps: at the least pitch
// through the 0.01 mm gap, and with few sides through a 1 mm one (pitch
// 11), into the turn above, where the winding number is 2. (Regression:
// TestSketchPropCoil, SKETCH_PROPS_TIMES=10, SKETCH_PROPS_SEED=2, 5 and
// 6.)
func TestSketchPropCoilTurnsNeverOverlap(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{{Closed: true, Points: skPts(-0.05, 0, -0.4, 0, -4, 5, -0.05, 10)}}}
	for _, c := range []struct {
		pitch float64
		sides int
	}{{0.1, 4}, {0.1, 12}, {0.1, 48}, {11, 5}} {
		pitch := math.Max(c.pitch, CoilMinPitch(10))
		sides := c.sides
		m := coilMesh(sk, 3, c.pitch, 0, sides, 0)
		solidOK(t, fmt.Sprintf("pitch %v, %d sides", pitch, sides), m)
		worst, at := 0.0, geom.Vec3{}
		// the gap between the first turn's top and the second's bottom,
		// near the axis, followed round the turn
		for ri := 0; ri <= 12; ri++ {
			rho := 0.06 + float64(ri)*0.03
			for ai := 0; ai < 90; ai++ {
				f := float64(ai) / 90
				a := (90 + 360*f) * math.Pi / 180
				for z := 9.0; z < pitch+1; z += 0.02 {
					q := geom.V(rho*math.Cos(a), rho*math.Sin(a), z+pitch*(1+f))
					if w := windingAt(m, q); w > worst {
						worst, at = w, q
					}
				}
			}
		}
		if worst > 1.5 {
			t.Errorf("pitch %v, %d sides: winding %.3f at radius %.3f, height %.3f, %.1f° round: the turns overlap", pitch, sides, worst,
				math.Hypot(at.X, at.Y), at.Z, math.Mod(math.Atan2(at.Y, at.X)*180/math.Pi-90+360, 360))
		}
	}
}

// Three triangles, one off on its own, and one ending a hair (4.9e-6 mm)
// across another's side: the drawing is the three of them (91.9155 mm²),
// and so must its outlines be, not the lone one's alone (19.5 mm²).
// outlinesOf asks which side of each piece is inside at delta = 1e-7 × the
// drawing's size off it; the lone triangle makes the drawing bigger, and
// that step (2.5e-6 mm) crosses the sliver between the two sides, so a
// piece of their outline reads as inside on both sides. Left out, it would
// break its loop and the two triangles that join would be lost whole.
// Without the lone triangle (a smaller drawing) the step is small enough.
// (Regression: TestSketchPropTriangulation, SKETCH_PROPS_TIMES=10,
// SKETCH_PROPS_SEED=4, shrunk.)
func TestSketchPropHairlineOverlapKeepsTheOutline(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{
		{Closed: true, Points: skPts(7, -14, 7, -2, -4, -2)},
		{Closed: true, Points: skPts(-11, -16.5, -12.5, -13.25, -5, -3.5)},
		{Closed: true, Points: skPts(12.111303058594782, -10.448558223968453, 7.000004901030414, -10.01319054966807, 6.876805601202101, -7.431867576458719)},
	}}
	want, _ := newPropModel(sk, 0).measure(math.Inf(1))
	if got := outlineArea(sk); math.Abs(got-want) > 1e-6 {
		t.Errorf("outlines cover %.6f mm², the drawing %.6f", got, want)
	}
	if v := solidOK(t, "extruded", SketchSolid(sk, 0, 1)); math.Abs(v-want) > 1e-6 {
		t.Errorf("volume %.6f, want %.6f", v, want)
	}
}
