package shapes

import (
	"math"
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

func TestAllShapesClosedAndSized(t *testing.T) {
	for _, k := range All() {
		v := k.Defaults()
		m := Build(k.ID, k.Size, v)
		if len(m.Tris) == 0 {
			t.Errorf("%s: empty", k.ID)
			continue
		}
		s := m.Bounds().Size()
		if s.Sub(k.Size).Len() > 1e-6 {
			t.Errorf("%s: size %v want %v", k.ID, s, k.Size)
		}
		if m.Volume() <= 0 {
			t.Errorf("%s: volume %v", k.ID, m.Volume())
		}
		if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
			t.Errorf("%s: %d open, %d non-manifold edges", k.ID, o, n)
		}
	}
}

func TestRoundedBox(t *testing.T) {
	v := Values{Num: map[string]float64{"radius": 3, "steps": 6}}
	m := Build("box", geom.V(20, 20, 20), v)
	if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
		t.Fatalf("%d open %d nm", o, n)
	}
	if vol := m.Volume(); vol > 8000 || vol < 7500 {
		t.Fatalf("volume %v", vol)
	}
}

func TestTriangulateWithHole(t *testing.T) {
	outer := []Pt{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	hole := []Pt{{3, 3}, {7, 3}, {7, 7}, {3, 7}}
	m := ExtrudeContours([][]Pt{outer, hole}, 0, 1)
	if v := m.Volume(); math.Abs(v-84) > 1e-6 {
		t.Fatalf("volume %v want 84", v)
	}
	if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
		t.Fatalf("%d open %d nm", o, n)
	}
}

// Holes lined up in rows and columns: each hole's bridge must not run
// through another hole's vertices or reuse an earlier bridge's end.
func TestTriangulateAlignedHoles(t *testing.T) {
	contours := [][]Pt{roundedRect(54, 30, 2)}
	for _, c := range [][2]float64{{-24, -12}, {24, -12}, {24, 12}, {-24, 12}} {
		contours = append(contours, reversed(circlePts(c[0], c[1], 1.5, 20)))
	}
	m := ExtrudeContours(contours, 0, 2)
	if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
		t.Fatalf("%d open %d nm", o, n)
	}
}

// Many holes, all lined up in rows and columns (the worst case for
// bridging): ear clipping must give exactly V + 2H − 2 triangles covering
// exactly the region's area.
func TestTriangulateManyAlignedHoles(t *testing.T) {
	for _, staggered := range []bool{false, true} {
		contours := [][]Pt{{{-100, -60}, {100, -60}, {100, 60}, {-100, 60}}}
		for j := -10; j <= 10; j++ {
			off := 0.0
			if staggered && j%2 != 0 {
				off = 2.5
			}
			for i := -18; i <= 17; i++ {
				x, y := float64(i)*5+off, float64(j)*5
				contours = append(contours, reversed([]Pt{{x, y}, {x + 2, y}, {x + 2, y + 2}, {x, y + 2}}))
			}
		}
		regs := Regions(contours)
		if len(regs) != 1 {
			t.Fatalf("%d regions", len(regs))
		}
		r := regs[0]
		v := len(r.Outer)
		wantA := SignedArea(r.Outer)
		for _, h := range r.Holes {
			v += len(h)
			wantA += SignedArea(h)
		}
		tris := Triangulate(r)
		area := 0.0
		for _, tr := range tris {
			area += cross2(tr[0], tr[1], tr[2]) / 2
		}
		if want := v + 2*len(r.Holes) - 2; len(tris) != want || math.Abs(area-wantA) > 1e-6 {
			t.Fatalf("staggered=%v: %d holes: %d triangles (want %d), area %v (want %v)", staggered, len(r.Holes), len(tris), want, area, wantA)
		}
	}
}

// A large vent grille (hundreds of hexagonal holes) is watertight and quick.
func TestLargeVent(t *testing.T) {
	v := Values{Num: map[string]float64{"length": 120, "width": 80, "cell": 2, "wall": 0.5}}
	start := time.Now()
	m := Build("vent", geom.V(120, 80, 2), v)
	if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
		t.Fatalf("%d open, %d non-manifold edges", o, n)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %v", d)
	}
}

// Script letters overlap their neighbours: the text must still be one
// watertight solid, with no volume lost where they overlap.
func TestOverlappingGlyphs(t *testing.T) {
	for _, s := range []string{"ff", "fj", "Script text"} {
		m := TextMesh(s, "Script", 5)
		if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
			t.Errorf("%q: %d open, %d non-manifold edges", s, o, n)
		}
		// at least as much as the largest single letter; no more than all
		// of them apart
		sum, biggest := 0.0, 0.0
		for _, r := range s {
			v := TextMesh(string(r), "Script", 5).Volume()
			sum += v
			biggest = math.Max(biggest, v)
		}
		if v := m.Volume(); v < biggest || v > sum+1e-6 {
			t.Errorf("%q: volume %.1f, letters %.1f..%.1f", s, v, biggest, sum)
		}
	}
}

// A long, fine thread stays within its triangle budget and is still
// watertight.
func TestLongFineThreadIsBounded(t *testing.T) {
	for _, c := range []struct {
		z     float64
		sides int
	}{{300, 96}, {1000, 8}, {10000, 3}} {
		m := ThreadMesh(geom.V(10, 10, c.z), 0.25, c.sides, 1)
		if n := len(m.Tris); n > 520000 {
			t.Fatalf("%v: %d triangles", c, n)
		}
		if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
			t.Fatalf("%v: %d open, %d non-manifold edges", c, o, n)
		}
	}
	// the cap never adds sides: an 8-sided rod has 8 sides, long or not
	angles := map[float64]bool{}
	for _, tr := range ThreadMesh(geom.V(10, 10, 1000), 0.25, 8, 1).Tris {
		for _, p := range tr.P {
			if math.Hypot(p.X, p.Y) > 1e-9 {
				angles[math.Round(math.Atan2(p.Y, p.X)*1e6)/1e6] = true
			}
		}
	}
	if len(angles) != 8 {
		t.Fatalf("%d sides, want 8", len(angles))
	}
}

func TestText(t *testing.T) {
	m := TextMesh("Text 8", "Multilanguage", 10)
	if len(m.Tris) == 0 || m.Volume() <= 0 {
		t.Fatalf("text mesh empty or inverted: %d tris vol %v", len(m.Tris), m.Volume())
	}
	s := m.Bounds().Size()
	if s.Z != 10 || s.X < 50 {
		t.Fatalf("size %v", s)
	}
}
