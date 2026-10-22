// Package render draws ScottiCAD's 3D scene and 2D interface with OpenGL.
package render

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Camera orbits a target point. Z is up. Yaw rotates around Z, pitch is
// the elevation above the XY plane.
type Camera struct {
	Target     geom.Vec3
	Yaw, Pitch float64
	Dist       float64
	Fov        float64
	Ortho      bool
}

// DefaultCamera is the home view: looking at the build plate from the
// front-left, elevated, so the plate's far edge rises to the right.
func DefaultCamera() Camera {
	return Camera{Target: geom.V(0, 10, 0), Yaw: -math.Pi/2 - 0.40, Pitch: 0.56, Dist: 330, Fov: 45 * math.Pi / 180}
}

func (c *Camera) Eye() geom.Vec3 {
	cp := math.Cos(c.Pitch)
	return c.Target.Add(geom.V(cp*math.Cos(c.Yaw), cp*math.Sin(c.Yaw), math.Sin(c.Pitch)).Mul(c.Dist))
}

func (c *Camera) Up() geom.Vec3 {
	if math.Abs(math.Cos(c.Pitch)) < 1e-4 {
		return geom.V(-math.Cos(c.Yaw)*math.Copysign(1, c.Pitch), -math.Sin(c.Yaw)*math.Copysign(1, c.Pitch), 0)
	}
	return geom.V(0, 0, 1)
}

func (c *Camera) View() geom.Mat4 { return geom.LookAt(c.Eye(), c.Target, c.Up()) }

func (c *Camera) Proj(aspect float64) geom.Mat4 {
	near, far := c.Dist*0.01, c.Dist*20+10000
	if c.Ortho {
		h := c.Dist * math.Tan(c.Fov/2)
		return geom.Ortho(-h*aspect, h*aspect, -h, h, -far, far)
	}
	return geom.Perspective(c.Fov, aspect, near, far)
}

func (c *Camera) VP(aspect float64) geom.Mat4 { return c.Proj(aspect).Mul(c.View()) }

// Ray returns the world ray through pixel (x, y) of a w×h viewport.
func (c *Camera) Ray(x, y, w, h float64) geom.Ray {
	inv, _ := c.VP(w / h).Inverse()
	nx, ny := 2*x/w-1, 1-2*y/h
	n, _ := inv.Project(geom.V(nx, ny, -1))
	f, _ := inv.Project(geom.V(nx, ny, 1))
	return geom.Ray{O: n, D: f.Sub(n).Norm()}
}

// ToScreen projects a world point to viewport pixels. ok is false when the
// point is behind the camera.
func (c *Camera) ToScreen(p geom.Vec3, w, h float64) (float64, float64, bool) {
	q, wc := c.VP(w / h).Project(p)
	if wc <= 0 && !c.Ortho {
		return 0, 0, false
	}
	return (q.X + 1) / 2 * w, (1 - q.Y) / 2 * h, true
}

// PixelSize returns the approximate world size of one pixel at point p.
func (c *Camera) PixelSize(p geom.Vec3, h float64) float64 {
	d := c.Dist
	if !c.Ortho {
		d = p.Sub(c.Eye()).Len()
	}
	return 2 * d * math.Tan(c.Fov/2) / h
}
