package shapes

import (
	"math"
	"sort"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Pt is a 2D point.
type Pt struct{ X, Y float64 }

func cross2(o, a, b Pt) float64 { return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X) }

// SignedArea is positive for counter-clockwise contours.
func SignedArea(c []Pt) float64 {
	var a float64
	for i := range c {
		j := (i + 1) % len(c)
		a += c[i].X*c[j].Y - c[j].X*c[i].Y
	}
	return a / 2
}

func reversed(c []Pt) []Pt {
	r := make([]Pt, len(c))
	for i := range c {
		r[i] = c[len(c)-1-i]
	}
	return r
}

// PointInPolygon uses the even-odd rule.
func PointInPolygon(p Pt, c []Pt) bool {
	in := false
	for i, j := 0, len(c)-1; i < len(c); j, i = i, i+1 {
		a, b := c[i], c[j]
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in
}

// windingNumber is how many times the closed outline c goes round p,
// anticlockwise counting up: PointInPolygon's crossings, each counted by
// its edge's direction. Nonzero is inside for the nonzero fill rule (a
// path that goes round a place twice still fills it).
func windingNumber(p Pt, c []Pt) int {
	w := 0
	for i, j := 0, len(c)-1; i < len(c); j, i = i, i+1 {
		a, b := c[i], c[j]
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)+a.X {
			if a.Y > p.Y { // the edge j -> i runs up past p
				w++
			} else {
				w--
			}
		}
	}
	return w
}

// cleanContour removes consecutive duplicates and collinear points.
func cleanContour(c []Pt) []Pt {
	var out []Pt
	for _, p := range c {
		if len(out) > 0 && math.Abs(out[len(out)-1].X-p.X) < 1e-9 && math.Abs(out[len(out)-1].Y-p.Y) < 1e-9 {
			continue
		}
		out = append(out, p)
	}
	for len(out) > 1 && math.Abs(out[0].X-out[len(out)-1].X) < 1e-9 && math.Abs(out[0].Y-out[len(out)-1].Y) < 1e-9 {
		out = out[:len(out)-1]
	}
	changed := true
	for changed && len(out) > 3 {
		changed = false
		for i := 0; i < len(out); i++ {
			a, b, c2 := out[(i+len(out)-1)%len(out)], out[i], out[(i+1)%len(out)]
			if math.Abs(cross2(a, b, c2)) < 1e-12 {
				out = append(out[:i], out[i+1:]...)
				changed = true
				break
			}
		}
	}
	return out
}

// Region is an outer contour (CCW) with holes (CW).
type Region struct {
	Outer []Pt
	Holes [][]Pt
}

// Regions organises arbitrary closed contours into outer/hole regions using
// the even-odd nesting depth, which handles font and SVG outlines whatever
// their original winding.
func Regions(contours [][]Pt) []Region {
	var cs [][]Pt
	for _, c := range contours {
		c = cleanContour(c)
		if len(c) >= 3 && math.Abs(SignedArea(c)) > 1e-9 {
			cs = append(cs, c)
		}
	}
	depth := make([]int, len(cs))
	parent := make([]int, len(cs))
	for i := range cs {
		parent[i] = -1
		bestArea := math.Inf(1)
		for j := range cs {
			if i == j || !PointInPolygon(offEdgePoint(cs[i], cs[j]), cs[j]) {
				continue
			}
			depth[i]++
			if a := math.Abs(SignedArea(cs[j])); a < bestArea {
				bestArea = a
				parent[i] = j
			}
		}
	}
	idx := map[int]int{}
	var regs []Region
	for i, c := range cs {
		if depth[i]%2 == 0 {
			if SignedArea(c) < 0 {
				c = reversed(c)
			}
			idx[i] = len(regs)
			regs = append(regs, Region{Outer: c})
		}
	}
	for i, c := range cs {
		if depth[i]%2 == 1 && parent[i] >= 0 {
			if r, ok := idx[parent[i]]; ok {
				if SignedArea(c) > 0 {
					c = reversed(c)
				}
				regs[r].Holes = append(regs[r].Holes, c)
			}
		}
	}
	return regs
}

// offEdgePoint is a point of outline c (on it, or just inside it) that
// isn't on d's edges, to ask whether c lies inside d: a corner, the middle
// of a side, or a point just inside a corner. A hole whose corners all lie
// on the outline's sides, its sides running across inside, has no corner
// off the outline, and asked at one on it the answer could go either way
// (a hole in a hole, an island of solid, could pass for a plain hole).
func offEdgePoint(c, d []Pt) Pt {
	if i := offEdge(c, d); !onEdge(c[i], d) {
		return c[i]
	}
	n := len(c)
	for i := range c {
		if m := (Pt{(c[i].X + c[(i+1)%n].X) / 2, (c[i].Y + c[(i+1)%n].Y) / 2}); !onEdge(m, d) {
			return m
		}
	}
	// just inside a corner: along the middle of its sides, on the inside
	sign := 1.0
	if SignedArea(c) < 0 {
		sign = -1
	}
	b := boundsOf([][]Pt{c})
	eps := 1e-6 * math.Max(1e-9, math.Max(b.x1-b.x0, b.y1-b.y0))
	for i := range c {
		p, a, q := c[(i+n-1)%n], c[i], c[(i+1)%n]
		u, v := ptUnit(ptSub(q, a)), ptUnit(ptSub(p, a))
		dir := ptAdd(u, v)
		if math.Hypot(dir.X, dir.Y) < 1e-12 {
			dir = Pt{-u.Y * sign, u.X * sign}
		} else if cross2(p, a, q)*sign < 0 { // a reflex corner: inside is the other way
			dir = ptScale(dir, -1)
		}
		if m := ptAdd(a, ptScale(ptUnit(dir), eps)); !onEdge(m, d) {
			return m
		}
	}
	return c[0]
}

// offEdge is the index of a point of c that isn't on an edge of d, to test
// c's nesting in d with: on d's edge (outlines that touch at a point, such
// as the two lobes of a figure eight), the test could go either way. 0 when
// every point is on d.
func offEdge(c, d []Pt) int {
	for i, p := range c {
		if !onEdge(p, d) {
			return i
		}
	}
	return 0
}

// onEdge reports whether p is within 1e-9 mm of an edge of c.
func onEdge(p Pt, c []Pt) bool {
	for i, j := 0, len(c)-1; i < len(c); j, i = i, i+1 {
		a, b := c[j], c[i]
		if p.X < math.Min(a.X, b.X)-1e-9 || p.X > math.Max(a.X, b.X)+1e-9 || p.Y < math.Min(a.Y, b.Y)-1e-9 || p.Y > math.Max(a.Y, b.Y)+1e-9 {
			continue
		}
		d := Pt{b.X - a.X, b.Y - a.Y}
		l := d.X*d.X + d.Y*d.Y
		t := 0.0
		if l > 0 {
			t = math.Max(0, math.Min(1, ((p.X-a.X)*d.X+(p.Y-a.Y)*d.Y)/l))
		}
		dx, dy := a.X+d.X*t-p.X, a.Y+d.Y*t-p.Y
		if dx*dx+dy*dy <= 1e-18 {
			return true
		}
	}
	return false
}

// bridge merges the holes into the outer contour so the result is a single
// (weakly simple) polygon suitable for ear clipping. Each hole, rightmost
// first, joins the polygon at a vertex visible from its rightmost vertex
// M: cast a ray from M along +X to the nearest edge it hits; the edge's
// rightmost end P is visible unless a vertex inside triangle M-hit-P is
// in the way, in which case the one at the smallest angle from the ray
// is (Eberly, "Triangulation by Ear Clipping"). Bridges found this way
// never cross each other or a contour.
func bridge(r Region) []Pt {
	poly := append([]Pt(nil), r.Outer...)
	holes := append([][]Pt(nil), r.Holes...)
	rightmost := func(h []Pt) int {
		bi := 0
		for i := range h {
			if h[i].X > h[bi].X || (h[i].X == h[bi].X && h[i].Y < h[bi].Y) {
				bi = i
			}
		}
		return bi
	}
	sort.Slice(holes, func(i, j int) bool { return holes[i][rightmost(holes[i])].X > holes[j][rightmost(holes[j])].X })
	for _, h := range holes {
		// a hole that touches the polygon at a corner (the outline, or a
		// hole joined already, touching this one) joins it there: bridged
		// elsewhere, the polygon would pinch at that corner too, and the
		// ears cut from it would cover the holes
		if k, j, ok := touching(poly, h); ok {
			merged := make([]Pt, 0, len(poly)+len(h)+1)
			merged = append(merged, poly[:j+1]...)
			for s := 1; s < len(h); s++ {
				merged = append(merged, h[(k+s)%len(h)])
			}
			merged = append(merged, poly[j])
			poly = append(merged, poly[j+1:]...)
			continue
		}
		mi := rightmost(h)
		m := h[mi]
		pi := visibleVertex(poly, m)
		// the same point can appear more than once (earlier bridges end
		// there): join at the copy whose corner opens toward the hole
		if n := len(poly); n > 3 {
			for j := range poly {
				if j != pi && same(poly[j], poly[pi]) && inWedge(poly[(j+n-1)%n], poly[j], poly[(j+1)%n], m) {
					pi = j
					break
				}
			}
		}
		merged := make([]Pt, 0, len(poly)+len(h)+2)
		merged = append(merged, poly[:pi+1]...)
		if same(m, poly[pi]) {
			// the hole touches the outline there: no bridge, the hole's
			// loop goes round from that point and back to it (a bridge of
			// no length would leave four copies of the point, two edges of
			// no length, and the hole covered)
			for k := 1; k < len(h); k++ {
				merged = append(merged, h[(mi+k)%len(h)])
			}
		} else {
			for k := 0; k <= len(h); k++ {
				merged = append(merged, h[(mi+k)%len(h)])
			}
		}
		merged = append(merged, poly[pi])
		merged = append(merged, poly[pi+1:]...)
		poly = merged
	}
	return poly
}

// touching finds a corner hole h shares with poly: h[k] is poly[j], at the
// copy of that point (it can appear more than once) whose interior angle
// the hole lies in. ok is false when they share none.
func touching(poly, h []Pt) (k, j int, ok bool) {
	n, hn := len(poly), len(h)
	if n < 3 || hn < 3 {
		return 0, 0, false
	}
	b := boundsOf([][]Pt{poly})
	eps := 1e-7 * math.Max(1, math.Max(b.x1-b.x0, b.y1-b.y0))
	for k = range h {
		v := h[k]
		// a point just inside the hole's corner there: along the middle of
		// its two sides (or across, where they run straight on)
		a, c := ptUnit(ptSub(h[(k+1)%hn], v)), ptUnit(ptSub(h[(k+hn-1)%hn], v))
		d := ptAdd(a, c)
		if math.Hypot(d.X, d.Y) < 1e-9 {
			d = Pt{-a.Y, a.X}
		}
		probe := ptAdd(v, ptScale(ptUnit(d), eps))
		first := -1
		for j = range poly {
			if !same(poly[j], v) {
				continue
			}
			if first < 0 {
				first = j
			}
			if inWedge(poly[(j+n-1)%n], poly[j], poly[(j+1)%n], probe) {
				return k, j, true
			}
		}
		if first >= 0 {
			return k, first, true
		}
	}
	return 0, 0, false
}

// inWedge reports whether point m lies within the polygon's interior angle
// at vertex p (counter-clockwise polygon, neighbours prev and next).
func inWedge(prev, p, next, m Pt) bool {
	dn, dp, u := Pt{next.X - p.X, next.Y - p.Y}, Pt{prev.X - p.X, prev.Y - p.Y}, Pt{m.X - p.X, m.Y - p.Y}
	cr := func(a, b Pt) float64 { return a.X*b.Y - a.Y*b.X }
	if cr(dn, dp) >= 0 { // convex corner: between next and prev, turning left
		return cr(dn, u) > 0 && cr(u, dp) > 0
	}
	// reflex corner: anywhere but the outside wedge from prev to next
	return !(cr(dp, u) >= 0 && cr(u, dn) >= 0)
}

// visibleVertex is the index of a vertex of poly (counter-clockwise)
// visible from m, a point inside it.
func visibleVertex(poly []Pt, m Pt) int {
	n := len(poly)
	best, bi := math.Inf(1), -1
	hitVertex := false
	// on the ray's line: within rounding of it (taken exactly, a corner at
	// y -2.775 and a hole's at -2.7749999999999986 are apart, and the
	// bridge runs through the corner, along the side beyond it)
	on := func(y float64) bool { return math.Abs(y-m.Y) <= 1e-9 }
	for i := 0; i < n; i++ {
		a, b := poly[i], poly[(i+1)%n]
		if on(a.Y) && a.X >= m.X { // the ray passes through a vertex
			if d := a.X - m.X; d < best || (d == best && !hitVertex) {
				best, bi, hitVertex = d, i, true
			}
			continue
		}
		if on(a.Y) || on(b.Y) || (a.Y < m.Y) == (b.Y < m.Y) {
			continue // doesn't cross the ray's line (or ends on it: a vertex hit)
		}
		x := a.X + (m.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y)
		if d := x - m.X; d >= 0 && d < best {
			best, bi, hitVertex = d, i, false
		}
	}
	if bi < 0 {
		return 0 // no edge to the right: not inside (shouldn't happen)
	}
	if hitVertex {
		return bi
	}
	// the end of the hit edge further right, unless something is in the way
	hit := Pt{m.X + best, m.Y}
	a, b := bi, (bi+1)%n
	p := a
	if poly[b].X > poly[a].X {
		p = b
	}
	pp := poly[p]
	ang := func(q Pt) float64 { return math.Atan2(math.Abs(q.Y-m.Y), q.X-m.X) }
	dist := func(q Pt) float64 { return math.Hypot(q.X-m.X, q.Y-m.Y) }
	bestAng, bestDist := ang(pp), dist(pp)
	for i := 0; i < n; i++ {
		q := poly[i]
		if i == p || same(q, pp) || q.X < m.X {
			continue
		}
		// inside triangle m, hit, pp (either winding), or on the line from
		// m to pp: a vertex there is in the bridge's way (the bridge would
		// run through it), and nearer at the same angle, it wins below
		in := false
		if cross2(m, hit, pp) > 0 {
			in = cross2(m, hit, q) > 1e-12 && cross2(hit, pp, q) > 1e-12 && cross2(pp, m, q) >= -1e-12
		} else {
			in = cross2(m, hit, q) < -1e-12 && cross2(hit, pp, q) < -1e-12 && cross2(pp, m, q) <= 1e-12
		}
		if !in {
			continue
		}
		if qa, qd := ang(q), dist(q); qa < bestAng-1e-12 || (math.Abs(qa-bestAng) <= 1e-12 && qd < bestDist) {
			p, bestAng, bestDist = i, qa, qd
		}
	}
	return p
}

func inTri(p, a, b, c Pt) bool {
	return cross2(a, b, p) >= -1e-12 && cross2(b, c, p) >= -1e-12 && cross2(c, a, p) >= -1e-12
}

// inTriStrict reports whether p is inside triangle a b c, not on an edge.
func inTriStrict(p, a, b, c Pt) bool {
	return cross2(a, b, p) > 1e-12 && cross2(b, c, p) > 1e-12 && cross2(c, a, p) > 1e-12
}

func same(a, b Pt) bool { return math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) < 1e-9 }

// Triangulate returns CCW triangles covering the region: ear clipping of
// the bridged polygon, walking a ring of the remaining vertices so each
// step carries on from the last ear, with the vertices kept in a grid so
// an ear's emptiness is checked against only its neighbours.
//
// A region whose outlines touch at corners (a hole whose corners all lie
// on the outline, an outline touching itself) bridges to a polygon that
// pinches there; the ear clipper can find no ear across a pinch, and the
// cut it then forces covers more than the region. When the triangles'
// area doesn't match the region's, the region is cut in horizontal slabs
// instead (`slabTriangulate`), which touching outlines can't confuse. Its
// triangles have corners part way along the outlines' sides, so a mesh
// built with them needs FixTJunctions: callers that build walls from the
// outlines use triangulate, which says when that happened.
func Triangulate(r Region) [][3]Pt {
	tris, _ := triangulate(r)
	return tris
}

// triangulate is Triangulate, and whether it fell back to slabs (whose
// corners on the outlines' sides the walls don't have).
func triangulate(r Region) ([][3]Pt, bool) {
	poly := bridge(r)
	if len(poly) < 3 {
		return nil, false
	}
	tris := earClip(poly)
	want := math.Abs(SignedArea(r.Outer))
	for _, h := range r.Holes {
		want -= math.Abs(SignedArea(h))
	}
	area := func(ts [][3]Pt) float64 {
		s := 0.0
		for _, t := range ts {
			s += math.Abs(SignedArea([]Pt{t[0], t[1], t[2]}))
		}
		return s
	}
	tol := 1e-9 * math.Max(1, want)
	if got := area(tris); math.Abs(got-want) > tol {
		if slabs := slabTriangulate(r); math.Abs(area(slabs)-want) < math.Abs(got-want) {
			return slabs, true
		}
	}
	return tris, false
}

// slabTriangulate cuts a region into triangles slab by slab: between each
// two heights its corners are at, the sides crossing the slab, in order
// across, pair off (even-odd: outline and holes nest) into trapezoids.
func slabTriangulate(r Region) [][3]Pt {
	type side struct{ a, b Pt } // a below b
	var sides []side
	var ys []float64
	for _, c := range append([][]Pt{r.Outer}, r.Holes...) {
		for i := range c {
			a, b := c[i], c[(i+1)%len(c)]
			ys = append(ys, a.Y)
			if a.Y == b.Y {
				continue
			}
			if a.Y > b.Y {
				a, b = b, a
			}
			sides = append(sides, side{a, b})
		}
	}
	sort.Float64s(ys)
	xAt := func(s side, y float64) float64 {
		switch y {
		case s.a.Y:
			return s.a.X
		case s.b.Y:
			return s.b.X
		}
		return s.a.X + (y-s.a.Y)*(s.b.X-s.a.X)/(s.b.Y-s.a.Y)
	}
	var out [][3]Pt
	type cross struct{ x0, x1, xm float64 }
	for k := 0; k+1 < len(ys); k++ {
		y0, y1 := ys[k], ys[k+1]
		if y1-y0 < 1e-12 {
			continue
		}
		ym := (y0 + y1) / 2
		var cs []cross
		for _, s := range sides {
			if s.a.Y <= y0 && s.b.Y >= y1 {
				cs = append(cs, cross{xAt(s, y0), xAt(s, y1), xAt(s, ym)})
			}
		}
		sort.Slice(cs, func(i, j int) bool { return cs[i].xm < cs[j].xm })
		for i := 0; i+1 < len(cs); i += 2 {
			l, rt := cs[i], cs[i+1]
			bl, br, tr, tl := Pt{l.x0, y0}, Pt{rt.x0, y0}, Pt{rt.x1, y1}, Pt{l.x1, y1}
			for _, t := range [][3]Pt{{bl, br, tr}, {bl, tr, tl}} {
				if cross2(t[0], t[1], t[2]) > 1e-12 {
					out = append(out, t)
				}
			}
		}
	}
	return out
}

// earClip cuts a bridged polygon into triangles (see Triangulate).
func earClip(poly []Pt) [][3]Pt {
	n := len(poly)
	prev, next := make([]int, n), make([]int, n)
	for i := range poly {
		prev[i], next[i] = (i+n-1)%n, (i+1)%n
	}
	alive := make([]bool, n)
	for i := range alive {
		alive[i] = true
	}
	g := newPointGrid(poly)
	// isEar: b is convex and no other vertex is inside triangle a b c; a
	// strict test ignores vertices on its edges (vertices lined up on
	// rows can block every ear otherwise)
	isEar := func(ia, ib, ic int, strict bool) bool {
		a, b, c := poly[ia], poly[ib], poly[ic]
		if cross2(a, b, c) <= 1e-12 {
			return false
		}
		empty := true
		g.each(math.Min(a.X, math.Min(b.X, c.X)), math.Min(a.Y, math.Min(b.Y, c.Y)),
			math.Max(a.X, math.Max(b.X, c.X)), math.Max(a.Y, math.Max(b.Y, c.Y)), func(j int) bool {
				if !alive[j] || j == ia || j == ib || j == ic {
					return true
				}
				p := poly[j]
				if same(p, a) || same(p, b) || same(p, c) {
					return true
				}
				if inTri(p, a, b, c) && (!strict || inTriStrict(p, a, b, c)) {
					empty = false
					return false
				}
				return true
			})
		return empty
	}
	remove := func(i int) {
		alive[i] = false
		next[prev[i]], prev[next[i]] = next[i], prev[i]
	}
	var tris [][3]Pt
	count, i, stalled := n, 0, 0
	for count > 3 {
		a, c := prev[i], next[i]
		// a lap with the usual test, then one with the strict test
		if isEar(a, i, c, stalled >= count) {
			tris = append(tris, [3]Pt{poly[a], poly[i], poly[c]})
			remove(i)
			count--
			i, stalled = prev[c], 0 // carry on beside the ear
			continue
		}
		i = next[i]
		if stalled++; stalled <= 2*count {
			continue
		}
		// a full lap without an ear: degenerate input; clip the most
		// convex vertex (or drop a collinear one) to make progress
		best, bv := i, math.Inf(-1)
		for k, j := 0, i; k < count; k, j = k+1, next[j] {
			if v := cross2(poly[prev[j]], poly[j], poly[next[j]]); v > bv {
				best, bv = j, v
			}
		}
		if bv > 1e-12 {
			tris = append(tris, [3]Pt{poly[prev[best]], poly[best], poly[next[best]]})
		}
		i = next[best]
		remove(best)
		count--
		stalled = 0
	}
	a := i
	b, c := next[a], next[next[a]]
	if cross2(poly[a], poly[b], poly[c]) > 1e-12 {
		tris = append(tris, [3]Pt{poly[a], poly[b], poly[c]})
	}
	return tris
}

// pointGrid buckets point indices by position for range queries.
type pointGrid struct {
	x0, y0, cell float64
	nx, ny       int
	cells        [][]int
}

func newPointGrid(ps []Pt) *pointGrid {
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range ps {
		minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
		maxX, maxY = math.Max(maxX, p.X), math.Max(maxY, p.Y)
	}
	w, h := math.Max(maxX-minX, 1e-9), math.Max(maxY-minY, 1e-9)
	// about two points per cell
	cell := math.Max(math.Sqrt(w*h/float64(len(ps))*2), 1e-9)
	// cells along a span, at most 4096: limited before the conversion to
	// int (a span of 1e300 over a small cell would overflow it); NaN and
	// infinity give one cell
	count := func(span, cell float64) int {
		v := span / cell
		if !(v < 4095) {
			v = 4095
		}
		if !(v >= 0) {
			v = 0
		}
		return int(v) + 1
	}
	g := &pointGrid{x0: minX, y0: minY, cell: cell, nx: count(w, cell), ny: count(h, cell)}
	g.cell = math.Max(w/float64(g.nx), h/float64(g.ny)) * (1 + 1e-9)
	g.nx, g.ny = count(w, g.cell), count(h, g.cell)
	g.cells = make([][]int, g.nx*g.ny)
	for i, p := range ps {
		cx, cy := g.at(p.X, p.Y)
		g.cells[cy*g.nx+cx] = append(g.cells[cy*g.nx+cx], i)
	}
	return g
}

func (g *pointGrid) at(x, y float64) (int, int) {
	// clamped as floats: converting an out-of-range float to int is
	// undefined
	idx := func(v float64, n int) int {
		if !(v > 0) {
			return 0
		}
		return int(math.Min(v, float64(n-1)))
	}
	return idx((x-g.x0)/g.cell, g.nx), idx((y-g.y0)/g.cell, g.ny)
}

// each calls f for every point in cells overlapping the box, until f
// returns false.
func (g *pointGrid) each(x0, y0, x1, y1 float64, f func(int) bool) {
	ax, ay := g.at(x0-1e-9, y0-1e-9)
	bx, by := g.at(x1+1e-9, y1+1e-9)
	for cy := ay; cy <= by; cy++ {
		for cx := ax; cx <= bx; cx++ {
			for _, j := range g.cells[cy*g.nx+cx] {
				if !f(j) {
					return
				}
			}
		}
	}
}

// ExtrudeRegions extrudes regions from z0 to z1 into a closed mesh. The
// side walls stand on the caps' own boundary (each cap edge with no
// opposite edge in the caps), not on the outlines: where triangulation
// leaves out a degenerate sliver (an outline doubling back on itself, a
// spike of no width) the walls follow what was capped, and the mesh stays
// closed.
func ExtrudeRegions(regs []Region, z0, z1 float64) *geom.Mesh {
	m := &geom.Mesh{}
	type edge struct{ a, b Pt }
	count := map[edge]int{}
	var order []edge // first appearance, for a deterministic mesh
	for _, r := range regs {
		for _, t := range Triangulate(r) {
			if math.Abs(cross2(t[0], t[1], t[2]))/2 <= 1e-12 {
				continue // as Clean would drop it: its edges mustn't get walls
			}
			m.Tris = append(m.Tris,
				geom.Tri{P: [3]geom.Vec3{geom.V(t[0].X, t[0].Y, z1), geom.V(t[1].X, t[1].Y, z1), geom.V(t[2].X, t[2].Y, z1)}},
				geom.Tri{P: [3]geom.Vec3{geom.V(t[0].X, t[0].Y, z0), geom.V(t[2].X, t[2].Y, z0), geom.V(t[1].X, t[1].Y, z0)}})
			for k := 0; k < 3; k++ {
				e := edge{t[k], t[(k+1)%3]}
				if e.a == e.b {
					continue
				}
				if _, ok := count[e]; !ok {
					order = append(order, e)
				}
				count[e]++
			}
		}
	}
	for _, e := range order {
		// an edge used both ways (inside the cap, or a bridge to a hole)
		// is no boundary
		for n := count[e] - count[edge{e.b, e.a}]; n > 0; n-- {
			p, q := e.a, e.b
			p0, q0 := geom.V(p.X, p.Y, z0), geom.V(q.X, q.Y, z0)
			p1, q1 := geom.V(p.X, p.Y, z1), geom.V(q.X, q.Y, z1)
			m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{p0, q0, q1}}, geom.Tri{P: [3]geom.Vec3{p0, q1, p1}})
		}
	}
	return m.Clean()
}

// ExtrudeContours organises contours into regions and extrudes them.
func ExtrudeContours(contours [][]Pt, z0, z1 float64) *geom.Mesh {
	return ExtrudeRegions(Regions(contours), z0, z1)
}
