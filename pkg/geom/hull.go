package geom

import "math"

// ConvexHull returns the convex hull of pts as a closed mesh facing
// outwards: the smallest shape with no dents that holds them all. Its
// corners are points of pts, unchanged. It returns nil when the points
// have no volume between them (fewer than four, or all on one plane).
//
// Points within a hundred-millionth of the set's size of a face count as
// on it, so the rounding errors of points meant to share a plane (a box's
// face, a cylinder's end) don't raise slivers; the hull may leave a point
// outside by that much.
func ConvexHull(pts []Vec3) *Mesh {
	h := newHull(pts)
	if h == nil {
		return nil
	}
	h.build()
	m := &Mesh{}
	for _, f := range h.faces {
		if !f.dead {
			m.Tris = append(m.Tris, Tri{P: [3]Vec3{h.orig[f.v[0]], h.orig[f.v[1]], h.orig[f.v[2]]}})
		}
	}
	// built face by face it is closed by construction; a check that costs
	// little guards the callers against a surface that isn't
	if open, bad := m.EdgeCheck(0); open != 0 || bad != 0 || !(m.Volume() > 0) {
		return nil
	}
	return m
}

// hull is the state of one quickhull run: the hull so far is a closed
// surface of triangles, each knowing its three neighbours and the points
// still outside it.
type hull struct {
	orig  []Vec3 // the points as given (welded)
	p     []Vec3 // the same about the set's centre, for the arithmetic
	eps   float64
	faces []*hullFace
	stamp int
}

type hullFace struct {
	v    [3]int
	n    Vec3 // unit normal, outwards
	d    float64
	nb   [3]*hullFace // across the edge v[i] -> v[i+1]
	out  []int        // points outside this face
	far  int          // the farthest of them
	farD float64
	dead bool
	seen int // the stamp of the search that last found it visible
}

func (f *hullFace) dist(p Vec3) float64 { return f.n.Dot(p) - f.d }

// newHull welds the points and makes the first tetrahedron; nil when the
// points are flat.
func newHull(pts []Vec3) *hull {
	b := EmptyBox()
	for _, p := range pts {
		if finiteVec(p) {
			b = b.Extend(p)
		}
	}
	if b.IsEmpty() {
		return nil
	}
	size := b.Size().Len()
	if !(size > 0) || math.IsInf(size, 0) {
		return nil
	}
	h := &hull{eps: 1e-8 * size}
	// points a rounding error apart are one point: the first given is kept
	weld := 1e-7 * size
	c := b.Center()
	seen := make(map[[3]int64]struct{}, len(pts))
	for _, p := range pts {
		if !finiteVec(p) {
			continue
		}
		q := p.Sub(c)
		k := [3]int64{int64(math.Round(q.X / weld)), int64(math.Round(q.Y / weld)), int64(math.Round(q.Z / weld))}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		h.orig = append(h.orig, p)
		h.p = append(h.p, q)
	}
	if len(h.p) < 4 {
		return nil
	}
	// the first tetrahedron: the two ends of the longest axis, the point
	// farthest from the line through them, the point farthest from the
	// plane through all three
	ax := 0
	for i := 1; i < 3; i++ {
		if b.Size().Axis(i) > b.Size().Axis(ax) {
			ax = i
		}
	}
	i0, i1 := 0, 0
	for i, p := range h.p {
		if p.Axis(ax) < h.p[i0].Axis(ax) {
			i0 = i
		}
		if p.Axis(ax) > h.p[i1].Axis(ax) {
			i1 = i
		}
	}
	line := h.p[i1].Sub(h.p[i0]).Norm()
	i2, best := -1, h.eps
	for i, p := range h.p {
		if d := p.Sub(h.p[i0]).Cross(line).Len(); d > best {
			i2, best = i, d
		}
	}
	if i2 < 0 {
		return nil
	}
	n := h.p[i1].Sub(h.p[i0]).Cross(h.p[i2].Sub(h.p[i0])).Norm()
	i3, best := -1, h.eps
	for i, p := range h.p {
		if d := math.Abs(p.Sub(h.p[i0]).Dot(n)); d > best {
			i3, best = i, d
		}
	}
	if i3 < 0 {
		return nil
	}
	if h.p[i3].Sub(h.p[i0]).Dot(n) > 0 {
		i1, i2 = i2, i1 // the fourth point goes below the first face
	}
	for _, v := range [][3]int{{i0, i1, i2}, {i0, i3, i1}, {i1, i3, i2}, {i2, i3, i0}} {
		f := h.face(v[0], v[1], v[2])
		if f == nil {
			return nil
		}
		h.faces = append(h.faces, f)
	}
	// neighbours: the face with the same edge the other way round
	type edge [2]int
	at := map[edge]*hullFace{}
	for _, f := range h.faces {
		for i := 0; i < 3; i++ {
			at[edge{f.v[i], f.v[(i+1)%3]}] = f
		}
	}
	for _, f := range h.faces {
		for i := 0; i < 3; i++ {
			f.nb[i] = at[edge{f.v[(i+1)%3], f.v[i]}]
		}
	}
	all := make([]int, 0, len(h.p))
	for i := range h.p {
		if i != i0 && i != i1 && i != i2 && i != i3 {
			all = append(all, i)
		}
	}
	h.assign(all, h.faces)
	return h
}

// face makes the triangle a, b, c with its plane; nil if it has no area.
func (h *hull) face(a, b, c int) *hullFace {
	n := h.p[b].Sub(h.p[a]).Cross(h.p[c].Sub(h.p[a]))
	l := n.Len()
	if !(l > 0) {
		return nil
	}
	n = n.Mul(1 / l)
	// the plane through the corners' mean: the least error at each
	d := (n.Dot(h.p[a]) + n.Dot(h.p[b]) + n.Dot(h.p[c])) / 3
	return &hullFace{v: [3]int{a, b, c}, n: n, d: d, far: -1}
}

// assign gives each point to the first of the faces it is outside; a point
// outside none is inside the hull and is dropped.
func (h *hull) assign(pts []int, faces []*hullFace) {
	for _, i := range pts {
		for _, f := range faces {
			if d := f.dist(h.p[i]); d > h.eps {
				f.out = append(f.out, i)
				if f.far < 0 || d > f.farD {
					f.far, f.farD = i, d
				}
				break
			}
		}
	}
}

// horizonEdge is an edge between a face the new point sees and one it
// doesn't: a -> b as the seen face has it, and the unseen face beyond.
type horizonEdge struct {
	a, b   int
	beyond *hullFace
}

// build adds the outside points one at a time, the farthest from a face
// first: the faces the point sees are taken out, and new ones join it to
// the edge of the gap.
func (h *hull) build() {
	var work []*hullFace
	for _, f := range h.faces {
		if len(f.out) > 0 {
			work = append(work, f)
		}
	}
	for len(work) > 0 {
		f := work[len(work)-1]
		if f.dead || len(f.out) == 0 {
			work = work[:len(work)-1]
			continue
		}
		pi := f.far
		p := h.p[pi]
		// the faces p sees, spreading from f. A face p is level with
		// (within eps either way) is taken too, so every new face stands
		// clear of the face beyond its edge
		h.stamp++
		f.seen = h.stamp
		visible := []*hullFace{f}
		var horizon []horizonEdge
		for k := 0; k < len(visible); k++ {
			g := visible[k]
			for i := 0; i < 3; i++ {
				nb := g.nb[i]
				if nb.seen == h.stamp {
					continue
				}
				if nb.dist(p) > -h.eps {
					nb.seen = h.stamp
					visible = append(visible, nb)
					continue
				}
				horizon = append(horizon, horizonEdge{g.v[i], g.v[(i+1)%3], nb})
			}
		}
		fresh := h.cone(horizon, pi)
		if fresh == nil {
			// the gap's edge isn't one loop, or a new face would have no
			// area: rounding at its limit. The point is within eps of the
			// hull as it stands, and is left out
			h.drop(f, pi)
			continue
		}
		var orphans []int
		for _, g := range visible {
			g.dead = true
			for _, i := range g.out {
				if i != pi {
					orphans = append(orphans, i)
				}
			}
			g.out = nil
		}
		h.faces = append(h.faces, fresh...)
		h.assign(orphans, fresh)
		work = work[:len(work)-1]
		for _, g := range fresh {
			if len(g.out) > 0 {
				work = append(work, g)
			}
		}
	}
}

// cone makes the faces from the horizon's edges to point pi, joined to each
// other and to the faces beyond; nil if the horizon isn't a single loop or
// a face has no area (nothing is changed then).
func (h *hull) cone(horizon []horizonEdge, pi int) []*hullFace {
	if len(horizon) < 3 {
		return nil
	}
	from := make(map[int]int, len(horizon)) // start corner -> edge
	for i, e := range horizon {
		if _, dup := from[e.a]; dup {
			return nil
		}
		from[e.a] = i
	}
	order := make([]int, 0, len(horizon))
	i := 0
	for n := 0; n < len(horizon); n++ {
		order = append(order, i)
		next, ok := from[horizon[i].b]
		if !ok {
			return nil
		}
		i = next
		if i == 0 && n != len(horizon)-1 {
			return nil // a shorter loop: the horizon is more than one
		}
	}
	if i != 0 {
		return nil // it never came back to its start
	}
	fresh := make([]*hullFace, len(order))
	for k, i := range order {
		e := horizon[i]
		f := h.face(e.a, e.b, pi)
		if f == nil {
			return nil
		}
		fresh[k] = f
	}
	for k, i := range order {
		e := horizon[i]
		f := fresh[k]
		f.nb[0] = e.beyond
		f.nb[1] = fresh[(k+1)%len(fresh)]
		f.nb[2] = fresh[(k+len(fresh)-1)%len(fresh)]
		for j := 0; j < 3; j++ {
			if e.beyond.v[j] == e.b && e.beyond.v[(j+1)%3] == e.a {
				e.beyond.nb[j] = f
			}
		}
	}
	return fresh
}

// drop takes point pi out of f's outside points.
func (h *hull) drop(f *hullFace, pi int) {
	keep := f.out[:0]
	f.far = -1
	for _, i := range f.out {
		if i == pi {
			continue
		}
		keep = append(keep, i)
		if d := f.dist(h.p[i]); f.far < 0 || d > f.farD {
			f.far, f.farD = i, d
		}
	}
	f.out = keep
}

func finiteVec(p Vec3) bool {
	return !math.IsNaN(p.X+p.Y+p.Z) && !math.IsInf(p.X+p.Y+p.Z, 0)
}
