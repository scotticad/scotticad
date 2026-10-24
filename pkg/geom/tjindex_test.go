package geom

import (
	"math"
	"math/rand"
	"testing"
)

// A mesh of corners drawn from a few points, so that most repeat, with a
// -0 standing in for some zeros.
func repeatingMesh(r *rand.Rand, n int) *Mesh {
	vals := []float64{0, 1, 2.5, -3}
	pt := func() Vec3 {
		var c [3]float64
		for i := range c {
			c[i] = vals[r.Intn(len(vals))]
			if c[i] == 0 && r.Intn(2) == 0 {
				c[i] = math.Copysign(0, -1)
			}
		}
		return V(c[0], c[1], c[2])
	}
	m := &Mesh{}
	for i := 0; i < n; i++ {
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{pt(), pt(), pt()}})
	}
	return m
}

// The numbering is the one a map[Vec3]int gives, filled triangle by
// triangle: -0 and +0 one position, as map keys are.
func TestVertexIndexNumbersAsAMapDoes(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, n := range []int{0, 1, 7, 300, 5000} {
		m := repeatingMesh(r, n)
		vi := indexMesh(m)
		want := map[Vec3]int32{}
		for i, tr := range m.Tris {
			for k, p := range tr.P {
				id, ok := want[p]
				if !ok {
					id = int32(len(want))
					want[p] = id
					if vi.pos[id] != p || math.Signbit(vi.pos[id].X) != math.Signbit(p.X) {
						t.Fatalf("vertex %d is %v, first seen as %v", id, vi.pos[id], p)
					}
				}
				if vi.tri[i][k] != id {
					t.Fatalf("n=%d: triangle %d corner %d numbered %d, want %d", n, i, k, vi.tri[i][k], id)
				}
			}
		}
		if len(vi.pos) != len(want) {
			t.Fatalf("n=%d: %d positions, want %d", n, len(vi.pos), len(want))
		}
	}
	// a NaN equals nothing, itself included
	nan := V(math.NaN(), 0, 0)
	vi := indexMesh(&Mesh{Tris: []Tri{{P: [3]Vec3{nan, nan, V(1, 0, 0)}}}})
	if vi.tri[0][0] == vi.tri[0][1] || len(vi.pos) != 3 {
		t.Fatalf("NaN corners share a number: %v", vi.tri[0])
	}
}

// Edge counts and open vertices as counting with maps gives them.
func TestEdgeTableCountsAsAMapDoes(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for _, n := range []int{1, 50, 3000} {
		m := repeatingMesh(r, n)
		vi := indexMesh(m)
		e := edgesOf(vi.tri)
		want := map[[2]int32]int32{}
		for _, tr := range vi.tri {
			for k := 0; k < 3; k++ {
				if a, b := tr[k], tr[(k+1)%3]; a != b {
					want[[2]int32{a, b}]++
				}
			}
		}
		seen := 0
		e.each(func(a, b, c int32) {
			seen++
			if want[[2]int32{a, b}] != c {
				t.Fatalf("edge %d-%d counted %d, want %d", a, b, c, want[[2]int32{a, b}])
			}
		})
		if seen != len(want) {
			t.Fatalf("%d edges, want %d", seen, len(want))
		}
		for a := int32(0); a < int32(len(vi.pos)); a++ {
			for b := int32(0); b < int32(len(vi.pos)); b++ {
				if e.get(a, b) != want[[2]int32{a, b}] {
					t.Fatalf("get(%d, %d) = %d, want %d", a, b, e.get(a, b), want[[2]int32{a, b}])
				}
			}
		}
		flag, count := openVertices(vi)
		wantOpen := map[int32]bool{}
		for k, c := range want {
			if r := want[[2]int32{k[1], k[0]}]; r != c || c > 1 {
				wantOpen[k[0]], wantOpen[k[1]] = true, true
			}
		}
		if count != len(wantOpen) {
			t.Fatalf("%d open vertices, want %d", count, len(wantOpen))
		}
		for v, f := range flag {
			if f != wantOpen[int32(v)] {
				t.Fatalf("vertex %d open %v, want %v", v, f, !f)
			}
		}
	}
}

// The weld grid's cells as a map would hold them, the filter in front of
// the table never hiding a cell that is set.
func TestCellTableHoldsAsAMapDoes(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	c := newCellTable(2000)
	want := map[[3]int64]int{}
	for i := 0; i < 2000; i++ {
		k := [3]int64{r.Int63n(40) - 20, r.Int63n(40) - 20, r.Int63n(40) - 20}
		if _, ok := want[k]; !ok {
			want[k] = i
			c.set(k, i)
		}
	}
	for x := int64(-21); x <= 21; x++ {
		for y := int64(-21); y <= 21; y++ {
			for z := int64(-21); z <= 21; z++ {
				k := [3]int64{x, y, z}
				v, ok := c.get(k)
				w, wok := want[k]
				if ok != wok || (ok && v != w) {
					t.Fatalf("cell %v: %d %v, want %d %v", k, v, ok, w, wok)
				}
			}
		}
	}
}
