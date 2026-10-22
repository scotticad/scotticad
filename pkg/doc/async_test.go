package doc

import (
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

// A slow group shows a preview at once and its real result once the
// background evaluation finishes; Sync and exports are always exact.
func TestAsyncEvaluation(t *testing.T) {
	SetAsync(true)
	defer SetAsync(false)
	ready := make(chan struct{}, 16)
	OnReady = func() { ready <- struct{}{} }
	defer func() { OnReady = nil }()

	// a plate with a grid of hole cylinders: real booleans
	g := &Node{ID: 1, Kind: KindGroup, M: geom.Identity(), Color: "#ff0000"}
	g.Children = append(g.Children, &Node{ID: 2, Kind: "box", Size: geom.V(60, 60, 4), M: geom.Translate(geom.V(0, 0, 2)), Color: "#ff0000"})
	id := 3
	for i := 0; i < 5; i++ {
		for j := 0; j < 5; j++ {
			g.Children = append(g.Children, &Node{ID: id, Kind: "cylinder", Hole: true, Size: geom.V(4, 4, 10),
				M: geom.Translate(geom.V(float64(i)*10-20, float64(j)*10-20, 2)), Color: "#888888",
				Params: map[string]float64{"sides": 24}})
			id++
		}
	}
	parts := g.LocalParts()
	if Pending() == 0 {
		t.Fatal("no background evaluation started")
	}
	preview := parts[0].Mesh
	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		t.Fatal("background evaluation never finished")
	}
	WaitIdle()
	real := g.LocalParts()[0].Mesh
	if real == preview || real.Volume() >= preview.Volume() {
		t.Fatalf("expected the holed result (volume %v) to replace the preview (%v)", real.Volume(), preview.Volume())
	}
	// synchronous evaluation gives the same exact mesh
	var exact *geom.Mesh
	Sync(func() { exact = g.UnionMesh() })
	if exact != real {
		t.Fatal("Sync did not return the cached exact result")
	}
}

// Shared snapshots keep large mesh data once, and read back identically.
func TestSharedSnapshots(t *testing.T) {
	big := EncodeMesh(geom.Merge(sampleMesh(), sampleMesh().Transform(geom.Translate(geom.V(50, 0, 0)))))
	for len(big) < 3*blobMin {
		big += EncodeMesh(sampleMesh())
	}
	d := New("t")
	d.Nodes = append(d.Nodes, &Node{ID: 1, Kind: KindMesh, MeshData: big, Size: geom.V(10, 10, 10), M: geom.Identity(), Color: "#ffffff"})
	d.NextID = 2
	var s BlobStore
	a, _ := d.MarshalShared(&s)
	b, _ := d.MarshalShared(&s)
	if len(a) >= len(big) || s.Len() != 1 || string(a) != string(b) {
		t.Fatalf("snapshot %d bytes for %d of mesh data, %d blobs", len(a), len(big), s.Len())
	}
	back, err := UnmarshalShared(a, &s)
	if err != nil || back.Nodes[0].MeshData != big {
		t.Fatalf("round trip lost the mesh (%v)", err)
	}
}

func sampleMesh() *geom.Mesh {
	m := &geom.Mesh{}
	for i := 0; i < 200; i++ {
		f := float64(i)
		m.Tris = append(m.Tris, geom.Tri{P: [3]geom.Vec3{geom.V(f, 0, 0), geom.V(f+1, 0, 0), geom.V(f, 1, f)}})
	}
	return m
}
