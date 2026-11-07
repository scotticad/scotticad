package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Involute gears: spur, helical and herringbone gears, internal (ring)
// gears, and a print-in-place planetary set.
//
// Every gear here uses a 20° pressure angle and a transverse module (the
// module across the gear, not across the tooth), so any two of them with
// the same module mesh at centre distance m(z1+z2)/2, whatever their helix
// angles. A positive helix angle is right-handed: the teeth turn
// anticlockwise (seen from above) as they rise. Two outside gears mesh
// when their helix angles are opposite; a ring gear and the gears inside
// it mesh when theirs are the same.
//
// Clearance is how far each tooth face is cut back, measured square to the
// face, so the clearances of two meshing gears add up to the gap between
// their teeth. The tip gap is how far a gear's tooth roots sit beyond the
// tips of a standard gear meshing with it (tips a module out from the pitch
// circle): the gap a meshing tooth's tip leaves at the bottom of the space
// it turns in. Standard gears have a quarter of a module there. Printed in
// place, a gear that is only held by its teeth can be pushed sideways until
// those tips reach those roots, so a small tip gap is what keeps it
// centred; the faces' clearance doesn't change with it.

const gearAlpha = 20 * math.Pi / 180

// toothedOutline is a gear's outline, counter-clockwise: z teeth with the
// first on +X, tips at radius ra, roots at rf, each flank an involute of
// the base circle, turned in by cut radians (thinning each tooth by 2 cut;
// a negative cut fattens them). Below the base circle the flanks run
// straight to the root. With a cut, a tip that would come to a point is
// lowered, and a gap that would close at the root is kept open.
func toothedOutline(m float64, z int, ra, rf, cut float64) []Pt {
	r := m * float64(z) / 2
	rb := r * math.Cos(gearAlpha)
	inv := func(rho float64) float64 {
		if rho <= rb {
			return 0
		}
		t := math.Sqrt(rho*rho/(rb*rb) - 1)
		return t - math.Atan(t)
	}
	invA := math.Tan(gearAlpha) - gearAlpha
	r0 := math.Max(rb, rf)
	// half a tooth's angular width at radius rho (reads cut as it changes)
	half := func(rho float64) float64 { return math.Pi/(2*float64(z)) + invA - inv(rho) - cut }
	if cut != 0 {
		// at least 0.2 mm of gap at the root and of land at the tip
		cut = math.Max(cut, math.Pi/(2*float64(z))+invA-inv(r0)-math.Pi/float64(z)+0.1/r0)
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
	polar := func(rho, a float64) Pt { return Pt{rho * math.Cos(a), rho * math.Sin(a)} }
	var out []Pt
	const fl = 10
	for k := 0; k < z; k++ {
		c := 2 * math.Pi * float64(k) / float64(z)
		// root, then the flank up, tip, flank down
		if rf < rb {
			out = append(out, polar(rf, c-half(rb)))
		}
		for i := 0; i <= fl; i++ {
			rho := r0 + (ra-r0)*float64(i)/fl
			out = append(out, polar(rho, c-half(rho)))
		}
		for i := 1; i < 4; i++ {
			out = append(out, polar(ra, c-half(ra)+2*half(ra)*float64(i)/4))
		}
		for i := fl; i >= 0; i-- {
			rho := r0 + (ra-r0)*float64(i)/fl
			out = append(out, polar(rho, c+half(rho)))
		}
		if rf < rb {
			out = append(out, polar(rf, c+half(rb)))
		}
		// root arc to the next tooth
		next := c + 2*math.Pi/float64(z)
		a0, a1 := c+half(r0), next-half(r0)
		for i := 1; i < 4; i++ {
			out = append(out, polar(rf, a0+(a1-a0)*float64(i)/4))
		}
	}
	return out
}

// involuteOutline is a spur gear's outline: module m, z teeth, 20°
// pressure angle, first tooth on +X.
func involuteOutline(m float64, z int) []Pt {
	z = max(z, 8)
	r := m * float64(z) / 2
	return toothedOutline(m, z, r+m, r-1.25*m, 0)
}

// twistExtrude extrudes a region (outer outline counter-clockwise, holes
// clockwise) through the heights zs, turned about the Z axis by angs[k]
// radians at zs[k]. Straight walls join each point to itself turned, so an
// outline turned steadily as it rises makes helical teeth.
func twistExtrude(outer []Pt, holes [][]Pt, zs, angs []float64) *geom.Mesh {
	at := func(p Pt, k int) geom.Vec3 {
		s, c := math.Sincos(angs[k])
		return geom.V(p.X*c-p.Y*s, p.X*s+p.Y*c, zs[k])
	}
	m := &geom.Mesh{}
	for _, c := range append([][]Pt{outer}, holes...) {
		n := len(c)
		for k := 0; k+1 < len(zs); k++ {
			for i := 0; i < n; i++ {
				j := (i + 1) % n
				a0, a1, b0, b1 := at(c[i], k), at(c[j], k), at(c[i], k+1), at(c[j], k+1)
				m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{a0, a1, b1}}, geom.Tri{P: [3]geom.Vec3{a0, b1, b0}})
			}
		}
	}
	top := len(zs) - 1
	for _, t := range Triangulate(Region{Outer: outer, Holes: holes}) {
		m.Tris = append(m.Tris,
			geom.Tri{P: [3]geom.Vec3{at(t[0], 0), at(t[2], 0), at(t[1], 0)}},
			geom.Tri{P: [3]geom.Vec3{at(t[0], top), at(t[1], top), at(t[2], top)}})
	}
	// the caps' triangulation may skip a point in line with its neighbours
	// that the walls use: split those edges so the mesh closes
	return done(m)
}

// gearTwist is how a gear's teeth turn with height: helix in degrees,
// herringbone (turning one way to half height and back), for a gear of
// pitch radius r and height h. All gears measure the turn from the same
// heights, so gears placed in mesh stay in mesh all the way up.
type gearTwist struct {
	helix, r, h float64
	herringbone bool
}

// at is the turn in radians at height z: the teeth's sideways travel at
// the pitch circle, over the radius. It is zero a quarter of the way up a
// herringbone and half way up a helical gear, so the turn is balanced
// either side of the gear as built.
func (g gearTwist) at(z float64) float64 {
	s := z - g.h/2
	if g.herringbone {
		s = g.h/4 - math.Abs(z-g.h/2)
	}
	return s * math.Tan(g.helix*math.Pi/180) / g.r
}

// sections are the heights and turns to build at: steps of at most 2°.
func (g gearTwist) sections() (zs, angs []float64) {
	run := []float64{0, g.h}
	if g.herringbone {
		run = []float64{0, g.h / 2, g.h}
	}
	zs = []float64{0}
	for i := 0; i+1 < len(run); i++ {
		n := max(1, int(math.Ceil(math.Abs(g.at(run[i+1])-g.at(run[i]))/(2*math.Pi/180))))
		for k := 1; k <= n; k++ {
			zs = append(zs, run[i]+(run[i+1]-run[i])*float64(k)/float64(n))
		}
	}
	for _, z := range zs {
		angs = append(angs, g.at(z))
	}
	return zs, angs
}

// flankCut turns a clearance square to the tooth face into radians round
// the base circle of a gear of pitch radius r: across the gear the face
// sits steeper by the helix angle at the base circle.
func flankCut(clr, r, helix float64) float64 {
	rb := r * math.Cos(gearAlpha)
	betaB := math.Atan(math.Tan(helix*math.Pi/180) * math.Cos(gearAlpha))
	return clr / (rb * math.Cos(betaB))
}

// outerGear is an outside gear (sun, planet, spur or helical), first tooth
// on +X at the heights where the twist is zero, with an optional bore.
func outerGear(m float64, z int, h, helix float64, herringbone bool, clr, tip, bore float64) *geom.Mesh {
	r := m * float64(z) / 2
	outline := toothedOutline(m, z, r+m, r-m-tip, flankCut(clr, r, helix))
	var holes [][]Pt
	if bore > 0 && bore < m*float64(z)-3*m {
		holes = append(holes, reversed(circlePts(0, 0, bore/2, 48)))
	}
	zs, angs := gearTwist{helix, r, h, herringbone}.sections()
	return twistExtrude(outline, holes, zs, angs)
}

// ringGear is an internal gear with a rim of the given width outside its
// tooth roots. Its tooth spaces are an outside gear's teeth, the first on
// +X turned by phase radians, cut out of the rim; their faces are moved out
// by the clearance.
func ringGear(m float64, z int, h, helix float64, herringbone bool, clr, tip, rim, phase float64) *geom.Mesh {
	r := m * float64(z) / 2
	space := toothedOutline(m, z, r+m+tip, r-m, -flankCut(clr, r, helix))
	s, c := math.Sincos(phase)
	for i, p := range space {
		space[i] = Pt{p.X*c - p.Y*s, p.X*s + p.Y*c}
	}
	ro := r + m + tip + math.Max(rim, 0.8)
	zs, angs := gearTwist{helix, r, h, herringbone}.sections()
	return twistExtrude(circlePts(0, 0, ro, max(96, 4*z)), [][]Pt{reversed(space)}, zs, angs)
}

func helicalGearModel(v Values) *geom.Mesh {
	return outerGear(v.Get("module", 1.5), max(int(v.Get("teeth", 20)), 8), v.Get("thickness", 8),
		v.Get("helix", 20), v.Get("herringbone", 0) >= 0.5, v.Get("clearance", 0), v.Get("tipgap", 0.4), v.Get("bore", 5))
}

func ringGearModel(v Values) *geom.Mesh {
	return ringGear(v.Get("module", 1.5), max(int(v.Get("teeth", 40)), 16), v.Get("thickness", 8),
		v.Get("helix", 20), v.Get("herringbone", 0) >= 0.5, v.Get("clearance", 0), v.Get("tipgap", 0.4), v.Get("rim", 4), 0)
}

// --- planetary set ---------------------------------------------------------------------
//
// A sun gear in the middle, planet gears round it and a ring gear round
// them, all herringbone (by default) so nothing can slide out: printed in
// place, already meshed, it turns as soon as it comes off the plate. The
// ring has sun + 2 × planet teeth. The planets sit where their teeth fit
// both the sun's and the ring's: evenly spaced when the sun and ring teeth
// together divide by the number of planets, as near even as fits when not.
// Planets that would touch each other are left out.

type planetaryDims struct {
	m, h, helix, clr, tip, rim, bore float64
	zs, zp, zr                       int
	herringbone                      bool
	angles                           []float64 // the planets' positions round the sun
}

// planetaryValues clamps v as the parts' builds clamp it, so the layout
// and the parts agree.
func planetaryValues(v Values) Values {
	if k, ok := Lookup("planetary"); ok {
		return k.clamp(v)
	}
	return v
}

func planetaryLayout(v Values) planetaryDims {
	v = planetaryValues(v)
	d := planetaryDims{
		m: v.Get("module", 1.5), h: v.Get("thickness", 10), helix: v.Get("helix", 30),
		clr: v.Get("clearance", 0.4), tip: v.Get("tipgap", 0.3), rim: v.Get("rim", 5), bore: v.Get("bore", 0),
		zs: max(int(v.Get("sun", 18)), 8), zp: max(int(v.Get("planet", 12)), 8),
		herringbone: v.Get("herringbone", 1) >= 0.5,
	}
	d.zr = d.zs + 2*d.zp
	a := d.m * float64(d.zs+d.zp) / 2
	slots := d.zs + d.zr // the planets go at whole multiples of 2π/slots
	for n := max(int(v.Get("planets", 5)), 1); n >= 1; n-- {
		d.angles = d.angles[:0]
		ok := true
		for i := 0; i < n; i++ {
			k := int(math.Round(float64(i*slots) / float64(n)))
			d.angles = append(d.angles, 2*math.Pi*float64(k)/float64(slots))
		}
		for i := 0; i < n && n > 1; i++ {
			gap := d.angles[(i+1)%n] - d.angles[i]
			if i == n-1 {
				gap += 2 * math.Pi
			}
			if 2*a*math.Sin(gap/2) < d.m*float64(d.zp+2)+2*d.clr {
				ok = false
			}
		}
		if ok {
			break
		}
	}
	return d
}

// planetTurn is how far a planet gear is turned (first tooth on +X before)
// to mesh at angle theta round a sun whose first tooth is on +X.
func (d planetaryDims) planetTurn(theta float64) float64 {
	return theta + math.Pi + (float64(d.zs)*theta-math.Pi)/float64(d.zp)
}

// PlanetaryCentres gives the axes of a planetary set's planets for its
// parameters, in the set's frame (the sun's axis at the origin), and how
// far each planet is turned from its first tooth on +X.
func PlanetaryCentres(v Values) (centres []geom.Vec3, turns []float64) {
	d := planetaryLayout(v)
	a := d.m * float64(d.zs+d.zp) / 2
	for _, th := range d.angles {
		centres = append(centres, geom.V(a*math.Cos(th), a*math.Sin(th), 0))
		turns = append(turns, d.planetTurn(th))
	}
	return centres, turns
}

func planetarySun(v Values) *geom.Mesh {
	d := planetaryLayout(v)
	return outerGear(d.m, d.zs, d.h, d.helix, d.herringbone, d.clr/2, d.tip, d.bore)
}

func planetaryPlanet(v Values) *geom.Mesh {
	d := planetaryLayout(v)
	return outerGear(d.m, d.zp, d.h, -d.helix, d.herringbone, d.clr/2, d.tip, 0)
}

func planetaryRing(v Values) *geom.Mesh {
	d := planetaryLayout(v)
	// the ring's teeth face the planets' gaps: its spaces are half a
	// tooth on from its teeth
	phase := math.Pi * float64(d.zp+1) / float64(d.zr)
	return ringGear(d.m, d.zr, d.h, -d.helix, d.herringbone, d.clr/2, d.tip, d.rim, phase)
}

func planetaryParts(v Values) []Placed {
	v = planetaryValues(v)
	// each part's mesh is centred on its bounds, which for an odd number
	// of teeth is off its axis: put the axis back where it belongs
	axis := func(id string, model func(Values) *geom.Mesh) geom.Mat4 {
		return geom.Translate(partCentre(id, v, model))
	}
	parts := []Placed{
		{Kind: "planetary-ring", Name: "Ring Gear", Color: "#2f3640", M: axis("planetary-ring", planetaryRing)},
		{Kind: "planetary-sun", Name: "Sun Gear", Color: "#e1b12c", M: axis("planetary-sun", planetarySun)},
	}
	centres, turns := PlanetaryCentres(v)
	for i, c := range centres {
		parts = append(parts, Placed{Kind: "planetary-planet", Name: "Planet Gear", Color: "#c8ccd0",
			M: geom.Translate(c).Mul(geom.RotateZ(turns[i])).Mul(axis("planetary-planet", planetaryPlanet))})
	}
	return parts
}

// registerGears adds the gears after the spur gear and rack.
func registerGears(mk *[]string) {
	helix := slider("helix", "Helix Angle", -45, 45, 1, 20, false)
	registerPart(mk, "helicalgear", "Helical Gear", "#c8ccd0", []Param{
		mslider("module", "Module", 0.3, 5, 0.1, 1.5, false),
		slider("teeth", "Teeth", 8, 120, 1, 20, true),
		lslider("thickness", "Thickness", 0.5, 50, 0.5, 8, false),
		helix,
		slider("herringbone", "Herringbone", 0, 1, 1, 0, true),
		lslider("bore", "Bore Dia", 0, 50, 0.5, 5, false),
		lslider("clearance", "Clearance", 0, 1, 0.05, 0, false),
		lslider("tipgap", "Tip Gap", 0.1, 2, 0.05, 0.4, false),
	}, helicalGearModel)
	registerPart(mk, "ringgear", "Ring Gear", "#2f3640", []Param{
		mslider("module", "Module", 0.3, 5, 0.1, 1.5, false),
		slider("teeth", "Teeth", 16, 200, 1, 40, true),
		lslider("thickness", "Thickness", 0.5, 50, 0.5, 8, false),
		helix,
		slider("herringbone", "Herringbone", 0, 1, 1, 0, true),
		lslider("rim", "Rim", 0.8, 30, 0.5, 4, false),
		lslider("clearance", "Clearance", 0, 1, 0.05, 0, false),
		lslider("tipgap", "Tip Gap", 0.1, 2, 0.05, 0.4, false),
	}, ringGearModel)
	set := []Param{
		mslider("module", "Module", 0.5, 5, 0.1, 1.5, false),
		slider("sun", "Sun Teeth", 8, 60, 1, 18, true),
		slider("planet", "Planet Teeth", 8, 60, 1, 12, true),
		slider("planets", "Planets", 1, 12, 1, 5, true),
		lslider("thickness", "Thickness", 2, 50, 0.5, 10, false),
		slider("helix", "Helix Angle", 0, 45, 1, 30, false),
		slider("herringbone", "Herringbone", 0, 1, 1, 1, true),
		lslider("clearance", "Clearance", 0, 1, 0.05, 0.4, false),
		lslider("tipgap", "Tip Gap", 0.1, 2, 0.05, 0.3, false),
		lslider("rim", "Rim", 0.8, 30, 0.5, 5, false),
		lslider("bore", "Sun Bore Dia", 0, 50, 0.5, 0, false),
	}
	registerHiddenPart("planetary-sun", "Sun Gear", "#e1b12c", set, planetarySun)
	registerHiddenPart("planetary-planet", "Planet Gear", "#c8ccd0", set, planetaryPlanet)
	registerHiddenPart("planetary-ring", "Ring Gear", "#2f3640", set, planetaryRing)
	registerAssembly(mk, "planetary", "Planetary Gear Set", "#c8ccd0", set, planetaryParts)
}
