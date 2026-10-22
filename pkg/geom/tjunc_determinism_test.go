package geom

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"testing"
)

func meshPrint(m *Mesh) [32]byte {
	h := sha256.New()
	for _, t := range m.Tris {
		for _, p := range t.P {
			for _, v := range []float64{p.X, p.Y, p.Z} {
				binary.Write(h, binary.LittleEndian, math.Float64bits(v))
			}
		}
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// T-junction repair gives the same mesh every time. Two vertices on an
// open edge at exactly the same distance along it (one just above it, one
// just below, too far apart to weld but both within the tolerance) must
// not be ordered by Go's random map order, or the same design would export
// a slightly different mesh from run to run.
func TestFixTJunctionsIsDeterministic(t *testing.T) {
	m := &Mesh{}
	// a long triangle whose bottom edge runs along X, unshared
	m.Tris = append(m.Tris, Tri{P: [3]Vec3{V(0, 0, 0), V(10, 0, 0), V(5, 0, 5)}})
	// small triangles hanging off two points at x = 5, 1.5e-5 either side
	// of that edge (3e-5 apart: more than the weld, each within onEps)
	for _, y := range []float64{1.5e-5, -1.5e-5} {
		p := V(5, y, 0)
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{p, V(5, y*1e5, -1), V(6, y*1e5, -1)}})
	}
	// and more ties along other edges, so the map order has many chances
	for k := 1; k <= 8; k++ {
		x := float64(k)
		if k == 5 {
			continue
		}
		for _, z := range []float64{1.5e-5, -1.5e-5} {
			p := V(x, 0, z)
			m.Tris = append(m.Tris, Tri{P: [3]Vec3{p, V(x, -1, z*1e5), V(x+0.5, -1, z*1e5)}})
		}
	}
	first := meshPrint(m.FixTJunctions())
	for i := 0; i < 50; i++ {
		if got := meshPrint(m.FixTJunctions()); got != first {
			t.Fatalf("run %d gave a different mesh", i+1)
		}
	}
}
