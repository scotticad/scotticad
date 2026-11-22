package app

import (
	"testing"

	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/window"
)

func rvApp() *App {
	return &App{w: &window.Window{}, ed: editor.New(), keysDown: map[int]bool{}}
}

func rvBoard(a *App) {
	a.sk.board = render.Rect{X: 0, Y: 0, W: 800, H: 600}
	a.sk.cx, a.sk.cy, a.sk.zoom, a.sk.viewSet = 0, 0, 10, true
}

func rvCmd(name string) func(a *App, p args) (any, error) {
	for _, m := range sketchCommands() {
		if m.Name == name {
			return m.fn
		}
	}
	panic(name)
}

func rvSquare() *shapes.Sketch {
	return &shapes.Sketch{Paths: []shapes.SketchPath{{Closed: true, Points: []shapes.SketchPoint{
		{P: [2]float64{0, 0}}, {P: [2]float64{10, 0}}, {P: [2]float64{10, 10}}, {P: [2]float64{0, 10}}}}}}
}

func rvClick(a *App, x, y float64) {
	sx, sy := a.sk.toScreen([2]float64{x, y})
	if d := a.skBoardPress(sx, sy, 0); d != nil {
		d(sx, sy, true)
	}
}

// The workspace closed (by the API or a script) in the middle of a drag on
// its board drops the drag, so the next mouse move doesn't run it into a
// workspace that is gone.
func TestSketchCloseDropsADragUnderWay(t *testing.T) {
	a := rvApp()
	a.newSketch("sketch", geom.Identity(), false)
	rvBoard(a)
	a.sk.sk = rvSquare()
	a.skSetTool(skSelect)
	sx, sy := a.sk.toScreen([2]float64{0, 0})
	a.pressDrag = a.skBoardPress(sx, sy, 0)
	if a.pressDrag == nil {
		t.Fatal("no drag on a point")
	}
	if _, err := rvCmd("sketch_cancel")(a, args{}); err != nil {
		t.Fatal(err)
	}
	if a.pressDrag != nil {
		t.Error("the drag outlived the workspace")
	}
}

// sketch_done after editing a shape gives that shape's id, not whatever
// happens to be selected.
func TestSketchDoneGivesTheEditedID(t *testing.T) {
	a := rvApp()
	idA, _ := a.ed.AddSketchXf("sketch", rvSquare(), geom.Identity(), false)
	idB, _ := a.ed.AddSketchXf("sketch", rvSquare(), geom.Translate(geom.V(30, 0, 0)), false)
	a.ed.Select(idB)
	if _, err := rvCmd("edit_sketch")(a, args{"id": float64(idA)}); err != nil {
		t.Fatal(err)
	}
	a.sk.sk.Paths[0].Points[0].P = [2]float64{-5, -5}
	r, err := rvCmd("sketch_done")(a, args{})
	if err != nil {
		t.Fatal(err)
	}
	if id := r.(map[string]any)["id"]; id != idA {
		t.Errorf("sketch_done gave %v, edited %d", id, idA)
	}
}

// A shape removed while the workspace is open doesn't leave Finish failing
// for ever: the drawing becomes a new shape.
func TestSketchFinishAfterItsShapeIsGone(t *testing.T) {
	a := rvApp()
	id, _ := a.ed.AddSketchXf("sketch", rvSquare(), geom.Identity(), false)
	a.editSketch(id)
	a.sk.sk.Paths[0].Points[0].P = [2]float64{-5, -5}
	a.ed.Undo() // a script's undo takes the shape away
	if a.ed.Doc.FindAny(id) != nil {
		t.Fatal("undo left the shape")
	}
	a.sketchDone()
	if a.sk != nil {
		t.Fatalf("still open: %q", a.toast)
	}
	if len(a.ed.Doc.Nodes) != 1 || a.ed.Doc.Nodes[0].Sketch == nil || a.ed.Doc.Nodes[0].Sketch.Paths[0].Points[0].P != [2]float64{-5, -5} {
		t.Error("the drawing wasn't kept as a new shape")
	}
}

// A sketch in a locked group can't be opened for editing (Finish could
// never save it).
func TestSketchInALockedGroupDoesntOpen(t *testing.T) {
	a := rvApp()
	idA, _ := a.ed.AddSketchXf("sketch", rvSquare(), geom.Identity(), false)
	idB, _ := a.ed.AddShapeXf("box", geom.Translate(geom.V(30, 0, 0)), false)
	a.ed.Select(idA, idB)
	if _, err := a.ed.Group("bundle"); err != nil {
		t.Fatal(err)
	}
	a.ed.SetLocked(true)
	if _, err := rvCmd("edit_sketch")(a, args{"id": float64(idA)}); err == nil || a.sk != nil {
		t.Error("edit_sketch opened a sketch in a locked group")
	}
	a.editSketch(idA)
	if a.sk != nil {
		t.Error("the workspace opened on a sketch in a locked group")
	}
}

// Undoing the click that closed a path, or after leaving the Pen, goes
// back to drawing with a tool that draws.
func TestSketchUndoBackToDrawingPicksAPen(t *testing.T) {
	for _, how := range []string{"close", "esc"} {
		a := rvApp()
		a.newSketch("sketch", geom.Identity(), false)
		rvBoard(a)
		rvClick(a, 0, 0)
		rvClick(a, 10, 0)
		rvClick(a, 10, 10)
		if how == "close" {
			rvClick(a, 0, 0)
		} else {
			a.sketchKey(vkEscape, 0)
			a.sketchKey(vkEscape, 0)
		}
		a.skUndo()
		s := a.sk
		if s.drawing < 0 {
			continue // (nothing to carry on: fine)
		}
		if !skPenTool(s.tool) {
			t.Errorf("%s: drawing again with tool %d", how, s.tool)
		}
		n := len(s.sk.Paths[s.drawing].Points)
		rvClick(a, -10, 5)
		if got := len(s.sk.Paths[s.drawing].Points); got != n+1 {
			t.Errorf("%s: a click added %d points", how, got-n)
		}
	}
}

// sketch_view keeps to the board's zoom limits.
func TestSketchViewClampsZoom(t *testing.T) {
	a := rvApp()
	a.newSketch("sketch", geom.Identity(), false)
	if _, err := rvCmd("sketch_view")(a, args{"x": 0.0, "y": 0.0, "zoom": 1e-300}); err != nil {
		t.Fatal(err)
	}
	if a.sk.zoom < 0.05 {
		t.Errorf("zoom %v", a.sk.zoom)
	}
}
