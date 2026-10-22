package geom

import "math"

// Mat4 is a 4x4 matrix stored column-major (the layout OpenGL expects).
// Element (row r, column c) lives at index c*4+r.
type Mat4 [16]float64

func Identity() Mat4 { return Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1} }

func Translate(v Vec3) Mat4 {
	m := Identity()
	m[12], m[13], m[14] = v.X, v.Y, v.Z
	return m
}

func Scale(v Vec3) Mat4 {
	m := Identity()
	m[0], m[5], m[10] = v.X, v.Y, v.Z
	return m
}

func RotateX(rad float64) Mat4 {
	c, s := math.Cos(rad), math.Sin(rad)
	m := Identity()
	m[5], m[6], m[9], m[10] = c, s, -s, c
	return m
}

func RotateY(rad float64) Mat4 {
	c, s := math.Cos(rad), math.Sin(rad)
	m := Identity()
	m[0], m[2], m[8], m[10] = c, -s, s, c
	return m
}

func RotateZ(rad float64) Mat4 {
	c, s := math.Cos(rad), math.Sin(rad)
	m := Identity()
	m[0], m[1], m[4], m[5] = c, s, -s, c
	return m
}

// RotateAxis rotates about world axis 0=X, 1=Y, 2=Z.
func RotateAxis(axis int, rad float64) Mat4 {
	switch axis {
	case 0:
		return RotateX(rad)
	case 1:
		return RotateY(rad)
	}
	return RotateZ(rad)
}

// RotateAround rotates by rad about the unit axis k (through the origin).
func RotateAround(k Vec3, rad float64) Mat4 {
	c, s := math.Cos(rad), math.Sin(rad)
	t := 1 - c
	x, y, z := k.X, k.Y, k.Z
	m := Identity()
	m.SetCol(0, V(t*x*x+c, t*x*y+s*z, t*x*z-s*y))
	m.SetCol(1, V(t*x*y-s*z, t*y*y+c, t*y*z+s*x))
	m.SetCol(2, V(t*x*z+s*y, t*y*z-s*x, t*z*z+c))
	return m
}

// RotateFromTo is the shortest rotation taking direction f onto t. For
// opposite directions it turns half a turn about an axis square to f.
func RotateFromTo(f, t Vec3) Mat4 {
	f, t = f.Norm(), t.Norm()
	c := f.Dot(t)
	if c > 1-1e-9 {
		return Identity()
	}
	if c < -1+1e-9 {
		k := f.Cross(V(1, 0, 0))
		if k.Len() < 1e-6 {
			k = f.Cross(V(0, 1, 0))
		}
		return RotateAround(k.Norm(), math.Pi)
	}
	return RotateAround(f.Cross(t).Norm(), math.Acos(math.Max(-1, math.Min(1, c))))
}

// Mul returns a*b.
func (a Mat4) Mul(b Mat4) Mat4 {
	var out Mat4
	for c := 0; c < 4; c++ {
		for r := 0; r < 4; r++ {
			var s float64
			for k := 0; k < 4; k++ {
				s += a[k*4+r] * b[c*4+k]
			}
			out[c*4+r] = s
		}
	}
	return out
}

// Point transforms a point (w=1).
func (m Mat4) Point(p Vec3) Vec3 {
	return Vec3{
		m[0]*p.X + m[4]*p.Y + m[8]*p.Z + m[12],
		m[1]*p.X + m[5]*p.Y + m[9]*p.Z + m[13],
		m[2]*p.X + m[6]*p.Y + m[10]*p.Z + m[14],
	}
}

// Dir transforms a direction (w=0).
func (m Mat4) Dir(d Vec3) Vec3 {
	return Vec3{
		m[0]*d.X + m[4]*d.Y + m[8]*d.Z,
		m[1]*d.X + m[5]*d.Y + m[9]*d.Z,
		m[2]*d.X + m[6]*d.Y + m[10]*d.Z,
	}
}

// Project transforms p and returns the result divided by w, plus w.
func (m Mat4) Project(p Vec3) (Vec3, float64) {
	x := m[0]*p.X + m[4]*p.Y + m[8]*p.Z + m[12]
	y := m[1]*p.X + m[5]*p.Y + m[9]*p.Z + m[13]
	z := m[2]*p.X + m[6]*p.Y + m[10]*p.Z + m[14]
	w := m[3]*p.X + m[7]*p.Y + m[11]*p.Z + m[15]
	if w == 0 {
		return Vec3{x, y, z}, 0
	}
	return Vec3{x / w, y / w, z / w}, w
}

// Col returns column c (0..3) as a vector (ignoring the 4th row).
func (m Mat4) Col(c int) Vec3 { return Vec3{m[c*4], m[c*4+1], m[c*4+2]} }

// SetCol sets column c's first three rows.
func (m *Mat4) SetCol(c int, v Vec3) { m[c*4], m[c*4+1], m[c*4+2] = v.X, v.Y, v.Z }

func (m Mat4) Translation() Vec3 { return Vec3{m[12], m[13], m[14]} }

func (m Mat4) Transpose() Mat4 {
	var t Mat4
	for c := 0; c < 4; c++ {
		for r := 0; r < 4; r++ {
			t[r*4+c] = m[c*4+r]
		}
	}
	return t
}

// Det3 is the determinant of the upper-left 3x3 block. Negative means the
// transform mirrors geometry.
func (m Mat4) Det3() float64 {
	return m[0]*(m[5]*m[10]-m[6]*m[9]) + m[1]*(m[6]*m[8]-m[4]*m[10]) + m[2]*(m[4]*m[9]-m[5]*m[8])
}

// Inverse returns the inverse of m and whether it exists.
func (m Mat4) Inverse() (Mat4, bool) {
	var inv Mat4
	inv[0] = m[5]*m[10]*m[15] - m[5]*m[11]*m[14] - m[9]*m[6]*m[15] + m[9]*m[7]*m[14] + m[13]*m[6]*m[11] - m[13]*m[7]*m[10]
	inv[4] = -m[4]*m[10]*m[15] + m[4]*m[11]*m[14] + m[8]*m[6]*m[15] - m[8]*m[7]*m[14] - m[12]*m[6]*m[11] + m[12]*m[7]*m[10]
	inv[8] = m[4]*m[9]*m[15] - m[4]*m[11]*m[13] - m[8]*m[5]*m[15] + m[8]*m[7]*m[13] + m[12]*m[5]*m[11] - m[12]*m[7]*m[9]
	inv[12] = -m[4]*m[9]*m[14] + m[4]*m[10]*m[13] + m[8]*m[5]*m[14] - m[8]*m[6]*m[13] - m[12]*m[5]*m[10] + m[12]*m[6]*m[9]
	inv[1] = -m[1]*m[10]*m[15] + m[1]*m[11]*m[14] + m[9]*m[2]*m[15] - m[9]*m[3]*m[14] - m[13]*m[2]*m[11] + m[13]*m[3]*m[10]
	inv[5] = m[0]*m[10]*m[15] - m[0]*m[11]*m[14] - m[8]*m[2]*m[15] + m[8]*m[3]*m[14] + m[12]*m[2]*m[11] - m[12]*m[3]*m[10]
	inv[9] = -m[0]*m[9]*m[15] + m[0]*m[11]*m[13] + m[8]*m[1]*m[15] - m[8]*m[3]*m[13] - m[12]*m[1]*m[11] + m[12]*m[3]*m[9]
	inv[13] = m[0]*m[9]*m[14] - m[0]*m[10]*m[13] - m[8]*m[1]*m[14] + m[8]*m[2]*m[13] + m[12]*m[1]*m[10] - m[12]*m[2]*m[9]
	inv[2] = m[1]*m[6]*m[15] - m[1]*m[7]*m[14] - m[5]*m[2]*m[15] + m[5]*m[3]*m[14] + m[13]*m[2]*m[7] - m[13]*m[3]*m[6]
	inv[6] = -m[0]*m[6]*m[15] + m[0]*m[7]*m[14] + m[4]*m[2]*m[15] - m[4]*m[3]*m[14] - m[12]*m[2]*m[7] + m[12]*m[3]*m[6]
	inv[10] = m[0]*m[5]*m[15] - m[0]*m[7]*m[13] - m[4]*m[1]*m[15] + m[4]*m[3]*m[13] + m[12]*m[1]*m[7] - m[12]*m[3]*m[5]
	inv[14] = -m[0]*m[5]*m[14] + m[0]*m[6]*m[13] + m[4]*m[1]*m[14] - m[4]*m[2]*m[13] - m[12]*m[1]*m[6] + m[12]*m[2]*m[5]
	inv[3] = -m[1]*m[6]*m[11] + m[1]*m[7]*m[10] + m[5]*m[2]*m[11] - m[5]*m[3]*m[10] - m[9]*m[2]*m[7] + m[9]*m[3]*m[6]
	inv[7] = m[0]*m[6]*m[11] - m[0]*m[7]*m[10] - m[4]*m[2]*m[11] + m[4]*m[3]*m[10] + m[8]*m[2]*m[7] - m[8]*m[3]*m[6]
	inv[11] = -m[0]*m[5]*m[11] + m[0]*m[7]*m[9] + m[4]*m[1]*m[11] - m[4]*m[3]*m[9] - m[8]*m[1]*m[7] + m[8]*m[3]*m[5]
	inv[15] = m[0]*m[5]*m[10] - m[0]*m[6]*m[9] - m[4]*m[1]*m[10] + m[4]*m[2]*m[9] + m[8]*m[1]*m[6] - m[8]*m[2]*m[5]
	det := m[0]*inv[0] + m[1]*inv[4] + m[2]*inv[8] + m[3]*inv[12]
	if det == 0 {
		return Identity(), false
	}
	d := 1 / det
	for i := range inv {
		inv[i] *= d
	}
	return inv, true
}

// NormalMatrix returns the inverse-transpose of m's linear part (as a Mat4).
func (m Mat4) NormalMatrix() Mat4 {
	l := m
	l[12], l[13], l[14] = 0, 0, 0
	inv, ok := l.Inverse()
	if !ok {
		return Identity()
	}
	return inv.Transpose()
}

func Perspective(fovy, aspect, near, far float64) Mat4 {
	f := 1 / math.Tan(fovy/2)
	nf := 1 / (near - far)
	var m Mat4
	m[0] = f / aspect
	m[5] = f
	m[10] = (far + near) * nf
	m[11] = -1
	m[14] = 2 * far * near * nf
	return m
}

func Ortho(l, r, b, t, n, f float64) Mat4 {
	m := Identity()
	m[0] = 2 / (r - l)
	m[5] = 2 / (t - b)
	m[10] = -2 / (f - n)
	m[12] = -(r + l) / (r - l)
	m[13] = -(t + b) / (t - b)
	m[14] = -(f + n) / (f - n)
	return m
}

func LookAt(eye, center, up Vec3) Mat4 {
	z := eye.Sub(center).Norm()
	x := up.Cross(z).Norm()
	if x.Len() < 1e-9 {
		x = V(1, 0, 0)
	}
	y := z.Cross(x)
	return Mat4{
		x.X, y.X, z.X, 0,
		x.Y, y.Y, z.Y, 0,
		x.Z, y.Z, z.Z, 0,
		-x.Dot(eye), -y.Dot(eye), -z.Dot(eye), 1,
	}
}

// Float32 converts to a float32 array for upload to OpenGL.
func (m Mat4) Float32() [16]float32 {
	var f [16]float32
	for i, v := range m {
		f[i] = float32(v)
	}
	return f
}

// AxisScales returns the lengths of the first three columns (local scale).
func (m Mat4) AxisScales() Vec3 { return Vec3{m.Col(0).Len(), m.Col(1).Len(), m.Col(2).Len()} }

// IsOrthogonal reports whether the first three columns are mutually
// perpendicular (no shear).
func (m Mat4) IsOrthogonal() bool {
	a, b, c := m.Col(0).Norm(), m.Col(1).Norm(), m.Col(2).Norm()
	const eps = 1e-6
	return math.Abs(a.Dot(b)) < eps && math.Abs(a.Dot(c)) < eps && math.Abs(b.Dot(c)) < eps
}
