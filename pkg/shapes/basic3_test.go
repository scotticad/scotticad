package shapes

import (
	"fmt"
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Tests for the rounded box, the crescent and the port opening.

// roundedBoxVolume is the volume a rounded box's surface encloses: the
// box inside the rounding swept by the corner's ellipsoid (Steiner's
// formula for a box and a convex body).
func roundedBoxVolume(s geom.Vec3, rc, rt, rb float64) float64 {
	a, b, c := s.X-2*rc, s.Y-2*rc, s.Z-rt-rb
	e := rt + rb
	return a*b*c + a*b*e + 2*rc*c*(a+b) + math.Pi*rc*rc*c + math.Pi*rc*e*(a+b)/2 + 2*math.Pi*rc*rc*e/3
}

func TestRoundedBoxShape(t *testing.T) {
	s := geom.V(40, 30, 20)
	for _, c := range []struct{ rc, rt, rb float64 }{
		{5, 0, 0}, {5, 3, 3}, {5, 8, 0}, {5, 0, 12}, {15, 10, 10}, // a round end: the corner is half the depth
		{0, 5, 5}, // no corner radius: square edges, whatever the edges' radii
		{4, 4, 4}, // one radius everywhere: the Box with Radius set
	} {
		v := vals("roundedbox", map[string]float64{"radius": c.rc, "top": c.rt, "bottom": c.rb, "steps": 64})
		name := fmt.Sprintf("rounded box %+v", c)
		m := Build("roundedbox", s, v)
		checkMesh(t, name, m, s)
		want := roundedBoxVolume(s, c.rc, c.rt, c.rb)
		if c.rc == 0 {
			want = s.X * s.Y * s.Z
		}
		if got := m.Volume(); !approx(got, want, 0.002*want) {
			t.Errorf("%s: volume %.1f, want %.1f", name, got, want)
		}
	}
	// the same rounding as the Box's (a sphere's eighth at each corner)
	v := vals("roundedbox", map[string]float64{"radius": 4, "top": 4, "bottom": 4, "steps": 10})
	a, b := Build("roundedbox", s, v).Volume(), Build("box", s, vals("box", map[string]float64{"radius": 4, "steps": 10})).Volume()
	if !approx(a, b, 1e-6*b) {
		t.Errorf("all radii 4: volume %.3f, the Box's %.3f", a, b)
	}
	// radii too large for the box are cut down to fit; the size is the size
	for _, big := range []map[string]float64{{"radius": 100}, {"top": 100, "bottom": 100}, {"radius": 100, "top": 100, "bottom": 100, "steps": 1}} {
		checkMesh(t, fmt.Sprintf("rounded box %v", big), Build("roundedbox", s, vals("roundedbox", big)), s)
	}
}

// Radii are millimetres: resizing the box doesn't stretch them.
func TestRoundedBoxKeepsItsRadii(t *testing.T) {
	v := vals("roundedbox", map[string]float64{"radius": 5, "steps": 64})
	for _, s := range []geom.Vec3{geom.V(40, 30, 20), geom.V(80, 30, 5)} {
		want := (s.X*s.Y - (4-math.Pi)*25) * s.Z
		if got := Build("roundedbox", s, v).Volume(); !approx(got, want, 0.001*want) {
			t.Errorf("%v: volume %.1f, want %.1f", s, got, want)
		}
	}
}

func TestCrescent(t *testing.T) {
	for _, c := range []struct {
		s          geom.Vec3
		thick, tip float64
		steps      float64
	}{
		{geom.V(40, 40, 10), 14, 0, 32},
		{geom.V(20, 60, 5), 3, 0, 32},  // deep: the tips are the outline's ends
		{geom.V(60, 20, 5), 50, 0, 64}, // thicker than 85% of the width: kept to it
		{geom.V(40, 40, 10), 14, 0, 33},
		{geom.V(40, 40, 10), 14, 0.5, 32},
		{geom.V(20, 60, 5), 3, 8, 64},
	} {
		name := fmt.Sprintf("crescent %v thickness %g tip %g steps %g", c.s, c.thick, c.tip, c.steps)
		v := vals("crescent", map[string]float64{"thickness": c.thick, "tip": c.tip, "steps": c.steps})
		checkMesh(t, name, Build("crescent", c.s, v), c.s)
		// the back's thickness, across the middle: exact where the outer edge
		// has a point there (an even number of them) and the tips aren't
		// rounded (fitting the rounded outline to the box stretches it)
		if int(math.Round(c.steps/2))%2 == 1 || c.tip > 0 {
			continue
		}
		out := crescentOutline(c.s.X, c.s.Y, v)
		var xs []float64
		for i := range out {
			p, q := out[i], out[(i+1)%len(out)]
			if (p.Y <= 0) != (q.Y <= 0) {
				xs = append(xs, p.X+(q.X-p.X)*(0-p.Y)/(q.Y-p.Y))
			}
		}
		thick := math.Min(c.thick, 0.85*c.s.X)
		if len(xs) != 2 || !approx(math.Abs(xs[0]-xs[1]), thick, 1e-6) || !approx(math.Min(xs[0], xs[1]), -c.s.X/2, 1e-6) {
			t.Errorf("%s: the middle is crossed at %v, want -%g and %g further", name, xs, c.s.X/2, thick)
		}
	}
}

// Every connector and every custom outline builds a closed opening of its
// size: Width × Depth × Height with the clearance round it, wider for a
// D-Sub's screw holes.
func TestPortOpenings(t *testing.T) {
	k, _ := Lookup("port")
	for i := 0; i <= len(portConnectors); i++ {
		for _, clear := range []float64{0, 0.5} {
			v := vals("port", map[string]float64{"connector": float64(i), "clearance": clear})
			w, h := v.Get("width", 0), v.Get("height", 0)
			name := "Custom"
			if i > 0 {
				c := portConnectors[i-1]
				w, h, name = c.width, c.height, c.name
				if c.holeDia > 0 {
					w = c.holeSpacing + c.holeDia
				}
			}
			want := geom.V(w+2*clear, 10, h+2*clear)
			got, _ := k.NaturalSize(v)
			if got.Sub(want).Len() > 1e-6 {
				t.Errorf("%s, clearance %g: size %v, want %v", name, clear, got, want)
			}
			checkMesh(t, name, Build("port", got, v), got)
		}
	}
	for o := range portOutlines {
		for _, r := range []float64{0, 1, 100} {
			v := vals("port", map[string]float64{"connector": 0, "outline": float64(o), "width": 16, "height": 8, "corner": r})
			s, _ := k.NaturalSize(v)
			name := fmt.Sprintf("%s corner %g", portOutlines[o], r)
			if s.Sub(geom.V(16, 10, 8)).Len() > 1e-6 {
				t.Errorf("%s: size %v", name, s)
			}
			checkMesh(t, name, Build("port", s, v), s)
		}
	}
}

// The openings' areas, seen from the front: a rounded rectangle, a USB-C's
// round ends, and an HDMI's cut lower corners; DisplayPort's one cut is on
// the left.
func TestPortOutlines(t *testing.T) {
	area := func(c []Pt) float64 {
		a := 0.0
		for i := range c {
			p, q := c[i], c[(i+1)%len(c)]
			a += p.X*q.Y - q.X*p.Y
		}
		return a / 2
	}
	near := func(name string, got, want float64) {
		t.Helper()
		if !approx(got, want, 1e-3*want) {
			t.Errorf("%s: area %.4f, want %.4f", name, got, want)
		}
	}
	near("rounded", area(portOutline(16, 8, 2, 0, 1024)), 16*8-(4-math.Pi)*4)
	near("round ends", area(portOutline(9.4, 3.7, 1.85, 0, 1024)), (9.4-3.7)*3.7+math.Pi*1.85*1.85)
	cx, cy := 2.45, 1.2 // HDMI's cuts at its own size
	near("HDMI", area(portOutline(15.7, 6.2, 0.3, 1, 1024)), 15.7*6.2-cx*cy-(4-math.Pi)*0.09/2)
	for _, p := range portOutline(17.1, 5.76, 0, 4, 32) {
		if p.Y < 1e-9 && p.X < -17.1/2+1.8-1e-9 {
			t.Errorf("DisplayPort: a point %v in the cut lower left corner", p)
		}
	}
	if ps := portOutline(17.1, 5.76, 0, 4, 32); !hasPoint(ps, Pt{17.1 / 2, 0}) {
		t.Errorf("DisplayPort: its lower right corner is cut too: %v", ps)
	}
	// a polygon outline's rounding keeps it to its box
	for o := 6; o <= 9; o++ {
		ps := portOutline(16, 8, 1, o, 64)
		lo, hi := Pt{math.Inf(1), math.Inf(1)}, Pt{math.Inf(-1), math.Inf(-1)}
		for _, p := range ps {
			lo, hi = Pt{math.Min(lo.X, p.X), math.Min(lo.Y, p.Y)}, Pt{math.Max(hi.X, p.X), math.Max(hi.Y, p.Y)}
		}
		if !approx(lo.X, -8, 1e-3) || !approx(hi.X, 8, 1e-3) || !approx(lo.Y, 0, 1e-3) || !approx(hi.Y, 8, 1e-3) || area(ps) <= 0 {
			t.Errorf("%s: bounds %v to %v, area %g", portOutlines[o], lo, hi, area(ps))
		}
	}
}

func hasPoint(ps []Pt, q Pt) bool {
	for _, p := range ps {
		if math.Hypot(p.X-q.X, p.Y-q.Y) < 1e-9 {
			return true
		}
	}
	return false
}

// The opening stands up: its outline across X and up Z, its depth along Y,
// and HDMI's cut corners at the bottom.
func TestPortStandsUp(t *testing.T) {
	m := portModel(vals("port", map[string]float64{"connector": 5})) // HDMI
	b := m.Bounds()
	if s := b.Size(); !approx(s.X, 15.7, 1e-9) || !approx(s.Y, 10, 1e-9) || !approx(s.Z, 6.2, 1e-9) {
		t.Fatalf("size %v", s)
	}
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			if p.Z < b.Min.Z+1e-9 && math.Abs(p.X) > 15.7/2-2.45+1e-9 {
				t.Fatalf("a point %v at the bottom outside the cuts", p)
			}
		}
	}
}

// The new kinds' settings that are missing (an older or hand-written file)
// build as their sliders' defaults.
func TestBasic3FallBackToSliderDefaults(t *testing.T) {
	for _, id := range []string{"roundedbox", "crescent"} {
		k, _ := Lookup(id)
		a := Build(id, k.Size, k.Defaults()).Volume()
		b := Build(id, k.Size, Values{Num: map[string]float64{}}).Volume()
		if !approx(a, b, 1e-9*a) {
			t.Errorf("%s: with no settings %g, with the defaults %g", id, b, a)
		}
	}
	k, _ := Lookup("port")
	want, _ := k.NaturalSize(k.Defaults())
	if got, _ := k.NaturalSize(Values{Num: map[string]float64{}}); got.Sub(want).Len() > 1e-9 {
		t.Errorf("port: with no settings %v, with the defaults %v", got, want)
	}
}

// A D-Sub's screw holes, grown by a large clearance into the opening, are
// joined to it: nowhere is the port covered twice.
func TestPortScrewHolesJoinTheOpening(t *testing.T) {
	for _, clear := range []float64{0, 0.4, 1, 2} {
		for _, sides := range []float64{8, 32, 256} {
			v := Values{Num: map[string]float64{"connector": 15, "clearance": clear, "sides": sides}}
			m := Build("port", geom.V(40, 10, 20), v)
			if open, nm := m.EdgeCheck(1e-7); open+nm > 0 {
				t.Errorf("clearance %v, sides %v: %d open, %d non-manifold edges", clear, sides, open, nm)
			}
			b := m.Bounds()
			worst := 0.0
			for i := 0; i < 400; i++ {
				for j := 0; j < 12; j++ {
					q := geom.V(b.Min.X+(b.Max.X-b.Min.X)*(float64(i)+0.5)/400, 0.37, b.Min.Z+(b.Max.Z-b.Min.Z)*(float64(j)+0.5)/12)
					worst = math.Max(worst, windingAt(m, q))
				}
			}
			if worst > 1.5 {
				t.Errorf("clearance %v, sides %v: covered twice (winding %.2f)", clear, sides, worst)
			}
		}
	}
}
