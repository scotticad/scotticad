package geom

import (
	"math"
	"testing"
)

// Surface counts edges and shells in one pass of its own; it finds what
// EdgeCheck and Shells find.
func TestSurfaceAgreesWithEdgeCheckAndShells(t *testing.T) {
	doubled := box(V(0, 0, 0), V(1, 1, 1))
	doubled.Tris = append(doubled.Tris, doubled.Tris[0])
	for name, m := range map[string]*Mesh{
		"box":           box(V(0, 0, 0), V(1, 1, 1)),
		"hollow":        Merge(box(V(0, 0, 0), V(10, 10, 10)), box(V(0, 0, 0), V(6, 6, 6)).Flipped()),
		"edge to edge":  Merge(box(V(0, 0, 0), V(1, 1, 1)), box(V(1, 1, 0), V(1, 1, 1))),
		"doubled":       doubled,
		"T-junction":    AsWritten(splitBox(0)),
		"disc":          disc(40, 3),
		"three islands": Merge(box(V(0, 0, 0), V(1, 1, 1)), box(V(5, 0, 0), V(1, 1, 1)), disc(8, 1)),
	} {
		r := Surface(m)
		o, nm := m.EdgeCheck(0)
		if r.Open != o || r.NonManifold != nm {
			t.Errorf("%s: open %d, non-manifold %d; EdgeCheck says %d, %d", name, r.Open, r.NonManifold, o, nm)
		}
		shells := m.Shells()
		if len(r.Shells) != len(shells) {
			t.Fatalf("%s: %d shells, Shells finds %d", name, len(r.Shells), len(shells))
		}
		for i, s := range shells {
			so, snm := s.EdgeCheck(0)
			got := r.Shells[i]
			if got.Tris != len(s.Tris) || got.Closed != (so == 0 && snm == 0) || math.Abs(got.Volume-s.Volume()) > 1e-9 || got.Bounds != s.Bounds() {
				t.Errorf("%s: shell %d is %+v; Shells gives %d triangles, closed %v, volume %g, bounds %v",
					name, i, got, len(s.Tris), so == 0 && snm == 0, s.Volume(), s.Bounds())
			}
		}
	}
}

func TestSurfaceClosedBox(t *testing.T) {
	r := Surface(AsWritten(box(V(0, 0, 5), V(10, 10, 10))))
	if r.Open != 0 || r.NonManifold != 0 || r.Gap != 0 || r.InsideOut != 0 || r.Empty != 0 {
		t.Fatalf("a box: %+v", r)
	}
	if len(r.Shells) != 1 || !r.Shells[0].Closed || math.Abs(r.Shells[0].Volume-1000) > 1e-9 {
		t.Fatalf("a box's shell: %+v", r.Shells)
	}
}

func TestSurfaceMissingTriangle(t *testing.T) {
	m := box(V(0, 0, 0), V(10, 10, 10))
	m = &Mesh{Tris: m.Tris[1:]}
	r := Surface(m)
	if r.Open != 3 || len(OpenEdges(m)) != 3 {
		t.Fatalf("open edges: %+v", r)
	}
	// the hole is a right triangle with 10 mm sides
	if r.Gap < 5 || r.Gap > 15 || r.GapAtLeast {
		t.Fatalf("gap %g (at least: %v), want about the triangle's size", r.Gap, r.GapAtLeast)
	}
}

func TestSurfaceHairlineCrack(t *testing.T) {
	m := box(V(0, 0, 0), V(10, 10, 10))
	// the top face (z = +5) lifted 1e-4 mm, its sides left where they were
	for i := range m.Tris {
		tr := &m.Tris[i]
		if tr.P[0].Z > 0 && tr.P[1].Z > 0 && tr.P[2].Z > 0 {
			for k := range tr.P {
				tr.P[k].Z += 1e-4
			}
		}
	}
	r := Surface(m)
	if r.Open == 0 {
		t.Fatal("the crack isn't open")
	}
	if r.Gap < 0.5e-4 || r.Gap > 2e-4 {
		t.Fatalf("gap %g, want about 1e-4", r.Gap)
	}
}

// A large hole with short edges is a hole, not a crack: neighbouring
// edges along its rim don't count as the other side.
func TestSurfaceFineHoleIsWide(t *testing.T) {
	const n = 256
	r := Surface(disc(n, 5))
	if r.Open != n || r.Gap < 5 {
		t.Fatalf("a disc's rim: open %d, gap %g; want %d, at least 5", r.Open, r.Gap, n)
	}
}

// disc is a flat fan of n triangles, radius r: its rim is all open.
func disc(n int, r float64) *Mesh {
	m := &Mesh{}
	at := func(i int) Vec3 {
		a := 2 * math.Pi * float64(i%n) / float64(n)
		return V(r*math.Cos(a), r*math.Sin(a), 0)
	}
	for i := 0; i < n; i++ {
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{{}, at(i), at(i + 1)}})
	}
	return m
}

// The grid search, used for many open edges, finds what trying every
// pair finds, or says it stopped short.
func TestGapGridMatchesBrute(t *testing.T) {
	lifted := box(V(0, 0, 0), V(10, 10, 10))
	for i := range lifted.Tris {
		if tr := &lifted.Tris[i]; tr.P[0].Z > 0 && tr.P[1].Z > 0 && tr.P[2].Z > 0 {
			for k := range tr.P {
				tr.P[k].Z += 1e-4
			}
		}
	}
	missing := box(V(0, 0, 0), V(10, 10, 10))
	missing = &Mesh{Tris: missing.Tris[1:]}
	for _, c := range []struct {
		name string
		m    *Mesh
	}{
		{"crack", lifted},
		{"missing triangle", missing},
		{"T-junction", AsWritten(splitBox(0))},
		{"disc", disc(300, 5)},
	} {
		g := newGapSegs(OpenEdges(c.m))
		bg, bAt := g.brute()
		gg, gAt := g.grid()
		if bAt {
			t.Fatalf("%s: every pair tried, and no match", c.name)
		}
		if gAt && gg > bg+1e-12 || !gAt && math.Abs(gg-bg) > 1e-12 {
			t.Fatalf("%s: grid %g (at least: %v), every pair %g", c.name, gg, gAt, bg)
		}
	}
	// an open edge with nothing running against it has no size to
	// measure: at least 0, not a made-up number
	if g, at := gapWidth([][2]Vec3{{V(0, 0, 0), V(1, 0, 0)}, {V(50, 0, 0), V(50, 1, 0)}}); g != 0 || !at {
		t.Errorf("stray edges: gap %g, at least %v; want 0, true", g, at)
	}
	// the grid too, though the search runs out of rings first
	stray := newGapSegs([][2]Vec3{{V(0, 0, 0), V(1, 0, 0)}, {V(50, 0, 0), V(50, 1, 0)}})
	if g, at := stray.grid(); g != 0 || !at {
		t.Errorf("stray edges, grid: gap %g, at least %v; want 0, true", g, at)
	}
	// and beyond its rings it reports the far side's distance, as a lower
	// bound: two open edges 40 apart, against each other
	far := newGapSegs([][2]Vec3{{V(0, 0, 0), V(1, 0, 0)}, {V(1, 40, 0), V(0, 40, 0)}})
	if g, at := far.grid(); g != 40 || !at {
		t.Errorf("far edges, grid: gap %g, at least %v; want 40, true", g, at)
	}
	// past gapEdges the grid is used, and a fine crack still measures fine
	m := disc(3*gapEdges, 5)
	if r := Surface(m); r.Gap < 1 {
		t.Fatalf("a disc of %d edges: %+v", 3*gapEdges, r)
	}
}

func TestSurfaceNonManifold(t *testing.T) {
	// a triangle doubled: its edges are used twice one way, once the other
	m := box(V(0, 0, 0), V(1, 1, 1))
	m.Tris = append(m.Tris, m.Tris[0])
	if r := Surface(m); r.Open != 0 || r.NonManifold == 0 {
		t.Fatalf("a doubled triangle: %+v", r)
	}
	// two cubes touching along one edge use it twice each way, which
	// unions make all the time and which prints fine
	m = Merge(box(V(0, 0, 0), V(1, 1, 1)), box(V(1, 1, 0), V(1, 1, 1)))
	if r := Surface(m); r.Open != 0 || r.NonManifold != 0 {
		t.Fatalf("two cubes sharing an edge: %+v", r)
	}
}

func TestSurfaceInsideOut(t *testing.T) {
	if r := Surface(box(V(0, 0, 0), V(10, 10, 10)).Flipped()); r.InsideOut != 1 {
		t.Fatalf("a flipped box: %+v", r)
	}
	// a cavity is a shell inside out too, but inside another
	hollow := Merge(box(V(0, 0, 0), V(10, 10, 10)), box(V(0, 0, 0), V(6, 6, 6)).Flipped())
	if r := Surface(hollow); r.InsideOut != 0 || len(r.Shells) != 2 {
		t.Fatalf("a hollow box: %+v", r)
	}
}

func TestSurfaceEmptyShell(t *testing.T) {
	// two triangles back to back: closed, with no volume
	sheet := &Mesh{Tris: []Tri{
		{P: [3]Vec3{V(0, 0, 0), V(1, 0, 0), V(0, 1, 0)}},
		{P: [3]Vec3{V(0, 0, 0), V(0, 1, 0), V(1, 0, 0)}},
	}}
	if r := Surface(Merge(box(V(5, 5, 5), V(2, 2, 2)), sheet)); r.Empty != 1 || r.Open != 0 {
		t.Fatalf("a box and a sheet: %+v", r)
	}
}

// splitBox is a unit cube whose triangle (1, 7, 5) on the x = +0.5 face is
// split at m, a point on (or, by d, just off) the diagonal 1-7, with a
// sliver (7, m, 1) filling the T-junction: closed, but only through the
// sliver.
func splitBox(d float64) *Mesh {
	p := [8]Vec3{}
	for i := range p {
		p[i] = V(float64(i&1)-0.5, float64(i>>1&1)-0.5, float64(i>>2&1)-0.5)
	}
	mid := p[1].Lerp(p[7], 0.5).Add(V(d, 0, 0))
	m := &Mesh{}
	for _, tr := range box(V(0, 0, 0), V(1, 1, 1)).Tris {
		if tr.P == [3]Vec3{p[1], p[7], p[5]} {
			m.Tris = append(m.Tris,
				Tri{P: [3]Vec3{p[1], mid, p[5]}},
				Tri{P: [3]Vec3{mid, p[7], p[5]}},
				Tri{P: [3]Vec3{p[7], mid, p[1]}})
			continue
		}
		m.Tris = append(m.Tris, tr)
	}
	if len(m.Tris) != 14 {
		panic("the box's triangles have changed; splitBox can't find (1, 7, 5)")
	}
	return m
}

func TestSurfaceClosedOnlyByZeroAreaTriangle(t *testing.T) {
	m := splitBox(0)
	if r := Surface(m); r.Open != 0 || r.NonManifold != 0 {
		t.Fatalf("as built: %+v", r)
	}
	r := Surface(AsWritten(m))
	if r.Open == 0 {
		t.Fatal("as written it should be open: the sliver isn't written")
	}
	if r.Gap > 1e-9 {
		t.Fatalf("a T-junction's gap is %g, want about zero", r.Gap)
	}
}

// A sliver that rounding to float32 flattens is still written, and its
// corners still meet their neighbours': the mesh stays closed.
func TestSurfaceSliverFlattenedByRounding(t *testing.T) {
	// 0.5 + 1e-9 is 0.5 in float32: the sliver has area in float64, and
	// none once written
	m := splitBox(1e-9)
	if len(m.Clean().Tris) != 14 {
		t.Fatal("the sliver should survive Clean in float64")
	}
	w := AsWritten(m)
	flat := 0
	for _, tr := range w.Tris {
		if tr.Area() <= 1e-12 {
			flat++
		}
	}
	if len(w.Tris) != 14 || flat != 1 {
		t.Fatalf("%d triangles written, %d flat; the test needs the sliver flattened and still written", len(w.Tris), flat)
	}
	if r := Surface(w); r.Open != 0 || r.NonManifold != 0 {
		t.Fatalf("as written: %+v; want closed", r)
	}
}

// frustum is an n-sided frustum standing on z = 0, radius r0 at the bottom
// and r1 at the top, h tall.
func frustum(n int, r0, r1, h float64) *Mesh {
	at := func(i int, r, z float64) Vec3 {
		a := 2 * math.Pi * float64(i%n) / float64(n)
		return V(r*math.Cos(a), r*math.Sin(a), z)
	}
	m := &Mesh{}
	for i := 0; i < n; i++ {
		b0, b1, t0, t1 := at(i, r0, 0), at(i+1, r0, 0), at(i, r1, h), at(i+1, r1, h)
		m.Tris = append(m.Tris,
			Tri{P: [3]Vec3{b0, b1, t1}}, Tri{P: [3]Vec3{b0, t1, t0}},
			Tri{P: [3]Vec3{V(0, 0, 0), b1, b0}}, Tri{P: [3]Vec3{V(0, 0, h), t0, t1}})
	}
	return m
}

func TestOverhangs(t *testing.T) {
	// a box: only its bottom faces down, flat
	r := Overhangs(box(V(0, 0, 5), V(10, 10, 10)), 0)
	if r.Sloped != 0 || len(r.Flat) != 1 || r.Flat[0].Z != 0 || math.Abs(r.Flat[0].Area-100) > 1e-9 {
		t.Fatalf("a box: %+v", r)
	}
	// a cone on its point (63° from upright): all of its side
	m := frustum(32, 1, 11, 5)
	side := 0.0
	for i, tr := range m.Tris {
		if i%4 < 2 {
			side += tr.Area()
		}
	}
	if r := Overhangs(m, 0); math.Abs(r.Sloped-side) > 1e-6 || len(r.Flat) != 1 {
		t.Fatalf("an upside-down cone: sloped %g, want %g; flat %+v", r.Sloped, side, r.Flat)
	}
	// its side is all below 6 mm
	if r := Overhangs(m, 6); r.Sloped != 0 {
		t.Fatalf("below low: %g", r.Sloped)
	}
	// 45° exactly, and a cone the right way up: none
	for name, m := range map[string]*Mesh{"45°": AsWritten(frustum(32, 1, 6, 5)), "upright": frustum(32, 11, 1, 5)} {
		if r := Overhangs(m, 0); r.Sloped != 0 {
			t.Errorf("%s: sloped %g, want 0", name, r.Sloped)
		}
	}
}

func TestProblemEdges(t *testing.T) {
	m := box(V(0, 0, 0), V(10, 10, 10))
	if es := ProblemEdges(m); len(es) != 0 {
		t.Fatalf("a closed box: %d edges", len(es))
	}
	open := &Mesh{Tris: m.Tris[1:]}
	if es := ProblemEdges(open); len(es) != 3 {
		t.Fatalf("a box missing a triangle: %d edges, want 3", len(es))
	}
	doubled := &Mesh{Tris: append(append([]Tri{}, m.Tris...), m.Tris[0])}
	if es := ProblemEdges(doubled); len(es) != 3 {
		t.Fatalf("a box with a triangle doubled: %d edges, want its 3", len(es))
	}
}
