package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Gridfinity bins and baseplates (registered in zz_added.go), built to the
// open Gridfinity storage standard: a 42 mm grid, bins in 7 mm height
// units. The standard's measures:
//
//   - A bin is 42 mm a unit less 0.5 mm, its corners rounded 3.75 mm. Under
//     each unit is a foot: from its bottom, a 0.8 mm 45° chamfer, 1.8 mm
//     straight, then a 2.15 mm 45° chamfer out to the bin's 41.5 mm, 4.75 mm
//     up. The bin's floor is 7 mm up, and its height in units includes the
//     base: 7 mm a unit.
//   - The stacking lip goes on top of that height: from its bottom, 2.6 mm in
//     from the outside, a 0.7 mm chamfer, 1.8 mm straight, then a 1.9 mm
//     chamfer out to the outside, 4.4 mm up. It is the shape a foot seats in,
//     so bins stack. Below it the wall widens at 45° to carry it.
//   - A baseplate is 42 mm a unit, its corners rounded 4 mm, 5 mm tall. Each
//     cell is a pocket: from the top, a 2.15 mm chamfer, 1.8 mm straight,
//     then a 0.7 mm chamfer in to 2.85 mm, and straight on down.
//   - Magnet holes are 6.5 mm across and 2.4 mm deep, screw holes 3 mm, at
//     ±13 mm from each cell's centre both ways.
//
// The lip and the baseplate's ridges would come to a knife edge at the top.
// Like other generators (which round it) these cut it flat: the lip's top
// is 0.4 mm wide, 0.4 mm lower than 4.4 mm, and the ridges 0.5 mm wide
// (0.25 mm round the edge), 0.25 mm lower than 5 mm. A foot seats on the
// chamfers below, which are unchanged.

const (
	gfGrid   = 42.0
	gfUnitH  = 7.0
	gfGap    = 0.5  // a bin is this much smaller than its units
	gfBinR   = 3.75 // a bin's corner radius
	gfFoot   = 4.75 // a foot's height
	gfFloor  = 7.0  // the bin's floor, from its bottom
	gfLip    = 4.4  // the stacking lip's height
	gfLipW   = 2.6  // and depth, in from the outside
	gfLipCut = 0.4  // the lip's knife edge cut flat this far
	gfPlateR = 4.0  // a baseplate's corner radius
	gfPlateH = 5.0  // a baseplate's pockets' height
	gfRidge  = 0.25 // the baseplate's knife edges cut flat this far
	gfHole   = 13.0 // magnet and screw holes, from a cell's centre
	gfMagD   = 6.5
	gfMagH   = 2.4
	gfScrewD = 3.0
	gfScrewH = 6.0
	gfDivT   = 1.2 // a divider's thickness
)

// insetLoft is a solid whose sections are a w × d rectangle with corners
// rounded r, each inset by an amount at a height: prof is (height, inset)
// from the bottom up. A negative inset grows the rectangle. Every section
// keeps a rounded corner, so all have the same points and the walls join
// corner to corner.
func insetLoft(w, d, r float64, prof [][2]float64) *geom.Mesh {
	loops := make([][]Pt, len(prof))
	zs := make([]float64, len(prof))
	for i, p := range prof {
		in := p[1]
		zs[i] = p[0]
		loops[i] = roundedRect(w-2*in, d-2*in, math.Max(r-in, 0.05))
	}
	return loftMesh(loops, zs)
}

// clipProfile keeps the part of an (height, inset) profile from z up,
// starting with the inset it has at z. Nil if nothing is left.
func clipProfile(prof [][2]float64, z float64) [][2]float64 {
	for i := 1; i < len(prof); i++ {
		a, b := prof[i-1], prof[i]
		if b[0] <= z {
			continue
		}
		f := (z - a[0]) / (b[0] - a[0])
		out := [][2]float64{{z, a[1] + math.Max(0, f)*(b[1]-a[1])}}
		if f < 0 {
			out[0] = a
		}
		return append(out, prof[i:]...)
	}
	return nil
}

// gfCells are the centres of an nx × ny grid of cells, centred on the
// origin.
func gfCells(nx, ny int) [][2]float64 {
	var out [][2]float64
	for j := 0; j < ny; j++ {
		for i := 0; i < nx; i++ {
			out = append(out, [2]float64{(float64(i) - float64(nx-1)/2) * gfGrid, (float64(j) - float64(ny-1)/2) * gfGrid})
		}
	}
	return out
}

// gfHoles are a cell's four holes: a magnet's, a screw's, or a magnet's
// with a screw's going on up from it, each from z0 up (the magnet's to
// z0+magnet, the screw's to z0+screw).
func gfHoles(c [2]float64, magnet, screw, z0 float64) []*geom.Mesh {
	var prof [][2]float64
	switch {
	case magnet > 0 && screw > magnet:
		prof = [][2]float64{{0, z0}, {gfMagD / 2, z0}, {gfMagD / 2, z0 + magnet}, {gfScrewD / 2, z0 + magnet}, {gfScrewD / 2, z0 + screw}, {0, z0 + screw}}
	case magnet > 0:
		prof = [][2]float64{{0, z0}, {gfMagD / 2, z0}, {gfMagD / 2, z0 + magnet}, {0, z0 + magnet}}
	default:
		prof = [][2]float64{{0, z0}, {gfScrewD / 2, z0}, {gfScrewD / 2, z0 + screw}, {0, z0 + screw}}
	}
	hole := revolve(prof, 32, false)
	var out []*geom.Mesh
	for _, s := range [][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		out = append(out, hole.Transform(geom.Translate(geom.V(c[0]+s[0]*gfHole, c[1]+s[1]*gfHole, 0))))
	}
	return out
}

func gfUnits(v Values, key string, def float64) int {
	return max(1, min(10, int(math.Round(v.Get(key, def)))))
}

// along X: a profile drawn in (y, z) and extruded from x0 to x1.
func prismX(prof []Pt, x0, x1 float64) *geom.Mesh {
	if SignedArea(prof) < 0 {
		prof = reversed(prof)
	}
	return ExtrudeContours([][]Pt{prof}, x0, x1).Transform(alongX)
}

// gfBinModel is a Gridfinity bin: Units Long × Units Wide, Units High tall,
// with or without the stacking lip, magnet and screw holes under its feet,
// compartments, a label shelf along the back of each row and a scoop along
// the front of each.
func gfBinModel(v Values) *geom.Mesh {
	nx, ny := gfUnits(v, "unitsx", 2), gfUnits(v, "unitsy", 1)
	hu := max(1, min(20, int(math.Round(v.Get("unitsz", 3)))))
	lip := v.Get("lip", 1) >= 0.5
	wall := math.Max(0.4, math.Min(v.Get("wall", 1.2), 3))
	L, W := float64(nx)*gfGrid-gfGap, float64(ny)*gfGrid-gfGap
	top := float64(hu) * gfUnitH // the walls' top, where the lip starts

	// the outside, from the feet's top up, less the inside
	T := top
	var inside [][2]float64
	if lip {
		T = top + gfLip - gfLipCut
		// (a wall thicker than the lip steps out to it at the top: the
		// slope under the lip would run downwards, folding the cutter)
		inside = [][2]float64{{-100, wall}, {math.Min(top, top-(gfLipW-wall)), wall}, {top, gfLipW},
			{top + 0.7, gfLipW - 0.7}, {top + 2.5, gfLipW - 0.7}, {top + gfLip, 0}, {top + gfLip + 1, -1}}
	} else {
		inside = [][2]float64{{-100, wall}, {top + 1, wall}}
	}
	body := insetLoft(L, W, gfBinR, [][2]float64{{gfFoot, 0}, {T, 0}})
	if p := clipProfile(inside, gfFloor); p != nil && gfFloor < T {
		body = geom.Subtract(body, insetLoft(L, W, gfBinR, p))
	}

	// the feet, which don't touch each other, and what goes inside (which
	// doesn't touch them) are joined to the walls in one go: one union with
	// many separate pieces is far quicker than many unions
	var feet []*geom.Mesh
	cells := gfCells(nx, ny)
	foot := insetLoft(gfGrid-gfGap, gfGrid-gfGap, gfBinR, [][2]float64{{0, 2.95}, {0.8, 2.15}, {2.6, 2.15}, {gfFoot, 0}})
	for _, c := range cells {
		feet = append(feet, foot.Transform(geom.Translate(geom.V(c[0], c[1], 0))))
	}
	var parts []*geom.Mesh

	// compartments: dividers across the inside, up to the walls' top, or
	// with the lip 1.2 mm below it: the foot of a bin stacked on this one
	// reaches 0.35 mm down past the lip's bottom
	li, wi := L-2*wall, W-2*wall
	cx, cy := gfUnits(v, "compx", 1), gfUnits(v, "compy", 1)
	if lip {
		top -= 1.2
	}
	inner := top > gfFloor+1
	var rows []float64 // each row's front, then its back (the inside's y)
	if inner {
		for k := 1; k < cx; k++ {
			x := -li/2 + float64(k)*(li+gfDivT)/float64(cx) - gfDivT/2
			parts = append(parts, cuboid(x-gfDivT/2, -W/2, gfFloor-0.5, x+gfDivT/2, W/2, top))
		}
		rows = append(rows, -wi/2)
		for k := 1; k < cy; k++ {
			y := -wi/2 + float64(k)*(wi+gfDivT)/float64(cy) - gfDivT/2
			parts = append(parts, cuboid(-L/2, y-gfDivT/2, gfFloor-0.5, L/2, y+gfDivT/2, top))
			rows = append(rows, y-gfDivT/2, y+gfDivT/2)
		}
		rows = append(rows, wi/2)
	}
	room := top - gfFloor // the compartments' depth
	rowDepth := (wi-float64(cy-1)*gfDivT)/float64(cy) - 1
	x0, x1 := -L/2, L/2
	// a scoop: a quarter-round fillet along each row's front, at the floor
	if r := math.Min(v.Get("scoop", 0), math.Min(room, rowDepth)); inner && r > 0.2 {
		for k := 0; k < len(rows); k += 2 {
			y := rows[k]
			prof := []Pt{{y - 0.3, gfFloor - 0.3}, {y + r, gfFloor - 0.3}}
			for i := 0; i <= 16; i++ {
				a := math.Pi / 2 * float64(i) / 16
				prof = append(prof, Pt{y + r - r*math.Sin(a), gfFloor + r - r*math.Cos(a)})
			}
			prof = append(prof, Pt{y - 0.3, gfFloor + r})
			parts = append(parts, prismX(prof, x0, x1))
		}
	}
	// a label shelf: a ledge along each row's back, at the walls' top, its
	// underside at 45° so it prints without support
	if d := math.Min(v.Get("label", 0), math.Min(room-1.5, rowDepth)); inner && d > 1 {
		const lt = 1.2 // the shelf's front edge
		for k := 1; k < len(rows); k += 2 {
			y := rows[k]
			prof := []Pt{{y + 0.3, top}, {y - d, top}, {y - d, top - lt}, {y, top - lt - d}, {y + 0.3, top - lt - d}}
			parts = append(parts, prismX(prof, x0, x1))
		}
	}
	// what goes inside runs into the walls square-ended, and at the bin's
	// rounded corners that would come out the far side: keep what is within
	// the walls' middle
	fill := union(parts...)
	if len(parts) > 0 {
		fill = geom.Intersect(fill, insetLoft(L, W, gfBinR, [][2]float64{{gfFloor - 1, wall / 2}, {T + 1, wall / 2}}))
	}
	m := geom.Union(body, geom.Merge(append(feet, fill)...))

	magnet, screw := 0.0, 0.0
	switch int(v.Get("holes", 0)) {
	case 1:
		magnet = gfMagH
	case 2:
		screw = gfScrewH
	case 3:
		magnet, screw = gfMagH, gfScrewH
	}
	if magnet > 0 || screw > 0 {
		var holes []*geom.Mesh
		for _, c := range cells {
			// from below the bottom, so no face lies on it
			holes = append(holes, gfHoles(c, magnet+1, screw+1, -1)...)
		}
		m = geom.Subtract(m, geom.Merge(holes...))
	}
	return done(m)
}

// gfBaseplateModel is a Gridfinity baseplate: Units Long × Units Wide pockets
// bins drop into, open underneath, on a solid floor, or on a floor with a
// magnet hole under each of a bin's.
func gfBaseplateModel(v Values) *geom.Mesh {
	nx, ny := gfUnits(v, "unitsx", 3), gfUnits(v, "unitsy", 2)
	bottom := int(v.Get("bottom", 0))
	F := 0.0
	switch bottom {
	case 1:
		F = math.Max(0.4, v.Get("floor", 3))
	case 2:
		F = math.Max(gfMagH+0.6, v.Get("floor", 3))
	}
	L, W := float64(nx)*gfGrid, float64(ny)*gfGrid
	T := F + gfPlateH - gfRidge
	m := insetLoft(L, W, gfPlateR, [][2]float64{{0, 0}, {T, 0}})
	z0 := F
	if bottom == 0 {
		z0 = -1
	}
	H := F + gfPlateH
	// each pocket stops 0.2 mm above the top, still apart from the next, so
	// all of them are cut in one go
	pocket := [][2]float64{{z0, 2.85}, {H - 4.65, 2.85}, {H - 3.95, 2.15}, {H - 2.15, 2.15}, {T + 0.2, gfRidge - 0.2}}
	var cutters []*geom.Mesh
	cell := insetLoft(gfGrid, gfGrid, gfPlateR, pocket)
	for _, c := range gfCells(nx, ny) {
		p := cell.Transform(geom.Translate(geom.V(c[0], c[1], 0)))
		if bottom == 2 { // the magnet holes reach up into the pocket
			p = geom.Union(p, geom.Merge(gfHoles(c, gfMagH+0.5, 0, F-gfMagH)...))
		}
		cutters = append(cutters, p)
	}
	return done(geom.Subtract(m, geom.Merge(cutters...)))
}
