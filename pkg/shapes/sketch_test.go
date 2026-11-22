package shapes

import (
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

func skPts(ps ...float64) []SketchPoint {
	var out []SketchPoint
	for i := 0; i+1 < len(ps); i += 2 {
		out = append(out, SketchPoint{P: [2]float64{ps[i], ps[i+1]}})
	}
	return out
}

func sq(x0, y0, x1, y1 float64) SketchPath {
	return SketchPath{Closed: true, Points: skPts(x0, y0, x1, y0, x1, y1, x0, y1)}
}

func skNear(a, b, tol float64) bool { return math.Abs(a-b) <= tol*math.Max(1, math.Abs(b)) }

func solidOK(t *testing.T, name string, m *geom.Mesh) float64 {
	t.Helper()
	watertight(t, name, m)
	v := m.Oriented().Volume()
	if v <= 0 {
		t.Errorf("%s: volume %v", name, v)
	}
	return v
}

func TestSketchAreas(t *testing.T) {
	cases := []struct {
		name string
		sk   Sketch
		vol  float64
	}{
		{"square", Sketch{Paths: []SketchPath{sq(0, 0, 20, 20)}}, 4000},
		// one inside another joins it: no hole
		{"square inside a square", Sketch{Paths: []SketchPath{sq(0, 0, 20, 20), sq(5, 5, 15, 15)}}, 4000},
		{"inside, touching", Sketch{Paths: []SketchPath{sq(0, 0, 20, 20), sq(0, 5, 10, 15)}}, 4000},
		{"two overlapping", Sketch{Paths: []SketchPath{sq(0, 0, 20, 20), sq(10, 0, 30, 20)}}, 6000},
		{"apart", Sketch{Paths: []SketchPath{sq(0, 0, 10, 10), sq(20, 0, 30, 10)}}, 2000},
		{"bowtie", Sketch{Paths: []SketchPath{{Closed: true, Points: skPts(0, 0, 20, 20, 20, 0, 0, 20)}}}, 2000},
		{"straight line", Sketch{Paths: []SketchPath{{Points: skPts(0, 0, 30, 0), Stroke: 2}}}, 600},
		{"square ends", Sketch{Paths: []SketchPath{{Points: skPts(0, 0, 30, 0), Stroke: 2, Cap: CapSquare}}}, 640},
		{"frame", Sketch{Paths: []SketchPath{{Closed: true, Points: skPts(0, 0, 20, 0, 20, 20, 0, 20), Stroke: 2}}}, (22*22 - 18*18) * 10},
		{"L line", Sketch{Paths: []SketchPath{{Points: skPts(0, 0, 20, 0, 20, 20), Stroke: 2}}}, (21*2 + 19*2) * 10},
		{"line on an area", Sketch{Paths: []SketchPath{sq(0, 0, 20, 20), {Points: skPts(10, 10, 40, 10), Stroke: 2}}}, 4000 + 20*2*10},
	}
	for _, c := range cases {
		m := SketchSolid(&c.sk, 0, 10)
		v := solidOK(t, c.name, m)
		if !skNear(v, c.vol, 1e-6) {
			t.Errorf("%s: volume %v, want %v", c.name, v, c.vol)
		}
	}
}

func TestSketchRoundLine(t *testing.T) {
	sk := Sketch{Paths: []SketchPath{{Points: skPts(0, 0, 30, 0), Stroke: 4, Cap: CapRound}}}
	v := solidOK(t, "round ends", SketchSolid(&sk, 0, 1))
	// a polygon circle is a little smaller than the round one
	want := 30*4 + math.Pi*4
	if v > want || v < want*0.99 {
		t.Errorf("volume %v, want about %v", v, want)
	}
}

// A curve built from handles follows the cubic, and the automatic points'
// curve passes through its points.
func TestSketchCurves(t *testing.T) {
	// a quarter circle of radius 10 from handles 0.5523 of the radius long
	k := 0.5523 * 10
	p := SketchPath{Points: []SketchPoint{
		{P: [2]float64{10, 0}, Out: [2]float64{0, k}},
		{P: [2]float64{0, 10}, In: [2]float64{k, 0}},
	}}
	f := p.Flatten()
	if len(f) < 8 {
		t.Fatalf("only %d points", len(f))
	}
	for _, q := range f {
		if r := math.Hypot(q.X, q.Y); math.Abs(r-10) > 0.003 {
			t.Fatalf("point %v is %v from the centre", q, r)
		}
	}
	auto := SketchPath{Closed: true}
	for _, q := range skPts(0, 0, 10, 0, 10, 10, 0, 10) {
		q.Kind = PointAuto
		auto.Points = append(auto.Points, q)
	}
	f = auto.Flatten()
	for _, q := range auto.Points {
		found := false
		for _, g := range f {
			found = found || same(g, Pt{q.P[0], q.P[1]})
		}
		if !found {
			t.Errorf("the curve misses its point %v", q.P)
		}
	}
	solidOK(t, "smooth", SketchSolid(&Sketch{Paths: []SketchPath{auto}}, 0, 1))
}

func TestRevolve(t *testing.T) {
	cases := []struct {
		name         string
		sk           Sketch
		sweep        float64
		vol          float64
		tol          float64
		buildableNot bool
	}{
		{"tube", Sketch{Paths: []SketchPath{sq(-10, 0, -5, 10)}}, 360, math.Pi * (100 - 25) * 10, 0.01, false},
		{"across the axis", Sketch{Paths: []SketchPath{sq(-10, 0, 5, 10)}}, 360, math.Pi * 100 * 10, 0.01, false},
		{"half", Sketch{Paths: []SketchPath{sq(-10, 0, 0, 10)}}, 180, math.Pi * 100 * 10 / 2, 0.01, false},
		{"quarter tube", Sketch{Paths: []SketchPath{sq(-10, 0, -5, 10)}}, 90, math.Pi * (100 - 25) * 10 / 4, 0.01, false},
		{"one inside another", Sketch{Paths: []SketchPath{sq(-10, 0, 0, 10), sq(-6, 3, -3, 6)}}, 360, math.Pi * 100 * 10, 0.01, false},
		{"right only", Sketch{Paths: []SketchPath{sq(1, 0, 10, 10)}}, 360, 0, 0, true},
	}
	for _, c := range cases {
		if Buildable("revolve", &c.sk) == c.buildableNot {
			t.Errorf("%s: buildable %v", c.name, !c.buildableNot)
			continue
		}
		if c.buildableNot {
			continue
		}
		m := revolveMesh(&c.sk, 0, c.sweep, 96, 0)
		v := solidOK(t, c.name, m)
		if !skNear(v, c.vol, c.tol) {
			t.Errorf("%s: volume %v, want %v", c.name, v, c.vol)
		}
	}
	// the built shape fills its size
	v := Values{Num: map[string]float64{"sides": 32, "start": 30, "sweep": 200}, Sketch: &Sketch{Paths: []SketchPath{sq(-10, 0, 0, 10)}}}
	m := Build("revolve", geom.V(20, 15, 10), v)
	watertight(t, "built", m)
	if s := m.Bounds().Size(); !skNear(s.X, 20, 1e-9) || !skNear(s.Y, 15, 1e-9) || !skNear(s.Z, 10, 1e-9) {
		t.Errorf("size %v", s)
	}
}

func TestSketchDefaults(t *testing.T) {
	for _, id := range []string{"sketch", "revolve"} {
		k, _ := Lookup(id)
		m := Build(id, k.Size, k.Defaults())
		solidOK(t, id, m)
		if !Buildable(id, k.Defaults().Sketch) {
			t.Errorf("%s: default not buildable", id)
		}
	}
}

func TestCheckSketch(t *testing.T) {
	bad := []Sketch{
		{Paths: []SketchPath{{}}},
		{Paths: []SketchPath{{Points: skPts(0, 0, math.NaN(), 1)}}},
		{Paths: []SketchPath{{Points: skPts(0, 0, 1e9, 1)}}},
		{Paths: []SketchPath{{Points: skPts(0, 0, 1, 1), Stroke: -1}}},
		{Paths: []SketchPath{{Points: skPts(0, 0, 1, 1), Join: 7}}},
		{Paths: []SketchPath{{Points: []SketchPoint{{Kind: 9}}}}},
	}
	for i, sk := range bad {
		if CheckSketch(&sk) == nil {
			t.Errorf("case %d passed", i)
		}
		if len(SketchSolid(&sk, 0, 1).Tris) != 0 {
			t.Errorf("case %d built", i)
		}
	}
	if CheckSketch(&Sketch{}) != nil {
		t.Error("an empty sketch is refused")
	}
	if Buildable("sketch", &Sketch{}) || Buildable("sketch", &Sketch{Paths: []SketchPath{{Points: skPts(0, 0)}}}) {
		t.Error("nothing is buildable")
	}
}

// Random drawings (areas, lines with every corner and end, curves, paths
// crossing themselves and each other) always build closed solids, quickly.
func TestSketchRandom(t *testing.T) {
	r := rand.New(rand.NewSource(sketchSeed))
	start := time.Now()
	for i := 0; i < int(sketchCases); i++ {
		sk := &Sketch{}
		for p := 0; p < 1+r.Intn(3); p++ {
			path := SketchPath{Closed: r.Intn(2) == 0, Join: r.Intn(3), Cap: r.Intn(3)}
			if r.Intn(3) == 0 {
				path.Stroke = 0.5 + r.Float64()*4
				path.Align = r.Intn(3)
			}
			n := 2 + r.Intn(7)
			for k := 0; k < n; k++ {
				q := SketchPoint{P: [2]float64{math.Round(r.Float64()*40*4) / 4, math.Round(r.Float64()*40*4) / 4}, Kind: r.Intn(3)}
				if r.Intn(3) == 0 {
					q.In = [2]float64{r.Float64()*20 - 10, r.Float64()*20 - 10}
					q.Out = [2]float64{r.Float64()*20 - 10, r.Float64()*20 - 10}
				}
				path.Points = append(path.Points, q)
			}
			sk.Paths = append(sk.Paths, path)
		}
		m := SketchSolid(sk, 0, 3)
		if len(m.Tris) > 0 {
			if o, nm := m.EdgeCheck(1e-7); o != 0 || nm != 0 {
				t.Errorf("case %d: %d open, %d non-manifold edges: %+v", i, o, nm, sk)
			}
		}
		if i%4 == 0 && len(m.Tris) > 0 {
			// tapered and twisted
			e := Extrude{Quality: 1 + r.Intn(16), TopScale: float64(r.Intn(3) * 60), BaseScale: float64(40 + r.Intn(3)*60), Twist: r.Float64()*180 - 90,
				TwistSteps: r.Intn(4), Smooth: i%8 == 0}
			lm := SketchExtrude(sk, 0, 3, e)
			if o, nm := lm.EdgeCheck(1e-7); o != 0 || nm != 0 {
				t.Errorf("case %d lofted %+v: %d open, %d non-manifold edges", i, e, o, nm)
			}
		}
		if Buildable("revolve", sk) {
			// a full turn, and part of one from somewhere else
			for _, sweep := range []float64{360, 30 + r.Float64()*300} {
				rv := revolveMesh(sk, r.Float64()*360, sweep, 3+r.Intn(30), r.Intn(8))
				if rv == nil {
					continue // nothing of it at this quality (sketchModel raises it)
				}
				if o, nm := rv.EdgeCheck(1e-7); o != 0 || nm != 0 || rv.Oriented().Volume() <= 0 {
					t.Errorf("case %d revolved %v°: %d open, %d non-manifold edges: %+v", i, sweep, o, nm, sk)
				}
			}
		}
		if Buildable("coil", sk) && i%3 == 0 {
			cm := coilMesh(sk, 0.25+r.Float64()*3, r.Float64()*30, r.Float64()*360, 3+r.Intn(30), r.Intn(8))
			if cm == nil {
				continue
			}
			if o, nm := cm.EdgeCheck(1e-7); o != 0 || nm != 0 || cm.Oriented().Volume() <= 0 {
				t.Errorf("case %d coiled: %d open, %d non-manifold edges: %+v", i, o, nm, sk)
			}
		}
	}
	if d := time.Since(start); d > 60*time.Second {
		t.Errorf("took %v", d)
	}
}

var sketchSeed, sketchCases int64 = 7, 300

// A wide stroke round a closed curve crossing itself: at Quality 64 two
// cuts that are one point land 1.9e-6 mm apart, and the loop through them
// must still close, or the stroke's outer outline is dropped, leaving only
// what it goes round. Each Quality must give the same band. (Regression:
// FuzzSketch, through a coil.)
func TestSketchStrokeKeepsItsOuterOutline(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{{Closed: true, Stroke: 5.55, Join: JoinRound, Cap: CapRound, Align: AlignOutside, Points: []SketchPoint{
		{P: [2]float64{-17, 33.75}, In: [2]float64{-1.125, -6.875}, Out: [2]float64{1.125, 6.875}, Kind: PointSmooth},
		{P: [2]float64{-12.75, -23.5}, In: [2]float64{-10, 0.875}, Out: [2]float64{-0.875, -10}},
	}}}}
	for _, q := range []int{12, 40, 63, 64} {
		cs := SketchOutlinesN(sk, q)
		area := 0.0
		for _, c := range cs {
			area += SignedArea(c)
		}
		if len(cs) != 2 || area < 755 || area > 762 {
			t.Errorf("quality %d: %d outlines of %.2f mm², want the band: 2 of about 759", q, len(cs), area)
		}
		m := coilMesh(sk, 0.25, 0.1, 24, 113, q)
		if o, nm := m.EdgeCheck(1e-7); o != 0 || nm != 0 {
			t.Errorf("quality %d: coil has %d open, %d non-manifold edges", q, o, nm)
		}
	}
}

// A revolve through 1° at Quality 64 has walls so thin that Clean drops
// some as slivers; the revolve must repair what that opens, as an
// extrusion and a coil do. (Regression: FuzzSketch.)
func TestRevolveThinSweepStaysClosed(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{{Points: []SketchPoint{
		{P: [2]float64{27.25, -24.5}, In: [2]float64{10, -12}, Out: [2]float64{-10, 12}, Kind: PointSmooth},
		{P: [2]float64{-12.5, -0.75}, In: [2]float64{-3, 1.5}, Out: [2]float64{3, -1.5}, Kind: PointSmooth},
		{P: [2]float64{29, -5}},
		{P: [2]float64{12.25, 9.5}, Kind: PointSmooth},
		{P: [2]float64{25.25, -13.5}, Kind: PointAuto},
		{P: [2]float64{-28.75, -4.75}},
	}}}}
	for _, sweep := range []float64{1, 30, 360} {
		m := revolveMesh(sk, 5.333, sweep, 3, 64)
		if o, nm := m.EdgeCheck(1e-7); o != 0 || nm != 0 {
			t.Errorf("sweep %v: %d open, %d non-manifold edges", sweep, o, nm)
		}
	}
}

// A hole that touches its outline at a corner (as a stroke's inside edge
// can, meeting the path at a sharp turn) is cut out of the triangles: the
// bridge to it has no length, and spliced in as one it would leave four
// copies of the point and the hole covered (a coil's caps would then miss
// its walls). (Regression: FuzzSketch seed e7cbd255.)
func TestTriangulateHoleTouchingTheOutline(t *testing.T) {
	outer := []Pt{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	for _, hole := range [][]Pt{
		{{10, 5}, {6, 3}, {6, 7}},  // touching the right side at a corner put there
		{{10, 10}, {8, 6}, {6, 8}}, // touching the top right corner
	} {
		r := Region{Outer: outer, Holes: [][]Pt{hole}}
		if hole[0] == (Pt{10, 5}) {
			r.Outer = []Pt{{0, 0}, {10, 0}, {10, 5}, {10, 10}, {0, 10}} // the point is a corner of the outline too
		}
		want := math.Abs(SignedArea(r.Outer)) - math.Abs(SignedArea(hole))
		got := 0.0
		for _, tr := range Triangulate(r) {
			got += math.Abs(SignedArea([]Pt{tr[0], tr[1], tr[2]}))
		}
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("hole %v: triangles cover %v, the region is %v", hole, got, want)
		}
	}
	// two holes touching each other at a corner (bridged from its own
	// rightmost corner, the second would pinch the polygon at the shared
	// one too; regression: FuzzSketch seed 7ab1f0d8); clockwise, as holes
	// are
	r := Region{Outer: outer, Holes: [][]Pt{
		{{5, 5}, {8, 7}, {8, 3}},
		{{5, 5}, {2, 3}, {2, 7}},
	}}
	got := 0.0
	for _, tr := range Triangulate(r) {
		got += math.Abs(SignedArea([]Pt{tr[0], tr[1], tr[2]}))
	}
	if want := 100 - 6 - 6.0; math.Abs(got-want) > 1e-9 {
		t.Errorf("two touching holes: triangles cover %v, the region is %v", got, want)
	}
}

// A hole whose rightmost corner is a rounding hair off the height of an
// outline corner ahead of it bridges to that corner, not through it along
// the side beyond, which would give 24 mm² too much cap. The drawing is
// FuzzSketch seed 82245286's, read from its seed file (no smaller drawing
// shows it).
func TestBridgeRayWithinRoundingOfACorner(t *testing.T) {
	raw, err := os.ReadFile("testdata/fuzz/FuzzSketch/82245286e1a6fc5d")
	if err != nil {
		t.Fatal(err)
	}
	s, err := strconv.Unquote(strings.TrimSuffix(strings.TrimPrefix(strings.Split(string(raw), "\n")[1], "[]byte("), ")"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range Regions(SketchOutlinesN(fzSketch([]byte(s)), 21)) {
		want := math.Abs(SignedArea(r.Outer))
		for _, h := range r.Holes {
			want -= math.Abs(SignedArea(h))
		}
		got := 0.0
		for _, tr := range earClip(bridge(r)) {
			got += math.Abs(SignedArea([]Pt{tr[0], tr[1], tr[2]}))
		}
		if math.Abs(got-want) > 1e-9*math.Max(1, want) {
			t.Errorf("ear clipping covers %v, the region is %v", got, want)
		}
	}
}

// An outline whose corners all lie on another's sides nests by a point
// off them: asked at a corner on the outline, a hole in a hole (an island
// of solid) could pass for a plain hole. (Regression: FuzzSketch seed
// 81f81b8c.)
func TestRegionsNestByAPointOffTheSides(t *testing.T) {
	square := []Pt{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	diamond := []Pt{{5, 0}, {10, 5}, {5, 10}, {0, 5}} // its corners on the square's sides
	rs := Regions([][]Pt{square, diamond})
	if len(rs) != 1 || len(rs[0].Holes) != 1 {
		t.Fatalf("%d regions, holes %v", len(rs), rs)
	}
	area := 0.0
	for _, tr := range Triangulate(rs[0]) {
		area += math.Abs(SignedArea([]Pt{tr[0], tr[1], tr[2]}))
	}
	if math.Abs(area-50) > 1e-9 {
		t.Errorf("area %v, want 50", area)
	}
}

// The slab triangulation (the fallback when ear clipping can't cover a
// pinched region) covers exactly the region, even-odd, whatever touches.
func TestSlabTriangulate(t *testing.T) {
	square := []Pt{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	for _, c := range []struct {
		r    Region
		want float64
	}{
		{Region{Outer: square}, 100},
		{Region{Outer: square, Holes: [][]Pt{{{5, 0}, {0, 5}, {5, 10}, {10, 5}}}}, 50}, // a hole touching at four corners
		{Region{Outer: square, Holes: [][]Pt{{{10, 10}, {8, 6}, {6, 8}}, {{5, 5}, {2, 3}, {2, 7}}}}, 100 - 6 - 6},
	} {
		got := 0.0
		for _, tr := range slabTriangulate(c.r) {
			if cross2(tr[0], tr[1], tr[2]) <= 0 {
				t.Errorf("a triangle runs clockwise: %v", tr)
			}
			got += math.Abs(SignedArea([]Pt{tr[0], tr[1], tr[2]}))
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%v: slabs cover %v, want %v", c.r.Holes, got, c.want)
		}
	}
}

// A thick line crossing itself leaves two holes in its outline, and the
// second hole's bridge to the outline runs exactly through one of its
// vertices (to within 2e-16). The bridge must end there: run on past, it
// lets the ear clipper cut a triangle outside the outline, and a coil of
// it has open ends. (Regression: FuzzSketch.)
func TestSketchBridgeThroughAVertex(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{{Stroke: 2.55, Cap: CapSquare, Align: AlignInside, Points: []SketchPoint{
		{P: [2]float64{23, 5.75}, Kind: PointAuto},
		{P: [2]float64{3, -10.75}, In: [2]float64{-4.125, -0.375}, Out: [2]float64{0.375, -4.125}, Kind: PointSmooth},
		{P: [2]float64{29.75, 35}, Kind: PointAuto},
		{P: [2]float64{-20.25, 5}},
		{P: [2]float64{-20, 37.75}},
		{P: [2]float64{-15.5, -3}},
		{P: [2]float64{-20, 5}},
		{P: [2]float64{-20, -13}},
	}}}}
	for _, q := range []int{1, 12} {
		for _, r := range coilProfile(sk, q) {
			want := SignedArea(r.Outer)
			for _, h := range r.Holes {
				want += SignedArea(h)
			}
			got := 0.0
			for _, tr := range Triangulate(r) {
				got += SignedArea(tr[:])
			}
			if math.Abs(got-want) > 1e-6*want {
				t.Errorf("quality %d: the triangles cover %.4f mm², the region %.4f", q, got, want)
			}
		}
		m := coilMesh(sk, 1, 0.333, 60, 3, q)
		if o, nm := m.EdgeCheck(1e-7); o != 0 || nm != 0 {
			t.Errorf("quality %d: coil has %d open, %d non-manifold edges", q, o, nm)
		}
	}
}

// A curve that crosses itself makes two lobes touching at a point, and at
// Quality 64 the big lobe's first point is that point. Its nesting must not
// be read there, where it can come out a hole in the small lobe and give a
// part-turn revolve open ends. (Regression: FuzzSketch.)
func TestSketchFigureEightLobes(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{{Closed: true, Points: []SketchPoint{
		{P: [2]float64{-7.75, -13}},
		{P: [2]float64{-31.75, -24.25}, Kind: PointAuto},
		{P: [2]float64{-32, -25}, In: [2]float64{1, 16}, Out: [2]float64{-1, -16}, Kind: PointSmooth},
	}}}}
	for _, q := range []int{12, 64} {
		regs := revolveProfile(sk, q)
		area := 0.0
		for _, r := range regs {
			if len(r.Holes) != 0 {
				t.Errorf("quality %d: a lobe has %d holes", q, len(r.Holes))
			}
			area += SignedArea(r.Outer)
		}
		if len(regs) != 2 || area < 130 || area > 136 {
			t.Errorf("quality %d: %d regions of %.2f mm², want 2 lobes of about 133", q, len(regs), area)
		}
		for _, sweep := range []float64{225, 360} {
			m := revolveMesh(sk, 1, sweep, 3, q)
			if o, nm := m.EdgeCheck(1e-7); o != 0 || nm != 0 {
				t.Errorf("quality %d, sweep %v: %d open, %d non-manifold edges", q, sweep, o, nm)
			}
		}
	}
}

// A line with round corners and ends covers every point within half its
// width of its centre line, however tightly it bends or crosses itself:
// compared by area against points sampled on a grid.
func TestSketchLineCoversItsWidth(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 25; i++ {
		path := SketchPath{Stroke: 1 + r.Float64()*5, Join: JoinRound, Cap: CapRound}
		for k := 0; k < 3+r.Intn(4); k++ {
			q := SketchPoint{P: [2]float64{r.Float64() * 30, r.Float64() * 30}, Kind: r.Intn(3)}
			path.Points = append(path.Points, q)
		}
		sk := &Sketch{Paths: []SketchPath{path}}
		area := 0.0
		for _, c := range SketchOutlines(sk) {
			area += SignedArea(c) // holes count negative
		}
		line := path.Flatten()
		h := path.Width() / 2
		distSq := func(p Pt) float64 {
			best := math.Inf(1)
			for k := 0; k+1 < len(line); k++ {
				a, b := line[k], line[k+1]
				d := ptSub(b, a)
				t := math.Max(0, math.Min(1, ptDot(ptSub(p, a), d)/ptDot(d, d)))
				f := ptSub(p, ptAdd(a, ptScale(d, t)))
				best = math.Min(best, ptDot(f, f))
			}
			return best
		}
		const step = 0.25
		in := 0
		for x := -h - 1; x < 30+h+1; x += step {
			for y := -h - 1; y < 30+h+1; y += step {
				if distSq(Pt{x, y}) <= h*h {
					in++
				}
			}
		}
		want := float64(in) * step * step
		if math.Abs(area-want) > 0.02*want+2 {
			t.Errorf("case %d: area %v, want about %v", i, area, want)
		}
	}
}

// A closed line wider than the shape it goes round (a narrow slot, a small
// loop, drawn thick) is solid: its inside edge, turned inside out, isn't a
// hole in it.
func TestSketchClosedLineWiderThanItsShape(t *testing.T) {
	rect := func(x0, y0, x1, y1 float64) SketchPath {
		return SketchPath{Closed: true, Points: []SketchPoint{
			{P: [2]float64{x0, y0}}, {P: [2]float64{x1, y0}}, {P: [2]float64{x1, y1}}, {P: [2]float64{x0, y1}}}}
	}
	circle := func(r float64) SketchPath {
		var pts []SketchPoint
		for i := 0; i < 4; i++ {
			a := float64(i) * math.Pi / 2
			pts = append(pts, SketchPoint{P: [2]float64{r * math.Cos(a), r * math.Sin(a)}, Kind: PointAuto})
		}
		return SketchPath{Closed: true, Points: pts}
	}
	for _, c := range []struct {
		name        string
		p           SketchPath
		stroke      float64
		join, align int
		want        float64
	}{
		{"slot", rect(-1, -10, 1, 10), 4, JoinSharp, AlignCenter, 6 * 24},
		{"square", rect(-1, -1, 1, 1), 10, JoinSharp, AlignCenter, 12 * 12},
		{"square round", rect(-1, -1, 1, 1), 10, JoinRound, AlignCenter, 4 + 8*5 + math.Pi*25},
		{"square inside", rect(-1, -1, 1, 1), 10, JoinSharp, AlignInside, 4},
		{"square outside", rect(-1, -1, 1, 1), 10, JoinSharp, AlignOutside, 22*22 - 4},
		{"circle", circle(1), 3, JoinRound, AlignCenter, math.Pi * 2.5 * 2.5},
		{"circle inside", circle(1), 10, JoinRound, AlignInside, -1}, // the filled circle, as flattened
		// and one that fits keeps its hole
		{"frame", rect(-10, -10, 10, 10), 4, JoinSharp, AlignCenter, 24*24 - 16*16},
	} {
		p := c.p
		p.Stroke, p.Join, p.Align = c.stroke, c.join, c.align
		area := 0.0
		for _, o := range SketchOutlines(&Sketch{Paths: []SketchPath{p}}) {
			area += SignedArea(o)
		}
		want := c.want
		if want < 0 {
			want = math.Abs(SignedArea(p.Flatten()))
		}
		if math.Abs(area-want) > 0.02*want {
			t.Errorf("%s: area %.2f, want about %.2f", c.name, area, want)
		}
	}
}

// A path that goes round a place twice fills it (the nonzero rule): a
// five-pointed star drawn in one stroke is solid, its middle
// pentagon included, and so is a square gone round twice.
func TestSketchPathRoundTwiceFills(t *testing.T) {
	star := SketchPath{Closed: true, Points: skPts(0, 10, 6, -8, -10, 3, 10, 3, -6, -8)}
	m := SketchSolid(&Sketch{Paths: []SketchPath{star}}, 0, 1)
	// the star's outline: its five points and the five corners between
	// them, where the lines cross
	var outer []Pt
	tips := []Pt{{0, 10}, {6, -8}, {-10, 3}, {10, 3}, {-6, -8}}
	cross := func(a, b, c, d Pt) Pt {
		t := ((c.X-a.X)*(d.Y-c.Y) - (c.Y-a.Y)*(d.X-c.X)) / ((b.X-a.X)*(d.Y-c.Y) - (b.Y-a.Y)*(d.X-c.X))
		return Pt{a.X + t*(b.X-a.X), a.Y + t*(b.Y-a.Y)}
	}
	// the drawing's edges in order; each tip's two neighbors on the outline
	// are where its edges cross the edges next to the ones opposite
	e := func(i int) (Pt, Pt) { return tips[i%5], tips[(i+1)%5] }
	for i := 0; i < 5; i++ {
		a, b := e(i)
		c, d := e(i + 2)
		outer = append(outer, cross(a, b, c, d))
	}
	// (the inner pentagon's corners alone; the full outline adds the tips)
	inner := math.Abs(SignedArea(outer))
	full := 0.0
	for _, o := range SketchOutlines(&Sketch{Paths: []SketchPath{star}}) {
		full += SignedArea(o)
	}
	if v := solidOK(t, "star", m); !skNear(v, full, 1e-6) || full < inner+50 {
		t.Errorf("star: volume %v, outline area %v, middle %v", v, full, inner)
	}
	if n := len(SketchOutlines(&Sketch{Paths: []SketchPath{star}})); n != 1 {
		t.Errorf("star: %d outlines, want 1 (no hole in the middle)", n)
	}
	twice := SketchPath{Closed: true, Points: skPts(0, 0, 10, 0, 10, 10, 0, 10, 0, 0, 10, 0, 10, 10, 0, 10)}
	if v := solidOK(t, "square twice", SketchSolid(&Sketch{Paths: []SketchPath{twice}}, 0, 1)); !skNear(v, 100, 1e-6) {
		t.Errorf("square gone round twice: volume %v, want 100", v)
	}
}

// windingAt is the generalized winding number of a closed mesh at q: 1
// inside, 0 outside, 2 where it covers a place twice.
func windingAt(m *geom.Mesh, q geom.Vec3) float64 {
	w := 0.0
	for _, t := range m.Tris {
		a, b, c := t.P[0].Sub(q), t.P[1].Sub(q), t.P[2].Sub(q)
		la, lb, lc := a.Len(), b.Len(), c.Len()
		w += 2 * math.Atan2(a.Dot(b.Cross(c)), la*lb*lc+a.Dot(b)*lc+b.Dot(c)*la+c.Dot(a)*lb)
	}
	return w / (4 * math.Pi)
}

// A coil at its least pitch doesn't run into itself: the middle of the gap
// between one turn and the next is outside the mesh everywhere, whatever
// the turns and sides (facets that differ from turn to turn can be off by
// up to a step's rise, far more than the gap).
func TestCoilAtLeastPitchKeepsItsTurnsApart(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{{Closed: true, Points: []SketchPoint{
		{P: [2]float64{-15, 0}}, {P: [2]float64{-5, 0}}, {P: [2]float64{-5, 10}}, {P: [2]float64{-15, 10}}}}}}
	pitch := CoilMinPitch(10)
	gap := pitch - 10
	r := rand.New(rand.NewSource(1))
	for _, c := range []struct{ turns, sides float64 }{{3, 3}, {2.5, 3}, {2.25, 4}, {1.5, 5}, {2.5, 48}, {3, 48}, {1.25, 7}, {3, 128}} {
		m := sketchModel("coil", catalogue["coil"].clamp(Values{Num: map[string]float64{"turns": c.turns, "sides": c.sides, "pitch": 0.1}, Sketch: sk}))
		if open, nm := m.EdgeCheck(1e-7); open+nm > 0 {
			t.Errorf("%v: %d open, %d non-manifold edges", c, open, nm)
		}
		// Between steps the facets are off the true helix by more than the
		// gap, so the place to look is a step's rise either side of it: no
		// point there may be inside two turns at once.
		rise := pitch / float64(max(3, int(c.sides)))
		worst, inside := 0.0, math.Inf(1)
		for i := 0; i < 1500; i++ {
			tt := r.Float64() * (c.turns - 1) // a place with a turn above it
			a := (90 + 360*tt) * math.Pi / 180
			rho := 5.2 + r.Float64()*9.6
			q := geom.V(rho*math.Cos(a), rho*math.Sin(a), 10+pitch*tt+gap/2+(2*r.Float64()-1)*rise)
			worst = math.Max(worst, windingAt(m, q))
			if c.sides >= 48 && i < 300 { // (few sides cut the corners off the turns)
				rho = 6 + r.Float64()*8
				inside = math.Min(inside, windingAt(m, geom.V(rho*math.Cos(a), rho*math.Sin(a), 5+pitch*tt)))
			}
		}
		if worst > 1.5 {
			t.Errorf("turns %v, sides %v: turns run into each other (winding %.2f)", c.turns, c.sides, worst)
		}
		if c.sides >= 48 && inside < 0.5 {
			t.Errorf("turns %v, sides %v: the middle of a turn is outside the mesh (winding %.2f)", c.turns, c.sides, inside)
		}
	}
}
