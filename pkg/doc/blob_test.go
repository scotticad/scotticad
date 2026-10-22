package doc

import (
	"strings"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// A group's boolean doesn't depend on its colour or its own hole flag, so
// recolouring it or making it a hole needs no new boolean; anything that
// changes its shape does.
func TestShapeKeyIgnoresColour(t *testing.T) {
	d := New("t")
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := d.AddShape("box", geom.V(10, 0, 0), true)
	g, _ := d.Group([]int{a.ID, b.ID}, ModeUnion)
	k := g.shapeKey()
	SetColor(g, "#123456")
	g.Hole = true
	if g.shapeKey() != k {
		t.Fatal("recolouring or making it a hole changed the shape key")
	}
	if g.key() == k {
		t.Fatal("the full key should differ")
	}
	g.Children[1].Hole = false // a part stops being a hole: another shape
	if g.shapeKey() == k {
		t.Fatal("a part's hole flag didn't change the shape key")
	}
	g.Children[1].Hole = true
	g.Multicolor = true // its parts' colours now show: they count
	k2 := g.shapeKey()
	SetColor(g.Children[0], "#ff0000")
	if g.shapeKey() == k2 {
		t.Fatal("a multicolour group's part colour didn't change the shape key")
	}
	if v := g.UnionMesh().Volume(); v <= 0 {
		t.Fatalf("volume %v", v)
	}
}

// Copies of a mesh (pastes, reopened files) share its stored entry and
// aren't kept alive by the store; only the one stored copy is.
func TestBlobStoreKeepsOneCopy(t *testing.T) {
	var s BlobStore
	data := strings.Repeat("A", blobMin)
	h := s.put(data)
	for i := 0; i < 20; i++ {
		if got := s.put(strings.Clone(data)); got != h {
			t.Fatalf("copy %d hashed to %s, want %s", i, got, h)
		}
	}
	if len(s.byHash) != 1 || len(s.byPtr) != 1 {
		t.Fatalf("%d stored, %d looked up by address; want 1 and 1", len(s.byHash), len(s.byPtr))
	}
	if s.put(data) != h { // the stored copy, found by its address
		t.Fatal("the stored copy hashed differently")
	}
	s.Reset()
	if s.Len() != 0 {
		t.Fatalf("%d stored after Reset", s.Len())
	}
	if s.put(data) != h {
		t.Fatal("put after Reset")
	}
}
