//go:build windows
// +build windows

package wallpaper

import (
	"syscall"
	"unsafe"

	"github.com/dixieflatline76/Spice/pkg/sysinfo"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	systemParametersInfo = user32.NewProc("SystemParametersInfoW")
	getSystemMetrics     = user32.NewProc("GetSystemMetrics")
)

func init() {
	if user32 == nil {
		panic("Failed to load user32.dll")
	}
}

// windowsOS implements the OS interface for Windows.
type windowsOS struct{}

// Windows API constants (defined manually)
const (
	SPISetDeskWallpaper  = 0x0014
	SPIFUpdateIniFile    = 0x01
	SPIFSendChange       = 0x02
	SPIFSendWinIniChange = 0x02
	SMCXScreen           = 0
	SMCYScreen           = 1
)

// setWallpaper sets the wallpaper to the given image file path.
func (w *windowsOS) setWallpaper(imagePath string) error {
	// Convert the image path to UTF-16
	imagePathUTF16, err := syscall.UTF16PtrFromString(imagePath) // Convert the image path to UTF-16
	if err != nil {
		return err
	}

	ret, _, err := systemParametersInfo.Call(
		uintptr(SPISetDeskWallpaper),
		uintptr(0),
		uintptr(unsafe.Pointer(imagePathUTF16)),
		uintptr(SPIFUpdateIniFile|SPIFSendChange),
	)
	if ret == 0 {
		return err
	}

	return nil
}

// getDesktopDimension returns the desktop dimension (width and height) in pixels.
func (w *windowsOS) getDesktopDimension() (int, int, error) {
	return sysinfo.GetScreenDimensions()
}

// getOS returns a new instance of the windowsOS struct.
func getOS() OS {
	return &windowsOS{}
}
