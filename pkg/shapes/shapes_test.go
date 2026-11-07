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

// The trapezoid: base width at the front (-Y), top width at the back, its
// natural size from its settings; either may be wider, or 0 (a triangle).
func TestTrapezoid(t *testing.T) {
	k, _ := Lookup("trapezoid")
	for _, c := range []struct{ base, top float64 }{{51, 115}, {20, 10}, {30, 0}, {0, 12}} {
		v := k.Defaults()
		v.Num["base"], v.Num["top"], v.Num["height"], v.Num["thickness"] = c.base, c.top, 50, 30
		s, ok := k.NaturalSize(v)
		if !ok || s != geom.V(math.Max(c.base, c.top), 50, 30) {
			t.Fatalf("%v: natural size %v", c, s)
		}
		m := Build("trapezoid", s, v)
		if open, nm := m.EdgeCheck(0); open != 0 || nm != 0 {
			t.Fatalf("%v: %d open, %d non-manifold edges", c, open, nm)
		}
		if got, want := m.Volume(), (c.base+c.top)/2*50*30; math.Abs(got-want) > 1e-6*want {
			t.Fatalf("%v: volume %v, want %v", c, got, want)
		}
		// the widths at the front and back faces
		front, back := 0.0, 0.0
		for _, tr := range m.Tris {
			for _, p := range tr.P {
				if p.Y < -25+1e-9 {
					front = math.Max(front, 2*math.Abs(p.X))
				}
				if p.Y > 25-1e-9 {
					back = math.Max(back, 2*math.Abs(p.X))
				}
			}
		}
		if math.Abs(front-c.base) > 1e-6 || math.Abs(back-c.top) > 1e-6 {
			t.Fatalf("%v: %v wide at the front, %v at the back", c, front, back)
		}
	}
}

// The tapered box: closed, its volume a frustum's (h/6 × (base + top + 4 ×
// the middle section)), and still closed as a pyramid (a top of no size).
func TestTaperedBox(t *testing.T) {
	k, _ := Lookup("taperedbox")
	for _, c := range []struct{ bw, bl, tw, tl float64 }{{80, 80, 40, 40}, {30, 10, 10, 30}, {20, 20, 0, 0}} {
		v := k.Defaults()
		v.Num["bw"], v.Num["bl"], v.Num["tw"], v.Num["tl"], v.Num["height"] = c.bw, c.bl, c.tw, c.tl, 22
		s, _ := k.NaturalSize(v)
		m := Build("taperedbox", s, v)
		if open, nm := m.EdgeCheck(0); open != 0 || nm != 0 {
			t.Fatalf("%v: %d open, %d non-manifold edges", c, open, nm)
		}
		mid := (c.bw + c.tw) / 2 * (c.bl + c.tl) / 2
		if got, want := m.Volume(), 22.0/6*(c.bw*c.bl+c.tw*c.tl+4*mid); math.Abs(got-want) > 1e-6*want {
			t.Fatalf("%v: volume %v, want %v", c, got, want)
		}
	}
}

// A big cone keeps its top radius: the slider's range must not clamp a
// 54 mm cone narrowing to 44.
func TestBigConeKeepsItsTop(t *testing.T) {
	m := Build("cone", geom.V(54, 54, 5), Values{Num: map[string]float64{"top": 22, "sides": 64}})
	top := 0.0
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			if math.Abs(p.Z-m.Bounds().Max.Z) < 1e-9 {
				top = math.Max(top, math.Hypot(p.X, p.Y))
			}
		}
	}
	if math.Abs(top-22) > 1e-6 {
		t.Fatalf("top radius %v, want 22", top)
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

// A value out of its slider's range (as a damaged file might hold) is
// clamped for the natural size as it is for the build, so the two agree
// and a huge value can't build a huge mesh.
func TestNaturalSizeClamps(t *testing.T) {
	k, _ := Lookup("spring")
	v := k.Defaults()
	v.Num["turns"] = 1e9
	done := make(chan geom.Vec3, 1)
	go func() { s, _ := k.NaturalSize(v); done <- s }()
	select {
	case s := <-done:
		v.Num["turns"] = 30 // the slider's maximum
		if want, _ := k.NaturalSize(v); s.Sub(want).Len() > 1e-9 {
			t.Errorf("natural size %v, want %v", s, want)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("NaturalSize built the unclamped value")
	}
}
