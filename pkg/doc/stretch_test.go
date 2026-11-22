package doc

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// stretch wraps the nodes in a stretch group square to world axis at at.
func stretch(t *testing.T, d *Document, axis int, at, amount float64, ns ...*Node) *Node {
	t.Helper()
	g, err := d.Stretch(IDs(ns), geom.Identity(), axis, at, amount)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func volumeOf(ns []*Node, holes bool) float64 {
	v := 0.0
	for _, n := range ns {
		v += n.WorldMesh(holes).Volume()
	}
	return v
}

func nearly(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// A 20 mm box stretched along x through its middle is a longer box; its low
// end stays put. The settings read back as set, can be changed, survive a
// save, and a stretch of 0 ungroups to the box as it was.
func TestStretchGroup(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(5, 0, 0), false)
	before, m0 := a.WorldBounds(), a.M
	g := stretch(t, d, 0, 5, 10, a)
	if len(d.Nodes) != 1 || d.Nodes[0] != g || !g.IsStretch() || !g.IsBundle() || g.IsUnion() || g.Label() != "Stretch" || g.Hole {
		t.Fatalf("not a stretch: %+v", g)
	}
	want := geom.Box{Min: before.Min, Max: before.Max.Add(geom.V(10, 0, 0))}
	if got := g.WorldBounds(); !boxWithin(got, want, 1e-9) {
		t.Errorf("bounds %v, want %v", got, want)
	}
	if v := g.WorldMesh(false).Volume(); !nearly(v, 12000, 1e-6) {
		t.Errorf("volume %v, want 12000", v)
	}
	for k, want := range map[string]float64{"axis": 0, "at": 10, "amount": 10, "length": 30} {
		if v := g.StretchValue(k); !nearly(v, want, 1e-9) {
			t.Errorf("%s reads %v, want %v", k, v, want)
		}
	}
	// shrunk instead, then set by its length
	if err := SetStretchParam(g, "amount", -5); err != nil {
		t.Fatal(err)
	}
	if v := g.WorldMesh(false).Volume(); !nearly(v, 6000, 1e-6) || !nearly(g.StretchValue("length"), 15, 1e-9) {
		t.Errorf("shrunk by 5: volume %v, length %v", v, g.StretchValue("length"))
	}
	if err := SetStretchParam(g, "length", 50); err != nil {
		t.Fatal(err)
	}
	if !nearly(g.StretchValue("amount"), 30, 1e-9) || !nearly(g.WorldBounds().Size().X, 50, 1e-9) {
		t.Errorf("length 50: amount %v, size %v", g.StretchValue("amount"), g.WorldBounds().Size())
	}
	for _, c := range []struct {
		key string
		v   float64
	}{{"amount", -20}, {"length", 0}, {"at", 0}, {"at", 20}, {"axis", 3}, {"amount", math.NaN()}, {"wobble", 1}} {
		if err := SetStretchParam(g, c.key, c.v); err == nil {
			t.Errorf("%s = %v: no error", c.key, c.v)
		}
	}
	// saved and read back: an older format number, the same result
	data, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	d2, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if d2.Version != 1 {
		t.Errorf("saved as format %d, want 1", d2.Version)
	}
	if g2 := d2.Nodes[0]; !g2.IsStretch() || !boxWithin(g2.WorldBounds(), g.WorldBounds(), 1e-9) {
		t.Errorf("read back: bounds %v, want %v", g2.WorldBounds(), g.WorldBounds())
	}
	// moved and turned, it takes its plane with it
	RotateAbout([]*Node{g}, 2, 90, geom.V(0, 0, 0))
	if s := g.WorldBounds().Size(); !nearly(s.Y, 50, 1e-9) || !nearly(s.X, 20, 1e-9) {
		t.Errorf("turned: size %v", s)
	}
	RotateAbout([]*Node{g}, 2, -90, geom.V(0, 0, 0))
	// a stretch of nothing ungroups to the box as it was
	SetStretchParam(g, "amount", 0)
	rel := d.Ungroup([]int{g.ID})
	if len(rel) != 1 || rel[0].ID != a.ID || rel[0].Kind != "box" || !boxWithin(rel[0].WorldBounds(), before, 1e-9) {
		t.Fatalf("released %+v", rel)
	}
	a = rel[0]
	for i := range m0 {
		if math.Abs(a.M[i]-m0[i]) > 1e-12 {
			t.Fatalf("the box's matrix is %v, was %v", a.M, m0)
		}
	}
}

// Shapes the plane misses stay shapes when it is baked in: the one beyond
// it moves, the one before stays, only the one it crosses becomes a mesh.
// What ungrouping gives is what the stretch showed.
func TestStretchUngroupBakes(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("cylinder", geom.V(40, 0, 0), false)
	c, _ := d.AddShape("sphere", geom.V(80, 0, 0), false)
	bb, cb := b.WorldBounds(), c.WorldBounds()
	g := stretch(t, d, 0, 40, 15, a, b, c)
	shown, vol := g.WorldBounds(), volumeOf([]*Node{g}, false)
	rel := d.Ungroup([]int{g.ID})
	if len(rel) != 3 || rel[0].ID != a.ID || rel[2].ID != c.ID {
		t.Fatalf("released %d: %+v", len(rel), rel)
	}
	a, c = rel[0], rel[2]
	if a.Kind != "box" || c.Kind != "sphere" || rel[1].Kind != KindMesh || rel[1].Name != "Cylinder" {
		t.Errorf("kinds %s %s %s", a.Kind, rel[1].Kind, c.Kind)
	}
	if got := c.WorldBounds(); !boxWithin(got, geom.Box{Min: cb.Min.Add(geom.V(15, 0, 0)), Max: cb.Max.Add(geom.V(15, 0, 0))}, 1e-9) {
		t.Errorf("the sphere is at %v", got)
	}
	if got := rel[1].WorldBounds(); !boxWithin(got, geom.Box{Min: bb.Min, Max: bb.Max.Add(geom.V(15, 0, 0))}, 1e-5) {
		t.Errorf("the cylinder is %v, was %v", got, bb)
	}
	sound(t, "the stretched cylinder", rel[1].WorldMesh(false))
	if got := Bounds(d.Nodes); !boxWithin(got, shown, 1e-5) || !nearly(volumeOf(d.Nodes, false), vol, 1e-3) {
		t.Errorf("ungrouped: %v %v, shown %v %v", got, volumeOf(d.Nodes, false), shown, vol)
	}
}

// Shrinking takes out a slab: a shape wholly in it is gone, shown and
// baked; those beyond move back.
func TestStretchShrinkRemoves(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("box", geom.V(30, 0, 0), false)
	c, _ := d.AddShape("box", geom.V(60, 0, 0), false)
	g := stretch(t, d, 0, 30, -22, a, b, c)
	if n := len(g.Parts()); n != 2 {
		t.Errorf("%d parts shown, want 2", n)
	}
	// -10 to 10 stays, 20 to 40 is in the slab (19 to 41), 50 to 70 moves 22 back
	if s := g.WorldBounds().Size(); !nearly(s.X, 58, 1e-9) {
		t.Errorf("size %v", s)
	}
	rel := d.Ungroup([]int{g.ID})
	if len(rel) != 2 || rel[0].ID != a.ID || rel[1].ID != c.ID || rel[1].Kind != "box" || !nearly(rel[1].WorldBounds().Min.X, 28, 1e-9) {
		t.Errorf("released %+v", rel)
	}
}

// A hole stays a hole: a box with a hole through it, in a union, stretched
// across the hole: the hole is longer, shown and baked; baked, the union
// keeps its two parts, both meshes now.
func TestStretchHoles(t *testing.T) {
	d := New()
	plate, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	hole, _ := d.AddShape("cylinder", geom.V(0, 0, 0), true)
	ScaleAbout([]*Node{hole}, geom.V(0.5, 0.5, 2), geom.V(0, 0, 0))
	u, err := d.Group(IDs([]*Node{plate, hole}), ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	solid := u.WorldMesh(false).Volume()
	g := stretch(t, d, 1, 1, 6, u)
	shown := g.WorldMesh(false)
	sound(t, "shown", shown)
	// the cut through y = 1 is 20 × 20 less the hole's chord, 20 tall
	if v := shown.Volume(); !(v > solid+6*(20-10)*20-1 && v < solid+6*20*20) {
		t.Errorf("volume %v from %v", v, solid)
	}
	rel := d.Ungroup([]int{g.ID})
	if len(rel) != 1 || rel[0].ID != u.ID || len(rel[0].Children) != 2 || rel[0].Children[0].Kind != KindMesh || !rel[0].Children[1].Hole {
		t.Fatalf("released %+v", rel)
	}
	u = rel[0]
	if v := u.WorldMesh(false).Volume(); !nearly(v, shown.Volume(), 1e-3) {
		t.Errorf("baked volume %v, shown %v", v, shown.Volume())
	}
}

// A stretch inside a union, a union inside a stretch, a pattern and a
// stretch inside a stretch: each shows and bakes the same.
func TestStretchNesting(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("cylinder", geom.V(10, 0, 0), false)
	inner := stretch(t, d, 2, 10, 5, a)
	u, _ := d.Group(IDs([]*Node{inner, b}), ModeUnion)
	sound(t, "a stretch in a union", u.WorldMesh(false))
	p, _ := d.AddShape("tube", geom.V(0, 40, 0), false)
	pat, _ := d.Pattern([]int{p.ID})
	outer := stretch(t, d, 0, 12, 7, u, pat)
	shown, vol := outer.WorldBounds(), volumeOf([]*Node{outer}, false)
	data, _ := d.Marshal()
	d2, _ := Unmarshal(data)
	if got := d2.Nodes[0].WorldBounds(); !boxWithin(got, shown, 1e-9) {
		t.Errorf("read back %v, shown %v", got, shown)
	}
	d.Ungroup([]int{outer.ID})
	if got := Bounds(d.Nodes); !boxWithin(got, shown, 1e-4) || !nearly(volumeOf(d.Nodes, false), vol, 1e-2) {
		t.Errorf("ungrouped %v %v, shown %v %v", got, volumeOf(d.Nodes, false), shown, vol)
	}
	// the inner stretch is wholly before the plane: it is left as it was
	if u := d.Find(u.ID); u == nil || !u.Children[0].IsStretch() {
		t.Errorf("the inner stretch wasn't kept")
	}
}

// Split cuts what the stretch shows.
func TestStretchSplit(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	g := stretch(t, d, 0, 0, 20, a)
	vol := g.WorldMesh(false).Volume()
	hs, err := d.Split([]int{g.ID}, geom.Identity(), 0, 5, KeepBoth, 0)
	if err != nil || len(hs) != 2 {
		t.Fatalf("split: %v, %d halves", err, len(hs))
	}
	if v := volumeOf(hs, false); !nearly(v, vol, 1e-3) {
		t.Errorf("halves %v, whole %v", v, vol)
	}
	if b := hs[1].WorldBounds(); !nearly(b.Min.X, 5, 1e-6) || !nearly(b.Max.X, 30, 1e-6) {
		t.Errorf("the right half is %v", b)
	}
}

// The plane follows a turned construction plane: the stretch is along its
// axis, and the group's axes are its.
func TestStretchOnATurnedPlane(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	f := geom.Translate(geom.V(3, 4, 0)).Mul(geom.RotateZ(math.Pi / 4))
	b := BoundsIn([]*Node{a}, f)
	g, err := d.Stretch([]int{a.ID}, f, 0, b.Center().X, 10)
	if err != nil {
		t.Fatal(err)
	}
	got := BoundsIn([]*Node{g}, f)
	if s := got.Size(); !nearly(s.X, b.Size().X+10, 1e-9) || !nearly(s.Y, b.Size().Y, 1e-9) || !nearly(got.Min.X, b.Min.X, 1e-9) {
		t.Errorf("in the plane's frame: %v, was %v", got, b)
	}
	if !nearly(g.StretchValue("length"), b.Size().X+10, 1e-9) {
		t.Errorf("length %v", g.StretchValue("length"))
	}
}

// What a file might hold: numbers out of range or missing are made safe.
func TestStretchBadNumbers(t *testing.T) {
	for _, ps := range []map[string]float64{nil, {"axis": 7, "at": math.NaN(), "amount": 1e300}, {"axis": -1, "amount": math.Inf(-1)}, {"amount": -1e9}} {
		d := New()
		a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
		g := stretch(t, d, 2, 10, 4, a)
		g.Params = ps
		b := g.WorldBounds()
		if b.IsEmpty() && ps["amount"] > -1e8 {
			t.Errorf("%v: nothing shown", ps)
		}
		if s := b.Size(); !b.IsEmpty() && (math.IsNaN(s.X+s.Y+s.Z) || s.Len() > 3*maxSpacing) {
			t.Errorf("%v: size %v", ps, s)
		}
		d.Ungroup([]int{g.ID})
	}
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	for _, c := range []struct {
		axis       int
		at, amount float64
	}{{3, 10, 1}, {0, 50, 1}, {0, -10, 1}, {0, 0, -20}, {0, 0, math.NaN()}, {0, 0, 2 * maxSpacing}} {
		if _, err := d.Stretch([]int{a.ID}, geom.Identity(), c.axis, c.at, c.amount); err == nil {
			t.Errorf("%v: no error", c)
		}
	}
	if len(d.Nodes) != 1 || d.Nodes[0] != a {
		t.Errorf("a refused stretch changed the document")
	}
}

// Random shapes, turned and moved, some of them holes, in a stretch at
// random: what ungrouping gives is what the stretch showed, every baked
// part closed.
func TestStretchRandomShapes(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	kinds := []string{"box", "cylinder", "sphere", "cone", "wedge", "tube", "star", "roof", "torus"}
	for trial := 0; trial < 40; trial++ {
		d := New()
		var ns []*Node
		for i := 0; i <= r.Intn(3); i++ {
			n, err := d.AddShape(kinds[r.Intn(len(kinds))], geom.V(r.Float64()*40, r.Float64()*40, r.Float64()*10), r.Intn(4) == 0)
			if err != nil {
				t.Fatal(err)
			}
			Apply([]*Node{n}, geom.RotateZ(r.Float64()*6).Mul(geom.RotateX(r.Float64())))
			ns = append(ns, n)
		}
		axis := r.Intn(3)
		b := BoundsIn(ns, geom.Identity())
		at := b.Min.Axis(axis) + (0.1+0.8*r.Float64())*b.Size().Axis(axis)
		amount := r.Float64()*30 - 0.6*b.Size().Axis(axis)/2
		name := fmt.Sprintf("trial %d: axis %d at %.3f by %.3f", trial, axis, at, amount)
		g, err := d.Stretch(IDs(ns), geom.Identity(), axis, at, amount)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		Apply([]*Node{g}, geom.Translate(geom.V(5, -3, 2)).Mul(geom.RotateZ(r.Float64()*6)))
		shown, vol, withHoles := g.WorldBounds(), volumeOf([]*Node{g}, false), volumeOf([]*Node{g}, true)
		d.Ungroup([]int{g.ID})
		for _, n := range d.Nodes {
			if n.Kind == KindMesh {
				sound(t, name+": "+n.Name, n.WorldMesh(true))
			}
		}
		if got := Bounds(d.Nodes); !boxWithin(got, shown, 1e-4) {
			t.Errorf("%s: ungrouped bounds %v, shown %v", name, got, shown)
		}
		if v, h := volumeOf(d.Nodes, false), volumeOf(d.Nodes, true); !nearly(v, vol, 1e-4*(1+vol)) || !nearly(h, withHoles, 1e-4*(1+withHoles)) {
			t.Errorf("%s: ungrouped volume %v/%v, shown %v/%v", name, v, h, vol, withHoles)
		}
	}
}
