package doc

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestSplit(t *testing.T) {
	d := New("t")
	box, _ := d.AddShape("box", geom.V(0, 0, 0), false) // 20 cube, z 0..20
	id := box.ID
	halves, err := d.Split([]int{id}, geom.Identity(), 2, 5, KeepBoth, 0)
	if err != nil || len(halves) != 2 || len(d.Nodes) != 2 {
		t.Fatalf("split: %v, %d halves, %d nodes", err, len(halves), len(d.Nodes))
	}
	below, above := halves[0], halves[1]
	if v := below.WorldMesh(false).Volume(); !near(v, 20*20*5) {
		t.Fatalf("below volume %v", v)
	}
	if v := above.WorldMesh(false).Volume(); !near(v, 20*20*15) {
		t.Fatalf("above volume %v", v)
	}
	if b := above.WorldBounds(); !near(b.Min.Z, 5) || !near(b.Max.Z, 20) {
		t.Fatalf("above spans z %v..%v", b.Min.Z, b.Max.Z)
	}
	for _, h := range halves {
		if o, nm := h.WorldMesh(false).EdgeCheck(1e-7); o != 0 || nm != 0 {
			t.Fatalf("a half has %d open and %d non-manifold edges", o, nm)
		}
	}
	// ungrouping a half gives the whole shape back, and the hole box
	parts := d.Ungroup([]int{below.ID})
	if len(parts) != 2 || parts[0].Kind != "box" || parts[0].Hole || !parts[1].Hole {
		t.Fatalf("ungrouped %+v", parts)
	}
	if b := parts[0].WorldBounds(); !near(b.Size().Z, 20) || !near(b.Min.Z, 0) {
		t.Fatalf("the shape came back as %v..%v", b.Min, b.Max)
	}
}

func TestSplitKeepGapAndMisses(t *testing.T) {
	d := New("t")
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("cylinder", geom.V(50, 0, 0), false) // x 40..60
	// a plane at x = 0 crosses the box only; keep the + side, gap 4
	halves, err := d.Split([]int{a.ID, b.ID}, geom.Identity(), 0, 0, KeepAbove, 4)
	if err != nil || len(halves) != 1 || len(d.Nodes) != 2 {
		t.Fatalf("%v, %d halves, %d nodes", err, len(halves), len(d.Nodes))
	}
	if bb := halves[0].WorldBounds(); !near(bb.Min.X, 4) || !near(bb.Max.X, 14) {
		t.Fatalf("kept half spans x %v..%v, want 4..14", bb.Min.X, bb.Max.X)
	}
	if d.Find(b.ID) == nil {
		t.Fatal("the cylinder the plane missed was changed")
	}
	if _, err := d.Split([]int{b.ID}, geom.Identity(), 2, 50, KeepBoth, 0); err == nil {
		t.Fatal("a plane above the shape split it")
	}
	if _, err := d.Split([]int{b.ID}, geom.Identity(), 2, 0, KeepBoth, 0); err == nil {
		t.Fatal("a plane on the shape's base split it")
	}
}

// On a tilted workplane the plane follows the workplane's axes.
func TestSplitInATiltedFrame(t *testing.T) {
	d := New("t")
	box, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	f := geom.RotateX(math.Pi / 6)
	Apply([]*Node{box}, f) // the box lies square to the frame
	halves, err := d.Split([]int{box.ID}, f, 2, 8, KeepBoth, 0)
	if err != nil {
		t.Fatal(err)
	}
	if v := halves[0].WorldMesh(false).Volume(); !near(v, 20*20*8) {
		t.Fatalf("below volume %v", v)
	}
	if bb := BoundsIn([]*Node{halves[1]}, f); !near(bb.Min.Z, 8) || !near(bb.Max.Z, 20) || !near(bb.Size().X, 20) {
		t.Fatalf("above in the frame %v..%v", bb.Min, bb.Max)
	}
}

// A note goes with the half it is on, not with both.
func TestSplitNotesGoWithTheirHalf(t *testing.T) {
	d := New("t")
	box, _ := d.AddShape("box", geom.V(0, 0, 0), false) // x -10..10
	box.Notes = []Note{{P: box.NoteParam(geom.V(10, 0, 0)), Text: "right face"}}
	halves, err := d.Split([]int{box.ID}, geom.Identity(), 0, 0, KeepBoth, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := func(n *Node) int {
		c := 0
		n.Walk(func(x *Node) { c += len(x.Notes) })
		return c
	}
	if count(halves[0]) != 0 || count(halves[1]) != 1 {
		t.Fatalf("notes: left half %d, right half %d", count(halves[0]), count(halves[1]))
	}
}

// A split hole stays a hole, and a bundle keeps its parts' colours.
func TestSplitHoleAndBundle(t *testing.T) {
	d := New("t")
	h, _ := d.AddShape("cylinder", geom.V(0, 0, 0), true)
	halves, err := d.Split([]int{h.ID}, geom.Identity(), 2, 10, KeepBoth, 0)
	if err != nil || !halves[0].Hole || !halves[1].Hole {
		t.Fatalf("%v %+v", err, halves)
	}
	p, _ := d.AddShape("projectbox", geom.V(80, 0, 0), false)
	halves, err = d.Split([]int{p.ID}, geom.Identity(), 2, 10, KeepBoth, 0)
	if err != nil || !halves[0].IsBundle() || !halves[1].IsBundle() {
		t.Fatalf("bundle halves: %v %+v", err, halves)
	}
	// a bundle with a hole part: the hole cuts nothing in a bundle, and
	// mustn't start to in its halves
	box, _ := d.AddShape("box", geom.V(200, 0, 0), false)
	hole, _ := d.AddShape("box", geom.V(200, 0, 0), true)
	ScaleAbout([]*Node{hole}, geom.V(0.5, 0.5, 1.5), geom.V(200, 0, 0))
	g, _ := d.Group([]int{box.ID, hole.ID}, ModeBundle)
	before := g.WorldMesh(false).Volume()
	halves, err = d.Split([]int{g.ID}, geom.Identity(), 2, 10, KeepBoth, 0)
	if err != nil {
		t.Fatal(err)
	}
	after := halves[0].WorldMesh(false).Volume() + halves[1].WorldMesh(false).Volume()
	if !near(before, 8000) || !near(after, before) {
		t.Fatalf("volume %v before, %v after", before, after)
	}
}

// A bundle's notes go with the half they are on, as a shape's do.
func TestSplitBundleNotesGoWithTheirHalf(t *testing.T) {
	d := New("t")
	a, _ := d.AddShape("box", geom.V(-20, 0, 0), false) // x -30..-10
	b, _ := d.AddShape("box", geom.V(20, 0, 0), false)  // x 10..30
	g, _ := d.Group([]int{a.ID, b.ID}, ModeBundle)
	inv, _ := g.M.Inverse()
	g.Notes = []Note{{P: g.NoteParam(inv.Point(geom.V(30, 0, 10))), Text: "right end"}}
	halves, err := d.Split([]int{g.ID}, geom.Identity(), 0, 0, KeepBoth, 0)
	if err != nil || len(halves) != 2 {
		t.Fatalf("%v %v", err, halves)
	}
	if len(halves[0].Notes) != 0 || len(halves[1].Notes) != 1 {
		t.Fatalf("notes: left half %d, right half %d", len(halves[0].Notes), len(halves[1].Notes))
	}
}
