package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"slices"
	"strings"

	"golang.org/x/image/draw"

	"github.com/scotticad/scotticad/pkg/script"
)

// Which API methods are tools. Every method is classified, and a test
// fails on one that isn't, so a new API method needs a decision here.
const (
	never = iota // not a tool
	full         // listed by default
	core         // listed in the core profile (--mcp-tools core) too
)

var toolSet = map[string]int{
	// design files
	"describe": never, "new_design": core, "open": core, "save": core, "get_document": full,
	"get_state": full, "list_nodes": core, "get_node": core, "shape_kinds": full,
	"version": never, "fonts": full,
	// creation
	"add_shape": core, "add_outline": full, "add_loft": full, "set_outline": full, "set_loft": full,
	// notes
	"add_note": never, "notes": never, "set_note_text": never, "delete_note": never,
	// import and export
	"import": full, "export": core,
	// selection: tools pass ids instead, the user may be selecting too
	"select": full, "add_to_selection": never, "select_all": never, "clear_selection": full,
	"get_selection": full,
	// transforms
	"move": core, "set_position": full, "set_size": core, "set_bounds": core, "rotate": core,
	"mirror": full, "align": full, "drop": full, "nudge": never,
	// appearance
	"set_hole": core, "set_color": core, "set_transparent": full, "set_multicolor": full,
	"set_locked": full, "set_name": full, "set_param": full, "set_text": full,
	// visibility
	"hide": full, "toggle_hidden": never, "show_all": full,
	// grouping
	"group": core, "ungroup": full, "set_group_mode": full,
	// split, lay flat and section view
	"split": full, "lay_flat": full, "set_section": full,
	// filament
	"filament_estimate": full, "set_filament_settings": full,
	// clipboard and undo
	"duplicate": full, "copy": never, "cut": never, "paste": never, "delete": core,
	"undo": core, "redo": core,
	// settings and tools: display aids, not design
	"set_units": never, "set_snap": never, "set_build_plate": full, "set_settings": never,
	"place_ruler": never, "remove_ruler": never, "toggle_construction_plane": full,
	"attach_construction_plane": full, "set_tool": never,
	// camera and output
	"get_camera": full, "set_camera": core, "home_view": full, "fit_view": core, "screenshot": core,
	"project": never, "get_ui_state": never, "list_controls": never, "list_handles": never,
	"click_control": never,
	// input simulation: an assistant clicking its way round the window is
	// fragile and slow
	"mouse_move": never, "mouse_down": never, "mouse_up": never, "click": never, "drag": never,
	"wheel": never, "key": never, "type_text": never,
	// scripting (stop_script is how a cancelled run_script stops)
	"run_script": core, "stop_script": never, "get_console": never, "script_api": never,
	// automated runs
	"answer_dialogs": never, "close_window": never, "lose_capture": never, "drop_files": never,
	"stats": never, "quit": never,
}

// readOnly are the tools that change nothing; destructive, those that
// discard work or overwrite files (the rest are undoable edits).
var (
	readOnly = map[string]bool{"get_document": true, "get_state": true, "list_nodes": true, "get_node": true,
		"shape_kinds": true, "fonts": true, "get_selection": true, "get_camera": true,
		"filament_estimate": true, "screenshot": true}
	destructive = map[string]bool{"new_design": true, "open": true, "save": true, "export": true, "delete": true}
)

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

// schemas are the JSON Schemas of the API's parameter types.
var schemas = map[string]map[string]any{
	"string":     {"type": "string"},
	"float":      {"type": "number"},
	"int":        {"type": "integer"},
	"bool":       {"type": "boolean"},
	"int[]":      {"type": "array", "items": map[string]any{"type": "integer"}},
	"string[]":   {"type": "array", "items": map[string]any{"type": "string"}},
	"float[3]":   {"type": "array", "items": map[string]any{"type": "number"}, "minItems": 3, "maxItems": 3},
	"axis":       {"type": "string", "enum": []string{"x", "y", "z"}},
	"any[]":      {"type": "array"},
	"float[][2]": {"type": "array", "items": point2},
	"float[][2] or float[][][2]": {"anyOf": []any{
		map[string]any{"type": "array", "items": point2},
		map[string]any{"type": "array", "items": map[string]any{"type": "array", "items": point2}},
	}},
	"[z, float[][2]][]": {"type": "array", "items": map[string]any{"type": "array", "minItems": 2, "maxItems": 2}},
}

var point2 = map[string]any{"type": "array", "items": map[string]any{"type": "number"}, "minItems": 2, "maxItems": 2}

// inputSchema is a method's parameters as a JSON Schema object.
func inputSchema(m Method) map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, p := range m.Params {
		sc := map[string]any{}
		for k, v := range schemas[p.Type] {
			sc[k] = v
		}
		d := p.Doc
		if _, ok := schemas[p.Type]; !ok || strings.Contains(p.Type, "[][") || p.Type == "any[]" {
			// the API's own notation says what goes in the arrays
			d = strings.TrimSpace(p.Type + ". " + d)
		}
		if d != "" {
			sc["description"] = d
		}
		props[p.Name] = sc
		if !p.Optional {
			required = append(required, p.Name)
		}
	}
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

const scriptPrimer = `

Scripts are Starlark, a Python dialect (functions, loops, lists, dicts; no classes, try or import; math is built in). The design is reached through cad:

    stand = cad.add_shape("box")
    cad.set_bounds(min=[0, 0, 0], max=[90, 20, 20], ids=[stand])
    hole = cad.add_shape("cylinder", hole=True)
    cad.set_bounds(min=[3, 8, 4], max=[7, 12, 21], ids=[hole])
    cad.group("union", ids=[stand, hole])   # cuts the hole out
    print(cad.node(stand))

Lengths are mm, angles degrees, Z up. Each run is one undo step. Put the sizes in named constants at the top. Every cad method is in the resource scotticad://script-api; help(cad.add_shape) prints one.`

const screenshotDoc = "Take a picture of the ScottiCAD window, to check your work. Frame it first with fit_view or set_camera; set_section cuts the design open to show the inside."

// tools are the tools listed for the profile, from describe's methods.
func (s *server) tools() ([]tool, error) {
	ms, err := s.listMethods()
	if err != nil {
		return nil, err
	}
	annotate := s.atLeast("2025-03-26")
	var out []tool
	for _, m := range ms {
		if !s.listed(m.Name) {
			continue
		}
		t := tool{Name: m.Name, Description: m.Doc, InputSchema: inputSchema(m)}
		switch m.Name {
		case "screenshot":
			t.Description = screenshotDoc
			t.InputSchema = map[string]any{"type": "object", "properties": map[string]any{
				"max_size": map[string]any{"type": "integer", "minimum": 256, "maximum": 4096,
					"description": "the picture's longest side in pixels (default 1024)"},
			}}
		case "run_script":
			t.Description += scriptPrimer
		}
		if annotate {
			t.Annotations = map[string]any{"readOnlyHint": readOnly[m.Name], "openWorldHint": false}
			if !readOnly[m.Name] {
				t.Annotations["destructiveHint"] = destructive[m.Name]
			}
		}
		out = append(out, t)
	}
	return out, nil
}

// listed reports whether a method is a tool in this profile.
func (s *server) listed(name string) bool {
	switch toolSet[name] {
	case core:
		return true
	case full:
		return s.o.Tools == "full"
	}
	return false
}

func textContent(t string) map[string]any { return map[string]any{"type": "text", "text": t} }

func toolError(t string) map[string]any {
	return map[string]any{"content": []any{textContent(t)}, "isError": true}
}

func (s *server) callTool(c *call, name string, a map[string]any) (any, error) {
	ms, err := s.listMethods()
	if err != nil {
		return toolError(err.Error()), nil
	}
	if !s.listed(name) || !slices.ContainsFunc(ms, func(m Method) bool { return m.Name == name }) {
		return nil, errorf(-32602, "unknown tool %q", name)
	}
	if a == nil {
		a = map[string]any{}
	}
	switch name {
	case "screenshot":
		return s.screenshot(a)
	case "run_script":
		return s.runScript(c, a)
	}
	r, err := s.call(name, a, s.o.Timeout)
	if err != nil {
		return toolError(err.Error()), nil
	}
	res := map[string]any{"content": []any{textContent(string(r))}}
	if bytes.HasPrefix(bytes.TrimSpace(r), []byte("{")) && s.atLeast("2025-06-18") {
		res["structuredContent"] = r
	}
	return res, nil
}

func (s *server) runScript(c *call, a map[string]any) (any, error) {
	if s.awaitCancelledScripts(c) {
		return nil, nil // cancelled before it started: no script, no reply
	}
	r, err := s.call("run_script", a, 0) // as long as it takes: Stop ends it
	if err != nil {
		return toolError(err.Error()), nil
	}
	var res struct {
		Output string  `json:"output"`
		Error  *string `json:"error"`
	}
	if err := json.Unmarshal(r, &res); err != nil {
		return toolError("run_script's reply: " + err.Error()), nil
	}
	if res.Error != nil && *res.Error != "" {
		t := "The script failed:\n" + *res.Error
		if res.Output != "" {
			t += "\n\nIts output before that:\n" + res.Output
		}
		return toolError(t), nil
	}
	out := res.Output
	if out == "" {
		out = "(the script ran and printed nothing)"
	}
	return map[string]any{"content": []any{textContent(out)}}, nil
}

func (s *server) screenshot(a map[string]any) (any, error) {
	size := 1024
	if v, ok := a["max_size"].(float64); ok {
		size = min(4096, max(256, int(v)))
	}
	r, err := s.call("screenshot", nil, s.o.Timeout)
	if err != nil {
		return toolError(err.Error()), nil
	}
	var res struct {
		PNG []byte `json:"png"` // base64 in the JSON
	}
	if err := json.Unmarshal(r, &res); err != nil || len(res.PNG) == 0 {
		return toolError("the screenshot came back without a picture: this ScottiCAD may be too old for it"), nil
	}
	pic, w, h, err := shrinkPNG(res.PNG, size)
	if err != nil {
		return toolError("the screenshot: " + err.Error()), nil
	}
	return map[string]any{"content": []any{
		map[string]any{"type": "image", "mimeType": "image/png", "data": pic},
		textContent(fmt.Sprintf("The ScottiCAD window, %d × %d px.", w, h)),
	}}, nil
}

// shrinkPNG scales a PNG down so its longest side is at most size pixels,
// returning it with its new width and height.
func shrinkPNG(b []byte, size int) ([]byte, int, int, error) {
	src, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, 0, 0, err
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if w <= size && h <= size {
		return b, w, h, nil
	}
	if w >= h {
		w, h = size, max(1, h*size/w)
	} else {
		w, h = max(1, w*size/h), size
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), w, h, nil
}

// --- resources

type resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

var resourceList = []resource{
	{"scotticad://script-api", "Script reference", "Every cad method scripts (run_script) can call, with its parameters.", "text/markdown"},
	{"scotticad://design", "The open design", "The design open in ScottiCAD, as its native JSON.", "application/json"},
	{"scotticad://shape-kinds", "Shape catalogue", "Every shape kind add_shape takes, with its parameters.", "application/json"},
}

func (s *server) readResource(uri string) (any, error) {
	var text, mime string
	switch uri {
	case "scotticad://script-api":
		t, err := s.scriptAPI()
		if err != nil {
			return nil, errorf(-32603, "%v", err)
		}
		text, mime = t, "text/markdown"
	case "scotticad://design", "scotticad://shape-kinds":
		method := map[string]string{"scotticad://design": "get_document", "scotticad://shape-kinds": "shape_kinds"}[uri]
		r, err := s.call(method, nil, s.o.Timeout)
		if err != nil {
			return nil, errorf(-32603, "%v", err)
		}
		text, mime = string(r), "application/json"
	default:
		return nil, errorf(-32002, "no resource %q", uri)
	}
	return map[string]any{"contents": []any{map[string]any{"uri": uri, "mimeType": mime, "text": text}}}, nil
}

// scriptAPI is the cad reference: each script method's call, and its API
// method's doc.
func (s *server) scriptAPI() (string, error) {
	r, err := s.call("script_api", nil, s.o.Timeout)
	if err != nil {
		return "", err
	}
	var sms []script.Method
	if err := json.Unmarshal(r, &sms); err != nil {
		return "", err
	}
	docs := map[string]string{}
	if ms, err := s.listMethods(); err == nil {
		for _, m := range ms {
			docs[m.Name] = m.Doc
		}
	}
	var b strings.Builder
	b.WriteString("# ScottiCAD scripts: the cad methods\n\n")
	b.WriteString("Scripts are Starlark, a dialect of Python: functions, loops, lists, dicts and comprehensions as in Python; no classes, try/except or import. math, json and struct are built in, and print writes to the output.\n\n")
	b.WriteString("Lengths are millimetres and angles degrees; Z is up and the build plate is Z = 0. Methods that act on the selection also take ids, a list of node ids: pass them. \"The current plane\" is the construction plane if one is placed, otherwise the build plate.\n\n")
	for i := range sms {
		fmt.Fprintf(&b, "- `cad.%s`", script.Signature(&sms[i]))
		if d := docs[sms[i].API]; d != "" {
			b.WriteString(": " + d)
		}
		b.WriteString("\n")
	}
	b.WriteString("- `cad.call(method, **params)`: any API method by name\n")
	return b.String(), nil
}

// instructions go to the assistant with initialize.
const instructions = `ScottiCAD is a CAD program for 3D printing, open in a window the user can see and use while you work. Lengths are millimetres and angles degrees; Z is up and the build plate is Z = 0.

- To build anything with more than a few shapes, or anything repeated or calculated, write a Starlark script and run it with run_script: one call, and one undo step. Keep the sizes as named constants at the top. The cad reference is the resource scotticad://script-api.
- Use the other tools to look at a design (list_nodes, get_node) and for small edits to shapes already there.
- Pass ids rather than relying on the selection: the user may be working in the same window and can change it under you. For the same reason, list the nodes again after the user has made changes.
- After a change, take a screenshot to check it: fit_view or set_camera frame it, and set_section shows the inside.
- Holes are shapes made with hole=True (or set_hole), cut out when grouped with solid shapes. Make a hole for a part that fits into it 0.2 to 0.4 mm bigger, to allow for printing.
- Don't save over the user's file unless asked: save to a new path.`
