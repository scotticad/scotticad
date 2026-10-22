package doc

// Property-based tests on random documents: save/load round trips are
// exact, splitting conserves volume, and group volumes obey their modes.

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/scotticad/scotticad/pkg/geom"
)

// cheap, closed shape kinds (no text, threads or big grilles)
var ptKinds = []string{"box", "cylinder", "sphere", "cone", "pyramid", "roof", "wedge", "tube", "torus", "halfsphere", "polygon", "icosahedron", "star", "ring", "paraboloid", "diamond"}

func ptRandVec(r *rand.Rand, s float64) geom.Vec3 {
	return geom.V((r.Float64()*2-1)*s, (r.Float64()*2-1)*s, (r.Float64()*2-1)*s)
}

// ptAdd adds a random shape, randomly scaled and turned.
func ptAdd(t *testing.T, d *Document, r *rand.Rand, hole bool) *Node {
	n, err := d.AddShape(ptKinds[r.Intn(len(ptKinds))], ptRandVec(r, 15), hole)
	if err != nil {
		t.Fatal(err)
	}
	c := n.WorldBounds().Center()
	ScaleAbout([]*Node{n}, geom.V(0.3+r.Float64()*1.5, 0.3+r.Float64()*1.5, 0.3+r.Float64()*1.5), c)
	RotateAbout([]*Node{n}, r.Intn(3), r.Float64()*360, c)
	return n
}

func ptDesc(n *Node) string {
	b, _ := (&Document{Nodes: []*Node{n}}).Marshal()
	return string(b)
}

// Marshal → Unmarshal → Marshal gives the same bytes, also through the
// history's shared-blob form.
func TestPropertyMarshalRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(31))
	for i := 0; i < 40; i++ {
		d := New(fmt.Sprintf("design %d \"q\" ✓", i))
		for k := 0; k < 2+r.Intn(8); k++ {
			ptAdd(t, d, r, r.Intn(4) == 0)
		}
		if r.Intn(2) == 0 {
			d.AddShape("projectbox", ptRandVec(r, 30), false)
		}
		for k := 0; k < 3 && len(d.Nodes) >= 2; k++ {
			ids := []int{d.Nodes[r.Intn(len(d.Nodes))].ID, d.Nodes[r.Intn(len(d.Nodes))].ID}
			if ids[0] != ids[1] {
				d.Group(ids, []string{ModeUnion, ModeBundle, ModeIntersect}[r.Intn(3)])
			}
		}
		d.AddMesh("imp", geom.Merge(ptAddMesh(r)), ptRandVec(r, 20))
		b1, err := d.Marshal()
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		d2, err := Unmarshal(b1)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		b2, _ := d2.Marshal()
		if !bytes.Equal(b1, b2) {
			t.Fatalf("case %d: round trip changed the file\n%s\n----\n%s", i, b1, b2)
		}
		var s BlobStore
		sh, err := d.MarshalShared(&s)
		if err != nil {
			t.Fatal(err)
		}
		d3, err := UnmarshalShared(sh, &s)
		if err != nil {
			t.Fatal(err)
		}
		if b3, _ := d3.Marshal(); !bytes.Equal(b1, b3) {
			t.Fatalf("case %d: shared round trip changed the file", i)
		}
	}
}

func ptAddMesh(r *rand.Rand) *geom.Mesh {
	d := New("m")
	n, _ := d.AddShape(ptKinds[r.Intn(len(ptKinds))], geom.V(0, 0, 0), false)
	return n.WorldMesh(false)
}

// Splitting a solid keeping both halves conserves its volume, and each half
// is watertight and on its side of the plane.
func TestPropertySplitConservesVolume(t *testing.T) {
	r := rand.New(rand.NewSource(32))
	for i := 0; i < 150; i++ {
		d := New("t")
		n := ptAdd(t, d, r, false)
		if r.Intn(3) == 0 { // a union group with a hole in it
			m := ptAdd(t, d, r, false)
			h := ptAdd(t, d, r, true)
			g, err := d.Group([]int{n.ID, m.ID, h.ID}, ModeUnion)
			if err != nil {
				t.Fatal(err)
			}
			n = g
		}
		desc := ptDesc(n)
		whole := n.WorldMesh(false)
		v := whole.Volume()
		if o, nm := whole.EdgeCheck(1e-7); o != 0 || nm != 0 || v <= 0 {
			t.Logf("case %d: input not a clean solid (%d open, %d nm, volume %g): skipped", i, o, nm, v)
			continue
		}
		b := n.WorldBounds()
		axis := r.Intn(3)
		at := b.Min.Axis(axis) + (0.1+0.8*r.Float64())*b.Size().Axis(axis)
		hs, err := d.Split([]int{n.ID}, geom.Identity(), axis, at, KeepBoth, 0)
		if err != nil || len(hs) != 2 {
			t.Fatalf("case %d: split: %v, %d halves\n%s", i, err, len(hs), desc)
		}
		lo, hi := hs[0].WorldMesh(false), hs[1].WorldMesh(false)
		if d := lo.Volume() + hi.Volume() - v; math.Abs(d) > 1e-6*math.Max(1, v) {
			t.Errorf("case %d: halves %g + %g, whole %g (axis %d at %g)\n%s", i, lo.Volume(), hi.Volume(), v, axis, at, desc)
		}
		if lb, hb := lo.Bounds(), hi.Bounds(); lb.Max.Axis(axis) > at+1e-6 || hb.Min.Axis(axis) < at-1e-6 {
			t.Errorf("case %d: halves cross the plane at %g: below to %g, above from %g\n%s", i, at, lb.Max.Axis(axis), hb.Min.Axis(axis), desc)
		}
		for k, h := range []*geom.Mesh{lo, hi} {
			if o, nm := h.EdgeCheck(1e-7); o != 0 || nm != 0 {
				t.Errorf("case %d: half %d has %d open, %d non-manifold edges (axis %d at %g)\n%s", i, k, o, nm, axis, at, desc)
			}
		}
	}
}

// A union group's volume is between its largest child's and the sum; an
// intersection's is at most the smallest child's; union + intersection of
// two equals the sum of the two.
func TestPropertyGroupVolumes(t *testing.T) {
	r := rand.New(rand.NewSource(33))
	for i := 0; i < 150; i++ {
		d := New("t")
		a, b := ptAdd(t, d, r, false), ptAdd(t, d, r, false)
		// bring them together so they overlap often
		Move([]*Node{b}, a.WorldBounds().Center().Sub(b.WorldBounds().Center()).Add(ptRandVec(r, 8)))
		va, vb := a.WorldMesh(false).Volume(), b.WorldMesh(false).Volume()
		desc := ptDesc(a) + "\n" + ptDesc(b)
		ca, cb := a.Clone(), b.Clone()
		d.renumber(ca)
		d.renumber(cb)
		d.Nodes = append(d.Nodes, ca, cb)
		u, err := d.Group([]int{a.ID, b.ID}, ModeUnion)
		if err != nil {
			t.Fatal(err)
		}
		x, err := d.Group([]int{ca.ID, cb.ID}, ModeIntersect)
		if err != nil {
			t.Fatal(err)
		}
		vu, vx := u.WorldMesh(false).Volume(), x.WorldMesh(false).Volume()
		tol := 1e-6 * math.Max(1, va+vb)
		if vu < math.Max(va, vb)-tol || vu > va+vb+tol {
			t.Errorf("case %d: union %g of %g and %g\n%s", i, vu, va, vb, desc)
		}
		if vx < -tol || vx > math.Min(va, vb)+tol {
			t.Errorf("case %d: intersection %g of %g and %g\n%s", i, vx, va, vb, desc)
		}
		if math.Abs(vu+vx-va-vb) > tol {
			t.Errorf("case %d: union %g + intersection %g ≠ %g + %g\n%s", i, vu, vx, va, vb, desc)
		}
	}
}
