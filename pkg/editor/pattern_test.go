package editor

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
)

// Pattern wraps the selection in a pattern group in one undo step, with
// the settings given; its settings are then set like a shape's parameters.
func TestPattern(t *testing.T) {
	e := New()
	if _, err := e.Pattern(nil); err == nil {
		t.Error("a pattern of nothing")
	}
	a, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	depth := e.Hist.Depth()
	g, err := e.Pattern(map[string]float64{"columns": 4, "dx": 30, "rows": 2, "dy": 40})
	if err != nil {
		t.Fatal(err)
	}
	if e.Hist.Depth() != depth+1 || len(e.Sel) != 1 || e.Sel[0] != g {
		t.Errorf("%d undo steps, selection %v", e.Hist.Depth()-depth, e.Sel)
	}
	n := e.Doc.Find(g)
	if n == nil || !n.IsPattern() || len(n.Children) != 1 || n.Children[0].ID != a || len(n.Parts()) != 8 {
		t.Fatalf("not a pattern of the box: %+v", n)
	}
	if got, want := n.WorldBounds(), (geom.Box{Min: geom.V(-10, -10, 0), Max: geom.V(100, 50, 20)}); got.Min.Sub(want.Min).Len() > 1e-9 || got.Max.Sub(want.Max).Len() > 1e-9 {
		t.Errorf("bounds %v, want %v", got, want)
	}
	// a setting: one undo step, kept within its range, whole if it counts
	if err := e.SetParam(g, "columns", 6.4); err != nil {
		t.Fatal(err)
	}
	if n = e.Doc.Find(g); n.Params["columns"] != 6 || len(n.Parts()) != 12 || e.Hist.Depth() != depth+2 {
		t.Errorf("columns %v, %d parts, %d undo steps", n.Params["columns"], len(n.Parts()), e.Hist.Depth()-depth)
	}
	e.SetParam(g, "rows", 900)
	if n = e.Doc.Find(g); n.Params["rows"] != 50 {
		t.Errorf("rows %v, want 50", n.Params["rows"])
	}
	if err := e.SetParam(g, "sides", 3); err == nil {
		t.Error("a setting a pattern hasn't was accepted")
	}
	// the shape in it changed: every copy follows
	e.SetParam(g, "rows", 1)
	if err := e.SetParam(a, "height", 5); err != nil {
		t.Fatal(err)
	}
	if v := e.Doc.Find(g).WorldMesh(false).Volume(); math.Abs(v-6*20*20*5) > 1e-6 {
		t.Errorf("six boxes 5 mm high: volume %v", v)
	}
	// a slider drag is one step, and can be cancelled
	depth = e.Hist.Depth()
	e.BeginEdit()
	for _, v := range []float64{2, 3, 4} {
		e.SetParamLive(g, "columns", v)
	}
	e.EndEdit()
	if e.Doc.Find(g).Params["columns"] != 4 || e.Hist.Depth() != depth+1 {
		t.Errorf("after a drag: columns %v, %d undo steps", e.Doc.Find(g).Params["columns"], e.Hist.Depth()-depth)
	}
	// locked, its settings stay
	e.Select(g)
	e.SetLocked(true)
	if err := e.SetParam(g, "columns", 2); err == nil {
		t.Error("a locked pattern's setting was changed")
	}
	if _, err := e.Pattern(nil); err == nil {
		t.Error("a locked pattern was put in a pattern")
	}
	e.SetLocked(false)
	// ungrouped: four boxes, selected; undone: the pattern again
	ids := e.Ungroup()
	if len(ids) != 4 || len(e.Doc.Nodes) != 4 || len(e.Sel) != 4 {
		t.Fatalf("ungrouped to %d, %d nodes, %d selected", len(ids), len(e.Doc.Nodes), len(e.Sel))
	}
	if !e.Undo() || len(e.Doc.Nodes) != 1 || !e.Doc.Nodes[0].IsPattern() {
		t.Fatal("undo didn't bring the pattern back")
	}
	// made another kind of group, it has a pattern's numbers no more
	e.Select(g)
	if err := e.SetGroupMode(doc.ModeUnion); err != nil {
		t.Fatal(err)
	}
	if n = e.Doc.Find(g); n.IsPattern() || n.Params != nil || len(n.Parts()) != 1 {
		t.Errorf("as a union: %+v", n)
	}
	if err := e.SetGroupMode(doc.ModePattern); err == nil {
		t.Error("a group was made a pattern by its mode")
	}
	// a bad setting leaves nothing behind
	e.Undo()
	e.Ungroup()
	e.Undo()
	before := ptDocBytes(t, e)
	e.Select(g)
	if _, err := e.Pattern(map[string]float64{"columns": 2, "wobble": 1}); err == nil {
		t.Error("a setting a pattern hasn't was accepted")
	}
	if string(ptDocBytes(t, e)) != string(before) {
		t.Error("a pattern that failed changed the design")
	}
}

// patternStep is wideStep, with patterns, hulls and stretches: now and
// then the selection is made a pattern, a pattern's setting is changed,
// shapes are grouped as a hull, or stretched, or a stretch's setting is
// changed.
func patternStep(e *Editor, r *rand.Rand) (string, bool) {
	switch r.Intn(9) {
	case 0:
		ids := doc.IDs(e.Doc.Nodes)
		if len(ids) == 0 {
			break
		}
		e.Select(ids[r.Intn(len(ids))])
		parts := 0
		for _, n := range e.Selected() {
			parts += len(n.Parts())
		}
		if parts > 12 { // patterns of patterns: kept to a few hundred copies
			break
		}
		set := map[string]float64{"type": float64(r.Intn(2)), "columns": float64(1 + r.Intn(3)), "rows": float64(1 + r.Intn(2)),
			"dx": r.Float64()*60 - 10, "dy": r.Float64() * 40, "stagger": float64(r.Intn(2)),
			"count": float64(1 + r.Intn(5)), "radius": r.Float64() * 50, "angle": float64(1 + r.Intn(360)), "turn": float64(r.Intn(2))}
		id, err := e.Pattern(set)
		return fmt.Sprintf("Pattern(%v, %v) = %d, %v", e.Sel, set, id, err), true
	case 1:
		var ps []*doc.Node
		for _, n := range e.Doc.Nodes {
			n.Walk(func(x *doc.Node) {
				if x.IsPattern() {
					ps = append(ps, x)
				}
			})
		}
		if len(ps) == 0 {
			break
		}
		n := ps[r.Intn(len(ps))]
		key, v := [][2]any{{"type", r.Intn(2)}, {"columns", 1 + r.Intn(3)}, {"rows", 1 + r.Intn(2)}, {"dx", r.Intn(50)}, {"dy", -r.Intn(50)},
			{"stagger", r.Intn(2)}, {"count", 1 + r.Intn(5)}, {"radius", r.Intn(40)}, {"angle", 1 + r.Intn(500)}, {"turn", r.Intn(3) - 1}}[r.Intn(10)], 0.0
		v = float64(key[1].(int))
		err := e.SetParam(n.ID, key[0].(string), v)
		return fmt.Sprintf("SetParam(%d pattern, %q, %g) = %v", n.ID, key[0], v, err), true
	case 2: // a hull group, of two or three shapes or of what a group held
		ids := doc.IDs(e.Doc.Nodes)
		if len(ids) < 2 {
			break
		}
		r.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
		e.Select(ids[:min(len(ids), 2+r.Intn(2))]...)
		if r.Intn(3) == 0 {
			e.Select(ids[0])
			err := e.SetGroupMode(doc.ModeHull)
			return fmt.Sprintf("SetGroupMode(%v, hull) = %v", e.Sel, err), true
		}
		id, err := e.Group(doc.ModeHull)
		return fmt.Sprintf("Group(%v, hull) = %d, %v", e.Sel, id, err), true
	case 3: // a stretch of one shape or more, through somewhere in them
		ids := doc.IDs(e.Doc.Nodes)
		if len(ids) == 0 {
			break
		}
		r.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
		e.Select(ids[:min(len(ids), 1+r.Intn(2))]...)
		axis := r.Intn(3)
		b := e.SelectionBoundsIn()
		if b.IsEmpty() {
			break
		}
		at := b.Min.Axis(axis) + (0.05+0.9*r.Float64())*b.Size().Axis(axis)
		amount := r.Float64()*20 - 0.4*b.Size().Axis(axis)
		id, err := e.Stretch(nil, axis, at, amount)
		return fmt.Sprintf("Stretch(%v, %d, %g, %g) = %d, %v", e.Sel, axis, at, amount, id, err), true
	case 4:
		var ss []*doc.Node
		for _, n := range e.Doc.Nodes {
			n.Walk(func(x *doc.Node) {
				if x.IsStretch() {
					ss = append(ss, x)
				}
			})
		}
		if len(ss) == 0 {
			break
		}
		n := ss[r.Intn(len(ss))]
		key, v := [][2]any{{"axis", r.Intn(3)}, {"amount", r.Intn(40) - 10}, {"length", r.Intn(80)}, {"at", r.Intn(30)}}[r.Intn(4)], 0.0
		v = float64(key[1].(int))
		err := e.SetParam(n.ID, key[0].(string), v)
		return fmt.Sprintf("SetParam(%d stretch, %q, %g) = %v", n.ID, key[0], v, err), true
	}
	return wideStep(e, r)
}

func TestPropertyPatternEditSequences(t *testing.T) {
	seeds, steps := int64(12), 120
	if testing.Short() {
		seeds, steps = 3, 60
	}
	wideSequences(t, seeds, steps, 104729, patternStep)
}
