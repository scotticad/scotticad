package doc

import (
	"bytes"
	"testing"
	"time"

	"github.com/scotticad/scotticad/pkg/geom"
)

// Fuzz tests. Run with
//
//	go test ./pkg/doc -run '^$' -fuzz '^FuzzUnmarshal$' -fuzztime 3m
//
// Crashers are saved under testdata/fuzz/FuzzUnmarshal/ and run as seeds
// by a plain go test.

// fzWithin runs f and fails if it doesn't finish within d. A panic in f
// crashes the test binary with f's own stack, which the fuzzer records as
// a crasher like any other.
func fzWithin(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s: still running after %v", what, d)
	}
}

// fzDesign builds a design the way the editor does: shapes, a hole, nested
// groups of each mode, an assembly, an imported mesh and notes.
func fzDesign(t testing.TB) []byte {
	d := New("fuzz")
	box, _ := d.AddShape("box", geom.V(0, 0, 0), false)
	hole, _ := d.AddShape("cylinder", geom.V(0, 0, 0), true)
	hole.Size = geom.V(10, 10, 30)
	hole.M = geom.Translate(geom.V(0, 0, 10))
	g, err := d.Group([]int{box.ID, hole.ID}, ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	sph, _ := d.AddShape("sphere", geom.V(30, 0, 0), false)
	sph.Notes = []Note{{P: geom.V(0.5, 0.5, 1), Text: "top"}}
	b, err := d.Group([]int{g.ID, sph.ID}, ModeBundle)
	if err != nil {
		t.Fatal(err)
	}
	b.Name = "bundle"
	c1, _ := d.AddShape("box", geom.V(60, 0, 0), false)
	c2, _ := d.AddShape("sphere", geom.V(65, 0, 0), false)
	if _, err := d.Group([]int{c1.ID, c2.ID}, ModeIntersect); err != nil {
		t.Fatal(err)
	}
	d.AddShape("text", geom.V(0, 40, 0), false)
	d.AddShape("projectbox", geom.V(0, -60, 0), false)
	o, x, y, z := geom.V(0, 0, 0), geom.V(10, 0, 0), geom.V(0, 10, 0), geom.V(0, 0, 10)
	m := &geom.Mesh{Tris: []geom.Tri{
		{P: [3]geom.Vec3{o, y, x}},
		{P: [3]geom.Vec3{o, z, y}},
		{P: [3]geom.Vec3{o, x, z}},
		{P: [3]geom.Vec3{x, y, z}},
	}}
	d.AddMesh("tetra", m, geom.V(-40, 0, 0))
	d.Camera = &Camera{Target: geom.V(1, 2, 3), Yaw: 0.5, Pitch: 0.3, Dist: 300, Ortho: true}
	d.Notes = []Note{{P: geom.V(5, 5, 0), Text: "workplane note"}}
	data, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func FuzzUnmarshal(f *testing.F) {
	f.Add(fzDesign(f))
	f.Add([]byte(`{"version":1,"name":"t","settings":{"units":"in","workplaneW":100,"workplaneL":100,"snap":0.5,"showGrid":true},
"nodes":[{"id":1,"kind":"box","size":[20,20,20],"m":[1,0,0,0,0,1,0,0,0,0,1,0,0,0,10,1],"color":"#ff0000"},
{"id":1,"kind":"group","groupMode":"union","size":[0,0,0],"m":[1,0,0,0,0,1,0,0,0,0,1,0,0,0,0,1],"children":[
{"id":3,"kind":"cylinder","params":{"sides":200,"bevel":-1},"size":[5,5,50],"hole":true},
{"id":4,"kind":"nonsense","size":[1,2,3]},null]}],"nextId":-5}`))
	f.Add([]byte(`{"nodes":[{"kind":"mesh","meshData":"AAAAAAAAAAAAAAAAAACAPwAAAAAAAAAAAAAAAAAAgD8AAAAA","size":[10,10,10]}],"camera":{"dist":0}}`))
	f.Add([]byte(`{"nodes":[{"kind":"mesh","meshData":"AADAfwAAAAAAAAAAAACAPwAAAAAAAAAAAAAAAAAAgD8AAAAA","size":[10,10,10]}]}`)) // NaN vertex
	f.Add([]byte(`{"nodes":[{"kind":"group","groupMode":"intersect","children":[{"kind":"box","size":[1e300,1,1]},{"kind":"box","size":[-1,0,1]}]}]}`))
	f.Add([]byte(`{"nodes":[{"kind":"text","text":"Hi","font":"..\\..\\x","size":[30,10,5],"params":{"height":1e308}}]}`))
	f.Add([]byte(`{"nodes":[{"kind":"box","size":[20,20,20],"m":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1]}]}`)) // singular matrix
	f.Add([]byte(`{"nodes":[null,null],"settings":{"units":"furlong","workplaneW":-1,"snap":1e9}}`))
	// the largest id: NextID = maxID+1 overflows, and the next new id is 1 again
	f.Add([]byte(`{"nodes":[{"id":9223372036854775807,"kind":"box"},{"id":1,"kind":"box"},{"kind":"box"}]}`))
	f.Add([]byte(`{"nodes":[{"id":1,"kind":"box"}],"nextId":9223372036854775807}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var d *Document
		var err error
		fzWithin(t, 5*time.Second, "Unmarshal", func() { d, err = Unmarshal(data) })
		if err != nil {
			return
		}
		if d == nil {
			t.Fatal("nil document without an error")
		}
		m1, err := d.Marshal()
		if err != nil {
			t.Fatalf("Marshal of an unmarshalled document: %v", err)
		}
		d2, err := Unmarshal(m1)
		if err != nil {
			t.Fatalf("Unmarshal(Marshal(d)): %v\n%s", err, m1)
		}
		m2, err := d2.Marshal()
		if err != nil {
			t.Fatalf("second Marshal: %v", err)
		}
		if !bytes.Equal(m1, m2) {
			t.Fatalf("round trip changed the document:\n%s\n----\n%s", m1, m2)
		}
		// ids are unique and positive after the repair
		seen := map[int]bool{}
		for _, n := range d.Nodes {
			n.Walk(func(x *Node) {
				if x.ID <= 0 || seen[x.ID] || x.ID >= d.NextID {
					t.Fatalf("bad id %d (next %d)", x.ID, d.NextID)
				}
				seen[x.ID] = true
			})
		}
		// a fuzzed design can ask for a lot of geometry; keep it small
		nodes := 0
		for _, n := range d.Nodes {
			n.Walk(func(*Node) { nodes++ })
		}
		if nodes > 64 {
			return
		}
		fzWithin(t, 20*time.Second, "evaluating the design", func() {
			for _, n := range d.Nodes {
				n.Walk(func(x *Node) {
					x.WorldBounds()
					x.Bounds()
				})
				n.WorldMesh(false)
				n.WorldMesh(true)
			}
			Bounds(d.Nodes)
		})
	})
}
