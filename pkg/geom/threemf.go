package geom

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

// Write3MF writes m as a 3MF package (3MF core specification 1.3): a zip
// holding [Content_Types].xml, _rels/.rels and 3D/3dmodel.model. The model
// is one object in millimetres, its vertices shared between triangles (3MF
// meshes are indexed), each triangle coloured from a base-materials table
// with one entry per colour, so slicers that read colour (PrusaSlicer,
// Bambu Studio, Cura) see it. Triangles of no area are left out, as the
// other writers do; 3MF requires every triangle's three vertices to be
// distinct.
func Write3MF(w io.Writer, m *Mesh, name string) error {
	m = m.Clean()
	if len(m.Tris) == 0 {
		return errors.New("nothing to export: the mesh is empty")
	}
	z := zip.NewWriter(w)
	// a fixed date: the same design exports the same bytes
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	part := func(path string) (io.Writer, error) {
		return z.CreateHeader(&zip.FileHeader{Name: path, Method: zip.Deflate, Modified: stamp})
	}
	for _, f := range []struct{ path, body string }{
		{"[Content_Types].xml", threeMFContentTypes},
		{"_rels/.rels", threeMFRels},
	} {
		pw, err := part(f.path)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(pw, f.body); err != nil {
			return err
		}
	}
	pw, err := part("3D/3dmodel.model")
	if err != nil {
		return err
	}
	if err := write3MFModel(pw, m, name); err != nil {
		return err
	}
	return z.Close()
}

const threeMFContentTypes = `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
 <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
 <Default Extension="model" ContentType="application/vnd.ms-package.3dmanufacturing-3dmodel+xml"/>
</Types>
`

const threeMFRels = `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
 <Relationship Target="/3D/3dmodel.model" Id="rel0" Type="http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel"/>
</Relationships>
`

// write3MFModel writes the model part: metadata, the colours, the mesh and
// the build item.
func write3MFModel(w io.Writer, m *Mesh, name string) error {
	bw := bufio.NewWriter(w)
	// shared vertices, in first-use order; colours likewise
	vi := map[Vec3]int{}
	var verts []Vec3
	id := func(p Vec3) int {
		if i, ok := vi[p]; ok {
			return i
		}
		vi[p] = len(verts)
		verts = append(verts, p)
		return len(verts) - 1
	}
	ci := map[Color]int{}
	var colors []Color
	type tri struct{ a, b, c, col int }
	tris := make([]tri, 0, len(m.Tris))
	for _, t := range m.Tris {
		a, b, c := id(t.P[0]), id(t.P[1]), id(t.P[2])
		if a == b || b == c || c == a {
			continue // a safeguard: Clean leaves no repeated corners
		}
		k, ok := ci[t.C]
		if !ok {
			k = len(colors)
			ci[t.C] = k
			colors = append(colors, t.C)
		}
		tris = append(tris, tri{a, b, c, k})
	}
	num := func(v float64) string { return strconv.FormatFloat(float64(float32(v)), 'g', -1, 32) }

	fmt.Fprint(bw, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")
	fmt.Fprint(bw, `<model unit="millimeter" xml:lang="en-US" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02">`+"\n")
	fmt.Fprint(bw, ` <metadata name="Title">`)
	xml.EscapeText(bw, []byte(name))
	fmt.Fprint(bw, "</metadata>\n")
	fmt.Fprint(bw, ` <metadata name="Application">ScottiCAD</metadata>`+"\n")
	fmt.Fprint(bw, " <resources>\n  <basematerials id=\"1\">\n")
	for i, c := range colors {
		fmt.Fprintf(bw, "   <base name=\"Color %d\" displaycolor=\"%s\"/>\n", i+1, hexColor(c))
	}
	fmt.Fprint(bw, "  </basematerials>\n")
	fmt.Fprint(bw, `  <object id="2" type="model" pid="1" pindex="0" name="`)
	xml.EscapeText(bw, []byte(name))
	fmt.Fprint(bw, "\">\n   <mesh>\n    <vertices>\n")
	for _, v := range verts {
		fmt.Fprintf(bw, "     <vertex x=\"%s\" y=\"%s\" z=\"%s\"/>\n", num(v.X), num(v.Y), num(v.Z))
	}
	fmt.Fprint(bw, "    </vertices>\n    <triangles>\n")
	for _, t := range tris {
		fmt.Fprintf(bw, "     <triangle v1=\"%d\" v2=\"%d\" v3=\"%d\" pid=\"1\" p1=\"%d\"/>\n", t.a, t.b, t.c, t.col)
	}
	fmt.Fprint(bw, "    </triangles>\n   </mesh>\n  </object>\n </resources>\n")
	fmt.Fprint(bw, " <build>\n  <item objectid=\"2\"/>\n </build>\n</model>\n")
	return bw.Flush()
}

// hexColor is c as #RRGGBB.
func hexColor(c Color) string {
	b := func(f float32) int {
		v := int(f*255 + 0.5)
		return max(0, min(255, v))
	}
	return fmt.Sprintf("#%02X%02X%02X", b(c.R), b(c.G), b(c.B))
}
