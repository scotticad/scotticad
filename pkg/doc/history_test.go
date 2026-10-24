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
