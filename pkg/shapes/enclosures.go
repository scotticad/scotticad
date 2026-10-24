package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Enclosures are assemblies: placing one creates a bundle group of
// separate parts (a box and its lid, a tube and its end panels), so
// ungrouping separates them. Each part is its own catalogue kind built from
// the assembly's parameters; the parts are hidden from the libraries.

// Placed is one part of an assembly: a catalogue kind, its name, colour, and
// the matrix placing its (centred) mesh in the assembly's frame, whose
// origin is the centre of the assembly's base.
type Placed struct {
	Kind, Name, Color string
	M                 geom.Mat4
}

// Parts lays out an assembly for parameters v; ok is false for ordinary
// shapes.
func (k *Kind) Parts(v Values) (parts []Placed, ok bool) {
	if k.parts == nil {
		return nil, false
	}
	return k.parts(v), true
}

// PartSize is the size a part kind has for v (its natural size).
func PartSize(kind string, v Values) geom.Vec3 {
	k, ok := Lookup(kind)
	if !ok {
		return geom.V(1, 1, 1)
	}
	if s, ok := k.NaturalSize(v); ok {
		return s
	}
	return k.Size
}

// registerAssembly registers an assembly kind. Its own mesh (library
// thumbnail, placement preview) is all its parts in place.
func registerAssembly(id, label, color string, params []Param, parts func(Values) []Placed) {
	merged := func(v Values) *geom.Mesh {
		var ms []*geom.Mesh
		for _, p := range parts(v) {
			ms = append(ms, Build(p.Kind, PartSize(p.Kind, v), v).Colored(geom.ParseColor(p.Color)).Transform(p.M))
		}
		return geom.Merge(ms...)
	}
	k := &Kind{ID: id, Label: label, Color: color, Params: params, parts: parts}
	k.natural = func(v Values) geom.Vec3 { return merged(v).Bounds().Size() }
	k.build = func(s geom.Vec3, v Values) *geom.Mesh { return merged(v).FitTo(s) }
	k.Size = k.natural(k.Defaults())
	register(k)
	HardwareKinds = append(HardwareKinds, id)
}

// registerHiddenPart registers an assembly's part kind (not shown in the
// libraries).
func registerHiddenPart(id, label, color string, params []Param, model func(Values) *geom.Mesh) {
	k := &Kind{ID: id, Label: label, Color: color, Params: params, Hidden: true}
	k.build = func(s geom.Vec3, v Values) *geom.Mesh { return cachedPart(id, v, model).FitTo(s) }
	k.natural = func(v Values) geom.Vec3 { return cachedPart(id, v, model).Bounds().Size() }
	k.Size = k.natural(k.Defaults())
	register(k)
}

// --- project box ---------------------------------------------------------------------

type boxDims struct{ l, w, h, wall, r, ri float64 }

func projectBoxDims(v Values) boxDims {
	l, w, h := v.Get("length", 60), v.Get("width", 40), v.Get("height", 25)
	wall := math.Max(0.4, math.Min(v.Get("wall", 2), math.Min(l, w)/2-1))
	r := v.Get("radius", 3)
	return boxDims{l, w, h, wall, r, math.Max(0, r-wall)}
}

const lidTol, lidLip = 0.2, 3.0 // lid clearance inside the walls, lip height

func projectBoxBody(v Values) *geom.Mesh {
	d := projectBoxDims(v)
	return done(geom.Subtract(
		ExtrudeContours([][]Pt{roundedRect(d.l, d.w, d.r)}, 0, d.h),
		ExtrudeContours([][]Pt{roundedRect(d.l-2*d.wall, d.w-2*d.wall, d.ri)}, d.wall, d.h+1)))
}

// projectBoxLid is the lid as printed: a plate with a lip on top that fits
// inside the walls with lidTol clearance.
func projectBoxLid(v Values) *geom.Mesh {
	d := projectBoxDims(v)
	lw, lwid := d.l-2*d.wall-2*lidTol, d.w-2*d.wall-2*lidTol
	plate := ExtrudeContours([][]Pt{roundedRect(d.l, d.w, d.r)}, 0, d.wall)
	const minLip = 0.8 // mm: thinner can't be printed
	if lw < minLip || lwid < minLip {
		return done(plate) // no room inside the walls for a lip
	}
	lipOutline := [][]Pt{roundedRect(lw, lwid, math.Max(0, d.ri-lidTol))}
	if iw, ih := lw-2*d.wall, lwid-2*d.wall; iw >= minLip && ih >= minLip {
		lipOutline = append(lipOutline, reversed(roundedRect(iw, ih, math.Max(0, d.ri-lidTol-d.wall))))
	} // else too small to hollow: a solid lip
	lip := ExtrudeContours(lipOutline, d.wall-0.1, d.wall+lidLip)
	return done(geom.Union(plate, lip))
}

func projectBoxParts(v Values) []Placed {
	d := projectBoxDims(v)
	parts := []Placed{{Kind: "projectbox-body", Name: "Project Box", Color: "#f7c51e", M: geom.Translate(geom.V(0, 0, d.h/2))}}
	if v.Get("lid", 1) >= 0.5 {
		// closed: the lid turned over, its plate on top of the walls
		t := d.wall + lidLip
		parts = append(parts, Placed{Kind: "projectbox-lid", Name: "Project Box Lid", Color: "#f7c51e",
			M: geom.Translate(geom.V(0, 0, d.h+d.wall-t/2)).Mul(geom.RotateX(math.Pi))})
	}
	return parts
}

// --- extruded enclosure --------------------------------------------------------------
//
// An aluminium-style extruded tube along X: rounded outside with shallow
// grooves along both sides, card-guide slots inside both side walls, and a
// screw boss in each inner corner for the end panels' screws.

type extrudedDims struct {
	l, w, h, t, r  float64 // length, width, height, wall, outside corner radius
	wi, hi         float64 // inside width and height
	gd, gw, sd, sw float64 // outside groove depth/width, inside slot depth/width
	br, hr, ha, pt float64 // boss radius, screw hole radius, hole inset from inner corner, panel thickness
	grooves, slots int
	groovez, slotz float64 // half-span the grooves and slots spread over
}

func extrudedBoxDims(v Values) extrudedDims {
	var d extrudedDims
	d.l, d.w, d.h = v.Get("length", 80), v.Get("width", 54), v.Get("height", 30)
	d.t = math.Max(1.2, math.Min(v.Get("wall", 2), math.Min(d.w, d.h)/4))
	d.r = math.Max(0, math.Min(v.Get("radius", 2), math.Min(d.w, d.h)/2-d.t))
	d.wi, d.hi = d.w-2*d.t, d.h-2*d.t
	d.gd, d.gw = math.Min(0.5, d.t*0.3), 1.0
	d.sd, d.sw = math.Min(0.8, d.t*0.35), 1.6
	d.br = math.Min(2.6, math.Min(d.wi, d.hi)/4)
	d.hr = math.Min(1.1, d.br*0.42)
	d.ha = d.br * 0.38
	d.pt = math.Max(0.8, v.Get("panel", 2.5))
	// the outside corners can't be rounded so far that they cut into the
	// panel's screw holes (a 0.4 mm wall around them)
	s := d.t + d.ha
	if rmax := (s*math.Sqrt2 - d.panelHole() - 0.4) / (math.Sqrt2 - 1); d.r > rmax {
		d.r = math.Max(0, rmax)
	}
	d.groovez = d.h/2 - d.r - 1
	d.slotz = d.hi/2 - d.br - 1
	d.grooves = fitCount(int(v.Get("grooves", 6)), 2*d.groovez, 2*d.gw)
	d.slots = fitCount(int(v.Get("slots", 3)), 2*d.slotz, 2*d.sw)
	return d
}

// fitCount limits n features of pitch at least step to a span.
func fitCount(n int, span, step float64) int {
	if span <= 0 {
		return 0
	}
	return max(0, min(n, int(span/step)))
}

// spread returns n positions evenly over -half..half.
func spread(n int, half float64) []float64 {
	var out []float64
	for i := 0; i < n; i++ {
		out = append(out, -half+(float64(i)+0.5)*2*half/float64(n))
	}
	return out
}

func arcPts(cx, cy, r, a0, a1 float64, n int) []Pt {
	var out []Pt
	for i := 0; i <= n; i++ {
		a := a0 + (a1-a0)*float64(i)/float64(n)
		out = append(out, Pt{cx + r*math.Cos(a), cy + r*math.Sin(a)})
	}
	return out
}

// screwHoles are the end-panel screw positions in the profile plane.
func (d extrudedDims) screwHoles() [][2]float64 {
	x, y := d.wi/2-d.ha, d.hi/2-d.ha
	return [][2]float64{{-x, -y}, {x, -y}, {x, y}, {-x, y}}
}

// extrudedProfile is the tube's cross-section (width along X, height
// along Y): the outside with its grooves, the inside with its slots and
// corner bosses, and the bosses' screw holes.
func extrudedProfile(d extrudedDims) [][]Pt {
	W, H, r := d.w/2, d.h/2, d.r
	grooves := spread(d.grooves, d.groovez)
	var outer []Pt
	outer = append(outer, arcPts(W-r, -H+r, r, -math.Pi/2, 0, 6)...)
	for _, z := range grooves { // right side, upwards
		outer = append(outer, Pt{W, z - d.gw/2}, Pt{W - d.gd, z - d.gw/2}, Pt{W - d.gd, z + d.gw/2}, Pt{W, z + d.gw/2})
	}
	outer = append(outer, arcPts(W-r, H-r, r, 0, math.Pi/2, 6)...)
	outer = append(outer, arcPts(-W+r, H-r, r, math.Pi/2, math.Pi, 6)...)
	for i := len(grooves) - 1; i >= 0; i-- { // left side, downwards
		z := grooves[i]
		outer = append(outer, Pt{-W, z + d.gw/2}, Pt{-W + d.gd, z + d.gw/2}, Pt{-W + d.gd, z - d.gw/2}, Pt{-W, z - d.gw/2})
	}
	outer = append(outer, arcPts(-W+r, -H+r, r, math.Pi, 3*math.Pi/2, 6)...)

	Wi, Hi, br := d.wi/2, d.hi/2, d.br
	slots := spread(d.slots, d.slotz)
	var inner []Pt
	inner = append(inner, arcPts(Wi, -Hi, br, math.Pi, math.Pi/2, 8)...)
	for _, z := range slots { // right wall, upwards; slots cut into the wall
		inner = append(inner, Pt{Wi, z - d.sw/2}, Pt{Wi + d.sd, z - d.sw/2}, Pt{Wi + d.sd, z + d.sw/2}, Pt{Wi, z + d.sw/2})
	}
	inner = append(inner, arcPts(Wi, Hi, br, -math.Pi/2, -math.Pi, 8)...)
	inner = append(inner, arcPts(-Wi, Hi, br, 0, -math.Pi/2, 8)...)
	for i := len(slots) - 1; i >= 0; i-- { // left wall, downwards
		z := slots[i]
		inner = append(inner, Pt{-Wi, z + d.sw/2}, Pt{-Wi - d.sd, z + d.sw/2}, Pt{-Wi - d.sd, z - d.sw/2}, Pt{-Wi, z - d.sw/2})
	}
	inner = append(inner, arcPts(-Wi, -Hi, br, math.Pi/2, 0, 8)...)

	contours := [][]Pt{outer, inner}
	for _, c := range d.screwHoles() {
		contours = append(contours, circlePts(c[0], c[1], d.hr, 20))
	}
	return contours
}

// alongX maps a profile extruded along Z onto the tube's frame: profile X
// becomes Y (width), profile Y becomes Z (height), the extrusion runs
// along X (length).
var alongX = geom.Mat4{0, 1, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 1}

func extrudedBody(v Values) *geom.Mesh {
	d := extrudedBoxDims(v)
	return ExtrudeContours(extrudedProfile(d), -d.l/2, d.l/2).Transform(alongX)
}

// panelHole is the radius of the end panels' clearance holes.
func (d extrudedDims) panelHole() float64 { return math.Min(d.hr+0.5, d.br*0.6) }

// extrudedPanel is an end panel as printed (lying flat): the tube's outside
// outline with clearance holes for the screws.
func extrudedPanel(v Values) *geom.Mesh {
	d := extrudedBoxDims(v)
	contours := [][]Pt{roundedRect(d.w, d.h, d.r)}
	for _, c := range d.screwHoles() {
		contours = append(contours, reversed(circlePts(c[0], c[1], d.panelHole(), 20)))
	}
	return ExtrudeContours(contours, 0, d.pt)
}

func extrudedParts(v Values) []Placed {
	d := extrudedBoxDims(v)
	parts := []Placed{{Kind: "extrudedbox-body", Name: "Extruded Box", Color: "#c3c8cd", M: geom.Translate(geom.V(0, 0, d.h/2))}}
	if v.Get("panels", 1) >= 0.5 {
		x := d.l/2 + d.pt/2
		for _, side := range []struct {
			x    float64
			name string
		}{{x, "Front Panel"}, {-x, "Rear Panel"}} {
			parts = append(parts, Placed{Kind: "extrudedbox-panel", Name: side.name, Color: "#2b2e33",
				M: geom.Translate(geom.V(side.x, 0, d.h/2)).Mul(alongX)})
		}
	}
	return parts
}

func init() {
	box := []Param{
		lslider("length", "Length", 10, 300, 1, 60, false),
		lslider("width", "Width", 10, 300, 1, 40, false),
		lslider("height", "Height", 5, 200, 1, 25, false),
		lslider("wall", "Wall", 0.4, 10, 0.1, 2, false),
		lslider("radius", "Corner Radius", 0, 30, 0.5, 3, false),
		slider("lid", "Lid", 0, 1, 1, 1, true),
	}
	registerHiddenPart("projectbox-body", "Project Box", "#f7c51e", box, projectBoxBody)
	registerHiddenPart("projectbox-lid", "Project Box Lid", "#f7c51e", box, projectBoxLid)
	registerAssembly("projectbox", "Project Box", "#f7c51e", box, projectBoxParts)

	ext := []Param{
		lslider("length", "Length", 20, 300, 1, 80, false),
		lslider("width", "Width", 15, 200, 1, 54, false),
		lslider("height", "Height", 10, 150, 1, 30, false),
		lslider("wall", "Wall", 1.2, 6, 0.1, 2, false),
		lslider("radius", "Corner Radius", 0, 15, 0.5, 2, false),
		slider("grooves", "Side Grooves", 0, 20, 1, 6, true),
		slider("slots", "Card Slots", 0, 10, 1, 3, true),
		lslider("panel", "Panel Thickness", 0.8, 6, 0.1, 2.5, false),
		slider("panels", "End Panels", 0, 1, 1, 1, true),
	}
	registerHiddenPart("extrudedbox-body", "Extruded Box", "#c3c8cd", ext, extrudedBody)
	registerHiddenPart("extrudedbox-panel", "End Panel", "#2b2e33", ext, extrudedPanel)
	registerAssembly("extrudedbox", "Extruded Box", "#c3c8cd", ext, extrudedParts)
}
