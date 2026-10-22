//go:build windows

package app

import (
	"slices"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
)

// Section view: a plane square to one of the workplane's axes hides one
// side of the shapes being worked on, so their inside can be seen. Only
// the view changes: the design, the undo history and exports are
// untouched. Faces seen through the cut are drawn as the cut face (a
// hatched cap), and the hidden side can't be clicked.
//
// Opened with shapes selected it cuts those (with nothing selected,
// everything). Like the other tools it closes when another shape is
// selected, on clicking off, and on Esc; pinned, it stays open while
// shapes are selected and moved (to adjust parts while looking inside),
// until Close, Esc or the toolbar button.

type sectionState struct {
	on   bool
	axis int     // the workplane's axis the plane is square to
	at   float64 // where it crosses that axis, workplane mm
	keep float64 // +1 keeps (shows) the + side of the plane, -1 the - side
	// ids are the top-level shapes it cuts; nil cuts every shape
	ids []int
	// pinned keeps it open when the selection changes
	pinned bool
	// sel is the selection when it opened or was unpinned: unpinned, a
	// change from it closes the view
	sel []int
}

// sectionNodes are the shapes the view cuts.
func (a *App) sectionNodes() []*doc.Node {
	if a.section.ids != nil {
		return a.ed.Doc.FindAll(a.section.ids)
	}
	var ns []*doc.Node
	for _, n := range a.ed.Doc.Nodes {
		if !n.Hidden {
			ns = append(ns, n)
		}
	}
	return ns
}

// sectionCuts reports whether the view cuts top-level node n.
func (a *App) sectionCuts(n *doc.Node) bool {
	return a.section.on && (a.section.ids == nil || slices.Contains(a.section.ids, n.ID))
}

// sectionBounds is what the plane spans and moves through: the shapes it
// cuts (or a patch around the origin if there are none).
func (a *App) sectionBounds() geom.Box {
	b := doc.BoundsIn(a.sectionNodes(), a.ed.Work())
	if b.IsEmpty() {
		b = geom.Box{Min: geom.V(-20, -20, 0), Max: geom.V(20, 20, 20)}
	}
	return b
}

// setSection turns the view on or off. Turning it on aims it at the
// selection (or everything) with the plane through its middle, showing
// the side away from the camera: it cuts away what is in front.
func (a *App) setSection(on bool) {
	s := &a.section
	if on && !s.on {
		s.on, s.pinned = true, false
		s.axis = 1
		a.aimSection()
	}
	s.on = on
	a.w.Redraw()
}

// aimSection points the view at the current selection (all shapes if
// there is none) and centres the plane on it.
func (a *App) aimSection() {
	s := &a.section
	s.ids = nil
	if len(a.ed.Sel) > 0 {
		s.ids = slices.Clone(a.ed.Sel)
	}
	s.sel = slices.Clone(a.ed.Sel)
	a.centreSection(0)
}

// followSelection closes the view when the selection changes (another
// shape clicked, or clicking off), as the other tools close. Called once
// a frame.
func (a *App) followSelection() {
	s := &a.section
	if s.on && !s.pinned && !slices.Equal(a.ed.Sel, s.sel) {
		a.setSection(false)
	}
}

// centreSection puts the plane through the middle of what it cuts and,
// unless keep is ±1, shows the side away from the camera.
func (a *App) centreSection(keep float64) {
	s := &a.section
	b := a.sectionBounds()
	s.at = b.Center().Axis(s.axis)
	if keep != 0 {
		s.keep = keep
		return
	}
	s.keep = 1
	if a.eyeW().Axis(s.axis) > s.at {
		s.keep = -1
	}
}

// sectionClip is the clip the view applies to the shapes it cuts.
func (a *App) sectionClip() ([4]float32, bool) {
	s := a.section
	if !s.on {
		return [4]float32{}, false
	}
	n, p := a.planeWorld(s.axis, s.at)
	return clipPlane(n.Mul(s.keep), p), true
}

// sectionShows reports whether world point p is on the shown side.
func (a *App) sectionShows(p geom.Vec3) bool {
	s := a.section
	if !s.on {
		return true
	}
	n, q := a.planeWorld(s.axis, s.at)
	return n.Dot(p.Sub(q))*s.keep >= -1e-9
}

// drawSectionPlane outlines the plane in the scene.
func (a *App) drawSectionPlane() {
	if a.section.on {
		a.drawPlaneInScene(planeCorners(a.sectionBounds(), a.section.axis, a.section.at), false)
	}
}

// drawSection draws the view's grip, its position label and its panel.
func (a *App) drawSection() {
	s := &a.section
	if !s.on {
		return
	}
	b := a.sectionBounds()
	gx, gy, ok := a.drawPlaneGrip("section-grip", "Drag to move the section", planeCorners(b, s.axis, s.at), s.axis, s.at,
		a.planeDrag(s.axis, &s.at, a.sectionBounds, false))
	if ok {
		lo := b.Min.Axis(s.axis)
		a.labels = append(a.labels, dimLabel{id: "section-at", x: gx, y: gy + 24, value: s.at - lo,
			commit: func(v float64) { s.at = lo + v; a.w.Redraw() }})
	}
	items := []panelItem{{kind: panelCaption, label: "Section along"}}
	items = append(items, axisItems("section-axis-", s.axis, func(i int) { s.axis = i; a.centreSection(0) })...)
	items = append(items,
		panelItem{kind: panelButton, id: "section-flip", label: "Flip", tip: "Show the other side", click: func() { s.keep = -s.keep }},
		panelItem{kind: panelToggle, id: "section-pin", label: "Pin", on: s.pinned, tip: "Keep the section open while selecting and moving shapes",
			click: func() { s.pinned = !s.pinned; s.sel = slices.Clone(a.ed.Sel) }},
		panelItem{kind: panelButton, id: "section-close", label: "Close", tip: "Esc", click: func() { a.setSection(false) }},
	)
	y := a.vp.Y + 12
	if a.mode == modeSplit || a.bannerText() != "" {
		y += 48 // below the Split tool's panel or a hint banner
	}
	a.drawToolPanel(y, items)
}
