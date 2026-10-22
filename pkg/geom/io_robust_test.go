package geom

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"
	"time"
)

// Files that would bring infinite or unreadable numbers in, or shift faces
// onto the wrong vertices, are refused rather than imported wrong.
func TestImportRefusesBadNumbers(t *testing.T) {
	for name, src := range map[string]string{
		"stl inf":   "solid x\nfacet normal 0 0 1\nouter loop\nvertex 1 1 1\nvertex 1e400 2 3\nvertex 4 5 6\nendloop\nendfacet\nendsolid x\n",
		"stl junk":  "solid x\nfacet normal 0 0 1\nouter loop\nvertex 1 1 1\nvertex a 2 3\nvertex 4 5 6\nendloop\nendfacet\nendsolid x\n",
		"obj inf":   "v inf 2 3\nv 1 0 0\nv 0 1 0\nf 1 2 3\n",
		"obj short": "v 0 0 0\nv 1 0\nv 1 0 0\nv 0 1 0\nf 1 2 3\n",
		"obj face":  "v 0 0 0\nv 1 0 0\nv 0 1 0\nf 1/x x 3\n",
	} {
		var err error
		if strings.HasPrefix(name, "stl") {
			_, err = ReadSTL([]byte(src))
		} else {
			_, err = ReadOBJ([]byte(src))
		}
		if err == nil {
			t.Errorf("%s: imported", name)
		}
	}
}

// A very long line (a comment, say) doesn't end the file early.
func TestImportLongLines(t *testing.T) {
	src := "v 0 0 0\nv 1 0 0\nv 0 1 0\nv 0 0 1\nf 1 2 3\n# " + strings.Repeat("x", 2<<20) + "\nf 1 2 4\nf 1 3 4\nf 2 3 4\n"
	m, err := ReadOBJ([]byte(src))
	if err != nil || len(m.Tris) != 4 {
		t.Fatalf("%v triangles, err %v", len(m.Tris), err)
	}
}

// A binary STL whose header begins "solid", or with padding after its
// triangles, is still read as binary.
func TestBinarySTLVariants(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteSTL(&buf, cube(10, Color{}), ""); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	copy(b, "solid exported by something")
	for name, data := range map[string][]byte{"solid header": b, "padded": append(append([]byte{}, b...), 0, 0, 0, 0)} {
		m, err := ReadSTL(data)
		if err != nil || len(m.Tris) != 12 || math.Abs(m.Volume()-1000) > 1e-3 {
			t.Errorf("%s: err %v", name, err)
		}
	}
	// a binary triangle with a NaN is refused
	c := append([]byte{}, b...)
	binary.LittleEndian.PutUint32(c[84+12:], math.Float32bits(float32(math.NaN())))
	if _, err := ReadSTL(c); err == nil {
		t.Error("NaN coordinate imported")
	}
}

// T-junction repair on long edges running along an axis stays fast: it
// must not look at every vertex beside every such edge.
func TestTJunctionsOnLongEdgesAreFast(t *testing.T) {
	m := &Mesh{}
	for i := 0; i < 8000; i++ {
		y := float64(i)
		m.Tris = append(m.Tris, Tri{P: [3]Vec3{V(0, y, 0), V(100, y, 0), V(50, y+0.5, 0)}})
	}
	start := time.Now()
	m.FixTJunctions()
	if d := time.Since(start); d > 300*time.Millisecond {
		t.Fatalf("took %v", d)
	}
}
