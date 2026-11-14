# ScottiCAD release notes

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
