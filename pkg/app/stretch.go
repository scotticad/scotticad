package app

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/window"
)

// The Stretch tool (T): a plane square to one of the current plane's axes,
// like Split's, where the selection is lengthened or shortened. The amount
// is typed: +10 adds 10 mm at the plane, -10 takes out a 10 mm slab centred
// on it, and a plain 60 makes the selection 60 mm long along the axis. The
// low end stays put. Stretch makes a stretch group, which keeps the shapes
// as they were and can be changed later in Properties.

type stretchState struct {
	axis     int     // the workplane's axis the plane is square to
	at       float64 // where the plane crosses it, workplane mm
	amount   float64 // mm to add (taken out if negative)
	byLength bool    // the amount was typed as a length to reach
	length   float64 // that length
}

// initStretch opens the tool on the selection: a cut through the middle,
// across the longest side, adding nothing yet.
func (a *App) initStretch() {
	s := &a.stretch
	*s = stretchState{}
	b := a.stretchBounds()
	if !b.IsEmpty() {
		sz := b.Size()
		for i := 1; i < 3; i++ {
			if sz.Axis(i) > sz.Axis(s.axis) {
				s.axis = i
			}
		}
	}
	a.centreStretch()
}

func (a *App) centreStretch() {
	if b := a.stretchBounds(); !b.IsEmpty() {
		a.stretch.at = b.Center().Axis(a.stretch.axis)
	}
}

// stretchBounds is the bounds, in the workplane's frame, of what will be
// stretched.
func (a *App) stretchBounds() geom.Box { return doc.BoundsIn(a.ed.Editable(), a.ed.Work()) }

// stretchAmount is the amount the tool will apply: as typed, or what makes
// the length typed.
func (a *App) stretchAmount() float64 {
	s := a.stretch
	if s.byLength {
		return s.length - a.stretchBounds().Size().Axis(s.axis)
	}
	return s.amount
}

// drawStretchPlane draws the plane in the scene (translucent pass), and
// when shrinking the slab's two faces.
func (a *App) drawStretchPlane() {
	if a.mode != modeStretch {
		return
	}
	b := a.stretchBounds()
	if b.IsEmpty() {
		return
	}
	s := a.stretch
	if d := a.stretchAmount(); d < 0 {
		a.drawPlaneInScene(planeCorners(b, s.axis, s.at+d/2), true)
		a.drawPlaneInScene(planeCorners(b, s.axis, s.at-d/2), true)
		return
	}
	a.drawPlaneInScene(planeCorners(b, s.axis, s.at), true)
}

// drawStretch draws the tool's overlay: the grip that drags the plane, its
// distance label, and the options panel.
func (a *App) drawStretch() {
	if a.mode != modeStretch {
		return
	}
	if len(a.ed.Editable()) == 0 {
		a.setMode(modeNone)
		return
	}
	b := a.stretchBounds()
	if b.IsEmpty() {
		a.setMode(modeNone)
		return
	}
	s := &a.stretch
	gx, gy, ok := a.drawPlaneGrip("stretch-grip", "Drag to move the plane", planeCorners(b, s.axis, s.at), s.axis, s.at,
		a.planeDrag(s.axis, &s.at, a.stretchBounds, true))
	if ok {
		lo := b.Min.Axis(s.axis)
		a.labels = append(a.labels, dimLabel{id: "stretch-at", x: gx, y: gy + 24, value: s.at - lo,
			commit: func(v float64) {
				if v <= 0 || lo+v >= b.Max.Axis(s.axis) {
					a.showToast("the plane must go through the shapes")
					return
				}
				s.at = lo + v
				a.w.Redraw()
			}})
	}
	a.drawStretchPanel()
}

// drawStretchPanel is the options panel at the top of the viewport.
func (a *App) drawStretchPanel() {
	s := &a.stretch
	items := []panelItem{{kind: panelCaption, label: "Stretch along"}}
	items = append(items, axisItems("stretch-axis-", s.axis, func(i int) { s.axis = i; a.centreStretch() })...)
	items = append(items,
		panelItem{kind: panelCaption, label: "   By"},
		panelItem{kind: panelCustom, id: "stretch-amount", w: 80, draw: a.drawStretchField},
		panelItem{kind: panelCaption, label: "   " + a.stretchResult()},
		panelItem{kind: panelButton, id: "stretch-cancel", label: "Cancel", click: func() { a.setMode(modeNone) }},
		panelItem{kind: panelPrimary, id: "stretch-apply", label: "Stretch", click: a.applyStretch},
	)
	a.drawToolPanel(a.vp.Y+12, items)
}

// stretchResult is the length the selection will have, for the panel.
func (a *App) stretchResult() string {
	l := a.stretchBounds().Size().Axis(a.stretch.axis)
	return "→ " + strings.TrimSpace(a.fmtLen(l+a.stretchAmount()))
}

// stretchText is the amount as the field shows it: signed, or a length.
func (a *App) stretchText() string {
	s := a.stretch
	if s.byLength {
		return strings.TrimSpace(editor.Format(s.length, a.units(), true))
	}
	t := strings.TrimSpace(editor.Format(math.Abs(s.amount), a.units(), true))
	if s.amount < 0 {
		return "-" + t
	}
	return "+" + t
}

// parseStretch reads the amount field: "+10" adds 10, "-10" takes 10 out,
// "60" is the length to reach. ok is false for anything else.
func parseStretch(t, units string) (st stretchState, ok bool) {
	t = strings.TrimSpace(t)
	sign := 0.0
	switch {
	case strings.HasPrefix(t, "+"):
		sign = 1
	case strings.HasPrefix(t, "-"), strings.HasPrefix(t, "−"):
		sign = -1
	}
	if sign != 0 {
		_, w := utf8.DecodeRuneInString(t)
		t = strings.TrimSpace(t[w:])
	}
	v, err := editor.Parse(t, units)
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return st, false
	}
	if sign == 0 {
		if v <= 0 {
			return st, false
		}
		return stretchState{byLength: true, length: v}, true
	}
	return stretchState{amount: sign * v}, true
}

// drawStretchField is the amount box.
func (a *App) drawStretchField(r render.Rect) {
	b := a.batch
	b.RoundRect(r, 3, colChrome)
	border := colFieldBorder
	if a.field != nil && a.field.id == "stretch-amount" {
		border = colBlue
	}
	b.RoundRectStroke(r, 3, 1, border)
	a.drawFieldText("stretch-amount", r, a.stretchText(), fontUI, true, colText)
	a.reg(region{r: r, id: "stretch-amount", cursor: window.CursorIBeam,
		tip: "+10 adds 10 at the plane, -10 takes out 10 around it, 60 makes it 60 long", tipSide: tipBelow,
		press: func(x, y float64, m window.Mods) func(float64, float64, bool) {
			a.openField(&textField{id: "stretch-amount", text: a.stretchText(),
				commit: func(t string) bool {
					st, ok := parseStretch(t, a.units())
					if !ok {
						a.showToast("type +10 to add, -10 to take out, or a length: " + t)
						return true
					}
					s := &a.stretch
					s.amount, s.byLength, s.length = st.amount, st.byLength, st.length
					return true
				}})
			return nil
		}})
}

// applyStretch stretches the selection as the tool shows it, and opens
// Properties on the stretch.
func (a *App) applyStretch() {
	if a.field != nil && !a.commitField() {
		return
	}
	s := a.stretch
	d := a.stretchAmount()
	if d == 0 {
		a.showToast("type how much to stretch it by")
		return
	}
	if _, err := a.ed.Stretch(nil, s.axis, s.at, d); err != nil {
		a.showToast(err.Error())
		return
	}
	a.setMode(modeNone)
	a.inspectorOpen, a.propsOpen = true, true
}
