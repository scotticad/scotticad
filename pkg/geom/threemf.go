package geom

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Write3MF writes m as a 3MF package (3MF core specification 1.3): a zip
// holding [Content_Types].xml, _rels/.rels and 3D/3dmodel.model. The model
// is in millimetres: a mesh per colour (its vertices shared between its
// triangles: 3MF meshes are indexed), coloured from a base-materials
// table, the meshes the parts of one object when there are several.
// Triangles of no area are left out, as the other writers do; 3MF requires
// every triangle's three vertices to be distinct.
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

// write3MFModel writes the model part: metadata, the colours, a mesh
// object per colour and the build item. With several colours the meshes
// are the parts (components) of one object, which a slicer takes as a part
// per filament: PrusaSlicer and Bambu Studio load them so, where they
// ignore colours given triangle by triangle in a file they didn't write.
// Each part has its colour too, for the slicers that read it.
//
// A shell of several colours (a multicolour group's) can't be split so:
// each colour's share of its surface is open, the faces between colours
// gone in the union. It stays one part, closed, its triangles coloured one
// by one, the part taking its most-used colour.
func write3MFModel(w io.Writer, m *Mesh, name string) error {
	bw := bufio.NewWriter(w)
	// the colours in first-use order
	ci := map[Color]int{}
	var colors []Color
	for _, t := range m.Tris {
		if _, ok := ci[t.C]; !ok {
			ci[t.C] = len(colors)
			colors = append(colors, t.C)
		}
	}
	// the parts: a colour's single-coloured shells together, each shell of
	// several colours apart, in first-use order
	type part struct {
		tris  []Tri
		color int  // the part's colour
		mixed bool // its triangles' colours differ
	}
	var parts []*part
	single := map[int]*part{}
	for _, s := range m.Shells() {
		uses := map[int]int{}
		for _, t := range s.Tris {
			uses[ci[t.C]]++
		}
		if len(uses) == 1 {
			k := ci[s.Tris[0].C]
			if single[k] == nil {
				single[k] = &part{color: k}
				parts = append(parts, single[k])
			}
			single[k].tris = append(single[k].tris, s.Tris...)
			continue
		}
		p := &part{tris: s.Tris, color: -1, mixed: true}
		for k, n := range uses {
			if p.color < 0 || n > uses[p.color] || n == uses[p.color] && k < p.color {
				p.color = k
			}
		}
		parts = append(parts, p)
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
	for j, p := range parts {
		// shared vertices, in first-use order
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
		type tri struct{ a, b, c, k int }
		idx := make([]tri, 0, len(p.tris))
		for _, t := range p.tris {
			a, b, c := id(t.P[0]), id(t.P[1]), id(t.P[2])
			if a == b || b == c || c == a {
				continue // a safeguard: Clean leaves no repeated corners
			}
			idx = append(idx, tri{a, b, c, ci[t.C]})
		}
		objName := name
		if len(parts) > 1 {
			objName += " " + hexColor(colors[p.color])
			if p.mixed {
				objName += " (multicolour)"
			}
		}
		fmt.Fprintf(bw, `  <object id="%d" type="model" pid="1" pindex="%d" name="`, 2+j, p.color)
		xml.EscapeText(bw, []byte(objName))
		fmt.Fprint(bw, "\">\n   <mesh>\n    <vertices>\n")
		for _, v := range verts {
			fmt.Fprintf(bw, "     <vertex x=\"%s\" y=\"%s\" z=\"%s\"/>\n", num(v.X), num(v.Y), num(v.Z))
		}
		fmt.Fprint(bw, "    </vertices>\n    <triangles>\n")
		for _, t := range idx {
			if p.mixed && t.k != p.color {
				fmt.Fprintf(bw, "     <triangle v1=\"%d\" v2=\"%d\" v3=\"%d\" pid=\"1\" p1=\"%d\"/>\n", t.a, t.b, t.c, t.k)
				continue
			}
			fmt.Fprintf(bw, "     <triangle v1=\"%d\" v2=\"%d\" v3=\"%d\"/>\n", t.a, t.b, t.c)
		}
		fmt.Fprint(bw, "    </triangles>\n   </mesh>\n  </object>\n")
	}
	top := 2
	if len(parts) > 1 {
		// the parts together: one object, so a slicer keeps them in place
		top = 2 + len(parts)
		fmt.Fprintf(bw, `  <object id="%d" type="model" name="`, top)
		xml.EscapeText(bw, []byte(name))
		fmt.Fprint(bw, "\">\n   <components>\n")
		for j := range parts {
			fmt.Fprintf(bw, "    <component objectid=\"%d\"/>\n", 2+j)
		}
		fmt.Fprint(bw, "   </components>\n  </object>\n")
	}
	fmt.Fprintf(bw, " </resources>\n <build>\n  <item objectid=\"%d\"/>\n </build>\n</model>\n", top)
	return bw.Flush()
}

// hexColor is c as #RRGGBB (3MF's display colours are upper case).
func hexColor(c Color) string { return strings.ToUpper(c.Hex()) }
