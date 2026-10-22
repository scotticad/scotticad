// Package editor is ScottiCAD's command layer: every change the GUI or the
// automation API makes to a design goes through an Editor method, which
// handles selection, undo history and validation. It has no GL or Win32
// dependencies so it can be tested headless.
package editor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/shapes"
)

// Ruler is the L-shaped ruler widget placed on the workplane.
type Ruler struct {
	Origin geom.Vec3 `json:"origin"`
	Mid    bool      `json:"mid"`             // measure to bounding-box midpoints
	Turns  int       `json:"turns,omitempty"` // quarter turns counter-clockwise
}

// Arm returns the direction of the ruler's arm k (0: the one along +X when
// unturned, 1: the one along +Y) on the workplane.
func (r *Ruler) Arm(k int) geom.Vec3 {
	t := float64((r.Turns%4+4)%4+k) * math.Pi / 2
	return geom.V(math.Round(math.Cos(t)), math.Round(math.Sin(t)), 0)
}

// Measure returns where box lies along arm k, relative to the origin: the
// near edge (the one closest to the origin's side), or the middle when the
// ruler measures to midpoints.
func (r *Ruler) Measure(box geom.Box, k int) float64 {
	d := r.Arm(k)
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, x := range []float64{box.Min.X, box.Max.X} {
		for _, y := range []float64{box.Min.Y, box.Max.Y} {
			v := geom.V(x-r.Origin.X, y-r.Origin.Y, 0).Dot(d)
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	switch {
	case r.Mid:
		return (lo + hi) / 2
	case hi < 0:
		return hi
	}
	return lo
}

// MoveFor is how far along arm k box must move for the ruler to read v
// there. The ruler measures a box's edge nearer the origin, so the edge
// placed at v is the one that will be nearer once moved: the low edge for
// v ≥ 0, the high edge for v < 0 (or the middle with midpoints).
func (r *Ruler) MoveFor(box geom.Box, k int, v float64) float64 {
	d := r.Arm(k)
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, x := range []float64{box.Min.X, box.Max.X} {
		for _, y := range []float64{box.Min.Y, box.Max.Y} {
			p := geom.V(x-r.Origin.X, y-r.Origin.Y, 0).Dot(d)
			lo, hi = math.Min(lo, p), math.Max(hi, p)
		}
	}
	switch {
	case r.Mid:
		return v - (lo+hi)/2
	case v < 0:
		return v - hi // on the negative side: its edge nearer the origin is the high one
	}
	return v - lo
}

// Anchor is the side of box the ruler measures along a box axis (0: X,
// 1: Y): -1 for box.Min, +1 for box.Max, as Measure picks it (the edge
// nearer the origin). A size typed along that axis keeps this side where
// it is and grows from it. 0 when the ruler measures to midpoints (a size
// then grows about the middle) or no arm runs along the axis.
func (r *Ruler) Anchor(box geom.Box, axis int) int {
	if r.Mid || axis < 0 || axis > 1 {
		return 0
	}
	for k := 0; k < 2; k++ {
		s := r.Arm(k).Axis(axis)
		if s == 0 {
			continue
		}
		pMin := (box.Min.Axis(axis) - r.Origin.Axis(axis)) * s
		pMax := (box.Max.Axis(axis) - r.Origin.Axis(axis)) * s
		measuredHigh := math.Max(pMin, pMax) < 0 // as Measure: the high edge when all is behind the origin
		minIsLow := s > 0                        // along the arm, box.Min is the low edge
		if measuredHigh != minIsLow {
			return -1
		}
		return 1
	}
	return 0
}

// Editor holds a design plus its editing state.
type Editor struct {
	Doc  *doc.Document
	Sel  []int
	Hist doc.History
	Path string
	// saved is the document state last written to disk.
	saved []byte

	Ruler *Ruler
	// Workplane is the active workplane's frame (frame coordinates to
	// world: the plane is the frame's z = 0, its axes the frame's); nil
	// when the default workplane is active. The Workplane tool attaches it
	// to a shape, Shape Workplane lifts it to the selection's base.
	Workplane *geom.Mat4

	clip     []byte
	pasteN   int
	lastDup  geom.Mat4
	dupIDs   []int
	dupStart map[int]geom.Mat4

	// SetClipboard and GetClipboard, when set, mirror copy/paste to the OS
	// clipboard.
	SetClipboard func(string)
	GetClipboard func() string

	// Replaced is called when New or Open replaces the design (state held
	// for the old one must go).
	Replaced func()

	// Changed is called after every modification (for redraws).
	Changed func()
	// Version counts changes to the design (not to the selection, the
	// path or other editing state): what was worked out from the design at
	// one version is stale at another.
	Version int

	drag []byte // snapshot taken at BeginEdit

	// blobs holds imported mesh data once for all history snapshots
	blobs doc.BlobStore
}

const clipPrefix = "SCOTTICAD-CLIP:"

func New() *Editor {
	e := &Editor{Doc: doc.New("Untitled design")}
	e.saved = e.snapshot()
	return e
}

// snapshot is the document for history and change detection, with large
// imported meshes shared through e.blobs rather than copied.
func (e *Editor) snapshot() []byte {
	b, _ := e.Doc.MarshalShared(&e.blobs)
	return b
}

// state is a history entry: the document plus the selection, so undo and
// redo restore what was selected (the selection is not part of the file).
func (e *Editor) state() []byte {
	sel, _ := json.Marshal(e.Sel)
	return append(append(e.snapshot(), 0), sel...)
}

func splitState(b []byte) ([]byte, []int) {
	i := bytes.LastIndexByte(b, 0)
	if i < 0 {
		return b, nil
	}
	var sel []int
	json.Unmarshal(b[i+1:], &sel)
	return b[:i], sel
}

func (e *Editor) setState(b []byte) {
	d, sel := splitState(b)
	if nd, err := doc.UnmarshalShared(d, &e.blobs); err == nil {
		e.Doc = nd
	}
	e.Sel = sel
	e.pruneSelection()
}

func (e *Editor) changed() {
	if e.Changed != nil {
		e.Changed()
	}
}

// errInvalid is returned when an edit would leave a design that can't be
// saved (e.g. NaN from transforming a shape with no geometry); the edit is
// undone.
var errInvalid = errors.New("that would leave the design invalid, so it wasn't done")

// edit runs f as one undoable step. If f fails, or leaves a design that
// can't be saved, the design is restored.
func (e *Editor) edit(f func() error) error {
	before := e.state()
	if err := f(); err != nil {
		e.setState(before)
		return err
	}
	cur, err := e.Doc.MarshalShared(&e.blobs)
	if err != nil {
		e.setState(before)
		e.changed()
		return errInvalid
	}
	if bd, _ := splitState(before); !bytes.Equal(bd, cur) {
		e.Hist.Push(before)
		e.Version++
	}
	e.pruneSelection()
	e.changed()
	return nil
}

// BeginEdit / EndEdit bracket an interactive change (a drag) so it
// becomes a single undo step. A BeginEdit while one is open keeps the
// first snapshot.
func (e *Editor) BeginEdit() {
	if e.drag == nil {
		e.drag = e.state()
	}
}

func (e *Editor) EndEdit() {
	e.Version++
	if e.drag == nil {
		return
	}
	cur, err := e.Doc.MarshalShared(&e.blobs)
	if err != nil { // the drag made the design unsavable: drop it
		e.CancelEdit()
		return
	}
	if d, _ := splitState(e.drag); !bytes.Equal(d, cur) {
		e.Hist.Push(e.drag)
	}
	e.drag = nil
	e.changed()
}

// CancelEdit restores the state from BeginEdit.
func (e *Editor) CancelEdit() {
	e.Version++
	if e.drag == nil {
		return
	}
	e.setState(e.drag)
	e.drag = nil
	e.changed()
}

func (e *Editor) Dirty() bool { return !bytes.Equal(e.saved, e.snapshot()) }

func (e *Editor) pruneSelection() {
	var keep []int
	for _, id := range e.Sel {
		if n := e.Doc.Find(id); n != nil && !n.Hidden {
			keep = append(keep, id)
		}
	}
	e.Sel = keep
}

// --- selection -----------------------------------------------------------

func (e *Editor) Selected() []*doc.Node { return e.Doc.FindAll(e.Sel) }

// Editable returns the selected nodes that are not locked.
func (e *Editor) Editable() []*doc.Node {
	var out []*doc.Node
	for _, n := range e.Selected() {
		if !n.Locked {
			out = append(out, n)
		}
	}
	return out
}

func (e *Editor) IsSelected(id int) bool {
	for _, s := range e.Sel {
		if s == id {
			return true
		}
	}
	return false
}

func (e *Editor) Select(ids ...int) {
	e.Sel = nil
	for _, id := range ids {
		if n := e.Doc.Find(id); n != nil && !n.Hidden && !e.IsSelected(id) {
			e.Sel = append(e.Sel, id)
		}
	}
	e.dupIDs = nil
	e.changed()
}

// ToggleSelect adds or removes id (Shift+click).
func (e *Editor) ToggleSelect(id int) {
	if e.IsSelected(id) {
		var keep []int
		for _, s := range e.Sel {
			if s != id {
				keep = append(keep, s)
			}
		}
		e.Sel = keep
	} else if n := e.Doc.Find(id); n != nil && !n.Hidden {
		e.Sel = append(e.Sel, id)
	}
	e.changed()
}

func (e *Editor) SelectAll() {
	var ids []int
	for _, n := range e.Doc.Nodes {
		if !n.Hidden {
			ids = append(ids, n.ID)
		}
	}
	e.Select(ids...)
}

func (e *Editor) ClearSelection() { e.Select() }

func (e *Editor) target(ids []int) []*doc.Node {
	if ids == nil {
		return e.Editable()
	}
	var out []*doc.Node
	for _, n := range e.Doc.FindAll(ids) {
		if !n.Locked {
			out = append(out, n)
		}
	}
	return out
}

// SelectionBounds is the world bounding box of the selection.
func (e *Editor) SelectionBounds() geom.Box { return doc.Bounds(e.Selected()) }

// --- design files ---------------------------------------------------------

func (e *Editor) NewDesign() {
	e.Version++
	e.Doc = doc.New("Untitled design")
	e.Sel = nil
	e.Hist.Clear()
	e.drag = nil // a snapshot of the design being replaced
	e.blobs.Reset()
	if e.Replaced != nil {
		e.Replaced()
	}
	e.Path = ""
	e.Ruler = nil
	e.Workplane = nil
	e.saved = e.snapshot()
	e.changed()
}

func (e *Editor) Open(path string) error {
	e.Version++
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	d, err := doc.Unmarshal(b)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	d.Prewarm()
	e.Doc = d
	e.Sel = nil
	e.Hist.Clear()
	e.drag = nil // a snapshot of the design being replaced
	e.blobs.Reset()
	e.blobs.Intern(e.Doc.Nodes)
	if e.Replaced != nil {
		e.Replaced()
	}
	e.Path = path
	e.Ruler = nil
	e.Workplane = nil
	e.saved = e.snapshot()
	e.changed()
	return nil
}

func (e *Editor) Save(path string) error {
	if path == "" {
		path = e.Path
	}
	if path == "" {
		return errors.New("no file name")
	}
	if filepath.Ext(path) == "" {
		path += ".scotticad"
	}
	b, err := e.Doc.Marshal()
	if err != nil {
		return err
	}
	// written beside the file and renamed over it, so a failure part way
	// (a full disk, a crash) never leaves the design half written
	if err := WriteFileAtomic(path, b); err != nil {
		return err
	}
	e.Path = path
	e.saved = e.snapshot()
	e.changed()
	return nil
}

// --- undo ------------------------------------------------------------------

func (e *Editor) restore(b []byte) {
	e.Version++
	e.setState(b)
	e.dupIDs = nil
	e.changed()
}

// Undo steps back. An interactive change still open (a drag) is finished
// first, so it becomes the step undone and the history stays in order.
func (e *Editor) Undo() bool {
	e.EndEdit()
	b, ok := e.Hist.Undo(e.state())
	if ok {
		e.restore(b)
	}
	return ok
}

func (e *Editor) Redo() bool {
	e.EndEdit()
	b, ok := e.Hist.Redo(e.state())
	if ok {
		e.restore(b)
	}
	return ok
}

// --- creation --------------------------------------------------------------

// PlaneZ is the world height of the workplane's origin.
func (e *Editor) PlaneZ() float64 { return e.Work().Translation().Z }

// Work returns the workplane frame (identity for the default workplane).
func (e *Editor) Work() geom.Mat4 {
	if e.Workplane != nil {
		return *e.Workplane
	}
	return geom.Identity()
}

// SelectionBoundsIn is the selection's bounding box measured in the
// workplane frame: what handles, sizes and the ruler work with.
func (e *Editor) SelectionBoundsIn() geom.Box { return doc.BoundsIn(e.Selected(), e.Work()) }

// MoveIn moves the selection by d given in workplane coordinates.
func (e *Editor) MoveIn(ids []int, d geom.Vec3) { e.Move(ids, e.Work().Dir(d)) }

// SetBoundsIn is SetBounds with b measured in the workplane frame, so a
// shape attached to the workplane scales along its own axes.
func (e *Editor) SetBoundsIn(ids []int, b geom.Box) error {
	ns := e.target(ids)
	if len(ns) == 0 {
		return errors.New("nothing selected")
	}
	return e.edit(func() error { setBoundsIn(ns, b, e.Work()); return nil })
}

// SetBoundsLiveIn is SetBoundsIn without an undo step.
func (e *Editor) SetBoundsLiveIn(ns []*doc.Node, b geom.Box) {
	e.Version++
	setBoundsIn(ns, b, e.Work())
	e.changed()
}

func setBoundsIn(ns []*doc.Node, b geom.Box, f geom.Mat4) {
	if f == geom.Identity() {
		setBounds(ns, b)
		return
	}
	inv, ok := f.Inverse()
	if !ok {
		return
	}
	cur := doc.BoundsIn(ns, f)
	cs, ns2 := cur.Size(), b.Size()
	k := geom.V(1, 1, 1)
	for i := 0; i < 3; i++ {
		if cs.Axis(i) > 1e-9 && ns2.Axis(i) > 1e-9 {
			k = k.WithAxis(i, ns2.Axis(i)/cs.Axis(i))
		}
	}
	t := geom.Translate(b.Min).Mul(geom.Scale(k)).Mul(geom.Translate(cur.Min.Neg()))
	doc.Apply(ns, f.Mul(t).Mul(inv))
}

// RotateIn turns nodes about the workplane's axis through center (both in
// workplane coordinates); deg is counter-clockwise looking down the axis.
func (e *Editor) RotateIn(ns []*doc.Node, axis int, deg float64, center geom.Vec3) {
	f := e.Work()
	inv, ok := f.Inverse()
	if !ok {
		return
	}
	r := geom.Translate(center).Mul(geom.RotateAxis(axis, deg*math.Pi/180)).Mul(geom.Translate(center.Neg()))
	doc.Apply(ns, f.Mul(r).Mul(inv))
}

// ShapeFrame is the frame of a workplane attached to a shape: the shape's
// own axes, on its own bottom (its lowest side along its z axis), centred.
func ShapeFrame(n *doc.Node) geom.Mat4 {
	r := n.M
	for i := 0; i < 3; i++ {
		if c := r.Col(i); c.Len() > 1e-12 {
			r.SetCol(i, c.Mul(1/c.Len()))
		}
	}
	r[12], r[13], r[14] = 0, 0, 0
	if r.Det3() < 0 {
		// a mirrored shape's axes are left-handed: shapes placed or
		// imported on that workplane would come out mirrored too
		r.SetCol(0, r.Col(0).Neg())
	}
	b := doc.BoundsIn([]*doc.Node{n}, r)
	c := b.Center()
	return r.Mul(geom.Translate(geom.V(c.X, c.Y, b.Min.Z)))
}

// AttachWorkplane puts the workplane on the top-level shape id (see
// ShapeFrame); id 0 restores the default workplane.
func (e *Editor) AttachWorkplane(id int) error {
	e.Version++ // what was measured in the old workplane's frame is stale
	if id == 0 {
		e.Workplane = nil
		e.changed()
		return nil
	}
	n := e.Doc.Find(id)
	if n == nil {
		return fmt.Errorf("no shape %d", id)
	}
	f := ShapeFrame(n)
	for _, v := range f {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			// its parts combine to nothing: no bottom to stand on
			return errNoSurface
		}
	}
	e.Workplane = &f
	e.changed()
	return nil
}

// errNoSurface refuses to put the workplane on a shape with no geometry
// (an intersect group of shapes that don't meet, a group whose holes cut
// away everything).
var errNoSurface = errors.New("that shape has no surface for a construction plane: its parts combine to nothing")

// AddShape places a catalogue shape with its bottom centre at pos and
// selects it.
func (e *Editor) AddShape(kind string, pos geom.Vec3, hole bool) (int, error) {
	var id int
	err := e.edit(func() error {
		n, err := e.Doc.AddShape(kind, pos, hole)
		if err != nil {
			return err
		}
		id = n.ID
		return nil
	})
	if err == nil {
		e.Select(id)
	}
	return id, err
}

// AddProfile adds an outline or loft from its sections (world millimetres,
// see doc.AddProfile) and selects it.
func (e *Editor) AddProfile(kind string, ss []shapes.Section, hole bool) (int, error) {
	var id int
	err := e.edit(func() error {
		n, err := e.Doc.AddProfile(kind, ss, hole)
		if err != nil {
			return err
		}
		id = n.ID
		return nil
	})
	if err == nil {
		e.Select(id)
	}
	return id, err
}

// SetSections replaces an outline's or loft's sections (see
// doc.SetSections).
func (e *Editor) SetSections(id int, ss []shapes.Section) error {
	n := e.Doc.FindAny(id)
	if n == nil {
		return fmt.Errorf("no node %d", id)
	}
	if !shapes.IsProfile(n.Kind) {
		return fmt.Errorf("node %d is not an outline or loft", id)
	}
	if e.locked(id) {
		return errLocked
	}
	if err := shapes.CheckSections(n.Kind, ss); err != nil {
		return err // before edit: a refused change leaves no undo step
	}
	return e.edit(func() error { return doc.SetSections(n, ss) })
}

// HoleColor is the grey of a hole being placed from the library. Holes keep
// their kind's colour, so a hole that still carries this grey (designs from
// early builds) gets its kind's default colour back when made solid.
const HoleColor = "#a0a0a0"

// AddShapeXf places a shape with its bottom centre at the origin, then
// applies xf (used to drop shapes onto tilted faces) — one undo step.
func (e *Editor) AddShapeXf(kind string, xf geom.Mat4, hole bool) (int, error) {
	var id int
	err := e.edit(func() error {
		n, err := e.Doc.AddShape(kind, geom.Vec3{}, hole)
		if err != nil {
			return err
		}
		doc.Apply([]*doc.Node{n}, xf)
		id = n.ID
		return nil
	})
	if err == nil {
		e.Select(id)
	}
	return id, err
}

// AddMesh adds a mesh (imports) with its bottom centre at pos and selects
// it.
func (e *Editor) AddMesh(name string, m *geom.Mesh, pos geom.Vec3) (int, error) {
	return e.AddMeshXf(name, m, geom.Translate(pos))
}

// AddMeshXf adds a mesh with its bottom centre at the origin, then applies
// xf, and selects it.
func (e *Editor) AddMeshXf(name string, m *geom.Mesh, xf geom.Mat4) (int, error) {
	if m == nil || len(m.Tris) == 0 {
		return 0, errors.New("no geometry")
	}
	var id int
	err := e.edit(func() error {
		n := e.Doc.AddMesh(name, m, geom.Vec3{})
		doc.Apply([]*doc.Node{n}, xf)
		id = n.ID
		return nil
	})
	if err == nil {
		e.Select(id)
	}
	return id, err
}

// --- clipboard ---------------------------------------------------------------

func (e *Editor) Copy() bool { return e.copyIDs(e.Sel) }

func (e *Editor) copyIDs(ids []int) bool {
	if len(ids) == 0 {
		return false
	}
	e.clip = e.Doc.CopyJSON(ids)
	e.pasteN = 0
	if e.SetClipboard != nil {
		e.SetClipboard(clipPrefix + string(e.clip))
	}
	return true
}

// Cut copies and deletes the unlocked shapes selected (locked ones stay,
// and so aren't copied either).
func (e *Editor) Cut() bool {
	ids := doc.IDs(e.Editable())
	if !e.copyIDs(ids) {
		return false
	}
	e.Delete(ids)
	return true
}

// Paste inserts the clipboard. Normal paste offsets each successive paste
// by half the pasted object's size along X and Y (of the workplane); paste
// in place keeps the original position.
func (e *Editor) Paste(inPlace bool) ([]int, error) {
	data := e.clip
	if e.GetClipboard != nil {
		if s := e.GetClipboard(); strings.HasPrefix(s, clipPrefix) {
			data = []byte(strings.TrimPrefix(s, clipPrefix))
		}
	}
	if len(data) == 0 {
		return nil, errors.New("clipboard is empty")
	}
	var ids []int
	err := e.edit(func() error {
		ns, err := e.Doc.PasteJSON(data, geom.Vec3{})
		if err != nil {
			return err
		}
		e.blobs.Intern(ns)
		if !inPlace {
			e.pasteN++
			f := e.Work()
			s := doc.BoundsIn(ns, f).Size().Mul(0.5 * float64(e.pasteN))
			doc.Move(ns, f.Dir(geom.V(s.X, s.Y, 0)))
		}
		ids = doc.IDs(ns)
		return nil
	})
	if err == nil {
		e.Select(ids...)
	}
	return ids, err
}

// Duplicate copies the selection in place (Ctrl+D). If the previous
// duplicate was moved, repeating Ctrl+D repeats that transform.
func (e *Editor) Duplicate() []int {
	// the unlocked shapes selected: a copy of a locked one would sit on it,
	// and couldn't be moved off
	src := doc.IDs(e.Editable())
	if len(src) == 0 {
		return nil
	}
	repeat := e.dupIDs != nil && sameIDs(e.dupIDs, e.Sel)
	var xf geom.Mat4
	if repeat {
		xf = e.lastDup
	}
	var ids []int
	srcM := map[int]geom.Mat4{}
	e.edit(func() error {
		ns := e.Doc.Duplicate(src, geom.Vec3{})
		for i, n := range ns {
			if repeat {
				n.M = xf.Mul(n.M)
				doc.Normalize(n)
			}
			ids = append(ids, n.ID)
			srcM[n.ID] = ns[i].M
		}
		return nil
	})
	e.Select(ids...)
	e.dupIDs = ids
	e.dupStart = srcM
	if !repeat {
		e.lastDup = geom.Identity()
	}
	return ids
}

// NoteMoved records the transform applied to a fresh duplicate so that
// the next Ctrl+D repeats it.
func (e *Editor) NoteMoved() {
	if e.dupIDs == nil || !sameIDs(e.dupIDs, e.Sel) || len(e.dupIDs) == 0 {
		return
	}
	n := e.Doc.Find(e.dupIDs[0])
	start, ok := e.dupStart[e.dupIDs[0]]
	if n == nil || !ok {
		return
	}
	inv, ok := start.Inverse()
	if !ok {
		return
	}
	e.lastDup = n.M.Mul(inv)
}

func sameIDs(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]int(nil), a...)
	y := append([]int(nil), b...)
	sort.Ints(x)
	sort.Ints(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// Delete removes nodes (the selection if ids is nil).
func (e *Editor) Delete(ids []int) {
	ns := e.target(ids)
	if len(ns) == 0 {
		return
	}
	e.edit(func() error { e.Doc.Remove(doc.IDs(ns)); return nil })
}

// --- grouping ----------------------------------------------------------------

func (e *Editor) Group(mode string) (int, error) {
	ns := e.Editable()
	if len(ns) < 2 {
		return 0, errors.New("select at least two shapes to group")
	}
	if mode == "" {
		mode = doc.ModeUnion
	}
	if mode != doc.ModeUnion && mode != doc.ModeBundle && mode != doc.ModeIntersect {
		return 0, fmt.Errorf("unknown group mode %q", mode)
	}
	var id int
	err := e.edit(func() error {
		g, err := e.Doc.Group(doc.IDs(ns), mode)
		if err != nil {
			return err
		}
		id = g.ID
		return nil
	})
	if err == nil {
		e.Select(id)
	}
	return id, err
}

func (e *Editor) Ungroup() []int {
	var groups []int
	for _, n := range e.Editable() {
		if n.IsGroup() {
			groups = append(groups, n.ID)
		}
	}
	if len(groups) == 0 {
		return nil
	}
	var ids []int
	e.edit(func() error {
		// a group's own notes go to its first child, staying put
		type moved struct {
			to    *doc.Node
			world geom.Vec3
			text  string
		}
		var notes []moved
		for _, id := range groups {
			if g := e.Doc.Find(id); g != nil && len(g.Children) > 0 {
				for _, nt := range g.Notes {
					notes = append(notes, moved{g.Children[0], g.M.Point(g.NotePoint(nt.P)), nt.Text})
				}
			}
		}
		for _, n := range e.Doc.Ungroup(groups) {
			doc.Normalize(n)
			ids = append(ids, n.ID)
		}
		for _, m := range notes {
			if inv, ok := m.to.M.Inverse(); ok {
				m.to.Notes = append(m.to.Notes, doc.Note{P: m.to.NoteParam(inv.Point(m.world)), Text: m.text})
			}
		}
		return nil
	})
	keep := ids
	for _, id := range e.Sel {
		if e.Doc.Find(id) != nil {
			keep = append(keep, id)
		}
	}
	e.Select(keep...)
	return ids
}

// SetGroupMode sets the mode (union, bundle or intersect) of the selected
// unlocked groups.
func (e *Editor) SetGroupMode(mode string) error {
	return e.edit(func() error {
		for _, n := range e.Editable() {
			if n.IsGroup() {
				n.GroupMode = mode
			}
		}
		return nil
	})
}

// --- appearance ----------------------------------------------------------------

func (e *Editor) SetHole(hole bool) {
	e.edit(func() error {
		for _, n := range e.Editable() {
			n.Hole = hole
		}
		return nil
	})
}

// SetColorLive is SetColor without an undo step (use inside
// BeginEdit/EndEdit, e.g. while dragging in the colour picker).
func (e *Editor) SetColorLive(hex string) {
	e.Version++
	if _, ok := geom.TryParseColor(hex); !ok {
		return
	}
	for _, n := range e.Editable() {
		n.Hole = false
		doc.SetColor(n, hex)
	}
	e.changed()
}

func (e *Editor) SetColor(hex string) error {
	if _, ok := geom.TryParseColor(hex); !ok {
		return fmt.Errorf("bad colour %q", hex)
	}
	return e.edit(func() error {
		for _, n := range e.Editable() {
			n.Hole = false
			doc.SetColor(n, hex)
		}
		return nil
	})
}

func (e *Editor) SetTransparent(on bool) {
	e.edit(func() error {
		for _, n := range e.Editable() {
			n.Transparent = on
		}
		return nil
	})
}

func (e *Editor) SetMulticolor(on bool) {
	e.edit(func() error {
		for _, n := range e.Editable() {
			if n.IsGroup() {
				n.Multicolor = on
			}
		}
		return nil
	})
}

func (e *Editor) SetLocked(on bool) {
	e.edit(func() error {
		for _, n := range e.Selected() {
			n.Locked = on
		}
		return nil
	})
}

func (e *Editor) SetName(name string) {
	e.edit(func() error {
		for _, n := range e.Selected() {
			n.Name = name
		}
		return nil
	})
}

// --- visibility --------------------------------------------------------------

func (e *Editor) HideSelected() {
	ns := e.Selected()
	if len(ns) == 0 {
		return
	}
	e.edit(func() error {
		for _, n := range ns {
			n.Hidden = true
		}
		return nil
	})
}

// ToggleHidden flips a node's visibility (visibility mode clicks).
func (e *Editor) ToggleHidden(id int) {
	n := e.Doc.Find(id)
	if n == nil {
		return
	}
	e.edit(func() error { n.Hidden = !n.Hidden; return nil })
}

func (e *Editor) ShowAll() {
	e.edit(func() error {
		for _, n := range e.Doc.Nodes {
			n.Hidden = false
		}
		return nil
	})
}

func (e *Editor) AnyHidden() bool {
	for _, n := range e.Doc.Nodes {
		if n.Hidden {
			return true
		}
	}
	return false
}

// --- transforms ----------------------------------------------------------------

// Move translates the selection (or ids).
func (e *Editor) Move(ids []int, d geom.Vec3) {
	ns := e.target(ids)
	if len(ns) == 0 {
		return
	}
	e.edit(func() error { doc.Move(ns, d); return nil })
	e.NoteMoved()
}

// MoveLive moves without creating an undo step (use inside Begin/EndEdit).
func (e *Editor) MoveLive(ns []*doc.Node, d geom.Vec3) { e.Version++; doc.Move(ns, d); e.changed() }

// SetBounds resizes and positions the selection so its bounding box
// becomes b. Zero-size axes keep their current size.
func (e *Editor) SetBounds(ids []int, b geom.Box) error {
	ns := e.target(ids)
	if len(ns) == 0 {
		return errors.New("nothing selected")
	}
	return e.edit(func() error { setBounds(ns, b); return nil })
}

func setBounds(ns []*doc.Node, b geom.Box) {
	cur := doc.Bounds(ns)
	cs := cur.Size()
	ns2 := b.Size()
	f := geom.V(1, 1, 1)
	for i := 0; i < 3; i++ {
		if cs.Axis(i) > 1e-9 && ns2.Axis(i) > 1e-9 {
			f = f.WithAxis(i, ns2.Axis(i)/cs.Axis(i))
		}
	}
	doc.ScaleAbout(ns, f, cur.Min)
	doc.Move(ns, b.Min.Sub(cur.Min))
}

// SetBoundsLive is SetBounds without an undo step.
func (e *Editor) SetBoundsLive(ns []*doc.Node, b geom.Box) {
	e.Version++
	setBounds(ns, b)
	e.changed()
}

// Resize sets the selection's bounding-box size, anchored at its minimum
// corner (typed sizes keep the min corner fixed).
func (e *Editor) Resize(ids []int, size geom.Vec3) error {
	ns := e.target(ids)
	if len(ns) == 0 {
		return errors.New("nothing selected")
	}
	b := doc.Bounds(ns)
	nb := geom.Box{Min: b.Min, Max: b.Min.Add(size)}
	for i := 0; i < 3; i++ {
		if size.Axis(i) <= 0 {
			nb.Max = nb.Max.WithAxis(i, b.Max.Axis(i))
		}
	}
	return e.SetBounds(ids, nb)
}

// Rotate turns the selection about a world axis through its centre.
func (e *Editor) Rotate(ids []int, axis int, deg float64) {
	ns := e.target(ids)
	if len(ns) == 0 {
		return
	}
	c := doc.Bounds(ns).Center()
	e.edit(func() error { doc.RotateAbout(ns, axis, deg, c); return nil })
}

func (e *Editor) Mirror(ids []int, axis int) {
	ns := e.target(ids)
	if len(ns) == 0 {
		return
	}
	e.edit(func() error { doc.Apply(ns, e.MirrorTransform(ns, axis)); return nil })
}

// MirrorTransform flips ns along the workplane's axis about their centre
// (world axes with the default workplane).
func (e *Editor) MirrorTransform(ns []*doc.Node, axis int) geom.Mat4 {
	f := e.Work()
	inv, ok := f.Inverse()
	if !ok {
		return geom.Identity()
	}
	c := doc.BoundsIn(ns, f).Center()
	flip := geom.V(1, 1, 1).WithAxis(axis, -1)
	return f.Mul(geom.Translate(c)).Mul(geom.Scale(flip)).Mul(geom.Translate(c.Neg())).Mul(inv)
}

// LayFlat turns the selection (or ids), as one piece, so that world
// direction n (the outward normal of a face) points straight down into
// the workplane, then drops it on: the face lies on the workplane. One
// undo step.
func (e *Editor) LayFlat(ids []int, n geom.Vec3) error {
	ns := e.target(ids)
	if len(ns) == 0 {
		return errors.New("nothing selected")
	}
	if n.Len() < 1e-9 {
		return errors.New("lay_flat needs a direction")
	}
	f := e.Work()
	r := geom.RotateFromTo(n, f.Dir(geom.V(0, 0, -1)))
	return e.edit(func() error {
		c := doc.Bounds(ns).Center()
		doc.Apply(ns, geom.Translate(c).Mul(r).Mul(geom.Translate(c.Neg())))
		b := doc.BoundsIn(ns, f)
		doc.Move(ns, f.Dir(geom.V(0, 0, -b.Min.Z)))
		return nil
	})
}

// Split cuts the selection (or ids) with a plane square to the
// workplane's axis, at position at along it (workplane coordinates); see
// doc.Document.Split. The halves are selected. One undo step.
func (e *Editor) Split(ids []int, axis int, at float64, keep string, gap float64) ([]int, error) {
	ns := e.target(ids)
	if len(ns) == 0 {
		return nil, errors.New("nothing selected")
	}
	var out []int
	err := e.edit(func() error {
		hs, err := e.Doc.Split(doc.IDs(ns), e.Work(), axis, at, keep, gap)
		out = doc.IDs(hs)
		return err
	})
	if err == nil {
		e.Select(out...)
	}
	return out, err
}

// Align aligns the selection on axis (where -1 min, 0 centre, 1 max). A
// single shape aligns against the workplane.
func (e *Editor) Align(ids []int, axis, where int) { e.AlignTo(ids, axis, where, 0) }

// AlignBox is the box alignment works against, measured in frame f: the
// key object's bounds when key is one of ns (click a selected shape while
// aligning), otherwise the bounds of all of ns.
func AlignBox(ns []*doc.Node, key int, f geom.Mat4) geom.Box {
	for _, n := range ns {
		if key != 0 && n.ID == key {
			return doc.BoundsIn([]*doc.Node{n}, f)
		}
	}
	return doc.BoundsIn(ns, f)
}

// AlignTo aligns the selection (or ids) to the key object, which stays
// put; key 0 aligns to the combined bounds. Axes are the workplane's.
func (e *Editor) AlignTo(ids []int, axis, where, key int) {
	ns := e.target(ids)
	if len(ns) == 0 {
		return
	}
	ref := e.Selected()
	if ids != nil {
		ref = e.Doc.FindAll(ids)
	}
	e.edit(func() error {
		for i, d := range e.alignDeltas(ns, ref, axis, where, key) {
			doc.Move([]*doc.Node{ns[i]}, d)
		}
		return nil
	})
}

// AlignPreview returns the (world) translation each node would get from
// aligning the selection, without modifying anything.
func (e *Editor) AlignPreview(ns []*doc.Node, axis, where, key int) []geom.Vec3 {
	return e.alignDeltas(ns, e.Selected(), axis, where, key)
}

// alignDeltas moves ns (the unlocked shapes) against ref (all of them,
// locked ones included: they stay put, and the others align to them). A
// single shape aligns against the workplane: its centre, its edges (± half
// its size), or its surface.
func (e *Editor) alignDeltas(ns, ref []*doc.Node, axis, where, key int) []geom.Vec3 {
	f := e.Work()
	pick := func(b geom.Box) float64 {
		return map[int]float64{-1: b.Min.Axis(axis), 0: b.Center().Axis(axis), 1: b.Max.Axis(axis)}[where]
	}
	if len(ref) == 0 {
		ref = ns
	}
	var target float64
	if len(ref) == 1 {
		b := doc.BoundsIn(ref, f)
		if axis == 2 {
			target = float64(where+1) / 2 * b.Size().Z
		} else {
			half := e.Doc.Settings.WorkplaneW / 2
			if axis == 1 {
				half = e.Doc.Settings.WorkplaneL / 2
			}
			target = float64(where) * half
		}
	} else {
		target = pick(AlignBox(ref, key, f))
	}
	out := make([]geom.Vec3, len(ns))
	for i, n := range ns {
		d := target - pick(doc.BoundsIn([]*doc.Node{n}, f))
		out[i] = f.Dir(geom.V(0, 0, 0).WithAxis(axis, d))
	}
	return out
}

// Drop moves the selection down onto the current plane.
func (e *Editor) Drop(ids []int) {
	ns := e.target(ids)
	if len(ns) == 0 {
		return
	}
	f := e.Work()
	e.edit(func() error {
		for _, n := range ns {
			b := doc.BoundsIn([]*doc.Node{n}, f)
			doc.Move([]*doc.Node{n}, f.Dir(geom.V(0, 0, -b.Min.Z)))
		}
		return nil
	})
}

// Nudge moves the selection by one snap step (or 1 mm when snap is off)
// times steps along axis.
func (e *Editor) Nudge(axis int, steps float64) {
	s := e.Doc.Settings.Snap
	if s <= 0 {
		s = 1
		if e.Doc.Settings.Units == "in" {
			s = 25.4 / 16
		}
	}
	e.MoveIn(nil, geom.V(0, 0, 0).WithAxis(axis, s*steps))
}

// --- parameters ----------------------------------------------------------------

// Node returns any node by id.
func (e *Editor) Node(id int) *doc.Node { return e.Doc.FindAny(id) }

// SetParam changes a shape parameter (one undo step).
func (e *Editor) SetParam(id int, key string, v float64) error {
	if _, _, err := e.param(id, key); err != nil {
		return err
	}
	return e.edit(func() error { return e.setParam(id, key, v) })
}

// SetParamLive changes a parameter without an undo step (slider drags,
// bracketed by BeginEdit/EndEdit).
func (e *Editor) SetParamLive(id int, key string, v float64) error {
	e.Version++
	err := e.setParam(id, key, v)
	e.changed()
	return err
}

// errLocked refuses a change to a locked shape (its parameters as much as
// its place).
var errLocked = errors.New("that shape is locked")

// locked reports whether node id (any depth) belongs to a locked
// top-level shape.
func (e *Editor) locked(id int) bool {
	for _, n := range e.Doc.Nodes {
		found := false
		n.Walk(func(x *doc.Node) { found = found || x.ID == id })
		if found {
			return n.Locked
		}
	}
	return false
}

func (e *Editor) param(id int, key string) (*doc.Node, *shapes.Param, error) {
	n := e.Doc.FindAny(id)
	if n == nil {
		return nil, nil, fmt.Errorf("no node %d", id)
	}
	if e.locked(id) {
		return nil, nil, errLocked
	}
	kind := n.Kind
	if n.IsGroup() && n.Assembly != "" {
		kind = n.Assembly
	}
	k, ok := shapes.Lookup(kind)
	if !ok {
		return nil, nil, fmt.Errorf("node %d has no parameters", id)
	}
	for i := range k.Params {
		if k.Params[i].Key == key {
			return n, &k.Params[i], nil
		}
	}
	return nil, nil, fmt.Errorf("%s has no parameter %q", k.ID, key)
}

func (e *Editor) setParam(id int, key string, v float64) error {
	n, p, err := e.param(id, key)
	if err != nil {
		return err
	}
	if p.Int {
		v = math.Round(v)
	}
	if p.Max > p.Min {
		v = math.Max(p.Min, math.Min(p.Max, v))
	}
	if n.IsGroup() && n.Assembly != "" {
		return e.Doc.SetAssemblyParam(n, key, v)
	}
	if p.SizeAxis >= 0 && p.Kind == shapes.Number {
		// L/W/H fields resize the shape along its own axis, about its
		// base centre: scaled in its own space, which is exact however
		// it is turned or sheared
		old := n.AxisLength(p.SizeAxis)
		if old > 0 && v > 0 {
			f := geom.V(1, 1, 1).WithAxis(p.SizeAxis, v/old)
			anchor := geom.V(0, 0, -n.Size.Z/2)
			if p.SizeAxis != 2 {
				anchor = geom.Vec3{}
			}
			n.M = n.M.Mul(geom.Translate(anchor)).Mul(geom.Scale(f)).Mul(geom.Translate(anchor.Neg()))
			doc.Normalize(n)
		}
		return nil
	}
	if shapes.IsProfile(n.Kind) {
		doc.SetProfileParam(n, key, v) // its points stay where they are
		return nil
	}
	bottom := n.M.Point(geom.V(0, 0, -n.Size.Z/2))
	k, _ := shapes.Lookup(n.Kind)
	vals := n.Values
	before, hasNatural := k.NaturalSize(vals())
	doc.SetParam(n, key, v)
	if hasNatural {
		// a generator parameter: keep the shape's scale, so its size
		// follows the generator's own output
		after, _ := k.NaturalSize(vals())
		for i := 0; i < 3; i++ {
			if b := before.Axis(i); b > 1e-9 {
				n.Size = n.Size.WithAxis(i, n.Size.Axis(i)*after.Axis(i)/b)
			}
		}
	}
	// the base stays where it was, so a change in height (text height, a
	// generator's output) grows upward from it
	n.M = geom.Translate(bottom.Sub(n.M.Point(geom.V(0, 0, -n.Size.Z/2)))).Mul(n.M)
	return nil
}

func (e *Editor) SetText(id int, text, font string) error {
	n := e.Doc.FindAny(id)
	if n == nil || n.Kind != "text" {
		return fmt.Errorf("node %d is not text", id)
	}
	if e.locked(id) {
		return errLocked
	}
	if font == "" {
		font = n.Font
	}
	return e.edit(func() error { doc.SetText(n, text, font); return nil })
}

// SetTextLive updates text without an undo step (typing in the field).
func (e *Editor) SetTextLive(n *doc.Node, text, font string) {
	e.Version++
	if e.locked(n.ID) {
		return
	}
	doc.SetText(n, text, font)
	e.changed()
}

// --- settings ----------------------------------------------------------------

func (e *Editor) SetUnits(u string) error {
	if u != "mm" && u != "in" {
		return fmt.Errorf("units must be mm or in")
	}
	return e.edit(func() error {
		s := &e.Doc.Settings
		if s.Units != u {
			s.Units = u
			if u == "in" {
				s.Snap = 25.4 / 16
				s.WorkplaneW = math.Round(s.WorkplaneW/25.4*100) / 100 * 25.4
				s.WorkplaneL = math.Round(s.WorkplaneL/25.4*100) / 100 * 25.4
			} else {
				s.Snap = 1
				s.WorkplaneW = math.Round(s.WorkplaneW)
				s.WorkplaneL = math.Round(s.WorkplaneL)
			}
		}
		return nil
	})
}

func (e *Editor) SetSnap(mm float64) {
	e.edit(func() error { e.Doc.Settings.Snap = mm; return nil })
}

func (e *Editor) SetWorkplaneSize(w, l float64) error {
	if w <= 0 || l <= 0 || w > 10000 || l > 10000 {
		return errors.New("bad build plate size")
	}
	return e.edit(func() error { e.Doc.Settings.WorkplaneW, e.Doc.Settings.WorkplaneL = w, l; return nil })
}

func (e *Editor) SetSettings(s doc.Settings) {
	e.edit(func() error { e.Doc.Settings = s; return nil })
}

// --- notes -------------------------------------------------------------------

// AddNote pins a note to the top-level node id (0: the workplane) at the
// world point p and returns its index there.
func (e *Editor) AddNote(id int, p geom.Vec3, text string) (int, error) {
	if id == 0 {
		idx := 0
		err := e.edit(func() error {
			e.Doc.Notes = append(e.Doc.Notes, doc.Note{P: p, Text: text})
			idx = len(e.Doc.Notes) - 1
			return nil
		})
		return idx, err
	}
	n := e.Doc.Find(id)
	if n == nil {
		return 0, fmt.Errorf("no shape %d", id)
	}
	inv, ok := n.M.Inverse()
	if !ok {
		return 0, errors.New("shape has a degenerate transform")
	}
	idx := 0
	err := e.edit(func() error {
		n.Notes = append(n.Notes, doc.Note{P: n.NoteParam(inv.Point(p)), Text: text})
		idx = len(n.Notes) - 1
		return nil
	})
	return idx, err
}

// notes returns the note list of node id (0: the workplane's), checking i.
func (e *Editor) notes(id, i int) (*[]doc.Note, error) {
	list := &e.Doc.Notes
	if id != 0 {
		n := e.Doc.FindAny(id)
		if n == nil {
			return nil, fmt.Errorf("no shape %d", id)
		}
		list = &n.Notes
	}
	if i < 0 || i >= len(*list) {
		return nil, fmt.Errorf("no note %d on %d", i, id)
	}
	return list, nil
}

// SetNoteText changes a note's text.
func (e *Editor) SetNoteText(id, i int, text string) error {
	list, err := e.notes(id, i)
	if err != nil {
		return err
	}
	return e.edit(func() error { (*list)[i].Text = text; return nil })
}

// DeleteNote removes a note.
func (e *Editor) DeleteNote(id, i int) error {
	list, err := e.notes(id, i)
	if err != nil {
		return err
	}
	return e.edit(func() error {
		*list = append((*list)[:i:i], (*list)[i+1:]...)
		if len(*list) == 0 {
			*list = nil
		}
		return nil
	})
}

// NoteRef identifies a note: the node it is pinned to (0: the workplane)
// and its index there.
type NoteRef struct {
	Node, Index int
	World       geom.Vec3
	Text        string
	Hidden      bool // its shape is hidden
}

// Notes lists every note with its world position, in document order.
func (e *Editor) Notes() []NoteRef {
	var out []NoteRef
	for i, nt := range e.Doc.Notes {
		out = append(out, NoteRef{Index: i, World: nt.P, Text: nt.Text})
	}
	var walk func(n *doc.Node, parent geom.Mat4, hidden bool)
	walk = func(n *doc.Node, parent geom.Mat4, hidden bool) {
		m := parent.Mul(n.M)
		hidden = hidden || n.Hidden
		for i, nt := range n.Notes {
			out = append(out, NoteRef{Node: n.ID, Index: i, World: m.Point(n.NotePoint(nt.P)), Text: nt.Text, Hidden: hidden})
		}
		for _, c := range n.Children {
			walk(c, m, hidden)
		}
	}
	for _, n := range e.Doc.Nodes {
		walk(n, geom.Identity(), false)
	}
	return out
}

// --- ruler / workplane --------------------------------------------------------

func (e *Editor) PlaceRuler(p geom.Vec3) { e.Ruler = &Ruler{Origin: p}; e.changed() }
func (e *Editor) RemoveRuler()           { e.Ruler = nil; e.changed() }

// TurnRuler rotates the ruler a quarter turn counter-clockwise about its
// origin (clicking the ruler's origin).
func (e *Editor) TurnRuler() {
	if e.Ruler != nil {
		e.Ruler.Turns = (e.Ruler.Turns + 1) % 4
		e.changed()
	}
}

// ToggleShapeWorkplane (the Shape Workplane button) puts the workplane on
// the selected shape — its own axes, on its own bottom, as the Workplane
// tool does — or removes the current one.
func (e *Editor) ToggleShapeWorkplane() error {
	switch {
	case e.Workplane != nil:
		return e.AttachWorkplane(0)
	case len(e.Sel) > 0:
		return e.AttachWorkplane(e.Sel[0])
	}
	return nil
}

// --- export --------------------------------------------------------------------

// ExportMesh evaluates the selection (or everything visible).
func (e *Editor) ExportMesh(selectionOnly bool) (*geom.Mesh, error) {
	var ids []int
	if selectionOnly {
		if len(e.Sel) == 0 {
			return nil, errors.New("you need to select something first")
		}
		ids = e.Sel
	}
	m := e.Doc.ExportMesh(ids)
	if len(m.Tris) == 0 {
		return nil, errors.New("nothing to export")
	}
	return m, nil
}

// Name is what the design is called: its file's name, or "" before its
// first save. (The title some files carry, such as files written by other
// tools, isn't used: the file name is the name.)
func (e *Editor) Name() string {
	if e.Path == "" {
		return ""
	}
	base := filepath.Base(e.Path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (e *Editor) Export(path string, selectionOnly bool) error {
	m, err := e.ExportMesh(selectionOnly)
	if err != nil {
		return err
	}
	// made in memory first, then written beside the file and renamed over
	// it: a failure never leaves an earlier export truncated
	var buf, mtl bytes.Buffer
	mtlPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".mtl"
	name := e.Name() // the model's name inside the file: the design's, whatever the export is called
	if name == "" {
		name = "design"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".stl":
		err = geom.WriteSTL(&buf, m, name)
	case ".obj":
		// the colours go in a material file beside it, as OBJ files do
		err = geom.WriteOBJ(&buf, &mtl, m, filepath.Base(mtlPath))
	case ".glb":
		err = geom.WriteGLB(&buf, m, name)
	case ".3mf":
		err = geom.Write3MF(&buf, m, name)
	case ".svg":
		// for laser cutting: the outline just above the design's base
		b := m.Bounds()
		loops := geom.Slice(m, b.Min.Z+0.05)
		if len(loops) == 0 {
			return errors.New("nothing to cut: the design has no outline at its base")
		}
		err = geom.WriteSVG(&buf, loops)
	default:
		return fmt.Errorf("unsupported export format %q", filepath.Ext(path))
	}
	if err != nil {
		return err
	}
	if err := WriteFileAtomic(path, buf.Bytes()); err != nil {
		return err
	}
	if mtl.Len() > 0 {
		return WriteFileAtomic(mtlPath, mtl.Bytes())
	}
	return nil
}

// WriteFileAtomic writes data to a file beside path and renames it over
// path, so path holds either its old contents or all of the new.
func WriteFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
