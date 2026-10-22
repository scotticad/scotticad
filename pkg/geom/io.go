package geom

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// WriteSTL writes a binary STL. Triangles of no area are left out: they
// would have no normal, and slicers flag them as damaged facets.
func WriteSTL(w io.Writer, m *Mesh, name string) error {
	m = m.Clean()
	hdr := make([]byte, 80)
	copy(hdr, "ScottiCAD "+name)
	buf := make([]byte, 0, 84+50*len(m.Tris))
	buf = append(buf, hdr...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(m.Tris)))
	put := func(f float64) { buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(float32(f))) }
	for _, t := range m.Tris {
		n := t.Normal()
		put(n.X)
		put(n.Y)
		put(n.Z)
		for _, p := range t.P {
			put(p.X)
			put(p.Y)
			put(p.Z)
		}
		buf = append(buf, 0, 0)
	}
	_, err := w.Write(buf)
	return err
}

// ReadSTL reads a binary or ASCII STL.
func ReadSTL(data []byte) (*Mesh, error) {
	grey := Color{0.6, 0.6, 0.6}
	ascii := bytes.HasPrefix(bytes.TrimSpace(data), []byte("solid")) &&
		bytes.Contains(data[:min(len(data), 4096)], []byte("facet"))
	if len(data) >= 84 {
		n := binary.LittleEndian.Uint32(data[80:84])
		end := uint64(84) + uint64(n)*50
		// exactly the length the count gives; or, for a file that isn't
		// text, up to 1 KB longer (some exporters pad the end, and some begin
		// the binary header with "solid"); any more and it isn't an STL
		if n > 0 && (end == uint64(len(data)) || (!ascii && end <= uint64(len(data)) && uint64(len(data))-end <= 1024)) {
			m := &Mesh{Tris: make([]Tri, 0, n)}
			f := func(o int) float64 { return float64(math.Float32frombits(binary.LittleEndian.Uint32(data[o:]))) }
			for i := 0; i < int(n); i++ {
				o := 84 + i*50 + 12
				var t Tri
				for k := 0; k < 3; k++ {
					t.P[k] = V(f(o+k*12), f(o+k*12+4), f(o+k*12+8))
				}
				if !finiteTri(t) {
					return nil, fmt.Errorf("STL triangle %d has a coordinate that isn't a number", i+1)
				}
				t.C = grey
				m.Tris = append(m.Tris, t)
			}
			return m.Clean(), nil
		}
	}
	if !ascii {
		return nil, errors.New("not a valid STL file")
	}
	m := &Mesh{}
	var pts []Vec3
	for ln, line := range lines(data) {
		f := strings.Fields(line)
		if len(f) >= 1 && f[0] == "vertex" {
			p, err := parseVec(f[1:])
			if err != nil {
				return nil, fmt.Errorf("STL line %d: %v", ln, err)
			}
			pts = append(pts, p)
		} else if len(f) >= 1 && f[0] == "endfacet" {
			for i := 1; i+1 < len(pts); i++ {
				m.Tris = append(m.Tris, Tri{P: [3]Vec3{pts[0], pts[i], pts[i+1]}, C: grey})
			}
			pts = pts[:0]
		}
	}
	if len(m.Tris) == 0 {
		return nil, errors.New("STL contains no triangles")
	}
	return m.Clean(), nil
}

// lines yields each line of data with its 1-based number, however long.
func lines(data []byte) func(func(int, string) bool) {
	return func(yield func(int, string) bool) {
		for n := 1; len(data) > 0; n++ {
			line := data
			if i := bytes.IndexByte(data, '\n'); i >= 0 {
				line, data = data[:i], data[i+1:]
			} else {
				data = nil
			}
			if !yield(n, string(bytes.TrimRight(line, "\r"))) {
				return
			}
		}
	}
}

// parseVec reads three finite numbers (any after them are ignored).
func parseVec(f []string) (Vec3, error) {
	if len(f) < 3 {
		return Vec3{}, errors.New("a point needs three coordinates")
	}
	var c [3]float64
	for i := range c {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return Vec3{}, fmt.Errorf("%q isn't a number", f[i])
		}
		c[i] = v
	}
	return V(c[0], c[1], c[2]), nil
}

func finiteTri(t Tri) bool {
	for _, p := range t.P {
		for _, v := range []float64{p.X, p.Y, p.Z} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return false
			}
		}
	}
	return true
}

// WriteOBJ writes a Wavefront OBJ with an accompanying MTL describing the
// colours (mtlName is referenced by mtllib; mtl receives the material text).
func WriteOBJ(w io.Writer, mtl io.Writer, m *Mesh, mtlName string) error {
	// as the STL and glTF writers: triangles of no area are left out, so
	// every format holds the same faces
	m = m.Clean()
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "# ScottiCAD\n")
	if mtl != nil {
		fmt.Fprintf(bw, "mtllib %s\n", mtlName)
	}
	idx := map[Vec3]int{}
	var verts []Vec3
	vi := func(p Vec3) int {
		if i, ok := idx[p]; ok {
			return i
		}
		verts = append(verts, p)
		idx[p] = len(verts)
		return len(verts)
	}
	type face struct {
		a, b, c int
		col     Color
	}
	faces := make([]face, len(m.Tris))
	for i, t := range m.Tris {
		faces[i] = face{vi(t.P[0]), vi(t.P[1]), vi(t.P[2]), t.C}
	}
	for _, v := range verts {
		fmt.Fprintf(bw, "v %g %g %g\n", v.X, v.Y, v.Z)
	}
	mats := map[Color]string{}
	cur := ""
	for _, f := range faces {
		if mtl != nil {
			name, ok := mats[f.col]
			if !ok {
				name = fmt.Sprintf("color_%d", len(mats))
				mats[f.col] = name
				fmt.Fprintf(mtl, "newmtl %s\nKd %g %g %g\n\n", name, f.col.R, f.col.G, f.col.B)
			}
			if name != cur {
				fmt.Fprintf(bw, "usemtl %s\n", name)
				cur = name
			}
		}
		fmt.Fprintf(bw, "f %d %d %d\n", f.a, f.b, f.c)
	}
	return bw.Flush()
}

// ReadOBJ reads vertices and (possibly polygonal) faces from an OBJ file.
func ReadOBJ(data []byte) (*Mesh, error) {
	var verts []Vec3
	m := &Mesh{}
	grey := Color{0.6, 0.6, 0.6}
	for ln, line := range lines(data) {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i] // a comment, to the end of the line
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "v":
			// a vertex skipped would shift every face after it onto the
			// wrong vertices
			p, err := parseVec(f[1:])
			if err != nil {
				return nil, fmt.Errorf("OBJ line %d: %v", ln, err)
			}
			verts = append(verts, p)
		case "f":
			var ids []int
			for _, s := range f[1:] {
				s = strings.SplitN(s, "/", 2)[0]
				i, err := strconv.Atoi(s)
				if err != nil {
					return nil, fmt.Errorf("OBJ line %d: bad vertex %q", ln, s)
				}
				if i < 0 {
					i = len(verts) + i + 1
				}
				if i < 1 || i > len(verts) {
					return nil, fmt.Errorf("OBJ face index %d out of range", i)
				}
				ids = append(ids, i-1)
			}
			for i := 1; i+1 < len(ids); i++ {
				m.Tris = append(m.Tris, Tri{P: [3]Vec3{verts[ids[0]], verts[ids[i]], verts[ids[i+1]]}, C: grey})
			}
		}
	}
	if len(m.Tris) == 0 {
		return nil, errors.New("OBJ contains no faces")
	}
	return m.Clean(), nil
}
