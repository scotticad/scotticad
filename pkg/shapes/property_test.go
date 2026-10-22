package shapes

// Property-based tests: every catalogue kind built with random in-range
// slider values and random sizes is a watertight, outward-facing solid
// filling its box; random polygons with holes triangulate exactly.

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

// ptValues draws random slider values within [Min, Max] (endpoints a
// quarter of the time), whole numbers for whole-number sliders.
func ptValues(r *rand.Rand, k *Kind) Values {
	v := k.Defaults()
	for _, p := range k.Params {
		if p.Kind != Slider {
			continue
		}
		var x float64
		switch r.Intn(8) {
		case 0:
			x = p.Min
		case 1:
			x = p.Max
		default:
			x = p.Min + r.Float64()*(p.Max-p.Min)
		}
		if p.Int {
			x = math.Round(x)
		}
		v.Num[p.Key] = x
	}
	return v
}

func ptDescribe(id string, size geom.Vec3, v Values) string {
	keys := make([]string, 0, len(v.Num))
	for k := range v.Num {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s size=(%g, %g, %g)", id, size.X, size.Y, size.Z)
	for _, k := range keys {
		fmt.Fprintf(&sb, " %s=%g", k, v.Num[k])
	}
	return sb.String()
}

// shapes whose mesh doesn't touch every face of the requested box by design
// (the thread's facets reach its major diameter only where a crest meets a
// facet corner, so its X and Y extents fall short of the box): for these,
// the mesh need only fit inside the box and fill it in Z
var ptNotFilling = map[string]bool{"thread": true}

// slow kinds (big meshes at large sizes) get fewer random cases
var ptCases = map[string]int{"thread": 4, "hexbolt": 4}

func finite3(v geom.Vec3) bool {
	for _, x := range []float64{v.X, v.Y, v.Z} {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return false
		}
	}
	return true
}

// ptCheckShape returns what is wrong with kind id built at size with v.
func ptCheckShape(k *Kind, size geom.Vec3, v Values) []string {
	var bad []string
	m := Build(k.ID, size, v)
	if len(m.Tris) == 0 {
		return []string{"empty mesh"}
	}
	if k.parts == nil { // an assembly's own mesh is its parts side by side
		if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
			bad = append(bad, fmt.Sprintf("%d open, %d non-manifold edges", o, n))
		}
	}
	if vol := m.Volume(); !(vol > 0) {
		bad = append(bad, fmt.Sprintf("volume %g", vol))
	}
	b := m.Bounds()
	if !finite3(b.Min) || !finite3(b.Max) {
		bad = append(bad, fmt.Sprintf("non-finite bounds %v", b))
	} else if ptNotFilling[k.ID] {
		h := size.Mul(0.5 * (1 + 1e-6))
		if b.Min.X < -h.X || b.Min.Y < -h.Y || b.Max.X > h.X || b.Max.Y > h.Y ||
			math.Abs(b.Size().Z-size.Z) > 1e-6*size.Z {
			bad = append(bad, fmt.Sprintf("bounds %v outside the box %v", b, size))
		}
	} else {
		s := b.Size()
		for i := 0; i < 3; i++ {
			if math.Abs(s.Axis(i)-size.Axis(i)) > 1e-6*size.Axis(i) {
				bad = append(bad, fmt.Sprintf("size %v, want %v", s, size))
				break
			}
		}
		if c := b.Center(); c.Len() > 1e-6*size.Len() {
			bad = append(bad, fmt.Sprintf("centre %v, want origin", c))
		}
	}
	if ns, ok := k.NaturalSize(v); ok {
		if !finite3(ns) || ns.X <= 0 || ns.Y <= 0 || ns.Z <= 0 {
			bad = append(bad, fmt.Sprintf("natural size %v", ns))
		}
	}
	return bad
}

// ptMinimize moves each slider value towards its default while the build
// still fails, giving a smaller reproduction.
func ptMinimize(k *Kind, size geom.Vec3, v Values) Values {
	def := k.Defaults()
	cur := Values{Num: map[string]float64{}, Text: v.Text, Font: v.Font}
	for kk, x := range v.Num {
		cur.Num[kk] = x
	}
	for _, p := range k.Params {
		if p.Kind != Slider {
			continue
		}
		old := cur.Num[p.Key]
		cur.Num[p.Key] = def.Num[p.Key]
		if len(ptCheckShape(k, size, cur)) == 0 {
			cur.Num[p.Key] = old
		}
	}
	return cur
}

func TestPropertyShapesRandomParams(t *testing.T) {
	type failure struct{ desc, why string }
	var fails, known []failure
	for ki, k := range All() {
		// each kind its own stream, so cases don't shift when kinds are added
		r := rand.New(rand.NewSource(int64(21 + 1000*ki)))
		perKind := 12
		if n, ok := ptCases[k.ID]; ok {
			perKind = n
		}
		start := time.Now()
		for i := 0; i < perKind; i++ {
			v := ptValues(r, k)
			size := geom.V(0.5+r.Float64()*299.5, 0.5+r.Float64()*299.5, 0.5+r.Float64()*299.5)
			if i == 0 { // the natural size, as placing from the library does
				if ns, ok := k.NaturalSize(v); ok && finite3(ns) && ns.X > 0 && ns.Y > 0 && ns.Z > 0 {
					size = ns
				}
			}
			if bad := ptCheckShape(k, size, v); len(bad) > 0 {
				mv := ptMinimize(k, size, v)
				why := strings.Join(bad, "; ")
				if b2 := ptCheckShape(k, size, mv); len(b2) > 0 {
					why = strings.Join(b2, "; ")
				}
				f := failure{ptDescribe(k.ID, size, mv), why}
				if k.ID == "hexbolt" && strings.HasSuffix(why, "non-manifold edges") {
					known = append(known, f) // see TestPropertyHexBoltSeam
				} else {
					fails = append(fails, f)
				}
			}
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Logf("%s: %d builds took %v", k.ID, perKind, d)
		}
	}
	for _, f := range known {
		t.Logf("known bug: %s: %s", f.desc, f.why)
	}
	for _, f := range fails {
		t.Errorf("%s: %s", f.desc, f.why)
	}
}

// A hex bolt is its head unioned with a thread standing on the head's top
// face. For some turn counts (M3 with 10 or 13 turns among them) two
// vertices on that face lie ~2.6e-5 mm apart, beyond FixTJunctions' 1e-5
// weld; the union must still leave no open edges there.
func TestPropertyHexBoltSeam(t *testing.T) {
	var bad []string
	for _, c := range []struct {
		size  float64
		turns int
	}{{3, 10}, {3, 13}, {3, 11}, {4, 10}} {
		v := Values{Num: map[string]float64{"size": c.size, "length": ThreadLength(metricFor(c.size).pitch, c.turns)}}
		if o, n := hexBoltModel(v).EdgeCheck(1e-7); o != 0 || n != 0 {
			bad = append(bad, fmt.Sprintf("M%g × %d turns (length %g): %d open, %d non-manifold", c.size, c.turns, v.Num["length"], o, n))
		}
	}
	if len(bad) > 0 {
		t.Error("hexbolt head∪thread union leaves open edges on the head's top face:\n" + strings.Join(bad, "\n"))
	}
}

// ---- triangulation ----

// ptStar is a star-shaped polygon about c with n random radii in
// [rmin, rmax], counter-clockwise.
func ptStar(r *rand.Rand, c Pt, n int, rmin, rmax float64) []Pt {
	angs := make([]float64, n)
	for i := range angs {
		angs[i] = r.Float64() * 2 * math.Pi
	}
	sort.Float64s(angs)
	out := make([]Pt, 0, n)
	for i, a := range angs {
		if i > 0 && a-angs[i-1] < 1e-3 {
			continue
		}
		rad := rmin + r.Float64()*(rmax-rmin)
		out = append(out, Pt{c.X + rad*math.Cos(a), c.Y + rad*math.Sin(a)})
	}
	return out
}

func TestPropertyTriangulate(t *testing.T) {
	r := rand.New(rand.NewSource(22))
	for i := 0; i < 300; i++ {
		R := 10 + r.Float64()*90
		outer := ptStar(r, Pt{0, 0}, 3+r.Intn(60), 0.6*R, R)
		if len(outer) < 3 || !PointInPolygon(Pt{0, 0}, outer) {
			continue
		}
		// the polygon is star-shaped about the origin (each ray from it
		// crosses the boundary once), so the disc of radius rin, the
		// nearest boundary distance, is inside it
		rin := math.Inf(1)
		for j := range outer {
			a, b := outer[j], outer[(j+1)%len(outer)]
			dx, dy := b.X-a.X, b.Y-a.Y
			s := math.Max(0, math.Min(1, -(a.X*dx+a.Y*dy)/(dx*dx+dy*dy)))
			rin = math.Min(rin, math.Hypot(a.X+s*dx, a.Y+s*dy))
		}
		// holes: small stars in disjoint discs inside 0.9 rin
		var holes [][]Pt
		type disc struct {
			c Pt
			r float64
		}
		var discs []disc
		nh := r.Intn(8)
		for tries := 0; len(discs) < nh && tries < 200; tries++ {
			hr := (0.05 + r.Float64()*0.2) * rin
			ang, dist := r.Float64()*2*math.Pi, r.Float64()*(0.9*rin-hr)
			c := Pt{dist * math.Cos(ang), dist * math.Sin(ang)}
			ok := true
			for _, d := range discs {
				if math.Hypot(c.X-d.c.X, c.Y-d.c.Y) < hr+d.r+0.01*R {
					ok = false
					break
				}
			}
			if ok {
				discs = append(discs, disc{c, hr})
			}
		}
		for _, d := range discs {
			h := ptStar(r, d.c, 3+r.Intn(12), 0.4*d.r, d.r)
			if len(h) < 3 {
				continue
			}
			if r.Intn(2) == 0 {
				h = reversed(h) // either winding
			}
			holes = append(holes, h)
		}
		desc := fmt.Sprintf("case %d: outer %v holes %v", i, outer, holes)
		regs := Regions(append([][]Pt{outer}, holes...))
		if len(regs) != 1 || len(regs[0].Holes) != len(holes) {
			t.Errorf("%d regions (%v holes), want 1 with %d holes\n%s", len(regs), func() []int {
				var n []int
				for _, g := range regs {
					n = append(n, len(g.Holes))
				}
				return n
			}(), len(holes), desc)
			continue
		}
		reg := regs[0]
		v := len(reg.Outer)
		wantA := SignedArea(reg.Outer)
		for _, h := range reg.Holes {
			v += len(h)
			wantA += SignedArea(h)
		}
		tris := Triangulate(reg)
		area, neg := 0.0, 0
		for _, tr := range tris {
			a := cross2(tr[0], tr[1], tr[2]) / 2
			if a < -1e-9 {
				neg++
			}
			area += math.Abs(a)
		}
		if want := v + 2*len(reg.Holes) - 2; len(tris) != want || math.Abs(area-wantA) > 1e-9*R*R || neg > 0 {
			t.Errorf("%d triangles (want %d), area %.12g (want %.12g), %d clockwise\n%s", len(tris), want, area, wantA, neg, desc)
			continue
		}
		m := ExtrudeContours(append([][]Pt{outer}, holes...), 0, 1+r.Float64()*5)
		if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
			t.Errorf("extrusion %d open, %d non-manifold\n%s", o, n, desc)
		}
		h := m.Bounds().Size().Z
		if vol := m.Volume(); math.Abs(vol-wantA*h) > 1e-9*R*R*h {
			t.Errorf("extrusion volume %.12g, want %.12g\n%s", vol, wantA*h, desc)
		}
	}
}
