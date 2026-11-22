package app

import (
	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/gl"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// skRefStyle is how another shape looks on the board: a translucent tan
// sheet with its edges and outline in a deeper tan.
func skRefStyle(m geom.Mat4) render.Style {
	return render.Style{
		Model:   m,
		Tint:    [4]float32{0.86, 0.68, 0.45, 1},
		Alpha:   0.28,
		Outline: [4]float32{0.78, 0.52, 0.22, 0.9},
	}
}

// The extruded sketch's board shows the design's other shapes where they
// are, seen from above the drawing (translucent tan, with their edges),
// so a drawing can be made to fit them. A revolve's or coil's board is a
// slice through the axis, so it shows none.

// skDrawingFrame maps the drawing's millimeters (x, y, and z up out of the
// board) to world space: where a new sketch will go, or where the one being
// changed is. ok is false when there is no such frame.
func (a *App) skDrawingFrame() (geom.Mat4, bool) {
	s := a.sk
	if s.nodeID == 0 {
		if s.xf == (geom.Mat4{}) {
			return geom.Identity(), true
		}
		return s.xf, true
	}
	var found *doc.Node
	parent := geom.Identity()
	var walk func(ns []*doc.Node, m geom.Mat4) bool
	walk = func(ns []*doc.Node, m geom.Mat4) bool {
		for _, n := range ns {
			if n.ID == s.nodeID {
				found, parent = n, m
				return true
			}
			if walk(n.Children, m.Mul(n.M)) {
				return true
			}
		}
		return false
	}
	if !walk(a.ed.Doc.Nodes, geom.Identity()) {
		return geom.Mat4{}, false
	}
	// as FrameToParent: the drawing's box centered, scaled to the shape's
	// size, then the shape's matrix
	b := shapes.FrameBounds(found.Kind, found.Values())
	if b.IsEmpty() {
		return geom.Mat4{}, false
	}
	f := geom.V(1, 1, 1)
	for i := 0; i < 3; i++ {
		if sz := b.Size().Axis(i); sz > 1e-12 {
			f = f.WithAxis(i, found.Size.Axis(i)/sz)
		}
	}
	return parent.Mul(found.M).Mul(geom.Scale(f)).Mul(geom.Translate(b.Center().Neg())), true
}

// skDrawReferences draws the other shapes on the board, under the drawing.
// Called inside the board's clip: it flushes the 2D drawing so far and
// draws in 3D with the board as the viewport.
func (a *App) skDrawReferences() {
	s := a.sk
	if s.kind != "sketch" {
		return
	}
	w, ok := a.skDrawingFrame()
	if !ok {
		return
	}
	winv, ok := w.Inverse()
	if !ok {
		return
	}
	var parts []scenePart
	for _, p := range a.sceneParts() {
		if p.top.Hidden || (s.nodeID != 0 && p.Owner != nil && p.Owner.ID == s.nodeID) {
			continue
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return
	}
	r := s.board
	a.batch.Flush()
	d := a.w.DPI
	gl.Viewport(int32(r.X*d), int32((a.H-r.Y-r.H)*d), int32(r.W*d), int32(r.H*d))
	gl.Clear(gl.DEPTH_BUFFER_BIT)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LEQUAL)
	gl.DepthMask(true)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	hw, hh := r.W/2/s.zoom, r.H/2/s.zoom
	vp := geom.Ortho(s.cx-hw, s.cx+hw, s.cy-hh, s.cy+hh, -1e6, 1e6).Mul(winv)
	a.scene.Begin(vp, w.Point(geom.V(s.cx, s.cy, 1e5)))
	for _, p := range parts {
		a.scene.DrawMesh(p.Mesh, skRefStyle(p.M))
	}
	gl.Disable(gl.DEPTH_TEST)
	gl.Viewport(0, 0, int32(a.w.W), int32(a.w.H))
}
