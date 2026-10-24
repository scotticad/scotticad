// Package geom holds the vector math, meshes, CSG and file I/O used by
// ScottiCAD. Units are millimetres and Z points up.
package geom

import "math"

// Vec3 is a 3D point or direction.
type Vec3 struct{ X, Y, Z float64 }

func V(x, y, z float64) Vec3 { return Vec3{x, y, z} }

func (a Vec3) Add(b Vec3) Vec3    { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Sub(b Vec3) Vec3    { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Mul(s float64) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func (a Vec3) MulV(b Vec3) Vec3   { return Vec3{a.X * b.X, a.Y * b.Y, a.Z * b.Z} }
func (a Vec3) Dot(b Vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a Vec3) Neg() Vec3          { return Vec3{-a.X, -a.Y, -a.Z} }
func (a Vec3) Len() float64       { return math.Sqrt(a.Dot(a)) }

func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

func (a Vec3) Norm() Vec3 {
	l := a.Len()
	if l == 0 {
		return a
	}
	return a.Mul(1 / l)
}

func (a Vec3) Lerp(b Vec3, t float64) Vec3 { return a.Add(b.Sub(a).Mul(t)) }

func (a Vec3) Min(b Vec3) Vec3 {
	return Vec3{math.Min(a.X, b.X), math.Min(a.Y, b.Y), math.Min(a.Z, b.Z)}
}

func (a Vec3) Max(b Vec3) Vec3 {
	return Vec3{math.Max(a.X, b.X), math.Max(a.Y, b.Y), math.Max(a.Z, b.Z)}
}

// Axis returns component i (0=X, 1=Y, 2=Z).
func (a Vec3) Axis(i int) float64 {
	switch i {
	case 0:
		return a.X
	case 1:
		return a.Y
	}
	return a.Z
}

// WithAxis returns a copy of a with component i set to v.
func (a Vec3) WithAxis(i int, v float64) Vec3 {
	switch i {
	case 0:
		a.X = v
	case 1:
		a.Y = v
	default:
		a.Z = v
	}
	return a
}

// Box is an axis-aligned bounding box.
type Box struct{ Min, Max Vec3 }

func EmptyBox() Box {
	inf := math.Inf(1)
	return Box{V(inf, inf, inf), V(-inf, -inf, -inf)}
}

func (b Box) IsEmpty() bool     { return b.Min.X > b.Max.X }
func (b Box) Extend(p Vec3) Box { return Box{b.Min.Min(p), b.Max.Max(p)} }
func (b Box) Size() Vec3        { return b.Max.Sub(b.Min) }
func (b Box) Center() Vec3      { return b.Min.Add(b.Max).Mul(0.5) }

func (b Box) Union(o Box) Box {
	if o.IsEmpty() {
		return b
	}
	if b.IsEmpty() {
		return o
	}
	return Box{b.Min.Min(o.Min), b.Max.Max(o.Max)}
}

// Overlaps reports whether the boxes intersect (with tolerance eps).
func (b Box) Overlaps(o Box, eps float64) bool {
	return b.Min.X <= o.Max.X+eps && o.Min.X <= b.Max.X+eps &&
		b.Min.Y <= o.Max.Y+eps && o.Min.Y <= b.Max.Y+eps &&
		b.Min.Z <= o.Max.Z+eps && o.Min.Z <= b.Max.Z+eps
}

// Corners returns the eight corners of the box.
func (b Box) Corners() [8]Vec3 {
	var c [8]Vec3
	for i := range c {
		p := b.Min
		if i&1 != 0 {
			p.X = b.Max.X
		}
		if i&2 != 0 {
			p.Y = b.Max.Y
		}
		if i&4 != 0 {
			p.Z = b.Max.Z
		}
		c[i] = p
	}
	return c
}

// Transform returns the axis-aligned box enclosing b transformed by m.
func (b Box) Transform(m Mat4) Box {
	if b.IsEmpty() {
		return b
	}
	r := EmptyBox()
	for _, c := range b.Corners() {
		r = r.Extend(m.Point(c))
	}
	return r
}
