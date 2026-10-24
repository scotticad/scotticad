package doc

import (
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// An imported mesh with a gap in its surface (here a box with a triangle
// missing) is closed before booleans use it: a union with an open operand
// misjudges what is inside it and can lose whole stretches of surface.
func TestImportedMeshGapIsClosedBeforeBooleans(t *testing.T) {
	whole := shapes.Build("box", geom.V(20, 20, 20), shapes.Values{})
	open := &geom.Mesh{Tris: append(append([]geom.Tri{}, whole.Tris[:6]...), whole.Tris[7:]...)}
	data := EncodeMesh(open)
	d := New()
	var ids []int
	for _, deg := range []float64{0, 7} {
		n := &Node{ID: d.newID(), Kind: KindMesh, MeshData: data, Size: geom.V(20, 20, 20),
			M: geom.RotateZ(deg * math.Pi / 180), Color: "#ffffff"}
		d.Nodes = append(d.Nodes, n)
		ids = append(ids, n.ID)
	}
	g, err := d.Group(ids, ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	got := g.UnionMesh()
	want := geom.Union(whole, whole.Transform(geom.RotateZ(7*math.Pi/180))).FixTJunctions()
	if o, nm := got.EdgeCheck(0); o != 0 || nm != 0 {
		t.Errorf("the union has %d open and %d non-manifold edges", o, nm)
	}
	if math.Abs(got.Volume()-want.Volume()) > 1e-6*want.Volume() {
		t.Errorf("volume %.1f, want %.1f", got.Volume(), want.Volume())
	}
}

// The imported mesh itself is closed, and one that is closed already comes
// through as it is (no repair pass reshapes it).
func TestImportedMeshLeaf(t *testing.T) {
	whole := shapes.Build("box", geom.V(20, 20, 20), shapes.Values{})
	leaf := func(m *geom.Mesh) *geom.Mesh {
		n := &Node{ID: 1, Kind: KindMesh, MeshData: EncodeMesh(m), Size: geom.V(20, 20, 20), M: geom.Identity(), Color: "#ffffff"}
		return n.LeafMesh()
	}
	open := &geom.Mesh{Tris: append(append([]geom.Tri{}, whole.Tris[:6]...), whole.Tris[7:]...)}
	if o, nm := leaf(open).EdgeCheck(0); o != 0 || nm != 0 {
		t.Errorf("an open import's leaf has %d open and %d non-manifold edges", o, nm)
	}
	got := leaf(whole)
	if len(got.Tris) != len(whole.Tris) {
		t.Fatalf("a closed import came back with %d triangles, not %d", len(got.Tris), len(whole.Tris))
	}
	for i := range got.Tris {
		if got.Tris[i].P != whole.Tris[i].P {
			t.Fatalf("a closed import's triangle %d changed", i)
		}
	}
}
