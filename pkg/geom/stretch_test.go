package geom

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// closedSolid checks m is a closed solid facing out, as built and as an
// export writes it.
func closedSolid(t *testing.T, name string, m *Mesh) {
	t.Helper()
	if open, bad := m.EdgeCheck(0); open != 0 || bad != 0 {
		t.Errorf("%s: %d open, %d non-manifold edges", name, open, bad)
	}
	if r := Surface(AsWritten(m)); r.Open != 0 || r.NonManifold != 0 || r.InsideOut != 0 {
		t.Errorf("%s: as written: %d open, %d non-manifold, %d inside out", name, r.Open, r.NonManifold, r.InsideOut)
	}
	if !(m.Volume() > 0) {
		t.Errorf("%s: volume %g", name, m.Volume())
	}
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// roundPrism is an n-sided prism of radius r standing h tall on z = 0, its
// ends fans from their centres; with taper, its top is that much narrower
// (1: a cone). The last side meets the first exactly.
func roundPrism(r, h float64, n int, taper float64) *Mesh {
	pt := func(i int, rr, z float64) Vec3 {
		a := 2 * math.Pi * float64(i%n) / float64(n)
		return V(rr*math.Cos(a), rr*math.Sin(a), z)
	}
	rt := r * (1 - taper)
	m := &Mesh{}
	for i := 0; i < n; i++ {
		p0, p1 := pt(i, r, 0), pt(i+1, r, 0)
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{V(0, 0, 0), p1, p0}})
		if rt == 0 {
			m.Tris = append(m.Tris, Tri{P: [3]Vec3{p0, p1, V(0, 0, h)}})
			continue
		}
		q0, q1 := pt(i, rt, h), pt(i+1, rt, h)
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{p0, p1, q1}}, Tri{P: [3]Vec3{p0, q1, q0}}, Tri{P: [3]Vec3{V(0, 0, h), q0, q1}})
	}
	return m
}

// A box stretched along each axis is a longer box: closed, the size and
// volume it should be, its low end where it was. Shrunk, a shorter one.
func TestStretchBox(t *testing.T) {
	m := boxMesh(V(0, 0, 0), V(10, 20, 30))
	for axis := 0; axis < 3; axis++ {
		size := m.Bounds().Size()
		mid := size.Axis(axis) / 2
		area := size.X * size.Y * size.Z / size.Axis(axis)
		for _, d := range []float64{7, -4} {
			s := Stretch(m, axis, mid, d)
			name := fmt.Sprintf("axis %d by %g", axis, d)
			closedSolid(t, name, s)
			if v := s.Volume(); !near(v, m.Volume()+area*d, 1e-6) {
				t.Errorf("%s: volume %g, want %g", name, v, m.Volume()+area*d)
			}
			b := s.Bounds()
			if b.Min != m.Bounds().Min || !near(b.Max.Axis(axis), size.Axis(axis)+d, 1e-9) {
				t.Errorf("%s: bounds %v", name, b)
			}
			if len(s.Tris) > 100 {
				t.Errorf("%s: %d triangles (a boolean?)", name, len(s.Tris))
			}
		}
	}
}

// Across a round prism (a 48-sided cylinder lying along x), the cut through
// its middle is a rectangle 2r × h: that much is added a millimetre.
func TestStretchAcrossACylinder(t *testing.T) {
	const r, h = 5.0, 12.0
	m := roundPrism(r, h, 48, 0)
	closedSolid(t, "the prism", m)
	for _, d := range []float64{3, 10} {
		s := Stretch(m, 0, 0, d)
		closedSolid(t, "cylinder", s)
		if v := s.Volume(); !near(v, m.Volume()+2*r*h*d, 1e-6) {
			t.Errorf("by %g: volume %g, want %g", d, v, m.Volume()+2*r*h*d)
		}
		if w := s.Bounds().Size().X; !near(w, 2*r+d, 1e-9) {
			t.Errorf("by %g: %g across, want %g", d, w, 2*r+d)
		}
	}
}

// Where the outline differs at the slab's two faces (a cone), shrinking
// joins two different cuts: what is left is exactly the cone less the slab.
func TestStretchShrinkACone(t *testing.T) {
	const r, h = 10.0, 20.0
	m := roundPrism(r, h, 32, 1)
	closedSolid(t, "the cone", m)
	s := Stretch(m, 2, 8, -6)
	closedSolid(t, "cone", s)
	slab := Intersect(m, boxMesh(V(-20, -20, 5), V(20, 20, 11)))
	if want := m.Volume() - slab.Volume(); !near(s.Volume(), want, 1e-6*m.Volume()) {
		t.Errorf("volume %g, want %g", s.Volume(), want)
	}
	if b := s.Bounds(); !near(b.Max.Z, h-6, 1e-9) || b.Min.Z != 0 {
		t.Errorf("bounds %v", b)
	}
}

// A shape that only meets the plane is lengthened across it too: boxes
// stacked with the plane between them stay one solid; a box ending at the
// plane, below or above, grows by the amount.
func TestStretchTouchingThePlane(t *testing.T) {
	below := boxMesh(V(0, 0, 0), V(10, 10, 5))
	above := boxMesh(V(0, 0, 5), V(10, 10, 9))
	for name, c := range map[string]struct {
		m        *Mesh
		min, max float64
	}{"below": {below, 0, 8}, "above": {above, 5, 12}} {
		s := Stretch(c.m, 2, 5, 3)
		closedSolid(t, name, s)
		if b := s.Bounds(); !near(b.Min.Z, c.min, 1e-9) || !near(b.Max.Z, c.max, 1e-9) {
			t.Errorf("%s: bounds %v, want z %g to %g", name, b, c.min, c.max)
		}
		if v, want := s.Volume(), c.m.Volume()+300; !near(v, want, 1e-6) {
			t.Errorf("%s: volume %g, want %g", name, v, want)
		}
	}
	// shrinking up to a face leaves the shape below alone and moves the
	// one above back
	if s := Stretch(below, 2, 7, -4); s.Bounds() != below.Bounds() || len(s.Tris) != len(below.Tris) {
		t.Errorf("a box below the slab changed: %v", s.Bounds())
	}
	if s := Stretch(above, 2, 3, -4); !near(s.Bounds().Min.Z, 1, 1e-9) {
		t.Errorf("a box above the slab is at %v", s.Bounds())
	}
	if s := Stretch(above, 2, 7, -4.2); len(s.Tris) != 0 {
		t.Errorf("a box inside the slab is left as %d triangles", len(s.Tris))
	}
}

// A box with a round hole through it, stretched across the hole: closed,
// the hole longer, colours kept.
func TestStretchAHole(t *testing.T) {
	b := boxMesh(V(-10, -10, 0), V(10, 10, 6)).Colored(Color{1, 0, 0})
	hole := roundPrism(4, 10, 32, 0).Transform(Translate(V(0, 0, -2))).Colored(Color{0, 0, 1})
	m := Subtract(b, hole).FixTJunctions()
	closedSolid(t, "holed", m)
	for _, d := range []float64{5, -2} {
		s := Stretch(m, 1, 0.5, d)
		closedSolid(t, fmt.Sprintf("by %g", d), s)
		// expanded: the cut at y = 0.5 (20 × 6 less the hole's chord there)
		// carried d across; shrunk: less what was in the slab
		var want float64
		if d > 0 {
			const e = 1e-3
			want = m.Volume() + d*Intersect(m, boxMesh(V(-20, 0.5-e, -5), V(20, 0.5+e, 15))).Volume()/(2*e)
		} else {
			want = m.Volume() - Intersect(m, boxMesh(V(-20, 0.5+d/2, -5), V(20, 0.5-d/2, 15))).Volume()
		}
		if v := s.Volume(); !near(v, want, 1e-4) {
			t.Errorf("by %g: volume %g, want %g", d, v, want)
		}
		if !near(s.Bounds().Size().Y, 20+d, 1e-9) {
			t.Errorf("by %g: %v", d, s.Bounds())
		}
		for _, tr := range s.Tris {
			if tr.C != (Color{1, 0, 0}) && tr.C != (Color{0, 0, 1}) {
				t.Fatalf("by %g: a triangle coloured %v", d, tr.C)
			}
		}
	}
}

// Random solids, turned and moved, stretched at random: each comes out
// closed, and taking the inserted slab out again gives back the volume
// and bounds it had.
func TestStretchRandomSolids(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	ran := 0
	defer func() {
		if ran < 100 {
			t.Errorf("only %d of the cases were clean solids to start from", ran)
		}
	}()
	for i := 0; i < 150; i++ {
		a, _ := ptRandSolid(r)
		b, _ := ptRandSolid(r)
		m := Union(a, b).FixTJunctions()
		if open, bad := m.EdgeCheck(0); open != 0 || bad != 0 {
			continue // not a clean solid to start from
		}
		ran++
		axis := r.Intn(3)
		bb := m.Bounds()
		at := bb.Min.Axis(axis) + r.Float64()*bb.Size().Axis(axis)
		d := 0.5 + r.Float64()*20
		name := fmt.Sprintf("case %d: axis %d at %g by %g", i, axis, at, d)
		s := Stretch(m, axis, at, d)
		closedSolid(t, name, s)
		if !near(s.Bounds().Size().Axis(axis), bb.Size().Axis(axis)+d, 1e-6) {
			t.Errorf("%s: size %v from %v", name, s.Bounds().Size(), bb.Size())
		}
		back := Stretch(s, axis, at+d/2, -d)
		closedSolid(t, name+", taken back out", back)
		if !near(back.Volume(), m.Volume(), 1e-6*math.Max(1, m.Volume())) {
			t.Errorf("%s: volume %g after, %g before", name, back.Volume(), m.Volume())
		}
		if bk := back.Bounds(); bk.Min.Sub(bb.Min).Len() > 1e-6 || bk.Max.Sub(bb.Max).Len() > 1e-6 {
			t.Errorf("%s: bounds %v after, %v before", name, bk, bb)
		}
		// and shrunk anywhere: closed, never longer
		w := r.Float64() * bb.Size().Axis(axis)
		c := bb.Min.Axis(axis) + r.Float64()*bb.Size().Axis(axis)
		if sh := Stretch(m, axis, c, -w); len(sh.Tris) > 0 {
			closedSolid(t, fmt.Sprintf("%s, shrunk by %g at %g", name, w, c), sh)
			if sh.Bounds().Size().Axis(axis) > bb.Size().Axis(axis)+1e-9 {
				t.Errorf("%s: shrinking made it longer", name)
			}
		}
	}
}

// Nothing to do: no amount, no triangles, a bad axis or number.
func TestStretchNothing(t *testing.T) {
	m := boxMesh(V(0, 0, 0), V(1, 1, 1))
	for _, c := range []struct {
		axis       int
		at, amount float64
	}{{0, 0.5, 0}, {3, 0.5, 1}, {-1, 0.5, 1}, {0, math.NaN(), 1}, {0, 0.5, math.Inf(1)}} {
		if s := Stretch(m, c.axis, c.at, c.amount); s != m {
			t.Errorf("%v: changed", c)
		}
	}
	if s := Stretch(&Mesh{}, 0, 0, 1); len(s.Tris) != 0 {
		t.Errorf("empty mesh: %d triangles", len(s.Tris))
	}
}
