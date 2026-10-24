// Package gl is a small hand-written OpenGL 3.3 binding with no cgo. Only the
// entry points ScottiCAD uses are bound. On Windows it calls the driver
// through syscall; on Linux through purego (see gl_linux.go).
package gl

const (
	DEPTH_BUFFER_BIT     = 0x00000100
	COLOR_BUFFER_BIT     = 0x00004000
	TRIANGLES            = 0x0004
	LINES                = 0x0001
	DEPTH_TEST           = 0x0B71
	BLEND                = 0x0BE2
	CULL_FACE            = 0x0B44
	SCISSOR_TEST         = 0x0C11
	SRC_ALPHA            = 0x0302
	ONE_MINUS_SRC_ALPHA  = 0x0303
	LEQUAL               = 0x0203
	FLOAT                = 0x1406
	UNSIGNED_BYTE        = 0x1401
	ARRAY_BUFFER         = 0x8892
	STATIC_DRAW          = 0x88E4
	DYNAMIC_DRAW         = 0x88E8
	FRAGMENT_SHADER      = 0x8B30
	VERTEX_SHADER        = 0x8B31
	COMPILE_STATUS       = 0x8B81
	LINK_STATUS          = 0x8B82
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
	UNPACK_ALIGNMENT     = 0x0CF5
	PACK_ALIGNMENT       = 0x0D05
	FRAMEBUFFER          = 0x8D40
	READ_FRAMEBUFFER     = 0x8CA8
	DRAW_FRAMEBUFFER     = 0x8CA9
	RENDERBUFFER         = 0x8D41
	COLOR_ATTACHMENT0    = 0x8CE0
	DEPTH_STENCIL_ATTACH = 0x821A
	DEPTH24_STENCIL8     = 0x88F0
	VERSION              = 0x1F02
	MAX_SAMPLES          = 0x8D57
)
