package geom

import (
	"math"
	"sort"
)

// AsWritten is m as the exporters write it: triangles of no area left out
// (so a mesh closed only by one is open in the file), then every
// coordinate rounded to float32.
//
// Rounding flattens some slivers, and those are written too. They aren't
// dropped here: corners that matched still match once rounded, so the
// mesh stays closed, and a slicer only drops a triangle with two corners
// in the same place. Leaving the flat ones out would report zero-width
// "holes", often millimetres long.
func AsWritten(m *Mesh) *Mesh {
	m = m.Clean()
	f := func(p Vec3) Vec3 { return Vec3{float64(float32(p.X)), float64(float32(p.Y)), float64(float32(p.Z))} }
	out := &Mesh{Tris: make([]Tri, len(m.Tris))}
	for i, t := range m.Tris {
		out.Tris[i] = Tri{P: [3]Vec3{f(t.P[0]), f(t.P[1]), f(t.P[2])}, C: t.C}
	}
	return out
}

// ShellInfo describes one connected piece of a mesh (see Shells).
type ShellInfo struct {
	Volume float64 // signed: negative for a shell inside out (or a cavity)
	Bounds Box
	Tris   int
	Closed bool // no open or non-manifold edges of its own
}

// Empty is whether the shell is closed with nothing inside: a sheet, two
// faces back to back. It's the test Solidify uses.
func (s ShellInfo) Empty() bool {
	return s.Closed && math.Abs(s.Volume) < 1e-9*math.Max(1, s.Bounds.Size().Len())
}

// SurfaceReport is what Surface finds.
type SurfaceReport struct {
	// Open and NonManifold are EdgeCheck(0)'s counts: edges with no
	// triangle running the other way, and edges used a different number
	// of times each way. An edge used twice each way (two solids touching
	// along it) isn't counted: unions of shapes set edge to edge make
	// those all the time, and they print fine.
	Open, NonManifold int
	// Gap is the widest gap between open edges and the surface across
	// from them: about zero for a T-junction, the crack's width for a
	// crack, the hole's size for a hole. It is a lower bound when
	// GapAtLeast is set (the search stopped there, or some open edges
	// have nothing across from them to measure to: then perhaps 0), and 0
	// with no open edges.
	Gap        float64
	GapAtLeast bool
	Shells     []ShellInfo
	InsideOut  int // closed shells inside out, not a cavity in another shell
	Empty      int // closed shells with no volume: sheets, debris
}

// Surface checks m's surface exactly as given; pass AsWritten(m) to check
// what an export holds.
func Surface(m *Mesh) SurfaceReport {
	t := topologyOf(m)
	r := SurfaceReport{Open: len(t.open), NonManifold: t.nonManifold, Shells: t.shells}
	if r.Open > 0 {
		r.Gap, r.GapAtLeast = gapWidth(t.open)
	}
	for i, s := range r.Shells {
		if s.Empty() {
			r.Empty++
			continue
		}
		if !s.Closed || s.Volume > 0 {
			continue
		}
		cavity := false
		for j, o := range r.Shells {
			if j != i && o.Volume > 0 && boxWithin(s.Bounds, o.Bounds) {
				cavity = true
				break
			}
		}
		if !cavity {
			r.InsideOut++
		}
	}
	return r
}

// OpenEdges returns m's open edges, each once, in the direction its
// triangle runs: edges with no triangle running the other way.
func OpenEdges(m *Mesh) [][2]Vec3 { return topologyOf(m).open }

// ProblemEdges returns the edges Surface counts: the open ones, then the
// non-manifold ones, each once.
func ProblemEdges(m *Mesh) [][2]Vec3 {
	t := topologyOf(m)
	return append(t.open, t.nmEdges...)
}

// topology is what one pass over a mesh's edges finds.
type topology struct {
	open        [][2]Vec3 // as OpenEdges
	nonManifold int       // as EdgeCheck(0)
	nmEdges     [][2]Vec3 // the non-manifold edges, each once
	shells      []ShellInfo
}

// OverhangLimit is the steepest a face can lean out, from vertical, and
// still print without support.
const OverhangLimit = 45.0

// FlatOverhang is a mesh's faces pointing straight down at one height:
// the underside of a slab, say, which may rest on something else.
type FlatOverhang struct {
	Z      float64
	Area   float64
	Bounds Box // of those faces
}

// OverhangReport is the area of a mesh's faces leaning out more than
// OverhangLimit from vertical, facing down: those leaning, and those flat,
// by height. Whether a flat one needs support depends on what's under it.
type OverhangReport struct {
	Sloped float64
	Flat   []FlatOverhang // lowest first
}

// overhangFlat is how near straight down a face's normal is (about 0.8°)
// to count as flat.
const overhangFlat = 0.9999

// Overhangs measures m's overhangs. Faces exactly at the limit (a 45°
// chamfer, a cone's side) don't count; m's triangles are as written, so
// it allows a little for rounding. Leaning faces no higher than low (a
// rounded bottom edge, just above the plate at 0) don't count either:
// they print onto the first layers without support.
func Overhangs(m *Mesh, low float64) OverhangReport {
	limit := -math.Cos(OverhangLimit*math.Pi/180) - 1e-3
	var r OverhangReport
	flat := map[float64]int{} // height -> index in r.Flat
	for _, t := range m.Tris {
		c := t.P[1].Sub(t.P[0]).Cross(t.P[2].Sub(t.P[0]))
		l := c.Len()
		if l == 0 || c.Z/l >= limit {
			continue
		}
		area := l / 2
		if c.Z/l > -overhangFlat {
			if math.Max(t.P[0].Z, math.Max(t.P[1].Z, t.P[2].Z)) > low {
				r.Sloped += area
			}
			continue
		}
		z := (t.P[0].Z + t.P[1].Z + t.P[2].Z) / 3
		k, ok := flat[z]
		if !ok {
			k = len(r.Flat)
			flat[z] = k
			r.Flat = append(r.Flat, FlatOverhang{Z: z, Bounds: EmptyBox()})
		}
		f := &r.Flat[k]
		f.Area += area
		f.Bounds = f.Bounds.Extend(t.P[0]).Extend(t.P[1]).Extend(t.P[2])
	}
	sort.Slice(r.Flat, func(i, j int) bool { return r.Flat[i].Z < r.Flat[j].Z })
	return r
}

// topologyOf finds m's open and non-manifold edges and its shells (as
// Shells: triangles sharing an edge, by exact end points, are in one) in a
// single pass, vertices numbered by position first so edges are pairs of
// numbers.
func topologyOf(m *Mesh) topology {
	vid := make(map[Vec3]int32, len(m.Tris)/2+1)
	var pos []Vec3
	idx := make([][3]int32, len(m.Tris))
	for i, t := range m.Tris {
		for k, p := range t.P {
			v, ok := vid[p]
			if !ok {
				v = int32(len(pos))
				vid[p] = v
				pos = append(pos, p)
			}
			idx[i][k] = v
		}
	}
	parent := make([]int32, len(m.Tris))
	for i := range parent {
		parent[i] = int32(i)
	}
	find := func(i int32) int32 {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	// each edge by its vertices, the lesser first; fwd counts the
	// triangles running from the lesser, back the others; tri is the first
	// triangle on it
	type use struct{ fwd, back, tri int32 }
	edges := make(map[uint64]use, len(m.Tris)*3/2)
	for i, v := range idx {
		for k := 0; k < 3; k++ {
			a, b := v[k], v[(k+1)%3]
			if a == b {
				continue
			}
			lo, hi, fwd := a, b, true
			if b < a {
				lo, hi, fwd = b, a, false
			}
			key := uint64(lo)<<32 | uint64(hi)
			u, ok := edges[key]
			if !ok {
				u.tri = int32(i)
			} else if x, y := find(int32(i)), find(u.tri); x != y {
				parent[x] = y
			}
			if fwd {
				u.fwd++
			} else {
				u.back++
			}
			edges[key] = u
		}
	}
	var t topology
	bad := map[int32]bool{} // shells with an open or non-manifold edge, by root
	for key, u := range edges {
		lo, hi := pos[key>>32], pos[key&0xffffffff]
		switch {
		case u.back == 0:
			t.open = append(t.open, [2]Vec3{lo, hi})
		case u.fwd == 0:
			t.open = append(t.open, [2]Vec3{hi, lo})
		case u.fwd != u.back:
			t.nonManifold += 2 // EdgeCheck counts it each way
			t.nmEdges = append(t.nmEdges, [2]Vec3{lo, hi})
		default:
			continue
		}
		bad[find(u.tri)] = true
	}
	// map order is random; the same mesh gives the same answer
	sort.Slice(t.open, func(i, j int) bool {
		if t.open[i][0] != t.open[j][0] {
			return lessVec(t.open[i][0], t.open[j][0])
		}
		return lessVec(t.open[i][1], t.open[j][1])
	})
	shell := map[int32]int{} // root -> index in t.shells, in order of first triangle
	for i, tr := range m.Tris {
		r := find(int32(i))
		k, ok := shell[r]
		if !ok {
			k = len(t.shells)
			shell[r] = k
			t.shells = append(t.shells, ShellInfo{Bounds: EmptyBox(), Closed: !bad[r]})
		}
		s := &t.shells[k]
		s.Volume += tr.P[0].Dot(tr.P[1].Cross(tr.P[2])) / 6
		s.Bounds = s.Bounds.Extend(tr.P[0]).Extend(tr.P[1]).Extend(tr.P[2])
		s.Tris++
	}
	return t
}

// gapEdges is the most open edges gapWidth tries every pair of; beyond
// that it searches a grid.
const gapEdges = 2000

// gapWidth measures how far open edges are from the surface across from
// them. Across a crack the two sides' edges run against each other, so
// for points along each edge it takes the distance to the nearest open
// edge running the other way (more than 120 degrees apart). Edges along
// the same side of a hole turn gently from one to the next and don't
// count, so a large hole with short edges still measures large, and a
// T-junction (one long edge against two short ones on the same line)
// measures zero. The result is the largest of those distances.
//
// A point whose nearest match is no farther than the widest gap so far
// can't change the answer, so its search stops at the first such match.
func gapWidth(edges [][2]Vec3) (gap float64, atLeast bool) {
	if len(edges) == 0 {
		return 0, false
	}
	g := newGapSegs(edges)
	if len(edges) <= gapEdges {
		return g.brute()
	}
	return g.grid()
}

type gapSeg struct{ a, b, d Vec3 } // d: unit direction

type gapSegs struct {
	segs   []gapSeg
	bounds Box
	length float64 // mean edge length
}

// a match runs more than 120 degrees from the edge
const gapAcross = -0.5

// the points measured along each edge
var gapSamples = [...]float64{0, 0.25, 0.5, 0.75, 1}

func newGapSegs(edges [][2]Vec3) *gapSegs {
	g := &gapSegs{segs: make([]gapSeg, len(edges)), bounds: EmptyBox()}
	for i, e := range edges {
		g.segs[i] = gapSeg{e[0], e[1], e[1].Sub(e[0]).Norm()}
		g.bounds = g.bounds.Extend(e[0]).Extend(e[1])
		g.length += e[1].Sub(e[0]).Len()
	}
	g.length /= float64(len(edges))
	return g
}

// brute tries every pair. A point with no edge running against it
// anywhere (a stray open edge beside non-manifold ones, say) has no size
// to measure: the gap is then at least what the others measure, perhaps 0.
func (g *gapSegs) brute() (gap float64, atLeast bool) {
	for i, s := range g.segs {
		for _, f := range gapSamples {
			p := s.a.Lerp(s.b, f)
			best := math.Inf(1)
			for j, o := range g.segs {
				if j != i && o.d.Dot(s.d) < gapAcross {
					if best = math.Min(best, pointSegDist(p, o.a, o.b)); best <= gap {
						break
					}
				}
			}
			if math.IsInf(best, 1) {
				atLeast = true
				continue
			}
			gap = math.Max(gap, best)
		}
	}
	return gap, atLeast
}

// grid puts the edges in cells and searches outwards from each point ring
// by ring. A point with nothing within gapRings rings is measured against
// every edge, and the search stops there: the gap is then at least that.
func (g *gapSegs) grid() (gap float64, atLeast bool) {
	const gapRings = 16
	// cells about an edge long, but no more than 64 across the open edges'
	// bounds; a cell of zero size can't happen (edges have length)
	cell := math.Max(g.length, g.bounds.Size().Len()/64)
	type key [3]int
	at := func(p Vec3) key {
		q := p.Sub(g.bounds.Min).Mul(1 / cell)
		return key{int(math.Floor(q.X)), int(math.Floor(q.Y)), int(math.Floor(q.Z))}
	}
	cells := map[key][]int{}
	for i, s := range g.segs {
		lo, hi := at(s.a.Min(s.b)), at(s.a.Max(s.b))
		for x := lo[0]; x <= hi[0]; x++ {
			for y := lo[1]; y <= hi[1]; y++ {
				for z := lo[2]; z <= hi[2]; z++ {
					cells[key{x, y, z}] = append(cells[key{x, y, z}], i)
				}
			}
		}
	}
	for i, s := range g.segs {
		for _, f := range gapSamples {
			p := s.a.Lerp(s.b, f)
			c := at(p)
			best := math.Inf(1)
			for k := 0; ; k++ {
				if k > gapRings {
					// nothing near: whatever is farther, if anything, is a
					// lower bound, and the search stops there; a point with
					// nothing across anywhere has no size, as in brute
					if best = g.nearestAcross(i, p); math.IsInf(best, 1) {
						atLeast = true
						break
					}
					return math.Max(gap, best), true
				}
				for x := -k; x <= k; x++ {
					for y := -k; y <= k; y++ {
						// the ring's cells only: inside its x-y square, just
						// the top and bottom
						step := 1
						if k > 0 && abs(x) < k && abs(y) < k {
							step = 2 * k
						}
						for z := -k; z <= k; z += step {
							for _, j := range cells[key{c[0] + x, c[1] + y, c[2] + z}] {
								if o := g.segs[j]; j != i && o.d.Dot(s.d) < gapAcross {
									best = math.Min(best, pointSegDist(p, o.a, o.b))
								}
							}
						}
					}
				}
				// edges not seen yet are in rings beyond k, at least k
				// cells away; and a match within the widest gap so far
				// changes nothing
				if best <= float64(k)*cell || best <= gap {
					break
				}
			}
			if !math.IsInf(best, 1) {
				gap = math.Max(gap, best)
			}
		}
	}
	return gap, atLeast
}

// nearestAcross is the distance from p to the nearest edge running against
// edge i, trying them all: +Inf for none.
func (g *gapSegs) nearestAcross(i int, p Vec3) float64 {
	best := math.Inf(1)
	d := g.segs[i].d
	for j, o := range g.segs {
		if j != i && o.d.Dot(d) < gapAcross {
			best = math.Min(best, pointSegDist(p, o.a, o.b))
		}
	}
	return best
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// pointSegDist is the distance from p to the segment ab.
func pointSegDist(p, a, b Vec3) float64 {
	ab := b.Sub(a)
	l := ab.Dot(ab)
	if l == 0 {
		return p.Sub(a).Len()
	}
	t := math.Max(0, math.Min(1, p.Sub(a).Dot(ab)/l))
	return p.Sub(a.Add(ab.Mul(t))).Len()
}
