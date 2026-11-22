package app

import (
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"unsafe"

	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/window"
)

// A picture under the drawing, to trace (a part photographed on paper, a
// scan, a drawing). It belongs to the sketch: saved with it, shown again
// when it is edited, never part of the shape or of anything exported.
//
// The Picture button opens its panel. While the panel is open the board
// moves the picture rather than drawing: drag it, set its scale from two
// points a known distance apart, level it from two points that should be
// level, turn it a quarter turn, fade it. Done (or Esc) goes back to
// drawing.

const (
	imgMove  = iota // a drag on the board moves the picture
	imgScale        // two clicks, then the real distance between them
	imgLevel        // two clicks that should be level
)

// skImageState is the picture's part of the workspace.
type skImageState struct {
	panel bool         // the picture's panel is open
	mode  int          // imgMove, imgScale or imgLevel
	picks [][2]float64 // the points clicked so far (mm), for imgScale and imgLevel
	tex   *render.Texture
	texOf [2]uintptr // the data the texture was made from (its string's address and length)
}

func strID(s string) [2]uintptr {
	return [2]uintptr{uintptr(unsafe.Pointer(unsafe.StringData(s))), uintptr(len(s))}
}

// skImageTexture is the picture's texture, made when the picture changes.
func (a *App) skImageTexture() *render.Texture {
	s := a.sk
	im := s.sk.Image
	if im == nil {
		a.skDropImageTexture()
		return nil
	}
	if s.img.tex != nil && s.img.texOf == strID(im.Data) {
		return s.img.tex
	}
	a.skDropImageTexture()
	px, err := im.Pixels()
	if err != nil {
		return nil
	}
	s.img.tex, s.img.texOf = render.NewTexture(px), strID(im.Data)
	return s.img.tex
}

// skDropImageTexture frees the picture's texture (the workspace closing,
// or another picture).
func (a *App) skDropImageTexture() {
	if s := a.sk; s != nil && s.img.tex != nil {
		s.img.tex.Delete()
		s.img.tex, s.img.texOf = nil, [2]uintptr{}
	}
}

// skDrawImage draws the picture under the drawing (inside the board's clip).
func (a *App) skDrawImage() {
	s := a.sk
	im := s.sk.Image
	if im == nil {
		return
	}
	t := a.skImageTexture()
	if t == nil {
		return
	}
	var q [4][2]float64
	for i, c := range im.Corners() {
		x, y := s.toScreen(c)
		q[i] = [2]float64{x, y}
	}
	a.batch.DrawTexture(t, q, render.Color{1, 1, 1, float32(im.Opacity)})
	if s.img.panel {
		// its outline, while it is being placed
		for i := range q {
			j := (i + 1) % 4
			a.batch.Line(q[i][0], q[i][1], q[j][0], q[j][1], 1, colBlue)
		}
	}
}

// skDrawImagePicks draws the points clicked for a scale or level, over the
// drawing.
func (a *App) skDrawImagePicks() {
	s := a.sk
	if !s.img.panel || len(s.img.picks) == 0 {
		return
	}
	b := a.batch
	var pts [][2]float64
	for _, p := range s.img.picks {
		x, y := s.toScreen(p)
		pts = append(pts, [2]float64{x, y})
	}
	if len(pts) == 1 && a.hot == "sketch-board" {
		pts = append(pts, [2]float64{a.mx, a.my}) // to the pointer
	}
	if len(pts) == 2 {
		b.Line(pts[0][0], pts[0][1], pts[1][0], pts[1][1], 2, colBlue)
	}
	for _, p := range pts[:len(s.img.picks)] {
		b.Ring(p[0], p[1], 6, 2, colBlue)
		b.Line(p[0]-9, p[1], p[0]+9, p[1], 1, colBlue)
		b.Line(p[0], p[1]-9, p[0], p[1]+9, 1, colBlue)
	}
}

// skImageHint is the hint while the picture's panel is open.
func (a *App) skImageHint() string {
	s := a.sk
	switch {
	case s.sk.Image == nil:
		return "Load a picture to trace: a part photographed straight down on paper, or a scan."
	case s.img.mode == imgScale && len(s.img.picks) < 2:
		return "Click two points on the picture a known distance apart (two marks on a ruler, say)."
	case s.img.mode == imgScale:
		return "Type the real distance between the two points, in the panel."
	case s.img.mode == imgLevel:
		return "Click two points on the picture that should be level: the picture turns to level them."
	}
	return "Drag to move the picture. Set scale and Level place it exactly. Done goes back to drawing."
}

// skLoadImage loads a picture from a file, placed in the middle of the
// view, about two thirds of its width across (Set scale makes it the
// right size).
func (a *App) skLoadImage(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	im, err := shapes.NewSketchImage(data)
	if err != nil {
		return err
	}
	s := a.sk
	im.X, im.Y = s.cx, s.cy
	im.Width = 0.66 * s.board.W / s.zoom
	if h := im.Height(); h > 0.8*s.board.H/s.zoom {
		im.Width *= 0.8 * s.board.H / s.zoom / h
	}
	if old := s.sk.Image; old != nil {
		im.Opacity = old.Opacity
	}
	a.skPush()
	s.sk.Image = im
	s.img.mode, s.img.picks = imgMove, nil
	a.w.Redraw()
	return nil
}

// skPickImageFile asks for a picture and loads it.
func (a *App) skPickImageFile() {
	p, ok := a.openFile("Picture to trace", "Pictures (*.png;*.jpg;*.jpeg)", "*.png;*.jpg;*.jpeg", "All files", "*.*")
	if !ok {
		return
	}
	if err := a.skLoadImage(p); err != nil {
		a.showToast(err.Error())
	}
}

// skImagePress is a press on the board while the picture's panel is open.
func (a *App) skImagePress(x, y float64) func(float64, float64, bool) {
	s := a.sk
	im := s.sk.Image
	if im == nil {
		return nil
	}
	at := s.toMM(x, y)
	switch s.img.mode {
	case imgScale, imgLevel:
		if len(s.img.picks) >= 2 {
			s.img.picks = nil // start again
		}
		s.img.picks = append(s.img.picks, at)
		if s.img.mode == imgLevel && len(s.img.picks) == 2 {
			a.skLevelImage(s.img.picks[0], s.img.picks[1])
		}
		a.w.Redraw()
		return nil
	}
	a.skPush()
	last := at
	return func(nx, ny float64, up bool) {
		p := s.toMM(nx, ny)
		im := s.sk.Image
		if im == nil {
			return
		}
		im.X += p[0] - last[0]
		im.Y += p[1] - last[1]
		last = p
		a.w.Redraw()
	}
}

// skScaleImage makes the two picked points d apart, keeping the first
// where it is.
func (a *App) skScaleImage(d float64) error {
	s := a.sk
	im := s.sk.Image
	if im == nil || len(s.img.picks) < 2 {
		return errors.New("click two points on the picture first")
	}
	p, q := s.img.picks[0], s.img.picks[1]
	now := math.Hypot(q[0]-p[0], q[1]-p[1])
	if !(d > 0) || now < 1e-9 {
		return errors.New("the distance must be above 0, between two different points")
	}
	f := d / now
	if w := im.Width * f; !(w > 1e-3 && w <= 2e5) {
		return errors.New("that would make the picture far too small or large")
	}
	a.skPush()
	im.Width *= f
	im.X, im.Y = p[0]+(im.X-p[0])*f, p[1]+(im.Y-p[1])*f
	s.img.mode, s.img.picks = imgMove, nil
	a.w.Redraw()
	return nil
}

// skLevelImage turns the picture so the line from p to q is level, about
// its middle.
func (a *App) skLevelImage(p, q [2]float64) {
	s := a.sk
	im := s.sk.Image
	if im == nil || math.Hypot(q[0]-p[0], q[1]-p[1]) < 1e-9 {
		s.img.picks = nil
		return
	}
	turn := -math.Atan2(q[1]-p[1], q[0]-p[0]) * 180 / math.Pi
	for turn > 90 { // level either way along, by the smaller turn
		turn -= 180
	}
	for turn <= -90 {
		turn += 180
	}
	a.skPush()
	a.skTurnImage(turn, [2]float64{(p[0] + q[0]) / 2, (p[1] + q[1]) / 2})
	s.img.mode, s.img.picks = imgMove, nil
}

// skTurnImage turns the picture by deg degrees anticlockwise about c.
func (a *App) skTurnImage(deg float64, c [2]float64) {
	im := a.sk.sk.Image
	r := deg * math.Pi / 180
	cs, sn := math.Cos(r), math.Sin(r)
	dx, dy := im.X-c[0], im.Y-c[1]
	im.X, im.Y = c[0]+dx*cs-dy*sn, c[1]+dx*sn+dy*cs
	im.Angle = math.Mod(im.Angle+deg, 360)
	a.w.Redraw()
}

// skImagePanelItems are the picture panel's controls.
func (a *App) skImagePanelItems() []panelItem {
	s := a.sk
	im := s.sk.Image
	done := panelItem{kind: panelPrimary, id: "sk-img-done", label: "Done", tip: "Back to drawing (Esc)", click: a.skCloseImagePanel}
	if im == nil {
		return []panelItem{
			{kind: panelButton, id: "sk-img-load", label: "Load picture…", tip: "A PNG or JPEG to trace", click: a.skPickImageFile},
			done,
		}
	}
	mode := func(m int) func() {
		return func() {
			if s.img.mode == m {
				m = imgMove
			}
			s.img.mode, s.img.picks = m, nil
		}
	}
	items := []panelItem{
		{kind: panelButton, id: "sk-img-load", label: "Replace…", tip: "Load another picture in its place", click: a.skPickImageFile},
		{kind: panelToggle, id: "sk-img-scale", label: "Set scale", on: s.img.mode == imgScale,
			tip: "Click two points a known distance apart, then type the distance", click: mode(imgScale)},
	}
	if s.img.mode == imgScale && len(s.img.picks) == 2 {
		p, q := s.img.picks[0], s.img.picks[1]
		now := math.Hypot(q[0]-p[0], q[1]-p[1])
		items = append(items, panelItem{kind: panelCaption, label: "Distance"},
			panelItem{kind: panelCustom, w: 80, draw: func(r render.Rect) {
				a.skField("sk-img-dist", r, now, func(v float64) {
					if err := a.skScaleImage(v); err != nil {
						a.showToast(err.Error())
					}
				})
			}})
	}
	items = append(items,
		panelItem{kind: panelToggle, id: "sk-img-level", label: "Level", on: s.img.mode == imgLevel,
			tip: "Click two points that should be level", click: mode(imgLevel)},
		panelItem{kind: panelButton, id: "sk-img-turn", label: "Turn 90°", tip: "A quarter turn anticlockwise", click: func() {
			a.skPush()
			a.skTurnImage(90, [2]float64{im.X, im.Y})
		}},
		panelItem{kind: panelCaption, label: "Opacity %"},
		panelItem{kind: panelCustom, w: 56, draw: func(r render.Rect) { a.skOpacityField(r) }},
		panelItem{kind: panelButton, id: "sk-img-remove", label: "Remove", tip: "Take the picture away", click: func() {
			a.skPush()
			s.sk.Image = nil
			s.img.mode, s.img.picks = imgMove, nil
		}},
		done)
	return items
}

// skOpacityField is the picture's opacity, typed as a percentage.
func (a *App) skOpacityField(r render.Rect) {
	s := a.sk
	b := a.batch
	const id = "sk-img-opacity"
	b.RoundRect(r, 3, colChrome)
	border := colFieldBorder
	if a.field != nil && a.field.id == id {
		border = colBlue
	}
	b.RoundRectStroke(r, 3, 1, border)
	text := strconv.Itoa(int(math.Round(s.sk.Image.Opacity * 100)))
	a.drawFieldText(id, r, text, fontUI, true, colText)
	a.reg(region{r: r, id: id, cursor: window.CursorIBeam,
		press: func(x, y float64, m window.Mods) func(float64, float64, bool) {
			a.openField(&textField{id: id, text: text, commit: func(t string) bool {
				v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(t), "%")), 64)
				if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
					a.showToast("not a number: " + t)
					return true
				}
				if s.sk.Image != nil {
					a.skPush()
					s.sk.Image.Opacity = math.Max(0.05, math.Min(1, v/100))
				}
				return true
			}})
			return nil
		}})
}

// skOpenImagePanel opens the picture's panel (and, with no picture yet,
// asks for one).
func (a *App) skOpenImagePanel() {
	s := a.sk
	a.skFinish()
	s.img.panel, s.img.mode, s.img.picks = true, imgMove, nil
	if s.sk.Image == nil {
		a.skPickImageFile()
	}
}

func (a *App) skCloseImagePanel() {
	s := a.sk
	s.img.panel, s.img.mode, s.img.picks = false, imgMove, nil
	a.w.Redraw()
}

// skDrawImagePanel draws the picture's panel when it is open.
func (a *App) skDrawImagePanel() bool {
	s := a.sk
	if !s.img.panel {
		return false
	}
	a.drawToolPanel(s.board.Y+14+2*skBtnR+12, a.skImagePanelItems())
	return true
}
