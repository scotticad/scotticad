package shapes

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

func watertight(t *testing.T, name string, m *geom.Mesh) {
	t.Helper()
	if len(m.Tris) == 0 {
		t.Errorf("%s: empty", name)
		return
	}
	if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
		t.Errorf("%s: %d open, %d non-manifold edges", name, o, n)
	}
}

// Path data that must neither hang the parser nor lose outlines.
func TestSVGPathEdgeCases(t *testing.T) {
	done := make(chan [][]Pt, 2)
	go func() { done <- parsePath("M0 0 L10 0 L0 10 Z 5 5") }()
	go func() { done <- parsePath("M0 0 L10 0 L0 10 z.5") }()
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("parsePath hung on numbers after Z")
		}
	}
	// a command right after Z starts the next outline at the closed one's
	// start
	if cs := parsePath("M0 0 L10 0 L10 10 Z L0 10 L-10 10 Z"); len(cs) != 2 {
		t.Fatalf("%d outlines, want 2", len(cs))
	}
	// an exponent ends the number: 1e5 .5 2
	if ns := numbers("1e5.5 2"); len(ns) != 3 || ns[0] != 1e5 || ns[1] != 0.5 || ns[2] != 2 {
		t.Fatalf("numbers %v", ns)
	}
}

// A shape inside another is a hole (across the whole drawing, so outline
// drawings make rings); shapes that cross or touch are joined. The result
// is watertight however they meet.
func TestSVGFill(t *testing.T) {
	head := `<svg width="100mm" height="100mm" viewBox="0 0 100 100">`
	for _, c := range []struct {
		name, body string
		area       float64
	}{
		{"two rects", `<rect x="0" y="0" width="10" height="10"/><rect x="5" y="5" width="10" height="10"/>`, 175},
		{"plus", `<rect x="0" y="15" width="40" height="10"/><rect x="15" y="0" width="10" height="40"/>`, 700},
		{"touching", `<rect x="0" y="0" width="10" height="10"/><rect x="10" y="0" width="10" height="10"/>`, 200},
		{"inside another", `<rect x="0" y="0" width="40" height="40"/><rect x="10" y="10" width="10" height="10"/>`, 1500},
		{"one path", `<path d="M0 0h10v10h-10z M5 5h10v10h-10z"/>`, 175},
		{"nested hole", `<path d="M0 0h20v20h-20z M5 5v10h10v-10z"/>`, 300},
		{"hole touching its outline", `<rect x="0" y="0" width="20" height="20"/><rect x="0" y="5" width="10" height="10"/>`, 300},
		{"bowtie", `<path d="M0 0 L10 10 L10 0 L0 10 Z"/>`, 50},
		{"outline ring", `<g fill="none" stroke="black"><rect x="0" y="0" width="20" height="20"/><rect x="5" y="5" width="10" height="10"/></g>`, 300},
	} {
		m, err := SVGMesh(strings.NewReader(head+c.body+`</svg>`), 2, 1)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		watertight(t, c.name, m)
		if v := m.Volume(); math.Abs(v-2*c.area) > 1e-6 {
			t.Errorf("%s: volume %v, want %v", c.name, v, 2*c.area)
		}
	}
}

// An outline that crosses itself hundreds of times (a zigzag closed by one
// long edge) imports in reasonable time. (Its pieces touch at single
// points, which no closed mesh can hold, so only the time is checked.)
func TestSVGZigzagIsQuick(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<svg width="200mm" height="200mm" viewBox="0 0 200 200"><path d="M0 50`)
	for i := 0; i < 600; i++ {
		y := 0
		if i%2 == 1 {
			y = 100
		}
		fmt.Fprintf(&sb, " L%d %d", i+1, y)
	}
	sb.WriteString(` Z"/></svg>`)
	start := time.Now()
	m, err := SVGMesh(strings.NewReader(sb.String()), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
	if len(m.Tris) == 0 {
		t.Fatal("empty")
	}
}

// A drawing of many overlapping shapes imports in reasonable time.
func TestSVGManyOverlapsIsQuick(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<svg width="200mm" height="200mm" viewBox="0 0 200 200">`)
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			fmt.Fprintf(&sb, `<circle cx="%d" cy="%d" r="5.2"/>`, 10+10*i, 10+10*j)
		}
	}
	sb.WriteString(`</svg>`)
	start := time.Now()
	m, err := SVGMesh(strings.NewReader(sb.String()), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 15*time.Second {
		t.Fatalf("took %v", d)
	}
	watertight(t, "circles", m)
}

// Hidden groups are left out with everything in them; the drawing keeps
// its proportions in a viewport of another shape.
func TestSVGDisplayNoneAndAspect(t *testing.T) {
	cs, err := SVGContours(strings.NewReader(`<svg width="100mm" height="100mm" viewBox="0 0 100 100"><g style="display: none"><rect width="5" height="5"/></g><rect style="display:none" width="5" height="5"/><rect x="50" width="5" height="5"/></svg>`))
	if err != nil || len(cs) != 1 {
		t.Fatalf("%d outlines (%v), want 1", len(cs), err)
	}
	cs, _ = SVGContours(strings.NewReader(`<svg width="100mm" height="50mm" viewBox="0 0 100 100"><rect width="100" height="100"/></svg>`))
	b := geom.EmptyBox()
	for _, p := range cs[0] {
		b = b.Extend(geom.V(p.X, p.Y, 0))
	}
	if s := b.Size(); math.Abs(s.X-50) > 1e-9 || math.Abs(s.Y-50) > 1e-9 {
		t.Fatalf("size %v, want 50 × 50", s)
	}
}

// Slider values that are each in range but extreme together.
func TestGearAndStarExtremes(t *testing.T) {
	for _, slop := range []float64{0.7, 0.8, 1} {
		k, _ := Lookup("gear")
		v := Values{Num: map[string]float64{"pitch": 0.5, "slop": slop, "teeth": 6, "bore": 0}}
		n, _ := k.NaturalSize(v)
		watertight(t, "gear", Build("gear", n, v))
	}
	for _, v := range []map[string]float64{{"points": 2e6}, {"inner": 0}, {"inner": math.NaN()}, {"inner": -1}} {
		watertight(t, "star", Build("star", geom.V(20, 20, 5), Values{Num: v}))
	}
}

// Cached builds of nearly equal sizes are not mixed up.
func TestBuildCacheIsExact(t *testing.T) {
	a := Build("box", geom.V(100.0001, 20, 20), Values{}).Bounds().Size().X
	b := Build("box", geom.V(100.0004, 20, 20), Values{}).Bounds().Size().X
	if a == b {
		t.Fatalf("both %v", a)
	}
}

// A letter missing from the font (U+E000, private use) keeps its place.
func TestMissingGlyphKeepsItsSpace(t *testing.T) {
	w := func(s string) float64 { return TextMesh(s, "Multilanguage", 5).Bounds().Size().X }
	if a, b := w("A\uE000V"), w("AV"); a <= b {
		t.Fatalf("with a missing glyph %v, without %v", a, b)
	}
}

// Two curves sampled through the same point cross at that corner, not
// inside an edge: the outline is split there like any crossing, and the
// shape comes out watertight (found by FuzzSVGMesh).
func TestSVGCrossingAtACorner(t *testing.T) {
	for _, d := range []string{"C0 0 1 0 0 1C0 0 1 0 0 0", "C0 0 100 0 0 100C0 0 100 0 0 0"} {
		m, err := SVGMesh(strings.NewReader(`<svg><path d="`+d+`"></svg>`), 2, 1)
		if err != nil {
			t.Fatal(err)
		}
		if open, nm := m.EdgeCheck(1e-7); open != 0 || nm != 0 {
			t.Fatalf("%s: %d open and %d non-manifold edges", d, open, nm)
		}
	}
}

// An outline whose closing line runs back over the start of a curve (a
// spike of no width) is triangulated without that sliver; the walls follow
// the caps, so the shape stays closed (found by FuzzSVGMesh).
// Two thin triangles crossing (joined, then sliced) leave cap triangles
// too small to keep; their edges get walls only if they are kept.
func TestSVGZeroWidthSpikeStaysClosed(t *testing.T) {
	for _, src := range []string{
		`<svg><path d="S0 0 00 70 0 0 10 10T0 90"></svg>`,
		`<svg><path d="M020 0 0000001000 70 000 770"/><path d="M1020 10 .0 1 0 21"/></svg>`,
	} {
		m, err := SVGMesh(strings.NewReader(src), 2, 1)
		if err != nil {
			t.Fatal(err)
		}
		if open, nm := m.EdgeCheck(1e-7); open != 0 || nm != 0 {
			t.Fatalf("%s: %d open and %d non-manifold edges", src, open, nm)
		}
	}
}

// Coordinates spanning 1e300, as an SVG arc with extreme numbers can give,
// must not overflow the triangulator's grid (found by FuzzSVGMesh).
func TestTriangulateHugeSpan(t *testing.T) {
	r := Region{Outer: []Pt{{1e-300, -1e-300}, {1, -1e300}, {0, -1}}}
	Triangulate(r)
	SVGMesh(strings.NewReader(`<path d="M0 A 1e-300 1e-300 0 1 1 1e300 1e300 Z"/>`), 2, 1)
}
