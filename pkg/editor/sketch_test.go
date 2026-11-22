package editor

import (
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

func sketchSquare(x0, y0, x1, y1 float64) *shapes.Sketch {
	return &shapes.Sketch{Paths: []shapes.SketchPath{{Closed: true, Points: []shapes.SketchPoint{
		{P: [2]float64{x0, y0}}, {P: [2]float64{x1, y0}}, {P: [2]float64{x1, y1}}, {P: [2]float64{x0, y1}}}}}}
}

// Adding and changing a sketch are one undo step each; a drawing that
// builds nothing, or a locked shape, changes nothing and leaves no step.
func TestSketchEdits(t *testing.T) {
	e := New()
	depth := e.Hist.Depth()
	id, err := e.AddSketchXf("sketch", sketchSquare(0, 0, 10, 10), geom.Translate(geom.V(50, 0, 0)), false)
	if err != nil {
		t.Fatal(err)
	}
	n := e.Doc.Find(id)
	if e.Hist.Depth() != depth+1 || len(e.Sel) != 1 || e.Sel[0] != id || n == nil {
		t.Fatalf("depth %d, selection %v", e.Hist.Depth()-depth, e.Sel)
	}
	if b := n.WorldBounds(); !nearlyE(b.Min.X, 50) || !nearlyE(b.Max.X, 60) {
		t.Errorf("bounds %v", b)
	}
	before := ptDocBytes(t, e)
	for _, sk := range []*shapes.Sketch{{}, {Paths: []shapes.SketchPath{{Points: []shapes.SketchPoint{{}}, Stroke: -2}}}} {
		if err := e.SetSketch(id, sk); err == nil {
			t.Errorf("%+v was taken", sk)
		}
	}
	if err := e.SetSketch(9999, sketchSquare(0, 0, 1, 1)); err == nil {
		t.Error("no such node, but no error")
	}
	if string(ptDocBytes(t, e)) != string(before) || e.Hist.Depth() != depth+1 {
		t.Error("a refused drawing changed the design")
	}
	if err := e.SetSketch(id, sketchSquare(0, 0, 20, 10)); err != nil {
		t.Fatal(err)
	}
	if b := e.Doc.Find(id).WorldBounds(); !nearlyE(b.Size().X, 20) {
		t.Errorf("size %v", b.Size())
	}
	e.Undo()
	if string(ptDocBytes(t, e)) != string(before) {
		t.Error("undo didn't restore the drawing")
	}
	e.Select(id)
	e.SetLocked(true)
	if err := e.SetSketch(id, sketchSquare(0, 0, 20, 10)); err == nil {
		t.Error("a locked sketch was changed")
	}
	e.Undo() // the lock
	e.Undo() // the shape
	if len(e.Doc.Nodes) != 0 {
		t.Errorf("%d nodes after undoing the add", len(e.Doc.Nodes))
	}
}
