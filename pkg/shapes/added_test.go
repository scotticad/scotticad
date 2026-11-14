package shapes

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Tests for the shapes registered in zz_added.go. The catalogue-wide
// tests (closed, sized, random settings) cover them too.

func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func vals(k string, num map[string]float64) Values {
	kk, _ := Lookup(k)
	v := kk.Defaults()
	for key, x := range num {
		v.Num[key] = x
	}
	return v
}

func TestSlotEndsAreRound(t *testing.T) {
	for _, s := range []geom.Vec3{geom.V(30, 10, 5), geom.V(8, 40, 3), geom.V(12, 12, 4)} {
		m := Build("slot", s, vals("slot", nil))
		r := math.Min(s.X, s.Y) / 2
		long := math.Max(s.X, s.Y) - 2*r
		want := (long*2*r + math.Pi*r*r) * s.Z
		if v := m.Volume(); !approx(v, want, 0.01*want) { // 48 sides: within 1% of a true circle
			t.Errorf("slot %v: volume %.2f, want about %.2f", s, v, want)
		}
	}
}

func TestArcKeepsItsRadius(t *testing.T) {
	k, _ := Lookup("arc")
	for _, c := range []struct {
		ang, wall float64
		want      geom.Vec3
	}{
		{90, 5, geom.V(20, 20, 5)},
		{180, 5, geom.V(40, 20, 5)},
		{360, 5, geom.V(40, 40, 5)},
		{90, 0, geom.V(20, 20, 5)},
	} {
		s, _ := k.NaturalSize(vals("arc", map[string]float64{"angle": c.ang, "wall": c.wall}))
		if s.Sub(c.want).Len() > 1e-6 {
			t.Errorf("arc %g° wall %g: size %v, want %v", c.ang, c.wall, s, c.want)
		}
	}
	// a ring's area: the part of the annulus
	m := arcModel(vals("arc", map[string]float64{"angle": 90, "wall": 5, "sides": 256}))
	want := math.Pi * (20*20 - 15*15) / 4 * 5
	if v := m.Volume(); !approx(v, want, 0.002*want) {
		t.Errorf("quarter ring volume %.1f, want %.1f", v, want)
	}
}

func TestFilletFillsTheInsideCorner(t *testing.T) {
	s := geom.V(20, 6, 4)
	m := Build("fillet", s, vals("fillet", map[string]float64{"steps": 64}))
	want := (1 - math.Pi/4) * s.X * s.Y * s.Z
	if v := m.Volume(); !approx(v, want, 0.002*want) {
		t.Errorf("volume %.3f, want %.3f", v, want)
	}
	// the wall is behind it: it reaches the top only at the back (+Y; the
	// front is -Y) and the front only at the bottom
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			if approx(p.Z, s.Z/2, 1e-9) && !approx(p.Y, s.Y/2, 1e-9) {
				t.Fatalf("a point on the top away from the back: %v", p)
			}
			if approx(p.Y, -s.Y/2, 1e-9) && !approx(p.Z, -s.Z/2, 1e-9) {
				t.Fatalf("a point on the front above the bottom: %v", p)
			}
		}
	}
}

// A bevel pair turns together without its teeth colliding, all through a
// tooth's travel, and the teeth are in contact: with no clearance, a mate
// a sixth of a degree out either way digs in.
func TestBevelPairTurns(t *testing.T) {
	for _, c := range [][2]float64{{20, 20}, {15, 30}, {30, 12}} {
		turned := func(a, b *geom.Mesh, apexZ, th, off float64) float64 {
			apex := geom.V(0, 0, apexZ)
			ph := th*c[0]/c[1] + off
			gb := b.Transform(geom.Translate(apex).Mul(geom.RotateX(ph)).Mul(geom.Translate(apex.Neg())))
			return geom.Intersect(a.Transform(geom.RotateZ(th)), gb).Volume()
		}
		v := vals("bevelpair", map[string]float64{"teeth": c[0], "mate": c[1], "module": 1.5})
		a, b, apexZ := bevelPair(v)
		const steps = 8
		for i := 0; i < steps; i++ {
			th := 2 * math.Pi / c[0] * float64(i) / steps
			if x := turned(a, b, apexZ, th, 0); x > 1e-3 {
				t.Errorf("%v teeth, %d/%d of a tooth on: the gears overlap by %.4f mm³", c, i, steps, x)
			}
		}
		v.Num["clearance"] = 0
		a, b, apexZ = bevelPair(v)
		for _, off := range []float64{-0.15, 0.15} {
			if x := turned(a, b, apexZ, 0.3*2*math.Pi/c[0], off*math.Pi/180); x < 0.005 {
				t.Errorf("%v teeth: a mate %.2f° out doesn't touch (%.4f mm³): the teeth are loose", c, off, x)
			}
		}
	}
}

func TestTeardropShape(t *testing.T) {
	k, _ := Lookup("teardrop")
	s, _ := k.NaturalSize(vals("teardrop", map[string]float64{"diameter": 10, "length": 30}))
	if !approx(s.X, 10, 1e-9) || !approx(s.Y, 30, 1e-9) || !approx(s.Z, 5+5*math.Sqrt2, 1e-9) {
		t.Errorf("size %v, want 10 × 30 × %.3f", s, 5+5*math.Sqrt2)
	}
	s, _ = k.NaturalSize(vals("teardrop", map[string]float64{"diameter": 10, "length": 30, "flat": 1}))
	if !approx(s.Z, 10, 1e-9) {
		t.Errorf("flat top: height %g, want 10", s.Z)
	}
	// no face overhangs more than 45°: every downward-facing face is on the
	// circle's lower half
	m := teardropModel(vals("teardrop", nil))
	for _, tr := range m.Tris {
		n := tr.Normal()
		c := tr.P[0].Add(tr.P[1]).Add(tr.P[2]).Mul(1.0 / 3)
		if n.Z < -1e-9 && c.Z > 1e-9 {
			t.Fatalf("a face above the middle faces down: centre %v normal %v", c, n)
		}
	}
}

func TestCrystalSeeds(t *testing.T) {
	s := geom.V(12, 12, 30)
	a := Build("crystal", s, vals("crystal", map[string]float64{"irregular": 60, "seed": 7}))
	b := Build("crystal", s, vals("crystal", map[string]float64{"irregular": 60, "seed": 7}))
	c := Build("crystal", s, vals("crystal", map[string]float64{"irregular": 60, "seed": 8}))
	if a.Volume() != b.Volume() {
		t.Error("the same seed made different crystals")
	}
	if approx(a.Volume(), c.Volume(), 1e-9) {
		t.Error("different seeds made the same crystal")
	}
	// regular: the point starts 30% from the top all round
	m := Build("crystal", s, vals("crystal", nil))
	var shoulders int
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			if approx(p.Z, 15-0.3*30, 1e-9) {
				shoulders++
			}
		}
	}
	if shoulders == 0 {
		t.Error("no shoulder corner at 30% from the top")
	}
}

func TestBearingSeatFitsItsBearing(t *testing.T) {
	for i, b := range bearings {
		v := Values{Num: map[string]float64{"bearing": float64(i), "fit": 0.1, "below": 0}}
		s := bearingSeatModel(v).Bounds().Size()
		if !approx(s.X, b.od+0.1, 1e-6) || !approx(s.Z, seatGap+b.wid+0.5, 1e-6) {
			t.Errorf("%s: seat %v, want %g across and %g deep", b.name, s, b.od+0.1, seatGap+b.wid+0.5)
		}
	}
	if n := bearingNames()[6]; !strings.HasPrefix(n, "608: 8 × 22 × 7") {
		t.Errorf("the default is %q, want the 608", n)
	}
}

func TestKeyholeTrapsTheHead(t *testing.T) {
	v := vals("keyhole", nil) // head 8, shank 4, slot 8, depth 6, recess 3
	m := keyholeModel(v)
	b := m.Bounds()
	if !approx(b.Min.X, -4.25, 1e-6) || !approx(b.Max.X, 8+4.25, 1e-6) || !approx(b.Size().Z, 6.5, 1e-9) {
		t.Errorf("bounds %v", b)
	}
	// the lip: at the slot's end, above the recess, the cut is only the shank's width
	lip := geom.Intersect(m, cuboid(7.5, -10, 4, 8.5, 10, 5))
	if w := lip.Bounds().Size().Y; !approx(w, 4, 1e-3) {
		t.Errorf("slot width above the recess %.3f, want 4", w)
	}
}

func TestBevelPairMeshes(t *testing.T) {
	for _, c := range [][2]float64{{20, 20}, {15, 30}, {13, 17}, {30, 12}} {
		v := vals("bevelpair", map[string]float64{"teeth": c[0], "mate": c[1], "module": 1.5})
		a, b, apexZ := bevelPair(v)
		if o, n := a.EdgeCheck(1e-7); o+n != 0 {
			t.Errorf("%v: first gear %d open, %d non-manifold", c, o, n)
		}
		// in mesh: their teeth interleave without touching (each has 0.1 mm
		// of clearance on its faces)
		if x := geom.Intersect(a, b).Volume(); x > 1e-3 {
			t.Errorf("%v teeth: the gears overlap by %.3f mm³", c, x)
		}
		// ...and close: the second gear turned half a tooth about its axis
		// (along X through the shared apex), its teeth hit
		apex := geom.V(0, 0, apexZ)
		turn := geom.Translate(apex).Mul(geom.RotateX(math.Pi / c[1])).Mul(geom.Translate(apex.Neg()))
		if x := geom.Intersect(a, b.Transform(turn)).Volume(); x < 1 {
			t.Errorf("%v teeth: turned half a tooth the gears still miss (%.3f mm³): they aren't in mesh", c, x)
		}
	}
	// the pitch radius at the back is m·z/2
	g := bevelOf(2, 25, 40)
	if !approx(g.R, 25, 1e-12) || !approx(g.apex, 40, 1e-12) || !approx(g.d, math.Atan(25.0/40), 1e-12) {
		t.Errorf("geometry %+v", g)
	}
}

func TestGT2PulleySize(t *testing.T) {
	for _, n := range []int{12, 20, 36, 60} {
		o := gt2Outline(n, 0.2)
		r := 0.0
		for _, p := range o {
			r = math.Max(r, math.Hypot(p.X, p.Y))
		}
		if want := float64(n)/math.Pi - 0.254; !approx(r, want, 1e-9) {
			t.Errorf("%d teeth: outside radius %.4f, want %.4f", n, r, want)
		}
	}
	m := gt2Model(vals("gt2pulley", nil))
	b := m.Bounds()
	if !approx(b.Size().Z, 1+7+2+6, 1e-9) {
		t.Errorf("height %g, want flange 1 + teeth 7 + flange 2 + hub 6", b.Size().Z)
	}
	// it prints flange down, hub up: widest on the plate, the hub's width on top
	wide := func(z0, z1 float64) float64 {
		w := 0.0
		for _, tr := range m.Tris {
			for _, p := range tr.P {
				if p.Z >= z0-1e-9 && p.Z <= z1+1e-9 {
					w = math.Max(w, math.Hypot(p.X, p.Y))
				}
			}
		}
		return w
	}
	ro := 20/math.Pi - 0.254
	if lo, hi := wide(b.Min.Z, b.Min.Z), wide(b.Max.Z, b.Max.Z); !approx(lo, ro+1.5, 1e-6) || !approx(hi, 7, 1e-6) {
		t.Errorf("radius %.3f on the plate and %.3f on top, want the flange (%.3f) and the hub (7)", lo, hi, ro+1.5)
	}
	// nothing overhangs by more than 45 degrees, but the grooves' roofs under the second flange
	for _, tr := range m.Tris {
		n := tr.Normal()
		c := tr.P[0].Add(tr.P[1]).Add(tr.P[2]).Mul(1.0 / 3)
		if n.Z < -0.7072 && c.Z > b.Min.Z+1e-6 && math.Hypot(c.X, c.Y) > ro+1e-6 {
			t.Fatalf("a face outside the teeth faces down at %v", c)
		}
	}
}

// Each joint's key fits inside its socket with its clearance, apart from
// the part of the key sunk into the other piece.
func TestJointsFit(t *testing.T) {
	v := vals("dovetail", nil)
	d := dovetailOf(v)
	left := geom.Subtract(dovetailKey(v), dovetailSocket(v)).Volume()
	if want := d.neck * 0.5 * d.h; !approx(left, want, 1e-3*want) { // the neck from y = -1 to -0.5
		t.Errorf("dovetail key outside its socket %.3f, want %.3f", left, want)
	}
	v = vals("alignpin", nil)
	r, _, _, _ := pinDims(v)
	left = geom.Subtract(pinKey(v), pinSocket(v)).Volume()
	if want := math.Pi * r * r * 0.5; !approx(left, want, 0.01*want) {
		t.Errorf("pin outside its socket %.3f, want about %.3f", left, want)
	}
	for _, id := range []string{"dovetail", "alignpin"} {
		k, _ := Lookup(id)
		ps, _ := k.Parts(k.Defaults())
		if len(ps) != 2 || ps[0].Hole || !ps[1].Hole {
			t.Errorf("%s: parts %+v, want a solid key then a hole", id, ps)
		}
	}
}

// Each Flanges option stacks what it says; the hub is no wider than what
// it stands on.
func TestGT2Flanges(t *testing.T) {
	ro := 20/math.Pi - 0.254
	for _, c := range []struct {
		flanges, height, hubR float64
	}{
		{0, 7 + 6, ro},
		{1, 1 + 7 + 6, ro},
		{2, 1 + 7 + 2 + 6, 7},
	} {
		m := gt2Model(vals("gt2pulley", map[string]float64{"flanges": c.flanges}))
		b := m.Bounds()
		top := 0.0
		for _, tr := range m.Tris {
			for _, p := range tr.P {
				if approx(p.Z, b.Max.Z, 1e-9) {
					top = math.Max(top, math.Hypot(p.X, p.Y))
				}
			}
		}
		if !approx(b.Size().Z, c.height, 1e-9) || !approx(top, c.hubR, 1e-6) {
			t.Errorf("flanges %g: height %g, hub radius %.3f; want %g and %.3f", c.flanges, b.Size().Z, top, c.height, c.hubR)
		}
	}
}

// Double Pointed puts a point at the foot too; Taper narrows the foot.
func TestCrystalDoubleAndTaper(t *testing.T) {
	s := geom.V(12, 12, 30)
	atFoot := func(m *geom.Mesh) (n int, width float64) {
		seen := map[geom.Vec3]bool{}
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, tr := range m.Tris {
			for _, p := range tr.P {
				if approx(p.Z, -s.Z/2, 1e-9) && !seen[p] {
					seen[p] = true
					lo, hi = math.Min(lo, p.X), math.Max(hi, p.X)
				}
			}
		}
		return len(seen), hi - lo
	}
	if n, _ := atFoot(Build("crystal", s, vals("crystal", map[string]float64{"double": 1}))); n != 1 {
		t.Errorf("double pointed: %d corners at the foot, want the one point", n)
	}
	if n, w := atFoot(Build("crystal", s, vals("crystal", map[string]float64{"taper": 50}))); n != 7 || !approx(w, s.X/2, 1e-9) {
		t.Errorf("taper 50%%: %d corners at the foot, %g across; want the 6 sides and a centre, %g across", n, w, s.X/2)
	}
}

// Hole Below goes on down through the floor, narrower than the outer race.
func TestBearingSeatHoleBelow(t *testing.T) {
	v := vals("bearingseat", map[string]float64{"below": 3}) // the 608: 8 × 22 × 7
	m := bearingSeatModel(v)
	if b := m.Bounds(); !approx(b.Min.Z, -3, 1e-9) {
		t.Errorf("hole below reaches %g, want -3", b.Min.Z)
	}
	r := 0.0
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			if approx(p.Z, -3, 1e-9) {
				r = math.Max(r, math.Hypot(p.X, p.Y))
			}
		}
	}
	if !approx(r, (22+8)/4.0, 1e-6) { // midway across the bearing's face
		t.Errorf("hole below radius %.3f, want 7.5", r)
	}
}

// With no recess the keyhole is the plain outline all the way down; the
// tip chamfer is kept to what the pin has room for.
func TestKeyholeWithoutRecessAndPinChamfer(t *testing.T) {
	m := keyholeModel(vals("keyhole", map[string]float64{"recess": 0}))
	lip := geom.Intersect(m, cuboid(7.5, -10, -1, 8.5, 10, 10))
	if s := lip.Bounds().Size(); !approx(s.Y, 4, 1e-3) || !approx(s.Z, 6.5, 1e-9) {
		t.Errorf("no recess: the slot's end %v, want 4 wide all 6.5 deep", s)
	}
	topRadius := func(v Values) float64 {
		m := pinKey(v)
		zt, r := m.Bounds().Max.Z, 0.0
		for _, tr := range m.Tris {
			for _, p := range tr.P {
				if approx(p.Z, zt, 1e-9) {
					r = math.Max(r, math.Hypot(p.X, p.Y))
				}
			}
		}
		return r
	}
	for _, c := range [][3]float64{{0, 5, 2.5}, {0.6, 5, 1.9}, {3, 5, 0.5}} { // chamfer, diameter, tip radius
		if r := topRadius(vals("alignpin", map[string]float64{"chamfer": c[0], "diameter": c[1]})); !approx(r, c[2], 1e-6) {
			t.Errorf("chamfer %g: tip radius %.3f, want %g", c[0], r, c[2])
		}
	}
}

// A setting missing from the values (a hand-written or older file) builds
// as its slider's default: the code's fallbacks match the sliders.
func TestAddedShapesFallBackToSliderDefaults(t *testing.T) {
	for _, id := range []string{"arc", "teardrop", "keyhole", "bearingseat", "bevelgear",
		"bevelpair-gear", "bevelpair-mate", "gt2pulley", "dovetail-key", "dovetail-socket",
		"alignpin-pin", "alignpin-socket", "qrcode-base", "qrcode-code"} {
		k, _ := Lookup(id)
		v := k.Defaults()
		want, _ := k.NaturalSize(v)
		got, _ := k.NaturalSize(Values{Num: map[string]float64{}, Text: v.Text})
		if got.Sub(want).Len() > 1e-9 {
			t.Errorf("%s: with no settings %v, with the defaults %v", id, got, want)
		}
	}
}

// --- QR codes ----------------------------------------------------------------------------

// The encoder makes exactly the codes an independent encoder makes for
// the same text, level, version and mask (testdata/qr_reference.json,
// from python-qrcode).
func TestQRMatchesReference(t *testing.T) {
	b, err := os.ReadFile("testdata/qr_reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Cases []struct {
			Text                 string
			Level, Version, Mask int
			Rows                 []string
		}
	}
	if err := json.Unmarshal(b, &ref); err != nil {
		t.Fatal(err)
	}
	for _, c := range ref.Cases {
		got := qrEncodeAt([]byte(c.Text), c.Level, c.Version, c.Mask)
		for y, row := range c.Rows {
			for x, ch := range row {
				if got[y][x] != (ch == '1') {
					t.Fatalf("%q level %d version %d mask %d: module (%d, %d) differs", c.Text, c.Level, c.Version, c.Mask, x, y)
				}
			}
		}
	}
	if s, ok := qrEncode(nil, qrHigh); !ok || len(s) != 21 {
		t.Errorf("no text: %d modules, want a version 1 code", len(s))
	}
	if len(ref.Cases) < 20 {
		t.Fatalf("only %d reference codes", len(ref.Cases))
	}
}

func TestQRCapacityAndVersion(t *testing.T) {
	// the standard's byte-mode capacities at version 1 and 10
	for level, want := range [4][2]int{{17, 271}, {14, 213}, {11, 151}, {7, 119}} {
		if a, b := qrCapacity(1, level), qrCapacity(10, level); a != want[0] || b != want[1] {
			t.Errorf("level %d: capacity %d and %d, want %d and %d", level, a, b, want[0], want[1])
		}
	}
	s, ok := qrEncode([]byte("https://scotticad.com"), qrMedium)
	if !ok || len(s) != 25 { // 21 bytes at M: version 2
		t.Errorf("version %d, want 2", (len(s)-17)/4)
	}
	if _, ok := qrEncode(make([]byte, 272), qrLow); ok {
		t.Error("272 bytes fitted version 10 at L")
	}
	if CheckText("qrcode", strings.Repeat("x", 271)) != nil || CheckText("qrcode", strings.Repeat("x", 272)) == nil {
		t.Error("CheckText's limit isn't 271 bytes")
	}
	// a level raised past what the text fits falls back to one it fits
	v := vals("qrcode", map[string]float64{"level": qrHigh})
	v.Text = strings.Repeat("x", 200)
	if s := qrFor(v); len(s) != 57 {
		t.Errorf("fallback made a %d-module code", len(s))
	}
}

// Size is the whole width, the Margin's blank modules included; the
// code's modules sit on the base's top.
func TestQRSizeAndMargin(t *testing.T) {
	for _, margin := range []float64{0, 4, 8} {
		v := vals("qrcode", map[string]float64{"margin": margin, "size": 50, "base": 2})
		n := 25.0 // the default text is a version 2 code
		u := 50 / (n + 2*margin)
		base, code := qrBase(v).Bounds(), qrCode(v).Bounds()
		if !approx(base.Size().X, 50, 1e-9) || !approx(base.Size().Z, 2, 1e-9) {
			t.Errorf("margin %g: base %v, want 50 across and 2 thick", margin, base.Size())
		}
		if !approx(code.Size().X, n*u, 1e-9) || !approx(code.Size().Y, n*u, 1e-9) || !approx(code.Min.Z, 2, 1e-9) {
			t.Errorf("margin %g: code %v, want %.3f square from z = 2", margin, code, n*u)
		}
	}
}

// Modules that touch only at a corner (a checkerboard is all corners)
// still make a closed solid: each outline keeps to its own module there.
func TestQRCornersMakeASolid(t *testing.T) {
	n := 9
	s := make(qrSymbol, n)
	for y := range s {
		s[y] = make([]bool, n)
		for x := range s[y] {
			s[y][x] = (x+y)%2 == 0
		}
	}
	cs := qrContours(s, 0.02)
	if len(cs) != 41 {
		t.Errorf("%d outlines, want 41 (one per dark module)", len(cs))
	}
	m := ExtrudeContours(cs, 0, 1)
	if o, nm := m.EdgeCheck(1e-7); o+nm != 0 {
		t.Errorf("%d open, %d non-manifold edges", o, nm)
	}
	// the code part for real text is closed too, and covers the dark modules
	v := vals("qrcode", nil)
	code := qrCode(v)
	if o, nm := code.EdgeCheck(1e-7); o+nm != 0 {
		t.Errorf("code: %d open, %d non-manifold edges", o, nm)
	}
	d := qrDimsOf(v)
	dark := 0
	for _, row := range d.sym {
		for _, x := range row {
			if x {
				dark++
			}
		}
	}
	want := float64(dark) * d.u * d.u * d.high
	if got := code.Volume(); !approx(got, want, 0.01*want) {
		t.Errorf("code volume %.2f, want about %.2f", got, want)
	}
}
