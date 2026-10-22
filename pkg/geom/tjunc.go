package geom

import (
	"math"
	"sort"
)

// FixTJunctions makes a CSG result watertight, in stages:
//
//  1. triangles of no area are dropped (the writers leave them out);
//  2. vertices within csgEps are welded and every open edge is split at
//     the vertices lying on it: the T-junctions a BSP boolean leaves, where
//     one face has a single long edge and the face beside it several short
//     ones;
//  3. while edges are still open, the vertices on them are welded and
//     split within stitchTol (the slits near-flush float32 faces leave),
//     and slivers and back-to-back sheets that match nothing are dropped;
//  4. the holes that remain are filled where they are flat or slit-like.
//
// A mesh with no open edges after stage 2 is returned from there: stages 3
// and 4 never touch a closed mesh. The result is repaired again while it
// holds triangles of no area (a split or a fill can make a cap), so that
// it stays closed once a writer drops them.
func (m *Mesh) FixTJunctions() *Mesh {
	if m == nil || len(m.Tris) == 0 {
		return m
	}
	// the result must stay closed once the writers drop its triangles of
	// no area (Clean): where it has any (a split or a fill can make a cap,
	// three points in a line), it is repaired again without them
	//
	// The work is bounded: the mesh may never grow past repairBudget of its
	// input. A stitch that keeps splitting edges it can't close (each pass
	// doubling the mesh while closing nothing) stops there, and the best
	// mesh seen is the result.
	budget := repairBudget(len(m.Tris))
	cur := m
	for i := 0; i < 3; i++ {
		cur = fixOnce(cur, budget)
		if len(cur.Clean().Tris) == len(cur.Tris) {
			break
		}
	}
	return cur
}

// repairBudget is the most triangles a repair of n may make: a repair
// that closes cracks adds a few triangles per crack, never this many.
func repairBudget(n int) int { return 2*n + 10000 }

func fixOnce(m *Mesh, budget int) *Mesh {
	all := func(Vec3) bool { return true }
	weld := csgEps // mm: the booleans' own tolerance
	// triangles of no area first, as every writer leaves them out: a
	// needle or a cap dropped there would open its three edges (the
	// splits below close what dropping one leaves)
	cur := m.Clean()
	for pass := 0; pass < 4; pass++ {
		next, changed := splitPass(cur, weld, 2*weld, all)
		cur = next
		if !changed {
			break
		}
	}
	// Stitch what is still open. Booleans of near-flush faces (float32
	// noise: faces meant to meet that sit 1e-5 to 3e-4 mm apart) leave
	// slits: two edges that should coincide, a fraction of a micron apart,
	// beyond the weld above. Only the vertices on open edges are welded
	// and split here, within stitchTol, so nothing away from a crack moves
	// and a slit narrower than stitchTol zips shut, moving the surface by
	// no more than its width. Welding a slit can leave slivers on it that
	// match nothing (a nanometre triangle twice over, a needle folded back
	// on its neighbour); those are dropped and the stitch run again.
	//
	// Each round must leave fewer edges open than the best so far, or the
	// stitch stops and the best mesh is kept: on a surface it can't close
	// (a boolean that went badly wrong), re-splitting the same edges only
	// multiplies the triangles.
	best := cur
	bestOpen := len(openEdgeVertices(cur))
	if bestOpen == 0 {
		return cur
	}
	for round := 0; round < 8; round++ {
		for pass := 0; pass < 4; pass++ {
			edge := openEdgeVertices(cur)
			if len(edge) == 0 {
				return cur
			}
			next, changed := splitPass(cur, stitchTol, stitchTol, func(p Vec3) bool { return edge[p] })
			if len(next.Tris) > budget {
				changed = false // over budget: this pass is not kept
				next = cur
			}
			cur = next
			if !changed {
				break
			}
		}
		next, changed := dropStraySlivers(cur)
		next, cancelled := cancelSheets(next)
		cur = next
		open := len(openEdgeVertices(cur))
		if open == 0 {
			return cur
		}
		if open >= bestOpen {
			break // no progress: the best so far stands
		}
		best, bestOpen = cur, open
		if !changed && !cancelled {
			break
		}
	}
	return fillHoles(best)
}

// fillHoles closes what is still open where the hole is flat or a slit:
// the loops of unmatched edges, each triangulated in its own plane. A
// boolean of noisy faces (flush to a few millionths of a millimetre) can
// leave a patch of one face out, or both of two faces pressed together
// with one's outline a hair off the other's; the solid's volume is right,
// the surface just has a gap in a plane. A loop that isn't flat (to within
// stitchTol) is left open: filling it could cross the surface.
func fillHoles(m *Mesh) *Mesh {
	cnt := directedEdges(m)
	colOf := map[vedge]Color{} // a face's colour by its edges: the patch's
	for _, t := range m.Tris {
		for k := 0; k < 3; k++ {
			colOf[vedge{t.P[k], t.P[(k+1)%3]}] = t.C
		}
	}
	// the patch runs each unmatched edge the other way: from b to a
	next := map[Vec3][]Vec3{}
	var starts []Vec3
	for e, n := range cnt {
		for d := n - cnt[vedge{e[1], e[0]}]; d > 0; d-- {
			if len(next[e[1]]) == 0 {
				starts = append(starts, e[1])
			}
			next[e[1]] = append(next[e[1]], e[0])
		}
	}
	if len(next) == 0 {
		return m
	}
	// deterministic: start from the least point
	sort.Slice(starts, func(i, j int) bool { return lessVec(starts[i], starts[j]) })
	for _, s := range starts {
		ns := next[s]
		sort.Slice(ns, func(i, j int) bool { return lessVec(ns[i], ns[j]) })
	}
	out := &Mesh{Tris: append([]Tri(nil), m.Tris...)}
	for _, s := range starts {
		for len(next[s]) > 0 {
			loop := []Vec3{s}
			cur := s
			ok := false
			for steps := 0; steps < 1<<20; steps++ {
				ns := next[cur]
				if len(ns) == 0 {
					break // not a loop (can't be, for counts that balance)
				}
				nx := ns[0]
				next[cur] = ns[1:]
				if nx == s {
					ok = true
					break
				}
				loop = append(loop, nx)
				cur = nx
			}
			if ok && len(loop) >= 3 {
				// coloured as the face beside its first edge
				out.Tris = append(out.Tris, fillLoop(loop, colOf[vedge{loop[1], loop[0]}])...)
			}
		}
	}
	return out
}

// lessVec orders points by X, then Y, then Z: the one fixed order every
// stage sorts by, so a repair gives the same mesh from run to run.
func lessVec(a, b Vec3) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.Z < b.Z
}

// vedge is a directed edge, by its end points' exact positions.
type vedge [2]Vec3

// directedEdges counts the uses of each directed edge. An edge whose ends
// coincide (a triangle of no area) is left out.
func directedEdges(m *Mesh) map[vedge]int {
	cnt := make(map[vedge]int, 3*len(m.Tris))
	for _, t := range m.Tris {
		for k := 0; k < 3; k++ {
			if a, b := t.P[k], t.P[(k+1)%3]; a != b {
				cnt[vedge{a, b}]++
			}
		}
	}
	return cnt
}

// fillLoop triangulates a flat (or slit) loop, keeping its winding; nil if
// it isn't flat.
func fillLoop(loop []Vec3, col Color) []Tri {
	var nrm, c Vec3
	for i, p := range loop {
		nrm = nrm.Add(p.Cross(loop[(i+1)%len(loop)]))
		c = c.Add(p)
	}
	c = c.Mul(1 / float64(len(loop)))
	per := 0.0
	for i, p := range loop {
		per += p.Sub(loop[(i+1)%len(loop)]).Len()
	}
	area := nrm.Len() / 2
	if area <= 1e-12 {
		return nil // no area to cover
	}
	n := nrm.Mul(1 / nrm.Len())
	if 2*area/per > slitTol {
		// wider than a slit: only if flat
		for _, p := range loop {
			if math.Abs(p.Sub(c).Dot(n)) > stitchTol {
				return nil
			}
		}
	}
	// ear clipping in the loop's plane
	u := n.Cross(V(1, 0, 0))
	if u.Len() < 0.5 {
		u = n.Cross(V(0, 1, 0))
	}
	u = u.Norm()
	v := n.Cross(u)
	type p2 struct{ x, y float64 }
	pts := make([]p2, len(loop))
	for i, p := range loop {
		d := p.Sub(c)
		pts[i] = p2{d.Dot(u), d.Dot(v)}
	}
	cross := func(a, b, o p2) float64 { return (a.x-o.x)*(b.y-o.y) - (a.y-o.y)*(b.x-o.x) }
	idx := make([]int, len(loop))
	for i := range idx {
		idx[i] = i
	}
	var tris []Tri
	for len(idx) > 3 {
		found := -1
		for k := range idx {
			a, b, cc := idx[(k+len(idx)-1)%len(idx)], idx[k], idx[(k+1)%len(idx)]
			if cross(pts[b], pts[cc], pts[a]) <= 0 {
				continue // reflex (or flat): not an ear
			}
			clear := true
			for _, o := range idx {
				if o == a || o == b || o == cc || loop[o] == loop[a] || loop[o] == loop[b] || loop[o] == loop[cc] {
					continue
				}
				if cross(pts[b], pts[o], pts[a]) >= 0 && cross(pts[cc], pts[o], pts[b]) >= 0 && cross(pts[a], pts[o], pts[cc]) >= 0 {
					clear = false
					break
				}
			}
			if clear {
				found = k
				break
			}
		}
		if found < 0 {
			// no clean ear (numerically flat corners): cut the most
			// convex corner
			best := 0.0
			for k := range idx {
				a, b, cc := idx[(k+len(idx)-1)%len(idx)], idx[k], idx[(k+1)%len(idx)]
				if x := cross(pts[b], pts[cc], pts[a]); found < 0 || x > best {
					found, best = k, x
				}
			}
		}
		a, b, cc := idx[(found+len(idx)-1)%len(idx)], idx[found], idx[(found+1)%len(idx)]
		tris = append(tris, Tri{P: [3]Vec3{loop[a], loop[b], loop[cc]}, C: col})
		idx = append(idx[:found], idx[found+1:]...)
	}
	tris = append(tris, Tri{P: [3]Vec3{loop[idx[0]], loop[idx[1]], loop[idx[2]]}, C: col})
	return tris
}

// cancelSheets removes pairs of triangles on the same three points facing
// opposite ways: a sheet with no inside, left where welding flattened a
// slab a few nanometres thick. Each edge loses one use each way, so the
// mesh is no less closed, and the folds the pair made are gone.
func cancelSheets(m *Mesh) (*Mesh, bool) {
	// a triangle's key: its points rotated to start at the least, so the
	// same triangle reversed has the key of (p0, p2, p1)
	least := func(p [3]Vec3) [3]Vec3 {
		k := 0
		for i := 1; i < 3; i++ {
			if lessVec(p[i], p[k]) {
				k = i
			}
		}
		return [3]Vec3{p[k], p[(k+1)%3], p[(k+2)%3]}
	}
	byKey := map[[3]Vec3][]int{}
	for i, t := range m.Tris {
		k := least(t.P)
		byKey[k] = append(byKey[k], i)
	}
	drop := map[int]bool{}
	for i, t := range m.Tris {
		if drop[i] {
			continue
		}
		rev := least([3]Vec3{t.P[0], t.P[2], t.P[1]})
		for _, j := range byKey[rev] {
			if !drop[j] && j != i {
				drop[i], drop[j] = true, true
				break
			}
		}
	}
	if len(drop) == 0 {
		return m, false
	}
	out := &Mesh{Tris: make([]Tri, 0, len(m.Tris)-len(drop))}
	for i, t := range m.Tris {
		if !drop[i] {
			out.Tris = append(out.Tris, t)
		}
	}
	return out, true
}

// dropStraySlivers removes triangles thinner than stitchTol that sit on
// edges the mesh doesn't close, one at a time, while each removal leaves
// the mesh no less closed. Such a triangle has no area worth keeping: it
// is debris from welding a slit, or a fold the stitch can then re-split.
func dropStraySlivers(m *Mesh) (*Mesh, bool) {
	cnt := directedEdges(m)
	// how far an edge is from being closed: uses each way should match,
	// and more than one pair is a fold
	bad := func(f, r int) int {
		d := f - r
		if d < 0 {
			d = -d
		}
		if f > 1 && r > 1 {
			d += 2 * (min(f, r) - 1)
		}
		return d
	}
	keep := make([]bool, len(m.Tris))
	changed := false
	for i, t := range m.Tris {
		keep[i] = true
		onBad := false
		for k := 0; k < 3; k++ {
			a, b := t.P[k], t.P[(k+1)%3]
			if cnt[vedge{a, b}] != cnt[vedge{b, a}] || cnt[vedge{a, b}] > 1 {
				onBad = true
			}
		}
		if !onBad || !sliver(t, stitchTol) {
			continue
		}
		// the change removing it makes, edge by edge
		delta := 0
		for k := 0; k < 3; k++ {
			a, b := t.P[k], t.P[(k+1)%3]
			if a == b {
				continue
			}
			f, r := cnt[vedge{a, b}], cnt[vedge{b, a}]
			delta += bad(f-1, r) - bad(f, r)
		}
		if delta > 0 {
			continue
		}
		keep[i] = false
		changed = true
		for k := 0; k < 3; k++ {
			if a, b := t.P[k], t.P[(k+1)%3]; a != b {
				cnt[vedge{a, b}]--
			}
		}
	}
	if !changed {
		return m, false
	}
	out := &Mesh{Tris: make([]Tri, 0, len(m.Tris))}
	for i, t := range m.Tris {
		if keep[i] {
			out.Tris = append(out.Tris, t)
		}
	}
	return out, true
}

// sliver reports whether t is thinner than h: its height over its longest
// edge.
func sliver(t Tri, h float64) bool {
	longest := 0.0
	for k := 0; k < 3; k++ {
		longest = math.Max(longest, t.P[k].Sub(t.P[(k+1)%3]).Len())
	}
	if longest == 0 {
		return true
	}
	area2 := t.P[1].Sub(t.P[0]).Cross(t.P[2].Sub(t.P[0])).Len()
	return area2/longest < h
}

// stitchTol is the widest crack the repair closes (mm): far below what any
// printer resolves, well above the slits booleans of float32 geometry
// leave (measured on 163 real designs: under 3e-4).
const stitchTol = 1e-3

// slitTol is the widest hole (mean width, mm) filled even where it isn't
// flat. Two copies of a surface that should coincide but are faceted
// differently (an STL's fillet, overlapped with a shifted copy of itself)
// cross each other within a micron or so; their union leaves slits a few
// microns wide that follow the curve. Filling one moves the surface by
// no more than its width: a hundredth of the finest a printer resolves.
const slitTol = 1e-2

// openEdgeVertices are the vertices (exact positions) on edges the mesh
// doesn't close: used a different number of times each way, or folded
// (used more than once each way).
func openEdgeVertices(m *Mesh) map[Vec3]bool {
	count := directedEdges(m)
	out := map[Vec3]bool{}
	for e, n := range count {
		if r := count[vedge{e[1], e[0]}]; r != n || n > 1 {
			out[e[0]], out[e[1]] = true, true
		}
	}
	return out
}

// splitPass welds vertices within weld of each other (those weldable
// allows; any two at exactly the same place are always one), then splits
// every open edge (used a different number of times each way) at the
// vertices lying within onEps of it. A split triangle is re-fanned from
// its centroid, which keeps every new triangle non-degenerate. It reports
// whether anything was split; the welded mesh is the result either way.
func splitPass(m *Mesh, weld, onEps float64, weldable func(Vec3) bool) (*Mesh, bool) {
	type vkey [3]int64
	qk := func(p Vec3) vkey {
		return vkey{int64(math.Round(p.X / weld)), int64(math.Round(p.Y / weld)), int64(math.Round(p.Z / weld))}
	}
	index := map[vkey]int{} // weldable vertices by grid cell
	exact := map[Vec3]int{}
	var pos []Vec3
	// weld by distance: a point within weld of an existing vertex (in any
	// neighbouring grid cell, so near-equal points straddling a cell
	// boundary still merge) reuses it
	id := func(p Vec3) int {
		if i, ok := exact[p]; ok {
			return i
		}
		if weldable(p) {
			k := qk(p)
			if i, ok := index[k]; ok {
				exact[p] = i
				return i
			}
			for dx := int64(-1); dx <= 1; dx++ {
				for dy := int64(-1); dy <= 1; dy++ {
					for dz := int64(-1); dz <= 1; dz++ {
						if i, ok := index[vkey{k[0] + dx, k[1] + dy, k[2] + dz}]; ok && pos[i].Sub(p).Len() <= weld {
							index[k] = i
							exact[p] = i
							return i
						}
					}
				}
			}
			index[k] = len(pos)
		}
		exact[p] = len(pos)
		pos = append(pos, p)
		return len(pos) - 1
	}
	tris := make([][3]int, len(m.Tris))
	for i, t := range m.Tris {
		tris[i] = [3]int{id(t.P[0]), id(t.P[1]), id(t.P[2])}
	}
	// uses of each edge, each way: an edge is open where they differ (used
	// once, or folded over a sliver and used three times)
	type ekey [2]int
	count := map[ekey]int{}
	for _, t := range tris {
		for k := 0; k < 3; k++ {
			if t[k] != t[(k+1)%3] {
				count[ekey{t[k], t[(k+1)%3]}]++
			}
		}
	}
	open := func(a, b int) bool { return count[ekey{a, b}] != count[ekey{b, a}] }
	// candidate vertices: endpoints of open edges
	candSet := map[int]bool{}
	for e := range count {
		if open(e[0], e[1]) {
			candSet[e[0]], candSet[e[1]] = true, true
		}
	}
	// (no candidates means nothing to split, but the welded mesh is still
	// the result: the welding alone may have closed every gap)
	//
	// the candidates sorted along each axis: an edge is searched along the
	// axis it spans least, so a long edge running along X looks only at
	// the few vertices near its Y (or Z), not at every vertex beside it
	var byAxis [3][]int
	var coord [3][]float64
	for ax := 0; ax < 3; ax++ {
		cs := make([]int, 0, len(candSet))
		for v := range candSet {
			cs = append(cs, v)
		}
		// ties broken by vertex number (the input's order), not by the
		// map's random order: the result must not vary from run to run
		sort.Slice(cs, func(i, j int) bool {
			if a, b := pos[cs[i]].Axis(ax), pos[cs[j]].Axis(ax); a != b {
				return a < b
			}
			return cs[i] < cs[j]
		})
		xs := make([]float64, len(cs))
		for i, v := range cs {
			xs[i] = pos[v].Axis(ax)
		}
		byAxis[ax], coord[ax] = cs, xs
	}
	// inside lists the vertices within onEps of segment a-b, strictly
	// between its ends, ordered from a to b
	inside := func(a, b int) []int {
		pa, pb := pos[a], pos[b]
		d := pb.Sub(pa)
		l2 := d.Dot(d)
		if l2 < 1e-18 {
			return nil
		}
		// the axis whose range around the edge holds the fewest candidates
		ax, i, j := 0, 0, 0
		for k := 0; k < 3; k++ {
			lo, hi := math.Min(pa.Axis(k), pb.Axis(k))-onEps, math.Max(pa.Axis(k), pb.Axis(k))+onEps
			first, end := sort.SearchFloat64s(coord[k], lo), sort.SearchFloat64s(coord[k], math.Nextafter(hi, math.Inf(1)))
			if k == 0 || end-first < j-i {
				ax, i, j = k, first, end
			}
		}
		cands := byAxis[ax]
		type hit struct {
			v int
			t float64
		}
		var hits []hit
		for ; i < j; i++ {
			v := cands[i]
			if v == a || v == b {
				continue
			}
			p := pos[v]
			t := p.Sub(pa).Dot(d) / l2
			if t <= 1e-9 || t >= 1-1e-9 {
				continue
			}
			if p.Sub(pa.Add(d.Mul(t))).Len() > onEps {
				continue
			}
			hits = append(hits, hit{v, t})
		}
		// two vertices can lie at the same distance along the edge (one
		// either side of it, too far apart to weld): ordered by number, so
		// the same mesh is repaired the same way every run
		sort.Slice(hits, func(i, j int) bool {
			if hits[i].t != hits[j].t {
				return hits[i].t < hits[j].t
			}
			return hits[i].v < hits[j].v
		})
		out := make([]int, len(hits))
		for i, h := range hits {
			out[i] = h.v
		}
		return out
	}

	out := &Mesh{Tris: make([]Tri, 0, len(m.Tris))}
	limit := 4*len(m.Tris) + 10000
	changed := false
	for ti, t := range tris {
		src := m.Tris[ti]
		if t[0] == t[1] || t[1] == t[2] || t[2] == t[0] {
			continue // collapsed by welding
		}
		var ring []int
		split := false
		for k := 0; k < 3; k++ {
			a, b := t[k], t[(k+1)%3]
			ring = append(ring, a)
			if open(a, b) {
				if mid := inside(a, b); len(mid) > 0 {
					ring = append(ring, mid...)
					split = true
				}
			}
		}
		if !split {
			out.Tris = append(out.Tris, Tri{P: [3]Vec3{pos[t[0]], pos[t[1]], pos[t[2]]}, C: src.C})
			continue
		}
		changed = true
		if len(out.Tris)+len(ring) > limit {
			// far more splitting than any crack needs: something about
			// the input is degenerate (thousands of points on one edge,
			// over and over). Better left as it was than the memory spent
			return m, false
		}
		c := pos[t[0]].Add(pos[t[1]]).Add(pos[t[2]]).Mul(1.0 / 3)
		for i := range ring {
			p, q := pos[ring[i]], pos[ring[(i+1)%len(ring)]]
			out.Tris = append(out.Tris, Tri{P: [3]Vec3{c, p, q}, C: src.C})
		}
	}
	return out, changed
}
