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
)

// SetWallpaper sets the desktop wallpaper
func (w *windowsOS) SetWallpaper(imagePath string) error {
	imagePathUTF16, err := syscall.UTF16PtrFromString(imagePath)
	if err != nil {
		return err
	}

	ret, _, _ := systemParametersInfo.Call(
		uintptr(SPISetDeskWallpaper),
		uintptr(0),
		uintptr(unsafe.Pointer(imagePathUTF16)),
		uintptr(SPIFUpdateIniFile|SPIFSendWinIniChange),
	)
	if ret == 0 {
		return syscall.GetLastError()
	}
	return nil
}

// GetDesktopDimension returns the resolution of the primary monitor
func (w *windowsOS) GetDesktopDimension() (int, int, error) {
	return sysinfo.GetScreenDimensions()
}

// getOS returns a new instance of the windowsOS struct.
func getOS() OS {
	return &windowsOS{}
}
