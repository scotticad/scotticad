package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/window"
)

// The sketch workspace: a flat drawing board that takes over the window
// while a sketch shape is drawn or changed. Its left bar has two modes:
// Pen, whose tools draw new paths (straight lines, Bezier curves whose
// handles are dragged out as each point is placed, and smooth curves that
// flow through the points), and Edit, whose tools change them (select and
// move points and handles, add and remove points). Outside both, a click
// picks whole paths to move, resize or delete. The bar at the foot opens or
// closes the selected paths, sets their stroke (a line of a width instead
// of the area they enclose) and their points' kind. Lengths show as they
// are drawn and can be typed. Finish sketch builds the shape; Cancel leaves
// it as it was. The workspace keeps its own undo history, and finishing is
// one step in the design's.

// sketch tools
const (
	skSelect = iota // Edit: points and handles
	skLine          // Pen
	skBezier        // Pen
	skSmooth        // Pen
	skRefine        // Edit: add and remove points
	skShape         // neither mode: whole paths
)

var skToolInfo = []struct {
	id, tip, key, label string
	icon                int
}{
	{"sk-select", "Select and move", "V", "Select", icSkSelect},
	{"sk-line", "Create line", "1", "Line", icSkLine},
	{"sk-bezier", "Create Bézier curve", "2", "Bézier", icSkBezier},
	{"sk-smooth", "Create smooth curve", "3", "Smooth", icSkSmooth},
	{"sk-refine", "Add & remove points", "4", "Points", icSkRefine},
}

// skPenTool and skEditTool say which mode a tool belongs to.
func skPenTool(t int) bool  { return t == skLine || t == skBezier || t == skSmooth }
func skEditTool(t int) bool { return t == skSelect || t == skRefine }

// skRef names a point: its path and its place in it.
type skRef struct{ path, pt int }

type sketchSession struct {
	kind   string    // "sketch", "revolve" or "coil"
	nodeID int       // the shape being changed; 0 for a new one
	xf     geom.Mat4 // where a new one goes
	hole   bool
	sk     *shapes.Sketch // the drawing, the session's own copy
	start  string         // its key when opened: unchanged, Done changes nothing
	// startSk is the drawing as opened, for its picture (which the key
	// leaves out: a picture moved is a change too)
	startSk *shapes.Sketch
	undo    []skSnapshot
	redo    []skSnapshot

	cx, cy, zoom float64 // the view: the mm at the board's centre, pixels per mm
	board        render.Rect
	viewSet      bool

	tool              int
	lastPen, lastEdit int // the tool each mode opens with

	preview skPreview    // a revolve's or coil's live 3D view (sketchpreview.go)
	img     skImageState // the picture to trace (sketchimage.go)
	sel     map[skRef]bool
	drawing int // the path being drawn, -1 for none

	// the drawing as it was before the path being drawn was started (or
	// carried on), for Clear
	drawBase *shapes.Sketch

	dims       bool    // lengths and sizes shown
	strokeOpen bool    // the stroke settings are showing
	xformOpen  bool    // Mirror and turn is showing
	turnBy     float64 // the angle Turn by turns, degrees anticlockwise
	// a curve handle being dragged (its point, which one), for its
	// offsets' labels
	dragHandle *skRef
	dragWhich  int

	// the pointer, in mm, snapped as a new point would be
	at [2]float64
	// a box being dragged to select points, in screen pixels
	marquee *[4]float64

	// the drawing's filled outline, triangulated (mm), and the key it is for
	fill    [][3]shapes.Pt
	fillKey string
}

func newSketchSession(kind string, sk *shapes.Sketch) *sketchSession {
	if sk == nil {
		sk = &shapes.Sketch{}
	}
	sk = shapes.CopySketch(sk)
	return &sketchSession{kind: kind, sk: sk, start: shapes.SketchKey(sk), startSk: shapes.CopySketch(sk), sel: map[skRef]bool{}, drawing: -1, zoom: 6,
		tool: skShape, lastPen: skLine, lastEdit: skSelect, dims: true, turnBy: 15}
}

// editSketch opens the workspace on sketch shape id.
func (a *App) editSketch(id int) {
	n := a.ed.Doc.FindAny(id)
	if n == nil || !shapes.IsSketch(n.Kind) {
		return
	}
	if a.ed.Locked(id) { // (or the group it is in: Finish couldn't save it)
		a.showToast("That shape is locked")
		return
	}
	a.commitField()
	a.setMode(modeNone)
	a.place = nil
	s := newSketchSession(n.Kind, n.Sketch)
	s.nodeID = id
	a.sk = s
	a.w.Redraw()
}

// newSketch opens the workspace on an empty drawing for a new shape of
// kind, placed by xf when it is done.
func (a *App) newSketch(kind string, xf geom.Mat4, hole bool) {
	a.commitField()
	a.setMode(modeNone)
	s := newSketchSession(kind, nil)
	s.xf, s.hole = xf, hole
	s.tool = skLine
	a.sk = s
	a.w.Redraw()
}

// closeSketch leaves the workspace without changing the design.
func (a *App) closeSketch() {
	a.skDropImageTexture()
	a.sk = nil
	a.field, a.pendingOpen = nil, nil
	// a press under way on the board (an API call or a script can close
	// the workspace mid-drag) would carry on into a workspace that's gone
	a.pressDrag, a.pressed, a.pressRegion = nil, "", nil
	a.w.Redraw()
}

// sketchDone builds the shape from the drawing and leaves the workspace.
func (a *App) sketchDone() {
	s := a.sk
	if s == nil {
		return
	}
	if a.field != nil && !a.commitField() {
		return
	}
	a.skFinish()
	sk := s.cleaned()
	if s.nodeID != 0 && shapes.SketchKey(sk) == s.start && shapes.SameImage(sk, s.startSk) {
		a.closeSketch()
		return
	}
	gone := false
	if s.nodeID != 0 && a.ed.Doc.FindAny(s.nodeID) == nil {
		// the shape was deleted or undone while the workspace was open (an
		// API call or a script): the drawing becomes a new shape rather
		// than a Finish that can never succeed
		s.nodeID, s.xf, gone = 0, geom.Identity(), true
	}
	var err error
	if s.nodeID != 0 {
		err = a.ed.SetSketch(s.nodeID, sk)
	} else {
		_, err = a.ed.AddSketchXf(s.kind, sk, s.xf, s.hole)
	}
	if err != nil {
		a.showToast(err.Error())
		return
	}
	a.closeSketch()
	a.inspectorOpen, a.propsOpen = true, true
	if gone {
		a.showToast("The shape being edited was removed; the drawing was added as a new shape")
	}
}

// changed is whether the drawing differs from what the workspace opened
// with.
func (s *sketchSession) changed() bool {
	return shapes.SketchKey(s.sk) != s.start || !shapes.SameImage(s.sk, s.startSk)
}

// pruneSel drops selected points that are no longer in the drawing.
func (s *sketchSession) pruneSel() {
	for r := range s.sel {
		if r.path >= len(s.sk.Paths) || r.pt >= len(s.sk.Paths[r.path].Points) {
			delete(s.sel, r)
		}
	}
	if s.drawing >= len(s.sk.Paths) {
		s.drawing = -1
	}
}

// cleaned is the drawing without paths that have no use (a single point).
func (s *sketchSession) cleaned() *shapes.Sketch {
	out := &shapes.Sketch{Image: s.sk.Image}
	for _, p := range s.sk.Paths {
		if len(p.Points) >= 2 {
			out.Paths = append(out.Paths, p)
		}
	}
	return shapes.CopySketch(out)
}

// --- history -------------------------------------------------------------------

// skSnapshot is the workspace's state at one undo step: the drawing, and
// the path being drawn (so undoing a point carries on drawing from the
// one before).
type skSnapshot struct {
	sk       *shapes.Sketch
	drawing  int
	drawBase *shapes.Sketch
}

func (s *sketchSession) snapshot() skSnapshot {
	return skSnapshot{sk: shapes.CopySketch(s.sk), drawing: s.drawing, drawBase: s.drawBase}
}

func (s *sketchSession) restore(n skSnapshot) {
	s.sk, s.drawing, s.drawBase = n.sk, n.drawing, n.drawBase
	s.sel = map[skRef]bool{}
	if s.drawing >= len(s.sk.Paths) {
		s.drawing = -1
	}
	if s.drawing >= 0 {
		s.sel[skRef{s.drawing, len(s.sk.Paths[s.drawing].Points) - 1}] = true
		// back to drawing it, so with a tool that draws (closing a path,
		// or Esc, may have left for the Edit tools or the Shape panel)
		if !skPenTool(s.tool) {
			s.tool = s.lastPen
		}
	}
}

// skPush keeps the workspace as it is, for undo, before a change.
func (a *App) skPush() {
	s := a.sk
	s.undo = append(s.undo, s.snapshot())
	if len(s.undo) > 200 {
		s.undo = s.undo[1:]
	}
	s.redo = nil
}

func (a *App) skUndo() {
	s := a.sk
	if len(s.undo) == 0 {
		return
	}
	s.redo = append(s.redo, s.snapshot())
	s.restore(s.undo[len(s.undo)-1])
	s.undo = s.undo[:len(s.undo)-1]
}

func (a *App) skRedo() {
	s := a.sk
	if len(s.redo) == 0 {
		return
	}
	s.undo = append(s.undo, s.snapshot())
	s.restore(s.redo[len(s.redo)-1])
	s.redo = s.redo[:len(s.redo)-1]
}

// skSettle finishes a change made by typing or pasting (after its skPush):
// on a revolve or coil board the points it moved (refs) shift left
// together until none is right of the center line, and a change that
// takes the drawing out of range is taken back, with a word why.
func (a *App) skSettle(refs []skRef) bool {
	s := a.sk
	if shapes.Spins(s.kind) {
		most := 0.0
		for _, r := range refs {
			most = math.Max(most, s.sk.Paths[r.path].Points[r.pt].P[0])
		}
		for _, r := range refs {
			s.sk.Paths[r.path].Points[r.pt].P[0] -= most
		}
	}
	if err := shapes.CheckSketch(s.sk); err != nil {
		sel := s.sel // the selection stays: nothing happened
		s.restore(s.undo[len(s.undo)-1])
		s.undo = s.undo[:len(s.undo)-1]
		s.sel = sel
		s.pruneSel()
		a.showToast(err.Error())
		return false
	}
	return true
}

// pathRefs is every point of the paths at is.
func (s *sketchSession) pathRefs(is []int) []skRef {
	var out []skRef
	for _, i := range is {
		for j := range s.sk.Paths[i].Points {
			out = append(out, skRef{i, j})
		}
	}
	return out
}

// --- view ----------------------------------------------------------------------

func (s *sketchSession) toScreen(p [2]float64) (float64, float64) {
	return s.board.CX() + (p[0]-s.cx)*s.zoom, s.board.CY() - (p[1]-s.cy)*s.zoom
}

func (s *sketchSession) ptScreen(p shapes.Pt) (float64, float64) {
	return s.toScreen([2]float64{p.X, p.Y})
}

func (s *sketchSession) toMM(x, y float64) [2]float64 {
	return [2]float64{s.cx + (x-s.board.CX())/s.zoom, s.cy - (y-s.board.CY())/s.zoom}
}

// skHomeZoom is the board's home scale, in pixels a millimeter (a 10 mm
// square 273 px across, as the board opens and after Home), where the 1 mm
// squares are big enough to draw on.
const skHomeZoom = 27.3

// skHome is the home view: the home scale, with the origin in the middle
// (a revolve's or coil's at its center line, two thirds across, with the
// plate toward the bottom, leaving room for the profile).
func (a *App) skHome() {
	s := a.sk
	s.cx, s.cy, s.zoom = 0, 0, skHomeZoom
	if shapes.Spins(s.kind) {
		s.cy = (s.board.H/2 - 120) / skHomeZoom
		s.pinAxis()
	}
	s.viewSet = true
}

// skOpenView is the view a workspace opens with: the home view, unless the
// drawing doesn't fit in it, then the whole drawing.
func (a *App) skOpenView() {
	a.skHome()
	s := a.sk
	for _, p := range s.sk.Paths {
		for _, q := range p.Flatten() {
			x, y := s.toScreen([2]float64{q.X, q.Y})
			if !s.board.Inset(40).Contains(x, y) {
				a.fitSketch()
				return
			}
		}
	}
}

// fitSketch shows the whole drawing (and the origin), or the home view when
// there is nothing yet.
func (a *App) fitSketch() {
	s := a.sk
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	grow := func(x, y float64) {
		x0, y0, x1, y1 = math.Min(x0, x), math.Min(y0, y), math.Max(x1, x), math.Max(y1, y)
	}
	for _, p := range s.sk.Paths {
		for _, q := range p.Flatten() {
			grow(q.X, q.Y)
		}
	}
	if math.IsInf(x0, 1) {
		a.skHome()
		return
	}
	grow(0, 0)
	w, h := math.Max(x1-x0, 10), math.Max(y1-y0, 10)
	s.cx, s.cy = (x0+x1)/2, (y0+y1)/2
	s.zoom = math.Min((s.board.W-120)/w, (s.board.H-160)/h)
	if shapes.Spins(s.kind) {
		// what is spun, from the drawing's left to the center line, in the
		// board's drawing side
		s.zoom = math.Min((skAxisAt*s.board.W-80)/math.Max(-x0, 10), (s.board.H-160)/h)
	}
	s.zoom = math.Max(0.05, math.Min(400, s.zoom))
	s.pinAxis()
	s.viewSet = true
}

func (a *App) skZoomAt(x, y, f float64) {
	s := a.sk
	before := s.toMM(x, y)
	s.zoom = math.Max(0.05, math.Min(400, s.zoom*f))
	after := s.toMM(x, y)
	s.cx += before[0] - after[0]
	s.cy += before[1] - after[1]
	s.pinAxis()
	a.w.Redraw()
}

// skAxisAt is how far across the board a revolve's or coil's center line
// stays: the drawing side two thirds, the right third blanked (it isn't
// spun) and holding the preview.
const skAxisAt = 2.0 / 3

// pinAxis keeps a revolve's or coil's center line at skAxisAt across the
// board, whatever the view was set to: the view pans up and down, and
// zooms about the line.
func (s *sketchSession) pinAxis() {
	if shapes.Spins(s.kind) && s.zoom > 0 {
		s.cx = -(skAxisAt - 0.5) * s.board.W / s.zoom
	}
}

// skPan is the drag that slides the board.
func (a *App) skPan(x, y float64) func(float64, float64, bool) {
	s := a.sk
	cx, cy := s.cx, s.cy
	return func(nx, ny float64, up bool) {
		s.cx = cx - (nx-x)/s.zoom
		s.cy = cy + (ny-y)/s.zoom
		s.pinAxis()
		a.w.Redraw()
	}
}

// --- snapping ------------------------------------------------------------------

const skPick = 8.0 // pixels: how near the pointer must be to pick a point

// skSnap is where a point put down at screen (x, y) goes: on an existing
// point within reach (other than those in skip), else on the grid (not with
// Alt), or with Shift along a 15° direction from from.
func (a *App) skSnap(x, y float64, skip map[skRef]bool, from *[2]float64) [2]float64 {
	s := a.sk
	if r, ok := s.pointAt(x, y, skip); ok {
		return s.sk.Paths[r.path].Points[r.pt].P
	}
	p := s.toMM(x, y)
	if from != nil && a.mods&window.Shift != 0 {
		dx, dy := p[0]-from[0], p[1]-from[1]
		l := math.Hypot(dx, dy)
		ang := math.Round(math.Atan2(dy, dx)/(math.Pi/12)) * (math.Pi / 12)
		if g := a.snap(); g > 0 && a.mods&window.Alt == 0 {
			l = math.Round(l/g) * g
		}
		return s.keepLeft([2]float64{from[0] + l*math.Cos(ang), from[1] + l*math.Sin(ang)})
	}
	if g := a.snap(); g > 0 && a.mods&window.Alt == 0 {
		p = [2]float64{math.Round(p[0]/g) * g, math.Round(p[1]/g) * g}
	}
	return s.keepLeft(p)
}

// keepLeft keeps a point of a revolve or coil off the blanked-out side of
// the center line: nothing right of it is spun, so nothing is drawn there.
func (s *sketchSession) keepLeft(p [2]float64) [2]float64 {
	if shapes.Spins(s.kind) && p[0] > 0 {
		p[0] = 0
	}
	return p
}

// --- hit tests -----------------------------------------------------------------

// pointAt is the point under screen (x, y): the nearest within reach, the
// path being drawn's first.
func (s *sketchSession) pointAt(x, y float64, skip map[skRef]bool) (skRef, bool) {
	best, bd := skRef{}, skPick
	found := false
	for i, p := range s.sk.Paths {
		for j, q := range p.Points {
			r := skRef{i, j}
			if skip[r] {
				continue
			}
			sx, sy := s.toScreen(q.P)
			d := math.Hypot(sx-x, sy-y)
			if i == s.drawing && j == 0 {
				d -= 2 // closing a path wins over a point beneath it
			}
			if d <= bd {
				best, bd, found = r, d, true
			}
		}
	}
	return best, found
}

// handleAt is the curve handle under screen (x, y), of a selected point:
// which is 0 for the one in, 1 for the one out.
func (s *sketchSession) handleAt(x, y float64) (skRef, int, bool) {
	for r := range s.sel {
		if r.path >= len(s.sk.Paths) || r.pt >= len(s.sk.Paths[r.path].Points) {
			continue
		}
		p := s.sk.Paths[r.path]
		q := p.Points[r.pt]
		if q.Kind == shapes.PointAuto {
			continue
		}
		for which, h := range [][2]float64{q.In, q.Out} {
			if h == ([2]float64{}) {
				continue
			}
			sx, sy := s.toScreen([2]float64{q.P[0] + h[0], q.P[1] + h[1]})
			if math.Hypot(sx-x, sy-y) <= skPick {
				return r, which, true
			}
		}
	}
	return skRef{}, 0, false
}

// segmentAt is the stretch of path under screen (x, y): its path, the
// point it starts at, and the curve's parameter there.
func (s *sketchSession) segmentAt(x, y float64) (path, seg int, t float64, ok bool) {
	bd := skPick
	for i, p := range s.sk.Paths {
		n := len(p.Points)
		segs := n - 1
		if p.Closed {
			segs = n
		}
		for j := 0; j < segs; j++ {
			poly := p.Segment(j)
			for k := 0; k+1 < len(poly); k++ {
				ax, ay := s.ptScreen(poly[k])
				bx, by := s.ptScreen(poly[k+1])
				dx, dy := bx-ax, by-ay
				u := 0.0
				if l := dx*dx + dy*dy; l > 0 {
					u = math.Max(0, math.Min(1, ((x-ax)*dx+(y-ay)*dy)/l))
				}
				if d := math.Hypot(ax+u*dx-x, ay+u*dy-y); d <= bd {
					bd, path, seg, ok = d, i, j, true
					t = (float64(k) + u) / float64(len(poly)-1)
				}
			}
		}
	}
	return
}

// areaAt is the closed path whose inside is under screen (x, y), the
// smallest if several.
func (s *sketchSession) areaAt(x, y float64) (int, bool) {
	q := s.toMM(x, y)
	pt := shapes.Pt{X: q[0], Y: q[1]}
	best, ba := -1, math.Inf(1)
	for i, p := range s.sk.Paths {
		if !p.Closed {
			continue
		}
		c := p.Flatten()
		if len(c) >= 3 && shapes.PointInPolygon(pt, c) {
			if a := math.Abs(shapes.SignedArea(c)); a < ba {
				best, ba = i, a
			}
		}
	}
	return best, best >= 0
}

// --- selection -----------------------------------------------------------------

// selPaths is the paths with a selected point, in order.
func (s *sketchSession) selPaths() []int {
	var out []int
	for i := range s.sk.Paths {
		for r := range s.sel {
			if r.path == i {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

func (s *sketchSession) selectPath(i int, add bool) {
	if !add {
		s.sel = map[skRef]bool{}
	}
	for j := range s.sk.Paths[i].Points {
		s.sel[skRef{i, j}] = true
	}
}

func (s *sketchSession) selectAll() {
	s.sel = map[skRef]bool{}
	for i := range s.sk.Paths {
		s.selectPath(i, true)
	}
}

// --- drawing -------------------------------------------------------------------

// skNewPoint is a point the current tool puts down at p.
func (s *sketchSession) skNewPoint(p [2]float64) shapes.SketchPoint {
	q := shapes.SketchPoint{P: p}
	if s.tool == skSmooth {
		q.Kind = shapes.PointAuto
	}
	return q
}

// skFinish ends the path being drawn: a single point is dropped.
func (a *App) skFinish() {
	s := a.sk
	if s == nil || s.drawing < 0 {
		return
	}
	i := s.drawing
	s.drawing = -1
	if i < len(s.sk.Paths) && len(s.sk.Paths[i].Points) < 2 {
		s.sk.Paths = append(s.sk.Paths[:i], s.sk.Paths[i+1:]...)
		s.sel = map[skRef]bool{}
		return
	}
	s.closeIfEndsMeet(i) // (a line ended on its own start: an outline)
}

// skDrawPress is a press on the board with a drawing tool: it starts a
// path, adds a point, closes the path (on its first point) or ends it (on
// its last). With the Bezier tool, dragging before letting go pulls out the
// new point's handles.
func (a *App) skDrawPress(x, y float64) func(float64, float64, bool) {
	s := a.sk
	if s.drawing >= 0 {
		path := &s.sk.Paths[s.drawing]
		from := &path.Points[len(path.Points)-1].P
		r, onPoint := s.pointAt(x, y, nil)
		if onPoint && r.path == s.drawing {
			switch {
			case r.pt == 0 && len(path.Points) >= 2:
				// closed: on to the Edit tools to change it
				a.skPush()
				path.Closed = true
				i := s.drawing
				s.drawing = -1
				s.selectPath(i, false)
				drag := a.skHandleDrag(skRef{i, 0}, x, y)
				s.lastPen = s.tool
				s.tool = skSelect
				return drag
			case r.pt == len(path.Points)-1:
				a.skFinish()
				return nil
			}
		}
		// (on an end of another line: a point there, the two lines staying
		// apart: a box of four lines stays four lines, their ends
		// overlapping)
		a.skPush()
		p := a.skSnap(x, y, nil, from)
		path.Points = append(path.Points, s.skNewPoint(p))
		r = skRef{s.drawing, len(path.Points) - 1}
		s.sel = map[skRef]bool{r: true}
		return a.skHandleDrag(r, x, y)
	}
	// a new path, even from the end of an open one (it doesn't carry
	// that one on)
	a.skPush()
	s.drawBase = shapes.CopySketch(s.sk)
	p := a.skSnap(x, y, nil, nil)
	s.sk.Paths = append(s.sk.Paths, shapes.SketchPath{Points: []shapes.SketchPoint{s.skNewPoint(p)}})
	s.drawing = len(s.sk.Paths) - 1
	r := skRef{s.drawing, 0}
	s.sel = map[skRef]bool{r: true}
	return a.skHandleDrag(r, x, y)
}

// skHandleDrag, for the Bezier tool, pulls the handles of the point just
// put down out as the pointer is dragged (mirrored: a smooth point).
func (a *App) skHandleDrag(r skRef, x, y float64) func(float64, float64, bool) {
	s := a.sk
	if s.tool != skBezier {
		return nil
	}
	return func(nx, ny float64, up bool) {
		if up {
			s.dragHandle = nil
		}
		if math.Hypot(nx-x, ny-y) < 3 {
			return
		}
		q := &s.sk.Paths[r.path].Points[r.pt]
		m := s.toMM(nx, ny)
		d := [2]float64{m[0] - q.P[0], m[1] - q.P[1]}
		q.Kind = shapes.PointSmooth
		q.Out = d
		q.In = [2]float64{-d[0], -d[1]}
		if !up {
			s.dragHandle, s.dragWhich = &r, 1
		}
		a.w.Redraw()
	}
}

// --- the select tool -------------------------------------------------------------

func (a *App) skSelectPress(x, y float64, m window.Mods) func(float64, float64, bool) {
	s := a.sk
	shift := m&window.Shift != 0
	if r, which, ok := s.handleAt(x, y); ok {
		return a.skDragHandle(r, which, x, y)
	}
	var anchor skRef
	if r, ok := s.pointAt(x, y, nil); ok {
		anchor = r
		if shift {
			if s.sel[r] {
				delete(s.sel, r)
				return nil
			}
			s.sel[r] = true
		} else if !s.sel[r] {
			s.sel = map[skRef]bool{r: true}
		}
	} else if path, seg, _, ok := s.segmentAt(x, y); ok {
		// the stretch between two points: both its ends
		anchor = skRef{path, seg}
		other := skRef{path, (seg + 1) % len(s.sk.Paths[path].Points)}
		if !(s.sel[anchor] && s.sel[other] && !shift) {
			if !shift {
				s.sel = map[skRef]bool{}
			}
			s.sel[anchor], s.sel[other] = true, true
		} else {
			// already selected: a drag moves the whole selection, a click
			// picks just this stretch
			drag := a.skDragPoints(anchor, x, y)
			return func(nx, ny float64, up bool) {
				drag(nx, ny, up)
				if up && math.Hypot(nx-x, ny-y) < 3 {
					s.sel = map[skRef]bool{anchor: true, other: true}
				}
			}
		}
	} else if path, ok := s.areaAt(x, y); ok {
		anchor = skRef{path, 0}
		if !(s.sel[anchor] && !shift) {
			s.selectPath(path, shift)
		}
	} else {
		// a box to select points in
		return a.skMarquee(x, y, shift, func(in func(sx, sy float64) bool) {
			for i, p := range s.sk.Paths {
				for j, q := range p.Points {
					if in(s.toScreen(q.P)) {
						s.sel[skRef{i, j}] = true
					}
				}
			}
		})
	}
	return a.skDragPoints(anchor, x, y)
}

// skShapePress is a press outside both modes: it picks whole paths (Shift
// adds or takes away) and drags them, or on empty space drags a box that
// picks the paths it touches.
func (a *App) skShapePress(x, y float64, m window.Mods) func(float64, float64, bool) {
	s := a.sk
	shift := m&window.Shift != 0
	path := -1
	if r, ok := s.pointAt(x, y, nil); ok {
		path = r.path
	} else if p, _, _, ok := s.segmentAt(x, y); ok {
		path = p
	} else if p, ok := s.areaAt(x, y); ok {
		path = p
	}
	if path >= 0 {
		anchor := skRef{path, 0}
		// the whole path selected (not just some of its points, left from
		// the Edit tools: then only those moved, and the shape skewed)
		whole := true
		for j := range s.sk.Paths[path].Points {
			whole = whole && s.sel[skRef{path, j}]
		}
		switch {
		case shift && whole:
			for j := range s.sk.Paths[path].Points {
				delete(s.sel, skRef{path, j})
			}
			return nil
		case shift:
			s.selectPath(path, true)
		case !whole:
			s.selectPath(path, false)
		}
		return a.skDragPoints(anchor, x, y)
	}
	// a box that picks the paths it touches
	return a.skMarquee(x, y, shift, func(in func(sx, sy float64) bool) {
		for i, p := range s.sk.Paths {
			for _, q := range p.Flatten() {
				if in(s.ptScreen(q)) {
					s.selectPath(i, true)
					break
				}
			}
		}
	})
}

// skMarquee is the drag of a selection box from screen (x, y). As the box
// changes, the selection is what it was (with Shift) or nothing, plus what
// pick adds: pick is given a test for a screen point being in the box.
func (a *App) skMarquee(x, y float64, shift bool, pick func(in func(sx, sy float64) bool)) func(float64, float64, bool) {
	s := a.sk
	base := map[skRef]bool{}
	if shift {
		for r := range s.sel {
			base[r] = true
		}
	}
	s.marquee = &[4]float64{x, y, x, y}
	return func(nx, ny float64, up bool) {
		s.marquee[2], s.marquee[3] = nx, ny
		x0, x1 := math.Min(x, nx), math.Max(x, nx)
		y0, y1 := math.Min(y, ny), math.Max(y, ny)
		s.sel = map[skRef]bool{}
		for r := range base {
			s.sel[r] = true
		}
		pick(func(sx, sy float64) bool { return sx >= x0 && sx <= x1 && sy >= y0 && sy <= y1 })
		if up {
			s.marquee = nil
		}
		a.w.Redraw()
	}
}

// skDragPoints moves the selected points with the pointer: the one pressed
// (anchor) snaps, and the rest keep their places relative to it.
func (a *App) skDragPoints(anchor skRef, x, y float64) func(float64, float64, bool) {
	s := a.sk
	orig := map[skRef][2]float64{}
	for r := range s.sel {
		orig[r] = s.sk.Paths[r.path].Points[r.pt].P
	}
	a0 := s.sk.Paths[anchor.path].Points[anchor.pt].P
	moved := false
	return func(nx, ny float64, up bool) {
		if !moved && math.Hypot(nx-x, ny-y) < 3 {
			return
		}
		if !moved {
			a.skPush()
			moved = true
		}
		// where the anchor goes: the pointer's move, snapped
		sx, sy := s.toScreen(a0)
		to := a.skSnap(sx+nx-x, sy+ny-y, s.sel, nil)
		dx, dy := s.leftStep(orig, to[0]-a0[0]), to[1]-a0[1]
		for r, p := range orig {
			s.sk.Paths[r.path].Points[r.pt].P = [2]float64{p[0] + dx, p[1] + dy}
		}
		if up {
			// an end dropped on its path's other end closes the path
			done := map[int]bool{}
			for r := range orig {
				if !done[r.path] {
					done[r.path] = true
					s.closeIfEndsMeet(r.path)
				}
			}
		}
		a.w.Redraw()
	}
}

// skCloseMeetingEnds closes every path whose ends have come together, by
// whatever edit: a drag, a nudge, a typed position, a point deleted beyond
// the end, so no edit has to call closeIfEndsMeet itself. The path being
// drawn is left alone, and so is everything during a press (a drag closes
// it on release: its points must keep their places meanwhile). Called each
// frame and before the API reports the drawing.
func (a *App) skCloseMeetingEnds() {
	s := a.sk
	if s == nil || a.pressDrag != nil {
		return
	}
	for i := range s.sk.Paths {
		if i != s.drawing {
			s.closeIfEndsMeet(i)
		}
	}
}

// closeIfEndsMeet closes open path i when its last point is on its first
// (an end dragged onto the other, a line finished there): the last point
// goes, its incoming handle carried to the first. It reports whether it
// closed.
func (s *sketchSession) closeIfEndsMeet(i int) bool {
	if i < 0 || i >= len(s.sk.Paths) {
		return false
	}
	p := &s.sk.Paths[i]
	n := len(p.Points)
	if p.Closed || n < 4 {
		return false // (three points with the ends together: a line there and back)
	}
	first, last := p.Points[0].P, p.Points[n-1].P
	if math.Abs(first[0]-last[0]) > 1e-9 || math.Abs(first[1]-last[1]) > 1e-9 {
		return false
	}
	if in := p.Points[n-1].In; in != ([2]float64{}) {
		p.Points[0].In = in
	}
	p.Points = append([]shapes.SketchPoint(nil), p.Points[:n-1]...)
	p.Closed = true
	if s.sel[skRef{i, n - 1}] {
		delete(s.sel, skRef{i, n - 1})
		s.sel[skRef{i, 0}] = true
	}
	return true
}

// skDragHandle drags one of a point's curve handles: a smooth point's other
// handle mirrors it; a corner's stays. A click without a drag changes
// nothing.
func (a *App) skDragHandle(r skRef, which int, x, y float64) func(float64, float64, bool) {
	s := a.sk
	moved := false
	return func(nx, ny float64, up bool) {
		if up {
			s.dragHandle = nil
		}
		if !moved && math.Hypot(nx-x, ny-y) < 3 {
			return
		}
		if !moved {
			a.skPush()
			moved = true
		}
		if !up {
			s.dragHandle, s.dragWhich = &r, which
		}
		q := &s.sk.Paths[r.path].Points[r.pt]
		m := a.skSnapHandle(nx, ny)
		d := [2]float64{m[0] - q.P[0], m[1] - q.P[1]}
		this, other := &q.In, &q.Out
		if which == 1 {
			this, other = &q.Out, &q.In
		}
		// dragged back onto its point, a handle goes: with none left the
		// point is a sharp corner
		px, py := s.toScreen(q.P)
		if math.Hypot(nx-px, ny-py) <= skPick {
			d = [2]float64{}
		}
		*this = d
		switch {
		case q.Kind == shapes.PointSmooth:
			// linked and symmetrical
			*other = [2]float64{-d[0], -d[1]}
		case q.In == [2]float64{} && q.Out == [2]float64{}:
			q.Kind = shapes.PointCorner
		}
		a.w.Redraw()
	}
}

// skSnapHandle is where a handle dragged to screen (x, y) goes: the grid,
// unless Alt is held.
func (a *App) skSnapHandle(x, y float64) [2]float64 {
	p := a.sk.toMM(x, y)
	if g := a.snap(); g > 0 && a.mods&window.Alt == 0 {
		p = [2]float64{math.Round(p[0]/g) * g, math.Round(p[1]/g) * g}
	}
	return p
}

// --- the add-or-remove-points tool -----------------------------------------------------

// skRefinePress removes the point pressed, or puts a new one on the line
// pressed, which can be dragged into place before letting go.
func (a *App) skRefinePress(x, y float64) func(float64, float64, bool) {
	s := a.sk
	if r, ok := s.pointAt(x, y, nil); ok {
		a.skPush()
		s.sel = map[skRef]bool{r: true}
		a.skDeleteSelected()
		return nil
	}
	path, seg, t, ok := s.segmentAt(x, y)
	if !ok {
		return nil
	}
	a.skPush()
	insertSketchPoint(&s.sk.Paths[path], seg, t)
	r := skRef{path, seg + 1}
	s.sel = map[skRef]bool{r: true}
	p0 := s.sk.Paths[path].Points[r.pt].P
	return func(nx, ny float64, up bool) {
		if math.Hypot(nx-x, ny-y) < 3 {
			return
		}
		sx, sy := s.toScreen(p0)
		s.sk.Paths[path].Points[r.pt].P = a.skSnap(sx+nx-x, sy+ny-y, map[skRef]bool{r: true}, nil)
		a.w.Redraw()
	}
}

// insertSketchPoint puts a point on path p at parameter t of the curve
// from point seg to the next, leaving the curve as it was: its ends' handles
// are fixed (an automatic point's would change with a new neighbour).
func insertSketchPoint(p *shapes.SketchPath, seg int, t float64) {
	n := len(p.Points)
	j := (seg + 1) % n
	c := p.Bezier(seg)
	freeze(p, seg)
	freeze(p, j)
	lerp := func(a, b shapes.Pt, t float64) shapes.Pt {
		return shapes.Pt{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t}
	}
	p01, p12, p23 := lerp(c[0], c[1], t), lerp(c[1], c[2], t), lerp(c[2], c[3], t)
	p012, p123 := lerp(p01, p12, t), lerp(p12, p23, t)
	mid := lerp(p012, p123, t)
	q := shapes.SketchPoint{P: [2]float64{mid.X, mid.Y}}
	if c[1] != c[0] || c[2] != c[3] {
		p.Points[seg].Out = [2]float64{p01.X - c[0].X, p01.Y - c[0].Y}
		p.Points[j].In = [2]float64{p23.X - c[3].X, p23.Y - c[3].Y}
		q.In = [2]float64{p012.X - mid.X, p012.Y - mid.Y}
		q.Out = [2]float64{p123.X - mid.X, p123.Y - mid.Y}
		q.Kind = shapes.PointSmooth
	}
	p.Points = append(p.Points[:seg+1], append([]shapes.SketchPoint{q}, p.Points[seg+1:]...)...)
}

// freeze turns an automatic point into a smooth one with the handles it
// has now.
func freeze(p *shapes.SketchPath, i int) {
	if p.Points[i].Kind != shapes.PointAuto {
		return
	}
	in, out := p.Handles(i)
	p.Points[i].In, p.Points[i].Out = [2]float64{in.X, in.Y}, [2]float64{out.X, out.Y}
	p.Points[i].Kind = shapes.PointSmooth
}

// skDeleteSelected removes the selected points. A path left with one point
// goes; a closed path left with two opens.
func (a *App) skDeleteSelected() {
	s := a.sk
	var paths []shapes.SketchPath
	for i, p := range s.sk.Paths {
		var keep []shapes.SketchPoint
		for j, q := range p.Points {
			if !s.sel[skRef{i, j}] {
				keep = append(keep, q)
			}
		}
		if len(keep) < 2 {
			continue
		}
		p.Points = keep
		if p.Closed && len(keep) < 3 {
			p.Closed = false
		}
		paths = append(paths, p)
	}
	s.sk.Paths = paths
	s.sel = map[skRef]bool{}
	s.drawing = -1
}

// --- changes to the selection ------------------------------------------------------

// skSetPointKind makes the selected points sharp corners, smooth, or
// corners with split handles.
func (a *App) skSetPointKind(kind int, split bool) {
	s := a.sk
	a.skPush()
	for r := range s.sel {
		p := &s.sk.Paths[r.path]
		in, out := p.Handles(r.pt)
		q := &p.Points[r.pt]
		switch {
		case split:
			q.In, q.Out, q.Kind = [2]float64{in.X, in.Y}, [2]float64{out.X, out.Y}, shapes.PointCorner
		case kind == shapes.PointCorner:
			q.In, q.Out, q.Kind = [2]float64{}, [2]float64{}, shapes.PointCorner
		default:
			if in == (shapes.Pt{}) && out == (shapes.Pt{}) {
				// a sharp corner: the handles a smooth curve would have
				q.Kind = shapes.PointAuto
				in, out = p.Handles(r.pt)
			}
			// linked and symmetrical: along
			// the average direction, both the average length
			li, lo := math.Hypot(in.X, in.Y), math.Hypot(out.X, out.Y)
			d := shapes.Pt{X: out.X/math.Max(lo, 1e-12) - in.X/math.Max(li, 1e-12), Y: out.Y/math.Max(lo, 1e-12) - in.Y/math.Max(li, 1e-12)}
			if l := math.Hypot(d.X, d.Y); l > 1e-12 {
				d = shapes.Pt{X: d.X / l, Y: d.Y / l}
				m := (li + lo) / 2
				in, out = shapes.Pt{X: -d.X * m, Y: -d.Y * m}, shapes.Pt{X: d.X * m, Y: d.Y * m}
			}
			q.In, q.Out, q.Kind = [2]float64{in.X, in.Y}, [2]float64{out.X, out.Y}, shapes.PointSmooth
		}
	}
}

// skSetPaths changes every selected path: an undo step, unless nothing
// came of it (a click on the setting they already have).
func (a *App) skSetPaths(f func(p *shapes.SketchPath)) {
	s := a.sk
	was, key := s.snapshot(), shapes.SketchKey(s.sk)
	for _, i := range s.selPaths() {
		f(&s.sk.Paths[i])
	}
	if shapes.SketchKey(s.sk) == key {
		return
	}
	now := s.sk
	s.sk = was.sk
	a.skPush()
	s.sk = now
}

// skCopy keeps the selected paths, whole, for pasting into this sketch or
// another.
func (a *App) skCopy() {
	s := a.sk
	var out []shapes.SketchPath
	for _, i := range s.selPaths() {
		// (not a line's first point, just put down: pasted back, it would
		// be a stray point no path grows from)
		if len(s.sk.Paths[i].Points) >= 2 {
			out = append(out, s.sk.Paths[i])
		}
	}
	if len(out) > 0 {
		a.skClip = shapes.CopySketch(&shapes.Sketch{Paths: out}).Paths
	}
}

// skPaste adds the copied paths where they were, selected (drag them
// away, or leave them for a pattern of strokes).
func (a *App) skPaste() {
	s := a.sk
	if len(a.skClip) == 0 {
		return
	}
	a.skFinish()
	a.skPush()
	s.sel = map[skRef]bool{}
	var added []int
	for _, p := range shapes.CopySketch(&shapes.Sketch{Paths: a.skClip}).Paths {
		s.sk.Paths = append(s.sk.Paths, p)
		added = append(added, len(s.sk.Paths)-1)
	}
	// (copied from another kind of sketch, they may lie right of this one's
	// center line)
	if a.skSettle(s.pathRefs(added)) {
		for _, i := range added {
			s.selectPath(i, true)
		}
	}
}

func (a *App) skNudge(dx, dy float64) {
	s := a.sk
	if len(s.sel) == 0 {
		return
	}
	a.skPush()
	orig := map[skRef][2]float64{}
	for r := range s.sel {
		orig[r] = s.sk.Paths[r.path].Points[r.pt].P
	}
	dx = s.leftStep(orig, dx)
	for r := range s.sel {
		q := &s.sk.Paths[r.path].Points[r.pt]
		q.P = [2]float64{q.P[0] + dx, q.P[1] + dy}
	}
}

// leftStep is how far right points at pts may move by dx in a revolve or
// coil: no further than the center line.
func (s *sketchSession) leftStep(pts map[skRef][2]float64, dx float64) float64 {
	if !shapes.Spins(s.kind) || dx <= 0 {
		return dx
	}
	most := math.Inf(-1)
	for _, p := range pts {
		most = math.Max(most, p[0])
	}
	return math.Max(0, math.Min(dx, -most))
}

// skSetTool picks a tool. Going from one drawing tool to another carries on
// the path being drawn, so a path can mix straight lines and curves.
func (a *App) skSetTool(t int) {
	s := a.sk
	if s.img.panel { // a tool picked: back to drawing, not placing the picture
		a.skCloseImagePanel()
	}
	if !skPenTool(t) {
		a.skFinish()
	}
	switch {
	case skPenTool(t):
		s.lastPen = t
	case skEditTool(t):
		s.lastEdit = t
	}
	s.tool = t
	a.w.Redraw()
}

// skClear throws away the path being drawn, leaving the drawing as it was
// before it was started (or carried on). With nothing being drawn it does
// nothing.
func (a *App) skClear() {
	s := a.sk
	if s.drawing < 0 || s.drawBase == nil {
		return
	}
	a.skPush()
	s.sk = shapes.CopySketch(s.drawBase)
	s.drawing = -1
	s.drawBase = nil
	s.sel = map[skRef]bool{}
	a.w.Redraw()
}

// skBack leaves the Pen or Edit mode, keeping the selection.
func (a *App) skBack() { a.skSetTool(skShape) }

// skSegment is the stretch of path that is selected, if exactly one is:
// two neighbouring points of one path and nothing else. seg is the point
// it starts at.
func (s *sketchSession) skSegment() (path, seg int, ok bool) {
	if len(s.sel) != 2 {
		return 0, 0, false
	}
	var rs []skRef
	for r := range s.sel {
		rs = append(rs, r)
	}
	if rs[0].path != rs[1].path {
		return 0, 0, false
	}
	p := s.sk.Paths[rs[0].path]
	n := len(p.Points)
	i, j := rs[0].pt, rs[1].pt
	switch {
	case j == i+1 || (p.Closed && i == n-1 && j == 0):
		return rs[0].path, i, true
	case i == j+1 || (p.Closed && j == n-1 && i == 0):
		return rs[0].path, j, true
	}
	return 0, 0, false
}

// skToggleClosed closes the selected open paths, or opens the selected
// closed ones: at the selected stretch, if one is, else between the last
// point and the first.
func (a *App) skToggleClosed() {
	s := a.sk
	ps := s.selPaths()
	if len(ps) == 0 {
		return
	}
	segPath, seg, segOK := s.skSegment()
	a.skPush()
	for _, i := range ps {
		p := &s.sk.Paths[i]
		if !p.Closed {
			// (ends already together: closed without a side of no length)
			if !s.closeIfEndsMeet(i) && len(p.Points) >= 2 {
				p.Closed = true
			}
			continue
		}
		if segOK && segPath == i {
			// start after the stretch, so it is the one left out
			k := seg + 1
			p.Points = append(append([]shapes.SketchPoint(nil), p.Points[k:]...), p.Points[:k]...)
		}
		p.Closed = false
	}
	s.sel = map[skRef]bool{}
	for _, i := range ps {
		s.selectPath(i, true)
	}
}

// skSetSegmentLength makes the selected stretch l long, its ends moving
// apart or together evenly about its middle.
func (a *App) skSetSegmentLength(l float64) {
	s := a.sk
	path, seg, ok := s.skSegment()
	if !ok {
		return
	}
	if !(l > 0) {
		a.showToast("the length must be above 0")
		return
	}
	p := &s.sk.Paths[path]
	j := (seg + 1) % len(p.Points)
	a0, a1 := p.Points[seg].P, p.Points[j].P
	dx, dy := a1[0]-a0[0], a1[1]-a0[1]
	d := math.Hypot(dx, dy)
	if d == 0 {
		return
	}
	a.skPush()
	mx, my := (a0[0]+a1[0])/2, (a0[1]+a1[1])/2
	ux, uy := dx/d*l/2, dy/d*l/2
	p.Points[seg].P = [2]float64{mx - ux, my - uy}
	p.Points[j].P = [2]float64{mx + ux, my + uy}
	a.skSettle([]skRef{{path, seg}, {path, j}})
}

// skSelBounds is the box round the selected paths (mm).
func (s *sketchSession) skSelBounds() (x0, y0, x1, y1 float64, ok bool) {
	x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, i := range s.selPaths() {
		for _, q := range s.sk.Paths[i].Flatten() {
			x0, y0, x1, y1 = math.Min(x0, q.X), math.Min(y0, q.Y), math.Max(x1, q.X), math.Max(y1, q.Y)
		}
	}
	return x0, y0, x1, y1, !math.IsInf(x0, 1)
}

// skSetSize stretches the selected paths about their box's middle so the
// box is v wide (axis 0) or high (axis 1).
func (a *App) skSetSize(axis int, v float64) {
	s := a.sk
	x0, y0, x1, y1, ok := s.skSelBounds()
	if !ok {
		return
	}
	if !(v > 0) {
		a.showToast("the size must be above 0")
		return
	}
	lo, hi := x0, x1
	if axis == 1 {
		lo, hi = y0, y1
	}
	if hi-lo < 1e-9 {
		a.showToast("it has no size that way to change")
		return
	}
	a.skPush()
	f, mid := v/(hi-lo), (lo+hi)/2
	for _, i := range s.selPaths() {
		for j := range s.sk.Paths[i].Points {
			q := &s.sk.Paths[i].Points[j]
			q.P[axis] = mid + (q.P[axis]-mid)*f
			q.In[axis] *= f
			q.Out[axis] *= f
		}
	}
	a.skSettle(s.pathRefs(s.selPaths()))
}

// skAddAtLength puts the next point of the path being drawn l from the
// last, toward the pointer.
func (a *App) skAddAtLength(l float64) {
	s := a.sk
	if s.drawing < 0 || l <= 0 {
		return
	}
	path := &s.sk.Paths[s.drawing]
	last := path.Points[len(path.Points)-1].P
	dx, dy := s.at[0]-last[0], s.at[1]-last[1]
	d := math.Hypot(dx, dy)
	if d == 0 {
		dx, dy, d = 1, 0, 1
		if shapes.Spins(s.kind) {
			dx = -1 // away from the center line
		}
	}
	a.skPush()
	p := s.keepLeft([2]float64{last[0] + dx/d*l, last[1] + dy/d*l})
	path.Points = append(path.Points, s.skNewPoint(p))
	r := skRef{s.drawing, len(path.Points) - 1}
	if a.skSettle(nil) {
		s.sel = map[skRef]bool{r: true}
	}
}

// skTypeLength opens a box to type the next stretch's length in.
func (a *App) skTypeLength() {
	s := a.sk
	if s.drawing < 0 {
		return
	}
	path := s.sk.Paths[s.drawing]
	last := path.Points[len(path.Points)-1].P
	l := math.Hypot(s.at[0]-last[0], s.at[1]-last[1])
	a.openField(&textField{id: "sk-len", text: strings.TrimSpace(editor.Format(l, a.units(), true)), commit: func(t string) bool {
		v, err := editor.Parse(t, a.units())
		if err != nil || !(v > 0) || math.IsInf(v, 0) {
			a.showToast("the length must be a number above 0")
			return true
		}
		a.skAddAtLength(v)
		return true
	}})
}

// --- input -----------------------------------------------------------------------

// sketchKey handles a key in the workspace.
func (a *App) sketchKey(vk int, m window.Mods) {
	s := a.sk
	ctrl, shift := m&window.Ctrl != 0, m&window.Shift != 0
	step := a.snap()
	if step <= 0 {
		step = 1
	}
	if shift {
		step *= 10
	}
	if s.img.panel && vk == vkEscape {
		// a scale or level part done is dropped first, then the panel
		if len(s.img.picks) > 0 || s.img.mode != imgMove {
			s.img.mode, s.img.picks = imgMove, nil
		} else {
			a.skCloseImagePanel()
		}
		return
	}
	switch {
	case ctrl && (vk == 'Y' || (vk == 'Z' && shift)):
		a.skRedo()
	case ctrl && vk == 'Z':
		a.skUndo()
	case ctrl && vk == 'A':
		a.skFinish()
		s.selectAll()
	case ctrl && vk == 'C':
		a.skCopy()
	case ctrl && vk == 'X':
		a.skCopy()
		if len(s.sel) > 0 {
			a.skPush()
			for _, i := range s.selPaths() {
				s.selectPath(i, true)
			}
			a.skDeleteSelected()
		}
	case ctrl && vk == 'V':
		a.skPaste()
	case ctrl && vk == 'D':
		a.skCopy()
		a.skPaste()
	case ctrl:
	case vk == vkReturn:
		a.skFinish()
	case vk == vkTab:
		a.skTypeLength()
	case vk == vkEscape:
		switch {
		case s.drawing >= 0:
			a.skFinish()
		case s.strokeOpen:
			s.strokeOpen = false
		case s.xformOpen:
			s.xformOpen = false
		case s.tool != skShape:
			a.skBack()
		default:
			s.sel = map[skRef]bool{}
		}
	case vk == vkDelete || vk == vkBack:
		if len(s.sel) > 0 {
			a.skPush()
			a.skDeleteSelected()
		}
	case vk == vkLeft:
		a.skNudge(-step, 0)
	case vk == vkRight:
		a.skNudge(step, 0)
	case vk == vkUp:
		a.skNudge(0, step)
	case vk == vkDown:
		a.skNudge(0, -step)
	case vk == 'P':
		a.skSetTool(s.lastPen)
	case vk == 'A':
		a.skSetTool(s.lastEdit)
	case vk == 'V':
		a.skSetTool(skSelect)
	case vk == '1':
		a.skSetTool(skLine)
	case vk == '2':
		a.skSetTool(skBezier)
	case vk == '3':
		a.skSetTool(skSmooth)
	case vk == '4':
		a.skSetTool(skRefine)
	case vk == 'O':
		if s.drawing < 0 { // as its button: not while a path is being drawn
			a.skToggleClosed()
		}
	case vk == 'H':
		if s.drawing < 0 { // as its button
			a.skSetHole(!s.skAllHoles())
		}
	case vk == 'M':
		if s.drawing < 0 && len(s.selPaths()) > 0 {
			s.xformOpen = !s.xformOpen
			s.strokeOpen = false
		}
	case vk == 'D':
		s.dims = !s.dims
	case vk == 'F':
		a.fitSketch()
	case vk == vkPlus || vk == vkAdd:
		a.skZoomAt(s.board.CX(), s.board.CY(), 1.25)
	case vk == vkMinus || vk == vkSub:
		a.skZoomAt(s.board.CX(), s.board.CY(), 0.8)
	case vk == vkF1:
		a.openPopup(popHelp)
	}
	a.w.Redraw()
}

// skBoardPress is a left press on the board.
func (a *App) skBoardPress(x, y float64, m window.Mods) func(float64, float64, bool) {
	s := a.sk
	a.closePopup()
	switch s.tool {
	case skShape:
		return a.skShapePress(x, y, m)
	case skSelect:
		return a.skSelectPress(x, y, m)
	case skRefine:
		return a.skRefinePress(x, y)
	}
	return a.skDrawPress(x, y)
}

// --- drawing the workspace ---------------------------------------------------------

func (a *App) skLayout() {
	s := a.sk
	top := titleH + toolbarH
	// (the rulers take the left and bottom edges)
	s.board = render.Rect{X: skRulerW, Y: top, W: math.Max(50, a.W-skRulerW), H: math.Max(50, a.H-top-skRulerH)}
	a.vp = s.board
	if !s.viewSet {
		a.skOpenView()
	}
	s.pinAxis() // (the board may have been resized)
	a.skCloseMeetingEnds()
}

// drawSketch draws the whole workspace (in place of the 3D view, the
// toolbar, the shape panel and Properties).
func (a *App) drawSketch() {
	s := a.sk
	b := a.batch
	r := s.board
	s.pruneSel()
	b.Fill(r, a.bgColor)
	a.reg(region{r: r, id: "sketch-board", cursor: window.CursorCross,
		press: func(x, y float64, m window.Mods) func(float64, float64, bool) {
			if f := a.field; f != nil && f.id == "sk-len" {
				// a length being typed for the next point: a click elsewhere
				// drops it (committed, it would add a point where the click
				// then lands, ending the path)
				a.field = nil
			} else if f != nil {
				a.commitField()
			}
			if a.sk.img.panel { // placing the picture, not drawing
				return a.skImagePress(x, y)
			}
			return a.skBoardPress(x, y, m)
		},
		wheel: func(d float64) bool {
			a.skZoomAt(a.mx, a.my, math.Pow(1/0.87, d*a.zoomSpeed))
			return true
		}})
	if a.hot == "sketch-board" && a.pressDrag == nil {
		var from *[2]float64
		if s.drawing >= 0 {
			pts := s.sk.Paths[s.drawing].Points
			from = &pts[len(pts)-1].P
		}
		s.at = a.skSnap(a.mx, a.my, nil, from)
	}
	b.PushClip(r)
	a.skDrawGrid()
	a.skDrawImage()
	a.skDrawReferences()
	a.skDrawFill()
	a.skDrawPaths()
	a.skDrawRubberBand()
	a.skDrawImagePicks()
	if m := s.marquee; m != nil {
		mr := render.Rect{X: math.Min(m[0], m[2]), Y: math.Min(m[1], m[3]), W: math.Abs(m[2] - m[0]), H: math.Abs(m[3] - m[1])}
		b.Fill(mr, render.Color{colBlue[0], colBlue[1], colBlue[2], 0.08})
		b.RoundRectStroke(mr, 0, 1, colBlue)
	}
	a.skDrawDims()
	a.skDrawHint()
	b.PopClip()
	a.skDrawRulers()
	a.skDrawPreview()
	a.skDrawViewButtons()
	a.skDrawModeBar()
	a.skDrawActionBar()
	if !a.skDrawImagePanel() {
		a.skDrawPointPanel()
	}
	a.drawTitleBar()
	a.skDrawToolbar()
}

// skGridStep is the grid's spacing: the smallest of 1, 2, 5 times a power of
// ten at least 14 pixels apart (at the home view's scale, 1 mm squares).
func skGridStep(zoom float64) float64 {
	for e := -2.0; e < 6; e++ {
		for _, m := range []float64{1, 2, 5} {
			if st := m * math.Pow(10, e); st*zoom >= 14 {
				return st
			}
		}
	}
	return 1e6
}

func (a *App) skDrawGrid() {
	s := a.sk
	b := a.batch
	r := s.board
	st := skGridStep(s.zoom)
	major := st * 10
	lo, hi := s.toMM(r.X, r.Y+r.H), s.toMM(r.X+r.W, r.Y)
	for _, g := range []struct {
		step float64
		col  render.Color
	}{{st, colGridMinor}, {major, colGridMajor}} {
		c := g.col
		c[3] *= 0.6
		// (counted, not stepped: far from the origin a step can be too
		// small to change a float, and the loop would never end)
		x0, y0 := math.Floor(lo[0]/g.step), math.Floor(lo[1]/g.step)
		for i := 0.0; i <= math.Min(4000, math.Ceil((hi[0]-lo[0])/g.step)+1); i++ {
			sx, _ := s.toScreen([2]float64{(x0 + i) * g.step, 0})
			b.Fill(render.Rect{X: math.Round(sx), Y: r.Y, W: 1, H: r.H}, c)
		}
		for i := 0.0; i <= math.Min(4000, math.Ceil((hi[1]-lo[1])/g.step)+1); i++ {
			_, sy := s.toScreen([2]float64{0, (y0 + i) * g.step})
			b.Fill(render.Rect{X: r.X, Y: math.Round(sy), W: r.W, H: 1}, c)
		}
	}
	ox, oy := s.toScreen([2]float64{0, 0})
	axis := colGridMajor
	b.Fill(render.Rect{X: r.X, Y: math.Round(oy) - 0.5, W: r.W, H: 2}, axis)
	if shapes.Spins(s.kind) {
		// what is right of the center line isn't spun: it is blanked out,
		// grid and all, and nothing can be drawn there
		blank := render.Rect{X: ox, Y: r.Y, W: math.Max(0, r.X+r.W-ox), H: r.H}
		b.Fill(blank, colDivider)
		if blank.W > 160 {
			msg := "Not used: draw left of the center line"
			tw := b.TextWidth(msg, fontUI, false)
			if tw+24 < blank.W {
				b.Text(msg, blank.X+(blank.W-tw)/2, blank.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, colTextDim)
			}
		}
		b.Dashed(ox, r.Y, ox, r.Y+r.H, 2, 8, colBlue)
		b.Text("Center line", ox+6, r.Y+r.H-100, fontSmall, false, colBlue)
	} else {
		b.Fill(render.Rect{X: math.Round(ox) - 0.5, Y: r.Y, W: 2, H: r.H}, axis)
	}
	// the scale
	unit := " mm"
	if a.units() == "in" {
		unit = " in"
	}
	label := fmt.Sprintf("Grid: small squares %s, large %s", strings.TrimSpace(editor.Format(st, a.units(), true))+unit, strings.TrimSpace(editor.Format(major, a.units(), true))+unit)
	b.Text(label, r.X+12, r.Y+r.H-24, fontSmall, false, colTextDim)
}

// The rulers along the board's left and bottom edges.
const (
	skRulerW = 34.0 // the left one's width
	skRulerH = 22.0 // the bottom one's height
)

// skDrawRulers draws the rulers: a tick at each small grid square, a
// longer one at five, and the place at each large square, in the
// settings' units; and where the pointer is, in the sketch kind's color.
func (a *App) skDrawRulers() {
	s := a.sk
	b := a.batch
	r := s.board
	left := render.Rect{X: 0, Y: r.Y, W: r.X, H: r.H}
	foot := render.Rect{X: r.X, Y: r.Y + r.H, W: r.W, H: skRulerH}
	corner := render.Rect{X: 0, Y: r.Y + r.H, W: r.X, H: skRulerH}
	for _, rr := range []render.Rect{left, foot, corner} {
		b.Fill(rr, colChrome)
	}
	b.Fill(render.Rect{X: r.X - 1, Y: r.Y, W: 1, H: r.H + skRulerH}, colBorder)
	b.Fill(render.Rect{X: 0, Y: r.Y + r.H, W: r.X + r.W, H: 1}, colBorder)
	unit, per := "mm", 1.0
	if a.units() == "in" {
		unit, per = "in", 25.4
	}
	const fs = 10.0
	b.Text(unit, corner.CX()-b.TextWidth(unit, fs, false)/2, corner.CY()-b.LineHeight(fs, false)/2, fs, false, colTextDim)
	st := skGridStep(s.zoom)
	label := func(v float64) string {
		v = math.Round(v/per*1000) / 1000
		if v == 0 {
			v = 0 // (no "-0")
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	// ticks: whole numbers of small squares, counted (as the grid)
	// (at is given each tick's place, its length, and whether it is a
	// large square's, which gets its number)
	ticks := func(lo, hi float64, at func(v, size float64, major bool)) {
		k0 := math.Floor(lo / st)
		for i := 0.0; i <= math.Min(4000, math.Ceil((hi-lo)/st)+1); i++ {
			k := k0 + i
			switch {
			case math.Mod(k, 10) == 0:
				at(k*st, 9, true)
			case math.Mod(k, 5) == 0:
				at(k*st, 6, false)
			default:
				at(k*st, 3, false)
			}
		}
	}
	lo, hi := s.toMM(r.X, r.Y+r.H), s.toMM(r.X+r.W, r.Y)
	b.PushClip(foot)
	ticks(lo[0], hi[0], func(v, size float64, major bool) {
		x, _ := s.toScreen([2]float64{v, 0})
		x = math.Round(x)
		b.Fill(render.Rect{X: x, Y: foot.Y, W: 1, H: size}, colTextDim)
		if major {
			b.Text(label(v), x+3, foot.Y+foot.H-b.LineHeight(fs, false)-2, fs, false, colTextDim)
		}
	})
	b.PopClip()
	b.PushClip(left)
	ticks(lo[1], hi[1], func(v, size float64, major bool) {
		_, y := s.toScreen([2]float64{0, v})
		y = math.Round(y)
		b.Fill(render.Rect{X: left.X + left.W - size, Y: y, W: size, H: 1}, colTextDim)
		if major {
			t := label(v)
			b.Text(t, left.X+left.W-4-b.TextWidth(t, fs, false), y-b.LineHeight(fs, false)-1, fs, false, colTextDim)
		}
	})
	b.PopClip()
	if a.hot == "sketch-board" {
		// the pointer's place, as a point would go down
		col := geom.ParseColor(kindColor(s.kind))
		kc := render.Color{col.R, col.G, col.B, 1}
		x, y := s.toScreen(s.at)
		if x >= foot.X && x <= foot.X+foot.W {
			b.Poly([][2]float64{{x, foot.Y + 7}, {x - 4.5, foot.Y + 1}, {x + 4.5, foot.Y + 1}}, kc)
		}
		if y >= left.Y && y <= left.Y+left.H {
			b.Poly([][2]float64{{left.X + left.W - 7, y}, {left.X + left.W - 1, y - 4.5}, {left.X + left.W - 1, y + 4.5}}, kc)
		}
	}
}

// skDrawFill shades what the drawing builds (for a revolve, what is spun).
func (a *App) skDrawFill() {
	s := a.sk
	b := a.batch
	key := shapes.SketchKey(s.sk) + s.kind
	if key != s.fillKey {
		s.fillKey = key
		s.fill = nil
		cs := shapes.SketchOutlines(s.sk)
		switch s.kind {
		case "revolve":
			cs = shapes.RevolveOutlines(s.sk)
		case "coil":
			cs = shapes.CoilOutlines(s.sk)
		}
		for _, reg := range shapes.Regions(cs) {
			s.fill = append(s.fill, shapes.Triangulate(reg)...)
		}
	}
	col := geom.ParseColor(kindColor(s.kind))
	if n := a.ed.Doc.FindAny(s.nodeID); n != nil {
		col = geom.ParseColor(n.Color)
	}
	fc := render.Color{col.R, col.G, col.B, 0.45}
	for _, t := range s.fill {
		x0, y0 := s.ptScreen(t[0])
		x1, y1 := s.ptScreen(t[1])
		x2, y2 := s.ptScreen(t[2])
		b.Tri(x0, y0, x1, y1, x2, y2, fc)
	}
}

func (a *App) skDrawPaths() {
	s := a.sk
	b := a.batch
	sel := map[int]bool{}
	for _, i := range s.selPaths() {
		sel[i] = true
	}
	for i, p := range s.sk.Paths {
		f := p.Flatten()
		if len(f) == 0 {
			continue
		}
		pts := make([][2]float64, 0, len(f)+1)
		for _, q := range f {
			x, y := s.ptScreen(q)
			pts = append(pts, [2]float64{x, y})
		}
		if p.Closed {
			pts = append(pts, pts[0])
		}
		c := colText
		if sel[i] {
			c = colBlue
		}
		if p.Hole {
			// a hole: dashed, as it is a gap in what builds
			dashedPolyline(b, pts, 1.6, 6, 4, c)
			continue
		}
		b.Polyline(pts, 1.6, c)
	}
	// handles of the selected points
	for r := range s.sel {
		if r.path >= len(s.sk.Paths) || r.pt >= len(s.sk.Paths[r.path].Points) {
			continue
		}
		q := s.sk.Paths[r.path].Points[r.pt]
		if q.Kind == shapes.PointAuto {
			continue
		}
		px, py := s.toScreen(q.P)
		for _, h := range [][2]float64{q.In, q.Out} {
			if h == ([2]float64{}) {
				continue
			}
			hx, hy := s.toScreen([2]float64{q.P[0] + h[0], q.P[1] + h[1]})
			b.Line(px, py, hx, hy, 1.2, colHandleHot)
			b.Circle(hx, hy, 4.5, colOnColor)
			b.Ring(hx, hy, 4.5, 1.5, colHandleHot)
		}
	}
	// the points
	hover, hok := skRef{}, false
	if a.hot == "sketch-board" && a.pressDrag == nil {
		hover, hok = s.pointAt(a.mx, a.my, nil)
	}
	for i, p := range s.sk.Paths {
		for j, q := range p.Points {
			x, y := s.toScreen(q.P)
			r := skRef{i, j}
			fill, edge := colOnColor, colText
			if s.sel[r] {
				fill, edge = colBlue, colBlue
			}
			if hok && hover == r {
				edge = colHandleHot
			}
			rad := 4.0
			if i == s.drawing && j == 0 && len(p.Points) >= 2 {
				rad = 5.5 // the point that closes the path
			}
			switch q.Kind {
			case shapes.PointCorner:
				b.Fill(render.Rect{X: x - rad, Y: y - rad, W: 2 * rad, H: 2 * rad}, edge)
				b.Fill(render.Rect{X: x - rad + 1.5, Y: y - rad + 1.5, W: 2*rad - 3, H: 2*rad - 3}, fill)
			case shapes.PointSmooth:
				b.Circle(x, y, rad+0.5, edge)
				b.Circle(x, y, rad-1, fill)
			default:
				d := rad + 1.5
				b.Poly([][2]float64{{x, y - d}, {x + d, y}, {x, y + d}, {x - d, y}}, edge)
				d -= 2
				b.Poly([][2]float64{{x, y - d}, {x + d, y}, {x, y + d}, {x - d, y}}, fill)
			}
		}
	}
}

// dashedPolyline draws pts dashed, on pixels drawn and off left out, the
// dashes running on round corners (a curve is many short pieces).
func dashedPolyline(b *render.Batch, pts [][2]float64, w, on, off float64, c render.Color) {
	at, drawing := 0.0, true // how far along the current dash or gap
	for i := 0; i+1 < len(pts); i++ {
		p, q := pts[i], pts[i+1]
		l := math.Hypot(q[0]-p[0], q[1]-p[1])
		for t := 0.0; t < l; {
			limit := on
			if !drawing {
				limit = off
			}
			e := math.Min(l, t+limit-at)
			if drawing {
				f0, f1 := t/l, e/l
				b.Line(p[0]+(q[0]-p[0])*f0, p[1]+(q[1]-p[1])*f0, p[0]+(q[0]-p[0])*f1, p[1]+(q[1]-p[1])*f1, w, c)
			}
			at += e - t
			t = e
			if at >= limit-1e-9 {
				at, drawing = 0, !drawing
			}
		}
	}
}

// skDrawRubberBand shows, while a path is being drawn, the stretch the next
// click would add, with its length and angle.
func (a *App) skDrawRubberBand() {
	s := a.sk
	// (a length being typed keeps the stretch and its box showing wherever
	// the pointer has gone, and with the dimensions switched off)
	typing := a.field != nil && a.field.id == "sk-len"
	if s.tool == skSelect || s.tool == skRefine || (a.hot != "sketch-board" && !typing) {
		return
	}
	b := a.batch
	cx, cy := s.toScreen(s.at)
	b.Ring(cx, cy, 3, 1.2, colBlue)
	if s.drawing < 0 || a.pressDrag != nil {
		return
	}
	p := s.sk.Paths[s.drawing]
	tmp := shapes.SketchPath{Points: append(append([]shapes.SketchPoint(nil), p.Points...), s.skNewPoint(s.at))}
	n := len(tmp.Points)
	from := 0
	if n >= 3 {
		from = n - 3 // an automatic point's curve changes with the next
	}
	var pts [][2]float64
	for i := from; i < n-1; i++ {
		for _, q := range tmp.Segment(i) {
			x, y := s.ptScreen(q)
			pts = append(pts, [2]float64{x, y})
		}
	}
	b.Polyline(pts, 1.4, colBlue)
	last := p.Points[len(p.Points)-1].P
	dx, dy := s.at[0]-last[0], s.at[1]-last[1]
	if l := math.Hypot(dx, dy); (l > 0 && s.dims) || typing {
		a.skLengthLabel(cx, cy, l, math.Atan2(dy, dx)*180/math.Pi)
	}
}

// skDrawHint says what the current tool does, at the foot of the board.
func (a *App) skDrawHint() {
	s := a.sk
	b := a.batch
	var t string
	switch s.tool {
	case skShape:
		t = "Click a shape to select it, and drag to move it; Shift adds to the selection. P: Pen to draw, A: Edit to change points."
	case skSelect:
		t = "Click a point, line or area to select it; drag to move. Shift adds to the selection. Drag on empty space to select points in a box."
	case skRefine:
		t = "Click a line to add a point on it (drag to place it); click a point to remove it."
	default:
		if s.drawing >= 0 {
			t = "Click to add points. Click the first point to close the outline; the green check, the last point or Enter ends a line. Tab: type the length. Shift: 15° steps. Alt: off the grid."
		} else {
			t = "Click to start a path."
		}
		if s.tool == skBezier {
			t += " Drag as you place a point to pull out its curve."
		}
	}
	if shapes.Spins(s.kind) {
		t += "  Only what is left of the center line is spun."
	}
	if s.img.panel {
		t = a.skImageHint()
	}
	// at the top, right of the view buttons, wrapped to stop short of the
	// preview card (or the board's edge): two lines at most, then cut
	r := s.board
	x := r.X + 14 + 4*(2*skBtnR+8) + 12
	right := r.X + r.W - 14
	if card, ok := a.skPreviewRect(); ok {
		right = card.X - 12
	}
	w := right - x
	if w < 60 {
		return
	}
	var lines []string
	line := ""
	for _, word := range strings.Fields(t) {
		if next := strings.TrimSpace(line + " " + word); line == "" || b.TextWidth(next, fontSmall, false) <= w {
			line = next
		} else {
			lines, line = append(lines, line), word
		}
	}
	lines = append(lines, line)
	if len(lines) > 2 {
		lines = []string{lines[0], elideEnd(b, strings.Join(lines[1:], " "), w, fontSmall, false)}
	}
	lh := b.LineHeight(fontSmall, false)
	y := r.Y + 14 + skBtnR - lh*float64(len(lines))/2
	for i, l := range lines {
		b.Text(l, x, y+lh*float64(i), fontSmall, false, colTextDim)
	}
}

// skField draws a number box that types a length.
func (a *App) skField(id string, r render.Rect, value float64, set func(v float64)) {
	b := a.batch
	b.RoundRect(r, 3, colChrome)
	border := colFieldBorder
	if a.field != nil && a.field.id == id {
		border = colBlue
	}
	b.RoundRectStroke(r, 3, 1, border)
	text := strings.TrimSpace(editor.Format(value, a.units(), true))
	a.drawFieldText(id, r, text, fontUI, true, colText)
	a.reg(region{r: r, id: id, cursor: window.CursorIBeam,
		press: func(x, y float64, m window.Mods) func(float64, float64, bool) {
			a.openField(&textField{id: id, text: text, commit: func(t string) bool {
				v, err := editor.Parse(t, a.units())
				if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
					a.showToast("not a number: " + t)
					return true
				}
				set(v)
				return true
			}})
			return nil
		}})
}

func (a *App) skDrawPosField(r render.Rect, ref skRef, axis int) {
	s := a.sk
	if ref.path >= len(s.sk.Paths) || ref.pt >= len(s.sk.Paths[ref.path].Points) {
		return
	}
	v := s.sk.Paths[ref.path].Points[ref.pt].P[axis]
	a.skField(fmt.Sprintf("sk-pos-%d", axis), r, v, func(v float64) {
		if ref.path >= len(s.sk.Paths) || ref.pt >= len(s.sk.Paths[ref.path].Points) {
			return
		}
		a.skPush()
		s.sk.Paths[ref.path].Points[ref.pt].P[axis] = v
		a.skSettle([]skRef{ref})
	})
}

func (a *App) skDrawWidthField(r render.Rect) {
	s := a.sk
	w := 0.0
	for _, i := range s.selPaths() {
		if p := s.sk.Paths[i]; p.IsLine() {
			w = p.Width()
			break
		}
	}
	a.skField("sk-width", r, w, func(v float64) {
		if !(v > 0) || v > 1000 {
			a.showToast("the width must be above 0, and 1000 mm at most")
			return
		}
		a.skSetPaths(func(p *shapes.SketchPath) {
			if p.IsLine() {
				p.Stroke = v
			}
		})
	})
}
