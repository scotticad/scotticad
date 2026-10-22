package geom

import "math"

// insideTest answers whether points are inside a closed mesh, robustly: by
// signed ray crossings (winding along a ray) along each axis, which must
// all agree.
// A BSP tree of noisy geometry (faces a few hundredths of a micron off
// flat, as float32 imports are) can classify a whole region wrongly: a
// sliver cut off by a near-coincident plane stands alone in a subtree,
// and the space past it is taken as inside. Crossings don't care how the
// surface is cut up, and count overlapping shells right (inside twice is
// inside).
type insideTest struct {
	tris []Tri
	nrm  []Vec3
	ax   [3]axisGrid
}

// axisGrid bins triangles by their projection along axis k onto the
// other two axes (i, j).
type axisGrid struct {
	k, i, j    int
	lo0, lo1   float64
	cell       float64
	nx, ny     int
	bins       [][]int32
	degenerate bool
}

// nearTol is how close (mm, along a ray) to the surface a point must be
// for the test to give no answer: a fragment lying on the other operand's
// surface is the BSP's to decide (coplanar faces kept once, and so on).
const nearTol = 3e-5

func newInsideTest(m *Mesh) *insideTest {
	t := &insideTest{tris: m.Tris, nrm: make([]Vec3, len(m.Tris))}
	for i, tr := range m.Tris {
		t.nrm[i] = tr.P[1].Sub(tr.P[0]).Cross(tr.P[2].Sub(tr.P[0]))
	}
	b := m.Bounds()
	side := int(math.Ceil(math.Sqrt(float64(len(m.Tris)) / 2)))
	side = max(1, min(side, 512))
	for k := 0; k < 3; k++ {
		g := axisGrid{k: k, i: (k + 1) % 3, j: (k + 2) % 3}
		s0, s1 := b.Size().Axis(g.i), b.Size().Axis(g.j)
		g.cell = math.Max(s0, s1) / float64(side)
		if g.cell <= 0 || math.IsNaN(g.cell) || math.IsInf(g.cell, 0) {
			g.degenerate = true
			t.ax[k] = g
			continue
		}
		g.lo0, g.lo1 = b.Min.Axis(g.i), b.Min.Axis(g.j)
		g.nx = max(1, int(math.Ceil(s0/g.cell)))
		g.ny = max(1, int(math.Ceil(s1/g.cell)))
		g.bins = make([][]int32, g.nx*g.ny)
		for ti, tr := range m.Tris {
			if t.nrm[ti].Axis(k) == 0 {
				continue // edge-on along this axis: never crossed
			}
			mn0, mx0 := math.Inf(1), math.Inf(-1)
			mn1, mx1 := math.Inf(1), math.Inf(-1)
			for _, p := range tr.P {
				mn0, mx0 = math.Min(mn0, p.Axis(g.i)), math.Max(mx0, p.Axis(g.i))
				mn1, mx1 = math.Min(mn1, p.Axis(g.j)), math.Max(mx1, p.Axis(g.j))
			}
			x0, x1 := g.binX(mn0), g.binX(mx0)
			y0, y1 := g.binY(mn1), g.binY(mx1)
			for y := y0; y <= y1; y++ {
				for x := x0; x <= x1; x++ {
					g.bins[y*g.nx+x] = append(g.bins[y*g.nx+x], int32(ti))
				}
			}
		}
		t.ax[k] = g
	}
	return t
}

func (g *axisGrid) binX(v float64) int {
	return max(0, min(g.nx-1, int((v-g.lo0)/g.cell)))
}

func (g *axisGrid) binY(v float64) int {
	return max(0, min(g.ny-1, int((v-g.lo1)/g.cell)))
}

// cast counts signed crossings of the ray from p along +k: +1 where it
// leaves through a face (the face's normal along +k), -1 where it enters.
// near reports a crossing within nearTol of p.
func (t *insideTest) cast(g *axisGrid, p Vec3) (winding int, near bool) {
	if g.degenerate {
		return 0, true
	}
	p0, p1, pk := p.Axis(g.i), p.Axis(g.j), p.Axis(g.k)
	if p0 < g.lo0 || p1 < g.lo1 || p0 > g.lo0+float64(g.nx)*g.cell || p1 > g.lo1+float64(g.ny)*g.cell {
		return 0, false // outside the mesh's shadow: crosses nothing
	}
	for _, ti := range g.bins[g.binY(p1)*g.nx+g.binX(p0)] {
		tr := &t.tris[ti]
		a, b, c := tr.P[0], tr.P[1], tr.P[2]
		ax, ay := a.Axis(g.i)-p0, a.Axis(g.j)-p1
		bx, by := b.Axis(g.i)-p0, b.Axis(g.j)-p1
		cx, cy := c.Axis(g.i)-p0, c.Axis(g.j)-p1
		// edge functions: twice the area of p with each edge
		wa := bx*cy - by*cx // opposite a
		wb := cx*ay - cy*ax // opposite b
		wc := ax*by - ay*bx // opposite c
		area := wa + wb + wc
		if area == 0 {
			continue
		}
		s := 1.0
		if area < 0 {
			s, wa, wb, wc, area = -1, -wa, -wb, -wc, -area
		}
		// on an edge: counted for one of the two triangles sharing it
		// only (the edge's direction decides), so a ray through an edge
		// crosses once
		if !edgeIn(wa, s*(cx-bx), s*(cy-by)) || !edgeIn(wb, s*(ax-cx), s*(ay-cy)) || !edgeIn(wc, s*(bx-ax), s*(by-ay)) {
			continue
		}
		z := (wa*a.Axis(g.k) + wb*b.Axis(g.k) + wc*c.Axis(g.k)) / area
		if math.Abs(z-pk) <= nearTol {
			near = true
			continue
		}
		if z < pk {
			continue
		}
		if t.nrm[ti].Axis(g.k) > 0 {
			winding++
		} else {
			winding--
		}
	}
	return winding, near
}

// edgeIn is whether a point with edge function w lies on the inner side of
// an edge running (dx, dy); on the edge itself, for edges going one way
// only.
func edgeIn(w, dx, dy float64) bool {
	if w != 0 {
		return w > 0
	}
	return dy > 0 || (dy == 0 && dx < 0)
}

// inside is 1 (inside), 0 (outside) or -1 (no answer: p is on or close to
// the surface, or the rays disagree).
func (t *insideTest) inside(p Vec3) int {
	in := 0
	for k := range t.ax {
		w, near := t.cast(&t.ax[k], p)
		if near {
			return -1
		}
		if w > 0 {
			in++
		}
	}
	switch in {
	case 3:
		return 1
	case 0:
		return 0
	}
	return -1
}
