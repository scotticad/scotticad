package editor

import (
	"archive/zip"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestEditorFlow(t *testing.T) {
	e := New()
	box, err := e.AddShape("box", geom.V(0, 0, 0), false)
	if err != nil {
		t.Fatal(err)
	}
	cyl, _ := e.AddShape("cylinder", geom.V(0, 0, 0), true)
	if e.Doc.Find(cyl).Color != "#f6851f" {
		t.Fatalf("hole should keep the cylinder colour, got %s", e.Doc.Find(cyl).Color)
	}
	e.Select(cyl)
	if err := e.Resize(nil, geom.V(5, 5, 30)); err != nil {
		t.Fatal(err)
	}
	b := e.SelectionBounds()
	if !near(b.Size().X, 5) || !near(b.Size().Z, 30) {
		t.Fatalf("resize %v", b.Size())
	}
	e.Select(box, cyl)
	e.Align(nil, 0, 0)
	e.Align(nil, 1, 0)
	g, err := e.Group("union")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Sel) != 1 || e.Sel[0] != g {
		t.Fatal("group not selected")
	}
	m, _ := e.ExportMesh(true)
	v := m.Volume()
	want := 8000 - math.Pi*2.5*2.5*20*0.99 // 48-sided cylinder ~ 1% smaller
	if math.Abs(v-want) > 25 {
		t.Fatalf("volume %v want ~%v", v, want)
	}

	// save, reopen, ungroup
	dir := t.TempDir()
	p := filepath.Join(dir, "a.scotticad")
	if err := e.Save(p); err != nil {
		t.Fatal(err)
	}
	e2 := New()
	if err := e2.Open(p); err != nil {
		t.Fatal(err)
	}
	e2.SelectAll()
	ids := e2.Ungroup()
	if len(ids) != 2 {
		t.Fatalf("ungroup gave %d", len(ids))
	}
	holes := 0
	for _, n := range e2.Selected() {
		if n.Hole {
			holes++
		}
	}
	if holes != 1 {
		t.Fatal("hole flag lost")
	}

	// undo restores the selection of that moment
	e.Undo()
	if len(e.Sel) != 2 {
		t.Fatalf("undo of group selected %v", e.Sel)
	}
	e.Redo()
	if len(e.Sel) != 1 || e.Sel[0] != g {
		t.Fatalf("redo of group selected %v", e.Sel)
	}

	// undo back to start
	for e.Undo() {
	}
	if len(e.Doc.Nodes) != 0 {
		t.Fatalf("undo left %d nodes", len(e.Doc.Nodes))
	}
	for e.Redo() {
	}
	if len(e.Doc.Nodes) != 1 {
		t.Fatalf("redo gave %d nodes", len(e.Doc.Nodes))
	}
}

func TestClipboardAndDuplicate(t *testing.T) {
	e := New()
	id, _ := e.AddShape("sphere", geom.V(0, 0, 0), false)
	e.Select(id)
	e.Copy()
	ids, err := e.Paste(false)
	if err != nil || len(ids) != 1 || ids[0] == id {
		t.Fatal("paste", err)
	}
	// each paste lands another half-size step along X and Y
	ids2, _ := e.Paste(false)
	for i, want := range []float64{10, 20} { // 20 mm sphere: 10 mm steps
		c := e.Doc.Find([][]int{ids, ids2}[i][0]).WorldBounds().Center()
		if !near(c.X, want) || !near(c.Y, want) {
			t.Fatalf("paste %d at %v, want (%v, %v)", i+1, c, want, want)
		}
	}
	e.Cut() // the second paste
	if len(e.Doc.Nodes) != 2 {
		t.Fatal("cut")
	}
	e.Select(id)
	d1 := e.Duplicate()
	e.Move(nil, geom.V(30, 0, 0))
	d2 := e.Duplicate()
	x := e.Doc.Find(d2[0]).WorldBounds().Center().X
	if !near(x, 60) || d1[0] == d2[0] {
		t.Fatalf("repeat duplicate x=%v", x)
	}
}

// A generator parameter keeps the shape's scale: doubling the faceted
// star's radius doubles its footprint, even after a resize.
func TestGeneratorParamKeepsScale(t *testing.T) {
	e := New()
	id, _ := e.AddShape("star3d", geom.V(0, 0, 0), false)
	n := e.Doc.Find(id)
	if math.Abs(n.Size.X-38.04) > 0.01 {
		t.Fatalf("size %v", n.Size)
	}
	e.Select(id)
	b := e.SelectionBounds()
	b.Max.Z = b.Min.Z + 20 // stretch to twice the height
	e.SetBounds(nil, b)
	if err := e.SetParam(id, "radius", 40); err != nil {
		t.Fatal(err)
	}
	n = e.Doc.Find(id)
	if math.Abs(n.Size.X-76.08) > 0.02 || math.Abs(n.Size.Z-20) > 1e-6 {
		t.Fatalf("after radius 40: %v", n.Size)
	}
	// six points: the footprint becomes the 6-point star's own (80 x 69.28)
	e.SetParam(id, "points", 6)
	n = e.Doc.Find(id)
	if math.Abs(n.Size.X-69.28) > 0.02 && math.Abs(n.Size.Y-69.28) > 0.02 {
		t.Fatalf("after 6 points: %v", n.Size)
	}
}

// An enclosure is placed as a bundle of parts; its parameters rebuild
// every part, and ungrouping separates them.
func TestEnclosureAssembly(t *testing.T) {
	e := New()
	id, err := e.AddShape("projectbox", geom.V(0, 0, 0), false)
	if err != nil {
		t.Fatal(err)
	}
	g := e.Doc.Find(id)
	if !g.IsBundle() || len(g.Children) != 2 || g.Assembly != "projectbox" {
		t.Fatalf("placed %+v", g)
	}
	lidID := g.Children[1].ID
	if err := e.SetParam(id, "length", 100); err != nil {
		t.Fatal(err)
	}
	g = e.Doc.Find(id)
	if b := e.Doc.Find(id).WorldBounds(); !near(b.Size().X, 100) || g.Children[1].ID != lidID {
		t.Fatalf("after length 100: %v, lid id %d", b.Size(), g.Children[1].ID)
	}
	if err := e.SetParam(id, "lid", 0); err != nil || len(e.Doc.Find(id).Children) != 1 {
		t.Fatalf("lid off: %v children, %v", len(e.Doc.Find(id).Children), err)
	}
	e.SetParam(id, "lid", 1)
	e.Select(id)
	parts := e.Ungroup()
	if len(parts) != 2 || len(e.Doc.Nodes) != 2 {
		t.Fatalf("ungroup gave %v", parts)
	}
	for _, n := range e.Doc.Nodes {
		if n.Params["length"] != 100 {
			t.Fatalf("part %s lost its parameters: %v", n.Kind, n.Params)
		}
	}
}

func TestUnits(t *testing.T) {
	for _, c := range []struct {
		s, u string
		mm   float64
	}{{"10", "mm", 10}, {"1 1/2", "in", 38.1}, {"-3/4", "in", -19.05}, {"25mm", "in", 25}, {"1in", "mm", 25.4}, {"2cm", "mm", 20}} {
		v, err := Parse(c.s, c.u)
		if err != nil || !near(v, c.mm) {
			t.Errorf("Parse(%q,%s)=%v,%v", c.s, c.u, v, err)
		}
	}
	if Format(20, "in", false) != "0.787" || Format(20, "in", true) != "0.7874" || Format(20, "mm", false) != "20.00" {
		t.Error("format")
	}
	// a field's text reads back as the value it shows (1/16" exactly)
	for _, mm := range []float64{25.4, 25.4 / 16, 3.175, 50.8} {
		v, err := Parse(Format(mm, "in", true), "in")
		if err != nil || !near(v, mm) {
			t.Errorf("%v mm shows as %q, reads back as %v", mm, Format(mm, "in", true), v)
		}
	}
	if Format(25.4, "in", true) != "1.00" {
		t.Errorf("1 in: %q", Format(25.4, "in", true))
	}
	// only a whole number and a fraction make a pair; signs only in front
	for _, s := range []string{"1 2", "1 -1/2", "1.5 1/2", "1/2 1/2", "--1", "+-1", "1/-2", "1_0", "0x1p4", "0x10"} {
		if v, err := Parse(s, "in"); err == nil {
			t.Errorf("Parse(%q) = %v, want an error", s, v)
		}
	}
}

func TestImportSVGAndExport(t *testing.T) {
	dir := t.TempDir()
	svg := filepath.Join(dir, "s.svg")
	os.WriteFile(svg, []byte(`<svg width="10mm" height="10mm" viewBox="0 0 10 10"><rect width="10" height="10"/></svg>`), 0o644)
	e := New()
	id, err := e.Import(svg, geom.V(0, 0, 0), DefaultImportOptions())
	if err != nil {
		t.Fatal(err)
	}
	if s := e.Doc.Find(id).WorldBounds().Size(); !near(s.X, 10) || !near(s.Z, 10) {
		t.Fatalf("svg size %v", s)
	}
	for _, ext := range []string{".stl", ".obj"} {
		p := filepath.Join(dir, "out"+ext)
		if err := e.Export(p, false); err != nil {
			t.Fatal(err)
		}
		id2, err := e.Import(p, geom.V(50, 0, 0), DefaultImportOptions())
		if err != nil {
			t.Fatal(ext, err)
		}
		if v := e.Doc.Find(id2).WorldMesh(false).Volume(); !near(math.Round(v), 1000) {
			t.Fatalf("%s volume %v", ext, v)
		}
		e.Delete([]int{id2})
	}
}

// Export to .3mf writes a 3MF package holding the design's mesh.
func TestExport3MF(t *testing.T) {
	e := New()
	if _, err := e.AddShape("box", geom.V(0, 0, 0), false); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "out.3mf")
	if err := e.Export(p, false); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"[Content_Types].xml", "_rels/.rels", "3D/3dmodel.model"} {
		if !names[want] {
			t.Errorf("the package has no %s", want)
		}
	}
}
