package editor

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// ImportOptions are the Import dialog's settings.
type ImportOptions struct {
	Scale  float64 // multiplier (1 = 100 %)
	Height float64 // SVG extrusion height in mm
	// Units of the file for STL/OBJ: "mm" (default) or "in".
	Units string
}

func DefaultImportOptions() ImportOptions { return ImportOptions{Scale: 1, Height: 10, Units: "mm"} }

// LoadMesh reads an STL, OBJ or SVG file into a mesh in millimetres.
func LoadMesh(path string, o ImportOptions) (m *geom.Mesh, err error) {
	// a file is outside data: a reader's bug it triggers is an error for
	// that file, not the end of the program (and the design open in it)
	defer func() {
		if r := recover(); r != nil {
			m, err = nil, fmt.Errorf("couldn't read %s: %v", filepath.Base(path), r)
		}
	}()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// NaN and infinity too (a script or a hand-made call can pass them)
	if !(o.Scale > 0) || math.IsInf(o.Scale, 0) {
		o.Scale = 1
	}
	if !(o.Height > 0) || math.IsInf(o.Height, 0) {
		o.Height = 10
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".stl":
		m, err = geom.ReadSTL(data)
	case ".obj":
		m, err = geom.ReadOBJ(data)
	case ".svg":
		return shapes.SVGMesh(bytes.NewReader(data), o.Height, o.Scale)
	default:
		return nil, fmt.Errorf("unsupported file type %q (use .stl, .obj or .svg)", filepath.Ext(path))
	}
	if err != nil {
		return nil, err
	}
	if len(m.Tris) == 0 {
		return nil, fmt.Errorf("%s contains no triangles", filepath.Base(path))
	}
	// an inside-out file (every face wound the wrong way, as some programs
	// write them) is turned right side out: inside out it would cut where
	// it should add, and weigh less than nothing. Its signed volume is
	// taken about its own centre: about the origin, a file with a missing
	// face far from the origin (common in CAD exports) can measure negative
	// though it's wound right, the missing face's share outweighing the
	// part.
	if c := m.Bounds().Center(); m.Transform(geom.Translate(c.Neg())).Volume() < 0 {
		m = m.Flipped()
	}
	s := o.Scale
	if o.Units == "in" {
		s *= 25.4
	}
	if s != 1 {
		m = m.Transform(geom.Scale(geom.V(s, s, s)))
	}
	b := m.Bounds()
	far := 0.0
	for _, v := range []float64{b.Min.X, b.Min.Y, b.Min.Z, b.Max.X, b.Max.Y, b.Max.Z} {
		far = math.Max(far, math.Abs(v))
	}
	if !(far <= shapes.MaxImportSize) { // NaN too
		return nil, fmt.Errorf("%s is far too large (a coordinate over 10 km): check its units or scale", filepath.Base(path))
	}
	return m, nil
}

// Import loads a file and places its bottom centre at world (pos.X, pos.Y)
// at the workplane's height.
func (e *Editor) Import(path string, pos geom.Vec3, o ImportOptions) (int, error) {
	return e.ImportXf(path, geom.Translate(geom.V(pos.X, pos.Y, e.PlaneZ())), o)
}

// ImportOn loads a file and stands it on the workplane at q (workplane
// coordinates), so on a workplane attached to a shape it follows that
// shape's axes.
func (e *Editor) ImportOn(path string, q geom.Vec3, o ImportOptions) (int, error) {
	return e.ImportXf(path, e.Work().Mul(geom.Translate(geom.V(q.X, q.Y, 0))), o)
}

// ImportXf loads a file, puts its bottom centre at the origin and applies
// xf.
func (e *Editor) ImportXf(path string, xf geom.Mat4, o ImportOptions) (int, error) {
	m, err := LoadMesh(path, o)
	if err != nil {
		return 0, err
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return e.AddMeshXf(name, m, xf)
}
