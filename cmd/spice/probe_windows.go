//go:build windows

package main

import "syscall"

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	setErrorMode = kernel32.NewProc("SetErrorMode")
)

const (
	semFailCriticalErrors = 0x0001
	semNoGPFaultErrorBox  = 0x0002
)

// suppressCrashDialogs tells Windows not to show "has stopped working" dialogs
// for this process. Used by the -probe-gl subprocess so that when Fyne's
// OpenGL initialization fails and calls os.Exit(1), the user never sees
// a crash dialog for the hidden probe process.
func suppressCrashDialogs() {
	_, _, _ = setErrorMode.Call(uintptr(semFailCriticalErrors | semNoGPFaultErrorBox))
}
