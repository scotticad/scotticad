package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// Undo history doesn't copy an imported mesh into every step: many edits
// of a design with a big import stay small, and undo still restores it.
func TestHistoryKeepsMeshesOnce(t *testing.T) {
	stl := filepath.Join(t.TempDir(), "big.stl")
	f, _ := os.Create(stl)
	m := shapes.Build("sphere", geom.V(30, 30, 30), shapes.Values{Num: map[string]float64{"steps": 64}})
	geom.WriteSTL(f, m, "big")
	f.Close()

	e := New()
	id, err := e.Import(stl, geom.V(0, 0, 0), DefaultImportOptions())
	if err != nil {
		t.Fatal(err)
	}
	data := e.Doc.Find(id).MeshData
	if len(data) < 100000 {
		t.Fatalf("test mesh too small: %d bytes", len(data))
	}
	box, _ := e.AddShape("box", geom.V(50, 0, 0), false)
	for i := 0; i < 50; i++ {
		e.Select(box)
		e.Move(nil, geom.V(1, 0, 0))
	}
	total := 0
	for _, s := range e.Hist.States() {
		total += len(s)
	}
	if total > 50*len(data)/10 {
		t.Fatalf("history holds %d bytes for 50 small edits of a design with %d bytes of mesh", total, len(data))
	}
	for e.Undo() {
	}
	e.Redo() // the import
	n := e.Doc.Find(id)
	if n == nil || n.MeshData != data || n.Kind != doc.KindMesh {
		t.Fatal("undo/redo lost the imported mesh")
	}
}
