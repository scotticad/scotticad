package shapes

import (
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
)

// The thread generator: an external ISO metric thread (a threaded rod),
// written from the ISO basic profile (ISO 68-1) — 60° flanks, thread depth
// 5/8 of the fundamental triangle height H = √3/2 × pitch, a crest flat of
// pitch/8 and a root flat of pitch/4 — swept as a right-handed helix.

// ThreadMesh builds a threaded rod filling size: major diameter size.X
// (size.Y stretches it into an ellipse if different), length size.Z,
// centred on the origin. The thread's phase starts 3/8 of a pitch above
// the bottom, so a rod (turns + 0.75) × pitch long has whole turns between
// run-outs of about half a pitch at each end.
// sides is the number of facets around; depthScale scales the thread depth
// (1 = standard).
func ThreadMesh(size geom.Vec3, pitch float64, sides int, depthScale float64) *geom.Mesh {
	return ThreadMeshTapered(size, pitch, sides, depthScale, 0, 0)
}

// ThreadMeshTapered is ThreadMesh with a tapered tip at the top: over the
// last taperTurns turns the rod narrows, ending at (1 − taper) of its
// radius (taper 0: no tip, 1: a point).
func ThreadMeshTapered(size geom.Vec3, pitch float64, sides int, depthScale, taper, taperTurns float64) *geom.Mesh {
	if sides < 3 {
		sides = 3
	}
	if pitch <= 0 || size.X <= 0 || size.Z <= 0 {
		return &geom.Mesh{}
	}
	if depthScale <= 0 {
		depthScale = 1
	}
	rMaj := size.X / 2
	sy := size.Y / size.X
	depth := 5.0 / 8 * math.Sqrt(3) / 2 * pitch * depthScale
	rMin := math.Max(rMaj*0.05, rMaj-depth)
	// radius at phase u in [0, pitch): crest, falling flank, root, rising flank
	profile := func(u float64) float64 {
		crest, flank, root := pitch/8, 5*pitch/16, pitch/4
		switch {
		case u < crest:
			return rMaj
		case u < crest+flank:
			return rMaj - (rMaj-rMin)*(u-crest)/flank
		case u < crest+flank+root:
			return rMin
		default:
			return rMin + (rMaj-rMin)*(u-crest-flank-root)/flank
		}
	}
	z0, z1 := -size.Z/2, size.Z/2
	phase0 := z0 + 3*pitch/8
	taper = math.Max(0, math.Min(taper, 1))
	tl := math.Min(math.Max(taperTurns, 0)*pitch, size.Z)
	// the tip's envelope: full radius below the taper, then narrowing
	envelope := func(z float64) float64 {
		if taper == 0 || tl <= 0 || z <= z1-tl {
			return math.Inf(1)
		}
		return math.Max(rMaj*(1-taper*(z-(z1-tl))/tl), rMaj*0.02)
	}
	// 24 rows a pitch, within a budget of triangles: a long, fine thread
	// gets fewer rows a pitch (no fewer than 8, which still shows the
	// profile), then fewer sides (never more than asked for), and as a
	// last resort fewer rows still
	const budget = 500000
	pitches := (z1 - z0) / pitch
	perPitch := 24.0
	if float64(2*sides)*pitches*perPitch > budget {
		perPitch = math.Max(8, budget/(float64(2*sides)*pitches))
	}
	if float64(2*sides)*pitches*perPitch > budget {
		sides = min(sides, max(8, int(budget/(2*perPitch*pitches))))
	}
	rows := max(1, min(int(math.Ceil(pitches*perPitch)), budget/(2*sides)))
	pt := func(k, i int) geom.Vec3 {
		th := 2 * math.Pi * float64(k) / float64(sides)
		z := z0 + (z1-z0)*float64(i)/float64(rows)
		u := math.Mod(z-phase0-pitch*th/(2*math.Pi), pitch)
		if u < 0 {
			u += pitch
		}
		r := math.Min(profile(u), envelope(z))
		return geom.V(r*math.Cos(th), r*math.Sin(th)*sy, z)
	}
	m := &geom.Mesh{Tris: make([]geom.Tri, 0, sides*(2*rows+2))}
	add := func(a, b, c geom.Vec3) { m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{a, b, c}}) }
	for k := 0; k < sides; k++ {
		k2 := (k + 1) % sides
		for i := 0; i < rows; i++ {
			a, b := pt(k, i), pt(k2, i)
			c, d := pt(k2, i+1), pt(k, i+1)
			add(a, b, c)
			add(a, c, d)
		}
		// flat end caps
		add(geom.V(0, 0, z0), pt(k2, 0), pt(k, 0))
		add(geom.V(0, 0, z1), pt(k, rows), pt(k2, rows))
	}
	return m.Clean().Oriented()
}

// ThreadLength is the rod length for a whole number of turns.
func ThreadLength(pitch float64, turns int) float64 { return (float64(turns) + 0.75) * pitch }

func init() {
	register(&Kind{ID: "thread", Label: "ISO Thread", Color: "#9aa3ab",
		Size: geom.V(12, 12, ThreadLength(1.75, 10)), // M12 × 1.75, 10 turns
		Params: []Param{
			mslider("pitch", "Pitch", 0.25, 6, 0.05, 1.75, false),
			slider("sides", "Sides", 8, 96, 1, 48, true),
			slider("depth", "Thread Depth", 0.1, 1.5, 0.05, 1, false),
			slider("taper", "Tip Taper", 0, 1, 0.05, 0, false),
			slider("taperturns", "Taper Length (turns)", 0.5, 10, 0.5, 2, false),
		},
		build: func(s geom.Vec3, v Values) *geom.Mesh {
			return ThreadMeshTapered(s, v.Get("pitch", 1.75), int(v.Get("sides", 48)), v.Get("depth", 1),
				v.Get("taper", 0), v.Get("taperturns", 2))
		}})
}
