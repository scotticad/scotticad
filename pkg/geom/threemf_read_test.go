package geom

import (
	"archive/zip"
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// pack3MF zips parts (name to contents) into a package.
func pack3MF(t testing.TB, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const tmfRels = `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
 <Relationship Target="/3D/3dmodel.model" Id="rel0" Type="http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel"/>
</Relationships>`

// tmfCube is a unit cube object (from 0 to 1 on each axis) with id.
func tmfCube(id string, extra string) string {
	return `<object id="` + id + `" type="model"` + extra + `><mesh><vertices>
 <vertex x="0" y="0" z="0"/><vertex x="1" y="0" z="0"/><vertex x="1" y="1" z="0"/><vertex x="0" y="1" z="0"/>
 <vertex x="0" y="0" z="1"/><vertex x="1" y="0" z="1"/><vertex x="1" y="1" z="1"/><vertex x="0" y="1" z="1"/>
 </vertices><triangles>
 <triangle v1="0" v2="2" v3="1"/><triangle v1="0" v2="3" v3="2"/>
 <triangle v1="4" v2="5" v3="6"/><triangle v1="4" v2="6" v3="7"/>
 <triangle v1="0" v2="1" v3="5"/><triangle v1="0" v2="5" v3="4"/>
 <triangle v1="1" v2="2" v3="6"/><triangle v1="1" v2="6" v3="5"/>
 <triangle v1="2" v2="3" v3="7"/><triangle v1="2" v2="7" v3="6"/>
 <triangle v1="3" v2="0" v3="4"/><triangle v1="3" v2="4" v3="7"/>
 </triangles></mesh></object>`
}

func tmfModelXML(unit, resources, build string) string {
	u := ""
	if unit != "" {
		u = ` unit="` + unit + `"`
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<model` + u + ` xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02"
 xmlns:p="http://schemas.microsoft.com/3dmanufacturing/production/2015/06">
<resources>` + resources + `</resources><build>` + build + `</build></model>`
}

// What ScottiCAD writes it reads back: the same solid, in place, with its
// colours, whether the pieces are objects or parts of one.
func TestRead3MFRoundTrip(t *testing.T) {
	red, blue := Color{1, 0, 0}, Color{0, 0, 1}
	m := Merge(
		box(V(0, 0, 0), V(10, 10, 2)).Colored(red),
		box(V(20, 0, 0), V(10, 10, 2)).Colored(red),
		box(V(20, 0, 2), V(2, 2, 2)).Colored(blue))
	var buf bytes.Buffer
	if err := Write3MF(&buf, m, "d"); err != nil {
		t.Fatal(err)
	}
	got, err := Read3MF(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tris) != len(m.Tris) {
		t.Fatalf("%d triangles, want %d", len(got.Tris), len(m.Tris))
	}
	if b, w := got.Bounds(), m.Bounds(); b.Min.Sub(w.Min).Len() > 1e-5 || b.Max.Sub(w.Max).Len() > 1e-5 {
		t.Errorf("bounds %v, want %v", b, w)
	}
	if v, w := got.Volume(), m.Volume(); math.Abs(v-w) > 1e-3 {
		t.Errorf("volume %g, want %g", v, w)
	}
	n := map[string]int{}
	for _, tr := range got.Tris {
		n[tr.C.Hex()]++
	}
	if n["#ff0000"] != 24 || n["#0000ff"] != 12 {
		t.Errorf("colours %v, want 24 red and 12 blue", n)
	}
	if open, nm := got.EdgeCheck(0); open != 0 || nm != 0 {
		t.Errorf("%d open, %d non-manifold edges", open, nm)
	}
}

// A slicer's project keeps its meshes in other model files, referred to by
// path from components; items and components carry transforms, which
// compose, and a mirroring one keeps the faces outward.
func TestRead3MFProductionParts(t *testing.T) {
	object := tmfModelXML("", tmfCube("1", ""), "")
	root := tmfModelXML("",
		`<object id="5" type="model"><components>
		  <component p:path="/3D/Objects/object_1.model" objectid="1" transform="2 0 0 0 3 0 0 0 4 10 0 0"/>
		  <component p:path="/3D/Objects/object_1.model" objectid="1" transform="-1 0 0 0 1 0 0 0 1 0 0 0"/>
		 </components></object>`,
		`<item objectid="5" transform="1 0 0 0 1 0 0 0 1 0 0 5"/>`)
	data := pack3MF(t, map[string]string{
		"_rels/.rels": tmfRels, "3D/3dmodel.model": root, "3D/Objects/object_1.model": object,
	})
	m, err := Read3MF(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Tris) != 24 {
		t.Fatalf("%d triangles, want 24", len(m.Tris))
	}
	// a 2×3×4 box from x 10 and a mirrored unit cube from x -1, both lifted 5
	b := m.Bounds()
	if b.Min.Sub(V(-1, 0, 5)).Len() > 1e-9 || b.Max.Sub(V(12, 3, 9)).Len() > 1e-9 {
		t.Errorf("bounds %v", b)
	}
	if v := m.Volume(); math.Abs(v-25) > 1e-9 {
		t.Errorf("volume %g, want 24 + 1 (a mirrored part inside out would subtract)", v)
	}
}

// The model's unit scales it to millimetres; colours come from base
// materials (by object) and colour groups (by triangle); support objects
// are left out.
func TestRead3MFUnitsColoursAndSupports(t *testing.T) {
	res := `<basematerials id="2"><base name="a" displaycolor="#00FF00FF"/><base name="b" displaycolor="#FF0000"/></basematerials>
	 <m:colorgroup id="3" xmlns:m="http://schemas.microsoft.com/3dmanufacturing/material/2015/02"><m:color color="#0000FF"/></m:colorgroup>` +
		tmfCube("1", ` pid="2" pindex="1"`) +
		strings.Replace(tmfCube("4", ` pid="2" pindex="0"`), `<triangle v1="0" v2="2" v3="1"/>`, `<triangle v1="0" v2="2" v3="1" pid="3" p1="0"/>`, 1) +
		strings.Replace(tmfCube("6", ""), `type="model"`, `type="support"`, 1)
	build := `<item objectid="1"/><item objectid="4" transform="1 0 0 0 1 0 0 0 1 2 0 0"/><item objectid="6"/>`
	data := pack3MF(t, map[string]string{"_rels/.rels": tmfRels, "3D/3dmodel.model": tmfModelXML("centimeter", res, build)})
	m, err := Read3MF(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Tris) != 24 {
		t.Fatalf("%d triangles, want 24 (the support left out)", len(m.Tris))
	}
	if b := m.Bounds(); b.Max.Sub(V(30, 10, 10)).Len() > 1e-9 {
		t.Errorf("bounds %v: want 30 × 10 × 10 mm", b)
	}
	n := map[string]int{}
	for _, tr := range m.Tris {
		n[tr.C.Hex()]++
	}
	if n["#ff0000"] != 12 || n["#00ff00"] != 11 || n["#0000ff"] != 1 {
		t.Errorf("colours %v", n)
	}
}

// Without a relationships part the usual root is read.
func TestRead3MFWithoutRels(t *testing.T) {
	data := pack3MF(t, map[string]string{"3D/3dmodel.model": tmfModelXML("", tmfCube("1", ""), `<item objectid="1"/>`)})
	if m, err := Read3MF(data); err != nil || len(m.Tris) != 12 {
		t.Fatalf("err %v", err)
	}
}

// tmfDoubling is objects from to up: each is two of the one before.
func tmfDoubling(from, to int) string {
	var sb strings.Builder
	for id := from; id <= to+1; id++ {
		fmt.Fprintf(&sb, `<object id="%d"><components><component objectid="%d"/><component objectid="%d" transform="1 0 0 0 1 0 0 0 1 2 0 0"/></components></object>`, id, id-1, id-1)
	}
	return sb.String()
}

// Broken files are errors for that file, never a panic or a hang.
func TestRead3MFRefusesBrokenFiles(t *testing.T) {
	cube := tmfCube("1", "")
	for name, parts := range map[string]map[string]string{
		"not a zip":         nil,
		"no model":          {"_rels/.rels": tmfRels},
		"no build":          {"3D/3dmodel.model": tmfModelXML("", cube, "")},
		"missing object":    {"3D/3dmodel.model": tmfModelXML("", cube, `<item objectid="9"/>`)},
		"vertex range":      {"3D/3dmodel.model": tmfModelXML("", strings.Replace(cube, `v3="1"/>`, `v3="99"/>`, 1), `<item objectid="1"/>`)},
		"bad number":        {"3D/3dmodel.model": tmfModelXML("", strings.Replace(cube, `x="1" y="0" z="0"`, `x="nan" y="0" z="0"`, 1), `<item objectid="1"/>`)},
		"bad transform":     {"3D/3dmodel.model": tmfModelXML("", cube, `<item objectid="1" transform="1 0 0"/>`)},
		"bad unit":          {"3D/3dmodel.model": tmfModelXML("parsec", cube, `<item objectid="1"/>`)},
		"missing part file": {"3D/3dmodel.model": tmfModelXML("", `<object id="2"><components><component p:path="/3D/x.model" objectid="1"/></components></object>`, `<item objectid="2"/>`)},
		"component loop":    {"3D/3dmodel.model": tmfModelXML("", `<object id="2"><components><component objectid="2"/></components></object>`, `<item objectid="2"/>`)},
		"broken xml":        {"3D/3dmodel.model": "<model><resources><object id=\"1\">"},
		// each object two of the next, forty deep: a million million cubes
		"component bomb": {"3D/3dmodel.model": tmfModelXML("", cube+tmfDoubling(2, 40), `<item objectid="41"/>`)},
		// two of itself: a loop that branches
		"branching loop": {"3D/3dmodel.model": tmfModelXML("", `<object id="2"><components><component objectid="2"/><component objectid="2"/></components></object>`, `<item objectid="2"/>`)},
		"out of range":   {"3D/3dmodel.model": tmfModelXML("", cube+`<object id="2"><components><component objectid="1" transform="1e300 0 0 0 1 0 0 0 1 0 0 0"/></components></object>`, `<item objectid="2" transform="1e300 0 0 0 1 0 0 0 1 0 0 0"/>`)},
	} {
		data := []byte("this is not a zip")
		if parts != nil {
			data = pack3MF(t, parts)
		}
		if m, err := Read3MF(data); err == nil {
			t.Errorf("%s: read %d triangles, want an error", name, len(m.Tris))
		}
	}
}

func FuzzRead3MF(f *testing.F) {
	var buf bytes.Buffer
	Write3MF(&buf, Merge(box(V(0, 0, 0), V(4, 4, 4)).Colored(Color{1, 0, 0}), box(V(4, 0, 0), V(4, 4, 4)).Colored(Color{0, 0, 1})), "x")
	f.Add(buf.Bytes())
	f.Add(pack3MF(f, map[string]string{"3D/3dmodel.model": tmfModelXML("inch", tmfCube("1", ""), `<item objectid="1" transform="0 1 0 1 0 0 0 0 -1 0 0 0"/>`)}))
	f.Add([]byte("PK\x03\x04"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var m *Mesh
		var err error
		fzWithin(t, 5*time.Second, "Read3MF", func() { m, err = Read3MF(data) })
		if err != nil {
			return
		}
		fzCheckMesh(t, "Read3MF", m)
	})
}
