//go:build windows
// +build windows

package sysinfo

import (
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	getSystemMetrics = user32.NewProc("GetSystemMetrics")
)

const (
	SMCXScreen = 0
	SMCYScreen = 1
)

// GetScreenDimensions returns the primary desktop dimension (width and height) in pixels.
func GetScreenDimensions() (int, int, error) {
	var width, height uintptr
	var err error

	width, _, err = getSystemMetrics.Call(uintptr(SMCXScreen))
	if err != windows.NOERROR {
		return 0, 0, err
	}
	height, _, err = getSystemMetrics.Call(uintptr(SMCYScreen))
	if err != windows.NOERROR {
		return 0, 0, err
	}

	return int(width), int(height), nil
}
