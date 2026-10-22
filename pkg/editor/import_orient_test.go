package editor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
)

// cubeMesh is a closed, outward-wound cube of side s standing on z = 0,
// centred on the origin in x and y.
func cubeMesh(s float64) *geom.Mesh {
	h := s / 2
	p := func(x, y, z float64) geom.Vec3 { return geom.V(x*h, y*h, z*h+h) }
	m := &geom.Mesh{}
	quad := func(a, b, c, d geom.Vec3) {
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{a, b, c}}, geom.Tri{P: [3]geom.Vec3{a, c, d}})
	}
	quad(p(-1, -1, -1), p(-1, 1, -1), p(1, 1, -1), p(1, -1, -1)) // bottom, facing down
	quad(p(-1, -1, 1), p(1, -1, 1), p(1, 1, 1), p(-1, 1, 1))     // top, facing up
	quad(p(-1, -1, -1), p(1, -1, -1), p(1, -1, 1), p(-1, -1, 1)) // front, -y
	quad(p(-1, 1, -1), p(-1, 1, 1), p(1, 1, 1), p(1, 1, -1))     // back, +y
	quad(p(-1, -1, -1), p(-1, -1, 1), p(-1, 1, 1), p(-1, 1, -1)) // left, -x
	quad(p(1, -1, -1), p(1, 1, -1), p(1, 1, 1), p(1, -1, 1))     // right, +x
	return m
}

// An inside-out STL (every face wound the wrong way, as some programs
// write them) imports right side out: a positive volume, adding in a
// union rather than cutting.
func TestInsideOutImportIsTurnedRightSideOut(t *testing.T) {
	cube := cubeMesh(10)
	if open, nm := cube.EdgeCheck(0); !near(cube.Volume(), 1000) || open != 0 || nm != 0 {
		t.Fatalf("the test's cube has volume %v, %d open and %d non-manifold edges", cube.Volume(), open, nm)
	}
	var buf bytes.Buffer
	if err := geom.WriteSTL(&buf, cube.Flipped(), "inside out"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "inside-out.stl")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	e := New()
	id, err := e.Import(p, geom.V(0, 0, 0), DefaultImportOptions())
	if err != nil {
		t.Fatal(err)
	}
	if v := e.Doc.Find(id).WorldMesh(false).Volume(); !near(v, 1000) {
		t.Fatalf("imported with volume %v", v)
	}
	// a 20 mm box standing on z = 5 over the cube (z 0..10): overlap 10 x 10 x 5
	b, _ := e.AddShape("box", geom.V(0, 0, 5), false)
	e.Select(id, b)
	g, err := e.Group(doc.ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	if v := e.Doc.Find(g).WorldMesh(false).Volume(); !near(v, 1000+8000-500) {
		t.Fatalf("union with the import %v, want %v", v, 1000+8000-500)
	}
}

// A file wound right but with a face missing (a common defect) stays as
// it is, wherever it sits: measured about the origin, a missing face far
// from it can make the volume negative and the file look inside out.
func TestOpenImportFarFromOriginIsNotFlipped(t *testing.T) {
	for _, off := range []float64{-5000, -1000, 0, 1000, 5000} {
		m := cubeMesh(20).Transform(geom.Translate(geom.V(off, off, off)))
		m.Tris = m.Tris[1:] // a triangle of the bottom missing
		var buf bytes.Buffer
		if err := geom.WriteSTL(&buf, m, "open"); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(t.TempDir(), "open.stl")
		if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := LoadMesh(p, DefaultImportOptions())
		if err != nil {
			t.Fatal(err)
		}
		if got.Tris[0].Normal().Dot(m.Tris[0].Normal()) < 0 {
			t.Errorf("at %v: a file wound right was turned inside out", off)
		}
	}
	// and inside out far from the origin is still turned right
	m := cubeMesh(20).Transform(geom.Translate(geom.V(-5000, -5000, -5000))).Flipped()
	var buf bytes.Buffer
	geom.WriteSTL(&buf, m, "inside out")
	p := filepath.Join(t.TempDir(), "io.stl")
	os.WriteFile(p, buf.Bytes(), 0o644)
	if got, err := LoadMesh(p, DefaultImportOptions()); err != nil || got.Volume() <= 0 {
		t.Fatalf("inside out far away: %v, volume %v", err, got.Volume())
	}
}
