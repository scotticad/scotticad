package geom

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Color is an sRGB colour (as in "#rrggbb") with components in 0..1.
type Color struct{ R, G, B float32 }

// ParseColor parses "#rrggbb". Invalid input yields mid grey.
func ParseColor(s string) Color {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return Color{0.6, 0.6, 0.6}
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return Color{0.6, 0.6, 0.6}
	}
	return Color{float32(v>>16&255) / 255, float32(v>>8&255) / 255, float32(v&255) / 255}
}

// TryParseColor parses "#rrggbb" and reports whether it was valid.
func TryParseColor(s string) (Color, bool) {
	t := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(t) != 6 {
		return Color{}, false
	}
	if _, err := strconv.ParseUint(t, 16, 32); err != nil {
		return Color{}, false
	}
	return ParseColor(s), true
}

func (c Color) Hex() string {
	b := func(f float32) int { return int(math.Round(float64(f) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", b(c.R), b(c.G), b(c.B))
}

// Tri is a triangle with a colour. Vertices are counter-clockwise when
// seen from outside.
type Tri struct {
	P [3]Vec3
	C Color
}

func (t Tri) Normal() Vec3 { return t.P[1].Sub(t.P[0]).Cross(t.P[2].Sub(t.P[0])).Norm() }

func (t Tri) Area() float64 { return t.P[1].Sub(t.P[0]).Cross(t.P[2].Sub(t.P[0])).Len() / 2 }

// Mesh is a triangle soup. Meshes are treated as immutable once built so
// that they can be shared and cached.
type Mesh struct{ Tris []Tri }

// Transform returns a new mesh with every vertex transformed by t. If t
// mirrors space the winding is flipped so faces stay outward-facing.
func (m *Mesh) Transform(t Mat4) *Mesh {
	out := &Mesh{Tris: make([]Tri, len(m.Tris))}
	flip := t.Det3() < 0
	for i, tr := range m.Tris {
		a, b, c := t.Point(tr.P[0]), t.Point(tr.P[1]), t.Point(tr.P[2])
		if flip {
			b, c = c, b
		}
		out.Tris[i] = Tri{P: [3]Vec3{a, b, c}, C: tr.C}
	}
	return out
}

func (m *Mesh) Bounds() Box {
	b := EmptyBox()
	for _, t := range m.Tris {
		b = b.Extend(t.P[0]).Extend(t.P[1]).Extend(t.P[2])
	}
	return b
}

// Volume returns the signed volume (positive for outward-facing closed
// meshes).
func (m *Mesh) Volume() float64 {
	var v float64
	for _, t := range m.Tris {
		v += t.P[0].Dot(t.P[1].Cross(t.P[2]))
	}
	return v / 6
}

// Colored returns a copy with every triangle set to c.
func (m *Mesh) Colored(c Color) *Mesh {
	out := &Mesh{Tris: make([]Tri, len(m.Tris))}
	for i, t := range m.Tris {
		t.C = c
		out.Tris[i] = t
	}
	return out
}

// Flipped returns a copy with reversed winding.
func (m *Mesh) Flipped() *Mesh {
	out := &Mesh{Tris: make([]Tri, len(m.Tris))}
	for i, t := range m.Tris {
		t.P[1], t.P[2] = t.P[2], t.P[1]
		out.Tris[i] = t
	}
	return out
}

// Merge concatenates meshes without any boolean processing.
func Merge(ms ...*Mesh) *Mesh {
	n := 0
	for _, m := range ms {
		if m != nil {
			n += len(m.Tris)
		}
	}
	out := &Mesh{Tris: make([]Tri, 0, n)}
	for _, m := range ms {
		if m != nil {
			out.Tris = append(out.Tris, m.Tris...)
		}
	}
	return out
}

// Clean drops degenerate triangles.
func (m *Mesh) Clean() *Mesh {
	out := &Mesh{Tris: make([]Tri, 0, len(m.Tris))}
	for _, t := range m.Tris {
		if t.Area() > 1e-12 {
			out.Tris = append(out.Tris, t)
		}
	}
	return out
}

// Oriented flips the mesh if its signed volume is negative.
func (m *Mesh) Oriented() *Mesh {
	if m.Volume() < 0 {
		return m.Flipped()
	}
	return m
}

// FitTo scales and translates the mesh so its bounding box is centred at
// the origin with the given size. Zero extents are left alone.
func (m *Mesh) FitTo(size Vec3) *Mesh {
	b := m.Bounds()
	if b.IsEmpty() {
		return m
	}
	s := b.Size()
	f := V(1, 1, 1)
	for i := 0; i < 3; i++ {
		if s.Axis(i) > 1e-12 {
			f = f.WithAxis(i, size.Axis(i)/s.Axis(i))
		}
	}
	return m.Transform(Scale(f).Mul(Translate(b.Center().Neg())))
}

// EdgeCheck counts boundary and non-manifold edges after welding vertices
// within eps. A watertight mesh returns (0, 0). T-junctions produced by CSG
// show up as open edges, so this is mainly useful for primitives.
func (m *Mesh) EdgeCheck(eps float64) (open, nonManifold int) {
	// vertices within eps are one; rounded as floats (as int64, coordinates
	// past 1e11 or so over a small eps would overflow, and distinct
	// vertices collide); eps 0: exact
	type key [3]float64
	q := func(p Vec3) key {
		if eps <= 0 {
			return key{p.X, p.Y, p.Z}
		}
		return key{math.Round(p.X / eps), math.Round(p.Y / eps), math.Round(p.Z / eps)}
	}
	type edge [2]key
	count := map[edge]int{}
	for _, t := range m.Tris {
		for i := 0; i < 3; i++ {
			a, b := q(t.P[i]), q(t.P[(i+1)%3])
			if a == b {
				continue
			}
			count[edge{a, b}]++
		}
	}
	for e, n := range count {
		r := count[edge{e[1], e[0]}]
		if r == 0 {
			open++
		} else if n != r {
			nonManifold++
		}
	}
	return
}
