package app

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/gl"
	"github.com/scotticad/scotticad/pkg/render"
)

// The print check: whether each part will reach the slicer as a clean
// solid sitting on the plate, shown in the filament panel and returned by
// the print_check API method. Every message has to be true and say what to
// do: a warning on a design that prints fine teaches people to ignore the
// panel.
//
// The surface is checked as the exporters write it (see geom.AsWritten),
// part by part: a multicolour group is checked whole, since each colour's
// share of it is open by design. Placement is checked against the plate,
// whose size is the document's workplane.

const (
	// hairlineGap is the widest crack that slices as if it weren't there:
	// wider, it's a hole. Cubes with cracks round the top loaded without a
	// warning in Bambu Studio and PrusaSlicer at 0.001 to 0.2 mm; Bambu
	// Studio sliced them clean up to 0.15 mm, PrusaSlicer showed a flaw
	// from 0.1 mm.
	hairlineGap = 0.075
	// plateTol is how far a part's lowest point can be from the plate and
	// still count as on it.
	plateTol = 0.01
	// checkInline is the most triangles checked on the UI thread (about
	// 40 ms); when more need checking, they're checked in the background,
	// the panel saying "Checking…"
	checkInline = 50_000
	// checkCache is how many parts' checks are kept beyond the design's
	// own parts. Each key holds its mesh, so the cache is emptied when it's
	// full rather than keep every mesh the design has had
	checkCache = 128
	// overhangLow is how high above the plate a leaning face can reach and
	// still print without support, onto the first layers: a rounded or
	// chamfered bottom edge
	overhangLow = 1.0
)

const (
	sevNote    = "note"
	sevWarning = "warning"
)

// checkFinding is one thing the check found about a part.
type checkFinding struct {
	kind     string // hole, hairline, non-manifold, inside-out, below-plate, floating, mid-air, too-big, off-plate
	severity string
	message  string
	tip      string // what to do about it
	// the numbers behind it, where they apply
	open       int     // open edges
	gap        float64 // the widest gap, mm
	gapAtLeast bool    // the gap is at least that
	height     float64 // the lowest point's height above the plate, mm
}

// partCheck is the check of one top-level shape.
type partCheck struct {
	node     *doc.Node
	findings []checkFinding // warnings first
	overhang float64        // mm² of faces that may need support
}

// status is ok, note or warning: the worst finding.
func (p partCheck) status() string {
	if len(p.findings) == 0 {
		return "ok"
	}
	return p.findings[0].severity
}

type printCheckResult struct {
	parts   []partCheck
	pending bool // booleans or surface checks are still running
}

// surfaceKey is a part's surface: its local mesh (cached and shared, so
// an unchanged part has the same one) and where it is placed.
type surfaceKey struct {
	m  *geom.Mesh
	xf geom.Mat4
}

// surfaceCheck is the check of one part's mesh as written: its surface
// and its overhangs.
type surfaceCheck struct {
	geom.SurfaceReport
	over geom.OverhangReport
}

type printCheckState struct {
	key    string // what result was worked out for
	result printCheckResult

	mu       sync.Mutex
	surfaces map[surfaceKey]surfaceCheck
	parts    int  // how many parts the last check had: room is kept for them all
	running  bool // a background check is running

	// marked is the shape whose problem edges are drawn in the view, after
	// a click on a surface warning, until the selection or the design
	// changes
	marked markedEdges

	// the Export dialog's count of warnings (see exportWarnings), and
	// what it was worked out for
	exportKey   string
	exportCount int
}

// markedEdges are a shape's open and non-manifold edges, as written, and
// the design's revision they're of.
type markedEdges struct {
	id    int
	rev   int
	edges [][2]geom.Vec3
}

// surface is the cached check of k, if there is one.
func (s *printCheckState) surface(k surfaceKey) (surfaceCheck, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.surfaces[k]
	return r, ok
}

func (s *printCheckState) store(k surfaceKey, r surfaceCheck) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.surfaces == nil || len(s.surfaces) >= checkCache+s.parts {
		s.surfaces = map[surfaceKey]surfaceCheck{}
	}
	s.surfaces[k] = r
}

func checkSurface(k surfaceKey) surfaceCheck {
	m := geom.AsWritten(k.m.Transform(k.xf))
	return surfaceCheck{geom.Surface(m), geom.Overhangs(m, overhangLow)}
}

// checkShell is a piece of a part, for placement: the part (an index into
// the shapes checked) and its bounds.
type checkShell struct {
	node int
	box  geom.Box
}

// unsupported is how much of a flat underside rests on nothing: none on
// the plate; on the tops of pieces below it, less the area where their
// bounds and its own overlap. Faces can't rest on more than that, so it
// may say too little, never too much.
func unsupported(f geom.FlatOverhang, shells []checkShell) float64 {
	if math.Abs(f.Z) <= plateTol {
		return 0
	}
	area := f.Area
	for _, s := range shells {
		if math.Abs(s.box.Max.Z-f.Z) > plateTol {
			continue
		}
		w := math.Min(f.Bounds.Max.X, s.box.Max.X) - math.Max(f.Bounds.Min.X, s.box.Min.X)
		l := math.Min(f.Bounds.Max.Y, s.box.Max.Y) - math.Max(f.Bounds.Min.Y, s.box.Min.Y)
		if w > 0 && l > 0 {
			area -= w * l
		}
	}
	return math.Max(0, area)
}

// printCheckNow is the check for the panel, worked out again only when
// the design, the selection or the plate change. Big parts are checked in
// the background, and the result is pending until they're done.
func (a *App) printCheckNow() printCheckResult {
	c := &a.check
	st := a.ed.Doc.Settings
	key := fmt.Sprint(a.rev, a.ed.Sel, doc.Pending(), st.WorkplaneW, st.WorkplaneL, st.Units)
	if key != c.key {
		c.key, c.result = key, a.printCheck(a.filamentNodes(nil), true)
	}
	return c.result
}

// exportWarnings is how many warnings the print check has for what the
// Export dialog would export (the selection, or every visible shape), or
// -1 while big parts are still being checked. It never stops an export.
func (a *App) exportWarnings() int {
	c := &a.check
	sel := a.exportSel && len(a.ed.Sel) > 0
	st := a.ed.Doc.Settings
	key := fmt.Sprint(a.rev, sel, a.ed.Sel, doc.Pending(), st.WorkplaneW, st.WorkplaneL)
	if key != c.exportKey {
		scope := a.ed.Selected()
		if !sel {
			scope = nil
			for _, n := range a.ed.Doc.Nodes {
				if !n.Hidden {
					scope = append(scope, n)
				}
			}
		}
		r := a.printCheck(scope, true)
		c.exportKey, c.exportCount = key, 0
		if r.pending {
			c.exportCount = -1
		}
		for _, p := range r.parts {
			for _, f := range p.findings {
				if f.severity == sevWarning {
					c.exportCount++
				}
			}
		}
	}
	return c.exportCount
}

// drawExportWarnings draws the Export dialog's line about the print
// check's warnings, if there are any, at y; Show opens the print check.
func (a *App) drawExportWarnings(x, y float64) {
	n := a.exportWarnings()
	if n <= 0 {
		return
	}
	b := a.batch
	drawIcon(b, icWarning, render.Rect{X: x, Y: y, W: 14, H: 14}, colConError)
	text := "1 warning in the print check"
	if n > 1 {
		text = fmt.Sprintf("%d warnings in the print check", n)
	}
	tw := b.Text(text, x+20, y+7-b.LineHeight(fontSmall+0.5, false)/2, fontSmall+0.5, false, colText)
	sr := render.Rect{X: x + 28 + tw, Y: y - 4, W: 44, H: 22}
	sc := colBlue
	if a.isHot("exp-check") {
		sc = colIconHover
	}
	b.TextCentered("Show", sr, fontSmall+0.5, false, sc)
	a.button("exp-check", sr, "Open the print check, in the filament panel", "", tipBelow, func() {
		a.closePopup()
		a.fil.shown = true
		a.w.Redraw()
	})
}

// printCheck checks scope (top-level shapes). Every visible printed shape
// is looked at, since what's in scope may rest on what isn't. With
// background set, big uncached parts are checked on another goroutine and
// the result is pending; otherwise everything is checked now.
func (a *App) printCheck(scope []*doc.Node, background bool) printCheckResult {
	type part struct {
		node int // index into nodes
		key  surfaceKey
	}
	var nodes []*doc.Node
	var parts []part
	for _, n := range a.ed.Doc.Nodes {
		if n.Hidden || n.Hole {
			continue
		}
		for _, lp := range n.LocalParts() {
			if !lp.Hole {
				parts = append(parts, part{len(nodes), surfaceKey{lp.Mesh, lp.M}})
			}
		}
		nodes = append(nodes, n)
	}
	// LocalParts gives previews of groups still being combined (and
	// starts combining them)
	if background && doc.Pending() > 0 {
		return printCheckResult{pending: true}
	}
	c := &a.check
	c.mu.Lock()
	c.parts = len(parts)
	c.mu.Unlock()
	reports := make([]surfaceCheck, len(parts))
	cached := make([]bool, len(parts))
	uncached := 0 // triangles
	for i, p := range parts {
		reports[i], cached[i] = c.surface(p.key)
		if !cached[i] {
			uncached += len(p.key.m.Tris)
		}
	}
	var later []surfaceKey
	for i, p := range parts {
		switch {
		case cached[i]:
		case background && uncached > checkInline:
			later = append(later, p.key)
		default:
			reports[i] = checkSurface(p.key)
			c.store(p.key, reports[i])
		}
	}
	if len(later) > 0 {
		c.mu.Lock()
		start := !c.running
		c.running = true
		c.mu.Unlock()
		if start {
			go func() {
				for _, k := range later {
					c.store(k, checkSurface(k))
				}
				a.post(func() {
					c.mu.Lock()
					c.running = false
					c.mu.Unlock()
					c.key, c.exportKey = "", "" // work them out again, with these
					a.w.Redraw()
				})
			}()
		}
		return printCheckResult{pending: true}
	}

	// shells resting on the plate, and those resting on them, and so on;
	// "resting on" is bounding boxes touching, which can say a piece is
	// supported when it isn't, never the other way round
	var shells []checkShell
	for i, p := range parts {
		for _, s := range reports[i].Shells {
			if !s.Empty() { // a sheet prints as nothing, wherever it is
				shells = append(shells, checkShell{p.node, s.Bounds})
			}
		}
	}
	grounded := make([]bool, len(shells))
	var queue []int
	for i, s := range shells {
		if s.box.Min.Z <= plateTol {
			grounded[i] = true
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		g := shells[queue[0]].box
		queue = queue[1:]
		for i, s := range shells {
			if !grounded[i] && s.box.Overlaps(g, plateTol) {
				grounded[i] = true
				queue = append(queue, i)
			}
		}
	}

	inScope := map[*doc.Node]bool{}
	for _, n := range scope {
		inScope[n] = true
	}
	st := a.ed.Doc.Settings
	var out printCheckResult
	for ni, n := range nodes {
		if !inScope[n] {
			continue
		}
		var fs []checkFinding
		add := func(f checkFinding) { fs = append(fs, f) }

		// the surface, over the shape's parts
		var hole, hair, open, nonManifold, insideOut int
		var gap, hairGap float64
		gapAtLeast := false
		for i, p := range parts {
			if p.node != ni {
				continue
			}
			r := reports[i]
			open += r.Open
			nonManifold += r.NonManifold
			insideOut += r.InsideOut
			switch {
			case r.Open == 0:
			case r.Gap <= hairlineGap && !r.GapAtLeast:
				hair++
				hairGap = math.Max(hairGap, r.Gap)
			default:
				hole++
				if r.Gap > gap {
					gap, gapAtLeast = r.Gap, r.GapAtLeast
				}
			}
		}
		if hole > 0 {
			msg := fmt.Sprintf("a hole in the surface (%s)", fmtLen(gap, st.Units))
			switch {
			case gapAtLeast && gap == 0:
				msg = "a hole in the surface" // nothing across it to measure to
			case gapAtLeast:
				msg = fmt.Sprintf("a hole in the surface (at least %s)", fmtLen(gap, st.Units))
			}
			add(checkFinding{kind: "hole", severity: sevWarning, open: open, gap: gap, gapAtLeast: gapAtLeast,
				message: msg,
				tip:     "Slicers may not warn about it, and it can print as a flaw. Usually from an imported STL: try importing it again"})
		}
		if nonManifold > 0 {
			add(checkFinding{kind: "non-manifold", severity: sevWarning,
				message: "faces that don't join up (non-manifold edges)",
				tip:     "Usually from an imported STL: faces doubled, or a stray face on an edge. Try repairing it in your slicer"})
		}
		if insideOut > 0 {
			add(checkFinding{kind: "inside-out", severity: sevWarning,
				message: "inside out",
				tip:     "Imported inside out; ScottiCAD reads it as a hole. Fix it where it was made, or in your slicer"})
		}

		// placement
		b := geom.EmptyBox()
		placed, ground := false, false
		for i, s := range shells {
			if s.node == ni {
				b = b.Union(s.box)
				placed = true
				ground = ground || grounded[i]
			}
		}
		if placed {
			switch {
			case b.Min.Z < -plateTol:
				add(checkFinding{kind: "below-plate", severity: sevWarning, height: b.Min.Z,
					message: fmt.Sprintf("%s below the plate", fmtLen(-b.Min.Z, st.Units)),
					tip:     "The slicer cuts off what's below the plate, or lifts the part. Press D to drop it onto the plate"})
			case !ground:
				add(checkFinding{kind: "floating", severity: sevWarning, height: b.Min.Z,
					message: fmt.Sprintf("floating %s above the plate", fmtLen(b.Min.Z, st.Units)),
					tip:     "Press D to drop it onto the plate"})
			}
			s := b.Size()
			hw, hl := st.WorkplaneW/2, st.WorkplaneL/2
			plate := fmt.Sprintf("%s × %s mm", trimDecimals(st.WorkplaneW, 1), trimDecimals(st.WorkplaneL, 1))
			if st.Units == "in" {
				plate = fmt.Sprintf("%s × %s in", trimDecimals(st.WorkplaneW/25.4, 2), trimDecimals(st.WorkplaneL/25.4, 2))
			}
			switch {
			case s.X > st.WorkplaneW+plateTol || s.Y > st.WorkplaneL+plateTol:
				add(checkFinding{kind: "too-big", severity: sevWarning,
					message: "bigger than the " + plate + " plate",
					tip:     "Scale it down or split it. If your printer's bed is bigger, set its size in Settings"})
			case b.Min.X < -hw-plateTol || b.Max.X > hw+plateTol || b.Min.Y < -hl-plateTol || b.Max.Y > hl+plateTol:
				add(checkFinding{kind: "off-plate", severity: sevWarning,
					message: "partly off the plate",
					tip:     "Move it back onto the plate; the slicer may not place it for you"})
			}
		}

		// notes
		if hair > 0 && hole == 0 {
			add(checkFinding{kind: "hairline", severity: sevNote, open: open, gap: hairGap,
				message: "hairline cracks; these slice fine",
				tip:     "Too narrow to show in a print; nothing to do"})
		}
		if ground && placed {
			for i, s := range shells {
				if s.node == ni && !grounded[i] {
					add(checkFinding{kind: "mid-air", severity: sevNote, height: s.box.Min.Z,
						message: "a piece in mid-air (needs support)",
						tip:     "A piece resting on nothing prints on supports, or not at all. Lower it, or join it to the rest"})
					break
				}
			}
		}
		sort.SliceStable(fs, func(i, j int) bool { return fs[i].severity == sevWarning && fs[j].severity != sevWarning })

		// overhangs: leaning faces, and flat undersides resting on neither
		// the plate nor the top of another piece. Not of a part floating or
		// sunk: its warning comes first, and dropping it changes them
		over := 0.0
		for i, p := range parts {
			if p.node != ni || !ground || b.Min.Z < -plateTol {
				continue
			}
			over += reports[i].over.Sloped
			for _, f := range reports[i].over.Flat {
				over += unsupported(f, shells)
			}
		}
		out.parts = append(out.parts, partCheck{node: n, findings: fs, overhang: over})
	}
	// the worst first, in design order otherwise
	rank := map[string]int{sevWarning: 0, sevNote: 1, "ok": 2}
	sort.SliceStable(out.parts, func(i, j int) bool { return rank[out.parts[i].status()] < rank[out.parts[j].status()] })
	return out
}

// checkLine is a line of the filament panel's Print check section.
type checkLine struct {
	icon  int
	color render.Color
	text  string
	tip   string // what to do about it
	id    int    // the shape a click selects; 0 for none
	mark  bool   // a click also marks the shape's problem edges in the view
}

// checkMaxLines is the most findings the panel lists; the rest are
// counted on one more line.
const checkMaxLines = 6

// overhangShown is the least overhang area (mm²) the panel mentions: a
// fillet's underside or a few letters' worth isn't worth a line.
const overhangShown = 25.0

func checkLines(r printCheckResult, units string) []checkLine {
	if r.pending {
		return []checkLine{{icon: -1, color: colTextDim, text: "Checking…"}}
	}
	var ls []checkLine
	ok := 0
	over, overParts, overID := 0.0, 0, 0
	for _, p := range r.parts {
		if len(p.findings) == 0 {
			ok++
		}
		for _, f := range p.findings {
			l := checkLine{icon: icInfo, color: colIcon, text: p.node.Label() + ": " + f.message, tip: f.tip, id: p.node.ID,
				mark: f.kind == "hole" || f.kind == "non-manifold"}
			if f.severity == sevWarning {
				l.icon, l.color = icWarning, colConError
			}
			ls = append(ls, l)
		}
		if p.overhang > 0 {
			over += p.overhang
			overParts++
			overID = p.node.ID
		}
	}
	if len(ls) > checkMaxLines {
		more := len(ls) - (checkMaxLines - 1)
		ls = append(ls[:checkMaxLines-1], checkLine{icon: -1, color: colTextDim, text: fmt.Sprintf("and %d more", more)})
	}
	if len(ls) == 0 {
		switch ok {
		case 0:
			return []checkLine{{icon: -1, color: colTextDim, text: "Nothing to print"}}
		case 1:
			ls = []checkLine{{icon: icCheck, color: colBlue, text: "Ready to print"}}
		default:
			ls = []checkLine{{icon: icCheck, color: colBlue, text: fmt.Sprintf("%d parts ready to print", ok)}}
		}
	}
	// overhangs are information, never a finding: bridges and gentle
	// curves often print fine without support
	if over >= overhangShown {
		l := checkLine{icon: icInfo, color: colIcon, text: fmtArea(over, units) + " of overhangs (may need support)",
			tip: fmt.Sprintf("Faces leaning out more than %g° from upright, facing down. Short bridges and gentle curves often print without support; the slicer's preview shows where it adds it", geom.OverhangLimit)}
		if overParts == 1 {
			l.id = overID
		}
		ls = append(ls, l)
	}
	return ls
}

// fmtArea is an area (mm²) for a message, in the design's units.
func fmtArea(v float64, units string) string {
	switch {
	case units == "in":
		return fmt.Sprintf("%.2f in²", v/(25.4*25.4))
	case v < 100:
		return fmt.Sprintf("%.0f mm²", v)
	}
	return fmt.Sprintf("%.1f cm²", v/100)
}

// markEdges marks shape id's open and non-manifold edges, as the
// exporters write them, to be drawn over it in the view.
func (a *App) markEdges(id int) {
	a.check.marked = markedEdges{}
	n := a.ed.Doc.Find(id)
	if n == nil {
		return
	}
	var es [][2]geom.Vec3
	for _, lp := range n.LocalParts() {
		if !lp.Hole {
			es = append(es, geom.ProblemEdges(geom.AsWritten(lp.Mesh.Transform(lp.M)))...)
		}
	}
	a.check.marked = markedEdges{id: id, rev: a.rev, edges: es}
}

// markedNow is the marked edges, if they still apply: the shape is the
// selection, and the design hasn't changed since.
func (a *App) markedNow() [][2]geom.Vec3 {
	m := &a.check.marked
	if m.id == 0 {
		return nil
	}
	if m.rev != a.rev || len(a.ed.Sel) != 1 || a.ed.Sel[0] != m.id {
		*m = markedEdges{}
		return nil
	}
	return m.edges
}

// drawMarkedEdges draws the marked edges in red over everything: faint
// where something is in front of them, strong where nothing is.
func (a *App) drawMarkedEdges() {
	es := a.markedNow()
	if len(es) == 0 {
		return
	}
	var l render.Lines
	col := [4]float32{colConError[0], colConError[1], colConError[2], 1}
	for _, e := range es {
		l.Add(e[0], e[1], col)
	}
	gl.Disable(gl.DEPTH_TEST)
	a.scene.DrawLines(&l, 0.35)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LEQUAL)
	a.scene.DrawLines(&l, 1)
}

const checkLineH = 22.0

// printCheckHeight is how tall drawPrintCheck draws lines.
func printCheckHeight(lines []checkLine) float64 { return 30 + float64(len(lines))*checkLineH + 6 }

// drawPrintCheck draws the panel's Print check section at y.
func (a *App) drawPrintCheck(pr render.Rect, y float64, lines []checkLine) {
	b := a.batch
	px := pr.X + 16
	b.Fill(render.Rect{X: pr.X + 8, Y: y, W: pr.W - 16, H: 1}, colDivider)
	y += 10
	b.Text("Print check", px, y, fontUI, false, colText)
	y += 24
	for i, l := range lines {
		lr := render.Rect{X: pr.X + 8, Y: y - 3, W: pr.W - 16, H: checkLineH}
		id := ""
		if l.id != 0 {
			id = fmt.Sprintf("check-%d", i)
			if a.isHot(id) {
				b.RoundRect(lr, 3, colHoverBG)
			}
		}
		tx := px
		if l.icon >= 0 {
			drawIcon(b, l.icon, render.Rect{X: px, Y: lr.CY() - 7, W: 14, H: 14}, l.color)
			tx += 20
		}
		col := colText
		if l.icon < 0 {
			col = l.color
		}
		b.Text(elideEnd(b, l.text, lr.X+lr.W-8-tx, fontSmall+0.5, false), tx, lr.CY()-b.LineHeight(fontSmall+0.5, false)/2, fontSmall+0.5, false, col)
		if id != "" {
			sel, mark := l.id, l.mark
			a.button(id, lr, l.tip, "", tipBelow, func() {
				a.ed.Select(sel)
				if mark {
					a.markEdges(sel)
				}
				a.w.Redraw()
			})
		}
		y += checkLineH
	}
}

// fmtLen is a length (mm) for a message, in the design's units: two
// significant figures when it is small.
func fmtLen(v float64, units string) string {
	if units == "in" {
		if v /= 25.4; v < 0.1 {
			return fmt.Sprintf("%.2g in", v)
		}
		return fmt.Sprintf("%.2f in", v)
	}
	if v < 1 {
		return fmt.Sprintf("%.2g mm", v)
	}
	return fmt.Sprintf("%.1f mm", v)
}

// printCheckJSON is a check for the API.
func printCheckJSON(r printCheckResult) map[string]any {
	parts := []any{}
	for _, p := range r.parts {
		fs := []any{}
		for _, f := range p.findings {
			m := map[string]any{"kind": f.kind, "severity": f.severity, "message": f.message, "tip": f.tip}
			if f.open > 0 {
				m["open_edges"] = f.open
			}
			if f.kind == "hole" || f.kind == "hairline" {
				m["gap"] = f.gap
				if f.gapAtLeast {
					m["gap_at_least"] = true
				}
			}
			if f.kind == "floating" || f.kind == "below-plate" || f.kind == "mid-air" {
				m["height"] = f.height
			}
			fs = append(fs, m)
		}
		parts = append(parts, map[string]any{"id": p.node.ID, "name": p.node.Label(), "status": p.status(), "findings": fs,
			"overhang_area": math.Round(p.overhang*100) / 100})
	}
	return map[string]any{"pending": r.pending, "parts": parts}
}
