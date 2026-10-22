package geom

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// uvSphere is a closed sphere of radius r with n segments around and n/2
// down: about n² triangles.
func uvSphere(r float64, n int) *Mesh {
	m := &Mesh{}
	pt := func(i, j int) Vec3 {
		th := math.Pi * float64(j) / float64(n/2)
		ph := 2 * math.Pi * float64(i) / float64(n)
		return V(r*math.Sin(th)*math.Cos(ph), r*math.Sin(th)*math.Sin(ph), r*math.Cos(th))
	}
	for j := 0; j < n/2; j++ {
		for i := 0; i < n; i++ {
			a, b, c, d := pt(i, j), pt(i+1, j), pt(i+1, j+1), pt(i, j+1)
			if j > 0 {
				m.Tris = append(m.Tris, Tri{P: [3]Vec3{a, c, b}})
			}
			if j < n/2-1 {
				m.Tris = append(m.Tris, Tri{P: [3]Vec3{a, d, c}})
			}
		}
	}
	return m.Oriented()
}

// Divider planes (used on big convex surfaces) change only how the work is
// split up: every boolean gives the same solid with or without them, and
// stays watertight.
func TestDividersDontChangeResults(t *testing.T) {
	saved := DividerMin
	defer func() { DividerMin = saved }()
	sphere := uvSphere(10, 48)
	box := cube(12, Color{})
	rot := RotateAround(V(1, 2, 3).Norm(), 0.7)
	cases := []struct {
		name string
		a, b *Mesh
	}{
		{"sphere, box", sphere, box},
		{"sphere, rotated box", sphere, box.Transform(rot)},
		{"sphere, shifted sphere", sphere, uvSphere(8, 40).Transform(Translate(V(9, 1, 0.5)))},
		{"box, sphere", cube(18, Color{}), sphere.Transform(Translate(V(6, 4, 3)))},
		{"sphere, thin slab", sphere, cube(1, Color{}).Transform(Scale(V(30, 30, 0.6)))},
	}
	ops := map[string]func(a, b *Mesh) *Mesh{"union": Union, "subtract": Subtract, "intersect": Intersect}
	for _, c := range cases {
		for name, op := range ops {
			DividerMin = 1 << 30
			want := op(c.a, c.b)
			DividerMin = 8
			got := op(c.a, c.b)
			label := fmt.Sprintf("%s: %s", c.name, name)
			if math.Abs(got.Volume()-want.Volume()) > 1e-6*math.Max(1, math.Abs(want.Volume())) {
				t.Errorf("%s: volume %v with dividers, %v without", label, got.Volume(), want.Volume())
			}
			if o, n := got.FixTJunctions().EdgeCheck(1e-7); o != 0 || n != 0 {
				t.Errorf("%s: %d open, %d non-manifold edges with dividers", label, o, n)
			}
		}
	}
}

// A boolean on a smooth 40,000-triangle mesh takes a fraction of a second
// (about 18 s if a convex surface makes the tree a chain).
func TestBooleanOnASmoothMeshIsFast(t *testing.T) {
	s := uvSphere(10, 200)
	c := cube(8, Color{}).Transform(Scale(V(1, 1, 4)))
	start := time.Now()
	m := Subtract(s, c)
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("subtracting from %d triangles took %v", len(s.Tris), d)
	}
	if o, n := m.FixTJunctions().EdgeCheck(1e-7); o != 0 || n != 0 {
		t.Fatalf("%d open, %d non-manifold edges", o, n)
	}
}
