package shapes

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/scotticad/scotticad/pkg/geom"
)

// The font menu's names mapped to Windows font files (first existing file
// wins).
var fontFiles = map[string][]string{
	"Multilanguage": {"arialbd.ttf", "segoeuib.ttf"},
	"Sans":          {"arial.ttf", "segoeui.ttf"},
	"Sans Bold":     {"arialbd.ttf"},
	"Sans Mono":     {"consola.ttf", "cour.ttf"},
	"Serif":         {"times.ttf", "georgia.ttf"},
	"Serif Bold":    {"timesbd.ttf"},
	"Script":        {"segoesc.ttf", "comic.ttf"},
}

// FontNames lists the font menu's names first, then installed system fonts
// by file name.
func FontNames() []string {
	names := []string{"Multilanguage", "Sans", "Sans Bold", "Sans Mono", "Serif", "Serif Bold", "Script"}
	dir := fontDir()
	ents, _ := os.ReadDir(dir)
	var sys []string
	for _, e := range ents {
		n := strings.ToLower(e.Name())
		if strings.HasSuffix(n, ".ttf") || strings.HasSuffix(n, ".otf") {
			sys = append(sys, e.Name())
		}
	}
	sort.Strings(sys)
	return append(names, sys...)
}

func fontDir() string {
	w := os.Getenv("WINDIR")
	if w == "" {
		w = `C:\Windows`
	}
	return filepath.Join(w, "Fonts")
}

var fonts sync.Map

func loadFont(name string) *sfnt.Font {
	if f, ok := fonts.Load(name); ok {
		return f.(*sfnt.Font)
	}
	files := fontFiles[name]
	if files == nil {
		// a font file's name (from a design file): only in the fonts folder
		files = []string{filepath.Base(name)}
	}
	var f *sfnt.Font
	for _, fn := range files {
		data, err := os.ReadFile(filepath.Join(fontDir(), fn))
		if err != nil {
			continue
		}
		if ff, err := sfnt.Parse(data); err == nil {
			f = ff
			break
		}
		if coll, err := sfnt.ParseCollection(data); err == nil {
			if ff, err := coll.Font(0); err == nil {
				f = ff
				break
			}
		}
	}
	if f == nil {
		f, _ = sfnt.Parse(gobold.TTF)
	}
	fonts.Store(name, f)
	return f
}

// TextContours returns flattened glyph outlines for s in millimetres, with
// the baseline at y=0 and an em size of emMM.
func TextContours(s, fontName string, emMM float64) [][]Pt {
	var out [][]Pt
	for _, g := range glyphContours(s, fontName, emMM) {
		out = append(out, g...)
	}
	return out
}

// glyphContours is TextContours glyph by glyph.
func glyphContours(s, fontName string, emMM float64) [][][]Pt {
	f := loadFont(fontName)
	var buf sfnt.Buffer
	const ppem = 256
	scale := emMM / ppem
	var glyphs [][][]Pt
	var out [][]Pt
	x := 0.0
	var prev sfnt.GlyphIndex
	for i, r := range s {
		gi, err := f.GlyphIndex(&buf, r)
		if err != nil || gi == 0 {
			switch {
			case r == ' ':
				x += 0.3 * emMM
			case r >= ' ':
				// not in the font: its space stays (as the font's missing
				// glyph's), so the letters either side aren't run together
				if adv, err := f.GlyphAdvance(&buf, 0, fixed.I(ppem), font.HintingNone); err == nil {
					x += float64(adv) / 64 * scale
				}
			}
			prev = 0 // nothing to kern against
			continue
		}
		if i > 0 && prev != 0 {
			if k, err := f.Kern(&buf, prev, gi, fixed.I(ppem), font.HintingNone); err == nil {
				x += float64(k) / 64 * scale
			}
		}
		segs, err := f.LoadGlyph(&buf, gi, fixed.I(ppem), nil)
		if err == nil {
			var cur []Pt
			var last Pt
			tp := func(p fixed.Point26_6) Pt {
				return Pt{x + float64(p.X)/64*scale, -float64(p.Y) / 64 * scale}
			}
			flush := func() {
				if len(cur) >= 3 {
					out = append(out, cur)
				}
				cur = nil
			}
			for _, sg := range segs {
				switch sg.Op {
				case sfnt.SegmentOpMoveTo:
					flush()
					last = tp(sg.Args[0])
					cur = append(cur, last)
				case sfnt.SegmentOpLineTo:
					last = tp(sg.Args[0])
					cur = append(cur, last)
				case sfnt.SegmentOpQuadTo:
					c, e := tp(sg.Args[0]), tp(sg.Args[1])
					for k := 1; k <= 8; k++ {
						t := float64(k) / 8
						u := 1 - t
						cur = append(cur, Pt{u*u*last.X + 2*u*t*c.X + t*t*e.X, u*u*last.Y + 2*u*t*c.Y + t*t*e.Y})
					}
					last = e
				case sfnt.SegmentOpCubeTo:
					c1, c2, e := tp(sg.Args[0]), tp(sg.Args[1]), tp(sg.Args[2])
					for k := 1; k <= 10; k++ {
						t := float64(k) / 10
						u := 1 - t
						cur = append(cur, Pt{
							u*u*u*last.X + 3*u*u*t*c1.X + 3*u*t*t*c2.X + t*t*t*e.X,
							u*u*u*last.Y + 3*u*u*t*c1.Y + 3*u*t*t*c2.Y + t*t*t*e.Y})
					}
					last = e
				}
			}
			flush()
		}
		if len(out) > 0 {
			glyphs = append(glyphs, out)
			out = nil
		}
		if adv, err := f.GlyphAdvance(&buf, gi, fixed.I(ppem), font.HintingNone); err == nil {
			x += float64(adv) / 64 * scale
		}
		prev = gi
	}
	return glyphs
}

// TextMesh extrudes text to the given height. Each glyph is extruded on its
// own (its outlines nest within it only), and glyphs that overlap, as
// script letters do, are joined with a union: taken together, the overlap
// would count as a hole and the outlines would cross.
func TextMesh(s, fontName string, height float64) *geom.Mesh {
	if fontName == "" {
		fontName = "Multilanguage"
	}
	var parts []*geom.Mesh
	var cluster *geom.Mesh
	var cb geom.Box
	for _, g := range glyphContours(s, fontName, 27.7) {
		m := ExtrudeContours(g, 0, height)
		if len(m.Tris) == 0 {
			continue
		}
		b := m.Bounds()
		if cluster != nil && overlaps2D(cb, b) {
			cluster = geom.Union(cluster, m)
			cb = cb.Union(b)
			continue
		}
		if cluster != nil {
			parts = append(parts, cluster)
		}
		cluster, cb = m, b
	}
	if cluster != nil {
		parts = append(parts, cluster)
	}
	return done(geom.Merge(parts...))
}

// overlaps2D reports whether two boxes overlap seen from above.
func overlaps2D(a, b geom.Box) bool {
	return a.Min.X < b.Max.X && b.Min.X < a.Max.X && a.Min.Y < b.Max.Y && b.Min.Y < a.Max.Y
}

// TextNaturalSize returns the size of text at the default em (27.7 mm ≈
// 20 mm cap height) with the given extrusion height.
func TextNaturalSize(s, fontName string, height float64) geom.Vec3 {
	m := TextMesh(s, fontName, height)
	b := m.Bounds()
	if b.IsEmpty() {
		return geom.V(1, 1, height)
	}
	return b.Size()
}
