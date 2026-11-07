# ScottiCAD release notes

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
  snapping, alignment, a ruler, work planes and lay-flat.
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
