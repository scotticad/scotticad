# ScottiCAD release notes

## 0.4.0 (November 2026)

### New

- **Sketches.** Three new shapes are drawn rather than set with
  sliders: **Sketch** stands a drawing up, like a biscuit cutter;
  **Revolve Sketch** spins half a profile round a centre line, for
  bowls, knobs and wheels; **Coil Sketch** spins it while rising, for
  springs, threads and spiral ramps. Their tools have their own row at
  the top of the shape panel, under the Construction plane, Ruler and
  Notes tools, so they are there whichever library is showing. Dropping
  one onto the plate opens the sketch workspace, a flat drawing board:
  - **Pen** draws straight lines, Bézier curves and smooth curves
    through the points. **Edit** moves points, lines, handles and whole
    shapes, and adds or removes points. Points snap to the grid, and
    lengths, sizes and positions can be typed (**Tab** types the next
    line's length while drawing).
  - A closed outline is a filled area. Outlines that overlap, or sit one
    inside another, join; an outline that crosses itself is filled
    everywhere it goes round. A line whose end is dragged onto its start
    becomes a closed outline. **Make hole** (**H**) turns an outline or
    line into a cut-out: a ring, a notch, a slot, or a hollow revolve.
  - **Mirror and turn** (**M**) flips the selection, turns it by 90° or
    a typed angle, or adds a mirror copy on any side, so half of a
    symmetric part drawn becomes the whole.
  - Any path can instead be a stroke of a set width: centred on the
    path, inside it or outside it, with sharp, round or bevelled corners
    and flat, round or square ends.
  - A sketch can taper (**Top** and **Base Scale**) and twist. A revolve
    can turn through part of a circle. A coil's **Turns** and **Pitch**
    are set in Properties, and its turns never run into each other.
  - While you draw a revolve or coil, a live 3D preview of the result
    shows beside the drawing. A sketch's board shows the design's other
    shapes from above, to draw against. Rulers run along the board's
    edges, the top bar shows which kind of sketch it is, in its color,
    and the tools down the left are named, the one in use in that color.
  - **Picture to trace**: a photo or scan can lie under the drawing.
    Move it, level it, and set it to its true size from two points a
    known distance apart. It is saved with the sketch and is never part
    of the shape.
  - Double-click a sketch, or use **Edit sketch** in Properties, to
    change its drawing later; a move, turn or resize it has had is kept.

  Scripts and the API have `add_sketch`, `set_sketch` and the sketch
  workspace's commands. A design with a sketch needs this ScottiCAD or
  later to open.
- **Stretch.** A new **Stretch** tool (**T**) makes a part longer or
  shorter at a plane through it: type **+30** to add 30 mm there, **-30**
  to take out a 30 mm slice, or **90** to make it 90 mm long. Walls and
  grooves running through the plane just get longer, and the ends keep
  their shape, so a case or cartridge shell can be lengthened in one
  step. The stretch is a group that keeps the shapes as they were: its
  amount, plane and direction can be changed later in Properties, the
  shapes inside can still be edited, and an amount of 0 and Ungroup
  gives them back. Scripts and the API have `stretch`. An earlier
  ScottiCAD shows the shapes unstretched.
- **Gridfinity bins and baseplates.** Two new shapes in Hardware &
  Enclosures, made to the open Gridfinity storage standard, so they fit
  bins and baseplates from other designs. A bin is sized in grid units
  (42 mm across, 7 mm high), with or without the stacking lip, magnet or
  screw holes under its feet, compartments, a scoop along the front of
  each and a label shelf along the back. A baseplate is open
  underneath, or on a floor with or without magnet holes. An earlier
  ScottiCAD shows a box in their place.
- **Patterns.** A new **Pattern** tool (**P**) repeats what is selected
  in a grid, or round a circle: a grid of vent holes, a ring of bolt
  holes, a row of pegs. The pattern keeps one shape and the numbers, so
  the count, spacing, radius and angle can be changed at any time in
  Properties, and a change to the shape changes every copy. A pattern of
  a hole is a set of holes: group it with a part to cut them all.
  Ungroup turns the copies into separate shapes. A pattern shows at most
  20,000 shapes, counting the patterns inside it. Scripts and the API
  have `pattern`. An earlier ScottiCAD shows only the first copy.
- **Hull groups.** A fourth group type beside Bundle, Union and
  Intersect: **Hull** wraps the group's solids in one skin with no
  dents and fills the space between them. Two cylinders apart become a
  lever arm with round ends; four at the corners become a plate with
  round corners. Group the shapes, then click **Hull** in Properties;
  scripts and the API take `"hull"` as a group mode. Holes are cut from
  the hull, and Ungroup gives the shapes back. An earlier ScottiCAD
  shows a hull group as a union of its shapes.
- **New shapes.**
  - Basic Shapes: the **Rounded Box**, with its corners and its top and
    bottom edges rounded separately, in millimetres that stay put when
    you resize it, and the **Crescent**.
  - Hardware: the **Port Opening**, the cut-out for a connector in a
    case's wall, from a list of USB, HDMI, DisplayPort, RJ45, power and
    D-Sub connectors, or a size of your own; and the **Finishing
    Washer**, the domed cup a countersunk screw's head sits in.

  A design that uses them shows a box in their place in an earlier
  ScottiCAD.
- **New settings** on existing shapes:
  - **Countersunk Hole** and **Counterbored Hole**: **Screw** set to
    **Custom** takes your own shaft and head diameters (and the head's
    angle or pocket depth), for wood screws and inch screws. A
    countersunk hole's **Recess** sinks the head below flush.
  - **PCB Standoff**: **Gussets**, up to six ribs that brace a tall post.
  - **Spring**: **Hand**, right or left.
  - **Teardrop**: **Tip Angle**, from 40° to 140° (90° as before).

  Designs made before keep exactly their shapes: each new setting starts
  where the shape already was. An earlier ScottiCAD ignores the new
  settings, so it shows these shapes as they would be without them.
- **Import 3MF files**, including slicer projects from Bambu Studio and
  PrusaSlicer. Every object in the file comes in together as one shape,
  where the file puts it.

### Changed

- Round shapes can have up to 256 sides (or steps), up from 64: the
  cylinder, cone, sphere, half sphere, round roof, tube, ring, torus and
  paraboloid. Their defaults are unchanged.
- In the Python client, `screenshot()` without a path returns the
  picture instead of writing a file.

### Fixed

- Dragging shapes you had just duplicated moved only one of the
  originals underneath them. A duplicate sits exactly on its original,
  and the click went to the original. Where two shapes are under the
  pointer at the same place, the click now goes to the selected one.
- **Diamond Grid** lost some of its diamonds when **Gap** was 0.
- With **Keyboard shortcuts** or **About** open, keys still acted on the
  design behind: Delete removed the selection unseen. They do nothing
  now until the dialog is closed.
- The font list of a text shape opens on the current font each time, and
  no longer needs scrolling back after reaching either end.

## 0.3.0 (November 2026)

### New

- **Print check.** The filament panel (the spool button) now says whether
  each part is ready to print: holes or cracks in its surface, faces
  inside out, a part floating above the plate, sunk into it or off its
  edge, and pieces in mid-air. Each line says what to do about it, and
  clicking it selects the part; for a hole in the surface, its edges are
  marked in red in the view. The panel also gives the area of overhangs
  that may need support, and the Export dialog says when what you're
  exporting has warnings (it never stops an export). Assistants and
  scripts get the same check with `print_check`.
- **Bent tubes.** A new Bent Tube shape (in the Mechanical library) is
  made the way a tube bender makes one, from a table of straight lengths
  and bends, so it can turn in any direction. Scripts set the table with
  `cad.set_bends`. Designs with bent tubes are saved in file format 3,
  which earlier ScottiCAD versions won't open.
- **Rename shapes.** Click a shape's name at the top of the Properties
  panel to type a new one; clear it to go back to the shape's own name.
- **The ruler and construction plane are saved** with the design, so a
  design reopens with them where you left them. Placing or moving them
  still isn't an unsaved change or an undo step. Earlier versions open
  these files and ignore both.
- **New shapes.**
  - Basic Shapes: the **Slot** (round-ended), **Arc** (part of a ring,
    or a pie slice), **Fillet** (rounds an inside corner; as a hole, it
    rounds one outside edge), **Teardrop** (as a hole, a sideways hole
    that prints without support) and **Crystal** (quartz-like, with
    irregular faces from a seed).
  - Hardware: the **Keyhole Slot**, for hanging a part on a screw, and
    the **Bearing Seat**, a press-fit pocket for a bearing chosen from a
    list of common sizes.
  - Mechanical: the **Bevel Gear** and a meshed **Bevel Gear Pair**, the
    **GT2 Pulley**, and two joints that come as a key and its socket,
    the **Dovetail Joint** and the **Alignment Pin**.
  - Parametric: the **QR Code**, a scannable code for any text up to 271
    characters, in two parts for two-colour printing.

  Designs using these shapes don't show them correctly in earlier
  versions of ScottiCAD.
- **User manual.** Fourteen chapters with pictures, starting with a
  ten-minute tutorial, in the `docs/manual` folder on the ScottiCAD
  GitHub page.
- **New example designs:** a motorized Wankel rotary engine, and a
  two-colour Vault-Tec-themed Raspberry Pi case for the Pi 5, Pi 4 and
  Zero 2 W, each with the script that makes it.

### Fixed

- Esc now cancels a shape being dragged in from the shape panel, and
  switching away from ScottiCAD mid-drag no longer drops it.
- Print check messages give lengths in inches in a design set to inches.
- Opening a file that isn't a ScottiCAD design says so plainly.
- An SVG with a malformed curve could add a broken point to the outline.
- The shortcut list (**?**) now includes N, Ctrl+I, Ctrl+E and + / -.
- In the automation API, an empty selection is an empty list, not null,
  and the Python client refuses a NaN or infinite number before sending it.
- 3MF export: a piece of one colour set flush into another (an inlay
  filling its pocket) is now a part of its own colour. Before, it was
  joined to the piece around it with only its triangles coloured, which
  slicers drop, so Bambu Studio showed it in the other colour.
- 3MF export: pieces laid out apart are now separate objects, each with
  its colours as its parts. A slicer gets, say, a case's base and its
  two-colour lid as two objects, without "split to objects" pulling the
  lid's colours apart. Pieces that touch or overlap stay one object.
- 3MF export: corners that differ by less than the file's precision are
  now one vertex. Before, a slicer could see a crack between them.
- Helical gears, ring gears and the planetary set could come out with
  teeth that cross themselves at some settings (a deep Tip Gap on a
  small gear or one with many teeth), and a bore could cut into the
  tooth roots.
- A Pipe Elbow turned through a full 360° exported with a hairline crack
  where its ends meet.
- The program's wording now uses one spelling throughout (color,
  center, millimeters, modeling); tooltips, messages and the API's
  descriptions had mixed it with British forms.
- The Python client's `launch()` no longer leaves ScottiCAD running when
  it fails to start, and says what ScottiCAD printed.

## 0.2.0 (November 2026)

### New

- **Work with an AI assistant.** ScottiCAD can now act as an MCP server,
  so an assistant such as Claude can build and change designs in a
  ScottiCAD window while you watch. You can take over with the mouse at
  any time, and you can keep the files it reads and writes in one folder.
- **Set the center of rotation.** A new button beside the zoom buttons
  sets the point the view turns around: click a shape, the build plate or
  empty space. While you orbit, a marker shows the center; you can turn
  it off in Settings.
- **More gears:** helical and herringbone gears, a ring gear, and a
  planetary gear set that prints in place.
- **New example designs:** a folding stand, a planetary gear spinner and
  a crank reducer, each with the script that makes it. The stand and the
  spinner have been test-printed.
- On Windows, `scotticad --help` and `scotticad --version` now print to
  the command prompt.

### Fixed

- **The center of rotation could drift far from the design.** Zooming
  near the horizon, in the orthographic view, or back out after zooming
  inside a part could move the center a long way off, so the next orbit
  swung the build plate out of view. Zooming in still goes toward the
  point under the pointer; zooming out no longer moves the center nearer
  or farther away.
- **Section view:** looking down into a section, the build plate's grid
  no longer shows through the cut face. A section aimed at particular
  shapes keeps cutting them after they are grouped.

## 0.1.1 (October 2026)

The first public release, for Windows and Linux (x86-64 and ARM).

- Build by dragging shapes together, cutting holes and grouping, with
  snapping, alignment, a ruler, a construction plane and lay flat.
- A library of ready-made and adjustable shapes: basic solids, text,
  boxes and enclosures, fasteners and threads, gears, springs, hinges and
  more.
- Scripting in a Python-style language, from a built-in console or from
  Python programs.
- Section view, a filament estimate (weight, length and cost), and repair
  of broken imported meshes.
- Import STL, OBJ and SVG; export STL, OBJ, 3MF (with colours), GLB and
  SVG.
- A dozen example designs, with their scripts, on the ScottiCAD GitHub
  page: cable clips, coasters, knobs, a plant pot, a spool holder, board
  mounts, a bit holder, three versions of a small boat and more.
- Runs entirely on your own computer: no account and no internet
  connection.
