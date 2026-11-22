package app

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// The API's sketch commands: shapes made from a drawing given as paths, and
// the sketch workspace opened, driven and closed (for tests and pictures).

// sketchArg reads a drawing's paths: each {points, closed, stroke, align,
// join, cap}, its points [x, y] or {p: [x, y], in, out, kind}.
func sketchArg(v any) (*shapes.Sketch, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, errors.New("paths must be a list of {points, closed, stroke, align, join, cap, hole}")
	}
	num := func(x any, what string) (float64, error) {
		if x == nil {
			return 0, nil
		}
		f, ok := x.(float64)
		if !ok {
			return 0, fmt.Errorf("%s must be a number", what)
		}
		return f, nil
	}
	pair := func(x any, what string) ([2]float64, error) {
		if x == nil {
			return [2]float64{}, nil
		}
		l, ok := x.([]any)
		if !ok || len(l) != 2 {
			return [2]float64{}, fmt.Errorf("%s must be [x, y]", what)
		}
		var out [2]float64
		for i := range out {
			f, ok := l[i].(float64)
			if !ok {
				return out, fmt.Errorf("%s must be [x, y]", what)
			}
			out[i] = f
		}
		return out, nil
	}
	named := func(x any, names []string, what string) (int, error) {
		switch x := x.(type) {
		case nil:
			return 0, nil
		case float64:
			if x != float64(int(x)) || int(x) < 0 || int(x) >= len(names) {
				return 0, fmt.Errorf("%s must be one of %s", what, strings.Join(names, ", "))
			}
			return int(x), nil
		case string:
			for i, n := range names {
				if strings.EqualFold(x, n) {
					return i, nil
				}
			}
		}
		return 0, fmt.Errorf("%s must be one of %s", what, strings.Join(names, ", "))
	}
	sk := &shapes.Sketch{}
	for i, pv := range list {
		pm, ok := pv.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %d must be {points, closed, stroke, align, join, cap, hole}", i+1)
		}
		var p shapes.SketchPath
		where := fmt.Sprintf("path %d", i+1)
		pts, ok := pm["points"].([]any)
		if !ok {
			return nil, fmt.Errorf("%s: points must be a list", where)
		}
		for j, qv := range pts {
			at := fmt.Sprintf("%s, point %d", where, j+1)
			var q shapes.SketchPoint
			var err error
			switch qv := qv.(type) {
			case []any:
				q.P, err = pair(qv, at)
			case map[string]any:
				if q.P, err = pair(qv["p"], at+": p"); err != nil {
					return nil, err
				}
				if q.In, err = pair(qv["in"], at+": in"); err != nil {
					return nil, err
				}
				if q.Out, err = pair(qv["out"], at+": out"); err != nil {
					return nil, err
				}
				q.Kind, err = named(qv["kind"], []string{"corner", "smooth", "auto"}, at+": kind")
			default:
				err = fmt.Errorf("%s must be [x, y] or {p, in, out, kind}", at)
			}
			if err != nil {
				return nil, err
			}
			p.Points = append(p.Points, q)
		}
		if c, ok := pm["closed"]; ok {
			b, ok := c.(bool)
			if !ok {
				return nil, fmt.Errorf("%s: closed must be true or false", where)
			}
			p.Closed = b
		}
		if h, ok := pm["hole"]; ok {
			b, ok := h.(bool)
			if !ok {
				return nil, fmt.Errorf("%s: hole must be true or false", where)
			}
			p.Hole = b
		}
		var err error
		if p.Stroke, err = num(pm["stroke"], where+": stroke"); err != nil {
			return nil, err
		}
		if p.Join, err = named(pm["join"], []string{"sharp", "round", "bevel"}, where+": join"); err != nil {
			return nil, err
		}
		if p.Cap, err = named(pm["cap"], []string{"flat", "round", "square"}, where+": cap"); err != nil {
			return nil, err
		}
		if p.Align, err = named(pm["align"], []string{"center", "inside", "outside"}, where+": align"); err != nil {
			return nil, err
		}
		sk.Paths = append(sk.Paths, p)
	}
	if err := shapes.CheckSketch(sk); err != nil {
		return nil, err
	}
	return sk, nil
}

var skToolNames = []string{"select", "line", "bezier", "smooth", "refine", "shape"}

// sketchCommands are the API's sketch commands.
func sketchCommands() []apiMethod {
	ok := func() (any, error) { return map[string]any{"ok": true}, nil }
	kindOf := func(p args) (string, error) {
		k := p.s("kind", "sketch")
		if !shapes.IsSketch(k) {
			return "", errors.New("kind must be sketch, revolve or coil")
		}
		return k, nil
	}
	open := func(a *App) error {
		if a.sk == nil {
			return errors.New("the sketch workspace isn't open (see edit_sketch)")
		}
		return nil
	}
	return []apiMethod{
		// --- sketches (and the sketch workspace)
		{Name: "add_sketch", Doc: "Make a sketch shape from a drawing: paths of points joined by straight lines and curves. A closed path is an area unless it has a stroke width (areas join, one inside another too); an open path is a line (2 mm wide unless stroke says). A path with hole true cuts what it covers (its area or its line) out of the rest of the drawing instead. A closed path's line lies on it (align center), inside it or outside it. A point is [x, y] (a sharp corner) or {p: [x, y], in: [dx, dy], out: [dx, dy], kind: corner | smooth | auto}, its curve handles relative to it (auto: the curve flows through the points, each handle a quarter of the way between its neighbors). kind sketch extrudes the drawing 10 mm up (resize to change; set_param quality, topscale, basescale, twist, twistmode (0 steps, 1 smooth) and twiststeps taper and twist it); revolve spins what is left of x = 0 round the vertical line there; coil spins it too while rising (set_param turns, pitch: a spring, thread or ramp). The drawing's origin goes at (x, y, z). Selects it.",
			Params: []apiParam{req("paths", "path[]", "[{points: [...], closed, stroke (mm), align: center | inside | outside, join: sharp | round | bevel, cap: flat | round | square, hole}, ...]"),
				opt("kind", "string", "sketch (default), revolve or coil"), opt("x", "float", "mm"), opt("y", "float", "mm"), opt("z", "float", "mm; default: on the current plane"), opt("hole", "bool", "")},
			fn: func(a *App, p args) (any, error) {
				kind, err := kindOf(p)
				if err != nil {
					return nil, err
				}
				sk, err := sketchArg(p["paths"])
				if err != nil {
					return nil, err
				}
				xf := geom.Translate(geom.V(p.f("x", 0), p.f("y", 0), p.f("z", a.ed.PlaneZ())))
				id, err := a.ed.AddSketchXf(kind, sk, xf, p.b("hole", false))
				return map[string]any{"id": id}, err
			}},
		{Name: "set_sketch", Doc: "Replace a sketch shape's drawing (paths as add_sketch). The drawing is in the shape's own frame: what is unchanged stays where it is.",
			Params: []apiParam{req("id", "int", "node id (any depth)"), req("paths", "path[]", "as add_sketch")},
			fn: func(a *App, p args) (any, error) {
				sk, err := sketchArg(p["paths"])
				if err != nil {
					return nil, err
				}
				return ok2(a.ed.SetSketch(p.i("id", 0), sk))
			}},
		{Name: "edit_sketch", Doc: "Open the sketch workspace on sketch shape id, or with no id on a new drawing of kind whose origin goes at (x, y, z) when it is done.",
			Params: []apiParam{opt("id", "int", "node id"), opt("kind", "string", "sketch, revolve or coil, for a new one"), opt("x", "float", "mm"), opt("y", "float", "mm"), opt("z", "float", "mm")},
			fn: func(a *App, p args) (any, error) {
				// (as Import, New and Open: a drawing in progress isn't
				// thrown away unasked)
				if a.sk != nil {
					return nil, errors.New("the sketch workspace is already open (see sketch_done, sketch_cancel)")
				}
				if id := p.i("id", 0); id != 0 {
					n := a.ed.Doc.FindAny(id)
					if n == nil || !shapes.IsSketch(n.Kind) {
						return nil, fmt.Errorf("node %d is not a sketch", id)
					}
					if a.ed.Locked(id) {
						return nil, fmt.Errorf("node %d is locked, or in a locked group", id)
					}
					a.editSketch(id)
					return ok()
				}
				kind, err := kindOf(p)
				if err != nil {
					return nil, err
				}
				a.newSketch(kind, geom.Translate(geom.V(p.f("x", 0), p.f("y", 0), p.f("z", a.ed.PlaneZ()))), false)
				return ok()
			}},
		{Name: "sketch_state", Doc: "The sketch workspace's state: open, the drawing's paths, the tool, the path being drawn, the selected points ([path, point], from 0), and the view (board rectangle in window px, the mm at its center, px per mm).",
			fn: func(a *App, p args) (any, error) {
				s := a.sk
				if s == nil {
					return map[string]any{"open": false}, nil
				}
				a.skCloseMeetingEnds() // (as each frame does)
				sel := [][2]int{}      // empty lists, not null
				paths := s.sk.Paths
				if paths == nil {
					paths = []shapes.SketchPath{}
				}
				for i, pt := range s.sk.Paths {
					for j := range pt.Points {
						if s.sel[skRef{i, j}] {
							sel = append(sel, [2]int{i, j})
						}
					}
				}
				out := map[string]any{"open": true, "kind": s.kind, "id": s.nodeID, "paths": paths, "tool": skToolNames[s.tool],
					"drawing": s.drawing, "selected": sel, "board": []float64{s.board.X, s.board.Y, s.board.W, s.board.H},
					"center": []float64{s.cx, s.cy}, "zoom": s.zoom, "can_undo": len(s.undo) > 0,
					"image": nil, "image_panel": s.img.panel, "image_mode": []string{"move", "scale", "level"}[s.img.mode]}
				if im := s.sk.Image; im != nil {
					out["image"] = map[string]any{"x": im.X, "y": im.Y, "width": im.Width, "height": im.Height(), "angle": im.Angle,
						"opacity": im.Opacity, "pixels": []int{im.PW, im.PH}}
				}
				return out, nil
			}},
		{Name: "sketch_image", Doc: "Load a picture (PNG or JPEG) under the open sketch's drawing, to trace, or place the one there: its center (mm), width across (mm; its height follows), angle (degrees anticlockwise) and opacity (0 to 1). A new picture goes in the middle of the view unless placed. It is saved with the sketch and never part of the shape.",
			Params: []apiParam{opt("path", "string", "picture file; omit to place the one already there"), opt("x", "float", "mm"), opt("y", "float", "mm"),
				opt("width", "float", "mm"), opt("angle", "float", "degrees"), opt("opacity", "float", "0 to 1")},
			fn: func(a *App, p args) (any, error) {
				if err := open(a); err != nil {
					return nil, err
				}
				s := a.sk
				if path := p.s("path", ""); path != "" {
					if err := a.skLoadImage(path); err != nil {
						return nil, err
					}
				} else if s.sk.Image == nil {
					return nil, errors.New("the sketch has no picture: give a path")
				}
				im := *s.sk.Image
				im.X, im.Y = p.f("x", im.X), p.f("y", im.Y)
				im.Width, im.Angle, im.Opacity = p.f("width", im.Width), p.f("angle", im.Angle), p.f("opacity", im.Opacity)
				if err := shapes.CheckSketch(&shapes.Sketch{Image: &im}); err != nil {
					return nil, err
				}
				if im != *s.sk.Image {
					a.skPush()
					s.sk.Image = &im
				}
				return ok()
			}},
		{Name: "sketch_image_remove", Doc: "Take the picture away from under the open sketch's drawing.",
			fn: func(a *App, p args) (any, error) {
				if err := open(a); err != nil {
					return nil, err
				}
				if a.sk.sk.Image != nil {
					a.skPush()
					a.sk.sk.Image = nil
				}
				return ok()
			}},
		{Name: "sketch_tool", Doc: "Pick the sketch workspace's tool.", Params: []apiParam{req("tool", "string", strings.Join(skToolNames, ", "))},
			fn: func(a *App, p args) (any, error) {
				if err := open(a); err != nil {
					return nil, err
				}
				for i, n := range skToolNames {
					if n == p.s("tool", "") {
						a.skSetTool(i)
						return ok()
					}
				}
				return nil, fmt.Errorf("tool must be one of %s", strings.Join(skToolNames, ", "))
			}},
		{Name: "sketch_view", Doc: "Set the sketch workspace's view: the mm at the board's center and px per mm. A revolve's or coil's center line stays two thirds across the board, so there x is ignored.",
			Params: []apiParam{req("x", "float", "mm"), req("y", "float", "mm"), req("zoom", "float", "px per mm")},
			fn: func(a *App, p args) (any, error) {
				if err := open(a); err != nil {
					return nil, err
				}
				a.Frame()
				s := a.sk
				s.cx, s.cy, s.zoom, s.viewSet = p.f("x", 0), p.f("y", 0), p.f("zoom", 6), true
				if !(s.zoom > 0) {
					s.zoom = 6
				}
				// within the board's own zoom limits, and the drawing's
				// range (at 1e-300 px per mm a click would put points at 1e302 mm)
				s.zoom = math.Max(0.05, math.Min(400, s.zoom))
				s.cx = math.Max(-1e5, math.Min(1e5, s.cx))
				s.cy = math.Max(-1e5, math.Min(1e5, s.cy))
				s.pinAxis() // (a revolve's or coil's center line stays put: x is ignored)
				return ok()
			}},
		{Name: "sketch_done", Doc: "Build the shape from the drawing and close the workspace (an error, and the workspace stays open, if the drawing builds nothing).",
			fn: func(a *App, p args) (any, error) {
				if err := open(a); err != nil {
					return nil, err
				}
				before := a.toast
				edited := a.sk.nodeID
				a.sketchDone()
				if a.sk != nil {
					msg := a.toast
					if msg == before || msg == "" {
						msg = "the sketch could not be built"
					}
					return nil, errors.New(msg)
				}
				// the shape edited (which Finish doesn't select), or the one
				// added
				id := 0
				if edited != 0 && a.ed.Doc.FindAny(edited) != nil {
					id = edited
				} else if len(a.ed.Sel) == 1 {
					id = a.ed.Sel[0]
				}
				return map[string]any{"id": id}, nil
			}},
		{Name: "sketch_cancel", Doc: "Close the sketch workspace, leaving the design as it was.",
			fn: func(a *App, p args) (any, error) {
				if err := open(a); err != nil {
					return nil, err
				}
				a.closeSketch()
				return ok()
			}},
	}
}
