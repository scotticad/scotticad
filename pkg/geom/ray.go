package geom

import "math"

// Ray is a half-line O + t*D, t >= 0.
type Ray struct{ O, D Vec3 }

func (r Ray) At(t float64) Vec3 { return r.O.Add(r.D.Mul(t)) }

// Tri intersects a triangle (two-sided). It returns the distance and true
// on a hit.
func (r Ray) Tri(a, b, c Vec3) (float64, bool) {
	e1, e2 := b.Sub(a), c.Sub(a)
	p := r.D.Cross(e2)
	det := e1.Dot(p)
	if math.Abs(det) < 1e-14 {
		return 0, false
	}
	inv := 1 / det
	s := r.O.Sub(a)
	u := s.Dot(p) * inv
	if u < 0 || u > 1 {
		return 0, false
	}
	q := s.Cross(e1)
	v := r.D.Dot(q) * inv
	if v < 0 || u+v > 1 {
		return 0, false
	}
	t := e2.Dot(q) * inv
	if t <= 1e-9 {
		return 0, false
	}
	return t, true
}

// Mesh returns the nearest hit on a mesh, its triangle normal, and true.
func (r Ray) Mesh(m *Mesh) (float64, Vec3, bool) { return r.MeshWhere(m, nil) }

// MeshWhere is Mesh counting only hits at distances keep accepts (e.g.
// points not cut away by a section plane); nil accepts all.
func (r Ray) MeshWhere(m *Mesh, keep func(t float64) bool) (float64, Vec3, bool) {
	best := math.Inf(1)
	var n Vec3
	for _, t := range m.Tris {
		if d, ok := r.Tri(t.P[0], t.P[1], t.P[2]); ok && d < best && (keep == nil || keep(d)) {
			best = d
			n = t.Normal()
		}
	}
	return best, n, !math.IsInf(best, 1)
}

// Box intersects an AABB (slab method) and returns the entry distance.
func (r Ray) Box(b Box) (float64, bool) {
	tmin, tmax := math.Inf(-1), math.Inf(1)
	for i := 0; i < 3; i++ {
		o, d := r.O.Axis(i), r.D.Axis(i)
		lo, hi := b.Min.Axis(i), b.Max.Axis(i)
		if math.Abs(d) < 1e-14 {
			if o < lo || o > hi {
				return 0, false
			}
			continue
		}
		t1, t2 := (lo-o)/d, (hi-o)/d
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tmin, tmax = math.Max(tmin, t1), math.Min(tmax, t2)
		if tmin > tmax {
			return 0, false
		}
	}
	if tmax < 0 {
		return 0, false
	}
	return math.Max(tmin, 0), true
}

// Plane intersects the plane through p with normal n.
func (r Ray) Plane(p, n Vec3) (float64, bool) {
	d := r.D.Dot(n)
	if math.Abs(d) < 1e-12 {
		return 0, false
	}
	t := p.Sub(r.O).Dot(n) / d
	if t < 0 {
		return 0, false
	}
	return t, true
}

// PlaneZ intersects the horizontal plane at height z.
func (r Ray) PlaneZ(z float64) (float64, bool) { return r.Plane(V(0, 0, z), V(0, 0, 1)) }

// ClosestOnLine returns the parameter s of the point p + s*u nearest to
// the ray (treated as an infinite line).
func (r Ray) ClosestOnLine(p, u Vec3) float64 {
	w0 := p.Sub(r.O)
	a, b, c := u.Dot(u), u.Dot(r.D), r.D.Dot(r.D)
	d, e := u.Dot(w0), r.D.Dot(w0)
	den := a*c - b*b
	if math.Abs(den) < 1e-12 {
		return 0
	}
	return (b*e - c*d) / den
}
