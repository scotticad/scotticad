package shapes

import (
	"fmt"
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// More hardware holes: the keyhole slot and the bearing seat
// (registered in zz_added.go). Like the countersunk hole they cut down
// into a part's top face, and reach half a millimetre above it so the
// cut is clean.

// --- keyhole slot ----------------------------------------------------------------------

// keyholeOutline is a round hole of radius R at the origin joined to a
// slot of half-width w running along +X to a round end centred at L,
// counter-clockwise.
func keyholeOutline(R, w, L float64, n int) []Pt {
	a0 := math.Asin(w / R)
	x0 := R * math.Cos(a0)
	var out []Pt
	segs := 2 * max(4, n/2) // even: the point at 180° is on it
	for i := 0; i <= segs; i++ {
		a := a0 + (2*math.Pi-2*a0)*float64(i)/float64(segs)
		out = append(out, Pt{R * math.Cos(a), R * math.Sin(a)})
	}
	// the slot's lower side, its round end, its upper side back to (x0, w)
	if L-x0 > 1e-6 {
		out = append(out, Pt{L, -w})
	} else {
		out[len(out)-1] = Pt{L, -w}
	}
	end := max(4, n/4)
	for i := 1; i < end; i++ {
		a := -math.Pi/2 + math.Pi*float64(i)/float64(end)
		out = append(out, Pt{L + w*math.Cos(a), w * math.Sin(a)})
	}
	if L-x0 > 1e-6 {
		out = append(out, Pt{L, w})
	} else {
		out[0] = Pt{L, w}
	}
	return out
}

// keyholeModel is a keyhole for hanging a part on a screw: the head goes
// in through the round hole and slides along the slot, where the slot's
// lip holds it. Below the lip a wider channel (Recess Depth deep) gives
// the head room to slide; with no recess it is a plain keyhole cut.
func keyholeModel(v Values) *geom.Mesh {
	R := v.Get("head", 8) / 2
	w := math.Min(v.Get("shank", 4)/2, R-0.4)
	L := math.Max(v.Get("slot", 8), R)
	depth := v.Get("depth", 6)
	rec := math.Min(v.Get("recess", 3), depth-0.4)
	const n = 48
	if rec < 0.05 {
		return ExtrudeContours([][]Pt{keyholeOutline(R, w, L, n)}, 0, depth+0.5)
	}
	// the channel is a little wider than the head's hole: room for the head
	// to slide, and no wall of the one exactly on a wall of the other
	channel := ExtrudeContours([][]Pt{slotOutlineAt(R+0.25, L, n)}, 0, rec)
	lip := ExtrudeContours([][]Pt{keyholeOutline(R, w, L, n)}, rec, depth+0.5)
	return done(geom.Union(channel, lip))
}

// slotOutlineAt is a stadium of radius r from the origin to (L, 0).
func slotOutlineAt(r, L float64, n int) []Pt {
	out := slotOutline(L+2*r, 2*r, n/2)
	for i := range out {
		out[i].X += L / 2
	}
	return out
}

// --- bearing seat ----------------------------------------------------------------------

// bearing is a ball bearing's bore, outside diameter and width, in mm.
type bearing struct {
	name       string
	d, od, wid float64
}

// bearings are the common small bearings, by size.
var bearings = []bearing{
	{"MR105", 5, 10, 4},
	{"623", 3, 10, 4},
	{"624", 4, 13, 5},
	{"625", 5, 16, 5},
	{"688", 8, 16, 5},
	{"626", 6, 19, 6},
	{"608", 8, 22, 7},
	{"6000", 10, 26, 8},
	{"6001", 12, 28, 8},
}

// bearingNames are the bearings as the Bearing dropdown lists them: the
// name, then bore × outside × width.
func bearingNames() []string {
	var out []string
	for _, b := range bearings {
		out = append(out, fmt.Sprintf("%s: %g × %g × %g", b.name, b.d, b.od, b.wid))
	}
	return out
}

const (
	seatBevel = 1.0 // the 45° bevel round the seat's floor
	seatGap   = 0.5 // how much deeper the seat is than the bearing is wide
)

// bearingSeatModel is a pocket a bearing presses into. Its floor's edge is
// bevelled at 45°, so the bearing's outer race stops on the bevel and its
// inner race, which turns, stays clear of the floor. Where on the bevel
// the race stops depends on the radius of its own edge, so the bearing's
// face ends level with the part's top or up to 0.45 mm above it. Fit is
// added to the bearing's outside diameter; Hole
// Below carries on through the floor, narrower than the outer race, so a
// rod can push the bearing out and a shaft can pass.
func bearingSeatModel(v Values) *geom.Mesh {
	b := bearings[int(math.Max(0, math.Min(v.Get("bearing", 6), float64(len(bearings)-1))))]
	R := b.od/2 + v.Get("fit", 0.1)/2
	top := seatGap + b.wid + 0.5
	below := v.Get("below", 2)
	var prof [][2]float64
	if below > 0.05 {
		rt := math.Min((b.od+b.d)/4, R-seatBevel-0.2)
		prof = [][2]float64{{0, -below}, {rt, -below}, {rt, 0}}
	} else {
		prof = [][2]float64{{0, 0}}
	}
	prof = append(prof, [2]float64{R - seatBevel, 0}, [2]float64{R, seatBevel}, [2]float64{R, top}, [2]float64{0, top})
	return revolve(prof, 96, false)
}
