//go:build windows

package render

import (
	"image"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"sync"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/scotticad/scotticad/pkg/gl"
)

const batchVS = `#version 330 core
layout(location=0) in vec2 aPos;
layout(location=1) in vec2 aUV;
layout(location=2) in vec4 aCol;
uniform vec2 uScreen;
out vec2 vUV;
out vec4 vCol;
void main() {
  vUV = aUV; vCol = aCol;
  gl_Position = vec4(aPos.x / uScreen.x * 2.0 - 1.0, 1.0 - aPos.y / uScreen.y * 2.0, 0.0, 1.0);
}`

const batchFS = `#version 330 core
in vec2 vUV;
in vec4 vCol;
uniform sampler2D uTex;
out vec4 o;
void main() { o = vCol * texture(uTex, vUV); }`

// Color is an RGBA colour with 0..1 components.
type Color [4]float32

func RGB(r, g, b uint8) Color     { return Color{float32(r) / 255, float32(g) / 255, float32(b) / 255, 1} }
func (c Color) A(a float32) Color { c[3] = a; return c }

// Rect is a screen rectangle in pixels.
type Rect struct{ X, Y, W, H float64 }

func (r Rect) Contains(x, y float64) bool { return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H }
func (r Rect) Inset(d float64) Rect       { return Rect{r.X + d, r.Y + d, r.W - 2*d, r.H - 2*d} }
func (r Rect) CX() float64                { return r.X + r.W/2 }
func (r Rect) CY() float64                { return r.Y + r.H/2 }

// Sprite is a region of the atlas.
type Sprite struct {
	U0, V0, U1, V1 float32
	W, H           int
}

// Batch draws 2D geometry with a texture atlas (fonts and images).
type Batch struct {
	prog          uint32
	uScreen, uTex int32
	vao, vbo      uint32
	data          []float32
	W, H          float64
	atlas         uint32
	atlasSize     int
	ax, ay, rowH  int
	pix           *image.RGBA
	dirty         bool
	white         Sprite
	fonts         map[fontKey]*fontFace
	ttf           map[int]*opentype.Font
	clipStack     []Rect
	DPI           float64
	// Generation counts atlas rebuilds: sprites from an earlier generation
	// (images added with AddImage) point at stale places and must be added
	// again.
	Generation int
}

// maxAtlas is the largest the atlas grows to (64 MB).
const maxAtlas = 4096

type fontKey struct {
	size  float64
	style int
}

// font styles
const (
	styleRegular = iota
	styleBold
	styleMono
)

func style(bold bool) int {
	if bold {
		return styleBold
	}
	return styleRegular
}

type glyph struct {
	sp      Sprite
	ox, oy  float64 // offset from pen position to top-left
	advance float64
}

// glyphKey is a glyph and whether it is drawn light (see lightText).
type glyphKey struct {
	r     rune
	light bool
}

type fontFace struct {
	face   font.Face
	glyphs map[glyphKey]glyph
	ascent float64
	height float64
}

func NewBatch() (*Batch, error) {
	b := &Batch{atlasSize: 2048, fonts: map[fontKey]*fontFace{}, ttf: map[int]*opentype.Font{}, DPI: 1}
	var err error
	if b.prog, err = gl.Program(batchVS, batchFS); err != nil {
		return nil, err
	}
	b.uScreen, b.uTex = gl.Uniform(b.prog, "uScreen"), gl.Uniform(b.prog, "uTex")
	b.vao, b.vbo = gl.GenVertexArray(), gl.GenBuffer()
	gl.BindVertexArray(b.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, b.vbo)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 32, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(1, 2, gl.FLOAT, false, 32, 8)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointer(2, 4, gl.FLOAT, false, 32, 16)
	gl.BindVertexArray(0)
	b.pix = image.NewRGBA(image.Rect(0, 0, b.atlasSize, b.atlasSize))
	b.atlas = gl.GenTexture()
	gl.BindTexture(gl.TEXTURE_2D, b.atlas)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	b.addWhite()
	b.dirty = true
	return b, nil
}

// addWhite packs the white block every solid shape is drawn with.
func (b *Batch) addWhite() {
	white := image.NewRGBA(image.Rect(0, 0, 4, 4))
	draw.Draw(white, white.Bounds(), image.White, image.Point{}, draw.Src)
	b.white = b.AddImage(white)
	// sample the centre of the white block
	b.white.U0 += 1.5 / float32(b.atlasSize)
	b.white.V0 += 1.5 / float32(b.atlasSize)
	b.white.U1, b.white.V1 = b.white.U0, b.white.V0
}

// AtlasSize is the atlas's width and height in pixels.
func (b *Batch) AtlasSize() int { return b.atlasSize }

// rebuild starts the atlas over when it is full: what is queued is drawn
// first (it points into the old atlas), the atlas doubles if it can (a
// high-DPI screen needs the room), the glyphs are rasterised again as they
// are used, and Generation tells holders of other sprites to add theirs
// again.
func (b *Batch) rebuild() {
	b.Flush() // while pix still matches the atlas size it is uploaded at
	if b.atlasSize < maxAtlas {
		b.atlasSize *= 2
	}
	b.Reset()
}

// Reset empties the atlas, keeping its size (to add a set of images again
// into a clean one). Generation tells holders of sprites to add theirs
// again.
func (b *Batch) Reset() {
	b.Flush()
	b.pix = image.NewRGBA(image.Rect(0, 0, b.atlasSize, b.atlasSize))
	b.ax, b.ay, b.rowH = 0, 0, 0
	// the faces stay (a string being drawn holds one): only their glyphs'
	// places in the atlas are stale
	for _, f := range b.fonts {
		f.glyphs = map[glyphKey]glyph{}
	}
	b.Generation++
	b.addWhite()
}

// AddImage packs an image into the atlas.
func (b *Batch) AddImage(img image.Image) Sprite {
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	if b.ax+w+2 > b.atlasSize {
		b.ax = 0
		b.ay += b.rowH + 2
		b.rowH = 0
	}
	if b.ay+h+2 > b.atlasSize {
		b.rebuild()
	}
	x, y := b.ax+1, b.ay+1
	draw.Draw(b.pix, image.Rect(x, y, x+w, y+h), img, r.Min, draw.Src)
	b.ax += w + 2
	if h > b.rowH {
		b.rowH = h
	}
	b.dirty = true
	s := float32(b.atlasSize)
	return Sprite{float32(x) / s, float32(y) / s, float32(x+w) / s, float32(y+h) / s, w, h}
}

func (b *Batch) loadTTF(st int) *opentype.Font {
	if f, ok := b.ttf[st]; ok {
		return f
	}
	name := []string{"segoeui.ttf", "segoeuib.ttf", "consola.ttf"}[st]
	wd := os.Getenv("WINDIR")
	if wd == "" {
		wd = `C:\Windows`
	}
	var f *opentype.Font
	if data, err := os.ReadFile(filepath.Join(wd, "Fonts", name)); err == nil {
		f, _ = opentype.Parse(data)
	}
	if f == nil && st == styleMono {
		f, _ = opentype.Parse(gomono.TTF)
	}
	if f == nil {
		f, _ = opentype.Parse(goregular.TTF)
	}
	b.ttf[st] = f
	return f
}

var faceMu sync.Mutex

func (b *Batch) face(size float64, bold bool) *fontFace { return b.faceStyle(size, style(bold)) }

func (b *Batch) faceStyle(size float64, st int) *fontFace {
	k := fontKey{size * b.DPI, st}
	if f, ok := b.fonts[k]; ok {
		return f
	}
	faceMu.Lock()
	defer faceMu.Unlock()
	face, _ := opentype.NewFace(b.loadTTF(st), &opentype.FaceOptions{Size: size * b.DPI, DPI: 72, Hinting: font.HintingFull})
	m := face.Metrics()
	f := &fontFace{face: face, glyphs: map[glyphKey]glyph{}, ascent: float64(m.Ascent) / 64, height: float64(m.Height) / 64}
	b.fonts[k] = f
	return f
}

func (b *Batch) glyph(f *fontFace, r rune) glyph { return b.glyphFor(f, r, false) }

// lightText is how far light text's glyph coverage is lifted (an exponent
// on it): coverage blended linearly makes light text on a dark ground look
// thinner and fainter than dark text on a light one at the same weight,
// and lifting the partly covered edge pixels gives it back its weight.
const lightText = 0.6

// isLight is whether text in c is drawn with lifted coverage.
func isLight(c Color) bool { return 0.2126*c[0]+0.7152*c[1]+0.0722*c[2] > 0.6 }

func (b *Batch) glyphFor(f *fontFace, r rune, light bool) glyph {
	k := glyphKey{r, light}
	if g, ok := f.glyphs[k]; ok {
		return g
	}
	dr, mask, mp, adv, ok := f.face.Glyph(fixed.P(0, 0), r)
	g := glyph{advance: float64(adv) / 64}
	if ok && !dr.Empty() {
		img := image.NewRGBA(image.Rect(0, 0, dr.Dx(), dr.Dy()))
		draw.DrawMask(img, img.Bounds(), image.White, image.Point{}, mask, mp, draw.Src)
		if light {
			var lut [256]uint8
			for i := range lut {
				lut[i] = uint8(math.Round(255 * math.Pow(float64(i)/255, lightText)))
			}
			for i := range img.Pix { // white, premultiplied: every channel is the coverage
				img.Pix[i] = lut[img.Pix[i]]
			}
		}
		g.sp = b.AddImage(img)
		g.ox, g.oy = float64(dr.Min.X), float64(dr.Min.Y)
	}
	f.glyphs[k] = g
	return g
}

// Begin starts a frame of 2D drawing.
func (b *Batch) Begin(w, h float64) {
	b.W, b.H = w, h
	b.data = b.data[:0]
	b.clipStack = nil
}

func (b *Batch) vert(x, y float64, u, v float32, c Color) {
	b.data = append(b.data, float32(x), float32(y), u, v, c[0], c[1], c[2], c[3])
}

// Tri draws a solid triangle.
func (b *Batch) Tri(x0, y0, x1, y1, x2, y2 float64, c Color) {
	u, v := b.white.U0, b.white.V0
	b.vert(x0, y0, u, v, c)
	b.vert(x1, y1, u, v, c)
	b.vert(x2, y2, u, v, c)
}

// Quad draws a solid quadrilateral.
func (b *Batch) Quad(x0, y0, x1, y1, x2, y2, x3, y3 float64, c Color) {
	b.Tri(x0, y0, x1, y1, x2, y2, c)
	b.Tri(x0, y0, x2, y2, x3, y3, c)
}

func (b *Batch) Fill(r Rect, c Color) {
	b.Quad(r.X, r.Y, r.X+r.W, r.Y, r.X+r.W, r.Y+r.H, r.X, r.Y+r.H, c)
}

// RoundRect fills a rounded rectangle.
func (b *Batch) RoundRect(r Rect, rad float64, c Color) {
	rad = math.Min(rad, math.Min(r.W, r.H)/2)
	if rad <= 0.5 {
		b.Fill(r, c)
		return
	}
	pts := roundedPoints(r, rad)
	cx, cy := r.CX(), r.CY()
	for i := range pts {
		j := (i + 1) % len(pts)
		b.Tri(cx, cy, pts[i][0], pts[i][1], pts[j][0], pts[j][1], c)
	}
}

func roundedPoints(r Rect, rad float64) [][2]float64 {
	var pts [][2]float64
	corners := [][3]float64{{r.X + r.W - rad, r.Y + rad, -math.Pi / 2}, {r.X + r.W - rad, r.Y + r.H - rad, 0}, {r.X + rad, r.Y + r.H - rad, math.Pi / 2}, {r.X + rad, r.Y + rad, math.Pi}}
	for _, c := range corners {
		for k := 0; k <= 6; k++ {
			a := c[2] + float64(k)/6*math.Pi/2
			pts = append(pts, [2]float64{c[0] + rad*math.Cos(a), c[1] + rad*math.Sin(a)})
		}
	}
	return pts
}

// RoundRectStroke outlines a rounded rectangle.
func (b *Batch) RoundRectStroke(r Rect, rad, width float64, c Color) {
	rad = math.Min(rad, math.Min(r.W, r.H)/2)
	var pts [][2]float64
	if rad <= 0.5 {
		pts = [][2]float64{{r.X, r.Y}, {r.X + r.W, r.Y}, {r.X + r.W, r.Y + r.H}, {r.X, r.Y + r.H}}
	} else {
		pts = roundedPoints(r, rad)
	}
	pts = append(pts, pts[0])
	b.Polyline(pts, width, c)
}

// Circle fills a circle.
func (b *Batch) Circle(cx, cy, rad float64, c Color) {
	n := int(math.Max(12, math.Min(64, rad*1.5)))
	for i := 0; i < n; i++ {
		a0 := 2 * math.Pi * float64(i) / float64(n)
		a1 := 2 * math.Pi * float64(i+1) / float64(n)
		b.Tri(cx, cy, cx+rad*math.Cos(a0), cy+rad*math.Sin(a0), cx+rad*math.Cos(a1), cy+rad*math.Sin(a1), c)
	}
}

// Ring strokes a circle.
func (b *Batch) Ring(cx, cy, rad, width float64, c Color) {
	n := int(math.Max(16, math.Min(96, rad*1.5)))
	pts := make([][2]float64, n+1)
	for i := 0; i <= n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = [2]float64{cx + rad*math.Cos(a), cy + rad*math.Sin(a)}
	}
	b.Polyline(pts, width, c)
}

// Line draws a thick line segment.
func (b *Batch) Line(x0, y0, x1, y1, width float64, c Color) {
	dx, dy := x1-x0, y1-y0
	l := math.Hypot(dx, dy)
	if l < 1e-9 {
		return
	}
	nx, ny := -dy/l*width/2, dx/l*width/2
	b.Quad(x0+nx, y0+ny, x1+nx, y1+ny, x1-nx, y1-ny, x0-nx, y0-ny, c)
}

// Polyline draws connected thick segments with round-ish joins.
func (b *Batch) Polyline(pts [][2]float64, width float64, c Color) {
	for i := 0; i+1 < len(pts); i++ {
		b.Line(pts[i][0], pts[i][1], pts[i+1][0], pts[i+1][1], width, c)
		if width > 1.5 && i > 0 {
			b.Circle(pts[i][0], pts[i][1], width/2, c)
		}
	}
}

// Dashed draws a dashed line. Only the part on screen is dashed: a point
// just in front of the camera projects millions of pixels away, and
// dashing all of it would stall the frame.
func (b *Batch) Dashed(x0, y0, x1, y1, width, dash float64, c Color) {
	l := math.Hypot(x1-x0, y1-y0)
	if l < 1e-9 || !(dash > 0) || math.IsInf(l, 0) || math.IsNaN(l) {
		return
	}
	s0, s1, ok := clipSegment(x0, y0, x1, y1, -width-dash, -width-dash, b.W+width+dash, b.H+width+dash)
	if !ok {
		return
	}
	// whole dash periods from the line's start, so dashes don't crawl as
	// the clipped start moves
	start := math.Floor(s0*l/(2*dash)) * 2 * dash
	end := s1 * l
	// counted, not stepped: at 1e17 px from the line's start, adding a
	// dash's length no longer changes a float64, and a stepped loop would
	// never end. Past what a screen can show, the visible part is drawn
	// solid.
	n := (end - start) / (2 * dash)
	if !(n <= 10000) || start+dash == start {
		b.Line(x0+(x1-x0)*s0, y0+(y1-y0)*s0, x0+(x1-x0)*s1, y0+(y1-y0)*s1, width, c)
		return
	}
	for k := 0; k <= int(n); k++ {
		s := start + float64(k)*2*dash
		if s >= end {
			break
		}
		e := math.Min(s+dash, l)
		b.Line(x0+(x1-x0)*s/l, y0+(y1-y0)*s/l, x0+(x1-x0)*e/l, y0+(y1-y0)*e/l, width, c)
	}
}

// clipSegment is the part of the segment (x0,y0)-(x1,y1) inside the
// rectangle [minX, maxX] × [minY, maxY], as the fractions s0 ≤ s1 of the
// way along it (Liang–Barsky); ok is false if none of it is inside or the
// segment isn't finite.
func clipSegment(x0, y0, x1, y1, minX, minY, maxX, maxY float64) (s0, s1 float64, ok bool) {
	dx, dy := x1-x0, y1-y0
	s0, s1 = 0, 1
	for _, e := range [4][2]float64{{-dx, x0 - minX}, {dx, maxX - x0}, {-dy, y0 - minY}, {dy, maxY - y0}} {
		p, q := e[0], e[1]
		if math.IsNaN(p) || math.IsNaN(q) {
			return 0, 0, false
		}
		if p == 0 {
			if q < 0 {
				return 0, 0, false // parallel to this edge and outside it
			}
			continue
		}
		t := q / p
		if p < 0 {
			s0 = math.Max(s0, t)
		} else {
			s1 = math.Min(s1, t)
		}
	}
	return s0, s1, s0 < s1
}

// Poly fills a convex polygon.
func (b *Batch) Poly(pts [][2]float64, c Color) {
	for i := 1; i+1 < len(pts); i++ {
		b.Tri(pts[0][0], pts[0][1], pts[i][0], pts[i][1], pts[i+1][0], pts[i+1][1], c)
	}
}

// Image draws a sprite into r.
func (b *Batch) Image(sp Sprite, r Rect, c Color) {
	b.vert(r.X, r.Y, sp.U0, sp.V0, c)
	b.vert(r.X+r.W, r.Y, sp.U1, sp.V0, c)
	b.vert(r.X+r.W, r.Y+r.H, sp.U1, sp.V1, c)
	b.vert(r.X, r.Y, sp.U0, sp.V0, c)
	b.vert(r.X+r.W, r.Y+r.H, sp.U1, sp.V1, c)
	b.vert(r.X, r.Y+r.H, sp.U0, sp.V1, c)
}

// TextWidth measures a string in pixels.
func (b *Batch) TextWidth(s string, size float64, bold bool) float64 {
	f := b.face(size, bold)
	w := 0.0
	for _, r := range s {
		w += b.glyph(f, r).advance
	}
	return w / b.DPI
}

// LineHeight returns the font's line height in logical pixels.
func (b *Batch) LineHeight(size float64, bold bool) float64 { return b.face(size, bold).height / b.DPI }

// Text draws s with its top-left at (x, y) in logical pixels. Returns the
// advance width.
func (b *Batch) Text(s string, x, y, size float64, bold bool, c Color) float64 {
	return b.text(b.face(size, bold), s, x, y, c)
}

// Mono draws s in the monospaced (code) font; see Text.
func (b *Batch) Mono(s string, x, y, size float64, c Color) float64 {
	return b.text(b.faceStyle(size, styleMono), s, x, y, c)
}

// MonoAdvance is the width of one character of the monospaced font.
func (b *Batch) MonoAdvance(size float64) float64 {
	return b.glyph(b.faceStyle(size, styleMono), 'M').advance / b.DPI
}

// MonoLineHeight is the monospaced font's line height.
func (b *Batch) MonoLineHeight(size float64) float64 {
	return b.faceStyle(size, styleMono).height / b.DPI
}

func (b *Batch) text(f *fontFace, s string, x, y float64, c Color) float64 {
	// work in device pixels for crisp glyphs
	px := math.Round(x * b.DPI)
	py := math.Round(y*b.DPI + f.ascent)
	start := px
	light := isLight(c)
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		s = s[n:]
		g := b.glyphFor(f, r, light)
		if g.sp.W > 0 {
			b.Image(g.sp, Rect{(px + g.ox) / b.DPI, (py + g.oy) / b.DPI, float64(g.sp.W) / b.DPI, float64(g.sp.H) / b.DPI}, c)
		}
		px += g.advance
	}
	return (px - start) / b.DPI
}

// TextCentered draws s centred in r.
func (b *Batch) TextCentered(s string, r Rect, size float64, bold bool, c Color) {
	w := b.TextWidth(s, size, bold)
	h := b.LineHeight(size, bold)
	b.Text(s, r.CX()-w/2, r.CY()-h/2, size, bold, c)
}

// PushClip restricts drawing to r (by flushing and using scissor).
func (b *Batch) PushClip(r Rect) {
	b.Flush()
	b.clipStack = append(b.clipStack, r)
	b.applyClip()
}

func (b *Batch) PopClip() {
	b.Flush()
	if len(b.clipStack) > 0 {
		b.clipStack = b.clipStack[:len(b.clipStack)-1]
	}
	b.applyClip()
}

func (b *Batch) applyClip() {
	if len(b.clipStack) == 0 {
		gl.Disable(gl.SCISSOR_TEST)
		return
	}
	r := b.clipStack[len(b.clipStack)-1]
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(int32(r.X*b.DPI), int32(b.H*b.DPI-(r.Y+r.H)*b.DPI), int32(r.W*b.DPI), int32(r.H*b.DPI))
}

// Flush draws accumulated geometry. Coordinates are logical pixels; the
// viewport is device pixels.
func (b *Batch) Flush() {
	if b.dirty {
		gl.BindTexture(gl.TEXTURE_2D, b.atlas)
		gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(b.atlasSize), int32(b.atlasSize), gl.RGBA, gl.UNSIGNED_BYTE, b.pix.Pix)
		b.dirty = false
	}
	if len(b.data) == 0 {
		return
	}
	gl.Disable(gl.DEPTH_TEST)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.UseProgram(b.prog)
	gl.Uniform2f(b.uScreen, float32(b.W), float32(b.H))
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, b.atlas)
	gl.Uniform1i(b.uTex, 0)
	gl.BindVertexArray(b.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, b.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, b.data, gl.DYNAMIC_DRAW)
	gl.DrawArrays(gl.TRIANGLES, 0, int32(len(b.data)/8))
	gl.BindVertexArray(0)
	b.data = b.data[:0]
}
