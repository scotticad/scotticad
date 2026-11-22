package shapes

import (
	"math"
	"math/rand"
	"testing"
)

// wobblyRing is a closed ring of n points round (cx, cy), its radius
// wandering, with some points repeated on a grid so rings touch and share
// corners.
func wobblyRing(rnd *rand.Rand, n int, cx, cy, r float64) []Pt {
	c := make([]Pt, n)
	k, ph := float64(2+rnd.Intn(6)), rnd.Float64()*6
	for i := range c {
		a := 2 * math.Pi * float64(i) / float64(n)
		rr := r * (1 + 0.3*math.Sin(k*a+ph))
		p := Pt{cx + rr*math.Cos(a), cy + rr*math.Sin(a)}
		if rnd.Intn(4) == 0 {
			p = Pt{math.Round(p.X), math.Round(p.Y)}
		}
		c[i] = p
	}
	return c
}

// With an index or without, the questions asked of a ring have the same
// answers: inside or not, one ring within another, two rings meeting.
func TestRingIndexChangesNoAnswer(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	was := ringIndexMin
	defer func() { ringIndexMin = was }()
	meets, withins := 0, 0
	for trial := 0; trial < 300; trial++ {
		a := wobblyRing(rnd, 48+rnd.Intn(300), rnd.Float64()*20, rnd.Float64()*20, 5+rnd.Float64()*30)
		b := wobblyRing(rnd, 48+rnd.Intn(300), rnd.Float64()*20, rnd.Float64()*20, 5+rnd.Float64()*30)
		if trial%5 == 0 {
			// one a copy of the other, shrunk: within it, and in step with it
			b = make([]Pt, len(a))
			f := 0.5 + rnd.Float64()*0.6
			for i, p := range a {
				b[i] = Pt{10 + (p.X-10)*f, 10 + (p.Y-10)*f}
			}
		}
		x := newRingIndex(b)
		for k := 0; k < 200; k++ {
			p := Pt{-30 + rnd.Float64()*80, -30 + rnd.Float64()*80}
			if k%3 == 0 { // on a corner's height, where ray casting is touchy
				p.Y = b[rnd.Intn(len(b))].Y
			}
			if got, want := x.contains(p), PointInPolygon(p, b); got != want {
				t.Fatalf("trial %d: contains(%v) = %v, PointInPolygon says %v", trial, p, got, want)
			}
			if got, want := x.winding(p), windingNumber(p, b); got != want {
				t.Fatalf("trial %d: winding(%v) = %v, windingNumber says %v", trial, p, got, want)
			}
			if (windingNumber(p, b)%2 != 0) != PointInPolygon(p, b) {
				t.Fatalf("trial %d: winding %d at %v, but PointInPolygon says %v", trial, windingNumber(p, b), p, PointInPolygon(p, b))
			}
		}
		ringIndexMin = 1 << 30
		m0, w0, w1 := outlinesMeet(a, b), within(a, b), within(b, a)
		ringIndexMin = was
		m, wa, wb := outlinesMeet(a, b), within(a, b), within(b, a)
		if m != m0 || wa != w0 || wb != w1 {
			t.Fatalf("trial %d: meet %v, within %v %v; without an index %v, %v %v", trial, m, wa, wb, m0, w0, w1)
		}
		if m {
			meets++
		}
		if wa || wb {
			withins++
		}
	}
	// (the trials must be of every kind for the comparison to mean much)
	if meets < 30 || meets > 270 || withins < 20 {
		t.Errorf("%d of 300 pairs met and %d nested: the trials are lopsided", meets, withins)
	}
}
