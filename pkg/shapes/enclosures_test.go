package shapes

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

func TestEnclosureAssemblies(t *testing.T) {
	for _, c := range []struct {
		id    string
		kinds []string
	}{
		{"projectbox", []string{"projectbox-body", "projectbox-lid"}},
		{"extrudedbox", []string{"extrudedbox-body", "extrudedbox-panel", "extrudedbox-panel"}},
	} {
		k, _ := Lookup(c.id)
		parts, ok := k.Parts(k.Defaults())
		if !ok || len(parts) != len(c.kinds) {
			t.Fatalf("%s parts %v", c.id, parts)
		}
		for i, p := range parts {
			if p.Kind != c.kinds[i] {
				t.Errorf("%s part %d is %s", c.id, i, p.Kind)
			}
			pk, _ := Lookup(p.Kind)
			if !pk.Hidden {
				t.Errorf("%s should be hidden from the libraries", p.Kind)
			}
		}
	}
	// project box: the lid closes the box exactly, its lip inside the walls
	k, _ := Lookup("projectbox")
	v := k.Defaults()
	parts, _ := k.Parts(v)
	body := Build("projectbox-body", PartSize("projectbox-body", v), v).Transform(parts[0].M).Bounds()
	lid := Build("projectbox-lid", PartSize("projectbox-lid", v), v).Transform(parts[1].M).Bounds()
	if math.Abs(body.Min.Z) > 1e-9 || math.Abs(lid.Max.Z-(body.Max.Z+2)) > 1e-9 || math.Abs(lid.Min.Z-(body.Max.Z-3)) > 1e-9 {
		t.Fatalf("box z %v..%v, lid z %v..%v", body.Min.Z, body.Max.Z, lid.Min.Z, lid.Max.Z)
	}
	// no lid: one part
	if p, _ := k.Parts(Values{Num: map[string]float64{"lid": 0}}); len(p) != 1 {
		t.Fatalf("lid off gave %d parts", len(p))
	}
	// extruded box: panels close both ends of the tube, outside it
	k, _ = Lookup("extrudedbox")
	v = k.Defaults()
	parts, _ = k.Parts(v)
	tube := Build("extrudedbox-body", PartSize("extrudedbox-body", v), v).Transform(parts[0].M).Bounds()
	front := Build("extrudedbox-panel", PartSize("extrudedbox-panel", v), v).Transform(parts[1].M).Bounds()
	if s := tube.Size(); math.Abs(s.X-80) > 1e-6 || math.Abs(s.Y-54) > 1e-6 || math.Abs(s.Z-30) > 1e-6 {
		t.Fatalf("tube size %v", s)
	}
	if math.Abs(front.Min.X-tube.Max.X) > 1e-6 || math.Abs(front.Size().X-2.5) > 1e-6 || front.Size().Sub(geom.V(2.5, 54, 30)).Len() > 1e-6 {
		t.Fatalf("front panel %v..%v", front.Min, front.Max)
	}
	// the tube is hollow: much less than its solid volume
	m := Build("extrudedbox-body", PartSize("extrudedbox-body", v), v)
	if vol := m.Volume(); vol <= 0 || vol > 0.35*80*54*30 {
		t.Fatalf("tube volume %v", vol)
	}
}
