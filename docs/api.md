# ScottiCAD automation API

ScottiCAD can be driven by another program: everything the editor can do,
from adding shapes and grouping them to exporting STL files, is available
as a command, including simulated mouse and keyboard input and clicking
named on-screen controls. This document describes the API and every
command.

There are three ways in:

- **Python**: the client in [`python/scotticad`](../python/scotticad)
  starts ScottiCAD and wraps every command in a method.
- **Scripts** inside ScottiCAD: the Scripting console's `cad` object has
  the same methods as the Python client (see
  [Scripting](../README.md#scripting)).
- **Any language**: the API is JSON-RPC 2.0 over a TCP socket, one JSON
  object per line.

All lengths are millimetres and all angles degrees, whatever units the
design shows. Methods that act on "the selection" also take `ids`, a list
of top-level node ids, to act on those instead. "The current plane" is the
construction plane if one is placed, otherwise the build plate.

## Starting ScottiCAD with the API

```
scotticad --api 127.0.0.1:47800
scotticad --api 127.0.0.1:0 --headless
```

`--api HOST:PORT` serves the API on that address; port 0 picks a free one.
Once it is listening, ScottiCAD prints one line to standard output with
the address it chose, which a program starting it can read:

```
SCOTTICAD_API 127.0.0.1:53114
```

Other options:

| Option | Meaning |
|---|---|
| `--headless` | a hidden window, for automated tests |
| `--width`, `--height` | the window's size in logical pixels (200 to 16384; default 1600 × 1000) |
| `--scale` | the interface scale (0.5 to 5; 2 is 200%); default: the display's |
| `--version` | print the version and exit |

A design file named on the command line is opened at start.

**The API has no authentication.** Whatever can connect to the socket can
do anything the editor can, including opening and saving files. Serve it
on `127.0.0.1` (the loopback address) only, never on an address other
machines can reach. A line that isn't JSON (a web page's HTTP request,
say) gets an error reply, and the connection is closed.

## From Python

```python
from scotticad import launch

with launch(headless=True) as cad:
    box  = cad.add_shape("box", x=0, y=0)
    hole = cad.add_shape("cylinder", hole=True)
    cad.set_size(x=8, y=8, ids=[hole])
    cad.select([box, hole])
    cad.align("x", "center"); cad.align("y", "center")
    cad.group("union")
    cad.export("part.stl")
```

`launch(exe=None, headless=True, width=1600, height=1000, design=None,
timeout=30.0, scale=None)` starts ScottiCAD with the API on a free port
and returns a connected client; leaving the `with` block quits ScottiCAD.
It runs the program built at the top of the repository (`scotticad.exe`
on Windows, `scotticad` on Linux), or the one the `SCOTTICAD_EXE`
environment variable names.

To connect to a ScottiCAD that is already running with `--api`, make a
`Client(host="127.0.0.1", port=47800)`; `close()` disconnects without
quitting it.

Each method below lists the client method that calls it. Every command is
also reachable as `cad.call("method", param=value, ...)`, which returns
the reply as is. A parameter passed as `None` is left out of the call. A
command that fails raises `ScottiCADError` with the server's message.

## From scripts

In a script run in ScottiCAD's Scripting console (or sent with
`run_script`), the `cad` object has the Python client's methods with the
same names, parameters and results, so the same code runs in both places.
A few client methods aren't available in scripts; the reference notes
which and why.

## The protocol

ScottiCAD speaks [JSON-RPC 2.0](https://www.jsonrpc.org/specification)
over TCP: each request is one JSON object on one line, and each reply is
one line too. `params` is an object (or left out); `id` is echoed back.

```
→ {"jsonrpc": "2.0", "id": 1, "method": "add_shape", "params": {"kind": "box", "x": 10}}
← {"jsonrpc":"2.0","id":1,"result":{"id":1}}
→ {"jsonrpc": "2.0", "id": 2, "method": "set_size", "params": {"x": "wide"}}
← {"jsonrpc":"2.0","id":2,"error":{"code":-32000,"message":"parameter \"x\" must be float"}}
```

A command with nothing else to return replies `{"ok": true}`.

| Error code | Meaning |
|---|---|
| -32700 | the line isn't JSON (the connection is then closed) |
| -32602 | `params` isn't an object |
| -32000 | the command failed: a missing or malformed parameter, an unknown method, or the command's own error |
| -32603 | the result couldn't be encoded (a reply holding a NaN, say) |

Parameters are checked before a command runs: a required one missing, a
number that isn't one, or `ids` that aren't a list of node ids is an
error, and nothing changes. A `null` parameter is the same as leaving it
out.

**When commands run.** Each command runs on the editor's own thread between
frames, one at a time, in the order received, so it sees exactly what the
window shows. Shapes combine in the background; a command first waits for
that to finish, so it sees exact geometry. Pass `"live": true` among any
command's parameters to skip the wait and see the window as it is (a
screenshot while shapes are still being combined, say).

**Window coordinates.** The commands that simulate input (`click`, `drag`,
`mouse_move`, ...) and those that report where things are on screen
(`project`, `list_controls`, `list_handles`) use window coordinates in
logical pixels: (0, 0) is the window's top left, and a pixel is a pixel
at 100% scale.

**Discovering commands.** `describe` returns the method table this
reference is made from: every method with its parameters.

<!-- Everything below is generated from pkg/app/api.go: go test ./pkg/app -run TestAPIDoc -update -->

## Methods

- **Design files**: [`describe`](#describe), [`new_design`](#new_design), [`open`](#open), [`save`](#save), [`get_document`](#get_document), [`get_state`](#get_state), [`list_nodes`](#list_nodes), [`get_node`](#get_node), [`shape_kinds`](#shape_kinds), [`version`](#version), [`fonts`](#fonts)
- **Creation**: [`add_shape`](#add_shape), [`add_outline`](#add_outline), [`add_loft`](#add_loft), [`set_outline`](#set_outline), [`set_loft`](#set_loft)
- **Notes**: [`add_note`](#add_note), [`notes`](#notes), [`set_note_text`](#set_note_text), [`delete_note`](#delete_note)
- **Import and export**: [`import`](#import), [`export`](#export)
- **Selection**: [`select`](#select), [`add_to_selection`](#add_to_selection), [`select_all`](#select_all), [`clear_selection`](#clear_selection), [`get_selection`](#get_selection)
- **Transforms**: [`move`](#move), [`set_position`](#set_position), [`set_size`](#set_size), [`set_bounds`](#set_bounds), [`rotate`](#rotate), [`mirror`](#mirror), [`align`](#align), [`drop`](#drop), [`nudge`](#nudge)
- **Appearance**: [`set_hole`](#set_hole), [`set_color`](#set_color), [`set_transparent`](#set_transparent), [`set_multicolor`](#set_multicolor), [`set_locked`](#set_locked), [`set_name`](#set_name), [`set_param`](#set_param), [`set_text`](#set_text)
- **Visibility**: [`hide`](#hide), [`toggle_hidden`](#toggle_hidden), [`show_all`](#show_all)
- **Grouping**: [`group`](#group), [`ungroup`](#ungroup), [`set_group_mode`](#set_group_mode)
- **Split, lay flat and section view**: [`split`](#split), [`lay_flat`](#lay_flat), [`set_section`](#set_section)
- **Filament**: [`filament_estimate`](#filament_estimate), [`set_filament_settings`](#set_filament_settings)
- **Clipboard and undo**: [`duplicate`](#duplicate), [`copy`](#copy), [`cut`](#cut), [`paste`](#paste), [`delete`](#delete), [`undo`](#undo), [`redo`](#redo)
- **Settings and tools**: [`set_units`](#set_units), [`set_snap`](#set_snap), [`set_build_plate`](#set_build_plate), [`set_settings`](#set_settings), [`place_ruler`](#place_ruler), [`remove_ruler`](#remove_ruler), [`toggle_construction_plane`](#toggle_construction_plane), [`attach_construction_plane`](#attach_construction_plane), [`set_tool`](#set_tool)
- **Camera and output**: [`get_camera`](#get_camera), [`set_camera`](#set_camera), [`home_view`](#home_view), [`fit_view`](#fit_view), [`screenshot`](#screenshot), [`project`](#project), [`get_ui_state`](#get_ui_state), [`list_controls`](#list_controls), [`list_handles`](#list_handles), [`click_control`](#click_control)
- **Input simulation (window coordinates in logical px)**: [`mouse_move`](#mouse_move), [`mouse_down`](#mouse_down), [`mouse_up`](#mouse_up), [`click`](#click), [`drag`](#drag), [`wheel`](#wheel), [`key`](#key), [`type_text`](#type_text)
- **Scripting**: [`run_script`](#run_script), [`get_console`](#get_console), [`script_api`](#script_api)
- **Automated runs**: [`answer_dialogs`](#answer_dialogs), [`close_window`](#close_window), [`lose_capture`](#lose_capture), [`drop_files`](#drop_files), [`stats`](#stats), [`quit`](#quit)

### Design files

#### `describe`

List every API method with its parameters.

Python: `cad.describe()`

#### `new_design`

Start a new empty design (discards unsaved changes).

Python: `cad.new_design()` returns nothing

#### `open`

Open a .scotticad design.

Python: `cad.open(path)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `path` | string | required | file path |

#### `save`

Save the design (native .scotticad JSON with the full group tree).

Python: `cad.save(path=None)` returns the reply's `path`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `path` | string | optional | file path; defaults to the current file |

#### `get_document`

Return the design as native JSON.

Python: `cad.get_document()`

#### `get_state`

Design and editor state summary.

Python: `cad.state()`

#### `list_nodes`

Top-level nodes with bounds and volume.

Python: `cad.nodes(deep=False)`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `deep` | bool | optional | include children trees |

#### `get_node`

One node (any depth) with its children tree.

Python: `cad.node(id)`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | int | required |  |

#### `shape_kinds`

The shape catalogue with parameters.

Python: `cad.shape_kinds()`

#### `version`

ScottiCAD's version (from the VERSION file).

Python: `cad.version()`

#### `fonts`

Font names usable by text shapes.

Python: `cad.fonts()`

### Creation

#### `add_shape`

Place a catalogue shape with its bottom centre at (x, y, z); selects it.

Python: `cad.add_shape(kind, x=0, y=0, z=None, hole=False)` returns the reply's `id`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `kind` | string | required | shape id, see shape_kinds |
| `x` | float | optional | mm |
| `y` | float | optional | mm |
| `z` | float | optional | mm; default: on the current plane |
| `hole` | bool | optional |  |

#### `add_outline`

Extrude closed outlines straight up from z0 to z1 (outlines inside others are holes); points are world mm. Selects it.

Python: `cad.add_outline(points, z0=0, z1=10, hole=False)` returns the reply's `id`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `points` | float[][2] or float[][][2] | required | [[x, y], ...], or a list of such outlines |
| `z0` | float | optional | mm, default 0 |
| `z1` | float | optional | mm, default 10 |
| `hole` | bool | optional |  |

#### `add_loft`

Join closed outlines at rising heights with straight walls and close the ends; points are world mm. Selects it.

Python: `cad.add_loft(sections, hole=False)` returns the reply's `id`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `sections` | [z, float[][2]][] | required | [[z, [[x, y], ...]], ...], at least 2, z rising; same point counts join point to point |
| `hole` | bool | optional |  |

#### `set_outline`

Replace an outline's points (and heights); they are in the shape's own frame, so an unmoved shape puts them where they say.

Python: `cad.set_outline(id, points, z0=None, z1=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | int | required | node id (any depth) |
| `points` | float[][2] or float[][][2] | required | as add_outline |
| `z0` | float | optional | mm, default unchanged |
| `z1` | float | optional | mm, default unchanged |

#### `set_loft`

Replace a loft's sections; they are in the shape's own frame, so an unmoved shape puts them where they say.

Python: `cad.set_loft(id, sections)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | int | required | node id (any depth) |
| `sections` | [z, float[][2]][] | required | as add_loft |

### Notes

#### `add_note`

Pin a note to top-level shape id at the world point (x, y, z); returns its index on that shape.

Python: `cad.add_note(id, x, y, z, text="")` returns the reply's `index`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | int | required |  |
| `x` | float | required | mm |
| `y` | float | required | mm |
| `z` | float | required | mm |
| `text` | string | optional |  |

#### `notes`

Every note: shape id, index on that shape, text, world position, and whether it is open.

Python: `cad.notes()`

#### `set_note_text`

Change a note's text.

Python: `cad.set_note_text(id, index, text)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | int | required |  |
| `index` | int | required |  |
| `text` | string | required |  |

#### `delete_note`

Remove a note.

Python: `cad.delete_note(id, index)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | int | required |  |
| `index` | int | required |  |

### Import and export

#### `import`

Import an STL, OBJ or SVG file onto the current plane; selects it.

Python: `cad.import_file(path, x=0, y=0, scale=1.0, units="mm", height=10.0)` returns the reply's `id`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `path` | string | required |  |
| `x` | float | optional | mm |
| `y` | float | optional | mm |
| `scale` | float | optional | multiplier, default 1 |
| `units` | string | optional | mm or in (STL/OBJ) |
| `height` | float | optional | SVG extrusion height, mm |

#### `export`

Export evaluated geometry to .stl, .obj, .3mf or .glb, or its base outline to .svg (laser cutting).

Python: `cad.export(path, selection_only=False)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `path` | string | required |  |
| `selection_only` | bool | optional | export only the selection |

### Selection

#### `select`

Replace the selection.

Python: `cad.select(ids)`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `ids` | int[] | required |  |

#### `add_to_selection`

Add a shape to the selection (already selected: no change); returns the selection.

Python: `cad.add_to_selection(id)`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | int | required |  |

#### `select_all`

Select every visible shape.

Python: `cad.select_all()`

#### `clear_selection`

Deselect everything.

Python: `cad.clear_selection()` returns nothing

#### `get_selection`

Selected ids and their combined bounds.

Python: `cad.selection()`

### Transforms

#### `move`

Translate by (dx, dy, dz) mm.

Python: `cad.move(dx=0, dy=0, dz=0, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `dx` | float | optional |  |
| `dy` | float | optional |  |
| `dz` | float | optional |  |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_position`

Move so the bounding box's anchor point lands at (x, y, z); omitted axes are unchanged.

Python: `cad.set_position(x=None, y=None, z=None, anchor="min", ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `x` | float | optional |  |
| `y` | float | optional |  |
| `z` | float | optional |  |
| `anchor` | string | optional | "min" (default) or "center" |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_size`

Resize the bounding box (mm); omitted axes keep their size. X/Y stay centred, Z keeps the base.

Python: `cad.set_size(x=None, y=None, z=None, uniform=False, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `x` | float | optional |  |
| `y` | float | optional |  |
| `z` | float | optional |  |
| `uniform` | bool | optional | scale all axes by the one given |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_bounds`

Fit the selection's bounding box exactly to [min, max].

Python: `cad.set_bounds(min, max, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `min` | float[3] | required |  |
| `max` | float[3] | required |  |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `rotate`

Rotate about a world axis through the selection centre.

Python: `cad.rotate(axis, degrees, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `axis` | axis | required | x, y or z |
| `degrees` | float | required | positive = counter-clockwise looking down the axis |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `mirror`

Mirror the selection about its centre.

Python: `cad.mirror(axis, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `axis` | axis | required | x, y or z |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `align`

Align the selection (a single shape aligns to the current plane). With key, the others align to that shape, which stays put.

Python: `cad.align(axis, where, ids=None, key=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `axis` | axis | required | x, y or z |
| `where` | string | required | min, center or max |
| `key` | int | optional | id of the key object |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `drop`

Drop down onto the current plane (D).

Python: `cad.drop(ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `nudge`

Move by snap-grid steps (arrow keys).

Python: `cad.nudge(axis, steps=1)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `axis` | axis | required | x, y or z |
| `steps` | float | optional | default 1 |

### Appearance

#### `set_hole`

Make the selection Hole (true) or Solid (false).

Python: `cad.set_hole(hole=True, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `hole` | bool | required |  |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_color`

Set colour (#rrggbb); makes the shapes solid.

Python: `cad.set_color(color, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `color` | string | required | #rrggbb |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_transparent`

Transparent display on/off.

Python: `cad.set_transparent(on=True, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `on` | bool | required |  |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_multicolor`

Union groups: keep each child's colour.

Python: `cad.set_multicolor(on=True, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `on` | bool | required |  |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_locked`

Lock or unlock.

Python: `cad.set_locked(on=True, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `on` | bool | required |  |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_name`

Name shown in the Properties header.

Python: `cad.set_name(name, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `name` | string | required |  |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_param`

Set a shape parameter (see shape_kinds).

Python: `cad.set_param(id, key, value)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | int | required | node id (any depth) |
| `key` | string | required |  |
| `value` | float | required |  |

#### `set_text`

Change a text shape's string and/or font.

Python: `cad.set_text(id, text, font=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | int | required |  |
| `text` | string | required |  |
| `font` | string | optional |  |

### Visibility

#### `hide`

Hide the selection (Ctrl+H).

Python: `cad.hide(ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `toggle_hidden`

Toggle one shape's visibility.

Python: `cad.toggle_hidden(id)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | int | required |  |

#### `show_all`

Show all hidden shapes (Ctrl+Shift+H).

Python: `cad.show_all()` returns nothing

### Grouping

#### `group`

Group the selection; returns the new group id.

Python: `cad.group(mode="union", ids=None)` returns the reply's `id`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `mode` | string | optional | "union" (default) or "bundle" |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `ungroup`

Ungroup selected groups one level; returns released ids.

Python: `cad.ungroup(ids=None)` returns the reply's `ids`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_group_mode`

Switch selected groups between union, bundle and intersect.

Python: `cad.set_group_mode(mode, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `mode` | string | required | union, bundle or intersect |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

### Split, lay flat and section view

#### `split`

Split shapes with a plane square to an axis of the current plane; each becomes two halves (groups of the shape and a hole box). Returns the halves' ids.

Python: `cad.split(axis, at=None, keep="both", gap=0, ids=None)` returns the reply's `ids`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `axis` | axis | required | x, y or z (the current plane's) |
| `at` | float | optional | where along the axis, mm; default: the selection's centre |
| `keep` | string | optional | both, above or below (the + or - side of the axis) |
| `gap` | float | optional | mm to move the half above (+ side) away; the one below stays |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `lay_flat`

Turn the selection so the world direction normal (a face's outward normal) points down into the current plane, and drop it there.

Python: `cad.lay_flat(normal, ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `normal` | float[3] | required | the face's outward normal |
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_section`

Section view: hide everything on one side of a plane square to an axis of the current plane (the view only; the design is unchanged).

Python: `cad.set_section(on=True, axis=None, at=None, keep=None, pin=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `on` | bool | optional | default true |
| `axis` | axis | optional | x, y or z; default y |
| `at` | float | optional | where along the axis, mm; default: the design's middle |
| `keep` | string | optional | which side shows: above or below (the + or - side); default: the side away from the camera |
| `pin` | bool | optional | stay open when the selection changes |

### Filament

#### `filament_estimate`

Estimate the filament the selection (or ids, or the whole design) takes: grams, metres, cost, and per colour. Supports, brims and purge aren't included.

Python: `cad.filament_estimate(ids=None)`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `set_filament_settings`

Change the filament estimate's settings (omitted ones are kept); returns them. material: a preset (PLA, PETG, ABS, ASA, TPU, PC, Nylon) sets its density.

Python: `cad.set_filament_settings(material=None, density=None, diameter=None, layer_height=None, line_width=None, walls=None, top_layers=None, bottom_layers=None, infill=None, price_per_kg=None, calibration=None)`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `material` | string | optional |  |
| `density` | float | optional | g/cm³ |
| `diameter` | float | optional | mm |
| `layer_height` | float | optional | mm |
| `line_width` | float | optional | mm |
| `walls` | int | optional |  |
| `top_layers` | int | optional |  |
| `bottom_layers` | int | optional |  |
| `infill` | float | optional | percent |
| `price_per_kg` | float | optional |  |
| `calibration` | float | optional | percent added to every estimate |

### Clipboard and undo

#### `duplicate`

Ctrl+D; returns new ids.

Python: `cad.duplicate()` returns the reply's `ids`

#### `copy`

Ctrl+C.

Python: `cad.copy()` returns nothing

#### `cut`

Ctrl+X.

Python: `cad.cut()` returns nothing

#### `paste`

Ctrl+V (or Ctrl+Shift+V in place); returns new ids.

Python: `cad.paste(in_place=False)` returns the reply's `ids`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `in_place` | bool | optional |  |

#### `delete`

Delete shapes.

Python: `cad.delete(ids=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `ids` | int[] | optional | top-level node ids; defaults to the selection |

#### `undo`

Undo one step.

Python: `cad.undo()` returns the reply's `done`

#### `redo`

Redo one step.

Python: `cad.redo()` returns the reply's `done`

### Settings and tools

#### `set_units`

Units: mm or in.

Python: `cad.set_units(units)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `units` | string | required | mm or in |

#### `set_snap`

Snap grid in mm (0 = off).

Python: `cad.set_snap(mm)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `mm` | float | required |  |

#### `set_build_plate`

Build plate size in mm.

Python: `cad.set_build_plate(width, length)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `width` | float | required |  |
| `length` | float | required |  |

#### `set_settings`

Settings toggles.

Python: `cad.set_settings(show_grid=None, show_shadows=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `show_grid` | bool | optional |  |
| `show_shadows` | bool | optional |  |

#### `place_ruler`

Place the ruler origin on the current plane.

Python: `cad.place_ruler(x, y, midpoint=False, turns=0)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `x` | float | required |  |
| `y` | float | required |  |
| `midpoint` | bool | optional |  |
| `turns` | int | optional | quarter turns counter-clockwise |

#### `remove_ruler`

Remove the ruler.

Python: `cad.remove_ruler()` returns nothing

#### `toggle_construction_plane`

The Construction plane on shape button: put a construction plane on the selected shape, or remove it.

Python: `cad.toggle_construction_plane()` returns nothing

#### `attach_construction_plane`

Put a construction plane on top-level shape id (its own axes, on its bottom); id 0 removes it, back to the build plate.

Python: `cad.attach_construction_plane(id)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | int | required |  |

#### `set_tool`

Activate a tool mode.

Python: `cad.set_tool(tool)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `tool` | string | required | none, align, mirror, measure, visibility, ruler, notes, construction_plane, split, layflat, cruise |

### Camera and output

#### `get_camera`

Camera (angles in degrees).

Python: `cad.camera()`

#### `set_camera`

Set camera fields (angles in degrees).

Python: `cad.set_camera(target=None, yaw=None, pitch=None, dist=None, ortho=None)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `target` | float[3] | optional |  |
| `yaw` | float | optional |  |
| `pitch` | float | optional |  |
| `dist` | float | optional |  |
| `ortho` | bool | optional |  |

#### `home_view`

Home view.

Python: `cad.home_view()` returns nothing

#### `fit_view`

Fit selection (or all) in view.

Python: `cad.fit_view()` returns nothing

#### `screenshot`

Render the window to a PNG.

Python: `cad.screenshot(path)`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `path` | string | required |  |

#### `project`

World point (mm) to window coordinates (logical px).

Python: `cad.project(x, y, z)`

| Parameter | Type | | Meaning |
|---|---|---|---|
| `x` | float | required |  |
| `y` | float | required |  |
| `z` | float | required |  |

#### `get_ui_state`

Tool mode, open popup, hover, active field, banner and toast; the file an export from the Export dialog is writing in the background; the scale and font atlas; the section; in align, mirror and measure modes the align dots, mirror arrows (screen x, y) or measure points.

Python: `cad.ui_state()`

#### `list_controls`

Every clickable control drawn in the last frame (id, rect, tooltip).

Python: `cad.controls()`

#### `list_handles`

Selection handles on screen: kind (corner, edge, top, lift, rotate), index (corner/edge number or rotation axis 0-2), x, y.

Python: `cad.handles()`

Python: `cad.handle(kind, index=0)`

#### `click_control`

Click a control by id (see list_controls).

Python: `cad.click_control(id)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `id` | string | required |  |

### Input simulation (window coordinates in logical px)

#### `mouse_move`

Move the mouse.

Python: `cad.mouse_move(x, y, mods=[])` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `x` | float | required |  |
| `y` | float | required |  |
| `mods` | string[] | optional | shift, ctrl, alt |

#### `mouse_down`

Press a button.

Python: `cad.mouse_down(x, y, button="left", mods=[])` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `x` | float | required |  |
| `y` | float | required |  |
| `button` | string | optional | left, right, middle |
| `mods` | string[] | optional |  |

#### `mouse_up`

Release a button.

Python: `cad.mouse_up(x, y, button="left", mods=[])` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `x` | float | required |  |
| `y` | float | required |  |
| `button` | string | optional |  |
| `mods` | string[] | optional |  |

#### `click`

Move, press and release.

Python: `cad.click(x, y, button="left", mods=[])` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `x` | float | required |  |
| `y` | float | required |  |
| `button` | string | optional |  |
| `mods` | string[] | optional |  |

#### `drag`

Press at (x0,y0), move in steps to (x1,y1), release.

Python: `cad.drag(x0, y0, x1, y1, steps=10, button="left", mods=[])` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `x0` | float | required |  |
| `y0` | float | required |  |
| `x1` | float | required |  |
| `y1` | float | required |  |
| `steps` | int | optional | default 10 |
| `button` | string | optional |  |
| `mods` | string[] | optional |  |

#### `wheel`

Mouse wheel (positive = zoom in).

Python: `cad.wheel(x, y, delta)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `x` | float | required |  |
| `y` | float | required |  |
| `delta` | float | required | notches |

#### `key`

Press a key chord, e.g. "ctrl+g", "shift+ctrl+z", "delete", "esc", "left", "h".

Python: `cad.key(keys)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `keys` | string | required |  |

#### `type_text`

Type characters (into the active field). A newline starts a new line in a field that takes several (a note); keys such as Enter and Tab are sent with key.

Python: `cad.type_text(text)` returns nothing

| Parameter | Type | | Meaning |
|---|---|---|---|
| `text` | string | required |  |

### Scripting

#### `run_script`

Run a Starlark script (as the console's Editor tab does) and return its printed output and any error; one undo step.

Python: `cad.run_script(...)` (not in scripts: a script can't start another script)

| Parameter | Type | | Meaning |
|---|---|---|---|
| `code` | string | required | the script |
| `name` | string | optional | file name for errors |
| `cwd` | string | optional | folder relative paths resolve against |

#### `get_console`

The Scripting console: open, tab, running, transcript lines, prompt and script text.

Python: `cad.console()`

#### `script_api`

The cad methods scripts get (mirroring the Python client).

Python: `cad.call("script_api", ...)`

### Automated runs

#### `answer_dialogs`

Queue answers to the prompts and file dialogs that come next (for automated runs): each {kind, value} with kind message (value yes, no or cancel), open or save (value: a path, or empty to cancel). Returns how many are queued.

Python: `cad.answer_dialogs(...)` (not in scripts: for automated runs: answers the prompts and file dialogs)

| Parameter | Type | | Meaning |
|---|---|---|---|
| `answers` | any[] | required |  |

#### `close_window`

What closing the window would do (the Save changes? prompt included): closes true if it would close. The window stays open.

Python: `cad.close_window(...)` (not in scripts: for automated runs: what closing the window would do)

#### `lose_capture`

The mouse is taken away mid-drag (another window, Alt+Tab): what the window does then.

Python: `cad.lose_capture(...)` (not in scripts: for automated runs: the mouse taken away mid-drag)

#### `drop_files`

Drop files on the window (imports them).

Python: `cad.drop_files(...)` (not in scripts: for automated runs: files dropped on the window)

| Parameter | Type | | Meaning |
|---|---|---|---|
| `paths` | string[] | required |  |

#### `stats`

Memory and cache figures (for soak tests): heap bytes in use and held, goroutines, cached meshes and their triangles, bounds entries, background jobs, undo depth.

Python: `cad.stats(...)` (not in scripts: for soak tests: the program's memory and cache figures)

#### `quit`

Close ScottiCAD without saving.

Python: `cad.quit(...)` (not in scripts: a script can't close the program it runs in)
