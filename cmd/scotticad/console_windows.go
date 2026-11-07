package main

import (
	"os"
	"sync"
	"syscall"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	pAttachConsole      = kernel32.NewProc("AttachConsole")
	pGetFileType        = kernel32.NewProc("GetFileType")
	attachConsoleOnce   sync.Once
	attachParentProcess = ^uintptr(0) // ATTACH_PARENT_PROCESS (-1)
)

// useConsole makes standard output and error reach the terminal that
// started ScottiCAD, for --help, --version and error messages. The release
// build is a GUI program (-H windowsgui), so started from cmd or
// PowerShell it has neither, and what it prints is lost. A handle that
// already works (a pipe from an MCP client or a test, a file it was
// redirected to, Git Bash's pipes) is kept.
//
// The terminal doesn't wait for a GUI program, so its prompt may already
// be showing when the text appears.
func useConsole() {
	attachConsoleOnce.Do(func() {
		outOK, errOK := usable(os.Stdout), usable(os.Stderr)
		if outOK && errOK {
			return
		}
		if r, _, _ := pAttachConsole.Call(attachParentProcess); r == 0 {
			return // started from Explorer, a shortcut: no terminal
		}
		if !outOK {
			if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
				os.Stdout = f
			}
		}
		if !errOK {
			if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
				os.Stderr = f
			}
		}
	})
}

// usable reports whether f is a real handle: a console, file or pipe.
// A GUI program started without one gets 0, which GetFileType calls
// FILE_TYPE_UNKNOWN (0).
func usable(f *os.File) bool {
	if f == nil {
		return false
	}
	t, _, _ := pGetFileType.Call(f.Fd())
	return t != 0
}
