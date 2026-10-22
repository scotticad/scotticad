//go:build windows

package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"runtime"
	"slices"
	"strings"

	"github.com/scotticad/scotticad"
	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/filament"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/script"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/win"
)

// The automation API is JSON-RPC 2.0 over a localhost TCP socket, one JSON
// object per line. Every call runs on the UI thread between frames, so it
// sees exactly what the GUI sees. "describe" returns the method table.

type apiCall struct {
	method string
	params args
	reply  chan apiResult
	do     func() // instead of a method: run this on the UI thread
}

type apiResult struct {
	result any
	err    error
}

// args are a call's named parameters.
type args map[string]any

func (p args) has(k string) bool { _, ok := p[k]; return ok }

func (p args) f(k string, def float64) float64 {
	switch v := p[k].(type) {
	case float64:
		return v
	case string:
		var f float64
		if _, err := fmt.Sscan(v, &f); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return f
		}
	case bool:
		if v {
			return 1
		}
		return 0
	}
	return def
}

func (p args) s(k, def string) string {
	if v, ok := p[k].(string); ok {
		return v
	}
	return def
}

func (p args) b(k string, def bool) bool {
	if v, ok := p[k].(bool); ok {
		return v
	}
	return def
}

func (p args) i(k string, def int) int { return int(math.Round(p.f(k, float64(def)))) }

func (p args) ids(k string) []int {
	v, ok := p[k].([]any)
	if !ok {
		if f, ok := p[k].(float64); ok {
			return []int{int(f)}
		}
		return nil
	}
	out := []int{}
	for _, x := range v {
		if f, ok := x.(float64); ok {
			out = append(out, int(f))
		}
	}
	return out
}

func (p args) vec(k string) (geom.Vec3, bool) {
	v, ok := p[k].([]any)
	if !ok || len(v) != 3 {
		return geom.Vec3{}, false
	}
	var f [3]float64
	for i, x := range v {
		f[i], _ = x.(float64)
	}
	return geom.V(f[0], f[1], f[2]), true
}

// contoursArg reads an outline's points: one contour as [[x, y], ...], or
// several (holes inside others) as [[[x, y], ...], ...].
func contoursArg(v any, what string) ([][][2]float64, error) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return nil, fmt.Errorf("%s must be a list of [x, y] points", what)
	}
	if inner, ok := list[0].([]any); ok && len(inner) > 0 {
		if _, nested := inner[0].([]any); nested {
			var out [][][2]float64
			for i, c := range list {
				pts, err := pointsArg(c, fmt.Sprintf("%s outline %d", what, i+1))
				if err != nil {
					return nil, err
				}
				out = append(out, pts)
			}
			return out, nil
		}
	}
	pts, err := pointsArg(v, what)
	if err != nil {
		return nil, err
	}
	return [][][2]float64{pts}, nil
}

func pointsArg(v any, what string) ([][2]float64, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list of [x, y] points", what)
	}
	out := make([][2]float64, len(list))
	for i, p := range list {
		xy, ok := p.([]any)
		if !ok || len(xy) != 2 {
			return nil, fmt.Errorf("%s: point %d must be [x, y]", what, i+1)
		}
		for k := 0; k < 2; k++ {
			f, ok := xy[k].(float64)
			if !ok {
				return nil, fmt.Errorf("%s: point %d must be two numbers", what, i+1)
			}
			out[i][k] = f
		}
	}
	return out, nil
}

// sectionsArg reads a loft's sections: [[z, points], ...] (or
// [{"z": z, "points": points}, ...]), one outline each.
func sectionsArg(v any) ([]shapes.Section, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, errors.New("sections must be a list of [z, points]")
	}
	var out []shapes.Section
	for i, s := range list {
		var z, pts any
		switch s := s.(type) {
		case []any:
			if len(s) != 2 {
				return nil, fmt.Errorf("section %d must be [z, points]", i+1)
			}
			z, pts = s[0], s[1]
		case map[string]any:
			z, pts = s["z"], s["points"]
		default:
			return nil, fmt.Errorf("section %d must be [z, points]", i+1)
		}
		zf, ok := z.(float64)
		if !ok {
			return nil, fmt.Errorf("section %d: z must be a number", i+1)
		}
		cs, err := contoursArg(pts, fmt.Sprintf("section %d", i+1))
		if err != nil {
			return nil, err
		}
		out = append(out, shapes.Section{Z: zf, Contours: cs})
	}
	return out, nil
}

// outlineSections makes an outline's bottom and top sections.
func outlineSections(cs [][][2]float64, z0, z1 float64) []shapes.Section {
	return []shapes.Section{{Z: z0, Contours: cs}, {Z: z1, Contours: cs}}
}

func axisArg(p args, k string) (int, error) {
	switch v := p[k].(type) {
	case string:
		switch strings.ToLower(v) {
		case "x":
			return 0, nil
		case "y":
			return 1, nil
		case "z":
			return 2, nil
		}
	case float64:
		if v >= 0 && v <= 2 {
			return int(v), nil
		}
	}
	return 0, fmt.Errorf("%s must be \"x\", \"y\" or \"z\"", k)
}

func whereArg(p args) (int, error) {
	switch p.s("where", "") {
	case "min":
		return -1, nil
	case "center", "centre", "mid":
		return 0, nil
	case "max":
		return 1, nil
	}
	return 0, errors.New(`where must be "min", "center" or "max"`)
}

type apiParam struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Doc      string `json:"doc"`
	Optional bool   `json:"optional,omitempty"`
}

type apiMethod struct {
	Name   string     `json:"name"`
	Doc    string     `json:"doc"`
	Params []apiParam `json:"params"`
	fn     func(a *App, p args) (any, error)
}

func req(name, typ, d string) apiParam { return apiParam{name, typ, d, false} }
func opt(name, typ, d string) apiParam { return apiParam{name, typ, d, true} }

var idsParam = opt("ids", "int[]", "top-level node ids; defaults to the selection")

// errNoSize: the shapes combine to nothing (an intersect group of shapes
// that don't meet), so they have no bounds to place or size by.
var errNoSize = errors.New("the selection has no size: its shapes combine to nothing")

func boundsJSON(b geom.Box) map[string]any {
	if b.IsEmpty() {
		return nil
	}
	return map[string]any{"min": []float64{b.Min.X, b.Min.Y, b.Min.Z}, "max": []float64{b.Max.X, b.Max.Y, b.Max.Z},
		"size": []float64{b.Size().X, b.Size().Y, b.Size().Z}, "center": []float64{b.Center().X, b.Center().Y, b.Center().Z}}
}

func nodeJSON(n *doc.Node, deep bool, top bool) map[string]any {
	m := map[string]any{"id": n.ID, "kind": n.Kind, "label": n.Label(), "name": n.Name, "hole": n.Hole, "color": n.Color,
		"transparent": n.Transparent, "hidden": n.Hidden, "locked": n.Locked, "size": []float64{n.Size.X, n.Size.Y, n.Size.Z}}
	if n.IsGroup() {
		m["group_mode"] = n.Mode()
		m["multicolor"] = n.Multicolor
		m["child_count"] = len(n.Children)
		if n.Assembly != "" {
			m["assembly"] = n.Assembly
		}
	}
	if len(n.Params) > 0 {
		m["params"] = n.Params
	}
	if n.Kind == "text" {
		m["text"], m["font"] = n.Text, n.Font
	}
	if len(n.Sections) > 0 {
		m["sections"] = n.Sections
	}
	if top {
		m["bounds"] = boundsJSON(n.WorldBounds())
		v := n.WorldMesh(false).Volume()
		m["volume"] = v
	}
	if deep && n.IsGroup() {
		var cs []any
		for _, c := range n.Children {
			cs = append(cs, nodeJSON(c, true, false))
		}
		m["children"] = cs
	}
	return m
}

func (a *App) nodesArg(p args) []int {
	if p.has("ids") {
		return p.ids("ids")
	}
	return nil
}

// withSel runs f on the call's ids by temporarily selecting them (on the
// selection when none are given).
func (a *App) withSel(p args, f func()) {
	if ids := a.nodesArg(p); ids != nil {
		saved := a.ed.Sel
		a.ed.Sel = ids
		f()
		a.ed.Select(saved...) // drops deleted and hidden shapes
		return
	}
	f()
}

var apiMethods []apiMethod

func init() {
	ok := func() (any, error) { return map[string]any{"ok": true}, nil }
	apiMethods = []apiMethod{
		// --- design files
		{Name: "describe", Doc: "List every API method with its parameters.", fn: func(a *App, p args) (any, error) { return apiMethods, nil }},
		{Name: "new_design", Doc: "Start a new empty design (discards unsaved changes).", fn: func(a *App, p args) (any, error) {
			a.ed.NewDesign()
			a.homeCamera(false)
			return ok()
		}},
		{Name: "open", Doc: "Open a .scotticad design.", Params: []apiParam{req("path", "string", "file path")}, fn: func(a *App, p args) (any, error) {
			if err := a.ed.Open(p.s("path", "")); err != nil {
				return nil, err
			}
			a.restoreCamera()
			a.noteRecent(a.ed.Path)
			return ok()
		}},
		{Name: "save", Doc: "Save the design (native .scotticad JSON with the full group tree).", Params: []apiParam{opt("path", "string", "file path; defaults to the current file")},
			fn: func(a *App, p args) (any, error) {
				a.storeCamera()
				if err := a.ed.Save(p.s("path", "")); err != nil {
					return nil, err
				}
				a.noteRecent(a.ed.Path)
				return map[string]any{"path": a.ed.Path}, nil
			}},
		{Name: "get_document", Doc: "Return the design as native JSON.", fn: func(a *App, p args) (any, error) {
			b, err := a.ed.Doc.Marshal()
			return json.RawMessage(b), err
		}},
		{Name: "get_state", Doc: "Design and editor state summary.", fn: func(a *App, p args) (any, error) {
			s := a.ed.Doc.Settings
			st := map[string]any{"name": a.designName(), "path": a.ed.Path, "dirty": a.ed.Dirty(), "settings": s,
				"selection": a.ed.Sel, "can_undo": a.ed.Hist.CanUndo(), "can_redo": a.ed.Hist.CanRedo(),
				"undo_depth": a.ed.Hist.Depth(), "node_count": len(a.ed.Doc.Nodes), "selection_bounds": boundsJSON(a.ed.SelectionBounds())}
			if a.ed.Ruler != nil {
				st["ruler"] = a.ed.Ruler
			}
			if a.ed.Workplane != nil {
				st["construction_plane"] = a.ed.Workplane[:]
			}
			return st, nil
		}},
		{Name: "list_nodes", Doc: "Top-level nodes with bounds and volume.", Params: []apiParam{opt("deep", "bool", "include children trees")},
			fn: func(a *App, p args) (any, error) {
				out := []any{}
				for _, n := range a.ed.Doc.Nodes {
					out = append(out, nodeJSON(n, p.b("deep", false), true))
				}
				return out, nil
			}},
		{Name: "get_node", Doc: "One node (any depth) with its children tree.", Params: []apiParam{req("id", "int", "")}, fn: func(a *App, p args) (any, error) {
			id := p.i("id", 0)
			if n := a.ed.Doc.Find(id); n != nil {
				return nodeJSON(n, true, true), nil
			}
			if n := a.ed.Node(id); n != nil {
				return nodeJSON(n, true, false), nil
			}
			return nil, fmt.Errorf("no node %d", id)
		}},
		{Name: "shape_kinds", Doc: "The shape catalogue with parameters.", fn: func(a *App, p args) (any, error) {
			var out []any
			for _, k := range shapes.All() {
				var ps []any
				for _, pr := range k.Params {
					ps = append(ps, map[string]any{"key": pr.Key, "label": pr.Label, "kind": []string{"slider", "number", "text", "font"}[pr.Kind],
						"min": pr.Min, "max": pr.Max, "step": pr.Step, "default": pr.Default, "int": pr.Int, "length": pr.Length, "metric": pr.Metric})
				}
				out = append(out, map[string]any{"id": k.ID, "label": k.Label, "color": k.Color, "size": []float64{k.Size.X, k.Size.Y, k.Size.Z}, "params": ps})
			}
			return out, nil
		}},
		{Name: "version", Doc: "ScottiCAD's version (from the VERSION file).", fn: func(a *App, p args) (any, error) { return scotticad.Version(), nil }},
		{Name: "fonts", Doc: "Font names usable by text shapes.", fn: func(a *App, p args) (any, error) { return shapes.FontNames(), nil }},

		// --- creation
		{Name: "add_shape", Doc: "Place a catalogue shape with its bottom centre at (x, y, z); selects it.",
			Params: []apiParam{req("kind", "string", "shape id, see shape_kinds"), opt("x", "float", "mm"), opt("y", "float", "mm"), opt("z", "float", "mm; default: on the current plane"), opt("hole", "bool", "")},
			fn: func(a *App, p args) (any, error) {
				z := p.f("z", a.ed.PlaneZ())
				id, err := a.ed.AddShape(p.s("kind", ""), geom.V(p.f("x", 0), p.f("y", 0), z), p.b("hole", false))
				return map[string]any{"id": id}, err
			}},
		{Name: "add_outline", Doc: "Extrude closed outlines straight up from z0 to z1 (outlines inside others are holes); points are world mm. Selects it.",
			Params: []apiParam{req("points", "float[][2] or float[][][2]", "[[x, y], ...], or a list of such outlines"),
				opt("z0", "float", "mm, default 0"), opt("z1", "float", "mm, default 10"), opt("hole", "bool", "")},
			fn: func(a *App, p args) (any, error) {
				cs, err := contoursArg(p["points"], "points")
				if err != nil {
					return nil, err
				}
				id, err := a.ed.AddProfile("outline", outlineSections(cs, p.f("z0", 0), p.f("z1", 10)), p.b("hole", false))
				return map[string]any{"id": id}, err
			}},
		{Name: "add_loft", Doc: "Join closed outlines at rising heights with straight walls and close the ends; points are world mm. Selects it.",
			Params: []apiParam{req("sections", "[z, float[][2]][]", "[[z, [[x, y], ...]], ...], at least 2, z rising; same point counts join point to point"),
				opt("hole", "bool", "")},
			fn: func(a *App, p args) (any, error) {
				ss, err := sectionsArg(p["sections"])
				if err != nil {
					return nil, err
				}
				id, err := a.ed.AddProfile("loft", ss, p.b("hole", false))
				return map[string]any{"id": id}, err
			}},
		{Name: "set_outline", Doc: "Replace an outline's points (and heights); they are in the shape's own frame, so an unmoved shape puts them where they say.",
			Params: []apiParam{req("id", "int", "node id (any depth)"), req("points", "float[][2] or float[][][2]", "as add_outline"),
				opt("z0", "float", "mm, default unchanged"), opt("z1", "float", "mm, default unchanged")},
			fn: func(a *App, p args) (any, error) {
				n := a.ed.Doc.FindAny(p.i("id", 0))
				if n == nil || n.Kind != "outline" || len(n.Sections) != 2 {
					return nil, fmt.Errorf("node %d is not an outline", p.i("id", 0))
				}
				cs, err := contoursArg(p["points"], "points")
				if err != nil {
					return nil, err
				}
				ss := outlineSections(cs, p.f("z0", n.Sections[0].Z), p.f("z1", n.Sections[1].Z))
				return ok2(a.ed.SetSections(n.ID, ss))
			}},
		{Name: "set_loft", Doc: "Replace a loft's sections; they are in the shape's own frame, so an unmoved shape puts them where they say.",
			Params: []apiParam{req("id", "int", "node id (any depth)"), req("sections", "[z, float[][2]][]", "as add_loft")},
			fn: func(a *App, p args) (any, error) {
				n := a.ed.Doc.FindAny(p.i("id", 0))
				if n == nil || n.Kind != "loft" {
					return nil, fmt.Errorf("node %d is not a loft", p.i("id", 0))
				}
				ss, err := sectionsArg(p["sections"])
				if err != nil {
					return nil, err
				}
				return ok2(a.ed.SetSections(n.ID, ss))
			}},
		// --- notes
		{Name: "add_note", Doc: "Pin a note to top-level shape id at the world point (x, y, z); returns its index on that shape.",
			Params: []apiParam{req("id", "int", ""), req("x", "float", "mm"), req("y", "float", "mm"), req("z", "float", "mm"), opt("text", "string", "")},
			fn: func(a *App, p args) (any, error) {
				i, err := a.ed.AddNote(p.i("id", 0), geom.V(p.f("x", 0), p.f("y", 0), p.f("z", 0)), p.s("text", ""))
				return map[string]any{"index": i}, err
			}},
		{Name: "notes", Doc: "Every note: shape id, index on that shape, text, world position, and whether it is open.",
			fn: func(a *App, p args) (any, error) {
				out := []any{}
				o := a.openNoteRef()
				for _, r := range a.ed.Notes() {
					out = append(out, map[string]any{"id": r.Node, "index": r.Index, "text": r.Text,
						"world": []float64{r.World.X, r.World.Y, r.World.Z}, "hidden": r.Hidden,
						"open": o != nil && o.Node == r.Node && o.Index == r.Index})
				}
				return out, nil
			}},
		{Name: "set_note_text", Doc: "Change a note's text.", Params: []apiParam{req("id", "int", ""), req("index", "int", ""), req("text", "string", "")},
			fn: func(a *App, p args) (any, error) {
				return ok2(a.ed.SetNoteText(p.i("id", 0), p.i("index", 0), p.s("text", "")))
			}},
		{Name: "delete_note", Doc: "Remove a note.", Params: []apiParam{req("id", "int", ""), req("index", "int", "")},
			fn: func(a *App, p args) (any, error) { return ok2(a.ed.DeleteNote(p.i("id", 0), p.i("index", 0))) }},

		{Name: "import", Doc: "Import an STL, OBJ or SVG file onto the current plane; selects it.",
			Params: []apiParam{req("path", "string", ""), opt("x", "float", "mm"), opt("y", "float", "mm"), opt("scale", "float", "multiplier, default 1"),
				opt("units", "string", "mm or in (STL/OBJ)"), opt("height", "float", "SVG extrusion height, mm")},
			fn: func(a *App, p args) (any, error) {
				o := editor.ImportOptions{Scale: p.f("scale", 1), Units: p.s("units", "mm"), Height: p.f("height", 10)}
				id, err := a.ed.Import(p.s("path", ""), geom.V(p.f("x", 0), p.f("y", 0), 0), o)
				return map[string]any{"id": id}, err
			}},
		{Name: "export", Doc: "Export evaluated geometry to .stl, .obj, .3mf or .glb, or its base outline to .svg (laser cutting).", Params: []apiParam{req("path", "string", ""), opt("selection_only", "bool", "export only the selection")},
			fn: func(a *App, p args) (any, error) {
				return ok2(a.ed.Export(p.s("path", ""), p.b("selection_only", false)))
			}},

		// --- selection
		{Name: "select", Doc: "Replace the selection.", Params: []apiParam{req("ids", "int[]", "")}, fn: func(a *App, p args) (any, error) {
			a.ed.Select(p.ids("ids")...)
			return a.ed.Sel, nil
		}},
		{Name: "add_to_selection", Doc: "Add a shape to the selection (already selected: no change); returns the selection.", Params: []apiParam{req("id", "int", "")}, fn: func(a *App, p args) (any, error) {
			id := p.i("id", 0)
			if a.ed.Doc.Find(id) == nil {
				return nil, fmt.Errorf("no shape %d", id)
			}
			if !a.ed.IsSelected(id) {
				a.ed.Select(append(slices.Clone(a.ed.Sel), id)...)
			}
			return append([]int{}, a.ed.Sel...), nil
		}},
		{Name: "select_all", Doc: "Select every visible shape.", fn: func(a *App, p args) (any, error) { a.ed.SelectAll(); return a.ed.Sel, nil }},
		{Name: "clear_selection", Doc: "Deselect everything.", fn: func(a *App, p args) (any, error) { a.ed.ClearSelection(); return ok() }},
		{Name: "get_selection", Doc: "Selected ids and their combined bounds.", fn: func(a *App, p args) (any, error) {
			return map[string]any{"ids": a.ed.Sel, "bounds": boundsJSON(a.ed.SelectionBounds())}, nil
		}},

		// --- transforms
		{Name: "move", Doc: "Translate by (dx, dy, dz) mm.", Params: []apiParam{opt("dx", "float", ""), opt("dy", "float", ""), opt("dz", "float", ""), idsParam},
			fn: func(a *App, p args) (any, error) {
				a.ed.Move(a.nodesArg(p), geom.V(p.f("dx", 0), p.f("dy", 0), p.f("dz", 0)))
				return ok()
			}},
		{Name: "set_position", Doc: "Move so the bounding box's anchor point lands at (x, y, z); omitted axes are unchanged.",
			Params: []apiParam{opt("x", "float", ""), opt("y", "float", ""), opt("z", "float", ""), opt("anchor", "string", `"min" (default) or "center"`), idsParam},
			fn: func(a *App, p args) (any, error) {
				var ns []*doc.Node
				if ids := a.nodesArg(p); ids != nil {
					ns = a.ed.Doc.FindAll(ids)
				} else {
					ns = a.ed.Selected()
				}
				if len(ns) == 0 {
					return nil, errors.New("nothing selected")
				}
				b := doc.Bounds(ns)
				if b.IsEmpty() {
					return nil, errNoSize
				}
				ref := b.Min
				if p.s("anchor", "min") == "center" {
					ref = b.Center()
				}
				d := geom.Vec3{}
				for i, k := range []string{"x", "y", "z"} {
					if p.has(k) {
						d = d.WithAxis(i, p.f(k, 0)-ref.Axis(i))
					}
				}
				a.ed.Move(doc.IDs(ns), d)
				return ok()
			}},
		{Name: "set_size", Doc: "Resize the bounding box (mm); omitted axes keep their size. X/Y stay centred, Z keeps the base.",
			Params: []apiParam{opt("x", "float", ""), opt("y", "float", ""), opt("z", "float", ""), opt("uniform", "bool", "scale all axes by the one given"), idsParam},
			fn: func(a *App, p args) (any, error) {
				ids := a.nodesArg(p)
				var ns []*doc.Node
				if ids != nil {
					ns = a.ed.Doc.FindAll(ids)
				} else {
					ns = a.ed.Selected()
				}
				if len(ns) == 0 {
					return nil, errors.New("nothing selected")
				}
				b := doc.Bounds(ns)
				if b.IsEmpty() {
					return nil, errNoSize
				}
				s, c := b.Size(), b.Center()
				ns2 := s
				f := 0.0
				for i, k := range []string{"x", "y", "z"} {
					if p.has(k) {
						v := p.f(k, 0)
						if v <= 0 {
							return nil, fmt.Errorf("%s must be positive", k)
						}
						ns2 = ns2.WithAxis(i, v)
						if s.Axis(i) > 0 {
							f = v / s.Axis(i)
						}
					}
				}
				if p.b("uniform", false) && f > 0 {
					ns2 = s.Mul(f)
				}
				nb := geom.Box{Min: geom.V(c.X-ns2.X/2, c.Y-ns2.Y/2, b.Min.Z), Max: geom.V(c.X+ns2.X/2, c.Y+ns2.Y/2, b.Min.Z+ns2.Z)}
				return ok2(a.ed.SetBounds(doc.IDs(ns), nb))
			}},
		{Name: "set_bounds", Doc: "Fit the selection's bounding box exactly to [min, max].",
			Params: []apiParam{req("min", "float[3]", ""), req("max", "float[3]", ""), idsParam},
			fn: func(a *App, p args) (any, error) {
				mn, okMin := p.vec("min")
				mx, okMax := p.vec("max")
				if !okMin || !okMax {
					return nil, errors.New("min and max must be [x, y, z]")
				}
				if mx.X < mn.X || mx.Y < mn.Y || mx.Z < mn.Z {
					return nil, errors.New("max must not be below min")
				}
				return ok2(a.ed.SetBounds(a.nodesArg(p), geom.Box{Min: mn, Max: mx}))
			}},
		{Name: "rotate", Doc: "Rotate about a world axis through the selection centre.", Params: []apiParam{req("axis", "axis", "x, y or z"), req("degrees", "float", "positive = counter-clockwise looking down the axis"), idsParam},
			fn: func(a *App, p args) (any, error) {
				ax, err := axisArg(p, "axis")
				if err != nil {
					return nil, err
				}
				a.ed.Rotate(a.nodesArg(p), ax, p.f("degrees", 0))
				return ok()
			}},
		{Name: "mirror", Doc: "Mirror the selection about its centre.", Params: []apiParam{req("axis", "axis", "x, y or z"), idsParam},
			fn: func(a *App, p args) (any, error) {
				ax, err := axisArg(p, "axis")
				if err != nil {
					return nil, err
				}
				a.ed.Mirror(a.nodesArg(p), ax)
				return ok()
			}},
		{Name: "align", Doc: "Align the selection (a single shape aligns to the current plane). With key, the others align to that shape, which stays put.",
			Params: []apiParam{req("axis", "axis", "x, y or z"), req("where", "string", "min, center or max"), opt("key", "int", "id of the key object"), idsParam},
			fn: func(a *App, p args) (any, error) {
				ax, err := axisArg(p, "axis")
				if err != nil {
					return nil, err
				}
				w, err := whereArg(p)
				if err != nil {
					return nil, err
				}
				a.ed.AlignTo(a.nodesArg(p), ax, w, p.i("key", 0))
				return ok()
			}},
		{Name: "drop", Doc: "Drop down onto the current plane (D).", Params: []apiParam{idsParam}, fn: func(a *App, p args) (any, error) { a.ed.Drop(a.nodesArg(p)); return ok() }},
		{Name: "nudge", Doc: "Move by snap-grid steps (arrow keys).", Params: []apiParam{req("axis", "axis", "x, y or z"), opt("steps", "float", "default 1")},
			fn: func(a *App, p args) (any, error) {
				ax, err := axisArg(p, "axis")
				if err != nil {
					return nil, err
				}
				a.ed.Nudge(ax, p.f("steps", 1))
				return ok()
			}},

		// --- appearance
		{Name: "set_hole", Doc: "Make the selection Hole (true) or Solid (false).", Params: []apiParam{req("hole", "bool", ""), idsParam},
			fn: func(a *App, p args) (any, error) {
				a.withSel(p, func() {
					if p.b("hole", true) {
						a.ed.SetHole(true)
					} else {
						a.makeSolid()
					}
				})
				return ok()
			}},
		{Name: "set_color", Doc: "Set colour (#rrggbb); makes the shapes solid.", Params: []apiParam{req("color", "string", "#rrggbb"), idsParam},
			fn: func(a *App, p args) (any, error) {
				var err error
				a.withSel(p, func() { err = a.ed.SetColor(p.s("color", "")) })
				return ok2(err)
			}},
		{Name: "set_transparent", Doc: "Transparent display on/off.", Params: []apiParam{req("on", "bool", ""), idsParam}, fn: func(a *App, p args) (any, error) {
			a.withSel(p, func() { a.ed.SetTransparent(p.b("on", true)) })
			return ok()
		}},
		{Name: "set_multicolor", Doc: "Union groups: keep each child's colour.", Params: []apiParam{req("on", "bool", ""), idsParam}, fn: func(a *App, p args) (any, error) {
			a.withSel(p, func() { a.ed.SetMulticolor(p.b("on", true)) })
			return ok()
		}},
		{Name: "set_locked", Doc: "Lock or unlock.", Params: []apiParam{req("on", "bool", ""), idsParam}, fn: func(a *App, p args) (any, error) {
			a.withSel(p, func() { a.ed.SetLocked(p.b("on", true)) })
			return ok()
		}},
		{Name: "set_name", Doc: "Name shown in the Properties header.", Params: []apiParam{req("name", "string", ""), idsParam}, fn: func(a *App, p args) (any, error) {
			a.withSel(p, func() { a.ed.SetName(p.s("name", "")) })
			return ok()
		}},
		{Name: "set_param", Doc: "Set a shape parameter (see shape_kinds).", Params: []apiParam{req("id", "int", "node id (any depth)"), req("key", "string", ""), req("value", "float", "")},
			fn: func(a *App, p args) (any, error) {
				return ok2(a.ed.SetParam(p.i("id", 0), p.s("key", ""), p.f("value", 0)))
			}},
		{Name: "set_text", Doc: "Change a text shape's string and/or font.", Params: []apiParam{req("id", "int", ""), req("text", "string", ""), opt("font", "string", "")},
			fn: func(a *App, p args) (any, error) {
				return ok2(a.ed.SetText(p.i("id", 0), p.s("text", ""), p.s("font", "")))
			}},

		// --- visibility
		{Name: "hide", Doc: "Hide the selection (Ctrl+H).", Params: []apiParam{idsParam}, fn: func(a *App, p args) (any, error) {
			a.withSel(p, a.ed.HideSelected)
			return ok()
		}},
		{Name: "toggle_hidden", Doc: "Toggle one shape's visibility.", Params: []apiParam{req("id", "int", "")}, fn: func(a *App, p args) (any, error) {
			a.ed.ToggleHidden(p.i("id", 0))
			return ok()
		}},
		{Name: "show_all", Doc: "Show all hidden shapes (Ctrl+Shift+H).", fn: func(a *App, p args) (any, error) { a.ed.ShowAll(); return ok() }},

		// --- grouping and clipboard
		{Name: "group", Doc: "Group the selection; returns the new group id.", Params: []apiParam{opt("mode", "string", `"union" (default) or "bundle"`), idsParam},
			fn: func(a *App, p args) (any, error) {
				var id int
				var err error
				if ids := a.nodesArg(p); ids != nil {
					a.ed.Select(ids...)
				}
				id, err = a.ed.Group(p.s("mode", doc.ModeUnion))
				return map[string]any{"id": id}, err
			}},
		{Name: "ungroup", Doc: "Ungroup selected groups one level; returns released ids.", Params: []apiParam{idsParam}, fn: func(a *App, p args) (any, error) {
			if ids := a.nodesArg(p); ids != nil {
				a.ed.Select(ids...)
			}
			return map[string]any{"ids": a.ed.Ungroup()}, nil
		}},
		{Name: "set_group_mode", Doc: "Switch selected groups between union, bundle and intersect.", Params: []apiParam{req("mode", "string", "union, bundle or intersect"), idsParam},
			fn: func(a *App, p args) (any, error) {
				m := p.s("mode", "")
				if m != doc.ModeUnion && m != doc.ModeBundle && m != doc.ModeIntersect {
					return nil, errors.New("mode must be union, bundle or intersect")
				}
				var err error
				a.withSel(p, func() { err = a.ed.SetGroupMode(m) })
				return ok2(err)
			}},
		{Name: "split", Doc: "Split shapes with a plane square to an axis of the current plane; each becomes two halves (groups of the shape and a hole box). Returns the halves' ids.",
			Params: []apiParam{req("axis", "axis", "x, y or z (the current plane's)"), opt("at", "float", "where along the axis, mm; default: the selection's centre"),
				opt("keep", "string", "both, above or below (the + or - side of the axis)"), opt("gap", "float", "mm to move the half above (+ side) away; the one below stays"), idsParam},
			fn: func(a *App, p args) (any, error) {
				ax, err := axisArg(p, "axis")
				if err != nil {
					return nil, err
				}
				at := p.f("at", math.NaN())
				if math.IsNaN(at) {
					ns := a.ed.Selected()
					if ids := a.nodesArg(p); ids != nil {
						ns = a.ed.Doc.FindAll(ids)
					}
					at = doc.BoundsIn(ns, a.ed.Work()).Center().Axis(ax)
				}
				ids, err := a.ed.Split(a.nodesArg(p), ax, at, p.s("keep", doc.KeepBoth), p.f("gap", 0))
				return map[string]any{"ids": ids}, err
			}},
		{Name: "lay_flat", Doc: "Turn the selection so the world direction normal (a face's outward normal) points down into the current plane, and drop it there.",
			Params: []apiParam{req("normal", "float[3]", "the face's outward normal"), idsParam},
			fn: func(a *App, p args) (any, error) {
				n, ok := p.vec("normal")
				if !ok {
					return nil, errors.New("normal must be [x, y, z]")
				}
				return ok2(a.ed.LayFlat(a.nodesArg(p), n))
			}},
		{Name: "set_section", Doc: "Section view: hide everything on one side of a plane square to an axis of the current plane (the view only; the design is unchanged).",
			Params: []apiParam{opt("on", "bool", "default true"), opt("axis", "axis", "x, y or z; default y"), opt("at", "float", "where along the axis, mm; default: the design's middle"),
				opt("keep", "string", "which side shows: above or below (the + or - side); default: the side away from the camera"),
				opt("pin", "bool", "stay open when the selection changes")},
			fn: func(a *App, p args) (any, error) {
				if !p.b("on", true) {
					a.setSection(false)
					return ok()
				}
				// checked before anything changes
				ax := -1
				if p.has("axis") {
					var err error
					if ax, err = axisArg(p, "axis"); err != nil {
						return nil, err
					}
				}
				keep := 0.0
				switch p.s("keep", "") {
				case doc.KeepAbove:
					keep = 1
				case doc.KeepBelow:
					keep = -1
				case "":
				default:
					return nil, errors.New("keep must be above or below")
				}
				a.setSection(true)
				s := &a.section
				if ax >= 0 {
					s.axis = ax
				}
				a.centreSection(keep)
				if p.has("at") {
					s.at = p.f("at", s.at)
				}
				if p.has("pin") {
					s.pinned = p.b("pin", false)
				}
				return ok()
			}},
		{Name: "filament_estimate", Doc: "Estimate the filament the selection (or ids, or the whole design) takes: grams, metres, cost, and per colour. Supports, brims and purge aren't included.",
			Params: []apiParam{idsParam},
			fn: func(a *App, p args) (any, error) {
				return filamentJSON(a.estimateFilament(a.filamentNodes(a.nodesArg(p))), a.fil.settings), nil
			}},
		{Name: "set_filament_settings", Doc: "Change the filament estimate's settings (omitted ones are kept); returns them. material: a preset (PLA, PETG, ABS, ASA, TPU, PC, Nylon) sets its density.",
			Params: []apiParam{opt("material", "string", ""), opt("density", "float", "g/cm³"), opt("diameter", "float", "mm"), opt("layer_height", "float", "mm"),
				opt("line_width", "float", "mm"), opt("walls", "int", ""), opt("top_layers", "int", ""), opt("bottom_layers", "int", ""), opt("infill", "float", "percent"),
				opt("price_per_kg", "float", ""), opt("calibration", "float", "percent added to every estimate")},
			fn: func(a *App, p args) (any, error) {
				s := a.fil.settings
				if p.has("material") {
					name := p.s("material", "")
					found := false
					for _, m := range filament.Materials {
						if strings.EqualFold(m.Name, name) {
							s.Material, s.Density, found = m.Name, m.Density, true
						}
					}
					if !found {
						return nil, fmt.Errorf("unknown material %q", name)
					}
				}
				for _, row := range filamentRows {
					if p.has(row.key) {
						row.set(&s, p.f(row.key, row.get(s)))
					}
				}
				a.fil.settings = s.Normalized()
				a.saveFilament()
				return a.fil.settings, nil
			}},
		{Name: "duplicate", Doc: "Ctrl+D; returns new ids.", fn: func(a *App, p args) (any, error) { return map[string]any{"ids": a.ed.Duplicate()}, nil }},
		{Name: "copy", Doc: "Ctrl+C.", fn: func(a *App, p args) (any, error) { a.ed.Copy(); return ok() }},
		{Name: "cut", Doc: "Ctrl+X.", fn: func(a *App, p args) (any, error) { a.ed.Cut(); return ok() }},
		{Name: "paste", Doc: "Ctrl+V (or Ctrl+Shift+V in place); returns new ids.", Params: []apiParam{opt("in_place", "bool", "")}, fn: func(a *App, p args) (any, error) {
			ids, err := a.ed.Paste(p.b("in_place", false))
			return map[string]any{"ids": ids}, err
		}},
		{Name: "delete", Doc: "Delete shapes.", Params: []apiParam{idsParam}, fn: func(a *App, p args) (any, error) { a.ed.Delete(a.nodesArg(p)); return ok() }},
		{Name: "undo", Doc: "Undo one step.", fn: func(a *App, p args) (any, error) { return map[string]any{"done": a.undo()}, nil }},
		{Name: "redo", Doc: "Redo one step.", fn: func(a *App, p args) (any, error) { return map[string]any{"done": a.redo()}, nil }},

		// --- settings and tools
		{Name: "set_units", Doc: "Units: mm or in.", Params: []apiParam{req("units", "string", "mm or in")}, fn: func(a *App, p args) (any, error) {
			return ok2(a.ed.SetUnits(p.s("units", "")))
		}},
		{Name: "set_snap", Doc: "Snap grid in mm (0 = off).", Params: []apiParam{req("mm", "float", "")}, fn: func(a *App, p args) (any, error) {
			a.ed.SetSnap(p.f("mm", 1))
			return ok()
		}},
		{Name: "set_build_plate", Doc: "Build plate size in mm.", Params: []apiParam{req("width", "float", ""), req("length", "float", "")}, fn: func(a *App, p args) (any, error) {
			return ok2(a.ed.SetWorkplaneSize(p.f("width", 200), p.f("length", 200)))
		}},
		{Name: "set_settings", Doc: "Settings toggles.", Params: []apiParam{opt("show_grid", "bool", ""), opt("show_shadows", "bool", "")},
			fn: func(a *App, p args) (any, error) {
				s := a.ed.Doc.Settings
				s.ShowGrid = p.b("show_grid", s.ShowGrid)
				s.ShowShadows = p.b("show_shadows", s.ShowShadows)
				a.ed.SetSettings(s)
				return ok()
			}},
		{Name: "place_ruler", Doc: "Place the ruler origin on the current plane.", Params: []apiParam{req("x", "float", ""), req("y", "float", ""), opt("midpoint", "bool", ""), opt("turns", "int", "quarter turns counter-clockwise")},
			fn: func(a *App, p args) (any, error) {
				a.ed.PlaceRuler(geom.V(p.f("x", 0), p.f("y", 0), 0))
				a.ed.Ruler.Mid = p.b("midpoint", false)
				a.ed.Ruler.Turns = ((int(p.f("turns", 0)) % 4) + 4) % 4
				return ok()
			}},
		{Name: "remove_ruler", Doc: "Remove the ruler.", fn: func(a *App, p args) (any, error) { a.ed.RemoveRuler(); return ok() }},
		{Name: "toggle_construction_plane", Doc: "The Construction plane on shape button: put a construction plane on the selected shape, or remove it.", fn: func(a *App, p args) (any, error) { return ok2(a.ed.ToggleShapeWorkplane()) }},
		{Name: "attach_construction_plane", Doc: "Put a construction plane on top-level shape id (its own axes, on its bottom); id 0 removes it, back to the build plate.",
			Params: []apiParam{req("id", "int", "")},
			fn:     func(a *App, p args) (any, error) { return ok2(a.ed.AttachWorkplane(p.i("id", 0))) }},
		{Name: "set_tool", Doc: "Activate a tool mode.", Params: []apiParam{req("tool", "string", "none, align, mirror, measure, visibility, ruler, notes, construction_plane, split, layflat, cruise")},
			fn: func(a *App, p args) (any, error) {
				m := map[string]toolMode{"none": modeNone, "align": modeAlign, "mirror": modeMirror, "measure": modeMeasure, "visibility": modeVisibility, "ruler": modeRulerPlace, "notes": modeNotes, "construction_plane": modeWorkplane, "split": modeSplit, "layflat": modeLayFlat}
				t := p.s("tool", "")
				if t == "cruise" {
					a.cruise = !a.cruise
					return ok()
				}
				mm, okk := m[t]
				if !okk {
					return nil, fmt.Errorf("unknown tool %q", t)
				}
				a.mode = modeNone
				a.setMode(mm)
				if a.mode != mm {
					return nil, fmt.Errorf("%s needs an unlocked shape selected", t)
				}
				return ok()
			}},

		// --- camera and output
		{Name: "get_camera", Doc: "Camera (angles in degrees).", fn: func(a *App, p args) (any, error) {
			c := a.cam
			return map[string]any{"target": []float64{c.Target.X, c.Target.Y, c.Target.Z}, "yaw": c.Yaw * 180 / math.Pi, "pitch": c.Pitch * 180 / math.Pi, "dist": c.Dist, "ortho": c.Ortho}, nil
		}},
		{Name: "set_camera", Doc: "Set camera fields (angles in degrees).", Params: []apiParam{opt("target", "float[3]", ""), opt("yaw", "float", ""), opt("pitch", "float", ""), opt("dist", "float", ""), opt("ortho", "bool", "")},
			fn: func(a *App, p args) (any, error) {
				a.anim = nil
				if t, okk := p.vec("target"); okk {
					a.cam.Target = t
				}
				if p.has("yaw") {
					a.cam.Yaw = p.f("yaw", 0) * math.Pi / 180
				}
				if p.has("pitch") {
					a.cam.Pitch = math.Max(-89.9, math.Min(89.9, p.f("pitch", 0))) * math.Pi / 180
				}
				if p.has("dist") {
					a.cam.Dist = math.Max(1, p.f("dist", 300))
				}
				a.cam.Ortho = p.b("ortho", a.cam.Ortho)
				a.w.Redraw()
				return ok()
			}},
		{Name: "home_view", Doc: "Home view.", fn: func(a *App, p args) (any, error) { a.homeCamera(false); return ok() }},
		{Name: "fit_view", Doc: "Fit selection (or all) in view.", fn: func(a *App, p args) (any, error) {
			a.fitView()
			if a.anim != nil {
				a.cam = a.anim.to
				a.anim = nil
			}
			return ok()
		}},
		{Name: "screenshot", Doc: "Render the window to a PNG.", Params: []apiParam{req("path", "string", "")}, fn: func(a *App, p args) (any, error) {
			if err := a.screenshotPNG(p.s("path", "")); err != nil {
				return nil, err
			}
			return map[string]any{"width": a.w.W, "height": a.w.H, "dpi": a.w.DPI}, nil
		}},
		{Name: "project", Doc: "World point (mm) to window coordinates (logical px).", Params: []apiParam{req("x", "float", ""), req("y", "float", ""), req("z", "float", "")},
			fn: func(a *App, p args) (any, error) {
				a.layout()
				x, y, okk := a.project(geom.V(p.f("x", 0), p.f("y", 0), p.f("z", 0)))
				return map[string]any{"x": x, "y": y, "visible": okk && a.vp.Contains(x, y)}, nil
			}},
		{Name: "get_ui_state", Doc: "Tool mode, open popup, hover, active field, banner and toast; the scale and font atlas; the section; in align, mirror and measure modes the align dots, mirror arrows (screen x, y) or measure points.", fn: func(a *App, p args) (any, error) {
			mode := []string{"none", "visibility", "measure", "ruler", "align", "mirror", "place", "notes", "construction_plane", "split", "layflat"}[a.mode]
			pop := []string{"", "visibility", "snap", "color", "font", "library", "design_menu", "settings", "import", "export", "help", "about", "start"}[a.popup]
			st := map[string]any{"mode": mode, "popup": pop, "hot": a.hot, "cruise": a.cruise, "notes_hidden": a.notesHidden, "recent": a.recent, "banner": a.bannerText(), "toast": a.toast,
				"align_key": a.alignKey, "pressed": a.pressed, "frames": a.frames, "window": []float64{a.W, a.H}, "viewport": []float64{a.vp.X, a.vp.Y, a.vp.W, a.vp.H}, "hover_node": a.hoverNode, "placing": a.place != nil,
				"export_selected": a.exportSel && len(a.ed.Sel) > 0,
				"background":      rgbHex(float64(a.bgColor[0]), float64(a.bgColor[1]), float64(a.bgColor[2])), "theme": currentTheme.Name,
				"background_jobs": doc.Pending(), "filament_panel": a.fil.shown, "scale": a.w.DPI, "min_window": []int{a.w.MinW, a.w.MinH},
				"atlas": map[string]any{"size": a.batch.AtlasSize(), "generation": a.batch.Generation}}
			if s := a.section; s.on {
				keep := doc.KeepAbove
				if s.keep < 0 {
					keep = doc.KeepBelow
				}
				st["section"] = map[string]any{"axis": string(rune('x' + s.axis)), "at": s.at, "keep": keep, "ids": s.ids, "pinned": s.pinned}
			}
			if a.field != nil {
				st["field"] = map[string]any{"id": a.field.id, "text": a.field.text}
			}
			// the viewport widgets, where a click lands on them
			switch a.mode {
			case modeAlign:
				ds := []any{}
				for _, d := range a.alignDots() {
					ds = append(ds, map[string]any{"axis": string(rune('x' + d.d.axis)), "where": []string{"min", "center", "max"}[d.d.where+1], "x": d.x, "y": d.y})
				}
				st["align_dots"] = ds
			case modeMirror:
				ms := []any{}
				for _, m := range a.mirrorArrows() {
					ms = append(ms, map[string]any{"axis": string(rune('x' + m.axis)), "x": (m.x0 + m.x1) / 2, "y": (m.y0 + m.y1) / 2})
				}
				st["mirror_arrows"] = ms
			case modeMeasure:
				ms := a.measure
				m := map[string]any{"stage": ms.stage}
				if ms.stage > 0 {
					m["start"] = []float64{ms.a.X, ms.a.Y, ms.a.Z}
				}
				if ms.stage == 2 {
					m["end"] = []float64{ms.b.X, ms.b.Y, ms.b.Z}
					m["distance"] = ms.b.Sub(ms.a).Len()
				}
				st["measure"] = m
			}
			return st, nil
		}},
		{Name: "list_controls", Doc: "Every clickable control drawn in the last frame (id, rect, tooltip).", fn: func(a *App, p args) (any, error) {
			a.Frame()
			var out []any
			for _, r := range a.prevRegions {
				if r.id == "" || r.id == "catcher" || r.id == "panel" {
					continue
				}
				out = append(out, map[string]any{"id": r.id, "rect": []float64{r.r.X, r.r.Y, r.r.W, r.r.H}, "tip": r.tip, "key": r.tipKey})
			}
			return out, nil
		}},
		{Name: "list_handles", Doc: "Selection handles on screen: kind (corner, edge, top, lift, rotate), index (corner/edge number or rotation axis 0-2), x, y.",
			fn: func(a *App, p args) (any, error) {
				a.Frame()
				names := []string{"corner", "edge", "top", "lift", "rotate"}
				var out []any
				for _, h := range a.computeHandles() {
					if h.ok {
						out = append(out, map[string]any{"kind": names[h.kind], "index": h.idx, "x": h.x, "y": h.y})
					}
				}
				return out, nil
			}},
		{Name: "click_control", Doc: "Click a control by id (see list_controls).", Params: []apiParam{req("id", "string", "")}, fn: func(a *App, p args) (any, error) {
			a.Frame()
			id := p.s("id", "")
			for i := len(a.prevRegions) - 1; i >= 0; i-- {
				r := a.prevRegions[i]
				if r.id == id {
					x, y := r.r.CX(), r.r.CY()
					// as a real click would: refused if something (a popup's
					// backdrop, another panel) lies over it there
					if top := a.regionAt(x, y); top != nil && top.id != id && !top.passThrough {
						return nil, fmt.Errorf("control %q is covered by %q", id, top.id)
					}
					a.simMove(x, y, 0)
					a.simDown(win.Left, x, y, 0)
					a.simUp(win.Left, x, y, 0)
					return ok()
				}
			}
			return nil, fmt.Errorf("no control %q on screen", id)
		}},

		// --- input simulation (window coordinates in logical px)
		{Name: "mouse_move", Doc: "Move the mouse.", Params: []apiParam{req("x", "float", ""), req("y", "float", ""), opt("mods", "string[]", "shift, ctrl, alt")},
			fn: func(a *App, p args) (any, error) {
				a.Frame()
				a.simMove(p.f("x", 0), p.f("y", 0), modsArg(p))
				return ok()
			}},
		{Name: "mouse_down", Doc: "Press a button.", Params: []apiParam{req("x", "float", ""), req("y", "float", ""), opt("button", "string", "left, right, middle"), opt("mods", "string[]", "")},
			fn: func(a *App, p args) (any, error) {
				a.Frame()
				a.simDown(buttonArg(p), p.f("x", 0), p.f("y", 0), modsArg(p))
				return ok()
			}},
		{Name: "mouse_up", Doc: "Release a button.", Params: []apiParam{req("x", "float", ""), req("y", "float", ""), opt("button", "string", ""), opt("mods", "string[]", "")},
			fn: func(a *App, p args) (any, error) {
				a.simUp(buttonArg(p), p.f("x", 0), p.f("y", 0), modsArg(p))
				return ok()
			}},
		{Name: "click", Doc: "Move, press and release.", Params: []apiParam{req("x", "float", ""), req("y", "float", ""), opt("button", "string", ""), opt("mods", "string[]", "")},
			fn: func(a *App, p args) (any, error) {
				a.Frame()
				x, y, m, btn := p.f("x", 0), p.f("y", 0), modsArg(p), buttonArg(p)
				a.simMove(x, y, m)
				a.simDown(btn, x, y, m)
				a.simUp(btn, x, y, m)
				return ok()
			}},
		{Name: "drag", Doc: "Press at (x0,y0), move in steps to (x1,y1), release.",
			Params: []apiParam{req("x0", "float", ""), req("y0", "float", ""), req("x1", "float", ""), req("y1", "float", ""), opt("steps", "int", "default 10"), opt("button", "string", ""), opt("mods", "string[]", "")},
			fn: func(a *App, p args) (any, error) {
				a.Frame()
				x0, y0, x1, y1 := p.f("x0", 0), p.f("y0", 0), p.f("x1", 0), p.f("y1", 0)
				n := p.i("steps", 10)
				if n < 1 {
					n = 1
				}
				m, btn := modsArg(p), buttonArg(p)
				a.simMove(x0, y0, m)
				a.simDown(btn, x0, y0, m)
				for i := 1; i <= n; i++ {
					t := float64(i) / float64(n)
					a.simMove(x0+(x1-x0)*t, y0+(y1-y0)*t, m)
				}
				a.simUp(btn, x1, y1, m)
				return ok()
			}},
		{Name: "wheel", Doc: "Mouse wheel (positive = zoom in).", Params: []apiParam{req("x", "float", ""), req("y", "float", ""), req("delta", "float", "notches")},
			fn: func(a *App, p args) (any, error) {
				a.Frame()
				d := a.w.DPI
				a.MouseWheel(p.f("delta", 1), p.f("x", 0)*d, p.f("y", 0)*d, 0)
				return ok()
			}},
		{Name: "key", Doc: `Press a key chord, e.g. "ctrl+g", "shift+ctrl+z", "delete", "esc", "left", "h".`, Params: []apiParam{req("keys", "string", "")},
			fn: func(a *App, p args) (any, error) {
				vk, m, err := parseChord(p.s("keys", ""))
				if err != nil {
					return nil, err
				}
				a.Frame()
				a.KeyDown(vk, m, false)
				a.KeyUp(vk, m)
				return ok()
			}},
		{Name: "type_text", Doc: "Type characters (into the active field). A newline starts a new line in a field that takes several (a note); keys such as Enter and Tab are sent with key.",
			Params: []apiParam{req("text", "string", "")}, fn: func(a *App, p args) (any, error) {
				for _, r := range p.s("text", "") {
					if r == '\n' && a.field != nil && a.field.multiline {
						a.fieldInsert("\n") // as Shift+Enter there
						continue
					}
					a.Char(r)
				}
				return ok()
			}},
		// --- scripting
		{Name: "run_script", Doc: "Run a Starlark script (as the console's Editor tab does) and return its printed output and any error; one undo step.",
			Params: []apiParam{req("code", "string", "the script"), opt("name", "string", "file name for errors"), opt("cwd", "string", "folder relative paths resolve against")},
			fn:     func(a *App, p args) (any, error) { return nil, errors.New("a script can't start another script") }},
		{Name: "get_console", Doc: "The Scripting console: open, tab, running, transcript lines, prompt and script text.",
			fn: func(a *App, p args) (any, error) { return a.consoleJSON(), nil }},
		{Name: "script_api", Doc: "The cad methods scripts get (mirroring the Python client).",
			fn: func(a *App, p args) (any, error) { return script.Methods, nil }},
		{Name: "answer_dialogs", Doc: "Queue answers to the prompts and file dialogs that come next (for automated runs): each {kind, value} with kind message (value yes, no or cancel), open or save (value: a path, or empty to cancel). Returns how many are queued.",
			Params: []apiParam{req("answers", "any[]", "")}, fn: func(a *App, p args) (any, error) {
				l, _ := p["answers"].([]any)
				for _, x := range l {
					m, ok := x.(map[string]any)
					kind, _ := m["kind"].(string)
					value, _ := m["value"].(string)
					if !ok || (kind != "message" && kind != "open" && kind != "save") {
						return nil, fmt.Errorf("an answer is {kind: message|open|save, value: ...}, not %v", x)
					}
					a.answers = append(a.answers, dialogAnswer{kind, value})
				}
				return map[string]any{"queued": len(a.answers)}, nil
			}},
		{Name: "close_window", Doc: "What closing the window would do (the Save changes? prompt included): closes true if it would close. The window stays open.", fn: func(a *App, p args) (any, error) {
			return map[string]any{"closes": a.Close()}, nil
		}},
		{Name: "lose_capture", Doc: "The mouse is taken away mid-drag (another window, Alt+Tab): what the window does then.", fn: func(a *App, p args) (any, error) {
			a.CaptureLost()
			return ok()
		}},
		{Name: "drop_files", Doc: "Drop files on the window (imports them).", Params: []apiParam{req("paths", "string[]", "")}, fn: func(a *App, p args) (any, error) {
			var paths []string
			l, _ := p["paths"].([]any)
			for _, x := range l {
				if s, ok := x.(string); ok {
					paths = append(paths, s)
				}
			}
			a.DropFiles(paths, -1, -1)
			return ok()
		}},
		{Name: "stats", Doc: "Memory and cache figures (for soak tests): heap bytes in use and held, goroutines, cached meshes and their triangles, bounds entries, background jobs, undo depth.", fn: func(a *App, p args) (any, error) {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			st := map[string]any{"heap_alloc": ms.HeapAlloc, "heap_sys": ms.HeapSys, "heap_objects": ms.HeapObjects,
				"goroutines": runtime.NumGoroutine(), "background_jobs": doc.Pending(), "undo_depth": a.ed.Hist.Depth(),
				"gpu_meshes": a.scene.Cached()}
			for k, v := range doc.CacheStats() {
				st[k] = v
			}
			return st, nil
		}},
		{Name: "quit", Doc: "Close ScottiCAD without saving.", fn: func(a *App, p args) (any, error) {
			a.w.Quit()
			return ok()
		}},
	}
}

func ok2(err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func modsArg(p args) win.Mods {
	var m win.Mods
	v, _ := p["mods"].([]any)
	for _, x := range v {
		switch strings.ToLower(fmt.Sprint(x)) {
		case "shift":
			m |= win.Shift
		case "ctrl", "control":
			m |= win.Ctrl
		case "alt":
			m |= win.Alt
		}
	}
	return m
}

func buttonArg(p args) int {
	switch p.s("button", "left") {
	case "right":
		return win.Right
	case "middle":
		return win.Middle
	}
	return win.Left
}

var keyNames = map[string]int{
	"backspace": vkBack, "tab": vkTab, "enter": vkReturn, "return": vkReturn, "esc": vkEscape, "escape": vkEscape,
	"left": vkLeft, "right": vkRight, "up": vkUp, "down": vkDown, "delete": vkDelete, "del": vkDelete,
	"home": vkHome, "end": vkEnd, "plus": vkPlus, "+": vkPlus, "minus": vkMinus, "-": vkMinus, "f1": vkF1,
	"f5": vkF5, "pgup": vkPgUp, "pgdn": vkPgDn,
}

func parseChord(s string) (int, win.Mods, error) {
	var m win.Mods
	t := strings.ToLower(strings.TrimSpace(s))
	var parts []string
	switch {
	case t == "+":
		parts = []string{"+"}
	case strings.HasSuffix(t, "++"): // ctrl++: the key is "+"
		parts = append(strings.Split(strings.TrimSuffix(t, "++"), "+"), "+")
	default:
		parts = strings.Split(t, "+")
	}
	vk := 0
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if vk != 0 && p != "shift" && p != "ctrl" && p != "control" && p != "alt" {
			return 0, 0, fmt.Errorf("two keys in %q: a chord is modifiers and one key", s)
		}
		switch p {
		case "shift":
			m |= win.Shift
		case "ctrl", "control":
			m |= win.Ctrl
		case "alt":
			m |= win.Alt
		default:
			if k, ok := keyNames[p]; ok {
				vk = k
			} else if len(p) == 1 && (p[0] >= 'a' && p[0] <= 'z' || p[0] >= '0' && p[0] <= '9') {
				vk = int(strings.ToUpper(p)[0])
			} else {
				return 0, 0, fmt.Errorf("unknown key %q", p)
			}
		}
	}
	if vk == 0 {
		return 0, 0, fmt.Errorf("no key in %q", s)
	}
	return vk, m, nil
}

// simulated input goes through the same handlers as real input
func (a *App) simMove(x, y float64, m win.Mods) {
	d := a.w.DPI
	a.MouseMove(x*d, y*d, m)
}

func (a *App) simDown(btn int, x, y float64, m win.Mods) {
	d := a.w.DPI
	a.MouseDown(btn, x*d, y*d, m, false)
}

func (a *App) simUp(btn int, x, y float64, m win.Mods) {
	d := a.w.DPI
	a.MouseUp(btn, x*d, y*d, m)
	a.Frame()
}

func (a *App) runCall(c apiCall) {
	var res apiResult
	defer func() {
		if r := recover(); r != nil {
			res = apiResult{err: fmt.Errorf("internal error: %v", r)}
		}
		c.reply <- res
		a.w.Redraw()
	}()
	for _, m := range apiMethods {
		if m.Name == c.method {
			if err := checkParams(m, c.params); err != nil {
				res = apiResult{err: err}
				return
			}
			// automation sees exact geometry, unless a call asks to see
			// the window as it is ("live": true; e.g. a screenshot while
			// shapes are still being combined)
			var r any
			var err error
			if c.params.b("live", false) {
				r, err = m.fn(a, c.params)
			} else {
				doc.Sync(func() { r, err = m.fn(a, c.params) })
			}
			res = apiResult{r, err}
			return
		}
	}
	res = apiResult{err: fmt.Errorf("unknown method %q (see describe)", c.method)}
}

// checkParams refuses a call whose parameters a handler would misread: a
// required one missing, a number that isn't one, ids that aren't ids (read
// as "not given", they would act on the selection instead) or a point
// that isn't three numbers. A null is the same as leaving it out.
func checkParams(m apiMethod, p args) error {
	for k, v := range p {
		if v == nil {
			delete(p, k)
		}
	}
	isNum := func(v any) bool {
		switch v := v.(type) {
		case float64:
			return true
		case string:
			var f float64
			_, err := fmt.Sscan(v, &f)
			return err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
		case bool:
			return true
		}
		return false
	}
	for _, prm := range m.Params {
		v, ok := p[prm.Name]
		if !ok {
			if !prm.Optional {
				return fmt.Errorf("missing parameter %q", prm.Name)
			}
			continue
		}
		bad := false
		switch prm.Type {
		case "float", "int":
			bad = !isNum(v)
		case "axis": // "x", "y", "z" or 0, 1, 2 (axisArg reads both)
			switch v.(type) {
			case string, float64:
			default:
				bad = true
			}
		case "bool":
			_, ok := v.(bool) // "false" would read as the default
			bad = !ok
		case "string":
			_, ok := v.(string)
			bad = !ok
		case "string[]":
			l, ok := v.([]any)
			bad = !ok
			for _, x := range l {
				if _, ok := x.(string); !ok {
					bad = true
				}
			}
		case "int[]":
			switch v := v.(type) {
			case float64:
			case []any:
				for _, x := range v {
					if _, ok := x.(float64); !ok {
						bad = true
					}
				}
			default:
				bad = true
			}
		case "float[3]":
			l, ok := v.([]any)
			bad = !ok || len(l) != 3
			for _, x := range l {
				if _, ok := x.(float64); !ok {
					bad = true
				}
			}
		}
		if bad {
			return fmt.Errorf("parameter %q must be %s", prm.Name, prm.Type)
		}
	}
	return nil
}

// ServeAPI listens on addr (e.g. "127.0.0.1:47800") until the app exits.
func (a *App) ServeAPI(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go a.serveConn(conn)
		}
	}()
	return ln.Addr().String(), nil
}

type rpcRequest struct {
	ID     any             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

func (a *App) serveConn(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	enc := json.NewEncoder(conn)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var req rpcRequest
		resp := rpcResponse{JSONRPC: "2.0"}
		if err := json.Unmarshal(line, &req); err != nil {
			// and hang up: a client speaking something else (a web page's
			// HTTP request, whose body could hold a command) gets no further
			resp.Error = &rpcError{-32700, "parse error: " + err.Error()}
			enc.Encode(resp)
			return
		}
		resp.ID = req.ID
		p := args{}
		if len(req.Params) > 0 && string(req.Params) != "null" {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &rpcError{-32602, "params must be an object"}
				enc.Encode(resp)
				continue
			}
		}
		var r apiResult
		if req.Method == "run_script" {
			// runs here, off the UI thread: its cad calls need it
			r.result, r.err = a.runScriptAPI(p)
		} else {
			reply := make(chan apiResult, 1)
			a.calls <- apiCall{method: req.Method, params: p, reply: reply}
			a.w.Wake()
			r = <-reply
		}
		if r.err != nil {
			resp.Error = &rpcError{-32000, r.err.Error()}
		} else {
			resp.Result = r.result
			if resp.Result == nil {
				resp.Result = map[string]any{}
			}
		}
		line, err := json.Marshal(resp)
		if err != nil {
			// a result that can't be sent (a NaN in it): an error reply, not
			// a dropped connection
			resp.Result = nil
			resp.Error = &rpcError{-32603, "internal error: the result can't be sent: " + err.Error()}
			line, _ = json.Marshal(resp)
		}
		if _, err := conn.Write(append(line, '\n')); err != nil {
			return
		}
	}
}
