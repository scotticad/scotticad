//go:build linux

// The Linux binding calls the driver through purego, with typed function
// signatures so float arguments travel the way the platform's C calling
// convention expects.

package gl

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/scotticad/scotticad/pkg/window"
)

var (
	glViewport                       func(x, y, w, h int32)
	glClearColor                     func(r, g, b, a float32)
	glClear                          func(mask uint32)
	glEnable                         func(c uint32)
	glDisable                        func(c uint32)
	glBlendFunc                      func(s, d uint32)
	glDepthFunc                      func(f uint32)
	glDepthMask                      func(v uint8)
	glScissor                        func(x, y, w, h int32)
	glGetString                      func(name uint32) uintptr
	glGetIntegerv                    func(name uint32, v *int32)
	glGenBuffers                     func(n int32, ids *uint32)
	glBindBuffer                     func(target, id uint32)
	glBufferData                     func(target uint32, size int, data unsafe.Pointer, usage uint32)
	glDeleteBuffers                  func(n int32, ids *uint32)
	glGenVertexArrays                func(n int32, ids *uint32)
	glBindVertexArray                func(id uint32)
	glDeleteVertexArrays             func(n int32, ids *uint32)
	glEnableVertexAttribArray        func(i uint32)
	glVertexAttribPointer            func(i uint32, size int32, typ uint32, norm uint8, stride int32, offset uintptr)
	glCreateShader                   func(kind uint32) uint32
	glShaderSource                   func(s uint32, n int32, src **byte, lens *int32)
	glCompileShader                  func(s uint32)
	glGetShaderiv                    func(s, p uint32, v *int32)
	glGetShaderInfoLog               func(s uint32, size int32, n *int32, buf *byte)
	glCreateProgram                  func() uint32
	glAttachShader                   func(p, s uint32)
	glLinkProgram                    func(p uint32)
	glGetProgramiv                   func(p, q uint32, v *int32)
	glGetProgramInfoLog              func(p uint32, size int32, n *int32, buf *byte)
	glUseProgram                     func(p uint32)
	glGetUniformLocation             func(p uint32, name *byte) int32
	glBindAttribLocation             func(p, i uint32, name *byte)
	glUniformMatrix4fv               func(loc, count int32, transpose uint8, m *float32)
	glUniform1f                      func(loc int32, v float32)
	glUniform2f                      func(loc int32, a, b float32)
	glUniform3f                      func(loc int32, a, b, c float32)
	glUniform4f                      func(loc int32, a, b, c, d float32)
	glUniform1i                      func(loc, v int32)
	glDrawArrays                     func(mode uint32, first, count int32)
	glGenTextures                    func(n int32, ids *uint32)
	glBindTexture                    func(target, id uint32)
	glDeleteTextures                 func(n int32, ids *uint32)
	glActiveTexture                  func(t uint32)
	glTexParameteri                  func(t, p uint32, v int32)
	glPixelStorei                    func(p uint32, v int32)
	glTexImage2D                     func(target uint32, level, internal, w, h, border int32, format, typ uint32, data unsafe.Pointer)
	glReadPixels                     func(x, y, w, h int32, format, typ uint32, data unsafe.Pointer)
	glGenFramebuffers                func(n int32, ids *uint32)
	glBindFramebuffer                func(target, id uint32)
	glDeleteFramebuffers             func(n int32, ids *uint32)
	glFramebufferRenderbuffer        func(target, attach, rbt, rb uint32)
	glFramebufferTexture2D           func(target, attach, textarget, tex uint32, level int32)
	glGenRenderbuffers               func(n int32, ids *uint32)
	glBindRenderbuffer               func(target, id uint32)
	glDeleteRenderbuffers            func(n int32, ids *uint32)
	glRenderbufferStorageMultisample func(target uint32, samples int32, format uint32, w, h int32)
	glRenderbufferStorage            func(target, format uint32, w, h int32)
	glBlitFramebuffer                func(sx0, sy0, sx1, sy1, dx0, dy0, dx1, dy1 int32, mask, filter uint32)
)

// Init resolves all entry points. The GL context must be current.
func Init() error {
	fns := map[string]any{
		"glViewport": &glViewport, "glClearColor": &glClearColor, "glClear": &glClear, "glEnable": &glEnable,
		"glDisable": &glDisable, "glBlendFunc": &glBlendFunc, "glDepthFunc": &glDepthFunc, "glDepthMask": &glDepthMask,
		"glScissor": &glScissor, "glGetString": &glGetString, "glGetIntegerv": &glGetIntegerv,
		"glGenBuffers": &glGenBuffers, "glBindBuffer": &glBindBuffer, "glBufferData": &glBufferData,
		"glDeleteBuffers": &glDeleteBuffers, "glGenVertexArrays": &glGenVertexArrays, "glBindVertexArray": &glBindVertexArray,
		"glDeleteVertexArrays": &glDeleteVertexArrays, "glEnableVertexAttribArray": &glEnableVertexAttribArray,
		"glVertexAttribPointer": &glVertexAttribPointer, "glCreateShader": &glCreateShader, "glShaderSource": &glShaderSource,
		"glCompileShader": &glCompileShader, "glGetShaderiv": &glGetShaderiv, "glGetShaderInfoLog": &glGetShaderInfoLog,
		"glCreateProgram": &glCreateProgram, "glAttachShader": &glAttachShader, "glLinkProgram": &glLinkProgram,
		"glGetProgramiv": &glGetProgramiv, "glGetProgramInfoLog": &glGetProgramInfoLog, "glUseProgram": &glUseProgram,
		"glGetUniformLocation": &glGetUniformLocation, "glBindAttribLocation": &glBindAttribLocation,
		"glUniformMatrix4fv": &glUniformMatrix4fv, "glUniform1f": &glUniform1f, "glUniform2f": &glUniform2f,
		"glUniform3f": &glUniform3f, "glUniform4f": &glUniform4f, "glUniform1i": &glUniform1i, "glDrawArrays": &glDrawArrays,
		"glGenTextures": &glGenTextures, "glBindTexture": &glBindTexture, "glDeleteTextures": &glDeleteTextures,
		"glActiveTexture": &glActiveTexture, "glTexParameteri": &glTexParameteri, "glPixelStorei": &glPixelStorei,
		"glTexImage2D": &glTexImage2D, "glReadPixels": &glReadPixels,
		"glGenFramebuffers": &glGenFramebuffers, "glBindFramebuffer": &glBindFramebuffer,
		"glDeleteFramebuffers": &glDeleteFramebuffers, "glFramebufferRenderbuffer": &glFramebufferRenderbuffer,
		"glFramebufferTexture2D": &glFramebufferTexture2D, "glGenRenderbuffers": &glGenRenderbuffers,
		"glBindRenderbuffer":    &glBindRenderbuffer,
		"glDeleteRenderbuffers": &glDeleteRenderbuffers, "glRenderbufferStorageMultisample": &glRenderbufferStorageMultisample,
		"glRenderbufferStorage": &glRenderbufferStorage, "glBlitFramebuffer": &glBlitFramebuffer,
	}
	var missing []string
	for name, f := range fns {
		p := window.GetProcAddress(name)
		if p == 0 {
			missing = append(missing, name)
			continue
		}
		purego.RegisterFunc(f, p)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("OpenGL 3.3 not available (missing %v); version %q", missing, GetString(VERSION))
	}
	// GLX resolves any name, even one the driver lacks: only the version
	// string tells whether 3.3 is really there
	if v := GetString(VERSION); !atLeast33(v) {
		return fmt.Errorf("OpenGL 3.3 not available (the driver offers %q); "+
			"software rendering may work: run with LIBGL_ALWAYS_SOFTWARE=1", v)
	}
	return nil
}

// atLeast33 reports a GL_VERSION string of 3.3 or later ("4.5 (Compatibility
// Profile) Mesa 25.2", "3.3.0 NVIDIA ...").
func atLeast33(v string) bool {
	maj, rest, ok := strings.Cut(v, ".")
	if !ok {
		return false
	}
	mi := rest
	if i := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
		mi = rest[:i]
	}
	a, err1 := strconv.Atoi(maj)
	b, err2 := strconv.Atoi(mi)
	return err1 == nil && err2 == nil && (a > 3 || a == 3 && b >= 3)
}

func b8(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}

func Viewport(x, y, w, h int32)      { glViewport(x, y, w, h) }
func ClearColor(r, g, bl, a float32) { glClearColor(r, g, bl, a) }
func Clear(mask uint32)              { glClear(mask) }
func Enable(c uint32)                { glEnable(c) }
func Disable(c uint32)               { glDisable(c) }
func BlendFunc(s, d uint32)          { glBlendFunc(s, d) }
func DepthFunc(f uint32)             { glDepthFunc(f) }
func DepthMask(v bool)               { glDepthMask(b8(v)) }
func Scissor(x, y, w, h int32)       { glScissor(x, y, w, h) }

func SwapInterval(n int) { window.SwapInterval(n) }

func GetString(name uint32) string {
	if glGetString == nil { // Init failed before binding it: the error message still wants the version
		p := window.GetProcAddress("glGetString")
		if p == 0 {
			return ""
		}
		purego.RegisterFunc(&glGetString, p)
	}
	p := glGetString(name)
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
	glGetIntegerv(name, &v)
	return v
}

func GenBuffer() uint32 {
	var id uint32
	glGenBuffers(1, &id)
	return id
}
func BindBuffer(target, id uint32) { glBindBuffer(target, id) }
func BufferData(target uint32, data []float32, usage uint32) {
	var p unsafe.Pointer
	if len(data) > 0 {
		p = unsafe.Pointer(&data[0])
	}
	glBufferData(target, len(data)*4, p, usage)
}
func DeleteBuffer(id uint32) { glDeleteBuffers(1, &id) }

func GenVertexArray() uint32 {
	var id uint32
	glGenVertexArrays(1, &id)
	return id
}
func BindVertexArray(id uint32)        { glBindVertexArray(id) }
func DeleteVertexArray(id uint32)      { glDeleteVertexArrays(1, &id) }
func EnableVertexAttribArray(i uint32) { glEnableVertexAttribArray(i) }
func VertexAttribPointer(i uint32, size int32, typ uint32, norm bool, stride int32, offset int) {
	glVertexAttribPointer(i, size, typ, b8(norm), stride, uintptr(offset))
}

func cstr(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

func compile(kind uint32, src string) (uint32, error) {
	s := glCreateShader(kind)
	cs := cstr(src)
	glShaderSource(s, 1, &cs, nil)
	glCompileShader(s)
	var ok int32
	glGetShaderiv(s, COMPILE_STATUS, &ok)
	if ok == 0 {
		buf := make([]byte, 4096)
		var n int32
		glGetShaderInfoLog(s, 4096, &n, &buf[0])
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
	p := glCreateProgram()
	glAttachShader(p, v)
	glAttachShader(p, f)
	for i, a := range attribs {
		glBindAttribLocation(p, uint32(i), cstr(a))
	}
	glLinkProgram(p)
	var ok int32
	glGetProgramiv(p, LINK_STATUS, &ok)
	if ok == 0 {
		buf := make([]byte, 4096)
		var n int32
		glGetProgramInfoLog(p, 4096, &n, &buf[0])
		return 0, fmt.Errorf("program link: %s", buf[:n])
	}
	return p, nil
}

func UseProgram(p uint32)                     { glUseProgram(p) }
func Uniform(p uint32, name string) int32     { return glGetUniformLocation(p, cstr(name)) }
func UniformMatrix4(loc int32, m [16]float32) { glUniformMatrix4fv(loc, 1, 0, &m[0]) }
func Uniform1f(loc int32, v float32)          { glUniform1f(loc, v) }
func Uniform2f(loc int32, a, c float32)       { glUniform2f(loc, a, c) }
func Uniform3f(loc int32, a, c, d float32)    { glUniform3f(loc, a, c, d) }
func Uniform4f(loc int32, a, c, d, e float32) { glUniform4f(loc, a, c, d, e) }
func Uniform1i(loc int32, v int32)            { glUniform1i(loc, v) }
func DrawArrays(mode uint32, first, count int32) {
	glDrawArrays(mode, first, count)
}

func GenTexture() uint32 {
	var id uint32
	glGenTextures(1, &id)
	return id
}
func BindTexture(target, id uint32)      { glBindTexture(target, id) }
func DeleteTexture(id uint32)            { glDeleteTextures(1, &id) }
func ActiveTexture(t uint32)             { glActiveTexture(t) }
func TexParameteri(t, p uint32, v int32) { glTexParameteri(t, p, v) }
func PixelStorei(p uint32, v int32)      { glPixelStorei(p, v) }
func TexImage2D(target uint32, level, internal int32, w, h int32, format, typ uint32, data []byte) {
	var p unsafe.Pointer
	if len(data) > 0 {
		p = unsafe.Pointer(&data[0])
	}
	glTexImage2D(target, level, internal, w, h, 0, format, typ, p)
}
func ReadPixels(x, y, w, h int32, format, typ uint32, data []byte) {
	glReadPixels(x, y, w, h, format, typ, unsafe.Pointer(&data[0]))
}

func GenFramebuffer() uint32 {
	var id uint32
	glGenFramebuffers(1, &id)
	return id
}
func BindFramebuffer(target, id uint32) { glBindFramebuffer(target, id) }
func DeleteFramebuffer(id uint32)       { glDeleteFramebuffers(1, &id) }
func FramebufferRenderbuffer(target, attach, rbt, rb uint32) {
	glFramebufferRenderbuffer(target, attach, rbt, rb)
}
func FramebufferTexture2D(target, attach, textarget, tex uint32) {
	glFramebufferTexture2D(target, attach, textarget, tex, 0)
}
func GenRenderbuffer() uint32 {
	var id uint32
	glGenRenderbuffers(1, &id)
	return id
}
func BindRenderbuffer(target, id uint32) { glBindRenderbuffer(target, id) }
func DeleteRenderbuffer(id uint32)       { glDeleteRenderbuffers(1, &id) }
func RenderbufferStorageMultisample(target uint32, samples int32, format uint32, w, h int32) {
	glRenderbufferStorageMultisample(target, samples, format, w, h)
}
func RenderbufferStorage(target, format uint32, w, h int32) {
	glRenderbufferStorage(target, format, w, h)
}
func BlitFramebuffer(sx0, sy0, sx1, sy1, dx0, dy0, dx1, dy1 int32, mask, filter uint32) {
	glBlitFramebuffer(sx0, sy0, sx1, sy1, dx0, dy0, dx1, dy1, mask, filter)
}
