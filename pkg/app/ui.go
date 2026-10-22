//go:build windows

package app

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/win"
)

// region is an interactive screen area registered while drawing a frame.
// Regions registered later are on top.
type region struct {
	r       render.Rect
	id      string
	cursor  int
	tip     string // tooltip title
	tipKey  string // shortcut line
	tipSide int    // 0 below, 1 right, 2 above
	// press is called on mouse down; returning a drag handler captures the
	// mouse until release.
	press func(x, y float64, m win.Mods) func(x, y float64, up bool)
	click func() // called on release over the same region
	wheel func(delta float64) bool
	// passThrough: after press, the click goes on to the region beneath
	// (a light popup's click-outside catcher: clicking another shape both
	// closes the popup and selects the shape)
	passThrough bool
}

const (
	tipBelow = iota
	tipRight
	tipAbove
)

func (a *App) reg(r region) { a.regions = append(a.regions, r) }

// clipRegions cuts the regions registered since index first down to clip
// (what has scrolled out of a panel can't be clicked), dropping any left
// with nothing showing.
func (a *App) clipRegions(first int, clip render.Rect) {
	kept := a.regions[:first]
	for _, rg := range a.regions[first:] {
		x0, y0 := math.Max(rg.r.X, clip.X), math.Max(rg.r.Y, clip.Y)
		x1, y1 := math.Min(rg.r.X+rg.r.W, clip.X+clip.W), math.Min(rg.r.Y+rg.r.H, clip.Y+clip.H)
		if x1 <= x0 || y1 <= y0 {
			continue
		}
		rg.r = render.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
		kept = append(kept, rg)
	}
	a.regions = kept
}

// button registers a clickable region.
func (a *App) button(id string, r render.Rect, tip, key string, side int, click func()) {
	a.reg(region{r: r, id: id, cursor: win.CursorHand, tip: tip, tipKey: key, tipSide: side, click: click})
}

func (a *App) isHot(id string) bool { return a.hot == id && id != "" }

// regionAt finds the topmost region under (x, y) in last frame's list.
func (a *App) regionAt(x, y float64) *region { return a.regionBelow(x, y, len(a.prevRegions)) }

// regionBelow is regionAt among the regions registered before index top.
func (a *App) regionBelow(x, y float64, top int) *region {
	for i := top - 1; i >= 0; i-- {
		r := &a.prevRegions[i]
		if r.wheel != nil && r.press == nil && r.click == nil && r.tip == "" {
			continue // scroll-only area: transparent to the pointer
		}
		if r.r.Contains(x, y) {
			return r
		}
	}
	return nil
}

func (a *App) updateHot() {
	id := ""
	cur := -1
	if r := a.regionAt(a.mx, a.my); r != nil {
		id = r.id
		cur = r.cursor
	}
	if id != a.hot {
		a.hot = id
		a.hoverSince = time.Now()
		if id != "" {
			a.wakeAfter(tooltipDelay * time.Millisecond)
		}
		a.w.Redraw()
	}
	if cur > 0 {
		a.w.SetCursor(cur)
	} else {
		a.w.SetCursor(a.viewCursor())
	}
}

// wakeAfter schedules a redraw (for tooltips and toasts). Every timer
// forces its own redraw, so overlapping timers never cancel each other.
func (a *App) wakeAfter(d time.Duration) {
	time.AfterFunc(d+5*time.Millisecond, func() {
		a.wakePending.Store(true)
		a.w.Wake()
	})
}

// --- tooltips ---------------------------------------------------------------

func (a *App) drawTooltip() {
	if a.hot == "" || a.pressed != "" || a.drag != nil {
		return
	}
	if time.Since(a.hoverSince) < tooltipDelay*time.Millisecond {
		return
	}
	var r *region
	for i := range a.prevRegions {
		if a.prevRegions[i].id == a.hot {
			r = &a.prevRegions[i]
		}
	}
	if r == nil || r.tip == "" {
		return
	}
	b := a.batch
	tw := b.TextWidth(r.tip, fontUI, true)
	kw := 0.0
	if r.tipKey != "" {
		kw = b.TextWidth(r.tipKey, fontSmall, false)
	}
	pad := 10.0
	var box render.Rect
	if r.tipSide == tipRight {
		// title and key on one line, key in a badge
		w := tw + pad*2
		if kw > 0 {
			w += kw + 16
		}
		box = render.Rect{X: r.r.X + r.r.W + 8, Y: r.r.CY() - 15, W: w, H: 30}
	} else {
		w := tw
		if kw > w {
			w = kw
		}
		h := 30.0
		if kw > 0 {
			h = 46
		}
		box = render.Rect{X: r.r.CX() - (w+pad*2)/2, Y: r.r.Y + r.r.H + 6, W: w + pad*2, H: h}
		if r.tipSide == tipAbove {
			box.Y = r.r.Y - 6 - h
		}
	}
	if box.X+box.W > a.W-4 {
		box.X = a.W - 4 - box.W
	}
	if box.X < 4 {
		box.X = 4
	}
	b.RoundRect(box, 5, colTooltip)
	if r.tipSide == tipRight {
		b.Text(r.tip, box.X+pad, box.CY()-b.LineHeight(fontUI, true)/2, fontUI, true, colOnColor)
		if kw > 0 {
			kb := render.Rect{X: box.X + pad + tw + 8, Y: box.CY() - 9, W: kw + 8, H: 18}
			b.RoundRectStroke(kb, 3, 1, colTooltipKey)
			b.TextCentered(r.tipKey, kb, fontSmall, false, colTooltipKey)
		}
		return
	}
	b.Text(r.tip, box.X+pad, box.Y+6, fontUI, true, colOnColor)
	if kw > 0 {
		b.Text(r.tipKey, box.X+pad, box.Y+25, fontSmall, false, colTooltipKey)
	}
}

// --- text fields -------------------------------------------------------------

// textField is the single active editable field.
type textField struct {
	id       string
	text     string
	cursor   int                 // byte offset
	selAll   bool                // whole text selected (initial state)
	commit   func(s string) bool // returns false to keep editing
	live     func(s string)      // called on every edit
	cancel   func()
	tabOrder []string // ids for Tab navigation
	openNext func(id string)
	// multiline: Shift+Enter starts a new line (Enter still commits)
	multiline bool
}

// openField makes f the active field. Clicking a field that is already
// being edited keeps what has been typed.
func (a *App) openField(f *textField) {
	if a.field != nil && a.field.id == f.id {
		return
	}
	if a.field != nil {
		a.commitField()
	}
	f.cursor = len(f.text)
	f.selAll = true
	a.field = f
	a.w.Redraw()
}

func (a *App) commitField() bool {
	f := a.field
	if f == nil {
		return true
	}
	a.field = nil
	if f.commit != nil && !f.commit(f.text) {
		a.field = f
		return false
	}
	a.w.Redraw()
	return true
}

func (a *App) cancelField() {
	if a.field != nil && a.field.cancel != nil {
		a.field.cancel()
	}
	a.field = nil
	a.w.Redraw()
}

func (a *App) fieldKey(vk int, m win.Mods) bool {
	f := a.field
	if f == nil {
		return false
	}
	switch vk {
	case vkReturn:
		if f.multiline && m&win.Shift != 0 {
			a.fieldInsert("\n")
			break
		}
		a.commitField()
	case vkEscape:
		a.cancelField()
	case vkTab:
		next := ""
		if len(f.tabOrder) > 0 {
			idx := 0
			for i, id := range f.tabOrder {
				if id == f.id {
					idx = i
				}
			}
			step := 1
			if m&win.Shift != 0 {
				step = len(f.tabOrder) - 1
			}
			next = f.tabOrder[(idx+step)%len(f.tabOrder)]
		}
		open := f.openNext
		if a.commitField() && next != "" && open != nil {
			// on the next frame: the fields are drawn again from the
			// design as the commit left it (opened now, the next one would
			// commit values from before it)
			a.pendingOpen = func() { open(next) }
			a.w.Redraw()
		}
	case vkBack:
		if f.selAll {
			f.text, f.cursor, f.selAll = "", 0, false
		} else if f.cursor > 0 {
			_, n := utf8.DecodeLastRuneInString(f.text[:f.cursor])
			f.text = f.text[:f.cursor-n] + f.text[f.cursor:]
			f.cursor -= n
		}
		a.fieldLive()
	case vkDelete:
		if f.selAll {
			f.text, f.cursor, f.selAll = "", 0, false
		} else if f.cursor < len(f.text) {
			_, n := utf8.DecodeRuneInString(f.text[f.cursor:])
			f.text = f.text[:f.cursor] + f.text[f.cursor+n:]
		}
		a.fieldLive()
	case vkLeft:
		if f.selAll {
			f.cursor, f.selAll = 0, false
		} else if f.cursor > 0 {
			_, n := utf8.DecodeLastRuneInString(f.text[:f.cursor])
			f.cursor -= n
		}
	case vkRight:
		f.selAll = false
		if f.cursor < len(f.text) {
			_, n := utf8.DecodeRuneInString(f.text[f.cursor:])
			f.cursor += n
		}
	case vkHome:
		f.cursor, f.selAll = 0, false
	case vkEnd:
		f.cursor, f.selAll = len(f.text), false
	case 'A':
		if m&win.Ctrl != 0 {
			f.selAll = true
		}
	case 'V':
		if m&win.Ctrl != 0 {
			a.fieldInsert(pastable(a.w.Clipboard(), f.multiline))
		}
	case 'C':
		if m&win.Ctrl != 0 {
			a.w.SetClipboard(f.text)
		}
	}
	a.w.Redraw()
	return true
}

// pastable is clipboard text as a field takes it: Windows line breaks
// (\r\n, or a lone \r) made plain ones, kept in a field of several lines
// (a note) and turned to spaces in a one-line field; tabs become spaces.
func pastable(s string, multiline bool) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	s = strings.ReplaceAll(s, "\t", " ")
	if !multiline {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	return s
}

func (a *App) fieldInsert(s string) {
	f := a.field
	if f.selAll {
		f.text, f.cursor, f.selAll = "", 0, false
	}
	f.text = f.text[:f.cursor] + s + f.text[f.cursor:]
	f.cursor += len(s)
	a.fieldLive()
}

func (a *App) fieldLive() {
	if a.field != nil && a.field.live != nil {
		a.field.live(a.field.text)
	}
}

func (a *App) fieldChar(r rune) bool {
	if a.field == nil {
		return false
	}
	if r < 32 || r == 127 {
		return true
	}
	a.fieldInsert(string(r))
	a.w.Redraw()
	return true
}

// drawFieldText renders the active field (or a static value) into r.
func (a *App) drawFieldText(id string, r render.Rect, value string, size float64, center bool, col render.Color) {
	b := a.batch
	if a.field != nil && a.field.id == id {
		f := a.field
		tw := b.TextWidth(f.text, size, false)
		x := r.X + labelPadX
		if center {
			x = r.CX() - tw/2
		}
		lh := b.LineHeight(size, false)
		y := r.CY() - lh/2
		if f.selAll && f.text != "" {
			b.Fill(render.Rect{X: x - 1, Y: y, W: tw + 2, H: lh}, colTextSel)
			b.Text(f.text, x, y, size, false, colOnColor)
		} else {
			b.Text(f.text, x, y, size, false, col)
			cx := x + b.TextWidth(f.text[:f.cursor], size, false)
			if (time.Now().UnixMilli()/530)%2 == 0 || a.drag != nil {
				b.Fill(render.Rect{X: cx, Y: y + 1, W: 1.2, H: lh - 2}, colText)
			}
			a.blink = true
		}
		return
	}
	if center {
		b.TextCentered(value, r, size, false, col)
	} else {
		b.Text(value, r.X+labelPadX, r.CY()-b.LineHeight(size, false)/2, size, false, col)
	}
}

// wrapLines splits text into lines no wider than w: at newlines, and at
// spaces (or anywhere, for a word longer than w). Each line is a byte range
// of text; the spaces and newlines it broke at are left out.
func wrapLines(b *render.Batch, text string, w, size float64) [][2]int {
	var out [][2]int
	start := 0
	for start <= len(text) {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += start
		}
		// wrap the paragraph text[start:end]
		ls := start
		for b.TextWidth(text[ls:end], size, false) > w {
			cut, lastSpace := ls, -1
			for i, r := range text[ls:end] {
				_, n := utf8.DecodeRuneInString(text[ls+i:]) // its bytes (an invalid one is 1, not RuneLen's 3)
				j := ls + i + n
				if b.TextWidth(text[ls:j], size, false) > w {
					break
				}
				cut = j
				if r == ' ' {
					lastSpace = ls + i
				}
			}
			if lastSpace > ls {
				out = append(out, [2]int{ls, lastSpace})
				ls = lastSpace + 1
			} else {
				if cut == ls { // a single glyph wider than w
					_, n := utf8.DecodeRuneInString(text[ls:])
					cut = ls + n
				}
				out = append(out, [2]int{ls, cut})
				ls = cut
			}
		}
		out = append(out, [2]int{ls, end})
		start = end + 1
	}
	return out
}

// drawFieldTextWrapped is drawFieldText for multi-line text in r: lines
// are wrapped to r's width and drawn from its top.
func (a *App) drawFieldTextWrapped(id string, r render.Rect, value string, size float64, col render.Color) {
	b := a.batch
	f := a.field
	editing := f != nil && f.id == id
	if editing {
		value = f.text
	}
	lh := b.LineHeight(size, false)
	lines := wrapLines(b, value, r.W, size)
	for i, l := range lines {
		y := r.Y + float64(i)*lh
		s := value[l[0]:l[1]]
		if editing && f.selAll && value != "" {
			b.Fill(render.Rect{X: r.X - 1, Y: y, W: b.TextWidth(s, size, false) + 2, H: lh}, colTextSel)
			b.Text(s, r.X, y, size, false, colOnColor)
			continue
		}
		b.Text(s, r.X, y, size, false, col)
	}
	if editing && !f.selAll {
		// the caret: on the last line whose range holds it
		li := 0
		for i, l := range lines {
			if f.cursor >= l[0] && f.cursor <= l[1] {
				li = i
			}
		}
		l := lines[li]
		cx := r.X + b.TextWidth(value[l[0]:min(max(f.cursor, l[0]), l[1])], size, false)
		if (time.Now().UnixMilli()/530)%2 == 0 || a.drag != nil {
			b.Fill(render.Rect{X: cx, Y: r.Y + float64(li)*lh + 1, W: 1.2, H: lh - 2}, colText)
		}
		a.blink = true
	}
}

// --- popups -------------------------------------------------------------------

type popupKind int

const (
	popNone popupKind = iota
	popVisibility
	popSnap
	popColor
	popFont
	popLibrary
	popDesignMenu
	popSettings
	popImport
	popExport
	popHelp
	popAbout
	popStart
)

func (a *App) openPopup(p popupKind) {
	a.commitField()
	if a.popup == p {
		a.popup = popNone
	} else {
		a.popup = p
	}
	a.w.Redraw()
}

// closePopup closes the open popup; changes still being drafted in it (the
// Settings dialog) are discarded.
func (a *App) closePopup() {
	a.popup = popNone
	a.settingsDraft = nil
	a.bgPicker, a.pickTarget = false, nil
	a.openDrop = ""
	a.w.Redraw()
}

// catcher registers a full-window region that closes the popup when
// clicked outside its content (dim draws a modal backdrop).
func (a *App) catcher(dim bool) {
	if dim {
		a.batch.Fill(render.Rect{W: a.W, H: a.H}, colDim)
	}
	a.reg(region{r: render.Rect{W: a.W, H: a.H}, id: "catcher", passThrough: !dim, press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
		if !dim {
			a.closePopup()
		}
		return nil
	}})
}

// panel draws a rounded popup with a soft shadow.
func (a *App) panel(r render.Rect) {
	b := a.batch
	for i := 1.0; i <= 6; i++ {
		b.RoundRect(render.Rect{X: r.X - i + 1, Y: r.Y - i + 3, W: r.W + 2*i - 2, H: r.H + 2*i - 2}, 6+i, render.Color{0, 0, 0, 0.025})
	}
	b.RoundRect(r, 5, colChrome)
	b.RoundRectStroke(r, 5, 1, colBorder)
	// swallow clicks on the panel body
	a.reg(region{r: r, id: "panel", press: func(x, y float64, m win.Mods) func(float64, float64, bool) { return nil }})
}

// toggle draws an Off/On pill switch.
func (a *App) toggle(id string, r render.Rect, on bool, set func(bool)) {
	b := a.batch
	b.RoundRect(r, r.H/2, colHoverBG)
	b.RoundRectStroke(r, r.H/2, 1, colBorder)
	half := render.Rect{X: r.X + 2, Y: r.Y + 2, W: r.W/2 - 2, H: r.H - 4}
	if on {
		half.X = r.X + r.W/2
	}
	b.RoundRect(half, half.H/2, colBlue)
	offC, onC := colOnColor, colTextDim
	if on {
		offC, onC = colTextDim, colOnColor
	}
	b.TextCentered("Off", render.Rect{X: r.X, Y: r.Y, W: r.W / 2, H: r.H}, fontSmall, false, offC)
	b.TextCentered("On", render.Rect{X: r.X + r.W/2, Y: r.Y, W: r.W / 2, H: r.H}, fontSmall, false, onC)
	a.button(id, r, "", "", 0, func() { set(!on) })
}

// checkbox draws a square check with a label.
func (a *App) checkbox(id string, x, y float64, label string, on bool, set func(bool)) {
	b := a.batch
	box := render.Rect{X: x, Y: y, W: 16, H: 16}
	if on {
		b.RoundRect(box, 3, colBlue)
		p := newPen(b, box, colOnColor)
		p.w = 2
		p.poly(false, 5, 12, 10, 17, 19, 7)
	} else {
		b.RoundRectStroke(box, 3, 1.2, colFieldBorder)
	}
	tw := b.Text(label, x+24, y+8-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
	a.button(id, render.Rect{X: x - 2, Y: y - 3, W: tw + 30, H: 22}, "", "", 0, func() { set(!on) })
}

// primaryButton draws a filled blue rounded button.
func (a *App) primaryButton(id string, r render.Rect, label string, click func()) {
	b := a.batch
	c := colBlue
	if a.isHot(id) {
		c = colBlueDark
	}
	b.RoundRect(r, r.H/2, c)
	b.TextCentered(label, r, fontUI, true, colOnColor)
	a.button(id, r, "", "", 0, click)
}

// textButton draws a plain bordered button.
func (a *App) textButton(id string, r render.Rect, label string, active bool, click func()) {
	b := a.batch
	bg := colChrome
	if a.isHot(id) || active {
		bg = colHoverBG
	}
	b.RoundRect(r, 4, bg)
	b.RoundRectStroke(r, 4, 1, colBorder)
	c := colText
	if active {
		c = colBlue
	}
	b.TextCentered(label, r, fontUI, false, c)
	a.button(id, r, "", "", 0, click)
}
