package shapes

import (
	"math"
	"testing"
	"time"
)

// sketchValues is a kind's default settings with a drawing and some
// settings changed.
func sketchValues(kind string, sk *Sketch, set map[string]float64) Values {
	k, _ := Lookup(kind)
	v := k.Defaults()
	v.Sketch = sk
	for key, x := range set {
		v.Num[key] = x
	}
	return v
}

func buildSketch(kind string, v Values) (tris int, closed func(t *testing.T, name string)) {
	m := Build(kind, SketchBounds(kind, v).Size(), v)
	return len(m.Tris), func(t *testing.T, name string) { t.Helper(); solidOK(t, name, m) }
}

// An outline with points in line to within rounding (a wide outside stroke
// round a zigzag makes them) stays closed when it tapers or twists: the
// sliver the caps' triangulation makes of them must not leave a gap in the
// walls.
func TestSketchLoftNearCollinearIsClosed(t *testing.T) {
	p := SketchPath{Closed: true, Points: skPts(0, 0, -5, 5, 0, 5, -5, 10, 0, 10), Stroke: 30, Align: AlignOutside}
	sk := &Sketch{Paths: []SketchPath{p}}
	for name, set := range map[string]map[string]float64{
		"tapered":       {"topscale": 30},
		"smooth twist":  {"twist": 30, "twistmode": 1},
		"stepped twist": {"twist": 30, "twiststeps": 5},
		"straight":      {},
	} {
		_, closed := buildSketch("sketch", sketchValues("sketch", sk, set))
		closed(t, name)
	}
}

// Settings from a file can be anything: the box a sketch fills is worked
// out from them as Build would use them, not as written.
func TestSketchBoundsUseClampedSettings(t *testing.T) {
	sk := &Sketch{Paths: []SketchPath{sq(-12, 0, -8, 4)}}
	for _, kind := range []string{"revolve", "coil", "sketch"} {
		for _, set := range []map[string]float64{
			{"sides": 1e18}, {"sides": math.NaN()}, {"sides": -3}, {"sweep": -90}, {"sweep": 1e9},
			{"quality": 1e18}, {"turns": 1e12}, {"pitch": -1}, {"twiststeps": 1e18}, {"twist": math.Inf(1)},
		} {
			v := sketchValues(kind, sk, set)
			got := SketchBounds(kind, v)
			k, _ := Lookup(kind)
			want := sketchModel(kind, k.clamp(v)).Bounds()
			if got != want {
				t.Errorf("%s %v: bounds %v, but it builds %v", kind, set, got, want)
			}
		}
	}
}

// The sliders' ends can't ask for a mesh too big to work with: the sides,
// layers and cuts give way to keep it within sketchTriBudget.
func TestSketchTriangleBudget(t *testing.T) {
	ring := func(n int) *Sketch { // a closed curve of n automatic points round (cx, 0)
		p := SketchPath{Closed: true}
		for i := 0; i < n; i++ {
			a := 2 * math.Pi * float64(i) / float64(n)
			p.Points = append(p.Points, SketchPoint{P: [2]float64{-40 + 20*math.Cos(a), 30 + 20*math.Sin(a)}, Kind: PointAuto})
		}
		return &Sketch{Paths: []SketchPath{p}}
	}
	for _, c := range []struct {
		name, kind string
		sk         *Sketch
		set        map[string]float64
	}{
		{"coil at its sliders' ends", "coil", ring(4), map[string]float64{"turns": 50, "sides": 128, "quality": 64}},
		{"revolve of a long outline", "revolve", ring(1000), map[string]float64{"sides": 128, "quality": 64}},
		{"smooth twist of a long outline", "sketch", ring(1000), map[string]float64{"twist": 90, "twistmode": 1}},
		{"360 twist steps of a long outline", "sketch", ring(1000), map[string]float64{"twist": 90, "twiststeps": 360}},
	} {
		start := time.Now()
		tris, closed := buildSketch(c.kind, sketchValues(c.kind, c.sk, c.set))
		if tris > sketchTriBudget*11/10 {
			t.Errorf("%s: %d triangles, over the budget of %d", c.name, tris, sketchTriBudget)
		}
		closed(t, c.name)
		if d := time.Since(start); d > 20*time.Second {
			t.Errorf("%s: took %v", c.name, d)
		}
	}
	// well inside the budget nothing gives way: a coil has its sides as set
	sk := &Sketch{Paths: []SketchPath{sq(-12, 0, -8, 4)}}
	if tris, _ := buildSketch("coil", sketchValues("coil", sk, map[string]float64{"turns": 3, "sides": 48})); tris != 3*48*4*2+4 {
		t.Errorf("a small coil has %d triangles, want %d", tris, 3*48*4*2+4)
	}
}

// A drawing whose only bends are in its curves still builds at Quality 1,
// where each curve would be a single straight piece and a two-point closed
// path nothing at all.
func TestSketchQualityOneStillBuilds(t *testing.T) {
	lens := SketchPath{Closed: true, Points: []SketchPoint{
		{P: [2]float64{-20, 0}, In: [2]float64{0, -6}, Out: [2]float64{0, 6}},
		{P: [2]float64{-10, 0}, In: [2]float64{0, 6}, Out: [2]float64{0, -6}},
	}}
	sk := &Sketch{Paths: []SketchPath{lens}}
	for _, kind := range []string{"sketch", "revolve", "coil"} {
		if !Buildable(kind, sk) {
			t.Fatalf("%s: the lens isn't buildable", kind)
		}
		v := sketchValues(kind, sk, map[string]float64{"quality": 1})
		if SketchBounds(kind, v).IsEmpty() {
			t.Errorf("%s: nothing at quality 1", kind)
			continue
		}
		_, closed := buildSketch(kind, v)
		closed(t, kind+" lens at quality 1")
	}
}

// A long outline is built in a time a person would wait: the work must not
// grow with the square of its points (4000 points would take seconds as an
// area and far longer as a line).
func TestSketchLongOutlineIsQuick(t *testing.T) {
	p := SketchPath{Closed: true}
	const n = 4000
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / n
		r := 50 + 3*math.Sin(9*a)
		p.Points = append(p.Points, SketchPoint{P: [2]float64{r * math.Cos(a), r * math.Sin(a)}, Kind: PointAuto})
	}
	for _, stroke := range []float64{0, 2} {
		p.Stroke = stroke
		sk := &Sketch{Paths: []SketchPath{p}}
		start := time.Now()
		cs := SketchOutlinesN(sk, 12)
		d := time.Since(start)
		if len(cs) == 0 {
			t.Fatalf("stroke %v: no outline", stroke)
		}
		t.Logf("stroke %v: %d points in %v", stroke, n, d)
		if d > 5*time.Second {
			t.Errorf("stroke %v: outlines of %d points took %v", stroke, n, d)
		}
	}
}
