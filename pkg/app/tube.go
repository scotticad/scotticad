package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/window"
)

// The bent tube on the canvas: while one is selected, each row of its bend
// table has a marker on the tube (at the middle of the row's bend, or at
// the end of a row without one), numbered as in the table. Clicking a
// marker picks that row in the table; typing in a row picks its marker.

// pickedBendRow is the row of n's bend table that is picked: the one being
// typed in, else the one last picked on the canvas; -1 for none.
func (a *App) pickedBendRow(n *doc.Node) int {
	if a.field != nil && strings.HasPrefix(a.field.id, "bend-") {
		if parts := strings.Split(a.field.id, "-"); len(parts) == 3 {
			if r, err := strconv.Atoi(parts[1]); err == nil {
				return r
			}
		}
	}
	if a.bendRowOf == n.ID && a.bendRow < len(n.Bends) {
		return a.bendRow
	}
	return -1
}

// bendMarkers returns where each row of a top-level bent tube's table is
// marked, in the design's coordinates.
func bendMarkers(n *doc.Node) []geom.Vec3 {
	to, ok := n.FrameToParent()
	if !ok {
		return nil
	}
	_, _, r := shapes.TubeRadii(n.Values())
	path, ends := shapes.TubePath(n.Bends, r)
	out := make([]geom.Vec3, len(ends))
	start := 0
	for i, e := range ends {
		bendFrom := start
		if n.Bends[i].Length > 1e-9 {
			bendFrom++ // the straight run is one station
		}
		out[i] = to(path[(bendFrom+e)/2].P)
		start = e
	}
	return out
}

// drawTubeMarkers draws the bend markers of the one selected shape, if it
// is a bent tube.
func (a *App) drawTubeMarkers() {
	ns := a.ed.Selected()
	if len(ns) != 1 || !shapes.IsTube(ns[0].Kind) || a.drag != nil {
		return
	}
	n := ns[0]
	b := a.batch
	picked := a.pickedBendRow(n)
	for i, p := range bendMarkers(n) {
		x, y, ok := a.project(p)
		if !ok || !a.vp.Contains(x, y) {
			continue
		}
		id := fmt.Sprintf("bend-mark-%d", i)
		rad, fill := 9.0, colChrome
		if i == picked || a.isHot(id) {
			rad, fill = 11.0, colBlue
		}
		b.Circle(x, y, rad, fill)
		b.Ring(x, y, rad, 1.2, colBlue)
		tc := colText
		if fill == colBlue {
			tc = colOnColor
		}
		b.TextCentered(strconv.Itoa(i+1), render.Rect{X: x - rad, Y: y - rad, W: 2 * rad, H: 2 * rad}, fontSmall, true, tc)
		row, tube := i, n.ID
		a.reg(region{r: rectAround(x, y, 2*rad+4), id: id, cursor: window.CursorHand, tip: fmt.Sprintf("Row %d of the bend table", i+1), tipSide: tipAbove,
			click: func() {
				a.bendRow, a.bendRowOf = row, tube
				a.inspectorOpen, a.propsOpen = true, true
			}})
	}
}
