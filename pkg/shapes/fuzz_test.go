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
// plain go test. To look past a known finding while fuzzing, set
// SCOTTICAD_FUZZ_RELAX to a comma-separated list of checks to skip:
// "nan" (non-finite vertices), "edges" (open edges), "time" (slow inputs).

func fzRelaxed(check string) bool {
	for _, c := range strings.Split(os.Getenv("SCOTTICAD_FUZZ_RELAX"), ",") {
		if strings.TrimSpace(c) == check {
			return true
		}
	}
	return false
}

// fzWithin runs f and fails if it doesn't finish within d. A panic in f
// crashes the test binary with f's own stack, which the fuzzer records as
// a crasher like any other.
func fzWithin(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
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
			if p.Kind != Slider {
				continue
			}
			s := uint8(sel >> (4 * (i % 8)))
			if s&8 != 0 {
				continue // left at its default
			}
			v.Num[p.Key] = fzParam(pv[i%len(pv)], s)
		}
		var m *geom.Mesh
		fzWithin(t, 5*time.Second, "Build "+k.ID, func() { m = Build(k.ID, size, v) })
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
