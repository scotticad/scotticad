package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// PuffyMesh builds a rounded, slightly bulging box filling size: the
// superellipsoid |x|ⁿ + |y|ⁿ + |z|ⁿ = 1 (n = 2 is a sphere; larger n
// approaches a box). steps is the grid resolution on each of the six
// faces of the cube it is projected from.
func PuffyMesh(size geom.Vec3, n float64, steps int) *geom.Mesh {
	if steps < 1 {
		steps = 1
	}
	n = math.Max(n, 1.2)
	// cube point -> superellipsoid point along the same direction
	proj := func(p geom.Vec3) geom.Vec3 {
		s := math.Pow(math.Pow(math.Abs(p.X), n)+math.Pow(math.Abs(p.Y), n)+math.Pow(math.Abs(p.Z), n), -1/n)
		return geom.V(p.X*s*size.X/2, p.Y*s*size.Y/2, p.Z*s*size.Z/2)
	}
	m := &geom.Mesh{}
	// each face: an outward normal axis and two in-face axes (u × v = normal)
	faces := [][3]geom.Vec3{
		{geom.V(1, 0, 0), geom.V(0, 1, 0), geom.V(0, 0, 1)},
		{geom.V(-1, 0, 0), geom.V(0, 0, 1), geom.V(0, 1, 0)},
		{geom.V(0, 1, 0), geom.V(0, 0, 1), geom.V(1, 0, 0)},
		{geom.V(0, -1, 0), geom.V(1, 0, 0), geom.V(0, 0, 1)},
		{geom.V(0, 0, 1), geom.V(1, 0, 0), geom.V(0, 1, 0)},
		{geom.V(0, 0, -1), geom.V(0, 1, 0), geom.V(1, 0, 0)},
	}
	for _, f := range faces {
		pt := func(i, j int) geom.Vec3 {
			// tan spacing evens out the grid once projected
			a := math.Tan((float64(i)/float64(steps)*2 - 1) * math.Pi / 4)
			b := math.Tan((float64(j)/float64(steps)*2 - 1) * math.Pi / 4)
			return proj(f[0].Add(f[1].Mul(a)).Add(f[2].Mul(b)))
		}
		for i := 0; i < steps; i++ {
			for j := 0; j < steps; j++ {
				p00, p10, p11, p01 := pt(i, j), pt(i+1, j), pt(i+1, j+1), pt(i, j+1)
				m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{p00, p10, p11}}, geom.Tri{P: [3]geom.Vec3{p00, p11, p01}})
			}
		}
	}
	return m.Clean().Oriented()
}

func init() {
	register(&Kind{ID: "puffy", Label: "Puffy Box", Color: "#3fae2a", Size: geom.V(30, 30, 30),
		Params: []Param{
			slider("puff", "Squareness", 2, 10, 0.1, 4, false),
			slider("steps", "Steps", 2, 32, 1, 12, true),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			return PuffyMesh(s, v.Get("puff", 4), int(v.Get("steps", 12))).FitTo(s) // odd steps miss the faces' centres
		}})
}
