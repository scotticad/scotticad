package doc

import (
	"errors"
	"fmt"
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// What Split keeps: both halves, or only the one above (the + side of the
// axis) or below.
const (
	KeepBoth  = "both"
	KeepAbove = "above"
	KeepBelow = "below"
)

// Split cuts top-level nodes with a plane square to axis (0 x, 1 y, 2 z)
// of frame f, at position at along that axis (frame coordinates). Each
// node crossed by the plane is replaced by its halves; each half is a
// union group of a copy of the node and a hole box covering the other
// side, so ungrouping a half gives the whole node back. gap moves the
// half above (on the + side) that far along the axis, as a lid lifts off;
// the half below stays where it was. Nodes the plane misses are left
// alone; it is an error if it misses them all.
func (d *Document) Split(ids []int, f geom.Mat4, axis int, at float64, keep string, gap float64) ([]*Node, error) {
	if axis < 0 || axis > 2 {
		return nil, errors.New("axis must be x, y or z")
	}
	switch keep {
	case "":
		keep = KeepBoth
	case KeepBoth, KeepAbove, KeepBelow:
	default:
		return nil, fmt.Errorf("keep must be %q, %q or %q", KeepBoth, KeepAbove, KeepBelow)
	}
	ns := d.FindAll(ids)
	if len(ns) == 0 {
		return nil, errors.New("nothing to split")
	}
	var out []*Node
	for _, n := range ns {
		b := BoundsIn([]*Node{n}, f)
		const eps = 1e-6
		if b.IsEmpty() || at <= b.Min.Axis(axis)+eps || at >= b.Max.Axis(axis)-eps {
			continue // the plane misses it
		}
		pos := d.indexOf(n)
		var halves []*Node
		for _, side := range []string{KeepBelow, KeepAbove} {
			if keep != KeepBoth && keep != side {
				continue
			}
			h := d.half(n, b, f, axis, at, side)
			if h == nil {
				continue
			}
			if side == KeepAbove {
				Move([]*Node{h}, f.Dir(geom.V(0, 0, 0).WithAxis(axis, gap)))
			}
			halves = append(halves, h)
		}
		// the halves take the node's place in the list
		d.Remove([]int{n.ID})
		pos = min(pos, len(d.Nodes))
		d.Nodes = append(d.Nodes[:pos], append(halves, d.Nodes[pos:]...)...)
		out = append(out, halves...)
	}
	if len(out) == 0 {
		return nil, errors.New("the cutting plane doesn't cross the selection")
	}
	return out, nil
}

func (d *Document) indexOf(n *Node) int {
	for i, m := range d.Nodes {
		if m == n {
			return i
		}
	}
	return len(d.Nodes)
}

// half makes one half of n (placed by its own M, as a top-level node is):
// a union group of a copy of n and a hole box over the other side of the
// plane. A bundle is split part by part instead, into a bundle of halves:
// in a union group its hole parts would cut its other parts, which they
// never do in a bundle. b is n's bounds in frame f. The half isn't added
// to the document.
func (d *Document) half(n *Node, b geom.Box, f geom.Mat4, axis int, at float64, side string) *Node {
	if n.IsBundle() {
		return d.halfBundle(n, f, axis, at, side)
	}
	c := n.Clone()
	d.renumber(c)
	c.Hole, c.Transparent, c.Hidden = false, false, false // the half has them
	c.Notes = notesOnSide(n, f, axis, at, side)

	// the box covers the other side, past n's bounds
	s := b.Size()
	m := 1 + 0.05*math.Max(s.X, math.Max(s.Y, s.Z))
	lo, hi := b.Min.Sub(geom.V(m, m, m)), b.Max.Add(geom.V(m, m, m))
	if side == KeepBelow {
		lo = lo.WithAxis(axis, at)
	} else {
		hi = hi.WithAxis(axis, at)
	}
	k, _ := shapes.Lookup("box")
	box := &Node{ID: d.newID(), Kind: "box", Name: "Split", Params: k.Defaults().Num, Size: hi.Sub(lo), Color: k.Color, Hole: true,
		M: f.Mul(geom.Translate(lo.Add(hi).Mul(0.5)))}

	// grouped about the node's centre, as Group would
	ctr := Bounds([]*Node{n}).Center()
	if math.IsInf(ctr.X, 0) || math.IsNaN(ctr.X) {
		ctr = geom.Vec3{}
	}
	inv := geom.Translate(ctr.Neg())
	c.M, box.M = inv.Mul(c.M), inv.Mul(box.M)
	return &Node{ID: d.newID(), Kind: KindGroup, GroupMode: ModeUnion, M: geom.Translate(ctr), Children: []*Node{c, box},
		Name: n.Name, Color: n.Color, Multicolor: n.Multicolor, Hole: n.Hole, Transparent: n.Transparent, Hidden: n.Hidden}
}

// halfBundle is one side of bundle n: its parts on that side, those the
// plane crosses split, in a bundle like n (nil if none is on that side).
func (d *Document) halfBundle(n *Node, f geom.Mat4, axis int, at float64, side string) *Node {
	inv, ok := n.M.Inverse()
	if !ok {
		return nil
	}
	g := n.Clone()
	g.Children, g.Assembly, g.Params = nil, "", nil // no longer the assembly it was placed as
	g.Notes = notesOnSide(n, f, axis, at, side)
	d.renumber(g)
	const eps = 1e-6
	for _, ch := range n.Children {
		w := ch.Clone()
		w.M = n.M.Mul(ch.M) // placed in the world, as a top-level node
		b := BoundsIn([]*Node{w}, f)
		if b.IsEmpty() {
			continue
		}
		var piece *Node
		switch {
		case at > b.Min.Axis(axis)+eps && at < b.Max.Axis(axis)-eps:
			piece = d.half(w, b, f, axis, at, side)
		case (side == KeepBelow) == (b.Max.Axis(axis) <= at+eps):
			piece = w // wholly on this side
			d.renumber(piece)
		}
		if piece == nil {
			continue
		}
		piece.M = inv.Mul(piece.M) // back into the bundle
		g.Children = append(g.Children, piece)
	}
	if len(g.Children) == 0 {
		return nil
	}
	return g
}

// notesOnSide are n's notes on one side of the plane: each note goes with
// the half it is on (not both, one floating where the other half was).
func notesOnSide(n *Node, f geom.Mat4, axis int, at float64, side string) []Note {
	inv, ok := f.Inverse()
	if !ok {
		return append([]Note(nil), n.Notes...)
	}
	var keep []Note
	for _, nt := range n.Notes {
		p := inv.Point(n.M.Point(n.NotePoint(nt.P))).Axis(axis)
		if (side == KeepBelow) == (p <= at) {
			keep = append(keep, nt)
		}
	}
	return keep
}
