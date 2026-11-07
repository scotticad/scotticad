//go:build !windows

package main

// useConsole is for the Windows GUI build; elsewhere standard output and
// error are the terminal's already.
func useConsole() {}
