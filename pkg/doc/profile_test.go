package doc

import (
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

func loftSections(dx float64) []shapes.Section {
	return []shapes.Section{
		{Z: 3, Contours: [][][2]float64{{{dx - 10, -6}, {dx + 10, -6}, {dx + 10, 6}, {dx - 10, 6}}}},
		{Z: 9, Contours: [][][2]float64{{{dx - 4, -2}, {dx + 4, -2}, {dx, 5}}}},
	}
}

func boxNear(a, b geom.Box) bool {
	return a.Min.Sub(b.Min).Len() < 1e-9 && a.Max.Sub(b.Max).Len() < 1e-9
}

func TestAddProfileLandsWhereItsPointsSay(t *testing.T) {
	d := New("t")
	n, err := d.AddProfile("loft", loftSections(30), false)
	if err != nil {
		t.Fatal(err)
	}
	want := geom.Box{Min: geom.V(20, -6, 3), Max: geom.V(40, 6, 9)}
	if got := n.WorldBounds(); !boxNear(got, want) {
		t.Fatalf("bounds %v want %v", got, want)
	}
	if _, err := d.AddProfile("loft", loftSections(0)[:1], false); err == nil || !strings.Contains(err.Error(), "at least 2") {
		t.Errorf("one section: %v", err)
	}
	if _, err := d.AddProfile("box", loftSections(0), false); err == nil {
		t.Error("box from sections should be refused")
	}
}

func TestSetSectionsKeepsTheShapesFrame(t *testing.T) {
	d := New("t")
	n, _ := d.AddProfile("loft", loftSections(0), false)
	// unmoved: new points land where they say
	if err := SetSections(n, loftSections(50)); err != nil {
		t.Fatal(err)
	}
	if got, want := n.WorldBounds(), (geom.Box{Min: geom.V(40, -6, 3), Max: geom.V(60, 6, 9)}); !boxNear(got, want) {
		t.Fatalf("unmoved: bounds %v want %v", got, want)
	}
	// moved and doubled in x: the same points shift by the move and stretch
	// by the scale
	n.M = geom.Translate(geom.V(0, 100, 0)).Mul(n.M)
	n.Size.X *= 2
	if err := SetSections(n, loftSections(60)); err != nil {
		t.Fatal(err)
	}
	b := n.WorldBounds()
	if !(abs(b.Size().X-40) < 1e-9 && abs(b.Min.Y-94) < 1e-9 && abs(b.Center().X-(50+2*10)) < 1e-9) {
		t.Errorf("moved and stretched: bounds %v", b)
	}
	if err := SetSections(n, loftSections(0)[:1]); err == nil {
		t.Error("bad sections should be refused")
	}
}

func TestSmoothingKeepsThePointsInPlace(t *testing.T) {
	// a lopsided outline: smoothing bulges it more on one side, which
	// mustn't move its points
	pts := [][2]float64{{0, 0}, {30, 0}, {30, 4}, {2, 20}}
	d := New("t")
	n, err := d.AddProfile("outline", []shapes.Section{{Z: 0, Contours: [][][2]float64{pts}}, {Z: 5, Contours: [][][2]float64{pts}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	SetProfileParam(n, "smooth", 1)
	// the corner (30, 0) is a point the curve passes through: still there
	m := n.WorldMesh(false)
	found := false
	for _, tr := range m.Tris {
		for _, p := range tr.P {
			if p.Sub(geom.V(30, 0, 0)).Len() < 1e-6 {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("after smoothing, the point (30, 0, 0) moved; bounds %v", n.WorldBounds())
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func TestProfileFilesNeedVersion2(t *testing.T) {
	d := New("t")
	d.AddShape("box", geom.Vec3{}, false)
	b, _ := d.Marshal()
	if !strings.Contains(string(b), `"version": 1`) {
		t.Errorf("a design without lofts should stay version 1:\n%s", b[:60])
	}
	n, _ := d.AddProfile("outline", []shapes.Section{
		{Z: 0, Contours: [][][2]float64{{{0, 0}, {10, 0}, {0, 10}}}},
		{Z: 4, Contours: [][][2]float64{{{0, 0}, {10, 0}, {0, 10}}}},
	}, false)
	b, _ = d.Marshal()
	if !strings.Contains(string(b), `"version": 2`) {
		t.Errorf("a design with an outline needs version 2")
	}
	d2, err := Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	n2 := d2.Find(n.ID)
	if n2 == nil || len(n2.Sections) != 2 || !boxNear(n2.WorldBounds(), n.WorldBounds()) {
		t.Fatalf("round trip lost the outline: %+v", n2)
	}
	if v := n2.WorldMesh(false).Volume(); abs(v-200) > 1e-6 {
		t.Errorf("outline volume %v want 200", v)
	}
	// a newer format is refused, not half read
	if _, err := Unmarshal([]byte(`{"version": 3, "nodes": []}`)); err == nil {
		t.Error("version 3 should be refused")
	}
}

func TestProfileCacheFollowsItsPoints(t *testing.T) {
	d := New("t")
	n, _ := d.AddProfile("loft", loftSections(0), false)
	v1 := n.WorldMesh(false).Volume()
	c := n.Clone()
	if err := SetSections(c, []shapes.Section{
		{Z: 3, Contours: [][][2]float64{{{-10, -6}, {10, -6}, {10, 6}, {-10, 6}}}},
		{Z: 9, Contours: [][][2]float64{{{-10, -6}, {10, -6}, {10, 6}, {-10, 6}}}},
	}); err != nil {
		t.Fatal(err)
	}
	if v2 := c.WorldMesh(false).Volume(); abs(v2-20*12*6) > 1e-6 {
		t.Errorf("edited copy volume %v want %v", v2, 20*12*6)
	}
	if v := n.WorldMesh(false).Volume(); v != v1 {
		t.Errorf("editing a copy changed the original: %v -> %v", v1, v)
	}
}
