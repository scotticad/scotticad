package shapes

import (
	"encoding/json"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

func holePath(p SketchPath) SketchPath {
	p.Hole = true
	return p
}

// A hole path cuts what it covers out of the rest of the drawing, wherever
// it is in the list, and a drawing of holes alone builds nothing.
func TestSketchHolePaths(t *testing.T) {
	cases := []struct {
		name  string
		paths []SketchPath
		vol   float64
		loops int
	}{
		{"square with a square hole", []SketchPath{sq(0, 0, 20, 20), holePath(sq(5, 5, 15, 15))}, (400 - 100) * 10, 2},
		{"the hole listed first", []SketchPath{holePath(sq(5, 5, 15, 15)), sq(0, 0, 20, 20)}, (400 - 100) * 10, 2},
		{"a hole over the edge is a notch", []SketchPath{sq(0, 0, 20, 20), holePath(sq(10, 10, 30, 30))}, (400 - 100) * 10, 1},
		{"a hole outside cuts nothing", []SketchPath{sq(0, 0, 20, 20), holePath(sq(30, 30, 40, 40))}, 400 * 10, 1},
		{"two areas, one hole across both", []SketchPath{sq(0, 0, 10, 10), sq(20, 0, 30, 10), holePath(sq(5, 2, 25, 8))}, (200 - 2*5*6) * 10, 2},
		{"a hole line cuts a slot through", []SketchPath{sq(0, 0, 30, 10), holePath(SketchPath{Points: skPts(-5, 5, 35, 5), Stroke: 2})}, (300 - 60) * 10, 2},
		{"a hole stroke cuts a groove ring", []SketchPath{sq(0, 0, 30, 30), holePath(SketchPath{Closed: true, Points: skPts(10, 10, 20, 10, 20, 20, 10, 20), Stroke: 2})}, (900 - (12*12 - 8*8)) * 10, 3},
	}
	for _, c := range cases {
		sk := &Sketch{Paths: c.paths}
		if err := CheckSketch(sk); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if n := len(SketchOutlines(sk)); n != c.loops {
			t.Errorf("%s: %d outlines, want %d", c.name, n, c.loops)
		}
		v := solidOK(t, c.name, SketchSolid(sk, 0, 10))
		if !skNear(v, c.vol, 1e-6) {
			t.Errorf("%s: volume %v, want %v", c.name, v, c.vol)
		}
	}
	only := &Sketch{Paths: []SketchPath{holePath(sq(0, 0, 10, 10))}}
	for _, kind := range []string{"sketch", "revolve", "coil"} {
		if Buildable(kind, only) {
			t.Errorf("%s: a drawing of holes alone builds", kind)
		}
	}
	// the whole area cut away
	if all := (&Sketch{Paths: []SketchPath{sq(0, 0, 10, 10), holePath(sq(-1, -1, 11, 11))}}); Buildable("sketch", all) {
		t.Error("an area wholly under a hole builds")
	}
}

// A revolve of a square with a hole in it is a hollow ring; a coil the same
// profile, risen.
func TestSketchHoleSpun(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{sq(-20, 0, -4, 10), holePath(sq(-16, 2, -8, 8))}}
	m := revolveMesh(sk, 0, 360, 128, 12)
	v := solidOK(t, "hollow ring", m)
	want := math.Pi*(20*20-4*4)*10 - math.Pi*(16*16-8*8)*6
	if math.Abs(v-want) > 0.01*want {
		t.Errorf("hollow ring: volume %v, want about %v", v, want)
	}
	if got := len(revolveProfile(sk, 0)); got != 1 {
		t.Errorf("the profile has %d regions, want 1 (with a hole)", got)
	}
	if r := revolveProfile(sk, 0); len(r) == 1 && len(r[0].Holes) != 1 {
		t.Errorf("the profile has %d holes, want 1", len(r[0].Holes))
	}
	solidOK(t, "hollow coil", coilMesh(sk, 2, 12, 0, 48, 12))
}

// A hole's corner lying part way along a side of the outline (or of
// another hole) is given to that side as a point, so walls built from the
// sides end where the caps' triangles do; outlines that don't touch come
// back as they were.
func TestSplitTouching(t *testing.T) {
	outer := []Pt{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	apart := Region{Outer: outer, Holes: [][]Pt{{{8, 5}, {5, 3}, {5, 7}}}}
	if got := splitTouching([]Region{apart})[0]; len(got.Outer) != 4 || len(got.Holes[0]) != 3 {
		t.Errorf("outlines apart changed: %v", got)
	}
	// the hole's corner on the right side, another's on the first hole's side
	touch := Region{Outer: outer, Holes: [][]Pt{{{10, 5}, {5, 3}, {5, 7}}, {{5, 5}, {2, 4}, {2, 6}}}}
	got := splitTouching([]Region{touch})[0]
	want := []Pt{{0, 0}, {10, 0}, {10, 5}, {10, 10}, {0, 10}}
	if len(got.Outer) != len(want) {
		t.Fatalf("outline %v, want %v", got.Outer, want)
	}
	for i, p := range want {
		if !same(got.Outer[i], p) {
			t.Errorf("outline point %d is %v, want %v", i, got.Outer[i], p)
		}
	}
	if h := got.Holes[0]; len(h) != 4 || !same(h[2], Pt{5, 5}) {
		t.Errorf("the first hole %v, want (5, 5) added on its left side", h)
	}
	ts, _ := triangulate(got)
	if !capFits(ts, got) {
		t.Error("the cap doesn't use the split sides")
	}
	if ts, _ := triangulate(touch); capFits(ts, touch) {
		t.Error("the cap fits the unsplit outline: the test shows nothing")
	}
}

// straighten drops corners a hair off the line through their neighbours
// (two in a row too, over two passes), and leaves real corners and a fine
// curve alone.
func TestStraighten(t *testing.T) {
	const eps = 1e-8
	rect := []Pt{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	if got := straighten(rect, eps); len(got) != 4 {
		t.Errorf("a rectangle became %v", got)
	}
	// its left side with two points 4e-12 off it, next to each other, and
	// its bottom with one exactly on the first corner's other side
	hairs := []Pt{{0, 0}, {5, 2e-12}, {10, 0}, {10, 10}, {0, 10}, {4e-12, 7}, {-4e-12, 3}}
	got := straighten(hairs, eps)
	if len(got) != 4 {
		t.Fatalf("%d corners left, want 4: %v", len(got), got)
	}
	for i, p := range rect {
		if !same(got[i], p) {
			t.Errorf("corner %d is %v, want %v", i, got[i], p)
		}
	}
	// a corner 1e-6 off its side is a corner
	if got := straighten([]Pt{{0, 0}, {5, 1e-6}, {10, 0}, {10, 10}, {0, 10}}, eps); len(got) != 5 {
		t.Errorf("a real corner went: %v", got)
	}
	var circle []Pt
	for i := 0; i < 1000; i++ {
		a := 2 * math.Pi * float64(i) / 1000
		circle = append(circle, Pt{10 * math.Cos(a), 10 * math.Sin(a)})
	}
	if got := straighten(circle, eps); len(got) != len(circle) {
		t.Errorf("a circle of %d points became %d", len(circle), len(got))
	}
}

// capFits refuses a cap that leaves a side out, uses one twice, or has a
// triangle too many.
func TestCapFits(t *testing.T) {
	sq := Region{Outer: []Pt{{0, 0}, {10, 0}, {10, 10}, {0, 10}}}
	a, b := [3]Pt{{0, 0}, {10, 0}, {10, 10}}, [3]Pt{{0, 0}, {10, 10}, {0, 10}}
	if !capFits([][3]Pt{a, b}, sq) {
		t.Error("two triangles of a square don't fit")
	}
	for name, ts := range map[string][][3]Pt{
		"a triangle missing": {a},
		"a triangle twice":   {a, b, b},
		"a side split":       {{{0, 0}, {5, 0}, {10, 10}}, {{5, 0}, {10, 0}, {10, 10}}, b},
	} {
		if capFits(ts, sq) {
			t.Errorf("%s: fits", name)
		}
	}
}

// mendFitted passes a mesh that loses nothing straight through, and
// repairs one that loses a flattened triangle (the T it leaves in a
// side); fitCoil chooses between a coil's builds by them. Every way of building a coil
// comes out closed fitted into 20 mm, for the coils the fuzzer found
// broken that way (129d9130, 40b9c275, b32a7eba).
func TestMendFitted(t *testing.T) {
	v := func(x, y, z float64) geom.Vec3 { return geom.V(x, y, z) }
	tri := func(a, b, c geom.Vec3) geom.Tri { return geom.Tri{P: [3]geom.Vec3{a, b, c}} }
	quad := func(a, b, c, d geom.Vec3) []geom.Tri { return []geom.Tri{tri(a, b, c), tri(a, c, d)} }
	box := func() *geom.Mesh {
		m := &geom.Mesh{}
		for _, q := range [][4]geom.Vec3{
			{v(0, 0, 0), v(0, 10, 0), v(10, 10, 0), v(10, 0, 0)}, // bottom
			{v(0, 0, 10), v(10, 0, 10), v(10, 10, 10), v(0, 10, 10)},
			{v(0, 0, 0), v(10, 0, 0), v(10, 0, 10), v(0, 0, 10)},
			{v(10, 0, 0), v(10, 10, 0), v(10, 10, 10), v(10, 0, 10)},
			{v(10, 10, 0), v(0, 10, 0), v(0, 10, 10), v(10, 10, 10)},
			{v(0, 10, 0), v(0, 0, 0), v(0, 0, 10), v(0, 10, 10)},
		} {
			m.Tris = append(m.Tris, quad(q[0], q[1], q[2], q[3])...)
		}
		return m
	}
	whole := box()
	if out, ok := mendFitted(whole); !ok || out != whole {
		t.Error("a box that loses nothing isn't passed through")
	}
	// the top's first triangle split at the middle of its front side, the
	// front wall keeping the whole side, with a flat triangle between
	a, b, c, mid := v(0, 0, 10), v(10, 0, 10), v(10, 10, 10), v(5, 0, 10)
	flat := box()
	flat.Tris[2] = tri(a, mid, c)
	flat.Tris = append(flat.Tris, tri(mid, b, c), tri(a, b, mid))
	out, ok := mendFitted(flat)
	if !ok {
		t.Fatal("a flat triangle's T isn't repaired")
	}
	watertight(t, "mended", out)

	// fitCoil: the quick build when it loses nothing (no other is made);
	// else the build without cuts when that loses nothing; else the first
	// whose repair is closed
	built := ""
	other := func(m *geom.Mesh) func(coilHow) *geom.Mesh {
		return func(how coilHow) *geom.Mesh {
			built += []string{"quick", "uncut", "fan"}[how]
			return m
		}
	}
	if got := fitCoil(whole, other(box())); got != whole || built != "" {
		t.Errorf("a whole quick build isn't taken as it is (built %q)", built)
	}
	uncut := box()
	if got := fitCoil(flat, other(uncut)); got != uncut || built != "uncut" {
		t.Errorf("the build without cuts isn't taken when the quick one loses a triangle (built %q)", built)
	}
	built = ""
	got := fitCoil(flat, other(flat))
	if built != "uncut" {
		t.Errorf("built %q, want the quick build's repair taken before the fan is built", built)
	}
	watertight(t, "repaired quick build", got)

	k, _ := Lookup("coil")
	size := geom.V(20, 20, 20)
	for _, q := range []float64{41, 20} {
		sk := &Sketch{Paths: []SketchPath{{Closed: true, Stroke: 6.05, Join: JoinBevel, Cap: CapSquare, Align: AlignOutside, Points: []SketchPoint{
			{P: [2]float64{30.75, 0}, In: [2]float64{3.5, 0}, Out: [2]float64{-3.5, 0}}, {P: [2]float64{-7, 5}}}}}}
		val := k.clamp(Values{Num: map[string]float64{"turns": 28, "pitch": 200, "start": 11.6, "sides": 3, "quality": q}, Sketch: sk})
		for how := coilQuick; how <= coilFan; how++ {
			out, ok := mendFitted(sketchModelAs("coil", val, how).FitTo(size))
			if !ok {
				t.Errorf("quality %v, build %d: not closed", q, how)
				continue
			}
			// as Build goes on to treat it
			kept := out.Clean()
			if len(kept.Tris) < len(out.Tris) {
				kept = kept.FixTJunctions()
			}
			watertight(t, "fitted coil", kept)
		}
		watertight(t, "built coil", Build("coil", size, val))
	}
}

// The cover's grid and its remembered clip answer as looking at every
// piece does: random points round wide strokes of many pieces, centered,
// inside and outside their paths, and as holes.
func TestSketchCoverGridSameAnswer(t *testing.T) {
	rnd := rand.New(rand.NewSource(3))
	var paths []SketchPath
	for k := 0; k < 4; k++ {
		p := SketchPath{Closed: k != 3, Stroke: 1 + float64(k), Align: k % 3, Join: k % 3, Hole: k == 2}
		for i := 0; i < 80; i++ {
			// a jagged ring: sides shorter than the stroke is wide
			a := float64(i) / 80 * 2 * math.Pi
			r := 10 + 3*rnd.Float64()
			q := SketchPoint{P: [2]float64{float64(k)*7 + r*math.Cos(a), r * math.Sin(a)}}
			p.Points = append(p.Points, q)
		}
		paths = append(paths, p)
	}
	var solid, cut sketchCover
	var ae, le [][2]Pt
	for _, p := range paths {
		cv := &solid
		if p.Hole {
			cv = &cut
		}
		cv.add(&ae, &le, p, 0)
	}
	if len(solid.lines) < coverGridMin {
		t.Fatalf("only %d pieces: the grid isn't used", len(solid.lines))
	}
	plain := solid
	solid.grid = newCoverGrid(solid.lines)
	// every piece asked in full, the clip each time
	ref := func(q Pt) bool {
		for _, l := range solid.lines {
			if !l.outer.has(q) || (l.clip != nil && l.clip.has(q) != l.inside) {
				continue
			}
			inHole := false
			for _, h := range l.holes {
				inHole = inHole || h.has(q)
			}
			if !inHole {
				return true
			}
		}
		return false
	}
	for i := 0; i < 20000; i++ {
		q := Pt{-25 + 70*rnd.Float64(), -25 + 50*rnd.Float64()}
		if a, b, c := solid.has(q), plain.has(q), ref(q); a != b || b != c {
			t.Fatalf("at %v: %v with the grid, %v without, %v asking every piece in full", q, a, b, c)
		}
	}
}

// Hole is saved only when set, so drawings without holes save as before,
// and it changes the sketch's key (and so its cached outlines).
func TestSketchHoleSaved(t *testing.T) {
	plain := Sketch{Paths: []SketchPath{sq(0, 0, 10, 10)}}
	b, _ := json.Marshal(plain)
	if strings.Contains(string(b), "hole") {
		t.Errorf("a path without a hole saves %s", b)
	}
	holed := Sketch{Paths: []SketchPath{holePath(sq(0, 0, 10, 10))}}
	b, _ = json.Marshal(holed)
	if !strings.Contains(string(b), `"hole":true`) {
		t.Errorf("a hole path saves %s", b)
	}
	var back Sketch
	if err := json.Unmarshal(b, &back); err != nil || !back.Paths[0].Hole {
		t.Errorf("read back %+v, %v", back, err)
	}
	if SketchKey(&plain) == SketchKey(&holed) {
		t.Error("a hole doesn't change the key")
	}
	if c := CopySketch(&holed); !c.Paths[0].Hole {
		t.Error("a copy loses the hole")
	}
}
