package geom

import (
	"math"
	"testing"
)

// A sliver triangle whose vertices sit further from its own plane than
// csgEps must not send tree building into endless recursion: such a
// polygon spans its own plane, so each split would make a piece that did
// the same.
func TestSliverTriangleInABoolean(t *testing.T) {
	sliver := &Mesh{Tris: []Tri{{P: [3]Vec3{V(0, 0, 0), V(10, 7, 3), V(20, 14, 6+1e-10)}}}}
	if len(toPolys(sliver, 0)) != 0 {
		t.Fatal("a sliver thinner than csgEps should be left out")
	}
	r := Subtract(cube(30, Color{}), Merge(cube(20, Color{}), sliver))
	if v := r.Volume(); math.Abs(v-19000) > 1 {
		t.Fatalf("volume %v, want 19000", v)
	}
	// a polygon is never split by its own plane, whatever its vertices say
	p := poly{v: []Vec3{V(0, 0, 0), V(1, 0, 0), V(0, 1, 1e-4)}, pl: plane{V(0, 0, 1), 0}}
	var cf, cb, f, b []poly
	p.pl.split(p, &cf, &cb, &f, &b)
	if len(cf) != 1 || len(cb)+len(f)+len(b) != 0 {
		t.Fatalf("split against its own plane: %d coplanar front, %d back, %d front, %d back", len(cf), len(cb), len(f), len(b))
	}
	p.pl.flipped().split(p, &cf, &cb, &f, &b)
	if len(cb) != 1 || len(f)+len(b) != 0 {
		t.Fatalf("split against its own plane, flipped: %d coplanar back, %d front, %d back", len(cb), len(f), len(b))
	}
}
