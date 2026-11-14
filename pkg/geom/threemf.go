package geom

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Write3MF writes m as a 3MF package (3MF core specification 1.3): a zip
// holding [Content_Types].xml, _rels/.rels and 3D/3dmodel.model. The model
// is in millimetres: an object per piece laid out apart, each a mesh per
// colour (its vertices shared between its triangles: 3MF meshes are
// indexed), coloured from a base-materials table, the meshes the parts of
// the object when there are several.
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

// write3MFModel writes the model part: metadata, the colours, the objects
// and a build item for each. An object is a cluster of pieces whose
// bounds touch or overlap (a slicer moves and arranges objects one by
// one; an inlay or lettering stays with what it sits on). With several
// colours an object's meshes, one per colour, are its parts (components),
// which a slicer takes as a part per filament: PrusaSlicer and Bambu
// Studio load them so, where they ignore colours given triangle by
// triangle in a file they didn't write. Each part has its colour too, for
// the slicers that read it.
//
// Each colour's closed shells are found first, so an inlay that shares
// edges with its pocket stays a part of its own colour. A shell of several
// colours (a multicolour group's) can't be split so:
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
	// the pieces: each colour's closed shells (an inlay is its own, though
	// it shares edges with the pocket it fills), then the shells of several
	// colours, from the triangles left: a multicolour group's, whose
	// colours are open on their own
	type piece struct {
		tris  []int // indexes into m.Tris, in order
		color int   // its colour (the most used one, when mixed)
		mixed bool
		box   Box
	}
	byColour := make([][]int, len(colors))
	for i, t := range m.Tris {
		k := ci[t.C]
		byColour[k] = append(byColour[k], i)
	}
	var pieces []*piece
	var rest []int
	for _, idx := range byColour {
		for _, s := range shellsOf(m.Tris, idx) {
			if closedShell(m.Tris, s) {
				pieces = append(pieces, &piece{tris: s, color: ci[m.Tris[s[0]].C]})
			} else {
				rest = append(rest, s...)
			}
		}
	}
	sort.Ints(rest)
	for _, s := range shellsOf(m.Tris, rest) {
		uses := map[int]int{}
		for _, i := range s {
			uses[ci[m.Tris[i].C]]++
		}
		p := &piece{tris: s, color: -1, mixed: len(uses) > 1}
		for k, n := range uses {
			if p.color < 0 || n > uses[p.color] || n == uses[p.color] && k < p.color {
				p.color = k
			}
		}
		pieces = append(pieces, p)
	}
	sort.SliceStable(pieces, func(a, b int) bool { return pieces[a].tris[0] < pieces[b].tris[0] })
	for _, p := range pieces {
		p.box = EmptyBox()
		for _, i := range p.tris {
			t := m.Tris[i]
			p.box = p.box.Extend(t.P[0]).Extend(t.P[1]).Extend(t.P[2])
		}
	}

	// the objects: pieces whose bounds touch or overlap go together (a
	// part's inlay, the lettering on it, an assembly's parts), so a piece
	// laid out apart comes into a slicer as an object of its own, its
	// colours still its parts
	parent := make([]int, len(pieces))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := range pieces {
		for j := i + 1; j < len(pieces); j++ {
			if pieces[i].box.Overlaps(pieces[j].box, 1e-6) {
				if a, b := find(i), find(j); a != b {
					parent[b] = a
				}
			}
		}
	}
	// each object's parts: a colour's single-coloured pieces together,
	// each piece of several colours apart, in first-use order
	type part struct {
		tris  []Tri
		color int  // the part's colour
		mixed bool // its triangles' colours differ
	}
	var objects [][]*part
	objectOf := map[int]int{}
	single := map[[2]int]*part{} // (object, colour)
	for i, p := range pieces {
		r := find(i)
		o, ok := objectOf[r]
		if !ok {
			o = len(objects)
			objectOf[r] = o
			objects = append(objects, nil)
		}
		tris := make([]Tri, len(p.tris))
		for k, ti := range p.tris {
			tris[k] = m.Tris[ti]
		}
		if p.mixed {
			objects[o] = append(objects[o], &part{tris: tris, color: p.color, mixed: true})
			continue
		}
		key := [2]int{o, p.color}
		if single[key] == nil {
			single[key] = &part{color: p.color}
			objects[o] = append(objects[o], single[key])
		}
		single[key].tris = append(single[key].tris, tris...)
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
	nextID := 2
	var items []int
	for o, parts := range objects {
		// the object's name: the design's, numbered when there are several
		oName := name
		if len(objects) > 1 {
			oName = fmt.Sprintf("%s %d", name, o+1)
		}
		first := nextID
		for _, p := range parts {
			writePart3MF(bw, p.tris, p.color, p.mixed, nextID, oName, len(parts) > 1, colors, ci, num)
			nextID++
		}
		if len(parts) == 1 {
			items = append(items, first)
			continue
		}
		// the parts together: one object, so a slicer keeps them in place
		fmt.Fprintf(bw, `  <object id="%d" type="model" name="`, nextID)
		xml.EscapeText(bw, []byte(oName))
		fmt.Fprint(bw, "\">\n   <components>\n")
		for id := first; id < nextID; id++ {
			fmt.Fprintf(bw, "    <component objectid=\"%d\"/>\n", id)
		}
		fmt.Fprint(bw, "   </components>\n  </object>\n")
		items = append(items, nextID)
		nextID++
	}
	fmt.Fprint(bw, " </resources>\n <build>\n")
	for _, id := range items {
		fmt.Fprintf(bw, "  <item objectid=\"%d\"/>\n", id)
	}
	fmt.Fprint(bw, " </build>\n</model>\n")
	return bw.Flush()
}

// writePart3MF writes one mesh object: a part of colour k (its other
// colours' triangles, in a mixed part, coloured one by one), named for its
// object, and for its colour when the object has several parts.
func writePart3MF(bw *bufio.Writer, tris []Tri, k int, mixed bool, objID int, name string, named bool,
	colors []Color, ci map[Color]int, num func(float64) string) {
	// shared vertices, in first-use order, at the float32 precision they are
	// written with: corners that differ only below it are one vertex
	vi := map[Vec3]int{}
	var verts []Vec3
	id := func(p Vec3) int {
		p = Vec3{float64(float32(p.X)), float64(float32(p.Y)), float64(float32(p.Z))}
		if i, ok := vi[p]; ok {
			return i
		}
		vi[p] = len(verts)
		verts = append(verts, p)
		return len(verts) - 1
	}
	type tri struct{ a, b, c, k int }
	idx := make([]tri, 0, len(tris))
	for _, t := range tris {
		a, b, c := id(t.P[0]), id(t.P[1]), id(t.P[2])
		if a == b || b == c || c == a {
			continue // two corners rounded together (3MF requires distinct ones)
		}
		idx = append(idx, tri{a, b, c, ci[t.C]})
	}
	objName := name
	if named {
		objName += " " + hexColor(colors[k])
		if mixed {
			objName += " (multicolor)"
		}
	}
	fmt.Fprintf(bw, `  <object id="%d" type="model" pid="1" pindex="%d" name="`, objID, k)
	xml.EscapeText(bw, []byte(objName))
	fmt.Fprint(bw, "\">\n   <mesh>\n    <vertices>\n")
	for _, v := range verts {
		fmt.Fprintf(bw, "     <vertex x=\"%s\" y=\"%s\" z=\"%s\"/>\n", num(v.X), num(v.Y), num(v.Z))
	}
	fmt.Fprint(bw, "    </vertices>\n    <triangles>\n")
	for _, t := range idx {
		if mixed && t.k != k {
			fmt.Fprintf(bw, "     <triangle v1=\"%d\" v2=\"%d\" v3=\"%d\" pid=\"1\" p1=\"%d\"/>\n", t.a, t.b, t.c, t.k)
			continue
		}
		fmt.Fprintf(bw, "     <triangle v1=\"%d\" v2=\"%d\" v3=\"%d\"/>\n", t.a, t.b, t.c)
	}
	fmt.Fprint(bw, "    </triangles>\n   </mesh>\n  </object>\n")
}

// shellsOf splits the triangles idx (indexes into tris, in order) into
// connected pieces: triangles that share an edge (exact end points) are in
// the same piece. Each piece keeps the order of idx; the pieces are in the
// order of their first triangles.
func shellsOf(tris []Tri, idx []int) [][]int {
	parent := make([]int, len(idx))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	owner := map[vedge]int{}
	for i, ti := range idx {
		t := tris[ti]
		for k := 0; k < 3; k++ {
			a, b := t.P[k], t.P[(k+1)%3]
			if lessVec(b, a) {
				a, b = b, a
			}
			if j, ok := owner[vedge{a, b}]; ok {
				if x, y := find(i), find(j); x != y {
					parent[x] = y
				}
			} else {
				owner[vedge{a, b}] = i
			}
		}
	}
	at := map[int]int{}
	var out [][]int
	for i, ti := range idx {
		r := find(i)
		k, ok := at[r]
		if !ok {
			k = len(out)
			at[r] = k
			out = append(out, nil)
		}
		out[k] = append(out[k], ti)
	}
	return out
}

// closedShell reports whether the triangles s (indexes into tris) close up
// on their own: each edge is crossed as often one way as the other.
func closedShell(tris []Tri, s []int) bool {
	uses := map[[2]Vec3]int{}
	for _, ti := range s {
		t := tris[ti]
		for k := 0; k < 3; k++ {
			uses[[2]Vec3{t.P[k], t.P[(k+1)%3]}]++
		}
	}
	for e, n := range uses {
		if uses[[2]Vec3{e[1], e[0]}] != n {
			return false
		}
	}
	return true
}

// hexColor is c as #RRGGBB (3MF's display colours are upper case).
func hexColor(c Color) string { return strings.ToUpper(c.Hex()) }
