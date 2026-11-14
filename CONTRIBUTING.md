# Contributing to ScottiCAD

Bug reports and ideas are welcome as
[issues](https://github.com/scotticad/scotticad/issues), and so are pull
requests. This page is how we write ScottiCAD. Most of these rules exist
because breaking them once caused a bug, so each comes with its reason.
When a rule and good sense disagree, use good sense and say why in the
pull request.

## Language

Everything a user reads uses American English: color, center,
millimeters, modeling, catalog, counter-clockwise. That covers labels,
tooltips, messages and errors, the start and About screens, `--help`,
the API's descriptions, the scripting help, the Python client's
docstrings, names written into exported files, and the Linux menu entry.

Identifiers use American English too: `color`, `center`, `orbit_center`.
Scripts depend on these names, so they can't change later. A few older
identifiers are British (`catalogue`, `PlanetaryCentres`); don't add
more.

Comments may use either spelling, as the author prefers.

Two exceptions keep both spellings on purpose. Search keywords list both
("aluminium aluminum"), so either finds the shape. And where the API
takes a word as input, it accepts both (`"center"` and `"centre"`).

The README and the manual still have British prose in places. They quote
the program's labels exactly as the program shows them.

## Building and testing

ScottiCAD is pure Go: no C compiler, no cgo. It must stay that way,
because every release is cross-compiled for Windows and Linux (x86-64
and ARM) from one machine.

```
.\build.ps1 -Test     Windows: build, then the Go and Python tests
make test             Linux: the same
```

Run the full suite before you send a pull request. Before a release, run
it on both Windows and Linux.

The Python tests drive the real program through its automation API, in a
hidden window. Set `SCOTTICAD_EXE` to test a program other than the one
at the top of the repository. On Linux they need an X display;
`xvfb-run` provides one.

Code is formatted with `gofmt` and passes `go vet`.

Add a dependency only when there is no reasonable way to do without it.
Each one is a license to track and a thing that can break the
cross-compile.

## Comments

A comment says what the code can't: why it is this way, what the units
are, where a formula comes from, which mistake it prevents.

```go
// a fixed date: the same design exports the same bytes
stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
```

Don't write the history of the code in comments ("now uses", "was
changed to", "fixed the bug where"). The commit message is the place for
that. Don't restate the line below in words.

Give a constant that came from measurement its measurement. "Cracks up
to 0.15 mm sliced clean in Bambu Studio" tells the next person whether
they can change it.

## Text the user reads

Say what happened and what to do about it, in plain words.

- A message must be true. The print check's rule is that a warning on a
  design that prints fine teaches people to ignore the panel.
- Give lengths in the design's units. A design set to inches gets
  inches.
- Don't stop the user's work to tell them something. An export with
  warnings still exports.

## Shapes

Shapes live in `pkg/shapes`. A shape is a function from its settings to
a mesh, registered with its sliders.

- **Register new shapes in `zz_added.go`.** A property test seeds each
  shape's random cases by its place in the catalog. New shapes at the
  end leave the older shapes' cases alone.
- **A mesh is closed, with its faces outward.** Check it as an export
  writes it, not only in memory: coordinates are rounded to float32 in
  the file, and two corners a rounding error apart stay apart near zero.
  The slider-limit test does this for every shape.
- **Compute a shared point once.** A full-circle shape whose last ring
  is computed at 360° doesn't land on its first ring; reuse the first.
- **A setting missing from a file builds as the slider's default.** The
  fallback in the code (`v.Get("height", 5)`) must match the slider.
- **Keep every setting in range when building.** A file can hold any
  number; the sliders only limit what the panel sends.
- **Avoid shapes that pinch to a point or fold through themselves** at
  the ends of a slider's range. Clamp the setting a little short.
- **New shapes come as solids.** Any shape becomes a hole with the Hole
  button; only the box, cylinder and sphere have hole tiles of their
  own.
- **Give the shape search keywords**, including the names people in
  other trades call it.

## The editor and the design

- **One action is one undo step.** Make changes through the editor's
  `edit` function, which records the step. Aids to editing (the camera,
  the ruler, the construction plane) are saved with the file but are not
  undo steps and don't mark the design as changed.
- **A locked shape stays where it is.** Check before moving or sizing.
- **Validate what a file gives you.** A hand-edited file can hold
  anything, and JSON can't hold NaN, so a "finite" check alone rejects
  nothing. Check that a saved matrix is really the kind of matrix you
  expect.
- **Raise the file format version only when an older ScottiCAD would
  misread the file.** Optional fields that older versions can ignore
  don't need a new version. Say in the release notes what older versions
  do with a new file.

## The window

- **Every control has an id,** and a test that clicks it by that id. A
  feature tested only through the API has untested buttons.
- **A hidden (headless) run never reads or writes the user's
  preferences.** Tests must not change someone's theme or recent files.
  A new preference needs that check on both its load and its save.
- **Slow work happens off the UI thread,** and its result comes back
  through `post`. The window says "Checking…" meanwhile.
- **A cache must be able to hold one whole job.** A cache that empties
  halfway through a design makes the job start over forever.

## The automation API

The API is how the Python client, scripts and AI assistants use
ScottiCAD. People build on it, so it changes carefully.

- Each method is one entry in the table in `pkg/app/api.go`, with its
  description and parameters. `docs/api.md` is generated from the table:
  after a change, run `go test ./pkg/app -run TestAPIDoc -update`.
- The Python client and the scripting language offer the same methods
  with the same parameters. A test compares them.
- A list with nothing in it is `[]`, never `null`.
- Refuse NaN and infinite numbers.
- Report an error for a request that can't be done. Don't quietly do
  something else.
- Don't rename or remove a method or a parameter. Add a new one.

## Tests

- **A bug fix comes with a test that fails without the fix.** Run it
  both ways. A test written after the fix, and never seen to fail, may
  not test the bug.
- **Don't sleep in a test.** Wait for the condition, with a deadline.
- **A test must be able to fail.** Check that an assertion isn't true
  for some unrelated reason.
- **Don't check floating-point results for exact equality** unless
  exactness is the point, as it is for the example designs.
- **Fuzz and sweep what you add.** A new kind of setting needs the
  catalog-wide tests to vary it.

## Examples

Each example script in `examples/star` makes the design saved beside it
in `examples/scotticad`, and a test checks that it still does, byte for
byte. After a deliberate change to a script or to a shape it uses,
remake that one design:

```
python examples/build.py --only NAME
```

`examples/published.txt` lists the examples, and `examples/README.md`
describes exactly those.

An example script is something to learn from. Its comments explain the
design: how it prints, how it goes together, why a clearance is what it
is. Keep the sizes as named numbers at the top.

## Exports

The same design exports the same bytes every time, on every platform.
That means fixed dates in archives, a sorted order wherever a map is
walked, and no dependence on which goroutine finished first.

## Documentation and releases

- When a label or message changes, the manual's pictures of it are out
  of date. `docs/manual/make_images.py` makes them again.
- The manual describes what the program does. If the two disagree and
  the program is wrong, fix the program.
- Release notes are for users: what changed and what it means for them,
  not how it was done.
- The version is the one line in the `VERSION` file.
