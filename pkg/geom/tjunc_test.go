package geom

import (
	"math"
	"testing"
)

// cyl builds a closed n-gon prism of radius r, height h centred at (cx, cy).
func cyl(cx, cy, r, h float64, n int) *Mesh {
	m := &Mesh{}
	for i := 0; i < n; i++ {
		a0 := 2 * math.Pi * float64(i) / float64(n)
		a1 := 2 * math.Pi * float64(i+1) / float64(n)
		p0 := V(cx+r*math.Cos(a0), cy+r*math.Sin(a0), 0)
		p1 := V(cx+r*math.Cos(a1), cy+r*math.Sin(a1), 0)
		q0, q1 := p0.Add(V(0, 0, h)), p1.Add(V(0, 0, h))
		c, ct := V(cx, cy, 0), V(cx, cy, h)
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{p0, p1, q1}}, Tri{P: [3]Vec3{p0, q1, q0}},
			Tri{P: [3]Vec3{c, p1, p0}}, Tri{P: [3]Vec3{ct, q0, q1}})
	}
	return m
}

func TestUnionBoxCylinderWatertight(t *testing.T) {
	b := box(V(0, 0, 10), V(20, 20, 20))
	c := cyl(10, 5, 10, 20, 48)
	u := Union(b, c).FixTJunctions()
	open, nm := u.EdgeCheck(1e-6)
	t.Logf("union: %d tris, open edges %d, non-manifold %d, volume %.3f", len(u.Tris), open, nm, u.Volume())
	if open != 0 || nm != 0 {
		t.Errorf("union is not watertight: %d open, %d non-manifold edges", open, nm)
	}
	s := Subtract(b, cyl(0, 0, 4, 30, 32).Transform(Translate(V(0, 0, -5)))).FixTJunctions()
	open, nm = s.EdgeCheck(1e-6)
	t.Logf("subtract: %d tris, open edges %d, non-manifold %d", len(s.Tris), open, nm)
	if open != 0 || nm != 0 {
		t.Errorf("difference is not watertight: %d open, %d non-manifold edges", open, nm)
	}
}
