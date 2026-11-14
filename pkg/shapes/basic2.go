package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// More basic shapes: the slot, arc, fillet, teardrop and crystal
// (registered in zz_added.go).

// --- slot ------------------------------------------------------------------------------

// slotOutline is a stadium l × w centred on the origin, counter-clockwise:
// a rectangle whose short ends are full half circles, each of segs
// segments (even, so each end's tip is a point and the outline fills its
// box exactly).
func slotOutline(l, w float64, segs int) []Pt {
	if w > l { // long along Y: make it along X, then turn it
		out := slotOutline(w, l, segs)
		for i, p := range out {
			out[i] = Pt{-p.Y, p.X}
		}
		return out
	}
	segs = max(2, segs+segs%2)
	r := w / 2
	s := l/2 - r // half the straight run
	var out []Pt
	for i := 0; i <= segs; i++ { // the +X end, from its bottom to its top
		a := -math.Pi/2 + math.Pi*float64(i)/float64(segs)
		out = append(out, Pt{s + r*math.Cos(a), r * math.Sin(a)})
	}
	for i := 0; i <= segs; i++ { // the -X end, from its top to its bottom
		if s < 1e-9 && (i == 0 || i == segs) {
			continue // a circle: the ends meet
		}
		a := math.Pi/2 + math.Pi*float64(i)/float64(segs)
		out = append(out, Pt{-s + r*math.Cos(a), r * math.Sin(a)})
	}
	return out
}

func slotMesh(s geom.Vec3, v Values) *geom.Mesh {
	return ExtrudeContours([][]Pt{slotOutline(s.X, s.Y, int(v.Get("sides", 48))/2)}, -s.Z/2, s.Z/2)
}

// --- arc -------------------------------------------------------------------------------

// arcModel is part of a ring (or, with no wall, a pie slice): outer radius
// Radius, Wall Thickness in from it, from +X counter-clockwise through
// Angle degrees, Height tall.
func arcModel(v Values) *geom.Mesh {
	R := v.Get("radius", 20)
	wall := v.Get("wall", 5)
	ang := math.Max(1, math.Min(v.Get("angle", 90), 360))
	h := v.Get("height", 5)
	sides := int(v.Get("sides", 96))
	ri := 0.0
	if wall > 0 && wall < 0.98*R {
		ri = R - wall
	}
	if ang > 359.999 {
		cs := [][]Pt{circlePts(0, 0, R, sides)}
		if ri > 0 {
			cs = append(cs, reversed(circlePts(0, 0, ri, sides)))
		}
		return ExtrudeContours(cs, 0, h)
	}
	a := ang * math.Pi / 180
	n := max(2, int(math.Ceil(float64(sides)*ang/360)))
	var out []Pt
	for i := 0; i <= n; i++ {
		t := a * float64(i) / float64(n)
		out = append(out, Pt{R * math.Cos(t), R * math.Sin(t)})
	}
	if ri == 0 {
		out = append(out, Pt{0, 0})
	} else {
		for i := n; i >= 0; i-- {
			t := a * float64(i) / float64(n)
			out = append(out, Pt{ri * math.Cos(t), ri * math.Sin(t)})
		}
	}
	return ExtrudeContours([][]Pt{out}, 0, h)
}

// --- fillet ----------------------------------------------------------------------------

// outlineToYZ turns a prism built along Z (outline in X-Y) so it lies
// along X, its outline's X becoming Y and its Y becoming Z.
var outlineToYZ = geom.Mat4{0, 1, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 1}

// filletMesh is a bar along X whose section is a square with a quarter
// circle taken out of its top front corner: the piece that rounds an
// inside corner, solid against the floor (-Z) and a wall behind it (+Y;
// the front is -Y), its curved face up and toward the front. The size
// stretches the curve to an ellipse.
func filletMesh(s geom.Vec3, v Values) *geom.Mesh {
	n := max(2, int(v.Get("steps", 16)))
	prof := []Pt{{-0.5, -0.5}, {0.5, -0.5}}
	for i := 1; i <= n; i++ {
		a := 1.5*math.Pi - 0.5*math.Pi*float64(i)/float64(n)
		x, y := 0.5+math.Cos(a), 0.5+math.Sin(a)
		if i == n {
			x, y = -0.5, 0.5
		}
		prof = append(prof, Pt{x, y})
	}
	// the outline's tall side is on -Y: turned half round, it is at the back
	return extrude(prof).Transform(outlineToYZ).Transform(geom.RotateZ(math.Pi)).FitTo(s)
}

// --- teardrop --------------------------------------------------------------------------

// outlineToXZ turns a prism built along Z (outline in X-Y) so it lies along
// Y, its outline's Y becoming Z.
var outlineToXZ = geom.Mat4{1, 0, 0, 0, 0, 0, 1, 0, 0, -1, 0, 0, 0, 0, 0, 1}

// teardropModel is a round bar lying along Y whose top comes to a point
// at 45°, so as a hole it prints without support; Flat Top cuts the point
// off level with the top of the circle.
func teardropModel(v Values) *geom.Mesh {
	r := v.Get("diameter", 10) / 2
	l := v.Get("length", 20)
	// steps of at most 45° divide the arc at the sides and the bottom
	k := max(1, int(math.Round(v.Get("sides", 48)*0.75/6)))
	n := 6 * k
	var prof []Pt
	for i := 0; i <= n; i++ { // 135° round the bottom to 45°
		a := 0.75*math.Pi + 1.5*math.Pi*float64(i)/float64(n)
		prof = append(prof, Pt{r * math.Cos(a), r * math.Sin(a)})
	}
	if v.Get("flat", 0) >= 0.5 {
		x := r * (math.Sqrt2 - 1)
		prof = append(prof, Pt{x, r}, Pt{-x, r})
	} else {
		prof = append(prof, Pt{0, r * math.Sqrt2})
	}
	return ExtrudeContours([][]Pt{prof}, -l/2, l/2).Transform(outlineToXZ)
}

// --- crystal ---------------------------------------------------------------------------

// crystalRand is a small random number generator (SplitMix64): the same
// seed gives the same crystal on every machine and every Go version.
type crystalRand uint64

// next is a number in [-0.5, 0.5).
func (r *crystalRand) next() float64 {
	*r += 0x9E3779B97F4A7C15
	z := uint64(*r)
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z ^= z >> 31
	return float64(z>>11)/(1<<53) - 0.5
}

// crystalMesh is a quartz-like crystal in a unit box, fitted to s: a
// column of Sides sides ending in a point (or one at each end). Point
// Height is the point's share of the height; Taper narrows the column's
// foot; Irregularity, with Seed, moves the corners, the shoulders and the
// tip so no two crystals match.
func crystalMesh(s geom.Vec3, v Values) *geom.Mesh {
	n := int(v.Get("sides", 6))
	// the shoulders vary by up to a quarter of the point's height either
	// way: kept off the foot, and (double pointed) off each other
	pt := math.Min(v.Get("point", 30)/100, 0.75)
	double := v.Get("double", 0) >= 0.5
	if double {
		pt = math.Min(pt, 0.35)
	}
	foot := 1 - v.Get("taper", 0)/100
	irr := v.Get("irregular", 0) / 100
	rng := crystalRand(uint64(v.Get("seed", 1)))
	// the corners: each its own direction and reach
	angs, reach := make([]float64, n), make([]float64, n)
	for i := range angs {
		angs[i] = 2*math.Pi*float64(i)/float64(n) + irr*0.5*(math.Pi/float64(n))*rng.next()
		reach[i] = 0.5 * (1 + irr*0.35*rng.next())
	}
	ring := func(r, z float64, i int) geom.Vec3 {
		return geom.V(r*reach[i]*math.Cos(angs[i]), r*reach[i]*math.Sin(angs[i]), z)
	}
	shoulder := func(base float64) []float64 { // each corner's height, about base
		zs := make([]float64, n)
		for i := range zs {
			zs[i] = base + irr*pt*0.5*rng.next()
		}
		return zs
	}
	tip := func(z float64) geom.Vec3 { return geom.V(irr*0.3*rng.next(), irr*0.3*rng.next(), z) }

	topZ := shoulder(0.5 - pt)
	apex := tip(0.5)
	top, bot := make([]geom.Vec3, n), make([]geom.Vec3, n)
	for i := range top {
		top[i] = ring(1, topZ[i], i)
	}
	var low geom.Vec3
	if double {
		botZ := shoulder(-0.5 + pt)
		for i := range bot {
			bot[i] = ring(foot, botZ[i], i)
		}
		low = tip(-0.5)
	} else {
		for i := range bot {
			bot[i] = ring(foot, -0.5, i)
		}
		low = geom.V(0, 0, -0.5)
	}
	m := &geom.Mesh{}
	tri := func(a, b, c geom.Vec3) { m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{a, b, c}}) }
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		tri(bot[i], bot[j], top[j]) // the column
		tri(bot[i], top[j], top[i])
		tri(top[i], top[j], apex) // the point
		tri(bot[j], bot[i], low)  // the foot, or the other point
	}
	return m.Clean().Oriented().FitTo(s)
}
