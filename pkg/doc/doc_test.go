package doc

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Export treats what it exports as grouped. A 20 mm box with a loose
// 13 mm cylinder hole through it: the loose hole cuts the box when the
// whole scene, or both, are exported; not when the box is exported alone or the
// hole is hidden; and a hole inside a bundle cuts nothing, as in a bundle.
// The cut faces take the box's colour.
func TestExportAppliesLooseHoles(t *testing.T) {
	scene := func() (*Document, *Node, *Node) {
		d := New()
		b, _ := d.AddShape("box", geom.V(0, 0, 0), false)
		b.Size, b.M, b.Color = geom.V(20, 20, 20), geom.Translate(geom.V(0, 0, 10)), "#ff0000"
		c, _ := d.AddShape("cylinder", geom.V(0, 0, 0), true)
		c.Size, c.M = geom.V(13, 13, 20), geom.Translate(geom.V(0, 0, 10))
		return d, b, c
	}
	cut := func(m *geom.Mesh) bool { v := m.Volume(); return v > 5300 && v < 5400 }
	whole := func(m *geom.Mesh) bool { return math.Abs(m.Volume()-8000) < 1e-6 }

	d, b, c := scene()
	if m := d.ExportMesh(nil); !cut(m) {
		t.Errorf("whole scene: volume %.1f, want the hole cut", m.Volume())
	} else {
		if open, nm := m.EdgeCheck(0); open+nm != 0 {
			t.Errorf("whole scene: %d open, %d non-manifold edges", open, nm)
		}
		red := geom.ParseColor("#ff0000")
		for _, tr := range m.Tris {
			if tr.C != red {
				t.Fatalf("a cut face is %v, not the box's colour", tr.C)
			}
		}
	}
	if m := d.ExportMesh([]int{b.ID, c.ID}); !cut(m) {
		t.Errorf("both selected: volume %.1f, want the hole cut", m.Volume())
	}
	if m := d.ExportMesh([]int{b.ID}); !whole(m) {
		t.Errorf("the box alone: volume %.1f, want it uncut", m.Volume())
	}
	c.Hidden = true
	if m := d.ExportMesh(nil); !whole(m) {
		t.Errorf("the hole hidden: volume %.1f, want it uncut", m.Volume())
	}

	d, b, c = scene()
	if _, err := d.Group([]int{b.ID, c.ID}, ModeBundle); err != nil {
		t.Fatal(err)
	}
	if m := d.ExportMesh(nil); !whole(m) {
		t.Errorf("in a bundle: volume %.1f, want it uncut", m.Volume())
	}
	d, b, c = scene()
	if _, err := d.Group([]int{b.ID, c.ID}, ModeUnion); err != nil {
		t.Fatal(err)
	}
	if m := d.ExportMesh(nil); !cut(m) {
		t.Errorf("in a union: volume %.1f, want the hole cut", m.Volume())
	}
}

// Export fuses what it exports: a pink body with an
// inlay pocket, the tan inlay in it, and a pink box overlapping the body.
// For STL it is one closed solid with the overlap counted once (two
// shells in the same space can print twice); with colours, each colour is
// one closed solid of its own, the inlay apart from the body (a part per
// filament for a slicer).
func TestExportFusesSolids(t *testing.T) {
	d := New()
	add := func(size, centre geom.Vec3, hole bool, colour string) *Node {
		n, _ := d.AddShape("box", geom.V(0, 0, 0), hole)
		n.Size, n.M, n.Color = size, geom.Translate(centre), colour
		return n
	}
	body := add(geom.V(40, 30, 10), geom.V(0, 0, 5), false, "#f969a4")
	pocket := add(geom.V(20, 10, 1), geom.V(0, 0, 9.5), true, "#f969a4")
	g, err := d.Group([]int{body.ID, pocket.ID}, ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	g.Color = "#f969a4"
	add(geom.V(20, 10, 1), geom.V(0, 0, 9.5), false, "#b5b899") // the inlay, filling the pocket
	add(geom.V(10, 10, 10), geom.V(20, 0, 5), false, "#f969a4") // half inside the body

	const bodyVol, inlay, boxOut = 40*30*10 - 20*10*1, 20 * 10 * 1, 5 * 10 * 10
	m := d.ExportMesh(nil)
	if v := m.Volume(); !near(v, bodyVol+inlay+boxOut) {
		t.Errorf("STL volume %.1f, want %d (the overlap once)", v, bodyVol+inlay+boxOut)
	}
	if s := len(m.Shells()); s != 1 {
		t.Errorf("STL: %d shells, want one solid", s)
	}
	if open, nm := m.EdgeCheck(0); open+nm != 0 {
		t.Errorf("STL: %d open, %d non-manifold edges", open, nm)
	}

	c := d.ExportColours(nil)
	per := map[geom.Color][]geom.Tri{}
	for _, tr := range c.Tris {
		per[tr.C] = append(per[tr.C], tr)
	}
	for hex, want := range map[string]float64{"#f969a4": bodyVol + boxOut, "#b5b899": inlay} {
		part := &geom.Mesh{Tris: per[geom.ParseColor(hex)]}
		if v := part.Volume(); !near(v, want) {
			t.Errorf("%s: volume %.1f, want %.0f", hex, v, want)
		}
		if open, nm := part.EdgeCheck(0); open+nm != 0 || len(part.Shells()) != 1 {
			t.Errorf("%s: %d shells, %d open, %d non-manifold edges", hex, len(part.Shells()), open, nm)
		}
	}
}

// A multicolour group exports, with colours, as one closed solid of its
// own, both its colours kept; a solid of one of its colours overlapping it
// is fused only with that colour's other solids, not with the group, so it
// comes out whole.
func TestExportColoursKeepsAMulticolourGroupWhole(t *testing.T) {
	d := New()
	add := func(centre geom.Vec3, colour string) *Node {
		n, _ := d.AddShape("box", geom.V(0, 0, 0), false)
		n.Size, n.M, n.Color = geom.V(20, 20, 20), geom.Translate(centre), colour
		return n
	}
	red, blue := add(geom.V(0, 0, 10), "#ff0000"), add(geom.V(15, 0, 10), "#0000ff")
	g, err := d.Group([]int{red.ID, blue.ID}, ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	g.Multicolor = true
	add(geom.V(30, 0, 10), "#ff0000") // 5 mm into the blue box

	shells := d.ExportColours(nil).Shells()
	if len(shells) != 2 {
		t.Fatalf("%d shells, want the group and the red box", len(shells))
	}
	for _, s := range shells {
		colours := map[geom.Color]bool{}
		for _, tr := range s.Tris {
			colours[tr.C] = true
		}
		want := 20.0 * 20 * 20
		if len(colours) == 2 {
			want = 20 * 20 * 35 // the group: the two boxes overlapping by 5 mm
		}
		if v := s.Volume(); !near(v, want) {
			t.Errorf("a shell of %d colours: volume %.1f, want %.0f", len(colours), v, want)
		}
		if open, nm := s.EdgeCheck(0); open+nm != 0 {
			t.Errorf("a shell of %d colours: %d open, %d non-manifold edges", len(colours), open, nm)
		}
	}
}

// A design has no title: new files don't carry one, and an older file's
// "name" is read without complaint and not written back.
func TestTitleIsNotKept(t *testing.T) {
	has := func(b []byte) bool {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		_, ok := m["name"]
		return ok
	}
	b, err := New().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if has(b) {
		t.Errorf("a new design has a name: %s", b)
	}
	old := []byte(`{"version":1,"name":"Bracket v2","settings":{"units":"mm","workplaneW":200,"workplaneL":200,"snap":1,"showGrid":true},"nodes":[],"nextId":1}`)
	d, err := Unmarshal(old)
	if err != nil {
		t.Fatalf("an older file with a name: %v", err)
	}
	if b, _ = d.Marshal(); has(b) {
		t.Errorf("the old name was written back: %s", b)
	}
}

func TestGroupSaveReloadUngroup(t *testing.T) {
	d := New()
	box, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	hole, _ := d.AddShape("box", geom.V(0, 0, 0), true)
	hole.Size = geom.V(10, 10, 30)
	hole.M = geom.Translate(geom.V(0, 0, 10))
	g, err := d.Group([]int{box.ID, hole.ID}, ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	if v := g.WorldMesh(false).Volume(); math.Abs(v-6000) > 1e-3 {
		t.Fatalf("group volume %v", v)
	}
	// Nest: group the union with another box as a bundle, then save/reload.
	other, _ := d.AddShape("cylinder", geom.V(40, 0, 0), false)
	b, _ := d.Group([]int{g.ID, other.ID}, ModeBundle)
	data, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	d2, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(d2.Nodes) != 1 || len(d2.Nodes[0].Children) != 2 {
		t.Fatalf("tree not preserved: %d nodes", len(d2.Nodes))
	}
	if d2.Nodes[0].GroupMode != ModeBundle || d2.Nodes[0].Children[0].GroupMode != ModeUnion {
		t.Fatal("group modes not preserved")
	}
	// Ungroup all the way back to primitives after reload.
	rel := d2.Ungroup([]int{b.ID})
	if len(rel) != 2 {
		t.Fatalf("released %d", len(rel))
	}
	rel = d2.Ungroup([]int{g.ID})
	if len(rel) != 2 {
		t.Fatalf("released %d", len(rel))
	}
	var holeBack *Node
	for _, n := range d2.Nodes {
		if n.Hole {
			holeBack = n
		}
	}
	if holeBack == nil {
		t.Fatal("hole flag lost")
	}
	bb := holeBack.Bounds()
	if math.Abs(bb.Min.Z-(-5)) > 1e-9 || math.Abs(bb.Size().Z-30) > 1e-9 {
		t.Fatalf("hole bounds after round trip %v", bb)
	}
}

func TestIntersectGroup(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("box", geom.V(10, 0, 0), false) // overlaps a by 10 mm in X
	g, err := d.Group([]int{a.ID, b.ID}, ModeIntersect)
	if err != nil {
		t.Fatal(err)
	}
	if v := g.WorldMesh(false).Volume(); math.Abs(v-4000) > 1e-6 {
		t.Fatalf("intersect volume %v, want 4000", v)
	}
	if g.Label() != "Intersect" {
		t.Fatal(g.Label())
	}
	d.Ungroup([]int{g.ID})
	if len(d.Nodes) != 2 {
		t.Fatal("ungroup")
	}
}

func TestScaleFoldsIntoSize(t *testing.T) {
	d := New()
	n, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	ScaleAbout([]*Node{n}, geom.V(2, 1, 0.5), geom.V(0, 0, 0))
	if n.Size.Sub(geom.V(40, 20, 10)).Len() > 1e-9 {
		t.Fatalf("size %v", n.Size)
	}
	if s := n.M.AxisScales(); s.Sub(geom.V(1, 1, 1)).Len() > 1e-9 {
		t.Fatalf("matrix still scaled %v", s)
	}
}

func TestHistory(t *testing.T) {
	var h History
	h.Push([]byte("a"))
	h.Push([]byte("b"))
	s, _ := h.Undo([]byte("c"))
	if string(s) != "b" {
		t.Fatal(string(s))
	}
	s, _ = h.Redo([]byte("b"))
	if string(s) != "c" {
		t.Fatal(string(s))
	}
}
