# ScottiCAD examples

Ready-made designs to open, print or take apart: some made by scripts,
which are here too, and some made by hand in ScottiCAD's editor.

> **Not every design here has been printed and tested.** They are examples
> of what ScottiCAD can do, not proven parts: check the sizes and fits
> against your own needs, and print a test piece before relying on one
> (a clip, a knob or an axle, say).

## Two folders: scripts and designs

- **[`star/`](star)** holds the examples as **scripts** (`.star` files),
  written in ScottiCAD's Python-style scripting language. They are where
  to look to learn how to build things with code: each one is commented,
  and each keeps its sizes as named numbers at the top, so you can change
  them (a cable's diameter, the number of rows, the size of the bit) and
  run it again to get a design made to your measurements.
- **[`scotticad/`](scotticad)** holds **designs** (`.scotticad` files),
  ready to open with *File → Open* without running anything: to look at,
  edit by hand, or export for printing. There are two kinds:
  - designs **made by the scripts** in `star/`, each named after its
    script (`coasters.star` makes `coasters.scotticad`);
  - **original designs** made by hand in the editor, with no script
    (`washer.scotticad`, for one). These are examples of building with
    the mouse rather than with code.

Every script has its design in `scotticad/`, but not every design there
has a script.

To run a script, open ScottiCAD's **Scripting** console, paste the script
into its **Editor** tab and press **F5**, or use *File → Run script…*.
Everything a script makes is one undo step, so **Ctrl+Z** takes it away
again.

To remake the script-made designs (after changing ScottiCAD, say), run
`make examples` on Linux, or `python examples/build.py` on any platform
after building ScottiCAD. Each script runs in a hidden ScottiCAD and its
design is saved in `scotticad/`, replacing the old one. Original designs
have no script, so this leaves them as they are; just don't give one the
same name as a script, or the script's design will replace it.

## The examples

Each entry names its script, or, for an original design, the design
itself.

### Around the house and workshop

- **[`cable-clips.star`](star/cable-clips.star)**: snap-in cable clips,
  one for each cable diameter in a list, each on a tab with a countersunk
  screw hole and its size engraved. Basic shapes only; shows text, holes
  and a loop that lays the clips out.
- **[`coasters.star`](star/coasters.star)**: a set of coasters with a word
  standing in a recess, and a holder the stack drops into. The script
  prints the height at which to swap filament, so the lettering comes out
  in a second colour.
- **[`hook-and-bracket.star`](star/hook-and-bracket.star)**: a wall hook
  and a shelf bracket, each drawn as a side outline (`cad.add_outline`)
  and extruded. They lie on their sides to print, so the load runs along
  the layers, not across them.
- **[`knobs.star`](star/knobs.star)**: three styles of replacement knob
  (fluted, skirted with tick marks, and octagonal), each with a different
  bore: a D-shaft, a slotted knurled shaft, or a round shaft with a set
  screw. Grooves and ticks are placed by loops over angles.
- **[`organizer.star`](star/organizer.star)**: a desk organizer with rows
  of holes stepping up toward the back, a different size per row, and a
  tray at the front. A loop places the dozens of holes that would take
  dozens of clicks by hand.
- **[`plant-pot.star`](star/plant-pot.star)**: a plant pot that turns from
  a circle at its base into a twisted polygon at its rim, with drainage
  holes and a matching saucer. Made with lofts (`cad.add_loft`); the
  inside is a second loft, offset by the wall's thickness.
- **[`washer.scotticad`](scotticad/washer.scotticad)** (original
  design, no script): a small washer, 5 mm across with a 2.1 mm hole and
  1 mm thick, made by hand in the editor. It's a cylinder with a hole
  cylinder grouped into it, the simplest use of a hole.

### For makers and 3D printing

- **[`bit-holder.star`](star/bit-holder.star)**: a block holding 1/4" hex
  screwdriver bits upright, each pocket labelled. The layout comes from a
  list of labels; the comments explain a hexagon's two widths (across
  flats and across corners) and why the pocket is a little bigger.
- **[`board-mount.star`](star/board-mount.star)**: mounting plates with
  standoffs for a Raspberry Pi, Pico, Arduino or your own board, with a
  see-through outline of the board on each. Grouped with a case, the
  outline cuts room for the board and openings for its ports.
- **[`spool-holder.star`](star/spool-holder.star)**: a freestanding
  filament spool holder: two A-frame uprights, braces, cone hubs on 608
  bearings, and a printed M8 axle and nut made with the ISO Thread shape.
  Every part is laid out flat on the build plate.

### The calibration boat: right and wrong ways to build it

Three versions of the well-known 3DBenchy test boat, about 60 × 31 × 48 mm:

- **[`benchy-loft.star`](star/benchy-loft.star)**: the boat built as
  closely to the official model as ScottiCAD's shapes allow, its hull made
  as a few lofts through outlines measured from the official model. About
  130 shapes, smooth, and quick to build. **This is the way to do it.**
- **[`benchy-simplified.star`](star/benchy-simplified.star)**: a tugboat
  in the same spirit, built only from boxes, cylinders and tubes. A short,
  easy script to read first.
- **[`benchy-layered-slow.star`](star/benchy-layered-slow.star)**: **a
  counterexample, kept to show what not to do; it is not good practice.**
  It builds the same boat as `benchy-loft.star`, but stacks the hull from
  55 thin layers, each assembled from boxes, slabs and cylinders: about
  1,060 shapes in all. It takes about a minute to run, the layers leave
  visible stair-steps on the hull, and it is further from the real shape.
  Compare it with `benchy-loft.star` to see why a loft is the right tool
  for a curved, changing outline.
