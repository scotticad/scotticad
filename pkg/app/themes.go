//go:build windows

package app

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"

	"github.com/scotticad/scotticad/pkg/render"
)

// Theme is the app's tint: every colour that carries the theme's hue (the
// accent, the workplane and grid, the selection outline, the size-line
// and protractor bands, the ruler, the logo). Neutral colours (text,
// borders, panels, handles) and colours that mean something regardless
// of theme (holes, the orange rotation and split guides, the red marquee)
// are not part of it.
type Theme struct {
	Name string

	// chrome
	Accent      render.Color // primary buttons, active labels, toggles
	AccentDark  render.Color // a primary button hovered
	AccentLight render.Color // active tool and tab backgrounds
	IconHover   render.Color // toolbar icon hovered or active
	IconAccent  render.Color // toolbar icons' key element
	TabIcon     render.Color // the Workplane / Ruler / Notes tabs
	Busy        render.Color // the "working…" pill
	TextSel     render.Color // a field's text, all selected
	CodeSel     render.Color // a selection in the code editor and console
	TooltipKey  render.Color // a tooltip's shortcut text

	// viewport
	Plane, PlaneEdge, GridMinor, GridMajor, Watermark render.Color
	Select, Hover                                     [3]float32 // shape outlines
	Glow                                              render.Color
	ProtBand, DimBand                                 render.Color
	ProtLine, RulerBlue, RulerTick                    render.Color

	// colours that contrast with the theme's hue: green in the blue
	// theme, blue in the green one
	ShapeWP     render.Color
	BundleChild [3]float32
	RulerBand   render.Color
	RulerArrow  render.Color

	// the logo cube: top, left and right faces, the hole and its bore
	Logo [5]render.Color

	// the untinted colours: light in every theme but the dark one
	N Neutrals
}

// Neutrals are a theme's untinted colours: the surfaces, text, borders
// and icons, the viewport's dark marks, and the code editor's and
// console's colours.
type Neutrals struct {
	BG          render.Color // the default viewport background
	Chrome      render.Color // title bar, toolbar, panels, fields
	Raised      render.Color // an inset block in a panel (Solid / Hole)
	Border      render.Color
	Divider     render.Color
	Text        render.Color
	TextDim     render.Color
	Icon        render.Color
	IconOff     render.Color
	HoverBG     render.Color
	Tooltip     render.Color // tooltips and toasts, under white text
	Banner      render.Color
	FieldBorder render.Color
	Dim         render.Color // a modal dialog's backdrop
	SwatchOff   render.Color // the Hole swatch, not chosen

	// viewport marks drawn in the theme's ink
	Protractor, DimLine, NotePin, NoteBubble render.Color
	HoleEdge                                 [4]float32

	// the code editor and the console
	CodeBG, CodeText, CodeKeyword, CodeString, CodeNumber, CodeComment, CodeName, CodeGutter render.Color
	ConPrint, ConResult, ConError, ConInfo, ConInput                                         render.Color
}

// lightNeutrals are every light theme's.
var lightNeutrals = Neutrals{
	BG:          render.RGB(0xff, 0xff, 0xff),
	Chrome:      render.RGB(0xff, 0xff, 0xff),
	Raised:      render.RGB(0xf7, 0xf8, 0xfa),
	Border:      render.RGB(0xe1, 0xe5, 0xea),
	Divider:     render.RGB(0xd5, 0xda, 0xe0),
	Text:        render.RGB(0x33, 0x3c, 0x47),
	TextDim:     render.RGB(0x8a, 0x94, 0x9e),
	Icon:        render.RGB(0x6f, 0x7a, 0x86),
	IconOff:     render.RGB(0xcf, 0xd5, 0xdb),
	HoverBG:     render.RGB(0xf0, 0xf3, 0xf6),
	Tooltip:     render.RGB(0x23, 0x2f, 0x40),
	Banner:      render.RGB(0xee, 0xf0, 0xf2),
	FieldBorder: render.RGB(0xc8, 0xcf, 0xd6),
	Dim:         render.Color{0, 0, 0, 0.35},
	SwatchOff:   render.RGB(0xc4, 0xcb, 0xd3),

	Protractor: render.RGB(0x55, 0x60, 0x6b),
	DimLine:    render.RGB(0x2b, 0x33, 0x3b),
	NotePin:    render.RGB(0x2b, 0x33, 0x3b),
	NoteBubble: render.RGB(0x8a, 0x93, 0x9c),
	HoleEdge:   [4]float32{0.27, 0.31, 0.36, 0.85},

	CodeBG:      render.RGB(0xfb, 0xfc, 0xfd),
	CodeText:    render.RGB(0x24, 0x29, 0x2e),
	CodeKeyword: render.RGB(0xa6, 0x26, 0xa4),
	CodeString:  render.RGB(0x50, 0x8a, 0x2e),
	CodeNumber:  render.RGB(0xb7, 0x6b, 0x01),
	CodeComment: render.RGB(0x8a, 0x94, 0x9e),
	CodeName:    render.RGB(0x2f, 0x6f, 0xc6),
	CodeGutter:  render.RGB(0xa8, 0xb0, 0xb8),
	ConPrint:    render.RGB(0x24, 0x29, 0x2e),
	ConResult:   render.RGB(0x2f, 0x6f, 0xc6),
	ConError:    render.RGB(0xc6, 0x28, 0x28),
	ConInfo:     render.RGB(0x8a, 0x94, 0x9e),
	ConInput:    render.RGB(0x5b, 0x66, 0x72),
}

// darkNeutrals are the dark theme's: slate greys, light text, and the
// viewport's marks in light ink.
var darkNeutrals = Neutrals{
	BG:          render.RGB(0x1c, 0x20, 0x26),
	Chrome:      render.RGB(0x25, 0x29, 0x30),
	Raised:      render.RGB(0x2d, 0x32, 0x3a),
	Border:      render.RGB(0x36, 0x3c, 0x45),
	Divider:     render.RGB(0x40, 0x47, 0x51),
	Text:        render.RGB(0xe9, 0xed, 0xf1),
	TextDim:     render.RGB(0xad, 0xb6, 0xc0), // small light-on-dark text renders thin: not too dim
	Icon:        render.RGB(0xa4, 0xad, 0xb8),
	IconOff:     render.RGB(0x4c, 0x53, 0x5d),
	HoverBG:     render.RGB(0x31, 0x37, 0x40),
	Tooltip:     render.RGB(0x0d, 0x10, 0x14),
	Banner:      render.RGB(0x2d, 0x32, 0x3a),
	FieldBorder: render.RGB(0x4a, 0x51, 0x5b),
	Dim:         render.Color{0, 0, 0, 0.5},
	SwatchOff:   render.RGB(0x5a, 0x62, 0x6c),

	Protractor: render.RGB(0xb4, 0xbc, 0xc6),
	DimLine:    render.RGB(0xdd, 0xe2, 0xe8),
	NotePin:    render.RGB(0xdd, 0xe2, 0xe8),
	NoteBubble: render.RGB(0x9a, 0xa3, 0xad),
	HoleEdge:   [4]float32{0.78, 0.81, 0.85, 0.85},

	CodeBG:      render.RGB(0x1e, 0x22, 0x28),
	CodeText:    render.RGB(0xd4, 0xd9, 0xdf),
	CodeKeyword: render.RGB(0xc6, 0x78, 0xdd),
	CodeString:  render.RGB(0x98, 0xc3, 0x79),
	CodeNumber:  render.RGB(0xd1, 0x9a, 0x66),
	CodeComment: render.RGB(0x7f, 0x88, 0x93),
	CodeName:    render.RGB(0x61, 0xaf, 0xef),
	CodeGutter:  render.RGB(0x5c, 0x63, 0x70),
	ConPrint:    render.RGB(0xd4, 0xd9, 0xdf),
	ConResult:   render.RGB(0x61, 0xaf, 0xef),
	ConError:    render.RGB(0xf0, 0x6c, 0x6c),
	ConInfo:     render.RGB(0x8b, 0x95, 0xa0),
	ConInput:    render.RGB(0xa4, 0xad, 0xb8),
}

// blueTheme matches the package's colour defaults (theme.go).
var blueTheme = Theme{
	Name:        "Blue",
	Accent:      render.RGB(0x2f, 0x7f, 0xe6),
	AccentDark:  render.RGB(0x1d, 0x6b, 0xd0),
	AccentLight: render.RGB(0xe6, 0xf1, 0xfd),
	IconHover:   render.RGB(0x1f, 0x7a, 0xe0),
	IconAccent:  render.RGB(0x3c, 0x95, 0xe0),
	TabIcon:     render.RGB(0x2b, 0x9c, 0xd8),
	Busy:        render.RGB(0x1b, 0x63, 0xb8),
	TextSel:     render.RGB(0x3d, 0x8e, 0xf0),
	CodeSel:     render.Color{0.24, 0.56, 0.94, 0.25},
	TooltipKey:  render.RGB(0x9f, 0xe3, 0xc6),

	Plane:     render.Color{0.80, 0.90, 0.97, 0.88},
	PlaneEdge: render.Color{0.60, 0.80, 0.94, 0.75},
	GridMinor: render.Color{0.62, 0.79, 0.91, 0.55},
	GridMajor: render.Color{0.45, 0.68, 0.87, 0.85},
	Watermark: render.Color{0.53, 0.76, 0.92, 0.55},
	Select:    [3]float32{0.09, 0.78, 1.0},
	Hover:     [3]float32{0.45, 0.86, 1.0},
	Glow:      render.Color{0.25, 0.80, 1.0, 1},
	ProtBand:  render.Color{0.42, 0.72, 0.89, 0.45},
	DimBand:   render.Color{0.55, 0.78, 0.93, 0.55},
	ProtLine:  render.RGB(0x3a, 0x9d, 0xdc),
	RulerBlue: render.RGB(0x2f, 0x5f, 0x9f),
	RulerTick: render.RGB(0x3a, 0x9d, 0xdc),

	ShapeWP:     render.Color{0.55, 0.85, 0.45, 0.35},
	BundleChild: [3]float32{0.24, 0.82, 0.35},
	RulerBand:   render.Color{0.45, 0.85, 0.35, 0.45},
	RulerArrow:  render.Color{0.22, 0.55, 0.20, 0.9},

	Logo: [5]render.Color{
		render.RGB(0x6c, 0xc4, 0xf0), render.RGB(0x2e, 0x86, 0xc8), render.RGB(0x1b, 0x55, 0x8c),
		render.RGB(0x0e, 0x2a, 0x47), render.RGB(0x16, 0x3d, 0x63),
	},

	N: lightNeutrals,
}

// darkTheme is dark slate chrome and a dark workplane, with the blue
// theme's accent lifted a little to stand out on it. The selection,
// glow and contrasting colours are the blue theme's: bright already.
var darkTheme = func() Theme {
	t := blueTheme
	t.Name = "Dark"
	t.N = darkNeutrals
	// as light as white text on it allows (3:1): it's also the active
	// labels' colour, small text on the dark panels
	t.Accent = render.RGB(0x4a, 0x94, 0xf0)
	t.AccentDark = render.RGB(0x4a, 0x93, 0xf0) // hovered: lighter, on dark
	t.AccentLight = render.RGB(0x1f, 0x35, 0x50)
	t.IconHover = render.RGB(0x6c, 0xb4, 0xff)
	t.IconAccent = render.RGB(0x4c, 0xa2, 0xf0)
	t.TabIcon = render.RGB(0x52, 0xb4, 0xea)
	t.TextSel = render.RGB(0x2f, 0x7f, 0xe6)
	t.CodeSel = render.Color{0.30, 0.56, 0.94, 0.35}

	t.Plane = render.Color{0.17, 0.21, 0.27, 0.92}
	t.PlaneEdge = render.Color{0.30, 0.44, 0.60, 0.8}
	t.GridMinor = render.Color{0.24, 0.30, 0.38, 0.6}
	t.GridMajor = render.Color{0.32, 0.45, 0.60, 0.85}
	t.Watermark = render.Color{0.30, 0.40, 0.52, 0.5}
	t.ProtBand = render.Color{0.25, 0.45, 0.65, 0.45}
	t.DimBand = render.Color{0.25, 0.45, 0.65, 0.55}
	t.ProtLine = render.RGB(0x52, 0xb4, 0xea)
	t.RulerBlue = render.RGB(0x8a, 0xbd, 0xf2)
	t.RulerTick = render.RGB(0x52, 0xb4, 0xea)
	return t
}()

// greenTheme is the blue theme turned green: each tinted colour's hue
// rotated by greenShift degrees, its lightness and saturation kept, so the
// look stays as light and pastel. The colours that contrast with the
// theme's hue (the shape workplane, bundled shapes, the ruler's distance
// band) go blue in turn, so they still stand out.
var greenTheme = func() Theme {
	t := blueTheme.Shifted("Green", greenShift, greenViewSat)
	t.contrastBlue()
	return t
}()

// orangeTheme turns the blues to orange. Its selection outline is amber,
// not the shifted cyan (a red-orange that barely shows on a red shape),
// and the contrasting colours go blue.
var orangeTheme = func() Theme {
	t := blueTheme.Shifted("Orange", orangeShift, orangeViewSat)
	// a workplane at the chrome's hue looks salmon; a yellower one, peach
	v := blueTheme.Shifted("", orangeViewShift, orangeViewSat)
	t.Plane, t.PlaneEdge, t.GridMinor, t.GridMajor = v.Plane, v.PlaneEdge, v.GridMinor, v.GridMajor
	t.Watermark, t.ProtBand, t.DimBand = v.Watermark, v.ProtBand, v.DimBand
	t.Select = [3]float32{1.0, 0.66, 0.0}
	t.Hover = [3]float32{1.0, 0.80, 0.40}
	t.Glow = render.Color{1.0, 0.70, 0.15, 1}
	// an orange as dark as the blue icon accent is rust; icons carry no
	// text, so theirs can be brighter (and hover, darker, still shows)
	t.IconAccent = render.RGB(0xf0, 0x86, 0x1c)
	t.contrastBlue()
	return t
}()

// purpleTheme turns the blues to violet. Its selection outline is a
// lighter violet than the shifted cyan, which is too dark to show on a
// red shape; the contrasting colours stay green.
var purpleTheme = func() Theme {
	t := blueTheme.Shifted("Purple", purpleShift, purpleViewSat)
	t.Select = [3]float32{0.74, 0.45, 1.0}
	t.Hover = [3]float32{0.85, 0.70, 1.0}
	t.Glow = render.Color{0.78, 0.55, 1.0, 1}
	t.ShapeWP = blueTheme.ShapeWP
	t.BundleChild = blueTheme.BundleChild
	t.RulerBand, t.RulerArrow = blueTheme.RulerBand, blueTheme.RulerArrow
	return t
}()

// tanTheme is warm and quiet: cream panels instead of white, warm greys
// for text and borders, caramel buttons and a sand build plate. Its
// selection outline is cyan, the one bright colour, to stand out on the
// tan and on the warm colours shapes usually are.
var tanTheme = func() Theme {
	t := blueTheme.Shifted("Tan", tanShift, tanViewSat)
	t.Accent = render.RGB(0xa0, 0x70, 0x3f)
	t.AccentDark = render.RGB(0x86, 0x5c, 0x31)
	t.AccentLight = render.RGB(0xf0, 0xe4, 0xd2)
	t.IconHover = render.RGB(0x86, 0x5c, 0x31)
	t.IconAccent = render.RGB(0xc0, 0x8b, 0x52)
	t.TabIcon = render.RGB(0xb0, 0x7e, 0x47)
	t.Busy = render.RGB(0x7a, 0x52, 0x2a)
	t.TextSel = render.RGB(0xb8, 0x86, 0x50)
	t.CodeSel = render.Color{0.72, 0.53, 0.31, 0.25}
	// the blue theme's cyan selection and its green contrasting colours:
	// apart from each other, and bright on tan
	b := blueTheme
	t.Select, t.Hover, t.Glow = b.Select, b.Hover, b.Glow
	t.ShapeWP, t.BundleChild, t.RulerBand, t.RulerArrow = b.ShapeWP, b.BundleChild, b.RulerBand, b.RulerArrow
	t.N = lightNeutrals
	t.N.BG = render.RGB(0xfa, 0xf6, 0xef)
	t.N.Chrome = render.RGB(0xfd, 0xfa, 0xf5)
	t.N.Raised = render.RGB(0xf4, 0xee, 0xe4)
	t.N.Border = render.RGB(0xe6, 0xde, 0xd1)
	t.N.Divider = render.RGB(0xdb, 0xd1, 0xc1)
	t.N.Text = render.RGB(0x3d, 0x35, 0x2c)
	t.N.TextDim = render.RGB(0x8c, 0x80, 0x72)
	t.N.Icon = render.RGB(0x78, 0x6c, 0x5f)
	t.N.IconOff = render.RGB(0xd5, 0xcc, 0xbe)
	t.N.HoverBG = render.RGB(0xf2, 0xeb, 0xdf)
	t.N.Tooltip = render.RGB(0x3a, 0x2f, 0x24)
	t.N.Banner = render.RGB(0xf0, 0xe9, 0xdd)
	t.N.FieldBorder = render.RGB(0xcd, 0xc2, 0xb2)
	t.N.SwatchOff = render.RGB(0xcc, 0xc2, 0xb4)
	t.N.CodeBG = render.RGB(0xfc, 0xf9, 0xf4)
	return t
}()

// Each theme's hue turn from the blues (about 205°), and how much of the
// blue's saturation its workplane, grid and bands keep: other hues look
// more vivid than blue at the same saturation (a mint or salmon
// workplane), so they're toned down to look as calm.
const (
	greenShift, greenViewSat   = -50, 0.45  // to emerald, about 155°
	orangeShift, orangeViewSat = -185, 0.65 // to orange, about 25° (less is mustard)
	orangeViewShift            = -172
	purpleShift, purpleViewSat = 55, 0.75  // to violet, about 260°
	tanShift, tanViewSat       = -172, 0.4 // to sand, about 33°, and greyer than orange
)

// contrastBlue makes the colours that must stand out from the theme's hue
// (the shape workplane, bundled shapes, the ruler's distance band) blue.
func (t *Theme) contrastBlue() {
	t.ShapeWP = render.Color{0.45, 0.68, 0.95, 0.35}
	t.BundleChild = [3]float32{0.26, 0.52, 1.0}
	t.RulerBand = render.Color{0.42, 0.66, 0.93, 0.45}
	t.RulerArrow = render.Color{0.16, 0.36, 0.66, 0.9}
}

// themes are the themes the Settings dialog offers, in its order.
var themes = []Theme{blueTheme, greenTheme, orangeTheme, purpleTheme, tanTheme, darkTheme}

// Shifted is t with every tinted colour's hue turned by deg degrees, and
// its lightness then set so it looks as bright as before (see
// matchLuminance): a green as light as a blue by the numbers looks far
// brighter, and white text on it hard to read. The workplane, its grid
// and watermark, and the protractor and size-line bands also keep only
// viewSat of their saturation.
func (t Theme) Shifted(name string, deg, viewSat float64) Theme {
	c := func(x render.Color) render.Color { return matchLuminance(hueShift(x, deg), x) }
	c3 := func(x [3]float32) [3]float32 {
		y := c(render.Color{x[0], x[1], x[2], 1})
		return [3]float32{y[0], y[1], y[2]}
	}
	t.Name = name
	for _, p := range []*render.Color{&t.Accent, &t.AccentDark, &t.AccentLight, &t.IconHover, &t.IconAccent,
		&t.TabIcon, &t.Busy, &t.TextSel, &t.CodeSel, &t.Glow, &t.ProtLine, &t.RulerBlue, &t.RulerTick,
		&t.ShapeWP, &t.RulerBand, &t.RulerArrow} {
		*p = c(*p)
	}
	for _, p := range []*render.Color{&t.Plane, &t.PlaneEdge, &t.GridMinor, &t.GridMajor,
		&t.Watermark, &t.ProtBand, &t.DimBand} {
		h, s, l := rgbToHSL(hueShift(*p, deg))
		*p = matchLuminance(hslToRGB(h, s*viewSat, l, (*p)[3]), *p)
	}
	for i := range t.Logo {
		t.Logo[i] = c(t.Logo[i])
	}
	t.Select, t.Hover, t.BundleChild = c3(t.Select), c3(t.Hover), c3(t.BundleChild)
	return t
}

// hueShift turns c's hue by deg degrees, keeping its lightness, saturation
// and alpha.
func hueShift(c render.Color, deg float64) render.Color {
	h, s, l := rgbToHSL(c)
	if s == 0 {
		return c // grey: no hue to turn
	}
	return hslToRGB(math.Mod(h+deg+720, 360), s, l, c[3])
}

// luminance is c's relative luminance (WCAG): how bright it looks.
func luminance(c render.Color) float64 {
	lin := func(v float32) float64 {
		x := float64(v)
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c[0]) + 0.7152*lin(c[1]) + 0.0722*lin(c[2])
}

// matchLuminance is c with its HSL lightness moved until it looks as
// bright as like, keeping its hue and saturation.
func matchLuminance(c, like render.Color) render.Color {
	want := luminance(like)
	h, s, _ := rgbToHSL(c)
	lo, hi := 0.0, 1.0
	for i := 0; i < 40; i++ { // luminance rises with lightness
		mid := (lo + hi) / 2
		if luminance(hslToRGB(h, s, mid, c[3])) < want {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hslToRGB(h, s, (lo+hi)/2, c[3])
}

func rgbToHSL(c render.Color) (h, s, l float64) {
	r, g, b := float64(c[0]), float64(c[1]), float64(c[2])
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	s = d / (1 - math.Abs(2*l-1))
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return math.Mod(h*60+360, 360), s, l
}

func hslToRGB(h, s, l float64, a float32) render.Color {
	ch := (1 - math.Abs(2*l-1)) * s
	x := ch * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - ch/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = ch, x, 0
	case h < 120:
		r, g, b = x, ch, 0
	case h < 180:
		r, g, b = 0, ch, x
	case h < 240:
		r, g, b = 0, x, ch
	case h < 300:
		r, g, b = x, 0, ch
	default:
		r, g, b = ch, 0, x
	}
	return render.Color{float32(r + m), float32(g + m), float32(b + m), a}
}

// currentTheme is the theme in use.
var currentTheme = blueTheme

// applyTheme makes t the theme in use: the package's colours take its.
func applyTheme(t Theme) {
	currentTheme = t
	colBlue, colBlueDark, colBlueLight = t.Accent, t.AccentDark, t.AccentLight
	colIconHover, colIconAccent, colTabIcon = t.IconHover, t.IconAccent, t.TabIcon
	colBusy, colTextSel, colCodeSel, colTooltipKey = t.Busy, t.TextSel, t.CodeSel, t.TooltipKey
	colPlane, colPlaneEdge, colGridMinor, colGridMajor, colWatermark = t.Plane, t.PlaneEdge, t.GridMinor, t.GridMajor, t.Watermark
	colSelect, colHover, colGlow = t.Select, t.Hover, t.Glow
	colProtBand, colDimBand = t.ProtBand, t.DimBand
	colProtLine, colRulerBlue, colRulerTick = t.ProtLine, t.RulerBlue, t.RulerTick
	colShapeWP, colBundleChild, colRulerBand, colRulerArrow = t.ShapeWP, t.BundleChild, t.RulerBand, t.RulerArrow
	logoTop, logoLeft, logoRight, logoHole, logoBore = t.Logo[0], t.Logo[1], t.Logo[2], t.Logo[3], t.Logo[4]
	n := t.N
	colBG, colChrome, colRaised, colBorder, colDivider = n.BG, n.Chrome, n.Raised, n.Border, n.Divider
	colText, colTextDim, colIcon, colIconOff, colHoverBG = n.Text, n.TextDim, n.Icon, n.IconOff, n.HoverBG
	colTooltip, colBanner, colFieldBorder, colDim, colSwatchOff = n.Tooltip, n.Banner, n.FieldBorder, n.Dim, n.SwatchOff
	colProtractor, colDimLine, colNotePin, colNoteBubble, colHoleEdge = n.Protractor, n.DimLine, n.NotePin, n.NoteBubble, n.HoleEdge
	colCodeBG, colCodeText, colCodeKeyword, colCodeString = n.CodeBG, n.CodeText, n.CodeKeyword, n.CodeString
	colCodeNumber, colCodeComment, colCodeName, colCodeGutter = n.CodeNumber, n.CodeComment, n.CodeName, n.CodeGutter
	colConPrint, colConResult, colConError, colConInfo, colConInput = n.ConPrint, n.ConResult, n.ConError, n.ConInfo, n.ConInput
}

// defaultTheme is the theme until the user chooses one.
var defaultTheme = tanTheme

// themeNamed is the theme with that name, or the default one.
func themeNamed(name string) Theme {
	for _, t := range themes {
		if t.Name == name {
			return t
		}
	}
	return defaultTheme
}

// The theme is a per-user preference, kept beside the filament settings
// (not in designs: a design looks the same in any theme).
type themePrefs struct {
	Theme string `json:"theme"`
}

func readThemePref(p string) string {
	var tp themePrefs
	if b, err := os.ReadFile(p); err == nil {
		json.Unmarshal(b, &tp)
	}
	return tp.Theme
}

func writeThemePref(p, name string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(themePrefs{Theme: name}, "", " ")
	return os.WriteFile(p, b, 0o644)
}

// loadTheme applies the theme the user chose last, or the default one.
func (a *App) loadTheme() {
	name := ""
	if p := prefsPath("theme.json"); p != "" {
		name = readThemePref(p)
	}
	applyTheme(themeNamed(name))
	a.bgColor = colBG
}

// setTheme switches to t and remembers it. The viewport background goes
// with it, unless the user chose their own.
func (a *App) setTheme(t Theme) {
	if a.bgColor == currentTheme.N.BG {
		a.bgColor = t.N.BG
	}
	applyTheme(t)
	if p := prefsPath("theme.json"); p != "" {
		if err := writeThemePref(p, t.Name); err != nil {
			a.showToast("couldn't save the theme: " + err.Error())
		}
	}
	a.w.Redraw()
}
