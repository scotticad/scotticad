package app

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
)

// cube is an axis-aligned box of size s, its corner at the origin.
func cube(s geom.Vec3) *geom.Mesh {
	p := [8]geom.Vec3{}
	for i := range p {
		p[i] = geom.V(float64(i&1)*s.X, float64(i>>1&1)*s.Y, float64(i>>2&1)*s.Z)
	}
	q := [][4]int{{0, 2, 3, 1}, {4, 5, 7, 6}, {0, 1, 5, 4}, {2, 6, 7, 3}, {0, 4, 6, 2}, {1, 3, 7, 5}}
	m := &geom.Mesh{}
	for _, f := range q {
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{p[f[0]], p[f[1]], p[f[2]]}}, geom.Tri{P: [3]geom.Vec3{p[f[0]], p[f[2]], p[f[3]]}})
	}
	return m
}

func checkApp() *App { return &App{ed: editor.New()} }

// add puts a 10 mm cube (or m) with its bottom centre at pos.
func add(t *testing.T, a *App, pos geom.Vec3, m *geom.Mesh) int {
	t.Helper()
	if m == nil {
		m = cube(geom.V(10, 10, 10))
	}
	id, err := a.ed.AddMesh("Part", m, pos)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// kinds is what the check of the whole design finds, by shape.
func kinds(a *App) map[int][]string {
	out := map[int][]string{}
	for _, p := range a.printCheck(a.ed.Doc.Nodes, false).parts {
		for _, f := range p.findings {
			out[p.node.ID] = append(out[p.node.ID], f.kind)
		}
	}
	return out
}

func TestPrintCheckPlacement(t *testing.T) {
	a := checkApp()
	ok := add(t, a, geom.V(0, 0, 0), nil)
	floating := add(t, a, geom.V(30, 0, 2), nil)
	sunk := add(t, a, geom.V(-30, 0, -3), nil)
	off := add(t, a, geom.V(100, 0, 0), nil) // the 200 mm plate ends at 100
	big := add(t, a, geom.V(0, 40, 0), cube(geom.V(250, 10, 10)))
	got := kinds(a)
	for id, want := range map[int]string{floating: "floating", sunk: "below-plate", off: "off-plate", big: "too-big"} {
		if len(got[id]) != 1 || got[id][0] != want {
			t.Errorf("shape %d: %v, want [%s]", id, got[id], want)
		}
	}
	if len(got[ok]) != 0 {
		t.Errorf("a cube on the plate: %v", got[ok])
	}
	// dropped, it's fine
	a.ed.Drop([]int{floating})
	if got := kinds(a); len(got[floating]) != 0 {
		t.Errorf("dropped onto the plate: %v", got[floating])
	}
	// a bigger plate fits it
	a.ed.Doc.Settings.WorkplaneW = 300
	if got := kinds(a); len(got[big]) != 0 {
		t.Errorf("on a 300 mm plate: %v", got[big])
	}
}

// Messages give lengths in the design's units.
func TestPrintCheckMessagesFollowTheUnits(t *testing.T) {
	a := checkApp()
	up := add(t, a, geom.V(0, 0, 25.4), nil)
	big := add(t, a, geom.V(0, 40, 0), cube(geom.V(250, 10, 10)))
	messages := func() map[int]string {
		out := map[int]string{}
		for _, p := range a.printCheck(a.ed.Doc.Nodes, false).parts {
			for _, f := range p.findings {
				out[p.node.ID] = f.message
			}
		}
		return out
	}
	if m := messages(); m[up] != "floating 25.4 mm above the plate" || m[big] != "bigger than the 200 × 200 mm plate" {
		t.Errorf("in millimetres: %q, %q", m[up], m[big])
	}
	a.ed.Doc.Settings.Units = "in"
	a.ed.Doc.Settings.WorkplaneW, a.ed.Doc.Settings.WorkplaneL = 8*25.4, 7.5*25.4
	if m := messages(); m[up] != "floating 1.00 in above the plate" || m[big] != "bigger than the 8 × 7.5 in plate" {
		t.Errorf("in inches: %q, %q", m[up], m[big])
	}
	for _, c := range []struct {
		mm          float64
		units, want string
	}{{0.004, "mm", "0.004 mm"}, {0.25, "mm", "0.25 mm"}, {7.14, "mm", "7.1 mm"}, {0.0254, "in", "0.001 in"}, {2.54, "in", "0.10 in"}, {31.75, "in", "1.25 in"}} {
		if got := fmtLen(c.mm, c.units); got != c.want {
			t.Errorf("fmtLen(%v, %s) = %q, want %q", c.mm, c.units, got, c.want)
		}
	}
}

func TestPrintCheckResting(t *testing.T) {
	a := checkApp()
	add(t, a, geom.V(0, 0, 0), nil)
	top := add(t, a, geom.V(0, 0, 10), nil) // standing on the first
	if got := kinds(a); len(got[top]) != 0 {
		t.Errorf("a cube standing on another: %v", got[top])
	}
	// the check of only the top one still sees what it stands on
	r := a.printCheck(a.ed.Doc.FindAll([]int{top}), false)
	if len(r.parts) != 1 || len(r.parts[0].findings) != 0 {
		t.Errorf("checked alone: %+v", r.parts)
	}
}

func TestPrintCheckMidAir(t *testing.T) {
	a := checkApp()
	plate := add(t, a, geom.V(0, 0, 0), cube(geom.V(40, 40, 2)))
	post := add(t, a, geom.V(0, 0, 10), nil) // 8 mm above the plate's top
	a.ed.Select(plate, post)
	g, err := a.ed.Group(doc.ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	got := kinds(a)
	if len(got[g]) != 1 || got[g][0] != "mid-air" {
		t.Fatalf("a post above a plate, grouped: %v, want [mid-air]", got[g])
	}
	r := a.printCheck(a.ed.Doc.Nodes, false)
	if r.parts[0].status() != sevNote {
		t.Errorf("status %s, want a note", r.parts[0].status())
	}
}

func TestPrintCheckMulticolourGroupIsWhole(t *testing.T) {
	a := checkApp()
	x, _ := a.ed.AddShape("box", geom.V(0, 0, 0), false)
	y, _ := a.ed.AddShape("box", geom.V(8, 0, 0), false)
	a.ed.Doc.Find(y).Color = "#ff0000"
	a.ed.Select(x, y)
	g, err := a.ed.Group(doc.ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	a.ed.Doc.Find(g).Multicolor = true
	if got := kinds(a); len(got[g]) != 0 {
		t.Errorf("a multicolour union of two boxes: %v", got[g])
	}
}

// The surface findings, from what geom.Surface reports. Imported meshes
// have their gaps closed as they're read (even a missing face), and
// booleans leave cracks only by accident, so the reports are made up and
// put in the check's cache; geom's tests check the measuring.
func TestPrintCheckSurface(t *testing.T) {
	a := checkApp()
	report := func(id int, r geom.SurfaceReport) {
		lp := a.ed.Doc.Find(id).LocalParts()[0]
		k := surfaceKey{lp.Mesh, lp.M}
		real := checkSurface(k)
		r.Shells = real.Shells // placement still sees the cube
		a.check.store(k, surfaceCheck{SurfaceReport: r})
	}
	hole := add(t, a, geom.V(0, 0, 0), nil)
	report(hole, geom.SurfaceReport{Open: 3, Gap: 7.1})
	crack := add(t, a, geom.V(30, 0, 0), nil)
	report(crack, geom.SurfaceReport{Open: 40, Gap: 0.07}) // under the 0.075 mm that slices clean
	wide := add(t, a, geom.V(-30, 0, 0), nil)
	report(wide, geom.SurfaceReport{Open: 40, Gap: 0.004, GapAtLeast: true}) // stopped short: not known to be small
	inside := add(t, a, geom.V(0, 30, 0), nil)
	report(inside, geom.SurfaceReport{InsideOut: 1})
	got := kinds(a)
	if len(got[wide]) != 1 || got[wide][0] != "hole" {
		t.Errorf("a gap of at least 0.004 mm: %v, want [hole]", got[wide])
	}
	if len(got[inside]) != 1 || got[inside][0] != "inside-out" {
		t.Errorf("inside out: %v", got[inside])
	}
	if len(got[hole]) != 1 || got[hole][0] != "hole" {
		t.Errorf("a 7 mm gap: %v, want [hole]", got[hole])
	}
	if len(got[crack]) != 1 || got[crack][0] != "hairline" {
		t.Errorf("a 0.07 mm crack: %v, want [hairline]", got[crack])
	}

	j := printCheckJSON(a.printCheck(a.ed.Doc.Nodes, false))
	parts := j["parts"].([]any)
	if len(parts) != 4 {
		t.Fatalf("%d parts", len(parts))
	}
	first := parts[0].(map[string]any)
	if first["id"] != hole || first["status"] != sevWarning {
		t.Errorf("warnings first, in design order: %v", first)
	}
	f := first["findings"].([]any)[0].(map[string]any)
	if f["kind"] != "hole" || f["gap"] != 7.1 || f["open_edges"] != 3 || f["message"] != "a hole in the surface (7.1 mm)" || f["tip"] == "" {
		t.Errorf("the hole: %v", f)
	}
	last := parts[3].(map[string]any)
	if last["id"] != crack || last["status"] != sevNote {
		t.Errorf("notes last: %v", last)
	}
}

// Overhangs: flat undersides count where nothing is under them, and
// they're information on a line of their own, not a finding.
func TestPrintCheckOverhangs(t *testing.T) {
	over := func(a *App) map[int]float64 {
		out := map[int]float64{}
		for _, p := range a.printCheck(a.ed.Doc.Nodes, false).parts {
			out[p.node.ID] = p.overhang
		}
		return out
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

	a := checkApp()
	c := add(t, a, geom.V(0, 0, 0), nil)
	if o := over(a); o[c] != 0 {
		t.Errorf("a cube on the plate: %g mm²", o[c])
	}
	// a 30 mm slab on a 10 mm post: grouped, the slab's underside round
	// the post overhangs
	post := add(t, a, geom.V(50, 0, 0), nil)
	slab := add(t, a, geom.V(50, 0, 10), cube30x2())
	if o := over(a); !near(o[slab], 900-100) || o[post] != 0 {
		t.Errorf("a slab on a post: %g mm², want 800", o[slab])
	}
	a.ed.Select(post, slab)
	g, err := a.ed.Group(doc.ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	if o := over(a); !near(o[g], 800) {
		t.Errorf("grouped: %g mm², want 800", o[g])
	}
	if r := a.printCheck(a.ed.Doc.Nodes, false); r.parts[0].status() != "ok" && r.parts[1].status() != "ok" {
		t.Errorf("overhangs aren't a finding: %+v", r.parts)
	}
	lines := checkLines(a.printCheck(a.ed.Doc.Nodes, false), "mm")
	if len(lines) != 2 || lines[0].text != "2 parts ready to print" || lines[1].text != "8.0 cm² of overhangs (may need support)" || lines[1].id != g {
		t.Errorf("the panel: %+v", lines)
	}
	if got := checkLines(a.printCheck(a.ed.Doc.Nodes, false), "in")[1].text; got != "1.24 in² of overhangs (may need support)" {
		t.Errorf("in inches: %q", got)
	}
	// under overhangShown, no line
	a2 := checkApp()
	add(t, a2, geom.V(0, 0, 0), nil)
	add(t, a2, geom.V(0, 0, 10), cube(geom.V(11, 11, 1)))
	if ls := checkLines(a2.printCheck(a2.ed.Doc.Nodes, false), "mm"); len(ls) != 1 {
		t.Errorf("21 mm² of overhang: %+v", ls)
	}
}

func cube30x2() *geom.Mesh { return cube(geom.V(30, 30, 2)) }

// A design of more parts than checkCache keeps every part's check: were
// the cache emptied part way through, the background check of a big design
// would find parts unchecked each time it finished, and start again for
// ever.
func TestPrintCheckCacheHoldsEveryPart(t *testing.T) {
	a := checkApp()
	for i := 0; i < checkCache+40; i++ {
		add(t, a, geom.V(float64(i%15)*12-90, float64(i/15)*12-90, 0), cube(geom.V(5, 5, 5+float64(i)/100)))
	}
	a.printCheck(a.ed.Doc.Nodes, false)
	for _, n := range a.ed.Doc.Nodes {
		for _, lp := range n.LocalParts() {
			if _, ok := a.check.surface(surfaceKey{lp.Mesh, lp.M}); !ok {
				t.Fatalf("shape %d's check isn't kept", n.ID)
			}
		}
	}
}

// A click on a surface warning marks the shape's problem edges, until the
// selection or the design changes. (Meshes are closed as they're added,
// so a closed cube's none are marked, and the edges for the rest are made
// up; geom's tests check finding them.)
func TestPrintCheckMarksEdges(t *testing.T) {
	a := checkApp()
	id := add(t, a, geom.V(0, 0, 0), nil)
	other := add(t, a, geom.V(30, 0, 0), nil)
	a.ed.Select(id)
	a.markEdges(id)
	if es := a.markedNow(); len(es) != 0 {
		t.Fatalf("a closed cube: %d edges marked", len(es))
	}
	mark := func() {
		a.ed.Select(id)
		a.check.marked = markedEdges{id: id, rev: a.rev, edges: [][2]geom.Vec3{{geom.V(0, 0, 0), geom.V(1, 0, 0)}}}
		if es := a.markedNow(); len(es) != 1 {
			t.Fatalf("marked: %d edges", len(es))
		}
	}
	mark()
	a.ed.Select(other)
	if es := a.markedNow(); len(es) != 0 {
		t.Errorf("another shape selected: %d edges still marked", len(es))
	}
	mark()
	a.rev++
	if es := a.markedNow(); len(es) != 0 {
		t.Errorf("the design changed: %d edges still marked", len(es))
	}
}
