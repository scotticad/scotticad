//go:build windows

package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/scotticad/scotticad/pkg/doc"
	"github.com/scotticad/scotticad/pkg/editor"
	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/shapes"
	"github.com/scotticad/scotticad/pkg/win"
)

// Preset colours: ten hues from red round to magenta plus two greys, in a
// pastel, a saturated and a dark row, generated from HSV.
var presetColors = presetPalette()

func presetPalette() [3][12]string {
	hues := []float64{0, 28, 52, 90, 140, 172, 203, 230, 268, 320}
	sv := [3][2]float64{{0.33, 0.97}, {0.85, 0.90}, {0.85, 0.52}} // saturation, value per row
	greys := [3][2]float64{{0.90, 1.0}, {0.60, 0.80}, {0.30, 0.12}}
	var out [3][12]string
	for r := range sv {
		for i, h := range hues {
			out[r][i] = rgbHex(hsvRGB(h, sv[r][0], sv[r][1]))
		}
		for j, g := range greys[r] {
			out[r][10+j] = rgbHex(g, g, g)
		}
	}
	return out
}

func (a *App) inspectorRect(h float64) render.Rect {
	return render.Rect{X: a.vp.X + a.vp.W - inspectorW - 12, Y: a.vp.Y + 10, W: inspectorW, H: h}
}

func (a *App) drawInspector() {
	ns := a.ed.Selected()
	if len(ns) == 0 {
		if a.popup == popColor || a.popup == popFont {
			a.popup = popNone
		}
		return
	}
	b := a.batch
	n := ns[0]
	multi := len(ns) > 1
	title := n.Label()
	if multi {
		title = fmt.Sprintf("Shapes(%d)", len(ns))
	}
	var params []shapes.Param
	if !multi {
		kind := n.Kind
		if n.IsGroup() && n.Assembly != "" {
			kind = n.Assembly // an enclosure's controls rebuild all its parts
		}
		if k, ok := shapes.Lookup(kind); ok {
			params = k.Params
		}
	}
	showProps := !multi && (len(params) > 0 || n.IsGroup())

	// measure height
	h := 40.0
	if a.inspectorOpen {
		h += 86
		if showProps {
			h += 34
			if a.propsOpen {
				h += a.paramsHeight(n, params) + 8
			}
		}
	}
	// no taller than the viewport allows: the body scrolls instead (a
	// shape of many sliders in a small window)
	avail := a.vp.H - 22 // 10 above, 12 below
	overflow := math.Max(0, h-avail)
	if overflow > 0 {
		h = avail
	}
	a.inspScroll = math.Max(0, math.Min(overflow, a.inspScroll))
	r := a.inspectorRect(h)
	a.panel(r)
	a.reg(region{r: r, id: "inspector", wheel: func(d float64) bool {
		if overflow <= 0 {
			return false
		}
		a.inspScroll = math.Max(0, math.Min(overflow, a.inspScroll-d*40))
		return true
	}})

	// header
	chev := icChevronUp
	if !a.inspectorOpen {
		chev = icChevronDown
	}
	cr := render.Rect{X: r.X + 6, Y: r.Y + 8, W: 24, H: 24}
	drawIcon(b, chev, cr.Inset(4), colIcon)
	a.button("insp-collapse", cr, "", "", 0, func() { a.inspectorOpen = !a.inspectorOpen })
	b.Text(title, r.X+36, r.Y+20-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
	locked := true
	for _, x := range ns {
		locked = locked && x.Locked
	}
	lr := render.Rect{X: r.X + r.W - 64, Y: r.Y + 8, W: 24, H: 24}
	lc := colIcon
	if a.isHot("insp-lock") || locked {
		lc = colIconHover
	}
	li := icUnlock
	if locked {
		li = icLock
	}
	drawIcon(b, li, lr.Inset(2), lc)
	lockTip := "Lock"
	if locked {
		lockTip = "Unlock"
	}
	a.button("insp-lock", lr, lockTip, "Ctrl + L", tipBelow, a.toggleLock)
	hr := render.Rect{X: r.X + r.W - 34, Y: r.Y + 8, W: 24, H: 24}
	hc := colIcon
	if a.isHot("insp-hide") {
		hc = colIconHover
	}
	drawIcon(b, icEyeOff, hr.Inset(2), hc)
	a.button("insp-hide", hr, "Hide", "Ctrl + H", tipBelow, a.ed.HideSelected)
	if !a.inspectorOpen {
		return
	}

	// the body, scrolled up by what the panel is too short to show: drawn,
	// and clickable, only within the panel
	clip := render.Rect{X: r.X, Y: r.Y + 40, W: r.W, H: r.H - 40}
	b.PushClip(clip)
	first := len(a.regions)
	endBody := func() { b.PopClip(); a.clipRegions(first, clip) }

	// Solid / Hole
	body := render.Rect{X: r.X + 8, Y: r.Y + 40 - a.inspScroll, W: r.W - 16, H: 78}
	b.RoundRect(body, 4, colRaised)
	allHole, anyHole := true, false
	for _, x := range ns {
		allHole = allHole && x.Hole
		anyHole = anyHole || x.Hole
	}
	sx, hx := body.X+body.W*0.3, body.X+body.W*0.7
	dy := body.Y + 28
	solidHot := a.isHot("insp-solid")
	rainbow := n.IsBundle() || multi && !sameColor(ns) || (n.IsGroup() && n.Multicolor)
	if solidHot || !allHole {
		b.Circle(sx, dy, 17, colSwatchOff)
	}
	if rainbow {
		drawRainbow(b, sx, dy, 14)
	} else {
		b.Circle(sx, dy, 14, colorOf(solidColor(ns)))
	}
	if !allHole || solidHot {
		b.Ring(sx, dy, 15.5, 1, colChrome)
	}
	if a.isHot("insp-hole") || allHole {
		b.Circle(hx, dy, 17, colSwatchOff)
	}
	drawStripedDisc(b, hx, dy, 14)
	sc, hc2 := colTextDim, colTextDim
	if !anyHole {
		sc = colBlue
	}
	if allHole {
		hc2 = colBlue
	}
	b.TextCentered("Solid", render.Rect{X: sx - 30, Y: dy + 18, W: 60, H: 18}, fontSmall, false, sc)
	b.TextCentered("Hole", render.Rect{X: hx - 30, Y: dy + 18, W: 60, H: 18}, fontSmall, false, hc2)
	a.button("insp-solid", rectAround(sx, dy+8, 26), "", "", 0, func() {
		if allHole {
			a.makeSolid()
		}
		a.openPopup(popColor)
	})
	a.button("insp-hole", rectAround(hx, dy+8, 26), "", "", 0, func() {
		a.ed.SetHole(true)
		if a.popup == popColor {
			a.closePopup()
		}
	})

	if !showProps {
		endBody()
		if a.popup == popColor {
			a.drawColorPicker(r, a.pickerNodes(ns))
		}
		return
	}

	// Properties section
	py := body.Y + body.H + 6
	pr := render.Rect{X: r.X + 8, Y: py, W: r.W - 16, H: 28}
	b.Text("Properties", pr.X+4, pr.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
	pc := icChevronUp
	if !a.propsOpen {
		pc = icChevronDown
	}
	drawIcon(b, pc, render.Rect{X: pr.X + pr.W - 22, Y: pr.CY() - 8, W: 16, H: 16}, colIcon)
	a.button("insp-props", pr, "", "", 0, func() { a.propsOpen = !a.propsOpen })
	if a.propsOpen {
		a.drawParams(n, params, pr.X, py+34, pr.W)
	}
	endBody()
	if a.popup == popColor {
		a.drawColorPicker(r, a.pickerNodes(ns))
	}
}

// pickerNodes are the shapes the colour picker shows and changes: the
// unlocked ones selected (it can't change the others), or all of them if
// every one is locked (then it only shows).
func (a *App) pickerNodes(ns []*doc.Node) []*doc.Node {
	if e := a.ed.Editable(); len(e) > 0 {
		return e
	}
	return ns
}

func sameColor(ns []*doc.Node) bool {
	for _, n := range ns {
		if n.Color != ns[0].Color {
			return false
		}
	}
	return true
}

func solidColor(ns []*doc.Node) string {
	for _, n := range ns {
		if !n.Hole {
			return n.Color
		}
	}
	return ns[0].Color
}

func colorOf(hex string) render.Color {
	c := geom.ParseColor(hex)
	return render.Color{c.R, c.G, c.B, 1}
}

func drawRainbow(b *render.Batch, cx, cy, r float64) {
	cols := []render.Color{render.RGB(0xe8, 0x30, 0x2f), render.RGB(0xf5, 0x8a, 0x1f), render.RGB(0xf7, 0xc5, 0x1e),
		render.RGB(0x2f, 0xa8, 0x4f), render.RGB(0x1a, 0x9f, 0xe0), render.RGB(0x7a, 0x3f, 0xc0)}
	n := len(cols)
	for i := 0; i < n; i++ {
		a0 := 2*math.Pi*float64(i)/float64(n) - math.Pi/2
		a1 := 2*math.Pi*float64(i+1)/float64(n) - math.Pi/2
		for k := 0; k < 6; k++ {
			t0 := a0 + (a1-a0)*float64(k)/6
			t1 := a0 + (a1-a0)*float64(k+1)/6
			b.Tri(cx, cy, cx+r*math.Cos(t0), cy+r*math.Sin(t0), cx+r*math.Cos(t1), cy+r*math.Sin(t1), cols[i])
		}
	}
}

func drawStripedDisc(b *render.Batch, cx, cy, r float64) {
	b.Circle(cx, cy, r, render.RGB(0xd0, 0xd3, 0xd7))
	c := render.RGB(0xa9, 0xae, 0xb4)
	for k := -4; k <= 4; k++ {
		d := float64(k) * r / 3.2
		// the chord of the line x - y = d (relative to the centre), which
		// lies d/√2 from the centre
		half := math.Sqrt(math.Max(0, r*r-d*d/2)) * 0.98
		mx, my := cx+d/2, cy-d/2
		b.Line(mx-half/math.Sqrt2, my-half/math.Sqrt2, mx+half/math.Sqrt2, my+half/math.Sqrt2, 2, c)
	}
}

// --- parameters ----------------------------------------------------------------

func (a *App) paramsHeight(n *doc.Node, ps []shapes.Param) float64 {
	if n.IsGroup() && n.Assembly == "" {
		return 70
	}
	h := 0.0
	if n.Assembly != "" {
		h += 40 // the "ungroup to separate" hint
	}
	for _, p := range ps {
		switch p.Kind {
		case shapes.Slider:
			h += 46
		default:
			h += 52
		}
	}
	return h
}

func (a *App) paramValue(n *doc.Node, p shapes.Param) float64 {
	if p.SizeAxis >= 0 && p.Kind == shapes.Number {
		return n.AxisLength(p.SizeAxis)
	}
	if v, ok := n.Params[p.Key]; ok {
		return v
	}
	return p.Default
}

func (a *App) drawParams(n *doc.Node, ps []shapes.Param, x, y, w float64) {
	b := a.batch
	if n.Assembly != "" {
		b.Text("Ungroup (Ctrl+Shift+G) to separate", x+4, y, fontSmall, false, colTextDim)
		b.Text("the parts.", x+4, y+16, fontSmall, false, colTextDim)
		y += 40
	} else if n.IsGroup() {
		mode := map[string]string{doc.ModeUnion: "Union", doc.ModeBundle: "Bundle", doc.ModeIntersect: "Intersect"}[n.Mode()]
		tw := b.Text("Group type: "+mode, x+4, y, fontSmall, false, colText)
		ir := render.Rect{X: x + 4 + tw + 6, Y: y - 1, W: 16, H: 16}
		drawIcon(b, icInfo, ir, colIcon)
		a.button("insp-groupinfo", ir, "Bundle keeps shapes separate; Union merges them; Intersect keeps only where they overlap", "", tipBelow, func() {})
		bx := x + 4
		for _, m := range []struct {
			mode, tip string
			icon      int
		}{{doc.ModeBundle, "Bundle", icBundleGroupType}, {doc.ModeUnion, "Union", icUnionGroupType}, {doc.ModeIntersect, "Intersect", icIntersectGroupType}} {
			r := render.Rect{X: bx, Y: y + 22, W: 40, H: 34}
			active := m.mode == n.Mode()
			b.RoundRect(r, 4, colChrome)
			bc := colBorder
			if active {
				bc = colBlue
			}
			b.RoundRectStroke(r, 4, 1.5, bc)
			ic := colIcon
			if active || a.isHot("insp-gt-"+m.mode) {
				ic = colIconHover
			}
			drawIcon(b, m.icon, render.Rect{X: r.CX() - 11, Y: r.CY() - 11, W: 22, H: 22}, ic)
			mm := m.mode
			a.button("insp-gt-"+m.mode, r, m.tip, "", tipBelow, func() { a.ed.SetGroupMode(mm) })
			bx += 48
		}
		return
	}
	units := a.units()
	for _, p := range ps {
		p := p
		id := "param-" + p.Key
		switch p.Kind {
		case shapes.Slider:
			label := p.Label
			if p.Metric && units == "in" {
				label += " (mm)" // a metric standard's value, in mm whatever the units
			}
			b.Text(label, x+4, y, fontSmall, false, colTextDim)
			a.slider(n, p, render.Rect{X: x + 8, Y: y + 18, W: w - 24, H: 20})
			y += 46
		case shapes.Number:
			b.Text(p.Label, x+4, y, fontSmall, false, colTextDim)
			fr := render.Rect{X: x + 4, Y: y + 18, W: w - 8, H: 26}
			v := a.paramValue(n, p)
			a.numberField(id, fr, editor.Format(v, units, true), func(s string) bool {
				mm, err := editor.Parse(s, units)
				if err != nil {
					a.showToast(err.Error())
					return true
				}
				if err := a.ed.SetParam(n.ID, p.Key, mm); err != nil {
					a.showToast(err.Error())
				}
				return true
			}, func(dir float64) {
				step := p.Step
				if step <= 0 {
					step = 1
				}
				if units == "in" && !p.Int {
					step = 25.4 / 16
				}
				if err := a.ed.SetParam(n.ID, p.Key, math.Max(p.Min, v+dir*step)); err != nil {
					a.showToast(err.Error())
				}
			})
			y += 52
		case shapes.TextParam:
			b.Text(p.Label, x+4, y, fontSmall, false, colTextDim)
			fr := render.Rect{X: x + 4, Y: y + 18, W: w - 8, H: 26}
			a.textInput(id, fr, n.Text, func(s string) {
				a.ed.SetTextLive(n, s, n.Font)
			}, func(s string) bool {
				// typing changed the doc live; record one undo step
				a.ed.EndEdit()
				return true
			}, func() {
				a.ed.CancelEdit()
			})
			y += 52
		case shapes.FontParam:
			b.Text(p.Label, x+4, y, fontSmall, false, colTextDim)
			fr := render.Rect{X: x + 4, Y: y + 18, W: w - 8, H: 26}
			bg := colChrome
			if a.isHot(id) || a.popup == popFont {
				bg = colHoverBG
			}
			b.RoundRect(fr, 3, bg)
			b.RoundRectStroke(fr, 3, 1, colFieldBorder)
			b.Text(n.Font, fr.X+8, fr.CY()-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
			drawIcon(b, icChevronDown, render.Rect{X: fr.X + fr.W - 20, Y: fr.CY() - 7, W: 14, H: 14}, colIcon)
			a.button(id, fr, "", "", 0, func() { a.openPopup(popFont) })
			if a.popup == popFont {
				a.fontMenuAt = fr
			}
			y += 52
		}
	}
}

// slider draws a slider: blue fill, round knob, value bubble.
func (a *App) slider(n *doc.Node, p shapes.Param, r render.Rect) {
	b := a.batch
	id := "slider-" + p.Key
	v := a.paramValue(n, p)
	t := 0.0
	if p.Max > p.Min {
		t = (v - p.Min) / (p.Max - p.Min)
	}
	t = math.Max(0, math.Min(1, t))
	cy := r.CY()
	kx := r.X + t*r.W
	b.RoundRect(render.Rect{X: r.X, Y: cy - 1.5, W: r.W, H: 3}, 1.5, colDivider)
	b.RoundRect(render.Rect{X: r.X, Y: cy - 1.5, W: kx - r.X, H: 3}, 1.5, colBlue)
	dragging := a.pressed == id
	units := a.units()
	show := func(v float64) string { return fmtSlider(v, p, units) }
	label := show(v)
	fid := "param-" + p.Key // the knob as a text field
	typing := a.field != nil && a.field.id == fid
	if typing {
		label = a.field.text
	}
	kw := math.Max(18, b.TextWidth(label, fontSmall-1, false)+8)
	if typing {
		kw = math.Max(40, kw+8)
	}
	knob := render.Rect{X: kx - kw/2, Y: cy - 9, W: kw, H: 18}
	b.RoundRect(knob, 9, colChrome)
	if typing {
		b.RoundRectStroke(knob, 9, 1.4, colBlue)
		a.drawFieldText(fid, knob, label, fontSmall-1, true, colText)
	} else {
		b.RoundRectStroke(knob, 9, 1, colFieldBorder)
		b.TextCentered(label, knob, fontSmall-1, false, colText)
	}
	if dragging {
		// dark value bubble above the knob
		bw := b.TextWidth(label, fontSmall, true) + 14
		br := render.Rect{X: kx - bw/2, Y: cy - 38, W: bw, H: 22}
		b.RoundRect(br, 4, colTooltip)
		b.Tri(kx-5, br.Y+br.H, kx+5, br.Y+br.H, kx, br.Y+br.H+5, colTooltip)
		b.TextCentered(label, br, fontSmall, true, colOnColor)
	}
	id2 := n.ID
	key := p.Key
	set := func(x float64) {
		tt := math.Max(0, math.Min(1, (x-r.X)/r.W))
		val := p.Min + tt*(p.Max-p.Min)
		if p.Length && units == "in" {
			// in steps of a round fraction of an inch
			st := inchStep(p.Step) * 25.4
			val = math.Max(p.Min, math.Min(p.Max, math.Round(val/st)*st))
		} else if p.Step > 0 {
			val = math.Round(val/p.Step) * p.Step
		}
		a.ed.SetParamLive(id2, key, val)
	}
	// the knob: click to type a value, drag to slide
	open := func() {
		text := show(a.paramValue(n, p))
		if p.Length {
			text = strings.TrimSuffix(text, `"`)
		}
		a.openField(&textField{id: fid, text: text, commit: func(s string) bool {
			if strings.TrimSpace(s) == text {
				return true // untouched: the shown value is rounded, the real one stays
			}
			var f float64
			var err error
			if p.Length {
				f, err = editor.Parse(s, units) // the design's units, or a suffix: 6mm, 1/4"
			} else {
				f, err = strconv.ParseFloat(strings.TrimSpace(s), 64)
			}
			if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				a.showToast("not a number: " + s)
				return true
			}
			if err := a.ed.SetParam(id2, key, f); err != nil {
				a.showToast(err.Error())
			}
			return true
		}})
	}
	a.reg(region{r: render.Rect{X: r.X - 10, Y: r.Y - 4, W: r.W + 20, H: r.H + 8}, id: id, cursor: win.CursorHand,
		press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
			a.ed.BeginEdit()
			set(x)
			return func(x, y float64, up bool) {
				set(x)
				if up {
					a.ed.EndEdit()
				}
			}
		}})
	// registered last so it is on top of the track
	a.reg(region{r: knob.Inset(-3), id: fid, cursor: win.CursorIBeam, tip: "Click to type a value",
		press: func(x0, y0 float64, m win.Mods) func(float64, float64, bool) {
			if typing {
				return nil
			}
			moved := false
			return func(x, y float64, up bool) {
				if !moved && math.Abs(x-x0) > 3 {
					moved = true
					a.ed.BeginEdit()
				}
				if moved {
					set(x)
					if up {
						a.ed.EndEdit()
					}
				} else if up {
					open()
				}
			}
		}})
}

// fmtSlider shows a slider's value: a length in the design's units (inches
// with up to three decimals and a ", mm as fmtParam), anything else as is.
func fmtSlider(v float64, p shapes.Param, units string) string {
	if !p.Length || units != "in" {
		return fmtParam(v, p)
	}
	s := strconv.FormatFloat(v/25.4, 'f', 3, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-0" {
		s = "0"
	}
	return s + `"`
}

// inchStep is the slider step in inches for one of stepMM millimetres: the
// round fraction nearest it.
func inchStep(stepMM float64) float64 {
	want := math.Max(stepMM, 0.01) / 25.4
	best := 0.001
	for _, s := range []float64{0.001, 0.005, 0.01, 1.0 / 64, 1.0 / 32, 1.0 / 16, 1.0 / 8, 1.0 / 4, 1.0 / 2, 1} {
		if math.Abs(math.Log(s/want)) < math.Abs(math.Log(best/want)) {
			best = s
		}
	}
	return best
}

func fmtParam(v float64, p shapes.Param) string {
	if p.Int {
		return strconv.Itoa(int(math.Round(v)))
	}
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-0" {
		s = "0"
	}
	return s
}

// numberField is a text field with stepper arrows.
func (a *App) numberField(id string, r render.Rect, value string, commit func(string) bool, step func(dir float64)) {
	b := a.batch
	active := a.field != nil && a.field.id == id
	b.RoundRect(r, 3, colChrome)
	bc := colFieldBorder
	if active {
		bc = colBlue
	}
	b.RoundRectStroke(r, 3, 1, bc)
	tr := render.Rect{X: r.X, Y: r.Y, W: r.W - 20, H: r.H}
	a.drawFieldText(id, tr, value, fontUI, false, colText)
	a.reg(region{r: tr, id: id, cursor: win.CursorIBeam, press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
		if a.field == nil || a.field.id != id {
			// untouched, the shown (rounded) value isn't written back over the real one
			a.openField(&textField{id: id, text: value, commit: func(s string) bool {
				if strings.TrimSpace(s) == strings.TrimSpace(value) {
					return true
				}
				return commit(s)
			}})
		}
		return nil
	}})
	up := render.Rect{X: r.X + r.W - 20, Y: r.Y + 1, W: 19, H: r.H/2 - 1}
	dn := render.Rect{X: r.X + r.W - 20, Y: r.CY(), W: 19, H: r.H/2 - 1}
	drawIcon(b, icChevronUp, up.Inset(1), colIcon)
	drawIcon(b, icChevronDown, dn.Inset(1), colIcon)
	a.button(id+"-up", up, "", "", 0, func() { step(1) })
	a.button(id+"-dn", dn, "", "", 0, func() { step(-1) })
}

// textInput is a single-line text field with live updates.
func (a *App) textInput(id string, r render.Rect, value string, live func(string), commit func(string) bool, cancel func()) {
	b := a.batch
	active := a.field != nil && a.field.id == id
	b.RoundRect(r, 3, colChrome)
	bc := colFieldBorder
	if active {
		bc = colBlue
	}
	b.RoundRectStroke(r, 3, 1, bc)
	b.PushClip(r)
	a.drawFieldText(id, r, value, fontUI, false, colText)
	b.PopClip()
	a.reg(region{r: r, id: id, cursor: win.CursorIBeam, press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
		if a.field == nil || a.field.id != id {
			a.ed.BeginEdit()
			a.openField(&textField{id: id, text: value, live: live, commit: commit, cancel: cancel})
		}
		return nil
	}})
}

// --- colour picker -------------------------------------------------------------------

func (a *App) drawColorPicker(insp render.Rect, ns []*doc.Node) {
	b := a.batch
	sw := 15.0
	gap := 4.0
	custom := a.colorTab == 1
	w := 12*(sw+gap) - gap + 32
	h := 270.0
	if custom {
		w, h = 512, 412
	}
	r := render.Rect{X: insp.X + insp.W - w, Y: insp.Y + 120, W: w, H: h}
	if r.X < a.vp.X+8 {
		r.X = a.vp.X + 8
	}
	a.catcher(false)
	a.panel(r)
	b.Text("Solid colors", r.X+16, r.Y+14, fontUI, true, colText)
	// Presets | Custom
	for i, l := range []string{"Presets", "Custom"} {
		tr := render.Rect{X: r.X + 16 + float64(i)*78, Y: r.Y + 40, W: 74, H: 24}
		if custom {
			tr = render.Rect{X: r.X + 120 + float64(i)*78, Y: r.Y + 10, W: 74, H: 26}
		}
		on := a.colorTab == i
		if on {
			// the active tab is outlined in blue
			b.RoundRect(tr, 13, colBlueLight)
			b.RoundRectStroke(tr, 13, 1.2, colBlue)
			b.TextCentered(l, tr, fontSmall, false, colBlue)
		} else {
			b.RoundRectStroke(tr, 13, 1, colBorder)
			b.TextCentered(l, tr, fontSmall, false, colText)
		}
		ii := i
		a.button("color-tab-"+l, tr, "", "", 0, func() { a.colorTab = ii })
	}
	cur := solidColor(ns)
	group := len(ns) == 1 && ns[0].IsGroup()
	allT := true
	for _, n := range ns {
		allT = allT && n.Transparent
	}
	multicolor := func(tr render.Rect) {
		if group {
			a.toggle("color-multi", tr, ns[0].Multicolor, func(v bool) { a.ed.SetMulticolor(v) })
		} else {
			b.RoundRect(tr, 12, colHoverBG)
			b.TextCentered("Off", render.Rect{X: tr.X, Y: tr.Y, W: tr.W / 2, H: tr.H}, fontSmall, false, colIconOff)
		}
	}
	mc := colText
	if !group {
		mc = colIconOff
	}
	if custom {
		b.Fill(render.Rect{X: r.X, Y: r.Y + 48, W: r.W, H: 1}, colBorder)
		a.drawCustomColor(render.Rect{X: r.X + 16, Y: r.Y + 62, W: r.W - 32, H: 270}, cur)
		y := r.Y + h - 66
		b.Fill(render.Rect{X: r.X, Y: y - 10, W: r.W, H: 1}, colBorder)
		// Multicolor on the left, Transparent on the right
		b.Text("Multicolor", r.X+16, y, fontUI, false, mc)
		drawRainbow(b, r.X+32, y+38, 10)
		multicolor(render.Rect{X: r.X + 56, Y: y + 26, W: 80, H: 24})
		b.Text("Transparent", r.X+r.W-160, y, fontUI, false, colText)
		b.RoundRect(render.Rect{X: r.X + r.W - 160, Y: y + 26, W: 26, H: 24}, 3, colorOf(cur))
		a.toggle("color-transp", render.Rect{X: r.X + r.W - 112, Y: y + 26, W: 80, H: 24}, allT, func(v bool) { a.ed.SetTransparent(v) })
		return
	}
	y := r.Y + 78
	for row := 0; row < 3; row++ {
		for col := 0; col < 12; col++ {
			hex := presetColors[row][col]
			cx := r.X + 16 + float64(col)*(sw+gap) + sw/2
			cy := y + float64(row)*(sw+gap) + sw/2
			id := "swatch-" + hex
			if strings.EqualFold(hex, cur) || a.isHot(id) {
				b.Ring(cx, cy, sw/2+2.5, 1.5, colBlue)
			}
			b.Circle(cx, cy, sw/2, colorOf(hex))
			if hex == "#ffffff" {
				b.Ring(cx, cy, sw/2, 1, colBorder)
			}
			hx := hex
			a.button(id, rectAround(cx, cy, sw/2+2), "", "", 0, func() { a.ed.SetColor(hx) })
		}
	}
	y += 3*(sw+gap) + 6
	b.Fill(render.Rect{X: r.X + 12, Y: y, W: r.W - 24, H: 1}, colBorder)
	y += 14
	b.Text("Multicolor", r.X+16, y+12-b.LineHeight(fontUI, false)/2, fontUI, false, mc)
	multicolor(render.Rect{X: r.X + r.W - 96, Y: y, W: 80, H: 24})
	y += 38
	b.Text("Transparent", r.X+16, y+12-b.LineHeight(fontUI, false)/2, fontUI, false, colText)
	b.Circle(r.X+r.W-118, y+12, 8, colorOf(cur).A(0.5))
	a.toggle("color-transp", render.Rect{X: r.X + r.W - 96, Y: y, W: 80, H: 24}, allT, func(v bool) { a.ed.SetTransparent(v) })
}
