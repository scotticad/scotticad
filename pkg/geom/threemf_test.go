package geom

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"math"
	"strings"
	"testing"
)

// model3MF and read3MF read back what Write3MF writes, independently of
// it: the package's parts, and the model's mesh and colours.
type model3MF struct {
	XMLName  xml.Name `xml:"model"`
	Unit     string   `xml:"unit,attr"`
	Metadata []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:",chardata"`
	} `xml:"metadata"`
	Bases []struct {
		Color string `xml:"displaycolor,attr"`
	} `xml:"resources>basematerials>base"`
	Objects []struct {
		ID       int    `xml:"id,attr"`
		Type     string `xml:"type,attr"`
		Vertices []struct {
			X float64 `xml:"x,attr"`
			Y float64 `xml:"y,attr"`
			Z float64 `xml:"z,attr"`
		} `xml:"mesh>vertices>vertex"`
		Triangles []struct {
			V1 int `xml:"v1,attr"`
			V2 int `xml:"v2,attr"`
			V3 int `xml:"v3,attr"`
			P1 int `xml:"p1,attr"`
		} `xml:"mesh>triangles>triangle"`
	} `xml:"resources>object"`
	Items []struct {
		ObjectID int `xml:"objectid,attr"`
	} `xml:"build>item"`
}

func read3MF(t *testing.T, data []byte) (model3MF, map[string]string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = string(b)
	}
	var m model3MF
	if err := xml.Unmarshal([]byte(parts["3D/3dmodel.model"]), &m); err != nil {
		t.Fatalf("model part: %v", err)
	}
	return m, parts
}

func TestWrite3MF(t *testing.T) {
	red, blue := Color{1, 0, 0}, Color{0, 0.5, 1}
	m := Merge(box(V(0, 0, 0), V(20, 10, 5)).Colored(red), box(V(40, 0, 0), V(4, 4, 4)).Colored(blue))
	// a triangle of no area: left out
	m.Tris = append(m.Tris, Tri{P: [3]Vec3{V(0, 0, 0), V(1, 0, 0), V(2, 0, 0)}, C: red})
	var buf bytes.Buffer
	if err := Write3MF(&buf, m, `a <"design"> & co`); err != nil {
		t.Fatal(err)
	}
	got, parts := read3MF(t, buf.Bytes())

	// the package: content types for both kinds of part, and the
	// relationship pointing at the model
	if ct := parts["[Content_Types].xml"]; !strings.Contains(ct, `Extension="model"`) || !strings.Contains(ct, `Extension="rels"`) {
		t.Errorf("content types: %s", ct)
	}
	if r := parts["_rels/.rels"]; !strings.Contains(r, `Target="/3D/3dmodel.model"`) || !strings.Contains(r, "3dmanufacturing/2013/01/3dmodel") {
		t.Errorf("relationships: %s", r)
	}
	if got.Unit != "millimeter" {
		t.Errorf("unit %q", got.Unit)
	}
	title := ""
	for _, md := range got.Metadata {
		if md.Name == "Title" {
			title = md.Value
		}
	}
	if title != `a <"design"> & co` {
		t.Errorf("title %q: the name wasn't escaped and read back", title)
	}
	if len(got.Objects) != 1 || got.Objects[0].Type != "model" {
		t.Fatalf("%d objects", len(got.Objects))
	}
	if len(got.Items) != 1 || got.Items[0].ObjectID != got.Objects[0].ID {
		t.Fatalf("build items %v don't place the object", got.Items)
	}
	obj := got.Objects[0]
	if len(obj.Vertices) != 16 {
		t.Errorf("%d vertices, want 16 (two boxes' corners, each once)", len(obj.Vertices))
	}
	if len(obj.Triangles) != 24 {
		t.Errorf("%d triangles, want 24 (the one of no area left out)", len(obj.Triangles))
	}
	// colours: one base each, triangles pointing at the right one
	if len(got.Bases) != 2 || got.Bases[0].Color != "#FF0000" || got.Bases[1].Color != "#0080FF" {
		t.Errorf("base materials %v", got.Bases)
	}
	// the mesh rebuilt: closed, same volume, same colours
	back := &Mesh{}
	for _, tr := range obj.Triangles {
		for _, v := range []int{tr.V1, tr.V2, tr.V3} {
			if v < 0 || v >= len(obj.Vertices) {
				t.Fatalf("vertex index %d out of range", v)
			}
		}
		if tr.V1 == tr.V2 || tr.V2 == tr.V3 || tr.V3 == tr.V1 {
			t.Fatalf("a triangle with a repeated vertex: %v", tr)
		}
		p := func(i int) Vec3 { v := obj.Vertices[i]; return V(v.X, v.Y, v.Z) }
		c := red
		if tr.P1 == 1 {
			c = blue
		}
		back.Tris = append(back.Tris, Tri{P: [3]Vec3{p(tr.V1), p(tr.V2), p(tr.V3)}, C: c})
	}
	if o, nm := back.EdgeCheck(0); o != 0 || nm != 0 {
		t.Errorf("read back: %d open, %d non-manifold edges", o, nm)
	}
	if v, want := back.Volume(), 20*10*5+4*4*4.0; math.Abs(v-want) > 1e-3 {
		t.Errorf("read back: volume %v, want %v", v, want)
	}
	nBlue := 0
	for _, tr := range back.Tris {
		if tr.C == blue {
			nBlue++
		}
	}
	if nBlue != 12 {
		t.Errorf("%d blue triangles, want the small box's 12", nBlue)
	}

	// the same mesh gives the same bytes
	var again bytes.Buffer
	Write3MF(&again, m, `a <"design"> & co`)
	if !bytes.Equal(buf.Bytes(), again.Bytes()) {
		t.Error("two exports of the same mesh differ")
	}
}

func TestWrite3MFEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := Write3MF(&buf, &Mesh{}, "empty"); err == nil || buf.Len() != 0 {
		t.Errorf("an empty mesh: err %v, %d bytes written", err, buf.Len())
	}
	degenerate := &Mesh{Tris: []Tri{{P: [3]Vec3{V(0, 0, 0), V(1, 1, 1), V(2, 2, 2)}}}}
	if err := Write3MF(&buf, degenerate, "flat"); err == nil {
		t.Error("a mesh of only triangles of no area exported")
	}
}

// Coordinates keep float32 precision (as STL), including large and small
// ones, which come out in exponent form the 3MF number type allows.
func TestWrite3MFNumbers(t *testing.T) {
	m := box(V(1e6, -0.000123, 3.25), V(2, 2, 2))
	var buf bytes.Buffer
	if err := Write3MF(&buf, m, "n"); err != nil {
		t.Fatal(err)
	}
	got, _ := read3MF(t, buf.Bytes())
	for i, v := range got.Objects[0].Vertices {
		found := false
		for _, tr := range m.Tris {
			for _, q := range tr.P {
				// the shortest decimal for a float32 reads back as that
				// float32
				if float32(q.X) == float32(v.X) && float32(q.Y) == float32(v.Y) && float32(q.Z) == float32(v.Z) {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("vertex %d %v isn't one of the mesh's, at float32 precision", i, v)
		}
	}
}
