package shapes

import (
	"fmt"
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// A baseplate squashed hard (ten units, 420 mm, made 5.36 mm across)
// flattens one sliver triangle to nothing; Build drops it and must close
// the gap it leaves.
func TestGridfinitySquashedBaseplateClosed(t *testing.T) {
	for _, bottom := range []float64{0, 1, 2} {
		v := Values{Num: map[string]float64{"unitsx": 3, "unitsy": 10, "bottom": bottom, "floor": 3}}
		m := Build("gridfinityplate", geom.V(239.47, 5.36, 53.88), v)
		if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
			t.Errorf("bottom %v: %d open, %d non-manifold edges", bottom, o, n)
		}
	}
}

func TestGridfinitySizes(t *testing.T) {
	for _, c := range []struct {
		kind string
		num  map[string]float64
		want geom.Vec3
	}{
		{"gridfinitybin", nil, geom.V(83.5, 41.5, 21+gfLip-gfLipCut)},
		{"gridfinitybin", map[string]float64{"lip": 0}, geom.V(83.5, 41.5, 21)},
		{"gridfinitybin", map[string]float64{"unitsx": 1, "unitsy": 3, "unitsz": 6}, geom.V(41.5, 125.5, 42+gfLip-gfLipCut)},
		{"gridfinityplate", nil, geom.V(126, 84, gfPlateH-gfRidge)},
		{"gridfinityplate", map[string]float64{"bottom": 1, "floor": 2}, geom.V(126, 84, 2+gfPlateH-gfRidge)},
		{"gridfinityplate", map[string]float64{"bottom": 2, "floor": 1}, geom.V(126, 84, 3+gfPlateH-gfRidge)}, // room for the magnets
	} {
		_, s := part(t, c.kind, c.num)
		if s.Sub(c.want).Len() > 1e-9 {
			t.Errorf("%s %v: size %v, want %v", c.kind, c.num, s, c.want)
		}
	}
}

// Every combination of settings makes a closed solid of the right size
// (part checks both).
func TestGridfinityClosed(t *testing.T) {
	for _, c := range []map[string]float64{
		{"unitsx": 1, "unitsy": 1, "unitsz": 1},
		{"unitsx": 1, "unitsy": 1, "unitsz": 1, "lip": 0},
		{"unitsx": 1, "unitsy": 1, "unitsz": 2, "holes": 3},
		{"unitsz": 2, "compx": 3, "compy": 2, "scoop": 8, "label": 12},
		{"unitsz": 6, "compx": 4, "compy": 3, "scoop": 30, "label": 30, "holes": 1},
		{"unitsx": 3, "unitsy": 2, "lip": 0, "compx": 2, "label": 10, "scoop": 5, "holes": 2},
		{"wall": 0.4, "compy": 10, "compx": 10, "label": 5, "scoop": 2},
		{"wall": 3, "unitsz": 4, "holes": 3},
	} {
		part(t, "gridfinitybin", c)
	}
	for _, c := range []map[string]float64{
		{"unitsx": 1, "unitsy": 1}, {"bottom": 1}, {"bottom": 2}, {"unitsx": 5, "unitsy": 4, "bottom": 2, "floor": 6},
	} {
		part(t, "gridfinityplate", c)
	}
}

// Dividers, scoops and label shelves stay inside the bin: none comes out
// through a rounded corner. And the scoop is along the front at the floor,
// the shelf along the back at the top.
func TestGridfinityInsidesStayInside(t *testing.T) {
	for _, c := range []map[string]float64{
		{"scoop": 8}, {"label": 12}, {"compx": 2, "compy": 2},
		{"wall": 0.4, "scoop": 8, "label": 12}, {"wall": 3, "scoop": 8, "label": 12},
		{"unitsx": 1, "compx": 10, "wall": 0.4}, {"lip": 0, "scoop": 30, "label": 30, "compy": 2},
	} {
		m, s := part(t, "gridfinitybin", c)
		ctr := m.Bounds().Center()
		for _, tr := range m.Tris {
			for _, p := range tr.P {
				dx, dy := math.Abs(p.X-ctr.X)-(s.X/2-gfBinR), math.Abs(p.Y-ctr.Y)-(s.Y/2-gfBinR)
				if dx > 0 && dy > 0 && math.Hypot(dx, dy) > gfBinR+1e-6 {
					t.Fatalf("%v: a corner at %v is %.3f mm outside the bin's rounded corner", c, p, math.Hypot(dx, dy)-gfBinR)
				}
			}
		}
	}
	plain, _ := part(t, "gridfinitybin", nil)
	half := func(m *geom.Mesh, y0, y1, z0, z1 float64) float64 {
		c := m.Bounds().Center()
		box := cuboid(-50, y0, z0, 50, y1, z1).Transform(geom.Translate(geom.V(c.X, c.Y, m.Bounds().Min.Z)))
		return geom.Intersect(m, box).Volume()
	}
	for _, c := range []struct {
		key            string
		y0, y1, z0, z1 float64 // where it adds material
	}{{"scoop", -21, 0, 7, 14}, {"label", 0, 21, 12, 21}} {
		m, _ := part(t, "gridfinitybin", map[string]float64{c.key: 6})
		if d := half(m, c.y0, c.y1, c.z0, c.z1) - half(plain, c.y0, c.y1, c.z0, c.z1); d < 100 {
			t.Errorf("%s adds %.1f mm³ where it should be", c.key, d)
		}
		if d := m.Volume() - plain.Volume() - (half(m, c.y0, c.y1, c.z0, c.z1) - half(plain, c.y0, c.y1, c.z0, c.z1)); math.Abs(d) > 1 {
			t.Errorf("%s adds %.1f mm³ elsewhere", c.key, d)
		}
	}
}

func TestGridfinityHoles(t *testing.T) {
	for _, c := range []struct {
		holes   float64
		d, h    float64
		perCell int
	}{{1, gfMagD, gfMagH, 4}, {2, gfScrewD, gfScrewH, 4}} {
		plain, _ := part(t, "gridfinitybin", map[string]float64{"unitsx": 2, "unitsy": 2})
		holed, _ := part(t, "gridfinitybin", map[string]float64{"unitsx": 2, "unitsy": 2, "holes": c.holes})
		want := float64(4*c.perCell) * ngonArea(c.d/2, 32) * c.h
		if got := plain.Volume() - holed.Volume(); !approx(got, want, 0.01*want) {
			t.Errorf("holes %g remove %.1f mm³, want %.1f", c.holes, got, want)
		}
	}
	plain := gfBaseplateModel(vals("gridfinityplate", map[string]float64{"bottom": 1}))
	holed := gfBaseplateModel(vals("gridfinityplate", map[string]float64{"bottom": 2}))
	want := 6 * 4 * ngonArea(gfMagD/2, 32) * gfMagH
	if got := plain.Volume() - holed.Volume(); !approx(got, want, 0.01*want) {
		t.Errorf("baseplate magnet holes remove %.1f mm³, want %.1f", got, want)
	}
}

// A bin drops into a baseplate's pockets, and stacks on another bin, without
// running into either: seated (a hair above where its chamfers meet), it
// shares no volume with what is under it.
func TestGridfinityFit(t *testing.T) {
	overlap := func(name string, a, b *geom.Mesh) {
		t.Helper()
		if v := geom.Intersect(a, b).Volume(); v > 1e-3 {
			t.Errorf("%s: overlaps by %.4f mm³", name, v)
		}
	}
	const lift = 0.05
	for _, size := range [][2]float64{{1, 1}, {2, 1}, {3, 2}} {
		bin := gfBinModel(vals("gridfinitybin", map[string]float64{"unitsx": size[0], "unitsy": size[1], "unitsz": 3, "label": 10, "compx": 2}))
		for _, bottom := range []float64{0, 1, 2} {
			plate := gfBaseplateModel(vals("gridfinityplate", map[string]float64{"unitsx": size[0], "unitsy": size[1], "bottom": bottom}))
			floor := plate.Bounds().Max.Z - (gfPlateH - gfRidge)
			overlap(fmt.Sprintf("%v bin in a plate with bottom %g", size, bottom), plate, bin.Transform(geom.Translate(geom.V(0, 0, floor+lift))))
		}
		// stacked: the upper bin's feet seat on the lower one's lip, their
		// top (4.75 mm up) level with where the lip would come to its edge
		upper := bin.Transform(geom.Translate(geom.V(0, 0, 21+gfLip-gfFoot+lift)))
		overlap(fmt.Sprintf("%v bins stacked", size), bin, upper)
		// and a bin seated any lower than that would run into it
		if v := geom.Intersect(bin, bin.Transform(geom.Translate(geom.V(0, 0, 21+gfLip-gfFoot-0.3)))).Volume(); v < 1e-2 {
			t.Errorf("%v: a bin 0.3 mm lower than seated doesn't touch the lip (%.4f mm³)", size, v)
		}
	}
}
