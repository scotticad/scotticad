package shapes

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/scotticad/scotticad/pkg/geom"
)

// A bent tube is a round tube made the way a tube bender makes one: from a
// bend table. Each row is a straight run, then a bend through an angle
// round the bend radius; the bend's plane can first be turned about the
// tube, so a tube can leave the plane it started in. The tube starts at
// the origin heading along +X and, unturned, bends toward +Y, so a table
// with no turns lies flat on the workplane.
//
// The tube's diameter, wall, bend radius and number of sides are ordinary
// parameters; the table is the shape's point data, like an outline's.

// Bend is one row of a bent tube's bend table: a straight run of Length
// mm, then a bend of Angle degrees (negative bends the other way), its
// plane first turned Turn degrees about the tube from the last bend's.
type Bend struct {
	Length float64 `json:"length"`
	Angle  float64 `json:"angle"`
	Turn   float64 `json:"turn,omitempty"`
}

// Limits on a bend table.
const (
	maxBends      = 200
	maxBendLength = 1e5 // mm
	tubeBendRoom  = 0.1 // mm, between the inside of a bend and its centre
)

// IsTube reports whether a kind is built from a bend table.
func IsTube(kind string) bool { return kind == "benttube" }

// HasFrame reports a kind built from point data (an outline, a loft or a
// bent tube): its data is in the shape's own frame, which changing the
// data keeps. An arc has one too, with its circle's centre at the origin:
// changing its angle or radius keeps its centre where it is.
func HasFrame(kind string) bool { return IsProfile(kind) || IsTube(kind) || kind == "arc" }

// CheckBends reports what is wrong with a bend table, if anything, in
// words fit to show a script's author.
func CheckBends(bs []Bend) error {
	if len(bs) == 0 {
		return errors.New("a bent tube needs at least one row")
	}
	if len(bs) > maxBends {
		return fmt.Errorf("too many bends (more than %d)", maxBends)
	}
	total := 0.0
	for i, b := range bs {
		if !finite(b.Length) || b.Length < 0 || b.Length > maxBendLength {
			return fmt.Errorf("row %d: length %v must be from 0 to %v", i+1, b.Length, maxBendLength)
		}
		if !finite(b.Angle) || math.Abs(b.Angle) > 180 {
			return fmt.Errorf("row %d: angle %v must be from -180 to 180", i+1, b.Angle)
		}
		if !finite(b.Turn) || math.Abs(b.Turn) > 360 {
			return fmt.Errorf("row %d: turn %v must be from -360 to 360", i+1, b.Turn)
		}
		total += b.Length + math.Abs(b.Angle)
	}
	if total <= 0 {
		return errors.New("a bent tube needs some length or a bend")
	}
	return nil
}

// BendsKey is a short text key for a bend table, for cache keys.
func BendsKey(bs []Bend) string {
	if len(bs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, b := range bs {
		fmt.Fprintf(&sb, "%g,%g,%g;", b.Length, b.Angle, b.Turn)
	}
	return sb.String()
}

// TubeStation is a point on a tube's centreline with the tube's direction
// there (T) and two axes square to it (U, W) that the round section is
// drawn in. The axes are carried along without twisting.
type TubeStation struct {
	P, T, U, W geom.Vec3
}

// rotate turns v about the unit axis k by a radians.
func rotate(v, k geom.Vec3, a float64) geom.Vec3 {
	c, s := math.Cos(a), math.Sin(a)
	return v.Mul(c).Add(k.Cross(v).Mul(s)).Add(k.Mul(k.Dot(v) * (1 - c)))
}

// TubePath returns the centreline of a bend table at bend radius r: the
// stations along it, and the index of the station at the end of each
// row (where its bend, if any, ends).
func TubePath(bs []Bend, r float64) ([]TubeStation, []int) {
	st := TubeStation{P: geom.V(0, 0, 0), T: geom.V(1, 0, 0), U: geom.V(0, 0, 1), W: geom.V(0, -1, 0)}
	toward := geom.V(0, 1, 0) // which way the next bend goes, before its turn
	out := []TubeStation{st}
	var ends []int
	for _, b := range bs {
		if b.Length > 1e-9 {
			st.P = st.P.Add(st.T.Mul(b.Length))
			out = append(out, st)
		}
		toward = rotate(toward, st.T, b.Turn*math.Pi/180)
		if a := b.Angle * math.Pi / 180; math.Abs(a) > 1e-9 {
			dir := toward
			if a < 0 {
				dir, a = dir.Neg(), -a
			}
			centre := st.P.Add(dir.Mul(r))
			axis := st.T.Cross(dir).Norm()
			n := int(math.Ceil(a / (math.Pi / 32))) // at most 5.6 degrees a step
			p0, t0, u0, w0, tw0 := st.P, st.T, st.U, st.W, toward
			for i := 1; i <= n; i++ {
				f := a * float64(i) / float64(n)
				st = TubeStation{
					P: centre.Add(rotate(p0.Sub(centre), axis, f)),
					T: rotate(t0, axis, f), U: rotate(u0, axis, f), W: rotate(w0, axis, f),
				}
				out = append(out, st)
			}
			toward = rotate(tw0, axis, a)
		}
		ends = append(ends, len(out)-1)
	}
	return out, ends
}

// tubeDims reads a bent tube's outer and inner radius (inner 0: solid),
// its bend radius and its number of sides. The bend radius is kept
// tubeBendRoom more than the outer radius: less, and the inside of a bend
// folds through itself; equal, it pinches to a point, where the triangles
// have no area and the exported surface cracks.
func tubeDims(v Values) (ro, ri, r float64, sides int) {
	ro = math.Max(0.05, v.Get("diameter", 10)/2)
	if w := v.Get("wall", 0); w > 0 && w < ro-0.05 {
		ri = ro - w
	}
	r = math.Max(v.Get("bend", 15), ro+tubeBendRoom)
	sides = int(math.Max(3, math.Min(256, math.Round(v.Get("sides", 32)))))
	return
}

// TubeRadii returns a bent tube's outer and inner radius (inner 0: solid)
// and the bend radius its centreline is built with.
func TubeRadii(v Values) (outer, inner, bend float64) {
	ro, ri, r, _ := tubeDims(v)
	return ro, ri, r
}

// tubeMesh sweeps the tube's round section along its centreline, or
// returns nil for a table it can't use.
func tubeMesh(v Values) *geom.Mesh {
	if CheckBends(v.Bends) != nil {
		return nil
	}
	ro, ri, r, sides := tubeDims(v)
	path, _ := TubePath(v.Bends, r)
	if len(path) < 2 {
		return nil
	}
	ring := func(s TubeStation, rad float64) []geom.Vec3 {
		pts := make([]geom.Vec3, sides)
		for j := range pts {
			// a corner on U, so a tube lying flat is exactly its diameter tall
			f := 2 * math.Pi * float64(j) / float64(sides)
			pts[j] = s.P.Add(s.U.Mul(rad * math.Cos(f))).Add(s.W.Mul(rad * math.Sin(f)))
		}
		return pts
	}
	m := &geom.Mesh{}
	tri := func(a, b, c geom.Vec3) { m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{a, b, c}}) }
	wall := func(rad float64, inward bool) {
		prev := ring(path[0], rad)
		for i := 1; i < len(path); i++ {
			cur := ring(path[i], rad)
			for j := 0; j < sides; j++ {
				k := (j + 1) % sides
				if inward {
					tri(prev[j], cur[k], prev[k])
					tri(prev[j], cur[j], cur[k])
				} else {
					tri(prev[j], prev[k], cur[k])
					tri(prev[j], cur[k], cur[j])
				}
			}
			prev = cur
		}
	}
	wall(ro, false)
	if ri > 0 {
		wall(ri, true)
	}
	capEnd := func(s TubeStation, start bool) {
		o := ring(s, ro)
		if ri > 0 {
			in := ring(s, ri)
			for j := 0; j < sides; j++ {
				k := (j + 1) % sides
				if start {
					tri(o[j], in[k], o[k])
					tri(o[j], in[j], in[k])
				} else {
					tri(o[j], o[k], in[k])
					tri(o[j], in[k], in[j])
				}
			}
			return
		}
		for j := 0; j < sides; j++ {
			k := (j + 1) % sides
			if start {
				tri(s.P, o[k], o[j])
			} else {
				tri(s.P, o[j], o[k])
			}
		}
	}
	capEnd(path[0], true)
	capEnd(path[len(path)-1], false)
	return m
}

// TubeBounds is the box a bent tube fills, in the coordinates of its
// table (the tube starting at the origin); empty for an unusable table.
func TubeBounds(v Values) geom.Box {
	m := tubeMesh(v)
	if m == nil {
		return geom.EmptyBox()
	}
	return m.Bounds()
}

// FrameBounds is the box a kind with point data fills in its own frame
// (see HasFrame).
func FrameBounds(kind string, v Values) geom.Box {
	if IsTube(kind) {
		return TubeBounds(v)
	}
	if kind == "arc" {
		return arcModel(catalogue["arc"].clamp(v)).Bounds()
	}
	return ProfileBounds(kind, v)
}

var tubeNatural sync.Map

func tubeKey(v Values) string {
	ro, ri, r, sides := tubeDims(v)
	return fmt.Sprintf("%v|%v|%v|%d|%s", ro, ri, r, sides, BendsKey(v.Bends))
}

// the default tube: an L, lying on the workplane
var defaultBends = []Bend{{Length: 20, Angle: 90}, {Length: 20}}

func init() {
	k := &Kind{ID: "benttube", Label: "Bent Tube", Color: "#c0392b", bends: defaultBends, Params: []Param{
		lslider("diameter", "Outer Dia", 1, 50, 0.5, 10, false),
		lslider("wall", "Wall (0 is solid)", 0, 20, 0.1, 0, false),
		lslider("bend", "Bend Radius", 0.5, 200, 0.5, 15, false),
		slider("sides", "Sides", 8, 64, 1, 32, true),
	}}
	k.build = func(size geom.Vec3, v Values) *geom.Mesh {
		m := tubeMesh(v)
		if m == nil || len(m.Tris) == 0 {
			return roundedBox(size, 0, 1)
		}
		return m.FitTo(size)
	}
	k.natural = func(v Values) geom.Vec3 {
		key := tubeKey(v)
		if s, ok := tubeNatural.Load(key); ok {
			return s.(geom.Vec3)
		}
		s := geom.V(1, 1, 1)
		if b := TubeBounds(v); !b.IsEmpty() {
			s = b.Size()
		}
		tubeNatural.Store(key, s)
		return s
	}
	k.Size = k.natural(k.Defaults())
	register(k)
	// in the Mechanical library, after the pipe elbow
	for i, id := range MechanicalKinds {
		if id == "elbow" {
			MechanicalKinds = append(MechanicalKinds[:i+1], append([]string{"benttube"}, MechanicalKinds[i+1:]...)...)
			return
		}
	}
	MechanicalKinds = append(MechanicalKinds, "benttube")
}
