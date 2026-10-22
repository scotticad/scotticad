//go:build windows

package app

import (
	"math"
	"time"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/gl"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/win"
)

// --- camera ------------------------------------------------------------------

type camAnim struct {
	from, to render.Camera
	start    time.Time
	dur      time.Duration
}

func (a *App) animateTo(c render.Camera) {
	if a.headless {
		a.cam = c
		a.w.Redraw()
		return
	}
	// shortest yaw path
	for c.Yaw-a.cam.Yaw > math.Pi {
		c.Yaw -= 2 * math.Pi
	}
	for c.Yaw-a.cam.Yaw < -math.Pi {
		c.Yaw += 2 * math.Pi
	}
	a.anim = &camAnim{from: a.cam, to: c, start: time.Now(), dur: 300 * time.Millisecond}
	a.w.Animate = true
	a.w.Redraw()
}

func (a *App) stepAnim() {
	if a.anim == nil {
		return
	}
	t := float64(time.Since(a.anim.start)) / float64(a.anim.dur)
	if t >= 1 {
		a.cam = a.anim.to
		a.anim = nil
		a.w.Animate = false
		return
	}
	e := t * t * (3 - 2*t)
	f, to := a.anim.from, a.anim.to
	lerp := func(x, y float64) float64 { return x + (y-x)*e }
	a.cam.Target = f.Target.Lerp(to.Target, e)
	a.cam.Yaw = lerp(f.Yaw, to.Yaw)
	a.cam.Pitch = lerp(f.Pitch, to.Pitch)
	a.cam.Dist = math.Exp(lerp(math.Log(f.Dist), math.Log(to.Dist)))
	a.cam.Ortho = to.Ortho
}

// homeCamera is the home view, framed on the workplane.
func (a *App) homeCamera(animate bool) {
	c := render.DefaultCamera()
	s := a.ed.Doc.Settings
	c.Dist = 1.3 * math.Max(s.WorkplaneW, s.WorkplaneL)
	c.Target = geom.V(0.1*s.WorkplaneW, 0.02*s.WorkplaneL, 0)
	c.Ortho = a.cam.Ortho
	if a.cam.Fov == 0 {
		c.Ortho = false
	}
	if animate {
		a.animateTo(c)
	} else {
		a.cam = c
		a.w.Redraw()
	}
}

// The camera's distance limits, for zooming and fitting alike (a 10 m
// workplane fits well inside).
const (
	minDist = 5.0
	maxDist = 100000.0
)

// fitView frames the selection, or every visible shape.
func (a *App) fitView() {
	var ns []*doc.Node
	if len(a.ed.Sel) > 0 {
		ns = a.ed.Selected()
	} else {
		for _, n := range a.ed.Doc.Nodes {
			if !n.Hidden {
				ns = append(ns, n)
			}
		}
	}
	if len(ns) == 0 {
		a.homeCamera(true)
		return
	}
	b := doc.Bounds(ns)
	if b.IsEmpty() { // shapes that combine to nothing: nowhere to look
		a.homeCamera(true)
		return
	}
	r := math.Max(b.Size().Len()/2, 5)
	c := a.cam
	c.Target = b.Center()
	aspect := math.Max(0.2, math.Min(1, a.vp.W/a.vp.H))
	c.Dist = math.Max(minDist, math.Min(maxDist, r/math.Sin(c.Fov/2*aspect)*1.15))
	a.animateTo(c)
}

func (a *App) setOrtho(on bool) {
	c := a.cam
	c.Ortho = on
	a.animateTo(c)
}

// zoomAt scales the camera distance by f, keeping the point under (x, y)
// fixed.
func (a *App) zoomAt(x, y, f float64) {
	a.anim = nil
	p, ok := a.pickPoint(x, y)
	if !ok {
		p = a.cam.Target
	}
	nd := math.Max(minDist, math.Min(maxDist, a.cam.Dist*f))
	f = nd / a.cam.Dist
	a.cam.Target = p.Add(a.cam.Target.Sub(p).Mul(f))
	a.cam.Dist = nd
	a.w.Redraw()
}

func (a *App) startCameraDrag(btn int, x, y float64, m win.Mods) {
	a.anim = nil
	kind := dragOrbit
	if btn == win.Middle || m&win.Shift != 0 || m&win.Ctrl != 0 {
		kind = dragPan
	}
	a.drag = &dragState{kind: kind, sx: x, sy: y, lx: x, ly: y}
	a.w.SetCursor(win.CursorSizeAll)
}

func (a *App) orbit(dx, dy float64) {
	a.cam.Yaw -= dx * 0.0075
	a.cam.Pitch += dy * 0.0075
	a.cam.Pitch = math.Max(-math.Pi/2+0.01, math.Min(math.Pi/2-0.01, a.cam.Pitch))
}

func (a *App) pan(dx, dy float64) {
	ps := a.cam.PixelSize(a.cam.Target, a.vp.H)
	fwd := a.cam.Target.Sub(a.cam.Eye()).Norm()
	right := fwd.Cross(a.cam.Up()).Norm()
	up := right.Cross(fwd).Norm()
	a.cam.Target = a.cam.Target.Add(right.Mul(-dx * ps)).Add(up.Mul(dy * ps))
}

// --- projection helpers -----------------------------------------------------------

func (a *App) aspect() float64 { return a.vp.W / a.vp.H }

func (a *App) project(p geom.Vec3) (float64, float64, bool) {
	x, y, ok := a.cam.ToScreen(p, a.vp.W, a.vp.H)
	return x + a.vp.X, y + a.vp.Y, ok
}

func (a *App) ray(x, y float64) geom.Ray { return a.cam.Ray(x-a.vp.X, y-a.vp.Y, a.vp.W, a.vp.H) }

// pixelSize is the world size of one logical pixel at p.
func (a *App) pixelSize(p geom.Vec3) float64 { return a.cam.PixelSize(p, a.vp.H) }

// Workplane space: handles, sizes, the ruler and the protractor work in
// the active workplane's frame (z = 0 is the workplane, its axes the
// frame's). These are the frame versions of project, pixelSize, ray and
// planeHit; with the default workplane they are the same.

func (a *App) toW(p geom.Vec3) geom.Vec3 {
	inv, _ := a.ed.Work().Inverse()
	return inv.Point(p)
}

func (a *App) projectW(p geom.Vec3) (float64, float64, bool) { return a.project(a.ed.Work().Point(p)) }

func (a *App) pixelSizeW(p geom.Vec3) float64 { return a.pixelSize(a.ed.Work().Point(p)) }

func (a *App) rayW(x, y float64) geom.Ray {
	r := a.ray(x, y)
	inv, _ := a.ed.Work().Inverse()
	return geom.Ray{O: inv.Point(r.O), D: inv.Dir(r.D)}
}

func (a *App) planeHitW(x, y, z float64) (geom.Vec3, bool) {
	r := a.rayW(x, y)
	t, ok := r.PlaneZ(z)
	if !ok {
		return geom.Vec3{}, false
	}
	// too near the horizon: measured from the eye, as an orthographic
	// ray starts far behind it
	p := r.At(t)
	if p.Sub(a.eyeW()).Len() > a.cam.Dist*50 {
		return geom.Vec3{}, false
	}
	return p, true
}

// eyeW is the camera position in workplane coordinates.
func (a *App) eyeW() geom.Vec3 { return a.toW(a.cam.Eye()) }

// --- scene parts ---------------------------------------------------------------------

type scenePart struct {
	doc.LocalPart
	top *doc.Node
}

func (a *App) sceneParts() []scenePart {
	var out []scenePart
	for _, n := range a.ed.Doc.Nodes {
		if n.Hidden && a.mode != modeVisibility {
			continue
		}
		for _, p := range n.LocalParts() {
			out = append(out, scenePart{p, n})
		}
	}
	return out
}

// --- scene rendering ---------------------------------------------------------------

func (a *App) glViewport() (int32, int32, int32, int32) {
	d := a.w.DPI
	return int32(a.vp.X * d), int32((a.H - a.vp.Y - a.vp.H) * d), int32(a.vp.W * d), int32(a.vp.H * d)
}

func (a *App) drawScene() {
	vx, vy, vw, vh := a.glViewport()
	gl.Viewport(vx, vy, vw, vh)
	sc := a.scene
	sc.Begin(a.cam.VP(a.aspect()), a.cam.Eye())
	sc.PixelScale = float32(a.w.DPI)
	s := a.ed.Doc.Settings
	parts := a.sceneParts()

	// soft shadows on the workplane (not seen from underneath)
	hw, hl := s.WorkplaneW/2, s.WorkplaneL/2
	srect := [4]float64{-hw * 1.5, -hl * 1.5, hw * 3, hl * 3}
	below := a.cam.Eye().Z < 0
	shadows := s.ShowShadows && !below
	if shadows {
		var sp []render.ShadowPart
		for _, p := range parts {
			if !p.top.Hidden {
				sp = append(sp, render.ShadowPart{Mesh: p.Mesh, M: p.M, Light: p.Hole})
			}
		}
		if a.place != nil && a.place.ok {
			sp = append(sp, render.ShadowPart{Mesh: a.place.mesh, M: a.place.m, Light: a.place.hole})
		}
		sc.RenderShadows(sp, srect, a.target.FBO, vx, vy, vw, vh)
	}

	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LEQUAL)
	gl.DepthMask(true)
	gl.Disable(gl.BLEND)
	gl.Disable(gl.CULL_FACE)

	// workplane: translucent sheet, grid, border band, watermarks
	drawPlane := func() {
		gl.Enable(gl.BLEND)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		gl.DepthMask(false)
		sc.DrawPlane(-hw, -hl, hw, hl, 0, colPlane, srect, shadows)
		var lines render.Lines
		if s.ShowGrid {
			minor, major := 1.0, 10.0
			if s.Units == "in" {
				minor, major = 25.4/8, 25.4
			}
			z := 0.02
			if below {
				z = -0.02
			}
			a.gridLines(&lines, hw, hl, z, minor, major)
		}
		sc.DrawLines(&lines, 1)
		sc.DrawTris(a.planeBorder(hw, hl), 1)
		sc.DrawTris(a.watermarkTris(hw, hl), 1)
	}
	// seen from underneath, the sheet is a backdrop: drawn first and
	// without depth, so shapes resting on it show in full and their bases
	// don't fight with it
	if below {
		gl.Disable(gl.DEPTH_TEST)
		drawPlane()
		gl.Enable(gl.DEPTH_TEST)
		gl.DepthMask(true)
		gl.Disable(gl.BLEND)
	}

	// draw draws a part: cut by Section view if it is on, or as its halves
	// while the Split tool is open. solid parts show their cut faces.
	section, sectionOn := a.sectionClip()
	draw := func(p scenePart, st render.Style, solid bool) {
		hs := a.splitHalves(p.top)
		if hs == nil {
			if sectionOn && a.sectionCuts(p.top) {
				st.Clip, st.Cap = section, solid
			}
			sc.DrawMesh(p.Mesh, st)
			return
		}
		for _, h := range hs {
			hst := st
			hst.Model = h.m.Mul(st.Model)
			hst.Clip, hst.Cap = h.clip, solid
			sc.DrawMesh(p.Mesh, hst)
		}
	}

	// opaque solids
	for _, p := range parts {
		if p.Hole || p.Transparent || p.top.Hidden {
			continue
		}
		draw(p, render.Style{Model: p.M}, true)
	}

	if !below {
		drawPlane()
	} else {
		gl.Enable(gl.BLEND)
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		gl.DepthMask(false)
	}
	if a.ed.Workplane != nil {
		// the active workplane: a green sheet with a border band and a
		// coarse grid, in its own frame
		f := *a.ed.Workplane
		e := math.Max(hw, hl) * 0.5 // 100 mm across on the default 200 mm bed
		t := e * 0.04
		edge := render.Color{colShapeWP[0], colShapeWP[1], colShapeWP[2], 0.6}
		var d []float32
		d = appendQuad(d, geom.V(-e, -e, 0), geom.V(e, -e, 0), geom.V(e, e, 0), geom.V(-e, e, 0), colShapeWP)
		d = appendQuad(d, geom.V(-e-t, -e-t, 0.01), geom.V(e+t, -e-t, 0.01), geom.V(e, -e, 0.01), geom.V(-e, -e, 0.01), edge)
		d = appendQuad(d, geom.V(e, -e, 0.01), geom.V(e+t, -e-t, 0.01), geom.V(e+t, e+t, 0.01), geom.V(e, e, 0.01), edge)
		d = appendQuad(d, geom.V(-e, e, 0.01), geom.V(e, e, 0.01), geom.V(e+t, e+t, 0.01), geom.V(-e-t, e+t, 0.01), edge)
		d = appendQuad(d, geom.V(-e-t, -e-t, 0.01), geom.V(-e, -e, 0.01), geom.V(-e, e, 0.01), geom.V(-e-t, e+t, 0.01), edge)
		sc.DrawTrisM(d, 1, f)
		var wl render.Lines
		a.gridLines(&wl, e, e, 0.02, 10, 50)
		sc.DrawLinesM(&wl, 0.35, f)
	}

	// translucent: holes, transparent solids, hidden shapes in visibility mode
	for _, p := range parts {
		switch {
		case p.top.Hidden:
			sc.DrawMesh(p.Mesh, render.Style{Model: p.M, Tint: [4]float32{0.55, 0.78, 0.98, 0.85}, Alpha: 0.35})
		case p.Hole:
			draw(p, holeStyle(p.M, 1), false)
		case p.Transparent:
			draw(p, render.Style{Model: p.M, Alpha: 0.42}, true)
		}
	}
	a.drawSplitPlane()
	a.drawSectionPlane()
	a.drawLayFace()
	// previews: hole hover, align/mirror ghosts, tile ghost
	a.drawGhosts()
	a.drawAlignExtent()
	gl.DepthMask(true)

	// outlines
	var mask []render.MaskPart
	hovered := a.hoverNode
	for _, p := range parts {
		if p.top.Hidden {
			continue
		}
		ch := -1
		if a.ed.IsSelected(p.top.ID) {
			ch = 0
			if p.top.IsBundle() {
				ch = 1
			}
		} else if p.top.ID == hovered && a.drag == nil {
			ch = 2
		}
		if ch >= 0 && a.splitHalves(p.top) == nil { // halves being previewed get no outline
			mp := render.MaskPart{Mesh: p.Mesh, M: p.M, Channel: ch}
			if a.sectionCuts(p.top) {
				mp.Clip = section
			}
			mask = append(mask, mp)
		}
	}
	if len(mask) > 0 {
		// the selection: a 3 px line and a soft glow beyond it; the hover
		// a thinner line, so the selection always reads stronger
		dpi := a.w.DPI
		sc.DrawOutlines(mask, a.target.FBO, vx, vy, vw, vh, float32(math.Max(3, 3*dpi)), float32(math.Max(2, 2.2*dpi)),
			float32(math.Max(9, 9*dpi)), 0.6, colSelect, colBundleChild, colHover)
	}

	a.drawViewCube()
	gl.Disable(gl.DEPTH_TEST)
}

func (a *App) gridLines(l *render.Lines, hw, hl, z, minor, major float64) {
	cm := [4]float32{colGridMinor[0], colGridMinor[1], colGridMinor[2], colGridMinor[3]}
	cM := [4]float32{colGridMajor[0], colGridMajor[1], colGridMajor[2], colGridMajor[3]}
	// lines from the centre outward so major lines stay on round numbers
	ps := a.pixelSize(a.cam.Target)
	showMinor := minor/ps > 2.5
	for i := 0; ; i++ {
		v := float64(i) * minor
		if v > hw+1e-9 {
			break
		}
		maj := math.Abs(math.Mod(v+1e-9, major)) < 1e-6
		if !maj && !showMinor {
			continue
		}
		c := cm
		if maj {
			c = cM
		}
		for _, x := range []float64{v, -v} {
			l.Add(geom.V(x, -hl, z), geom.V(x, hl, z), c)
			if v == 0 {
				break
			}
		}
	}
	for i := 0; ; i++ {
		v := float64(i) * minor
		if v > hl+1e-9 {
			break
		}
		maj := math.Abs(math.Mod(v+1e-9, major)) < 1e-6
		if !maj && !showMinor {
			continue
		}
		c := cm
		if maj {
			c = cM
		}
		for _, y := range []float64{v, -v} {
			l.Add(geom.V(-hw, y, z), geom.V(hw, y, z), c)
			if v == 0 {
				break
			}
		}
	}
}

func appendQuad(d []float32, p0, p1, p2, p3 geom.Vec3, c render.Color) []float32 {
	for _, p := range []geom.Vec3{p0, p1, p2, p0, p2, p3} {
		d = append(d, float32(p.X), float32(p.Y), float32(p.Z), c[0], c[1], c[2], c[3])
	}
	return d
}

// planeBorder is the translucent band around the workplane.
func (a *App) planeBorder(hw, hl float64) []float32 {
	t := math.Max(hw, hl) * 0.025
	z := 0.03
	c := colPlaneEdge
	var d []float32
	d = appendQuad(d, geom.V(-hw-t, -hl-t, z), geom.V(hw+t, -hl-t, z), geom.V(hw, -hl, z), geom.V(-hw, -hl, z), c)
	d = appendQuad(d, geom.V(hw, -hl, z), geom.V(hw+t, -hl-t, z), geom.V(hw+t, hl+t, z), geom.V(hw, hl, z), c)
	d = appendQuad(d, geom.V(-hw, hl, z), geom.V(hw, hl, z), geom.V(hw+t, hl+t, z), geom.V(-hw-t, hl+t, z), c)
	d = appendQuad(d, geom.V(-hw-t, -hl-t, z), geom.V(-hw, -hl, z), geom.V(-hw, hl, z), geom.V(-hw-t, hl+t, z), c)
	return d
}

// flatText triangulates text into the XY plane: baseline at y=0, starting
// at x=0, sheared for italics.
func flatText(s string, em, shear float64) ([][3]shapes.Pt, float64) {
	cs := shapes.TextContours(s, "Sans Bold", em)
	var tris [][3]shapes.Pt
	maxX := 0.0
	for _, r := range shapes.Regions(cs) {
		for _, t := range shapes.Triangulate(r) {
			for i := range t {
				t[i].X += t[i].Y * shear
				maxX = math.Max(maxX, t[i].X)
			}
			tris = append(tris, t)
		}
	}
	return tris, maxX
}

func (a *App) watermarkTris(hw, hl float64) []float32 {
	// the colour is baked in: a theme change makes it anew
	key := editor.Format(hw, "mm", true) + editor.Format(hl, "mm", true) + currentTheme.Name
	if key == a.watermarkKey {
		return a.watermark
	}
	em := math.Min(hw, hl) * 0.2
	z := 0.04
	c := colWatermark
	var d []float32
	add := func(tris [][3]shapes.Pt, ox, oy float64) {
		for _, t := range tris {
			for _, p := range t {
				d = append(d, float32(p.X+ox), float32(p.Y+oy), float32(z), c[0], c[1], c[2], c[3])
			}
		}
	}
	t1, _ := flatText("Build plate", em, 0.22)
	add(t1, -hw+em*0.25, -hl+em*0.3)
	a.watermark, a.watermarkKey = d, key
	return d
}

// --- view cube --------------------------------------------------------------------

var cubeMesh = func() *geom.Mesh {
	return shapes.Build("box", geom.V(2, 2, 2), shapes.Values{Num: map[string]float64{"radius": 0, "steps": 1}}).Colored(geom.Color{R: 0.93, G: 0.94, B: 0.95})
}()

func (a *App) cubeRect() render.Rect {
	return render.Rect{X: a.vp.X + leftColX - 6, Y: a.vp.Y + 10, W: viewCube + 12, H: viewCube + 12}
}

func (a *App) cubeCamera() render.Camera {
	c := a.cam
	c.Target = geom.Vec3{}
	c.Dist = 7.2
	c.Ortho = false
	c.Fov = 30 * math.Pi / 180
	return c
}

func (a *App) drawViewCube() {
	r := a.cubeRect()
	d := a.w.DPI
	gl.Viewport(int32(r.X*d), int32((a.H-r.Y-r.H)*d), int32(r.W*d), int32(r.H*d))
	gl.Clear(gl.DEPTH_BUFFER_BIT)
	c := a.cubeCamera()
	saved, savedEye := a.scene.VP, a.scene.Eye
	a.scene.VP, a.scene.Eye = c.VP(1), c.Eye()
	tint := [4]float32{}
	if a.isHot("viewcube") || (a.drag != nil && a.drag.kind == dragCube) {
		tint = [4]float32{0.80, 0.90, 1.0, 0.5}
	}
	gl.Enable(gl.DEPTH_TEST)
	a.scene.DrawMesh(cubeMesh, render.Style{Edges: true, Tint: tint})
	gl.Disable(gl.DEPTH_TEST)
	if a.cubeLabels == nil {
		a.cubeLabels = buildCubeLabels()
	}
	gl.Enable(gl.DEPTH_TEST)
	a.scene.DrawTris(a.cubeLabels, 1)
	a.scene.VP, a.scene.Eye = saved, savedEye
	vx, vy, vw, vh := a.glViewport()
	gl.Viewport(vx, vy, vw, vh)
}

// buildCubeLabels makes TOP/FRONT/... text lying on the cube faces.
func buildCubeLabels() []float32 {
	type face struct {
		label   string
		o, u, v geom.Vec3 // centre, text right, text up
	}
	faces := []face{
		{"TOP", geom.V(0, 0, 1.005), geom.V(1, 0, 0), geom.V(0, 1, 0)},
		{"BOTTOM", geom.V(0, 0, -1.005), geom.V(1, 0, 0), geom.V(0, -1, 0)},
		{"FRONT", geom.V(0, -1.005, 0), geom.V(1, 0, 0), geom.V(0, 0, 1)},
		{"BACK", geom.V(0, 1.005, 0), geom.V(-1, 0, 0), geom.V(0, 0, 1)},
		{"RIGHT", geom.V(1.005, 0, 0), geom.V(0, 1, 0), geom.V(0, 0, 1)},
		{"LEFT", geom.V(-1.005, 0, 0), geom.V(0, -1, 0), geom.V(0, 0, 1)},
	}
	var d []float32
	c := render.RGB(0x6b, 0x77, 0x84)
	for _, f := range faces {
		em := 0.62
		tris, w := flatText(f.label, em, 0)
		if w > 1.6 {
			em *= 1.6 / w
			tris, w = flatText(f.label, em, 0)
		}
		for _, t := range tris {
			for _, p := range t {
				x := p.X - w/2
				y := p.Y - em*0.36
				q := f.o.Add(f.u.Mul(x)).Add(f.v.Mul(y))
				d = append(d, float32(q.X), float32(q.Y), float32(q.Z), c[0], c[1], c[2], 1)
			}
		}
	}
	return d
}

// cubeFace returns the view for the cube face under (x, y).
func (a *App) cubeFace(x, y float64) (yaw, pitch float64, ok bool) {
	r := a.cubeRect()
	c := a.cubeCamera()
	ray := c.Ray(x-r.X, y-r.Y, r.W, r.H)
	t, ok := ray.Box(geom.Box{Min: geom.V(-1, -1, -1), Max: geom.V(1, 1, 1)})
	if !ok {
		return 0, 0, false
	}
	p := ray.At(t)
	switch {
	case p.Z > 0.99:
		return a.cam.Yaw, math.Pi/2 - 0.001, true
	case p.Z < -0.99:
		return a.cam.Yaw, -math.Pi/2 + 0.001, true
	case p.Y < -0.99:
		return -math.Pi / 2, 0, true
	case p.Y > 0.99:
		return math.Pi / 2, 0, true
	case p.X > 0.99:
		return 0, 0, true
	case p.X < -0.99:
		return math.Pi, 0, true
	}
	return 0, 0, false
}

// --- ghosts ------------------------------------------------------------------------

func (a *App) drawGhosts() {
	sc := a.scene
	// tile being placed
	if a.place != nil && a.place.ok {
		st := render.Style{Model: a.place.m, Alpha: 0.55}
		if a.place.hole {
			st = holeStyle(a.place.m, 0.8)
		}
		sc.DrawMesh(a.place.mesh, st)
	}
	// hovering the Hole disc previews the selection as a hole
	if a.hot == "insp-hole" {
		for _, n := range a.ed.Editable() { // what SetHole changes
			if n.Hole {
				continue
			}
			for _, p := range n.LocalParts() {
				sc.DrawMesh(p.Mesh, holeStyle(p.M, 1))
			}
		}
	}
	// align preview
	if a.mode == modeAlign && a.alignHover != nil {
		ns := a.ed.Editable() // what moves
		ds := a.alignDeltas(ns, a.alignHover.axis, a.alignHover.where)
		for i, n := range ns {
			for _, p := range n.LocalParts() {
				sc.DrawMesh(p.Mesh, render.Style{Model: geom.Translate(ds[i]).Mul(p.M), Tint: colGhostAlign, Alpha: 0.45})
			}
		}
	}
	// mirror preview
	if a.mode == modeMirror && a.mirrorHover > 0 {
		ns := a.ed.Editable() // what flips (about its own centre)
		xf := a.ed.MirrorTransform(ns, a.mirrorHover-1)
		for _, n := range ns {
			for _, p := range n.LocalParts() {
				sc.DrawMesh(p.Mesh, render.Style{Model: xf.Mul(p.M), Tint: [4]float32{0.6, 0.6, 0.65, 0.3}, Alpha: 0.45})
			}
		}
	}
}

// holeStyle is how holes look: a light, very translucent grey with fine
// hairline stripes, darker inner faces and a thin dark outline on rims and
// silhouettes. fade scales the opacity (ghosts).
func holeStyle(m geom.Mat4, fade float32) render.Style {
	return render.Style{
		Model:   m,
		Tint:    colHoleTint,
		Alpha:   colHoleAlpha * fade,
		Stripes: true,
		Outline: [4]float32{colHoleEdge[0], colHoleEdge[1], colHoleEdge[2], colHoleEdge[3] * fade},
	}
}

func (a *App) alignDeltas(ns []*doc.Node, axis, where int) []geom.Vec3 {
	return a.ed.AlignPreview(ns, axis, where, a.alignKey)
}

// drawAlignExtent shows, while aligning, the extent of everything being
// aligned as a lightly filled, dashed box on the workplane; it is part of
// the scene, so the shapes hide it where they stand on it.
func (a *App) drawAlignExtent() {
	if a.mode != modeAlign {
		return
	}
	ns := a.ed.Selected()
	if len(ns) == 0 {
		return
	}
	f := a.ed.Work()
	b := doc.BoundsIn(ns, f)
	if b.IsEmpty() {
		return
	}
	const z = 0.04
	c0, c1, c2, c3 := geom.V(b.Min.X, b.Min.Y, z), geom.V(b.Max.X, b.Min.Y, z), geom.V(b.Max.X, b.Max.Y, z), geom.V(b.Min.X, b.Max.Y, z)
	sc := a.scene
	sc.DrawTrisM(appendQuad(nil, c0, c1, c2, c3, colDimBand.A(0.35)), 1, f)
	dash := 5 * a.pixelSizeW(b.Center())
	var l render.Lines
	col := [4]float32{colDimLine[0], colDimLine[1], colDimLine[2], 0.9}
	for _, e := range [][2]geom.Vec3{{c0, c1}, {c1, c2}, {c2, c3}, {c3, c0}} {
		d := e[1].Sub(e[0])
		// at most 400 dashes an edge: a big part seen close up would
		// otherwise ask for millions (longer dashes then, mostly off screen)
		v := d.Len() / dash
		if !(v < 400) {
			v = 400
		}
		n := int(v)
		for i := 0; i < n; i += 2 {
			l.Add(e[0].Add(d.Mul(float64(i)/float64(n))), e[0].Add(d.Mul(float64(min(i+1, n))/float64(n))), col)
		}
	}
	sc.DrawLinesM(&l, 1, f)
}
