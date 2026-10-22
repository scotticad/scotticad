package shapes

// Triangulation of awkward polygons: vertices on a coarse grid (many
// collinear runs), extra vertices placed exactly on edges (they lie on
// candidate ears' sides, which is what the strict ear test exists for),
// and holes. The triangles must tile the region exactly: their areas sum
// to the outline's less the holes', each is counter-clockwise, and none
// pokes outside the outline or into a hole.

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// starPoly is a simple polygon around (cx, cy): vertices at increasing
// angles and random radii, snapped to a grid of step g (snapping can make
// neighbours coincide; those are dropped).
func starPoly(r *rand.Rand, cx, cy, rmin, rmax, g float64, n int) []Pt {
	var out []Pt
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * (float64(i) + 0.3*r.Float64()) / float64(n)
		rad := rmin + (rmax-rmin)*r.Float64()
		p := Pt{math.Round((cx+rad*math.Cos(a))/g) * g, math.Round((cy+rad*math.Sin(a))/g) * g}
		if len(out) > 0 && same(p, out[len(out)-1]) {
			continue
		}
		out = append(out, p)
	}
	if len(out) > 1 && same(out[0], out[len(out)-1]) {
		out = out[:len(out)-1]
	}
	return out
}

// withMidpoints puts k extra vertices exactly on some edges.
func withMidpoints(r *rand.Rand, c []Pt, k int) []Pt {
	for ; k > 0; k-- {
		i := r.Intn(len(c))
		a, b := c[i], c[(i+1)%len(c)]
		t := []float64{0.25, 0.5, 0.75}[r.Intn(3)]
		m := Pt{a.X + t*(b.X-a.X), a.Y + t*(b.Y-a.Y)}
		c = append(c[:i+1], append([]Pt{m}, c[i+1:]...)...)
	}
	return c
}

// simple reports whether c visits no point twice and no two of its edges
// cross or touch except neighbours at their shared corner.
func simple(c []Pt) bool {
	n := len(c)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if same(c[i], c[j]) {
				return false
			}
		}
	}
	onSeg := func(p, a, b Pt) bool {
		return math.Abs(cross2(a, b, p)) < 1e-12 && p.X >= math.Min(a.X, b.X)-1e-12 && p.X <= math.Max(a.X, b.X)+1e-12 &&
			p.Y >= math.Min(a.Y, b.Y)-1e-12 && p.Y <= math.Max(a.Y, b.Y)+1e-12
	}
	for i := 0; i < n; i++ {
		a, b := c[i], c[(i+1)%n]
		for j := i + 1; j < n; j++ {
			if j == i+1 || (i == 0 && j == n-1) {
				continue // neighbours
			}
			p, q := c[j], c[(j+1)%n]
			d1, d2, d3, d4 := cross2(a, b, p), cross2(a, b, q), cross2(p, q, a), cross2(p, q, b)
			if ((d1 > 0) != (d2 > 0) && d1 != 0 && d2 != 0) && ((d3 > 0) != (d4 > 0) && d3 != 0 && d4 != 0) {
				return false
			}
			if onSeg(p, a, b) || onSeg(q, a, b) || onSeg(a, p, q) || onSeg(b, p, q) {
				return false
			}
		}
	}
	return true
}

func polyArea(c []Pt) float64 {
	a := 0.0
	for i := range c {
		p, q := c[i], c[(i+1)%len(c)]
		a += p.X*q.Y - q.X*p.Y
	}
	return a / 2
}

func TestPropertyTriangulateAwkwardPolygons(t *testing.T) {
	seeds := int64(20000) // the strict ear test is needed only rarely: a few thousand reach it
	if testing.Short() {
		seeds = 2000
	}
	for seed := int64(1); seed <= seeds; seed++ {
		r := rand.New(rand.NewSource(seed))
		outer := starPoly(r, 0, 0, 8, 20, []float64{1, 2, 4}[r.Intn(3)], 6+r.Intn(30))
		if len(outer) < 3 || polyArea(outer) <= 0 || !simple(outer) {
			continue // snapping can pinch it: not a simple outline
		}
		outer = withMidpoints(r, outer, r.Intn(12))
		reg := Region{Outer: outer}
		want := polyArea(outer)
		// sometimes a hole near the middle, well inside every outline
		// vertex (rmin 8 against a hole of radius at most 4)
		if r.Intn(2) == 0 {
			h := starPoly(r, 0, 0, 1, 4, 1, 5+r.Intn(8))
			if len(h) >= 3 && polyArea(h) > 0 && simple(h) {
				h = withMidpoints(r, h, r.Intn(4))
				reg.Holes = [][]Pt{reversed(h)}
				want -= polyArea(h)
			}
		}
		desc := fmt.Sprintf("seed %d: outer %v holes %v", seed, reg.Outer, reg.Holes)
		tris := Triangulate(reg)
		got := 0.0
		for _, tr := range tris {
			a := cross2(tr[0], tr[1], tr[2]) / 2
			if a <= 0 {
				t.Fatalf("%s\na triangle wound clockwise or flat: %v", desc, tr)
			}
			got += a
			// its centre is inside the outline and outside every hole
			c := Pt{(tr[0].X + tr[1].X + tr[2].X) / 3, (tr[0].Y + tr[1].Y + tr[2].Y) / 3}
			if !PointInPolygon(c, reg.Outer) {
				t.Fatalf("%s\ntriangle %v lies outside the outline", desc, tr)
			}
			for _, h := range reg.Holes {
				if PointInPolygon(c, h) {
					t.Fatalf("%s\ntriangle %v lies in a hole", desc, tr)
				}
			}
		}
		if math.Abs(got-want) > 1e-9*math.Max(1, want) {
			t.Fatalf("%s\ntriangles cover %.12g, the region is %.12g", desc, got, want)
		}
	}
}
