package geom

import "testing"

// The ray-crossing inside test the booleans use to overrule the BSP tree
// where a fragment lies clear of the other operand's surface.

func TestInsideTestBox(t *testing.T) {
	it := newInsideTest(box(V(0, 0, 0), V(20, 20, 20)))
	for _, c := range []struct {
		p    Vec3
		want int
	}{
		{V(0, 0, 0), 1},
		{V(9.9, -9.9, 9.9), 1},
		{V(10.1, 0, 0), 0},
		{V(0, 0, 10.1), 0},
		{V(100, 100, 100), 0}, // outside the mesh's shadow altogether
		{V(0, 0, 10), -1},     // on a face: no answer
		{V(0, 0, 10-1e-5), -1},
		{V(10, 10, 10), -1}, // a corner
	} {
		if got := it.inside(c.p); got != c.want {
			t.Errorf("inside(%v) = %d, want %d", c.p, got, c.want)
		}
	}
}

// Overlapping shells count twice; an inside-out shell counts as outside.
func TestInsideTestShells(t *testing.T) {
	two := Merge(box(V(0, 0, 0), V(20, 20, 20)), box(V(10, 0, 0), V(20, 20, 20)))
	it := newInsideTest(two)
	for _, c := range []struct {
		p    Vec3
		want int
	}{
		{V(5, 0, 0), 1},  // in both
		{V(-5, 0, 0), 1}, // in one
		{V(15, 0, 0), 1}, // in the other
		{V(25, 0, 0), 0},
	} {
		if got := it.inside(c.p); got != c.want {
			t.Errorf("inside(%v) = %d, want %d", c.p, got, c.want)
		}
	}
	if got := newInsideTest(box(V(0, 0, 0), V(20, 20, 20)).Flipped()).inside(V(0, 0, 0)); got != 0 {
		t.Errorf("inside an inside-out box = %d, want 0", got)
	}
}

// A ray through an edge shared by two faces crosses it once: a point
// inside, cast up through the diagonal of the top face, has winding 1.
func TestInsideTestCountsSharedEdgeOnce(t *testing.T) {
	m := box(V(0, 0, 0), V(20, 20, 20))
	it := newInsideTest(m)
	// the diagonal of the top face, found from the +Z triangles
	var diag [2]Vec3
	for _, tr := range m.Tris {
		if tr.P[0].Z == 10 && tr.P[1].Z == 10 && tr.P[2].Z == 10 {
			for k := 0; k < 3; k++ {
				a, b := tr.P[k], tr.P[(k+1)%3]
				if a.X != b.X && a.Y != b.Y {
					diag = [2]Vec3{a, b}
				}
			}
		}
	}
	mid := diag[0].Add(diag[1]).Mul(0.5)
	for _, z := range []float64{0, -5, 9} {
		w, near := it.cast(&it.ax[2], V(mid.X, mid.Y, z))
		if near || w != 1 {
			t.Errorf("cast up through the diagonal from z=%g: winding %d near %v, want 1 false", z, w, near)
		}
	}
	// and a corner: the four top triangles' shared point is crossed once
	w, near := it.cast(&it.ax[2], V(10, 10, 0))
	if near || w != 1 {
		t.Errorf("cast up through a corner: winding %d near %v, want 1 false", w, near)
	}
}

// A mesh that isn't a solid (one triangle) gives no answer near it, and
// outside anywhere far from it. An empty mesh never panics.
func TestInsideTestDegenerate(t *testing.T) {
	one := &Mesh{Tris: []Tri{{P: [3]Vec3{V(0, 0, 0), V(10, 0, 0), V(0, 10, 0)}}}}
	it := newInsideTest(one)
	// from below, the ray up crosses it (winding 1) while the rays along
	// X and Y see it edge-on (0): no answer
	if got := it.inside(V(2, 2, -1)); got != -1 {
		t.Errorf("below a lone triangle: %d, want -1 (the rays disagree)", got)
	}
	if got := it.inside(V(50, 50, 50)); got != 0 {
		t.Errorf("far from a lone triangle: %d, want 0", got)
	}
	empty := newInsideTest(&Mesh{})
	if got := empty.inside(V(0, 0, 0)); got != -1 {
		t.Errorf("an empty mesh: %d, want -1", got)
	}
}

// The test only overrules the tree where it is sure: a boolean whose
// operands' faces coincide still keeps each shared face once.
func TestInsideTestKeepsCoplanarFacesOnce(t *testing.T) {
	a := box(V(0, 0, 0), V(20, 20, 20))
	b := box(V(10, 0, 0), V(20, 20, 20)) // shares the planes y, z = ±10
	for name, m := range map[string]*Mesh{"union": Union(a, b), "intersect": Intersect(a, b), "subtract": Subtract(a, b)} {
		if o, nm := m.FixTJunctions().EdgeCheck(1e-7); o != 0 || nm != 0 {
			t.Errorf("%s of boxes sharing face planes: %d open, %d non-manifold", name, o, nm)
		}
	}
	if v := Union(a, b).Volume(); v < 12000-1e-6 || v > 12000+1e-6 {
		t.Errorf("union volume %v, want 12000", v)
	}
}
