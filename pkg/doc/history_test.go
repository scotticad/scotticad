package doc

import "testing"

// A group's steps undo as one; undo inside the group still steps back
// one at a time.
func TestHistoryGroup(t *testing.T) {
	var h History
	h.Push([]byte("a"))
	h.Begin()
	h.Push([]byte("b"))
	h.Push([]byte("c"))
	h.Push([]byte("d"))
	if s, _ := h.Undo([]byte("e")); string(s) != "d" {
		t.Fatalf("undo inside the group gave %q", s)
	}
	h.Push([]byte("d2"))
	h.End()
	if h.Depth() != 2 {
		t.Fatalf("depth %d, want 2 (a, then the group)", h.Depth())
	}
	if s, _ := h.Undo([]byte("now")); string(s) != "b" {
		t.Fatalf("undoing the group gave %q, want the state before it", s)
	}
	// a group that undid past its start keeps what is left
	h = History{}
	h.Push([]byte("a"))
	h.Begin()
	h.Undo([]byte("x"))
	h.Push([]byte("y"))
	h.Push([]byte("z"))
	h.End()
	if h.Depth() != 1 {
		t.Fatalf("depth %d after undoing past the group's start", h.Depth())
	}
	// clearing inside a group (a script starting a new design) keeps the
	// rest of the group as one step
	h = History{}
	h.Push([]byte("a"))
	h.Push([]byte("b"))
	h.Begin()
	h.Push([]byte("c"))
	h.Clear()
	h.Push([]byte("d"))
	h.Push([]byte("e"))
	h.End()
	if h.Depth() != 1 {
		t.Fatalf("depth %d after a clear inside a group, want 1", h.Depth())
	}
}

// The history keeps the last historyLimit steps: older ones go, oldest
// first.
func TestHistoryLimit(t *testing.T) {
	var h History
	state := func(i int) []byte { return []byte{byte(i >> 8), byte(i)} }
	for i := 0; i < historyLimit+100; i++ {
		h.Push(state(i))
	}
	if h.Depth() != historyLimit {
		t.Fatalf("depth %d, want %d", h.Depth(), historyLimit)
	}
	cur := []byte("now")
	for i := historyLimit + 99; i >= 100; i-- {
		s, ok := h.Undo(cur)
		if !ok || string(s) != string(state(i)) {
			t.Fatalf("undo %d gave %v (%v), want state %d", historyLimit+99-i, s, ok, i)
		}
		cur = s
	}
	if _, ok := h.Undo(cur); ok {
		t.Fatal("more than the limit could be undone")
	}
	// all of it redoes
	n := 0
	for ; h.CanRedo(); n++ {
		cur, _ = h.Redo(cur)
	}
	if n != historyLimit || string(cur) != "now" {
		t.Fatalf("redid %d steps to %q", n, cur)
	}
}

// A group with more steps than the limit (a long script) still undoes as
// one step, to the state it started from, and the steps before it stay.
func TestHistoryLimitInsideAGroup(t *testing.T) {
	for _, before := range []int{0, 2, historyLimit - 1, historyLimit} {
		var h History
		for i := 0; i < before; i++ {
			h.Push([]byte{'h', byte(i >> 8), byte(i)})
		}
		h.Begin()
		h.Push([]byte("start")) // the design as the script found it
		for i := 0; i < historyLimit+200; i++ {
			h.Push([]byte{'g', byte(i >> 8), byte(i)})
		}
		if h.Depth() > historyLimit {
			t.Fatalf("%d steps before: depth %d inside the group, over the limit", before, h.Depth())
		}
		h.End()
		if s, ok := h.Undo([]byte("end")); !ok || string(s) != "start" {
			t.Fatalf("%d steps before: undoing the group gave %q, want the state it started from", before, s)
		}
		// what was there before is still there (a group this long may cost
		// the oldest two)
		left := 0
		last := []byte(nil)
		for h.CanUndo() {
			last, _ = h.Undo(nil)
			left++
		}
		if left < before-2 || left > before {
			t.Fatalf("%d steps before the group: %d left after it", before, left)
		}
		if before > 0 && before < historyLimit-1 && (left != before || last[0] != 'h' || last[2] != 0) {
			t.Fatalf("%d steps before the group: %d left, the oldest %v", before, left, last)
		}
	}
}

// Undo and redo inside a group (a script's cad.undo()) keep the steps
// before the group out of it.
func TestUndoRedoInsideAGroup(t *testing.T) {
	var h History
	for _, s := range []string{"S0", "S1", "S2"} {
		h.Push([]byte(s))
	}
	cur := []byte("S3")
	h.Begin()
	for i := 0; i < 2; i++ {
		cur, _ = h.Undo(cur)
	}
	for i := 0; i < 2; i++ {
		cur, _ = h.Redo(cur)
	}
	h.End()
	if s, _ := h.Undo(cur); string(s) != "S2" {
		t.Fatalf("undo went to %q, want S2", s)
	}
	// a clear inside the group (a script opening a design), then undo and
	// redo: the rest is still one step
	h = History{}
	h.Push([]byte("S0"))
	h.Begin()
	h.Clear()
	h.Push([]byte("A"))
	h.Push([]byte("B"))
	cur = []byte("C")
	cur, _ = h.Undo(cur)
	h.Redo(cur)
	h.End()
	if h.Depth() != 1 {
		t.Fatalf("depth %d after clear, undo, redo in a group; want 1", h.Depth())
	}
}
