package geom

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
)

// Limits on what a 3MF file may make the reader do: it is outside data.
const (
	max3MFEntries   = 10000   // files in the package
	max3MFPartBytes = 1 << 30 // one model part, unpacked
	max3MFDepth     = 32      // components inside components
	max3MFPlaced    = 1 << 20 // objects placed, components and all
	max3MFTriangles = 20e6    // triangles in what is read
)

// Read3MF reads the build of a 3MF package (3MF core specification 1.3)
// into one mesh in millimetres. Each build item is placed by its
// transform, and objects made of components are put together, including
// components kept in other model files of the package (the production
// extension, as Bambu Studio and PrusaSlicer save them). Colours come from
// base materials and colour groups; a triangle with neither is mid grey.
// Support objects and objects of type "other" are left out.
//
// All the model files are taken to be in the root model's unit.
func Read3MF(data []byte) (*Mesh, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("3MF: not a 3MF package: %v", err)
	}
	if len(z.File) > max3MFEntries {
		return nil, fmt.Errorf("3MF: too many files in the package (%d)", len(z.File))
	}
	r := &tmfReader{files: map[string]*zip.File{}, models: map[string]*tmfModel{}}
	for _, f := range z.File {
		r.files[tmfKey(f.Name)] = f
	}
	root := r.rootPath()
	m, err := r.model(root)
	if err != nil {
		return nil, err
	}
	out := &Mesh{}
	for _, it := range m.items {
		if err := r.emit(out, it.path, root, it.obj, it.xf, 0); err != nil {
			return nil, err
		}
	}
	if len(out.Tris) == 0 {
		return nil, errors.New("3MF contains no triangles to print")
	}
	if m.scale != 1 {
		out = out.Transform(Scale(V(m.scale, m.scale, m.scale)))
	}
	return out.Clean(), nil
}

type tmfRef struct {
	obj  int
	path string // the model file holding the object; empty for this one
	xf   Mat4
}

type tmfTri struct {
	v       [3]int
	pid, p1 int // the triangle's own property, when pid >= 0
}

type tmfObject struct {
	typ        string
	verts      []Vec3
	tris       []tmfTri
	pid, pidx  int // the object's default property, when pid >= 0
	components []tmfRef
}

type tmfModel struct {
	scale   float64 // the unit, in millimetres
	objects map[int]*tmfObject
	colors  map[int][]Color // base materials and colour groups by id
	items   []tmfRef
}

type tmfReader struct {
	files  map[string]*zip.File
	models map[string]*tmfModel
	placed int // objects placed so far
}

// tmfKey is a part name as the package's lookups use it: no leading slash,
// and without regard to case (package part names are case-insensitive).
func tmfKey(name string) string {
	return strings.ToLower(strings.TrimPrefix(path.Clean("/"+name), "/"))
}

// rootPath is the model part the package's relationships name as its 3D
// model, or the usual 3D/3dmodel.model.
func (r *tmfReader) rootPath() string {
	const fallback = "3D/3dmodel.model"
	f := r.files[tmfKey("_rels/.rels")]
	if f == nil {
		return fallback
	}
	b, err := r.read(f)
	if err != nil {
		return fallback
	}
	var rels struct {
		Rel []struct {
			Target string `xml:"Target,attr"`
			Type   string `xml:"Type,attr"`
		} `xml:"Relationship"`
	}
	if xml.Unmarshal(b, &rels) != nil {
		return fallback
	}
	for _, rel := range rels.Rel {
		if strings.HasSuffix(rel.Type, "/3dmodel") && rel.Target != "" {
			return rel.Target
		}
	}
	return fallback
}

// read unpacks one part, refusing one larger than max3MFPartBytes.
func (r *tmfReader) read(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > max3MFPartBytes {
		return nil, fmt.Errorf("3MF: %s is too large", f.Name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("3MF: %s: %v", f.Name, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, max3MFPartBytes+1))
	if err != nil {
		return nil, fmt.Errorf("3MF: %s: %v", f.Name, err)
	}
	if len(b) > max3MFPartBytes {
		return nil, fmt.Errorf("3MF: %s is too large", f.Name)
	}
	return b, nil
}

// model reads (once) the model part at p.
func (r *tmfReader) model(p string) (*tmfModel, error) {
	key := tmfKey(p)
	if m, ok := r.models[key]; ok {
		return m, nil
	}
	f := r.files[key]
	if f == nil {
		return nil, fmt.Errorf("3MF: the package has no model part %s", p)
	}
	b, err := r.read(f)
	if err != nil {
		return nil, err
	}
	m, err := parse3MFModel(b)
	if err != nil {
		return nil, fmt.Errorf("3MF: %s: %v", f.Name, err)
	}
	r.models[key] = m
	return m, nil
}

// emit adds object id of the model at p (the current model file when p is
// empty), placed by xf, to out.
func (r *tmfReader) emit(out *Mesh, p, cur string, id int, xf Mat4, depth int) error {
	if p == "" {
		p = cur
	}
	if depth > max3MFDepth {
		return errors.New("3MF: components nested too deeply (or in a loop)")
	}
	// a few objects, each made of several of the next, are a great many
	// placed: a small file must not be able to ask for that
	if r.placed++; r.placed > max3MFPlaced {
		return errors.New("3MF: too many components")
	}
	m, err := r.model(p)
	if err != nil {
		return err
	}
	o := m.objects[id]
	if o == nil {
		return fmt.Errorf("3MF: %s has no object %d", p, id)
	}
	if o.typ == "support" || o.typ == "other" {
		return nil
	}
	if len(out.Tris)+len(o.tris) > max3MFTriangles {
		return errors.New("3MF: too many triangles")
	}
	flip := xf.Det3() < 0
	grey := Color{0.6, 0.6, 0.6}
	for _, t := range o.tris {
		var tri Tri
		for k, vi := range t.v {
			if vi < 0 || vi >= len(o.verts) {
				return fmt.Errorf("3MF: object %d has a triangle with vertex %d of %d", id, vi, len(o.verts))
			}
			tri.P[k] = xf.Point(o.verts[vi])
			if !finiteVec(tri.P[k]) { // transforms that multiply out of range
				return fmt.Errorf("3MF: object %d is placed out of range", id)
			}
		}
		if flip {
			tri.P[1], tri.P[2] = tri.P[2], tri.P[1]
		}
		tri.C = grey
		pid, pi := o.pid, o.pidx
		if t.pid >= 0 {
			pid, pi = t.pid, t.p1
		}
		if cs, ok := m.colors[pid]; ok && pi >= 0 && pi < len(cs) {
			tri.C = cs[pi]
		}
		out.Tris = append(out.Tris, tri)
	}
	for _, c := range o.components {
		if err := r.emit(out, c.path, p, c.obj, xf.Mul(c.xf), depth+1); err != nil {
			return err
		}
	}
	return nil
}

// parse3MFModel reads a model part. Element and attribute names are
// matched without their namespaces, so the production extension's path
// attribute is found whatever prefix a writer gave it.
func parse3MFModel(b []byte) (*tmfModel, error) {
	m := &tmfModel{scale: 1, objects: map[int]*tmfObject{}, colors: map[int][]Color{}}
	d := xml.NewDecoder(bytes.NewReader(b))
	var obj *tmfObject
	group := -1 // the base materials or colour group being read
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch e := tok.(type) {
		case xml.StartElement:
			a := func(name string) (string, bool) {
				for _, at := range e.Attr {
					if at.Name.Local == name {
						return at.Value, true
					}
				}
				return "", false
			}
			switch e.Name.Local {
			case "model":
				if u, ok := a("unit"); ok {
					s, ok := tmfUnits[u]
					if !ok {
						return nil, fmt.Errorf("unknown unit %q", u)
					}
					m.scale = s
				}
			case "basematerials", "colorgroup":
				id, err := intAttr(a, "id", -1)
				if err != nil {
					return nil, err
				}
				group = id
				m.colors[id] = nil
			case "base", "color":
				if group < 0 {
					continue
				}
				name := "color"
				if e.Name.Local == "base" {
					name = "displaycolor"
				}
				s, _ := a(name)
				m.colors[group] = append(m.colors[group], tmfColor(s))
			case "object":
				id, err := intAttr(a, "id", -1)
				if err != nil {
					return nil, err
				}
				typ, _ := a("type")
				pid, err := intAttr(a, "pid", -1)
				if err != nil {
					return nil, err
				}
				pidx, err := intAttr(a, "pindex", 0)
				if err != nil {
					return nil, err
				}
				obj = &tmfObject{typ: typ, pid: pid, pidx: pidx}
				m.objects[id] = obj
			case "vertex":
				if obj == nil {
					continue
				}
				var p [3]float64
				for i, k := range []string{"x", "y", "z"} {
					s, _ := a(k)
					v, err := strconv.ParseFloat(s, 64)
					if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
						return nil, fmt.Errorf("vertex %d: bad %s %q", len(obj.verts), k, s)
					}
					p[i] = v
				}
				obj.verts = append(obj.verts, V(p[0], p[1], p[2]))
			case "triangle":
				if obj == nil {
					continue
				}
				var t tmfTri
				for i, k := range []string{"v1", "v2", "v3"} {
					v, err := intAttr(a, k, -1)
					if err != nil {
						return nil, err
					}
					t.v[i] = v
				}
				if t.pid, err = intAttr(a, "pid", -1); err != nil {
					return nil, err
				}
				if t.p1, err = intAttr(a, "p1", -1); err != nil {
					return nil, err
				}
				if t.pid >= 0 && t.p1 < 0 { // a group without an index: the object's index
					t.p1 = obj.pidx
				}
				if t.pid < 0 && t.p1 >= 0 { // an index into the object's group
					t.pid = obj.pid
				}
				obj.tris = append(obj.tris, t)
			case "component", "item":
				ref, err := tmfReference(a)
				if err != nil {
					return nil, err
				}
				if e.Name.Local == "item" {
					m.items = append(m.items, ref)
				} else if obj != nil {
					obj.components = append(obj.components, ref)
				}
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "object":
				obj = nil
			case "basematerials", "colorgroup":
				group = -1
			}
		}
	}
	return m, nil
}

// tmfUnits are the units a model may be in, in millimetres.
var tmfUnits = map[string]float64{
	"micron": 0.001, "millimeter": 1, "centimeter": 10, "inch": 25.4, "foot": 304.8, "meter": 1000,
}

func intAttr(a func(string) (string, bool), name string, def int) (int, error) {
	s, ok := a(name)
	if !ok {
		return def, nil
	}
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("bad %s %q", name, s)
	}
	return v, nil
}

// tmfReference reads an item's or component's object, path and transform.
func tmfReference(a func(string) (string, bool)) (tmfRef, error) {
	id, err := intAttr(a, "objectid", -1)
	if err != nil {
		return tmfRef{}, err
	}
	ref := tmfRef{obj: id, xf: Identity()}
	ref.path, _ = a("path")
	if s, ok := a("transform"); ok {
		f := strings.Fields(s)
		if len(f) != 12 {
			return tmfRef{}, fmt.Errorf("transform %q: want 12 numbers", s)
		}
		var v [12]float64
		for i, x := range f {
			n, err := strconv.ParseFloat(x, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return tmfRef{}, fmt.Errorf("transform %q: bad number %q", s, x)
			}
			v[i] = n
		}
		// 3MF gives the matrix's first three columns row by row, for
		// points as rows: x' = x·m00 + y·m10 + z·m20 + m30
		ref.xf = Mat4{v[0], v[1], v[2], 0, v[3], v[4], v[5], 0, v[6], v[7], v[8], 0, v[9], v[10], v[11], 1}
	}
	return ref, nil
}

// tmfColor reads "#RRGGBB" or "#RRGGBBAA" (the alpha is dropped).
func tmfColor(s string) Color {
	s = strings.TrimSpace(s)
	if len(s) == 9 {
		s = s[:7]
	}
	return ParseColor(s)
}
