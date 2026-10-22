package render

import (
	"math"
	"testing"
	"time"
)

// A segment is clipped to the rectangle; one wholly outside, parallel to
// an edge outside it, or not finite gives nothing.
func TestClipSegment(t *testing.T) {
	for _, c := range []struct {
		x0, y0, x1, y1 float64
		s0, s1         float64
		ok             bool
	}{
		{10, 10, 90, 90, 0, 1, true},                // inside
		{-100, 50, 200, 50, 1.0 / 3, 2.0 / 3, true}, // across, both ends out
		{50, 50, 50, 1e12, 0, 50 / (1e12 - 50), true},
		{-10, -10, -5, -5, 0, 0, false}, // outside
		{-10, 50, -10, 60, 0, 0, false}, // parallel to the left edge, outside it
		{0, 0, math.NaN(), 5, 0, 0, false},
	} {
		s0, s1, ok := clipSegment(c.x0, c.y0, c.x1, c.y1, 0, 0, 100, 100)
		if ok != c.ok || (ok && (math.Abs(s0-c.s0) > 1e-12 || math.Abs(s1-c.s1) > 1e-12)) {
			t.Errorf("clip (%v,%v)-(%v,%v) = %v, %v, %v; want %v, %v, %v", c.x0, c.y0, c.x1, c.y1, s0, s1, ok, c.s0, c.s1, c.ok)
		}
	}
}

// A dashed line to a point projected absurdly far away (just in front of
// the camera) finishes at once, drawing only around the screen; one on
// screen gets its dashes.
func TestDashedFarLineFinishes(t *testing.T) {
	b := &Batch{W: 800, H: 600, DPI: 1}
	done := make(chan int)
	go func() {
		for _, far := range []float64{1e6, 1e12, 1e17, 1e18, 3e18, 1e300} {
			b.Dashed(100, 100, far, far*0.5, 1, 5, Color{0, 0, 0, 1})
			// both ends far away, crossing the screen: the dashing starts
			// 1e17 px along, where a float step of 10 px changes nothing
			b.Dashed(-far, 300, far, 300, 1, 5, Color{0, 0, 0, 1})
		}
		done <- len(b.data)
	}()
	select {
	case n := <-done:
		if n == 0 {
			t.Fatal("nothing drawn")
		}
		// a few hundred dashes at most, not millions
		if n > 12*600*8*6*4 {
			t.Fatalf("%d floats of vertex data for twelve lines", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a far dashed line never finished")
	}
	b.data = b.data[:0]
	b.Dashed(0, 300, 800, 300, 1, 5, Color{0, 0, 0, 1}) // 80 dashes across
	if len(b.data) == 0 {
		t.Fatal("an on-screen dashed line drew nothing")
	}
}
