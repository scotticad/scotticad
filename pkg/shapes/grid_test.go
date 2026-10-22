package shapes

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

func TestGrid(t *testing.T) {
	if GridSpan(4, 3, 2) != 18 {
		t.Fatalf("span %v", GridSpan(4, 3, 2))
	}
	m := GridMesh(geom.V(18, 18, 10), 4, 4, 3, 2)
	s := m.Bounds().Size()
	if math.Abs(s.X-18) > 1e-9 || math.Abs(s.Y-18) > 1e-9 || math.Abs(s.Z-10) > 1e-9 {
		t.Fatalf("size %v", s)
	}
	if open, nm := m.EdgeCheck(1e-7); open != 0 || nm != 0 {
		t.Fatalf("not watertight: %d open, %d non-manifold", open, nm)
	}
	if v := m.Volume(); math.Abs(v-16*3*3*10) > 1e-6 {
		t.Fatalf("volume %v, want 1440", v)
	}
}
