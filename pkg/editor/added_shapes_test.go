package editor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// A QR code is an assembly made from its text: the text reaches its
// parts, changing it rebuilds them (one undo step), and text too long for
// a QR code is refused.
func TestQRCodeText(t *testing.T) {
	e := New()
	id, err := e.AddShape("qrcode", geom.V(0, 0, 0), false)
	if err != nil {
		t.Fatal(err)
	}
	g := e.Doc.Find(id)
	if g.Text != "https://scotticad.com" || len(g.Children) != 2 {
		t.Fatalf("placed with text %q and %d parts", g.Text, len(g.Children))
	}
	for _, c := range g.Children {
		if c.Text != g.Text {
			t.Fatalf("part %s has text %q", c.Kind, c.Text)
		}
	}
	before := g.Children[1].LeafMesh().Volume()
	if err := e.SetText(id, "HELLO", ""); err != nil {
		t.Fatal(err)
	}
	g = e.Doc.Find(id)
	if g.Text != "HELLO" || g.Children[1].Text != "HELLO" || g.Children[1].LeafMesh().Volume() == before {
		t.Fatal("the code wasn't rebuilt for its new text")
	}
	e.Undo()
	if g = e.Doc.Find(id); g.Text != "https://scotticad.com" || g.Children[1].Text != g.Text {
		t.Fatal("undo didn't bring the old text back")
	}
	if err := e.SetText(id, strings.Repeat("x", 300), ""); err == nil {
		t.Fatal("300 characters accepted")
	}
	if err := e.SetTextLive(e.Doc.Find(id), strings.Repeat("x", 300), ""); err == nil {
		t.Fatal("300 characters accepted while typing")
	}
	// a setting keeps the text; no base drops the base part
	if err := e.SetParam(id, "base", 0); err != nil {
		t.Fatal(err)
	}
	if g = e.Doc.Find(id); len(g.Children) != 1 || g.Children[0].Text != g.Text {
		t.Fatalf("base 0: %d parts", len(g.Children))
	}
	// saved and opened, it is the same code
	p := filepath.Join(t.TempDir(), "qr.scotticad")
	if err := e.Save(p); err != nil {
		t.Fatal(err)
	}
	o := New()
	if err := o.Open(p); err != nil {
		t.Fatal(err)
	}
	if h := o.Doc.Nodes[0]; h.Text != g.Text || h.Children[0].LeafMesh().Volume() != g.Children[0].LeafMesh().Volume() {
		t.Fatal("reopened, the code differs")
	}
	// the Text shape still works as before; a box has no text
	tx, _ := e.AddShape("text", geom.V(50, 0, 0), false)
	if err := e.SetText(tx, "ABC", ""); err != nil || e.Doc.Find(tx).Text != "ABC" {
		t.Fatalf("text shape: %v", err)
	}
	b, _ := e.AddShape("box", geom.V(90, 0, 0), false)
	if err := e.SetText(b, "x", ""); err == nil {
		t.Fatal("set the text of a box")
	}
}

// A joint places its key as a solid and its socket as a hole, and a
// setting change keeps them so.
func TestJointSocketIsAHole(t *testing.T) {
	e := New()
	for _, kind := range []string{"dovetail", "alignpin"} {
		id, err := e.AddShape(kind, geom.V(0, 0, 0), false)
		if err != nil {
			t.Fatal(err)
		}
		check := func(when string) {
			g := e.Doc.Find(id)
			if len(g.Children) != 2 || g.Children[0].Hole || !g.Children[1].Hole {
				t.Fatalf("%s %s: parts not a solid key and a hole socket", kind, when)
			}
		}
		check("placed")
		if err := e.SetParam(id, "clearance", 0.3); err != nil {
			t.Fatal(err)
		}
		check("after a setting")
	}
}

// A choice setting (the bearing) is an index, kept to its options.
func TestChoiceSetting(t *testing.T) {
	e := New()
	id, _ := e.AddShape("bearingseat", geom.V(0, 0, 0), true)
	before := e.Doc.Find(id).Size
	if err := e.SetParam(id, "bearing", 0); err != nil { // MR105: 10 mm across
		t.Fatal(err)
	}
	n := e.Doc.Find(id)
	if n.Params["bearing"] != 0 || n.Size.X >= before.X {
		t.Fatalf("bearing %g, size %v (was %v)", n.Params["bearing"], n.Size, before)
	}
	e.SetParam(id, "bearing", 99)
	if n = e.Doc.Find(id); n.Params["bearing"] != 8 {
		t.Fatalf("an index past the list became %g, want the last (8)", n.Params["bearing"])
	}
}

// Changing an arc's angle, radius or height keeps its circle's centre
// where it is (and so its radius): only the swept part changes.
func TestArcKeepsItsCentre(t *testing.T) {
	e := New()
	id, _ := e.AddShape("arc", geom.V(0, 0, 0), false)
	// placed by the middle of its box: a 90° arc of radius 20 from +X fills
	// x 0..20, y 0..20 from its centre, so the centre is at (-10, -10)
	b := e.Doc.Find(id).Bounds()
	if b.Min.Sub(geom.V(-10, -10, 0)).Len() > 1e-9 || b.Max.Sub(geom.V(10, 10, 5)).Len() > 1e-9 {
		t.Fatalf("placed at %v", b)
	}
	for _, c := range []struct {
		key      string
		val      float64
		min, max geom.Vec3
	}{
		{"angle", 180, geom.V(-30, -10, 0), geom.V(10, 10, 5)},
		{"angle", 360, geom.V(-30, -30, 0), geom.V(10, 10, 5)},
		{"radius", 10, geom.V(-20, -20, 0), geom.V(0, 0, 5)},
		{"height", 8, geom.V(-20, -20, 0), geom.V(0, 0, 8)},
		{"angle", 90, geom.V(-10, -10, 0), geom.V(0, 0, 8)},
	} {
		if err := e.SetParam(id, c.key, c.val); err != nil {
			t.Fatal(err)
		}
		b := e.Doc.Find(id).Bounds()
		if b.Min.Sub(c.min).Len() > 1e-9 || b.Max.Sub(c.max).Len() > 1e-9 {
			t.Fatalf("after %s = %g: %v, want %v to %v", c.key, c.val, b, c.min, c.max)
		}
	}
}
