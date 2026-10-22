package geom

import (
	"math"
	"testing"
)

func TestShells(t *testing.T) {
	b := box(V(0, 0, 0), V(20, 20, 20))
	if n := len(b.Shells()); n != 1 {
		t.Errorf("a box: %d shells, want 1", n)
	}
	far := Merge(b, box(V(50, 0, 0), V(5, 5, 5)))
	if s := far.Shells(); len(s) != 2 || len(s[0].Tris) != 12 || len(s[1].Tris) != 12 {
		t.Errorf("two boxes apart: %d shells", len(s))
	}
	// touching at one corner only: still two bodies
	corner := Merge(b, box(V(20, 20, 20), V(20, 20, 20)))
	if n := len(corner.Shells()); n != 2 {
		t.Errorf("two boxes touching at a corner: %d shells, want 2", n)
	}
	// sharing an edge: one body
	edge := Merge(b, box(V(20, 20, 0), V(20, 20, 20)))
	if n := len(edge.Shells()); n != 1 {
		t.Errorf("two boxes sharing an edge: %d shells, want 1", n)
	}
	if n := len((&Mesh{}).Shells()); n != 0 {
		t.Errorf("an empty mesh: %d shells, want 0", n)
	}
}

func TestSolidifyLeavesSimpleMeshesAlone(t *testing.T) {
	b := box(V(0, 0, 0), V(20, 20, 20))
	if b.Solidify() != b {
		t.Error("a single shell was rebuilt")
	}
	far := Merge(b, box(V(50, 0, 0), V(5, 5, 5)))
	if far.Solidify() != far {
		t.Error("two shells apart were rebuilt")
	}
	// a solid with a cavity: one solid shell, nothing to union
	hollow := Merge(b, box(V(0, 0, 0), V(10, 10, 10)).Flipped())
	if hollow.Solidify() != hollow {
		t.Error("a hollow solid was rebuilt")
	}
	if v := hollow.Volume(); math.Abs(v-7000) > 1e-9 {
		t.Errorf("hollow volume %v, want 7000", v)
	}
}

func TestSolidifyUnionsOverlappingShells(t *testing.T) {
	m := Merge(box(V(0, 0, 0), V(20, 20, 20)), box(V(10, 0, 0), V(20, 20, 20)))
	got := m.Solidify()
	if o, nm := got.EdgeCheck(0); o != 0 || nm != 0 {
		t.Errorf("not closed: %d open, %d non-manifold", o, nm)
	}
	if n := len(got.Shells()); n != 1 {
		t.Errorf("%d shells, want 1", n)
	}
	if v := got.Volume(); math.Abs(v-12000) > 1e-6 {
		t.Errorf("volume %v, want 12000 (16000 less the 4000 shared)", v)
	}
}

func TestSolidifyDropsSheets(t *testing.T) {
	sheet := &Mesh{Tris: []Tri{
		{P: [3]Vec3{V(30, 0, 0), V(35, 0, 0), V(30, 5, 0)}},
		{P: [3]Vec3{V(30, 0, 0), V(30, 5, 0), V(35, 0, 0)}},
	}}
	// shells apart plus a sheet: the sheet goes, the rest is untouched
	far := Merge(box(V(0, 0, 0), V(20, 20, 20)), box(V(60, 0, 0), V(5, 5, 5)), sheet)
	got := far.Solidify()
	if len(got.Tris) != 24 || len(got.Shells()) != 2 {
		t.Errorf("%d triangles in %d shells, want 24 in 2", len(got.Tris), len(got.Shells()))
	}
	// overlapping shells plus a sheet: unioned, the sheet gone
	over := Merge(box(V(0, 0, 0), V(20, 20, 20)), box(V(10, 0, 0), V(20, 20, 20)), sheet)
	got = over.Solidify()
	if o, nm := got.EdgeCheck(0); o != 0 || nm != 0 || len(got.Shells()) != 1 {
		t.Errorf("not one closed shell: %d open, %d non-manifold, %d shells", o, nm, len(got.Shells()))
	}
}

func TestSolidifyCutsCavities(t *testing.T) {
	m := Merge(
		box(V(0, 0, 0), V(20, 20, 20)),
		box(V(10, 0, 0), V(20, 20, 20)),
		box(V(-5, 0, 0), V(4, 4, 4)).Flipped(), // a void in the first box, clear of the overlap
	)
	got := m.Solidify()
	if o, nm := got.EdgeCheck(0); o != 0 || nm != 0 {
		t.Errorf("not closed: %d open, %d non-manifold", o, nm)
	}
	if v := got.Volume(); math.Abs(v-(12000-64)) > 1e-6 {
		t.Errorf("volume %v, want 11936", v)
	}
}

// A solid lying loose inside a cavity (a loose part in a hollow) is kept:
// unioned with the rest it would have been cut away with the cavity.
func TestSolidifyKeepsLooseSolidInCavity(t *testing.T) {
	m := Merge(
		box(V(0, 0, 0), V(30, 30, 30)),
		box(V(0, 0, 0), V(20, 20, 20)).Flipped(), // the hollow
		box(V(0, 0, 0), V(5, 5, 5)),              // loose inside it
		box(V(24, 0, 0), V(20, 20, 20)),          // overlapping the wall, so the union path runs
	)
	got := m.Solidify()
	if o, nm := got.EdgeCheck(0); o != 0 || nm != 0 {
		t.Errorf("not closed: %d open, %d non-manifold", o, nm)
	}
	// 27000 + 8000 - 400 overlapping, less the 8000 hollow, plus the 125 loose
	if v := got.Volume(); math.Abs(v-26725) > 1e-6 {
		t.Errorf("volume %v, want 26725", v)
	}
	if n := len(got.Shells()); n != 3 {
		t.Errorf("%d shells, want 3 (the hollow solid's outside and inside, and the loose part)", n)
	}
}

// A shell that isn't closed is passed through as it is.
func TestSolidifyPassesOpenShellsThrough(t *testing.T) {
	open := without(box(V(0, 0, 0), V(20, 20, 20)), 0)
	m := Merge(open, box(V(10, 0, 0), V(20, 20, 20)), box(V(50, 0, 0), V(5, 5, 5)))
	got := m.Solidify()
	if got != m {
		t.Error("one closed solid and an open shell: nothing to union, yet the mesh was rebuilt")
	}
	m = Merge(open, box(V(60, 0, 0), V(20, 20, 20)), box(V(70, 0, 0), V(20, 20, 20)))
	got = m.Solidify()
	o, _ := got.EdgeCheck(0)
	if o != 3 {
		t.Errorf("%d open edges, want the open shell's 3", o)
	}
	if len(got.Shells()) != 2 {
		t.Errorf("%d shells, want 2", len(got.Shells()))
	}
}
