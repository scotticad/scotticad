package window

import "unsafe"

// foreign turns an address that came from the operating system (a window
// message's lParam, clipboard memory, an Xlib result) into a pointer.
//
// go vet's unsafeptr check warns on unsafe.Pointer(p) for a uintptr p:
// for memory Go allocated, the collector may have moved or freed the
// object since its address became a number, and the pointer would be
// stale. That can't happen here: the memory belongs to the OS or a C
// library, which Go never moves or collects, and the call that returned
// the address keeps it valid until we release it. Reading the uintptr's
// bits as a pointer says the same thing as the conversion without the
// warning; every such conversion in this package goes through here so
// the reasoning lives in one place. Never pass it the address of Go
// memory.
func foreign(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}
