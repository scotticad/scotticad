package editor

// Property-based test: random edit sequences on an Editor never panic,
// always leave a savable design with unique ids and finite bounds, and
// undo/redo walk the history exactly.

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
)

var ptKinds = []string{"box", "cylinder", "sphere", "cone", "pyramid", "roof", "wedge", "tube", "torus", "halfsphere", "polygon", "icosahedron", "star", "ring", "heart", "diamond", "projectbox"}

func ptRandVec(r *rand.Rand, s float64) geom.Vec3 {
	return geom.V((r.Float64()*2-1)*s, (r.Float64()*2-1)*s, (r.Float64()*2-1)*s)
}

func ptFiniteBox(b geom.Box) bool {
	for _, v := range []float64{b.Min.X, b.Min.Y, b.Min.Z, b.Max.X, b.Max.Y, b.Max.Z} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func ptDocBytes(t *testing.T, e *Editor) []byte {
	b, err := e.Doc.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// ptCheck verifies the invariants after a step; it returns a problem or "".
func ptCheck(e *Editor) string {
	b, err := e.Doc.Marshal()
	if err != nil {
		return fmt.Sprintf("design doesn't marshal: %v", err)
	}
	if d2, err := doc.Unmarshal(b); err != nil {
		return fmt.Sprintf("design doesn't load back: %v", err)
	} else if b2, _ := d2.Marshal(); !bytes.Equal(b, b2) {
		return "saving the loaded design changes it"
	}
	seen := map[int]bool{}
	var bad string
	for _, n := range e.Doc.Nodes {
		n.Walk(func(x *doc.Node) {
			if bad != "" {
				return
			}
			if seen[x.ID] {
				bad = fmt.Sprintf("id %d used twice", x.ID)
			}
			if x.ID <= 0 || x.ID >= e.Doc.NextID {
				bad = fmt.Sprintf("id %d outside 1..NextID %d", x.ID, e.Doc.NextID)
			}
			seen[x.ID] = true
		})
		if bad != "" {
			return bad
		}
		if b := n.WorldBounds(); !b.IsEmpty() && !ptFiniteBox(b) {
			return fmt.Sprintf("node %d (%s) has bounds %v", n.ID, n.Label(), b)
		}
	}
	for _, id := range e.Sel {
		if e.Doc.Find(id) == nil {
			return fmt.Sprintf("selection holds %d, not a top-level node", id)
		}
	}
	return ""
}

// ptStep performs one random edit and returns its description.
func ptStep(e *Editor, r *rand.Rand) string {
	ids := doc.IDs(e.Doc.Nodes)
	// select a random subset first (most edits act on the selection)
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
	switch op := r.Intn(17); op {
	case 0, 1:
		k := ptKinds[r.Intn(len(ptKinds))]
		p, hole := ptRandVec(r, 40), r.Intn(4) == 0
		e.AddShape(k, p, hole)
		return fmt.Sprintf("AddShape(%q, %v, %v)", k, p, hole)
	case 2:
		d := ptRandVec(r, 20)
		e.Move(nil, d)
		return fmt.Sprintf("Move(%v, %v)", sel, d)
	case 3:
		ax, deg := r.Intn(3), []float64{90, 45, -30, 180, r.Float64() * 360}[r.Intn(5)]
		e.Rotate(nil, ax, deg)
		return fmt.Sprintf("Rotate(%v, %d, %g)", sel, ax, deg)
	case 4:
		s := geom.V(0.5+r.Float64()*60, 0.5+r.Float64()*60, 0.5+r.Float64()*60)
		if r.Intn(3) == 0 {
			s = s.WithAxis(r.Intn(3), 0)
		}
		err := e.Resize(nil, s)
		return fmt.Sprintf("Resize(%v, %v) = %v", sel, s, err)
	case 5, 6:
		mode := []string{doc.ModeUnion, doc.ModeBundle, doc.ModeIntersect}[r.Intn(3)]
		_, err := e.Group(mode)
		return fmt.Sprintf("Group(%v, %q) = %v", sel, mode, err)
	case 7:
		e.Ungroup()
		return fmt.Sprintf("Ungroup(%v)", sel)
	case 8:
		h := r.Intn(2) == 0
		e.SetHole(h)
		return fmt.Sprintf("SetHole(%v, %v)", sel, h)
	case 9:
		e.Duplicate()
		return fmt.Sprintf("Duplicate(%v)", sel)
	case 10:
		e.Delete(nil)
		return fmt.Sprintf("Delete(%v)", sel)
	case 11:
		b := e.SelectionBounds()
		ax := r.Intn(3)
		at := 0.0
		if !b.IsEmpty() {
			at = b.Min.Axis(ax) + r.Float64()*b.Size().Axis(ax)
		}
		keep := []string{doc.KeepBoth, doc.KeepAbove, doc.KeepBelow}[r.Intn(3)]
		gap := []float64{0, 0, 5}[r.Intn(3)]
		_, err := e.Split(nil, ax, at, keep, gap)
		return fmt.Sprintf("Split(%v, %d, %g, %q, %g) = %v", sel, ax, at, keep, gap, err)
	case 12:
		ax := r.Intn(3)
		e.Mirror(nil, ax)
		return fmt.Sprintf("Mirror(%v, %d)", sel, ax)
	case 13, 14:
		ok := e.Undo()
		return fmt.Sprintf("Undo() = %v", ok)
	case 15:
		ok := e.Redo()
		return fmt.Sprintf("Redo() = %v", ok)
	default: // a drag: live moves bracketed by Begin/EndEdit
		e.BeginEdit()
		ns := e.Editable()
		for k := 0; k < 3; k++ {
			e.MoveLive(ns, ptRandVec(r, 3))
		}
		if r.Intn(4) == 0 {
			e.CancelEdit()
			return fmt.Sprintf("Drag(%v) cancelled", sel)
		}
		e.EndEdit()
		return fmt.Sprintf("Drag(%v)", sel)
	}
}

func TestPropertyEditSequences(t *testing.T) {
	const seeds, steps = 30, 120
	empty := ptDocBytes(t, New())
	for seed := int64(1); seed <= seeds; seed++ {
		r := rand.New(rand.NewSource(seed))
		e := New()
		var log []string
		fail := func(f string, args ...any) {
			t.Fatalf("seed %d after %d steps: %s\nsteps:\n  %s", seed, len(log), fmt.Sprintf(f, args...), strings.Join(log, "\n  "))
		}
		for i := 0; i < steps; i++ {
			var desc string
			func() {
				defer func() {
					if p := recover(); p != nil {
						log = append(log, desc)
						fail("panic: %v\n%s", p, debug.Stack())
					}
				}()
				desc = ptStep(e, r)
			}()
			log = append(log, desc)
			if bad := ptCheck(e); bad != "" {
				fail("%s", bad)
			}
		}
		final := ptDocBytes(t, e)
		// steps undone by the sequence and not overwritten can still be
		// redone past the final state
		pending := len(e.Hist.States()) - e.Hist.Depth()
		depth := 0
		for e.Undo() {
			depth++
			if bad := ptCheck(e); bad != "" {
				fail("after %d undos: %s", depth, bad)
			}
		}
		if got := ptDocBytes(t, e); !bytes.Equal(got, empty) {
			fail("undoing all %d steps gives\n%s\nnot the empty design\n%s", depth, got, empty)
		}
		for k := 0; k < depth; k++ {
			if !e.Redo() {
				fail("%d undos but only %d redos", depth, k)
			}
		}
		if got := ptDocBytes(t, e); !bytes.Equal(got, final) {
			fail("redoing all %d undone steps gives a different design", depth)
		}
		more := 0
		for e.Redo() {
			more++
		}
		if more != pending {
			fail("%d further redos, %d were pending", more, pending)
		}
	}
}
