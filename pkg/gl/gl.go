//go:build windows

// Package gl is a small hand-written OpenGL 3.3 binding that calls the
// driver through syscall (no cgo). Only the entry points ScottiCAD uses are
// bound. Float arguments are passed as their bit patterns; the Windows x64
// syscall trampoline mirrors integer argument registers into XMM registers,
// so this works for float32 parameters.
package gl

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"

	"github.com/scotticad/scotticad/pkg/win"
)

const (
	DEPTH_BUFFER_BIT     = 0x00000100
	STENCIL_BUFFER_BIT   = 0x00000400
	COLOR_BUFFER_BIT     = 0x00004000
	TRIANGLES            = 0x0004
	LINES                = 0x0001
	LINE_STRIP           = 0x0003
	TRIANGLE_FAN         = 0x0006
	DEPTH_TEST           = 0x0B71
	BLEND                = 0x0BE2
	CULL_FACE            = 0x0B44
	SCISSOR_TEST         = 0x0C11
	POLYGON_OFFSET_FILL  = 0x8037
	MULTISAMPLE          = 0x809D
	SRC_ALPHA            = 0x0302
	ONE_MINUS_SRC_ALPHA  = 0x0303
	ONE                  = 1
	LEQUAL               = 0x0203
	LESS                 = 0x0201
	ALWAYS               = 0x0207
	FLOAT                = 0x1406
	UNSIGNED_BYTE        = 0x1401
	ARRAY_BUFFER         = 0x8892
	STATIC_DRAW          = 0x88E4
	DYNAMIC_DRAW         = 0x88E8
	FRAGMENT_SHADER      = 0x8B30
	VERTEX_SHADER        = 0x8B31
	COMPILE_STATUS       = 0x8B81
	LINK_STATUS          = 0x8B82
	INFO_LOG_LENGTH      = 0x8B84
	TEXTURE_2D           = 0x0DE1
	TEXTURE0             = 0x84C0
	TEXTURE_MIN_FILTER   = 0x2801
	TEXTURE_MAG_FILTER   = 0x2800
	TEXTURE_WRAP_S       = 0x2802
	TEXTURE_WRAP_T       = 0x2803
	LINEAR               = 0x2601
	NEAREST              = 0x2600
	CLAMP_TO_EDGE        = 0x812F
	RGBA                 = 0x1908
	RGBA8                = 0x8058
	RED                  = 0x1903
	R8                   = 0x8229
	UNPACK_ALIGNMENT     = 0x0CF5
	PACK_ALIGNMENT       = 0x0D05
	FRAMEBUFFER          = 0x8D40
	READ_FRAMEBUFFER     = 0x8CA8
	DRAW_FRAMEBUFFER     = 0x8CA9
	RENDERBUFFER         = 0x8D41
	COLOR_ATTACHMENT0    = 0x8CE0
	DEPTH_STENCIL_ATTACH = 0x821A
	DEPTH24_STENCIL8     = 0x88F0
	FRAMEBUFFER_COMPLETE = 0x8CD5
	VERSION              = 0x1F02
	RENDERER             = 0x1F01
	MAX_SAMPLES          = 0x8D57
	BACK                 = 0x0405
	FRONT                = 0x0404
)

type fn uintptr

var procs = map[string]*fn{}

var (
	viewport, clearColor, clear, enable, disable, blendFunc, depthFunc, depthMask,
	polygonOffset, genBuffers, bindBuffer, bufferData, deleteBuffers, genVertexArrays,
	bindVertexArray, deleteVertexArrays, enableVertexAttribArray, vertexAttribPointer,
	createShader, shaderSource, compileShader, getShaderiv, getShaderInfoLog, createProgram,
	attachShader, linkProgram, getProgramiv, getProgramInfoLog, useProgram, getUniformLocation,
	uniformMatrix4fv, uniform1f, uniform2f, uniform3f, uniform4f, uniform1i, drawArrays,
	genTextures, bindTexture, texImage2D, texSubImage2D, texParameteri, activeTexture,
	pixelStorei, readPixels, genFramebuffers, bindFramebuffer, framebufferRenderbuffer,
	framebufferTexture2D, genRenderbuffers, bindRenderbuffer, renderbufferStorageMultisample,
	renderbufferStorage, blitFramebuffer, deleteFramebuffers, deleteRenderbuffers,
	deleteTextures, checkFramebufferStatus, scissor, getError, getString, getIntegerv,
	bindAttribLocation, cullFace, colorMask, lineWidth, swapInterval fn
)

func init() {
	for name, p := range map[string]*fn{
		"glViewport": &viewport, "glClearColor": &clearColor, "glClear": &clear, "glEnable": &enable,
		"glDisable": &disable, "glBlendFunc": &blendFunc, "glDepthFunc": &depthFunc, "glDepthMask": &depthMask,
		"glPolygonOffset": &polygonOffset, "glGenBuffers": &genBuffers, "glBindBuffer": &bindBuffer,
		"glBufferData": &bufferData, "glDeleteBuffers": &deleteBuffers, "glGenVertexArrays": &genVertexArrays,
		"glBindVertexArray": &bindVertexArray, "glDeleteVertexArrays": &deleteVertexArrays,
		"glEnableVertexAttribArray": &enableVertexAttribArray, "glVertexAttribPointer": &vertexAttribPointer,
		"glCreateShader": &createShader, "glShaderSource": &shaderSource, "glCompileShader": &compileShader,
		"glGetShaderiv": &getShaderiv, "glGetShaderInfoLog": &getShaderInfoLog, "glCreateProgram": &createProgram,
		"glAttachShader": &attachShader, "glLinkProgram": &linkProgram, "glGetProgramiv": &getProgramiv,
		"glGetProgramInfoLog": &getProgramInfoLog, "glUseProgram": &useProgram, "glGetUniformLocation": &getUniformLocation,
		"glUniformMatrix4fv": &uniformMatrix4fv, "glUniform1f": &uniform1f, "glUniform2f": &uniform2f,
		"glUniform3f": &uniform3f, "glUniform4f": &uniform4f, "glUniform1i": &uniform1i, "glDrawArrays": &drawArrays,
		"glGenTextures": &genTextures, "glBindTexture": &bindTexture, "glTexImage2D": &texImage2D,
		"glTexSubImage2D": &texSubImage2D, "glTexParameteri": &texParameteri, "glActiveTexture": &activeTexture,
		"glPixelStorei": &pixelStorei, "glReadPixels": &readPixels, "glGenFramebuffers": &genFramebuffers,
		"glBindFramebuffer": &bindFramebuffer, "glFramebufferRenderbuffer": &framebufferRenderbuffer,
		"glFramebufferTexture2D": &framebufferTexture2D, "glGenRenderbuffers": &genRenderbuffers,
		"glBindRenderbuffer": &bindRenderbuffer, "glRenderbufferStorageMultisample": &renderbufferStorageMultisample,
		"glRenderbufferStorage": &renderbufferStorage, "glBlitFramebuffer": &blitFramebuffer,
		"glDeleteFramebuffers": &deleteFramebuffers, "glDeleteRenderbuffers": &deleteRenderbuffers,
		"glDeleteTextures": &deleteTextures, "glCheckFramebufferStatus": &checkFramebufferStatus, "glScissor": &scissor,
		"glGetError": &getError, "glGetString": &getString, "glGetIntegerv": &getIntegerv,
		"glBindAttribLocation": &bindAttribLocation, "glCullFace": &cullFace, "glColorMask": &colorMask,
		"glLineWidth": &lineWidth, "wglSwapIntervalEXT": &swapInterval,
	} {
		procs[name] = p
	}
}

// Init resolves all entry points. The GL context must be current.
func Init() error {
	var missing []string
	for name, p := range procs {
		*p = fn(win.GetProcAddress(name))
		if *p == 0 && name != "wglSwapIntervalEXT" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("OpenGL 3.3 not available (missing %v); version %s", missing, GetString(VERSION))
	}
	return nil
}

func call(f fn, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(uintptr(f), args...)
	return r
}

func f32(f float32) uintptr { return uintptr(math.Float32bits(f)) }
func b(v bool) uintptr {
	if v {
		return 1
	}
	return 0
}

func Viewport(x, y, w, h int32)           { call(viewport, uintptr(x), uintptr(y), uintptr(w), uintptr(h)) }
func ClearColor(r, g, bl, a float32)      { call(clearColor, f32(r), f32(g), f32(bl), f32(a)) }
func Clear(mask uint32)                   { call(clear, uintptr(mask)) }
func Enable(c uint32)                     { call(enable, uintptr(c)) }
func Disable(c uint32)                    { call(disable, uintptr(c)) }
func BlendFunc(s, d uint32)               { call(blendFunc, uintptr(s), uintptr(d)) }
func DepthFunc(f uint32)                  { call(depthFunc, uintptr(f)) }
func DepthMask(v bool)                    { call(depthMask, b(v)) }
func ColorMask(r, g, bl, a bool)          { call(colorMask, b(r), b(g), b(bl), b(a)) }
func CullFace(m uint32)                   { call(cullFace, uintptr(m)) }
func PolygonOffset(factor, units float32) { call(polygonOffset, f32(factor), f32(units)) }
func Scissor(x, y, w, h int32)            { call(scissor, uintptr(x), uintptr(y), uintptr(w), uintptr(h)) }
func GetError() uint32                    { return uint32(call(getError)) }

func SwapInterval(n int) {
	if swapInterval != 0 {
		call(swapInterval, uintptr(n))
	}
}

func GetString(name uint32) string {
	if getString == 0 {
		getString = fn(win.GetProcAddress("glGetString"))
	}
	p := call(getString, uintptr(name))
	if p == 0 {
		return ""
	}
	var s []byte
	for i := uintptr(0); ; i++ {
		c := *(*byte)(unsafe.Pointer(p + i))
		if c == 0 {
			break
		}
		s = append(s, c)
	}
	return string(s)
}

func GetInteger(name uint32) int32 {
	var v int32
	call(getIntegerv, uintptr(name), uintptr(unsafe.Pointer(&v)))
	return v
}

func GenBuffer() uint32 {
	var id uint32
	call(genBuffers, 1, uintptr(unsafe.Pointer(&id)))
	return id
}
func BindBuffer(target, id uint32) { call(bindBuffer, uintptr(target), uintptr(id)) }
func BufferData(target uint32, data []float32, usage uint32) {
	var p uintptr
	if len(data) > 0 {
		p = uintptr(unsafe.Pointer(&data[0]))
	}
	call(bufferData, uintptr(target), uintptr(len(data)*4), p, uintptr(usage))
}
func DeleteBuffer(id uint32) { call(deleteBuffers, 1, uintptr(unsafe.Pointer(&id))) }

func GenVertexArray() uint32 {
	var id uint32
	call(genVertexArrays, 1, uintptr(unsafe.Pointer(&id)))
	return id
}
func BindVertexArray(id uint32)        { call(bindVertexArray, uintptr(id)) }
func DeleteVertexArray(id uint32)      { call(deleteVertexArrays, 1, uintptr(unsafe.Pointer(&id))) }
func EnableVertexAttribArray(i uint32) { call(enableVertexAttribArray, uintptr(i)) }
func VertexAttribPointer(i uint32, size int32, typ uint32, norm bool, stride int32, offset int) {
	call(vertexAttribPointer, uintptr(i), uintptr(size), uintptr(typ), b(norm), uintptr(stride), uintptr(offset))
}

func cstr(s string) *byte { p, _ := syscall.BytePtrFromString(s); return p }

func compile(kind uint32, src string) (uint32, error) {
	s := uint32(call(createShader, uintptr(kind)))
	cs := cstr(src)
	call(shaderSource, uintptr(s), 1, uintptr(unsafe.Pointer(&cs)), 0)
	call(compileShader, uintptr(s))
	var ok int32
	call(getShaderiv, uintptr(s), COMPILE_STATUS, uintptr(unsafe.Pointer(&ok)))
	if ok == 0 {
		buf := make([]byte, 4096)
		var n int32
		call(getShaderInfoLog, uintptr(s), 4096, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
		return 0, fmt.Errorf("shader compile: %s", buf[:n])
	}
	return s, nil
}

// Program compiles and links a shader program. attribs binds attribute
// names to locations 0..n-1.
func Program(vs, fs string, attribs ...string) (uint32, error) {
	v, err := compile(VERTEX_SHADER, vs)
	if err != nil {
		return 0, err
	}
	f, err := compile(FRAGMENT_SHADER, fs)
	if err != nil {
		return 0, err
	}
	p := uint32(call(createProgram))
	call(attachShader, uintptr(p), uintptr(v))
	call(attachShader, uintptr(p), uintptr(f))
	for i, a := range attribs {
		call(bindAttribLocation, uintptr(p), uintptr(i), uintptr(unsafe.Pointer(cstr(a))))
	}
	call(linkProgram, uintptr(p))
	var ok int32
	call(getProgramiv, uintptr(p), LINK_STATUS, uintptr(unsafe.Pointer(&ok)))
	if ok == 0 {
		buf := make([]byte, 4096)
		var n int32
		call(getProgramInfoLog, uintptr(p), 4096, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
		return 0, fmt.Errorf("program link: %s", buf[:n])
	}
	return p, nil
}

func UseProgram(p uint32) { call(useProgram, uintptr(p)) }
func Uniform(p uint32, name string) int32 {
	return int32(call(getUniformLocation, uintptr(p), uintptr(unsafe.Pointer(cstr(name)))))
}
func UniformMatrix4(loc int32, m [16]float32) {
	call(uniformMatrix4fv, uintptr(loc), 1, 0, uintptr(unsafe.Pointer(&m[0])))
}
func Uniform1f(loc int32, v float32)       { call(uniform1f, uintptr(loc), f32(v)) }
func Uniform2f(loc int32, a, c float32)    { call(uniform2f, uintptr(loc), f32(a), f32(c)) }
func Uniform3f(loc int32, a, c, d float32) { call(uniform3f, uintptr(loc), f32(a), f32(c), f32(d)) }
func Uniform4f(loc int32, a, c, d, e float32) {
	call(uniform4f, uintptr(loc), f32(a), f32(c), f32(d), f32(e))
}
func Uniform1i(loc int32, v int32) { call(uniform1i, uintptr(loc), uintptr(v)) }
func DrawArrays(mode uint32, first, count int32) {
	call(drawArrays, uintptr(mode), uintptr(first), uintptr(count))
}

func GenTexture() uint32 {
	var id uint32
	call(genTextures, 1, uintptr(unsafe.Pointer(&id)))
	return id
}
func BindTexture(target, id uint32)      { call(bindTexture, uintptr(target), uintptr(id)) }
func DeleteTexture(id uint32)            { call(deleteTextures, 1, uintptr(unsafe.Pointer(&id))) }
func ActiveTexture(t uint32)             { call(activeTexture, uintptr(t)) }
func TexParameteri(t, p uint32, v int32) { call(texParameteri, uintptr(t), uintptr(p), uintptr(v)) }
func PixelStorei(p uint32, v int32)      { call(pixelStorei, uintptr(p), uintptr(v)) }
func TexImage2D(target uint32, level, internal int32, w, h int32, format, typ uint32, data []byte) {
	var p uintptr
	if len(data) > 0 {
		p = uintptr(unsafe.Pointer(&data[0]))
	}
	call(texImage2D, uintptr(target), uintptr(level), uintptr(internal), uintptr(w), uintptr(h), 0, uintptr(format), uintptr(typ), p)
}
func TexSubImage2D(target uint32, level, x, y, w, h int32, format, typ uint32, data []byte) {
	call(texSubImage2D, uintptr(target), uintptr(level), uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(format), uintptr(typ), uintptr(unsafe.Pointer(&data[0])))
}
func ReadPixels(x, y, w, h int32, format, typ uint32, data []byte) {
	call(readPixels, uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(format), uintptr(typ), uintptr(unsafe.Pointer(&data[0])))
}

func GenFramebuffer() uint32 {
	var id uint32
	call(genFramebuffers, 1, uintptr(unsafe.Pointer(&id)))
	return id
}
func BindFramebuffer(target, id uint32) { call(bindFramebuffer, uintptr(target), uintptr(id)) }
func DeleteFramebuffer(id uint32)       { call(deleteFramebuffers, 1, uintptr(unsafe.Pointer(&id))) }
func FramebufferRenderbuffer(target, attach, rbt, rb uint32) {
	call(framebufferRenderbuffer, uintptr(target), uintptr(attach), uintptr(rbt), uintptr(rb))
}
func FramebufferTexture2D(target, attach, textarget, tex uint32) {
	call(framebufferTexture2D, uintptr(target), uintptr(attach), uintptr(textarget), uintptr(tex), 0)
}
func CheckFramebufferStatus(target uint32) uint32 {
	return uint32(call(checkFramebufferStatus, uintptr(target)))
}
func GenRenderbuffer() uint32 {
	var id uint32
	call(genRenderbuffers, 1, uintptr(unsafe.Pointer(&id)))
	return id
}
func BindRenderbuffer(target, id uint32) { call(bindRenderbuffer, uintptr(target), uintptr(id)) }
func DeleteRenderbuffer(id uint32)       { call(deleteRenderbuffers, 1, uintptr(unsafe.Pointer(&id))) }
func RenderbufferStorageMultisample(target uint32, samples int32, format uint32, w, h int32) {
	call(renderbufferStorageMultisample, uintptr(target), uintptr(samples), uintptr(format), uintptr(w), uintptr(h))
}
func RenderbufferStorage(target, format uint32, w, h int32) {
	call(renderbufferStorage, uintptr(target), uintptr(format), uintptr(w), uintptr(h))
}
func BlitFramebuffer(sx0, sy0, sx1, sy1, dx0, dy0, dx1, dy1 int32, mask, filter uint32) {
	call(blitFramebuffer, uintptr(sx0), uintptr(sy0), uintptr(sx1), uintptr(sy1), uintptr(dx0), uintptr(dy0), uintptr(dx1), uintptr(dy1), uintptr(mask), uintptr(filter))
}
