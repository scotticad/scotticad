package editor

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
)

// Stretch wraps the selection in a stretch group in one undo step; its
// settings are then set like a shape's parameters, and undo takes back a
// change, an ungroup or the stretch itself.
func TestStretch(t *testing.T) {
	e := New()
	if _, err := e.Stretch(nil, 0, 0, 5); err == nil {
		t.Error("a stretch of nothing")
	}
	a, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	before := ptDocBytes(t, e)
	if _, err := e.Stretch(nil, 0, 40, 5); err == nil {
		t.Error("a plane that misses the box was accepted")
	}
	if _, err := e.Stretch(nil, 0, 0, -25); err == nil {
		t.Error("shrinking by more than the box's length was accepted")
	}
	if string(ptDocBytes(t, e)) != string(before) {
		t.Error("a stretch that failed changed the design")
	}
	depth := e.Hist.Depth()
	g, err := e.Stretch(nil, 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if e.Hist.Depth() != depth+1 || len(e.Sel) != 1 || e.Sel[0] != g {
		t.Errorf("%d undo steps, selection %v", e.Hist.Depth()-depth, e.Sel)
	}
	n := e.Doc.Find(g)
	if n == nil || !n.IsStretch() || len(n.Children) != 1 || n.Children[0].ID != a || !nearlyE(n.WorldBounds().Size().X, 30) {
		t.Fatalf("not a stretch of the box: %+v", n)
	}
	// its settings, one undo step each
	if err := e.SetParam(g, "length", 45); err != nil {
		t.Fatal(err)
	}
	if n = e.Doc.Find(g); !nearlyE(n.WorldBounds().Size().X, 45) || e.Hist.Depth() != depth+2 {
		t.Errorf("length 45: %v, %d undo steps", n.WorldBounds().Size(), e.Hist.Depth()-depth)
	}
	if err := e.SetParam(g, "axis", 2); err != nil {
		t.Fatal(err)
	}
	if s := e.Doc.Find(g).WorldBounds().Size(); !nearlyE(s.X, 20) || !nearlyE(s.Z, 45) {
		t.Errorf("along z: %v", s)
	}
	if err := e.SetParam(g, "sides", 3); err == nil {
		t.Error("a setting a stretch hasn't was accepted")
	}
	// the box in it changed: the stretch follows
	if err := e.SetParam(a, "length", 30); err != nil {
		t.Fatal(err)
	}
	if s := e.Doc.Find(g).WorldBounds().Size(); !nearlyE(s.X, 30) || !nearlyE(s.Z, 45) {
		t.Errorf("a longer box, stretched: %v", s)
	}
	// locked, its settings stay
	e.Select(g)
	e.SetLocked(true)
	if err := e.SetParam(g, "amount", 2); err == nil {
		t.Error("a locked stretch's setting was changed")
	}
	e.SetLocked(false)
	// ungrouped: the box baked in, a mesh; undone: the stretch again
	ids := e.Ungroup()
	if len(ids) != 1 || e.Doc.Find(ids[0]).Kind != doc.KindMesh || !nearlyE(e.Doc.Find(ids[0]).WorldBounds().Size().Z, 45) {
		t.Fatalf("ungrouped to %v", ids)
	}
	if !e.Undo() || !e.Doc.Nodes[0].IsStretch() {
		t.Fatal("undo didn't bring the stretch back")
	}
	// made a union, it has a stretch's numbers no more
	e.Select(g)
	if err := e.SetGroupMode(doc.ModeUnion); err != nil {
		t.Fatal(err)
	}
	if n = e.Doc.Find(g); n.IsStretch() || n.Params != nil || !nearlyE(n.WorldBounds().Size().Z, 20) {
		t.Errorf("as a union: %+v", n)
	}
	if err := e.SetGroupMode(doc.ModeStretch); err == nil {
		t.Error("a group was made a stretch by its mode")
	}
	// and undone to before it was made
	for e.Hist.Depth() > depth && e.Undo() {
	}
	if string(ptDocBytes(t, e)) != string(before) || len(e.Doc.Nodes) != 1 || e.Doc.Nodes[0].Kind != "box" {
		t.Errorf("undone all the way: %+v", e.Doc.Nodes)
	}
}

func nearlyE(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
