package geom

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"
)

// Fuzz tests. Run one with, e.g.
//
//	go test ./pkg/geom -run '^$' -fuzz '^FuzzReadSTL$' -fuzztime 60s
//
// Crashers are saved under testdata/fuzz/<Name>/ and run as seeds by a
// plain go test.

// fzWithin runs f and fails if it doesn't finish within d (a hang).
func fzWithin(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	// a panic in f crashes the test binary with f's own stack, which the
	// fuzzer records as a crasher like any other
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

func fzFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func fzCheckMesh(t *testing.T, what string, m *Mesh) {
	t.Helper()
	if m == nil {
		t.Fatalf("%s: nil mesh without an error", what)
	}
	for i, tr := range m.Tris {
		for _, p := range tr.P {
			if !fzFinite(p.X) || !fzFinite(p.Y) || !fzFinite(p.Z) {
				t.Fatalf("%s: triangle %d has a non-finite vertex %v", what, i, p)
			}
		}
	}
	for i, tr := range m.Clean().Tris {
		if !(tr.Area() > 1e-12) {
			t.Fatalf("%s: Clean() kept triangle %d of area %v", what, i, tr.Area())
		}
	}
}

func fzBinarySTL(tris [][9]float32, header string) []byte {
	var b bytes.Buffer
	h := make([]byte, 80)
	copy(h, header)
	b.Write(h)
	binary.Write(&b, binary.LittleEndian, uint32(len(tris)))
	for _, t := range tris {
		binary.Write(&b, binary.LittleEndian, [3]float32{}) // normal
		binary.Write(&b, binary.LittleEndian, t)
		b.Write([]byte{0, 0})
	}
	return b.Bytes()
}

func FuzzReadSTL(f *testing.F) {
	var buf bytes.Buffer
	WriteSTL(&buf, box(V(0, 0, 0), V(10, 20, 30)), "box")
	f.Add(buf.Bytes())
	f.Add([]byte("solid t\nfacet normal 0 0 1\nouter loop\nvertex 0 0 0\nvertex 1 0 0\nvertex 0 1 0\nendloop\nendfacet\nendsolid t\n"))
	f.Add([]byte("solid t\r\nfacet normal 0 0 1\r\nouter loop\r\nvertex 0 0 0\r\nvertex 1 0 0\r\nvertex 1 1 0\r\nvertex 0 1 0\r\nendloop\r\nendfacet\r\nendsolid\r\n"))
	f.Add([]byte("solid t\nfacet\nvertex 0 0 0\nvertex 0 0 0\nvertex 0 0 0\nendfacet\nendsolid\n"))     // degenerate
	f.Add([]byte("solid t\nfacet\nvertex nan 0 0\nvertex 1 0 0\nvertex 0 1 0\nendfacet\nendsolid\n"))   // NaN
	f.Add([]byte("solid t\nfacet\nvertex 1e400 0 0\nvertex 1 0 0\nvertex 0 1 0\nendfacet\nendsolid\n")) // overflow
	f.Add([]byte("solid t\nfacet\nvertex 1 2\nendfacet\n"))
	f.Add([]byte(""))
	f.Add([]byte("solid"))
	tri := [9]float32{0, 0, 0, 1, 0, 0, 0, 1, 0}
	f.Add(fzBinarySTL([][9]float32{tri, {0, 0, 1, 1, 0, 1, 0, 1, 1}}, "binary"))
	f.Add(fzBinarySTL([][9]float32{tri}, "solid but binary facet")) // header begins with solid
	f.Add(append(fzBinarySTL([][9]float32{tri}, "padded"), make([]byte, 100)...))
	f.Add(fzBinarySTL(nil, "empty"))
	f.Add(fzBinarySTL([][9]float32{{float32(math.NaN()), 0, 0, 1, 0, 0, 0, 1, 0}}, "nan"))
	f.Add(fzBinarySTL([][9]float32{{float32(math.Inf(1)), 0, 0, 1, 0, 0, 0, 1, 0}}, "inf"))
	f.Add(fzBinarySTL([][9]float32{{3e38, 3e38, 3e38, -3e38, 0, 0, 0, -3e38, 3e38}}, "huge"))
	bad := fzBinarySTL([][9]float32{tri}, "count lies")
	binary.LittleEndian.PutUint32(bad[80:], 0xFFFFFFFF)
	f.Add(bad)
	f.Fuzz(func(t *testing.T, data []byte) {
		var m *Mesh
		var err error
		fzWithin(t, 5*time.Second, "ReadSTL", func() { m, err = ReadSTL(data) })
		if err != nil {
			return
		}
		fzCheckMesh(t, "ReadSTL", m)
		for i, tr := range m.Tris {
			if !(tr.Area() > 1e-12) {
				t.Fatalf("ReadSTL returned zero-area triangle %d %v", i, tr.P)
			}
		}
	})
}

func FuzzReadOBJ(f *testing.F) {
	var obj, mtl bytes.Buffer
	WriteOBJ(&obj, &mtl, box(V(1, 2, 3), V(10, 20, 30)), "x.mtl")
	f.Add(obj.Bytes())
	f.Add([]byte("v 0 0 0\nv 1 0 0\nv 1 1 0\nv 0 1 0\nf 1 2 3 4\n"))
	f.Add([]byte("v 0 0 0\nv 1 0 0\nv 0 1 0\nf -3 -2 -1\n"))
	f.Add([]byte("v 0 0 0\r\nv 1 0 0\r\nv 0 1 0\r\nvt 0 0\r\nvn 0 0 1\r\nf 1/1/1 2/1/1 3//1\r\n"))
	f.Add([]byte("# comment\nv 0 0 0 # trailing\nv 1 0 0\nv 0 1 0 1.0\nf 1 2 3 # done\n"))
	f.Add([]byte("v 0 0 0\nv 1 0 0\nf 1 2 3\n")) // out of range
	f.Add([]byte("v 0 0 0\nv 1 0 0\nf 1 2 0\n")) // zero index
	f.Add([]byte("v nan 0 0\nv 1 0 0\nv 0 1 0\nf 1 2 3\n"))
	f.Add([]byte("v 0 0 0\nv 0 0 0\nv 0 0 0\nf 1 2 3\n")) // degenerate
	f.Add([]byte("v 0 0 0\nv 1 0 0\nv 0 1 0\nf 1 2 x\n"))
	f.Add([]byte("v 0 0 0\nv 1 0 0\nv 0 1 0\nf -9999999999999999999 1 2\n"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, data []byte) {
		var m *Mesh
		var err error
		fzWithin(t, 5*time.Second, "ReadOBJ", func() { m, err = ReadOBJ(data) })
		if err != nil {
			return
		}
		fzCheckMesh(t, "ReadOBJ", m)
		for i, tr := range m.Tris {
			if !(tr.Area() > 1e-12) {
				t.Fatalf("ReadOBJ returned zero-area triangle %d %v", i, tr.P)
			}
		}
	})
}

// fzRange maps any float (NaN and infinities included) into [lo, hi].
func fzRange(x, lo, hi float64) float64 {
	if math.IsNaN(x) {
		return lo
	}
	if math.IsInf(x, 0) {
		if x > 0 {
			return hi
		}
		return lo
	}
	// fold big values into the range so that the fuzzer's mutations of
	// either size still land somewhere useful
	if x < lo || x > hi {
		w := hi - lo
		x = lo + math.Mod(math.Abs(x-lo), w)
	}
	return x
}

// fzBox is a box at c of size s, turned by ang about axis.
func fzBox(cx, cy, cz, sx, sy, sz, ang float64, axis uint8) *Mesh {
	c := V(fzRange(cx, -30, 30), fzRange(cy, -30, 30), fzRange(cz, -30, 30))
	s := V(fzRange(sx, 0.5, 50), fzRange(sy, 0.5, 50), fzRange(sz, 0.5, 50))
	r := RotateAxis(int(axis%3), fzRange(ang, -7, 7))
	return box(V(0, 0, 0), s).Transform(Translate(c).Mul(r))
}

func FuzzFixTJunctions(f *testing.F) {
	f.Add(0.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 10.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), uint8(0))
	f.Add(0.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 10.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), uint8(1))
	f.Add(0.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.3, uint8(2), 5.0, 5.0, 10.0, 10.0, 10.0, 30.0, 0.7, uint8(1), uint8(1))
	f.Add(0.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 0.0, 0.0, 15.0, 10.0, 10.0, 10.0, 0.0, uint8(0), uint8(1))  // coplanar top
	f.Add(0.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 20.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), uint8(1))  // touching faces
	f.Add(0.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 10.0, 10.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), uint8(0)) // edge contact, merge
	f.Fuzz(func(t *testing.T, ax, ay, az, asx, asy, asz, aang float64, aaxis uint8,
		bx, by, bz, bsx, bsy, bsz, bang float64, baxis uint8, mode uint8) {
		a := fzBox(ax, ay, az, asx, asy, asz, aang, aaxis)
		b := fzBox(bx, by, bz, bsx, bsy, bsz, bang, baxis)
		var in *Mesh
		switch mode % 4 {
		case 0:
			in = Merge(a, b) // two closed boxes: closed
		case 1:
			in = Union(a, b)
		case 2:
			in = Subtract(a, b)
		case 3:
			in = Intersect(a, b)
		}
		var out *Mesh
		fzWithin(t, 20*time.Second, "FixTJunctions", func() { out = in.FixTJunctions() })
		if len(in.Tris) == 0 {
			return
		}
		fzCheckMesh(t, "FixTJunctions", out)
		openIn, _ := in.EdgeCheck(1e-6)
		openOut, _ := out.EdgeCheck(1e-6)
		if openOut > openIn {
			t.Fatalf("FixTJunctions (mode %d) raised open edges from %d to %d", mode%4, openIn, openOut)
		}
		if mode%4 == 0 && openOut != 0 {
			t.Fatalf("two closed boxes merged have %d open edges after FixTJunctions", openOut)
		}
	})
}

func FuzzSlice(f *testing.F) {
	f.Add(0.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 10.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 0.5)
	f.Add(0.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 10.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 0.0)   // at the bottom face
	f.Add(0.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 10.0, 0.0, 10.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 0.75) // at a face
	f.Add(0.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.785, uint8(0), 10.0, 10.0, 0.0, 20.0, 20.0, 20.0, 0.785, uint8(1), 0.5)
	f.Add(0.0, 0.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 20.0, 20.0, 0.0, 20.0, 20.0, 20.0, 0.0, uint8(0), 0.5) // touching at an edge
	f.Fuzz(func(t *testing.T, ax, ay, az, asx, asy, asz, aang float64, aaxis uint8,
		bx, by, bz, bsx, bsy, bsz, bang float64, baxis uint8, frac float64) {
		a := fzBox(ax, ay, az, asx, asy, asz, aang, aaxis)
		b := fzBox(bx, by, bz, bsx, bsy, bsz, bang, baxis)
		m := Union(a, b)
		bb := m.Bounds()
		// heights from a little below the solid to a little above
		h := bb.Min.Z + (bb.Max.Z-bb.Min.Z)*fzRange(frac, -0.1, 1.1)
		if math.IsNaN(frac) || math.IsInf(frac, 0) {
			h = frac
		}
		var loops [][]Vec3
		fzWithin(t, 10*time.Second, "Slice", func() { loops = Slice(m, h) })
		for i, l := range loops {
			if len(l) < 3 {
				t.Fatalf("loop %d has %d points", i, len(l))
			}
			for _, p := range l {
				if !fzFinite(p.X) || !fzFinite(p.Y) {
					t.Fatalf("loop %d has a non-finite point %v", i, p)
				}
			}
		}
		if len(loops) > 0 { // (with none, WriteSVG rightly says there's nothing to draw)
			var buf bytes.Buffer
			if err := WriteSVG(&buf, loops); err != nil {
				t.Fatal(err)
			}
		}
	})
}
