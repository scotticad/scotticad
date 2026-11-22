package doc

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// A stretch group shows its children lengthened or shortened at a plane
// square to one of its axes (see geom.Stretch): Amount above zero inserts
// that much at the plane, the cut outline carried straight across; below
// zero removes a slab that thick centred on the plane and closes the gap.
// The low end (the - side of the axis) stays where it is.
//
// It holds the children unchanged and the numbers (in Params), so it can be
// changed or taken off later, and every shape in it stays editable. Like a
// bundle it keeps its parts apart: each keeps its colour, and a hole stays
// a hole, stretched with the rest. Stretching commutes with the booleans,
// so stretching each part is the same as stretching what they make.
// Ungrouping bakes it in: shapes the plane crosses become meshes, those
// beyond it move, the rest stay as they are; with Amount 0 that is the
// shapes as they were.
//
// The plane is in the group's own frame: Axis is the group's axis, At the
// plane's position along it. A ScottiCAD from before stretches reads the
// mode as a union, so it shows the shapes unstretched, and keeps the
// numbers when it saves.
const ModeStretch = "stretch"

var axisNames = []string{"X", "Y", "Z"}

// stretchParams are a stretch's settings as the Properties panel presents
// them. Position is the plane's distance from the low end of the shapes
// as they are unstretched; Length is the stretched length (setting it sets
// Amount). Lengths are in the group's parent space (see stretchScale).
var stretchParams = []shapes.Param{
	{Key: "axis", Label: "Axis", Kind: shapes.Choice, Min: 0, Max: 2, Step: 1, Default: 2, Int: true, SizeAxis: -1, Options: axisNames},
	{Key: "at", Label: "Plane Position", Kind: shapes.Number, Min: -maxSpacing, Max: maxSpacing, Step: 1, Default: 0, SizeAxis: -1, Length: true},
	{Key: "amount", Label: "Amount", Kind: shapes.Number, Min: -maxSpacing, Max: maxSpacing, Step: 1, Default: 0, SizeAxis: -1, Length: true},
	{Key: "length", Label: "Length", Kind: shapes.Number, Min: 0, Max: 2 * maxSpacing, Step: 1, Default: 0, SizeAxis: -1, Length: true},
}

// StretchParams are a stretch's settings, in the panel's order.
func StretchParams() []shapes.Param { return stretchParams }

// StretchParam is the stretch setting named key.
func StretchParam(key string) (*shapes.Param, bool) {
	for i := range stretchParams {
		if stretchParams[i].Key == key {
			return &stretchParams[i], true
		}
	}
	return nil, false
}

// IsStretch reports a stretch group.
func (n *Node) IsStretch() bool { return n.IsGroup() && n.GroupMode == ModeStretch }

// stretchPlane is the stretch as stored, made safe: the axis, the plane's
// position along it and the amount, in the group's own frame.
func (n *Node) stretchPlane() (axis int, at, amount float64) {
	num := func(key string, lo, hi float64) float64 {
		v, ok := n.Params[key]
		if !ok || math.IsNaN(v) {
			return 0
		}
		return math.Max(lo, math.Min(hi, v))
	}
	axis = int(math.Round(num("axis", 0, 2)))
	if _, ok := n.Params["axis"]; !ok {
		axis = 2
	}
	return axis, num("at", -1e6, 1e6), num("amount", -maxSpacing, maxSpacing)
}

// stretchScale is what a length along the stretch's axis is multiplied by
// on the way to the parent's space (a stretch group that has been resized).
func (n *Node) stretchScale() float64 {
	axis, _, _ := n.stretchPlane()
	if s := n.M.Col(axis).Len(); s > 1e-12 && !math.IsInf(s, 0) && !math.IsNaN(s) {
		return s
	}
	return 1
}

// stretchCache keeps each stretch group's stretched pieces by its key.
var stretchCache = geom.NewCache(2_000_000, func(v any) int {
	w := 1
	for _, p := range v.([]piece) {
		w += len(p.mesh.Tris)
	}
	return w
})

// stretchPieces are the group's children's pieces, each stretched, in the
// group's own frame. exact is false when a merged group in it gave its
// preview (see collect); those aren't kept.
func (n *Node) stretchPieces(sync bool) ([]piece, bool) {
	// (the pieces carry the parts' transparency, which key leaves out)
	var tr strings.Builder
	var walk func(c *Node)
	walk = func(c *Node) {
		if c.Transparent {
			tr.WriteByte('t')
		} else {
			tr.WriteByte('o')
		}
		tr.WriteByte('(')
		for _, ch := range c.Children {
			walk(ch)
		}
		tr.WriteByte(')')
	}
	for _, c := range n.Children {
		walk(c)
	}
	k := "st:" + n.key() + "|" + tr.String()
	if v, ok := stretchCache.Load(k); ok {
		return v.([]piece), true
	}
	axis, at, amount := n.stretchPlane()
	var raw []piece
	exact := true
	for _, c := range n.Children {
		exact = c.collect(geom.Identity(), false, false, sync, func(p piece) { raw = append(raw, p) }) && exact
	}
	out := make([]piece, 0, len(raw))
	for _, p := range raw {
		m := geom.Stretch(p.mesh.Transform(p.m), axis, at, amount)
		if len(m.Tris) == 0 {
			continue // all in the slab taken out
		}
		out = append(out, piece{m, geom.Identity(), p.hole, p.transp, p.owner})
	}
	if exact {
		stretchCache.Store(k, out)
	}
	return out, exact
}

// extentIn is how far x reaches along axis of a frame: x placed in it by
// xf (its parent's frame there). ok is false when it has no geometry.
func (x *Node) extentIn(xf geom.Mat4, axis int) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(1), math.Inf(-1)
	x.collect(xf, false, false, true, func(p piece) {
		for _, t := range p.mesh.Tris {
			for _, v := range t.P {
				c := p.m.Point(v).Axis(axis)
				lo, hi = math.Min(lo, c), math.Max(hi, c)
			}
		}
	})
	return lo, hi, lo <= hi
}

var extentCache = geom.NewCache(100_000, nil) // key -> [2]float64

// stretchBase is how far the stretch's children reach along its axis, in
// its own frame, unstretched.
func (n *Node) stretchBase() (lo, hi float64, ok bool) {
	axis, _, _ := n.stretchPlane()
	k := fmt.Sprintf("sb:%s|%d", n.key(), axis)
	if v, ok := extentCache.Load(k); ok {
		e := v.([2]float64)
		return e[0], e[1], true
	}
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, c := range n.Children {
		if a, b, ok := c.extentIn(geom.Identity(), axis); ok {
			lo, hi = math.Min(lo, a), math.Max(hi, b)
		}
	}
	if !(lo <= hi) {
		return 0, 0, false
	}
	extentCache.Store(k, [2]float64{lo, hi})
	return lo, hi, true
}

// stretchedLength is how long the stretched result is along the axis, in
// the group's own frame.
func (n *Node) stretchedLength() float64 {
	axis, _, _ := n.stretchPlane()
	lo, hi := math.Inf(1), math.Inf(-1)
	ps, _ := n.stretchPieces(false)
	for _, p := range ps {
		b := p.mesh.Bounds()
		lo, hi = math.Min(lo, b.Min.Axis(axis)), math.Max(hi, b.Max.Axis(axis))
	}
	if !(lo <= hi) {
		return 0
	}
	return hi - lo
}

// StretchValue is a stretch setting as the panel and the API give it:
// lengths as they measure in the group's parent space.
func (n *Node) StretchValue(key string) float64 {
	axis, at, amount := n.stretchPlane()
	s := n.stretchScale()
	switch key {
	case "axis":
		return float64(axis)
	case "at":
		lo, _, _ := n.stretchBase()
		return (at - lo) * s
	case "amount":
		return amount * s
	case "length":
		return n.stretchedLength() * s
	}
	return 0
}

// StretchValues are all of a stretch's settings (see StretchValue).
func (n *Node) StretchValues() map[string]float64 {
	out := map[string]float64{}
	for _, p := range stretchParams {
		out[p.Key] = n.StretchValue(p.Key)
	}
	return out
}

// SetStretchParam sets a stretch setting from a value as StretchValue
// gives it. A new axis puts the plane through the middle of the shapes
// along it.
func SetStretchParam(n *Node, key string, v float64) error {
	if _, ok := StretchParam(key); !ok {
		return fmt.Errorf("a stretch has no setting %q", key)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("%s must be a number", key)
	}
	if n.Params == nil {
		n.Params = map[string]float64{}
	}
	axis, at, amount := n.stretchPlane()
	s := n.stretchScale()
	lo, hi, ok := n.stretchBase()
	switch key {
	case "axis":
		a := int(math.Round(v))
		if a < 0 || a > 2 {
			return errors.New("axis must be 0 (x), 1 (y) or 2 (z)")
		}
		old := n.Params["axis"]
		_, had := n.Params["axis"]
		n.Params["axis"] = float64(a)
		lo, hi, ok := n.stretchBase()
		if ok && amount < 0 && -amount >= hi-lo {
			// the shortening must still fit the shapes' length along the new
			// axis, or everything would be taken out
			if had {
				n.Params["axis"] = old
			} else {
				delete(n.Params, "axis")
			}
			return errors.New("it can't be shrunk by its whole length or more along that axis")
		}
		if ok {
			n.Params["at"] = (lo + hi) / 2
		}
		return nil
	case "at":
		if !ok {
			return errors.New("the stretch holds nothing to cut")
		}
		at = lo + v/s
		if at <= lo || at >= hi {
			return errors.New("the plane must go through the shapes")
		}
		n.Params["at"] = at
	case "amount":
		amount = v / s
	case "length":
		if !ok {
			return errors.New("the stretch holds nothing to lengthen")
		}
		amount = v/s - (hi - lo)
	}
	if amount < 0 && -amount >= hi-lo {
		return errors.New("it can't be shrunk by its whole length or more")
	}
	n.Params["axis"], n.Params["at"] = float64(axis), at
	n.Params["amount"] = math.Max(-maxSpacing, math.Min(maxSpacing, amount))
	return nil
}

// Stretch wraps top-level nodes in a stretch group whose plane is square to
// axis (0 x, 1 y, 2 z) of frame f at at (frame coordinates), lengthening
// them by amount (shortening them, if it is negative). The group's axes are
// f's, so the plane turns with it.
func (d *Document) Stretch(ids []int, f geom.Mat4, axis int, at, amount float64) (*Node, error) {
	if axis < 0 || axis > 2 {
		return nil, errors.New("axis must be x, y or z")
	}
	if math.IsNaN(at+amount) || math.IsInf(at+amount, 0) {
		return nil, errors.New("the plane and the amount must be numbers")
	}
	if math.Abs(amount) > maxSpacing {
		return nil, fmt.Errorf("the amount must be within %g mm", float64(maxSpacing))
	}
	ns := d.FindAll(ids)
	if len(ns) == 0 {
		return nil, errors.New("select a shape to stretch")
	}
	b := BoundsIn(ns, f)
	if b.IsEmpty() || at <= b.Min.Axis(axis)+geom.StretchEps || at >= b.Max.Axis(axis)-geom.StretchEps {
		return nil, errors.New("the plane doesn't cross the selection")
	}
	if amount < 0 && -amount >= b.Size().Axis(axis) {
		return nil, errors.New("it can't be shrunk by its whole length or more")
	}
	g := d.group(ns, ModeStretch)
	// turned to the frame's axes, about the same centre
	rot := geom.Identity()
	for i := 0; i < 3; i++ {
		rot.SetCol(i, f.Col(i).Norm())
	}
	frame := geom.Translate(g.M.Translation()).Mul(rot)
	inv, ok := frame.Inverse()
	if !ok {
		return nil, errors.New("the construction plane is degenerate")
	}
	for _, c := range g.Children {
		c.M = inv.Mul(g.M).Mul(c.M)
	}
	g.M = frame
	local := inv.Point(f.Point(geom.V(0, 0, 0).WithAxis(axis, at))).Axis(axis)
	g.Params = map[string]float64{"axis": float64(axis), "at": local, "amount": amount}
	return g, nil
}

// stretchMap is where the stretch takes a point on its axis.
func stretchMap(c, at, amount float64) float64 {
	if amount >= 0 {
		if c > at {
			return c + amount
		}
		return c
	}
	w := -amount
	switch {
	case c >= at+w/2:
		return c - w
	case c > at-w/2:
		return at - w/2
	}
	return c
}

// bakeStretch is stretch group n as a plain bundle holding what it shows
// (n is left as it was): see ModeStretch.
func (d *Document) bakeStretch(n *Node) *Node {
	g := n.Clone()
	g.GroupMode, g.Params, g.Children = ModeBundle, nil, nil
	axis, at, amount := n.stretchPlane()
	for _, c := range n.Children {
		c = c.Clone()
		if amount == 0 {
			g.Children = append(g.Children, c)
			continue
		}
		g.Children = append(g.Children, d.bakeInto(c, geom.Identity(), axis, at, amount)...)
	}
	return g
}

// bakeInto is what c (a copy, changed in place) becomes stretched: itself,
// moved, nothing (all in the slab taken out), its parts baked (a group
// whose boolean the stretch commutes with), or a mesh of it stretched. P
// is c's parent's frame in the stretch's.
func (d *Document) bakeInto(c *Node, P geom.Mat4, axis int, at, amount float64) []*Node {
	lo, hi, ok := c.extentIn(P, axis)
	Pinv, inv := P.Inverse()
	if !ok || !inv {
		return []*Node{c}
	}
	eps := geom.StretchEps
	shift := func(by float64) []*Node {
		c.M = Pinv.Mul(geom.Translate(geom.V(0, 0, 0).WithAxis(axis, by))).Mul(P).Mul(c.M)
		return []*Node{c}
	}
	if amount > 0 {
		switch {
		case hi < at-eps:
			return []*Node{c}
		case lo > at+eps:
			return shift(amount)
		}
	} else {
		p1, p2 := at+amount/2, at-amount/2
		switch {
		case hi <= p1+eps:
			return []*Node{c}
		case lo >= p2-eps:
			return shift(amount)
		case lo >= p1-eps && hi <= p2+eps:
			return nil
		}
	}
	// the plane crosses it
	switch {
	case c.IsStretch():
		return d.bakeInto(d.bakeStretch(c), P, axis, at, amount)
	case c.IsPattern():
		e := c.expanded()
		for _, k := range e.Children[len(c.Children):] {
			d.renumber(k) // the copies were sharing their originals' ids
		}
		return d.bakeInto(e, P, axis, at, amount)
	case c.IsGroup() && !c.IsHull():
		Q := P.Mul(c.M)
		hadSolid := false
		for _, ch := range c.Children {
			hadSolid = hadSolid || !ch.Hole
		}
		var kids []*Node
		for _, ch := range c.Children {
			kids = append(kids, d.bakeInto(ch, Q, axis, at, amount)...)
		}
		solid := false
		for _, k := range kids {
			solid = solid || !k.Hole
		}
		// a merged group with only its holes left would show them as solid
		if len(kids) == 0 || (hadSolid && !solid && !c.IsBundle()) {
			return nil
		}
		c.Children = kids
		if c.Assembly != "" { // no longer the assembly it was placed as
			c.Assembly, c.Params = "", nil
		}
		if qi, ok := Q.Inverse(); ok {
			for i, nt := range c.Notes {
				q := Q.Point(nt.P)
				c.Notes[i].P = qi.Point(q.WithAxis(axis, stretchMap(q.Axis(axis), at, amount)))
			}
		}
		return []*Node{c}
	}
	// a shape, or a hull (whose result the stretch doesn't commute with):
	// a mesh of what it is, stretched
	var src *geom.Mesh
	if c.IsGroup() {
		src = c.UnionMesh()
	} else {
		src = c.LeafMesh()
	}
	Q := P.Mul(c.M)
	m := geom.Stretch(src.Transform(Q), axis, at, amount)
	if len(m.Tris) == 0 {
		return nil
	}
	b := m.Bounds()
	ctr := b.Center()
	name := c.Name
	if name == "" {
		name = c.Label()
	}
	mn := &Node{ID: d.newID(), Kind: KindMesh, Name: name, Size: b.Size(), M: Pinv.Mul(geom.Translate(ctr)),
		MeshData: EncodeMesh(m.Transform(geom.Translate(ctr.Neg()))), Hole: c.Hole, Color: c.Color,
		Transparent: c.Transparent, Hidden: c.Hidden, Locked: c.Locked}
	for _, nt := range c.Notes {
		q := Q.Point(c.NotePoint(nt.P))
		q = q.WithAxis(axis, stretchMap(q.Axis(axis), at, amount))
		mn.Notes = append(mn.Notes, Note{P: mn.NoteParam(q.Sub(ctr)), Text: nt.Text})
	}
	return []*Node{mn}
}
