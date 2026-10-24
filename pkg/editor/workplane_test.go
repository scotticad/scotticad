package editor

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// A workplane attached to a tilted box sits on the box's own bottom and
// measures, resizes and drops along the box's axes.
func TestAttachedWorkplane(t *testing.T) {
	e := New()
	id, _ := e.AddShape("box", geom.V(0, 0, 0), false) // 20 cube
	e.Select(id)
	e.Rotate(nil, 0, 30) // tilt about X
	if err := e.AttachWorkplane(id); err != nil {
		t.Fatal(err)
	}
	b := e.SelectionBoundsIn()
	s := b.Size()
	if math.Abs(s.X-20) > 1e-6 || math.Abs(s.Y-20) > 1e-6 || math.Abs(s.Z-20) > 1e-6 || math.Abs(b.Min.Z) > 1e-6 {
		t.Fatalf("bounds in the workplane %v..%v", b.Min, b.Max)
	}
	// stretch along the box's own Y: it stays a box (no shear) 20 x 40 x 20
	nb := b
	nb.Max.Y = nb.Min.Y + 40
	if err := e.SetBoundsIn(nil, nb); err != nil {
		t.Fatal(err)
	}
	n := e.Doc.Find(id)
	if math.Abs(n.Size.X-20) > 1e-6 || math.Abs(n.Size.Y-40) > 1e-6 || math.Abs(n.Size.Z-20) > 1e-6 {
		t.Fatalf("size after resize %v", n.Size)
	}
	// lifted off the workplane along its normal, Drop puts it back
	e.MoveIn(nil, geom.V(0, 0, 7))
	if z := e.SelectionBoundsIn().Min.Z; math.Abs(z-7) > 1e-6 {
		t.Fatalf("lifted to %v", z)
	}
	e.Drop(nil)
	if z := e.SelectionBoundsIn().Min.Z; math.Abs(z) > 1e-6 {
		t.Fatalf("dropped to %v", z)
	}
	// align and mirror work along the workplane's axes: a second box,
	// aligned to the first's min on the workplane's Y, lines up along the
	// tilted box's own Y
	id2, _ := e.AddShape("box", geom.V(60, 0, 0), false)
	e.Select(id2)
	e.Rotate(nil, 0, 30)
	e.Select(id, id2)
	e.Align(nil, 1, -1)
	b1 := doc.BoundsIn([]*doc.Node{e.Doc.Find(id)}, e.Work())
	b2 := doc.BoundsIn([]*doc.Node{e.Doc.Find(id2)}, e.Work())
	if math.Abs(b1.Min.Y-b2.Min.Y) > 1e-6 {
		t.Fatalf("aligned min Y in the workplane: %v vs %v", b1.Min.Y, b2.Min.Y)
	}
	// mirroring keeps the bounds in the workplane (a flip in its frame)
	before := doc.BoundsIn([]*doc.Node{e.Doc.Find(id)}, e.Work())
	e.Select(id)
	e.Mirror(nil, 0)
	after := doc.BoundsIn([]*doc.Node{e.Doc.Find(id)}, e.Work())
	if after.Min.Sub(before.Min).Len() > 1e-6 || after.Max.Sub(before.Max).Len() > 1e-6 {
		t.Fatalf("mirror moved it in the workplane: %v..%v -> %v..%v", before.Min, before.Max, after.Min, after.Max)
	}
	// back to the default workplane
	e.AttachWorkplane(0)
	if e.Workplane != nil || e.PlaneZ() != 0 {
		t.Fatal("workplane not reset")
	}
}

// Lay flat puts a face on the workplane: a wedge laid on its slope ends up
// with the slope face down, resting on the plane.
func TestLayFlat(t *testing.T) {
	e := New()
	id, _ := e.AddShape("wedge", geom.V(0, 0, 0), false)
	n := e.Doc.Find(id)
	// the wedge's slope: the face whose normal has both a +Z and a sideways part
	var slope geom.Vec3
	var slopeArea float64
	for _, tr := range n.WorldMesh(false).Tris {
		nm := tr.Normal()
		if nm.Z > 0.3 && nm.Z < 0.95 {
			slope, slopeArea = nm, slopeArea+tr.Area()
		}
	}
	if slopeArea == 0 {
		t.Fatal("no sloped face found")
	}
	e.Select(id)
	depth := e.Hist.Depth()
	if err := e.LayFlat(nil, slope); err != nil {
		t.Fatal(err)
	}
	if e.Hist.Depth() != depth+1 {
		t.Fatal("lay flat should be one undo step")
	}
	m := e.Doc.Find(id).WorldMesh(false)
	down := 0.0
	for _, tr := range m.Tris {
		if tr.Normal().Z < -0.999 {
			down += tr.Area()
		}
	}
	if math.Abs(down-slopeArea) > 1e-6 || math.Abs(m.Bounds().Min.Z) > 1e-6 {
		t.Fatalf("face down area %v (slope %v), lowest point %v", down, slopeArea, m.Bounds().Min.Z)
	}
	// the top face of a box: a half turn
	b, _ := e.AddShape("box", geom.V(60, 0, 0), false)
	e.Select(b)
	e.LayFlat(nil, geom.V(0, 0, 1))
	if bb := e.Doc.Find(b).WorldBounds(); math.Abs(bb.Min.Z) > 1e-6 || math.Abs(bb.Size().Z-20) > 1e-6 {
		t.Fatalf("box after laying on its top: %v..%v", bb.Min, bb.Max)
	}
}

// Split cuts along the workplane's axes and undoes in one step.
func TestSplitOnAttachedWorkplane(t *testing.T) {
	e := New()
	base, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	e.Select(base)
	e.Rotate(nil, 0, 30)
	e.AttachWorkplane(base)
	e.Select(base)
	depth := e.Hist.Depth()
	ids, err := e.Split(nil, 2, 12, doc.KeepBoth, 0)
	if err != nil || len(ids) != 2 || len(e.Sel) != 2 {
		t.Fatalf("split: %v %v sel %v", err, ids, e.Sel)
	}
	top := doc.BoundsIn([]*doc.Node{e.Doc.Find(ids[1])}, e.Work())
	if math.Abs(top.Min.Z-12) > 1e-6 || math.Abs(top.Size().Z-8) > 1e-6 {
		t.Fatalf("the top half in the workplane: %v..%v", top.Min, top.Max)
	}
	if e.Hist.Depth() != depth+1 {
		t.Fatalf("split took %d undo steps", e.Hist.Depth()-depth)
	}
	e.Undo()
	if len(e.Doc.Nodes) != 1 || e.Doc.Nodes[0].ID != base {
		t.Fatal("undo didn't restore the shape")
	}
	// locked shapes aren't split
	e.Select(base)
	e.SetLocked(true)
	if _, err := e.Split(nil, 2, 12, doc.KeepBoth, 0); err == nil {
		t.Fatal("a locked shape was split")
	}
}

// On a workplane attached to a tilted shape, an import stands on that
// plane, square to its axes, like a shape placed from the library.
func TestImportOnAttachedWorkplane(t *testing.T) {
	stl := filepath.Join(t.TempDir(), "part.stl")
	f, _ := os.Create(stl)
	geom.WriteSTL(f, shapes.Build("box", geom.V(10, 6, 4), shapes.Values{}), "part")
	f.Close()

	e := New()
	id, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	e.Select(id)
	e.Rotate(nil, 1, 40)
	e.AttachWorkplane(id)
	imp, err := e.ImportOn(stl, geom.V(5, -3, 0), DefaultImportOptions())
	if err != nil {
		t.Fatal(err)
	}
	b := doc.BoundsIn([]*doc.Node{e.Doc.Find(imp)}, e.Work())
	if b.Size().Sub(geom.V(10, 6, 4)).Len() > 1e-6 || b.Center().Sub(geom.V(5, -3, 2)).Len() > 1e-6 {
		t.Fatalf("import in the workplane: %v..%v", b.Min, b.Max)
	}
	// a library shape placed at the same spot lands the same way
	sid, _ := e.AddShapeXf("box", e.Work().Mul(geom.Translate(geom.V(5, -3, 0))), false)
	if sb := doc.BoundsIn([]*doc.Node{e.Doc.Find(sid)}, e.Work()); math.Abs(sb.Min.Z) > 1e-6 {
		t.Fatalf("shape base at %v in the workplane", sb.Min.Z)
	}
}
