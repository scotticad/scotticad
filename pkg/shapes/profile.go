package shapes

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Outline and loft shapes are made from point data rather than sliders.
//
// An outline is closed contours extruded straight up between two heights;
// contours inside others are holes. A loft joins one closed contour per
// height with straight walls between neighbouring heights and closes the
// bottom and top flat.
//
// Points are in millimetres. Either kind can be smoothed: each contour then
// becomes a curve through its points, and a point given twice in a row is a
// sharp corner in the curve.

// Section is one height of an outline or loft: closed contours of (x, y)
// points at height Z. An outline has two sections (its bottom and top)
// with the same contours; a loft has one contour per section.
//
// Sections are never changed in place once made: a node's sections are
// replaced as a whole, so copies of a node may share them.
type Section struct {
	Z        float64        `json:"z"`
	Contours [][][2]float64 `json:"contours"`
}

// Limits on the point data.
const (
	maxProfilePoints = 20000  // points given, all sections together
	maxProfileRing   = 200000 // points after smoothing, all sections together
	maxProfileCoord  = 1e5    // mm
)

// IsProfile reports whether a kind is built from sections.
func IsProfile(kind string) bool { return kind == "outline" || kind == "loft" }

// CheckSections reports what is wrong with sections for kind, if anything,
// in words fit to show a script's author.
func CheckSections(kind string, ss []Section) error {
	total := 0
	for i, s := range ss {
		if !finite(s.Z) || math.Abs(s.Z) > maxProfileCoord {
			return fmt.Errorf("section %d: height %v is out of range", i+1, s.Z)
		}
		if len(s.Contours) == 0 {
			return fmt.Errorf("section %d has no outline", i+1)
		}
		for j, c := range s.Contours {
			pts := toPts(c)
			for _, p := range pts {
				if !finite(p.X) || !finite(p.Y) || math.Abs(p.X) > maxProfileCoord || math.Abs(p.Y) > maxProfileCoord {
					return fmt.Errorf("section %d, outline %d: point (%v, %v) is out of range", i+1, j+1, p.X, p.Y)
				}
			}
			total += len(pts)
			if total > maxProfilePoints {
				return fmt.Errorf("too many points (more than %d)", maxProfilePoints)
			}
			if len(dedupe(pts)) < 3 {
				return fmt.Errorf("section %d, outline %d needs at least 3 different points", i+1, j+1)
			}
			if inLine(pts) {
				return fmt.Errorf("section %d, outline %d encloses no area", i+1, j+1)
			}
			if a, b, ok := selfCrossing(pts); ok {
				return fmt.Errorf("section %d, outline %d crosses itself (edges %d and %d)", i+1, j+1, a+1, b+1)
			}
			if math.Abs(SignedArea(pts)) < 1e-9 {
				return fmt.Errorf("section %d, outline %d encloses no area", i+1, j+1)
			}
		}
	}
	switch kind {
	case "outline":
		if len(ss) != 2 {
			return errors.New("an outline has a bottom and a top section")
		}
		if !(ss[1].Z > ss[0].Z) {
			return errors.New("an outline's top must be above its bottom")
		}
	case "loft":
		if len(ss) < 2 {
			return errors.New("a loft needs at least 2 sections")
		}
		for i, s := range ss {
			if len(s.Contours) != 1 {
				return fmt.Errorf("section %d: a loft section has exactly one outline (it has %d)", i+1, len(s.Contours))
			}
			if i > 0 && !(s.Z > ss[i-1].Z) {
				return fmt.Errorf("section %d: heights must go up (%v after %v)", i+1, s.Z, ss[i-1].Z)
			}
		}
	default:
		return fmt.Errorf("%s is not an outline or loft", kind)
	}
	return nil
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// inLine reports points all on one straight line.
func inLine(c []Pt) bool {
	// the line through the first point and the one farthest from it
	far, fd := 0, 0.0
	for i, p := range c {
		if d := math.Hypot(p.X-c[0].X, p.Y-c[0].Y); d > fd {
			far, fd = i, d
		}
	}
	if fd == 0 {
		return true
	}
	for _, p := range c {
		if math.Abs(cross2(c[0], c[far], p))/fd > 1e-9 {
			return false
		}
	}
	return true
}

func toPts(c [][2]float64) []Pt {
	out := make([]Pt, len(c))
	for i, p := range c {
		out[i] = Pt{p[0], p[1]}
	}
	return out
}

// dedupe drops points equal to the one before (the last against the first
// too): a closed contour without zero-length edges.
func dedupe(c []Pt) []Pt {
	var out []Pt
	for _, p := range c {
		if len(out) > 0 && same(out[len(out)-1], p) {
			continue
		}
		out = append(out, p)
	}
	for len(out) > 1 && same(out[0], out[len(out)-1]) {
		out = out[:len(out)-1]
	}
	return out
}

// selfCrossing finds two edges of the closed contour c that cross or touch
// (other than neighbours meeting at their shared point).
func selfCrossing(c []Pt) (int, int, bool) {
	c = dedupe(c)
	n := len(c)
	if n < 4 {
		return 0, 0, false
	}
	type seg struct {
		a, b                   Pt
		minX, maxX, minY, maxY float64
	}
	s := make([]seg, n)
	for i := range c {
		a, b := c[i], c[(i+1)%n]
		s[i] = seg{a, b, math.Min(a.X, b.X), math.Max(a.X, b.X), math.Min(a.Y, b.Y), math.Max(a.Y, b.Y)}
	}
	// sweep along x: only edges whose x ranges overlap can meet
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sortBy(order, func(i int) float64 { return s[i].minX })
	var active []int
	for _, i := range order {
		keep := active[:0]
		for _, j := range active {
			if s[j].maxX >= s[i].minX {
				keep = append(keep, j)
			}
		}
		active = keep
		for _, j := range active {
			if s[j].maxY < s[i].minY || s[i].maxY < s[j].minY {
				continue
			}
			lo, hi := min(i, j), max(i, j)
			if hi-lo == 1 || (lo == 0 && hi == n-1) {
				// neighbours share a point; they cross only if they fold back
				// onto each other
				if overlapCollinear(s[i].a, s[i].b, s[j].a, s[j].b) {
					return lo, hi, true
				}
				continue
			}
			if segmentsMeet(s[i].a, s[i].b, s[j].a, s[j].b) {
				return lo, hi, true
			}
		}
		active = append(active, i)
	}
	return 0, 0, false
}

// sortBy stably sorts idx by key: a merge sort, as big outlines make an
// insertion sort too slow.
func sortBy(idx []int, key func(int) float64) {
	if len(idx) < 2 {
		return
	}
	m := len(idx) / 2
	a := append([]int(nil), idx[:m]...)
	b := append([]int(nil), idx[m:]...)
	sortBy(a, key)
	sortBy(b, key)
	i, j := 0, 0
	for k := range idx {
		if j >= len(b) || (i < len(a) && key(a[i]) <= key(b[j])) {
			idx[k] = a[i]
			i++
		} else {
			idx[k] = b[j]
			j++
		}
	}
}

func orient(a, b, c Pt) int {
	v := cross2(a, b, c)
	switch {
	case v > 1e-12:
		return 1
	case v < -1e-12:
		return -1
	}
	return 0
}

func onSegment(a, b, p Pt) bool {
	return math.Min(a.X, b.X)-1e-12 <= p.X && p.X <= math.Max(a.X, b.X)+1e-12 &&
		math.Min(a.Y, b.Y)-1e-12 <= p.Y && p.Y <= math.Max(a.Y, b.Y)+1e-12
}

func segmentsMeet(a, b, c, d Pt) bool {
	o1, o2, o3, o4 := orient(a, b, c), orient(a, b, d), orient(c, d, a), orient(c, d, b)
	if o1 != o2 && o3 != o4 {
		return true
	}
	return (o1 == 0 && onSegment(a, b, c)) || (o2 == 0 && onSegment(a, b, d)) ||
		(o3 == 0 && onSegment(c, d, a)) || (o4 == 0 && onSegment(c, d, b))
}

// overlapCollinear: two edges meeting at a point that run back along each
// other (a spike of no width).
func overlapCollinear(a, b, c, d Pt) bool {
	if orient(a, b, c) != 0 || orient(a, b, d) != 0 {
		return false
	}
	// the shared point is an end of both; they overlap if the far ends lie
	// on the same side of it
	var p, q, r Pt
	switch {
	case same(b, c):
		p, q, r = b, a, d
	case same(a, d):
		p, q, r = a, b, c
	case same(a, c):
		p, q, r = a, b, d
	default:
		p, q, r = b, a, c
	}
	return (q.X-p.X)*(r.X-p.X)+(q.Y-p.Y)*(r.Y-p.Y) > 0
}

// SectionsKey is a short hash of sections, for cache keys.
func SectionsKey(ss []Section) string {
	if len(ss) == 0 {
		return ""
	}
	h := sha256.New()
	var b [8]byte
	put := func(f float64) {
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(f))
		h.Write(b[:])
	}
	for _, s := range ss {
		put(s.Z)
		put(float64(len(s.Contours)))
		for _, c := range s.Contours {
			put(float64(len(c)))
			for _, p := range c {
				put(p[0])
				put(p[1])
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// smoothed turns a contour into a closed Catmull-Rom curve through its
// points, steps points per edge. A point repeated is a sharp corner: the
// curve arrives along the edge before it and leaves along the edge after.
func smoothed(c []Pt, steps int) []Pt {
	n := len(c)
	if steps <= 1 || n < 3 {
		return c
	}
	out := make([]Pt, 0, n*steps)
	for i := 0; i < n; i++ {
		p0, p1, p2, p3 := c[(i+n-1)%n], c[i], c[(i+1)%n], c[(i+2)%n]
		if same(p1, p2) {
			// between a repeated point's copies: nothing (its tangents
			// aren't zero, so the curve there would be a small loop)
			continue
		}
		for s := 0; s < steps; s++ {
			t := float64(s) / float64(steps)
			t2, t3 := t*t, t*t*t
			f := func(a, b, c, d float64) float64 {
				return 0.5 * (2*b + (-a+c)*t + (2*a-5*b+4*c-d)*t2 + (-a+3*b-3*c+d)*t3)
			}
			out = append(out, Pt{f(p0.X, p1.X, p2.X, p3.X), f(p0.Y, p1.Y, p2.Y, p3.Y)})
		}
	}
	return out
}

// rings returns each section's contours as they are built: smoothed when
// asked, with repeated points removed. Invalid data gives nil.
func rings(kind string, v Values) [][][]Pt {
	if CheckSections(kind, v.Sections) != nil {
		return nil
	}
	steps := 1
	if v.Get("smooth", 0) >= 0.5 {
		steps = int(math.Round(v.Get("steps", 8)))
		total := 0
		for _, s := range v.Sections {
			for _, c := range s.Contours {
				total += len(c)
			}
		}
		// keep big shapes buildable
		for steps > 1 && total*steps > maxProfileRing {
			steps--
		}
	}
	out := make([][][]Pt, len(v.Sections))
	for i, s := range v.Sections {
		for _, c := range s.Contours {
			out[i] = append(out[i], dedupe(smoothed(toPts(c), steps)))
		}
	}
	return out
}

// ProfileBounds is the box an outline or loft fills, in the coordinates of
// its points (empty for invalid data).
func ProfileBounds(kind string, v Values) geom.Box {
	rs := rings(kind, v)
	if rs == nil {
		return geom.EmptyBox()
	}
	b := geom.EmptyBox()
	for i, sec := range rs {
		z := v.Sections[i].Z
		for _, c := range sec {
			for _, p := range c {
				b = b.Extend(geom.V(p.X, p.Y, z))
			}
		}
	}
	return b
}

// profileMesh builds an outline or loft in the coordinates of its points,
// or returns nil for invalid data.
func profileMesh(kind string, v Values) *geom.Mesh {
	rs := rings(kind, v)
	if rs == nil {
		return nil
	}
	if kind == "outline" {
		return ExtrudeContours(rs[0], v.Sections[0].Z, v.Sections[1].Z)
	}
	zs := make([]float64, len(rs))
	loops := make([][]Pt, len(rs))
	for i := range rs {
		zs[i], loops[i] = v.Sections[i].Z, rs[i][0]
	}
	return loftMesh(loops, zs)
}

// loftMesh joins closed contours at rising heights with straight walls and
// closes the ends. Contours with the same number of points are joined
// point to point (so corners meet corners). Otherwise each neighbouring
// pair, started at matching points, gets each other's points at the same
// fractions of their length, and is then joined point to point: every
// contour keeps all its own points, and the walls are even strips rather
// than fans.
func loftMesh(loops [][]Pt, zs []float64) *geom.Mesh {
	for i, c := range loops {
		if SignedArea(c) < 0 {
			// counter-clockwise, keeping the first point first so points
			// given in matching order still match
			r := make([]Pt, len(c))
			r[0] = c[0]
			for k := 1; k < len(c); k++ {
				r[k] = c[len(c)-k]
			}
			loops[i] = r
		}
	}
	sameCount := true
	for _, c := range loops {
		sameCount = sameCount && len(c) == len(loops[0])
	}
	m := &geom.Mesh{}
	v3 := func(p Pt, z float64) geom.Vec3 { return geom.V(p.X, p.Y, z) }
	wall := func(a, b, c Pt, za, zb, zc float64) {
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{v3(a, za), v3(b, zb), v3(c, zc)}})
	}
	for k := 0; k+1 < len(loops); k++ {
		A, B := loops[k], loops[k+1]
		za, zb := zs[k], zs[k+1]
		if sameCount {
			n := len(A)
			for i := 0; i < n; i++ {
				j := (i + 1) % n
				wall(A[i], A[j], B[j], za, za, zb)
				wall(A[i], B[j], B[i], za, zb, zb)
			}
			continue
		}
		// give both outlines each other's points: every point's position
		// along its outline is added to the other as a point on its edge
		// (so neither changes shape and each keeps its corners), then they
		// join point to point
		B = startNear(B, A)
		ta, tb := arcParams(A), arcParams(B)
		ts := mergeParams(ta[:len(A)], tb[:len(B)])
		pa, pb := atParams(A, ta, ts), atParams(B, tb, ts)
		n := len(ts)
		for i := 0; i < n; i++ {
			j := (i + 1) % n
			wall(pa[i], pa[j], pb[j], za, za, zb)
			wall(pa[i], pb[j], pb[i], za, zb, zb)
		}
	}
	capEnd := func(c []Pt, z float64, up bool) {
		for _, t := range Triangulate(Region{Outer: c}) {
			if up {
				m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{v3(t[0], z), v3(t[1], z), v3(t[2], z)}})
			} else {
				m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{v3(t[0], z), v3(t[2], z), v3(t[1], z)}})
			}
		}
	}
	capEnd(loops[0], zs[0], false)
	capEnd(loops[len(loops)-1], zs[len(zs)-1], true)
	// the caps' triangulation may skip a point lying in line with its
	// neighbours that the walls use: split those edges so the mesh closes
	return m.Clean().FixTJunctions()
}

// arcParams: each point's distance along the closed contour, as a
// fraction of its length, with 1 appended for the return to the start.
func arcParams(c []Pt) []float64 {
	n := len(c)
	t := make([]float64, n+1)
	for i := 1; i <= n; i++ {
		a, b := c[i-1], c[i%n]
		t[i] = t[i-1] + math.Hypot(b.X-a.X, b.Y-a.Y)
	}
	for i := range t {
		t[i] /= t[n]
	}
	return t
}

// mergeParams merges two sorted lists of positions along an outline (0 up
// to, not including, 1), dropping near-duplicates.
func mergeParams(a, b []float64) []float64 {
	out := make([]float64, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		var t float64
		if j >= len(b) || (i < len(a) && a[i] <= b[j]) {
			t, i = a[i], i+1
		} else {
			t, j = b[j], j+1
		}
		if len(out) == 0 || t-out[len(out)-1] > 1e-9 {
			out = append(out, t)
		}
	}
	return out
}

// atParams returns the points of closed contour c (with positions tc from
// arcParams) at positions ts.
func atParams(c []Pt, tc, ts []float64) []Pt {
	n := len(c)
	out := make([]Pt, len(ts))
	k := 0
	for i, t := range ts {
		for k < n-1 && tc[k+1] <= t {
			k++
		}
		a, b := c[k], c[(k+1)%n]
		f := 0.0
		if d := tc[k+1] - tc[k]; d > 0 {
			f = (t - tc[k]) / d
		}
		if f < 1e-9 {
			out[i] = a // the contour's own point, exactly
		} else {
			out[i] = Pt{a.X + f*(b.X-a.X), a.Y + f*(b.Y-a.Y)}
		}
	}
	return out
}

func centroid(c []Pt) Pt {
	var x, y float64
	for _, p := range c {
		x += p.X
		y += p.Y
	}
	return Pt{x / float64(len(c)), y / float64(len(c))}
}

// startNear rotates contour b to start at the point nearest a's start,
// measured from each contour's centre.
func startNear(b, a []Pt) []Pt {
	ca, cb := centroid(a), centroid(b)
	ref := Pt{a[0].X - ca.X, a[0].Y - ca.Y}
	best, bd := 0, math.Inf(1)
	for i, p := range b {
		if d := math.Hypot(p.X-cb.X-ref.X, p.Y-cb.Y-ref.Y); d < bd {
			best, bd = i, d
		}
	}
	return append(append([]Pt(nil), b[best:]...), b[:best]...)
}

// profileBuild builds a profile kind at size, falling back to a box for
// data it can't use (a damaged file).
func profileBuild(kind string, size geom.Vec3, v Values) *geom.Mesh {
	m := profileMesh(kind, v)
	if m == nil || len(m.Tris) == 0 {
		return roundedBox(size, 0, 1)
	}
	return m.FitTo(size)
}

// profileNatural caches profile bounds by data: the editor asks for a
// shape's natural size on every parameter change.
var profileNatural sync.Map

func profileNaturalSize(kind string) func(Values) geom.Vec3 {
	return func(v Values) geom.Vec3 {
		key := fmt.Sprintf("%s|%s|%v|%v", kind, SectionsKey(v.Sections), v.Get("smooth", 0), v.Get("steps", 8))
		if s, ok := profileNatural.Load(key); ok {
			return s.(geom.Vec3)
		}
		b := ProfileBounds(kind, v)
		s := geom.V(1, 1, 1)
		if !b.IsEmpty() {
			s = b.Size()
		}
		profileNatural.Store(key, s)
		return s
	}
}

// the default shapes, for placing from the API without data
var (
	defaultOutline = []Section{
		{Z: 0, Contours: [][][2]float64{{{-10, -10}, {10, -10}, {10, 4}, {0, 10}, {-10, 4}}}},
		{Z: 20, Contours: [][][2]float64{{{-10, -10}, {10, -10}, {10, 4}, {0, 10}, {-10, 4}}}},
	}
	defaultLoft = []Section{
		{Z: 0, Contours: [][][2]float64{{{-10, -10}, {10, -10}, {10, 10}, {-10, 10}}}},
		{Z: 20, Contours: [][][2]float64{{{-5, -5}, {5, -5}, {5, 5}, {-5, 5}}}},
	}
)

func init() {
	params := func() []Param {
		return []Param{
			slider("smooth", "Smooth", 0, 1, 1, 0, true),
			slider("steps", "Curve Steps", 2, 16, 1, 8, true),
		}
	}
	for _, d := range []struct {
		id, label, color string
		sections         []Section
	}{
		{"outline", "Outline", "#3fae2a", defaultOutline},
		{"loft", "Loft", "#1c9ad6", defaultLoft},
	} {
		id := d.id
		k := &Kind{ID: id, Label: d.label, Color: d.color, Params: params(), Hidden: true, sections: d.sections,
			build: func(size geom.Vec3, v Values) *geom.Mesh { return profileBuild(id, size, v) }}
		k.natural = profileNaturalSize(id)
		k.Size = k.natural(k.Defaults())
		register(k)
	}
}
