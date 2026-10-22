package doc

// Complex boolean sequences checked against an exact oracle. Random trees
// of nested groups (union, intersect, bundle; holes at every level) are
// built from axis-aligned boxes on integer coordinates, so every face lies
// on a grid plane and a cell grid evaluates each group's semantics
// exactly, independently of the CSG code:
//
//   - union group: inside any solid part and no hole part;
//   - intersect group: inside every child's solids (a bundle child's parts
//     together) and no hole part;
//   - a group of holes only is the union of them (what UnionMesh does);
//   - a bundle's parts stand alone, its hole flag passed to its parts;
//   - a hole group cuts as its own result (its holes already taken out).
//
// The evaluated meshes must have the oracle's volume and no open edges,
// after building and after each of a series of edits (holes toggled,
// modes changed, parts moved and resized, groups dissolved and remade):
// the sequences nested groups put the booleans through.

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

type btBox struct{ lo, hi geom.Vec3 }

func (b btBox) has(p geom.Vec3) bool {
	return p.X > b.lo.X && p.X < b.hi.X && p.Y > b.lo.Y && p.Y < b.hi.Y && p.Z > b.lo.Z && p.Z < b.hi.Z
}

// btContrib is one piece taking part in a group: where it is solid, and
// whether it cuts (mirrors childContribs).
type btContrib struct {
	in   func(geom.Vec3) bool
	hole bool
}

func btLeaf(n *Node, xf geom.Mat4) btBox {
	c := xf.Mul(n.M).Point(geom.Vec3{})
	h := n.Size.Mul(0.5)
	return btBox{c.Sub(h), c.Add(h)}
}

func btContribs(c *Node, xf geom.Mat4) []btContrib {
	if c.IsBundle() {
		var out []btContrib
		for _, g := range c.Children {
			for _, p := range btContribs(g, xf.Mul(c.M)) {
				p.hole = p.hole || c.Hole
				out = append(out, p)
			}
		}
		return out
	}
	if c.IsGroup() {
		return []btContrib{{btInside(c, xf.Mul(c.M)), c.Hole}}
	}
	b := btLeaf(c, xf)
	return []btContrib{{b.has, c.Hole}}
}

func btAny(fs []func(geom.Vec3) bool) func(geom.Vec3) bool {
	return func(p geom.Vec3) bool {
		for _, f := range fs {
			if f(p) {
				return true
			}
		}
		return false
	}
}

// btInside is group g's own result (UnionMesh) placed by xf, which
// includes g.M.
func btInside(g *Node, xf geom.Mat4) func(geom.Vec3) bool {
	var solids, holes, perChild []func(geom.Vec3) bool
	for _, c := range g.Children {
		var mine []func(geom.Vec3) bool
		for _, p := range btContribs(c, xf) {
			if p.hole {
				holes = append(holes, p.in)
			} else {
				solids = append(solids, p.in)
				mine = append(mine, p.in)
			}
		}
		if len(mine) > 0 && g.IsIntersect() {
			perChild = append(perChild, btAny(mine))
		}
	}
	if len(solids) == 0 {
		return btAny(holes)
	}
	solid := btAny(solids)
	if g.IsIntersect() {
		solid = func(p geom.Vec3) bool {
			for _, f := range perChild {
				if !f(p) {
					return false
				}
			}
			return true
		}
	}
	anyHole := btAny(holes)
	return func(p geom.Vec3) bool { return solid(p) && !anyHole(p) }
}

// btGrid is every distinct face coordinate of every leaf, per axis.
func btGrid(d *Document) [3][]float64 {
	var g [3][]float64
	var walk func(n *Node, xf geom.Mat4)
	walk = func(n *Node, xf geom.Mat4) {
		if n.IsGroup() {
			for _, c := range n.Children {
				walk(c, xf.Mul(n.M))
			}
			return
		}
		b := btLeaf(n, xf)
		for ax := 0; ax < 3; ax++ {
			g[ax] = append(g[ax], b.lo.Axis(ax), b.hi.Axis(ax))
		}
	}
	for _, n := range d.Nodes {
		walk(n, geom.Identity())
	}
	for ax := 0; ax < 3; ax++ {
		sort.Float64s(g[ax])
		u := g[ax][:0]
		for _, v := range g[ax] {
			if len(u) == 0 || v != u[len(u)-1] {
				u = append(u, v)
			}
		}
		g[ax] = u
	}
	return g
}

// btVolume sums the cells whose centres f reports inside.
func btVolume(g [3][]float64, f func(geom.Vec3) bool) float64 {
	v := 0.0
	for i := 0; i+1 < len(g[0]); i++ {
		for j := 0; j+1 < len(g[1]); j++ {
			for k := 0; k+1 < len(g[2]); k++ {
				c := geom.V((g[0][i]+g[0][i+1])/2, (g[1][j]+g[1][j+1])/2, (g[2][k]+g[2][k+1])/2)
				if f(c) {
					v += (g[0][i+1] - g[0][i]) * (g[1][j+1] - g[1][j]) * (g[2][k+1] - g[2][k])
				}
			}
		}
	}
	return v
}

// btExpect is a top-level node's exported volume (WorldMesh without holes):
// a hole node exports nothing; a bundle's solid parts are summed as they
// are (overlaps counted twice, as Merge does); a group is its result.
func btExpect(g [3][]float64, n *Node) float64 {
	if n.Hole {
		return 0
	}
	switch {
	case n.IsBundle():
		v := 0.0
		for _, p := range btContribs(n, geom.Identity()) {
			if !p.hole {
				v += btVolume(g, p.in)
			}
		}
		return v
	case n.IsGroup():
		return btVolume(g, btInside(n, n.M))
	}
	return btVolume(g, btLeaf(n, geom.Identity()).has)
}

func btAddBox(d *Document, r *rand.Rand, hole bool) *Node {
	n, _ := d.AddShape("box", geom.Vec3{}, hole)
	// crowded into a small space, so parts overlap, touch and share face
	// planes often (intersections are rarely empty)
	s := geom.V(float64(4+r.Intn(16)), float64(4+r.Intn(16)), float64(4+r.Intn(16)))
	lo := geom.V(float64(r.Intn(16)), float64(r.Intn(16)), float64(r.Intn(16)))
	n.Size = s
	n.M = geom.Translate(lo.Add(s.Mul(0.5)))
	return n
}

func btAll(d *Document) []*Node {
	var out []*Node
	for _, n := range d.Nodes {
		n.Walk(func(x *Node) { out = append(out, x) })
	}
	return out
}

func btMode(r *rand.Rand, top bool) string {
	switch r.Intn(5) {
	case 0:
		return ModeIntersect
	case 1:
		if !top {
			return ModeBundle
		}
	}
	return ModeUnion
}

// btCheck compares every top-level node with the oracle.
func btCheck(t *testing.T, d *Document, what string) {
	t.Helper()
	g := btGrid(d)
	for _, n := range d.Nodes {
		want := btExpect(g, n)
		m := n.WorldMesh(false)
		got := m.Volume()
		if math.Abs(got-want) > 1e-6+1e-7*math.Max(want, 1) {
			t.Fatalf("%s: node %d (%s) volume %.9g, oracle %.9g\n%s", what, n.ID, n.Mode(), got, want, btDump(d))
		}
		if open, _ := m.EdgeCheck(1e-9); open != 0 && want > 0 {
			t.Fatalf("%s: node %d (%s) has %d open edges\n%s", what, n.ID, n.Mode(), open, btDump(d))
		}
	}
}

func btDump(d *Document) string {
	var sb strings.Builder
	var walk func(n *Node, xf geom.Mat4, depth int)
	walk = func(n *Node, xf geom.Mat4, depth int) {
		h := ""
		if n.Hole {
			h = " HOLE"
		}
		if n.IsGroup() {
			fmt.Fprintf(&sb, "%s%d %s%s\n", strings.Repeat("  ", depth), n.ID, n.Mode(), h)
			for _, c := range n.Children {
				walk(c, xf.Mul(n.M), depth+1)
			}
			return
		}
		b := btLeaf(n, xf)
		fmt.Fprintf(&sb, "%s%d box %v..%v%s\n", strings.Repeat("  ", depth), n.ID, b.lo, b.hi, h)
	}
	for _, n := range d.Nodes {
		walk(n, geom.Identity(), 0)
	}
	return sb.String()
}

func TestBooleanTreesMatchOracle(t *testing.T) {
	seeds := 30
	if testing.Short() {
		seeds = 8
	}
	for seed := 1; seed <= seeds; seed++ {
		r := rand.New(rand.NewSource(int64(seed)))
		d := New("t")
		for i, n := 0, 3+r.Intn(6); i < n; i++ {
			btAddBox(d, r, r.Intn(4) == 0)
		}
		// group bottom-up until one node is left
		for len(d.Nodes) > 1 {
			k := 2 + r.Intn(min(2, len(d.Nodes)-1))
			perm := r.Perm(len(d.Nodes))[:k]
			var ids []int
			for _, i := range perm {
				ids = append(ids, d.Nodes[i].ID)
			}
			g, err := d.Group(ids, btMode(r, len(d.Nodes) == k))
			if err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			if r.Intn(5) == 0 && !g.IsBundle() {
				g.Hole = true
			}
		}
		d.Nodes[0].Hole = false // a hole at the top exports nothing: nothing to check
		btCheck(t, d, fmt.Sprintf("seed %d built", seed))

		// then edit it: the booleans get recombined at every level
		for step := 0; step < 8; step++ {
			all := btAll(d)
			n := all[r.Intn(len(all))]
			var desc string
			switch op := r.Intn(6); op {
			case 0:
				if d.Find(n.ID) == nil { // nested: at the top a hole exports nothing
					n.Hole = !n.Hole
					desc = fmt.Sprintf("hole %d -> %v", n.ID, n.Hole)
				}
			case 1:
				if n.IsGroup() {
					n.GroupMode = btMode(r, false)
					desc = fmt.Sprintf("mode %d -> %s", n.ID, n.Mode())
				}
			case 2:
				v := geom.V(float64(r.Intn(11)-5), float64(r.Intn(11)-5), float64(r.Intn(11)-5))
				n.M = geom.Translate(v).Mul(n.M)
				desc = fmt.Sprintf("move %d by %v", n.ID, v)
			case 3:
				if !n.IsGroup() {
					ax := r.Intn(3)
					n.Size = n.Size.WithAxis(ax, math.Max(1, n.Size.Axis(ax)+float64(r.Intn(9)-4)))
					desc = fmt.Sprintf("resize %d -> %v", n.ID, n.Size)
				}
			case 4:
				var groups []int
				for _, x := range d.Nodes {
					if x.IsGroup() {
						groups = append(groups, x.ID)
					}
				}
				if len(groups) > 0 {
					id := groups[r.Intn(len(groups))]
					d.Ungroup([]int{id})
					desc = fmt.Sprintf("ungroup %d", id)
				}
			default:
				if len(d.Nodes) >= 2 {
					k := 2 + r.Intn(min(2, len(d.Nodes)-1))
					var ids []int
					for _, i := range r.Perm(len(d.Nodes))[:k] {
						ids = append(ids, d.Nodes[i].ID)
					}
					mode := btMode(r, true)
					g, err := d.Group(ids, mode)
					if err != nil {
						t.Fatal(err)
					}
					g.Hole = false
					desc = fmt.Sprintf("group %v as %s", ids, mode)
				}
			}
			if desc == "" {
				continue
			}
			btCheck(t, d, fmt.Sprintf("seed %d step %d (%s)", seed, step, desc))
		}
	}
}

// A hand-built sequence with volumes worked out by hand: a plate with
// corner holes, a boss on top, a bore through both, clipped to one half,
// then used as a hole in a block, dissolved, and made solid again and
// intersected with the block.
func TestBooleanSequenceByHand(t *testing.T) {
	d := New("t")
	box := func(lo, hi geom.Vec3, hole bool) *Node {
		n, _ := d.AddShape("box", geom.Vec3{}, hole)
		n.Size = hi.Sub(lo)
		n.M = geom.Translate(lo.Add(hi).Mul(0.5))
		return n
	}
	vol := func(n *Node) float64 { return n.WorldMesh(false).Volume() }
	closed := func(n *Node, what string) {
		t.Helper()
		if open, _ := n.WorldMesh(false).EdgeCheck(1e-9); open != 0 {
			t.Fatalf("%s: %d open edges", what, open)
		}
	}
	plate := box(geom.V(0, 0, 0), geom.V(60, 40, 10), false)
	var holes []int
	for _, c := range [][2]float64{{5, 5}, {51, 5}, {5, 31}, {51, 31}} {
		holes = append(holes, box(geom.V(c[0], c[1], -5), geom.V(c[0]+4, c[1]+4, 15), true).ID)
	}
	g1, _ := d.Group(append([]int{plate.ID}, holes...), ModeUnion)
	if v := vol(g1); !near(v, 24000-4*160) {
		t.Fatalf("plate with holes %v", v)
	}
	closed(g1, "plate")
	boss := box(geom.V(20, 10, 10), geom.V(40, 30, 25), false)
	bore := box(geom.V(25, 15, -5), geom.V(35, 25, 30), true)
	g2, _ := d.Group([]int{g1.ID, boss.ID, bore.ID}, ModeUnion)
	if v := vol(g2); !near(v, 23360+6000-1000-1500) {
		t.Fatalf("boss and bore %v", v)
	}
	closed(g2, "boss and bore")
	clip := box(geom.V(0, 0, 0), geom.V(30, 40, 30), false)
	g3, _ := d.Group([]int{g2.ID, clip.ID}, ModeIntersect)
	const left = 12000 - 320 + 3000 - 500 - 750
	if v := vol(g3); !near(v, left) {
		t.Fatalf("clipped %v, want %v", v, left)
	}
	closed(g3, "clipped")
	// the result as a hole in a block, moved first
	g3.M = geom.Translate(geom.V(3, -2, 1)).Mul(g3.M)
	if v := vol(g3); !near(v, left) {
		t.Fatalf("moved %v", v)
	}
	block := box(geom.V(-10, -10, -10), geom.V(70, 50, 40), false)
	g3.Hole = true
	g4, _ := d.Group([]int{block.ID, g3.ID}, ModeUnion)
	if v := vol(g4); !near(v, 240000-left) {
		t.Fatalf("block minus result %v", v)
	}
	closed(g4, "block minus result")
	// dissolved: the hole result stays a hole, so only the block exports
	d.Ungroup([]int{g4.ID})
	total := 0.0
	for _, n := range d.Nodes {
		total += vol(n)
	}
	if !near(total, 240000) {
		t.Fatalf("after ungroup %v", total)
	}
	// made solid again and intersected with the block: the result itself
	g3.Hole = false
	g5, _ := d.Group([]int{block.ID, g3.ID}, ModeIntersect)
	if v := vol(g5); !near(v, left) {
		t.Fatalf("intersect with the block %v", v)
	}
	closed(g5, "intersect with the block")
}
