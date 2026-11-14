package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// The GT2 timing pulley (registered in zz_added.go), for the 2 mm pitch
// belt most 3D printers use.
//
// From the GT2 belt's published dimensions: the pitch line is 0.254 mm
// outside the pulley, so the outside diameter is 2·(teeth/π − 0.254); each
// groove is 1.494 mm wide and 0.764 mm deep. The groove is drawn as half
// an ellipse of that width and depth, close to the belt tooth's round
// profile. Fit widens the grooves for printing (0.2 mm is the usual
// allowance).

const (
	gt2Pitch  = 2.0
	gt2Offset = 0.254 // pitch line outside the pulley
	gt2Width  = 1.494 // groove width
	gt2Depth  = 0.764 // groove depth
)

// gt2Outline is the toothed outline, counter-clockwise.
func gt2Outline(teeth int, fit float64) []Pt {
	ro := float64(teeth)*gt2Pitch/(2*math.Pi) - gt2Offset
	// no wider than 94% of a tooth's share of the rim (the Groove Fit
	// slider's whole range fits from 18 teeth up)
	hw := math.Min((gt2Width+fit)/2, 0.47*2*math.Pi*ro/float64(teeth))
	const gs, ls = 12, 3 // points in a groove, and between grooves
	var out []Pt
	for k := 0; k < teeth; k++ {
		c := 2 * math.Pi * (float64(k) + 0.5) / float64(teeth) // the groove's centre
		for i := 0; i <= gs; i++ {
			s := math.Pi * (1 - float64(i)/gs)
			t, d := hw*math.Cos(s), gt2Depth*math.Sin(s)
			a := c + t/ro
			out = append(out, Pt{(ro - d) * math.Cos(a), (ro - d) * math.Sin(a)})
		}
		a0, a1 := c+hw/ro, c+2*math.Pi/float64(teeth)-hw/ro
		for i := 1; i < ls; i++ {
			a := a0 + (a1-a0)*float64(i)/ls
			out = append(out, Pt{ro * math.Cos(a), ro * math.Sin(a)})
		}
	}
	return out
}

// gt2Model stacks, in the order it prints, from the plate up: a flat
// flange, the teeth (the belt's width plus half a millimetre each side), a
// second flange whose underside rises at 45° so it needs no support, and
// the hub. Flanges are none, the lower one, or both. A bore runs through
// it all. The hub is no wider than what it stands on.
func gt2Model(v Values) *geom.Mesh {
	teeth := max(int(v.Get("teeth", 20)), 12)
	ro := float64(teeth)*gt2Pitch/(2*math.Pi) - gt2Offset
	toothH := v.Get("belt", 6) + 1
	flanges := int(math.Round(v.Get("flanges", 2)))
	hubH, hubR := v.Get("hub", 6), v.Get("hubdia", 14)/2
	bore := v.Get("bore", 5)
	const flangeH, flangeOut, flangeRim, n = 1.0, 1.5, 0.5, 96
	outline := gt2Outline(teeth, v.Get("fit", 0.2))
	rb := 0.0 // the bore's radius; every part's bore has the same points
	if bore > 0 && bore/2 < ro-gt2Depth-0.6 {
		rb = bore / 2
	}
	var hole [][]Pt
	if rb > 0 {
		hole = [][]Pt{reversed(circlePts(0, 0, rb, n))}
	}
	with := func(c []Pt) [][]Pt { return append([][]Pt{c}, hole...) }
	var parts []*geom.Mesh
	z, top := 0.0, ro // top: the radius of what the next part stands on
	if flanges >= 1 {
		parts = append(parts, ExtrudeContours(with(circlePts(0, 0, ro+flangeOut, n)), z, z+flangeH))
		z += flangeH
	}
	parts = append(parts, ExtrudeContours(with(outline), z, z+toothH))
	z += toothH
	if flanges >= 2 {
		prof := [][2]float64{{rb, z}, {ro, z}, {ro + flangeOut, z + flangeOut}, {ro + flangeOut, z + flangeOut + flangeRim}, {rb, z + flangeOut + flangeRim}}
		parts = append(parts, revolve(prof, n, rb > 0))
		z += flangeOut + flangeRim
		top = ro + flangeOut
	}
	if hubR = math.Min(hubR, top); hubH > 0.05 && hubR > rb+0.6 {
		parts = append(parts, ExtrudeContours(with(circlePts(0, 0, hubR, n)), z, z+hubH))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return done(union(parts...))
}
