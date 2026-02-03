//go:build windows
// +build windows

package wallpaper

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"

	"math"

	"github.com/dixieflatline76/Spice/pkg/sysinfo"
)

var (
	modOle32             = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx   = modOle32.NewProc("CoInitializeEx")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")
	procCoUninitialize   = modOle32.NewProc("CoUninitialize")
	procCoTaskMemFree    = modOle32.NewProc("CoTaskMemFree")
)

const (
	COINIT_APARTMENTTHREADED = 0x2
	COINIT_MULTITHREADED     = 0x0
	CLSCTX_LOCAL_SERVER      = 0x4
	CLSCTX_INPROC_SERVER     = 0x1
)

var (
	// CLSID_DesktopWallpaper: {C2CF3110-460E-4fc1-B9D0-8A1C0C9CC4BD}
	CLSID_DesktopWallpaper = GUID{0xC2CF3110, 0x460E, 0x4fc1, [8]byte{0xB9, 0xD0, 0x8A, 0x1C, 0x0C, 0x9C, 0xC4, 0xBD}}
	// IID_IDesktopWallpaper: {B92B56A9-8B55-4E14-9A89-0199BBB6F93B}
	IID_IDesktopWallpaper = GUID{0xB92B56A9, 0x8B55, 0x4E14, [8]byte{0x9A, 0x89, 0x01, 0x99, 0xBB, 0xB6, 0xF9, 0x3B}}
)

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type IDesktopWallpaper struct {
	LpVtbl *IDesktopWallpaperVtbl
}

type IDesktopWallpaperVtbl struct {
	QueryInterface            uintptr
	AddRef                    uintptr
	Release                   uintptr
	SetWallpaper              uintptr
	GetWallpaper              uintptr
	GetMonitorDevicePathAt    uintptr
	GetMonitorDevicePathCount uintptr
	GetMonitorRECT            uintptr
}

type windowsOS struct{}

// GetMonitors returns all connected monitors using IDesktopWallpaper
func (w *windowsOS) GetMonitors() ([]Monitor, error) {
	// Initialize COM
	_ = modOle32.Load() // Ensure loaded
	_, _, _ = procCoInitializeEx.Call(0, COINIT_MULTITHREADED)
	defer func() { _, _, _ = procCoUninitialize.Call() }()

	var wallpaper *IDesktopWallpaper
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&CLSID_DesktopWallpaper)),
		0,
		uintptr(CLSCTX_LOCAL_SERVER|CLSCTX_INPROC_SERVER),
		uintptr(unsafe.Pointer(&IID_IDesktopWallpaper)),
		uintptr(unsafe.Pointer(&wallpaper)),
	)
	if hr != 0 {
		// Fallback to primary if COM fails (e.g. server SKU)
		return w.getPrimaryMonitorFallback()
	}
	defer w.release(wallpaper)

	var count uint32
	// Call GetMonitorDevicePathCount (Index 6)
	hr, _, _ = syscall.SyscallN(
		wallpaper.LpVtbl.GetMonitorDevicePathCount,
		uintptr(unsafe.Pointer(wallpaper)),
		uintptr(unsafe.Pointer(&count)),
	)
	if hr != 0 {
		return w.getPrimaryMonitorFallback()
	}

	var monitors []Monitor
	for i := uint32(0); i < count; i++ {
		var rect image.Rectangle
		var winRect struct {
			Left, Top, Right, Bottom int32
		}

		// Call GetMonitorRECT (Index 7)
		// Signature: HRESULT GetMonitorRECT([in] LPCWSTR monitorID, [out] RECT *displayRect)
		// Note usage: We first need the MonitorID (String) to get the RECT.
		// So we call GetMonitorDevicePathAt(i) -> String -> GetMonitorRECT(String)

		var monitorIDPtr uintptr
		// Call GetMonitorDevicePathAt (Index 5)
		// Signature: HRESULT GetMonitorDevicePathAt([in] UINT monitorIndex, [out] LPWSTR *monitorID)
		hr, _, _ = syscall.SyscallN(
			wallpaper.LpVtbl.GetMonitorDevicePathAt,
			uintptr(unsafe.Pointer(wallpaper)),
			uintptr(i),
			uintptr(unsafe.Pointer(&monitorIDPtr)),
		)
		if hr != 0 {
			continue
		}

		// Call GetMonitorRECT (Index 7)
		hr, _, _ = syscall.SyscallN(
			wallpaper.LpVtbl.GetMonitorRECT,
			uintptr(unsafe.Pointer(wallpaper)),
			monitorIDPtr,
			uintptr(unsafe.Pointer(&winRect)),
		)

		// Free the string returned by GetMonitorDevicePathAt
		_, _, _ = procCoTaskMemFree.Call(monitorIDPtr)

		if hr == 0 {
			rect = image.Rect(int(winRect.Left), int(winRect.Top), int(winRect.Right), int(winRect.Bottom))
			if rect.Dx() > 0 && rect.Dy() > 0 {
				monitors = append(monitors, Monitor{
					ID:   int(i),
					Name: "", // Let the UI handle the "Display N" labeling
					Rect: rect,
				})
			}
		}
	}

	if len(monitors) == 0 {
		return w.getPrimaryMonitorFallback()
	}

	return monitors, nil
}

func (w *windowsOS) getPrimaryMonitorFallback() ([]Monitor, error) {
	width, height, err := w.GetDesktopDimension()
	if err != nil {
		return nil, err
	}
	return []Monitor{{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, width, height)}}, nil
}

// SetWallpaper sets the desktop wallpaper
// TODO(Stage 6): Use IDesktopWallpaper for per-monitor support (monitorID)
func (w *windowsOS) SetWallpaper(imagePath string, monitorID int) error {
	imagePathUTF16, err := syscall.UTF16PtrFromString(imagePath)
	if err != nil {
		return err
	}

	// Initialize COM
	_, _, _ = procCoInitializeEx.Call(0, COINIT_MULTITHREADED)
	defer func() { _, _, _ = procCoUninitialize.Call() }()

	var wallpaper *IDesktopWallpaper
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&CLSID_DesktopWallpaper)),
		0,
		uintptr(CLSCTX_LOCAL_SERVER|CLSCTX_INPROC_SERVER),
		uintptr(unsafe.Pointer(&IID_IDesktopWallpaper)),
		uintptr(unsafe.Pointer(&wallpaper)),
	)
	if hr != 0 {
		return fmt.Errorf("CoCreateInstance failed: 0x%X", hr)
	}
	defer w.release(wallpaper)

	if monitorID < 0 || uint64(monitorID) > math.MaxUint32 {
		return fmt.Errorf("invalid monitor ID: %d", monitorID)
	}
	mID := uint32(monitorID) // #nosec G115 - check above ensures no overflow

	// Fetch the specific monitor ID string using the index
	var monitorIDPtr uintptr
	hr, _, _ = syscall.SyscallN(
		wallpaper.LpVtbl.GetMonitorDevicePathAt,
		uintptr(unsafe.Pointer(wallpaper)),
		uintptr(mID),
		uintptr(unsafe.Pointer(&monitorIDPtr)),
	)
	if hr != 0 {
		return fmt.Errorf("GetMonitorDevicePathAt failed for index %d: 0x%X", monitorID, hr)
	}
	defer func() { _, _, _ = procCoTaskMemFree.Call(monitorIDPtr) }() // Ensure we free the string

	// SetWallpaper (Index 3)
	// Signature: HRESULT SetWallpaper([in] LPCWSTR monitorID, [in] LPCWSTR wallpaper)
	hr, _, _ = syscall.SyscallN(
		wallpaper.LpVtbl.SetWallpaper,
		uintptr(unsafe.Pointer(wallpaper)),
		monitorIDPtr,
		uintptr(unsafe.Pointer(imagePathUTF16)),
	)
	if hr != 0 {
		return fmt.Errorf("SetWallpaper failed: 0x%X", hr)
	}

	return nil
}

func (w *windowsOS) release(obj *IDesktopWallpaper) {
	if obj != nil {
		_, _, _ = syscall.SyscallN(obj.LpVtbl.Release, uintptr(unsafe.Pointer(obj)))
	}
}

// GetDesktopDimension returns the resolution of the primary monitor
func (w *windowsOS) GetDesktopDimension() (int, int, error) {
	return sysinfo.GetScreenDimensions()
}

// getOS returns a new instance of the windowsOS struct.
func getOS() OS {
	return &windowsOS{}
}
