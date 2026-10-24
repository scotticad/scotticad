package geom

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

// gapNearMedian finds the plane sorting every coordinate finds, bit for
// bit: on rows of shared coordinates, on runs packed closer than the gap
// it wants (so its search runs past what it sorted first), with no gap
// wide enough anywhere, with -0 beside +0, with the median at either end.
func TestGapNearMedianAsSorting(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	g := 4 * csgEps
	gens := map[string]func(n int) []float64{
		"spread": func(n int) []float64 {
			v := make([]float64, n)
			for i := range v {
				v[i] = r.Float64()*200 - 100
			}
			return v
		},
		"rows": func(n int) []float64 {
			v := make([]float64, n)
			for i := range v {
				v[i] = float64(r.Intn(7)) * 2.5
			}
			return v
		},
		"packed": func(n int) []float64 { // steps under the gap, a few wide ones
			v := make([]float64, n)
			x := 0.0
			for i := range v {
				if r.Intn(n/3+1) == 0 {
					x += 2 * g
				} else {
					x += g * r.Float64() * 0.9
				}
				v[i] = x
			}
			r.Shuffle(n, func(i, j int) { v[i], v[j] = v[j], v[i] })
			return v
		},
		"no gap": func(n int) []float64 {
			v := make([]float64, n)
			for i := range v {
				v[i] = float64(i) * g / 2
			}
			r.Shuffle(n, func(i, j int) { v[i], v[j] = v[j], v[i] })
			return v
		},
		"one value": func(n int) []float64 {
			v := make([]float64, n)
			for i := range v {
				v[i] = 3
			}
			return v
		},
		"zeros": func(n int) []float64 {
			v := make([]float64, n)
			for i := range v {
				switch r.Intn(4) {
				case 0:
					v[i] = math.Copysign(0, -1)
				case 1:
					v[i] = 0
				default:
					v[i] = float64(r.Intn(5)-2) * g * 0.7
				}
			}
			return v
		},
		"lopsided": func(n int) []float64 { // most at the bottom: the median is its least
			v := make([]float64, n)
			for i := range v {
				if i < n*2/3 {
					v[i] = -5
				} else {
					v[i] = -5 + float64(i)*g*0.3
				}
			}
			r.Shuffle(n, func(i, j int) { v[i], v[j] = v[j], v[i] })
			return v
		},
	}
	for name, gen := range gens {
		for _, n := range []int{1, 2, 3, 8, 17, 64, 200, 1000, 20000} {
			for rep := 0; rep < 20; rep++ {
				v := gen(n)
				want, wok := gapNearMedianSorted(append([]float64(nil), v...))
				got, ok := gapNearMedian(v)
				if ok != wok || math.Float64bits(got) != math.Float64bits(want) {
					t.Fatalf("%s, n=%d: %v %v, sorting gives %v %v", name, n, got, ok, want, wok)
				}
			}
		}
	}
	if _, ok := gapNearMedian(nil); ok {
		t.Fatal("a plane among no coordinates")
	}
}

// selectKth puts the k-th value where sorting would, with nothing out of
// order either side of it.
func TestSelectKth(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	for _, n := range []int{1, 2, 5, 100, 5000} {
		for rep := 0; rep < 30; rep++ {
			a := make([]float64, n)
			for i := range a {
				a[i] = float64(r.Intn(n/2 + 1)) // duplicates
			}
			s := append([]float64(nil), a...)
			sort.Float64s(s)
			k := r.Intn(n)
			if got := selectKth(a, k); got != s[k] || a[k] != s[k] {
				t.Fatalf("n=%d k=%d: %v, want %v", n, k, got, s[k])
			}
			for i := range a {
				if i < k && a[i] > a[k] || i > k && a[i] < a[k] {
					t.Fatalf("n=%d k=%d: a[%d]=%v on the wrong side of %v", n, k, i, a[i], a[k])
				}
			}
		}
	}
	// sorted, reversed and all-equal input finish (no quadratic pivots)
	for _, mk := range []func(i int) float64{func(i int) float64 { return float64(i) }, func(i int) float64 { return float64(-i) }, func(int) float64 { return 1 }} {
		a := make([]float64, 100000)
		for i := range a {
			a[i] = mk(i)
		}
		s := append([]float64(nil), a...)
		sort.Float64s(s)
		if got := selectKth(a, 50000); got != s[50000] {
			t.Fatalf("got %v, want %v", got, s[50000])
		}
	}
}
