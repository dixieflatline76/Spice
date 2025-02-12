//go:build windows
// +build windows

package service

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/config"

	"golang.org/x/sys/windows"
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

// getWallpaperService returns the singleton instance of wallpaperService.
func getWallpaperService(cfg *config.Config) *wallpaperService {
	once.Do(func() {
		// Initialize the wallpaper service for Windows
		currentOS := &windowsOS{}
		p := cfg.GetPreferences()

		// Initialize the wallpaper service
		wsInstance = &wallpaperService{
			os:              currentOS,                                                                             // Initialize with Windows OS
			imgProcessor:    &smartImageProcessor{os: currentOS, aspectThreshold: 0.9, resampler: imaging.Lanczos}, // Initialize with smartCropper with a lenient threshold
			cfg:             cfg,
			prefs:           p,
			downloadMutex:   sync.Mutex{},
			downloadHistory: make(map[string]ImgSrvcImage),
			currentPage:     1,                                      // Start with the first page,
			fitImage:        p.BoolWithFallback("Smart Fit", false), // Initialize with smart fit preference
		}
	})
	return wsInstance
}
