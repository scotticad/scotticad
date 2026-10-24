package shapes

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

func TestStarGemAndGeneratorShapes(t *testing.T) {
	for _, c := range []struct {
		id   string
		size geom.Vec3
	}{
		{"star", geom.V(38.04, 36.18, 10)},
		{"star3d", geom.V(38.04, 36.18, 10)},
		{"dice", geom.V(16, 16, 16)},
		{"diamond", geom.V(20, 20, 10)},
		{"hyperboloid", geom.V(20, 20, 20)},
		{"sphereshell", geom.V(21, 21, 10.5)},
		{"gear", geom.V(27.44, 27.44, 20)},
	} {
		k, ok := Lookup(c.id)
		if !ok {
			t.Fatalf("%s missing", c.id)
		}
		if k.Size.Sub(c.size).Len() > 0.01 {
			t.Errorf("%s default size %v, want %v", c.id, k.Size, c.size)
		}
		m := Build(c.id, k.Size, k.Defaults())
		if s := m.Bounds().Size(); s.Sub(k.Size).Len() > 1e-6 {
			t.Errorf("%s builds %v", c.id, s)
		}
		if open, nm := m.EdgeCheck(1e-7); open != 0 || nm != 0 {
			t.Errorf("%s not watertight: %d open, %d non-manifold", c.id, open, nm)
		}
		if m.Volume() <= 0 {
			t.Errorf("%s inside out", c.id)
		}
	}
	// a 5-point star of radius 20 is 38.04 x 36.18
	k, _ := Lookup("star3d")
	if s, _ := k.NaturalSize(k.Defaults()); math.Abs(s.X-38.04) > 0.01 || math.Abs(s.Y-36.18) > 0.01 {
		t.Errorf("star3d natural size %v", s)
	}
	// the hyperboloid's waist is the inner radius: a 20 x 20 slice at
	// mid-height is a 20-gon of radius 5
	h := Build("hyperboloid", geom.V(20, 20, 20), Values{Num: map[string]float64{"outer": 10, "inner": 5, "sides": 20}})
	var waist float64
	for _, tr := range h.Tris {
		for _, p := range tr.P {
			if math.Abs(p.Z) < 1e-9 {
				waist = math.Max(waist, math.Hypot(p.X, p.Y))
			}
		}
	}
	if math.Abs(waist-5) > 1e-6 {
		t.Errorf("hyperboloid waist radius %v, want 5", waist)
	}
	// the shell is hollow: a hemisphere's wall, 1 mm thick at radius 10
	sv := Build("sphereshell", geom.V(21, 21, 10.5), Values{Num: map[string]float64{"angle": 90, "steps": 64, "radius": 10, "thickness": 1}}).Volume()
	if want := 2.0 / 3 * math.Pi * (10.5*10.5*10.5 - 9.5*9.5*9.5); math.Abs(sv-want) > 0.02*want {
		t.Errorf("shell volume %v, want about %v", sv, want)
	}
	// the gear: the default is 27.41 mm across (within 0.05 mm), with a
	// 6 mm bore through it
	k, _ = Lookup("gear")
	gs, _ := k.NaturalSize(k.Defaults())
	if math.Abs(gs.X-27.41) > 0.05 {
		t.Errorf("gear width %v, want about 27.41", gs.X)
	}
	gm := Build("gear", gs, k.Defaults())
	solid := Build("gear", gs, Values{Num: map[string]float64{"pitch": 2, "slop": 0.01, "teeth": 20, "bore": 0}})
	if dv := solid.Volume() - gm.Volume(); math.Abs(dv-math.Pi*9*20) > 0.03*math.Pi*9*20 {
		t.Errorf("bore volume %v, want about %v", dv, math.Pi*9*20)
	}
	// the die is rounded: less than the cube, more than its inscribed ball
	v := Build("dice", geom.V(16, 16, 16), Values{}).Volume()
	if v >= 16*16*16 || v <= 4.0/3*math.Pi*8*8*8 {
		t.Errorf("dice volume %v", v)
	}
}
