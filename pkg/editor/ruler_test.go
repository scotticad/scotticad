package editor

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// resizeFrom resizes box along axis to size v the way the size labels do
// with the ruler: from the anchored side, or about the middle.
func resizeFrom(r *Ruler, box geom.Box, axis int, v float64) geom.Box {
	switch r.Anchor(box, axis) {
	case -1:
		box.Max = box.Max.WithAxis(axis, box.Min.Axis(axis)+v)
	case 1:
		box.Min = box.Min.WithAxis(axis, box.Max.Axis(axis)-v)
	default:
		c := box.Center().Axis(axis)
		box.Min = box.Min.WithAxis(axis, c-v/2)
		box.Max = box.Max.WithAxis(axis, c+v/2)
	}
	return box
}

// In endpoint mode a size typed for the selection grows from the edge the
// ruler measures, so the ruler's readings don't change: for every turn of
// the ruler and boxes on every side of its origin (and straddling it).
func TestRulerAnchorKeepsMeasuredEdge(t *testing.T) {
	boxes := []geom.Box{
		{Min: geom.V(10, 20, 0), Max: geom.V(30, 35, 5)},     // +X +Y of the origin
		{Min: geom.V(-30, 20, 0), Max: geom.V(-10, 35, 5)},   // -X
		{Min: geom.V(10, -35, 0), Max: geom.V(30, -20, 5)},   // -Y
		{Min: geom.V(-30, -35, 0), Max: geom.V(-10, -20, 5)}, // both negative
		{Min: geom.V(-5, -8, 0), Max: geom.V(15, 12, 5)},     // straddling the origin
	}
	for turns := 0; turns < 4; turns++ {
		r := &Ruler{Turns: turns}
		for _, b := range boxes {
			for axis := 0; axis < 2; axis++ {
				if r.Anchor(b, axis) == 0 {
					t.Fatalf("turns %d axis %d: no anchor in endpoint mode", turns, axis)
				}
				// the arm along this axis
				k := 0
				if r.Arm(0).Axis(axis) == 0 {
					k = 1
				}
				before := r.Measure(b, k)
				side := func(bx geom.Box) float64 { // the anchored edge's coordinate
					if r.Anchor(b, axis) < 0 {
						return bx.Min.Axis(axis)
					}
					return bx.Max.Axis(axis)
				}
				straddles := b.Min.Axis(axis) < r.Origin.Axis(axis) && b.Max.Axis(axis) > r.Origin.Axis(axis)
				for _, v := range []float64{3, 50} { // shrink and grow
					nb := resizeFrom(r, b, axis, v)
					if s := nb.Size().Axis(axis); math.Abs(s-v) > 1e-9 {
						t.Fatalf("turns %d axis %d: size %v, want %v", turns, axis, s, v)
					}
					if side(nb) != side(b) {
						t.Errorf("turns %d box %v axis %d size %v: the measured edge moved", turns, b, axis, v)
					}
					// the reading holds, unless a box spanning the origin
					// shrinks off it (the ruler then reads its other edge)
					if after := r.Measure(nb, k); !straddles && math.Abs(after-before) > 1e-9 {
						t.Errorf("turns %d box %v axis %d size %v: ruler read %v, now %v", turns, b, axis, v, before, after)
					}
				}
			}
		}
	}
}

// Measuring to midpoints, a size grows about the middle (no anchor).
func TestRulerAnchorMidpoints(t *testing.T) {
	r := &Ruler{Mid: true}
	b := geom.Box{Min: geom.V(10, 20, 0), Max: geom.V(30, 35, 5)}
	for axis := 0; axis < 3; axis++ {
		if a := r.Anchor(b, axis); a != 0 {
			t.Errorf("axis %d: anchor %d with midpoints, want 0", axis, a)
		}
	}
	if a := (&Ruler{}).Anchor(b, 2); a != 0 {
		t.Errorf("height: anchor %d, want 0 (heights grow from the bottom)", a)
	}
}
