//go:build windows

package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/render"
	"github.com/scotticad/scotticad/pkg/win"
)

// The Custom tab of the colour picker: a hue ring around a saturation /
// brightness square, RGB or HSB sliders with number fields, and the hex
// code with a Copy link.

// --- colour maths ----------------------------------------------------------------------

func hexRGB(hex string) (r, g, b float64) {
	c := geom.ParseColor(hex)
	return float64(c.R), float64(c.G), float64(c.B)
}

func rgbHex(r, g, b float64) string {
	cl := func(v float64) int { return int(math.Round(math.Max(0, math.Min(1, v)) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", cl(r), cl(g), cl(b))
}

// rgbHSV returns hue in degrees, saturation and value in 0..1.
func rgbHSV(r, g, b float64) (h, s, v float64) {
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	v = mx
	d := mx - mn
	if mx > 0 {
		s = d / mx
	}
	if d == 0 {
		return 0, s, v
	}
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return
}

func hsvRGB(h, s, v float64) (r, g, b float64) {
	h = math.Mod(math.Mod(h, 360)+360, 360) / 60
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h, 2)-1))
	var r1, g1, b1 float64
	switch int(h) {
	case 0:
		r1, g1, b1 = c, x, 0
	case 1:
		r1, g1, b1 = x, c, 0
	case 2:
		r1, g1, b1 = 0, c, x
	case 3:
		r1, g1, b1 = 0, x, c
	case 4:
		r1, g1, b1 = x, 0, c
	default:
		r1, g1, b1 = c, 0, x
	}
	m := v - c
	return r1 + m, g1 + m, b1 + m
}

func hsvColor(h, s, v float64) render.Color {
	r, g, b := hsvRGB(h, s, v)
	return render.Color{float32(r), float32(g), float32(b), 1}
}

// --- picker state --------------------------------------------------------------------------

// pickerHSV is the HSV being edited. It is kept separately from the hex so
// the hue doesn't jump while saturation or brightness is zero.
func (a *App) pickerHSV(cur string) (h, s, v float64) {
	if !strings.EqualFold(a.pickHex, cur) {
		a.pickH, a.pickS, a.pickV = rgbHSV(hexRGB(cur))
		a.pickHex = strings.ToLower(cur)
	}
	return a.pickH, a.pickS, a.pickV
}

// setPickerHSV applies an HSV colour; live colours update without an undo
// step (inside a drag).
func (a *App) setPickerHSV(h, s, v float64, live bool) {
	a.pickH, a.pickS, a.pickV = h, math.Max(0, math.Min(1, s)), math.Max(0, math.Min(1, v))
	hex := rgbHex(hsvRGB(a.pickH, a.pickS, a.pickV))
	a.pickHex = hex
	if a.pickTarget != nil {
		a.pickTarget(hex, live)
		return
	}
	if live {
		a.ed.SetColorLive(hex)
	} else if err := a.ed.SetColor(hex); err != nil {
		a.showToast(err.Error())
	}
}

func (a *App) setPickerHex(hex string) {
	if _, ok := geom.TryParseColor(hex); !ok {
		a.showToast("not a colour: " + hex)
		return
	}
	a.pickHex = "" // re-read HSV from the new colour
	if a.pickTarget != nil {
		a.pickTarget(strings.ToLower(hex), false)
		return
	}
	if err := a.ed.SetColor(strings.ToLower(hex)); err != nil {
		a.showToast(err.Error())
	}
}

// liveDrag wraps a drag that changes colour continuously into one undo step.
func (a *App) liveDrag(apply func(x, y float64)) func(x, y float64, m win.Mods) func(float64, float64, bool) {
	return func(x, y float64, m win.Mods) func(float64, float64, bool) {
		if a.pickTarget != nil { // not the design: no undo step
			apply(x, y)
			return func(x, y float64, up bool) { apply(x, y) }
		}
		a.ed.BeginEdit()
		apply(x, y)
		return func(x, y float64, up bool) {
			apply(x, y)
			if up {
				a.ed.EndEdit()
			}
		}
	}
}

// --- drawing --------------------------------------------------------------------------------

// drawCustomColor draws the Custom tab into r (its top part).
func (a *App) drawCustomColor(r render.Rect, cur string) {
	b := a.batch
	h, s, v := a.pickerHSV(cur)

	// current colour
	b.Circle(r.X+16, r.Y+16, 15, colorOf(cur))

	// the hue ring and the saturation/brightness square inside it
	cx, cy := r.X+100, r.Y+110
	const ro, ri = 100.0, 80.0
	ang := func(hue float64) float64 { return (hue - 90) * math.Pi / 180 } // red at the top, clockwise
	const segs = 120
	for i := 0; i < segs; i++ {
		h0, h1 := 360*float64(i)/segs, 360*float64(i+1)/segs+0.6
		t0, t1 := ang(h0), ang(h1)
		b.Quad(cx+ri*math.Cos(t0), cy+ri*math.Sin(t0), cx+ro*math.Cos(t0), cy+ro*math.Sin(t0),
			cx+ro*math.Cos(t1), cy+ro*math.Sin(t1), cx+ri*math.Cos(t1), cy+ri*math.Sin(t1), hsvColor((h0+h1)/2, 1, 1))
	}
	side := ri * 1.38
	sq := render.Rect{X: cx - side/2, Y: cy - side/2, W: side, H: side}
	const cells = 24
	cw := side / cells
	for i := 0; i < cells; i++ {
		for j := 0; j < cells; j++ {
			sx, vy := (float64(i)+0.5)/cells, 1-(float64(j)+0.5)/cells
			b.Fill(render.Rect{X: sq.X + float64(i)*cw, Y: sq.Y + float64(j)*cw, W: cw + 0.6, H: cw + 0.6}, hsvColor(h, sx, vy))
		}
	}
	// markers
	mt := ang(h)
	mx, my := cx+(ro+ri)/2*math.Cos(mt), cy+(ro+ri)/2*math.Sin(mt)
	b.Circle(mx, my, 10, colOnColor)
	b.Circle(mx, my, 8, hsvColor(h, 1, 1))
	b.Ring(mx, my, 10, 1, colBorder)
	px, py := sq.X+s*side, sq.Y+(1-v)*side
	b.Circle(px, py, 10, colOnColor)
	b.Circle(px, py, 8, colorOf(cur))
	b.Ring(px, py, 10, 1, colBorder)
	a.reg(region{r: rectAround(cx, cy, ro), id: "color-wheel", cursor: win.CursorHand,
		press: func(x, y float64, m win.Mods) func(float64, float64, bool) {
			onRing := math.Hypot(x-cx, y-cy) > ri-4
			return a.liveDrag(func(x, y float64) {
				if onRing {
					hue := math.Atan2(y-cy, x-cx)*180/math.Pi + 90
					a.setPickerHSV(math.Mod(hue+360, 360), a.pickS, a.pickV, true)
				} else {
					a.setPickerHSV(a.pickH, (x-sq.X)/side, 1-(y-sq.Y)/side, true)
				}
			})(x, y, m)
		}})

	// RGB | HSB
	rx := r.X + 222
	for i, l := range []string{"RGB", "HSB"} {
		tr := render.Rect{X: rx + float64(i)*52, Y: r.Y + 2, W: 52, H: 28}
		on := a.colorMode == i
		if on {
			b.RoundRect(tr, 14, colBlueLight)
			b.RoundRectStroke(tr, 14, 1.2, colBlue)
			b.TextCentered(l, tr, fontUI, false, colBlue)
		} else {
			b.RoundRectStroke(tr, 14, 1, colBorder)
			b.TextCentered(l, tr, fontUI, false, colText)
		}
		ii := i
		a.button("color-mode-"+l, tr, "", "", 0, func() { a.colorMode = ii })
	}

	// three sliders with number fields
	type chan_ struct {
		label string
		val   float64 // in its own units
		max   float64
		color func(t float64) render.Color // the track at 0..1
		set   func(t float64, live bool)   // t in 0..1
	}
	rr, gg, bb := hexRGB(cur)
	var chans []chan_
	if a.colorMode == 0 {
		mk := func(label string, val float64, col func(t float64) (float64, float64, float64)) chan_ {
			return chan_{label, math.Round(val * 255), 255,
				func(t float64) render.Color {
					r1, g1, b1 := col(t)
					return render.Color{float32(r1), float32(g1), float32(b1), 1}
				},
				func(t float64, live bool) {
					r1, g1, b1 := col(t)
					hh, ss, vv := rgbHSV(r1, g1, b1)
					if ss == 0 || vv == 0 {
						hh = a.pickH
					}
					a.setPickerHSV(hh, ss, vv, live)
				}}
		}
		chans = []chan_{
			mk("Red", rr, func(t float64) (float64, float64, float64) { return t, gg, bb }),
			mk("Green", gg, func(t float64) (float64, float64, float64) { return rr, t, bb }),
			mk("Blue", bb, func(t float64) (float64, float64, float64) { return rr, gg, t }),
		}
	} else {
		chans = []chan_{
			{"Hue", math.Round(h), 360, func(t float64) render.Color { return hsvColor(t*360, math.Max(s, 0.6), math.Max(v, 0.6)) },
				func(t float64, live bool) { a.setPickerHSV(t*359.999, a.pickS, a.pickV, live) }},
			{"Saturation", math.Round(s * 100), 100, func(t float64) render.Color { return hsvColor(h, t, v) },
				func(t float64, live bool) { a.setPickerHSV(a.pickH, t, a.pickV, live) }},
			{"Brightness", math.Round(v * 100), 100, func(t float64) render.Color { return hsvColor(h, s, t) },
				func(t float64, live bool) { a.setPickerHSV(a.pickH, a.pickS, t, live) }},
		}
	}
	for i, c := range chans {
		y := r.Y + 48 + float64(i)*54
		b.Text(c.label, rx, y, fontUI, false, colText)
		tr := render.Rect{X: rx + 2, Y: y + 24, W: 196, H: 16}
		const n = 32
		for k := 0; k < n; k++ {
			seg := render.Rect{X: tr.X + tr.W*float64(k)/n, Y: tr.Y, W: tr.W/n + 0.6, H: tr.H}
			b.Fill(seg, c.color((float64(k)+0.5)/n))
		}
		// round the ends
		b.Circle(tr.X, tr.CY(), tr.H/2, c.color(0))
		b.Circle(tr.X+tr.W, tr.CY(), tr.H/2, c.color(1))
		t := c.val / c.max
		kx := tr.X + t*tr.W
		b.Circle(kx, tr.CY(), 12, colOnColor)
		b.Circle(kx, tr.CY(), 10, colorOf(cur))
		b.Ring(kx, tr.CY(), 12, 1, colBorder)
		set := c.set
		a.reg(region{r: render.Rect{X: tr.X - 10, Y: tr.Y - 8, W: tr.W + 20, H: tr.H + 16}, id: "color-slider-" + strings.ToLower(c.label), cursor: win.CursorHand,
			press: a.liveDrag(func(x, y float64) { set(math.Max(0, math.Min(1, (x-tr.X)/tr.W)), true) })})
		// the number
		fid := "color-num-" + strings.ToLower(c.label)
		fr := render.Rect{X: rx + 214, Y: tr.CY() - 13, W: 54, H: 26}
		b.RoundRect(fr, 13, colChrome)
		b.RoundRectStroke(fr, 13, 1, colFieldBorder)
		txt := strconv.Itoa(int(c.val))
		a.drawFieldText(fid, fr, txt, fontUI, true, colText)
		mx := c.max
		a.reg(region{r: fr, id: fid, cursor: win.CursorIBeam, press: func(float64, float64, win.Mods) func(float64, float64, bool) {
			a.openField(&textField{id: fid, text: txt, commit: func(s string) bool {
				f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
				if err != nil {
					a.showToast("not a number: " + s)
					return true
				}
				set(math.Max(0, math.Min(mx, f))/mx, false)
				return true
			}})
			return nil
		}})
	}

	// Hex # — Copy — field
	y := r.Y + 48 + 3*54 + 6
	b.Text("Hex #", rx, y, fontUI, false, colText)
	hex := strings.TrimPrefix(strings.ToLower(cur), "#")
	cpr := render.Rect{X: rx + 128, Y: y - 4, W: 40, H: 24}
	ccol := colBlue
	if a.isHot("color-copy") {
		ccol = colIconHover
	}
	b.TextCentered("Copy", cpr, fontUI, false, ccol)
	a.button("color-copy", cpr, "Copy the colour code", "", tipBelow, func() {
		a.w.SetClipboard(hex)
		a.showToast("Copied " + hex)
	})
	hr := render.Rect{X: rx + 180, Y: y - 6, W: 88, H: 28}
	b.RoundRect(hr, 14, colChrome)
	b.RoundRectStroke(hr, 14, 1, colFieldBorder)
	a.drawFieldText("color-hex", hr, hex, fontUI, true, colText)
	a.reg(region{r: hr, id: "color-hex", cursor: win.CursorIBeam, press: func(float64, float64, win.Mods) func(float64, float64, bool) {
		a.openField(&textField{id: "color-hex", text: hex, commit: func(s string) bool {
			s = strings.TrimPrefix(strings.TrimSpace(s), "#")
			a.setPickerHex("#" + s)
			return true
		}})
		return nil
	}})
}
