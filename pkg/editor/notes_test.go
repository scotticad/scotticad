package editor

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
)

func near3(a, b geom.Vec3) bool { return a.Sub(b).Len() < 1e-6 }

// Notes follow their shape through moves, resizes, grouping, ungrouping,
// undo and saving.
func TestNotes(t *testing.T) {
	e := New()
	id, err := e.AddShape("box", geom.V(0, 0, 0), false) // 20 x 20 x 20, top at z 20
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddNote(id, geom.V(5, 0, 20), "top"); err != nil {
		t.Fatal(err)
	}
	world := func() geom.Vec3 { return e.Notes()[0].World }
	if !near3(world(), geom.V(5, 0, 20)) {
		t.Fatalf("note at %v", world())
	}
	e.Move(nil, geom.V(10, 0, 0))
	if !near3(world(), geom.V(15, 0, 20)) {
		t.Fatalf("after move %v", world())
	}
	// resizing keeps it on the top face
	b := e.SelectionBounds()
	b.Max.Z = 40
	if err := e.SetBounds(nil, b); err != nil {
		t.Fatal(err)
	}
	if w := world(); math.Abs(w.Z-40) > 1e-6 {
		t.Fatalf("after resize %v", w)
	}
	// grouping keeps it; ungrouping a group's own note keeps it in place
	id2, _ := e.AddShape("box", geom.V(60, 0, 0), false)
	e.Select(id, id2)
	g, err := e.Group(doc.ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Notes()) != 1 {
		t.Fatalf("notes after grouping: %v", e.Notes())
	}
	if _, err := e.AddNote(g, geom.V(60, 0, 20), "group"); err != nil {
		t.Fatal(err)
	}
	e.Select(g)
	e.Ungroup()
	ns := e.Notes()
	if len(ns) != 2 || !near3(ns[0].World, geom.V(15, 0, 40)) && !near3(ns[1].World, geom.V(15, 0, 40)) {
		t.Fatalf("notes after ungroup: %+v", ns)
	}
	found := false
	for _, n := range ns {
		found = found || (n.Text == "group" && near3(n.World, geom.V(60, 0, 20)))
	}
	if !found {
		t.Fatalf("group note lost or moved: %+v", ns)
	}
	// text edits and deletes are undoable
	r := ns[0]
	if err := e.SetNoteText(r.Node, r.Index, "changed"); err != nil {
		t.Fatal(err)
	}
	if err := e.DeleteNote(r.Node, r.Index); err != nil {
		t.Fatal(err)
	}
	if len(e.Notes()) != 1 {
		t.Fatalf("after delete: %+v", e.Notes())
	}
	e.Undo()
	if len(e.Notes()) != 2 {
		t.Fatalf("after undo: %+v", e.Notes())
	}
	// notes can also sit on the workplane
	if i, err := e.AddNote(0, geom.V(-50, -50, 0), "plane"); err != nil || i != 0 {
		t.Fatal(i, err)
	}
	if ns := e.Notes(); len(ns) != 3 || ns[0].Node != 0 || !near3(ns[0].World, geom.V(-50, -50, 0)) {
		t.Fatalf("workplane note: %+v", ns)
	}
	// they survive saving
	data, err := e.Doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	d2, err := doc.Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	e2 := New()
	e2.Doc = d2
	if len(e2.Notes()) != 3 {
		t.Fatalf("after reload: %+v", e2.Notes())
	}
}
