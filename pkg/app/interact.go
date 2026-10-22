//go:build windows

package app

import (
	"math"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/win"
)

type dragKind int

const (
	dragOrbit dragKind = iota + 1
	dragPan
	dragMove
	dragResize
	dragLift
	dragRotate
	dragMarquee
	dragTile
	dragCube
)

type nodeState struct {
	M      geom.Mat4
	Size   geom.Vec3
	Params map[string]float64
}

type dragState struct {
	kind           dragKind
	sx, sy, lx, ly float64
	moved          bool
	dup            bool // an Alt-drag made duplicates (an undo step of its own)
	nodes          []*doc.Node
	orig           map[*doc.Node]nodeState
	box            geom.Box
	grab           geom.Vec3
	h              handle
	s0             float64 // line parameter at start (lift/height)
	axis           int
	center         geom.Vec3
	a0, angle      float64 // a0: the protractor's 0° direction
	aStart         float64 // cursor angle when the drag began
	ringR          float64
	onRing         bool
	clickNode      int
	wasSelected    bool
	value          float64 // live value for labels (angle, elevation)
}

// --- handles ----------------------------------------------------------------

const (
	hCorner = iota
	hEdge
	hTop
	hLift
	hRot
)

type handle struct {
	kind, idx int
	p         geom.Vec3
	x, y      float64
	ok        bool
	// for rotation handles: screen-space arc polyline
	arc [][2]float64
}

func (a *App) showHandles() bool {
	if len(a.ed.Sel) == 0 || a.mode == modeAlign || a.mode == modeMirror || a.mode == modeVisibility || a.mode == modeMeasure || a.mode == modeSplit {
		return false
	}
	for _, n := range a.ed.Selected() {
		if n.Locked {
			return false
		}
	}
	return a.place == nil
}

// rotation-plane basis for axis k: u, v with u×v = axis.
func rotBasis(k int) (geom.Vec3, geom.Vec3) {
	switch k {
	case 0:
		return geom.V(0, 1, 0), geom.V(0, 0, 1)
	case 1:
		return geom.V(0, 0, 1), geom.V(1, 0, 0)
	}
	return geom.V(1, 0, 0), geom.V(0, 1, 0)
}

func (a *App) computeHandles() []handle {
	if !a.showHandles() {
		return nil
	}
	b := a.ed.SelectionBoundsIn()
	if b.IsEmpty() {
		return nil
	}
	if a.drag != nil && (a.drag.kind == dragRotate) {
		b = a.drag.box
	}
	c := b.Center()
	z0 := b.Min.Z
	var hs []handle
	add := func(kind, idx int, p geom.Vec3) {
		x, y, ok := a.projectW(p)
		hs = append(hs, handle{kind: kind, idx: idx, p: p, x: x, y: y, ok: ok})
	}
	corners := []geom.Vec3{geom.V(b.Min.X, b.Min.Y, z0), geom.V(b.Max.X, b.Min.Y, z0), geom.V(b.Max.X, b.Max.Y, z0), geom.V(b.Min.X, b.Max.Y, z0)}
	for i, p := range corners {
		add(hCorner, i, p)
	}
	edges := []geom.Vec3{geom.V(c.X, b.Min.Y, z0), geom.V(b.Max.X, c.Y, z0), geom.V(c.X, b.Max.Y, z0), geom.V(b.Min.X, c.Y, z0)}
	for i, p := range edges {
		add(hEdge, i, p)
	}
	top := geom.V(c.X, c.Y, b.Max.Z)
	add(hTop, 0, top)
	ps := a.pixelSizeW(top)
	add(hLift, 0, top.Add(geom.V(0, 0, 26*ps)))

	// rotation handles: about Z on the base edge facing the camera; about
	// X and Y above the top edges facing away from it (in a front view the
	// Y handle floats above the lift handle)
	eye := a.eyeW()
	xs, ys := b.Min.X, b.Min.Y
	if eye.X > c.X {
		xs = b.Max.X
	}
	if eye.Y > c.Y {
		ys = b.Max.Y
	}
	xf, yf := b.Min.X+b.Max.X-xs, b.Min.Y+b.Max.Y-ys
	// about Z: below the middle of the base edge facing the camera, so the
	// protractor's 0° points straight at it
	zAnchor, zOut := geom.V(c.X, ys, z0), geom.V(0, math.Copysign(1, ys-c.Y), 0)
	if math.Abs(eye.X-c.X)*b.Size().Y > math.Abs(eye.Y-c.Y)*b.Size().X {
		zAnchor, zOut = geom.V(xs, c.Y, z0), geom.V(math.Copysign(1, xs-c.X), 0, 0)
	}
	rotAnchors := []geom.Vec3{
		geom.V(xf, c.Y, b.Max.Z), // about X: above the far X face
		geom.V(c.X, yf, b.Max.Z), // about Y: above the far Y face
		zAnchor,
	}
	for k, anc := range rotAnchors {
		ps := a.pixelSizeW(anc)
		// push outward from the box; the Z handle sits on the size line
		var p geom.Vec3
		if k < 2 {
			side := geom.V(0, 0, 0).WithAxis(k, anc.Axis(k)-c.Axis(k)).Norm()
			p = anc.Add(geom.V(0, 0, 24*ps)).Add(side.Mul(8 * ps))
		} else {
			p = anc.Add(zOut.Mul(dimOffset * ps))
		}
		x, y, ok := a.projectW(p)
		h := handle{kind: hRot, idx: k, p: p, x: x, y: y, ok: ok}
		// an arc in the rotation plane, bulging away from the box
		u, v := rotBasis(k)
		dir := p.Sub(c)
		base := math.Atan2(dir.Dot(v), dir.Dot(u))
		r := 14 * ps
		for i := 0; i <= 14; i++ {
			t := base - 1.0 + 2.0*float64(i)/14
			q := p.Add(u.Mul(r * (math.Cos(t) - math.Cos(base)))).Add(v.Mul(r * (math.Sin(t) - math.Sin(base))))
			sx, sy, _ := a.projectW(q)
			h.arc = append(h.arc, [2]float64{sx, sy})
		}
		hs = append(hs, h)
	}
	return hs
}

func (a *App) handleAt(x, y float64) int {
	best, bi := 10.0, -1
	for i, h := range a.handles {
		if !h.ok {
			continue
		}
		d := math.Hypot(h.x-x, h.y-y)
		if h.kind == hRot {
			d -= 7
		}
		if d < best {
			best, bi = d, i
		}
	}
	return bi
}

// --- picking -------------------------------------------------------------------

type hit struct {
	node *doc.Node
	p, n geom.Vec3
	t    float64
	// the part hit: its mesh, placement, and the hit triangle's normal in
	// the mesh's own space
	mesh *geom.Mesh
	m    geom.Mat4
	ln   geom.Vec3
}

// pick finds the nearest shape under (x, y). Nodes in skip are ignored.
func (a *App) pick(x, y float64, includeHidden bool, skip map[*doc.Node]bool) (hit, bool) {
	r := a.ray(x, y)
	best := hit{t: math.Inf(1)}
	// with Section view on, what it hides can't be clicked
	shown := func(t float64) bool { return a.sectionShows(r.At(t)) }
	for _, n := range a.ed.Doc.Nodes {
		if (n.Hidden && !includeHidden) || skip[n] {
			continue
		}
		var keep func(t float64) bool
		if a.sectionCuts(n) {
			keep = shown
		}
		if _, ok := r.Box(n.WorldBounds()); !ok {
			continue
		}
		for _, p := range n.LocalParts() {
			inv, ok := p.M.Inverse()
			if !ok {
				continue
			}
			lr := geom.Ray{O: inv.Point(r.O), D: inv.Dir(r.D)}
			t, ln, ok := lr.MeshWhere(p.Mesh, keep)
			if ok && t < best.t {
				nm := p.M.NormalMatrix().Dir(ln).Norm()
				if nm.Dot(r.D) > 0 {
					nm = nm.Neg()
				}
				best = hit{node: n, p: r.At(t), n: nm, t: t, mesh: p.Mesh, m: p.M, ln: ln}
			}
		}
	}
	return best, best.node != nil
}

// pickPoint returns the surface or workplane point under the cursor.
func (a *App) pickPoint(x, y float64) (geom.Vec3, bool) {
	if h, ok := a.pick(x, y, false, nil); ok {
		return h.p, true
	}
	if p, ok := a.planeHitW(x, y, 0); ok {
		return a.ed.Work().Point(p), true
	}
	return geom.Vec3{}, false
}

func (a *App) viewHover(x, y float64) {
	oldN, oldH := a.hoverNode, a.hoverHandle
	oldA, oldM := a.alignHover, a.mirrorHover
	a.hoverNode, a.hoverHandle = 0, -1
	a.alignHover, a.mirrorHover = nil, 0
	switch {
	case a.place != nil:
		a.updatePlace(x, y)
	case a.mode == modeAlign:
		a.alignHover = a.alignDotAt(x, y)
	case a.mode == modeMirror:
		a.mirrorHover = a.mirrorArrowAt(x, y)
	case a.mode == modeLayFlat:
		a.layHover = a.layFaceAt(x, y)
		a.w.Redraw()
	}
	if a.alignHover == nil && a.mirrorHover == 0 && a.place == nil {
		if i := a.handleAt(x, y); i >= 0 && a.mode == modeNone {
			a.hoverHandle = i
		} else if a.mode != modeMeasure && a.mode != modeRulerPlace {
			if h, ok := a.pick(x, y, a.mode == modeVisibility, nil); ok {
				a.hoverNode = h.node.ID
			}
		}
	}
	if oldN != a.hoverNode || oldH != a.hoverHandle || oldM != a.mirrorHover || (oldA == nil) != (a.alignHover == nil) ||
		(oldA != nil && a.alignHover != nil && *oldA != *a.alignHover) {
		a.w.Redraw()
	}
	a.w.SetCursor(a.viewCursor())
}

func (a *App) viewCursor() int {
	if a.drag != nil {
		switch a.drag.kind {
		case dragOrbit, dragPan, dragMove:
			return win.CursorSizeAll
		}
	}
	if !a.vp.Contains(a.mx, a.my) || (a.hot != "" && a.hot != "viewport") {
		return win.CursorArrow
	}
	if a.place != nil || a.mode == modeMeasure || a.mode == modeRulerPlace {
		return win.CursorCross
	}
	if a.alignHover != nil || a.mirrorHover > 0 {
		return win.CursorHand
	}
	if a.hoverHandle >= 0 && a.hoverHandle < len(a.handles) {
		h := a.handles[a.hoverHandle]
		switch h.kind {
		case hCorner:
			return win.CursorSizeNWSE
		case hEdge:
			return win.CursorSizeWE
		case hTop, hLift:
			return win.CursorSizeNS
		case hRot:
			return win.CursorHand
		}
	}
	if a.hoverNode != 0 {
		if a.mode == modeVisibility {
			return win.CursorHand
		}
		return win.CursorSizeAll
	}
	return win.CursorArrow
}

// --- press -------------------------------------------------------------------

func (a *App) viewPress(x, y float64, m win.Mods) {
	a.closePopup()
	ed := a.ed
	if a.mode == modeNotes && a.place == nil {
		if a.addNoteAt(x, y) {
			return
		}
	}
	if a.mode == modeLayFlat && a.place == nil {
		if a.layFlatClick(x, y) {
			return
		}
		a.setMode(modeNone) // a click off every shape leaves the tool
	}
	if a.mode == modeWorkplane && a.place == nil {
		// on a shape: the workplane goes onto it; elsewhere: back to the
		// default workplane
		id := 0
		if h, ok := a.pick(x, y, false, nil); ok {
			id = h.node.ID
		}
		ed.AttachWorkplane(id)
		a.setMode(modeNone)
		return
	}
	// a click anywhere else closes an open note
	a.closeNote()
	if a.place != nil {
		a.updatePlace(x, y)
		a.commitPlace()
		return
	}
	switch a.mode {
	case modeRulerPlace:
		if p, ok := a.planeHitW(x, y, 0); ok {
			ed.PlaceRuler(editorSnapXY(p, ed.Doc.Settings.Snap))
		}
		a.mode = modeNone
		return
	case modeMeasure:
		a.measureClick(x, y)
		return
	case modeVisibility:
		if h, ok := a.pick(x, y, true, nil); ok {
			ed.ToggleHidden(h.node.ID)
		}
		return
	case modeAlign:
		if d := a.alignDotAt(x, y); d != nil {
			ed.AlignTo(nil, d.axis, d.where, a.alignKey)
			return
		}
		// clicking one of the shapes being aligned makes it the key
		// object: the markers move to it and the others align to it
		if h, ok := a.pick(x, y, false, nil); ok && ed.IsSelected(h.node.ID) && len(ed.Sel) > 1 {
			if a.alignKey == h.node.ID {
				a.alignKey = 0
			} else {
				a.alignKey = h.node.ID
			}
			a.alignHover = nil
			a.w.Redraw()
			return
		}
		a.mode = modeNone
		a.alignKey = 0
	case modeMirror:
		if ax := a.mirrorArrowAt(x, y); ax > 0 {
			ed.Mirror(nil, ax-1)
			return
		}
		a.mode = modeNone
	case modeSplit:
		// clicking off the tool's controls leaves it, as Mirror does
		a.setMode(modeNone)
	}
	a.handles = a.computeHandles()
	if i := a.handleAt(x, y); i >= 0 {
		a.startHandleDrag(a.handles[i], x, y)
		return
	}
	h, ok := a.pick(x, y, false, nil)
	if !ok {
		a.drag = &dragState{kind: dragMarquee, sx: x, sy: y, lx: x, ly: y}
		return
	}
	id := h.node.ID
	wasSel := ed.IsSelected(id)
	if m&win.Shift != 0 {
		if !wasSel {
			ed.ToggleSelect(id)
		}
	} else if !wasSel {
		ed.Select(id)
	}
	a.drag = &dragState{kind: dragMove, sx: x, sy: y, lx: x, ly: y, grab: a.toW(h.p), clickNode: id, wasSelected: wasSel}
}

func (a *App) beginNodeDrag(d *dragState) {
	ns := a.ed.Editable()
	d.nodes = ns
	d.orig = map[*doc.Node]nodeState{}
	for _, n := range ns {
		st := nodeState{M: n.M, Size: n.Size}
		if n.Params != nil {
			st.Params = map[string]float64{}
			for k, v := range n.Params {
				st.Params[k] = v
			}
		}
		d.orig[n] = st
	}
	d.box = doc.BoundsIn(ns, a.ed.Work())
	a.ed.BeginEdit()
	a.rotKeep, a.dimPin, a.moveKeep = nil, nil, nil
}

func (d *dragState) restore() {
	for n, st := range d.orig {
		n.M, n.Size = st.M, st.Size
		if st.Params != nil {
			for k, v := range st.Params {
				n.Params[k] = v
			}
		}
	}
}

func (a *App) startHandleDrag(h handle, x, y float64) {
	d := &dragState{sx: x, sy: y, lx: x, ly: y, h: h}
	a.beginNodeDrag(d)
	if len(d.nodes) == 0 {
		a.ed.EndEdit()
		return
	}
	switch h.kind {
	case hCorner, hEdge:
		d.kind = dragResize
	case hTop:
		d.kind = dragResize
		d.s0 = a.rayW(x, y).ClosestOnLine(h.p, geom.V(0, 0, 1))
	case hLift:
		d.kind = dragLift
		d.s0 = a.rayW(x, y).ClosestOnLine(h.p, geom.V(0, 0, 1))
	case hRot:
		d.kind = dragRotate
		d.axis = h.idx
		d.center = d.box.Center()
		d.ringR = a.protractorRadius(d.box, d.axis)
		d.aStart, _ = a.planeAngle(x, y, d.center, d.axis)
		// 0° is the axis direction from the centre towards the handle
		u, v := rotBasis(d.axis)
		dir := h.p.Sub(d.center)
		d.a0 = math.Round(math.Atan2(dir.Dot(v), dir.Dot(u))/(math.Pi/2)) * (math.Pi / 2)
	}
	a.drag = d
}

// planeAngle intersects the cursor ray with the rotation plane and returns
// the angle around axis and the distance from the centre.
func (a *App) planeAngle(x, y float64, c geom.Vec3, axis int) (float64, float64) {
	r := a.rayW(x, y)
	n := geom.V(0, 0, 0).WithAxis(axis, 1)
	t, ok := r.Plane(c, n)
	if !ok {
		// plane edge-on or behind: use the line-of-sight projection
		t2, ok2 := r.Plane(c, n.Neg())
		if !ok2 {
			return 0, 0
		}
		t = t2
	}
	p := r.At(t).Sub(c)
	u, v := rotBasis(axis)
	return math.Atan2(p.Dot(v), p.Dot(u)), math.Hypot(p.Dot(u), p.Dot(v))
}

func (a *App) protractorRadius(b geom.Box, axis int) float64 {
	u, v := rotBasis(axis)
	s := b.Size()
	r := math.Hypot(math.Abs(s.Dot(u)), math.Abs(s.Dot(v))) / 2
	return r + 30*a.pixelSizeW(b.Center())
}

// --- drag ---------------------------------------------------------------------

func (a *App) snap() float64 { return a.ed.Doc.Settings.Snap }

func (a *App) dragMove(x, y float64, m win.Mods) {
	d := a.drag
	dx, dy := x-d.lx, y-d.ly
	d.lx, d.ly = x, y
	if math.Hypot(x-d.sx, y-d.sy) > 3 {
		d.moved = true
	}
	shift := m&win.Shift != 0
	alt := m&win.Alt != 0
	switch d.kind {
	case dragOrbit:
		a.orbit(dx, dy)
	case dragPan:
		a.pan(dx, dy)
	case dragCube:
		if d.moved {
			a.orbit(dx*1.6, dy*1.6)
		}
	case dragMarquee:
	case dragTile:
		a.updatePlace(x, y)
	case dragMove:
		if !d.moved {
			return
		}
		if d.orig == nil {
			if alt {
				d.dup = a.ed.Duplicate() != nil
			}
			a.beginNodeDrag(d)
		}
		a.applyMove(d, x, y, shift)
	case dragResize:
		a.applyResize(d, x, y, shift, alt)
	case dragLift:
		s := a.rayW(x, y).ClosestOnLine(d.h.p, geom.V(0, 0, 1))
		dz := editor.Snap(s-d.s0, a.snap())
		d.restore()
		doc.Move(d.nodes, a.ed.Work().Dir(geom.V(0, 0, dz)))
		d.value = d.box.Min.Z + dz
	case dragRotate:
		ang, dist := a.planeAngle(x, y, d.center, d.axis)
		delta := (ang - d.aStart) * 180 / math.Pi
		for delta > 180 {
			delta -= 360
		}
		for delta < -180 {
			delta += 360
		}
		// unwrap relative to the previous value so sweeps past 180° work
		for delta-d.angle > 180 {
			delta -= 360
		}
		for delta-d.angle < -180 {
			delta += 360
		}
		// 22.5° steps within the protractor, 1° steps once the cursor is out
		// past the band, towards the outer circle
		d.onRing = dist >= d.ringR*protBandOut
		step := 22.5
		if d.onRing {
			step = 1
		}
		if shift {
			step = 45
		}
		d.angle = math.Round(delta/step) * step
		d.restore()
		a.ed.RotateIn(d.nodes, d.axis, d.angle, d.center)
	}
}

func (a *App) applyMove(d *dragState, x, y float64, shift bool) {
	skip := map[*doc.Node]bool{}
	for _, n := range d.nodes {
		skip[n] = true
	}
	if a.cruise {
		if h, ok := a.pick(x, y, false, skip); ok {
			d.restore()
			base0 := a.ed.Work().Point(geom.V(d.box.Center().X, d.box.Center().Y, d.box.Min.Z))
			xf := geom.Translate(h.p).Mul(geom.RotateFromTo(geom.V(0, 0, 1), h.n)).Mul(geom.Translate(base0.Neg()))
			doc.Apply(d.nodes, xf)
			return
		}
	}
	p, ok := a.planeHitW(x, y, d.grab.Z)
	if !ok {
		return
	}
	delta := p.Sub(d.grab)
	delta.Z = 0
	if shift {
		if math.Abs(delta.X) > math.Abs(delta.Y) {
			delta.Y = 0
		} else {
			delta.X = 0
		}
	}
	s := a.snap()
	delta.X, delta.Y = editor.Snap(delta.X, s), editor.Snap(delta.Y, s)
	d.restore()
	doc.Move(d.nodes, a.ed.Work().Dir(delta))
	if a.cruise {
		// off any surface: rest on the workplane
		b := doc.BoundsIn(d.nodes, a.ed.Work())
		doc.Move(d.nodes, a.ed.Work().Dir(geom.V(0, 0, -b.Min.Z)))
	}
}

func (a *App) applyResize(d *dragState, x, y float64, shift, alt bool) {
	b := d.box
	s := a.snap()
	nb := b
	size := b.Size()
	c := b.Center()
	const minSize = 0.1
	switch d.h.kind {
	case hCorner, hEdge:
		p, ok := a.planeHitW(x, y, b.Min.Z)
		if !ok {
			return
		}
		p = editorSnapXY(p, s)
		// which sides move
		movX, movY := 0, 0 // -1 min side, +1 max side
		if d.h.kind == hCorner {
			movX = []int{-1, 1, 1, -1}[d.h.idx]
			movY = []int{-1, -1, 1, 1}[d.h.idx]
		} else {
			movX = []int{0, 1, 0, -1}[d.h.idx]
			movY = []int{-1, 0, 1, 0}[d.h.idx]
		}
		set := func(axis, mov int, v float64) {
			if mov == 0 {
				return
			}
			if alt {
				half := math.Max(minSize/2, math.Abs(v-c.Axis(axis)))
				nb.Min = nb.Min.WithAxis(axis, c.Axis(axis)-half)
				nb.Max = nb.Max.WithAxis(axis, c.Axis(axis)+half)
				return
			}
			if mov > 0 {
				nb.Max = nb.Max.WithAxis(axis, math.Max(v, b.Min.Axis(axis)+minSize))
			} else {
				nb.Min = nb.Min.WithAxis(axis, math.Min(v, b.Max.Axis(axis)-minSize))
			}
		}
		set(0, movX, p.X)
		set(1, movY, p.Y)
		if shift {
			// uniform: the largest ratio wins, applied to all three axes
			f := 1.0
			ns := nb.Size()
			if movX != 0 && size.X > 0 {
				f = ns.X / size.X
			}
			if movY != 0 && size.Y > 0 && (movX == 0 || math.Abs(ns.Y/size.Y-1) > math.Abs(f-1)) {
				f = ns.Y / size.Y
			}
			nb = scaleBoxFrom(b, f, movX, movY, alt)
		}
	case hTop:
		sv := a.rayW(x, y).ClosestOnLine(d.h.p, geom.V(0, 0, 1))
		h := math.Max(minSize, editor.Snap(size.Z+sv-d.s0, s))
		if alt {
			nb.Min.Z = c.Z - h/2
			nb.Max.Z = c.Z + h/2
		} else {
			nb.Max.Z = b.Min.Z + h
		}
		if shift && size.Z > 0 {
			f := h / size.Z
			nb = scaleBoxFrom(b, f, 0, 0, true)
			nb.Min.Z, nb.Max.Z = b.Min.Z, b.Min.Z+h
		}
	}
	d.restore()
	a.ed.SetBoundsLiveIn(d.nodes, nb)
}

// scaleBoxFrom scales b uniformly by f anchored at the side opposite the
// moving one (or the centre).
func scaleBoxFrom(b geom.Box, f float64, movX, movY int, center bool) geom.Box {
	s := b.Size().Mul(f)
	c := b.Center()
	anchor := geom.V(c.X, c.Y, b.Min.Z)
	if !center {
		if movX > 0 {
			anchor.X = b.Min.X
		} else if movX < 0 {
			anchor.X = b.Max.X
		}
		if movY > 0 {
			anchor.Y = b.Min.Y
		} else if movY < 0 {
			anchor.Y = b.Max.Y
		}
	}
	rel := func(axis int) (float64, float64) {
		lo := (b.Min.Axis(axis) - anchor.Axis(axis)) * f
		hi := (b.Max.Axis(axis) - anchor.Axis(axis)) * f
		return anchor.Axis(axis) + lo, anchor.Axis(axis) + hi
	}
	var nb geom.Box
	nb.Min.X, nb.Max.X = rel(0)
	nb.Min.Y, nb.Max.Y = rel(1)
	nb.Min.Z, nb.Max.Z = b.Min.Z, b.Min.Z+s.Z
	return nb
}

func (a *App) dragEnd(x, y float64, m win.Mods) {
	d := a.drag
	a.drag = nil
	switch d.kind {
	case dragOrbit, dragPan:
	case dragCube:
		if !d.moved {
			if yaw, pitch, ok := a.cubeFace(x, y); ok {
				c := a.cam
				c.Yaw, c.Pitch = yaw, pitch
				a.animateTo(c)
			}
		}
	case dragMarquee:
		if !d.moved {
			if m&win.Shift == 0 {
				a.ed.ClearSelection()
			}
			return
		}
		a.marqueeSelect(d, m&win.Shift != 0)
	case dragTile:
		if a.vp.Contains(x, y) && a.hot != "panel-area" {
			a.updatePlace(x, y)
			a.commitPlace()
		} else {
			a.place = nil
		}
	case dragMove:
		if d.orig == nil {
			// a click: shift toggles an already-selected shape off
			if m&win.Shift != 0 && d.wasSelected {
				a.ed.ToggleSelect(d.clickNode)
			} else if m&win.Shift == 0 && d.wasSelected && len(a.ed.Sel) > 1 {
				a.ed.Select(d.clickNode)
			}
			return
		}
		a.ed.EndEdit()
		a.ed.NoteMoved()
		if a.ed.Ruler == nil {
			a.moveKeep = &keptMove{from: d.box, sel: append([]int(nil), a.ed.Sel...), rev: a.ed.Version}
		}
	case dragRotate:
		a.ed.EndEdit()
		a.keepRotation(d)
	case dragResize, dragLift:
		// a click on a handle (or the end of its drag) keeps that
		// handle's sizes up for typing
		a.ed.EndEdit()
		a.dimPin = &dimPin{h: d.h, sel: append([]int(nil), a.ed.Sel...)}
	default:
		a.ed.EndEdit()
	}
	a.w.Redraw()
}

// keptRot is a finished rotation whose protractor stays up, with the
// angle editable, until the selection, the tool or the rotated shapes
// change.
type keptRot struct {
	d   *dragState
	sel []int
	rev int // the design's revision just after the rotation
}

func (a *App) keepRotation(d *dragState) {
	a.rotKeep = &keptRot{d: d, sel: append([]int(nil), a.ed.Sel...), rev: a.ed.Version}
}

// stillKept reports whether an overlay kept after an edit still applies:
// same tool, same selection and (if rev isn't -1) nothing changed in the
// design since revision rev. Any other edit (a parameter, an undo, which
// swaps every node for a copy) ends it: its saved state would be stale.
func (a *App) stillKept(sel []int, rev int) bool {
	if a.mode != modeNone || len(sel) != len(a.ed.Sel) || (rev >= 0 && rev != a.ed.Version) {
		return false
	}
	for i := range sel {
		if sel[i] != a.ed.Sel[i] {
			return false
		}
	}
	return true
}

// keptRotation returns the kept rotation if it still applies.
func (a *App) keptRotation() *keptRot {
	if k := a.rotKeep; k != nil && a.stillKept(k.sel, k.rev) {
		return k
	}
	a.rotKeep = nil
	return nil
}

// dimPin is a handle whose sizes stay up after it was clicked.
type dimPin struct {
	h   handle
	sel []int
}

func (a *App) pinnedDims() *dimPin {
	if p := a.dimPin; p != nil && a.stillKept(p.sel, -1) { // holds no design state
		return p
	}
	a.dimPin = nil
	return nil
}

// keptMove is a finished move whose distance bars stay up.
type keptMove struct {
	from geom.Box
	sel  []int
	rev  int
}

func (a *App) keptMove() *keptMove {
	if k := a.moveKeep; k != nil && a.stillKept(k.sel, k.rev) {
		return k
	}
	a.moveKeep = nil
	return nil
}

// setKeptAngle re-does the kept rotation at a typed angle (degrees,
// counter-clockwise looking down the axis), as one undo step.
func (a *App) setKeptAngle(deg float64) {
	k := a.keptRotation()
	if k == nil {
		return
	}
	d := k.d
	a.ed.BeginEdit()
	d.restore()
	d.angle = deg
	a.ed.RotateIn(d.nodes, d.axis, d.angle, d.center)
	a.ed.EndEdit()
	k.rev = a.ed.Version // this edit keeps it
	a.w.Redraw()
}

func (a *App) dragCancel() {
	d := a.drag
	a.drag = nil
	if d.orig != nil {
		d.restore()
		a.ed.CancelEdit()
	}
	if d.dup { // cancelling an Alt-drag takes its duplicates away too
		a.ed.Undo()
		a.ed.Hist.DropRedo()
	}
	if d.kind == dragTile {
		a.place = nil
	}
}

// marqueeSelect selects every shape the rectangle touches (any part of
// its surface on screen inside or crossing it).
func (a *App) marqueeSelect(d *dragState, add bool) {
	x0, x1 := math.Min(d.sx, d.lx), math.Max(d.sx, d.lx)
	y0, y1 := math.Min(d.sy, d.ly), math.Max(d.sy, d.ly)
	var ids []int
	if add {
		ids = append(ids, a.ed.Sel...)
	}
	for _, n := range a.ed.Doc.Nodes {
		if !n.Hidden && a.touchesRect(n, x0, y0, x1, y1) {
			ids = append(ids, n.ID)
		}
	}
	a.ed.Select(ids...)
}

// touchesRect reports whether any of n's triangles, on screen, overlaps
// the rectangle. With Section view cutting n, only what it shows counts.
func (a *App) touchesRect(n *doc.Node, x0, y0, x1, y1 float64) bool {
	cut := a.sectionCuts(n)
	// quick checks on the projected bounding box
	bx0, by0, bx1, by1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range n.WorldBounds().Corners() {
		sx, sy, ok := a.project(p)
		if !ok {
			bx0, by0, bx1, by1 = math.Inf(-1), math.Inf(-1), math.Inf(1), math.Inf(1) // partly behind the camera: test the triangles
			break
		}
		bx0, by0 = math.Min(bx0, sx), math.Min(by0, sy)
		bx1, by1 = math.Max(bx1, sx), math.Max(by1, sy)
	}
	if bx1 < x0 || bx0 > x1 || by1 < y0 || by0 > y1 {
		return false
	}
	if bx0 >= x0 && bx1 <= x1 && by0 >= y0 && by1 <= y1 && !cut {
		return true
	}
	for _, p := range n.LocalParts() {
		for _, t := range p.Mesh.Tris {
			var s [3][2]float64
			ok := true
			shown := !cut
			for i, v := range t.P {
				w := p.M.Point(v)
				shown = shown || a.sectionShows(w)
				x, y, vis := a.project(w)
				if !vis {
					ok = false
					break
				}
				s[i] = [2]float64{x, y}
			}
			if ok && shown && triTouchesRect(s, x0, y0, x1, y1) {
				return true
			}
		}
	}
	return false
}

// triTouchesRect reports whether a screen triangle overlaps a rectangle:
// a corner of either inside the other, or crossing edges.
func triTouchesRect(t [3][2]float64, x0, y0, x1, y1 float64) bool {
	in := func(p [2]float64) bool { return p[0] >= x0 && p[0] <= x1 && p[1] >= y0 && p[1] <= y1 }
	for _, p := range t {
		if in(p) {
			return true
		}
	}
	rc := [4][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
	cross := func(a, b, c [2]float64) float64 { return (b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0]) }
	inTri := func(p [2]float64) bool {
		d0, d1, d2 := cross(t[0], t[1], p), cross(t[1], t[2], p), cross(t[2], t[0], p)
		return (d0 >= 0 && d1 >= 0 && d2 >= 0) || (d0 <= 0 && d1 <= 0 && d2 <= 0)
	}
	for _, p := range rc {
		if inTri(p) {
			return true
		}
	}
	segs := func(p, q, r, s [2]float64) bool {
		return cross(p, q, r)*cross(p, q, s) < 0 && cross(r, s, p)*cross(r, s, q) < 0
	}
	for i := 0; i < 3; i++ {
		for j := 0; j < 4; j++ {
			if segs(t[i], t[(i+1)%3], rc[j], rc[(j+1)%4]) {
				return true
			}
		}
	}
	return false
}

// --- shape placement from the panel --------------------------------------------------

type placeState struct {
	kind string
	hole bool
	mesh *geom.Mesh
	m    geom.Mat4 // ghost transform
	xf   geom.Mat4 // placement transform (bottom centre at origin -> world)
	ok   bool
}

func newPlace(kind string, hole bool) *placeState {
	k, ok := shapes.Lookup(kind)
	if !ok {
		return nil
	}
	v := k.Defaults()
	size := k.Size
	if kind == "text" {
		size = shapes.TextNaturalSize(v.Text, v.Font, v.Num["height"])
	}
	col := geom.ParseColor(k.Color)
	if hole {
		col = geom.ParseColor(editor.HoleColor)
	}
	m := shapes.Build(kind, size, v).Colored(col)
	// local mesh is centred; lift so its base is at z=0
	return &placeState{kind: kind, hole: hole, mesh: m.Transform(geom.Translate(geom.V(0, 0, size.Z/2)))}
}

func (a *App) updatePlace(x, y float64) {
	p := a.place
	if p == nil {
		return
	}
	p.ok = false
	if !a.vp.Contains(x, y) {
		return
	}
	// shapes land on surfaces by default; hold C for the workplane
	if !a.keysDown['C'] {
		if h, ok := a.pick(x, y, false, nil); ok {
			p.xf = geom.Translate(h.p).Mul(geom.RotateFromTo(geom.V(0, 0, 1), h.n))
			p.m, p.ok = p.xf, true
			return
		}
	}
	q, ok := a.planeHitW(x, y, 0)
	if !ok {
		return
	}
	q = editorSnapXY(q, a.snap())
	p.xf = a.ed.Work().Mul(geom.Translate(q))
	p.m, p.ok = p.xf, true
}

func (a *App) commitPlace() {
	p := a.place
	a.place = nil
	if p == nil || !p.ok {
		return
	}
	if _, err := a.ed.AddShapeXf(p.kind, p.xf, p.hole); err != nil {
		a.showToast(err.Error())
	}
}

// --- measure tool --------------------------------------------------------------------

type measureState struct {
	stage int // 0 waiting for start, 1 waiting for end, 2 done
	a, b  geom.Vec3
	bNode int
}

func (a *App) measureClick(x, y float64) {
	h, onShape := a.pick(x, y, false, nil)
	var p geom.Vec3
	if onShape {
		p = h.p
	} else {
		q, ok := a.planeHitW(x, y, 0)
		if !ok {
			return
		}
		p = a.ed.Work().Point(q)
	}
	ms := &a.measure
	switch ms.stage {
	case 0, 2:
		*ms = measureState{stage: 1, a: p}
	case 1:
		ms.b = p
		ms.stage = 2
		ms.bNode = 0
		if onShape {
			ms.bNode = h.node.ID
		}
	}
}

// --- align / mirror widgets ------------------------------------------------------------

type alignDot struct {
	axis, where int
}

// alignHandle is an align dot: where it is on screen and the point on the
// box it belongs to (a short line joins them).
type alignHandle struct {
	d            alignDot
	x, y, ax, ay float64
}

// alignDots places the align handles around the box being aligned to, on
// the sides facing the camera (like the size lines): X handles along the
// near Y side, Y handles along the near X side, Z handles up the far-X,
// near-Y edge.
func (a *App) alignDots() []alignHandle {
	ns := a.ed.Selected()
	if len(ns) == 0 {
		return nil
	}
	b := editor.AlignBox(ns, a.alignKey, a.ed.Work())
	if b.IsEmpty() { // shapes that combine to nothing: nowhere to put the dots
		return nil
	}
	c := b.Center()
	ps := a.pixelSizeW(c)
	off := 22 * ps
	eye := a.eyeW()
	xn, sx := b.Min.X, -1.0
	if eye.X > c.X {
		xn, sx = b.Max.X, 1
	}
	yn, sy := b.Min.Y, -1.0
	if eye.Y > c.Y {
		yn, sy = b.Max.Y, 1
	}
	xf := b.Min.X + b.Max.X - xn
	var out []alignHandle
	add := func(axis, where int, anchor, dir geom.Vec3) {
		x, y, ok := a.projectW(anchor.Add(dir.Mul(off)))
		ax, ay, ok2 := a.projectW(anchor)
		if ok && ok2 {
			out = append(out, alignHandle{alignDot{axis, where}, x, y, ax, ay})
		}
	}
	vals := func(axis int) []float64 {
		return []float64{b.Min.Axis(axis), c.Axis(axis), b.Max.Axis(axis)}
	}
	for i, v := range vals(0) {
		add(0, i-1, geom.V(v, yn, b.Min.Z), geom.V(0, sy, 0))
	}
	for i, v := range vals(1) {
		add(1, i-1, geom.V(xn, v, b.Min.Z), geom.V(sx, 0, 0))
	}
	for i, v := range vals(2) {
		add(2, i-1, geom.V(xf, yn, v), geom.V(-sx, 0, 0))
	}
	return out
}

func (a *App) alignDotAt(x, y float64) *alignDot {
	for _, d := range a.alignDots() {
		if math.Hypot(d.x-x, d.y-y) < 9 {
			dd := d.d
			return &dd
		}
	}
	return nil
}

type mirrorArrow struct {
	axis           int
	x0, y0, x1, y1 float64
}

func (a *App) mirrorArrows() []mirrorArrow {
	ns := a.ed.Editable() // what the flip moves, about its centre
	if len(ns) == 0 {
		return nil
	}
	b := doc.BoundsIn(ns, a.ed.Work())
	if b.IsEmpty() {
		return nil
	}
	c := b.Center()
	ps := a.pixelSizeW(c)
	off := 24 * ps
	l := 16 * ps
	var out []mirrorArrow
	add := func(axis int, p, dir geom.Vec3) {
		x0, y0, ok0 := a.projectW(p.Sub(dir.Mul(l)))
		x1, y1, ok1 := a.projectW(p.Add(dir.Mul(l)))
		if ok0 && ok1 {
			out = append(out, mirrorArrow{axis, x0, y0, x1, y1})
		}
	}
	add(0, geom.V(c.X, b.Min.Y-off, b.Min.Z), geom.V(1, 0, 0))
	add(1, geom.V(b.Max.X+off, c.Y, b.Min.Z), geom.V(0, 1, 0))
	add(2, geom.V(b.Min.X-off, b.Min.Y-off, c.Z), geom.V(0, 0, 1))
	return out
}

func (a *App) mirrorArrowAt(x, y float64) int {
	for _, m := range a.mirrorArrows() {
		if distToSeg(x, y, m.x0, m.y0, m.x1, m.y1) < 9 {
			return m.axis + 1
		}
	}
	return 0
}

func distToSeg(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	l := dx*dx + dy*dy
	t := 0.0
	if l > 0 {
		t = math.Max(0, math.Min(1, ((px-x0)*dx+(py-y0)*dy)/l))
	}
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}

// rectAround is the square of half-size r centred on (x, y).
func rectAround(x, y, r float64) render.Rect {
	return render.Rect{X: x - r, Y: y - r, W: 2 * r, H: 2 * r}
}
