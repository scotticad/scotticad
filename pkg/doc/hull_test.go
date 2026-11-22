package doc

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// sound is what every group's mesh must be: closed as built and as an
// export writes it.
func sound(t *testing.T, name string, m *geom.Mesh) {
	t.Helper()
	if o, n := m.EdgeCheck(0); o != 0 || n != 0 {
		t.Errorf("%s: %d open, %d non-manifold edges", name, o, n)
	}
	if r := geom.Surface(geom.AsWritten(m)); r.Open != 0 || r.NonManifold != 0 || r.InsideOut != 0 {
		t.Errorf("%s: as written: %d open, %d non-manifold edges, %d shells inside out", name, r.Open, r.NonManifold, r.InsideOut)
	}
}

// A hull group wraps its solids: two 20 mm boxes 40 mm apart fill the gap
// between them; three at the corners of an L fill the L. It ungroups to
// its shapes as they were, and switching the type switches the result.
func TestHullGroup(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("box", geom.V(60, 0, 0), false)
	before := Bounds([]*Node{a, b})
	g, err := d.Group([]int{a.ID, b.ID}, ModeHull)
	if err != nil {
		t.Fatal(err)
	}
	m := g.WorldMesh(false)
	sound(t, "two boxes", m)
	if v := m.Volume(); math.Abs(v-32000) > 1e-6 {
		t.Errorf("two boxes: volume %v, want 32000", v)
	}
	if got := g.WorldBounds(); got.Min.Sub(before.Min).Len() > 1e-9 || got.Max.Sub(before.Max).Len() > 1e-9 {
		t.Errorf("bounds %v, the shapes' %v", got, before)
	}
	if g.Label() != "Hull" || !g.IsUnion() || g.IsBundle() || g.IsIntersect() {
		t.Errorf("label %q, union %v", g.Label(), g.IsUnion())
	}
	g.GroupMode = ModeUnion
	if v := g.WorldMesh(false).Volume(); math.Abs(v-16000) > 1e-6 {
		t.Errorf("as a union: volume %v, want 16000", v)
	}
	g.GroupMode = ModeHull
	if v := g.WorldMesh(false).Volume(); math.Abs(v-32000) > 1e-6 {
		t.Errorf("a hull again: volume %v, want 32000", v)
	}
	// saved and read back, it is still a hull
	data, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	d2, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if g2 := d2.Nodes[0]; !g2.IsHull() || math.Abs(g2.WorldMesh(false).Volume()-32000) > 1e-6 {
		t.Errorf("read back: mode %q, volume %v", g2.GroupMode, g2.WorldMesh(false).Volume())
	}
	if rel := d.Ungroup([]int{g.ID}); len(rel) != 2 || len(d.Nodes) != 2 {
		t.Fatalf("ungroup released %d", len(rel))
	}
	if got := Bounds(d.Nodes); got.Min.Sub(before.Min).Len() > 1e-9 || got.Max.Sub(before.Max).Len() > 1e-9 {
		t.Errorf("ungrouped: bounds %v, were %v", got, before)
	}
	// an L of three boxes: the hull fills the L's inside corner
	c, _ := d.AddShape("box", geom.V(0, 60, 0), false)
	g, _ = d.Group([]int{a.ID, b.ID, c.ID}, ModeHull)
	m = g.WorldMesh(false)
	sound(t, "an L", m)
	if v, want := m.Volume(), (80*80-60*60/2.0)*20; math.Abs(v-want) > 1e-6 {
		t.Errorf("an L: volume %v, want %v", v, want)
	}
}

// Holes in a hull group are cut from the hull: a hole between two boxes
// cuts the skin stretched across the gap. A hull of holes alone is a hole.
func TestHullGroupHoles(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("box", geom.V(60, 0, 0), false)
	h, _ := d.AddShape("cylinder", geom.V(30, 0, -10), true) // 20 across, in the gap
	h.Size = geom.V(20, 20, 40)
	h.M = geom.Translate(geom.V(30, 0, 10))
	g, _ := d.Group([]int{a.ID, b.ID, h.ID}, ModeHull)
	m := g.WorldMesh(false)
	sound(t, "with a hole", m)
	if v := m.Volume(); v > 32000-6200 || v < 32000-math.Pi*100*20 {
		t.Errorf("with a hole: volume %v, want 32000 less a 20 mm cylinder (about 25730)", v)
	}
	if g.Hole {
		t.Error("a hull with a solid in it became a hole")
	}
	d = New()
	a, _ = d.AddShape("box", geom.V(0, 0, 0), true)
	b, _ = d.AddShape("box", geom.V(60, 0, 0), true)
	g, _ = d.Group([]int{a.ID, b.ID}, ModeHull)
	if !g.Hole {
		t.Error("a hull of holes isn't a hole")
	}
	if v := g.WorldMesh(true).Volume(); math.Abs(v-32000) > 1e-6 {
		t.Errorf("a hull of holes: volume %v, want 32000", v)
	}
}

// Round shapes, groups inside the hull and a hull inside a group.
func TestHullGroupNesting(t *testing.T) {
	d := New()
	s1, _ := d.AddShape("sphere", geom.V(0, 0, 0), false)
	s2, _ := d.AddShape("sphere", geom.V(50, 0, 0), false)
	ball := s1.WorldMesh(false).Volume()
	g, _ := d.Group([]int{s1.ID, s2.ID}, ModeHull)
	m := g.WorldMesh(false)
	sound(t, "two spheres", m)
	// a capsule: the ball and a 20 mm cylinder 50 long, a little less for
	// the facets
	if v, want := m.Volume(), ball+math.Pi*100*50; v > want || v < 0.985*want {
		t.Errorf("capsule: volume %v, want a little under %v", v, want)
	}
	// the hull in a union with a box across its middle, then that in a hull
	box, _ := d.AddShape("box", geom.V(25, 0, 0), false)
	box.Size = geom.V(10, 60, 10)
	box.M = geom.Translate(geom.V(25, 0, 5))
	u, _ := d.Group([]int{g.ID, box.ID}, ModeUnion)
	um := u.WorldMesh(false)
	sound(t, "hull in a union", um)
	if um.Volume() <= m.Volume() {
		t.Errorf("hull in a union: volume %v, no more than the hull's %v", um.Volume(), m.Volume())
	}
	far, _ := d.AddShape("box", geom.V(0, 100, 0), false)
	top, _ := d.Group([]int{u.ID, far.ID}, ModeHull)
	tm := top.WorldMesh(false)
	sound(t, "union in a hull", tm)
	if tm.Volume() <= um.Volume()+8000 {
		t.Errorf("union in a hull: volume %v, no more than its parts' %v", tm.Volume(), um.Volume()+8000)
	}
	// a bundle's parts are wrapped like any others
	d = New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("box", geom.V(60, 0, 0), false)
	c, _ := d.AddShape("box", geom.V(120, 0, 0), false)
	bun, _ := d.Group([]int{a.ID, b.ID}, ModeBundle)
	g, _ = d.Group([]int{bun.ID, c.ID}, ModeHull)
	if v := g.WorldMesh(false).Volume(); math.Abs(v-140*20*20) > 1e-6 {
		t.Errorf("a bundle in a hull: volume %v, want %v", v, 140*20*20)
	}
}

// Hulls of shapes of every sort, turned, stretched and moved at random
// (so their flat faces are level only to a rounding error): each is
// closed, holds at least what a union of the same shapes holds, and is
// exactly as big as the shapes together.
func TestHullGroupOfRandomShapes(t *testing.T) {
	kinds := []string{"box", "cylinder", "sphere", "cone", "pyramid", "roof", "wedge", "tube", "torus", "halfsphere",
		"polygon", "icosahedron", "star", "ring", "paraboloid", "roundedbox", "crescent", "text"}
	r := rand.New(rand.NewSource(11))
	for trial := 0; trial < 60; trial++ {
		d := New()
		var ids []int
		var ns []*Node
		for i, n := 0, 2+r.Intn(3); i < n; i++ {
			s, err := d.AddShape(kinds[r.Intn(len(kinds))], geom.V(0, 0, 0), false)
			if err != nil {
				t.Fatal(err)
			}
			s.Size = geom.V(2+r.Float64()*30, 2+r.Float64()*30, 2+r.Float64()*30)
			s.M = geom.Translate(geom.V(r.Float64()*80-40, r.Float64()*80-40, r.Float64()*40)).
				Mul(geom.RotateZ(r.Float64() * 6)).Mul(geom.RotateX(r.Float64() * 6)).Mul(geom.RotateY(r.Float64() * 6))
			if trial%3 == 0 { // square to the axes, and on round numbers
				s.M = geom.Translate(geom.V(float64(r.Intn(9)*10-40), float64(r.Intn(9)*10-40), float64(r.Intn(4)*10)))
				s.Size = geom.V(float64(10+r.Intn(3)*10), float64(10+r.Intn(3)*10), float64(10+r.Intn(3)*10))
			}
			ids = append(ids, s.ID)
			ns = append(ns, s)
		}
		want := Bounds(ns)
		g, err := d.Group(ids, ModeHull)
		if err != nil {
			t.Fatal(err)
		}
		hm := g.WorldMesh(false)
		name := fmt.Sprintf("trial %d", trial)
		sound(t, name, hm)
		got := hm.Bounds()
		if tol := 1e-6 * want.Size().Len(); got.Min.Sub(want.Min).Len() > tol || got.Max.Sub(want.Max).Len() > tol {
			t.Errorf("%s: bounds %v, the shapes' %v", name, got, want)
		}
		g.GroupMode = ModeUnion
		if uv := g.WorldMesh(false).Volume(); hm.Volume() < uv-1e-6*math.Abs(uv) {
			t.Errorf("%s: hull volume %v, less than the union's %v", name, hm.Volume(), uv)
		}
	}
}

// A hull is one part in one colour: the group's, or with Multicolor on the
// first solid's (its new faces belong to no shape).
func TestHullGroupColour(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("box", geom.V(60, 0, 0), false)
	a.Color, b.Color = "#ff0000", "#0000ff"
	g, _ := d.Group([]int{a.ID, b.ID}, ModeHull)
	one := func(want string) {
		t.Helper()
		c := geom.ParseColor(want)
		for _, tr := range g.WorldMesh(false).Tris {
			if tr.C != c {
				t.Fatalf("a face is %v, want %s", tr.C, want)
			}
		}
	}
	one("#ff0000") // the group took its first solid's colour
	g.Color = "#00ff00"
	one("#00ff00")
	g.Multicolor = true
	one("#ff0000")
}
