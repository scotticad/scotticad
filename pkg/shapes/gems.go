package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// starOutline is a star of n points, outer radius r and inner radius
// r·inner, with its first point on +Y.
func starOutline(n int, r, inner float64) []Pt {
	n = max(n, 3)
	var prof []Pt
	for i := 0; i < 2*n; i++ {
		rr := r
		if i%2 == 1 {
			rr *= inner
		}
		a := math.Pi/2 + float64(i)*math.Pi/float64(n)
		prof = append(prof, Pt{rr * math.Cos(a), rr * math.Sin(a)})
	}
	return prof
}

func outlineSize(prof []Pt) (w, d float64) {
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range prof {
		minX, maxX = math.Min(minX, p.X), math.Max(maxX, p.X)
		minY, maxY = math.Min(minY, p.Y), math.Max(maxY, p.Y)
	}
	return maxX - minX, maxY - minY
}

// starPyramid is a faceted star: a flat star-shaped base with every edge
// rising to an apex over the centre, height h.
func starPyramid(n int, inner, h float64) *geom.Mesh {
	prof := starOutline(n, 1, inner)
	apex := geom.V(0, 0, h)
	c := geom.V(0, 0, 0)
	m := &geom.Mesh{}
	for i := range prof {
		a := geom.V(prof[i].X, prof[i].Y, 0)
		b := geom.V(prof[(i+1)%len(prof)].X, prof[(i+1)%len(prof)].Y, 0)
		m.Tris = append(m.Tris,
			geom.Tri{P: [3]geom.Vec3{a, b, apex}}, // side, outward (profile is counter-clockwise)
			geom.Tri{P: [3]geom.Vec3{c, b, a}},    // base, facing down
		)
	}
	return m.Clean().Oriented()
}

// dice is a cube rounded by a sphere (their intersection), like a die.
func dice() *geom.Mesh {
	cube := extrude([]Pt{{-0.5, -0.5}, {0.5, -0.5}, {0.5, 0.5}, {-0.5, 0.5}})
	ball := revolve(arc(0, 0, 0.72, -math.Pi/2, math.Pi/2, 24), 48, false)
	return geom.Intersect(cube, ball).FixTJunctions()
}

// diamond is an eight-sided brilliant: a point at the bottom, the girdle
// 70% of the way up, and a flat table on top.
func diamond() *geom.Mesh {
	prof := [][2]float64{{0, -0.5}, {0.5, 0.2}, {0.28, 0.5}, {0, 0.5}}
	return revolve(prof, 8, false).Transform(geom.RotateZ(math.Pi / 8))
}

// hyperboloid is a hyperboloid of one sheet: radius outer at both ends,
// narrowing to inner at mid-height (r² = inner² + (outer² − inner²)·t²,
// t from −1 to 1), with the given number of sides; unit height.
func hyperboloid(outer, inner float64, sides int) *geom.Mesh {
	inner = math.Min(math.Max(inner, 0.01), outer)
	const rows = 24
	prof := [][2]float64{{0, -0.5}}
	for i := 0; i <= rows; i++ {
		t := -1 + 2*float64(i)/rows
		r := math.Sqrt(inner*inner + (outer*outer-inner*inner)*t*t)
		prof = append(prof, [2]float64{r, t / 2})
	}
	prof = append(prof, [2]float64{0, 0.5})
	return revolve(prof, max(sides, 3), false)
}

// shellRadii returns a spherical shell's inner and outer radii: radius is
// the middle of the wall, thickness its width.
func shellRadii(radius, thickness float64) (ri, ro float64) {
	t := math.Min(math.Max(thickness, 0.01), 2*radius*0.98)
	return radius - t/2, radius + t/2
}

// shellAngle clamps the shell's opening angle (degrees, measured from the
// top: 90 is a hemisphere, 180 a closed hollow ball) to what can be built.
func shellAngle(deg float64) float64 { return math.Min(math.Max(deg, 1), 179.5) * math.Pi / 180 }

// sphericalShell is an open spherical shell in mm: the part of a hollow
// ball within angle a of its top, wall from ri to ro, steps segments
// down the arc (and twice that around).
func sphericalShell(ri, ro, a float64, steps int) *geom.Mesh {
	steps = max(steps, 2)
	prof := arc(0, 0, ri, math.Pi/2, math.Pi/2-a, steps)
	prof = append(prof, arc(0, 0, ro, math.Pi/2-a, math.Pi/2, steps)...)
	return revolve(prof, 2*steps, false).Clean().Oriented()
}

// shellSize is the bounding size of that shell.
func shellSize(ri, ro, a float64) geom.Vec3 {
	w := 2 * ro
	if a < math.Pi/2 {
		w = 2 * ro * math.Sin(a)
	}
	return geom.V(w, w, ro-math.Min(ro*math.Cos(a), ri*math.Cos(a)))
}

// gearOutline is a gear with round (non-involute) teeth: around the pitch
// circle (radius teeth·pitch/π), bumps of radius pitch/2 alternate with
// notches of the same radius, each made smaller/larger by slop so mating
// gears have clearance. The first tooth is on +X.
func gearOutline(pitch, slop float64, teeth int) []Pt {
	teeth = max(teeth, 3)
	pitch = math.Max(pitch, 0.1)
	R := float64(teeth) * pitch / math.Pi
	// a notch reaching the centre would fold the outline through it: the
	// root circle stays at least a quarter of the pitch circle
	slop = math.Min(slop, math.Max(0, 0.75*R-pitch/2))
	rb := math.Max(pitch/2-slop, 0.01) // bump
	rn := pitch/2 + slop               // notch
	half := math.Pi / float64(teeth)   // tooth to notch
	const per = 12                     // samples per half tooth
	var out []Pt
	for i := 0; i < 2*teeth*per; i++ {
		phi := 2 * math.Pi * float64(i) / float64(2*teeth*per)
		k := math.Round(phi / half) // nearest bump/notch centre
		d := phi - k*half           // offset from it
		sn, cs := math.Sin(d)*R, math.Cos(d)*R
		var r float64
		if int(k)%2 == 0 {
			r = cs + math.Sqrt(math.Max(0, rb*rb-sn*sn)) // bump: far side
		} else {
			r = cs - math.Sqrt(math.Max(0, rn*rn-sn*sn)) // notch: near side
		}
		out = append(out, Pt{r * math.Cos(phi), r * math.Sin(phi)})
	}
	return out
}

// gear is gearOutline extruded to unit height with a round bore through
// its centre (bore ≤ 0: none).
func gear(pitch, slop float64, teeth int, bore float64) *geom.Mesh {
	contours := [][]Pt{gearOutline(pitch, slop, teeth)}
	if bore > 0 {
		var hole []Pt
		for i := 0; i < 48; i++ {
			a := -2 * math.Pi * float64(i) / 48 // clockwise: a hole
			hole = append(hole, Pt{bore / 2 * math.Cos(a), bore / 2 * math.Sin(a)})
		}
		contours = append(contours, hole)
	}
	return ExtrudeContours(contours, -0.5, 0.5)
}
