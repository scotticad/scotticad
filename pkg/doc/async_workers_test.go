package doc

import (
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

// A group changed many times in quick succession (a slider dragged through
// its values) doesn't run a boolean for every value: a few run at a time,
// and a job whose value has been superseded before it starts is dropped.
// The last value is always computed.
func TestStaleBackgroundJobsAreDropped(t *testing.T) {
	SetAsync(true)
	defer SetAsync(false)
	OnReady = nil
	// a plate with a grid of hole cylinders: each evaluation takes a while
	g := &Node{ID: 100, Kind: KindGroup, M: geom.Identity(), Color: "#ff0000"}
	g.Children = append(g.Children, &Node{ID: 101, Kind: "box", Size: geom.V(60, 60, 4), M: geom.Translate(geom.V(0, 0, 2)), Color: "#ff0000"})
	id := 102
	for i := 0; i < 6; i++ {
		for j := 0; j < 6; j++ {
			g.Children = append(g.Children, &Node{ID: id, Kind: "cylinder", Hole: true, Size: geom.V(4, 4, 10),
				M: geom.Translate(geom.V(float64(i)*9-22, float64(j)*9-22, 2)), Color: "#888888",
				Params: map[string]float64{"sides": 32}})
			id++
		}
	}
	var keys []string
	for v := 0; v < 25; v++ {
		g.Children[0].Size.X = 60 + float64(v) // each a new value, as a drag gives
		keys = append(keys, "u:"+g.key())
		g.unionMeshFor()
	}
	deadline := time.Now().Add(60 * time.Second)
	for Pending() > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if Pending() > 0 {
		t.Fatal("background evaluations never finished")
	}
	if _, ok := meshCache.Load(keys[len(keys)-1]); !ok {
		t.Fatal("the last value wasn't evaluated")
	}
	computed := 0
	for _, k := range keys {
		if _, ok := meshCache.Load(k); ok {
			computed++
		}
	}
	if computed > 6 {
		t.Fatalf("%d of 25 superseded values were evaluated", computed)
	}
}
