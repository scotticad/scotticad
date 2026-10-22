package shapes

import (
	"fmt"
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// sizeFor is the size a shape takes for values v: its natural size if it
// has one (parts built in millimetres), else its default size.
func sizeFor(k *Kind, v Values) geom.Vec3 {
	if k.natural != nil {
		return k.natural(v)
	}
	return k.Size
}

// Every shape, with each slider at its minimum and at its maximum (the
// others at their defaults), builds a watertight mesh that fills its box.
func TestEveryShapeAtItsSliderLimits(t *testing.T) {
	for _, k := range All() {
		for _, p := range k.Params {
			if p.Kind != Slider {
				continue
			}
			for _, val := range []float64{p.Min, p.Max} {
				v := k.Defaults()
				v.Num[p.Key] = val
				name := fmt.Sprintf("%s %s=%g", k.ID, p.Key, val)
				if k.parts != nil {
					parts, _ := k.Parts(v)
					for _, pt := range parts {
						checkMesh(t, name+" "+pt.Kind, Build(pt.Kind, PartSize(pt.Kind, v), v), PartSize(pt.Kind, v))
					}
					continue
				}
				size := sizeFor(k, v)
				checkMesh(t, name, Build(k.ID, size, v), size)
			}
		}
	}
}

// Combinations of sliders the single-slider sweep can't reach.
func TestShapeSliderCombinations(t *testing.T) {
	for _, c := range []struct {
		kind string
		num  map[string]float64
	}{
		{"projectbox", map[string]float64{"length": 10, "width": 40, "wall": 4}},
		{"projectbox", map[string]float64{"length": 10, "width": 10, "wall": 10}},
		{"projectbox", map[string]float64{"length": 300, "width": 10, "wall": 3, "radius": 30}},
		{"extrudedbox", map[string]float64{"width": 15, "height": 10, "wall": 6, "radius": 15}},
		{"gear", map[string]float64{"teeth": 6, "pitch": 0.5, "bore": 50}},
		{"gear", map[string]float64{"teeth": 6, "pitch": 10, "slop": 1, "bore": 50}},
		{"plate", map[string]float64{"length": 10, "width": 10, "radius": 30, "hole": 10, "inset": 1}},
		{"torus", map[string]float64{"radius": 1, "tube": 20}},
		{"spring", map[string]float64{"diameter": 2, "wire": 10}},
		{"hinge", map[string]float64{"length": 10, "knuckles": 15, "clearance": 1.5}},
		{"hyperboloid", map[string]float64{"outer": 1, "inner": 100}},
	} {
		k, _ := Lookup(c.kind)
		v := k.Defaults()
		for key, val := range c.num {
			v.Num[key] = val
		}
		name := fmt.Sprintf("%s %v", c.kind, c.num)
		if k.parts != nil {
			parts, _ := k.Parts(v)
			for _, pt := range parts {
				checkMesh(t, name+" "+pt.Kind, Build(pt.Kind, PartSize(pt.Kind, v), v), PartSize(pt.Kind, v))
			}
			continue
		}
		checkMesh(t, name, Build(c.kind, sizeFor(k, v), v), sizeFor(k, v))
	}
}

// Values from a file aren't checked by the sliders: Build keeps them in
// range (no empty or broken meshes from sides 0, NaN, or a huge value).
func TestBuildClampsValues(t *testing.T) {
	for _, c := range []struct {
		kind string
		num  map[string]float64
	}{
		{"cylinder", map[string]float64{"sides": 0}},
		{"sphere", map[string]float64{"steps": -4}},
		{"cone", map[string]float64{"sides": 2.6}},
		{"polygon", map[string]float64{"sides": math.NaN()}},
		{"torus", map[string]float64{"sides": 1, "steps": 1e9}},
	} {
		k, _ := Lookup(c.kind)
		checkMesh(t, fmt.Sprintf("%s %v", c.kind, c.num), Build(c.kind, k.Size, Values{Num: c.num}), k.Size)
	}
	// a torus whose tube is fatter than its ring stays a torus (the tube
	// is kept off the axis): it has a torus's volume, where a profile
	// crossing the axis folds over itself and loses volume
	s := geom.V(20, 20, 20)
	m := Build("torus", s, Values{Num: map[string]float64{"radius": 1, "tube": 20, "sides": 64, "steps": 64}})
	R, r := 1.0, 0.49 // Radius is the outside radius: the tube's centre is at R−r
	want := 2 * math.Pi * math.Pi * (R - r) * r * r * (s.X / (2 * R)) * (s.Y / (2 * R)) * (s.Z / (2 * r))
	if v := m.Volume(); math.Abs(v-want) > 0.02*want {
		t.Fatalf("torus volume %.1f, want about %.1f", v, want)
	}
}

func checkMesh(t *testing.T, name string, m *geom.Mesh, size geom.Vec3) {
	t.Helper()
	if len(m.Tris) == 0 {
		t.Errorf("%s: empty", name)
		return
	}
	if o, n := m.EdgeCheck(1e-7); o != 0 || n != 0 {
		t.Errorf("%s: %d open, %d non-manifold edges", name, o, n)
	}
	if s := m.Bounds().Size(); s.Sub(size).Len() > 1e-6*(1+size.Len()) {
		t.Errorf("%s: size %.4g×%.4g×%.4g, want %.4g×%.4g×%.4g", name, s.X, s.Y, s.Z, size.X, size.Y, size.Z)
	}
}
