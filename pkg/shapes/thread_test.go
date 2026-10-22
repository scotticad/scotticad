package shapes

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

func TestThread(t *testing.T) {
	// 12 mm, pitch 1.5, 16 segments, 5 turns
	// measures 12 × 12 × 8.63
	h := ThreadLength(1.5, 5)
	if math.Abs(h-8.625) > 1e-12 {
		t.Fatalf("length %v", h)
	}
	m := ThreadMesh(geom.V(12, 12, h), 1.5, 16, 1)
	s := m.Bounds().Size()
	if math.Abs(s.X-12) > 1e-6 || math.Abs(s.Y-12) > 1e-6 || math.Abs(s.Z-8.625) > 1e-9 {
		t.Fatalf("size %v, want 12 x 12 x 8.625", s)
	}
	if open, nm := m.EdgeCheck(1e-7); open != 0 || nm != 0 {
		t.Fatalf("not watertight: %d open, %d non-manifold", open, nm)
	}
	// volume lies between the minor and major cylinders (16-gons)
	area := func(r float64) float64 { return 0.5 * 16 * r * r * math.Sin(2*math.Pi/16) }
	depth := 5.0 / 8 * math.Sqrt(3) / 2 * 1.5
	v := m.Volume()
	if lo, hi := area(6-depth)*h, area(6)*h; v <= lo || v >= hi {
		t.Fatalf("volume %v outside (%v, %v)", v, lo, hi)
	}
	// a tapered tip: watertight, narrower at the top than the bottom
	tm := ThreadMeshTapered(geom.V(12, 12, 20), 1.5, 32, 1, 0.8, 3)
	if open, nm := tm.EdgeCheck(1e-7); open != 0 || nm != 0 {
		t.Fatalf("tapered thread not watertight: %d open, %d non-manifold", open, nm)
	}
	top, bottom := 0.0, 0.0
	for _, tr := range tm.Tris {
		for _, p := range tr.P {
			r := math.Hypot(p.X, p.Y)
			if p.Z > 9.99 {
				top = math.Max(top, r)
			}
			if p.Z < -9.99 {
				bottom = math.Max(bottom, r)
			}
		}
	}
	if top > 6*0.2+1e-6 || bottom < 5 {
		t.Fatalf("tip radius %v (want ≤ 1.2), base %v", top, bottom)
	}
	// the catalogue shape builds and fills its size
	b := Build("thread", geom.V(10, 10, 30), Values{Num: map[string]float64{"pitch": 1.5, "sides": 32, "depth": 1}}).Bounds().Size()
	if math.Abs(b.X-10) > 1e-6 || math.Abs(b.Z-30) > 1e-6 {
		t.Fatalf("catalogue thread size %v", b)
	}
}
