package shapes

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Practical parametric parts: enclosure hardware and mechanical parts.
// Each is built in millimetres from its parameters (its natural size);
// the shape's size then scales that.

// HardwareKinds and MechanicalKinds list the parts in library order;
// HoleKinds are the ones whose library tile is a hole.
var (
	HardwareKinds   []string
	MechanicalKinds []string
	HoleKinds       = map[string]bool{}
)

// partCache keeps recently built parts: a parameter change asks for the
// natural size before and after, then builds.
var partCache = geom.NewCache(1_000_000, geom.MeshWeight)

func cachedPart(id string, v Values, model func(Values) *geom.Mesh) *geom.Mesh {
	keys := make([]string, 0, len(v.Num))
	for k := range v.Num {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(id)
	for _, k := range keys {
		fmt.Fprintf(&sb, "|%s=%g", k, v.Num[k])
	}
	if m, ok := partCache.Load(sb.String()); ok {
		return m.(*geom.Mesh)
	}
	m := model(v)
	b := m.Bounds()
	m = m.Transform(geom.Translate(b.Center().Neg()))
	partCache.Store(sb.String(), m)
	return m
}

func registerPart(list *[]string, id, label, color string, params []Param, model func(Values) *geom.Mesh) {
	k := &Kind{ID: id, Label: label, Color: color, Params: params}
	k.build = func(s geom.Vec3, v Values) *geom.Mesh { return cachedPart(id, v, model).FitTo(s) }
	k.natural = func(v Values) geom.Vec3 { return cachedPart(id, v, model).Bounds().Size() }
	k.Size = k.natural(k.Defaults())
	register(k)
	*list = append(*list, id)
}

// --- 2D helpers ----------------------------------------------------------------------

// roundedRect is a counter-clockwise w × d rectangle centred on the
// origin with corner radius r.
func roundedRect(w, d, r float64) []Pt {
	r = math.Max(0, math.Min(r, math.Min(w, d)/2-1e-6))
	if r < 1e-6 {
		return []Pt{{-w / 2, -d / 2}, {w / 2, -d / 2}, {w / 2, d / 2}, {-w / 2, d / 2}}
	}
	var out []Pt
	cs := [][2]float64{{w/2 - r, -d/2 + r}, {w/2 - r, d/2 - r}, {-w/2 + r, d/2 - r}, {-w/2 + r, -d/2 + r}}
	for i, c := range cs {
		a0 := -math.Pi/2 + float64(i)*math.Pi/2
		for j := 0; j <= 8; j++ {
			a := a0 + math.Pi/2*float64(j)/8
			out = append(out, Pt{c[0] + r*math.Cos(a), c[1] + r*math.Sin(a)})
		}
	}
	return out
}

func circlePts(cx, cy, r float64, n int) []Pt {
	var out []Pt
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		out = append(out, Pt{cx + r*math.Cos(a), cy + r*math.Sin(a)})
	}
	return out
}

// hexagon has across-flats s, flats facing ±Y.
func hexagon(s float64) []Pt {
	R := s / math.Sqrt(3)
	var out []Pt
	for i := 0; i < 6; i++ {
		a := float64(i) * math.Pi / 3
		out = append(out, Pt{R * math.Cos(a), R * math.Sin(a)})
	}
	return out
}

func cylX(x0, x1, cy, cz, r float64, n int) *geom.Mesh {
	return cylZ(0, 0, -0.5, 0.5, r, n).Transform(geom.Translate(geom.V((x0+x1)/2, cy, cz)).Mul(geom.RotateY(math.Pi / 2)).Mul(geom.Scale(geom.V(1, 1, x1-x0))))
}

// --- metric screw table (ISO coarse) ---------------------------------------------------

type metric struct {
	d, pitch, nutAF, nutH, headH, capD, capH, cskD float64
}

var metrics = []metric{
	{2, 0.4, 4, 1.6, 1.4, 3.8, 2, 4},
	{2.5, 0.45, 5, 2, 1.7, 4.5, 2.5, 5},
	{3, 0.5, 5.5, 2.4, 2, 5.5, 3, 6},
	{4, 0.7, 7, 3.2, 2.8, 7, 4, 8},
	{5, 0.8, 8, 4.7, 3.5, 8.5, 5, 10},
	{6, 1, 10, 5.2, 4, 10, 6, 12},
	{8, 1.25, 13, 6.8, 5.3, 13, 8, 16},
	{10, 1.5, 16, 8.4, 6.4, 16, 10, 20},
	{12, 1.75, 18, 10.8, 7.5, 18, 12, 24},
}

func metricFor(d float64) metric {
	best := metrics[0]
	for _, m := range metrics {
		if math.Abs(m.d-d) < math.Abs(best.d-d) {
			best = m
		}
	}
	return best
}

func sizeSlider(def float64) Param { return slider("size", "Size (M)", 2, 12, 0.5, def, false) }

// --- hardware & enclosures ---------------------------------------------------------------

func standoffModel(v Values) *geom.Mesh {
	h, od, hole := v.Get("height", 8), v.Get("outer", 6), v.Get("hole", 2.5)
	fd, ft := v.Get("flange", 10), v.Get("flangeh", 1.5)
	ro := od / 2
	rh := math.Min(hole/2, ro-0.4)
	rf := math.Max(fd/2, ro)
	ft = math.Min(ft, h)
	var prof [][2]float64
	if rf > ro+0.01 {
		prof = [][2]float64{{rf, 0}, {rf, ft}, {ro, ft}, {ro, h}}
	} else {
		prof = [][2]float64{{ro, 0}, {ro, h}}
	}
	if rh > 0.05 {
		prof = append([][2]float64{{rh, 0}}, prof...)
		prof = append(prof, [2]float64{rh, h})
		return revolve(prof, 48, true)
	}
	prof = append([][2]float64{{0, 0}}, prof...)
	prof = append(prof, [2]float64{0, h})
	return revolve(prof, 48, false)
}

// cornerInset is how far in from both sides a hole of diameter hd must
// sit, at least in, to stay inside a corner rounded to radius r with a
// wall of at least m around it.
func cornerInset(r, hd, m, in float64) float64 {
	// in the corner's square the hole is nearest the arc: its centre is
	// √2·(r−inset) from the arc's centre
	return math.Max(in, math.Max(hd/2+m, r-(r-hd/2-m)/math.Sqrt2))
}

func roundedPlateModel(v Values) *geom.Mesh {
	l, w, t := v.Get("length", 50), v.Get("width", 30), v.Get("thickness", 3)
	r := math.Max(0, math.Min(v.Get("radius", 4), math.Min(l, w)/2))
	contours := [][]Pt{roundedRect(l, w, r)}
	hd := v.Get("hole", 3.2)
	// holes move inboard if the corners are rounded too far for them
	in := cornerInset(r, hd, 0.4, v.Get("inset", 4))
	if hd > 0 && 2*in-hd > 0.8 && 2*in < math.Min(l, w)-hd {
		for _, c := range [][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
			contours = append(contours, reversed(circlePts(c[0]*(l/2-in), c[1]*(w/2-in), hd/2, 32)))
		}
	}
	return ExtrudeContours(contours, 0, t)
}

func ventModel(v Values) *geom.Mesh {
	l, w, t := v.Get("length", 40), v.Get("width", 25), v.Get("thickness", 2)
	cell, wall, border := v.Get("cell", 4), math.Max(0.4, v.Get("wall", 1)), v.Get("border", 2)
	ix, iy := l/2-border, w/2-border
	contours := [][]Pt{{{-l / 2, -w / 2}, {l / 2, -w / 2}, {l / 2, w / 2}, {-l / 2, w / 2}}}
	if v.Get("pattern", 0) < 0.5 {
		// hexagons, flats facing ±X
		R := cell / math.Sqrt(3)
		dx, dy := cell+wall, (cell+wall)*math.Sqrt(3)/2
		var holes [][]Pt
		minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for j := -200; j <= 200; j++ {
			y := float64(j) * dy
			if math.Abs(y)+R > iy {
				continue
			}
			off := 0.0
			if j%2 != 0 {
				off = dx / 2
			}
			for i := -200; i <= 200; i++ {
				x := float64(i)*dx + off
				if math.Abs(x)+cell/2 > ix {
					continue
				}
				var hx []Pt
				for k := 5; k >= 0; k-- { // clockwise: a hole
					a := math.Pi/6 + float64(k)*math.Pi/3
					hx = append(hx, Pt{x + R*math.Cos(a), y + R*math.Sin(a)})
				}
				holes = append(holes, hx)
				minX, maxX = math.Min(minX, x), math.Max(maxX, x)
				minY, maxY = math.Min(minY, y), math.Max(maxY, y)
			}
		}
		// centre the pattern
		cx, cy := (minX+maxX)/2, (minY+maxY)/2
		for _, hx := range holes {
			for i := range hx {
				hx[i].X -= cx
				hx[i].Y -= cy
			}
			contours = append(contours, hx)
		}
	} else {
		// slots across the width
		n := int((2*ix + wall) / (cell + wall))
		x0 := -(float64(n)*(cell+wall) - wall) / 2
		for i := 0; i < n; i++ {
			x := x0 + float64(i)*(cell+wall)
			contours = append(contours, reversed([]Pt{{x, -iy}, {x + cell, -iy}, {x + cell, iy}, {x, iy}}))
		}
	}
	return ExtrudeContours(contours, 0, t)
}

func hexNutModel(v Values) *geom.Mesh {
	m := metricFor(v.Get("size", 3))
	return ExtrudeContours([][]Pt{hexagon(m.nutAF), reversed(circlePts(0, 0, m.d/2, 32))}, 0, m.nutH)
}

func hexBoltModel(v Values) *geom.Mesh {
	m := metricFor(v.Get("size", 3))
	l := math.Max(v.Get("length", 12), 2*m.pitch)
	head := ExtrudeContours([][]Pt{hexagon(m.nutAF)}, 0, m.headH)
	// the thread's run-outs make its length (turns + 0.75) pitches
	turns := math.Max(1, math.Round(l/m.pitch-0.75))
	tl := ThreadLength(m.pitch, int(turns))
	// the thread stands on the head's top face (the length is under the
	// head): touching faces join exactly, where a thread sunk into the
	// head would leave slivers where the head's face cuts the helix
	shank := ThreadMesh(geom.V(m.d, m.d, tl), m.pitch, 32, 1).Transform(geom.Translate(geom.V(0, 0, m.headH+tl/2)))
	return done(geom.Union(head, shank))
}

func nutTrapModel(v Values) *geom.Mesh {
	m := metricFor(v.Get("size", 3))
	depth := math.Max(v.Get("depth", 10), m.nutH+1)
	const c = 0.3 // clearance
	pocket := ExtrudeContours([][]Pt{hexagon(m.nutAF + 2*c)}, depth-m.nutH-c, depth)
	return done(geom.Union(pocket, cylZ(0, 0, 0, depth-m.nutH, m.d/2+c, 32)))
}

func countersinkModel(v Values) *geom.Mesh {
	m := metricFor(v.Get("size", 3))
	depth := v.Get("depth", 10)
	rc := m.d/2 + 0.2
	rk := m.cskD/2 + 0.2
	cs := rk - rc // 90°
	depth = math.Max(depth, cs+0.5)
	return revolve([][2]float64{{0, 0}, {rc, 0}, {rc, depth - cs}, {rk, depth}, {rk, depth + 0.5}, {0, depth + 0.5}}, 48, false)
}

func counterboreModel(v Values) *geom.Mesh {
	m := metricFor(v.Get("size", 3))
	depth := v.Get("depth", 10)
	rc := m.d/2 + 0.2
	rb := m.capD/2 + 0.3
	bd := m.capH + 0.3
	depth = math.Max(depth, bd+0.5)
	return revolve([][2]float64{{0, 0}, {rc, 0}, {rc, depth - bd}, {rb, depth - bd}, {rb, depth + 0.5}, {0, depth + 0.5}}, 48, false)
}

// --- mechanical ------------------------------------------------------------------------

// involuteOutline is a spur gear's outline: module m, z teeth, 20°
// pressure angle, first tooth on +X.
func involuteOutline(m float64, z int) []Pt {
	z = max(z, 8)
	alpha := 20 * math.Pi / 180
	r := m * float64(z) / 2
	rb, ra, rf := r*math.Cos(alpha), r+m, r-1.25*m
	inv := func(rho float64) float64 {
		if rho <= rb {
			return 0
		}
		t := math.Sqrt(rho*rho/(rb*rb) - 1)
		return t - math.Atan(t)
	}
	invA := math.Tan(alpha) - alpha
	half := func(rho float64) float64 { return math.Pi/(2*float64(z)) + invA - inv(rho) }
	r0 := math.Max(rb, rf)
	polar := func(rho, a float64) Pt { return Pt{rho * math.Cos(a), rho * math.Sin(a)} }
	var out []Pt
	const fl = 10
	for k := 0; k < z; k++ {
		c := 2 * math.Pi * float64(k) / float64(z)
		// root, then the flank up, tip, flank down
		if rf < rb {
			out = append(out, polar(rf, c-half(rb)))
		}
		for i := 0; i <= fl; i++ {
			rho := r0 + (ra-r0)*float64(i)/fl
			out = append(out, polar(rho, c-half(rho)))
		}
		for i := 1; i < 4; i++ {
			out = append(out, polar(ra, c-half(ra)+2*half(ra)*float64(i)/4))
		}
		for i := fl; i >= 0; i-- {
			rho := r0 + (ra-r0)*float64(i)/fl
			out = append(out, polar(rho, c+half(rho)))
		}
		if rf < rb {
			out = append(out, polar(rf, c+half(rb)))
		}
		// root arc to the next tooth
		next := c + 2*math.Pi/float64(z)
		a0, a1 := c+half(r0), next-half(r0)
		for i := 1; i < 4; i++ {
			out = append(out, polar(rf, a0+(a1-a0)*float64(i)/4))
		}
	}
	return out
}

func spurGearModel(v Values) *geom.Mesh {
	m, z := v.Get("module", 1), int(v.Get("teeth", 20))
	contours := [][]Pt{involuteOutline(m, z)}
	if b := v.Get("bore", 5); b > 0 && b < m*float64(z)-3*m {
		contours = append(contours, reversed(circlePts(0, 0, b/2, 48)))
	}
	return ExtrudeContours(contours, 0, v.Get("thickness", 5))
}

func gearRackModel(v Values) *geom.Mesh {
	m, n := v.Get("module", 1), max(int(v.Get("teeth", 10)), 1)
	back := v.Get("back", 4)
	p := math.Pi * m
	ta := math.Tan(20 * math.Pi / 180)
	tipH, rootH := p/4-m*ta, p/4+1.25*m*ta
	yb := -1.25*m - back
	L := float64(n) * p
	prof := []Pt{{0, yb}, {L, yb}, {L, -1.25 * m}}
	for k := n - 1; k >= 0; k-- {
		x := p/2 + float64(k)*p
		prof = append(prof, Pt{x + rootH, -1.25 * m}, Pt{x + tipH, m}, Pt{x - tipH, m}, Pt{x - rootH, -1.25 * m})
	}
	prof = append(prof, Pt{0, -1.25 * m})
	return ExtrudeContours([][]Pt{prof}, 0, v.Get("thickness", 5))
}

// pipeElbowModel is a bent tube lying on the workplane: the centreline is
// an arc of the bend radius through angle, the wall from the inner to the
// outer diameter.
func pipeElbowModel(v Values) *geom.Mesh {
	ang := math.Max(5, math.Min(v.Get("angle", 90), 360)) * math.Pi / 180
	ro := v.Get("diameter", 10) / 2
	ri := math.Max(0.1, ro-v.Get("wall", 1.5))
	R := math.Max(v.Get("bend", 15), ro+0.5)
	const ns, nc = 48, 32
	full := ang >= 2*math.Pi-1e-9
	pt := func(i, j int, r float64) geom.Vec3 {
		a := ang * float64(i) / ns
		b := 2 * math.Pi * float64(j%nc) / nc
		rr := R + r*math.Cos(b)
		return geom.V(rr*math.Cos(a), rr*math.Sin(a), r*math.Sin(b))
	}
	m := &geom.Mesh{}
	quad := func(a, b, c, d geom.Vec3) {
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{a, b, c}}, geom.Tri{P: [3]geom.Vec3{a, c, d}})
	}
	for i := 0; i < ns; i++ {
		for j := 0; j < nc; j++ {
			quad(pt(i, j, ro), pt(i+1, j, ro), pt(i+1, j+1, ro), pt(i, j+1, ro))
			quad(pt(i, j, ri), pt(i, j+1, ri), pt(i+1, j+1, ri), pt(i+1, j, ri))
		}
	}
	if !full {
		for j := 0; j < nc; j++ {
			quad(pt(0, j+1, ri), pt(0, j, ri), pt(0, j, ro), pt(0, j+1, ro))
			quad(pt(ns, j, ri), pt(ns, j+1, ri), pt(ns, j+1, ro), pt(ns, j, ro))
		}
	}
	return m.Clean().Oriented()
}

// springModel sweeps a round wire along a helix with flat ends.
func springModel(v Values) *geom.Mesh {
	turns := math.Max(0.5, v.Get("turns", 5))
	D, d := v.Get("diameter", 20), v.Get("wire", 2)
	R, r := D/2, math.Min(d/2, 0.45*D) // the wire can't be thicker than the coil
	d = 2 * r
	pitch := math.Max(v.Get("pitch", 4), d*1.05)
	const perTurn, nc = 48, 16
	ns := int(math.Ceil(turns * perTurn))
	frame := func(i int) (c, n, b geom.Vec3) {
		t := 2 * math.Pi * turns * float64(i) / float64(ns)
		c = geom.V(R*math.Cos(t), R*math.Sin(t), pitch*t/(2*math.Pi))
		tan := geom.V(-R*math.Sin(t), R*math.Cos(t), pitch/(2*math.Pi)).Norm()
		n = geom.V(math.Cos(t), math.Sin(t), 0) // outwards
		b = tan.Cross(n).Norm()
		n = b.Cross(tan).Norm()
		return
	}
	ring := func(i int) []geom.Vec3 {
		c, n, b := frame(i)
		var out []geom.Vec3
		for j := 0; j < nc; j++ {
			a := 2 * math.Pi * float64(j) / nc
			out = append(out, c.Add(n.Mul(r*math.Cos(a))).Add(b.Mul(r*math.Sin(a))))
		}
		return out
	}
	m := &geom.Mesh{}
	prev := ring(0)
	c0, _, _ := frame(0)
	for j := 0; j < nc; j++ {
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{c0, prev[(j+1)%nc], prev[j]}})
	}
	for i := 1; i <= ns; i++ {
		cur := ring(i)
		for j := 0; j < nc; j++ {
			k := (j + 1) % nc
			m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{prev[j], prev[k], cur[k]}}, geom.Tri{P: [3]geom.Vec3{prev[j], cur[k], cur[j]}})
		}
		prev = cur
	}
	c1, _, _ := frame(ns)
	for j := 0; j < nc; j++ {
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{c1, prev[j], prev[(j+1)%nc]}})
	}
	return m.Clean().Oriented()
}

func knobModel(v Values) *geom.Mesh {
	D, h := v.Get("diameter", 20), v.Get("height", 12)
	n := max(int(v.Get("knurls", 24)), 3)
	depth := math.Min(v.Get("depth", 1), D/4)
	R := D / 2
	var outline []Pt
	for i := 0; i < n*8; i++ {
		t := 2 * math.Pi * float64(i) / float64(n*8)
		rr := R - depth*(1-math.Cos(float64(n)*t))/2
		outline = append(outline, Pt{rr * math.Cos(t), rr * math.Sin(t)})
	}
	body := ExtrudeContours([][]Pt{outline}, 0, h)
	sd := math.Min(v.Get("shaft", 6), D-2*depth-2)
	if sd <= 0 {
		return body
	}
	// a blind bore from the bottom, D-shaped when asked
	bore := circlePts(0, 0, sd/2, 48)
	if v.Get("dshaft", 1) > 0.5 {
		flat := sd/2 - sd*0.15
		for i := range bore {
			bore[i].X = math.Min(bore[i].X, flat)
		}
	}
	return done(geom.Subtract(body, ExtrudeContours([][]Pt{bore}, -0.5, math.Max(1, h-2))))
}

// knuckle is a hinge knuckle: an n-sided cylinder along X with its axis at
// height r, turned so a flat face (not an edge) lies on the bottom plane.
// An edge touching the plane would meet the leaf along a single line,
// which booleans can't join cleanly; a face lying in the plane they can.
func knuckle(x0, x1, r float64, n int) *geom.Mesh {
	c := cylX(x0, x1, 0, r, r/math.Cos(math.Pi/float64(n)), n)
	return c.Transform(geom.Translate(geom.V(0, 0, r)).Mul(geom.RotateX(math.Pi / float64(n))).Mul(geom.Translate(geom.V(0, 0, -r))))
}

// hingeModel is a print-in-place hinge: two leaves whose alternating
// knuckles turn on a pin fixed to the first leaf.
func hingeModel(v Values) *geom.Mesh {
	L, wd, t := v.Get("length", 30), v.Get("leaf", 15), v.Get("thickness", 2)
	kd := math.Max(v.Get("knuckle", 6), t)
	pd := math.Min(v.Get("pin", 3), kd-1.2)
	clr := v.Get("clearance", 0.4)
	nk := max(int(v.Get("knuckles", 5)), 3)
	if nk%2 == 0 {
		nk++
	}
	kr := kd / 2
	seg := (L - float64(nk-1)*clr) / float64(nk)
	// knuckles at least 1 mm long: fewer of them, then less clearance
	for nk > 3 && seg < 1 {
		nk -= 2
		seg = (L - float64(nk-1)*clr) / float64(nk)
	}
	if seg < 1 {
		clr = math.Max(0.05, (L-float64(nk))/float64(nk-1))
		seg = (L - float64(nk-1)*clr) / float64(nk)
	}
	var aParts, bParts []*geom.Mesh
	aParts = append(aParts, cuboid(-L/2, kr+clr, 0, L/2, kr+clr+wd, t))
	bParts = append(bParts, cuboid(-L/2, -kr-clr-wd, 0, L/2, -kr-clr, t))
	for i := 0; i < nk; i++ {
		x0 := -L/2 + float64(i)*(seg+clr)
		x1 := x0 + seg
		k := knuckle(x0, x1, kr, 32)
		if i%2 == 0 {
			aParts = append(aParts, k, cuboid(x0, 0, 0, x1, kr+clr+0.5, t))
		} else {
			bParts = append(bParts, k, cuboid(x0, -kr-clr-0.5, 0, x1, 0, t))
		}
	}
	aParts = append(aParts, cylX(-L/2+0.01, L/2-0.01, 0, kr, pd/2, 24))
	a := union(aParts...)
	b := geom.Subtract(union(bParts...), cylX(-L/2-1, L/2+1, 0, kr, pd/2+clr, 24))
	return done(geom.Merge(a, b))
}

func init() {
	hw := &HardwareKinds
	registerPart(hw, "standoff", "PCB Standoff", "#d9d9d9", []Param{
		lslider("height", "Height", 1, 50, 0.5, 8, false),
		lslider("outer", "Outer Dia", 2, 20, 0.1, 6, false),
		lslider("hole", "Hole Dia", 0, 10, 0.1, 2.5, false),
		lslider("flange", "Flange Dia", 0, 30, 0.5, 10, false),
		lslider("flangeh", "Flange Height", 0.4, 5, 0.1, 1.5, false),
	}, standoffModel)
	registerPart(hw, "plate", "Rounded Plate", "#1a9fe0", []Param{
		lslider("length", "Length", 5, 300, 1, 50, false),
		lslider("width", "Width", 5, 300, 1, 30, false),
		lslider("thickness", "Thickness", 0.4, 20, 0.1, 3, false),
		lslider("radius", "Corner Radius", 0, 30, 0.5, 4, false),
		lslider("hole", "Hole Dia", 0, 10, 0.1, 3.2, false),
		lslider("inset", "Hole Inset", 1, 30, 0.5, 4, false),
	}, roundedPlateModel)
	registerPart(hw, "vent", "Vent Grille", "#9a9a9a", []Param{
		lslider("length", "Length", 5, 300, 1, 40, false),
		lslider("width", "Width", 5, 300, 1, 25, false),
		lslider("thickness", "Thickness", 0.4, 10, 0.1, 2, false),
		lslider("cell", "Cell Size", 1, 20, 0.5, 4, false),
		lslider("wall", "Wall", 0.4, 5, 0.1, 1, false),
		lslider("border", "Border", 0.5, 20, 0.5, 2, false),
		slider("pattern", "Pattern (Hex/Slots)", 0, 1, 1, 0, true),
	}, ventModel)
	registerPart(hw, "hexnut", "Hex Nut", "#9a9a9a", []Param{sizeSlider(3)}, hexNutModel)
	registerPart(hw, "hexbolt", "Hex Bolt", "#9a9a9a", []Param{sizeSlider(3), lslider("length", "Length", 2, 100, 1, 12, false)}, hexBoltModel)
	registerPart(hw, "nuttrap", "Nut Trap", "#9a9a9a", []Param{sizeSlider(3), lslider("depth", "Depth", 2, 50, 0.5, 10, false)}, nutTrapModel)
	registerPart(hw, "countersink", "Countersunk Hole", "#9a9a9a", []Param{sizeSlider(3), lslider("depth", "Depth", 2, 50, 0.5, 10, false)}, countersinkModel)
	registerPart(hw, "counterbore", "Counterbored Hole", "#9a9a9a", []Param{sizeSlider(3), lslider("depth", "Depth", 2, 50, 0.5, 10, false)}, counterboreModel)
	for _, id := range []string{"nuttrap", "countersink", "counterbore"} {
		HoleKinds[id] = true
	}

	mk := &MechanicalKinds
	registerPart(mk, "spurgear", "Spur Gear", "#c8ccd0", []Param{
		mslider("module", "Module", 0.3, 5, 0.1, 1, false),
		slider("teeth", "Teeth", 8, 120, 1, 20, true),
		lslider("thickness", "Thickness", 0.5, 50, 0.5, 5, false),
		lslider("bore", "Bore Dia", 0, 50, 0.5, 5, false),
	}, spurGearModel)
	registerPart(mk, "gearrack", "Gear Rack", "#c8ccd0", []Param{
		mslider("module", "Module", 0.3, 5, 0.1, 1, false),
		slider("teeth", "Teeth", 1, 100, 1, 10, true),
		lslider("thickness", "Thickness", 0.5, 50, 0.5, 5, false),
		lslider("back", "Back Height", 0.5, 30, 0.5, 4, false),
	}, gearRackModel)
	registerPart(mk, "elbow", "Pipe Elbow", "#7a3fc0", []Param{
		slider("angle", "Angle", 5, 360, 1, 90, false),
		lslider("bend", "Bend Radius", 1, 100, 0.5, 15, false),
		lslider("diameter", "Outer Dia", 1, 50, 0.5, 10, false),
		lslider("wall", "Wall", 0.2, 20, 0.1, 1.5, false),
	}, pipeElbowModel)
	registerPart(mk, "spring", "Spring", "#9a9a9a", []Param{
		slider("turns", "Turns", 0.5, 30, 0.5, 5, false),
		lslider("diameter", "Coil Dia", 2, 100, 0.5, 20, false),
		lslider("wire", "Wire Dia", 0.4, 10, 0.1, 2, false),
		lslider("pitch", "Pitch", 0.5, 30, 0.1, 4, false),
	}, springModel)
	registerPart(mk, "knob", "Knurled Knob", "#1f1f1f", []Param{
		lslider("diameter", "Diameter", 5, 100, 0.5, 20, false),
		lslider("height", "Height", 2, 50, 0.5, 12, false),
		slider("knurls", "Knurls", 6, 90, 1, 24, true),
		lslider("depth", "Knurl Depth", 0.1, 5, 0.1, 1, false),
		lslider("shaft", "Shaft Dia", 0, 20, 0.1, 6, false),
		slider("dshaft", "D-Shaft", 0, 1, 1, 1, true),
	}, knobModel)
	registerPart(mk, "hinge", "Hinge", "#2fa84f", []Param{
		lslider("length", "Length", 10, 200, 1, 30, false),
		lslider("leaf", "Leaf Width", 3, 100, 0.5, 15, false),
		lslider("thickness", "Thickness", 0.8, 10, 0.1, 2, false),
		lslider("knuckle", "Knuckle Dia", 3, 30, 0.5, 6, false),
		lslider("pin", "Pin Dia", 1, 20, 0.1, 3, false),
		lslider("clearance", "Clearance", 0.1, 1.5, 0.05, 0.4, false),
		slider("knuckles", "Knuckles", 3, 15, 2, 5, true),
	}, hingeModel)
}
