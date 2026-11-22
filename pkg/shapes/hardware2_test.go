package shapes

import (
	"fmt"
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Tests for the settings added to the screw holes, the standoff, the
// spring and the teardrop, and for the finishing washer.

// part builds a generated kind at its natural size (millimetres) and
// checks it is closed and that size.
func part(t *testing.T, id string, num map[string]float64) (*geom.Mesh, geom.Vec3) {
	t.Helper()
	k, _ := Lookup(id)
	v := vals(id, num)
	s, _ := k.NaturalSize(v)
	m := Build(id, s, v)
	checkMesh(t, fmt.Sprintf("%s %v", id, num), m, s)
	return m, s
}

// ngonArea is the area of a regular n-gon of circumradius r, as revolve
// makes a circle.
func ngonArea(r float64, n int) float64 {
	return float64(n) / 2 * math.Sin(2*math.Pi/float64(n)) * r * r
}

// frustum is the volume a revolve of n sides makes of a cone frustum.
func frustum(r0, r1, h float64, n int) float64 {
	k := ngonArea(1, n)
	return k * h * (r0*r0 + r0*r1 + r1*r1) / 3
}

func TestCountersinkCustom(t *testing.T) {
	for _, c := range []struct{ shaft, head, angle, recess, depth float64 }{
		{3.4, 6.6, 90, 0, 10}, {4.5, 9, 82, 0.6, 12}, {3, 8, 120, 2, 5}, {3, 6, 30, 0, 4},
		{5, 4, 90, 0, 10}, // a head narrower than the shank: a plain hole
	} {
		m, s := part(t, "countersink", map[string]float64{"screw": 1, "shaft": c.shaft, "head": c.head, "angle": c.angle, "recess": c.recess, "depth": c.depth})
		rc := c.shaft / 2
		rk := math.Max(c.head/2, rc+0.05)
		cs := (rk - rc) / math.Tan(c.angle/2*math.Pi/180)
		depth := math.Max(c.depth, cs+c.recess+0.5)
		if !approx(s.X, 2*rk, 1e-9) || !approx(s.Z, depth+0.5, 1e-9) {
			t.Errorf("%+v: size %v, want %g across and %g tall", c, s, 2*rk, depth+0.5)
		}
		want := frustum(rc, rc, depth-c.recess-cs, 48) + frustum(rc, rk, cs, 48) + frustum(rk, rk, c.recess+0.5, 48)
		if got := m.Volume(); !approx(got, want, 1e-6*want) {
			t.Errorf("%+v: volume %.4f, want %.4f", c, got, want)
		}
	}
	// Recess sinks a metric head too: the same hole, the cone lower
	plain, _ := part(t, "countersink", map[string]float64{"size": 3, "depth": 10})
	deep, _ := part(t, "countersink", map[string]float64{"size": 3, "depth": 10, "recess": 0.6})
	rk := metricFor(3).cskD/2 + 0.2
	if got, want := deep.Volume()-plain.Volume(), ngonArea(rk, 48)*0.6-ngonArea(metricFor(3).d/2+0.2, 48)*0.6; !approx(got, want, 1e-6) {
		t.Errorf("recess 0.6 adds %.4f mm³, want %.4f", got, want)
	}
}

func TestCounterboreCustom(t *testing.T) {
	for _, c := range []struct{ shaft, head, hd, depth float64 }{
		{3.4, 6.4, 3.2, 12}, {5.5, 10, 5.5, 20}, {2, 3, 10, 4}, // a pocket deeper than the hole: the hole grows
		{6, 5, 3, 10}, // a head narrower than the shank
	} {
		m, s := part(t, "counterbore", map[string]float64{"screw": 1, "shaft": c.shaft, "head": c.head, "headdepth": c.hd, "depth": c.depth})
		rc := c.shaft / 2
		rb := math.Max(c.head/2, rc+0.05)
		depth := math.Max(c.depth, c.hd+0.5)
		if !approx(s.X, 2*rb, 1e-9) || !approx(s.Z, depth+0.5, 1e-9) {
			t.Errorf("%+v: size %v", c, s)
		}
		want := frustum(rc, rc, depth-c.hd, 48) + frustum(rb, rb, c.hd+0.5, 48)
		if got := m.Volume(); !approx(got, want, 1e-6*want) {
			t.Errorf("%+v: volume %.4f, want %.4f", c, got, want)
		}
	}
}

func TestSpringHand(t *testing.T) {
	right, rs := part(t, "spring", map[string]float64{"turns": 3.5})
	left, ls := part(t, "spring", map[string]float64{"turns": 3.5, "hand": 1})
	if !approx(rs.X, ls.X, 1e-9) || !approx(rs.Y, ls.Y, 1e-9) || !approx(rs.Z, ls.Z, 1e-9) {
		t.Errorf("sizes differ: right %v, left %v", rs, ls)
	}
	if a, b := right.Volume(), left.Volume(); !approx(a, b, 1e-9*a) || a <= 0 {
		t.Errorf("volumes: right %.4f, left %.4f", a, b)
	}
	// a right-hand coil climbs turning anticlockwise seen from above; a
	// left-hand one clockwise. Where the wire is at a height: the mean
	// direction of the points in a thin slice there (the coil as built,
	// on the axis)
	turn := func(hand float64) float64 {
		m := springModel(vals("spring", map[string]float64{"turns": 3.5, "diameter": 20, "wire": 2, "pitch": 4, "hand": hand}))
		at := func(z0 float64) geom.Vec3 {
			var d geom.Vec3
			for _, tr := range m.Tris {
				for _, p := range tr.P {
					if math.Abs(p.Z-z0) < 0.3 {
						d = d.Add(geom.V(p.X, p.Y, 0).Norm())
					}
				}
			}
			return d.Norm()
		}
		return at(6).Cross(at(7)).Z // a quarter turn higher
	}
	if r, l := turn(0), turn(1); !(r > 0.5 && l < -0.5) {
		t.Errorf("turning a quarter turn up: right %.3f, left %.3f; want right > 0 > left", r, l)
	}
}

func TestTeardropTipAngle(t *testing.T) {
	const d = 10
	for _, tip := range []float64{40, 60, 89, 90, 91, 120, 140} {
		for _, flat := range []float64{0, 1} {
			_, s := part(t, "teardrop", map[string]float64{"diameter": d, "length": 20, "tip": tip, "flat": flat})
			rise := d / 2 / math.Sin(tip/2*math.Pi/180)
			want := d/2 + rise
			if flat == 1 {
				want = d
			}
			if !approx(s.X, d, 1e-9) || !approx(s.Y, 20, 1e-9) || !approx(s.Z, want, 1e-9) {
				t.Errorf("tip %g flat %g: size %v, want %g × 20 × %g", tip, flat, s, float64(d), want)
			}
		}
	}
	// the sides meet the circle at a tangent: the outline is convex, and
	// the side's slope is the tip's half angle from upright
	for _, tip := range []float64{40, 70, 120, 140} {
		for _, k := range []int{1, 2, 6} {
			out := teardropOutline(5, tip, k, false)
			n := len(out)
			for i := range out {
				a, b, c := out[i], out[(i+1)%n], out[(i+2)%n]
				if (b.X-a.X)*(c.Y-b.Y)-(b.Y-a.Y)*(c.X-b.X) < -1e-9 {
					t.Errorf("tip %g k %d: not convex at %d", tip, k, i+1)
				}
			}
			p, q := out[n-2], out[n-1] // the right tangent point, the tip
			if got := math.Atan2(p.X-q.X, q.Y-p.Y) * 180 / math.Pi; !approx(got, tip/2, 1e-9) {
				t.Errorf("tip %g: side at %.4f° from upright", tip, got)
			}
		}
	}
}

func TestStandoffGussets(t *testing.T) {
	for _, c := range []map[string]float64{
		{"gussets": 1}, {"gussets": 3},
		{"gussets": 4, "gusseth": 8},  // to the top
		{"gussets": 6, "gussett": 10}, // too thick for six: thinned
		{"gussets": 4, "hole": 0}, {"gussets": 2, "flange": 0},
		{"gussets": 4, "flange": 30}, // under the flange
		{"gussets": 4, "height": 40, "gusseth": 20, "gussetl": 12},
	} {
		m, s := part(t, "standoff", c)
		without := map[string]float64{}
		for k, x := range c {
			without[k] = x
		}
		without["gussets"] = 0
		post, ps := part(t, "standoff", without)
		if m.Volume() <= post.Volume() {
			t.Errorf("%v: volume %.2f, no more than the plain post's %.2f", c, m.Volume(), post.Volume())
		}
		if !approx(s.Z, ps.Z, 1e-9) {
			t.Errorf("%v: %g tall, the post %g", c, s.Z, ps.Z)
		}
		// four ribs point along ±X and ±Y: they reach Gusset Length past the
		// post (or the flange is wider)
		if c["gussets"] == 4 {
			gl := 4.0
			if x, ok := c["gussetl"]; ok {
				gl = x
			}
			if want := math.Max(2*(3+gl), ps.X); !approx(s.X, want, 1e-6) || !approx(s.Y, want, 1e-6) {
				t.Errorf("%v: %g × %g across, want %g", c, s.X, s.Y, want)
			}
		}
	}
	// four ribs, the default size: each adds its part outside the post
	m, _ := part(t, "standoff", map[string]float64{"gussets": 4, "flange": 0})
	plain, _ := part(t, "standoff", map[string]float64{"flange": 0})
	hg, tip, th := 5.0, 0.6, 1.6
	rib := th * 4 * (tip + hg) / 2 // the trapezium from the post out
	if got := m.Volume() - plain.Volume(); !approx(got, 4*rib, 0.05*4*rib) {
		t.Errorf("four gussets add %.2f mm³, want about %.2f", got, 4*rib)
	}
	// the hole stays clear: nothing inside its radius
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			if math.Hypot(p.X, p.Y) < 1.25-1e-6 {
				t.Fatalf("a point %v inside the 2.5 mm hole", p)
			}
		}
	}
}

func TestFinishingWasher(t *testing.T) {
	for _, c := range []map[string]float64{
		nil, {"angle": 82}, {"seat": 0}, {"seat": 30}, {"hole": 50}, {"hole": 0.2},
		{"diameter": 2, "height": 0.5}, {"angle": 30, "seat": 30}, {"sides": 8},
	} {
		m, s := part(t, "finishingwasher", c)
		if c == nil {
			if !approx(s.X, 13.4, 1e-9) || !approx(s.Y, 13.4, 1e-9) || !approx(s.Z, 3.95, 1e-9) {
				t.Errorf("default size %v, want 13.4 × 13.4 × 3.95", s)
			}
		}
		// the hole goes through: nothing inside it at any height
		v := vals("finishingwasher", c)
		outer := v.Get("diameter", 13.4) / 2
		shaft := math.Max(math.Min(0.1, outer/4), math.Min(v.Get("hole", 4.5)/2, outer-math.Min(0.2, outer/4)))
		inner := shaft * math.Cos(math.Pi/v.Get("sides", 48))
		for _, tr := range m.Tris {
			for _, p := range tr.P {
				if math.Hypot(p.X, p.Y) < inner-1e-6 {
					t.Fatalf("%v: point %v inside the hole (radius %g)", c, p, shaft)
				}
			}
		}
	}
	// the seat: a 90° cone from the hole up to the lip, 2.45 deep
	m, _ := part(t, "finishingwasher", map[string]float64{"sides": 256})
	b := m.Bounds()
	lip := 0.0
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			if approx(p.Z, b.Max.Z, 1e-9) {
				lip = math.Max(lip, math.Hypot(p.X, p.Y))
			}
		}
	}
	if want := 2.25 + 2.45; !approx(lip, want, 1e-6) {
		t.Errorf("lip radius %.4f, want %.4f", lip, want)
	}
}
