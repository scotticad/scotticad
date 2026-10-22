//go:build windows

package render

import (
	"image"

	"github.com/scotticad/scotticad/pkg/gl"
)

// Target is an offscreen multisampled framebuffer that is resolved to the
// window (or read back for screenshots).
type Target struct {
	FBO, color, depth uint32
	W, H              int32
	Samples           int32
	resolveFBO        uint32
	resolveTex        uint32
}

func NewTarget(samples int32) *Target { return &Target{Samples: samples} }

// Resize (re)allocates storage when the size changes.
func (t *Target) Resize(w, h int32) {
	if w == t.W && h == t.H && t.FBO != 0 {
		return
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if t.FBO != 0 {
		gl.DeleteFramebuffer(t.FBO)
		gl.DeleteRenderbuffer(t.color)
		gl.DeleteRenderbuffer(t.depth)
		gl.DeleteFramebuffer(t.resolveFBO)
		gl.DeleteTexture(t.resolveTex)
	}
	t.W, t.H = w, h
	t.color = gl.GenRenderbuffer()
	gl.BindRenderbuffer(gl.RENDERBUFFER, t.color)
	gl.RenderbufferStorageMultisample(gl.RENDERBUFFER, t.Samples, gl.RGBA8, w, h)
	t.depth = gl.GenRenderbuffer()
	gl.BindRenderbuffer(gl.RENDERBUFFER, t.depth)
	gl.RenderbufferStorageMultisample(gl.RENDERBUFFER, t.Samples, gl.DEPTH24_STENCIL8, w, h)
	t.FBO = gl.GenFramebuffer()
	gl.BindFramebuffer(gl.FRAMEBUFFER, t.FBO)
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, t.color)
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_STENCIL_ATTACH, gl.RENDERBUFFER, t.depth)

	t.resolveTex = gl.GenTexture()
	gl.BindTexture(gl.TEXTURE_2D, t.resolveTex)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, w, h, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	t.resolveFBO = gl.GenFramebuffer()
	gl.BindFramebuffer(gl.FRAMEBUFFER, t.resolveFBO)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, t.resolveTex)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

func (t *Target) Bind() {
	gl.BindFramebuffer(gl.FRAMEBUFFER, t.FBO)
	gl.Viewport(0, 0, t.W, t.H)
}

// BlitToWindow resolves to the default framebuffer.
func (t *Target) BlitToWindow() {
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, t.FBO)
	gl.BindFramebuffer(gl.DRAW_FRAMEBUFFER, 0)
	gl.BlitFramebuffer(0, 0, t.W, t.H, 0, 0, t.W, t.H, gl.COLOR_BUFFER_BIT, gl.NEAREST)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

// Read resolves and reads the rectangle (x, y from top-left) as an image.
func (t *Target) Read(x, y, w, h int32) *image.RGBA {
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, t.FBO)
	gl.BindFramebuffer(gl.DRAW_FRAMEBUFFER, t.resolveFBO)
	gl.BlitFramebuffer(0, 0, t.W, t.H, 0, 0, t.W, t.H, gl.COLOR_BUFFER_BIT, gl.NEAREST)
	gl.BindFramebuffer(gl.FRAMEBUFFER, t.resolveFBO)
	buf := make([]byte, w*h*4)
	gl.PixelStorei(gl.PACK_ALIGNMENT, 1)
	gl.ReadPixels(x, t.H-y-h, w, h, gl.RGBA, gl.UNSIGNED_BYTE, buf)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	stride := int(w) * 4
	for row := 0; row < int(h); row++ {
		copy(img.Pix[row*stride:(row+1)*stride], buf[(int(h)-1-row)*stride:(int(h)-row)*stride])
	}
	return img
}

// Delete frees the GPU resources.
func (t *Target) Delete() {
	if t.FBO == 0 {
		return
	}
	gl.DeleteFramebuffer(t.FBO)
	gl.DeleteRenderbuffer(t.color)
	gl.DeleteRenderbuffer(t.depth)
	gl.DeleteFramebuffer(t.resolveFBO)
	gl.DeleteTexture(t.resolveTex)
	t.FBO = 0
}
