package doc

import (
	"math"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

func nearV(a, b geom.Vec3, eps float64) bool { return a.Sub(b).Len() <= eps }

func TestTubeSavesWithVersion3(t *testing.T) {
	d := New()
	n, err := d.AddShape("benttube", geom.V(0, 0, 0), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Bends) == 0 {
		t.Fatal("a placed bent tube has no bend table")
	}
	b, _ := d.Marshal()
	if !strings.Contains(string(b), `"version": 3`) {
		t.Errorf("a design with a bent tube needs version 3")
	}
	d2, err := Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	n2 := d2.Find(n.ID)
	if n2 == nil || len(n2.Bends) != len(n.Bends) || !boxNear(n2.WorldBounds(), n.WorldBounds()) {
		t.Fatalf("round trip lost the tube: %+v", n2)
	}
	if v := n2.WorldMesh(false).Volume(); v <= 0 {
		t.Errorf("tube volume %v", v)
	}
	// without a tube, the version stays what the rest needs
	d3 := New()
	d3.AddShape("box", geom.V(0, 0, 0), false)
	b, _ = d3.Marshal()
	if !strings.Contains(string(b), `"version": 1`) {
		t.Errorf("a design without tubes or outlines should stay version 1")
	}
}

func TestSetBendsKeepsTheStart(t *testing.T) {
	d := New()
	n, _ := d.AddShape("benttube", geom.V(12, -7, 0), false)
	n.M = geom.Translate(geom.V(5, 3, 2)).Mul(geom.RotateZ(0.6)).Mul(n.M)
	s0, e0, ok := n.TubeEnds()
	if !ok {
		t.Fatal("no tube ends")
	}
	if err := SetBends(n, []shapes.Bend{{Length: 50, Angle: -45}, {Length: 10, Angle: 90, Turn: 90}, {Length: 30}}); err != nil {
		t.Fatal(err)
	}
	s1, e1, _ := n.TubeEnds()
	if !nearV(s0[0], s1[0], 1e-6) || !nearV(s0[1], s1[1], 1e-6) {
		t.Errorf("the start moved from %v to %v", s0, s1)
	}
	if nearV(e0[0], e1[0], 1e-3) {
		t.Error("the end should have moved")
	}
	// a thicker tube keeps its centreline too
	SetProfileParam(n, "diameter", 20)
	s2, e2, _ := n.TubeEnds()
	if !nearV(s1[0], s2[0], 1e-6) || !nearV(e1[0], e2[0], 1e-6) {
		t.Errorf("a new diameter moved the ends: %v %v -> %v %v", s1, e1, s2, e2)
	}
	// the end is where the table says: 50 along, a 45-degree bend, ...
	if l := e2[0].Sub(s2[0]).Len(); l < 60 || l > 110 || math.IsNaN(l) {
		t.Errorf("start to end %v mm", l)
	}
	if SetBends(n, []shapes.Bend{{Length: -1}}) == nil {
		t.Error("a bad table should be refused")
	}
	if box, _ := d.AddShape("box", geom.V(0, 0, 0), false); SetBends(box, []shapes.Bend{{Length: 10}}) == nil {
		t.Error("a box has no bend table")
	}
}
