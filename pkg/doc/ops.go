package doc

import (
	"encoding/json"
	"errors"
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// AddShape creates a shape of the given kind with its bottom centre at pos.
func (d *Document) AddShape(kind string, pos geom.Vec3, hole bool) (*Node, error) {
	k, ok := shapes.Lookup(kind)
	if !ok {
		return nil, errors.New("unknown shape kind: " + kind)
	}
	v := k.Defaults()
	if parts, ok := k.Parts(v); ok {
		g := &Node{ID: d.newID(), Kind: KindGroup, GroupMode: ModeBundle, Assembly: k.ID, Name: k.Label,
			Params: v.Num, Color: k.Color, M: geom.Translate(pos)}
		d.layout(g, parts, v, hole)
		d.Nodes = append(d.Nodes, g)
		return g, nil
	}
	n := &Node{ID: d.newID(), Kind: k.ID, Params: v.Num, Text: v.Text, Font: v.Font, Sections: v.Sections, Size: k.Size, Color: k.Color, Hole: hole}
	if k.ID == "text" {
		n.Size = shapes.TextNaturalSize(n.Text, n.Font, n.Params["height"])
	}
	n.M = geom.Translate(pos.Add(geom.V(0, 0, n.Size.Z/2)))
	d.Nodes = append(d.Nodes, n)
	return n, nil
}

// AddProfile creates an outline or loft from its sections (see
// shapes.Section). The points are millimetres in world space: the shape is
// placed so they land exactly where they say.
func (d *Document) AddProfile(kind string, ss []shapes.Section, hole bool) (*Node, error) {
	k, ok := shapes.Lookup(kind)
	if !ok || !shapes.IsProfile(kind) {
		return nil, errors.New("not an outline or loft kind: " + kind)
	}
	if err := shapes.CheckSections(kind, ss); err != nil {
		return nil, err
	}
	v := k.Defaults()
	v.Sections = ss
	b := shapes.ProfileBounds(kind, v)
	n := &Node{ID: d.newID(), Kind: kind, Params: v.Num, Sections: ss, Size: b.Size(), Color: k.Color, Hole: hole, M: geom.Translate(b.Center())}
	d.Nodes = append(d.Nodes, n)
	return n, nil
}

// SetSections replaces an outline's or loft's sections. The points are in
// the shape's own frame, the one they were first given in: a shape that
// hasn't been moved, turned or resized puts them exactly where they say,
// and one that has carries its move, turn or resize over to them.
func SetSections(n *Node, ss []shapes.Section) error {
	if !shapes.IsProfile(n.Kind) {
		return errors.New("not an outline or loft")
	}
	if err := shapes.CheckSections(n.Kind, ss); err != nil {
		return err
	}
	keepFrame(n, func() { n.Sections = ss })
	return nil
}

// SetProfileParam changes an outline's or loft's parameter (smoothing),
// keeping its points where they were (see SetSections).
func SetProfileParam(n *Node, key string, v float64) {
	keepFrame(n, func() { SetParam(n, key, v) })
}

// keepFrame makes a change to an outline or loft that may change the box
// its points fill, keeping the points in the shape's own frame: an
// unmoved shape's points stay exactly where they say, and a moved, turned
// or resized one carries that over.
func keepFrame(n *Node, change func()) {
	old := shapes.ProfileBounds(n.Kind, n.Values())
	change()
	nw := shapes.ProfileBounds(n.Kind, n.Values())
	if nw.IsEmpty() {
		return
	}
	if old.IsEmpty() {
		// a damaged shape: start again from the points
		n.Size, n.M = nw.Size(), n.M.Mul(geom.Translate(nw.Center()))
		return
	}
	// the shape's scale on each axis, kept
	f := geom.V(1, 1, 1)
	for i := 0; i < 3; i++ {
		if s := old.Size().Axis(i); s > 1e-12 {
			f = f.WithAxis(i, n.Size.Axis(i)/s)
		}
	}
	n.Size = nw.Size().MulV(f)
	n.M = n.M.Mul(geom.Translate(nw.Center().Sub(old.Center()).MulV(f)))
}

// layout gives assembly g one child per part, reusing existing children of
// the same kind (in order), so a part keeps its id, colour and notes.
// Children for parts no longer produced (e.g. the lid turned off) are
// removed.
func (d *Document) layout(g *Node, parts []shapes.Placed, v shapes.Values, hole bool) {
	used := map[*Node]bool{}
	var kids []*Node
	for _, p := range parts {
		var c *Node
		for _, ch := range g.Children {
			if !used[ch] && ch.Kind == p.Kind {
				c = ch
				break
			}
		}
		if c == nil {
			c = &Node{ID: d.newID(), Kind: p.Kind, Name: p.Name, Color: p.Color, Hole: hole}
		}
		used[c] = true
		c.Params = map[string]float64{}
		for k, x := range v.Num {
			c.Params[k] = x
		}
		c.Size = shapes.PartSize(p.Kind, v)
		c.M = p.M
		kids = append(kids, c)
	}
	g.Children = kids
}

// SetAssemblyParam changes a parameter of assembly group g and rebuilds
// its parts to match.
func (d *Document) SetAssemblyParam(g *Node, key string, val float64) error {
	k, ok := shapes.Lookup(g.Assembly)
	if !ok {
		return errors.New("unknown assembly: " + g.Assembly)
	}
	if g.Params == nil {
		g.Params = k.Defaults().Num
	}
	g.Params[key] = val
	v := shapes.Values{Num: g.Params}
	parts, _ := k.Parts(v)
	hole := len(g.Children) > 0 && g.Children[0].Hole
	d.layout(g, parts, v, hole)
	return nil
}

// AddMesh adds an imported mesh with its bottom centre at pos.
func (d *Document) AddMesh(name string, m *geom.Mesh, pos geom.Vec3) *Node {
	b := m.Bounds()
	local := m.Transform(geom.Translate(b.Center().Neg()))
	n := &Node{ID: d.newID(), Name: name, Kind: KindMesh, Size: b.Size(), Color: "#a0a0a0", MeshData: EncodeMesh(local)}
	n.M = geom.Translate(pos.Add(geom.V(0, 0, n.Size.Z/2)))
	d.Nodes = append(d.Nodes, n)
	return n
}

// Remove deletes top-level nodes.
func (d *Document) Remove(ids []int) {
	set := map[int]bool{}
	for _, id := range ids {
		set[id] = true
	}
	var keep []*Node
	for _, n := range d.Nodes {
		if !set[n.ID] {
			keep = append(keep, n)
		}
	}
	d.Nodes = keep
}

// Duplicate copies nodes in place and returns the copies.
func (d *Document) Duplicate(ids []int, offset geom.Vec3) []*Node {
	var out []*Node
	for _, n := range d.FindAll(ids) {
		c := n.Clone()
		d.renumber(c)
		c.M = geom.Translate(offset).Mul(c.M)
		d.Nodes = append(d.Nodes, c)
		out = append(out, c)
	}
	return out
}

// CopyJSON serialises nodes for the clipboard.
func (d *Document) CopyJSON(ids []int) []byte {
	b, _ := json.Marshal(d.FindAll(ids))
	return b
}

// PasteJSON inserts clipboard nodes with fresh IDs.
func (d *Document) PasteJSON(data []byte, offset geom.Vec3) ([]*Node, error) {
	var ns []*Node
	if err := json.Unmarshal(data, &ns); err != nil {
		return nil, err
	}
	ns = dropNil(ns)
	for _, n := range ns {
		n.Walk(func(x *Node) {
			if x.M == (geom.Mat4{}) {
				x.M = geom.Identity() // no matrix (as Unmarshal repairs)
			}
		})
	}
	for _, n := range ns {
		d.renumber(n)
		n.M = geom.Translate(offset).Mul(n.M)
		d.Nodes = append(d.Nodes, n)
	}
	return ns, nil
}

// Group combines top-level nodes into a group (mode union, bundle or
// intersect). The children are kept intact so the group can be ungrouped
// later.
func (d *Document) Group(ids []int, mode string) (*Node, error) {
	ns := d.FindAll(ids)
	if len(ns) < 2 {
		return nil, errors.New("select at least two shapes to group")
	}
	c := Bounds(ns).Center()
	if math.IsInf(c.X, 0) || math.IsNaN(c.X) {
		c = geom.V(0, 0, 0)
	}
	g := &Node{ID: d.newID(), Kind: KindGroup, GroupMode: mode, M: geom.Translate(c)}
	allHoles := true
	for _, n := range ns {
		if !n.Hole {
			allHoles = false
			if g.Color == "" {
				g.Color = n.Color
			}
		}
	}
	if g.Color == "" {
		g.Color = ns[0].Color
	}
	g.Hole = allHoles && mode != ModeBundle
	inv := geom.Translate(c.Neg())
	pos := -1
	for i, n := range d.Nodes {
		for _, s := range ns {
			if n == s && pos < 0 {
				pos = i
			}
		}
	}
	for _, n := range ns {
		n.M = inv.Mul(n.M)
		g.Children = append(g.Children, n)
	}
	d.Remove(ids)
	if pos < 0 || pos > len(d.Nodes) {
		pos = len(d.Nodes)
	}
	d.Nodes = append(d.Nodes[:pos], append([]*Node{g}, d.Nodes[pos:]...)...)
	return g, nil
}

// Ungroup dissolves groups one level, returning the released children.
func (d *Document) Ungroup(ids []int) []*Node {
	var released []*Node
	var out []*Node
	set := map[int]bool{}
	for _, id := range ids {
		set[id] = true
	}
	for _, n := range d.Nodes {
		if set[n.ID] && n.IsGroup() {
			for _, c := range n.Children {
				c.M = n.M.Mul(c.M)
				// what the group was, its parts are: a hole group's parts
				// stay holes (made solid, they would fill what it cut)
				c.Hole = c.Hole || n.Hole
				c.Transparent = c.Transparent || n.Transparent
				c.Hidden = c.Hidden || n.Hidden
				out = append(out, c)
				released = append(released, c)
			}
			continue
		}
		out = append(out, n)
	}
	d.Nodes = out
	return released
}

// Apply premultiplies nodes' transforms by a world transform and folds any
// pure axis scaling back into leaf sizes.
func Apply(ns []*Node, t geom.Mat4) {
	for _, n := range ns {
		n.M = t.Mul(n.M)
		Normalize(n)
	}
}

// Normalize moves scale out of a leaf's matrix into its Size when the
// matrix has no shear, so parametric features (bevels, radii, text height)
// stay correct.
func Normalize(n *Node) {
	if n.IsGroup() || !n.M.IsOrthogonal() {
		return
	}
	s := n.M.AxisScales()
	if math.Abs(s.X-1) < 1e-12 && math.Abs(s.Y-1) < 1e-12 && math.Abs(s.Z-1) < 1e-12 {
		return
	}
	for i := 0; i < 3; i++ {
		col := n.M.Col(i)
		l := col.Len()
		if l > 0 {
			n.M.SetCol(i, col.Mul(1/l))
		}
	}
	n.Size = geom.V(n.Size.X*s.X, n.Size.Y*s.Y, n.Size.Z*s.Z)
	if n.Kind == "text" {
		if n.Params == nil {
			n.Params = map[string]float64{}
		}
		n.Params["height"] = n.Size.Z
	}
}

// Move translates nodes.
func Move(ns []*Node, delta geom.Vec3) { Apply(ns, geom.Translate(delta)) }

// ScaleAbout scales nodes along world axes about anchor.
func ScaleAbout(ns []*Node, f, anchor geom.Vec3) {
	Apply(ns, geom.Translate(anchor).Mul(geom.Scale(f)).Mul(geom.Translate(anchor.Neg())))
}

// RotateAbout rotates nodes about a world axis through center.
func RotateAbout(ns []*Node, axis int, deg float64, center geom.Vec3) {
	Apply(ns, geom.Translate(center).Mul(geom.RotateAxis(axis, deg*math.Pi/180)).Mul(geom.Translate(center.Neg())))
}

// SetParam updates a leaf parameter, keeping text size consistent.
func SetParam(n *Node, key string, v float64) {
	if n.Params == nil {
		n.Params = map[string]float64{}
	}
	n.Params[key] = v
	if n.Kind == "text" && key == "height" {
		n.Size.Z = v
	}
}

// SetText changes a text node's string or font, preserving its scale.
func SetText(n *Node, text, font string) {
	h := n.Params["height"]
	if h == 0 {
		h = n.Size.Z
	}
	old := shapes.TextNaturalSize(n.Text, n.Font, h)
	n.Text, n.Font = text, font
	nw := shapes.TextNaturalSize(n.Text, n.Font, h)
	sx, sy := 1.0, 1.0
	if old.X > 0 {
		sx = n.Size.X / old.X
	}
	if old.Y > 0 {
		sy = n.Size.Y / old.Y
	}
	n.Size = geom.V(nw.X*sx, nw.Y*sy, n.Size.Z)
}

// SetColor sets a node's colour (recursively for groups).
func SetColor(n *Node, hex string) {
	n.Color = hex
	if n.IsGroup() && !n.Multicolor {
		for _, c := range n.Children {
			SetColor(c, hex)
		}
	}
}

// ExportMesh merges the solid parts of the given nodes (all visible nodes
// if none given).
func (d *Document) ExportMesh(ids []int) *geom.Mesh {
	ns := d.FindAll(ids)
	if len(ids) == 0 {
		for _, n := range d.Nodes {
			if !n.Hidden {
				ns = append(ns, n)
			}
		}
	}
	var ms []*geom.Mesh
	Sync(func() { // exports are always exact
		for _, n := range ns {
			ms = append(ms, n.WorldMesh(false))
		}
	})
	return geom.Merge(ms...)
}

// History is a bounded undo/redo stack of document snapshots.
type History struct {
	undo, redo [][]byte
	// group is the undo depth where an open group (Begin) started, or -1
	group int
	// start is where the group began (group drops below it while undo
	// steps back past it, and comes back up with redo)
	start int
	open  bool
}

const historyLimit = 500

func (h *History) Push(state []byte) {
	h.undo = append(h.undo, state)
	if len(h.undo) > historyLimit {
		switch {
		case h.open && h.group+2 < len(h.undo):
			// in a group, its own oldest step goes (End would merge it
			// away anyway); the state it started from stays, so End still
			// undoes to it, and so does the history before it
			h.undo = append(h.undo[:h.group+1], h.undo[h.group+2:]...)
		default:
			h.undo = h.undo[1:]
			if h.open && h.group > 0 {
				h.group--
			}
			if h.open && h.start > 0 {
				h.start--
			}
		}
	}
	h.redo = nil
}

func (h *History) Undo(cur []byte) ([]byte, bool) {
	if len(h.undo) == 0 {
		return nil, false
	}
	s := h.undo[len(h.undo)-1]
	h.undo = h.undo[:len(h.undo)-1]
	h.redo = append(h.redo, cur)
	if h.open && len(h.undo) < h.group {
		h.group = len(h.undo)
	}
	return s, true
}

// Begin starts a group: the steps pushed until End become one undo step
// (a script's run undoes as a whole). Undo and redo inside the group work
// step by step as usual.
func (h *History) Begin() { h.group, h.start, h.open = len(h.undo), len(h.undo), true }

// End closes the group, merging its steps into one.
func (h *History) End() {
	if h.open && len(h.undo) > h.group+1 {
		h.undo = h.undo[:h.group+1]
	}
	h.open = false
}

func (h *History) Redo(cur []byte) ([]byte, bool) {
	if len(h.redo) == 0 {
		return nil, false
	}
	s := h.redo[len(h.redo)-1]
	h.redo = h.redo[:len(h.redo)-1]
	h.undo = append(h.undo, cur)
	if h.open && h.group < h.start {
		// redone back toward where the group began: the steps before it
		// stay its own (End mustn't merge them in)
		h.group = min(h.start, len(h.undo))
	}
	return s, true
}

func (h *History) CanUndo() bool { return len(h.undo) > 0 }
func (h *History) CanRedo() bool { return len(h.redo) > 0 }
func (h *History) Depth() int    { return len(h.undo) }

// Clear empties the history; an open group (Begin) carries on from the
// start, so what follows still undoes as one step.
func (h *History) Clear() {
	h.undo, h.redo = nil, nil
	h.group, h.start = 0, 0
}

// DropRedo forgets the redo steps (after undoing something that
// shouldn't be redone, such as a cancelled drag's duplicate).
func (h *History) DropRedo() { h.redo = nil }

// States returns every stored undo and redo state (for inspection).
func (h *History) States() [][]byte { return append(append([][]byte{}, h.undo...), h.redo...) }
