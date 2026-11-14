package doc

import (
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// A part an assembly's change adds is a hole only when the whole assembly
// was made one. A joint's socket is a hole on its own, so it says nothing
// about that, wherever it comes in the list.
func TestRelayoutAddsPartsAsTheAssemblyIs(t *testing.T) {
	for _, made := range []bool{false, true} {
		d := New()
		g, err := d.AddShape("dovetail", geom.Vec3{}, made)
		if err != nil {
			t.Fatal(err)
		}
		if len(g.Children) != 2 || g.Children[0].Hole != made || !g.Children[1].Hole {
			t.Fatalf("made a hole %v: parts %+v", made, g.Children)
		}
		key, socket := g.Children[0], g.Children[1]
		if made {
			g.Children = []*Node{socket, key} // the socket first
		} else {
			g.Children = []*Node{socket} // the key gone: the change adds it again
		}
		if err := d.SetAssemblyParam(g, "height", 6); err != nil {
			t.Fatal(err)
		}
		if len(g.Children) != 2 || g.Children[0].Hole != made || !g.Children[1].Hole {
			t.Errorf("made a hole %v: after the change, key hole %v, socket hole %v",
				made, g.Children[0].Hole, g.Children[1].Hole)
		}
	}
}
