// Package shapes defines ScottiCAD's parametric primitive catalogue. Each
// shape builds a mesh centred on the origin whose bounding box equals the
// requested size, so a node's size maps directly onto its geometry.
package shapes

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/scotticad/scotticad/pkg/geom"
)

// ParamKind selects how the inspector presents a parameter.
type ParamKind int

const (
	Slider    ParamKind = iota // slider with value bubble
	Number                     // numeric field with stepper (used for L/W/H)
	TextParam                  // single-line text
	FontParam                  // font dropdown
)

// Param describes one inspector parameter.
type Param struct {
	Key, Label     string
	Kind           ParamKind
	Min, Max, Step float64
	Default        float64
	Int            bool
	SizeAxis       int // for Number params bound to the node size: 0,1,2; else -1
	// Length: the value is a length in mm, shown and typed in the design's
	// units. Metric: a value in mm whatever the units (a metric standard,
	// such as a thread's pitch or a gear's module), labelled so.
	Length, Metric bool
}

// Kind is a catalogue entry.
type Kind struct {
	ID, Label string
	Color     string
	Size      geom.Vec3
	Params    []Param
	build     func(size geom.Vec3, p Values) *geom.Mesh
	// natural, when set, is the size the shape has at scale 1 for given
	// parameters (a generator's own output size). Changing a parameter then
	// keeps the shape's scale: size' = size × natural(new) / natural(old).
	natural func(p Values) geom.Vec3
	// parts, when set, makes the kind an assembly (see Placed).
	parts func(v Values) []Placed
	// Hidden kinds (an assembly's parts, outlines and lofts) are not shown
	// in the libraries.
	Hidden bool
	// sections are a profile kind's default point data (see Section).
	sections []Section
}

// NaturalSize returns the shape's unscaled size for p, if it has one.
func (k *Kind) NaturalSize(p Values) (geom.Vec3, bool) {
	if k.natural == nil {
		return geom.Vec3{}, false
	}
	return k.natural(p), true
}

// Values carries parameter values for a build.
type Values struct {
	Num  map[string]float64
	Text string
	Font string
	// Sections are an outline's or loft's point data (see Section).
	Sections []Section
}

func (v Values) Get(k string, def float64) float64 {
	if x, ok := v.Num[k]; ok {
		return x
	}
	return def
}

var catalogue = map[string]*Kind{}
var order []string

func register(k *Kind) {
	catalogue[k.ID] = k
	order = append(order, k.ID)
}

// Lookup returns the catalogue entry for id.
func Lookup(id string) (*Kind, bool) {
	k, ok := catalogue[id]
	return k, ok
}

// All returns catalogue entries in registration order.
func All() []*Kind {
	out := make([]*Kind, 0, len(order))
	for _, id := range order {
		out = append(out, catalogue[id])
	}
	return out
}

// Defaults returns default parameter values for a kind.
func (k *Kind) Defaults() Values {
	v := Values{Num: map[string]float64{}, Sections: k.sections}
	for _, p := range k.Params {
		switch p.Kind {
		case Slider:
			v.Num[p.Key] = p.Default
		case TextParam:
			v.Text = "TEXT"
		case FontParam:
			v.Font = "Multilanguage"
		}
	}
	return v
}

// cache keeps built meshes, up to a million triangles
var cache = geom.NewCache(1_000_000, geom.MeshWeight)

// Build returns the mesh for a kind at a size. Results are cached.
func Build(id string, size geom.Vec3, v Values) *geom.Mesh {
	k, ok := catalogue[id]
	if !ok {
		k = catalogue["box"]
	}
	v = k.clamp(v)
	keys := make([]string, 0, len(v.Num))
	for kk := range v.Num {
		keys = append(keys, kk)
	}
	sort.Strings(keys)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s|%v|%v|%v|%q|%q|%s", k.ID, size.X, size.Y, size.Z, v.Text, v.Font, SectionsKey(v.Sections))
	for _, kk := range keys {
		fmt.Fprintf(&sb, "|%s=%v", kk, v.Num[kk])
	}
	key := sb.String()
	if m, ok := cache.Load(key); ok {
		return m.(*geom.Mesh)
	}
	m := k.build(size, v).Clean().Oriented()
	cache.Store(key, m)
	return m
}

// clamp keeps slider values within their sliders' ranges (whole numbers
// for whole-number sliders): the editor does this for what is typed, but
// values read from a file arrive unchecked.
func (k *Kind) clamp(v Values) Values {
	var out map[string]float64
	for _, p := range k.Params {
		if p.Kind != Slider {
			continue
		}
		x, ok := v.Num[p.Key]
		if !ok {
			continue
		}
		c := x
		if math.IsNaN(c) {
			c = p.Default
		}
		c = math.Max(p.Min, math.Min(p.Max, c))
		if p.Int {
			c = math.Round(c)
		}
		if c != x {
			if out == nil {
				out = make(map[string]float64, len(v.Num))
				for kk, vv := range v.Num {
					out[kk] = vv
				}
			}
			out[p.Key] = c
		}
	}
	if out != nil {
		v.Num = out
	}
	return v
}

// --- builders --------------------------------------------------------------

func sizeParams() []Param {
	return []Param{
		{Key: "length", Label: "Length", Kind: Number, Min: 0.1, Max: 10000, Step: 1, SizeAxis: 0},
		{Key: "width", Label: "Width", Kind: Number, Min: 0.1, Max: 10000, Step: 1, SizeAxis: 1},
		{Key: "height", Label: "Height", Kind: Number, Min: 0.1, Max: 10000, Step: 1, SizeAxis: 2},
	}
}

func slider(key, label string, min, max, step, def float64, isInt bool) Param {
	return Param{Key: key, Label: label, Kind: Slider, Min: min, Max: max, Step: step, Default: def, Int: isInt, SizeAxis: -1}
}

// lslider is a slider for a length (see Param.Length).
func lslider(key, label string, min, max, step, def float64, isInt bool) Param {
	p := slider(key, label, min, max, step, def, isInt)
	p.Length = true
	return p
}

// mslider is a slider for a metric standard's value in mm (see
// Param.Metric).
func mslider(key, label string, min, max, step, def float64, isInt bool) Param {
	p := slider(key, label, min, max, step, def, isInt)
	p.Metric = true
	return p
}

// revolve sweeps a profile of (radius, z) points around Z with n segments.
func revolve(prof [][2]float64, n int, closed bool) *geom.Mesh {
	m := &geom.Mesh{}
	segs := len(prof) - 1
	if closed {
		segs = len(prof)
	}
	ang := func(j int) (float64, float64) {
		a := 2 * math.Pi * float64(j%n) / float64(n)
		return math.Cos(a), math.Sin(a)
	}
	for i := 0; i < segs; i++ {
		a, b := prof[i], prof[(i+1)%len(prof)]
		for j := 0; j < n; j++ {
			c0, s0 := ang(j)
			c1, s1 := ang(j + 1)
			A0 := geom.V(a[0]*c0, a[0]*s0, a[1])
			A1 := geom.V(a[0]*c1, a[0]*s1, a[1])
			B0 := geom.V(b[0]*c0, b[0]*s0, b[1])
			B1 := geom.V(b[0]*c1, b[0]*s1, b[1])
			m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{A0, A1, B1}}, geom.Tri{P: [3]geom.Vec3{A0, B1, B0}})
		}
	}
	return m.Clean().Oriented()
}

func arc(cx, cy, r, a0, a1 float64, segs int) [][2]float64 {
	var out [][2]float64
	for i := 0; i <= segs; i++ {
		a := a0 + (a1-a0)*float64(i)/float64(segs)
		x, y := cx+r*math.Cos(a), cy+r*math.Sin(a)
		if math.Abs(x) < 1e-12 {
			x = 0
		}
		out = append(out, [2]float64{x, y})
	}
	return out
}

// bevelledProfile returns a cylinder-like revolve profile of radius R and
// half-height H with rounded/chamfered top and bottom edges.
func bevelledProfile(R, H, bevel float64, segs int) [][2]float64 {
	bevel = math.Min(bevel, math.Min(R, H)*0.999)
	if bevel <= 1e-9 {
		return [][2]float64{{0, -H}, {R, -H}, {R, H}, {0, H}}
	}
	if segs < 1 {
		segs = 1
	}
	prof := [][2]float64{{0, -H}}
	prof = append(prof, arc(R-bevel, -H+bevel, bevel, -math.Pi/2, 0, segs)...)
	prof = append(prof, arc(R-bevel, H-bevel, bevel, 0, math.Pi/2, segs)...)
	prof = append(prof, [2]float64{0, H})
	return prof
}

// extrude makes a prism from a CCW profile between z=-0.5 and 0.5.
func extrude(prof []Pt) *geom.Mesh {
	return ExtrudeContours([][]Pt{prof}, -0.5, 0.5)
}

var swapYZ = geom.Mat4{1, 0, 0, 0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 1}

func roundedBox(size geom.Vec3, r float64, steps int) *geom.Mesh {
	h := size.Mul(0.5)
	r = math.Min(r, math.Min(h.X, math.Min(h.Y, h.Z))*0.999)
	if r <= 1e-9 {
		return extrude([]Pt{{-0.5, -0.5}, {0.5, -0.5}, {0.5, 0.5}, {-0.5, 0.5}}).FitTo(size)
	}
	if steps < 1 {
		steps = 1
	}
	nu, nv := 4*(steps+1), 2*(steps+1)
	pt := func(iu, iv int) geom.Vec3 {
		q, k := iu/(steps+1), iu%(steps+1)
		hh, l := iv/(steps+1), iv%(steps+1)
		a := (float64(q) + float64(k)/float64(steps)) * math.Pi / 2
		var b float64 // polar angle from -90 to 90
		if hh == 0 {
			b = -math.Pi/2 + float64(l)/float64(steps)*math.Pi/2
		} else {
			b = float64(l) / float64(steps) * math.Pi / 2
		}
		d := geom.V(math.Cos(b)*math.Cos(a), math.Cos(b)*math.Sin(a), math.Sin(b))
		sx := []float64{1, -1, -1, 1}[q]
		sy := []float64{1, 1, -1, -1}[q]
		sz := -1.0
		if hh == 1 {
			sz = 1
		}
		c := geom.V(sx*(h.X-r), sy*(h.Y-r), sz*(h.Z-r))
		return c.Add(d.Mul(r))
	}
	m := &geom.Mesh{}
	for iv := 0; iv < nv-1; iv++ {
		for iu := 0; iu < nu; iu++ {
			a, b := pt(iu, iv), pt((iu+1)%nu, iv)
			c, d := pt((iu+1)%nu, iv+1), pt(iu, iv+1)
			m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{a, b, c}}, geom.Tri{P: [3]geom.Vec3{a, c, d}})
		}
	}
	m = m.Clean()
	// Orient the side surface outward, then add the flat top/bottom faces
	// left open at the poles of the parametrisation.
	var dot float64
	for _, t := range m.Tris {
		c := t.P[0].Add(t.P[1]).Add(t.P[2]).Mul(1.0 / 3)
		dot += t.Normal().Dot(c)
	}
	if dot < 0 {
		m = m.Flipped()
	}
	cx, cy := h.X-r, h.Y-r
	quad := []geom.Vec3{geom.V(cx, cy, 0), geom.V(-cx, cy, 0), geom.V(-cx, -cy, 0), geom.V(cx, -cy, 0)}
	if cx > 1e-9 || cy > 1e-9 {
		top, bot := h.Z, -h.Z
		for i := 1; i+1 < 4; i++ {
			a, b, c := quad[0], quad[i], quad[i+1]
			m.Tris = append(m.Tris,
				geom.Tri{P: [3]geom.Vec3{geom.V(a.X, a.Y, top), geom.V(b.X, b.Y, top), geom.V(c.X, c.Y, top)}},
				geom.Tri{P: [3]geom.Vec3{geom.V(a.X, a.Y, bot), geom.V(c.X, c.Y, bot), geom.V(b.X, b.Y, bot)}})
		}
	}
	return m.Clean()
}

func init() {
	register(&Kind{ID: "box", Label: "Box", Color: "#e2142a", Size: geom.V(20, 20, 20),
		Params: append([]Param{lslider("radius", "Radius", 0, 10, 0.1, 0, false), slider("steps", "Steps", 1, 20, 1, 10, true)}, sizeParams()...),
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			return roundedBox(s, v.Get("radius", 0), int(v.Get("steps", 10)))
		}})
	register(&Kind{ID: "cylinder", Label: "Cylinder", Color: "#f6851f", Size: geom.V(20, 20, 20),
		Params: []Param{slider("sides", "Sides", 3, 64, 1, 48, true), lslider("bevel", "Bevel", 0, 10, 0.1, 0, false), slider("segments", "Segments", 1, 10, 1, 1, true)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			R, H := s.X/2, s.Z/2
			m := revolve(bevelledProfile(R, H, v.Get("bevel", 0), int(v.Get("segments", 1))), int(v.Get("sides", 48)), false)
			return m.FitTo(s)
		}})
	register(&Kind{ID: "sphere", Label: "Sphere", Color: "#1c9ad6", Size: geom.V(20, 20, 20),
		Params: []Param{slider("steps", "Steps", 4, 64, 1, 24, true)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			n := int(v.Get("steps", 24))
			return revolve(arc(0, 0, 0.5, -math.Pi/2, math.Pi/2, max(n/2, 2)), n, false).FitTo(s)
		}})
	register(&Kind{ID: "cone", Label: "Cone", Color: "#7b2c9d", Size: geom.V(20, 20, 20),
		Params: []Param{lslider("top", "Top Radius", 0, 10, 0.1, 0, false), slider("sides", "Sides", 3, 64, 1, 48, true)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			rt := 0.5 * v.Get("top", 0) / math.Max(s.X/2, 1e-9)
			m := revolve([][2]float64{{0, -0.5}, {0.5, -0.5}, {math.Min(rt, 0.5), 0.5}, {0, 0.5}}, int(v.Get("sides", 48)), false)
			return m.FitTo(s)
		}})
	register(&Kind{ID: "pyramid", Label: "Pyramid", Color: "#f8d00e", Size: geom.V(20, 20, 20),
		Params: []Param{slider("sides", "Sides", 3, 12, 1, 4, true)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			n := int(v.Get("sides", 4))
			return revolve([][2]float64{{0, -0.5}, {0.5, -0.5}, {0, 0.5}}, n, false).Transform(geom.RotateZ(math.Pi / float64(n))).FitTo(s)
		}})
	register(&Kind{ID: "roof", Label: "Roof", Color: "#3fae2a", Size: geom.V(20, 20, 10), // 90° gable
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			return extrude([]Pt{{-0.5, -0.5}, {0.5, -0.5}, {0, 0.5}}).Transform(swapYZ).FitTo(s)
		}})
	register(&Kind{ID: "text", Label: "Text", Color: "#e2142a", Size: geom.V(74.06, 19.82, 10),
		Params: []Param{{Key: "text", Label: "Text", Kind: TextParam, SizeAxis: -1}, {Key: "font", Label: "Font", Kind: FontParam, SizeAxis: -1},
			lslider("height", "Height", 0.1, 100, 0.1, 10, false)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			m := TextMesh(v.Text, v.Font, 1)
			if len(m.Tris) == 0 {
				return &geom.Mesh{}
			}
			return m.FitTo(s)
		}})
	register(&Kind{ID: "roundroof", Label: "Round Roof", Color: "#3ab8c4", Size: geom.V(20, 20, 10),
		Params: []Param{slider("sides", "Sides", 3, 64, 1, 24, true)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			prof := make([]Pt, 0)
			for _, p := range arc(0, 0, 0.5, 0, math.Pi, int(v.Get("sides", 24))) {
				prof = append(prof, Pt{p[0], p[1]})
			}
			return extrude(prof).Transform(swapYZ).FitTo(s)
		}})
	register(&Kind{ID: "halfsphere", Label: "Half Sphere", Color: "#e2148f", Size: geom.V(20, 20, 10),
		Params: []Param{slider("steps", "Steps", 4, 64, 1, 24, true)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			n := int(v.Get("steps", 24))
			prof := append([][2]float64{{0, 0}}, arc(0, 0, 0.5, 0, math.Pi/2, max(n/4, 2))...)
			return revolve(prof, n, false).FitTo(s)
		}})
	register(&Kind{ID: "torus", Label: "Torus", Color: "#1c9ad6", Size: geom.V(20, 20, 4),
		Params: []Param{lslider("radius", "Radius", 1, 50, 0.5, 10, false), lslider("tube", "Tube", 0.5, 20, 0.1, 2, false), slider("sides", "Sides", 3, 64, 1, 24, true), slider("steps", "Steps", 3, 64, 1, 16, true)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			R, r := v.Get("radius", 10), v.Get("tube", 2)
			// Radius is the outside radius; the tube's centre is at R−r, so a
			// tube thicker than half of it would pass through the axis
			r = math.Min(r, 0.49*R)
			var prof [][2]float64
			ts := int(v.Get("steps", 16))
			for i := 0; i < ts; i++ {
				a := 2 * math.Pi * float64(i) / float64(ts)
				prof = append(prof, [2]float64{R - r + r*math.Cos(a), r * math.Sin(a)})
			}
			return revolve(prof, int(v.Get("sides", 24)), true).FitTo(s)
		}})
	register(&Kind{ID: "tube", Label: "Tube", Color: "#f6851f", Size: geom.V(20, 20, 20),
		Params: []Param{lslider("wall", "Wall Thickness", 0.1, 10, 0.1, 2, false), slider("sides", "Sides", 3, 64, 1, 48, true)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			ri := math.Max(0.5-v.Get("wall", 2)/math.Max(s.X, 1e-9), 0.01)
			return revolve([][2]float64{{ri, -0.5}, {0.5, -0.5}, {0.5, 0.5}, {ri, 0.5}}, int(v.Get("sides", 48)), true).FitTo(s)
		}})
	register(&Kind{ID: "ring", Label: "Ring", Color: "#8a5a2b", Size: geom.V(20, 20, 4),
		Params: []Param{lslider("thickness", "Thickness", 0.5, 10, 0.1, 2, false), slider("sides", "Sides", 3, 64, 1, 48, true)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			ri := math.Max(0.5-v.Get("thickness", 2)/math.Max(s.X, 1e-9), 0.01)
			return revolve([][2]float64{{ri, -0.5}, {0.5, -0.5}, {0.5, 0.5}, {ri, 0.5}}, int(v.Get("sides", 48)), true).FitTo(s)
		}})
	register(&Kind{ID: "wedge", Label: "Wedge", Color: "#273a8c", Size: geom.V(20, 20, 20),
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			// tall side at -X sloping down toward +X, length along Y
			return extrude([]Pt{{-0.5, -0.5}, {0.5, -0.5}, {-0.5, 0.5}}).Transform(swapYZ).FitTo(s)
		}})
	register(&Kind{ID: "polygon", Label: "Polygon", Color: "#273a8c", Size: geom.V(20, 17.32, 20),
		Params: []Param{slider("sides", "Sides", 3, 20, 1, 6, true), lslider("bevel", "Bevel", 0, 10, 0.1, 0, false), slider("segments", "Segments", 1, 10, 1, 1, true)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			R, H := s.X/2, s.Z/2
			return revolve(bevelledProfile(R, H, v.Get("bevel", 0), int(v.Get("segments", 1))), int(v.Get("sides", 6)), false).FitTo(s)
		}})
	register(&Kind{ID: "icosahedron", Label: "Icosahedron", Color: "#e2142a", Size: geom.V(20, 20, 20),
		build: func(s geom.Vec3, v Values) *geom.Mesh { return icosahedron().FitTo(s) }})
	register(&Kind{ID: "paraboloid", Label: "Paraboloid", Color: "#3ab8c4", Size: geom.V(20, 20, 20),
		Params: []Param{slider("steps", "Steps", 4, 64, 1, 24, true)},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			n := int(v.Get("steps", 24))
			prof := [][2]float64{{0, -0.5}}
			for i := 0; i <= n/2; i++ {
				t := float64(i) / float64(n/2)
				prof = append(prof, [2]float64{0.5 * math.Sqrt(1-t), -0.5 + t})
			}
			return revolve(append([][2]float64{{0, -0.5}, {0.5, -0.5}}, prof[2:]...), n, false).FitTo(s)
		}})
	// the extruded Star shows no parameters; "points" and "inner" are
	// honoured when a design sets them
	register(&Kind{ID: "star", Label: "Star", Color: "#f8d00e", Size: geom.V(38.04, 36.18, 10),
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			// no sliders clamp these (a file could hold anything): kept to
			// star3d's slider ranges
			n := int(math.Max(3, math.Min(20, v.Get("points", 5))))
			inner := v.Get("inner", 0.5)
			if math.IsNaN(inner) {
				inner = 0.5
			}
			inner = math.Max(0.1, math.Min(0.95, inner))
			return extrude(starOutline(n, 0.5, inner)).FitTo(s)
		}})
	register(&Kind{ID: "heart", Label: "Heart", Color: "#8a5a2b", Size: geom.V(20, 18, 6),
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			var prof []Pt
			for i := 0; i < 96; i++ {
				t := 2 * math.Pi * float64(i) / 96
				x := 16 * math.Pow(math.Sin(t), 3)
				y := 13*math.Cos(t) - 5*math.Cos(2*t) - 2*math.Cos(3*t) - math.Cos(4*t)
				prof = append(prof, Pt{x, y})
			}
			if SignedArea(prof) < 0 {
				prof = reversed(prof)
			}
			return ExtrudeContours([][]Pt{prof}, 0, 1).FitTo(s)
		}})
	// the faceted (teal) Star: a star pyramid, Radius in mm, Inner Radius
	// as a fraction of it
	register(&Kind{ID: "star3d", Label: "Star", Color: "#5bc0d6", Size: geom.V(38.04, 36.18, 10),
		Params: []Param{
			slider("points", "Points", 3, 20, 1, 5, true),
			lslider("radius", "Radius", 1, 100, 0.5, 20, false),
			slider("inner", "Inner Radius(%)", 0.1, 0.95, 0.05, 0.5, false),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			return starPyramid(int(v.Get("points", 5)), v.Get("inner", 0.5), 1).FitTo(s)
		},
		natural: func(v Values) geom.Vec3 {
			w, d := outlineSize(starOutline(int(v.Get("points", 5)), v.Get("radius", 20), v.Get("inner", 0.5)))
			return geom.V(w, d, 10)
		}})
	register(&Kind{ID: "dice", Label: "Dice", Color: "#c9b3e0", Size: geom.V(16, 16, 16),
		build: func(s geom.Vec3, v Values) *geom.Mesh { return dice().FitTo(s) }})
	register(&Kind{ID: "diamond", Label: "Diamond", Color: "#5bc0d6", Size: geom.V(20, 20, 10),
		build: func(s geom.Vec3, v Values) *geom.Mesh { return diamond().FitTo(s) }})
	// shape generator: a gear with round (non-involute) teeth
	register(&Kind{ID: "gear", Label: "Gear", Color: "#c8ccd0", Size: geom.V(27.44, 27.44, 20),
		Params: []Param{
			mslider("pitch", "Pitch", 0.5, 10, 0.1, 2, false),
			lslider("slop", "Slop", 0, 1, 0.01, 0.01, false),
			slider("teeth", "# of Teeth", 6, 100, 1, 20, true),
			lslider("height", "Height", 1, 200, 1, 20, false),
			lslider("bore", "Bore Dia", 0, 50, 0.5, 6, false),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			// built at its own size in mm (the bore is in mm of that), then
			// scaled to the shape's size
			p := v.Get("pitch", 2)
			teeth := max(int(v.Get("teeth", 20)), 3)
			slop := v.Get("slop", 0.01)
			// the bore leaves a wall inside the root circle (the bottom of
			// the notches); none if there's no room
			root := float64(teeth)*math.Max(p, 0.1)/math.Pi - (math.Max(p, 0.1)/2 + slop)
			bore := math.Min(v.Get("bore", 6), 2*(root-math.Max(0.5, p/4)))
			return gear(p, slop, teeth, bore).FitTo(s)
		},
		natural: func(v Values) geom.Vec3 {
			w, d := outlineSize(gearOutline(v.Get("pitch", 2), v.Get("slop", 0.01), int(v.Get("teeth", 20))))
			return geom.V(w, d, v.Get("height", 20))
		}})
	// shape generator: an open spherical shell; Radius is the middle of the
	// wall, Angle how far down from the top it reaches (90: a hemisphere)
	register(&Kind{ID: "sphereshell", Label: "Spherical Shell", Color: "#e0218a", Size: geom.V(21, 21, 10.5),
		Params: []Param{
			slider("angle", "Angle", 1, 180, 1, 90, false),
			slider("steps", "Steps", 3, 64, 1, 18, true),
			lslider("radius", "Radius", 1, 100, 0.5, 10, false),
			lslider("thickness", "Thickness", 0.1, 20, 0.1, 1, false),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			ri, ro := shellRadii(v.Get("radius", 10), v.Get("thickness", 1))
			return sphericalShell(ri, ro, shellAngle(v.Get("angle", 90)), int(v.Get("steps", 18))).FitTo(s)
		},
		natural: func(v Values) geom.Vec3 {
			ri, ro := shellRadii(v.Get("radius", 10), v.Get("thickness", 1))
			return shellSize(ri, ro, shellAngle(v.Get("angle", 90)))
		}})
	// shape generator: Height, Outer Radius, Inner Radius (mm) and Sides
	register(&Kind{ID: "hyperboloid", Label: "Hyperboloid", Color: "#3fae2a", Size: geom.V(20, 20, 20),
		Params: []Param{
			lslider("height", "Height", 1, 200, 1, 20, false),
			lslider("outer", "Outer Radius", 1, 100, 0.5, 10, false),
			lslider("inner", "Inner Radius", 0.5, 100, 0.5, 5, false),
			slider("sides", "Sides", 3, 64, 1, 20, true),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			return hyperboloid(v.Get("outer", 10), v.Get("inner", 5), int(v.Get("sides", 20))).FitTo(s)
		},
		natural: func(v Values) geom.Vec3 {
			d := 2 * v.Get("outer", 10)
			return geom.V(d, d, v.Get("height", 20))
		}})
}

func icosahedron() *geom.Mesh {
	t := (1 + math.Sqrt(5)) / 2
	v := []geom.Vec3{geom.V(-1, t, 0), geom.V(1, t, 0), geom.V(-1, -t, 0), geom.V(1, -t, 0), geom.V(0, -1, t), geom.V(0, 1, t), geom.V(0, -1, -t), geom.V(0, 1, -t), geom.V(t, 0, -1), geom.V(t, 0, 1), geom.V(-t, 0, -1), geom.V(-t, 0, 1)}
	f := [][3]int{{0, 11, 5}, {0, 5, 1}, {0, 1, 7}, {0, 7, 10}, {0, 10, 11}, {1, 5, 9}, {5, 11, 4}, {11, 10, 2}, {10, 7, 6}, {7, 1, 8}, {3, 9, 4}, {3, 4, 2}, {3, 2, 6}, {3, 6, 8}, {3, 8, 9}, {4, 9, 5}, {2, 4, 11}, {6, 2, 10}, {8, 6, 7}, {9, 8, 1}}
	m := &geom.Mesh{}
	for _, x := range f {
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{v[x[0]], v[x[1]], v[x[2]]}})
	}
	return m.Oriented()
}
