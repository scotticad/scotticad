package shapes

import (
	"encoding/xml"
	"errors"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/scotticad/scotticad/pkg/geom"
)

// affine is a 2D affine transform [a c e; b d f].
type affine [6]float64

var identity2 = affine{1, 0, 0, 1, 0, 0}

func (m affine) mul(n affine) affine {
	return affine{
		m[0]*n[0] + m[2]*n[1], m[1]*n[0] + m[3]*n[1],
		m[0]*n[2] + m[2]*n[3], m[1]*n[2] + m[3]*n[3],
		m[0]*n[4] + m[2]*n[5] + m[4], m[1]*n[4] + m[3]*n[5] + m[5],
	}
}

func (m affine) apply(p Pt) Pt { return Pt{m[0]*p.X + m[2]*p.Y + m[4], m[1]*p.X + m[3]*p.Y + m[5]} }

// svgElem is one drawn element's outlines.
type svgElem struct {
	cs [][]Pt
}

// SVGContours parses an SVG document and returns its outlines in
// millimetres with Y pointing up. Paths, rects, circles, ellipses,
// polygons and polylines are supported along with nested transforms.
func SVGContours(r io.Reader) ([][]Pt, error) {
	es, err := svgElements(r)
	if err != nil {
		return nil, err
	}
	var out [][]Pt
	for _, e := range es {
		out = append(out, e.cs...)
	}
	return out, nil
}

// svgStyle reads a presentation property from an element: its style
// attribute first, then the attribute itself; "" if neither sets it.
func svgStyle(a map[string]string, name string) string {
	for _, decl := range strings.Split(a["style"], ";") {
		if k, v, ok := strings.Cut(decl, ":"); ok && strings.TrimSpace(k) == name {
			return strings.TrimSpace(v)
		}
	}
	return strings.TrimSpace(a[name])
}

func svgElements(r io.Reader) ([]svgElem, error) {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	// the transform in force, inherited from groups
	type state struct {
		m affine
	}
	var stack []state
	cur := state{m: identity2}
	var out []svgElem
	rootSeen := false
	skip := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if skip > 0 {
				skip++
				continue
			}
			a := attrs(t)
			stack = append(stack, cur)
			name := t.Name.Local
			if name == "defs" || name == "clipPath" || name == "mask" || name == "symbol" || name == "title" || name == "metadata" {
				skip = 1
				continue
			}
			if name == "svg" && !rootSeen {
				rootSeen = true
				cur.m = cur.m.mul(rootTransform(a))
			}
			if tr, ok := a["transform"]; ok {
				cur.m = cur.m.mul(parseTransform(tr))
			}
			if svgStyle(a, "display") == "none" {
				skip = 1 // hidden, with everything inside it
				continue
			}
			var cs [][]Pt
			switch name {
			case "path":
				cs = parsePath(a["d"])
			case "rect":
				x, y, w, h := num(a["x"]), num(a["y"]), num(a["width"]), num(a["height"])
				if w > 0 && h > 0 {
					cs = [][]Pt{{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}}
				}
			case "circle":
				cs = [][]Pt{ellipse(num(a["cx"]), num(a["cy"]), num(a["r"]), num(a["r"]))}
			case "ellipse":
				cs = [][]Pt{ellipse(num(a["cx"]), num(a["cy"]), num(a["rx"]), num(a["ry"]))}
			case "polygon", "polyline":
				ns := numbers(a["points"])
				var c []Pt
				for i := 0; i+1 < len(ns); i += 2 {
					c = append(c, Pt{ns[i], ns[i+1]})
				}
				cs = [][]Pt{c}
			}
			var e svgElem
			for _, c := range cs {
				if len(c) < 3 {
					continue
				}
				tc := make([]Pt, len(c))
				for i, p := range c {
					q := cur.m.apply(p)
					tc[i] = Pt{q.X, -q.Y}
				}
				e.cs = append(e.cs, tc)
			}
			if len(e.cs) > 0 {
				out = append(out, e)
			}
		case xml.EndElement:
			if skip > 0 {
				skip--
				if skip > 0 {
					continue
				}
			}
			if len(stack) > 0 {
				cur = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no shapes found in SVG")
	}
	return out, nil
}

// SVGMesh extrudes an SVG's outlines (scaled by scale) to height mm with
// the bottom at z=0. A shape inside another is a hole in it (even-odd, as
// SVG importers for extruding do), so a circle inside a circle is a ring
// whichever elements they come from; shapes that cross or touch are joined
// into one (their overlap filled: an even-odd overlap would leave pieces
// meeting at single points, which no closed mesh can hold).
func SVGMesh(r io.Reader, height, scale float64) (*geom.Mesh, error) {
	cs, err := SVGContours(r)
	if err != nil {
		return nil, err
	}
	if scale <= 0 {
		scale = 1
	}
	for _, c := range cs {
		for i := range c {
			c[i].X *= scale
			c[i].Y *= scale
			// far beyond any printer (areas of such shapes overflow)
			if !(math.Abs(c[i].X) <= MaxImportSize && math.Abs(c[i].Y) <= MaxImportSize) {
				return nil, errTooLarge
			}
		}
	}
	var simple [][]Pt
	for _, c := range cs {
		simple = append(simple, untangle(cleanContour(c))...)
	}
	m := ExtrudeContours(joinCrossing(simple), 0, height)
	if len(m.Tris) == 0 {
		return nil, errors.New("SVG has no closed filled shapes")
	}
	return m, nil
}

// MaxImportSize is the largest coordinate (mm) an imported drawing or mesh
// may have: 10 km, far beyond any printer, and well inside what the
// geometry's arithmetic handles.
const MaxImportSize = 1e7

var errTooLarge = errors.New("the drawing is far too large (a coordinate over 10 km): check its units or scale")

// joinCrossing replaces each set of outlines that cross or touch one
// another by the outlines of the region they make (joined where they cross,
// holes where one lies inside another), leaving outlines that only nest
// apart (or stand apart) as they are, for nesting to make holes of.
func joinCrossing(cs [][]Pt) [][]Pt {
	bs := make([]bounds2, len(cs))
	for i, c := range cs {
		bs[i] = boundsOf([][]Pt{c})
	}
	parent := make([]int, len(cs))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	// sorted by left edge, so only outlines whose X ranges overlap are
	// compared
	idx := make([]int, len(cs))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return bs[idx[a]].x0 < bs[idx[b]].x0 })
	for a, i := range idx {
		for _, j := range idx[a+1:] {
			if bs[j].x0 > bs[i].x1 {
				break
			}
			if bs[i].overlaps(bs[j]) && find(i) != find(j) && outlinesMeet(cs[i], cs[j]) {
				parent[find(i)] = find(j)
			}
		}
	}
	groups := map[int][]int{}
	var order []int
	for i := range cs {
		r := find(i)
		if _, ok := groups[r]; !ok {
			order = append(order, r)
		}
		groups[r] = append(groups[r], i)
	}
	var out [][]Pt
	for _, r := range order {
		g := groups[r]
		if len(g) == 1 {
			out = append(out, cs[g[0]])
			continue
		}
		// worked out in 3D (each extruded), then cut through the middle for
		// the outlines: an outline inside an odd number of the others is a
		// hole (one touching its outline, which triangulating can't take),
		// the rest are joined
		var solid, holes []*geom.Mesh
		for _, i := range g {
			depth := 0
			for _, j := range g {
				if i != j && within(cs[i], cs[j]) && !(within(cs[j], cs[i]) && j < i) {
					depth++
				}
			}
			m := ExtrudeContours([][]Pt{cs[i]}, 0, 1)
			if depth%2 == 1 {
				holes = append(holes, m)
			} else {
				solid = append(solid, m)
			}
		}
		joined := unionTree(solid)
		if len(holes) > 0 {
			joined = geom.Subtract(joined, unionTree(holes)).FixTJunctions()
		}
		for _, loop := range geom.Slice(joined, 0.5) {
			c := make([]Pt, len(loop))
			for k, p := range loop {
				c[k] = Pt{p.X, p.Y}
			}
			out = append(out, c)
		}
	}
	return out
}

// untangle splits an outline that crosses itself (a bowtie, a curve whose
// closing line cuts back through it) at each crossing into loops that
// don't, which triangulate; the even-odd handling of the loops then gives
// the region. Outlines of more than 5000 points are left as they are (the
// search is quadratic).
func untangle(c []Pt) [][]Pt {
	budget := 300_000_000 // edge pairs checked, in all: a second or so
	return untangleN(c, &budget)
}

func untangleN(c []Pt, budget *int) [][]Pt {
	n := len(c)
	if n < 4 || *budget <= 0 {
		return [][]Pt{c}
	}
	for i := 0; i < n; i++ {
		p, q := c[i], c[(i+1)%n]
		*budget -= n - i
		if *budget <= 0 {
			return [][]Pt{c}
		}
		for j := i + 2; j < n; j++ {
			if (j+1)%n == i {
				continue // the edge before i: they share a corner
			}
			// the outline comes back through one of its own corners (two
			// curves sampled at the same point cross there, not inside an
			// edge): two loops meeting at that corner
			if !(i == 0 && j == n-1) && same(c[i], c[j]) {
				a := append([]Pt(nil), c[i:j]...)
				b := append(append([]Pt(nil), c[j:]...), c[:i]...)
				if len(a) >= 3 && len(b) >= 3 {
					return append(untangleN(a, budget), untangleN(b, budget)...)
				}
			}
			r, s := c[j], c[(j+1)%n]
			d1, d2 := cross2(p, q, r), cross2(p, q, s)
			d3, d4 := cross2(r, s, p), cross2(r, s, q)
			if !((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) || !((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
				continue
			}
			t := d3 / (d3 - d4)
			x := Pt{p.X + t*(q.X-p.X), p.Y + t*(q.Y-p.Y)}
			// the loop between the two edges, and the rest
			a := append([]Pt{x}, c[i+1:j+1]...)
			b := append([]Pt{x}, c[j+1:]...)
			b = append(b, c[:i+1]...)
			return append(untangleN(a, budget), untangleN(b, budget)...)
		}
	}
	return [][]Pt{c}
}

// within reports whether outline a lies inside outline b, touching it
// perhaps but not crossing it (a hole tangent to its outline: nesting, for
// the even-odd rule, not shapes to join). Every corner of a, and the
// middle of every side, is inside b or on its edge.
func within(a, b []Pt) bool {
	bb := boundsOf([][]Pt{b})
	eps := 1e-6 * math.Max(1, math.Max(bb.x1-bb.x0, bb.y1-bb.y0))
	onEdge := func(p Pt) bool {
		for k := range b {
			q, r := b[k], b[(k+1)%len(b)]
			dx, dy := r.X-q.X, r.Y-q.Y
			t := 0.0
			if l := dx*dx + dy*dy; l > 0 {
				t = math.Max(0, math.Min(1, ((p.X-q.X)*dx+(p.Y-q.Y)*dy)/l))
			}
			if math.Hypot(p.X-q.X-t*dx, p.Y-q.Y-t*dy) <= eps {
				return true
			}
		}
		return false
	}
	for k := range a {
		p, m := a[k], Pt{(a[k].X + a[(k+1)%len(a)].X) / 2, (a[k].Y + a[(k+1)%len(a)].Y) / 2}
		for _, q := range []Pt{p, m} {
			if !PointInPolygon(q, b) && !onEdge(q) {
				return false
			}
		}
	}
	return true
}

type bounds2 struct{ x0, y0, x1, y1 float64 }

func boundsOf(cs [][]Pt) bounds2 {
	b := bounds2{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, c := range cs {
		for _, p := range c {
			b.x0, b.y0, b.x1, b.y1 = math.Min(b.x0, p.X), math.Min(b.y0, p.Y), math.Max(b.x1, p.X), math.Max(b.y1, p.Y)
		}
	}
	return b
}

func (a bounds2) overlaps(b bounds2) bool {
	return a.x0 <= b.x1 && b.x0 <= a.x1 && a.y0 <= b.y1 && b.y0 <= a.y1
}

// unionTree unions meshes two by two: a balanced tree is far quicker than
// adding each to one growing mesh.
func unionTree(ms []*geom.Mesh) *geom.Mesh {
	switch len(ms) {
	case 0:
		return &geom.Mesh{}
	case 1:
		return ms[0]
	}
	h := len(ms) / 2
	return geom.Union(unionTree(ms[:h]), unionTree(ms[h:])).FixTJunctions()
}

// outlinesMeet reports whether two closed outlines cross or touch.
func outlinesMeet(a, b []Pt) bool {
	const eps = 1e-9
	side := func(p, q, r Pt) int {
		v := cross2(p, q, r)
		switch {
		case v > eps:
			return 1
		case v < -eps:
			return -1
		}
		return 0
	}
	onSeg := func(p, q, r Pt) bool { // r on segment p-q, given collinear
		return math.Min(p.X, q.X)-eps <= r.X && r.X <= math.Max(p.X, q.X)+eps &&
			math.Min(p.Y, q.Y)-eps <= r.Y && r.Y <= math.Max(p.Y, q.Y)+eps
	}
	for i := range a {
		p, q := a[i], a[(i+1)%len(a)]
		for j := range b {
			r, s := b[j], b[(j+1)%len(b)]
			if math.Max(r.X, s.X) < math.Min(p.X, q.X)-eps || math.Min(r.X, s.X) > math.Max(p.X, q.X)+eps ||
				math.Max(r.Y, s.Y) < math.Min(p.Y, q.Y)-eps || math.Min(r.Y, s.Y) > math.Max(p.Y, q.Y)+eps {
				continue
			}
			d1, d2, d3, d4 := side(p, q, r), side(p, q, s), side(r, s, p), side(r, s, q)
			if d1*d2 < 0 && d3*d4 < 0 {
				return true
			}
			if (d1 == 0 && onSeg(p, q, r)) || (d2 == 0 && onSeg(p, q, s)) || (d3 == 0 && onSeg(r, s, p)) || (d4 == 0 && onSeg(r, s, q)) {
				return true
			}
		}
	}
	return false
}

func attrs(t xml.StartElement) map[string]string {
	m := map[string]string{}
	for _, a := range t.Attr {
		m[a.Name.Local] = a.Value
	}
	return m
}

// rootTransform maps user units to millimetres using width/height and
// viewBox. Unitless and px lengths are 96 per inch.
func rootTransform(a map[string]string) affine {
	w, wu := length(a["width"])
	h, hu := length(a["height"])
	vb := numbers(a["viewBox"])
	toMM := func(v float64, u string) float64 {
		switch u {
		case "mm":
			return v
		case "cm":
			return v * 10
		case "in":
			return v * 25.4
		case "pt":
			return v * 25.4 / 72
		case "pc":
			return v * 25.4 / 6
		}
		return v * 25.4 / 96
	}
	if len(vb) == 4 && vb[2] > 0 && vb[3] > 0 {
		sx, sy := 25.4/96, 25.4/96
		if w > 0 {
			sx = toMM(w, wu) / vb[2]
		}
		if h > 0 {
			sy = toMM(h, hu) / vb[3]
		}
		if w > 0 && h <= 0 {
			sy = sx
		}
		if h > 0 && w <= 0 {
			sx = sy
		}
		tx, ty := 0.0, 0.0
		par := strings.Fields(a["preserveAspectRatio"])
		if w > 0 && h > 0 && (len(par) == 0 || par[0] != "none") {
			// the drawing keeps its proportions: scaled to fit inside the
			// width and height (meet, the default) or to cover them
			// (slice), placed by the alignment (xMidYMid by default)
			s := math.Min(sx, sy)
			if len(par) > 1 && par[1] == "slice" {
				s = math.Max(sx, sy)
			}
			align := "xMidYMid"
			if len(par) > 0 && len(par[0]) == 8 {
				align = par[0]
			}
			place := func(spare float64, how string) float64 {
				switch how {
				case "Min":
					return 0
				case "Max":
					return spare
				}
				return spare / 2
			}
			tx = place(toMM(w, wu)-vb[2]*s, align[1:4])
			ty = place(toMM(h, hu)-vb[3]*s, align[5:8])
			sx, sy = s, s
		}
		return affine{sx, 0, 0, sy, tx - vb[0]*sx, ty - vb[1]*sy}
	}
	s := toMM(1, "px")
	return affine{s, 0, 0, s, 0, 0}
}

func length(s string) (float64, string) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasSuffix(s, "%") {
		return 0, ""
	}
	i := len(s)
	for i > 0 && unicode.IsLetter(rune(s[i-1])) {
		i--
	}
	v, _ := strconv.ParseFloat(s[:i], 64)
	return v, s[i:]
}

func num(s string) float64 {
	v, _ := length(s)
	return v
}

func ellipse(cx, cy, rx, ry float64) []Pt {
	if rx <= 0 || ry <= 0 {
		return nil
	}
	n := 64
	c := make([]Pt, n)
	for i := range c {
		a := 2 * math.Pi * float64(i) / float64(n)
		c[i] = Pt{cx + rx*math.Cos(a), cy + ry*math.Sin(a)}
	}
	return c
}

// numbers extracts all numbers from s, handling "1-2", "1.5.5" and
// exponents the way SVG does.
func numbers(s string) []float64 {
	var out []float64
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9') {
			j := i
			if s[j] == '-' || s[j] == '+' {
				j++
			}
			dot := false
			for j < len(s) {
				if s[j] >= '0' && s[j] <= '9' {
					j++
				} else if s[j] == '.' && !dot {
					dot = true
					j++
				} else if (s[j] == 'e' || s[j] == 'E') && j+1 < len(s) {
					j++
					dot = true // an exponent has no fraction: a dot after it starts the next number
					if s[j] == '-' || s[j] == '+' {
						j++
					}
				} else {
					break
				}
			}
			v, err := strconv.ParseFloat(s[i:j], 64)
			if err == nil {
				out = append(out, v)
			}
			if j == i {
				j++
			}
			i = j
			continue
		}
		i++
	}
	return out
}

func parseTransform(s string) affine {
	m := identity2
	for {
		s = strings.TrimLeft(s, " ,\t\n")
		i := strings.Index(s, "(")
		j := strings.Index(s, ")")
		if i < 0 || j < i {
			break
		}
		name := strings.TrimSpace(s[:i])
		a := numbers(s[i+1 : j])
		s = s[j+1:]
		var t affine
		switch name {
		case "matrix":
			if len(a) == 6 {
				t = affine{a[0], a[1], a[2], a[3], a[4], a[5]}
			} else {
				t = identity2
			}
		case "translate":
			t = identity2
			if len(a) >= 1 {
				t[4] = a[0]
			}
			if len(a) >= 2 {
				t[5] = a[1]
			}
		case "scale":
			t = identity2
			if len(a) >= 1 {
				t[0], t[3] = a[0], a[0]
			}
			if len(a) >= 2 {
				t[3] = a[1]
			}
		case "rotate":
			if len(a) == 0 {
				t = identity2
				break
			}
			r := a[0] * math.Pi / 180
			c, sn := math.Cos(r), math.Sin(r)
			t = affine{c, sn, -sn, c, 0, 0}
			if len(a) == 3 {
				t = affine{1, 0, 0, 1, a[1], a[2]}.mul(t).mul(affine{1, 0, 0, 1, -a[1], -a[2]})
			}
		case "skewX":
			t = identity2
			if len(a) == 1 {
				t[2] = math.Tan(a[0] * math.Pi / 180)
			}
		case "skewY":
			t = identity2
			if len(a) == 1 {
				t[1] = math.Tan(a[0] * math.Pi / 180)
			}
		default:
			t = identity2
		}
		m = m.mul(t)
	}
	return m
}

// parsePath flattens SVG path data into closed contours.
func parsePath(d string) [][]Pt {
	var out [][]Pt
	var c []Pt
	var p, start, ctrl Pt
	var lastCmd byte
	flush := func() {
		if len(c) >= 3 {
			out = append(out, c)
		}
		c = nil
	}
	i := 0
	nextCmd := func() byte {
		for i < len(d) {
			ch := d[i]
			if unicode.IsLetter(rune(ch)) && ch != 'e' && ch != 'E' {
				i++
				return ch
			}
			if ch == '-' || ch == '+' || ch == '.' || (ch >= '0' && ch <= '9') {
				return 0 // implicit repeat
			}
			i++
		}
		return 255
	}
	readNums := func(n int) ([]float64, bool) {
		vals := make([]float64, 0, n)
		for len(vals) < n {
			for i < len(d) && (d[i] == ' ' || d[i] == ',' || d[i] == '\t' || d[i] == '\n' || d[i] == '\r') {
				i++
			}
			if i >= len(d) {
				return nil, false
			}
			ch := d[i]
			if !(ch == '-' || ch == '+' || ch == '.' || (ch >= '0' && ch <= '9')) {
				return nil, false
			}
			j := i
			if d[j] == '-' || d[j] == '+' {
				j++
			}
			dot := false
			for j < len(d) {
				if d[j] >= '0' && d[j] <= '9' {
					j++
				} else if d[j] == '.' && !dot {
					dot = true
					j++
				} else if (d[j] == 'e' || d[j] == 'E') && j+1 < len(d) {
					j++
					dot = true // an exponent has no fraction: a dot after it starts the next number
					if d[j] == '-' || d[j] == '+' {
						j++
					}
				} else {
					break
				}
			}
			v, err := strconv.ParseFloat(d[i:j], 64)
			if err != nil {
				return nil, false
			}
			vals = append(vals, v)
			i = j
		}
		return vals, true
	}
	// arc flags may be written without separators ("a1 1 0 01-1 1")
	readFlag := func() (float64, bool) {
		for i < len(d) && (d[i] == ' ' || d[i] == ',' || d[i] == '\t' || d[i] == '\n' || d[i] == '\r') {
			i++
		}
		if i < len(d) && (d[i] == '0' || d[i] == '1') {
			v := float64(d[i] - '0')
			i++
			return v, true
		}
		return 0, false
	}
	cmd := byte(0)
	for {
		nc := nextCmd()
		if nc == 255 {
			break
		}
		if nc != 0 {
			cmd = nc
		} else if cmd == 'Z' || cmd == 'z' {
			i++ // numbers after a close mean nothing: skip them
			continue
		} else if cmd == 'M' {
			cmd = 'L'
		} else if cmd == 'm' {
			cmd = 'l'
		} else if cmd == 0 {
			i++
			continue
		}
		rel := cmd >= 'a' && cmd <= 'z'
		abs := func(x, y float64) Pt {
			if rel {
				return Pt{p.X + x, p.Y + y}
			}
			return Pt{x, y}
		}
		upper := cmd &^ 0x20
		ok := true
		switch upper {
		case 'Z':
			flush()
			p = start
			// a drawing command after the close starts the next outline
			// there (a move replaces it)
			c = []Pt{start}
		case 'M':
			var v []float64
			if v, ok = readNums(2); ok {
				flush()
				p = abs(v[0], v[1])
				start = p
				c = []Pt{p}
			}
		case 'L':
			var v []float64
			if v, ok = readNums(2); ok {
				p = abs(v[0], v[1])
				c = append(c, p)
			}
		case 'H':
			var v []float64
			if v, ok = readNums(1); ok {
				if rel {
					p.X += v[0]
				} else {
					p.X = v[0]
				}
				c = append(c, p)
			}
		case 'V':
			var v []float64
			if v, ok = readNums(1); ok {
				if rel {
					p.Y += v[0]
				} else {
					p.Y = v[0]
				}
				c = append(c, p)
			}
		case 'C', 'S':
			var v []float64
			n := 6
			if upper == 'S' {
				n = 4
			}
			if v, ok = readNums(n); ok {
				var c1 Pt
				if upper == 'S' {
					c1 = p
					if l := lastCmd &^ 0x20; l == 'C' || l == 'S' {
						c1 = Pt{2*p.X - ctrl.X, 2*p.Y - ctrl.Y}
					}
					v = append([]float64{0, 0}, v...)
				} else {
					c1 = abs(v[0], v[1])
				}
				c2 := abs(v[2], v[3])
				e := abs(v[4], v[5])
				c = append(c, cubic(p, c1, c2, e)...)
				ctrl = c2
				p = e
			}
		case 'Q', 'T':
			var v []float64
			n := 4
			if upper == 'T' {
				n = 2
			}
			if v, ok = readNums(n); ok {
				var q Pt
				var e Pt
				if upper == 'T' {
					q = p
					if l := lastCmd &^ 0x20; l == 'Q' || l == 'T' {
						q = Pt{2*p.X - ctrl.X, 2*p.Y - ctrl.Y}
					}
					e = abs(v[0], v[1])
				} else {
					q = abs(v[0], v[1])
					e = abs(v[2], v[3])
				}
				c = append(c, quad(p, q, e)...)
				ctrl = q
				p = e
			}
		case 'A':
			var v []float64
			if v, ok = readNums(3); ok {
				var fa, fs float64
				var ok1, ok2 bool
				fa, ok1 = readFlag()
				fs, ok2 = readFlag()
				var xy []float64
				var ok3 bool
				xy, ok3 = readNums(2)
				ok = ok1 && ok2 && ok3
				if ok {
					e := abs(xy[0], xy[1])
					c = append(c, svgArc(p, v[0], v[1], v[2], fa != 0, fs != 0, e)...)
					p = e
				}
			}
		default:
			ok = false
		}
		if !ok {
			// malformed data: skip a character to make progress
			if i < len(d) {
				i++
			}
		}
		lastCmd = cmd
	}
	flush()
	return out
}

func cubic(p0, p1, p2, p3 Pt) []Pt {
	n := 16
	out := make([]Pt, 0, n)
	for k := 1; k <= n; k++ {
		t := float64(k) / float64(n)
		u := 1 - t
		out = append(out, Pt{
			u*u*u*p0.X + 3*u*u*t*p1.X + 3*u*t*t*p2.X + t*t*t*p3.X,
			u*u*u*p0.Y + 3*u*u*t*p1.Y + 3*u*t*t*p2.Y + t*t*t*p3.Y,
		})
	}
	return out
}

func quad(p0, p1, p2 Pt) []Pt {
	n := 12
	out := make([]Pt, 0, n)
	for k := 1; k <= n; k++ {
		t := float64(k) / float64(n)
		u := 1 - t
		out = append(out, Pt{u*u*p0.X + 2*u*t*p1.X + t*t*p2.X, u*u*p0.Y + 2*u*t*p1.Y + t*t*p2.Y})
	}
	return out
}

// svgArc implements the SVG endpoint arc parameterisation.
func svgArc(p0 Pt, rx, ry, phiDeg float64, large, sweep bool, p1 Pt) []Pt {
	if rx == 0 || ry == 0 {
		return []Pt{p1}
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	phi := phiDeg * math.Pi / 180
	cp, sp := math.Cos(phi), math.Sin(phi)
	dx, dy := (p0.X-p1.X)/2, (p0.Y-p1.Y)/2
	x1 := cp*dx + sp*dy
	y1 := -sp*dx + cp*dy
	lam := x1*x1/(rx*rx) + y1*y1/(ry*ry)
	if lam > 1 {
		s := math.Sqrt(lam)
		rx, ry = rx*s, ry*s
	}
	num := rx*rx*ry*ry - rx*rx*y1*y1 - ry*ry*x1*x1
	den := rx*rx*y1*y1 + ry*ry*x1*x1
	co := 0.0
	if den > 0 && num > 0 {
		co = math.Sqrt(num / den)
	}
	if large == sweep {
		co = -co
	}
	cx1 := co * rx * y1 / ry
	cy1 := -co * ry * x1 / rx
	cx := cp*cx1 - sp*cy1 + (p0.X+p1.X)/2
	cy := sp*cx1 + cp*cy1 + (p0.Y+p1.Y)/2
	ang := func(ux, uy, vx, vy float64) float64 {
		return math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	}
	t1 := ang(1, 0, (x1-cx1)/rx, (y1-cy1)/ry)
	dt := ang((x1-cx1)/rx, (y1-cy1)/ry, (-x1-cx1)/rx, (-y1-cy1)/ry)
	if !sweep && dt > 0 {
		dt -= 2 * math.Pi
	} else if sweep && dt < 0 {
		dt += 2 * math.Pi
	}
	n := int(math.Ceil(math.Abs(dt) / (math.Pi / 24)))
	if n < 2 {
		n = 2
	}
	out := make([]Pt, 0, n)
	for k := 1; k <= n; k++ {
		t := t1 + dt*float64(k)/float64(n)
		x, y := rx*math.Cos(t), ry*math.Sin(t)
		p := Pt{cp*x - sp*y + cx, sp*x + cp*y + cy}
		if math.IsNaN(p.X+p.Y) || math.IsInf(p.X+p.Y, 0) {
			// radii or ends so far apart the arithmetic overflows: a
			// straight line to the end instead of points that are nowhere
			return []Pt{p1}
		}
		out = append(out, p)
	}
	return out
}
