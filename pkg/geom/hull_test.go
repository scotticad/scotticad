package geom

import (
	"math"
	"math/rand"
	"testing"
)

// checkHull is what every hull must be: closed (as built and as an export
// writes it), outward, with every corner one of the points and no point
// outside any face by more than tol.
func checkHull(t *testing.T, name string, pts []Vec3, m *Mesh, tol float64) {
	t.Helper()
	if m == nil {
		t.Errorf("%s: no hull", name)
		return
	}
	if o, n := m.EdgeCheck(0); o != 0 || n != 0 {
		t.Errorf("%s: %d open, %d non-manifold edges", name, o, n)
	}
	if r := Surface(AsWritten(m)); r.Open != 0 || r.NonManifold != 0 || r.InsideOut != 0 {
		t.Errorf("%s: as written: %d open, %d non-manifold edges, %d shells inside out", name, r.Open, r.NonManifold, r.InsideOut)
	}
	if v := m.Volume(); !(v > 0) {
		t.Errorf("%s: volume %g", name, v)
	}
	given := map[Vec3]bool{}
	for _, p := range pts {
		given[p] = true
	}
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			if !given[p] {
				t.Errorf("%s: corner %v isn't one of the points", name, p)
				return
			}
		}
		n := tr.P[1].Sub(tr.P[0]).Cross(tr.P[2].Sub(tr.P[0])).Norm()
		for _, p := range pts {
			if !finiteVec(p) {
				continue
			}
			if d := p.Sub(tr.P[0]).Dot(n); d > tol {
				t.Errorf("%s: point %v is %g outside a face", name, p, d)
				return
			}
		}
	}
}

func boxPoints(b Box, n int) []Vec3 {
	var out []Vec3
	s := b.Size()
	for i := 0; i <= n; i++ {
		for j := 0; j <= n; j++ {
			for k := 0; k <= n; k++ {
				out = append(out, b.Min.Add(V(s.X*float64(i)/float64(n), s.Y*float64(j)/float64(n), s.Z*float64(k)/float64(n))))
			}
		}
	}
	return out
}

func TestConvexHullOfABox(t *testing.T) {
	b := Box{V(-10, -5, 0), V(30, 25, 20)}
	c := b.Corners()
	m := ConvexHull(c[:])
	checkHull(t, "corners", c[:], m, 1e-9)
	if len(m.Tris) != 12 {
		t.Errorf("corners: %d triangles, want 12", len(m.Tris))
	}
	if v := m.Volume(); math.Abs(v-40*30*20) > 1e-6 {
		t.Errorf("corners: volume %g", v)
	}
	if got := m.Bounds(); got != b {
		t.Errorf("bounds %v, want %v", got, b)
	}
	// a grid of points all through it: those inside, on its faces and
	// along its edges change nothing
	pts := boxPoints(b, 6)
	m = ConvexHull(pts)
	checkHull(t, "grid", pts, m, 1e-9)
	if v := m.Volume(); math.Abs(v-40*30*20) > 1e-6 {
		t.Errorf("grid: volume %g", v)
	}
	if got := m.Bounds(); got != b {
		t.Errorf("grid: bounds %v, want %v", got, b)
	}
	// the same turned, so the faces' points are level only to a rounding
	// error
	xf := Translate(V(7, -3, 11)).Mul(RotateZ(0.7)).Mul(RotateX(1.1)).Mul(RotateY(-0.4))
	var turned []Vec3
	for _, p := range pts {
		turned = append(turned, xf.Point(p))
	}
	m = ConvexHull(turned)
	checkHull(t, "turned grid", turned, m, 1e-6)
	if v := m.Volume(); math.Abs(v-40*30*20) > 1e-6 {
		t.Errorf("turned grid: volume %.9f", v)
	}
}

func TestConvexHullOfRoundShapes(t *testing.T) {
	// a 48-sided cylinder: its ends are 48 points on a plane each
	var cyl []Vec3
	for i := 0; i < 48; i++ {
		a := 2 * math.Pi * float64(i) / 48
		cyl = append(cyl, V(10*math.Cos(a), 10*math.Sin(a), 0), V(10*math.Cos(a), 10*math.Sin(a), 30))
	}
	m := ConvexHull(cyl)
	checkHull(t, "cylinder", cyl, m, 1e-9)
	want := 24 * math.Sin(2*math.Pi/48) * 100 * 30
	if v := m.Volume(); math.Abs(v-want) > 1e-6 {
		t.Errorf("cylinder: volume %g, want %g", v, want)
	}
	if len(m.Tris) != 2*46+2*48 {
		t.Errorf("cylinder: %d triangles, want %d", len(m.Tris), 2*46+2*48)
	}
	// a sphere of points, and the same sphere twice: two spheres apart
	// make a capsule
	var sph []Vec3
	for i := 0; i <= 24; i++ {
		for j := 0; j < 48; j++ {
			a, b := math.Pi*float64(i)/24, 2*math.Pi*float64(j)/48
			sph = append(sph, V(5*math.Sin(a)*math.Cos(b), 5*math.Sin(a)*math.Sin(b), 5*math.Cos(a)))
		}
	}
	m = ConvexHull(sph)
	checkHull(t, "sphere", sph, m, 1e-9)
	ball := m.Volume()
	if want := 4.0 / 3 * math.Pi * 125; ball > want || ball < 0.98*want {
		t.Errorf("sphere: volume %g, want a little under %g", ball, want)
	}
	two := append([]Vec3{}, sph...)
	for _, p := range sph {
		two = append(two, p.Add(V(40, 0, 0)))
	}
	m = ConvexHull(two)
	checkHull(t, "two spheres", two, m, 1e-9)
	if v, want := m.Volume(), ball+math.Pi*25*40; v > want || v < 0.98*want {
		t.Errorf("capsule: volume %g, want a little under %g", v, want)
	}
}

func TestConvexHullOfNothing(t *testing.T) {
	flat := []Vec3{V(0, 0, 0), V(10, 0, 0), V(10, 10, 0), V(0, 10, 0), V(5, 5, 0), V(3, 8, 1e-12)}
	for name, pts := range map[string][]Vec3{
		"none": nil, "three": flat[:3], "flat": flat,
		"a line":      {V(0, 0, 0), V(1, 1, 1), V(2, 2, 2), V(3, 3, 3), V(4, 4, 4)},
		"one point":   {V(1, 2, 3), V(1, 2, 3), V(1, 2, 3), V(1, 2, 3), V(1, 2, 3)},
		"not numbers": {V(math.NaN(), 0, 0), V(math.Inf(1), 0, 0), V(0, math.Inf(-1), 0), V(0, 0, math.NaN())},
	} {
		if m := ConvexHull(pts); m != nil {
			t.Errorf("%s: a hull of %d triangles, want none", name, len(m.Tris))
		}
	}
	// points that aren't numbers are left out of a hull that has others
	pts := append(boxPoints(Box{V(0, 0, 0), V(1, 2, 3)}, 1), V(math.NaN(), 0, 0), V(0, math.Inf(1), 0))
	m := ConvexHull(pts)
	checkHull(t, "with NaN", pts, m, 1e-9)
	if v := m.Volume(); math.Abs(v-6) > 1e-9 {
		t.Errorf("with NaN: volume %g", v)
	}
}

// Random clouds, and clouds on a coarse lattice (full of points on one
// plane or one line, where a hull routine goes wrong): every one closed,
// outward and holding all its points; the same points give the same hull.
func TestConvexHullOfRandomPoints(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for trial := 0; trial < 400; trial++ {
		n := 4 + r.Intn(200)
		lattice := trial%2 == 0
		var pts []Vec3
		for i := 0; i < n; i++ {
			p := V(r.Float64()*100-50, r.Float64()*60, r.Float64()*20)
			if lattice {
				p = V(float64(r.Intn(5))*10, float64(r.Intn(4))*10, float64(r.Intn(3))*10)
			}
			pts = append(pts, p)
		}
		if trial%5 == 0 { // turned: level only to a rounding error
			xf := RotateZ(r.Float64() * 6).Mul(RotateX(r.Float64() * 6))
			for i := range pts {
				pts[i] = xf.Point(pts[i])
			}
		}
		m := ConvexHull(pts)
		if m == nil {
			if flatCloud(pts) { // only a flat cloud has no hull
				continue
			}
			t.Fatalf("trial %d: no hull of %d points", trial, n)
		}
		checkHull(t, "random", pts, m, 1e-6)
		again := ConvexHull(pts)
		if len(again.Tris) != len(m.Tris) {
			t.Fatalf("trial %d: %d triangles, then %d", trial, len(m.Tris), len(again.Tris))
		}
		for i := range m.Tris {
			if m.Tris[i].P != again.Tris[i].P {
				t.Fatalf("trial %d: triangle %d differs between runs", trial, i)
			}
		}
	}
}

// flatCloud is whether the points all lie on one plane.
func flatCloud(pts []Vec3) bool {
	for i := 1; i < len(pts); i++ {
		for j := i + 1; j < len(pts); j++ {
			n := pts[i].Sub(pts[0]).Cross(pts[j].Sub(pts[0]))
			if n.Len() < 1e-9 {
				continue
			}
			for _, p := range pts {
				if math.Abs(p.Sub(pts[0]).Dot(n.Norm())) > 1e-6 {
					return false
				}
			}
			return true
		}
	}
	return true
}

// A hull is no smaller than what it holds, and a convex shape's hull is
// itself.
func TestConvexHullHoldsItsMeshes(t *testing.T) {
	a := boxPoints(Box{V(0, 0, 0), V(10, 10, 10)}, 1)
	b := boxPoints(Box{V(30, 0, 0), V(40, 10, 10)}, 1)
	m := ConvexHull(append(a, b...))
	if v := m.Volume(); math.Abs(v-4000) > 1e-6 {
		t.Errorf("two boxes in a row: volume %g, want 4000", v)
	}
	// set at a corner to each other: an L's hull fills the L
	c := boxPoints(Box{V(0, 30, 0), V(10, 40, 10)}, 1)
	m = ConvexHull(append(append(a, b...), c...))
	if v, want := m.Volume(), (40*40-30*30/2.0)*10; math.Abs(v-want) > 1e-6 {
		t.Errorf("an L of boxes: volume %g, want %g", v, want)
	}
}

// Two finely made spheres' worth of points, each given six times over as
// a mesh's triangles give them.
func BenchmarkConvexHull(b *testing.B) {
	var pts []Vec3
	for s := 0; s < 2; s++ {
		for i := 0; i <= 128; i++ {
			for j := 0; j < 256; j++ {
				a, c := math.Pi*float64(i)/128, 2*math.Pi*float64(j)/256
				p := V(50*math.Sin(a)*math.Cos(c)+float64(s)*120, 50*math.Sin(a)*math.Sin(c), 50*math.Cos(a))
				for k := 0; k < 6; k++ {
					pts = append(pts, p)
				}
			}
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if ConvexHull(pts) == nil {
			b.Fatal("no hull")
		}
	}
}

func FuzzConvexHull(f *testing.F) {
	f.Add([]byte{0, 0, 0, 9, 0, 0, 0, 9, 0, 0, 0, 9, 9, 9, 9})
	f.Add([]byte{1, 2, 3, 1, 2, 3, 4, 4, 4, 4, 4, 5, 0, 0, 0, 7, 7, 7, 3, 3, 3})
	f.Add([]byte{0, 0, 0, 255, 0, 0, 0, 255, 0, 255, 255, 0, 128, 128, 0, 128, 128, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		var pts []Vec3
		for i := 0; i+2 < len(data) && len(pts) < 400; i += 3 {
			// a coarse lattice: points on shared planes and lines
			pts = append(pts, V(float64(data[i]%16), float64(data[i+1]%16), float64(data[i+2]%16)))
		}
		m := ConvexHull(pts)
		if m == nil {
			if !flatCloud(pts) && len(pts) >= 4 {
				t.Fatalf("no hull of %d points that aren't flat", len(pts))
			}
			return
		}
		checkHull(t, "fuzz", pts, m, 1e-6)
	})
}
