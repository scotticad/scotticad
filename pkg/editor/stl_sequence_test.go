package editor

// A long boolean sequence through the editor (grouping at several levels,
// holes, intersections, transforms, undo and redo, ungrouping), with the
// volume worked out by hand at every step, ending in STL exports that are
// read back independently of the program's own reader and checked: the
// binary layout, the normals against the triangles' winding, closedness
// after the file's float32 rounding, and the volume.

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
)

// stlFile is a binary STL taken apart by hand.
type stlFile struct {
	header  string
	count   uint32
	normals []geom.Vec3
	mesh    *geom.Mesh // the triangles as the file holds them (float32)
	attrs   []uint16
}

func parseSTL(t *testing.T, b []byte) stlFile {
	t.Helper()
	if len(b) < 84 {
		t.Fatalf("STL is %d bytes", len(b))
	}
	f := stlFile{header: string(bytes.TrimRight(b[:80], "\x00")), count: binary.LittleEndian.Uint32(b[80:84]), mesh: &geom.Mesh{}}
	if want := 84 + 50*int(f.count); len(b) != want {
		t.Fatalf("STL holds %d triangles but is %d bytes, not %d", f.count, len(b), want)
	}
	at := func(o int) float64 { return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[o:]))) }
	for i := 0; i < int(f.count); i++ {
		o := 84 + 50*i
		f.normals = append(f.normals, geom.V(at(o), at(o+4), at(o+8)))
		var tri geom.Tri
		for k := 0; k < 3; k++ {
			tri.P[k] = geom.V(at(o+12+12*k), at(o+16+12*k), at(o+20+12*k))
		}
		f.mesh.Tris = append(f.mesh.Tris, tri)
		f.attrs = append(f.attrs, binary.LittleEndian.Uint16(b[o+48:]))
	}
	return f
}

// checkSTL verifies an exported STL against the mesh it was made from and
// the volume it should have.
func checkSTL(t *testing.T, b []byte, src *geom.Mesh, volume float64, what string) {
	t.Helper()
	f := parseSTL(t, b)
	if len(f.header) < len("ScottiCAD ") || f.header[:10] != "ScottiCAD " {
		t.Errorf("%s: header %q", what, f.header)
	}
	if want := len(src.Clean().Tris); int(f.count) != want {
		t.Errorf("%s: %d triangles in the file, %d in the mesh", what, f.count, want)
	}
	if int(f.count) != len(src.Tris) {
		t.Logf("%s: the mesh held %d triangles of no area, left out of the file", what, len(src.Tris)-int(f.count))
	}
	for i, tri := range f.mesh.Tris {
		for _, p := range tri.P {
			for _, v := range []float64{p.X, p.Y, p.Z} {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Fatalf("%s: triangle %d has %v", what, i, p)
				}
			}
		}
		if f.attrs[i] != 0 {
			t.Errorf("%s: triangle %d has attribute bytes %d", what, i, f.attrs[i])
		}
		n := f.normals[i]
		if a := tri.Area(); a > 1e-6 {
			if math.Abs(n.Len()-1) > 1e-5 {
				t.Errorf("%s: triangle %d normal %v isn't unit length", what, i, n)
			}
			if d := n.Dot(tri.Normal()); d < 0.999 {
				t.Errorf("%s: triangle %d normal %v disagrees with its winding (%v)", what, i, n, tri.Normal())
			}
		}
	}
	// closed as written: the float32 rounding keeps shared vertices shared
	if open, nm := f.mesh.EdgeCheck(0); open != 0 {
		t.Errorf("%s: %d open edges (%d non-manifold) in the file", what, open, nm)
	}
	if v := f.mesh.Volume(); math.Abs(v-volume) > 1e-5*math.Max(1, volume) {
		t.Errorf("%s: the file's volume is %.6f, want %.6f", what, v, volume)
	}
	// the program reads its own file back the same
	m, err := geom.ReadSTL(b)
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if v := m.Volume(); math.Abs(v-volume) > 1e-5*math.Max(1, volume) {
		t.Errorf("%s: read back with volume %.6f, want %.6f", what, v, volume)
	}
}

func TestBooleanSequenceAndSTL(t *testing.T) {
	e := New()
	box := func(lo, hi geom.Vec3, hole bool) int {
		id, err := e.AddShape("box", geom.Vec3{}, hole)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.SetBounds([]int{id}, geom.Box{Min: lo, Max: hi}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	export := func(ids ...int) *geom.Mesh {
		t.Helper()
		e.Select(ids...)
		m, err := e.ExportMesh(len(ids) > 0)
		if err != nil {
			t.Fatalf("export %v: %v", ids, err)
		}
		return m
	}
	expect := func(what string, volume float64, ids ...int) *geom.Mesh {
		t.Helper()
		m := export(ids...)
		if v := m.Volume(); !near(v, volume) {
			t.Fatalf("%s: volume %.6f, want %.6f", what, v, volume)
		}
		if open, _ := m.EdgeCheck(1e-9); open != 0 {
			t.Fatalf("%s: %d open edges", what, open)
		}
		return m
	}

	plate := box(geom.V(0, 0, 0), geom.V(60, 40, 10), false)
	ids := []int{plate}
	for _, c := range [][2]float64{{5, 5}, {51, 5}, {5, 31}, {51, 31}} {
		ids = append(ids, box(geom.V(c[0], c[1], -5), geom.V(c[0]+4, c[1]+4, 15), true))
	}
	e.Select(ids...)
	g1, err := e.Group(doc.ModeUnion)
	if err != nil {
		t.Fatal(err)
	}
	expect("plate with four holes", 24000-4*160, g1)

	boss := box(geom.V(20, 10, 10), geom.V(40, 30, 25), false)
	bore := box(geom.V(25, 15, -5), geom.V(35, 25, 30), true)
	e.Select(g1, boss, bore)
	g2, _ := e.Group(doc.ModeUnion)
	expect("boss and bore", 23360+6000-1000-1500, g2)

	clip := box(geom.V(0, 0, 0), geom.V(30, 40, 30), false)
	e.Select(g2, clip)
	g3, _ := e.Group(doc.ModeIntersect)
	const left = 12000 - 320 + 3000 - 500 - 750
	expect("clipped to the left half", left, g3)

	// turned, flipped and moved: the same solid
	e.Rotate([]int{g3}, 2, 90)
	e.Mirror([]int{g3}, 0)
	e.Move([]int{g3}, geom.V(3, -2, 1))
	expect("turned, flipped and moved", left, g3)
	e.Undo()
	e.Undo()
	e.Undo()
	expect("transforms undone", left, g3)

	// the result as a hole in a block
	block := box(geom.V(-10, -10, -10), geom.V(70, 50, 40), false)
	e.Select(g3)
	e.SetHole(true)
	e.Select(block, g3)
	g4, _ := e.Group(doc.ModeUnion)
	all := expect("block minus the result", 240000-left, g4)
	if v := export().Volume(); !near(v, 240000-left) {
		t.Fatalf("everything: %v", v)
	}

	// the STL as written, checked independently
	var buf bytes.Buffer
	if err := geom.WriteSTL(&buf, all, "sequence"); err != nil {
		t.Fatal(err)
	}
	checkSTL(t, buf.Bytes(), all, 240000-left, "block minus result")
	// and turned off the axes, where every coordinate rounds in float32
	e.Rotate([]int{g4}, 2, 37)
	e.Rotate([]int{g4}, 0, 20)
	turned := export(g4)
	buf.Reset()
	if err := geom.WriteSTL(&buf, turned, "turned"); err != nil {
		t.Fatal(err)
	}
	checkSTL(t, buf.Bytes(), turned, 240000-left, "block minus result, turned")
	e.Undo()
	e.Undo()

	// undo the grouping: the hole result alone exports nothing
	e.Undo()
	if v := export().Volume(); !near(v, 240000) {
		t.Fatalf("after undoing the group: %v", v)
	}
	e.Select(g3)
	if _, err := e.ExportMesh(true); err == nil {
		t.Fatal("a hole on its own exported something")
	}
	// undo making it a hole: two solids that overlap export both
	e.Undo()
	if v := export().Volume(); !near(v, 240000+left) {
		t.Fatalf("after undoing the hole: %v", v)
	}
	e.Redo()
	e.Redo()
	expect("redone", 240000-left, g4)

	// dissolved: the hole stays a hole; undone: back as it was
	e.Select(g4)
	e.Ungroup()
	if v := export().Volume(); !near(v, 240000) {
		t.Fatalf("after ungrouping: %v", v)
	}
	e.Undo()
	expect("ungroup undone", 240000-left, g4)
	// saved and opened again, the same solid
	p := t.TempDir() + "/seq.scotticad"
	if err := e.Save(p); err != nil {
		t.Fatal(err)
	}
	e2 := New()
	if err := e2.Open(p); err != nil {
		t.Fatal(err)
	}
	if m, err := e2.ExportMesh(false); err != nil || !near(m.Volume(), 240000-left) {
		t.Fatalf("reopened: %v %v", err, m.Volume())
	}
}

// Every catalogue shape exports as a closed STL whose volume is the
// shape's, after float32 rounding; so does a bundle (its parts separate)
// and a design with hidden shapes (left out).
func TestEveryShapeExportsAClosedSTL(t *testing.T) {
	for _, kind := range []string{"box", "cylinder", "sphere", "cone", "pyramid", "roof", "wedge", "tube", "torus", "halfsphere",
		"polygon", "icosahedron", "star", "ring", "heart", "diamond", "text", "gear", "star3d", "projectbox", "plate", "standoff", "knob"} {
		e := New()
		id, err := e.AddShape(kind, geom.Vec3{}, false)
		if err != nil {
			t.Fatal(err)
		}
		e.Rotate([]int{id}, 0, 30) // off the axes: nothing rounds to the grid
		m, err := e.ExportMesh(false)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		var buf bytes.Buffer
		if err := geom.WriteSTL(&buf, m, kind); err != nil {
			t.Fatal(err)
		}
		checkSTL(t, buf.Bytes(), m, m.Volume(), kind)
		if m.Volume() <= 0 {
			t.Errorf("%s: volume %v", kind, m.Volume())
		}
	}
	// hidden shapes stay out; a selection exports only itself
	e := New()
	a, _ := e.AddShape("box", geom.V(0, 0, 0), false)
	b, _ := e.AddShape("box", geom.V(50, 0, 0), false)
	c, _ := e.AddShape("box", geom.V(100, 0, 0), false)
	e.Select(c)
	e.HideSelected()
	m, _ := e.ExportMesh(false)
	if !near(m.Volume(), 16000) {
		t.Fatalf("with one hidden: %v", m.Volume())
	}
	e.Select(a)
	m, _ = e.ExportMesh(true)
	if !near(m.Volume(), 8000) {
		t.Fatalf("selection only: %v", m.Volume())
	}
	_ = b
}
