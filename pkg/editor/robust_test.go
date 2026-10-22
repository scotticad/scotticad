package editor

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
)

func finite(m geom.Mat4) bool {
	for _, v := range m {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// Shapes with no geometry (an intersect group of shapes that don't meet)
// have empty bounds; transforming them must not write NaN into the design,
// which would make it impossible to save.
func TestTransformsOnEmptyGeometry(t *testing.T) {
	e := New()
	a, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := e.AddShape("box", geom.V(100, 0, 0), false)
	e.Select(a, b)
	g, err := e.Group(doc.ModeIntersect)
	if err != nil {
		t.Fatal(err)
	}
	if !e.Doc.Find(g).WorldBounds().IsEmpty() {
		t.Fatal("expected an empty intersection")
	}
	e.Select(g)
	e.Rotate(nil, 2, 90)
	e.Mirror(nil, 0)
	e.Drop(nil)
	e.Align(nil, 0, 0)
	e.LayFlat(nil, geom.V(0, 0, 1))
	e.Resize(nil, geom.V(10, 10, 10))
	if !finite(e.Doc.Find(g).M) {
		t.Fatalf("matrix poisoned: %v", e.Doc.Find(g).M)
	}
	if err := e.Save(filepath.Join(t.TempDir(), "x.scotticad")); err != nil {
		t.Fatalf("save after transforms: %v", err)
	}
	// undo and redo still work
	e.Move(nil, geom.V(5, 0, 0))
	if !e.Undo() || !e.Redo() {
		t.Fatal("undo/redo")
	}
}

// Undo while a drag is open finishes the drag first, so history stays in
// order; a second BeginEdit keeps the first snapshot.
func TestUndoDuringADrag(t *testing.T) {
	e := New()
	id, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	e.Select(id)
	x := func() float64 { return e.Doc.Find(id).WorldBounds().Center().X }
	e.Move(nil, geom.V(10, 0, 0))
	e.BeginEdit()
	e.MoveLive(e.Selected(), geom.V(5, 0, 0))
	e.Undo() // undoes the drag
	if !near(x(), 10) {
		t.Fatalf("after undoing the drag x = %v, want 10", x())
	}
	e.Undo()
	if !near(x(), 0) {
		t.Fatalf("after the second undo x = %v, want 0", x())
	}
	if e.Hist.Depth() != 1 { // adding the box
		t.Fatalf("history has %d steps left, want 1", e.Hist.Depth())
	}

	e.BeginEdit()
	e.MoveLive(e.Selected(), geom.V(5, 0, 0))
	e.BeginEdit()
	e.MoveLive(e.Selected(), geom.V(5, 0, 0))
	e.EndEdit()
	e.Undo()
	if !near(x(), 0) {
		t.Fatalf("nested BeginEdit: undo went to x = %v, want 0", x())
	}
}

// A duplicate's notes are its own.
func TestDuplicateNotesAreIndependent(t *testing.T) {
	e := New()
	id, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	for _, s := range []string{"a", "b", "c"} {
		e.AddNote(id, geom.V(0, 0, 20), s)
	}
	e.Select(id)
	dup := e.Duplicate()[0]
	e.SetNoteText(dup, 0, "changed")
	e.AddNote(dup, geom.V(0, 0, 20), "dup-note")
	e.AddNote(id, geom.V(0, 0, 20), "orig-note")
	orig, copy := e.Doc.Find(id).Notes, e.Doc.Find(dup).Notes
	if orig[0].Text != "a" || copy[3].Text != "dup-note" || orig[3].Text != "orig-note" {
		t.Fatalf("notes shared: orig %v, copy %v", orig, copy)
	}
}

// Bounds measured in an attached workplane's frame follow a group's
// evaluation: while it is still being combined in the background they are
// the preview's, and must not be kept once the real result is in.
func TestFramedBoundsFollowEvaluation(t *testing.T) {
	doc.SetAsync(true)
	defer doc.SetAsync(false)
	e := New()
	base, _ := e.AddShape("box", geom.V(100, 0, 0), false)
	e.Select(base)
	e.Rotate(nil, 0, 30)
	e.AttachWorkplane(base) // a frame turned 30° about X
	a, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	h, _ := e.AddShape("box", geom.V(0, 0, 10), true) // cuts the top half off
	e.Select(h)
	e.Resize(nil, geom.V(40, 40, 40))
	e.Select(a, h)
	g, err := e.Group(doc.ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	e.Select(g)
	e.SelectionBoundsIn() // the preview's, while the group is evaluated
	doc.WaitIdle()
	got := e.SelectionBoundsIn().Size()
	s, c := math.Sin(math.Pi/6), math.Cos(math.Pi/6)
	if want := geom.V(20, 20*c+10*s, 20*s+10*c); got.Sub(want).Len() > 1e-6 {
		t.Fatalf("size in the frame %v, want %v (the 20 × 20 × 10 result, turned)", got, want)
	}
}

// A damaged file with null entries in its node lists opens without them
// instead of crashing.
func TestOpenFileWithNullNodes(t *testing.T) {
	for _, src := range []string{
		`{"version":1,"nodes":[null]}`,
		`{"version":1,"nodes":[null,{"id":1,"kind":"group","children":[null,{"id":2,"kind":"box"}]}]}`,
	} {
		d, err := doc.Unmarshal([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, x := range d.Nodes {
			x.Walk(func(*doc.Node) { n++ })
		}
		if n != strings.Count(src, `"kind"`) {
			t.Fatalf("%d nodes kept from %s", n, src)
		}
	}
	// a node without a matrix stands where it would with the identity
	d, err := doc.Unmarshal([]byte(`{"version":1,"nodes":[{"id":1,"kind":"box","size":{"X":20,"Y":20,"Z":20},"color":"#ff0000"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if b := d.Nodes[0].WorldBounds(); b.Size().Sub(geom.V(20, 20, 20)).Len() > 1e-9 {
		t.Fatalf("bounds %v..%v", b.Min, b.Max)
	}
}

// A script's hundreds of steps undo as one, back to where it started, even
// past the history's limit.
func TestLongGroupUndoesToItsStart(t *testing.T) {
	var h doc.History
	h.Push([]byte("earlier"))
	h.Begin()
	for i := 0; i < 600; i++ {
		h.Push([]byte(fmt.Sprintf("before step %d", i)))
		if h.Depth() > 501 {
			t.Fatalf("depth %d inside the group: the history grows without bound", h.Depth())
		}
	}
	h.End()
	s, ok := h.Undo([]byte("after"))
	if !ok || string(s) != "before step 0" {
		t.Fatalf("undo restored %q", s)
	}
	if h.Depth() != 1 {
		t.Fatalf("depth %d after the undo, want 1 (the earlier step)", h.Depth())
	}
	for i := 0; i < 700; i++ { // the limit still holds outside a group
		h.Push([]byte("x"))
	}
	if h.Depth() > 500 {
		t.Fatalf("depth %d", h.Depth())
	}
}

// Snapshots refer to imported meshes by hash, found again by the string's
// address. A new mesh given a freed string's address must not be taken for
// the old one (undo would then bring back the wrong mesh).
func TestSnapshotsKeepTheRightMeshData(t *testing.T) {
	e := New()
	id, _ := e.AddMesh("a", bigMesh(20000), geom.V(0, 0, 0))
	data := e.Doc.Find(id).MeshData
	e.Doc.Nodes = append(e.Doc.Nodes, &doc.Node{ID: 99, Kind: doc.KindMesh, MeshData: strings.Clone(data), Size: geom.V(1, 1, 1), M: geom.Identity(), Color: "#fff"})
	e.Doc.NextID = 100
	e.snapshot()
	for i := 0; i < 200; i++ {
		runtime.GC()
		other := strings.Repeat("B", len(data)) // other contents, the same length
		e.Doc.Nodes = append(e.Doc.Nodes[:1], &doc.Node{ID: 99, Kind: doc.KindMesh, MeshData: other, Size: geom.V(1, 1, 1), M: geom.Identity(), Color: "#fff"})
		back, err := doc.UnmarshalShared(e.snapshot(), &e.blobs)
		if err != nil {
			t.Fatal(err)
		}
		if back.Nodes[1].MeshData != other {
			t.Fatalf("round %d: the snapshot came back with another mesh's data", i)
		}
	}
}

func bigMesh(n int) *geom.Mesh {
	m := &geom.Mesh{}
	for i := 0; i < n; i++ {
		f := float64(i)
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{geom.V(f, 0, 0), geom.V(f+1, 0, 0), geom.V(f, 1, f)}})
	}
	return m
}

// Setting the length of a sheared shape (turned, then stretched along a
// world axis) lengthens it along its own axis only.
func TestLengthOfAShearedShape(t *testing.T) {
	e := New()
	id, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	e.Select(id)
	e.Rotate(nil, 2, 30)
	b := e.SelectionBounds()
	b.Max.X += b.Size().X // stretched along world X: now sheared
	if err := e.SetBounds(nil, b); err != nil {
		t.Fatal(err)
	}
	n := e.Doc.Find(id)
	before := []float64{n.AxisLength(0), n.AxisLength(1), n.AxisLength(2)}
	if err := e.SetParam(id, "length", 2*before[0]); err != nil {
		t.Fatal(err)
	}
	n = e.Doc.Find(id)
	if !near(n.AxisLength(0), 2*before[0]) || !near(n.AxisLength(1), before[1]) || !near(n.AxisLength(2), before[2]) {
		t.Fatalf("lengths %v, %v, %v; before %v", n.AxisLength(0), n.AxisLength(1), n.AxisLength(2), before)
	}
}

// An intersect group keeps what all its parts share, a bundle part counting
// as one: A ∩ (B ∪ C), not A ∩ B ∩ C.
func TestIntersectWithABundle(t *testing.T) {
	e := New()
	a, _ := e.AddShape("box", geom.V(0, 0, 0), false) // 20 cube, -10..10
	b, _ := e.AddShape("box", geom.V(-5, 0, 5), false)
	c, _ := e.AddShape("box", geom.V(5, 0, 5), false)
	// two 4 mm cubes inside the 20 mm one, apart
	e.SetBounds([]int{b}, geom.Box{Min: geom.V(-8, -2, 2), Max: geom.V(-4, 2, 6)})
	e.SetBounds([]int{c}, geom.Box{Min: geom.V(4, -2, 2), Max: geom.V(8, 2, 6)})
	e.Select(b, c)
	bun, _ := e.Group(doc.ModeBundle)
	e.Select(a, bun)
	g, err := e.Group(doc.ModeIntersect)
	if err != nil {
		t.Fatal(err)
	}
	if v := e.Doc.Find(g).WorldMesh(false).Volume(); !near(v, 2*64) {
		t.Fatalf("volume %v, want 128", v)
	}
}

// A workplane on a mirrored shape is right-handed: what is placed on it
// isn't mirrored.
func TestWorkplaneOnAMirroredShape(t *testing.T) {
	e := New()
	id, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	e.Select(id)
	e.Mirror(nil, 0)
	e.AttachWorkplane(id)
	if d := e.Work().Det3(); d < 0 {
		t.Fatalf("workplane determinant %v", d)
	}
	tid, _ := e.AddShapeXf("text", e.Work(), false)
	if d := e.Doc.Find(tid).M.Det3(); d < 0 {
		t.Fatalf("text placed mirrored (determinant %v)", d)
	}
}

// Ungrouping keeps what the group was: a hole group's parts are holes.
func TestUngroupKeepsHoleAndTransparent(t *testing.T) {
	e := New()
	a, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := e.AddShape("box", geom.V(10, 0, 0), false)
	e.Select(a, b)
	e.Group(doc.ModeUnion)
	e.SetHole(true)
	e.SetTransparent(true)
	for _, id := range e.Ungroup() {
		if n := e.Doc.Find(id); !n.Hole || !n.Transparent {
			t.Fatalf("part %d: hole %v transparent %v", id, n.Hole, n.Transparent)
		}
	}
}

// The halves of a split keep the shape's transparency.
func TestSplitKeepsTransparency(t *testing.T) {
	e := New()
	id, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	e.Select(id)
	e.SetTransparent(true)
	hs, err := e.Split(nil, 2, 10, doc.KeepBoth, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hs {
		for _, p := range e.Doc.Find(h).Parts() {
			if !p.Transparent {
				t.Fatalf("half %d isn't transparent", h)
			}
		}
	}
}

// A failed export leaves an earlier one as it was.
func TestFailedExportKeepsTheOldFile(t *testing.T) {
	e := New()
	path := filepath.Join(t.TempDir(), "cut.svg")
	if err := os.WriteFile(path, []byte("<svg>old</svg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	flat := &geom.Mesh{Tris: []geom.Tri{{P: [3]geom.Vec3{geom.V(0, 0, 0), geom.V(10, 0, 0), geom.V(0, 10, 0)}}}}
	e.AddMesh("flat", flat, geom.V(0, 0, 0))
	if err := e.Export(path, false); err == nil {
		t.Fatal("exported an outline of nothing")
	}
	if b, _ := os.ReadFile(path); string(b) != "<svg>old</svg>" {
		t.Fatalf("the old export became %q", b)
	}
}

// A save that can't be written reports it, leaves no partial file behind,
// and the design stays unsaved.
func TestFailedSave(t *testing.T) {
	e := New()
	e.AddShape("box", geom.V(0, 0, 0), false)
	dir := t.TempDir()
	target := filepath.Join(dir, "taken.scotticad")
	if err := os.Mkdir(target, 0o755); err != nil { // a folder where the file would go
		t.Fatal(err)
	}
	if err := e.Save(target); err == nil {
		t.Fatal("saved over a folder")
	}
	if _, err := os.Stat(target + ".tmp"); err == nil {
		t.Fatal("left a .tmp file")
	}
	if !e.Dirty() || e.Path != "" {
		t.Fatalf("dirty %v, path %q after a failed save", e.Dirty(), e.Path)
	}
	if err := e.Save(""); err == nil {
		t.Fatal("saved with no name")
	}
}

// A file from a newer version isn't opened: re-saving it would drop what
// this version doesn't know.
func TestNewerFileIsRefused(t *testing.T) {
	newer := fmt.Sprintf(`{"version":%d,"nodes":[{"id":1,"kind":"box"}]}`, doc.FormatVersion+1)
	if _, err := doc.Unmarshal([]byte(newer)); err == nil {
		t.Fatal("opened a file from a newer version")
	}
	if _, err := doc.Unmarshal([]byte(`{"version":1,"nodes":[{"id":1,"kind":"box"}]}`)); err != nil {
		t.Fatal(err)
	}
}

// Opening a missing or damaged file leaves the open design as it was.
func TestFailedOpenKeepsTheDesign(t *testing.T) {
	e := New()
	id, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	bad := filepath.Join(t.TempDir(), "bad.scotticad")
	os.WriteFile(bad, []byte(`{"version":1,"nodes":[{"id":1,"kind":"bo`), 0o644)
	for _, p := range []string{bad, filepath.Join(t.TempDir(), "missing.scotticad")} {
		if err := e.Open(p); err == nil {
			t.Fatalf("opened %s", p)
		}
		if e.Doc.Find(id) == nil || !e.Hist.CanUndo() || !e.Dirty() {
			t.Fatalf("%s: the design or its history changed", p)
		}
	}
}

// Imports in inches and at a scale come in at the right size; other
// files are refused without changing the design.
func TestImportUnitsAndScale(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	m := doc.New("t")
	n, _ := m.AddShape("box", geom.V(0, 0, 0), false)
	n.Size = geom.V(10, 10, 10)
	geom.WriteSTL(&buf, n.WorldMesh(false), "cube")
	stl := filepath.Join(dir, "cube.stl")
	os.WriteFile(stl, buf.Bytes(), 0o644)
	cases := []struct {
		o    ImportOptions
		want float64
	}{{ImportOptions{Scale: 1, Units: "in"}, 254}, {ImportOptions{Scale: 0.5, Units: "mm"}, 5}}
	for _, c := range cases {
		e := New()
		id, err := e.ImportOn(stl, geom.Vec3{}, c.o)
		if err != nil {
			t.Fatal(err)
		}
		if s := e.Doc.Find(id).WorldBounds().Size(); math.Abs(s.X-c.want) > 1e-3 {
			t.Fatalf("%+v: size %v, want %v", c.o, s.X, c.want)
		}
	}
	e := New()
	txt := filepath.Join(dir, "notes.txt")
	os.WriteFile(txt, []byte("hello"), 0o644)
	if _, err := e.ImportOn(txt, geom.Vec3{}, DefaultImportOptions()); err == nil || len(e.Doc.Nodes) != 0 {
		t.Fatalf("imported a .txt: %v, %d nodes", err, len(e.Doc.Nodes))
	}
}

// Pasting nodes with no matrix (from elsewhere) places them, as opening a
// file would.
func TestPasteWithoutMatrix(t *testing.T) {
	e := New()
	e.clip = []byte(`[{"kind":"box","size":{"X":10,"Y":10,"Z":10},"color":"#ff0000"}]`)
	ids, err := e.Paste(true)
	if err != nil {
		t.Fatal(err)
	}
	if b := e.Doc.Find(ids[0]).WorldBounds(); !near(b.Size().X, 10) {
		t.Fatalf("bounds %v..%v", b.Min, b.Max)
	}
}

// The workplane can't go on a shape with no geometry: its frame would be
// NaN, and every measurement in it (and the API's state) with it.
func TestWorkplaneOnEmptyGeometry(t *testing.T) {
	e := New()
	a, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := e.AddShape("box", geom.V(100, 0, 0), false)
	e.Select(a, b)
	g, err := e.Group(doc.ModeIntersect)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.AttachWorkplane(g); err == nil || e.Workplane != nil {
		t.Fatalf("attached to an empty shape: %v, %v", err, e.Workplane)
	}
	e.Select(g)
	if err := e.ToggleShapeWorkplane(); err == nil || e.Workplane != nil {
		t.Fatalf("toggled onto an empty shape: %v", err)
	}
	c, _ := e.AddShape("box", geom.V(0, 50, 0), false)
	if err := e.AttachWorkplane(c); err != nil || e.Workplane == nil || !finite(*e.Workplane) {
		t.Fatalf("a real shape: %v, %v", err, e.Workplane)
	}
}

// A file with coordinates far beyond any printer (units mixed up, or a
// damaged file) is refused, not imported into a design that can't then be
// measured or drawn.
func TestImportFarTooLarge(t *testing.T) {
	p := filepath.Join(t.TempDir(), "huge.stl")
	stl := "solid x\nfacet normal 0 0 1\nouter loop\nvertex 0 0 0\nvertex 1e12 0 0\nvertex 0 1 0\nendloop\nendfacet\nendsolid x\n"
	if err := os.WriteFile(p, []byte(stl), 0o644); err != nil {
		t.Fatal(err)
	}
	e := New()
	if _, err := e.Import(p, geom.V(0, 0, 0), DefaultImportOptions()); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("imported a 1e12 mm mesh: %v", err)
	}
	if len(e.Doc.Nodes) != 0 || e.Hist.Depth() != 0 {
		t.Fatal("the refused import left something behind")
	}
}
