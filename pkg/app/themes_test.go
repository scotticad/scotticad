//go:build windows

package app

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/scotticad/scotticad/pkg/render"
)

// hsl is c's hue (degrees), saturation and lightness.
func hsl(c render.Color) (h, s, l float64) {
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

// The blue theme is the package's colour defaults: applying it changes
// nothing.
func TestBlueThemeIsTheDefault(t *testing.T) {
	cols := func() []render.Color {
		return []render.Color{colBlue, colBlueDark, colBlueLight, colIconHover, colIconAccent, colTabIcon,
			colBusy, colTextSel, colCodeSel, colPlane, colPlaneEdge, colGridMinor, colGridMajor, colWatermark,
			colGlow, colProtBand, colDimBand, colProtLine, colRulerBlue, colRulerTick, colShapeWP, colRulerBand,
			colRulerArrow, logoTop, logoLeft, logoRight, logoHole, logoBore,
			colBG, colChrome, colRaised, colBorder, colDivider, colText, colTextDim, colIcon, colIconOff,
			colHoverBG, colTooltip, colBanner, colFieldBorder, colDim, colSwatchOff, colProtractor, colDimLine,
			colNotePin, colNoteBubble, colHoleEdge, colCodeBG, colCodeText, colCodeKeyword, colCodeString,
			colCodeNumber, colCodeComment, colCodeName, colCodeGutter, colConPrint, colConResult, colConError,
			colConInfo, colConInput}
	}
	before := cols()
	sel, hov, bun := colSelect, colHover, colBundleChild
	defer applyTheme(currentTheme)
	applyTheme(blueTheme)
	after := cols()
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("colour %d: %v by default, %v in the blue theme", i, before[i], after[i])
		}
	}
	if sel != colSelect || hov != colHover || bun != colBundleChild {
		t.Error("an outline colour differs in the blue theme")
	}
}

// The dark theme is dark: dark surfaces under light text, readably
// (WCAG AA, 4.5:1), and white still readable on its accent.
func TestDarkTheme(t *testing.T) {
	n := darkTheme.N
	contrast := func(a, b render.Color) float64 {
		la, lb := luminance(a), luminance(b)
		return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
	}
	for name, bg := range map[string]render.Color{"chrome": n.Chrome, "raised": n.Raised, "hover": n.HoverBG, "code": n.CodeBG} {
		if luminance(bg) > 0.05 {
			t.Errorf("%s: luminance %.3f, not dark", name, luminance(bg))
		}
		if c := contrast(n.Text, bg); c < 4.5 {
			t.Errorf("text on %s: contrast %.2f", name, c)
		}
	}
	if c := contrast(n.TextDim, n.Chrome); c < 4.5 {
		t.Errorf("dim text on chrome: contrast %.2f", c)
	}
	if c := contrast(colOnColor, darkTheme.Accent); c < 3 {
		t.Errorf("white on the accent: contrast %.2f", c)
	}
	// the viewport's ink shows on the dark workplane
	if c := contrast(n.DimLine, n.BG); c < 4.5 {
		t.Errorf("size lines on the background: contrast %.2f", c)
	}
}

// Turning a hue keeps lightness, saturation and alpha; greys stay grey.
func TestHueShift(t *testing.T) {
	for _, c := range []render.Color{
		render.RGB(0x2f, 0x7f, 0xe6), {0.80, 0.90, 0.97, 0.88}, {0.09, 0.78, 1.0, 1}, render.RGB(0x0e, 0x2a, 0x47),
	} {
		h0, s0, l0 := hsl(c)
		for _, deg := range []float64{-60, 90, 200} {
			d := hueShift(c, deg)
			h1, s1, l1 := hsl(d)
			if math.Abs(s1-s0) > 1e-5 || math.Abs(l1-l0) > 1e-5 || d[3] != c[3] {
				t.Errorf("%v turned %v: s %.4f -> %.4f, l %.4f -> %.4f, alpha %v -> %v", c, deg, s0, s1, l0, l1, c[3], d[3])
			}
			if dh := math.Mod(h1-h0-deg+720, 360); math.Min(dh, 360-dh) > 0.01 {
				t.Errorf("%v turned %v: hue %.2f -> %.2f", c, deg, h0, h1)
			}
		}
	}
	grey := render.RGB(0x6f, 0x6f, 0x6f)
	if hueShift(grey, 90) != grey {
		t.Error("a grey changed")
	}
}

// A derived theme looks as bright as the blue one, colour for colour (a
// green as light by the numbers looks far brighter).
func TestShiftedThemeKeepsBrightness(t *testing.T) {
	for _, th := range []Theme{greenTheme, orangeTheme, purpleTheme} {
		pairs := map[string][2]render.Color{
			"accent": {blueTheme.Accent, th.Accent}, "plane": {blueTheme.Plane, th.Plane},
			"grid": {blueTheme.GridMajor, th.GridMajor}, "logo top": {blueTheme.Logo[0], th.Logo[0]},
			"busy": {blueTheme.Busy, th.Busy},
		}
		for name, p := range pairs {
			if a, b := luminance(p[0]), luminance(p[1]); math.Abs(a-b) > 0.005 {
				t.Errorf("%s %s: luminance %.3f in blue, %.3f", th.Name, name, a, b)
			}
		}
		// white text on the accent stays readable (WCAG large-text contrast)
		for name, c := range map[string]render.Color{"accent": th.Accent, "busy": th.Busy} {
			if r := 1.05 / (luminance(c) + 0.05); r < 3 {
				t.Errorf("white on the %s %s: contrast %.2f, under 3", th.Name, name, r)
			}
		}
		// the workplane is toned down, not more vivid than the blue one
		if _, s, _ := hsl(th.Plane); s > 0.8 {
			t.Errorf("%s plane: saturation %.2f", th.Name, s)
		}
	}
}

// Each theme is its colour: its tinted colours' hues sit in its range,
// and the colours that must stand out from them don't.
func TestThemeHues(t *testing.T) {
	for _, tc := range []struct {
		th     Theme
		lo, hi float64
		ownSel bool // the selection outline is in the theme's hues
	}{
		{blueTheme, 185, 225, true}, {greenTheme, 130, 175, true}, {orangeTheme, 15, 45, true},
		{purpleTheme, 245, 285, true}, {tanTheme, 20, 45, false},
	} {
		g := tc.th
		cs := map[string]render.Color{
			"accent": g.Accent, "icon accent": g.IconAccent, "plane": g.Plane, "grid": g.GridMajor,
			"dim band": g.DimBand, "logo": g.Logo[1],
		}
		if tc.ownSel {
			cs["select"] = render.Color{g.Select[0], g.Select[1], g.Select[2], 1}
		}
		for name, c := range cs {
			if h, _, _ := hsl(c); h < tc.lo || h > tc.hi {
				t.Errorf("%s %s: hue %.0f°, outside %.0f-%.0f°", g.Name, name, h, tc.lo, tc.hi)
			}
		}
		sel, _, _ := hsl(render.Color{g.Select[0], g.Select[1], g.Select[2], 1})
		for name, c := range map[string]render.Color{
			"shape workplane": g.ShapeWP, "ruler band": g.RulerBand,
			"bundle outline": {g.BundleChild[0], g.BundleChild[1], g.BundleChild[2], 1},
		} {
			h, _, _ := hsl(c)
			if d := math.Abs(h - sel); math.Min(d, 360-d) < 50 {
				t.Errorf("%s %s: hue %.0f°, too close to the selection's %.0f°", g.Name, name, h, sel)
			}
		}
	}
}

// The tan theme's warm neutrals stay readable: text on its cream panels
// (WCAG AA, 4.5:1) and white on its caramel buttons (3:1).
func TestTanThemeReadable(t *testing.T) {
	n := tanTheme.N
	contrast := func(a, b render.Color) float64 {
		la, lb := luminance(a), luminance(b)
		return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
	}
	for name, bg := range map[string]render.Color{"chrome": n.Chrome, "raised": n.Raised, "hover": n.HoverBG} {
		if c := contrast(n.Text, bg); c < 4.5 {
			t.Errorf("text on %s: contrast %.2f", name, c)
		}
	}
	if c := contrast(n.TextDim, n.Chrome); c < 3 {
		t.Errorf("dim text on chrome: contrast %.2f", c)
	}
	for name, c := range map[string]render.Color{"accent": tanTheme.Accent, "busy": tanTheme.Busy} {
		if r := contrast(colOnColor, c); r < 3 {
			t.Errorf("white on the %s: contrast %.2f", name, r)
		}
	}
}

// Every theme has its own name.
func TestThemeNames(t *testing.T) {
	names := map[string]bool{}
	for _, th := range themes {
		if names[th.Name] {
			t.Errorf("two themes named %q", th.Name)
		}
		names[th.Name] = true
	}
}

func TestThemePref(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "theme.json")
	if got := readThemePref(p); got != "" {
		t.Errorf("no file: %q", got)
	}
	if err := writeThemePref(p, "Green"); err != nil {
		t.Fatal(err)
	}
	if got := readThemePref(p); got != "Green" {
		t.Errorf("read back %q", got)
	}
	if themeNamed("Green").Name != "Green" || themeNamed("nonsense").Name != "Tan" || themeNamed("").Name != "Tan" {
		t.Error("themeNamed: no or an unknown name should give the default, tan theme")
	}
}
