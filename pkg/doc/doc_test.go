package doc

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

func TestGroupSaveReloadUngroup(t *testing.T) {
	d := New("t")
	box, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	hole, _ := d.AddShape("box", geom.V(0, 0, 0), true)
	hole.Size = geom.V(10, 10, 30)
	hole.M = geom.Translate(geom.V(0, 0, 10))
	g, err := d.Group([]int{box.ID, hole.ID}, ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	if v := g.WorldMesh(false).Volume(); math.Abs(v-6000) > 1e-3 {
		t.Fatalf("group volume %v", v)
	}
	// Nest: group the union with another box as a bundle, then save/reload.
	other, _ := d.AddShape("cylinder", geom.V(40, 0, 0), false)
	b, _ := d.Group([]int{g.ID, other.ID}, ModeBundle)
	data, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	d2, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(d2.Nodes) != 1 || len(d2.Nodes[0].Children) != 2 {
		t.Fatalf("tree not preserved: %d nodes", len(d2.Nodes))
	}
	if d2.Nodes[0].GroupMode != ModeBundle || d2.Nodes[0].Children[0].GroupMode != ModeUnion {
		t.Fatal("group modes not preserved")
	}
	// Ungroup all the way back to primitives after reload.
	rel := d2.Ungroup([]int{b.ID})
	if len(rel) != 2 {
		t.Fatalf("released %d", len(rel))
	}
	rel = d2.Ungroup([]int{g.ID})
	if len(rel) != 2 {
		t.Fatalf("released %d", len(rel))
	}
	var holeBack *Node
	for _, n := range d2.Nodes {
		if n.Hole {
			holeBack = n
		}
	}
	if holeBack == nil {
		t.Fatal("hole flag lost")
	}
	bb := holeBack.Bounds()
	if math.Abs(bb.Min.Z-(-5)) > 1e-9 || math.Abs(bb.Size().Z-30) > 1e-9 {
		t.Fatalf("hole bounds after round trip %v", bb)
	}
}

func TestIntersectGroup(t *testing.T) {
	d := New("t")
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("box", geom.V(10, 0, 0), false) // overlaps a by 10 mm in X
	g, err := d.Group([]int{a.ID, b.ID}, ModeIntersect)
	if err != nil {
		t.Fatal(err)
	}
	if v := g.WorldMesh(false).Volume(); math.Abs(v-4000) > 1e-6 {
		t.Fatalf("intersect volume %v, want 4000", v)
	}
	if g.Label() != "Intersect" {
		t.Fatal(g.Label())
	}
	d.Ungroup([]int{g.ID})
	if len(d.Nodes) != 2 {
		t.Fatal("ungroup")
	}
}

func TestScaleFoldsIntoSize(t *testing.T) {
	d := New("t")
	n, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	ScaleAbout([]*Node{n}, geom.V(2, 1, 0.5), geom.V(0, 0, 0))
	if n.Size.Sub(geom.V(40, 20, 10)).Len() > 1e-9 {
		t.Fatalf("size %v", n.Size)
	}
	if s := n.M.AxisScales(); s.Sub(geom.V(1, 1, 1)).Len() > 1e-9 {
		t.Fatalf("matrix still scaled %v", s)
	}
}

func TestHistory(t *testing.T) {
	var h History
	h.Push([]byte("a"))
	h.Push([]byte("b"))
	s, _ := h.Undo([]byte("c"))
	if string(s) != "b" {
		t.Fatal(string(s))
	}
	s, _ = h.Redo([]byte("b"))
	if string(s) != "c" {
		t.Fatal(string(s))
	}
}
