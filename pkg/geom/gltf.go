package geom

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
)

// WriteGLB writes m as binary glTF 2.0: one primitive per colour, each with
// its own material, flat-shaded (three vertices and a face normal per
// triangle). glTF is in metres with Y up, so millimetres are scaled by
// 0.001 and ScottiCAD's Z-up axes turned: (x, y, z) -> (x, z, -y).
func WriteGLB(w io.Writer, m *Mesh, name string) error {
	// zero-area triangles have no normal (glTF requires unit normals); an
	// empty file isn't valid glTF at all
	m = m.Clean()
	if len(m.Tris) == 0 {
		return errors.New("nothing to export: the mesh is empty")
	}
	toGL := func(v Vec3) [3]float32 {
		return [3]float32{float32(v.X * 0.001), float32(v.Z * 0.001), float32(-v.Y * 0.001)}
	}
	byColor := map[Color][]Tri{}
	var colors []Color
	for _, t := range m.Tris {
		if _, ok := byColor[t.C]; !ok {
			colors = append(colors, t.C)
		}
		byColor[t.C] = append(byColor[t.C], t)
	}

	type accessor struct {
		BufferView    int       `json:"bufferView"`
		ComponentType int       `json:"componentType"`
		Count         int       `json:"count"`
		Type          string    `json:"type"`
		Min           []float32 `json:"min,omitempty"`
		Max           []float32 `json:"max,omitempty"`
	}
	type bufferView struct {
		Buffer     int `json:"buffer"`
		ByteOffset int `json:"byteOffset"`
		ByteLength int `json:"byteLength"`
		Target     int `json:"target"`
	}
	type primitive struct {
		Attributes map[string]int `json:"attributes"`
		Material   int            `json:"material"`
	}
	type material struct {
		Name string         `json:"name"`
		PBR  map[string]any `json:"pbrMetallicRoughness"`
	}
	var (
		bin      bytes.Buffer
		views    []bufferView
		accs     []accessor
		prims    []primitive
		mats     []material
		f32      = func(v float32) { binary.Write(&bin, binary.LittleEndian, v) }
		addFloat = func(data [][3]float32, withBounds bool) int {
			off := bin.Len()
			mn := [3]float32{float32(math.Inf(1)), float32(math.Inf(1)), float32(math.Inf(1))}
			mx := [3]float32{float32(math.Inf(-1)), float32(math.Inf(-1)), float32(math.Inf(-1))}
			for _, v := range data {
				for k := 0; k < 3; k++ {
					f32(v[k])
					mn[k] = float32(math.Min(float64(mn[k]), float64(v[k])))
					mx[k] = float32(math.Max(float64(mx[k]), float64(v[k])))
				}
			}
			views = append(views, bufferView{Buffer: 0, ByteOffset: off, ByteLength: bin.Len() - off, Target: 34962})
			a := accessor{BufferView: len(views) - 1, ComponentType: 5126, Count: len(data), Type: "VEC3"}
			if withBounds {
				a.Min, a.Max = mn[:], mx[:]
			}
			accs = append(accs, a)
			return len(accs) - 1
		}
	)
	// glTF colours are linear; ours are sRGB
	lin := func(c float32) float64 {
		v := float64(c)
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	for i, c := range colors {
		var pos, nrm [][3]float32
		for _, t := range byColor[c] {
			n := t.Normal()
			gn := [3]float32{float32(n.X), float32(n.Z), float32(-n.Y)}
			for _, p := range t.P {
				pos = append(pos, toGL(p))
				nrm = append(nrm, gn)
			}
		}
		pa := addFloat(pos, true)
		na := addFloat(nrm, false)
		prims = append(prims, primitive{Attributes: map[string]int{"POSITION": pa, "NORMAL": na}, Material: i})
		mats = append(mats, material{
			Name: "color_" + itoa(i),
			PBR: map[string]any{
				"baseColorFactor": []float64{lin(c.R), lin(c.G), lin(c.B), 1},
				"metallicFactor":  0,
				"roughnessFactor": 0.6,
			},
		})
	}
	if name == "" {
		name = "ScottiCAD"
	}
	doc := map[string]any{
		"asset":       map[string]any{"version": "2.0", "generator": "ScottiCAD"},
		"scene":       0,
		"scenes":      []any{map[string]any{"nodes": []int{0}}},
		"nodes":       []any{map[string]any{"mesh": 0, "name": name}},
		"meshes":      []any{map[string]any{"name": name, "primitives": prims}},
		"materials":   mats,
		"accessors":   accs,
		"bufferViews": views,
		"buffers":     []any{map[string]any{"byteLength": bin.Len()}},
	}
	js, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	for len(js)%4 != 0 {
		js = append(js, ' ')
	}
	for bin.Len()%4 != 0 {
		bin.WriteByte(0)
	}
	total := 12 + 8 + len(js) + 8 + bin.Len()
	var out bytes.Buffer
	u32 := func(v uint32) { binary.Write(&out, binary.LittleEndian, v) }
	u32(0x46546C67) // "glTF"
	u32(2)
	u32(uint32(total))
	u32(uint32(len(js)))
	u32(0x4E4F534A) // "JSON"
	out.Write(js)
	u32(uint32(bin.Len()))
	u32(0x004E4942) // "BIN"
	out.Write(bin.Bytes())
	_, err = w.Write(out.Bytes())
	return err
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}
