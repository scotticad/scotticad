package geom

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func cube(size float64, c Color) *Mesh {
	h := size / 2
	v := func(x, y, z float64) Vec3 { return V(x*h, y*h, z*h+h) }
	quads := [][4]Vec3{
		{v(-1, -1, -1), v(-1, 1, -1), v(1, 1, -1), v(1, -1, -1)}, // bottom
		{v(-1, -1, 1), v(1, -1, 1), v(1, 1, 1), v(-1, 1, 1)},     // top
		{v(-1, -1, -1), v(1, -1, -1), v(1, -1, 1), v(-1, -1, 1)}, // front
		{v(1, -1, -1), v(1, 1, -1), v(1, 1, 1), v(1, -1, 1)},     // right
		{v(1, 1, -1), v(-1, 1, -1), v(-1, 1, 1), v(1, 1, 1)},     // back
		{v(-1, 1, -1), v(-1, -1, -1), v(-1, -1, 1), v(-1, 1, 1)}, // left
	}
	m := &Mesh{}
	for _, q := range quads {
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{q[0], q[1], q[2]}, C: c}, Tri{P: [3]Vec3{q[0], q[2], q[3]}, C: c})
	}
	return m
}

// glTF has no empty file and needs a unit normal on every vertex: an empty
// mesh is refused, a zero-area triangle left out.
func TestWriteGLBEmptyAndDegenerate(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteGLB(&buf, &Mesh{}, "empty"); err == nil || buf.Len() != 0 {
		t.Fatalf("empty mesh: err %v, %d bytes written", err, buf.Len())
	}
	m := cube(20, Color{1, 0, 0})
	m.Tris = append(m.Tris, Tri{P: [3]Vec3{V(0, 0, 0), V(1, 0, 0), V(2, 0, 0)}, C: Color{1, 0, 0}})
	if err := WriteGLB(&buf, m, "x"); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	jl := int(binary.LittleEndian.Uint32(b[12:]))
	var doc struct{ Accessors []struct{ Count int } }
	if err := json.Unmarshal(b[20:20+jl], &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Accessors[0].Count != 36 {
		t.Fatalf("%d vertices written, want 36 (the zero-area triangle left out)", doc.Accessors[0].Count)
	}
}

func TestWriteGLB(t *testing.T) {
	m := cube(20, Color{1, 0, 0})
	m.Tris = append(m.Tris, cube(10, Color{0, 0, 1}).Transform(Translate(V(40, 0, 0))).Tris...)
	var buf bytes.Buffer
	if err := WriteGLB(&buf, m, "test"); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	u32 := func(o int) uint32 { return binary.LittleEndian.Uint32(b[o:]) }
	if u32(0) != 0x46546C67 || u32(4) != 2 || int(u32(8)) != len(b) {
		t.Fatalf("bad header")
	}
	jl := int(u32(12))
	if u32(16) != 0x4E4F534A || len(b) < 20+jl+8 || u32(20+jl+4) != 0x004E4942 {
		t.Fatalf("bad chunks")
	}
	var doc struct {
		Meshes []struct {
			Primitives []struct {
				Attributes map[string]int
				Material   int
			}
		}
		Accessors []struct {
			Count int
			Min   []float64
			Max   []float64
		}
		Buffers []struct{ ByteLength int }
	}
	if err := json.Unmarshal(b[20:20+jl], &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Meshes) != 1 || len(doc.Meshes[0].Primitives) != 2 {
		t.Fatalf("want 2 primitives (one per colour): %+v", doc.Meshes)
	}
	pos := doc.Accessors[doc.Meshes[0].Primitives[0].Attributes["POSITION"]]
	if pos.Count != 36 {
		t.Fatalf("red cube vertices %d", pos.Count)
	}
	// 20 mm cube from z 0..20 -> metres, Y up: y spans 0..0.02
	if math.Abs(pos.Max[1]-0.02) > 1e-6 || math.Abs(pos.Min[1]) > 1e-6 {
		t.Fatalf("Y-up bounds %v..%v", pos.Min, pos.Max)
	}
	if doc.Buffers[0].ByteLength != int(u32(20+jl)) {
		t.Fatalf("buffer length mismatch")
	}
}

func TestSliceAndSVG(t *testing.T) {
	// a 20 mm cube and a separate 10 mm one: two square outlines
	m := cube(20, Color{})
	m.Tris = append(m.Tris, cube(10, Color{}).Transform(Translate(V(40, 0, 0))).Tris...)
	loops := Slice(m, 0.05)
	if len(loops) != 2 {
		t.Fatalf("loops: %d", len(loops))
	}
	for _, l := range loops {
		if len(l) != 4 {
			t.Fatalf("a square should have 4 corners: %v", l)
		}
	}
	// two squares touching at one corner stay two separate squares
	touch := cube(10, Color{})
	touch.Tris = append(touch.Tris, cube(10, Color{}).Transform(Translate(V(10, 10, 0))).Tris...)
	tl := Slice(touch, 0.05)
	if len(tl) != 2 || len(tl[0]) != 4 || len(tl[1]) != 4 {
		t.Fatalf("touching squares: %v", tl)
	}
	var buf bytes.Buffer
	if err := WriteSVG(&buf, loops); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, `width="57mm"`) || strings.Count(s, "M") != 2 || strings.Count(s, "Z") != 2 {
		t.Fatalf("svg:\n%s", s)
	}
}
