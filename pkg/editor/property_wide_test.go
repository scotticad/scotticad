package editor

// A wider property test than TestPropertyEditSequences: random sequences
// drawn from nearly every editing operation (appearance, visibility,
// locking, parameters, text, notes, the clipboard, workplanes, settings,
// live drags), checking after every step that
//
//   - the design saves, loads back and saves the same, with unique ids
//     and finite bounds (ptCheck);
//   - no operation but locking, naming, hiding, notes and undo/redo
//     changes a locked shape;
//   - any change to the design advances Version (kept overlays use it to
//     notice the design changed under them);
//
// and at the end that undoing everything gives the empty design and
// redoing it all gives the final one again.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

var wideKinds = append(append([]string{}, ptKinds...), "text", "gear", "thread", "tube", "spurgear", "plate", "hinge", "extrudedbox", "star3d")

// lockedState is each locked top-level shape as JSON, without what may
// change on a locked shape (its name, visibility, lock and notes).
func lockedState(e *Editor) map[int]string {
	out := map[int]string{}
	for _, n := range e.Doc.Nodes {
		if !n.Locked {
			continue
		}
		c := n.Clone()
		c.Name, c.Hidden, c.Locked = "", false, false
		c.Walk(func(x *doc.Node) { x.Notes = nil })
		b, _ := json.Marshal(c)
		out[n.ID] = string(b)
	}
	return out
}

// anyNode is a random node at any depth (nil if there are none).
func anyNode(e *Editor, r *rand.Rand) *doc.Node {
	var all []*doc.Node
	for _, n := range e.Doc.Nodes {
		n.Walk(func(x *doc.Node) { all = append(all, x) })
	}
	if len(all) == 0 {
		return nil
	}
	return all[r.Intn(len(all))]
}

// randParam is a random value for one of n's parameters: mostly within
// its range, sometimes outside it.
func randParam(n *doc.Node, r *rand.Rand) (string, float64, bool) {
	kind := n.Kind
	if n.IsGroup() && n.Assembly != "" {
		kind = n.Assembly
	}
	k, ok := shapes.Lookup(kind)
	if !ok || len(k.Params) == 0 {
		return "", 0, false
	}
	p := k.Params[r.Intn(len(k.Params))]
	if p.Kind == shapes.TextParam || p.Kind == shapes.FontParam {
		return "", 0, false
	}
	v := p.Min + r.Float64()*(p.Max-p.Min)
	switch r.Intn(8) {
	case 0:
		v = p.Min - 1 - r.Float64()*10
	case 1:
		v = p.Max + r.Float64()*100
	case 2:
		v = 0
	}
	return p.Key, v, true
}

func wideStep(e *Editor, r *rand.Rand) (desc string, lockSafe bool) {
	ids := doc.IDs(e.Doc.Nodes)
	if len(ids) > 0 && r.Intn(4) != 0 {
		var sel []int
		for _, id := range ids {
			if r.Intn(3) == 0 {
				sel = append(sel, id)
			}
		}
		if len(sel) == 0 {
			sel = []int{ids[r.Intn(len(ids))]}
		}
		e.Select(sel...)
	}
	sel := fmt.Sprint(e.Sel)
	pickID := func() int {
		if len(ids) == 0 {
			return 0
		}
		return ids[r.Intn(len(ids))]
	}
	switch op := r.Intn(46); op {
	case 0, 1, 2:
		k := wideKinds[r.Intn(len(wideKinds))]
		p, hole := ptRandVec(r, 40), r.Intn(5) == 0
		_, err := e.AddShape(k, p, hole)
		return fmt.Sprintf("AddShape(%q, %v, %v) = %v", k, p, hole, err), true
	case 3:
		d := ptRandVec(r, 20)
		e.Move(nil, d)
		return fmt.Sprintf("Move(%v, %v)", sel, d), true
	case 4:
		ax, deg := r.Intn(3), []float64{90, 45, -30, 180, r.Float64() * 360}[r.Intn(5)]
		e.Rotate(nil, ax, deg)
		return fmt.Sprintf("Rotate(%v, %d, %g)", sel, ax, deg), true
	case 5:
		s := geom.V(0.5+r.Float64()*60, 0.5+r.Float64()*60, 0.5+r.Float64()*60)
		err := e.Resize(nil, s)
		return fmt.Sprintf("Resize(%v, %v) = %v", sel, s, err), true
	case 6, 7:
		mode := []string{doc.ModeUnion, doc.ModeBundle, doc.ModeIntersect}[r.Intn(3)]
		_, err := e.Group(mode)
		return fmt.Sprintf("Group(%v, %q) = %v", sel, mode, err), true
	case 8:
		e.Ungroup()
		return fmt.Sprintf("Ungroup(%v)", sel), true
	case 9:
		h := r.Intn(2) == 0
		e.SetHole(h)
		return fmt.Sprintf("SetHole(%v, %v)", sel, h), true
	case 10:
		e.Duplicate()
		return fmt.Sprintf("Duplicate(%v)", sel), true
	case 11:
		e.Delete(nil)
		return fmt.Sprintf("Delete(%v)", sel), true
	case 12:
		b := e.SelectionBoundsIn()
		ax := r.Intn(3)
		at := 0.0
		if !b.IsEmpty() {
			at = b.Min.Axis(ax) + r.Float64()*b.Size().Axis(ax)
		}
		keep := []string{doc.KeepBoth, doc.KeepAbove, doc.KeepBelow}[r.Intn(3)]
		_, err := e.Split(nil, ax, at, keep, []float64{0, 3}[r.Intn(2)])
		return fmt.Sprintf("Split(%v, %d, %g, %q) = %v", sel, ax, at, keep, err), true
	case 13:
		ax := r.Intn(3)
		e.Mirror(nil, ax)
		return fmt.Sprintf("Mirror(%v, %d)", sel, ax), true
	case 14, 15:
		return fmt.Sprintf("Undo() = %v", e.Undo()), false
	case 16:
		return fmt.Sprintf("Redo() = %v", e.Redo()), false
	case 17:
		c := fmt.Sprintf("#%06x", r.Intn(1<<24))
		err := e.SetColor(c)
		return fmt.Sprintf("SetColor(%v, %s) = %v", sel, c, err), true
	case 18:
		on := r.Intn(2) == 0
		e.SetMulticolor(on)
		return fmt.Sprintf("SetMulticolor(%v, %v)", sel, on), true
	case 19:
		mode := []string{doc.ModeUnion, doc.ModeBundle, doc.ModeIntersect}[r.Intn(3)]
		err := e.SetGroupMode(mode)
		return fmt.Sprintf("SetGroupMode(%v, %q) = %v", sel, mode, err), true
	case 20:
		on := r.Intn(2) == 0
		e.SetTransparent(on)
		return fmt.Sprintf("SetTransparent(%v, %v)", sel, on), true
	case 21:
		on := r.Intn(3) != 0
		e.SetLocked(on)
		return fmt.Sprintf("SetLocked(%v, %v)", sel, on), false
	case 22:
		e.SetName(fmt.Sprintf("n%d", r.Intn(100)))
		return fmt.Sprintf("SetName(%v)", sel), true
	case 23:
		e.HideSelected()
		return fmt.Sprintf("HideSelected(%v)", sel), true
	case 24:
		e.ShowAll()
		return "ShowAll()", true
	case 25:
		id := pickID()
		e.ToggleHidden(id)
		return fmt.Sprintf("ToggleHidden(%d)", id), true
	case 26:
		ax, st := r.Intn(3), float64(r.Intn(21)-10)
		e.Nudge(ax, st)
		return fmt.Sprintf("Nudge(%v, %d, %g)", sel, ax, st), true
	case 27:
		ax, wh := r.Intn(3), r.Intn(3)-1
		key := 0
		if r.Intn(2) == 0 && len(e.Sel) > 0 {
			key = e.Sel[r.Intn(len(e.Sel))]
		}
		e.AlignTo(nil, ax, wh, key)
		return fmt.Sprintf("AlignTo(%v, %d, %d, %d)", sel, ax, wh, key), true
	case 28:
		e.Drop(nil)
		return fmt.Sprintf("Drop(%v)", sel), true
	case 29:
		n := ptRandVec(r, 1)
		if r.Intn(2) == 0 {
			n = geom.V(0, 0, 0).WithAxis(r.Intn(3), float64(2*r.Intn(2)-1))
		}
		err := e.LayFlat(nil, n)
		return fmt.Sprintf("LayFlat(%v, %v) = %v", sel, n, err), true
	case 30, 31:
		n := anyNode(e, r)
		if n == nil {
			return "SetParam (no nodes)", true
		}
		key, v, ok := randParam(n, r)
		if !ok {
			return fmt.Sprintf("SetParam(%d %s: none)", n.ID, n.Kind), true
		}
		err := e.SetParam(n.ID, key, v)
		return fmt.Sprintf("SetParam(%d %s, %q, %g) = %v", n.ID, n.Kind, key, v, err), true
	case 32:
		n := anyNode(e, r)
		if n == nil || n.Kind != "text" {
			return "SetText (no text)", true
		}
		t := []string{"", "A", "Hello", "  ", "wæ✓", strings.Repeat("x", 40)}[r.Intn(6)]
		err := e.SetText(n.ID, t, "")
		return fmt.Sprintf("SetText(%d, %q) = %v", n.ID, t, err), true
	case 33:
		ok := e.Copy()
		return fmt.Sprintf("Copy(%v) = %v", sel, ok), true
	case 34:
		ok := e.Cut()
		return fmt.Sprintf("Cut(%v) = %v", sel, ok), true
	case 35:
		in := r.Intn(2) == 0
		got, err := e.Paste(in)
		return fmt.Sprintf("Paste(%v) = %v, %v", in, got, err), true
	case 36:
		id := pickID()
		if id == 0 {
			return "AddNote (no shapes)", true
		}
		p := e.Doc.Find(id).WorldBounds().Center()
		if math.IsNaN(p.X) || math.IsInf(p.X, 0) {
			p = geom.Vec3{}
		}
		_, err := e.AddNote(id, p, "note")
		return fmt.Sprintf("AddNote(%d) = %v", id, err), false
	case 37:
		ns := e.Notes()
		if len(ns) == 0 {
			return "DeleteNote (none)", false
		}
		nr := ns[r.Intn(len(ns))]
		var err error
		if r.Intn(2) == 0 {
			err = e.DeleteNote(nr.Node, nr.Index)
		} else {
			err = e.SetNoteText(nr.Node, nr.Index, "edited")
		}
		return fmt.Sprintf("Delete/SetNoteText(%d, %d) = %v", nr.Node, nr.Index, err), false
	case 38:
		id := 0
		if r.Intn(3) != 0 {
			id = pickID()
		}
		err := e.AttachWorkplane(id)
		return fmt.Sprintf("AttachWorkplane(%d) = %v", id, err), true
	case 39:
		u := []string{"mm", "in"}[r.Intn(2)]
		err := e.SetUnits(u)
		e.SetSnap([]float64{0, 0.5, 1, 2.54}[r.Intn(4)])
		return fmt.Sprintf("SetUnits(%s)+SetSnap = %v", u, err), true
	case 40: // a parameter slider drag
		n := anyNode(e, r)
		if n == nil {
			return "SliderDrag (no nodes)", true
		}
		key, v, ok := randParam(n, r)
		if !ok {
			return "SliderDrag (no params)", true
		}
		e.BeginEdit()
		for k := 0; k < 3; k++ {
			e.SetParamLive(n.ID, key, v*(1+0.1*float64(k)))
		}
		if r.Intn(4) == 0 {
			e.CancelEdit()
			return fmt.Sprintf("SliderDrag(%d, %q) cancelled", n.ID, key), true
		}
		e.EndEdit()
		return fmt.Sprintf("SliderDrag(%d, %q, ~%g)", n.ID, key, v), true
	case 41: // a colour picker drag
		e.BeginEdit()
		for k := 0; k < 3; k++ {
			e.SetColorLive(fmt.Sprintf("#%06x", r.Intn(1<<24)))
		}
		e.EndEdit()
		return fmt.Sprintf("ColorDrag(%v)", sel), true
	case 42: // a resize-handle drag
		e.BeginEdit()
		ns := e.Editable()
		b := doc.BoundsIn(ns, e.Work())
		if !b.IsEmpty() {
			for k := 1; k <= 3; k++ {
				nb := b
				nb.Max = nb.Max.Add(ptRandVec(r, 5).Mul(float64(k)))
				if nb.Max.X > nb.Min.X && nb.Max.Y > nb.Min.Y && nb.Max.Z > nb.Min.Z {
					e.SetBoundsLiveIn(ns, nb)
				}
			}
		}
		e.EndEdit()
		return fmt.Sprintf("ResizeDrag(%v)", sel), true
	case 44: // a shape with no geometry: two shapes that don't meet, intersected
		a, _ := e.AddShape("box", geom.V(-200, 0, 0), false)
		b, _ := e.AddShape("sphere", geom.V(200, 0, 0), false)
		e.Select(a, b)
		g, err := e.Group(doc.ModeIntersect)
		return fmt.Sprintf("EmptyGroup() = %d, %v", g, err), true
	default: // a rotate drag about the selection's centre, in the workplane
		e.BeginEdit()
		ns := e.Editable()
		b := doc.BoundsIn(ns, e.Work())
		if !b.IsEmpty() {
			c := e.Work().Point(b.Center())
			ax := r.Intn(3)
			for k := 0; k < 3; k++ {
				e.RotateIn(ns, ax, 10, c)
			}
		}
		if r.Intn(4) == 0 {
			e.CancelEdit()
			return fmt.Sprintf("RotateDrag(%v) cancelled", sel), true
		}
		e.EndEdit()
		return fmt.Sprintf("RotateDrag(%v)", sel), true
	}
}

func TestPropertyWideEditSequences(t *testing.T) {
	seeds, steps := int64(25), 150
	if testing.Short() {
		seeds, steps = 5, 80
	}
	empty := ptDocBytes(t, New())
	for seed := int64(1); seed <= seeds; seed++ {
		r := rand.New(rand.NewSource(seed * 7919))
		e := New()
		var clip string
		e.SetClipboard = func(s string) { clip = s }
		e.GetClipboard = func() string { return clip }
		var log []string
		fail := func(f string, args ...any) {
			if len(log) > 40 {
				log = append([]string{"..."}, log[len(log)-40:]...)
			}
			t.Fatalf("seed %d: %s\nlast steps:\n  %s", seed, fmt.Sprintf(f, args...), strings.Join(log, "\n  "))
		}
		for i := 0; i < steps; i++ {
			beforeDoc := ptDocBytes(t, e)
			beforeLocked := lockedState(e)
			ver := e.Version
			var desc string
			var lockSafe bool
			func() {
				defer func() {
					if p := recover(); p != nil {
						log = append(log, desc)
						fail("panic: %v\n%s", p, debug.Stack())
					}
				}()
				desc, lockSafe = wideStep(e, r)
			}()
			log = append(log, desc)
			if bad := ptCheck(e); bad != "" {
				fail("step %d: %s", i, bad)
			}
			if e.Workplane != nil && !finite(*e.Workplane) {
				fail("step %d: workplane %v", i, *e.Workplane)
			}
			if r := e.Ruler; r != nil && !ptFiniteBox(geom.Box{Min: r.Origin, Max: r.Origin}) {
				fail("step %d: ruler at %v", i, r.Origin)
			}
			if b := e.SelectionBoundsIn(); !b.IsEmpty() && !ptFiniteBox(b) {
				fail("step %d: selection bounds in the workplane %v", i, b)
			}
			after := ptDocBytes(t, e)

			if !bytes.Equal(after, beforeDoc) && e.Version == ver {
				fail("step %d changed the design without advancing Version", i)
			}
			if lockSafe {
				for id, s := range lockedState(e) {
					if old, ok := beforeLocked[id]; ok && old != s {
						fail("step %d changed locked shape %d:\n%s\nto\n%s", i, id, old, s)
					}
				}
				for id := range beforeLocked {
					if e.Doc.Find(id) == nil {
						fail("step %d removed locked shape %d", i, id)
					}
				}
			}
		}
		final := ptDocBytes(t, e)
		depth := 0
		for e.Undo() {
			depth++
		}
		if got := ptDocBytes(t, e); !bytes.Equal(got, empty) {
			fail("undoing all %d steps doesn't give the empty design:\n%s", depth, got)
		}
		for k := 0; k < depth; k++ {
			if !e.Redo() {
				fail("%d undos but only %d redos", depth, k)
			}
		}
		if got := ptDocBytes(t, e); !bytes.Equal(got, final) {
			fail("redoing all %d steps gives a different design", depth)
		}
	}
}
