//go:build windows

package app

import (
	"math"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
)

// The Lay flat tool: click a face of a shape and the shape turns so that
// face lies on the workplane. The face under the pointer is highlighted:
// every triangle in its plane, so a whole flat side lights up.

type layFace struct {
	node *doc.Node
	n    geom.Vec3  // outward normal, world
	mesh *geom.Mesh // the face's triangles, world, lifted slightly toward the camera
}

var colLayFace = [4]float32{1.0, 0.45, 0.12, 1}

// layFaceAt is the face under (x, y), or nil.
func (a *App) layFaceAt(x, y float64) *layFace {
	h, ok := a.pick(x, y, false, nil)
	if !ok || h.node.Locked || h.mesh == nil {
		return nil
	}
	inv, ok := h.m.Inverse()
	if !ok {
		return nil
	}
	lp := inv.Point(h.p)
	tol := 1e-4 * math.Max(1, h.mesh.Bounds().Size().Len())
	lift := h.n.Mul(0.6 * a.pixelSize(h.p)) // keeps the highlight off the face's own depth
	face := &geom.Mesh{}
	for _, t := range h.mesh.Tris {
		if t.Normal().Dot(h.ln) < 0.9999 || math.Abs(h.ln.Dot(t.P[0].Sub(lp))) > tol {
			continue
		}
		var w geom.Tri
		for i, p := range t.P {
			w.P[i] = h.m.Point(p).Add(lift)
		}
		face.Tris = append(face.Tris, w)
	}
	if len(face.Tris) == 0 {
		return nil
	}
	// the face's outward normal: h.n is turned toward the camera, which
	// is the inward side when the face is seen from inside (through a
	// section cut)
	out := h.m.NormalMatrix().Dir(h.ln).Norm()
	return &layFace{node: h.node, n: out, mesh: face}
}

// drawLayFace highlights the face the tool would lay flat (cut like its
// shape when Section view is on).
func (a *App) drawLayFace() {
	if a.mode != modeLayFlat || a.layHover == nil {
		return
	}
	st := render.Style{Tint: colLayFace, Alpha: 0.8}
	if clip, ok := a.sectionClip(); ok && a.sectionCuts(a.layHover.node) {
		st.Clip = clip
	}
	a.scene.DrawMesh(a.layHover.mesh, st)
}

// layFlatClick lays the clicked face's shape (with the rest of the
// selection, if it is selected) on that face; false if no face was
// clicked.
func (a *App) layFlatClick(x, y float64) bool {
	f := a.layFaceAt(x, y)
	if f == nil {
		return false
	}
	if !a.ed.IsSelected(f.node.ID) {
		a.ed.Select(f.node.ID)
	}
	if err := a.ed.LayFlat(nil, f.n); err != nil {
		a.showToast(err.Error())
	}
	a.layHover = nil
	a.setMode(modeNone)
	return true
}
