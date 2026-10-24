package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// GridSpan is the length of n squares of side size with gaps between.
func GridSpan(n int, size, gap float64) float64 {
	return float64(n)*size + float64(max(n-1, 0))*gap
}

// GridMesh builds cols × rows square prisms (side size, spaced by gap)
// and stretches the layout to fill s, centred on the origin.
func GridMesh(s geom.Vec3, cols, rows int, size, gap float64) *geom.Mesh {
	cols, rows = max(cols, 1), max(rows, 1)
	size, gap = math.Max(size, 0.01), math.Max(gap, 0)
	w, d := GridSpan(cols, size, gap), GridSpan(rows, size, gap)
	m := &geom.Mesh{}
	for i := 0; i < cols; i++ {
		for j := 0; j < rows; j++ {
			x0, y0 := float64(i)*(size+gap)-w/2, float64(j)*(size+gap)-d/2
			cell := extrude([]Pt{{0, 0}, {1, 0}, {1, 1}, {0, 1}}).
				Transform(geom.Scale(geom.V(size, size, 1))).
				Transform(geom.Translate(geom.V(x0, y0, 0)))
			m.Tris = append(m.Tris, cell.Tris...)
		}
	}
	return m.FitTo(s)
}

// DiamondGridMesh builds cols × rows diamonds (squares on their points,
// size corner to corner, spaced size+gap) and (cols-1) × (rows-1) more in
// the gaps between them, half a step across and along, and stretches the
// layout to fill s, centred on the origin.
func DiamondGridMesh(s geom.Vec3, cols, rows int, size, gap float64) *geom.Mesh {
	cols, rows = max(cols, 1), max(rows, 1)
	size, gap = math.Max(size, 0.01), math.Max(gap, 0)
	w, d := GridSpan(cols, size, gap), GridSpan(rows, size, gap)
	step, h := size+gap, size/2
	var outs [][]Pt
	add := func(x, y float64) {
		x, y = x-w/2, y-d/2
		outs = append(outs, []Pt{{x + h, y}, {x, y + h}, {x - h, y}, {x, y - h}})
	}
	for i := 0; i < cols; i++ {
		for j := 0; j < rows; j++ {
			add(h+float64(i)*step, h+float64(j)*step)
			if i+1 < cols && j+1 < rows {
				add(h+(float64(i)+0.5)*step, h+(float64(j)+0.5)*step)
			}
		}
	}
	return ExtrudeContours(outs, -0.5, 0.5).FitTo(s)
}

// polygonGrid lays out rows × cols regular polygons (sides, radius r to
// their corners, a corner toward +Y turned rot degrees), their centres a
// step of 2r+gap apart along each row and between rows, the first row and
// every other one set half a step along. It returns their outlines, the
// lattice's corner (the first polygon's centre, before the offset) at the
// origin.
func polygonGrid(sides, rows, cols int, r, gap, rot float64) [][]Pt {
	sides, rows, cols = max(sides, 3), max(rows, 1), max(cols, 1)
	r, gap = math.Max(r, 0.01), math.Max(gap, 0)
	step := 2*r + gap
	cell := make([]Pt, sides)
	for k := range cell {
		a := math.Pi/2 + rot*math.Pi/180 + 2*math.Pi*float64(k)/float64(sides)
		cell[k] = Pt{r * math.Cos(a), r * math.Sin(a)}
	}
	var outs [][]Pt
	for j := 0; j < rows; j++ {
		off := 0.0
		if j%2 == 0 {
			off = step / 2
		}
		for i := 0; i < cols; i++ {
			cx, cy := float64(i)*step+off, float64(j)*step
			o := make([]Pt, sides)
			for k, p := range cell {
				o[k] = Pt{p.X + cx, p.Y + cy}
			}
			outs = append(outs, o)
		}
	}
	return outs
}

func polygonGridValues(v Values) (sides, rows, cols int, r, gap, rot float64) {
	return int(v.Get("sides", 6)), int(v.Get("rows", 6)), int(v.Get("cols", 6)), v.Get("radius", 1.5), v.Get("gap", 0.2), v.Get("rotation", 0)
}

// PolygonGridBox is the box of a honeycomb grid with settings v, about its
// lattice's corner; Z from 0 to its height.
func PolygonGridBox(v Values) geom.Box {
	b := geom.EmptyBox()
	for _, o := range polygonGrid(polygonGridValues(v)) {
		for _, p := range o {
			b = b.Extend(geom.V(p.X, p.Y, 0))
		}
	}
	return b.Extend(geom.V(b.Min.X, b.Min.Y, v.Get("height", 10)))
}

// polygonGridSize is the size of a honeycomb grid with settings v.
func polygonGridSize(v Values) geom.Vec3 { return PolygonGridBox(v).Size() }

// tubeJoint is a ring of n blades between radii ri and ro, each spanning
// thick percent of the step between them, its outer end turned angle
// degrees from its inner end (straight sides), z from -0.5 to 0.5.
func tubeJoint(ri, ro float64, n int, angle, thick float64) [][]Pt {
	n = max(n, 1)
	ri, ro = math.Min(ri, ro), math.Max(ri, ro) // either way round
	ri = math.Max(ri, 0)
	ro = math.Max(ro, ri+0.5)
	step := 2 * math.Pi / float64(n)
	w := step * math.Max(math.Min(thick, 100), 1) / 100
	tw := angle * math.Pi / 180
	const arcSegs = 4
	var outs [][]Pt
	for k := 0; k < n; k++ {
		a := float64(k) * step
		var o []Pt
		for i := 0; i <= arcSegs; i++ { // the inner edge, along the blade's width
			t := a + w*float64(i)/arcSegs
			o = append(o, Pt{ri * math.Cos(t), ri * math.Sin(t)})
		}
		for i := arcSegs; i >= 0; i-- { // the outer edge, back
			t := a + tw + w*float64(i)/arcSegs
			o = append(o, Pt{ro * math.Cos(t), ro * math.Sin(t)})
		}
		outs = append(outs, o)
	}
	return outs
}

// flutedOutline is a regular n-gon (diameter d to its corners, a corner
// on +X) with a round groove of diameter e centred on each corner: the
// cross-section of a fluted rod, flat sides between the grooves.
func flutedOutline(n int, d, e float64) []Pt {
	n = max(n, 3)
	R := math.Max(d/2, 0.01)
	side := 2 * R * math.Sin(math.Pi/float64(n))
	rho := math.Max(math.Min(e/2, side*0.45), 0) // a groove less than half a side
	const segs = 6
	corner := func(k int) Pt {
		a := 2 * math.Pi * float64(k) / float64(n)
		return Pt{R * math.Cos(a), R * math.Sin(a)}
	}
	var out []Pt
	for k := 0; k < n; k++ {
		c, prev, next := corner(k), corner(k-1+n), corner(k+1)
		if rho == 0 {
			out = append(out, c)
			continue
		}
		// where the groove meets the sides, and the arc between them
		// through the inside of the polygon
		dir := func(p Pt) Pt {
			dx, dy := p.X-c.X, p.Y-c.Y
			l := math.Hypot(dx, dy)
			return Pt{dx / l, dy / l}
		}
		in, outd := dir(prev), dir(next)
		a0 := math.Atan2(in.Y, in.X)
		a1 := math.Atan2(outd.Y, outd.X)
		// sweep from a0 to a1 the way that passes the centre's direction
		toC := math.Atan2(-c.Y, -c.X)
		norm := func(x float64) float64 {
			for x < 0 {
				x += 2 * math.Pi
			}
			for x >= 2*math.Pi {
				x -= 2 * math.Pi
			}
			return x
		}
		sweep := norm(a1 - a0)
		if norm(toC-a0) > sweep {
			sweep -= 2 * math.Pi
		}
		for i := 0; i <= segs; i++ {
			a := a0 + sweep*float64(i)/segs
			out = append(out, Pt{c.X + rho*math.Cos(a), c.Y + rho*math.Sin(a)})
		}
	}
	return out
}

func flutedValues(v Values) (n int, d1, d2, e1, e2 float64) {
	return int(v.Get("lobes", 12)), v.Get("d1", 22.5), v.Get("d2", 19), v.Get("e1", 1.5), v.Get("e2", 1.5)
}

// FlutedRodBox is the box of a fluted rod with settings v, about its axis;
// Z from 0 to its height.
func FlutedRodBox(v Values) geom.Box {
	n, d1, d2, e1, e2 := flutedValues(v)
	b := geom.EmptyBox()
	for _, o := range [][]Pt{flutedOutline(n, d1, e1), flutedOutline(n, d2, e2)} {
		for _, p := range o {
			b = b.Extend(geom.V(p.X, p.Y, 0))
		}
	}
	return b.Extend(geom.V(b.Min.X, b.Min.Y, v.Get("height", 37)))
}

// diamondMesh is a plate w × d with a border, its inside cut by rows of
// diamond holes: even rows of cols diamonds, odd rows of cols-1 set half a
// step along, gap apart across a row. It returns the outlines, the plate
// and its holes, about the centre.
func diamondMesh(w, d, border float64, rows, cols int, gap float64) [][]Pt {
	rows, cols = max(rows, 1), max(cols, 1)
	border, gap = math.Max(border, 0), math.Max(gap, 0)
	outs := [][]Pt{{{-w / 2, -d / 2}, {w / 2, -d / 2}, {w / 2, d / 2}, {-w / 2, d / 2}}}
	iw, id := w-2*border, d-2*border
	if iw <= 0 || id <= 0 {
		return outs
	}
	px := (iw + gap) / float64(cols)
	dw := px - gap
	if dw <= 0 {
		return outs
	}
	// a diamond is as much taller than wide as two rows' share of the
	// depth is than a step across
	dh := math.Min(dw*(2*id/float64(rows))/px, id)
	py := 0.0
	if rows > 1 {
		py = (id - dh) / float64(rows-1)
	}
	for r := 0; r < rows; r++ {
		n, x0 := cols, -iw/2+dw/2
		if r%2 == 1 {
			n, x0 = cols-1, x0+px/2
		}
		y := -id/2 + dh/2 + float64(r)*py
		for i := 0; i < n; i++ {
			x := x0 + float64(i)*px
			// clockwise: a hole
			outs = append(outs, []Pt{{x, y + dh/2}, {x + dw/2, y}, {x, y - dh/2}, {x - dw/2, y}})
		}
	}
	return outs
}

func tubeJointValues(v Values) [][]Pt {
	return tubeJoint(v.Get("inner", 12), v.Get("outer", 33), int(v.Get("tongs", 20)), v.Get("angle", 30), v.Get("thick", 30))
}

// TubeJointBox is the box of a tube joint with settings v, about its
// centre; Z from 0 to its height.
func TubeJointBox(v Values) geom.Box {
	b := geom.EmptyBox()
	for _, o := range tubeJointValues(v) {
		for _, p := range o {
			b = b.Extend(geom.V(p.X, p.Y, 0))
		}
	}
	return b.Extend(geom.V(b.Min.X, b.Min.Y, v.Get("height", 10)))
}

func init() {
	register(&Kind{ID: "tubejoint", Label: "Tube Joint", Color: "#d62f2f", Size: TubeJointBox(Values{Num: map[string]float64{}}).Size(),
		Params: []Param{
			lslider("inner", "Inner Radius", 0, 100, 0.5, 12, false),
			lslider("outer", "Outer Radius", 1, 200, 0.5, 33, false),
			slider("tongs", "Blades", 1, 64, 1, 20, true),
			slider("angle", "Blade Angle", -90, 90, 1, 30, false),
			slider("thick", "Blade Thickness (%)", 1, 100, 1, 30, false),
			lslider("height", "Height", 0.1, 200, 0.1, 10, false),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			// each blade on its own: thick or steep blades overlap their
			// neighbours, and outlines that overlap would cancel
			m := &geom.Mesh{}
			for _, o := range tubeJointValues(v) {
				m.Tris = append(m.Tris, ExtrudeContours([][]Pt{o}, -0.5, 0.5).Tris...)
			}
			return m.FitTo(s)
		},
		natural: func(v Values) geom.Vec3 { return TubeJointBox(v).Size() }})
	register(&Kind{ID: "diamondmesh", Label: "Diamond Mesh", Color: "#e2142a", Size: geom.V(60, 60, 4),
		Params: []Param{
			lslider("border", "Border", 0, 50, 0.1, 1, false),
			slider("rows", "Rows", 1, 60, 1, 11, true),
			slider("cols", "Columns", 1, 60, 1, 11, true),
			lslider("gap", "Gap", 0, 20, 0.1, 2, false),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			// built at its size: the border and gap are in mm
			outs := diamondMesh(s.X, s.Y, v.Get("border", 1), int(v.Get("rows", 11)), int(v.Get("cols", 11)), v.Get("gap", 2))
			return ExtrudeContours(outs, -s.Z/2, s.Z/2)
		}})
	register(&Kind{ID: "flutedrod", Label: "Fluted Rod", Color: "#3c4a5c", Size: FlutedRodBox(Values{Num: map[string]float64{}}).Size(),
		Params: []Param{
			lslider("d1", "Bottom Diameter", 1, 200, 0.5, 22.5, false),
			lslider("d2", "Top Diameter", 1, 200, 0.5, 19, false),
			lslider("height", "Height", 0.1, 300, 0.5, 37, false),
			slider("lobes", "Lobes", 3, 64, 1, 12, true),
			lslider("e1", "Groove at Bottom", 0, 20, 0.1, 1.5, false),
			lslider("e2", "Groove at Top", 0, 20, 0.1, 1.5, false),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			n, d1, d2, e1, e2 := flutedValues(v)
			return loftMesh([][]Pt{flutedOutline(n, d1, e1), flutedOutline(n, d2, e2)}, []float64{-0.5, 0.5}).FitTo(s)
		},
		natural: func(v Values) geom.Vec3 { return FlutedRodBox(v).Size() }})
	register(&Kind{ID: "honeycomb", Label: "Honeycomb Grid", Color: "#b5651d", Size: polygonGridSize(Values{Num: map[string]float64{}}),
		Params: []Param{
			slider("sides", "Polygon Sides", 3, 12, 1, 6, true),
			lslider("radius", "Radius", 0.5, 20, 0.1, 1.5, false),
			slider("rows", "Rows", 1, 50, 1, 6, true),
			slider("cols", "Columns", 1, 50, 1, 6, true),
			lslider("gap", "Wall Thickness", 0, 10, 0.05, 0.2, false),
			lslider("height", "Height", 0.1, 200, 0.1, 10, false),
			slider("rotation", "Rotation", 0, 360, 1, 0, false),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			return ExtrudeContours(polygonGrid(polygonGridValues(v)), -0.5, 0.5).FitTo(s)
		},
		natural: polygonGridSize})
	register(&Kind{ID: "diamondgrid", Label: "Diamond Grid", Color: "#f08a24", Size: geom.V(18, 18, 10),
		Params: []Param{
			slider("cols", "Columns", 1, 50, 1, 4, true),
			slider("rows", "Rows", 1, 50, 1, 4, true),
			lslider("size", "Size", 0.5, 20, 0.1, 3, false),
			lslider("gap", "Gap", 0, 20, 0.1, 2, false),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			return DiamondGridMesh(s, int(v.Get("cols", 4)), int(v.Get("rows", 4)), v.Get("size", 3), v.Get("gap", 2))
		}})
	register(&Kind{ID: "grid", Label: "Square Grid", Color: "#f08a24", Size: geom.V(18, 18, 10),
		Params: []Param{
			slider("cols", "Columns", 1, 50, 1, 4, true),
			slider("rows", "Rows", 1, 50, 1, 4, true),
			lslider("size", "Size", 0.5, 20, 0.1, 3, false),
			lslider("gap", "Gap", 0, 20, 0.1, 2, false),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			return GridMesh(s, int(v.Get("cols", 4)), int(v.Get("rows", 4)), v.Get("size", 3), v.Get("gap", 2))
		}})
}
