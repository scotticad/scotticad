package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Straight bevel gears (registered in zz_added.go): gears on cones, for
// turning a drive through a right angle. Two bevel gears with the same
// module mesh when each is made with the other's tooth count as its Mate
// Teeth; their shafts then cross at 90°.
//
// The teeth follow Tredgold's approximation, as bevel gears are usually
// cut: a tooth's profile at the gear's large end is the profile of a spur
// gear on the back cone (the cone square to the pitch cone there), which
// has z / cos δ teeth, δ being the pitch cone's angle. Every tooth face is
// a cone through the pitch cone's apex. The gear is built from that solid
// cut square to its axis at both ends: at the large end (the back, flat,
// where it is printed from) and Face Width further in along the pitch
// cone. Below the back an optional plain disc stiffens it.

// bevelGeom is a bevel gear's layout: pitch radius at the back, the pitch
// cone's apex height above the back, the cone distance and angle.
type bevelGeom struct {
	m             float64
	z             int
	R, apex, A, d float64
}

func bevelOf(m float64, z, zm int) bevelGeom {
	d := math.Atan2(float64(z), float64(zm))
	R := m * float64(z) / 2
	return bevelGeom{m: m, z: z, R: R, apex: m * float64(zm) / 2, A: math.Hypot(R, m*float64(zm)/2), d: d}
}

// backOutline is the gear's toothed outline on its flat back,
// counter-clockwise, first tooth on +X, each tooth face cut back by clr
// (square to the face) for printing. The back is the plane through the
// tooth tips' outer ends, zb above the pitch circle: there the tips are
// exactly as the back cone has them and the roots a little in from it.
// (A back through the pitch circle would carry every tip on past the back
// cone, making the teeth look and print too deep.)
func (g bevelGeom) backOutline(clr float64) (out []Pt, zb float64) {
	zv := float64(g.z) / math.Cos(g.d) // the back cone's spur gear
	Rv := g.R / math.Cos(g.d)
	rb := Rv * math.Cos(gearAlpha)
	ra, rf := Rv+g.m, Rv-1.25*g.m
	inv := func(rho float64) float64 {
		if rho <= rb {
			return 0
		}
		t := math.Sqrt(rho*rho/(rb*rb) - 1)
		return t - math.Atan(t)
	}
	invA := math.Tan(gearAlpha) - gearAlpha
	r0 := math.Max(rb, rf)
	cut := clr / rb
	half := func(rho float64) float64 { return math.Pi/(2*zv) + invA - inv(rho) - cut }
	// as toothedOutline: at least 0.2 mm of gap at the root and of land at the tip
	if cut != 0 {
		cut = math.Max(cut, math.Pi/(2*zv)+invA-inv(r0)-math.Pi/zv+0.1/r0)
		if half(r0)*r0 < 0.1 {
			cut -= 0.1/r0 - half(r0)
		}
		if half(ra)*ra < 0.1 {
			lo, hi := r0, ra
			for i := 0; i < 40; i++ {
				if mid := (lo + hi) / 2; half(mid)*mid < 0.1 {
					hi = mid
				} else {
					lo = mid
				}
			}
			ra = lo
		}
	}
	// a point (rho, phi) of the back cone's flat development is on the
	// back cone at radius rho·cos δ, height (rho − Rv)·sin δ above the
	// pitch circle, turned by phi·zv/z round the axis; seen from the apex,
	// it is on the back where the line from the apex through it meets it
	sd, cd := math.Sincos(g.d)
	zb = (ra - Rv) * sd
	onBack := func(rho, phi float64) Pt {
		h := (rho - Rv) * sd
		s := (g.apex - zb) / (g.apex - h)
		th := phi * zv / float64(g.z)
		return Pt{s * rho * cd * math.Cos(th), s * rho * cd * math.Sin(th)}
	}
	const fl = 10
	for k := 0; k < g.z; k++ {
		c := 2 * math.Pi * float64(k) / zv
		if rf < rb {
			out = append(out, onBack(rf, c-half(rb)))
		}
		for i := 0; i <= fl; i++ {
			rho := r0 + (ra-r0)*float64(i)/fl
			out = append(out, onBack(rho, c-half(rho)))
		}
		for i := 1; i < 4; i++ {
			out = append(out, onBack(ra, c-half(ra)+2*half(ra)*float64(i)/4))
		}
		for i := fl; i >= 0; i-- {
			rho := r0 + (ra-r0)*float64(i)/fl
			out = append(out, onBack(rho, c+half(rho)))
		}
		if rf < rb {
			out = append(out, onBack(rf, c+half(rb)))
		}
		next := c + 2*math.Pi/zv
		a0, a1 := c+half(r0), next-half(r0)
		for i := 1; i < 4; i++ {
			out = append(out, onBack(rf, a0+(a1-a0)*float64(i)/4))
		}
	}
	return out, zb
}

// coneExtrude makes the solid between outline outer on z = 0 and the same
// outline shrunk towards the point (0, 0, apex) on z = h, with holes
// (clockwise) that go straight through.
func coneExtrude(outer []Pt, holes [][]Pt, h, apex float64) *geom.Mesh {
	s := (apex - h) / apex
	top := make([]Pt, len(outer))
	for i, p := range outer {
		top[i] = Pt{p.X * s, p.Y * s}
	}
	m := &geom.Mesh{}
	quad := func(a0, a1, b1, b0 geom.Vec3) {
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{a0, a1, b1}}, geom.Tri{P: [3]geom.Vec3{a0, b1, b0}})
	}
	for i := range outer {
		j := (i + 1) % len(outer)
		quad(geom.V(outer[i].X, outer[i].Y, 0), geom.V(outer[j].X, outer[j].Y, 0), geom.V(top[j].X, top[j].Y, h), geom.V(top[i].X, top[i].Y, h))
	}
	for _, c := range holes {
		for i := range c {
			j := (i + 1) % len(c)
			quad(geom.V(c[i].X, c[i].Y, 0), geom.V(c[j].X, c[j].Y, 0), geom.V(c[j].X, c[j].Y, h), geom.V(c[i].X, c[i].Y, h))
		}
	}
	for _, t := range Triangulate(Region{Outer: outer, Holes: holes}) {
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{geom.V(t[0].X, t[0].Y, 0), geom.V(t[2].X, t[2].Y, 0), geom.V(t[1].X, t[1].Y, 0)}})
	}
	for _, t := range Triangulate(Region{Outer: top, Holes: holes}) {
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{geom.V(t[0].X, t[0].Y, h), geom.V(t[1].X, t[1].Y, h), geom.V(t[2].X, t[2].Y, h)}})
	}
	return done(m)
}

// bevelGear is the gear: its flat back on z = 0, teeth rising towards
// the pitch cone's apex (which is apex above the back), a bore, and Back
// mm of plain disc under the back.
func bevelGear(m float64, z, zm int, face, bore, back, clr float64) (gear *geom.Mesh, apex float64) {
	g := bevelOf(m, z, zm)
	face = math.Max(0.5, math.Min(face, g.A/3)) // longer, the small end's teeth get too thin
	outer, zb := g.backOutline(clr)
	apex = g.apex - zb
	h := face * math.Cos(g.d)
	rmin := math.Inf(1)
	for _, p := range outer {
		rmin = math.Min(rmin, math.Hypot(p.X, p.Y))
	}
	small := rmin * (apex - h) / apex // the roots at the small end
	var holes [][]Pt
	if bore > 0 && bore/2 < small-0.8 {
		holes = append(holes, reversed(circlePts(0, 0, bore/2, 48)))
	}
	gear = coneExtrude(outer, holes, h, apex)
	if back < 0.05 {
		return gear, apex
	}
	cs := [][]Pt{circlePts(0, 0, rmin, max(96, 4*z))}
	cs = append(cs, holes...)
	return done(geom.Union(gear, ExtrudeContours(cs, -back, 0))), apex
}

func bevelValues(v Values) (m float64, z, zm int, face, bore, back, clr float64) {
	return v.Get("module", 1.5), max(int(v.Get("teeth", 20)), 8), max(int(v.Get("mate", 20)), 8),
		v.Get("face", 6), v.Get("bore", 5), v.Get("back", 2), v.Get("clearance", 0.1)
}

func bevelGearModel(v Values) *geom.Mesh {
	m, z, zm, face, bore, back, clr := bevelValues(v)
	g, _ := bevelGear(m, z, zm, face, bore, back, clr)
	return g
}

// bevelPair is the two gears of a pair in mesh, in the pair's frame: the
// first on the plate with its axis up, the second (Mate Teeth teeth) on
// its side to the +X, its axis along X, a tooth space turned to meet the
// first gear's tooth. Both gears' pitch cones share their apex.
// apexZ is the shared apex's height.
func bevelPair(v Values) (first, second *geom.Mesh, apexZ float64) {
	m, z, zm, face, bore, back, clr := bevelValues(v)
	first, a1 := bevelGear(m, z, zm, face, bore, back, clr)
	first = first.Transform(geom.Translate(geom.V(0, 0, back)))
	phase := 0.0
	if zm%2 == 0 {
		phase = math.Pi / float64(zm)
	}
	// its axis (local Z) points along -X, from its back to the apex; its
	// local +X is up, so a tooth space is down when local -X is one
	second, a2 := bevelGear(m, zm, z, face, bore, back, clr)
	apexZ = back + a1
	second = second.Transform(geom.Translate(geom.V(a2, 0, apexZ)).Mul(geom.RotateY(-math.Pi / 2)).Mul(geom.RotateZ(phase)))
	if lo := second.Bounds().Min.Z; lo < 0 { // keep the pair on the plate
		up := geom.Translate(geom.V(0, 0, -lo))
		first, second, apexZ = first.Transform(up), second.Transform(up), apexZ-lo
	}
	return first, second, apexZ
}

func bevelPairFirst(v Values) *geom.Mesh  { f, _, _ := bevelPair(v); return f }
func bevelPairSecond(v Values) *geom.Mesh { _, s, _ := bevelPair(v); return s }

func bevelPairParts(v Values) []Placed {
	return []Placed{
		placedAsBuilt("bevelpair-gear", "Bevel Gear", "#c8ccd0", v, bevelPairFirst, false),
		placedAsBuilt("bevelpair-mate", "Mating Bevel Gear", "#a7b0b8", v, bevelPairSecond, false),
	}
}
