package editor

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// The ruler and the construction plane are saved with the design and come
// back when it is opened, without being undo steps or unsaved changes.
func TestRulerAndConstructionPlaneAreSaved(t *testing.T) {
	e := New()
	id, err := e.AddShape("box", geom.V(30, 0, 0), false)
	if err != nil {
		t.Fatal(err)
	}
	e.Rotate([]int{id}, 0, 30) // so the plane on it is tilted
	if err := e.AttachWorkplane(id); err != nil {
		t.Fatal(err)
	}
	wp := *e.Workplane
	e.PlaceRuler(geom.V(-40, -30, 0))
	e.TurnRuler()
	e.Ruler.Mid = true
	p := filepath.Join(t.TempDir(), "aids.scotticad")
	if err := e.Save(p); err != nil {
		t.Fatal(err)
	}
	if e.Dirty() {
		t.Fatal("dirty just after saving")
	}
	// moving the ruler or the plane is no change to the design
	e.PlaceRuler(geom.V(5, 5, 0))
	e.AttachWorkplane(0)
	if e.Dirty() {
		t.Fatal("moving the ruler or the plane made the design dirty")
	}

	o := New()
	if err := o.Open(p); err != nil {
		t.Fatal(err)
	}
	r := o.Ruler
	if r == nil || r.Origin != geom.V(-40, -30, 0) || !r.Mid || r.Turns != 1 {
		t.Fatalf("ruler after opening: %+v", r)
	}
	if o.Workplane == nil || *o.Workplane != wp {
		t.Fatalf("construction plane after opening: %v, want %v", o.Workplane, wp)
	}
	if o.Dirty() || o.Hist.CanUndo() {
		t.Fatal("a freshly opened design is dirty or has undo steps")
	}
	// they live in the editor, not in the document's snapshots
	if o.Doc.Ruler != nil || o.Doc.Workplane != nil {
		t.Fatal("the document still holds the ruler or the plane")
	}
	// removing them and saving again leaves them out of the file
	o.RemoveRuler()
	o.AttachWorkplane(0)
	if err := o.Save(p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if s := string(b); strings.Contains(s, `"ruler"`) || strings.Contains(s, `"constructionPlane"`) {
		t.Fatal("removed ruler or plane still in the file")
	}
	// a new design has neither
	o.Open(p)
	o.PlaceRuler(geom.V(1, 2, 0))
	o.NewDesign()
	if o.Ruler != nil || o.Workplane != nil {
		t.Fatal("a new design kept the ruler or the plane")
	}
}

// A ruler turned more than a full turn in a hand-edited file comes back
// as its quarter turns.
func TestSavedRulerTurnsWrap(t *testing.T) {
	p := filepath.Join(t.TempDir(), "turns.scotticad")
	f := `{"version":1,"settings":{"units":"mm","workplaneW":200,"workplaneL":200,"snap":1,"showGrid":true,"showShadows":true},
"nodes":[],"nextId":1,"ruler":{"origin":{"X":1,"Y":2,"Z":0},"turns":-3}}`
	os.WriteFile(p, []byte(f), 0o644)
	e := New()
	if err := e.Open(p); err != nil {
		t.Fatal(err)
	}
	if e.Ruler == nil || e.Ruler.Turns != 1 {
		t.Fatalf("ruler %+v, want 1 turn", e.Ruler)
	}
	if finiteMat(geom.Mat4{math.NaN()}) || finiteVec(geom.V(math.Inf(1), 0, 0)) {
		t.Fatal("finite checks")
	}
}

// A saved construction plane that isn't a usable frame (flat, so it can't
// be inverted, or projective) is dropped on opening, not restored to break
// every edit made in it.
func TestSavedConstructionPlaneMustBeAFrame(t *testing.T) {
	head := `{"version":1,"settings":{"units":"mm","workplaneW":200,"workplaneL":200,"snap":1,"showGrid":true,"showShadows":true},"nodes":[],"nextId":1,"constructionPlane":`
	for name, plane := range map[string]string{
		"good":       `[0,1,0,0, -1,0,0,0, 0,0,1,0, 5,6,7,1]`,
		"flat":       `[1,0,0,0, 0,1,0,0, 0,0,0,0, 0,0,0,1]`,
		"zero":       `[0,0,0,0, 0,0,0,0, 0,0,0,0, 0,0,0,0]`,
		"projective": `[1,0,0,0.5, 0,1,0,0, 0,0,1,0, 0,0,0,1]`,
		"huge":       `[1e300,0,0,0, 0,1e300,0,0, 0,0,1e300,0, 0,0,0,1]`,
	} {
		p := filepath.Join(t.TempDir(), name+".scotticad")
		os.WriteFile(p, []byte(head+plane+"}"), 0o644)
		e := New()
		if err := e.Open(p); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got, want := e.Workplane != nil, name == "good"; got != want {
			t.Errorf("%s: plane restored %v, want %v", name, got, want)
		}
	}
}

func TestRename(t *testing.T) {
	e := New()
	a, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := e.AddShape("cylinder", geom.V(40, 0, 0), false)
	e.Select(b) // the selection doesn't matter: Rename goes by id
	if err := e.Rename(a, "  base  "); err != nil {
		t.Fatal(err)
	}
	if n := e.Doc.Find(a); n.Name != "base" || n.Label() != "base" {
		t.Fatalf("name %q", n.Name)
	}
	if e.Doc.Find(b).Name != "" {
		t.Fatal("renamed the selected shape instead")
	}
	e.Undo()
	if e.Doc.Find(a).Name != "" {
		t.Fatal("undo didn't take the name back")
	}
	e.Redo()
	e.Rename(a, "")
	if l := e.Doc.Find(a).Label(); l != "Box" {
		t.Fatalf("an empty name should give back the kind's name, got %q", l)
	}
	if err := e.Rename(999, "x"); err == nil {
		t.Fatal("renamed a missing shape")
	}
}
