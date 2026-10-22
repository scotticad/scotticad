package geom

import (
	"compress/gzip"
	"io"
	"math"
	"os"
	"testing"
	"time"
)

// Each stage of FixTJunctions (listed on it), on a mesh built to need
// exactly that stage. The guarantees checked here are that a
// closed mesh passes through untouched, that each stage closes what it is
// for, that a hole it must not guess at stays open, and that no result
// holds a triangle of no area.

// closed is EdgeCheck(0) == (0, 0).
func closed(m *Mesh) bool {
	o, nm := m.EdgeCheck(0)
	return o == 0 && nm == 0
}

// settled fails if a repaired mesh holds a triangle a writer would drop,
// or if repairing it again changes it: a repair is a fixed point.
func settled(t *testing.T, m *Mesh) {
	t.Helper()
	if n := len(m.Clean().Tris); n != len(m.Tris) {
		t.Errorf("result holds %d triangles of no area", len(m.Tris)-n)
	}
	if meshPrint(m.FixTJunctions()) != meshPrint(m) {
		t.Error("repairing the result again changed it")
	}
}

// prism is an exactly closed n-gon prism of radius r and height h: each
// shared vertex is computed once (cyl computes its seam twice, 1e-15
// apart, and so is closed only by the weld).
func prism(r, h float64, n int) *Mesh {
	pts := make([]Vec3, n)
	for i := range pts {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = V(r*math.Cos(a), r*math.Sin(a), 0)
	}
	m := &Mesh{}
	c, ct := V(0, 0, 0), V(0, 0, h)
	for i := range pts {
		p0, p1 := pts[i], pts[(i+1)%n]
		q0, q1 := p0.Add(V(0, 0, h)), p1.Add(V(0, 0, h))
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{p0, p1, q1}}, Tri{P: [3]Vec3{p0, q1, q0}},
			Tri{P: [3]Vec3{c, p1, p0}}, Tri{P: [3]Vec3{ct, q0, q1}})
	}
	return m
}

// box20 is a 20 mm cube at the origin: volume 8000, 12 triangles.
func box20() *Mesh { return box(V(0, 0, 0), V(20, 20, 20)) }

// topFace returns the indices of box20's +Z triangles.
func topFace(m *Mesh) []int {
	var out []int
	for i, tr := range m.Tris {
		if tr.P[0].Z == 10 && tr.P[1].Z == 10 && tr.P[2].Z == 10 {
			out = append(out, i)
		}
	}
	return out
}

func without(m *Mesh, drop ...int) *Mesh {
	skip := map[int]bool{}
	for _, i := range drop {
		skip[i] = true
	}
	out := &Mesh{}
	for i, tr := range m.Tris {
		if !skip[i] {
			out.Tris = append(out.Tris, tr)
		}
	}
	return out
}

// A closed mesh comes back exactly as it was: the same triangles in the
// same order. The stitch, sliver, sheet and fill stages never run on it.
func TestFixTJunctionsLeavesClosedMeshAlone(t *testing.T) {
	for name, m := range map[string]*Mesh{
		"box":          box20(),
		"rotated box":  box20().Transform(RotateZ(0.37).Mul(RotateX(1.1))),
		"prism":        prism(7, 11, 37).Transform(Translate(V(3, -2, 1))),
		"two shells":   Merge(box20(), box(V(50, 0, 0), V(5, 5, 5))),
		"with cavity":  Merge(box20(), box(V(0, 0, 0), V(8, 8, 8)).Flipped()),
		"union result": Union(box20(), cyl(10, 5, 10, 20, 48)).FixTJunctions(),
	} {
		if !closed(m) || len(openEdgeVertices(m)) > 0 {
			t.Fatalf("%s: the test mesh should be exactly closed", name)
		}
		got := m.FixTJunctions()
		if meshPrint(got) != meshPrint(m) {
			t.Errorf("%s: a closed mesh was changed (%d -> %d triangles)", name, len(m.Tris), len(got.Tris))
		}
	}
}

// Two closed shells merged face to face share a doubled wall: each of its
// edges is used twice each way, which EdgeCheck accepts but the repair
// treats as a fold. The two coincident faces cancel, which joins the
// shells into one; the volume and the outer surface are unchanged. (A
// boolean union of the two gives the same.)
func TestFixTJunctionsJoinsShellsTouchingFaceToFace(t *testing.T) {
	m := Merge(box20(), box(V(20, 0, 0), V(20, 20, 20)))
	if !closed(m) {
		t.Fatal("EdgeCheck should accept the doubled wall")
	}
	got := m.FixTJunctions()
	if !closed(got) || len(openEdgeVertices(got)) > 0 {
		t.Fatal("not closed")
	}
	if n := len(got.Shells()); n != 1 {
		t.Errorf("%d shells, want 1", n)
	}
	if v := got.Volume(); math.Abs(v-16000) > 1e-9 {
		t.Errorf("volume %v, want 16000", v)
	}
	if len(got.Tris) != 20 {
		t.Errorf("%d triangles, want 20 (24 less the 4 of the shared wall)", len(got.Tris))
	}
	settled(t, got)
}

// Stage 2: a T-junction. A vertex on the far side of a face at the middle
// of an edge the near side doesn't know: welded exactly, split exactly.
func TestFixTJunctionsSplitsTJunction(t *testing.T) {
	m := box20()
	top := topFace(m)
	// the top face as four triangles around its centre, with the +X edge
	// split at its midpoint; the +X side face keeps that edge whole
	a, b, c, d := V(-10, -10, 10), V(10, -10, 10), V(10, 10, 10), V(-10, 10, 10)
	mid, ctr := V(10, 0, 10), V(0, 0, 10)
	m = without(m, top...)
	for _, q := range [][2]Vec3{{a, b}, {b, mid}, {mid, c}, {c, d}, {d, a}} {
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{ctr, q[0], q[1]}})
	}
	if closed(m) {
		t.Fatal("the test mesh should have a T-junction")
	}
	got := m.FixTJunctions()
	if !closed(got) {
		o, nm := got.EdgeCheck(0)
		t.Fatalf("not closed: %d open, %d non-manifold", o, nm)
	}
	if v := got.Volume(); math.Abs(v-8000) > 1e-9 {
		t.Errorf("volume %v, want 8000 exactly: a split moves nothing", v)
	}
	settled(t, got)
}

// Stage 3: a slit. The top face shifted sideways by 3e-4 mm, more than the
// boolean's weld (1e-5) and less than the stitch (1e-3): zipped shut.
func TestFixTJunctionsStitchesSlit(t *testing.T) {
	m := box20()
	for _, i := range topFace(m) {
		for k := range m.Tris[i].P {
			m.Tris[i].P[k] = m.Tris[i].P[k].Add(V(3e-4, 0, 0))
		}
	}
	if closed(m) {
		t.Fatal("the test mesh should be open")
	}
	got := m.FixTJunctions()
	if !closed(got) {
		o, nm := got.EdgeCheck(0)
		t.Fatalf("not closed: %d open, %d non-manifold", o, nm)
	}
	// the surface moved by no more than the slit's width
	if v := got.Volume(); math.Abs(v-8000) > 3e-4*400*2 {
		t.Errorf("volume %v, want about 8000", v)
	}
	settled(t, got)
}

// Stage 3: a stray sliver. A needle 1e-4 mm high hangs off an edge of the
// top face; its tip is spliced into that edge and the needle itself, now
// matching nothing, is dropped.
func TestFixTJunctionsDropsStraySliver(t *testing.T) {
	m := box20()
	a, b := V(-10, -10, 10), V(10, -10, 10) // an edge of the top face
	tip := V(2, -10+1e-4, 10)
	m.Tris = append(m.Tris, Tri{P: [3]Vec3{b, a, tip}})
	got := m.FixTJunctions()
	if !closed(got) {
		o, nm := got.EdgeCheck(0)
		t.Fatalf("not closed: %d open, %d non-manifold", o, nm)
	}
	if v := got.Volume(); math.Abs(v-8000) > 1e-2 {
		t.Errorf("volume %v, want about 8000", v)
	}
	for _, tr := range got.Tris {
		if sliver(tr, stitchTol) {
			t.Errorf("a sliver survived: %v", tr.P)
		}
	}
	settled(t, got)
}

// Stage 3: a back-to-back sheet. A top triangle doubled, once each way,
// folds every edge of it; one pair is cancelled and the mesh is closed.
func TestFixTJunctionsCancelsSheets(t *testing.T) {
	m := box20()
	tr := m.Tris[topFace(m)[0]]
	m.Tris = append(m.Tris, tr, Tri{P: [3]Vec3{tr.P[0], tr.P[2], tr.P[1]}})
	// EdgeCheck accepts an edge used twice each way; the repair doesn't
	if len(openEdgeVertices(m)) == 0 {
		t.Fatal("the test mesh should have folded edges")
	}
	got := m.FixTJunctions()
	if !closed(got) {
		o, nm := got.EdgeCheck(0)
		t.Fatalf("not closed: %d open, %d non-manifold", o, nm)
	}
	if len(got.Tris) != 12 {
		t.Errorf("%d triangles, want the box's 12", len(got.Tris))
	}
	if v := got.Volume(); math.Abs(v-8000) > 1e-9 {
		t.Errorf("volume %v, want 8000", v)
	}
}

// Stage 4: a flat hole. The top face missing altogether: a square loop,
// filled in its plane.
func TestFixTJunctionsFillsFlatHole(t *testing.T) {
	m := without(box20(), topFace(box20())...)
	got := m.FixTJunctions()
	if !closed(got) {
		o, nm := got.EdgeCheck(0)
		t.Fatalf("not closed: %d open, %d non-manifold", o, nm)
	}
	if v := got.Volume(); math.Abs(v-8000) > 1e-9 {
		t.Errorf("volume %v, want 8000", v)
	}
	settled(t, got)
}

// Stage 4: a flat hole with a reflex corner (five of the eight wedges of a
// fanned top face missing): ear clipping has to find the ears.
func TestFixTJunctionsFillsConcaveHole(t *testing.T) {
	m := without(box20(), topFace(box20())...)
	ctr := V(0, 0, 10)
	rim := []Vec3{V(10, -10, 10), V(10, 0, 10), V(10, 10, 10), V(0, 10, 10), V(-10, 10, 10), V(-10, 0, 10), V(-10, -10, 10), V(0, -10, 10)}
	for i := 0; i < 3; i++ { // wedges 0..2 present, 3..7 missing
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{ctr, rim[i], rim[(i+1)%8]}})
	}
	got := m.FixTJunctions()
	if !closed(got) {
		o, nm := got.EdgeCheck(0)
		t.Fatalf("not closed: %d open, %d non-manifold", o, nm)
	}
	if v := got.Volume(); math.Abs(v-8000) > 1e-9 {
		t.Errorf("volume %v, want 8000", v)
	}
	settled(t, got)
}

// Stage 4 must not guess: a hole spanning two faces (the top and one side
// missing) is not flat, so it stays open and nothing is added.
func TestFixTJunctionsLeavesNonFlatHoleOpen(t *testing.T) {
	m := box20()
	var side []int
	for i, tr := range m.Tris {
		if tr.P[0].X == 10 && tr.P[1].X == 10 && tr.P[2].X == 10 {
			side = append(side, i)
		}
	}
	m = without(m, append(topFace(m), side...)...)
	got := m.FixTJunctions()
	if closed(got) {
		t.Fatal("a hole across two planes was filled")
	}
	if len(got.Tris) != len(m.Tris) {
		t.Errorf("%d triangles, want the input's %d", len(got.Tris), len(m.Tris))
	}
}

// A slit narrower than slitTol is filled even where it isn't flat: a
// strip 5e-3 mm wide missing from the top face, bent over an edge of it.
func TestFixTJunctionsFillsNarrowSlit(t *testing.T) {
	m := box20()
	// the top face rebuilt leaving a strip along y = 0 open; the strip's
	// far ends are joined down the side faces, so the loop isn't flat
	const w = 5e-3
	a, b, c, d := V(-10, -10, 10), V(10, -10, 10), V(10, 10, 10), V(-10, 10, 10)
	p, q := V(10, -w/2, 10), V(-10, -w/2, 10)
	r, s := V(10, w/2, 10), V(-10, w/2, 10)
	m = without(m, topFace(m)...)
	m.Tris = append(m.Tris, Tri{P: [3]Vec3{a, b, p}}, Tri{P: [3]Vec3{a, p, q}})
	m.Tris = append(m.Tris, Tri{P: [3]Vec3{s, r, c}}, Tri{P: [3]Vec3{s, c, d}})
	// the side faces at x = ±10 split at the strip's ends, so the strip's
	// loop runs q-p along the top, then down and back on each side
	for _, x := range []float64{10, -10} {
		var old []int
		for i, tr := range m.Tris {
			if tr.P[0].X == x && tr.P[1].X == x && tr.P[2].X == x {
				old = append(old, i)
			}
		}
		m = without(m, old...)
		lo := V(x, 0, 9.99)
		e := func(y, z float64) Vec3 { return V(x, y, z) }
		corners := []Vec3{e(-10, -10), e(10, -10), e(10, 10), e(w/2, 10), lo, e(-w/2, 10), e(-10, 10)}
		if x < 0 {
			for i, j := 0, len(corners)-1; i < j; i, j = i+1, j-1 {
				corners[i], corners[j] = corners[j], corners[i]
			}
		}
		cc := e(0, 0)
		for i := range corners {
			m.Tris = append(m.Tris, Tri{P: [3]Vec3{cc, corners[i], corners[(i+1)%len(corners)]}})
		}
	}
	if closed(m) {
		t.Fatal("the test mesh should have a slit")
	}
	got := m.FixTJunctions()
	if !closed(got) {
		o, nm := got.EdgeCheck(0)
		t.Fatalf("not closed: %d open, %d non-manifold", o, nm)
	}
	if v := got.Volume(); math.Abs(v-8000) > 1 {
		t.Errorf("volume %v, want about 8000", v)
	}
	settled(t, got)
}

// The growth limit: a split pass that would fan thousands of points into
// every triangle on an edge gives up and returns its input unchanged.
func TestSplitPassBailsOnRunawayGrowth(t *testing.T) {
	m := &Mesh{}
	a, b := V(0, 0, 0), V(100, 0, 0)
	// ten big triangles all on the open edge a-b
	for i := 0; i < 10; i++ {
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{a, b, V(50, 10+float64(i), 0)}})
	}
	// and thousands of small ones each with a vertex on that edge
	for i := 1; i < 5000; i++ {
		x := 100 * float64(i) / 5000
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{V(x, 0, 0), V(x, -1, 0), V(x+0.005, -1, 0)}})
	}
	all := func(Vec3) bool { return true }
	got, changed := splitPass(m, csgEps, 2*csgEps, all)
	if changed || got != m {
		t.Fatalf("a runaway split was not refused (changed %v, %d triangles)", changed, len(got.Tris))
	}
}

// The repair's work is bounded. testdata/stitch-runaway.obj.gz is a design
// the torture test exported and re-imported: eight overlapping shells whose
// union is badly broken. Solidify unions them and repairs the result; a
// stitch that split the same unclosable edges again and again would double
// the mesh every pass. Each stitch round must close more than the best so
// far, and the mesh may never pass the repair's budget.
func TestFixTJunctionsIsBounded(t *testing.T) {
	f, err := os.Open("testdata/stitch-runaway.obj.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	m, err := ReadOBJ(data)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan *Mesh, 1)
	go func() { done <- m.Solidify() }()
	select {
	case got := <-done:
		// Solidify repairs a union of up to len(m.Tris) triangles
		if n, limit := len(got.Tris), repairBudget(4*len(m.Tris)); n > limit {
			t.Errorf("%d triangles from %d: the repair ran past its budget", n, len(m.Tris))
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Solidify still running after 30 s")
	}
	// and the repair on its own, of a mesh that stays open
	u := UnionAll(m.Shells())
	got := u.FixTJunctions()
	if len(got.Tris) > repairBudget(len(u.Tris)) {
		t.Errorf("%d triangles from %d: past the budget of %d", len(got.Tris), len(u.Tris), repairBudget(len(u.Tris)))
	}
	if o, _ := got.EdgeCheck(0); o > 0 {
		uo, _ := u.EdgeCheck(0)
		if o > uo {
			t.Errorf("the repair left %d open edges, more than the %d it was given", o, uo)
		}
	}
}

// Every stage sorts by one fixed order, so the fill is as deterministic as
// the split (TestFixTJunctionsIsDeterministic covers the split).
func TestFillHolesIsDeterministic(t *testing.T) {
	m := without(box20(), topFace(box20())...)
	first := meshPrint(m.FixTJunctions())
	for i := 0; i < 20; i++ {
		if meshPrint(m.FixTJunctions()) != first {
			t.Fatalf("run %d filled the hole differently", i+1)
		}
	}
}
