package shapes

import (
	"fmt"
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// The QR Code shape (registered in zz_added.go): a scannable code for its
// text, as an assembly of two parts so it prints in two colours: a base
// plate, including the blank margin a scanner needs, and the code's dark
// modules standing up from it. With Base at 0 it is the raised code alone,
// to stand on a part or, made a hole, to engrave one. Size is the whole
// width, margin included.
//
// "QR Code" is a registered trademark of DENSO WAVE INCORPORATED.

var qrLevels = []string{"Low (7%)", "Medium (15%)", "Quartile (25%)", "High (30%)"}

// CheckText says whether text is usable by a shape of kind (one with a
// text setting other than the Text shape): a QR code's text must fit
// version 10 at the lowest error correction.
func CheckText(kind, text string) error {
	switch kind {
	case "qrcode", "qrcode-base", "qrcode-code":
		if n := QRMaxBytes(qrLow); len(text) > n {
			return fmt.Errorf("too long for a QR code: %d bytes, at most %d (a letter, digit or space is 1 byte; an accented letter or symbol takes 2 to 4)", len(text), n)
		}
	}
	return nil
}

// qrFor is the code for v's text: at its error correction level, or the
// highest lower level it fits (a level raised after the text was typed).
func qrFor(v Values) qrSymbol {
	level := int(math.Max(0, math.Min(v.Get("level", qrMedium), qrHigh)))
	for l := level; l >= 0; l-- {
		if s, ok := qrEncode([]byte(v.Text), l); ok {
			return s
		}
	}
	s, _ := qrEncode(nil, qrLow) // CheckText keeps this from happening
	return s
}

type qrDims struct {
	sym                 qrSymbol
	size, u, base, high float64
	margin              int
}

func qrDimsOf(v Values) qrDims {
	sym := qrFor(v)
	margin := int(v.Get("margin", 4))
	size := v.Get("size", 40)
	return qrDims{sym: sym, size: size, u: size / float64(len(sym)+2*margin), base: v.Get("base", 1),
		high: v.Get("height", 0.6), margin: margin}
}

// baseTop is the base's thickness: none at (nearly) 0, else at least a
// printable 0.2 mm.
func (d qrDims) baseTop() float64 {
	if d.base <= 0.05 {
		return 0
	}
	return math.Max(d.base, 0.2)
}

func qrBase(v Values) *geom.Mesh {
	d := qrDimsOf(v)
	h := d.size / 2
	return ExtrudeContours([][]Pt{{{-h, -h}, {h, -h}, {h, h}, {-h, h}}}, 0, math.Max(d.baseTop(), 0.2))
}

// qrCode is the dark modules, raised from the base's top.
func qrCode(v Values) *geom.Mesh {
	d := qrDimsOf(v)
	z0 := d.baseTop()
	h := d.size / 2
	var cs [][]Pt
	for _, c := range qrContours(d.sym, 0.02) {
		out := make([]Pt, len(c))
		for i, p := range c { // module (column, row) → the plate, row 0 at the back
			out[i] = Pt{-h + (p.X+float64(d.margin))*d.u, h - (p.Y+float64(d.margin))*d.u}
		}
		cs = append(cs, reversed(out)) // the rows run down: flipping Y reverses the winding
	}
	return ExtrudeContours(cs, z0, z0+d.high)
}

func qrParts(v Values) []Placed {
	code := placedAsBuilt("qrcode-code", "QR Code", "#1e1e1e", v, qrCode, false)
	if v.Get("base", 1) <= 0.05 {
		return []Placed{code}
	}
	return []Placed{placedAsBuilt("qrcode-base", "QR Code Base", "#f2f2f2", v, qrBase, false), code}
}

// qrContours traces the dark modules' outlines in module units (x along
// the columns, y down the rows), each with the dark on its left as drawn
// with y pointing down (so clockwise in y-up coordinates). Modules that
// touch only at a corner are kept apart: there
// each outline keeps to its own module and its corner is pulled in by gap
// modules, so no two outlines share a point.
func qrContours(s qrSymbol, gap float64) [][]Pt {
	n := len(s)
	dark := func(x, y int) bool { return x >= 0 && y >= 0 && x < n && y < n && s[y][x] }
	type pt struct{ x, y int }
	// each boundary edge, directed with its dark module on the left (in
	// x-right, y-down coordinates the left of a step east is north)
	next := map[pt][]pt{}
	add := func(a, b pt) { next[a] = append(next[a], b) }
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if !dark(x, y) {
				continue
			}
			if !dark(x, y-1) { // top edge, going west
				add(pt{x + 1, y}, pt{x, y})
			}
			if !dark(x, y+1) { // bottom edge, going east
				add(pt{x, y + 1}, pt{x + 1, y + 1})
			}
			if !dark(x-1, y) { // left edge, going south
				add(pt{x, y}, pt{x, y + 1})
			}
			if !dark(x+1, y) { // right edge, going north
				add(pt{x + 1, y + 1}, pt{x + 1, y})
			}
		}
	}
	used := map[[2]pt]bool{}
	var out [][]Pt
	for y := 0; y <= n; y++ {
		for x := 0; x <= n; x++ {
			for _, first := range next[pt{x, y}] {
				start := pt{x, y}
				if used[[2]pt{start, first}] {
					continue
				}
				var loop []Pt
				prev, cur := start, first
				used[[2]pt{start, first}] = true
				for {
					// at a corner two outlines meet (diagonal modules), take
					// the turn that keeps to this outline's own module: with
					// the dark on the left, the left turn
					din := pt{cur.x - prev.x, cur.y - prev.y}
					cands := next[cur]
					nx := cands[0]
					if len(cands) > 1 {
						for _, c := range cands {
							d := pt{c.x - cur.x, c.y - cur.y}
							// left of din with y down: (din.y, -din.x)
							if d.x == din.y && d.y == -din.x {
								nx = c
							}
						}
					}
					dout := pt{nx.x - cur.x, nx.y - cur.y}
					p := Pt{float64(cur.x), float64(cur.y)}
					if len(cands) > 1 { // pulled in, into its own module
						p.X += gap * float64(dout.x-din.x)
						p.Y += gap * float64(dout.y-din.y)
					}
					if dout != din || len(cands) > 1 { // a corner (straight runs need no point)
						loop = append(loop, p)
					}
					used[[2]pt{cur, nx}] = true
					prev, cur = cur, nx
					if cur == first && prev == start {
						break
					}
				}
				out = append(out, loop)
			}
		}
	}
	return out
}
