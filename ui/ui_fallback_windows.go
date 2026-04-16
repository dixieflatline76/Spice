//go:build windows

package ui

import (
	"syscall"
	"unsafe"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

const (
	mbIconError = 0x00000010
	mbOk        = 0x00000000
)

// ShowNativeFallbackAlert displays a native Windows MessageBox.
// This is used when OpenGL/Fyne initialization fails, allowing us to show
// a human-readable error message without needing a graphics context.
func ShowNativeFallbackAlert(title, message string) {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	messagePtr, _ := syscall.UTF16PtrFromString(message)

	_, _, _ = procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(mbOk|mbIconError),
	)
}
