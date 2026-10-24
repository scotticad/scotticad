package script

// The cad object's methods mirror the Python client (python/scotticad):
// the same names, parameters, defaults and results, so a script reads the
// same in the console as in an external Python program. Each method is a
// call to an automation API method; python/tests checks the two lists
// against each other.

// Param is one parameter of a cad method. A nil Default is Python's None:
// left out of the call when not given.
type Param struct {
	Name     string `json:"name"`
	Required bool   `json:"required,omitempty"`
	Default  any    `json:"default"`
	// Path parameters are made absolute (against the script's folder) like
	// the Python client's os.path.abspath.
	Path bool `json:"path,omitempty"`
}

// Method is one cad method.
type Method struct {
	Name   string  `json:"name"`
	API    string  `json:"api"` // the automation API method it calls
	Params []Param `json:"params"`
	// Result names the field of the API's reply the method returns
	// ("" returns the whole reply).
	Result string `json:"result,omitempty"`
	// None returns None instead of the reply (the Python method returns
	// nothing).
	None bool `json:"-"`
	// special handling, see Session.callMethod
	special string
}

func p(name string) Param                { return Param{Name: name, Required: true} }
func d(name string, def any) Param       { return Param{Name: name, Default: def} }
func path(name string) Param             { return Param{Name: name, Required: true, Path: true} }
func optPath(name string, def any) Param { return Param{Name: name, Default: def, Path: true} }

var ids = d("ids", nil)

// Methods are the cad methods, in the Python client's order.
var Methods = []Method{
	// design files
	{Name: "describe", API: "describe"},
	{Name: "new_design", API: "new_design", None: true},
	{Name: "open", API: "open", Params: []Param{path("path")}, None: true},
	{Name: "save", API: "save", Params: []Param{optPath("path", nil)}, Result: "path"},
	{Name: "get_document", API: "get_document"},
	{Name: "state", API: "get_state"},
	{Name: "nodes", API: "list_nodes", Params: []Param{d("deep", false)}},
	{Name: "node", API: "get_node", Params: []Param{p("id")}},
	{Name: "shape_kinds", API: "shape_kinds"},
	{Name: "fonts", API: "fonts"},

	// creation
	{Name: "add_shape", API: "add_shape", Params: []Param{p("kind"), d("x", 0), d("y", 0), d("z", nil), d("hole", false)}, Result: "id"},
	{Name: "add_outline", API: "add_outline", Params: []Param{p("points"), d("z0", 0), d("z1", 10), d("hole", false)}, Result: "id"},
	{Name: "add_loft", API: "add_loft", Params: []Param{p("sections"), d("hole", false)}, Result: "id"},
	{Name: "import_file", API: "import", Params: []Param{path("path"), d("x", 0), d("y", 0), d("scale", 1.0), d("units", "mm"), d("height", 10.0)}, Result: "id"},
	{Name: "export", API: "export", Params: []Param{path("path"), d("selection_only", false)}, None: true},

	// selection
	{Name: "select", API: "select", Params: []Param{p("ids")}},
	{Name: "add_to_selection", API: "add_to_selection", Params: []Param{p("id")}},
	{Name: "select_all", API: "select_all"},
	{Name: "clear_selection", API: "clear_selection", None: true},
	{Name: "selection", API: "get_selection"},

	// transforms
	{Name: "move", API: "move", Params: []Param{d("dx", 0), d("dy", 0), d("dz", 0), ids}, None: true},
	{Name: "set_position", API: "set_position", Params: []Param{d("x", nil), d("y", nil), d("z", nil), d("anchor", "min"), ids}, None: true},
	{Name: "set_size", API: "set_size", Params: []Param{d("x", nil), d("y", nil), d("z", nil), d("uniform", false), ids}, None: true},
	{Name: "set_bounds", API: "set_bounds", Params: []Param{p("min"), p("max"), ids}, None: true},
	{Name: "rotate", API: "rotate", Params: []Param{p("axis"), p("degrees"), ids}, None: true},
	{Name: "mirror", API: "mirror", Params: []Param{p("axis"), ids}, None: true},
	{Name: "align", API: "align", Params: []Param{p("axis"), p("where"), ids, d("key", nil)}, None: true},
	{Name: "drop", API: "drop", Params: []Param{ids}, None: true},
	{Name: "lay_flat", API: "lay_flat", Params: []Param{p("normal"), ids}, None: true},
	{Name: "nudge", API: "nudge", Params: []Param{p("axis"), d("steps", 1)}, None: true},

	// appearance
	{Name: "set_hole", API: "set_hole", Params: []Param{d("hole", true), ids}, None: true},
	{Name: "set_color", API: "set_color", Params: []Param{p("color"), ids}, None: true},
	{Name: "set_transparent", API: "set_transparent", Params: []Param{d("on", true), ids}, None: true},
	{Name: "set_multicolor", API: "set_multicolor", Params: []Param{d("on", true), ids}, None: true},
	{Name: "set_locked", API: "set_locked", Params: []Param{d("on", true), ids}, None: true},
	{Name: "set_name", API: "set_name", Params: []Param{p("name"), ids}, None: true},
	{Name: "set_param", API: "set_param", Params: []Param{p("id"), p("key"), p("value")}, None: true},
	{Name: "set_text", API: "set_text", Params: []Param{p("id"), p("text"), d("font", nil)}, None: true},
	{Name: "set_outline", API: "set_outline", Params: []Param{p("id"), p("points"), d("z0", nil), d("z1", nil)}, None: true},
	{Name: "set_loft", API: "set_loft", Params: []Param{p("id"), p("sections")}, None: true},

	// visibility
	{Name: "hide", API: "hide", Params: []Param{ids}, None: true},
	{Name: "toggle_hidden", API: "toggle_hidden", Params: []Param{p("id")}, None: true},
	{Name: "show_all", API: "show_all", None: true},

	// grouping and clipboard
	{Name: "group", API: "group", Params: []Param{d("mode", "union"), ids}, Result: "id"},
	{Name: "ungroup", API: "ungroup", Params: []Param{ids}, Result: "ids"},
	{Name: "set_group_mode", API: "set_group_mode", Params: []Param{p("mode"), ids}, None: true},
	{Name: "split", API: "split", Params: []Param{p("axis"), d("at", nil), d("keep", "both"), d("gap", 0), ids}, Result: "ids"},
	{Name: "duplicate", API: "duplicate", Result: "ids"},
	{Name: "copy", API: "copy", None: true},
	{Name: "cut", API: "cut", None: true},
	{Name: "paste", API: "paste", Params: []Param{d("in_place", false)}, Result: "ids"},
	{Name: "delete", API: "delete", Params: []Param{ids}, None: true},
	{Name: "undo", API: "undo", Result: "done"},
	{Name: "redo", API: "redo", Result: "done"},

	// settings and tools
	{Name: "set_units", API: "set_units", Params: []Param{p("units")}, None: true},
	{Name: "set_snap", API: "set_snap", Params: []Param{p("mm")}, None: true},
	{Name: "set_build_plate", API: "set_build_plate", Params: []Param{p("width"), p("length")}, None: true},
	{Name: "set_settings", API: "set_settings", Params: []Param{d("show_grid", nil), d("show_shadows", nil)}, None: true},
	{Name: "place_ruler", API: "place_ruler", Params: []Param{p("x"), p("y"), d("midpoint", false), d("turns", 0)}, None: true},
	{Name: "add_note", API: "add_note", Params: []Param{p("id"), p("x"), p("y"), p("z"), d("text", "")}, Result: "index"},
	{Name: "notes", API: "notes", special: "list"},
	{Name: "set_note_text", API: "set_note_text", Params: []Param{p("id"), p("index"), p("text")}, None: true},
	{Name: "delete_note", API: "delete_note", Params: []Param{p("id"), p("index")}, None: true},
	{Name: "remove_ruler", API: "remove_ruler", None: true},
	{Name: "toggle_construction_plane", API: "toggle_construction_plane", None: true},
	{Name: "attach_construction_plane", API: "attach_construction_plane", Params: []Param{p("id")}, None: true},
	{Name: "set_tool", API: "set_tool", Params: []Param{p("tool")}, None: true},
	{Name: "filament_estimate", API: "filament_estimate", Params: []Param{ids}},
	{Name: "set_filament_settings", API: "set_filament_settings", Params: []Param{d("material", nil), d("density", nil), d("diameter", nil),
		d("layer_height", nil), d("line_width", nil), d("walls", nil), d("top_layers", nil), d("bottom_layers", nil), d("infill", nil),
		d("price_per_kg", nil), d("calibration", nil)}},
	{Name: "set_section", API: "set_section", Params: []Param{d("on", true), d("axis", nil), d("at", nil), d("keep", nil), d("pin", nil)}, None: true},

	// camera and output
	{Name: "camera", API: "get_camera"},
	{Name: "set_camera", API: "set_camera", Params: []Param{d("target", nil), d("yaw", nil), d("pitch", nil), d("dist", nil), d("ortho", nil)}, None: true},
	{Name: "home_view", API: "home_view", None: true},
	{Name: "fit_view", API: "fit_view", None: true},
	{Name: "screenshot", API: "screenshot", Params: []Param{path("path")}},
	{Name: "project", API: "project", Params: []Param{p("x"), p("y"), p("z")}, special: "xy"},
	{Name: "version", API: "version"},
	{Name: "ui_state", API: "get_ui_state"},
	{Name: "controls", API: "list_controls"},
	{Name: "handles", API: "list_handles", special: "list"},
	{Name: "handle", API: "list_handles", Params: []Param{p("kind"), d("index", 0)}, special: "handle"},
	{Name: "click_control", API: "click_control", Params: []Param{p("id")}, None: true},

	// input simulation
	{Name: "mouse_move", API: "mouse_move", Params: []Param{p("x"), p("y"), d("mods", []any{})}, None: true},
	{Name: "mouse_down", API: "mouse_down", Params: []Param{p("x"), p("y"), d("button", "left"), d("mods", []any{})}, None: true},
	{Name: "mouse_up", API: "mouse_up", Params: []Param{p("x"), p("y"), d("button", "left"), d("mods", []any{})}, None: true},
	{Name: "click", API: "click", Params: []Param{p("x"), p("y"), d("button", "left"), d("mods", []any{})}, None: true},
	{Name: "drag", API: "drag", Params: []Param{p("x0"), p("y0"), p("x1"), p("y1"), d("steps", 10), d("button", "left"), d("mods", []any{})}, None: true},
	{Name: "wheel", API: "wheel", Params: []Param{p("x"), p("y"), p("delta")}, None: true},
	{Name: "key", API: "key", Params: []Param{p("keys")}, None: true},
	{Name: "type_text", API: "type_text", Params: []Param{p("text")}, None: true},

	// the console
	{Name: "console", API: "get_console"},
}

// NotInScripts lists the Python client's methods that scripts don't get,
// with the reason.
var NotInScripts = map[string]string{
	"quit":           "a script can't close the program it runs in",
	"close":          "closes the Python client's connection",
	"run_script":     "a script can't start another script",
	"answer_dialogs": "for automated runs: answers the prompts and file dialogs",
	"close_window":   "for automated runs: what closing the window would do",
	"drop_files":     "for automated runs: files dropped on the window",
	"lose_capture":   "for automated runs: the mouse taken away mid-drag",
	"stats":          "for soak tests: the program's memory and cache figures",
}
