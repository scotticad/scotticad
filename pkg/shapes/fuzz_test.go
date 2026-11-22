package shapes

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Fuzz tests. Run one with, e.g.
//
//	go test ./pkg/shapes -run '^$' -fuzz '^FuzzSVGMesh$' -fuzztime 3m
//
// Crashers are saved under testdata/fuzz/<Name>/ and run as seeds by a
// plain go test. FuzzBuild's largest shapes (a hex bolt at its longest)
// take seconds each: on a machine with many cores, run it with -parallel 4
// or so, or the workers building them at once starve each other and the
// fuzzer reports a hang. The same goes for FuzzSketch: eight thick paths
// crossing each other dozens of times take seconds to join.
//
// To look past a known finding while fuzzing, set SCOTTICAD_FUZZ_RELAX to a comma-separated list of checks to skip:
// "nan" (non-finite vertices), "edges" (open edges), "time" (slow inputs).

func fzRelaxed(check string) bool {
	for _, c := range strings.Split(os.Getenv("SCOTTICAD_FUZZ_RELAX"), ",") {
		if strings.TrimSpace(c) == check {
			return true
		}
	}
	return false
}

// fzSlack stretches every fzWithin limit. The limits are written as what
// an input should take on an idle machine; the fuzzer runs a worker per
// core, and on a busy machine an input that takes 3 s alone can take over
// 15 and fail as a hang when it isn't one. A real hang still trips it.
const fzSlack = 4

// fzWithin runs f and fails if it doesn't finish within d (times fzSlack).
// A panic in f crashes the test binary with f's own stack, which the
// fuzzer records as a crasher like any other.
func fzWithin(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	d *= fzSlack
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(d):
		if fzRelaxed("time") {
			<-done
			return
		}
		t.Fatalf("%s: still running after %v", what, d)
	}
}

func fzFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func fzMeshFinite(t *testing.T, what string, m *geom.Mesh) {
	t.Helper()
	if m == nil {
		t.Fatalf("%s: nil mesh", what)
	}
	if fzRelaxed("nan") {
		return
	}
	for i, tr := range m.Tris {
		for _, p := range tr.P {
			if !fzFinite(p.X) || !fzFinite(p.Y) || !fzFinite(p.Z) {
				t.Fatalf("%s: triangle %d of %d has a non-finite vertex %v", what, i, len(m.Tris), p)
			}
		}
	}
}

var fzSVGSeeds = []string{
	`<svg xmlns="http://www.w3.org/2000/svg" width="20mm" height="10mm" viewBox="0 0 200 100"><rect x="0" y="0" width="100" height="100"/></svg>`,
	`<svg width="100mm" height="100mm" viewBox="0 0 100 100"><rect x="0" y="0" width="10" height="10"/><rect x="5" y="5" width="10" height="10"/></svg>`,
	`<svg width="100mm" height="100mm" viewBox="0 0 100 100"><path d="M0 0h20v20h-20z M5 5v10h10v-10z"/></svg>`,
	`<svg viewBox="0 0 100 100"><path d="M 20 80 A 5 5 0 0 1 30 80 L 30 85 Z"/><path d="M10 10 a 20 10 30 1 0 30 0 z"/></svg>`,
	`<svg viewBox="0 0 100 100"><path d="M10,10 C20,0 40,0 50,10 S80,20 90,10 Q95,50 90,90 T10,90 Z"/></svg>`,
	`<svg viewBox="0 0 100 100"><path d="m10 10 l10 0 0 10 -10 0z m30 0 h10 v10 h-10 z"/></svg>`,
	`<svg viewBox="0 0 100 100"><path d="M0 0 L10 0 L0 10 Z 5 5"/><path d="M0 0 L10 0 L0 10 z.5"/></svg>`,
	`<svg viewBox="0 0 100 100"><path d="M1e1.5 2e-1L3.5.5.5 3.5z"/></svg>`,
	`<svg viewBox="0 0 100 100"><g transform="translate(10,20) rotate(30 5 5) scale(2,0.5)"><g transform="matrix(1 0 0.5 1 3 4) skewX(10) skewY(-5)"><rect width="10" height="10"/></g></g></svg>`,
	`<svg width="100mm" height="50mm" viewBox="0 0 100 100" preserveAspectRatio="xMinYMax slice"><rect width="100" height="100"/></svg>`,
	`<svg width="100mm" height="50mm" viewBox="0 0 100 100" preserveAspectRatio="none"><circle cx="50" cy="50" r="40"/><circle cx="50" cy="50" r="20"/></svg>`,
	`<svg width="4in" height="3cm" viewBox="-10 -10 100 100" preserveAspectRatio="xMaxYMid"><ellipse cx="50" cy="50" rx="40" ry="10"/></svg>`,
	`<svg width="100pt" height="10pc"><polygon points="0,0 10,0 10,10 0,10"/><polyline points="20 0 30 0 30 10"/></svg>`,
	`<svg viewBox="0 0 100 100"><g style="display: none"><rect width="5" height="5"/></g><rect style="display:none" width="5" height="5"/><rect display="none" width="5" height="5"/><rect x="50" width="5" height="5"/></svg>`,
	`<svg viewBox="0 0 100 100"><defs><rect id="r" width="999" height="999"/></defs><clipPath><rect width="9" height="9"/></clipPath><title>t</title><metadata/><rect x="1" y="1" width="3" height="3"/></svg>`,
	`<svg viewBox="0 0 100 100"><g fill="none" stroke="black"><rect x="0" y="0" width="20" height="20"/><rect x="5" y="5" width="10" height="10"/></g></svg>`,
	`<svg viewBox="0 0 100 100"><rect x="0" y="0" width="10" height="10"/><rect x="10" y="0" width="10" height="10"/><rect x="0" y="10" width="10" height="10"/></svg>`,
	`<svg viewBox="0 0 0 0" width="0" height="-5"><rect width="-1" height="5"/><circle r="0"/></svg>`,
	`<?xml version="1.0"?><!DOCTYPE svg><svg><!-- c --><rect width="1" height="1"/></svg>`,
	`<svg><rect width="1" height="1">`,
	`<svg><svg width="5mm" viewBox="0 0 1 1"><rect width="1" height="1"/></svg></svg>`,
	// an arc whose radius scale overflows: SVGContours returns NaN points
	`<svg><path d="M0 0 A 1e-300 1e-300 0 1 1 1e300 1e300 Z"/><rect width="5" height="5"/></svg>`,
	`not svg`,
	``,
}

func FuzzSVGMesh(f *testing.F) {
	for _, s := range fzSVGSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		var m *geom.Mesh
		var err error
		fzWithin(t, 2*time.Second, "SVGMesh", func() { m, err = SVGMesh(strings.NewReader(src), 2, 1) })
		if err != nil {
			return
		}
		fzMeshFinite(t, "SVGMesh", m)
		if len(m.Tris) == 0 {
			t.Fatal("SVGMesh returned an empty mesh without an error")
		}
		if !fzRelaxed("edges") {
			if open, nm := m.EdgeCheck(1e-7); open != 0 {
				t.Fatalf("SVGMesh result has %d open and %d non-manifold edges", open, nm)
			}
		}
	})
}

func FuzzSVGContours(f *testing.F) {
	for _, s := range fzSVGSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		var cs [][]Pt
		var err error
		fzWithin(t, 2*time.Second, "SVGContours", func() { cs, err = SVGContours(strings.NewReader(src)) })
		if err != nil {
			return
		}
		if len(cs) == 0 {
			t.Fatal("no outlines and no error")
		}
		for i, c := range cs {
			if len(c) < 3 {
				t.Fatalf("outline %d has %d points", i, len(c))
			}
			if fzRelaxed("nan") {
				continue
			}
			for _, p := range c {
				if !fzFinite(p.X) || !fzFinite(p.Y) {
					t.Fatalf("outline %d has a non-finite point %v", i, p)
				}
			}
		}
	})
}

func FuzzParsePath(f *testing.F) {
	for _, s := range []string{
		"M0 0 L10 0 L0 10 Z", "M0 0 L10 0 L0 10 Z 5 5", "M0 0 L10 0 L0 10 z.5",
		"M0 0 L10 0 L10 10 Z L0 10 L-10 10 Z", "m1 1 h5 v5 h-5 z m10 0 h5 v5 h-5 z",
		"M10 10 A 5 5 0 0 1 30 80", "M10 10 a0 0 0 1 1 5 5 z", "M0 0 A 1 1 0 1 1 0 0 Z",
		"M0 0 A 1e-170 1e-170 0 0 1 10 0 L 5 5 Z",
		"M0 0 A 1e-300 1e-300 0 1 1 1e300 1e300 Z", // NaN: the radius scale overflows (svgArc)
		"M0,0 A5,5,45,1,0,10,0 L5 5z", "M0 0 A5 5 0 105 5 L0 5z",
		"M0 0 C1 2 3 4 5 6 S7 8 9 10 Z", "M0 0 Q1 2 3 4 T5 6 t1 1 Z", "M1e5.5 2L.5.5-1-1z",
		"M 1 1 L 1e308 1e308 L -1e308 1e308 Z", "Z", "M", "L1 2 3", "", "M0 0e", "M0 0 L 1 1 1 1 1 1 1",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, d string) {
		var cs [][]Pt
		fzWithin(t, 2*time.Second, "parsePath", func() { cs = parsePath(d) })
		for i, c := range cs {
			if len(c) < 3 {
				t.Fatalf("outline %d has %d points", i, len(c))
			}
			if fzRelaxed("nan") {
				continue
			}
			for _, p := range c {
				if math.IsNaN(p.X) || math.IsNaN(p.Y) {
					t.Fatalf("outline %d has a NaN point %v", i, p)
				}
			}
		}
	})
}

func FuzzNumbers(f *testing.F) {
	for _, s := range []string{"1-2", "1.5.5", "1e5.5 2", "1e", "1e+", "-.5e-3,+7", "--1", "1e999", "0 0 100 100", ".", "e5", "1E-400"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var ns []float64
		fzWithin(t, time.Second, "numbers", func() { ns = numbers(s) })
		if len(ns) > len(s) {
			t.Fatalf("%d numbers from %d bytes", len(ns), len(s))
		}
		for _, v := range ns {
			if !fzFinite(v) {
				t.Fatalf("non-finite number %v", v)
			}
		}
	})
}

// fzParam turns a fuzzed value into a parameter value; the low bits of
// sel pick special values so that each is reached often.
func fzParam(v float64, sel uint8) float64 {
	switch sel % 8 {
	case 0:
		return math.NaN()
	case 1:
		return math.Inf(1)
	case 2:
		return math.Inf(-1)
	case 3:
		return -math.Abs(v)
	case 4:
		return 1e300
	}
	return v
}

// FuzzTube builds bent tubes from bend tables made of the fuzzer's bytes:
// three per row, read as a length, an angle and a turn that reach a little
// past what a table may hold. A table CheckBends refuses builds the
// stand-in; one it accepts builds a tube that is finite, closed as built,
// and starts at the origin heading along +X.
func FuzzTube(f *testing.F) {
	f.Add([]byte{20, 148, 128, 20, 128, 128}, 10.0, 0.0, 15.0, 32.0) // the default L
	f.Add([]byte{0, 255, 0, 0, 0, 255, 255, 128, 128}, 10.0, 2.0, 5.0, 8.0)
	f.Add([]byte{1, 200, 190, 1, 60, 60, 1, 200, 190, 1, 60, 60}, 50.0, 20.0, 0.5, 64.0)
	f.Add([]byte{}, 1.0, 0.0, 200.0, 3.0)
	f.Add([]byte{255, 128, 128}, math.NaN(), math.Inf(1), -5.0, 1e9)
	k, _ := Lookup("benttube")
	f.Fuzz(func(t *testing.T, rows []byte, dia, wall, bend, sides float64) {
		// up to 40 rows: a full table of 200 is half a million triangles,
		// too many to build in every one of the fuzzer's workers at once
		// (TestTubeLongestTable builds one)
		if len(rows) > 3*40 {
			rows = rows[:3*40]
		}
		var bs []Bend
		for i := 0; i+2 < len(rows); i += 3 {
			bs = append(bs, Bend{
				Length: float64(rows[i]) - 2,             // -2 to 253
				Angle:  (float64(rows[i+1]) - 128) * 1.5, // -192 to 190.5
				Turn:   (float64(rows[i+2]) - 128) * 3,   // -384 to 381
			})
		}
		// the parameters as Build gives them to the shape: held to their sliders' ranges
		v := k.clamp(tubeValues(bs, dia, wall, bend, sides))
		var m *geom.Mesh
		fzWithin(t, 15*time.Second, "tubeMesh", func() { m = tubeMesh(v) })
		if CheckBends(bs) != nil {
			if m != nil {
				t.Fatalf("a table CheckBends refuses built a tube: %v", bs)
			}
			if b := Build("benttube", geom.V(10, 10, 10), v); b == nil || len(b.Tris) == 0 {
				t.Fatal("no stand-in for a refused table")
			}
			return
		}
		if m == nil {
			// accepted, but nothing to sweep: every run and bend too small to count
			if p, _ := TubePath(bs, 15); len(p) >= 2 {
				t.Fatalf("an accepted table with a path of %d stations built nothing: %v", len(p), bs)
			}
			return
		}
		fzMeshFinite(t, "tubeMesh", m)
		if open, nm := m.EdgeCheck(0); open != 0 || nm != 0 {
			t.Fatalf("tube not closed as built: %d open, %d non-manifold edges; table %v, parameters %v", open, nm, bs, v.Num)
		}
		_, _, r, _ := tubeDims(v)
		path, ends := TubePath(bs, r)
		if len(ends) != len(bs) || path[0].P != (geom.Vec3{}) || path[0].T != geom.V(1, 0, 0) {
			t.Fatalf("path: %d row ends for %d rows, starting at %v heading %v", len(ends), len(bs), path[0].P, path[0].T)
		}
		for i, s := range path {
			if math.Abs(s.T.Len()-1) > 1e-6 || math.Abs(s.T.Dot(s.U)) > 1e-6 || math.Abs(s.U.Dot(s.W)) > 1e-6 {
				t.Fatalf("station %d: its frame has drifted: %+v", i, s)
			}
		}
	})
}

// fzSketch reads a drawing from the fuzzer's bytes: a path is a header byte
// (closed, stroke, join, cap, align) and a count, then five bytes a point:
// its position (on a fine grid, so points meet, repeat and line up), its
// kind and a handle. Up to 5 paths of 24 points, 48 points in all: more
// outlines crossing each other dozens of times take seconds to join (8
// paths of 69 points, six of them thick lines, take about 3 s), and the
// fuzzer takes that for a hang.
func fzSketch(b []byte) *Sketch {
	sk := &Sketch{}
	total := 0
	for len(b) >= 2 && len(sk.Paths) < 5 && total < 48 {
		h, n := b[0], 1+int(b[1])%24
		b = b[2:]
		p := SketchPath{Closed: h&1 != 0, Join: int(h>>1) % 3, Cap: int(h>>3) % 3, Align: int(h>>5) % 3}
		if h&0x80 != 0 {
			p.Stroke = 0.05 + float64(h&0x1e)/4
		}
		for k := 0; k < n && len(b) >= 5 && total < 48; k++ {
			total++
			q := SketchPoint{P: [2]float64{(float64(b[0]) - 128) / 4, (float64(b[1]) - 100) / 4}, Kind: int(b[2]) % 3}
			if b[2] >= 96 {
				// a handle, mirrored for a smooth point as the editor makes it
				q.Out = [2]float64{(float64(b[3]) - 128) / 8, (float64(b[4]) - 128) / 8}
				q.In = [2]float64{-q.Out[0], -q.Out[1]}
				if b[2] >= 192 {
					q.In = [2]float64{q.Out[1], -q.Out[0]}
				}
			}
			p.Points = append(p.Points, q)
			b = b[5:]
		}
		if len(p.Points) > 0 {
			sk.Paths = append(sk.Paths, p)
		}
	}
	return sk
}

// FuzzSketch builds drawings made of the fuzzer's bytes as each sketch kind,
// with fuzzed settings held to their sliders as Build holds them. A drawing
// with something to build gives a finite, closed solid in time; one without
// gives the stand-in.
func FuzzSketch(f *testing.F) {
	sq := []byte{1, 4, 128, 100, 0, 0, 0, 168, 100, 0, 0, 0, 168, 140, 0, 0, 0, 128, 140, 0, 0, 0}
	f.Add(sq, uint8(0), 12.0, 100.0, 100.0, 0.0, 0.0)
	f.Add(sq, uint8(1), 3.0, 360.0, 0.0, 30.0, 0.0)
	f.Add(sq, uint8(2), 5.0, 3.0, 6.0, 48.0, 0.0)
	// a curve, a line with round ends crossing it, a hole inside the square
	curvy := append(append([]byte{1, 3, 100, 100, 1, 0, 0, 120, 140, 200, 160, 140, 90, 120, 2, 0, 0},
		0x81|0x0c|0x02, 3, 90, 90, 0, 0, 0, 130, 150, 100, 150, 100, 150, 90, 0, 0, 0), sq...)
	curvy = append(curvy, 1, 3, 140, 110, 0, 0, 0, 150, 110, 0, 0, 0, 145, 120, 0, 0, 0)
	f.Add(curvy, uint8(0), 1.0, 0.0, 200.0, -360.0, 360.0)
	f.Add(curvy, uint8(1), 1.0, 1.0, 128.0, 359.0, 1.0)
	f.Add(curvy, uint8(2), 64.0, 50.0, 0.1, 128.0, 360.0)
	// points on top of each other, and two curves between two points
	f.Add([]byte{1, 3, 120, 120, 0, 0, 0, 120, 120, 0, 0, 0, 120, 120, 0, 0, 0}, uint8(0), 12.0, 100.0, 100.0, 0.0, 0.0)
	f.Add([]byte{1, 2, 100, 100, 150, 128, 200, 120, 100, 150, 128, 50}, uint8(1), 1.0, 48.0, 0.0, 360.0, 1.0)
	f.Fuzz(func(t *testing.T, data []byte, which uint8, a, b, c, d, e float64) {
		fzCheckSketch(t, fzSketch(data), which, a, b, c, d, e)
	})
}

// FuzzSketchHoles is FuzzSketch with some of the paths holes: bit i of
// holes makes path i one. (A target of its own, so FuzzSketch's saved
// inputs keep meaning what they did.)
func FuzzSketchHoles(f *testing.F) {
	sq := []byte{1, 4, 128, 100, 0, 0, 0, 168, 100, 0, 0, 0, 168, 140, 0, 0, 0, 128, 140, 0, 0, 0}
	inner := []byte{1, 4, 138, 110, 0, 0, 0, 158, 110, 0, 0, 0, 158, 130, 0, 0, 0, 138, 130, 0, 0, 0}
	slot := []byte{0x80 | 0x08, 2, 120, 120, 0, 0, 0, 176, 120, 0, 0, 0}
	f.Add(append(append([]byte{}, sq...), inner...), uint8(2), uint8(0), 12.0, 100.0, 100.0, 0.0, 0.0)
	f.Add(append(append([]byte{}, sq...), inner...), uint8(1), uint8(1), 3.0, 360.0, 0.0, 30.0, 0.0)
	f.Add(append(append([]byte{}, sq...), slot...), uint8(2), uint8(2), 5.0, 3.0, 6.0, 48.0, 0.0)
	f.Add(append(append(append([]byte{}, sq...), inner...), slot...), uint8(6), uint8(0), 1.0, 0.0, 200.0, -360.0, 360.0)
	f.Fuzz(func(t *testing.T, data []byte, holes, which uint8, a, b, c, d, e float64) {
		sk := fzSketch(data)
		for i := range sk.Paths {
			sk.Paths[i].Hole = holes>>i&1 != 0
		}
		fzCheckSketch(t, sk, which, a, b, c, d, e)
	})
}

// fzCheckSketch builds sk as the kind which picks, with settings a to e:
// a finite, closed solid in time, or the stand-in when nothing builds.
func fzCheckSketch(t *testing.T, sk *Sketch, which uint8, a, b, c, d, e float64) {
	kind := []string{"sketch", "revolve", "coil"}[which%3]
	k, _ := Lookup(kind)
	if CheckSketch(sk) != nil {
		t.Fatalf("fzSketch made a sketch CheckSketch refuses: %v", CheckSketch(sk))
	}
	v := Values{Num: map[string]float64{}, Sketch: sk}
	for i, p := range k.Params {
		v.Num[p.Key] = []float64{a, b, c, d, e}[i%5]
	}
	if kind == "sketch" {
		v.Num["twistmode"] = float64(which / 3 % 2)
	}
	var m *geom.Mesh
	fzWithin(t, 15*time.Second, "Build "+kind, func() { m = Build(kind, geom.V(20, 20, 20), v) })
	if m == nil || len(m.Tris) == 0 {
		t.Fatalf("Build %s: no mesh", kind)
	}
	fzMeshFinite(t, "Build "+kind, m)
	if !Buildable(kind, sk) || fzRelaxed("edges") {
		return
	}
	if open, nm := m.EdgeCheck(1e-7); open != 0 || nm != 0 {
		t.Fatalf("%s: %d open, %d non-manifold edges, settings %v, drawing %+v", kind, open, nm, k.clamp(v).Num, sk)
	}
	if len(m.Tris) > 2*sketchTriBudget {
		t.Fatalf("%s: %d triangles, budget %d", kind, len(m.Tris), sketchTriBudget)
	}
	fzWithin(t, 15*time.Second, "SketchBounds "+kind, func() { SketchBounds(kind, v) })
}

func FuzzBuild(f *testing.F) {
	n := len(All())
	for i := 0; i < n; i++ {
		f.Add(uint16(i), 20.0, 20.0, 20.0, 1.0, 5.0, 10.0, 0.5, uint32(0xffffffff), "TEXT", "Multilanguage")
		f.Add(uint16(i), 1e-9, 20.0, 1e6, -3.0, 1e9, 0.0, -0.5, uint32(0x01234567), "Ag", "")
	}
	f.Add(uint16(0), math.NaN(), math.Inf(1), math.Inf(-1), 1.0, 1.0, 1.0, 1.0, uint32(0), "", "nonsense")
	f.Add(uint16(0), -20.0, -20.0, 0.0, 1.0, 1.0, 1.0, 1.0, uint32(0x55555555), "\x00\xff", "Multilanguage")
	f.Fuzz(func(t *testing.T, ki uint16, sx, sy, sz, p0, p1, p2, p3 float64, sel uint32, text, font string) {
		ks := All()
		k := ks[int(ki)%len(ks)]
		if len(text) > 64 {
			text = text[:64]
		}
		size := geom.V(sx, sy, sz)
		v := Values{Num: map[string]float64{}, Text: text, Font: font}
		pv := []float64{p0, p1, p2, p3}
		for i, p := range k.Params {
			if p.Kind != Slider && p.Kind != Choice {
				continue
			}
			s := uint8(sel >> (4 * (i % 8)))
			if s&8 != 0 {
				continue // left at its default
			}
			v.Num[p.Key] = fzParam(pv[i%len(pv)], s)
		}
		var m *geom.Mesh
		// the limit catches a build that never ends; a large hex bolt
		// legitimately takes about 4 s, more on a loaded machine
		fzWithin(t, 15*time.Second, "Build "+k.ID, func() { m = Build(k.ID, size, v) })
		if m == nil {
			t.Fatalf("Build %s: nil mesh", k.ID)
		}
		// a sane size (as the editor allows) gives a finite mesh
		sane := true
		for _, c := range []float64{sx, sy, sz} {
			if !(math.Abs(c) <= 1e5) {
				sane = false
			}
		}
		if sane {
			fzMeshFinite(t, "Build "+k.ID, m)
		}
		if k.natural != nil {
			fzWithin(t, 5*time.Second, "NaturalSize "+k.ID, func() { k.NaturalSize(k.clamp(v)) })
		}
	})
}
