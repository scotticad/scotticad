package doc

// Nested booleans of turned shapes, where no grid can say what the volume
// should be, checked through the identities that hold exactly whatever
// the shapes are: at every group of the tree
//
//	vol(A ∪ B) + vol(A ∩ B) = vol(A) + vol(B)
//	vol(S − H) + vol(S ∩ H) = vol(S)
//
// where A and B are the group's two solid children (each a tree of its
// own), H its hole and S the union without it. Each is evaluated as its
// own group, so a mistake in nesting, in how a hole group cuts, or in a
// child's own result shows as a broken identity. Every result must also
// be closed.

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

var brKinds = []string{"box", "box", "cylinder", "sphere", "wedge", "pyramid", "polygon", "roof"}

// brLeaf is a random shape, sized, turned and placed so that the leaves
// of a tree overlap often.
func brLeaf(d *Document, r *rand.Rand) *Node {
	n, _ := d.AddShape(brKinds[r.Intn(len(brKinds))], geom.Vec3{}, false)
	n.Size = geom.V(6+14*r.Float64(), 6+14*r.Float64(), 6+14*r.Float64())
	c := geom.V(10+10*r.Float64(), 10+10*r.Float64(), 10+10*r.Float64())
	n.M = geom.Translate(c)
	for ax := 0; ax < 3; ax++ {
		if r.Intn(2) == 0 {
			RotateAbout([]*Node{n}, ax, 360*r.Float64(), c)
		}
	}
	return n
}

// brTree groups the document's nodes into a binary tree of groups, each
// with two solid children and sometimes a hole, turned about its centre.
func brTree(d *Document, r *rand.Rand) *Node {
	for len(d.Nodes) > 1 {
		ids := []int{d.Nodes[0].ID, d.Nodes[1].ID}
		if len(d.Nodes) > 2 && r.Intn(2) == 0 {
			h := d.Nodes[2]
			h.Hole = true
			ids = append(ids, h.ID)
		}
		mode := ModeUnion
		if r.Intn(4) == 0 {
			mode = ModeIntersect
		}
		g, err := d.Group(ids, mode)
		if err != nil {
			panic(err)
		}
		g.Hole = false
		if r.Intn(2) == 0 {
			RotateAbout([]*Node{g}, r.Intn(3), 360*r.Float64(), g.M.Point(geom.Vec3{}))
		}
		// to the back of the list, so the next pair is fresh nodes first
		d.Nodes = append(d.Nodes[1:], d.Nodes[0])
	}
	return d.Nodes[0]
}

// brGroup is a scratch group of clones, evaluated on its own.
func brGroup(mode string, ns ...*Node) *Node {
	g := &Node{Kind: KindGroup, GroupMode: mode, M: geom.Identity()}
	for _, n := range ns {
		g.Children = append(g.Children, n.Clone())
	}
	return g
}

func brVol(n *Node) float64 { return n.WorldMesh(false).Volume() }

func brClosed(t *testing.T, n *Node, what string) {
	t.Helper()
	if open, _ := n.WorldMesh(false).EdgeCheck(1e-9); open != 0 {
		t.Errorf("%s: %d open edges", what, open)
	}
}

// brCheck checks the identities at g and at every group below it.
func brCheck(t *testing.T, g *Node, what string) {
	t.Helper()
	var solids, holes []*Node
	for _, c := range g.Children {
		if c.Hole {
			holes = append(holes, c)
		} else {
			solids = append(solids, c)
		}
		if c.IsGroup() {
			brCheck(t, c, what+"/"+fmt.Sprint(c.ID))
		}
	}
	if len(solids) != 2 {
		t.Fatalf("%s: %d solid children", what, len(solids))
	}
	a, b := solids[0], solids[1]
	a2, b2 := a.Clone(), b.Clone()
	a2.Hole, b2.Hole = false, false
	va, vb := brVol(a2), brVol(b2)
	union, inter := brGroup(ModeUnion, a2, b2), brGroup(ModeIntersect, a2, b2)
	vu, vi := brVol(union), brVol(inter)
	tol := 1e-6 * math.Max(1, va+vb)
	if diff := vu + vi - va - vb; math.Abs(diff) > tol {
		t.Errorf("%s: union %.6f + intersection %.6f != %.6f + %.6f (off by %.3g)", what, vu, vi, va, vb, diff)
	}
	brClosed(t, union, what+" union")
	brClosed(t, inter, what+" intersection")
	// the group itself: its solids combined (as its mode says) less its hole
	s := brGroup(g.Mode(), a2, b2)
	vs := brVol(s)
	vg := brVol(brGroup(g.Mode(), g.Children...))
	if len(holes) == 0 {
		if math.Abs(vg-vs) > tol {
			t.Errorf("%s: the group's volume %.6f, its solids' %.6f", what, vg, vs)
		}
		return
	}
	h := holes[0].Clone()
	h.Hole = false
	cut := brGroup(ModeIntersect, s, h)
	vc := brVol(cut)
	if diff := vg + vc - vs; math.Abs(diff) > tol {
		t.Errorf("%s: result %.6f + (solids ∩ hole) %.6f != solids %.6f (off by %.3g)", what, vg, vc, vs, diff)
	}
	brClosed(t, cut, what+" solids ∩ hole")
}

func TestRotatedBooleanTreesKeepTheIdentities(t *testing.T) {
	seeds := 25
	if testing.Short() {
		seeds = 6
	}
	for seed := 1; seed <= seeds; seed++ {
		r := rand.New(rand.NewSource(int64(seed)))
		d := New("t")
		for i, n := 0, 3+r.Intn(5); i < n; i++ {
			brLeaf(d, r)
		}
		top := brTree(d, r)
		brCheck(t, top, fmt.Sprintf("seed %d", seed))
		brClosed(t, top, fmt.Sprintf("seed %d top", seed))
		if t.Failed() {
			t.Fatalf("seed %d:\n%s", seed, btDump(d))
		}
	}
}
