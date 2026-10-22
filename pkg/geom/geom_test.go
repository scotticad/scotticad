package geom

import (
	"bytes"
	"math"
	"testing"
)

func box(c, s Vec3) *Mesh {
	// Unit cube built from 12 triangles, then scaled/translated.
	p := [8]Vec3{}
	for i := range p {
		p[i] = V(float64(i&1)-0.5, float64(i>>1&1)-0.5, float64(i>>2&1)-0.5)
	}
	q := [][4]int{{0, 2, 3, 1}, {4, 5, 7, 6}, {0, 1, 5, 4}, {2, 6, 7, 3}, {0, 4, 6, 2}, {1, 3, 7, 5}}
	m := &Mesh{}
	for _, f := range q {
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{p[f[0]], p[f[1]], p[f[2]]}}, Tri{P: [3]Vec3{p[f[0]], p[f[2]], p[f[3]]}})
	}
	return m.Transform(Translate(c).Mul(Scale(s)))
}

func TestInverse(t *testing.T) {
	m := Translate(V(1, 2, 3)).Mul(RotateZ(0.7)).Mul(RotateX(0.3)).Mul(Scale(V(2, 3, 4)))
	inv, ok := m.Inverse()
	if !ok {
		t.Fatal("not invertible")
	}
	p := V(5, -2, 7)
	if q := inv.Point(m.Point(p)); q.Sub(p).Len() > 1e-9 {
		t.Fatalf("round trip %v != %v", q, p)
	}
}

func TestBoxVolume(t *testing.T) {
	b := box(V(0, 0, 0), V(20, 20, 20))
	if v := b.Volume(); math.Abs(v-8000) > 1e-6 {
		t.Fatalf("volume %v", v)
	}
	if o, n := b.EdgeCheck(1e-6); o != 0 || n != 0 {
		t.Fatalf("box not closed: %d open %d nm", o, n)
	}
}

func TestCSG(t *testing.T) {
	a := box(V(0, 0, 10), V(20, 20, 20))
	hole := box(V(0, 0, 10), V(10, 10, 30))
	if v := Subtract(a, hole).Volume(); math.Abs(v-6000) > 1e-3 {
		t.Fatalf("subtract volume %v", v)
	}
	b := box(V(10, 0, 10), V(20, 20, 20))
	if v := Union(a, b).Volume(); math.Abs(v-12000) > 1e-3 {
		t.Fatalf("union volume %v", v)
	}
	if v := Intersect(a, b).Volume(); math.Abs(v-4000) > 1e-3 {
		t.Fatalf("intersect volume %v", v)
	}
	// Coplanar: hole flush with the top face (the most common case).
	flush := box(V(0, 0, 15), V(10, 10, 10))
	if v := Subtract(a, flush).Volume(); math.Abs(v-7000) > 1e-3 {
		t.Fatalf("flush subtract volume %v", v)
	}
	// Touching boxes union.
	c := box(V(20, 0, 10), V(20, 20, 20))
	if v := Union(a, c).Volume(); math.Abs(v-16000) > 1e-3 {
		t.Fatalf("touching union volume %v", v)
	}
}

func TestSTLRoundTrip(t *testing.T) {
	a := box(V(1, 2, 3), V(4, 5, 6))
	var buf bytes.Buffer
	if err := WriteSTL(&buf, a, "t"); err != nil {
		t.Fatal(err)
	}
	m, err := ReadSTL(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Tris) != 12 || math.Abs(m.Volume()-120) > 1e-3 {
		t.Fatalf("got %d tris volume %v", len(m.Tris), m.Volume())
	}
}

func TestOBJRoundTrip(t *testing.T) {
	a := box(V(0, 0, 0), V(2, 2, 2))
	var buf, mtl bytes.Buffer
	if err := WriteOBJ(&buf, &mtl, a, "x.mtl"); err != nil {
		t.Fatal(err)
	}
	m, err := ReadOBJ(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(m.Volume()-8) > 1e-9 {
		t.Fatalf("volume %v", m.Volume())
	}
}

func TestRay(t *testing.T) {
	a := box(V(0, 0, 10), V(20, 20, 20))
	r := Ray{V(0, 0, 100), V(0, 0, -1)}
	d, n, ok := r.Mesh(a)
	if !ok || math.Abs(d-80) > 1e-9 || n.Z < 0.99 {
		t.Fatalf("hit %v %v %v", d, n, ok)
	}
}

func TestRotateFromTo(t *testing.T) {
	dirs := []Vec3{V(1, 0, 0), V(-1, 0, 0), V(0, 1, 0), V(0, 0, 1), V(0, 0, -1), V(1, 2, -3).Norm(), V(-0.3, 0.1, 0.9).Norm()}
	for _, f := range dirs {
		for _, to := range dirs {
			m := RotateFromTo(f, to)
			if got := m.Dir(f); got.Sub(to).Len() > 1e-9 {
				t.Fatalf("%v onto %v gave %v", f, to, got)
			}
			if d := m.Det3(); math.Abs(d-1) > 1e-9 {
				t.Fatalf("not a rotation: det %v", d)
			}
		}
	}
}
