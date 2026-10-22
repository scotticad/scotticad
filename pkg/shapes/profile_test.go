package shapes

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

func square(h float64) [][2]float64 {
	return [][2]float64{{-h, -h}, {h, -h}, {h, h}, {-h, h}}
}

func circle(r float64, n int) [][2]float64 {
	out := make([][2]float64, n)
	for i := range out {
		a := 2 * math.Pi * float64(i) / float64(n)
		out[i] = [2]float64{r * math.Cos(a), r * math.Sin(a)}
	}
	return out
}

func backwards(c [][2]float64) [][2]float64 {
	out := make([][2]float64, len(c))
	for i := range c {
		out[i] = c[len(c)-1-i]
	}
	return out
}

func profileValues(ss []Section, smooth bool) Values {
	v := Values{Num: map[string]float64{"smooth": 0, "steps": 8}, Sections: ss}
	if smooth {
		v.Num["smooth"] = 1
	}
	return v
}

// buildRaw builds a profile at its natural size, checks it is closed and
// returns it with its volume.
func buildRaw(t *testing.T, kind string, v Values) (*geom.Mesh, float64) {
	t.Helper()
	if err := CheckSections(kind, v.Sections); err != nil {
		t.Fatalf("CheckSections: %v", err)
	}
	k, _ := Lookup(kind)
	size, _ := k.NaturalSize(v)
	m := Build(kind, size, v)
	if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
		t.Fatalf("%s: %d open, %d non-manifold edges", kind, o, n)
	}
	if got := m.Bounds().Size(); got.Sub(size).Len() > 1e-6 {
		t.Fatalf("%s: size %v want %v", kind, got, size)
	}
	return m, m.Volume()
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol*math.Max(1, math.Abs(b)) }

func TestOutlineWithHole(t *testing.T) {
	ss := []Section{
		{Z: 2, Contours: [][][2]float64{square(10), backwards(square(4))}},
		{Z: 7, Contours: [][][2]float64{square(10), square(4)}},
	}
	_, vol := buildRaw(t, "outline", profileValues(ss, false))
	if want := (400.0 - 64) * 5; !near(vol, want, 1e-9) {
		t.Errorf("volume %v want %v", vol, want)
	}
	b := ProfileBounds("outline", profileValues(ss, false))
	if b.Min != geom.V(-10, -10, 2) || b.Max != geom.V(10, 10, 7) {
		t.Errorf("bounds %v", b)
	}
}

func TestLoftPointToPoint(t *testing.T) {
	// a square frustum: same point counts join corner to corner, exactly
	ss := []Section{{Z: 0, Contours: [][][2]float64{square(10)}}, {Z: 20, Contours: [][][2]float64{square(5)}}}
	_, vol := buildRaw(t, "loft", profileValues(ss, false))
	if want := 20.0 / 3 * (400 + 100 + 200); !near(vol, want, 1e-9) {
		t.Errorf("frustum volume %v want %v", vol, want)
	}
	// the same with one outline given clockwise: still corner to corner
	ss[1].Contours[0] = [][2]float64{{-5, -5}, {-5, 5}, {5, 5}, {5, -5}}
	if _, v2 := buildRaw(t, "loft", profileValues(ss, false)); !near(v2, vol, 1e-9) {
		t.Errorf("clockwise outline: volume %v want %v", v2, vol)
	}
}

func TestLoftDifferentCounts(t *testing.T) {
	// square to circle to small square, many points against few
	ss := []Section{
		{Z: 0, Contours: [][][2]float64{square(10)}},
		{Z: 10, Contours: [][][2]float64{circle(8, 90)}},
		{Z: 15, Contours: [][][2]float64{backwards(circle(6, 7))}},
		{Z: 25, Contours: [][][2]float64{square(3)}},
	}
	_, vol := buildRaw(t, "loft", profileValues(ss, false))
	if vol <= 0 || vol > 400*25 {
		t.Errorf("volume %v", vol)
	}
	// a cylinder from many-point circles of different counts is close to
	// pi r^2 h
	ss = []Section{{Z: 0, Contours: [][][2]float64{circle(10, 200)}}, {Z: 10, Contours: [][][2]float64{circle(10, 151)}}}
	_, vol = buildRaw(t, "loft", profileValues(ss, false))
	if want := math.Pi * 100 * 10; !near(vol, want, 0.002) {
		t.Errorf("cylinder volume %v want about %v", vol, want)
	}
}

func TestSmoothedOutlineAndCorners(t *testing.T) {
	// a smoothed square through its corners bulges past them; with each
	// corner given twice it keeps its corners and straight sides
	round := []Section{{Z: 0, Contours: [][][2]float64{square(10)}}, {Z: 5, Contours: [][][2]float64{square(10)}}}
	_, vr := buildRaw(t, "outline", profileValues(round, true))
	b := ProfileBounds("outline", profileValues(round, true))
	if !(b.Size().X > 20.5) {
		t.Errorf("a smoothed square should bulge past its points: %v", b.Size())
	}
	sharp := [][2]float64{{-10, -10}, {-10, -10}, {10, -10}, {10, -10}, {10, 10}, {10, 10}, {-10, 10}, {-10, 10}}
	corners := []Section{{Z: 0, Contours: [][][2]float64{sharp}}, {Z: 5, Contours: [][][2]float64{sharp}}}
	_, vc := buildRaw(t, "outline", profileValues(corners, true))
	if !near(vc, 2000, 1e-9) {
		t.Errorf("doubled corners should stay a square: volume %v", vc)
	}
	if vr <= vc {
		t.Errorf("rounded %v should exceed square %v", vr, vc)
	}
	// a smoothed loft stays closed
	loft := []Section{{Z: 0, Contours: [][][2]float64{square(10)}}, {Z: 8, Contours: [][][2]float64{circle(6, 5)}}, {Z: 12, Contours: [][][2]float64{square(4)}}}
	buildRaw(t, "loft", profileValues(loft, true))
}

func TestCheckSections(t *testing.T) {
	bow := [][2]float64{{0, 0}, {10, 10}, {10, 0}, {0, 10}}
	cases := []struct {
		kind string
		ss   []Section
		want string
	}{
		{"loft", []Section{{Z: 0, Contours: [][][2]float64{square(5)}}}, "at least 2 sections"},
		{"loft", []Section{{Z: 5, Contours: [][][2]float64{square(5)}}, {Z: 5, Contours: [][][2]float64{square(5)}}}, "heights must go up"},
		{"loft", []Section{{Z: 0, Contours: [][][2]float64{square(5), square(2)}}, {Z: 5, Contours: [][][2]float64{square(5)}}}, "exactly one outline"},
		{"outline", []Section{{Z: 0, Contours: [][][2]float64{bow}}, {Z: 5, Contours: [][][2]float64{bow}}}, "crosses itself"},
		{"outline", []Section{{Z: 0, Contours: [][][2]float64{{{0, 0}, {1, 1}}}}, {Z: 5}}, "at least 3"},
		{"outline", []Section{{Z: 0, Contours: [][][2]float64{{{0, 0}, {1, 1}, {2, 2}}}}, {Z: 5}}, "no area"},
		{"outline", []Section{{Z: 0, Contours: [][][2]float64{{{0, 0}, {math.NaN(), 1}, {2, 0}}}}, {Z: 5}}, "out of range"},
		{"outline", []Section{{Z: 5, Contours: [][][2]float64{square(5)}}, {Z: 0, Contours: [][][2]float64{square(5)}}}, "above its bottom"},
		{"outline", []Section{{Z: 0, Contours: [][][2]float64{square(5)}}, {Z: 5}}, "no outline"},
		// a spike folding back on itself
		{"outline", []Section{{Z: 0, Contours: [][][2]float64{{{0, 0}, {10, 0}, {10, 10}, {10, 3}}}}, {Z: 5, Contours: [][][2]float64{square(5)}}}, "crosses itself"},
	}
	for _, c := range cases {
		err := CheckSections(c.kind, c.ss)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %v: got %v, want %q", c.kind, c.ss, err, c.want)
		}
	}
	// a closed outline repeating its first point at the end is fine
	closed := append(square(5), [2]float64{-5, -5})
	if err := CheckSections("outline", []Section{{Z: 0, Contours: [][][2]float64{closed}}, {Z: 1, Contours: [][][2]float64{closed}}}); err != nil {
		t.Errorf("closed outline: %v", err)
	}
}

func TestBigOutlinesAreQuick(t *testing.T) {
	c := circle(50, 5000)
	start := time.Now()
	ss := []Section{{Z: 0, Contours: [][][2]float64{c}}, {Z: 3, Contours: [][][2]float64{circle(40, 3000)}}, {Z: 6, Contours: [][][2]float64{c}}}
	buildRaw(t, "loft", profileValues(ss, false))
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("a 13,000-point loft took %v", d)
	}
}

func TestDamagedProfileBuildsABox(t *testing.T) {
	// data a file could hold but CheckSections refuses: a box, not a crash
	v := profileValues([]Section{{Z: 0, Contours: [][][2]float64{{{0, 0}}}}}, false)
	m := Build("loft", geom.V(5, 6, 7), v)
	if got := m.Bounds().Size(); got != geom.V(5, 6, 7) {
		t.Errorf("fallback size %v", got)
	}
}
