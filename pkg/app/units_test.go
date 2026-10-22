//go:build windows

package app

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/shapes"
)

// Length sliders show inches in an inch design; other sliders, and every
// slider in a millimetre design, show their value as is.
func TestSliderValueInTheDesignsUnits(t *testing.T) {
	length := shapes.Param{Key: "radius", Length: true}
	count := shapes.Param{Key: "sides", Int: true}
	for _, c := range []struct {
		v     float64
		p     shapes.Param
		units string
		want  string
	}{
		{4, length, "mm", "4"},
		{4, length, "in", `0.157"`},
		{25.4, length, "in", `1"`},
		{3.175, length, "in", `0.125"`},
		{0, length, "in", `0"`},
		{24, count, "in", "24"},
	} {
		if got := fmtSlider(c.v, c.p, c.units); got != c.want {
			t.Errorf("%v %s %s: %q, want %q", c.v, c.p.Key, c.units, got, c.want)
		}
	}
}

// A slider's step in inches is the round fraction nearest its step in mm.
func TestInchStep(t *testing.T) {
	for stepMM, want := range map[float64]float64{0.05: 0.001, 0.1: 0.005, 0.5: 1.0 / 64, 1: 1.0 / 32, 0.25: 0.01} {
		if got := inchStep(stepMM); math.Abs(got-want) > 1e-12 {
			t.Errorf("step %v mm: %v in, want %v", stepMM, got, want)
		}
	}
}

// Every length slider is marked: a slider whose label names a size of
// something (and isn't a count, angle or ratio) has Length or Metric set.
func TestLengthSlidersAreMarked(t *testing.T) {
	notLength := map[string]bool{"sides": true, "steps": true, "segments": true, "teeth": true, "points": true, "turns": true,
		"rows": true, "cols": true, "knurls": true, "knuckles": true, "slots": true, "grooves": true, "lid": true, "panels": true,
		"pattern": true, "dshaft": true, "smooth": true, "angle": true, "puff": true, "taper": true, "taperturns": true, "size": true}
	for _, k := range shapes.All() {
		for _, p := range k.Params {
			if p.Kind != shapes.Slider || p.Length || p.Metric {
				continue
			}
			switch {
			case notLength[p.Key]:
			case k.ID == "star3d" && p.Key == "inner": // a ratio
			case k.ID == "thread" && p.Key == "depth": // a scale
			case k.ID == "grid" && p.Key == "size": // checked below
			default:
				t.Errorf("%s: %s (%s) isn't marked as a length", k.ID, p.Key, p.Label)
			}
		}
	}
	g, _ := shapes.Lookup("grid")
	for _, p := range g.Params {
		if p.Key == "size" && !p.Length {
			t.Error("grid: size isn't marked as a length")
		}
	}
}
