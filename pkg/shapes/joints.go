package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Joints (registered in zz_added.go): a dovetail and an
// alignment pin, each an assembly of a key (a solid) and its socket (a
// hole), placed together where the two parts meet. Ungrouped, the key is
// grouped with one part and the socket with the other. The socket is the
// key grown by Clearance.

// --- dovetail --------------------------------------------------------------------------
//
// The joint's face is the plane y = 0: the key stands on the part on the
// -Y side (sunk 1 mm into it) and reaches into the part on the +Y side,
// widening as it goes, so the parts can only come apart by sliding along
// Z. It is Height tall, from z = 0.

type dovetailDims struct{ neck, length, flare, h, clr float64 }

func dovetailOf(v Values) dovetailDims {
	return dovetailDims{
		neck:   v.Get("neck", 8),
		length: v.Get("length", 8),
		flare:  v.Get("flare", 15) * math.Pi / 180,
		h:      v.Get("height", 5),
		clr:    v.Get("clearance", 0.2),
	}
}

// outline is the key's outline grown by c: the neck from y = from, then
// the flare out to the key's end (moved out by c).
func (d dovetailDims) outline(c, from float64) []Pt {
	n := d.neck/2 + c/math.Cos(d.flare)
	end := d.length + c
	t := n + end*math.Tan(d.flare)
	return []Pt{{-n, from}, {n, from}, {n, 0}, {t, end}, {-t, end}, {-n, 0}}
}

func dovetailKey(v Values) *geom.Mesh {
	d := dovetailOf(v)
	return ExtrudeContours([][]Pt{d.outline(0, -1)}, 0, d.h)
}

func dovetailSocket(v Values) *geom.Mesh {
	d := dovetailOf(v)
	return ExtrudeContours([][]Pt{d.outline(d.clr, -0.5)}, -0.5, d.h+0.5)
}

func dovetailParts(v Values) []Placed {
	return []Placed{
		placedAsBuilt("dovetail-key", "Dovetail Key", "#d35400", v, dovetailKey, false),
		placedAsBuilt("dovetail-socket", "Dovetail Socket", "#9a9a9a", v, dovetailSocket, true),
	}
}

// --- alignment pin ---------------------------------------------------------------------
//
// The joint's face is the plane z = 0: the pin stands on the part below
// (sunk 1 mm into it), with a chamfered tip so it finds its socket, and
// the socket goes up into the part above, half a millimetre deeper than
// the pin is long.

func pinDims(v Values) (r, l, clr, ch float64) {
	r = v.Get("diameter", 5) / 2
	l = v.Get("length", 6)
	clr = v.Get("clearance", 0.2)
	ch = math.Min(v.Get("chamfer", 0.6), math.Min(0.8*r, 0.5*l))
	return
}

func pinKey(v Values) *geom.Mesh {
	r, l, _, ch := pinDims(v)
	prof := [][2]float64{{0, -1}, {r, -1}, {r, l - ch}, {r - ch, l}, {0, l}}
	if ch < 1e-3 {
		prof = [][2]float64{{0, -1}, {r, -1}, {r, l}, {0, l}}
	}
	return revolve(prof, 48, false)
}

func pinSocket(v Values) *geom.Mesh {
	r, l, clr, _ := pinDims(v)
	return revolve([][2]float64{{0, -0.5}, {r + clr, -0.5}, {r + clr, l + 0.5}, {0, l + 0.5}}, 48, false)
}

func pinParts(v Values) []Placed {
	return []Placed{
		placedAsBuilt("alignpin-pin", "Alignment Pin", "#d35400", v, pinKey, false),
		placedAsBuilt("alignpin-socket", "Pin Socket", "#9a9a9a", v, pinSocket, true),
	}
}
