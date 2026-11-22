package app

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/gl"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/window"
)

// The revolve and coil workspaces show the shape the drawing makes, live, in
// a card at the board's top right: a shaded 3D view with reset view, zoom in
// and zoom out down its left side (an extruded sketch has none). Drag in
// it to turn the view. Half a profile is hard to picture whole, and a coil
// harder still.
//
// The mesh is built off the main thread from a copy of the drawing, so a
// drag on the board never waits for it; the card shows the last one built
// until the newest is ready.

type skPreview struct {
	key      string     // what mesh was built from
	building string     // what is being built ("" when nothing is)
	mesh     *geom.Mesh // nil: nothing to spin yet
	yaw      float64
	pitch    float64
	zoom     float64
	viewSet  bool
}

const skPreviewW, skPreviewH = 280.0, 200.0

func (p *skPreview) resetView() {
	p.yaw, p.pitch, p.zoom, p.viewSet = -math.Pi/2-0.5, 0.45, 1, true
}

// skPreviewRect is the card's place, and whether it shows: only for a
// revolve or coil, and only on a board with room for it.
func (a *App) skPreviewRect() (render.Rect, bool) {
	s := a.sk
	if s == nil || !shapes.Spins(s.kind) {
		return render.Rect{}, false
	}
	// in the blanked third right of the center line, over nothing drawn
	r := s.board
	w := math.Min(skPreviewW, (1-skAxisAt)*r.W-28)
	h := w * skPreviewH / skPreviewW
	if w < 170 || r.H < 2.5*h {
		return render.Rect{}, false
	}
	return render.Rect{X: math.Round(r.X + r.W - w - 14), Y: math.Round(r.Y + 14), W: math.Round(w), H: math.Round(h)}, true
}

// skPreviewValues are the settings the shape would be built with: the
// shape's own when one is being changed, a new one's otherwise; and its
// color.
func (a *App) skPreviewValues() (map[string]float64, string) {
	s := a.sk
	num := map[string]float64{}
	color := "#9fc7e0"
	if k, ok := shapes.Lookup(s.kind); ok {
		for key, v := range k.Defaults().Num {
			num[key] = v
		}
		color = k.Color
	}
	if s.nodeID != 0 {
		if n := a.ed.Doc.FindAny(s.nodeID); n != nil {
			for key, v := range n.Params {
				num[key] = v
			}
			if n.Color != "" {
				color = n.Color
			}
		}
	}
	return num, color
}

// skUpdatePreview starts building the preview of the drawing as it now is,
// unless that is the one shown or being built. One is built at a time; when
// it is done the next frame starts the newest.
func (a *App) skUpdatePreview() {
	s := a.sk
	p := &s.preview
	num, color := a.skPreviewValues()
	keys := make([]string, 0, len(num))
	for k := range num {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s|%s|%s|", s.kind, shapes.SketchKey(s.sk), color)
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s=%g,", k, num[k])
	}
	key := sb.String()
	if key == p.key || p.building != "" {
		return
	}
	p.building = key
	sk, kind := shapes.CopySketch(s.sk), s.kind
	go func() {
		var m *geom.Mesh
		defer func() {
			if recover() != nil {
				m = nil // shown as nothing to spin; the shape itself reports its own trouble
			}
			a.post(func() {
				if a.sk != s {
					return // the workspace closed, or another opened
				}
				p.building, p.key, p.mesh = "", key, m
				a.w.Redraw()
			})
		}()
		v := shapes.Values{Num: num, Sketch: sk}
		if !shapes.Buildable(kind, sk) {
			return
		}
		b := shapes.FrameBounds(kind, v)
		if b.IsEmpty() {
			return
		}
		m = shapes.Build(kind, b.Size(), v).Colored(geom.ParseColor(color))
	}()
}

// skDrawPreview draws the preview card. Called with no clip pushed: it
// flushes the 2D drawing so far, draws the mesh into the card with its own
// viewport, and leaves the viewport as the frame's.
func (a *App) skDrawPreview() {
	r, ok := a.skPreviewRect()
	if !ok {
		return
	}
	s := a.sk
	p := &s.preview
	if !p.viewSet {
		p.resetView()
	}
	a.skUpdatePreview()
	b := a.batch
	b.RoundRect(render.Rect{X: r.X - 1, Y: r.Y - 1, W: r.W + 2, H: r.H + 2}, 9, colFieldBorder)
	b.RoundRect(r, 8, colChrome)
	// drag to turn the view, the wheel to zoom it
	a.reg(region{r: r, id: "sk-preview", cursor: window.CursorHand, tip: "Drag to turn the view",
		press: func(x, y float64, m window.Mods) func(float64, float64, bool) {
			lx, ly := x, y
			return func(x, y float64, up bool) {
				p.yaw -= (x - lx) * 0.01
				p.pitch = math.Max(-1.5, math.Min(1.5, p.pitch+(y-ly)*0.01))
				lx, ly = x, y
				a.w.Redraw()
			}
		},
		wheel: func(d float64) bool {
			p.zoom = math.Max(0.3, math.Min(8, p.zoom*math.Pow(1/0.87, d*a.zoomSpeed)))
			a.w.Redraw()
			return true
		}})
	bx := r.X + 10 + skBtnR
	for i, t := range []struct {
		id, tip string
		icon    int
		click   func()
	}{
		{"sk-preview-reset", "Reset the preview's view", icHome, p.resetView},
		{"sk-preview-in", "Zoom the preview in", icZoomIn, func() { p.zoom = math.Min(8, p.zoom*1.25) }},
		{"sk-preview-out", "Zoom the preview out", icZoomOut, func() { p.zoom = math.Max(0.3, p.zoom*0.8) }},
	} {
		a.skRound(t.id, t.tip, "", t.icon, bx, r.Y+10+skBtnR+float64(i)*(2*skBtnR+8), false, true, tipBelow, t.click)
	}
	view := render.Rect{X: bx + skBtnR + 8, Y: r.Y + 6, W: r.X + r.W - 6 - (bx + skBtnR + 8), H: r.H - 12}
	if p.mesh == nil || len(p.mesh.Tris) == 0 {
		msg := "Draw left of the center line"
		if p.building != "" {
			msg = "…"
		}
		b.TextCentered(msg, view, fontSmall, false, colTextDim)
		return
	}
	b.Flush()
	d := a.w.DPI
	vx, vy := int32(math.Round(view.X*d)), int32(math.Round((a.H-view.Y-view.H)*d))
	vw, vh := int32(math.Round(view.W*d)), int32(math.Round(view.H*d))
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(vx, vy, vw, vh)
	gl.Viewport(vx, vy, vw, vh)
	gl.Clear(gl.DEPTH_BUFFER_BIT)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthMask(true)
	bb := p.mesh.Bounds()
	cam := render.Camera{Target: bb.Center(), Yaw: p.yaw, Pitch: p.pitch, Fov: 30 * math.Pi / 180}
	cam.Dist = math.Max(bb.Size().Len()/2, 1e-3) / math.Sin(cam.Fov/2) * 1.05 / p.zoom
	a.scene.Begin(cam.VP(view.W/view.H), cam.Eye())
	a.scene.DrawMesh(p.mesh, render.Style{Edges: true})
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.SCISSOR_TEST)
	gl.Viewport(0, 0, int32(a.w.W), int32(a.w.H))
}
