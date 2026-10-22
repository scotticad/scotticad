//go:build windows

package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/win"
)

// Notes: text pinned to points on shapes. The Notes tool (N) adds them by
// clicking a shape; every note shows as a speech-bubble marker over a pin,
// and an open note shows its text box above a "−" pin that closes it.

type noteID struct{ node, idx int }

var (
	colNotePin    = render.RGB(0x2b, 0x33, 0x3b)
	colNoteBubble = render.RGB(0x8a, 0x93, 0x9c)
)

// openNoteRef returns the open note, or nil (also when it has gone away,
// e.g. after an undo).
func (a *App) openNoteRef() *editor.NoteRef {
	o := a.noteOpen
	if o == nil {
		return nil
	}
	for _, r := range a.ed.Notes() {
		if r.Node == o.node && r.Index == o.idx && !r.Hidden { // its shape hidden: closed with it
			return &r
		}
	}
	a.noteOpen = nil
	return nil
}

// openNote shows a note's text box; edit also starts typing into it.
func (a *App) openNote(node, idx int, edit bool) {
	if o := a.noteOpen; o == nil || o.node != node || o.idx != idx {
		// closing an empty note deletes it: one before idx on the same
		// shape moves idx down
		closing := o != nil && o.node == node && o.idx < idx
		before := len(a.ed.Notes())
		a.closeNote()
		if closing && len(a.ed.Notes()) < before {
			idx--
		}
	}
	a.noteOpen = &noteID{node, idx}
	if edit {
		a.editNote()
	}
	a.w.Redraw()
}

func (a *App) editNote() {
	r := a.openNoteRef()
	if r == nil {
		return
	}
	node, idx, old := r.Node, r.Index, r.Text
	a.openField(&textField{id: "note-text", text: old, multiline: true, commit: func(s string) bool {
		if s != old {
			if err := a.ed.SetNoteText(node, idx, s); err != nil {
				a.showToast(err.Error())
			}
		}
		return true
	}})
}

// closeNote closes the open note; a note left empty is removed.
func (a *App) closeNote() {
	if a.noteOpen == nil {
		return
	}
	if a.field != nil && a.field.id == "note-text" {
		a.commitField()
	}
	if r := a.openNoteRef(); r != nil && strings.TrimSpace(r.Text) == "" {
		a.ed.DeleteNote(r.Node, r.Index)
	}
	a.noteOpen = nil
	a.w.Redraw()
}

// noteTarget is where a click would pin a note: on the shape under the
// cursor (node id), else on the workplane (id 0).
func (a *App) noteTarget(x, y float64) (id int, p geom.Vec3, ok bool) {
	if h, hit := a.pick(x, y, false, nil); hit {
		return h.node.ID, h.p, true
	}
	if p, hit := a.planeHitW(x, y, 0); hit {
		return 0, a.ed.Work().Point(p), true
	}
	return 0, geom.Vec3{}, false
}

// addNoteAt pins a new note under the cursor and opens it for typing. It
// reports whether there was anywhere to pin it.
func (a *App) addNoteAt(x, y float64) bool {
	id, p, ok := a.noteTarget(x, y)
	if !ok {
		return false
	}
	a.closeNote()
	idx, err := a.ed.AddNote(id, p, "")
	if err != nil {
		a.showToast(err.Error())
		return true
	}
	a.openNote(id, idx, true)
	return true
}

// deleteNote removes the open note.
func (a *App) deleteNote() {
	r := a.openNoteRef()
	if r == nil {
		return
	}
	if a.field != nil && a.field.id == "note-text" {
		a.field = nil // dropped with the note
	}
	a.noteOpen = nil
	if err := a.ed.DeleteNote(r.Node, r.Index); err != nil {
		a.showToast(err.Error())
	}
	a.w.Redraw()
}

// noteMarker draws a pin with a speech bubble above it; plus draws the
// "add a note here" cursor instead of the text lines, and halo a soft disc
// around the pin (hovered notes, and the cursor over a shape).
func noteMarker(b *render.Batch, x, y float64, plus bool, edge render.Color, halo bool) {
	if halo {
		b.Circle(x, y, 14, render.Color{1, 1, 1, 0.35})
		b.Circle(x, y, 9, render.Color{1, 1, 1, 0.3})
	}
	br := render.Rect{X: x - 12, Y: y - 36, W: 24, H: 21}
	b.Tri(x-5, br.Y+br.H-1, x+5, br.Y+br.H-1, x, y-8, colChrome)
	b.Line(x-5, br.Y+br.H, x, y-8, 1, edge)
	b.Line(x+5, br.Y+br.H, x, y-8, 1, edge)
	b.RoundRect(br, 3, colChrome)
	b.RoundRectStroke(br, 3, 1, edge)
	if plus {
		b.Line(x-5, br.CY(), x+5, br.CY(), 1.6, edge)
		b.Line(x, br.CY()-5, x, br.CY()+5, 1.6, edge)
	} else {
		for i, w := range []float64{6, 6, 4} {
			ly := br.Y + 6 + float64(i)*3.6
			b.Line(x-6, ly, x-6+2*w, ly, 1, edge)
		}
	}
	b.Circle(x, y, 5, colChrome)
	b.Ring(x, y, 5, 1.6, colNotePin)
}

// toggleNotes hides or shows every note; hiding closes the open one.
func (a *App) toggleNotes() {
	a.notesHidden = !a.notesHidden
	if a.notesHidden {
		a.closeNote()
	}
	a.w.Redraw()
}

func (a *App) drawNotes() {
	if a.notesHidden {
		return
	}
	b := a.batch
	open := a.openNoteRef()
	for _, r := range a.ed.Notes() {
		if r.Hidden && a.mode != modeVisibility {
			continue
		}
		if open != nil && r.Node == open.Node && r.Index == open.Index {
			continue // drawn last, on top
		}
		x, y, ok := a.project(r.World)
		if !ok || !a.vp.Contains(x, y) {
			continue
		}
		node, idx := r.Node, r.Index
		mid := fmt.Sprintf("note-%d-%d", node, idx)
		if a.isHot(mid) {
			noteMarker(b, x, y, false, colBlue, true)
		} else {
			noteMarker(b, x, y, false, colNoteBubble, false)
		}
		tip := strings.Join(strings.Fields(r.Text), " ")
		if rs := []rune(tip); len(rs) > 60 {
			tip = string(rs[:57]) + "…" // by letters: bytes would cut one in half
		}
		a.reg(region{r: render.Rect{X: x - 13, Y: y - 37, W: 26, H: 44}, id: mid, tip: tip, tipSide: tipAbove,
			press: func(float64, float64, win.Mods) func(float64, float64, bool) {
				a.openNote(node, idx, false)
				return nil
			}})
	}

	// the Notes tool: a note marker follows the pointer, blue over a shape
	// (with a halo on its surface), grey over the workplane
	if a.mode == modeNotes && a.drag == nil && a.hot == "viewport" {
		if id, _, ok := a.noteTarget(a.mx, a.my); ok {
			if id != 0 {
				noteMarker(b, a.mx, a.my, true, colBlue, true)
			} else {
				noteMarker(b, a.mx, a.my, true, colNoteBubble, false)
			}
		}
	}

	if open == nil {
		return
	}
	x, y, ok := a.project(open.World)
	if !ok {
		return
	}
	// "−" pin closes the note
	hot := a.isHot("note-close")
	pc := colBlue
	if hot {
		pc = colHandleHot
	}
	b.Circle(x, y, 8, colHandleLight)
	b.Ring(x, y, 8, 1.6, pc)
	b.Line(x-4, y, x+4, y, 1.6, pc)
	a.reg(region{r: rectAround(x, y, 9), id: "note-close", tip: "Close note", tipSide: tipBelow,
		press: func(float64, float64, win.Mods) func(float64, float64, bool) {
			a.closeNote()
			return nil
		}})
	// the text box, centred above the pin
	txt := open.Text
	editing := a.field != nil && a.field.id == "note-text"
	if editing {
		txt = a.field.text
	}
	// the box grows to fit (up to 440 wide), wrapping long lines
	w := 260.0
	for _, line := range strings.Split(txt, "\n") {
		w = math.Max(w, b.TextWidth(line, fontUI, false)+56)
	}
	w = math.Min(440, w)
	lh := b.LineHeight(fontUI, false)
	nl := len(wrapLines(b, txt, w-56, fontUI))
	h := math.Max(38, float64(nl)*lh+18)
	br := render.Rect{X: x - w/2, Y: y - 20 - h, W: w, H: h}
	b.RoundRect(br.Inset(-1), 4, colBorder)
	b.RoundRect(br, 4, colChrome)
	if editing {
		b.RoundRectStroke(br, 4, 1.2, colBlue)
	}
	tr := render.Rect{X: br.X + 10, Y: br.Y + (br.H-float64(nl)*lh)/2, W: br.W - 56, H: float64(nl) * lh}
	// trash: delete the note
	dr := render.Rect{X: br.X + br.W - 32, Y: br.Y + 7, W: 24, H: 24}
	dc := colIcon
	if a.isHot("note-delete") {
		b.RoundRect(dr, 4, colHoverBG)
		dc = colIconHover
	}
	drawIcon(b, icDelete, dr.Inset(3), dc)
	switch {
	case !editing && txt == "":
		b.Text("Click to edit this note", tr.X, tr.CY()-lh/2, fontUI, false, colTextDim)
	default:
		a.drawFieldTextWrapped("note-text", tr, txt, fontUI, colText)
	}
	a.reg(region{r: render.Rect{X: br.X, Y: br.Y, W: br.W - 36, H: br.H}, id: "note-box", cursor: win.CursorIBeam,
		press: func(float64, float64, win.Mods) func(float64, float64, bool) {
			if a.field == nil || a.field.id != "note-text" {
				a.editNote()
			}
			return nil
		}})
	a.button("note-delete", dr, "Delete note", "", tipBelow, a.deleteNote)
}
