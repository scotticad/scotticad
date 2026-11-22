package app

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

func xfApp(kind string, pts ...[2]float64) *App {
	a := rvApp()
	a.newSketch(kind, geom.Identity(), false)
	rvBoard(a)
	p := shapes.SketchPath{Closed: true}
	for _, q := range pts {
		p.Points = append(p.Points, shapes.SketchPoint{P: q})
	}
	a.sk.sk = &shapes.Sketch{Paths: []shapes.SketchPath{p}}
	a.sk.selectPath(0, false)
	return a
}

func xfPoints(t *testing.T, a *App, path int, want ...[2]float64) {
	t.Helper()
	got := a.sk.sk.Paths[path].Points
	if len(got) != len(want) {
		t.Fatalf("path %d has %d points, want %d", path, len(got), len(want))
	}
	for i, q := range got {
		if math.Abs(q.P[0]-want[i][0]) > 1e-9 || math.Abs(q.P[1]-want[i][1]) > 1e-9 {
			t.Errorf("path %d point %d at %v, want %v", path, i, q.P, want[i])
		}
	}
}

// Flips and quarter turns are about the selection's middle, exactly (a
// shape on the grid stays on it), turn the curve handles too, and undo.
func TestSketchFlipAndTurn(t *testing.T) {
	a := xfApp("sketch", [2]float64{0, 0}, [2]float64{10, 0}, [2]float64{0, 5})
	a.sk.sk.Paths[0].Points[1].Out = [2]float64{-2, 0} // (inward: the curve stays in the box)
	a.skFlip(true)
	xfPoints(t, a, 0, [2]float64{10, 0}, [2]float64{0, 0}, [2]float64{10, 5})
	if h := a.sk.sk.Paths[0].Points[1].Out; h != [2]float64{2, 0} {
		t.Errorf("flipped handle %v", h)
	}
	a.skUndo()
	xfPoints(t, a, 0, [2]float64{0, 0}, [2]float64{10, 0}, [2]float64{0, 5})
	a.sk.selectPath(0, false)
	a.skFlip(false)
	xfPoints(t, a, 0, [2]float64{0, 5}, [2]float64{10, 5}, [2]float64{0, 0})
	a.skUndo()
	a.sk.selectPath(0, false)
	a.skTurnBy(90) // about (5, 2.5), anticlockwise
	xfPoints(t, a, 0, [2]float64{7.5, -2.5}, [2]float64{7.5, 7.5}, [2]float64{2.5, -2.5})
	if h := a.sk.sk.Paths[0].Points[1].Out; h != [2]float64{0, -2} {
		t.Errorf("turned handle %v", h)
	}
	a.skTurnBy(-90)
	xfPoints(t, a, 0, [2]float64{0, 0}, [2]float64{10, 0}, [2]float64{0, 5})
	a.skTurnBy(30)
	p := a.sk.sk.Paths[0].Points[1].P
	c, s := math.Cos(math.Pi/6), math.Sin(math.Pi/6)
	if want := [2]float64{5 + 5*c + 2.5*s, 2.5 + 5*s - 2.5*c}; math.Abs(p[0]-want[0]) > 1e-9 || math.Abs(p[1]-want[1]) > 1e-9 {
		t.Errorf("turned 30°: %v, want %v", p, want)
	}
	// nothing while a path is being drawn
	n := len(a.sk.undo)
	a.sk.drawing = 0
	a.skFlip(true)
	if len(a.sk.undo) != n {
		t.Error("a flip while drawing")
	}
}

// A mirror copy lands beside the selection, mirrored about that edge of
// it, and is selected; the original stays. Half an outline becomes the
// whole one.
func TestSketchMirrorCopy(t *testing.T) {
	a := xfApp("sketch", [2]float64{0, 0}, [2]float64{10, 0}, [2]float64{10, 5}, [2]float64{0, 10})
	a.skMirrorCopy("right")
	if len(a.sk.sk.Paths) != 2 {
		t.Fatalf("%d paths", len(a.sk.sk.Paths))
	}
	xfPoints(t, a, 0, [2]float64{0, 0}, [2]float64{10, 0}, [2]float64{10, 5}, [2]float64{0, 10})
	xfPoints(t, a, 1, [2]float64{20, 0}, [2]float64{10, 0}, [2]float64{10, 5}, [2]float64{20, 10})
	if got := a.sk.selPaths(); len(got) != 1 || got[0] != 1 {
		t.Errorf("selected %v, want the copy", got)
	}
	// the halves join: one outline round both
	if cs := shapes.SketchOutlines(a.sk.sk); len(cs) != 1 || math.Abs(shapes.SignedArea(cs[0])-150) > 1e-9 {
		t.Errorf("outlines %v", cs)
	}
	a.skMirrorCopy("down") // of the copy, about its foot
	xfPoints(t, a, 2, [2]float64{20, 0}, [2]float64{10, 0}, [2]float64{10, -5}, [2]float64{20, -10})
	a.skUndo()
	a.skUndo()
	if len(a.sk.sk.Paths) != 1 {
		t.Errorf("undone, %d paths", len(a.sk.sk.Paths))
	}
}

// On a revolve's board a copy mirrored over the center line is moved back
// left of it.
func TestSketchMirrorCopyKeepsLeft(t *testing.T) {
	a := xfApp("revolve", [2]float64{-10, 0}, [2]float64{-2, 0}, [2]float64{-2, 5}, [2]float64{-10, 5})
	a.skMirrorCopy("right")
	for _, q := range a.sk.sk.Paths[1].Points {
		if q.P[0] > 1e-9 {
			t.Fatalf("a point right of the center line: %v", q.P)
		}
	}
	a.sk.selectPath(0, false)
	a.skTurnBy(90)
	for _, q := range a.sk.sk.Paths[0].Points {
		if q.P[0] > 1e-9 {
			t.Fatalf("turned right of the center line: %v", q.P)
		}
	}
}

// The hole switch sets the selected paths, as one undo step, and the
// drawing builds with the hole cut out.
func TestSketchHoleSwitch(t *testing.T) {
	a := xfApp("sketch", [2]float64{0, 0}, [2]float64{20, 0}, [2]float64{20, 20}, [2]float64{0, 20})
	a.sk.sk.Paths = append(a.sk.sk.Paths, shapes.SketchPath{Closed: true, Points: []shapes.SketchPoint{
		{P: [2]float64{5, 5}}, {P: [2]float64{15, 5}}, {P: [2]float64{15, 15}}, {P: [2]float64{5, 15}}}})
	a.sk.selectPath(1, false)
	if a.sk.skAllHoles() {
		t.Fatal("a hole before any was made")
	}
	a.skSetHole(true)
	if !a.sk.sk.Paths[1].Hole || a.sk.sk.Paths[0].Hole || !a.sk.skAllHoles() {
		t.Fatalf("holes %v %v", a.sk.sk.Paths[0].Hole, a.sk.sk.Paths[1].Hole)
	}
	if cs := shapes.SketchOutlines(a.sk.sk); len(cs) != 2 {
		t.Errorf("%d outlines, want the outside and the hole", len(cs))
	}
	a.skUndo()
	if a.sk.sk.Paths[1].Hole {
		t.Error("undo leaves the hole")
	}
}
