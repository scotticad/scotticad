// Package filament estimates how much filament a part takes to print.
//
// A printed part is a solid shell plus a partly filled inside. Every face
// of the part's mesh gets a shell as deep as the slicer's walls and
// top/bottom skins would be there: walls (perimeters × line width) across
// steep faces, skins (layers × layer height) through flat ones, the
// larger of the two on slopes. Shell volume is face area × that depth,
// capped at the part's volume (thin parts print solid); the rest of the
// volume is filled at the infill percentage. Supports, brims, skirts and
// multi-colour purge aren't counted.
package filament

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Material is a filament's name and density.
type Material struct {
	Name    string  `json:"name"`
	Density float64 `json:"density"` // g/cm³
}

// Materials are the presets, most common first.
var Materials = []Material{
	{"PLA", 1.24},
	{"PETG", 1.27},
	{"ABS", 1.04},
	{"ASA", 1.07},
	{"TPU", 1.21},
	{"PC", 1.20},
	{"Nylon", 1.14},
}

// Settings are the slicer settings the estimate depends on.
type Settings struct {
	Material     string  `json:"material"`     // a preset name, or "Custom"
	Density      float64 `json:"density"`      // g/cm³
	Diameter     float64 `json:"diameter"`     // mm
	LayerHeight  float64 `json:"layer_height"` // mm
	LineWidth    float64 `json:"line_width"`   // mm
	Walls        int     `json:"walls"`        // perimeters
	TopLayers    int     `json:"top_layers"`
	BottomLayers int     `json:"bottom_layers"`
	Infill       float64 `json:"infill"`       // percent
	PricePerKg   float64 `json:"price_per_kg"` // in the user's currency; 0 for no cost
	Calibration  float64 `json:"calibration"`  // percent added to every estimate (± to match a slicer)
}

// Defaults are typical 0.4 mm nozzle settings with PLA.
func Defaults() Settings {
	return Settings{Material: "PLA", Density: 1.24, Diameter: 1.75, LayerHeight: 0.2, LineWidth: 0.42,
		Walls: 2, TopLayers: 4, BottomLayers: 3, Infill: 15, PricePerKg: 20}
}

// Normalized returns s with unusable values replaced by defaults and the
// rest clamped into range.
func (s Settings) Normalized() Settings {
	d := Defaults()
	pos := func(v *float64, def float64) {
		if !(*v > 0) || math.IsInf(*v, 0) {
			*v = def
		}
	}
	pos(&s.Density, d.Density)
	pos(&s.Diameter, d.Diameter)
	pos(&s.LayerHeight, d.LayerHeight)
	pos(&s.LineWidth, d.LineWidth)
	s.Walls = max(0, s.Walls)
	s.TopLayers, s.BottomLayers = max(0, s.TopLayers), max(0, s.BottomLayers)
	bad := func(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) }
	if bad(s.Infill) {
		s.Infill = d.Infill
	}
	s.Infill = math.Max(0, math.Min(100, s.Infill))
	if bad(s.PricePerKg) {
		s.PricePerKg = 0
	}
	s.PricePerKg = math.Max(0, s.PricePerKg)
	if bad(s.Calibration) {
		s.Calibration = 0
	}
	// -100% or less would make the estimate nothing, or negative
	s.Calibration = math.Max(-90, math.Min(1000, s.Calibration))
	if s.Material == "" {
		s.Material = "Custom"
	}
	return s
}

// Estimate is how much filament something takes.
type Estimate struct {
	Volume float64 `json:"volume"` // mm³ of plastic (shell + infill)
	Shell  float64 `json:"shell"`  // of which shell, mm³
	Grams  float64 `json:"grams"`
	Meters float64 `json:"meters"`
	Cost   float64 `json:"cost"`
}

// Scale is e times k (a share of it).
func (e Estimate) Scale(k float64) Estimate {
	return Estimate{e.Volume * k, e.Shell * k, e.Grams * k, e.Meters * k, e.Cost * k}
}

// Add sums two estimates.
func (e Estimate) Add(o Estimate) Estimate {
	return Estimate{e.Volume + o.Volume, e.Shell + o.Shell, e.Grams + o.Grams, e.Meters + o.Meters, e.Cost + o.Cost}
}

// Of estimates a closed mesh (millimetres, Z up as printed).
func Of(m *geom.Mesh, s Settings) Estimate {
	if m == nil || len(m.Tris) == 0 {
		return Estimate{}
	}
	s = s.Normalized()
	vol := math.Abs(m.Volume())
	wall := float64(s.Walls) * s.LineWidth
	shell := 0.0
	for _, t := range m.Tris {
		a := t.Area()
		if a == 0 {
			continue
		}
		nz := t.Normal().Z
		skin := float64(s.BottomLayers) * s.LayerHeight
		if nz > 0 {
			skin = float64(s.TopLayers) * s.LayerHeight
		}
		// depth into the part, square to the face: walls are measured
		// across, skins up and down
		depth := math.Max(wall*math.Sqrt(math.Max(0, 1-nz*nz)), skin*math.Abs(nz))
		shell += a * depth
	}
	shell = math.Min(shell, vol)
	return FromVolume(shell+(vol-shell)*s.Infill/100, shell, s)
}

// FromVolume turns mm³ of plastic into an estimate.
func FromVolume(volume, shell float64, s Settings) Estimate {
	s = s.Normalized()
	k := 1 + s.Calibration/100
	volume, shell = volume*k, shell*k
	grams := volume / 1000 * s.Density
	area := math.Pi * s.Diameter * s.Diameter / 4
	return Estimate{Volume: volume, Shell: shell, Grams: grams, Meters: volume / area / 1000, Cost: grams / 1000 * s.PricePerKg}
}
