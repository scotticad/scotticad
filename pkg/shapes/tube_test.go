package shapes

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

func tubeValues(bs []Bend, d, wall, bend, sides float64) Values {
	return Values{Num: map[string]float64{"diameter": d, "wall": wall, "bend": bend, "sides": sides}, Bends: bs}
}

// polygonArea is the area of a regular polygon of n sides round radius r.
func polygonArea(r float64, n int) float64 {
	return 0.5 * float64(n) * r * r * math.Sin(2*math.Pi/float64(n))
}

func TestTubeClosedAndOutward(t *testing.T) {
	for _, tc := range []struct {
		name string
		bs   []Bend
		wall float64
	}{
		{"straight", []Bend{{Length: 30}}, 0},
		{"L", []Bend{{Length: 20, Angle: 90}, {Length: 20}}, 0},
		{"hollow L", []Bend{{Length: 20, Angle: 90}, {Length: 20}}, 1.5},
		{"bend only", []Bend{{Angle: 135}}, 0},
		{"negative and turned", []Bend{{Length: 10, Angle: -60}, {Length: 15, Angle: 45, Turn: 90}, {Length: 5}}, 2},
		{"U", []Bend{{Length: 30, Angle: 180}, {Length: 30}}, 0},
	} {
		m := tubeMesh(tubeValues(tc.bs, 10, tc.wall, 15, 32)).Clean()
		if open, nm := m.EdgeCheck(1e-7); open != 0 || nm != 0 {
			t.Errorf("%s: %d open and %d non-manifold edges", tc.name, open, nm)
		}
		if v := m.Volume(); v <= 0 {
			t.Errorf("%s: volume %v, want positive (faces outward as built)", tc.name, v)
		}
	}
}

// A bend radius no bigger than the tube's own radius is raised clear of
// it: at the tube's radius the inside of the bend is one point, and the
// surface written to a file (rounded to float32) has cracks there.
func TestTubeTightBendIsClosedAsWritten(t *testing.T) {
	// the library's tube made 50 mm across where it lands: its start stays
	// put, so its middle is here
	k, _ := Lookup("benttube")
	wide := k.Defaults()
	wide.Num["diameter"] = 50
	size, _ := k.NaturalSize(wide)
	m := Build("benttube", size, wide).Transform(geom.Translate(geom.V(15, -5, 5)))
	if s := geom.Surface(geom.AsWritten(m)); s.Open != 0 || s.NonManifold != 0 {
		t.Errorf("the default tube 50 mm across: %d open and %d non-manifold edges as written", s.Open, s.NonManifold)
	}
	for _, bend := range []float64{0.5, 15, 25, 25.05} {
		for _, wall := range []float64{0, 3} {
			v := tubeValues([]Bend{{Length: 20, Angle: 90}, {Length: 20, Angle: -180, Turn: 30}, {Length: 5}}, 50, wall, bend, 32)
			if _, _, r := TubeRadii(v); r < 25+tubeBendRoom {
				t.Errorf("bend %g: built at radius %g, inside the tube", bend, r)
			}
			// as placed in a design: off the origin, where rounding bites
			for _, at := range []geom.Vec3{{}, geom.V(0, 0, 25), geom.V(37.3, -12.7, 25)} {
				s := geom.Surface(geom.AsWritten(tubeMesh(v).Transform(geom.Translate(at))))
				if s.Open != 0 || s.NonManifold != 0 {
					t.Errorf("bend %g wall %g at %v: %d open and %d non-manifold edges as written", bend, wall, at, s.Open, s.NonManifold)
				}
			}
		}
	}
}

func TestTubeVolume(t *testing.T) {
	// a straight solid tube is a prism; a hollow one a prism less its bore
	n := 32
	m := tubeMesh(tubeValues([]Bend{{Length: 40}}, 10, 0, 15, float64(n)))
	if got, want := m.Volume(), polygonArea(5, n)*40; math.Abs(got-want) > 1e-6*want {
		t.Errorf("straight solid: volume %v, want %v", got, want)
	}
	m = tubeMesh(tubeValues([]Bend{{Length: 40}}, 10, 2, 15, float64(n)))
	if got, want := m.Volume(), (polygonArea(5, n)-polygonArea(3, n))*40; math.Abs(got-want) > 1e-6*want {
		t.Errorf("straight hollow: volume %v, want %v", got, want)
	}
	// a quarter bend sweeps the section round radius R: area x R x angle (Pappus),
	// near enough with 16 steps
	m = tubeMesh(tubeValues([]Bend{{Angle: 90}}, 10, 0, 20, float64(n)))
	if got, want := m.Volume(), polygonArea(5, n)*20*math.Pi/2; math.Abs(got-want) > 0.01*want {
		t.Errorf("quarter bend: volume %v, want about %v", got, want)
	}
}

func TestTubePath(t *testing.T) {
	// an L of 20, a 90-degree bend of radius 10 toward +Y, then 20
	path, ends := TubePath([]Bend{{Length: 20, Angle: 90}, {Length: 20}}, 10)
	end := path[len(path)-1]
	if d := end.P.Sub(geom.V(30, 30, 0)).Len(); d > 1e-9 {
		t.Errorf("L ends at %v, want (30, 30, 0)", end.P)
	}
	if d := end.T.Sub(geom.V(0, 1, 0)).Len(); d > 1e-9 {
		t.Errorf("L ends heading %v, want +Y", end.T)
	}
	if len(ends) != 2 || ends[1] != len(path)-1 {
		t.Errorf("row ends %v", ends)
	}
	// a turn of 90 makes the bend go up (+Z) instead of toward +Y
	path, _ = TubePath([]Bend{{Length: 10, Angle: 90, Turn: 90}, {Length: 10}}, 5)
	if p := path[len(path)-1].P; math.Abs(p.X-15) > 1e-9 || math.Abs(p.Y) > 1e-9 || math.Abs(p.Z-15) > 1e-9 {
		t.Errorf("turned bend ends at %v, want (15, 0, 15)", p)
	}
	// a negative bend goes toward -Y
	path, _ = TubePath([]Bend{{Angle: -90}}, 5)
	if p := path[len(path)-1].P; math.Abs(p.X-5) > 1e-9 || math.Abs(p.Y+5) > 1e-9 {
		t.Errorf("negative bend ends at %v, want (5, -5, 0)", p)
	}
	// a turn on a row with no bend carries on to the next bend, and an
	// empty row adds no station
	a, _ := TubePath([]Bend{{Length: 10, Turn: 90}, {}, {Angle: 90}}, 5)
	b, _ := TubePath([]Bend{{Length: 10, Angle: 90, Turn: 90}}, 5)
	if len(a) != len(b) {
		t.Errorf("%d stations, want %d", len(a), len(b))
	} else if d := a[len(a)-1].P.Sub(b[len(b)-1].P).Len(); d > 1e-9 {
		t.Errorf("turn on a straight row: ends at %v, want %v", a[len(a)-1].P, b[len(b)-1].P)
	}
	// two bends in one plane curl the same way: 90 + 90 is a U back
	// along -X, whose end is two bend radii across
	path, _ = TubePath([]Bend{{Angle: 90}, {Angle: 90}}, 5)
	if e := path[len(path)-1]; e.P.Sub(geom.V(0, 10, 0)).Len() > 1e-9 || e.T.Sub(geom.V(-1, 0, 0)).Len() > 1e-9 {
		t.Errorf("U ends at %v heading %v, want (0, 10, 0) heading -X", e.P, e.T)
	}
	// a negative bend then a positive one is an S: back to heading +X
	path, _ = TubePath([]Bend{{Angle: -90}, {Angle: 90}}, 5)
	if e := path[len(path)-1]; e.P.Sub(geom.V(10, -10, 0)).Len() > 1e-9 || e.T.Sub(geom.V(1, 0, 0)).Len() > 1e-9 {
		t.Errorf("S ends at %v heading %v, want (10, -10, 0) heading +X", e.P, e.T)
	}
	// the section's axes stay square to the tube all along
	path, _ = TubePath([]Bend{{Length: 10, Angle: 70}, {Length: 5, Angle: 50, Turn: 37}, {Length: 3, Angle: -120, Turn: -80}}, 8)
	for i, s := range path {
		if math.Abs(s.T.Dot(s.U)) > 1e-9 || math.Abs(s.T.Dot(s.W)) > 1e-9 || math.Abs(s.U.Dot(s.W)) > 1e-9 ||
			math.Abs(s.U.Cross(s.W).Dot(s.T)-1) > 1e-9 {
			t.Fatalf("station %d: frame not square: %+v", i, s)
		}
	}
}

func TestTubeBoundsAndFlat(t *testing.T) {
	// the default L lies on the workplane: its height is the tube's diameter
	k, _ := Lookup("benttube")
	s := k.Size
	if math.Abs(s.Z-10) > 1e-6 {
		t.Errorf("default tube is %v tall, want 10 (lying flat)", s.Z)
	}
	if s.X < 30 || s.Y < 30 {
		t.Errorf("default tube is %v, want an L about 35 x 35", s)
	}
	// a bend radius below the tube's radius is raised just clear of it
	_, _, r, _ := tubeDims(tubeValues(nil, 10, 0, 1, 32))
	if r != 5+tubeBendRoom {
		t.Errorf("bend radius %v, want %v (just clear of the tube's radius)", r, 5+tubeBendRoom)
	}
}

func TestCheckBends(t *testing.T) {
	for _, bad := range [][]Bend{
		nil,
		{{Length: -1}},
		{{Length: 10, Angle: 190}},
		{{Length: math.NaN()}},
		{{Length: 0, Angle: 0}},
		{{Length: 10, Turn: 400}},
	} {
		if CheckBends(bad) == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	if err := CheckBends(defaultBends); err != nil {
		t.Errorf("default table refused: %v", err)
	}
	if m := Build("benttube", geom.V(5, 5, 5), tubeValues([]Bend{{Length: -3}}, 10, 0, 15, 32)); m == nil || len(m.Tris) == 0 {
		t.Error("a bad table should still build a stand-in")
	}
}

// The longest table a tube can have, at its tightest bends and most sides,
// builds in reasonable time and is closed; one row more is refused.
func TestTubeLongestTable(t *testing.T) {
	bs := make([]Bend, maxBends)
	for i := range bs {
		bs[i] = Bend{Length: 5, Angle: 180, Turn: 90}
	}
	if err := CheckBends(bs); err != nil {
		t.Fatalf("a table of %d rows refused: %v", maxBends, err)
	}
	k, _ := Lookup("benttube")
	m := tubeMesh(k.clamp(tubeValues(bs, 10, 2, 0.5, 1e9)))
	if m == nil {
		t.Fatal("no tube")
	}
	if n := len(m.Tris); n < 500_000 || n > 2_000_000 {
		t.Errorf("%d triangles; expected about 1.7 million (200 rows of 33 rings of 64 sides, inside and out)", n)
	}
	if open, nm := m.EdgeCheck(0); open != 0 || nm != 0 {
		t.Errorf("%d open and %d non-manifold edges", open, nm)
	}
	if err := CheckBends(append(bs, Bend{Length: 1})); err == nil {
		t.Errorf("a table of %d rows accepted", maxBends+1)
	}
}

func TestTubeInLibrary(t *testing.T) {
	for i, id := range MechanicalKinds {
		if id == "benttube" {
			if i == 0 || MechanicalKinds[i-1] != "elbow" {
				t.Errorf("bent tube should follow the pipe elbow, got %v", MechanicalKinds)
			}
			return
		}
	}
	t.Error("bent tube not in the Mechanical library")
}
