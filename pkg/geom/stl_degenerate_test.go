package geom

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// A triangle of no area has no normal; it is left out of the STL rather
// than written with a zero one (a damaged facet to a slicer).
func TestWriteSTLDropsDegenerateTriangles(t *testing.T) {
	m := cube(10, Color{})
	m.Tris = append(m.Tris, Tri{P: [3]Vec3{V(0, 0, 0), V(1, 0, 0), V(2, 0, 0)}}, Tri{P: [3]Vec3{V(3, 3, 3), V(3, 3, 3), V(4, 4, 4)}})
	var buf bytes.Buffer
	if err := WriteSTL(&buf, m, "d"); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if n := binary.LittleEndian.Uint32(b[80:]); n != 12 || len(b) != 84+50*12 {
		t.Fatalf("%d triangles written, %d bytes", n, len(b))
	}
	for i := 0; i < 12; i++ {
		o := 84 + 50*i
		var n [3]float64
		for k := range n {
			n[k] = float64(math.Float32frombits(binary.LittleEndian.Uint32(b[o+4*k:])))
		}
		if l := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2]); math.Abs(l-1) > 1e-6 {
			t.Fatalf("triangle %d has normal %v", i, n)
		}
	}
	back, err := ReadSTL(b)
	if err != nil || math.Abs(back.Volume()-1000) > 1e-6 {
		t.Fatalf("read back: %v, volume %v", err, back.Volume())
	}
}

// A sliver's normal is the one its corners give once rounded to float32,
// as a reader of the file sees them, not the exact one.
func TestWriteSTLNormalsMatchTheWrittenCorners(t *testing.T) {
	o := V(1000.1, 1000.1, 1000.1)
	tri := Tri{P: [3]Vec3{o, o.Add(V(10, 0, 0)), o.Add(V(5, 4e-5, 1e-4))}}
	var buf bytes.Buffer
	if err := WriteSTL(&buf, &Mesh{Tris: []Tri{tri}}, "s"); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	var f [12]float64
	for k := range f {
		f[k] = float64(math.Float32frombits(binary.LittleEndian.Uint32(b[84+4*k:])))
	}
	n := V(f[0], f[1], f[2])
	w := Tri{P: [3]Vec3{V(f[3], f[4], f[5]), V(f[6], f[7], f[8]), V(f[9], f[10], f[11])}}.Normal()
	if tri.Normal().Dot(w) > 0.999 {
		t.Fatalf("not a sliver that rounding turns: exact %v, written %v", tri.Normal(), w)
	}
	if n.Dot(w) < 0.99999 {
		t.Fatalf("normal %v, written corners give %v", n, w)
	}
}
