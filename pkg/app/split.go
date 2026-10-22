//go:build windows

package app

import (
	"strings"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/win"
)

// The Split tool (K): a plane square to one of the workplane's axes cuts
// the selected shapes in two. While it is open the shapes show as their
// halves (clipped on the GPU, no booleans until Split is pressed); the
// plane is dragged by its grip or placed by typing its distance from the
// selection's low side.

type splitState struct {
	axis int     // the workplane's axis the plane is square to
	at   float64 // where the plane crosses it, workplane mm
	keep string  // doc.KeepBoth, KeepAbove or KeepBelow
	gap  float64 // mm the half above (+ side) moves away
}

var (
	colSplitPlane = render.Color{1.0, 0.45, 0.12, 0.18}
	colSplitEdge  = render.RGB(0xf0, 0x6a, 0x1c)
)

// initSplit opens the tool on the selection: a level cut through the
// middle.
func (a *App) initSplit() {
	a.split.axis = 2
	a.split.keep = doc.KeepBoth
	a.centreSplit()
}

func (a *App) centreSplit() {
	b := a.splitBounds()
	if !b.IsEmpty() {
		a.split.at = b.Center().Axis(a.split.axis)
	}
}

// splitBounds is the bounds, in the workplane's frame, of what will be
// split.
func (a *App) splitBounds() geom.Box { return doc.BoundsIn(a.ed.Editable(), a.ed.Work()) }

// splitHalf is how one half is previewed: moved by m, clipped by clip.
type splitHalf struct {
	m    geom.Mat4
	clip [4]float32
}

// splitHalves returns, while the tool is open, the halves to draw for a
// top-level node the plane crosses; nil means draw it as usual.
func (a *App) splitHalves(top *doc.Node) []splitHalf {
	if a.mode != modeSplit || top.Locked || !a.ed.IsSelected(top.ID) {
		return nil
	}
	b := doc.BoundsIn([]*doc.Node{top}, a.ed.Work())
	s := a.split
	if s.at <= b.Min.Axis(s.axis)+1e-6 || s.at >= b.Max.Axis(s.axis)-1e-6 {
		return nil
	}
	n, p := a.planeWorld(s.axis, s.at)
	var out []splitHalf
	for _, side := range []float64{-1, 1} {
		if (side < 0 && s.keep == doc.KeepAbove) || (side > 0 && s.keep == doc.KeepBelow) {
			continue
		}
		t := geom.Vec3{}
		if side > 0 {
			t = n.Mul(s.gap) // the half above moves; the one below stays
		}
		out = append(out, splitHalf{m: geom.Translate(t), clip: clipPlane(n.Mul(side), p.Add(t))})
	}
	return out
}

// drawSplitPlane draws the cutting plane in the scene (translucent pass).
func (a *App) drawSplitPlane() {
	if a.mode != modeSplit {
		return
	}
	if b := a.splitBounds(); !b.IsEmpty() {
		a.drawPlaneInScene(planeCorners(b, a.split.axis, a.split.at), true)
	}
}

// drawSplit draws the tool's overlay: the grip that drags the plane, its
// distance label, and the options panel.
func (a *App) drawSplit() {
	if a.mode != modeSplit {
		return
	}
	if len(a.ed.Editable()) == 0 {
		a.setMode(modeNone)
		return
	}
	b := a.splitBounds()
	if b.IsEmpty() { // nothing there to cut
		a.setMode(modeNone)
		return
	}
	s := &a.split
	gx, gy, ok := a.drawPlaneGrip("split-grip", "Drag to move the cut", planeCorners(b, s.axis, s.at), s.axis, s.at,
		a.planeDrag(s.axis, &s.at, a.splitBounds, true))
	if ok {
		// the plane's distance from the selection's low side, typed to set
		lo := b.Min.Axis(s.axis)
		a.labels = append(a.labels, dimLabel{id: "split-at", x: gx, y: gy + 24, value: s.at - lo,
			commit: func(v float64) {
				if v <= 0 || lo+v >= b.Max.Axis(s.axis) {
					a.showToast("the cut must go through the shapes")
					return
				}
				s.at = lo + v
				a.w.Redraw()
			}})
	}
	a.drawSplitPanel()
}

// splitSideNames names the plane's two sides for the Keep buttons.
func splitSideNames(axis int) (below, above string) {
	switch axis {
	case 0:
		return "Left", "Right"
	case 1:
		return "Front", "Back"
	}
	return "Bottom", "Top"
}

// drawSplitPanel is the options panel at the top of the viewport.
func (a *App) drawSplitPanel() {
	s := &a.split
	below, above := splitSideNames(s.axis)
	items := []panelItem{{kind: panelCaption, label: "Split along"}}
	items = append(items, axisItems("split-axis-", s.axis, func(i int) { s.axis = i; a.centreSplit() })...)
	items = append(items, panelItem{kind: panelCaption, label: "   Keep"})
	for _, k := range []struct{ id, label, keep string }{{"split-keep-both", "Both", doc.KeepBoth}, {"split-keep-below", below, doc.KeepBelow}, {"split-keep-above", above, doc.KeepAbove}} {
		k := k
		items = append(items, panelItem{kind: panelToggle, id: k.id, label: k.label, on: s.keep == k.keep, click: func() { s.keep = k.keep }})
	}
	items = append(items,
		panelItem{kind: panelCaption, label: "   Gap"},
		panelItem{kind: panelCustom, id: "split-gap", w: 64, draw: a.drawGapField},
		panelItem{kind: panelButton, id: "split-cancel", label: "Cancel", click: func() { a.setMode(modeNone) }},
		panelItem{kind: panelPrimary, id: "split-apply", label: "Split", click: a.applySplit},
	)
	a.drawToolPanel(a.vp.Y+12, items)
}

// drawGapField is the Gap box: the distance the half above moves away.
func (a *App) drawGapField(r render.Rect) {
	b := a.batch
	s := &a.split
	b.RoundRect(r, 3, colChrome)
	border := colFieldBorder
	if a.field != nil && a.field.id == "split-gap" {
		border = colBlue
	}
	b.RoundRectStroke(r, 3, 1, border)
	a.drawFieldText("split-gap", r, a.fmtLen(s.gap), fontUI, true, colText)
	a.reg(region{r: r, id: "split-gap", cursor: win.CursorIBeam, tip: "Move the top (or right, or back) half away by this much", tipSide: tipBelow,
		press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
			a.openField(&textField{id: "split-gap", text: strings.TrimSpace(editor.Format(s.gap, a.units(), true)),
				commit: func(t string) bool {
					v, err := editor.Parse(t, a.units())
					if err != nil || v < 0 {
						a.showToast("not a distance: " + t)
						return true
					}
					s.gap = v
					return true
				}})
			return nil
		}})
}

// applySplit splits the selection as the tool shows it.
func (a *App) applySplit() {
	if a.field != nil && !a.commitField() {
		return
	}
	s := a.split
	if _, err := a.ed.Split(nil, s.axis, s.at, s.keep, s.gap); err != nil {
		a.showToast(err.Error())
		return
	}
	a.setMode(modeNone)
}
