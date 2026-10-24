package shapes

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

func TestPuffy(t *testing.T) {
	m := PuffyMesh(geom.V(30, 20, 10), 4, 12)
	s := m.Bounds().Size()
	if math.Abs(s.X-30) > 1e-6 || math.Abs(s.Y-20) > 1e-6 || math.Abs(s.Z-10) > 1e-6 {
		t.Fatalf("size %v, want 30 x 20 x 10", s)
	}
	if open, nm := m.EdgeCheck(1e-7); open != 0 || nm != 0 {
		t.Fatalf("not watertight: %d open, %d non-manifold", open, nm)
	}
	// rounder than its box, fuller than its ellipsoid
	v, box, ell := m.Volume(), 30.0*20*10, math.Pi/6*30*20*10
	if v <= ell || v >= box {
		t.Fatalf("volume %v outside (%v, %v)", v, ell, box)
	}
}
