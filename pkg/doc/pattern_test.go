package doc

import (
	"math"
	"math/rand"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

func boxWithin(a, b geom.Box, tol float64) bool {
	return a.Min.Sub(b.Min).Len() <= tol && a.Max.Sub(b.Max).Len() <= tol
}

// pattern makes a pattern of the nodes and sets its numbers.
func pattern(t *testing.T, d *Document, set map[string]float64, ns ...*Node) *Node {
	t.Helper()
	g, err := d.Pattern(IDs(ns))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range set {
		if err := SetPatternParam(g, k, v); err != nil {
			t.Fatal(err)
		}
	}
	return g
}

// A grid pattern shows its shape in rows and columns: a 20 mm box, four
// columns 30 mm apart and three rows 25 mm apart, is twelve boxes. The box
// stays where it was, as the first.
func TestPatternGrid(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(5, 7, 0), false)
	before := a.WorldBounds()
	g, err := d.Pattern([]int{a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Nodes) != 1 || d.Nodes[0] != g || !g.IsPattern() || !g.IsBundle() || g.IsUnion() || g.Label() != "Pattern" || g.Hole {
		t.Fatalf("not a pattern: %+v", g)
	}
	// as made: a row of three, a shape and a half apart
	if got, want := g.WorldBounds(), (geom.Box{Min: before.Min, Max: before.Max.Add(geom.V(60, 0, 0))}); !boxWithin(got, want, 1e-9) {
		t.Errorf("as made: bounds %v, want %v", got, want)
	}
	for k, v := range map[string]float64{"columns": 4, "rows": 3, "dx": 30, "dy": 25} {
		if err := SetPatternParam(g, k, v); err != nil {
			t.Fatal(err)
		}
	}
	parts := g.Parts()
	if len(parts) != 12 || len(g.LocalParts()) != 12 {
		t.Fatalf("%d parts, %d local, want 12", len(parts), len(g.LocalParts()))
	}
	if !boxWithin(parts[0].Mesh.Bounds(), before, 1e-9) {
		t.Errorf("the first copy is at %v, the box was at %v", parts[0].Mesh.Bounds(), before)
	}
	want := geom.Box{Min: before.Min, Max: before.Max.Add(geom.V(90, 50, 0))}
	if got := g.WorldBounds(); !boxWithin(got, want, 1e-9) {
		t.Errorf("bounds %v, want %v", got, want)
	}
	if v := g.WorldMesh(false).Volume(); math.Abs(v-12*8000) > 1e-6 {
		t.Errorf("volume %v, want %v", v, 12*8000)
	}
	// every second row half a column along
	SetPatternParam(g, "stagger", 1)
	want.Max.X += 15
	if got := g.WorldBounds(); !boxWithin(got, want, 1e-9) {
		t.Errorf("staggered: bounds %v, want %v", got, want)
	}
	if b := g.Parts()[4].Mesh.Bounds(); math.Abs(b.Min.X-(before.Min.X+15)) > 1e-9 || math.Abs(b.Min.Y-(before.Min.Y+25)) > 1e-9 {
		t.Errorf("staggered: the second row starts at %v", b.Min)
	}
	// saved and read back
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
	if g2 := d2.Nodes[0]; !g2.IsPattern() || len(g2.Parts()) != 12 || !boxWithin(g2.WorldBounds(), want, 1e-9) {
		t.Errorf("read back: %d parts, bounds %v", len(g2.Parts()), g2.WorldBounds())
	}
}

// A circle pattern puts its copies round a centre one radius to the
// shape's left, turning each to face the same way out, or not.
func TestPatternCircle(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(30, 0, 0), false)
	ScaleAbout([]*Node{a}, geom.V(1, 0.5, 1), geom.V(30, 0, 0)) // 20 x 10
	g := pattern(t, d, map[string]float64{"type": PatternCircle, "count": 4, "radius": 30}, a)
	parts := g.Parts()
	if len(parts) != 4 {
		t.Fatalf("%d parts", len(parts))
	}
	// at 30 mm from the origin, on +x, +y, -x, -y; the long side pointing out
	for i, want := range []geom.Box{
		{Min: geom.V(20, -5, 0), Max: geom.V(40, 5, 20)}, {Min: geom.V(-5, 20, 0), Max: geom.V(5, 40, 20)},
		{Min: geom.V(-40, -5, 0), Max: geom.V(-20, 5, 20)}, {Min: geom.V(-5, -40, 0), Max: geom.V(5, -20, 20)},
	} {
		if got := parts[i].Mesh.Bounds(); !boxWithin(got, want, 1e-9) {
			t.Errorf("copy %d at %v, want %v", i, got, want)
		}
	}
	// not turned: each keeps the box's direction
	SetPatternParam(g, "turn", 0)
	if got, want := g.Parts()[1].Mesh.Bounds(), (geom.Box{Min: geom.V(-10, 25, 0), Max: geom.V(10, 35, 20)}); !boxWithin(got, want, 1e-9) {
		t.Errorf("unturned copy 1 at %v, want %v", got, want)
	}
	// an arc has a copy at each end: three over 90° are at 0°, 45° and 90°
	SetPatternParam(g, "count", 3)
	SetPatternParam(g, "angle", 90)
	parts = g.Parts()
	if c := parts[2].Mesh.Bounds().Center(); len(parts) != 3 || math.Abs(c.X) > 1e-9 || math.Abs(c.Y-30) > 1e-9 {
		t.Errorf("arc: %d parts, the last at %v", len(parts), c)
	}
	if c, s := parts[1].Mesh.Bounds().Center(), 30*math.Sqrt2/2; math.Abs(c.X-s) > 1e-9 || math.Abs(c.Y-s) > 1e-9 {
		t.Errorf("arc: the middle one at %v", c)
	}
	// its radius changed, the first copy stays put
	SetPatternParam(g, "radius", 50)
	if got, want := g.Parts()[0].Mesh.Bounds(), (geom.Box{Min: geom.V(20, -5, 0), Max: geom.V(40, 5, 20)}); !boxWithin(got, want, 1e-9) {
		t.Errorf("radius changed: the first copy at %v", got)
	}
}

// A pattern keeps its parts apart, as a bundle does: several shapes keep
// their colours, and a hole in it stays a hole, which cuts when the
// pattern is grouped with a solid: a row of four holes through a plate.
func TestPatternOfHoles(t *testing.T) {
	d := New()
	plate, _ := d.AddShape("box", geom.V(45, 0, 0), false)
	ScaleAbout([]*Node{plate}, geom.V(6, 1.5, 0.25), geom.V(45, 0, 0)) // 120 x 30 x 5
	hole, _ := d.AddShape("box", geom.V(0, 0, 0), true)
	ScaleAbout([]*Node{hole}, geom.V(0.5, 0.5, 1), geom.V(0, 0, 0)) // 10 x 10 x 20
	p := pattern(t, d, map[string]float64{"columns": 4, "dx": 30}, hole)
	if p.Hole || len(p.Parts()) != 4 || !p.Parts()[3].Hole {
		t.Fatalf("pattern of a hole: hole %v, %d parts", p.Hole, len(p.Parts()))
	}
	if v := p.WorldMesh(false).Volume(); v != 0 {
		t.Errorf("its solid volume is %v", v)
	}
	g, err := d.Group([]int{plate.ID, p.ID}, ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	m := g.WorldMesh(false)
	sound(t, "plate", m)
	if v, want := m.Volume(), 120*30*5-4*10*10*5.0; math.Abs(v-want) > 1e-6 {
		t.Errorf("plate: volume %v, want %v", v, want)
	}
	// the pattern's numbers are part of what the plate is: changed, it is
	// worked out again
	SetPatternParam(p, "columns", 2)
	if v, want := g.WorldMesh(false).Volume(), 120*30*5-2*10*10*5.0; math.Abs(v-want) > 1e-6 {
		t.Errorf("two holes: volume %v, want %v", v, want)
	}
	// and so is its shape
	SetParam(hole, "radius", 0)
	hole.Size = geom.V(20, 10, 20)
	if v, want := g.WorldMesh(false).Volume(), 120*30*5-2*20*10*5.0; math.Abs(v-want) > 1e-6 {
		t.Errorf("wider holes: volume %v, want %v", v, want)
	}
	// made a hole itself, a pattern of solids cuts too
	d2 := New()
	plate, _ = d2.AddShape("box", geom.V(45, 0, 0), false)
	ScaleAbout([]*Node{plate}, geom.V(6, 1.5, 0.25), geom.V(45, 0, 0))
	peg, _ := d2.AddShape("box", geom.V(0, 0, 0), false)
	ScaleAbout([]*Node{peg}, geom.V(0.5, 0.5, 1), geom.V(0, 0, 0))
	p = pattern(t, d2, map[string]float64{"columns": 4, "dx": 30}, peg)
	p.Hole = true
	g, _ = d2.Group([]int{plate.ID, p.ID}, ModeUnion)
	if v, want := g.WorldMesh(false).Volume(), 120*30*5-4*10*10*5.0; math.Abs(v-want) > 1e-6 {
		t.Errorf("a hole pattern of pegs: volume %v, want %v", v, want)
	}
}

// Ungrouping a pattern gives every copy as a shape of its own, where it
// was shown, the originals first and unchanged.
func TestPatternUngroup(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("cylinder", geom.V(0, 30, 0), true)
	g := pattern(t, d, map[string]float64{"type": PatternCircle, "count": 5, "radius": 40}, a, b)
	Apply([]*Node{g}, geom.Translate(geom.V(3, 4, 5)).Mul(geom.RotateZ(0.3)))
	g.Transparent = true
	var want []geom.Box
	for _, p := range g.Parts() {
		want = append(want, p.Mesh.Bounds())
	}
	rel := d.Ungroup([]int{g.ID})
	if len(rel) != 10 || len(d.Nodes) != 10 || rel[0] != a || rel[1] != b {
		t.Fatalf("released %d, %d nodes", len(rel), len(d.Nodes))
	}
	seen := map[int]bool{}
	for i, n := range rel {
		if seen[n.ID] || n.ID <= 0 {
			t.Errorf("node %d has id %d again", i, n.ID)
		}
		seen[n.ID] = true
		if n.Hole != (i%2 == 1) || !n.Transparent {
			t.Errorf("node %d: hole %v, transparent %v", i, n.Hole, n.Transparent)
		}
		if got := n.WorldBounds(); !boxWithin(got, want[i], 1e-9) {
			t.Errorf("node %d at %v, was shown at %v", i, got, want[i])
		}
	}
}

// A pattern in a pattern, moved, turned and stretched, shows what its
// copies ungrouped all the way down are; and a stretched pattern's lengths
// are given as they measure.
func TestPatternNestedAndStretched(t *testing.T) {
	d := New()
	a, _ := d.AddShape("cylinder", geom.V(0, 0, 0), false)
	inner := pattern(t, d, map[string]float64{"type": PatternCircle, "count": 3, "radius": 15}, a)
	outer := pattern(t, d, map[string]float64{"columns": 2, "rows": 2, "dx": 70, "dy": 60}, inner)
	if n := len(outer.Parts()); n != 12 {
		t.Fatalf("%d parts, want 12", n)
	}
	Apply([]*Node{outer}, geom.Translate(geom.V(10, 0, 0)).Mul(geom.RotateZ(0.5)).Mul(geom.Scale(geom.V(2, 1, 1))))
	if v := outer.PatternValue("dx"); math.Abs(v-140) > 1e-9 {
		t.Errorf("stretched twice along its columns: spacing %v, want 140", v)
	}
	if v := outer.PatternValue("dy"); math.Abs(v-60) > 1e-9 {
		t.Errorf("row spacing %v, want 60", v)
	}
	SetPatternParam(outer, "dx", 100)
	if v := outer.Params["dx"]; math.Abs(v-50) > 1e-9 {
		t.Errorf("set to 100: stored %v, want 50", v)
	}
	whole := outer.WorldMesh(false)
	wantB, wantV := outer.WorldBounds(), whole.Volume()
	d.Ungroup([]int{outer.ID})
	if len(d.Nodes) != 4 {
		t.Fatalf("%d nodes after ungrouping the grid", len(d.Nodes))
	}
	d.Ungroup(IDs(d.Nodes))
	if len(d.Nodes) != 12 {
		t.Fatalf("%d nodes after ungrouping the circles", len(d.Nodes))
	}
	v := 0.0
	for _, n := range d.Nodes {
		v += n.WorldMesh(false).Volume()
	}
	if got := Bounds(d.Nodes); !boxWithin(got, wantB, 1e-9) || math.Abs(v-wantV) > 1e-6 {
		t.Errorf("ungrouped: bounds %v volume %v, shown %v %v", got, v, wantB, wantV)
	}
}

// A pattern is split copy by copy: the halves hold what the pattern
// showed.
func TestPatternSplit(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	g := pattern(t, d, map[string]float64{"columns": 4, "dx": 30}, a) // x -10..100
	halves, err := d.Split([]int{g.ID}, geom.Identity(), 0, 35, KeepBoth, 0)
	if err != nil || len(halves) != 2 {
		t.Fatalf("%v, %d halves", err, len(halves))
	}
	// the plane goes through the second box, 15 mm along it
	l, r := halves[0].WorldMesh(false).Volume(), halves[1].WorldMesh(false).Volume()
	if !near(l, 8000+15*400) || !near(r, 5*400+2*8000) {
		t.Errorf("halves of volume %v and %v", l, r)
	}
	for _, h := range halves {
		if h.IsPattern() || !h.IsBundle() {
			t.Errorf("a half is a %s", h.Label())
		}
	}
	seen := map[int]bool{}
	for _, n := range d.Nodes {
		n.Walk(func(x *Node) {
			if seen[x.ID] {
				t.Errorf("id %d twice", x.ID)
			}
			seen[x.ID] = true
		})
	}
}

// Numbers a file might hold: each is brought within its range, so no
// pattern has no copies, a great many, or one off at infinity.
func TestPatternBadNumbers(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	g, _ := d.Pattern([]int{a.ID})
	for _, p := range []map[string]float64{
		nil, {}, {"columns": 0, "rows": -4}, {"columns": 1e9, "rows": 1e9}, {"dx": math.Inf(1), "dy": math.NaN(), "columns": math.NaN()},
		{"type": 7, "count": -1, "radius": -5}, {"type": 1, "count": 1e12, "radius": math.Inf(1), "angle": 0, "turn": 9},
		{"type": math.NaN()}, {"type": 0.5, "stagger": 0.5, "dx": 1e300}, {"columns": 2.5},
	} {
		g.Params = p
		cs := g.Copies()
		if len(cs) < 1 || len(cs) > 2500 || cs[0] != geom.Identity() {
			t.Errorf("%v: %d copies", p, len(cs))
		}
		for _, m := range cs {
			for _, x := range m {
				if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > 1e7 {
					t.Fatalf("%v: a copy at %v", p, m)
				}
			}
		}
		if b := g.WorldBounds(); b.IsEmpty() {
			t.Errorf("%v: no bounds", p)
		}
	}
	if err := SetPatternParam(g, "depth", 1); err == nil {
		t.Error("a setting it hasn't was accepted")
	}
	if err := SetPatternParam(g, "dx", math.NaN()); err == nil {
		t.Error("a spacing that isn't a number was accepted")
	}
	if _, err := d.Pattern(nil); err == nil {
		t.Error("a pattern of nothing")
	}
}

// Random patterns of random shapes show what their copies are, ungrouped.
func TestPatternOfRandomShapes(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	kinds := []string{"box", "cylinder", "sphere", "cone", "wedge", "tube", "star", "roof"}
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
		set := map[string]float64{"type": float64(r.Intn(2)), "columns": float64(1 + r.Intn(5)), "rows": float64(1 + r.Intn(4)),
			"dx": r.Float64()*80 - 20, "dy": r.Float64()*80 - 20, "stagger": float64(r.Intn(2)),
			"count": float64(1 + r.Intn(9)), "radius": r.Float64() * 60, "angle": float64(1 + r.Intn(360)), "turn": float64(r.Intn(2))}
		g := pattern(t, d, set, ns...)
		Apply([]*Node{g}, geom.Translate(geom.V(5, -3, 2)).Mul(geom.RotateZ(r.Float64()*6)).Mul(geom.RotateY(r.Float64())))
		wantB, wantV, wantN := g.WorldBounds(), g.WorldMesh(false).Volume(), len(g.Parts())
		wantH := g.WorldMesh(true).Volume()
		rel := d.Ungroup([]int{g.ID})
		if len(rel) != wantN {
			t.Fatalf("trial %d: %d shown, %d released", trial, wantN, len(rel))
		}
		v, h := 0.0, 0.0
		for _, n := range d.Nodes {
			v += n.WorldMesh(false).Volume()
			h += n.WorldMesh(true).Volume()
		}
		if got := Bounds(d.Nodes); !boxWithin(got, wantB, 1e-7) || math.Abs(v-wantV) > 1e-6*(1+wantV) || math.Abs(h-wantH) > 1e-6*(1+wantH) {
			t.Errorf("trial %d (%v): ungrouped bounds %v volume %v/%v, shown %v %v/%v", trial, set, got, v, h, wantB, wantV, wantH)
		}
	}
}
