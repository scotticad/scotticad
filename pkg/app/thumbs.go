//go:build windows

package app

import (
	"image"
	"image/png"
	"io"
	"math"

	"github.com/scotticad/scotticad/pkg/geom"
	"github.com/scotticad/scotticad/pkg/gl"
	"github.com/scotticad/scotticad/pkg/render"
)

func thumbKey(kind string, hole bool) string {
	if hole {
		return kind + "#hole"
	}
	return kind
}

// buildThumbs renders every library tile once into the UI atlas.
func (a *App) buildThumbs() {
	a.thumbsDone = true
	// twice the size shown, for smoothness, but no more than 256 px: every
	// tile (over a hundred) must fit in the atlas at once, even at 400%
	size := int32(math.Round(math.Min(2*tileSize*a.w.DPI, 256)))
	t := render.NewTarget(8)
	t.Resize(size, size)
	defer t.Delete()
	cam := render.Camera{Yaw: -math.Pi/2 + 0.55, Pitch: 0.42, Fov: 28 * math.Pi / 180}
	seen := map[string]bool{}
	var tiles []struct {
		kind string
		hole bool
	}
	// every tile of every library (duplicates are skipped below)
	for _, lib := range libraries {
		tiles = append(tiles, tilesFor(lib)...)
	}
	for _, tl := range tiles {
		key := thumbKey(tl.kind, tl.hole)
		if seen[key] {
			continue
		}
		seen[key] = true
		p := newPlace(tl.kind, tl.hole)
		if p == nil {
			continue
		}
		b := p.mesh.Bounds()
		r := b.Size().Len() / 2
		cam.Target = b.Center()
		cam.Dist = r / math.Sin(cam.Fov/2) * 1.02
		t.Bind()
		gl.ClearColor(1, 1, 1, 0)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		gl.Enable(gl.DEPTH_TEST)
		gl.Disable(gl.BLEND)
		gl.DepthMask(true)
		a.scene.Begin(cam.VP(1), cam.Eye())
		st := render.Style{Edges: true}
		if tl.hole {
			// tiles are opaque, so the hole look is drawn solid
			st = holeStyle(geom.Identity(), 1)
			st.Alpha = 1
		}
		a.scene.DrawMesh(p.mesh, st)
		img := t.Read(0, 0, size, size)
		a.thumbs[key] = a.batch.AddImage(img)
	}
	gl.Disable(gl.DEPTH_TEST)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	a.target.Bind()
}

func encodePNG(w io.Writer, img image.Image) error { return png.Encode(w, img) }

// setWindowIcons renders the logo offscreen and uses it as the window's
// title-bar and taskbar icons.
func (a *App) setWindowIcons() {
	for _, ic := range []struct {
		size int
		big  bool
	}{{int(math.Round(16 * a.w.DPI)), false}, {int(math.Round(32 * a.w.DPI)), true}} {
		s := int32(ic.size)
		t := render.NewTarget(8)
		t.Resize(s, s)
		t.Bind()
		gl.ClearColor(0, 0, 0, 0)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		dpi := a.batch.DPI
		a.batch.DPI = 1
		a.batch.Begin(float64(s), float64(s))
		drawLogo(a.batch, render.Rect{W: float64(s), H: float64(s)})
		a.batch.Flush()
		a.batch.DPI = dpi
		img := t.Read(0, 0, s, s)
		t.Delete()
		// the render is premultiplied (edge pixels are darkened by their
		// coverage); icons take straight alpha
		px := img.Pix
		for i := 0; i < len(px); i += 4 {
			if al := int(px[i+3]); al > 0 && al < 255 {
				for k := 0; k < 3; k++ {
					px[i+k] = byte(min(255, int(px[i+k])*255/al))
				}
			}
		}
		a.w.SetIcon(px, ic.size, ic.big)
	}
	a.target.Bind()
}
