package filament

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// A 20 mm cube by hand: 2 walls × 0.42 on four sides, 4 top and 3 bottom
// layers of 0.2, the rest at 15%.
func TestCube(t *testing.T) {
	s := Defaults()
	m := shapes.Build("box", geom.V(20, 20, 20), shapes.Values{})
	e := Of(m, s)
	shell := 4*400*0.84 + 400*0.8 + 400*0.6
	want := shell + (8000-shell)*0.15
	if !near(e.Shell, shell, 1e-6) || !near(e.Volume, want, 1e-6) {
		t.Fatalf("shell %v (want %v), volume %v (want %v)", e.Shell, shell, e.Volume, want)
	}
	// 1.24 g/cm³; 1.75 mm filament is 2.405 mm² across
	if !near(e.Grams, want/1000*1.24, 1e-9) || !near(e.Meters, want/(math.Pi*1.75*1.75/4)/1000, 1e-9) {
		t.Fatalf("grams %v metres %v", e.Grams, e.Meters)
	}
	if !near(e.Cost, e.Grams/1000*20, 1e-9) {
		t.Fatalf("cost %v", e.Cost)
	}
	// solid infill prints the whole volume
	s.Infill = 100
	if e := Of(m, s); !near(e.Volume, 8000, 1e-6) {
		t.Fatalf("100%% infill: %v", e.Volume)
	}
	// calibration scales everything
	s.Calibration = 10
	if e := Of(m, s); !near(e.Volume, 8800, 1e-6) {
		t.Fatalf("+10%% calibration: %v", e.Volume)
	}
}

// A thin plate is all shell: the estimate is its own volume.
func TestThinPartIsSolid(t *testing.T) {
	m := shapes.Build("box", geom.V(40, 40, 1), shapes.Values{})
	if e := Of(m, Defaults()); !near(e.Volume, 1600, 1e-6) || !near(e.Shell, 1600, 1e-6) {
		t.Fatalf("plate %v (shell %v)", e.Volume, e.Shell)
	}
}

// NaN and infinity (typed "nan"/"inf") never reach an estimate.
func TestNormalizedRejectsNaN(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	s := Settings{Density: nan, Diameter: inf, LayerHeight: nan, LineWidth: inf, Infill: nan, PricePerKg: inf, Calibration: inf}.Normalized()
	for _, v := range []float64{s.Density, s.Diameter, s.LayerHeight, s.LineWidth, s.Infill, s.PricePerKg, s.Calibration} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("%+v", s)
		}
	}
	m := shapes.Build("box", geom.V(20, 20, 20), shapes.Values{})
	if e := Of(m, s); math.IsNaN(e.Grams) || math.IsNaN(e.Cost) {
		t.Fatalf("%+v", e)
	}
}

func TestNormalized(t *testing.T) {
	s := Settings{Infill: 150, Walls: -1}.Normalized()
	d := Defaults()
	if s.Density != d.Density || s.Diameter != d.Diameter || s.Infill != 100 || s.Walls != 0 || s.Material != "Custom" {
		t.Fatalf("%+v", s)
	}
	// a calibration of -100% or less can't make an estimate nothing or
	// negative
	s = Defaults()
	s.Calibration = -150
	m := shapes.Build("box", geom.V(20, 20, 20), shapes.Values{})
	if e := Of(m, s); e.Grams <= 0 || e.Cost <= 0 {
		t.Fatalf("%+v", e)
	}
}
