//go:build windows

// The Windows binding calls the driver through syscall. Float arguments are
// passed as their bit patterns; the Windows x64 syscall trampoline mirrors
// integer argument registers into XMM registers, so this works for float32
// parameters.

package gl

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"

	"github.com/scotticad/scotticad/pkg/window"
)

type fn uintptr

var procs = map[string]*fn{}

var (
	viewport, clearColor, clear, enable, disable, blendFunc, depthFunc, depthMask,
	genBuffers, bindBuffer, bufferData, deleteBuffers, genVertexArrays,
	bindVertexArray, deleteVertexArrays, enableVertexAttribArray, vertexAttribPointer,
	createShader, shaderSource, compileShader, getShaderiv, getShaderInfoLog, createProgram,
	attachShader, linkProgram, getProgramiv, getProgramInfoLog, useProgram, getUniformLocation,
	uniformMatrix4fv, uniform1f, uniform2f, uniform3f, uniform4f, uniform1i, drawArrays,
	genTextures, bindTexture, texImage2D, texParameteri, activeTexture,
	pixelStorei, readPixels, genFramebuffers, bindFramebuffer, framebufferRenderbuffer,
	framebufferTexture2D, genRenderbuffers, bindRenderbuffer, renderbufferStorageMultisample,
	renderbufferStorage, blitFramebuffer, deleteFramebuffers, deleteRenderbuffers,
	deleteTextures, scissor, getString, getIntegerv,
	bindAttribLocation, swapInterval fn
)

func init() {
	for name, p := range map[string]*fn{
		"glViewport": &viewport, "glClearColor": &clearColor, "glClear": &clear, "glEnable": &enable,
		"glDisable": &disable, "glBlendFunc": &blendFunc, "glDepthFunc": &depthFunc, "glDepthMask": &depthMask,
		"glGenBuffers": &genBuffers, "glBindBuffer": &bindBuffer,
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
		"glTexParameteri": &texParameteri, "glActiveTexture": &activeTexture,
		"glPixelStorei": &pixelStorei, "glReadPixels": &readPixels, "glGenFramebuffers": &genFramebuffers,
		"glBindFramebuffer": &bindFramebuffer, "glFramebufferRenderbuffer": &framebufferRenderbuffer,
		"glFramebufferTexture2D": &framebufferTexture2D, "glGenRenderbuffers": &genRenderbuffers,
		"glBindRenderbuffer": &bindRenderbuffer, "glRenderbufferStorageMultisample": &renderbufferStorageMultisample,
		"glRenderbufferStorage": &renderbufferStorage, "glBlitFramebuffer": &blitFramebuffer,
		"glDeleteFramebuffers": &deleteFramebuffers, "glDeleteRenderbuffers": &deleteRenderbuffers,
		"glDeleteTextures": &deleteTextures, "glScissor": &scissor, "glGetString": &getString,
		"glGetIntegerv": &getIntegerv, "glBindAttribLocation": &bindAttribLocation,
		"wglSwapIntervalEXT": &swapInterval,
	} {
		procs[name] = p
	}
}

// Init resolves all entry points. The GL context must be current.
func Init() error {
	var missing []string
	for name, p := range procs {
		*p = fn(window.GetProcAddress(name))
		if *p == 0 && name != "wglSwapIntervalEXT" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("OpenGL 3.3 not available (missing %v); version %s", missing, GetString(VERSION))
	}
	return nil
}

// call is for arguments that are plain numbers. A call passing a pointer
// goes to syscall.SyscallN itself, with the uintptr(unsafe.Pointer(...))
// conversion in its argument list: only there does the compiler keep the
// object alive and in place (a stack variable could move if the stack
// grew in this wrapper).
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

func Viewport(x, y, w, h int32)      { call(viewport, uintptr(x), uintptr(y), uintptr(w), uintptr(h)) }
func ClearColor(r, g, bl, a float32) { call(clearColor, f32(r), f32(g), f32(bl), f32(a)) }
func Clear(mask uint32)              { call(clear, uintptr(mask)) }
func Enable(c uint32)                { call(enable, uintptr(c)) }
func Disable(c uint32)               { call(disable, uintptr(c)) }
func BlendFunc(s, d uint32)          { call(blendFunc, uintptr(s), uintptr(d)) }
func DepthFunc(f uint32)             { call(depthFunc, uintptr(f)) }
func DepthMask(v bool)               { call(depthMask, b(v)) }
func Scissor(x, y, w, h int32)       { call(scissor, uintptr(x), uintptr(y), uintptr(w), uintptr(h)) }

func SwapInterval(n int) {
	if swapInterval != 0 {
		call(swapInterval, uintptr(n))
	}
}

func GetString(name uint32) string {
	if getString == 0 {
		getString = fn(window.GetProcAddress("glGetString"))
	}
	p := call(getString, uintptr(name))
	if p == 0 {
		return ""
	}
	var s []byte
	for i := uintptr(0); ; i++ {
		c := *(*byte)(foreign(p + i))
		if c == 0 {
			break
		}
		s = append(s, c)
	}
	return string(s)
}

func GetInteger(name uint32) int32 {
	var v int32
	syscall.SyscallN(uintptr(getIntegerv), uintptr(name), uintptr(unsafe.Pointer(&v)))
	return v
}

func GenBuffer() uint32 {
	var id uint32
	syscall.SyscallN(uintptr(genBuffers), 1, uintptr(unsafe.Pointer(&id)))
	return id
}
func BindBuffer(target, id uint32) { call(bindBuffer, uintptr(target), uintptr(id)) }
func BufferData(target uint32, data []float32, usage uint32) {
	if len(data) == 0 {
		call(bufferData, uintptr(target), 0, 0, uintptr(usage))
		return
	}
	syscall.SyscallN(uintptr(bufferData), uintptr(target), uintptr(len(data)*4), uintptr(unsafe.Pointer(&data[0])), uintptr(usage))
}
func DeleteBuffer(id uint32) {
	syscall.SyscallN(uintptr(deleteBuffers), 1, uintptr(unsafe.Pointer(&id)))
}

func GenVertexArray() uint32 {
	var id uint32
	syscall.SyscallN(uintptr(genVertexArrays), 1, uintptr(unsafe.Pointer(&id)))
	return id
}
func BindVertexArray(id uint32) { call(bindVertexArray, uintptr(id)) }
func DeleteVertexArray(id uint32) {
	syscall.SyscallN(uintptr(deleteVertexArrays), 1, uintptr(unsafe.Pointer(&id)))
}
func EnableVertexAttribArray(i uint32) { call(enableVertexAttribArray, uintptr(i)) }
func VertexAttribPointer(i uint32, size int32, typ uint32, norm bool, stride int32, offset int) {
	call(vertexAttribPointer, uintptr(i), uintptr(size), uintptr(typ), b(norm), uintptr(stride), uintptr(offset))
}

func cstr(s string) *byte { p, _ := syscall.BytePtrFromString(s); return p }

func compile(kind uint32, src string) (uint32, error) {
	s := uint32(call(createShader, uintptr(kind)))
	cs := cstr(src)
	syscall.SyscallN(uintptr(shaderSource), uintptr(s), 1, uintptr(unsafe.Pointer(&cs)), 0)
	call(compileShader, uintptr(s))
	var ok int32
	syscall.SyscallN(uintptr(getShaderiv), uintptr(s), COMPILE_STATUS, uintptr(unsafe.Pointer(&ok)))
	if ok == 0 {
		buf := make([]byte, 4096)
		var n int32
		syscall.SyscallN(uintptr(getShaderInfoLog), uintptr(s), 4096, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
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
		syscall.SyscallN(uintptr(bindAttribLocation), uintptr(p), uintptr(i), uintptr(unsafe.Pointer(cstr(a))))
	}
	call(linkProgram, uintptr(p))
	var ok int32
	syscall.SyscallN(uintptr(getProgramiv), uintptr(p), LINK_STATUS, uintptr(unsafe.Pointer(&ok)))
	if ok == 0 {
		buf := make([]byte, 4096)
		var n int32
		syscall.SyscallN(uintptr(getProgramInfoLog), uintptr(p), 4096, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])))
		return 0, fmt.Errorf("program link: %s", buf[:n])
	}
	return p, nil
}

func UseProgram(p uint32) { call(useProgram, uintptr(p)) }
func Uniform(p uint32, name string) int32 {
	r, _, _ := syscall.SyscallN(uintptr(getUniformLocation), uintptr(p), uintptr(unsafe.Pointer(cstr(name))))
	return int32(r)
}
func UniformMatrix4(loc int32, m [16]float32) {
	syscall.SyscallN(uintptr(uniformMatrix4fv), uintptr(loc), 1, 0, uintptr(unsafe.Pointer(&m[0])))
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
	syscall.SyscallN(uintptr(genTextures), 1, uintptr(unsafe.Pointer(&id)))
	return id
}
func BindTexture(target, id uint32) { call(bindTexture, uintptr(target), uintptr(id)) }
func DeleteTexture(id uint32) {
	syscall.SyscallN(uintptr(deleteTextures), 1, uintptr(unsafe.Pointer(&id)))
}
func ActiveTexture(t uint32)             { call(activeTexture, uintptr(t)) }
func TexParameteri(t, p uint32, v int32) { call(texParameteri, uintptr(t), uintptr(p), uintptr(v)) }
func PixelStorei(p uint32, v int32)      { call(pixelStorei, uintptr(p), uintptr(v)) }
func TexImage2D(target uint32, level, internal int32, w, h int32, format, typ uint32, data []byte) {
	if len(data) == 0 { // storage only, no pixels
		call(texImage2D, uintptr(target), uintptr(level), uintptr(internal), uintptr(w), uintptr(h), 0, uintptr(format), uintptr(typ), 0)
		return
	}
	syscall.SyscallN(uintptr(texImage2D), uintptr(target), uintptr(level), uintptr(internal), uintptr(w), uintptr(h), 0, uintptr(format), uintptr(typ), uintptr(unsafe.Pointer(&data[0])))
}
func ReadPixels(x, y, w, h int32, format, typ uint32, data []byte) {
	syscall.SyscallN(uintptr(readPixels), uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(format), uintptr(typ), uintptr(unsafe.Pointer(&data[0])))
}

func GenFramebuffer() uint32 {
	var id uint32
	syscall.SyscallN(uintptr(genFramebuffers), 1, uintptr(unsafe.Pointer(&id)))
	return id
}
func BindFramebuffer(target, id uint32) { call(bindFramebuffer, uintptr(target), uintptr(id)) }
func DeleteFramebuffer(id uint32) {
	syscall.SyscallN(uintptr(deleteFramebuffers), 1, uintptr(unsafe.Pointer(&id)))
}
func FramebufferRenderbuffer(target, attach, rbt, rb uint32) {
	call(framebufferRenderbuffer, uintptr(target), uintptr(attach), uintptr(rbt), uintptr(rb))
}
func FramebufferTexture2D(target, attach, textarget, tex uint32) {
	call(framebufferTexture2D, uintptr(target), uintptr(attach), uintptr(textarget), uintptr(tex), 0)
}
func GenRenderbuffer() uint32 {
	var id uint32
	syscall.SyscallN(uintptr(genRenderbuffers), 1, uintptr(unsafe.Pointer(&id)))
	return id
}
func BindRenderbuffer(target, id uint32) { call(bindRenderbuffer, uintptr(target), uintptr(id)) }
func DeleteRenderbuffer(id uint32) {
	syscall.SyscallN(uintptr(deleteRenderbuffers), 1, uintptr(unsafe.Pointer(&id)))
}
func RenderbufferStorageMultisample(target uint32, samples int32, format uint32, w, h int32) {
	call(renderbufferStorageMultisample, uintptr(target), uintptr(samples), uintptr(format), uintptr(w), uintptr(h))
}
func RenderbufferStorage(target, format uint32, w, h int32) {
	call(renderbufferStorage, uintptr(target), uintptr(format), uintptr(w), uintptr(h))
}
func BlitFramebuffer(sx0, sy0, sx1, sy1, dx0, dy0, dx1, dy1 int32, mask, filter uint32) {
	call(blitFramebuffer, uintptr(sx0), uintptr(sy0), uintptr(sx1), uintptr(sy1), uintptr(dx0), uintptr(dy0), uintptr(dx1), uintptr(dy1), uintptr(mask), uintptr(filter))
}
