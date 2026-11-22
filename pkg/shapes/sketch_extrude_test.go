package shapes

import (
	"math"
	"testing"
)

func outlineArea(sk *Sketch) float64 {
	a := 0.0
	for _, c := range SketchOutlines(sk) {
		a += SignedArea(c) // holes count negative
	}
	return a
}

// A closed line can sit on its path's middle, inside it or outside it.
func TestSketchLineAlignment(t *testing.T) {
	path := sq(0, 0, 20, 20)
	path.Stroke = 2
	for _, c := range []struct {
		align int
		join  int
		want  float64
	}{
		{AlignCenter, JoinSharp, 22*22 - 18*18},
		{AlignInside, JoinSharp, 20*20 - 16*16},
		{AlignOutside, JoinSharp, 24*24 - 20*20},
		// outside with round corners: each a quarter circle of radius 2 in
		// place of a 2 × 2 square
		{AlignOutside, JoinRound, 24*24 - 20*20 - 4*(4-math.Pi)},
	} {
		p := path
		p.Align, p.Join = c.align, c.join
		sk := &Sketch{Paths: []SketchPath{p}}
		got := outlineArea(sk)
		tol := 1e-6
		if c.join == JoinRound {
			tol = 0.01 // the arcs are polygons
		}
		if !skNear(got, c.want, tol) {
			t.Errorf("align %d join %d: area %v, want %v", c.align, c.join, got, c.want)
		}
		solidOK(t, "aligned line", SketchSolid(sk, 0, 2))
	}
	// an open path has no inside: alignment leaves it centred
	open := SketchPath{Points: skPts(0, 0, 30, 0), Stroke: 2, Align: AlignOutside}
	if got := outlineArea(&Sketch{Paths: []SketchPath{open}}); !skNear(got, 60, 1e-6) {
		t.Errorf("open line: area %v, want 60", got)
	}
}

// A bevelled corner is cut square to the corner's bisector a full width
// from the corner: a square join.
func TestSketchBevelIsSquare(t *testing.T) {
	// a square with a 2 mm line outside: at each corner the cut is 2 mm out
	// along the diagonal, so it takes off a triangle with height
	// 2√2 − 2 and base twice that
	p := sq(0, 0, 20, 20)
	p.Stroke, p.Align, p.Join = 2, AlignOutside, JoinBevel
	h := 2*math.Sqrt2 - 2
	want := 24.0*24 - 20*20 - 4*h*h
	if got := outlineArea(&Sketch{Paths: []SketchPath{p}}); !skNear(got, want, 1e-6) {
		t.Errorf("area %v, want %v", got, want)
	}
	// every outline point is at least the width from the corner it rounds
	for _, c := range SketchOutlines(&Sketch{Paths: []SketchPath{p}}) {
		for _, q := range c {
			if q.X > 20 && q.Y > 20 && math.Hypot(q.X-20, q.Y-20) < 2-1e-9 {
				t.Errorf("point %v is nearer the corner than the width", q)
			}
		}
	}
}

// A reference outline: a boot-shaped closed path with a 0.9 mm line
// outside it and bevelled corners covers 34.3025 mm².
func TestSketchBevelReferenceArea(t *testing.T) {
	pt := func(x, y float64) SketchPoint { return SketchPoint{P: [2]float64{x / 10, -y / 10}} }
	pts := []SketchPoint{
		{P: [2]float64{-10, 0}, Out: [2]float64{-4, -5}},
		{P: [2]float64{-6, 0}, In: [2]float64{4, -3}},
		pt(-60, -30), pt(-60, -40), pt(-30, -40), pt(-25, -70), pt(-85, -70), pt(-80, -50), pt(-100, -50),
		pt(-100, -36.6052), pt(-100, -23.0613),
	}
	p := SketchPath{Closed: true, Points: pts, Stroke: 0.9, Align: AlignOutside, Join: JoinBevel}
	got := outlineArea(&Sketch{Paths: []SketchPath{p}})
	if math.Abs(got-34.3025) > 0.01*34.3025 {
		t.Errorf("area %v, want 34.3025", got)
	}
}

// Taper and twist: the scales and the turn act round the drawing's middle,
// and the solid stays closed, with a point at a scale of 0.
func TestSketchExtrudeTaperTwist(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{sq(-10, -10, 10, 10)}}
	cases := []struct {
		name string
		e    Extrude
		vol  float64
	}{
		{"straight", Extrude{TopScale: 100, BaseScale: 100}, 4000},
		{"both halved", Extrude{TopScale: 50, BaseScale: 50}, 1000},
		// a frustum: h/3 · (A0 + A1 + √(A0·A1))
		{"tapered", Extrude{TopScale: 50, BaseScale: 100}, 10.0 / 3 * (400 + 100 + 200)},
		{"pyramid", Extrude{TopScale: 0, BaseScale: 100}, 10.0 / 3 * 400},
		{"upside-down pyramid", Extrude{TopScale: 100, BaseScale: 0}, 10.0 / 3 * 400},
		// a quarter turn in one layer brings each corner onto the next:
		// the plain box's volume
		{"twisted a quarter", Extrude{TopScale: 100, BaseScale: 100, Twist: 90}, 4000},
	}
	for _, c := range cases {
		m := SketchExtrude(sk, 0, 10, c.e)
		v := solidOK(t, c.name, m)
		if !skNear(v, c.vol, 1e-6) {
			t.Errorf("%s: volume %v, want %v", c.name, v, c.vol)
		}
		b := m.Bounds()
		if !skNear(b.Min.X+b.Max.X, 0, 1e-9) || !skNear(b.Min.Y+b.Max.Y, 0, 1e-9) {
			t.Errorf("%s: off the middle: %v", c.name, b)
		}
	}
	// the top of a twist turns: the top's corner is at 45° after 45°
	m := SketchExtrude(sk, 0, 10, Extrude{TopScale: 100, BaseScale: 100, Twist: 45})
	top := 0.0
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			if p.Z == 10 {
				top = math.Max(top, p.Y)
			}
		}
	}
	if !skNear(top, 10*math.Sqrt2, 1e-9) {
		t.Errorf("twisted top reaches y %v, want %v", top, 10*math.Sqrt2)
	}
}

// Reference solids of a 10 mm box sketch, 10 tall, standing at
// (10…20, 10…20): Top Scale 50 is a frustum about the middle, and Twist
// 45 (Twist Steps 0) one layer, each side folded from its end at the
// bottom to its start at the top, 1138.0804 mm³.
func TestSketchExtrudeReference(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{sq(10, 10, 20, 20)}}
	m := SketchExtrude(sk, 0, 10, Extrude{TopScale: 50, BaseScale: 100})
	if v := solidOK(t, "top scale 50", m); !skNear(v, 583.3380, 1e-5) {
		t.Errorf("top scale 50: volume %v, want 583.3380", v)
	}
	m = SketchExtrude(sk, 0, 10, Extrude{TopScale: 100, BaseScale: 100, Twist: 45})
	if v := solidOK(t, "twist 45", m); !skNear(v, 1138.0804, 1e-5) {
		t.Errorf("twist 45: volume %v, want 1138.0804", v)
	}
	if b := m.Bounds(); !skNear(b.Min.X, 7.9289, 1e-4) || !skNear(b.Max.Y, 22.0711, 1e-4) || len(m.Tris) != 12 {
		t.Errorf("twist 45: bounds %v, %d triangles; want 7.9289…22.0711, 12", b, len(m.Tris))
	}
}

// Reference solids of a revolved 6 × 6 square (radius 10…16, 6 high),
// 48 segments: Sweep 180 from Start 30 runs 120°…300° in 3.75° steps,
// 1469.2354 mm³; Sweep 90 from Start 0 runs 90°…180°, 735.0113 mm³.
func TestRevolveReference(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{sq(-16, 0, -10, 6)}}
	for _, c := range []struct {
		start, sweep, from, to, vol float64
	}{
		{30, 180, 120, 300, 1469.2354},
		{0, 90, 90, 180, 735.0113},
	} {
		m := revolveMesh(sk, c.start, c.sweep, 48, 12)
		v := solidOK(t, "revolve", m)
		if !skNear(v, c.vol, 5e-5) { // an STL's float32 points: its 10 mm box is 1000.0082
			t.Errorf("start %v sweep %v: volume %v, want %v", c.start, c.sweep, v, c.vol)
		}
		angs := map[float64]bool{}
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, tr := range m.Tris {
			for _, p := range tr.P {
				if math.Hypot(p.X, p.Y) > 15.9 {
					a := math.Mod(math.Atan2(p.Y, p.X)*180/math.Pi+360, 360)
					a = math.Round(a*1000) / 1000
					if c.to > 270 && a < 90 { // past 360
						a += 360
					}
					angs[a] = true
					lo, hi = math.Min(lo, a), math.Max(hi, a)
				}
			}
		}
		if !skNear(lo, c.from, 1e-6) || !skNear(hi, c.to, 1e-6) || len(angs) != 49 {
			t.Errorf("start %v sweep %v: %v°…%v° in %d angles, want %v…%v in 49", c.start, c.sweep, lo, hi, len(angs), c.from, c.to)
		}
	}
}

// A reference solid: an L of a square with a tab (0…14 × 0…10), 10
// tall, Twist 25 in Twist Steps 5 (Twist Mode STEP): six layers, 80
// triangles, 1226.9258 mm³ (float32 points), turned about the drawing's
// middle to x −1.457…14, y −2.490…12.490.
func TestSketchTwistStepsReference(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{{Closed: true, Points: skPts(0, 0, 10, 0, 10, 5, 14, 5, 14, 10, 10, 10, 0, 10)}}}
	m := SketchExtrude(sk, 0, 10, Extrude{TopScale: 100, BaseScale: 100, Twist: 25, TwistSteps: 5})
	if v := solidOK(t, "twist steps 5", m); !skNear(v, 1226.9258, 5e-5) {
		t.Errorf("volume %v, want 1226.9258", v)
	}
	b := m.Bounds()
	if len(m.Tris) != 80 || !skNear(b.Min.X, -1.457, 1e-3) || !skNear(b.Min.Y, -2.490, 1e-3) || !skNear(b.Max.Y, 12.490, 1e-3) {
		t.Errorf("%d triangles, bounds %v; want 80, x −1.457, y −2.490…12.490", len(m.Tris), b)
	}
}

// The same L, at x 30…44, Twist 30 in Twist Mode
// Smooth: 96 layers, each side cut into 8 (48 points a layer),
// 1200.2661 mm³, x 28.4378…44, y −2.8301…12.8302.
func TestSketchSmoothTwistReference(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{{Closed: true, Points: skPts(30, 0, 40, 0, 40, 5, 44, 5, 44, 10, 40, 10, 30, 10)}}}
	m := SketchExtrude(sk, 0, 10, Extrude{TopScale: 100, BaseScale: 100, Twist: 30, Smooth: true})
	if v := solidOK(t, "smooth twist", m); !skNear(v, 1200.2661, 5e-5) {
		t.Errorf("volume %v, want 1200.2661", v)
	}
	b := m.Bounds()
	if !skNear(b.Min.X, 28.4378, 1e-4) || !skNear(b.Min.Y, -2.8301, 1e-4) || !skNear(b.Max.Y, 12.8302, 1e-4) {
		t.Errorf("bounds %v; want x 28.4378…44, y −2.8301…12.8302", b)
	}
	levels := map[float64]bool{}
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			levels[math.Round(p.Z*1e6)/1e6] = true
		}
	}
	if len(levels) != 97 {
		t.Errorf("%d heights, want 97", len(levels))
	}
	// another L (4…14 × −24…−4, a corner cut out), Smooth at 30° and 180°:
	// 96 layers and 8 cuts a side whatever the angle
	sk = &Sketch{Paths: []SketchPath{{Closed: true, Points: skPts(4, -24, 10, -24, 10, -20, 14, -20, 14, -4, 4, -4)}}}
	for _, c := range []struct {
		height, twist, vol, minX, maxX, minY float64
	}{
		{10, 30, 1840.4710, -0.3302, 16.3302, -25.1804},
		{10, 180, 1842.4920, -2.1802, 20.1802, -25.1802},
		// 30 tall: still 96 layers
		{30, 30, 5521.4129, -0.3302, 16.3302, -25.1804},
	} {
		m := SketchExtrude(sk, 0, c.height, Extrude{TopScale: 100, BaseScale: 100, Twist: c.twist, Smooth: true})
		v := solidOK(t, "smooth twist", m)
		b := m.Bounds()
		// (triangle counts aren't compared: caps can be cut up in more
		// than one way without changing the shape)
		if !skNear(v, c.vol, 5e-5) || !skNear(b.Min.X, c.minX, 1e-4) || !skNear(b.Max.X, c.maxX, 1e-4) || !skNear(b.Min.Y, c.minY, 1e-4) {
			t.Errorf("twist %v: volume %v, x %v…%v, y from %v; want %v, %v…%v, %v",
				c.twist, v, b.Min.X, b.Max.X, b.Min.Y, c.vol, c.minX, c.maxX, c.minY)
		}
	}
}

// A coil's profile stands in a plane through its axis, so its rise is a
// shear: its volume is the revolve's, times the turns (Pappus: 2π × the
// area × its centroid's radius), less a little for the sides' chords.
func TestCoil(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{sq(-12, 0, -8, 4)}}
	m := coilMesh(sk, 3, 6, 0, 48, 12)
	v := solidOK(t, "coil", m)
	if want := 3 * 2 * math.Pi * 16 * 10; !skNear(v, want, 5e-3) { // the sides are twisted strips of triangles
		t.Errorf("volume %v, want %v", v, want)
	}
	if b := m.Bounds(); !skNear(b.Min.Z, 0, 1e-9) || !skNear(b.Max.Z, 4+3*6, 1e-9) {
		t.Errorf("height %v…%v, want 0…22", b.Min.Z, b.Max.Z)
	}
	// a pitch below the profile's height is raised, so turns never touch
	m = coilMesh(sk, 2.5, 1, 30, 24, 12)
	solidOK(t, "tight coil", m)
	if b := m.Bounds(); b.Max.Z < 4+2.5*4 {
		t.Errorf("tight coil rose to %v: its pitch wasn't raised to the profile's 4", b.Max.Z)
	}
	// a profile reaching the axis is kept just off it, and stays closed
	solidOK(t, "coil on the axis", coilMesh(&Sketch{Paths: []SketchPath{sq(-6, 0, 3, 3)}}, 2, 5, 0, 32, 12))
	// the shape and its default drawing (a round wire: a spring)
	k, _ := Lookup("coil")
	solidOK(t, "default coil", Build("coil", k.Size, k.Defaults()))
	if Buildable("coil", &Sketch{Paths: []SketchPath{sq(1, 0, 5, 5)}}) {
		t.Error("a coil right of the center line is buildable")
	}
}

// Quality cuts each curve into that many pieces.
func TestSketchQuality(t *testing.T) {
	k := 0.5523 * 10
	p := SketchPath{Closed: true, Points: []SketchPoint{
		{P: [2]float64{10, 0}, Out: [2]float64{0, k}},
		{P: [2]float64{0, 10}, In: [2]float64{k, 0}},
		{P: [2]float64{0, 0}},
	}}
	sk := &Sketch{Paths: []SketchPath{p}}
	for _, q := range []int{1, 4, 12} {
		cs := SketchOutlinesN(sk, q)
		if len(cs) != 1 || len(cs[0]) != q+2 {
			t.Errorf("quality %d: %d outlines, %d points", q, len(cs), len(cs[0]))
		}
	}
	// the shape's setting
	v := Values{Num: map[string]float64{"quality": 4}, Sketch: sk}
	m := Build("sketch", SketchBounds("sketch", v).Size(), v)
	solidOK(t, "quality 4", m)
}
