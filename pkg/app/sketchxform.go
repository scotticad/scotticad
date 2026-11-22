package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/window"
)

// Mirror and turn: flip the selected paths, turn them, or add a mirrored
// copy beside them; and the hole switch.

// skLinear is a 2×2 matrix, rows first.
type skLinear [4]float64

func (m skLinear) apply(v [2]float64) [2]float64 {
	return [2]float64{m[0]*v[0] + m[1]*v[1], m[2]*v[0] + m[3]*v[1]}
}

var (
	skFlipX = skLinear{-1, 0, 0, 1} // left for right
	skFlipY = skLinear{1, 0, 0, -1} // up for down
)

// skTurn is a turn by deg degrees anticlockwise.
func skTurn(deg float64) skLinear {
	switch math.Mod(math.Mod(deg, 360)+360, 360) {
	case 90: // (exact, so a square turned stays on the grid)
		return skLinear{0, -1, 1, 0}
	case 180:
		return skLinear{-1, 0, 0, -1}
	case 270:
		return skLinear{0, 1, -1, 0}
	}
	a := deg * math.Pi / 180
	c, s := math.Cos(a), math.Sin(a)
	return skLinear{c, -s, s, c}
}

// skSelBox is the box round the selected paths, curves included.
func (s *sketchSession) skSelBox() (x0, y0, x1, y1 float64, ok bool) {
	x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, i := range s.selPaths() {
		for _, q := range s.sk.Paths[i].Flatten() {
			x0, y0, x1, y1 = math.Min(x0, q.X), math.Min(y0, q.Y), math.Max(x1, q.X), math.Max(y1, q.Y)
		}
	}
	return x0, y0, x1, y1, x0 <= x1
}

// skTransform maps the selected paths by m about pivot: in place, or as
// new copies (selected) when keep, the originals left as they were.
func (a *App) skTransform(m skLinear, pivot [2]float64, keep bool) {
	s := a.sk
	ps := s.selPaths()
	if len(ps) == 0 || s.drawing >= 0 {
		return
	}
	a.skPush()
	var changed []int
	for _, i := range ps {
		p := shapes.CopySketch(&shapes.Sketch{Paths: []shapes.SketchPath{s.sk.Paths[i]}}).Paths[0]
		for j := range p.Points {
			q := &p.Points[j]
			d := m.apply([2]float64{q.P[0] - pivot[0], q.P[1] - pivot[1]})
			q.P = [2]float64{pivot[0] + d[0], pivot[1] + d[1]}
			q.In, q.Out = m.apply(q.In), m.apply(q.Out)
		}
		if keep {
			s.sk.Paths = append(s.sk.Paths, p)
			changed = append(changed, len(s.sk.Paths)-1)
		} else {
			s.sk.Paths[i] = p
			changed = append(changed, i)
		}
	}
	// (on a revolve or coil board, kept left of the center line)
	if a.skSettle(s.pathRefs(changed)) {
		s.sel = map[skRef]bool{}
		for _, i := range changed {
			s.selectPath(i, true)
		}
	}
}

// skFlip flips the selected paths about their middle: left for right
// (x) or up for down.
func (a *App) skFlip(x bool) {
	x0, y0, x1, y1, ok := a.sk.skSelBox()
	if !ok {
		return
	}
	m := skFlipY
	if x {
		m = skFlipX
	}
	a.skTransform(m, [2]float64{(x0 + x1) / 2, (y0 + y1) / 2}, false)
}

// skTurnBy turns the selected paths about their middle, deg degrees
// anticlockwise.
func (a *App) skTurnBy(deg float64) {
	x0, y0, x1, y1, ok := a.sk.skSelBox()
	if !ok {
		return
	}
	a.skTransform(skTurn(deg), [2]float64{(x0 + x1) / 2, (y0 + y1) / 2}, false)
}

// skMirrorCopy adds a mirrored copy of the selected paths beside them, on
// side "left", "right", "up" or "down": mirrored about that edge of their
// box, so half a shape drawn becomes the whole one.
func (a *App) skMirrorCopy(side string) {
	x0, y0, x1, y1, ok := a.sk.skSelBox()
	if !ok {
		return
	}
	switch side {
	case "left":
		a.skTransform(skFlipX, [2]float64{x0, 0}, true)
	case "right":
		a.skTransform(skFlipX, [2]float64{x1, 0}, true)
	case "up":
		a.skTransform(skFlipY, [2]float64{0, y1}, true)
	case "down":
		a.skTransform(skFlipY, [2]float64{0, y0}, true)
	}
}

// skSetHole makes the selected paths holes, or not.
func (a *App) skSetHole(on bool) {
	a.skSetPaths(func(p *shapes.SketchPath) { p.Hole = on })
}

// skAllHoles is whether the selected paths are all holes.
func (s *sketchSession) skAllHoles() bool {
	ps := s.selPaths()
	for _, i := range ps {
		if !s.sk.Paths[i].Hole {
			return false
		}
	}
	return len(ps) > 0
}

// skDrawXformPanel draws Mirror and turn's options.
func (a *App) skDrawXformPanel(y float64) {
	items := []panelItem{
		{kind: panelButton, id: "sk-flip-x", label: "Flip left-right", click: func() { a.skFlip(true) }},
		{kind: panelButton, id: "sk-flip-y", label: "Flip up-down", click: func() { a.skFlip(false) }},
		{kind: panelCaption, label: "  Turn"},
		{kind: panelButton, id: "sk-turn-left", label: "90° left", tip: "Turn a quarter turn anticlockwise", click: func() { a.skTurnBy(90) }},
		{kind: panelButton, id: "sk-turn-right", label: "90° right", tip: "Turn a quarter turn clockwise", click: func() { a.skTurnBy(-90) }},
		{kind: panelCustom, id: "sk-turn-by", w: 64, draw: a.skDrawTurnField},
		{kind: panelCaption, label: "°   Mirror copy"},
	}
	for _, side := range []string{"Left", "Right", "Up", "Down"} {
		side := strings.ToLower(side)
		items = append(items, panelItem{kind: panelButton, id: "sk-mirror-" + side, label: strings.ToUpper(side[:1]) + side[1:],
			tip: fmt.Sprintf("Add a mirror image of the selection on its %s side", side), click: func() { a.skMirrorCopy(side) }})
	}
	a.drawToolPanel(y, items)
}

// skDrawTurnField is Turn by's angle: typed, the selection turns by it.
func (a *App) skDrawTurnField(r render.Rect) {
	s := a.sk
	b := a.batch
	const id = "sk-turn-by"
	b.RoundRect(r, 3, colChrome)
	border := colFieldBorder
	if a.field != nil && a.field.id == id {
		border = colBlue
	}
	b.RoundRectStroke(r, 3, 1, border)
	text := strconv.FormatFloat(math.Round(s.turnBy*100)/100, 'f', -1, 64)
	a.drawFieldText(id, r, text, fontUI, true, colText)
	a.reg(region{r: r, id: id, cursor: window.CursorIBeam, tip: "Type an angle to turn by (anticlockwise; minus for clockwise)", tipSide: tipAbove,
		press: func(x, y float64, m window.Mods) func(float64, float64, bool) {
			a.openField(&textField{id: id, text: text, commit: func(t string) bool {
				v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(t), "°")), 64)
				if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
					a.showToast("not a number: " + t)
					return true
				}
				s.turnBy = v
				a.skTurnBy(v)
				return true
			}})
			return nil
		}})
}
