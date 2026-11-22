package doc

import (
	"math"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

func square(x0, y0, x1, y1 float64) shapes.SketchPath {
	return shapes.SketchPath{Closed: true, Points: []shapes.SketchPoint{
		{P: [2]float64{x0, y0}}, {P: [2]float64{x1, y0}}, {P: [2]float64{x1, y1}}, {P: [2]float64{x0, y1}}}}
}

func skBoxNear(b geom.Box, min, max geom.Vec3) bool {
	return b.Min.Sub(min).Len() < 1e-6 && b.Max.Sub(max).Len() < 1e-6
}

// An extruded sketch stands on the workplane with its drawing where it
// says; a revolve turns about the vertical line through the origin.
func TestAddSketch(t *testing.T) {
	d := New()
	n, err := d.AddSketch("sketch", &shapes.Sketch{Paths: []shapes.SketchPath{square(5, 0, 25, 10)}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if b := n.WorldBounds(); !skBoxNear(b, geom.V(5, 0, 0), geom.V(25, 10, 10)) {
		t.Errorf("extruded: bounds %v", b)
	}
	if v := n.WorldMesh(false).Volume(); math.Abs(v-2000) > 1e-6 {
		t.Errorf("extruded: volume %v", v)
	}
	r, err := d.AddSketch("revolve", &shapes.Sketch{Paths: []shapes.SketchPath{square(-10, 5, 0, 15)}}, false)
	if err != nil {
		t.Fatal(err)
	}
	b := r.WorldBounds()
	// the drawing's heights kept: drawn from 5 to 15, it stands 5 up
	if math.Abs(b.Center().X) > 1e-6 || math.Abs(b.Center().Y) > 1e-6 || math.Abs(b.Min.Z-5) > 1e-9 || math.Abs(b.Max.Z-15) > 1e-9 {
		t.Errorf("revolved: bounds %v", b)
	}
	// nothing to build: refused, nothing added
	for _, c := range []struct {
		kind string
		sk   *shapes.Sketch
	}{
		{"sketch", &shapes.Sketch{}},
		{"revolve", &shapes.Sketch{Paths: []shapes.SketchPath{square(1, 0, 10, 10)}}},
		{"box", &shapes.Sketch{Paths: []shapes.SketchPath{square(0, 0, 10, 10)}}},
		{"sketch", &shapes.Sketch{Paths: []shapes.SketchPath{{Points: []shapes.SketchPoint{{P: [2]float64{math.NaN(), 0}}}}}}},
	} {
		if _, err := d.AddSketch(c.kind, c.sk, false); err == nil {
			t.Errorf("%s %+v was added", c.kind, c.sk)
		}
	}
	if len(d.Nodes) != 2 {
		t.Errorf("%d nodes", len(d.Nodes))
	}
}

// Changing a drawing keeps what didn't change where it was, through a move
// and a resize; the shape owns its drawing (changing the one passed in
// later changes nothing).
func TestSetSketchKeepsFrame(t *testing.T) {
	d := New()
	sk := &shapes.Sketch{Paths: []shapes.SketchPath{square(0, 0, 20, 10)}}
	n, err := d.AddSketch("sketch", sk, false)
	if err != nil {
		t.Fatal(err)
	}
	sk.Paths[0].Points[0].P = [2]float64{-100, -100}
	if b := n.WorldBounds(); !skBoxNear(b, geom.V(0, 0, 0), geom.V(20, 10, 10)) {
		t.Fatalf("the drawing passed in was shared: %v", b)
	}
	// moved 7 along x, then made twice as tall and 1.5 times as wide in x
	n.M = geom.Translate(geom.V(7, 0, 0)).Mul(n.M)
	n.Size = geom.V(30, 10, 20)
	n.M = n.M.Mul(geom.Translate(geom.V(5, 0, 5)))
	before := n.WorldBounds()
	// a square stuck on: the old one doesn't move
	two := &shapes.Sketch{Paths: []shapes.SketchPath{square(0, 0, 20, 10), square(20, 0, 30, 5)}}
	if err := SetSketch(n, two); err != nil {
		t.Fatal(err)
	}
	after := n.WorldBounds()
	if after.Min.Sub(before.Min).Len() > 1e-6 || math.Abs(after.Max.Y-before.Max.Y) > 1e-6 || math.Abs(after.Max.Z-before.Max.Z) > 1e-6 {
		t.Errorf("moved: %v -> %v", before, after)
	}
	if want := before.Max.X + 10*1.5; math.Abs(after.Max.X-want) > 1e-6 {
		t.Errorf("max x %v, want %v", after.Max.X, want)
	}
	if err := SetSketch(n, &shapes.Sketch{}); err == nil {
		t.Error("an empty drawing was taken")
	}
	box, _ := d.AddShape("box", geom.Vec3{}, false)
	if err := SetSketch(box, two); err == nil {
		t.Error("a box took a drawing")
	}
}

// A design with a sketch saves as format 4 (older ScottiCADs refuse it
// rather than lose the drawing) and reads back the same; one without stays
// at the oldest version that holds it.
func TestSketchSaves(t *testing.T) {
	d := New()
	d.AddShape("box", geom.Vec3{}, false)
	if v := d.neededVersion(); v != 1 {
		t.Errorf("a box needs version %d", v)
	}
	sk := &shapes.Sketch{Paths: []shapes.SketchPath{
		square(0, 0, 20, 10),
		{Points: []shapes.SketchPoint{{P: [2]float64{0, 20}, Out: [2]float64{5, 5}, Kind: shapes.PointSmooth}, {P: [2]float64{20, 20}, Kind: shapes.PointAuto}}, Stroke: 3, Join: shapes.JoinRound, Cap: shapes.CapSquare},
	}}
	n, err := d.AddSketch("sketch", sk, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"version": 4`) {
		t.Errorf("not saved as version 4")
	}
	d2, err := Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	m := d2.FindAny(n.ID)
	if m == nil || shapes.SketchKey(m.Sketch) != shapes.SketchKey(n.Sketch) || !m.Hole {
		t.Fatalf("read back as %+v", m)
	}
	if a, b := n.WorldMesh(true).Volume(), m.WorldMesh(true).Volume(); math.Abs(a-b) > 1e-9 || a <= 0 {
		t.Errorf("volume %v, read back %v", a, b)
	}
	// a later ScottiCAD's file is refused, not half read
	if _, err := Unmarshal([]byte(`{"version": 5, "nodes": []}`)); err == nil {
		t.Error("version 5 was read")
	}
}

// A changed drawing changes the cache key, so the shape is rebuilt.
func TestSketchKeyChanges(t *testing.T) {
	d := New()
	n, _ := d.AddSketch("sketch", &shapes.Sketch{Paths: []shapes.SketchPath{square(0, 0, 20, 10)}}, false)
	v0 := n.WorldMesh(false).Volume()
	// (a square beside it: one inside would join it, changing nothing)
	if err := SetSketch(n, &shapes.Sketch{Paths: []shapes.SketchPath{square(0, 0, 20, 10), square(25, 0, 35, 10)}}); err != nil {
		t.Fatal(err)
	}
	if v1 := n.WorldMesh(false).Volume(); math.Abs(v1-(v0+1000)) > 1e-6 {
		t.Errorf("volume %v after another square, was %v", v1, v0)
	}
}

// A sketch's picture is saved and read back whole, never changes the
// shape, and undo snapshots hold it once (by hash), not a copy per edit.
func TestSketchImageSavedAndShared(t *testing.T) {
	d := New()
	data := strings.Repeat("A", 300_000) // (base64-ish: only stored, not decoded, here)
	sk := &shapes.Sketch{Paths: []shapes.SketchPath{square(0, 0, 20, 10)},
		Image: &shapes.SketchImage{Data: data, PW: 400, PH: 200, X: 5, Y: 6, Width: 40, Angle: 30, Opacity: 0.4}}
	n, err := d.AddSketch("sketch", sk, false)
	if err != nil {
		t.Fatal(err)
	}
	v := n.WorldMesh(false).Volume()
	if math.Abs(v-2000) > 1e-6 {
		t.Errorf("volume %v with a picture", v)
	}
	// saved and opened
	b, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	d2, err := Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if im := d2.Nodes[0].Sketch.Image; im == nil || *im != *sk.Image {
		t.Fatalf("read back as %+v", im)
	}
	// undo snapshots: the picture stored once, the snapshot small
	var st BlobStore
	s1, err := d.MarshalShared(&st)
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := d.MarshalShared(&st)
	if len(s1) > 20_000 || len(s2) > 20_000 || st.Len() != 1 {
		t.Errorf("snapshots %d and %d bytes, %d stored", len(s1), len(s2), st.Len())
	}
	if n.Sketch.Image.Data != data {
		t.Error("taking a snapshot changed the design's picture")
	}
	d3, err := UnmarshalShared(s1, &st)
	if err != nil {
		t.Fatal(err)
	}
	if im := d3.Nodes[0].Sketch.Image; im == nil || im.Data != data {
		t.Error("the snapshot's picture didn't come back")
	}
}
