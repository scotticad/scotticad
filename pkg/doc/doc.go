// Package doc is ScottiCAD's document model: a tree of shape and group
// nodes (the full construction history, so groups can always be undone),
// evaluation of that tree into meshes, JSON persistence and undo history.
package doc

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// FormatVersion is the newest file format this build reads. A design is
// saved with the oldest version that holds everything in it, so older
// builds still open it unless it uses something they would lose:
//
//	1: everything before 2
//	2: outline and loft shapes (Node.Sections)
const FormatVersion = 2

// maxNodeID is the largest id a file may give a node (larger ones are
// renumbered): ids count up from the largest, and must not overflow.
const maxNodeID = 1 << 40

const (
	KindGroup = "group"
	KindMesh  = "mesh"

	ModeUnion     = "union"
	ModeBundle    = "bundle"
	ModeIntersect = "intersect"
)

// Node is a shape, an imported mesh, or a group of nodes.
//
// Leaves (shapes and meshes) have geometry centred on their local origin
// with a bounding box equal to Size; M maps local space to the parent's
// space. Groups keep their children (so they can always be ungrouped) and
// M maps the group's space to its parent's.
type Node struct {
	ID          int                `json:"id"`
	Name        string             `json:"name,omitempty"`
	Kind        string             `json:"kind"`
	Params      map[string]float64 `json:"params,omitempty"`
	Text        string             `json:"text,omitempty"`
	Font        string             `json:"font,omitempty"`
	Size        geom.Vec3          `json:"size"`
	M           geom.Mat4          `json:"m"`
	Hole        bool               `json:"hole,omitempty"`
	Color       string             `json:"color"`
	Transparent bool               `json:"transparent,omitempty"`
	Hidden      bool               `json:"hidden,omitempty"`
	Locked      bool               `json:"locked,omitempty"`
	GroupMode   string             `json:"groupMode,omitempty"`
	Multicolor  bool               `json:"multicolor,omitempty"`
	Children    []*Node            `json:"children,omitempty"`
	MeshData    string             `json:"meshData,omitempty"`
	// Sections are an outline's or loft's point data, in the shape's own
	// frame (see shapes.Section). They are replaced, never changed in
	// place, so clones share them.
	Sections []shapes.Section `json:"sections,omitempty"`
	Notes    []Note           `json:"notes,omitempty"`
	// Assembly, on a bundle group, is the assembly kind it was placed as
	// (its Params are the assembly's); see shapes.Placed.
	Assembly string `json:"assembly,omitempty"`

	// MeshData's hash and the string it was worked out for (the pointer
	// keeps that string, so the address can't be another string's)
	hash    string
	hashOf  *byte
	hashLen int
	// the same for Sections
	secHash    string
	secHashOf  *shapes.Section
	secHashLen int
}

// Note is a text note pinned to a point on a shape. It belongs to the node
// it was pinned to, so it moves, turns, groups and saves with it.
type Note struct {
	P    geom.Vec3 `json:"p"` // see Node.NotePoint
	Text string    `json:"text,omitempty"`
}

// NotePoint turns a stored note position into the node's local space
// (before M). On shapes it is stored relative to Size, so a note stays on
// the surface when the shape is resized; on groups it is local as is.
func (n *Node) NotePoint(p geom.Vec3) geom.Vec3 {
	if n.IsGroup() {
		return p
	}
	return p.MulV(n.Size)
}

// NoteParam is the inverse of NotePoint.
func (n *Node) NoteParam(local geom.Vec3) geom.Vec3 {
	if n.IsGroup() {
		return local
	}
	div := func(v, s float64) float64 {
		if math.Abs(s) < 1e-12 {
			return 0
		}
		return v / s
	}
	return geom.V(div(local.X, n.Size.X), div(local.Y, n.Size.Y), div(local.Z, n.Size.Z))
}

// AxisLength is how long a leaf is along its own axis (0 x, 1 y, 2 z): its
// size there times any scale its matrix still holds (a sheared shape's
// matrix keeps its scale; Normalize folds it into Size otherwise).
func (n *Node) AxisLength(axis int) float64 { return n.Size.Axis(axis) * n.M.Col(axis).Len() }

func (n *Node) IsGroup() bool  { return n.Kind == KindGroup }
func (n *Node) IsBundle() bool { return n.IsGroup() && n.GroupMode == ModeBundle }

// IsUnion reports a group that combines its parts (union or intersect),
// as opposed to a bundle.
func (n *Node) IsUnion() bool { return n.IsGroup() && n.GroupMode != ModeBundle }

// IsIntersect reports an intersect group (a merged group whose solid is
// the volume common to all its solid children).
func (n *Node) IsIntersect() bool { return n.IsGroup() && n.GroupMode == ModeIntersect }

// Mode returns the group mode with the default filled in.
func (n *Node) Mode() string {
	if n.GroupMode == "" {
		return ModeUnion
	}
	return n.GroupMode
}

// Label is the name shown in the inspector header.
func (n *Node) Label() string {
	if n.Name != "" {
		return n.Name
	}
	switch {
	case n.IsBundle():
		return "Bundle"
	case n.IsIntersect():
		return "Intersect"
	case n.IsGroup():
		return "Union"
	case n.Kind == KindMesh:
		return "Imported"
	}
	if k, ok := shapes.Lookup(n.Kind); ok {
		return k.Label
	}
	return n.Kind
}

// Values returns the parameter set used to build a shape leaf.
func (n *Node) Values() shapes.Values {
	return shapes.Values{Num: n.Params, Text: n.Text, Font: n.Font, Sections: n.Sections}
}

// sectionsHash is a hash of Sections, worked out once per slice (a loft
// holds thousands of points; its hash is wanted on every cache lookup).
func (n *Node) sectionsHash() string {
	if len(n.Sections) == 0 {
		return ""
	}
	if n.secHashOf != &n.Sections[0] || n.secHashLen != len(n.Sections) || n.secHash == "" {
		n.secHash, n.secHashOf, n.secHashLen = shapes.SectionsKey(n.Sections), &n.Sections[0], len(n.Sections)
	}
	return n.secHash
}

// Clone deep-copies a node (IDs included).
func (n *Node) Clone() *Node {
	c := *n
	if n.Params != nil {
		c.Params = map[string]float64{}
		for k, v := range n.Params {
			c.Params[k] = v
		}
	}
	c.Notes = append([]Note(nil), n.Notes...)
	c.Children = nil
	for _, ch := range n.Children {
		c.Children = append(c.Children, ch.Clone())
	}
	return &c
}

// Walk visits n and all descendants.
func (n *Node) Walk(f func(*Node)) {
	f(n)
	for _, c := range n.Children {
		c.Walk(f)
	}
}

// --- mesh data for imported meshes ---------------------------------------

// EncodeMesh stores triangles as base64 little-endian float32s.
func EncodeMesh(m *geom.Mesh) string {
	buf := make([]byte, 0, len(m.Tris)*36)
	for _, t := range m.Tris {
		for _, p := range t.P {
			buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(float32(p.X)))
			buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(float32(p.Y)))
			buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(float32(p.Z)))
		}
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func DecodeMesh(s string) (*geom.Mesh, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b)%36 != 0 {
		return nil, errors.New("bad mesh data length")
	}
	m := &geom.Mesh{}
	f := func(o int) float64 { return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[o:]))) }
	for o := 0; o < len(b); o += 36 {
		var t geom.Tri
		for k := 0; k < 3; k++ {
			t.P[k] = geom.V(f(o+k*12), f(o+k*12+4), f(o+k*12+8))
		}
		m.Tris = append(m.Tris, t)
	}
	return m, nil
}

// --- evaluation ----------------------------------------------------------

// meshCache keeps evaluated meshes (content key -> *geom.Mesh), up to 3
// million triangles: about 250 MB
var meshCache = geom.NewCache(3_000_000, geom.MeshWeight)

// CacheStats reports what the caches hold: the evaluated meshes, their
// triangles, and the bounds entries (for soak tests watching growth).
func CacheStats() map[string]int {
	return map[string]int{"meshes": meshCache.Len(), "mesh_triangles": meshCache.Weight(),
		"bounds": boundsCache.Len() + framedCache.Len()}
}

// key hashes everything that affects a node's local geometry.
func (n *Node) key() string {
	var sb strings.Builder
	n.writeKey(&sb, true, true)
	h := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(h[:16])
}

// shapeKey is key for a group's boolean before it is coloured: without the
// group's own hole flag and colour (applied afterwards), nor its parts'
// colours unless it keeps them (multicolour). Recolouring a group, or
// making it a hole, then needs no new boolean.
func (n *Node) shapeKey() string {
	var sb strings.Builder
	n.writeKey(&sb, false, n.Multicolor)
	h := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(h[:16])
}

// writeKey writes what n's geometry depends on; own: n's own hole flag
// and colour, colors: its parts' colours.
func (n *Node) writeKey(sb *strings.Builder, own, colors bool) {
	hole, color := n.Hole, n.Color
	if !own {
		hole, color = false, ""
	}
	if !colors && !own {
		color = ""
	}
	fmt.Fprintf(sb, "%s|%v|%s|%v|%s|%v|", n.Kind, hole, color, n.Multicolor, n.GroupMode, n.Size)
	if n.IsGroup() {
		sb.WriteString("[")
		for _, c := range n.Children {
			fmt.Fprintf(sb, "%v;", c.M)
			// a part's hole flag always counts; its colour only if kept
			c.writeKey(sb, colors, colors)
			if !colors {
				fmt.Fprintf(sb, "%v", c.Hole)
			}
		}
		sb.WriteString("]")
		return
	}
	keys := make([]string, 0, len(n.Params))
	for k := range n.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(sb, "%s=%g,", k, n.Params[k])
	}
	fmt.Fprintf(sb, "%q|%q|%s|%d|", n.Text, n.Font, n.sectionsHash(), len(n.MeshData))
	if n.Kind == KindMesh {
		sb.WriteString(n.meshDataHash())
	}
}

// meshDataHash is a hash of MeshData, worked out once per string (an
// imported mesh is megabytes; its hash is wanted on every cache lookup).
func (n *Node) meshDataHash() string {
	if n.hashOf != unsafe.StringData(n.MeshData) || n.hashLen != len(n.MeshData) || n.hash == "" {
		h := sha256.Sum256([]byte(n.MeshData))
		n.hash, n.hashOf, n.hashLen = hex.EncodeToString(h[:8]), unsafe.StringData(n.MeshData), len(n.MeshData)
	}
	return n.hash
}

// LeafMesh returns a leaf's geometry in local space, coloured.
func (n *Node) LeafMesh() *geom.Mesh {
	col := geom.ParseColor(n.Color)
	if n.Kind == KindMesh {
		k := "mesh:" + n.key()
		if m, ok := meshCache.Load(k); ok {
			return m.(*geom.Mesh)
		}
		raw, err := DecodeMesh(n.MeshData)
		if err != nil {
			raw = &geom.Mesh{}
		}
		// an STL is often several overlapping shells: made one solid,
		// or booleans with it leave holes
		m := raw.FitTo(n.Size).Solidify().Colored(col)
		meshCache.Store(k, m)
		return m
	}
	k := "leaf:" + n.key()
	if m, ok := meshCache.Load(k); ok {
		return m.(*geom.Mesh)
	}
	m := shapes.Build(n.Kind, n.Size, n.Values()).Colored(col)
	meshCache.Store(k, m)
	return m
}

// LocalPart is a renderable piece: a cached local-space mesh plus the
// matrix that places it in the world. Meshes are shared between frames so
// GPU uploads can be cached by pointer.
type LocalPart struct {
	Mesh        *geom.Mesh
	M           geom.Mat4
	Hole        bool
	Transparent bool
	Owner       *Node
}

// LocalParts is like Parts but without transforming geometry.
func (n *Node) LocalParts() []LocalPart {
	var out []LocalPart
	var rec func(x *Node, xf geom.Mat4, hole, transp bool)
	rec = func(x *Node, xf geom.Mat4, hole, transp bool) {
		w := xf.Mul(x.M)
		h := hole || x.Hole
		t := transp || x.Transparent
		switch {
		case x.IsBundle():
			for _, c := range x.Children {
				rec(c, w, h, t)
			}
		case x.IsGroup():
			m, _ := x.unionMeshFor()
			out = append(out, LocalPart{m, w, h, t, x})
		default:
			out = append(out, LocalPart{x.LeafMesh(), w, h, t, x})
		}
	}
	rec(n, geom.Identity(), false, false)
	return out
}

var boundsCache = geom.NewCache(100_000, nil) // key+matrix -> geom.Box

// WorldBounds is a cached Bounds. While a group is still being evaluated
// in the background it is the preview's bounds, and isn't cached.
func (n *Node) WorldBounds() geom.Box {
	k := fmt.Sprintf("%s|%v|%v", n.key(), n.M, n.Hidden)
	if b, ok := boundsCache.Load(k); ok {
		return b.(geom.Box)
	}
	parts, exact := n.parts()
	b := geom.EmptyBox()
	for _, p := range parts {
		b = b.Union(p.Mesh.Bounds())
	}
	if exact {
		boundsCache.Store(k, b)
	}
	return b
}

// contribution is one piece of geometry that takes part in a union group.
type contribution struct {
	mesh *geom.Mesh
	hole bool
}

// childContribs evaluates a child in its parent's space.
func childContribs(c *Node, xf geom.Mat4) []contribution {
	if c.IsBundle() {
		var out []contribution
		for _, g := range c.Children {
			for _, p := range childContribs(g, xf.Mul(c.M)) {
				p.hole = p.hole || c.Hole
				out = append(out, p)
			}
		}
		return out
	}
	if c.IsGroup() {
		return []contribution{{c.UnionMesh().Transform(xf.Mul(c.M)), c.Hole}}
	}
	return []contribution{{c.LeafMesh().Transform(xf.Mul(c.M)), c.Hole}}
}

// UnionMesh evaluates a merged group in its own space: solids are unioned
// (intersected for an intersect group), then holes are subtracted.
func (n *Node) UnionMesh() *geom.Mesh {
	k := "u:" + n.key()
	if m, ok := meshCache.Load(k); ok {
		return m.(*geom.Mesh)
	}
	sk := "ub:" + n.shapeKey()
	if m, ok := meshCache.Load(sk); ok {
		// the same shape, only recoloured (or made a hole)
		m := m.(*geom.Mesh)
		if !n.Multicolor {
			m = m.Colored(geom.ParseColor(n.Color))
		}
		meshCache.Store(k, m)
		return m
	}
	var solids, holes []*geom.Mesh
	var first *geom.Color
	// for an intersect group, each child is one solid (a bundle child's
	// parts together): A ∩ (B ∪ C), not A ∩ B ∩ C
	var perChild []*geom.Mesh
	// children are independent: evaluate them in parallel, then combine in
	// document order so results are deterministic
	contribs := make([][]contribution, len(n.Children))
	var wg sync.WaitGroup
	for i, c := range n.Children {
		wg.Add(1)
		go func(i int, c *Node) {
			defer wg.Done()
			contribs[i] = childContribs(c, geom.Identity())
		}(i, c)
	}
	wg.Wait()
	for _, cs := range contribs {
		var mine []*geom.Mesh
		for _, p := range cs {
			if p.hole {
				holes = append(holes, p.mesh)
			} else {
				solids = append(solids, p.mesh)
				mine = append(mine, p.mesh)
				if first == nil && len(p.mesh.Tris) > 0 {
					col := p.mesh.Tris[0].C
					first = &col
				}
			}
		}
		if len(mine) > 0 && n.IsIntersect() {
			perChild = append(perChild, geom.UnionAll(mine))
		}
	}
	var m *geom.Mesh
	if len(solids) == 0 {
		m = geom.UnionAll(holes)
	} else {
		var s *geom.Mesh
		if n.IsIntersect() {
			s = perChild[0]
			for _, m := range perChild[1:] {
				s = geom.Intersect(s, m)
			}
		} else {
			s = geom.UnionAll(solids)
		}
		if len(holes) > 0 {
			h := geom.UnionAll(holes)
			if first != nil {
				h = h.Colored(*first)
			}
			s = geom.Subtract(s, h)
		}
		m = s
	}
	// close the hairline cracks BSP booleans leave along seams, so exports
	// are watertight
	m = m.FixTJunctions()
	meshCache.Store(sk, m)
	if !n.Multicolor {
		m = m.Colored(geom.ParseColor(n.Color))
	}
	meshCache.Store(k, m)
	return m
}

// Part is one renderable/exportable piece of a top-level node in world
// space.
type Part struct {
	Mesh        *geom.Mesh
	Hole        bool
	Transparent bool
	Top         *Node // top-level node (selection unit)
	Owner       *Node // node the geometry came from
}

// Parts evaluates a top-level node into world-space parts. Bundles keep
// their children separate; union groups produce one merged part.
func (n *Node) Parts() []Part {
	p, _ := n.parts()
	return p
}

// PartsExact is Parts, also reporting whether every group's boolean has
// been evaluated (false: some parts are still previews).
func (n *Node) PartsExact() ([]Part, bool) { return n.parts() }

// parts is Parts, also reporting whether every group was evaluated (not a
// preview).
func (n *Node) parts() ([]Part, bool) {
	exact := true
	var out []Part
	var rec func(x *Node, xf geom.Mat4, hole, transp bool)
	rec = func(x *Node, xf geom.Mat4, hole, transp bool) {
		w := xf.Mul(x.M)
		h := hole || x.Hole
		t := transp || x.Transparent
		switch {
		case x.IsBundle():
			for _, c := range x.Children {
				rec(c, w, h, t)
			}
		case x.IsGroup():
			m, ok := x.unionMeshFor()
			exact = exact && ok
			out = append(out, Part{Mesh: m.Transform(w), Hole: h, Transparent: t, Top: n, Owner: x})
		default:
			out = append(out, Part{Mesh: x.LeafMesh().Transform(w), Hole: h, Transparent: t, Top: n, Owner: x})
		}
	}
	rec(n, geom.Identity(), false, false)
	return out, exact
}

// WorldMesh merges all parts of a top-level node (holes excluded unless
// includeHoles).
func (n *Node) WorldMesh(includeHoles bool) *geom.Mesh {
	var ms []*geom.Mesh
	for _, p := range n.Parts() {
		if p.Hole && !includeHoles {
			continue
		}
		ms = append(ms, p.Mesh)
	}
	return geom.Merge(ms...)
}

// Bounds returns the world bounding box of a top-level node.
func (n *Node) Bounds() geom.Box {
	b := geom.EmptyBox()
	for _, p := range n.Parts() {
		b = b.Union(p.Mesh.Bounds())
	}
	return b
}

// --- document ------------------------------------------------------------

// Settings holds per-document workspace settings.
type Settings struct {
	Units       string  `json:"units"` // "mm" or "in"
	WorkplaneW  float64 `json:"workplaneW"`
	WorkplaneL  float64 `json:"workplaneL"`
	Snap        float64 `json:"snap"` // mm; 0 = off
	ShowGrid    bool    `json:"showGrid"`
	ShowShadows bool    `json:"showShadows"`
}

func DefaultSettings() Settings {
	return Settings{Units: "mm", WorkplaneW: 200, WorkplaneL: 200, Snap: 1, ShowGrid: true, ShowShadows: true}
}

// Camera is saved with the document so it reopens with the same view.
type Camera struct {
	Target geom.Vec3 `json:"target"`
	Yaw    float64   `json:"yaw"`
	Pitch  float64   `json:"pitch"`
	Dist   float64   `json:"dist"`
	Ortho  bool      `json:"ortho,omitempty"`
}

// Document is a design.
type Document struct {
	Version  int      `json:"version"`
	Name     string   `json:"name"`
	Settings Settings `json:"settings"`
	Camera   *Camera  `json:"camera,omitempty"`
	Nodes    []*Node  `json:"nodes"`
	NextID   int      `json:"nextId"`
	// Notes pinned to the workplane rather than to a shape (P is a world
	// position)
	Notes []Note `json:"notes,omitempty"`
}

func New(name string) *Document {
	return &Document{Version: FormatVersion, Name: name, Settings: DefaultSettings(), NextID: 1}
}

func (d *Document) newID() int {
	if d.NextID < 1 {
		d.NextID = 1
	}
	id := d.NextID
	d.NextID++
	return id
}

func (d *Document) renumber(n *Node) {
	n.Walk(func(x *Node) { x.ID = d.newID() })
}

// Find returns the top-level node with id, or nil.
func (d *Document) Find(id int) *Node {
	for _, n := range d.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// FindAny searches the whole tree.
func (d *Document) FindAny(id int) *Node {
	var found *Node
	for _, n := range d.Nodes {
		n.Walk(func(x *Node) {
			if x.ID == id && found == nil {
				found = x
			}
		})
	}
	return found
}

// FindAll returns top-level nodes with the given ids in document order.
func (d *Document) FindAll(ids []int) []*Node {
	set := map[int]bool{}
	for _, id := range ids {
		set[id] = true
	}
	var out []*Node
	for _, n := range d.Nodes {
		if set[n.ID] {
			out = append(out, n)
		}
	}
	return out
}

func IDs(ns []*Node) []int {
	out := make([]int, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}

// Marshal serialises the document, with the oldest format version that
// holds it (see FormatVersion).
func (d *Document) Marshal() ([]byte, error) {
	c := *d
	c.Version = d.neededVersion()
	return json.MarshalIndent(&c, "", " ")
}

func (d *Document) neededVersion() int {
	v := 1
	for _, n := range d.Nodes {
		n.Walk(func(x *Node) {
			if len(x.Sections) > 0 {
				v = 2
			}
		})
	}
	return v
}

// Unmarshal parses a document and repairs IDs.
func Unmarshal(b []byte) (*Document, error) {
	d := &Document{}
	if err := json.Unmarshal(b, d); err != nil {
		return nil, err
	}
	if d.Version == 0 {
		d.Version = FormatVersion
	}
	if d.Version > FormatVersion {
		// read anyway and re-saved, whatever the newer format holds that
		// this version doesn't know would be lost
		return nil, fmt.Errorf("this design was saved by a newer ScottiCAD (file format %d; this one reads up to %d)", d.Version, FormatVersion)
	}
	d.Nodes = dropNil(d.Nodes)
	if d.Settings.Units == "" {
		d.Settings = DefaultSettings()
	}
	// a file's settings are trusted no further than the dialog's: a zero
	// workplane would put the home camera at distance 0
	def := DefaultSettings()
	s := &d.Settings
	if s.Units != "mm" && s.Units != "in" {
		s.Units = def.Units
	}
	size := func(v *float64, dv float64) {
		if !(*v > 0 && *v <= 10000) { // also false for NaN
			*v = dv
		}
	}
	size(&s.WorkplaneW, def.WorkplaneW)
	size(&s.WorkplaneL, def.WorkplaneL)
	if !(s.Snap >= 0 && s.Snap <= 10000) {
		s.Snap = def.Snap
	}
	if c := d.Camera; c != nil && !(c.Dist > 0 && c.Dist < 1e7 && math.Abs(c.Pitch) <= math.Pi/2 && !math.IsNaN(c.Yaw) && !math.IsNaN(c.Target.X+c.Target.Y+c.Target.Z)) {
		d.Camera = nil // the home view instead
	}
	maxID := 0
	seen := map[int]bool{}
	for _, n := range d.Nodes {
		n.Walk(func(x *Node) {
			if x.M == (geom.Mat4{}) {
				x.M = geom.Identity() // no matrix in the file
			}
			if x.ID <= 0 || x.ID > maxNodeID || seen[x.ID] { // too large: the next id would overflow
				x.ID = 0
			}
			seen[x.ID] = true
			if x.ID > maxID {
				maxID = x.ID
			}
		})
	}
	if d.NextID <= maxID || d.NextID > maxNodeID { // too large: it would overflow and repeat ids
		d.NextID = maxID + 1
	}
	for _, n := range d.Nodes {
		n.Walk(func(x *Node) {
			if x.ID == 0 {
				x.ID = d.newID()
			}
		})
	}
	return d, nil
}

// dropNil removes null entries (a damaged or hand-edited file) from a node
// list, at every depth.
func dropNil(ns []*Node) []*Node {
	out := ns[:0]
	for _, n := range ns {
		if n == nil {
			continue
		}
		n.Children = dropNil(n.Children)
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Prewarm evaluates every top-level node in parallel so their meshes are
// cached before the first frame (opening a large design).
func (d *Document) Prewarm() {
	var wg sync.WaitGroup
	for _, n := range d.Nodes {
		wg.Add(1)
		go func(n *Node) {
			defer wg.Done()
			n.LocalParts()
			n.WorldBounds()
		}(n)
	}
	wg.Wait()
}

// Bounds of a set of top-level nodes.
func Bounds(ns []*Node) geom.Box {
	b := geom.EmptyBox()
	for _, n := range ns {
		b = b.Union(n.WorldBounds())
	}
	return b
}

// BoundsIn returns the bounds of ns measured in frame f (f maps frame
// coordinates to world ones): the box a workplane attached to f sees.
func BoundsIn(ns []*Node, f geom.Mat4) geom.Box {
	if f == geom.Identity() {
		return Bounds(ns)
	}
	inv, ok := f.Inverse()
	if !ok {
		return Bounds(ns)
	}
	b := geom.EmptyBox()
	for _, n := range ns {
		b = b.Union(n.boundsIn(inv))
	}
	return b
}

var framedCache = geom.NewCache(100_000, nil) // key+matrix+frame -> geom.Box

func (n *Node) boundsIn(inv geom.Mat4) geom.Box {
	k := fmt.Sprintf("%s|%v|%v|%v", n.key(), n.M, n.Hidden, inv)
	if b, ok := framedCache.Load(k); ok {
		return b.(geom.Box)
	}
	b := geom.EmptyBox()
	parts, exact := n.parts()
	for _, p := range parts {
		for _, t := range p.Mesh.Tris {
			for _, v := range t.P {
				b = b.Extend(inv.Point(v))
			}
		}
	}
	if exact { // a preview's bounds (a group still being evaluated) aren't kept
		framedCache.Store(k, b)
	}
	return b
}
