package shapes

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// A diamond grid has the volume of its diamonds whatever the gap: at no gap
// at all (the slider's least) the ones in between share whole sides with
// their neighbours, and none may go missing.
func TestDiamondGridVolumeAtEveryGap(t *testing.T) {
	for _, c := range []struct{ cols, rows int }{{1, 1}, {2, 2}, {4, 4}, {5, 3}, {1, 6}} {
		for _, gap := range []float64{0, 0.001, 0.5, 2} {
			const size = 3.0
			w, d := GridSpan(c.cols, size, gap), GridSpan(c.rows, size, gap)
			m := DiamondGridMesh(geom.V(w, d, 10), c.cols, c.rows, size, gap)
			watertight(t, "diamond grid", m)
			n := c.cols*c.rows + (c.cols-1)*(c.rows-1)
			want := float64(n) * size * size / 2 * 10
			if got := m.Oriented().Volume(); math.Abs(got-want) > 1e-6*want {
				t.Errorf("%d × %d at gap %v: volume %v, want %d diamonds' %v", c.cols, c.rows, gap, got, n, want)
			}
		}
	}
}
