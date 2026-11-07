package shapes

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// planetarySet builds a planetary set's parts in place as an assembly lays
// them out: ring, sun, then the planets.
func planetarySet(t *testing.T, num map[string]float64) (ring, sun *geom.Mesh, planets []*geom.Mesh, d planetaryDims) {
	t.Helper()
	k, _ := Lookup("planetary")
	v := k.Defaults()
	for key, x := range num {
		v.Num[key] = x
	}
	parts, _ := k.Parts(v)
	var ms []*geom.Mesh
	for _, p := range parts {
		ms = append(ms, Build(p.Kind, PartSize(p.Kind, v), v).Transform(p.M))
	}
	return ms[0], ms[1], ms[2:], planetaryLayout(v)
}

func overlap(a, b *geom.Mesh) float64 { return geom.Intersect(a, b).Volume() }

// turnAbout turns m by a radians about the vertical line through c.
func turnAbout(m *geom.Mesh, c geom.Vec3, a float64) *geom.Mesh {
	return m.Transform(geom.Translate(c).Mul(geom.RotateZ(a)).Mul(geom.Translate(c.Neg())))
}

// checkPlanetary turns the sun through one tooth, the carrier held, the
// planets and ring driven as their teeth require, and fails on any parts
// that touch.
func checkPlanetary(t *testing.T, name string, num map[string]float64) {
	ring, sun, planets, d := planetarySet(t, num)
	k, _ := Lookup("planetary")
	v := k.Defaults()
	for key, x := range num {
		v.Num[key] = x
	}
	centres, _ := PlanetaryCentres(v)
	if len(planets) != len(centres) || len(planets) == 0 {
		t.Fatalf("%s: %d planets for %d centres", name, len(planets), len(centres))
	}
	const steps = 4
	for s := 0; s < steps; s++ {
		a := 2 * math.Pi / float64(d.zs) * float64(s) / steps
		sn := turnAbout(sun, geom.Vec3{}, a)
		rg := turnAbout(ring, geom.Vec3{}, -a*float64(d.zs)/float64(d.zr))
		var ps []*geom.Mesh
		for i, p := range planets {
			ps = append(ps, turnAbout(p, centres[i], -a*float64(d.zs)/float64(d.zp)))
		}
		for i, p := range ps {
			if v := overlap(sn, p); v > 1e-4 {
				t.Errorf("%s step %d: sun and planet %d overlap by %.4f mm³", name, s, i, v)
			}
			if v := overlap(rg, p); v > 1e-4 {
				t.Errorf("%s step %d: ring and planet %d overlap by %.4f mm³", name, s, i, v)
			}
			if j := (i + 1) % len(ps); j != i {
				if v := overlap(p, ps[j]); v > 1e-4 {
					t.Errorf("%s step %d: planets %d and %d overlap by %.4f mm³", name, s, i, j, v)
				}
			}
		}
		if v := overlap(sn, rg); v > 1e-4 {
			t.Errorf("%s step %d: sun and ring overlap by %.4f mm³", name, s, v)
		}
	}
}

func TestPlanetaryMeshes(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CSG")
	}
	checkPlanetary(t, "default", nil)
	// sun + ring = 15 + 35 = 50 teeth, not a multiple of 4: uneven planets
	checkPlanetary(t, "uneven", map[string]float64{"sun": 15, "planet": 10, "planets": 4, "thickness": 6})
	checkPlanetary(t, "spur", map[string]float64{"helix": 0, "herringbone": 0, "sun": 13, "planet": 9, "planets": 3, "thickness": 4})
	checkPlanetary(t, "helical", map[string]float64{"herringbone": 0, "helix": 20, "thickness": 6})
}

func TestPlanetaryClearanceIsTheGap(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CSG")
	}
	// turning the sun alone closes the gap on one side of its teeth: at
	// the pitch circle the faces' gap c, square to them, is
	// c / (cos α cos βb) round the circle
	ring, sun, planets, d := planetarySet(t, map[string]float64{"thickness": 6})
	_ = ring
	betaB := math.Atan(math.Tan(d.helix*math.Pi/180) * math.Cos(gearAlpha))
	free := d.clr / (math.Cos(gearAlpha) * math.Cos(betaB)) / (d.m * float64(d.zs) / 2)
	for _, f := range []float64{0.8, 1.25} {
		v := overlap(turnAbout(sun, geom.Vec3{}, f*free), planets[0])
		if f < 1 && v > 1e-4 {
			t.Errorf("sun turned %.2f of its free play hits a planet: %.4f mm³", f, v)
		}
		if f > 1 && v < 1e-3 {
			t.Errorf("sun turned %.2f of its free play still clear: %.4f mm³", f, v)
		}
	}
}

func TestPlanetaryTipGapIsThePlay(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CSG")
	}
	// pushed straight at a planet, the sun moves the tip gap before its
	// tips reach the planet's roots
	for _, tip := range []float64{0.25, 0.6} {
		_, sun, planets, d := planetarySet(t, map[string]float64{"thickness": 6, "tipgap": tip})
		th := d.angles[0]
		for _, f := range []float64{0.9, 1.15} {
			s := f * tip
			v := overlap(sun.Transform(geom.Translate(geom.V(s*math.Cos(th), s*math.Sin(th), 0))), planets[0])
			if f < 1 && v > 1e-4 {
				t.Errorf("tip gap %.2f: the sun moved %.2f of it hits a planet: %.4f mm³", tip, f, v)
			}
			if f > 1 && v < 1e-4 {
				t.Errorf("tip gap %.2f: the sun moved %.2f of it is still clear", tip, f)
			}
		}
	}
}

func TestHelicalPairAndRing(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CSG")
	}
	// two 16-tooth helical gears of opposite hand, side by side, the
	// second turned half a tooth so a gap faces the first's tooth, and a
	// 40-tooth ring of the second's hand round the second
	const m, z, zr, h, helix, clr = 1.5, 16, 40, 6.0, 25.0, 0.2
	a := outerGear(m, z, h, helix, false, clr, 0.4, 0)
	b := outerGear(m, z, h, -helix, false, clr, 0.4, 0)
	cd := m * z
	b = b.Transform(geom.Translate(geom.V(cd, 0, 0)).Mul(geom.RotateZ(math.Pi / z)))
	if v := overlap(a, b); v > 1e-4 {
		t.Errorf("helical pair overlaps by %.4f mm³", v)
	}
	// opposite hands are what mesh: the same hand jams
	same := outerGear(m, z, h, helix, false, clr, 0.4, 0).Transform(geom.Translate(geom.V(cd, 0, 0)).Mul(geom.RotateZ(math.Pi / z)))
	if v := overlap(a, same); v < 1e-2 {
		t.Errorf("same-hand helical gears mesh (%.4f mm³): the helix sign is wrong", v)
	}
	// a ring centred (zr-z)m/2 to the gear's -X: the gear's tooth at +X
	// needs one of the ring's spaces there, which an unturned ring has
	ring := ringGear(m, zr, h, helix, false, clr, 0.4, 3, 0)
	off := -m * float64(zr-z) / 2
	ring = ring.Transform(geom.Translate(geom.V(off, 0, 0)))
	if v := overlap(a, ring); v > 1e-4 {
		t.Errorf("gear in ring overlaps by %.4f mm³", v)
	}
}

func TestHelicalGearBoreAndSize(t *testing.T) {
	k, _ := Lookup("helicalgear")
	v := k.Defaults()
	s, _ := k.NaturalSize(v)
	m, z := v.Get("module", 0), v.Get("teeth", 0)
	if da := m * (z + 2); s.X > da+1e-6 || s.X < da-0.5 || s.Z != v.Get("thickness", 0) {
		t.Errorf("natural size %v for tip diameter %v", s, da)
	}
	mesh := Build("helicalgear", s, v)
	r := m * z / 2
	solid := math.Pi * r * r * s.Z
	bore := math.Pi * math.Pow(v.Get("bore", 0)/2, 2) * s.Z
	if vol := mesh.Volume(); math.Abs(vol-(solid-bore)) > 0.05*solid {
		t.Errorf("volume %.1f, a pitch-circle disc less the bore is %.1f", vol, solid-bore)
	}
}

func TestSpurOutlineUnchanged(t *testing.T) {
	// the spur gear's outline is toothedOutline with no cut: the shared
	// code must not have moved a point
	o := involuteOutline(2, 17)
	if len(o) != 17*(2+11+3+11+3) {
		t.Fatalf("%d points", len(o))
	}
	if p := o[0]; math.Abs(math.Hypot(p.X, p.Y)-(17-2.5)) > 1e-9 {
		t.Errorf("first root point %v not on the root circle", p)
	}
}
