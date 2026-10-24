package shapes

import (
	"math"
	"strings"
	"testing"
)

func TestSVG(t *testing.T) {
	src := `<svg xmlns="http://www.w3.org/2000/svg" width="20mm" height="10mm" viewBox="0 0 200 100">
	<rect x="0" y="0" width="100" height="100"/>
	<g transform="translate(100,0)"><path d="M10 10 h80 v80 h-80 z M30 30 v40 h40 v-40 z"/></g>
	<circle cx="50" cy="50" r="10" fill="white"/>
	<path d="M 20 80 A 5 5 0 0 1 30 80 L 30 85 Z"/>
	<defs><rect width="999" height="999"/></defs>
	</svg>`
	m, err := SVGMesh(strings.NewReader(src), 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Bounds().Size()
	if math.Abs(s.X-19) > 1e-6 || math.Abs(s.Y-10) > 1e-6 || math.Abs(s.Z-3) > 1e-9 {
		t.Fatalf("bounds %v", s)
	}
	if open, _ := m.EdgeCheck(1e-6); open != 0 {
		t.Fatalf("open edges %d", open)
	}
	// area: 10x10 square minus r=1 circle minus small arc piece, plus 8x8 ring minus 4x4
	if v := m.Volume(); v < 3*(100-math.Pi-1+48)-3 || v > 3*(100-math.Pi+48)+1 {
		t.Fatalf("volume %v", v)
	}
	ns := numbers("1-2.5.5e1,3")
	if len(ns) != 4 || ns[1] != -2.5 || ns[2] != 5 {
		t.Fatalf("numbers %v", ns)
	}
}
