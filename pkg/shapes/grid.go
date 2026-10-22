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

func init() {
	register(&Kind{ID: "grid", Label: "Square Grid", Color: "#f08a24", Size: geom.V(18, 18, 10),
		Params: []Param{
			slider("cols", "Columns", 1, 30, 1, 4, true),
			slider("rows", "Rows", 1, 30, 1, 4, true),
			lslider("size", "Size", 0.5, 20, 0.1, 3, false),
			lslider("gap", "Gap", 0, 20, 0.1, 2, false),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			return GridMesh(s, int(v.Get("cols", 4)), int(v.Get("rows", 4)), v.Get("size", 3), v.Get("gap", 2))
		}})
}
