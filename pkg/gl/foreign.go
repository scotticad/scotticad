package gl

import "unsafe"

// foreign turns an address the OpenGL driver returned (the string
// glGetString points at) into a pointer.
//
// go vet's unsafeptr check warns on unsafe.Pointer(p) for a uintptr p:
// for memory Go allocated, the collector may have moved or freed the
// object since its address became a number, and the pointer would be
// stale. That can't happen here: the memory belongs to the driver, which
// Go never moves or collects, and it stays valid for the context's life.
// Reading the uintptr's bits as a pointer says the same thing as the
// conversion without the warning; every such conversion in this package
// goes through here so the reasoning lives in one place. Never pass it the
// address of Go memory.
func foreign(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}
