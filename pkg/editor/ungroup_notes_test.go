package editor

import (
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
)

// A stretch group's own notes survive Ungroup, staying where they were. (A
// stretch releases copies of its children, so the notes must go to a copy,
// not to a child that is no longer in the design.)
func TestUngroupStretchKeepsItsNotes(t *testing.T) {
	for _, amount := range []float64{0.0001, 10} {
		e := New()
		id, _ := e.AddShape("box", geom.V(0, 0, 0), false)
		e.Select(id)
		g, err := e.Stretch(nil, 0, 0, amount)
		if err != nil {
			t.Fatal(err)
		}
		at := geom.V(-10, 0, 20)
		if _, err := e.AddNote(g, at, "keep me"); err != nil {
			t.Fatal(err)
		}
		var world geom.Vec3
		gn := e.Doc.Find(g)
		world = gn.M.Point(gn.NotePoint(gn.Notes[0].P))
		e.Select(g)
		e.Ungroup()
		found := 0
		for _, n := range e.Doc.Nodes {
			n.Walk(func(x *doc.Node) {
				for _, nt := range x.Notes {
					found++
					if p := x.M.Point(x.NotePoint(nt.P)); p.Sub(world).Len() > 1e-6 {
						t.Errorf("amount %v: note moved from %v to %v", amount, world, p)
					}
				}
			})
		}
		if found != 1 {
			t.Errorf("amount %v: %d notes after ungroup, want 1", amount, found)
		}
	}
}

// Ungrouping several groups at once still releases every child, and each
// group's note goes to one of its own shapes.
func TestUngroupSeveralGroupsKeepsEachNote(t *testing.T) {
	e := New()
	a, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := e.AddShape("box", geom.V(50, 0, 0), false)
	c, _ := e.AddShape("box", geom.V(100, 0, 0), false)
	e.Select(a, b)
	g1, err := e.Group(doc.ModeBundle)
	if err != nil {
		t.Fatal(err)
	}
	e.Select(c)
	g2, err := e.Stretch(nil, 0, 100, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddNote(g1, geom.V(0, 0, 20), "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.AddNote(g2, geom.V(100, 0, 20), "two"); err != nil {
		t.Fatal(err)
	}
	e.Select(g1, g2)
	if ids := e.Ungroup(); len(ids) != 3 {
		t.Fatalf("released %v", ids)
	}
	for _, n := range e.Doc.Nodes {
		for _, nt := range n.Notes {
			x := n.M.Translation().X
			if (nt.Text == "one") != (x < 75) {
				t.Errorf("note %q went to the shape at x=%v", nt.Text, x)
			}
		}
	}
}
