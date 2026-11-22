package shapes

import "math"

// ringIndex files a closed ring's edges in a grid, so a question about one
// place looks only at the edges near it. A long outline (an imported
// drawing, a curve at high quality) asks these questions once a point, and
// looking at every edge each time would make the work grow with the square
// of its length.
//
// Each question gives exactly the answer the plain loop over every edge
// does: the same test is applied to each edge, and only edges that test
// would turn away are passed over.
type ringIndex struct {
	c      []Pt
	b      bounds2
	nx, ny int
	cw, ch float64   // a cell's size
	cells  [][]int32 // edge i runs from c[i] to c[i+1]
	rows   [][]int32 // the edges crossing each row of cells, for contains
	stamp  []int32   // the last query each edge was offered to
	query  int32
}

// ringIndexMin is the fewest edges worth an index (a variable so a test
// can compare the answers with and without one).
var ringIndexMin = 48

func newRingIndex(c []Pt) *ringIndex {
	n := len(c)
	x := &ringIndex{c: c, b: boundsOf([][]Pt{c}), stamp: make([]int32, n)}
	side := int(math.Ceil(math.Sqrt(float64(n))))
	x.nx, x.ny = side, side
	x.cw, x.ch = (x.b.x1-x.b.x0)/float64(side), (x.b.y1-x.b.y0)/float64(side)
	if !(x.cw > 0) {
		x.nx, x.cw = 1, 1
	}
	if !(x.ch > 0) {
		x.ny, x.ch = 1, 1
	}
	x.cells = make([][]int32, x.nx*x.ny)
	x.rows = make([][]int32, x.ny)
	for i := range c {
		p, q := c[i], c[(i+1)%n]
		i0, i1 := x.col(math.Min(p.X, q.X)), x.col(math.Max(p.X, q.X))
		j0, j1 := x.row(math.Min(p.Y, q.Y)), x.row(math.Max(p.Y, q.Y))
		for j := j0; j <= j1; j++ {
			x.rows[j] = append(x.rows[j], int32(i))
			for k := i0; k <= i1; k++ {
				x.cells[j*x.nx+k] = append(x.cells[j*x.nx+k], int32(i))
			}
		}
	}
	return x
}

func (x *ringIndex) col(v float64) int {
	return max(0, min(x.nx-1, int(math.Floor((v-x.b.x0)/x.cw))))
}

func (x *ringIndex) row(v float64) int {
	return max(0, min(x.ny-1, int(math.Floor((v-x.b.y0)/x.ch))))
}

// contains is PointInPolygon(p, x.c).
func (x *ringIndex) contains(p Pt) bool {
	if p.Y < x.b.y0 || p.Y > x.b.y1 {
		return false // no edge reaches its height
	}
	c := x.c
	n := len(c)
	in := false
	for _, e := range x.rows[x.row(p.Y)] {
		// PointInPolygon's edge j -> i is this edge e -> e+1, the other way
		a, b := c[(int(e)+1)%n], c[e]
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in
}

// winding is windingNumber(p, x.c): contains, counting each crossing by
// the edge's direction.
func (x *ringIndex) winding(p Pt) int {
	if p.Y < x.b.y0 || p.Y > x.b.y1 {
		return 0
	}
	c := x.c
	n := len(c)
	w := 0
	for _, e := range x.rows[x.row(p.Y)] {
		a, b := c[(int(e)+1)%n], c[e]
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)+a.X {
			if b.Y > p.Y { // the edge e -> e+1 runs down past p
				w--
			} else {
				w++
			}
		}
	}
	return w
}

// near calls f with each edge whose box reaches the box (x0, y0)…(x1, y1),
// once each, until f returns true; it reports whether one did.
func (x *ringIndex) near(x0, y0, x1, y1 float64, f func(i int) bool) bool {
	if x1 < x.b.x0 || x0 > x.b.x1 || y1 < x.b.y0 || y0 > x.b.y1 {
		return false
	}
	x.query++
	i0, i1, j0, j1 := x.col(x0), x.col(x1), x.row(y0), x.row(y1)
	for j := j0; j <= j1; j++ {
		for k := i0; k <= i1; k++ {
			for _, e := range x.cells[j*x.nx+k] {
				if x.stamp[e] == x.query {
					continue
				}
				x.stamp[e] = x.query
				if f(int(e)) {
					return true
				}
			}
		}
	}
	return false
}
