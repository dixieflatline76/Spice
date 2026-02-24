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
	getDpiForSystem  = user32.NewProc("GetDpiForSystem")
)

const (
	// SMCXScreen is the index for the screen width system metric.
	SMCXScreen = 0
	// SMCYScreen is the index for the screen height system metric.
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

// GetOSDisplayScale returns the OS-level UI scaling factor (e.g. 1.0 for 100%, 1.75 for 175%).
// It safely falls back to 1.0 on older systems or error.
func GetOSDisplayScale() float32 {
	if err := getDpiForSystem.Find(); err != nil {
		return 1.0
	}

	dpi, _, _ := getDpiForSystem.Call()
	if dpi > 0 {
		return float32(dpi) / 96.0
	}
	return 1.0
}
