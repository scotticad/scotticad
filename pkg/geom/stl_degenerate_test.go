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
