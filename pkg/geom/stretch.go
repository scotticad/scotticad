package geom

import "math"

// StretchEps is how near a point must be to a stretch's plane to count as
// on it (mm). Points that near are moved onto it, so a face meant to lie
// in the plane does.
const StretchEps = 1e-6

// Stretch lengthens or shortens m along an axis (0 x, 1 y, 2 z) at the
// plane where that coordinate is at.
//
// amount > 0 inserts that much at the plane: what is beyond it (on the +
// side) moves amount along the axis, and the gap is filled with the cut
// outline carried straight across, so walls running along the axis just
// get longer. A face lying in the plane counts as cut there: a shape that
// ends at the plane is lengthened too, so shapes that meet there stay
// joined.
//
// amount < 0 removes a slab -amount thick centred on the plane, and moves
// what was beyond it back to close the gap. Where the outline at the
// slab's two faces differs, the result steps there.
//
// The low end (the - side) never moves. m is not changed.
func Stretch(m *Mesh, axis int, at, amount float64) *Mesh {
	if m == nil || len(m.Tris) == 0 || axis < 0 || axis > 2 || amount == 0 ||
		math.IsNaN(at+amount) || math.IsInf(at+amount, 0) {
		return m
	}
	b := m.Bounds()
	lo, hi := b.Min.Axis(axis), b.Max.Axis(axis)
	if amount > 0 {
		switch {
		case hi < at-StretchEps:
			return m // all below: stays
		case lo > at+StretchEps:
			return shiftAlong(m, axis, amount)
		}
		return stretchOut(m, axis, at, amount)
	}
	w := -amount
	p1, p2 := at-w/2, at+w/2
	switch {
	case hi <= p1+StretchEps:
		return m
	case lo >= p2-StretchEps:
		return shiftAlong(m, axis, -w)
	case lo >= p1-StretchEps && hi <= p2+StretchEps:
		return &Mesh{} // all in the slab
	}
	return stretchIn(m, axis, p1, p2)
}

// shiftAlong moves m d along axis.
func shiftAlong(m *Mesh, axis int, d float64) *Mesh {
	return m.Transform(Translate(V(0, 0, 0).WithAxis(axis, d)))
}

// snapped is t with every corner within StretchEps of the plane moved onto it.
func snapped(t Tri, axis int, p float64) Tri {
	for i, v := range t.P {
		if math.Abs(v.Axis(axis)-p) < StretchEps {
			t.P[i] = v.WithAxis(axis, p)
		}
	}
	return t
}

// planeCut is where edge a-b crosses the plane: worked out from the
// edge's ends in a fixed order, so the two triangles sharing the edge get
// the very same point.
func planeCut(a, b Vec3, axis int, p float64) Vec3 {
	if b.X < a.X || b.X == a.X && (b.Y < a.Y || b.Y == a.Y && b.Z < a.Z) {
		a, b = b, a
	}
	t := (p - a.Axis(axis)) / (b.Axis(axis) - a.Axis(axis))
	return a.Lerp(b, t).WithAxis(axis, p)
}

// cutTris splits triangles (already snapped) at the plane into those below
// it and those above. A triangle lying in the plane goes above when its
// normal points + and plusUp, or points - and not plusUp; below otherwise.
func cutTris(tris []Tri, axis int, p float64, plusUp bool) (below, above []Tri) {
	for _, t := range tris {
		var s [3]int
		neg, pos := false, false
		for i, v := range t.P {
			switch c := v.Axis(axis); {
			case c < p:
				s[i], neg = -1, true
			case c > p:
				s[i], pos = 1, true
			}
		}
		switch {
		case !neg && !pos: // in the plane
			n := t.P[1].Sub(t.P[0]).Cross(t.P[2].Sub(t.P[0])).Axis(axis)
			if n == 0 {
				continue // no area
			}
			if (n > 0) == plusUp {
				above = append(above, t)
			} else {
				below = append(below, t)
			}
		case !pos:
			below = append(below, t)
		case !neg:
			above = append(above, t)
		default:
			var lp, hp []Vec3
			for i := 0; i < 3; i++ {
				j := (i + 1) % 3
				if s[i] <= 0 {
					lp = append(lp, t.P[i])
				}
				if s[i] >= 0 {
					hp = append(hp, t.P[i])
				}
				if s[i]*s[j] < 0 {
					q := planeCut(t.P[i], t.P[j], axis, p)
					lp, hp = append(lp, q), append(hp, q)
				}
			}
			below = appendFan(below, lp, t.C)
			above = appendFan(above, hp, t.C)
		}
	}
	return below, above
}

// appendFan adds convex polygon ps as a fan of triangles.
func appendFan(out []Tri, ps []Vec3, c Color) []Tri {
	for i := 2; i < len(ps); i++ {
		out = append(out, Tri{P: [3]Vec3{ps[0], ps[i-1], ps[i]}, C: c})
	}
	return out
}

type stretchEdge [2]Vec3

// stretchOut inserts amount at the plane (see Stretch).
func stretchOut(m *Mesh, axis int, at, amount float64) *Mesh {
	tris := make([]Tri, len(m.Tris))
	for i, t := range m.Tris {
		tris[i] = snapped(t, axis, at)
	}
	below, above := cutTris(tris, axis, at, true)
	d := V(0, 0, 0).WithAxis(axis, amount)
	// the cut's outline: edges in the plane where a triangle that stays
	// meets one that moves. Each becomes a wall from where it stays to
	// where it moves to.
	stay := map[stretchEdge]int{}
	for _, t := range below {
		for i := 0; i < 3; i++ {
			a, b := t.P[i], t.P[(i+1)%3]
			if a.Axis(axis) == at && b.Axis(axis) == at {
				stay[stretchEdge{a, b}]++
			}
		}
	}
	out := &Mesh{Tris: make([]Tri, 0, len(below)+len(above)+64)}
	out.Tris = append(out.Tris, below...)
	var walls []Tri
	for _, t := range above {
		for i := 0; i < 3; i++ {
			v, u := t.P[i], t.P[(i+1)%3] // the stayer's edge runs u -> v
			if v.Axis(axis) != at || u.Axis(axis) != at || stay[stretchEdge{u, v}] == 0 {
				continue
			}
			stay[stretchEdge{u, v}]--
			u2, v2 := u.Add(d), v.Add(d)
			walls = append(walls, Tri{P: [3]Vec3{v, u, u2}, C: t.C}, Tri{P: [3]Vec3{v, u2, v2}, C: t.C})
		}
		for k := range t.P {
			t.P[k] = t.P[k].Add(d)
		}
		out.Tris = append(out.Tris, t)
	}
	out.Tris = append(out.Tris, walls...)
	return out
}

// stretchIn removes the slab between p1 and p2 and closes the gap (see
// Stretch).
func stretchIn(m *Mesh, axis int, p1, p2 float64) *Mesh {
	w := p2 - p1
	tris := make([]Tri, len(m.Tris))
	for i, t := range m.Tris {
		tris[i] = snapped(snapped(t, axis, p1), axis, p2)
	}
	// a face in a cut plane is kept with the side its solid is on
	near, rest := cutTris(tris, axis, p1, false)
	_, far := cutTris(rest, axis, p2, false)
	out := &Mesh{Tris: append([]Tri(nil), near...)}
	for _, t := range far {
		for k, v := range t.P {
			if c := v.Axis(axis); c == p2 {
				t.P[k] = v.WithAxis(axis, p1) // exactly onto the near side's cut
			} else {
				t.P[k] = v.WithAxis(axis, c-w)
			}
		}
		out.Tris = append(out.Tris, t)
	}
	// where the outline is the same at both cuts the two sides now meet
	// edge to edge (or, cut at different points along the same edges,
	// once those are joined), a closed solid with no boolean
	if open, _ := m.EdgeCheck(0); open != 0 {
		return out // it was open to begin with: nothing better to aim for
	}
	if open, bad := out.EdgeCheck(0); open == 0 && bad == 0 {
		return out
	}
	j := out.FixTJunctions()
	if open, bad := j.EdgeCheck(0); open == 0 && bad == 0 {
		return j
	}
	// the outlines differ: each side capped where it was cut, then joined
	b := m.Bounds()
	pad := V(1, 1, 1)
	lo, hi := b.Min.Sub(pad), b.Max.Add(pad)
	nearCap := Intersect(m, boxMesh(lo, hi.WithAxis(axis, p1)))
	farCap := Intersect(m, boxMesh(lo.WithAxis(axis, p2), hi))
	farCap = shiftAlong(farCap, axis, -w)
	switch {
	case len(nearCap.Tris) == 0:
		return farCap
	case len(farCap.Tris) == 0:
		return nearCap
	}
	return Union(nearCap, farCap).FixTJunctions()
}

// boxMesh is the box from lo to hi, facing out.
func boxMesh(lo, hi Vec3) *Mesh {
	c := func(i int) Vec3 {
		p := lo
		if i&1 != 0 {
			p.X = hi.X
		}
		if i&2 != 0 {
			p.Y = hi.Y
		}
		if i&4 != 0 {
			p.Z = hi.Z
		}
		return p
	}
	quads := [][4]int{{0, 2, 3, 1}, {4, 5, 7, 6}, {0, 1, 5, 4}, {2, 6, 7, 3}, {0, 4, 6, 2}, {1, 3, 7, 5}}
	m := &Mesh{}
	for _, q := range quads {
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{c(q[0]), c(q[1]), c(q[2])}}, Tri{P: [3]Vec3{c(q[0]), c(q[2]), c(q[3])}})
	}
	return m
}
