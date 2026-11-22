package doc

import (
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Patterns inside patterns multiply: 50 × 50 of 50 × 50 would be six
// million shapes. The settings that ask for it are refused, and a design
// file that holds them shows no more than MaxPatternShapes.
func TestPatternShapesAreCapped(t *testing.T) {
	d := New()
	box, _ := d.AddShape("box", geom.Vec3{}, false)
	inner, err := d.Pattern([]int{box.ID})
	if err != nil {
		t.Fatal(err)
	}
	for key, v := range map[string]float64{"columns": 50, "rows": 50} {
		if err := SetPatternParam(inner, key, v); err != nil {
			t.Fatal(err)
		}
	}
	outer, err := d.Pattern([]int{inner.ID})
	if err != nil {
		t.Fatal(err)
	}
	// 3 × 1 of 2500 is fine; 3 × 50 is 375 000: refused, and left as it was
	if err := SetPatternParam(outer, "rows", 50); err == nil {
		t.Error("150 copies of 2500 shapes were accepted")
	}
	if got := outer.PatternValue("rows"); got != 1 {
		t.Errorf("rows %v after the refusal, want 1", got)
	}
	if err := SetPatternParam(outer, "rows", 2); err != nil {
		t.Errorf("6 copies of 2500 shapes: %v", err)
	}
	// a file can hold anything: three deep at 50 × 50 each
	n, err := d.Pattern([]int{outer.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []*Node{inner, outer, n} {
		p.Params["columns"], p.Params["rows"] = 50, 50
	}
	start := time.Now()
	shown := 0
	var count func(x *Node, times int)
	count = func(x *Node, times int) {
		if !x.IsGroup() {
			shown += times
			return
		}
		if x.IsPattern() {
			times *= len(x.Copies())
		}
		for _, c := range x.Children {
			count(c, times)
		}
	}
	count(n, 1)
	if shown < 1 || shown > MaxPatternShapes {
		t.Errorf("%d shapes shown, want at most %d", shown, MaxPatternShapes)
	}
	if b := n.Bounds(); b.IsEmpty() {
		t.Error("the capped pattern has no bounds")
	}
	if e := time.Since(start); e > 20*time.Second {
		t.Errorf("took %v", e)
	}
}
