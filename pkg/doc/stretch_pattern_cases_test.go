package doc

import (
	"fmt"
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Changing a shortening stretch's axis to one along which the shapes are
// shorter than the shortening is refused, as setting the amount is: it
// would take everything out, and Ungroup would then delete it all.
func TestStretchAxisChangeKeepsTheShrinkCheck(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	a.Size = geom.V(20, 20, 60)
	a.M = geom.Translate(geom.V(0, 0, 30))
	g, err := d.Stretch(IDs([]*Node{a}), geom.Identity(), 2, 30, -40)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetStretchParam(g, "axis", 0); err == nil {
		t.Fatal("an axis along which the box is 20 mm took a 40 mm shortening")
	}
	if ax, _, _ := g.stretchPlane(); ax != 2 {
		t.Errorf("the refused axis stayed: %d", ax)
	}
	if g.WorldBounds().IsEmpty() {
		t.Error("the stretch shows nothing")
	}
	// a shortening that fits the new axis is still fine
	if err := SetStretchParam(g, "amount", -10); err != nil {
		t.Fatal(err)
	}
	if err := SetStretchParam(g, "axis", 0); err != nil {
		t.Fatalf("a 10 mm shortening of 20 mm refused: %v", err)
	}
	if n := len(d.Ungroup([]int{g.ID})); n == 0 {
		t.Error("ungroup released nothing")
	}
}

// Patterns nested in patterns cost time in proportion to their depth, not
// doubling at each level.
func TestNestedPatternsAreCheap(t *testing.T) {
	const depth = 40
	s := `{"id":1,"kind":"box","size":{"X":20,"Y":20,"Z":20},"m":[1,0,0,0,0,1,0,0,0,0,1,0,0,0,10,1],"color":"#ff0000"}`
	for i := 0; i < depth; i++ {
		s = fmt.Sprintf(`{"id":%d,"kind":"group","groupMode":"pattern","params":{"columns":1,"rows":1},"size":{"X":0,"Y":0,"Z":0},"m":[1,0,0,0,0,1,0,0,0,0,1,0,0,0,0,1],"color":"#ff0000","children":[%s]}`, i+2, s)
	}
	d, err := Unmarshal([]byte(`{"version":1,"settings":{"units":"mm"},"nodes":[` + s + `],"nextId":100}`))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	b := d.Nodes[0].WorldBounds()
	if dt := time.Since(start); dt > 2*time.Second {
		t.Fatalf("bounds took %v", dt)
	}
	if sz := b.Size(); sz.X < 19.9 || sz.X > 20.1 {
		t.Errorf("bounds %v", b)
	}
}

// The cap on what nested patterns show still counts every level.
func TestNestedPatternCapStillMultiplies(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	in := &Node{ID: d.newID(), Kind: KindGroup, GroupMode: ModePattern, M: geom.Identity(), Children: []*Node{a},
		Params: map[string]float64{"columns": 50, "rows": 50}}
	out := &Node{ID: d.newID(), Kind: KindGroup, GroupMode: ModePattern, M: geom.Identity(), Children: []*Node{in},
		Params: map[string]float64{"columns": 50, "rows": 50}}
	if got := len(out.Copies()) * out.shapesShown(); got > MaxPatternShapes {
		t.Errorf("shows %d shapes, over %d", got, MaxPatternShapes)
	}
	if got := len(out.Copies()); got != MaxPatternShapes/2500 {
		t.Errorf("outer pattern shows %d copies, want %d", got, MaxPatternShapes/2500)
	}
}

// A stretch remade the same way after a part's transparency changed shows
// the new transparency, not the cached pieces' old one.
func TestStretchCacheKeepsTransparency(t *testing.T) {
	d := New()
	a, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	a.Transparent = true
	snap, _ := d.Marshal()
	g, err := d.Stretch([]int{a.ID}, geom.Identity(), 0, 0, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !g.LocalParts()[0].Transparent {
		t.Fatal("transparent box shows opaque")
	}
	d, _ = Unmarshal(snap)
	d.Nodes[0].Transparent = false
	g, _ = d.Stretch([]int{d.Nodes[0].ID}, geom.Identity(), 0, 0, 7)
	if g.LocalParts()[0].Transparent {
		t.Error("opaque box shows transparent after stretching again")
	}
}
