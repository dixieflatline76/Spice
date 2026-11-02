//go:build windows
// +build windows

package wallpaper

import (
	"fmt"
	"image"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/dixieflatline76/Spice/v2/pkg/sysinfo"
	"golang.org/x/sys/windows"
)

var (
	modOle32             = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx   = modOle32.NewProc("CoInitializeEx")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")
	procCoUninitialize   = modOle32.NewProc("CoUninitialize")
	procCoTaskMemFree    = modOle32.NewProc("CoTaskMemFree")

	modUser32                       = windows.NewLazySystemDLL("user32.dll")
	procGetDisplayConfigBufferSizes = modUser32.NewProc("GetDisplayConfigBufferSizes")
	procQueryDisplayConfig          = modUser32.NewProc("QueryDisplayConfig")
	procDisplayConfigGetDeviceInfo  = modUser32.NewProc("DisplayConfigGetDeviceInfo")
)

const (
	COINIT_APARTMENTTHREADED = 0x2
	COINIT_MULTITHREADED     = 0x0
	CLSCTX_LOCAL_SERVER      = 0x4
	CLSCTX_INPROC_SERVER     = 0x1

	qdcOnlyActivePaths                = 0x00000002
	displayConfigDeviceInfoTargetName = 2
	errorInsufficientBuffer           = 122
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

type displayConfigLUID struct {
	LowPart  uint32
	HighPart int32
}

type displayConfigRational struct {
	Numerator   uint32
	Denominator uint32
}

type displayConfigPathSourceInfo struct {
	AdapterID   displayConfigLUID
	ID          uint32
	ModeInfoIdx uint32
	StatusFlags uint32
}

type displayConfigPathTargetInfo struct {
	AdapterID        displayConfigLUID
	ID               uint32
	ModeInfoIdx      uint32
	OutputTechnology uint32
	Rotation         uint32
	Scaling          uint32
	RefreshRate      displayConfigRational
	ScanLineOrdering uint32
	TargetAvailable  int32
	StatusFlags      uint32
}

type displayConfigPathInfo struct { // 72 bytes
	SourceInfo displayConfigPathSourceInfo
	TargetInfo displayConfigPathTargetInfo
	Flags      uint32
}

type displayConfigModeInfo struct { // 64 bytes
	InfoType  uint32
	ID        uint32
	AdapterID displayConfigLUID
	Mode      [48]byte
}

type displayConfigDeviceInfoHeader struct {
	Type      uint32
	Size      uint32
	AdapterID displayConfigLUID
	ID        uint32
}

type displayConfigTargetDeviceName struct { // 420 bytes
	Header                    displayConfigDeviceInfoHeader
	Flags                     uint32
	OutputTechnology          uint32
	EdidManufactureID         uint16
	EdidProductCodeID         uint16
	ConnectorInstance         uint32
	MonitorFriendlyDeviceName [64]uint16
	MonitorDevicePath         [128]uint16
}

// queryMonitorFriendlyNames maps lower-cased monitor device interface paths
// (same format as IDesktopWallpaper::GetMonitorDevicePathAt) to EDID friendly names.
// Returns an empty map on any failure; callers fall back to "Display N".
func queryMonitorFriendlyNames() map[string]string {
	result := make(map[string]string)
	for attempt := 0; attempt < 3; attempt++ {
		var numPath, numMode uint32
		ret, _, _ := procGetDisplayConfigBufferSizes.Call(
			uintptr(qdcOnlyActivePaths),
			uintptr(unsafe.Pointer(&numPath)),
			uintptr(unsafe.Pointer(&numMode)),
		)
		if ret != 0 || numPath == 0 {
			return result
		}
		paths := make([]displayConfigPathInfo, numPath)
		modes := make([]displayConfigModeInfo, max(numMode, 1))
		ret, _, _ = procQueryDisplayConfig.Call(
			uintptr(qdcOnlyActivePaths),
			uintptr(unsafe.Pointer(&numPath)),
			uintptr(unsafe.Pointer(&paths[0])),
			uintptr(unsafe.Pointer(&numMode)),
			uintptr(unsafe.Pointer(&modes[0])),
			0,
		)
		if ret == errorInsufficientBuffer {
			continue // topology changed between calls; retry
		}
		if ret != 0 {
			return result
		}
		for i := uint32(0); i < numPath; i++ {
			var tn displayConfigTargetDeviceName
			tn.Header.Type = displayConfigDeviceInfoTargetName
			tn.Header.Size = uint32(unsafe.Sizeof(tn))
			tn.Header.AdapterID = paths[i].TargetInfo.AdapterID
			tn.Header.ID = paths[i].TargetInfo.ID
			r, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&tn)))
			if r != 0 {
				continue
			}
			devPath := strings.ToLower(windows.UTF16ToString(tn.MonitorDevicePath[:]))
			friendly := strings.TrimSpace(windows.UTF16ToString(tn.MonitorFriendlyDeviceName[:]))
			if devPath != "" && friendly != "" {
				result[devPath] = friendly
			}
		}
		return result
	}
	return result
}

type windowsOS struct{}

// enumerateMonitors lists monitors via an existing IDesktopWallpaper instance.
// Returned slice is unsorted; IDs are the raw COM indices.
func (w *windowsOS) enumerateMonitors(wallpaper *IDesktopWallpaper) []Monitor {
	var count uint32
	// Call GetMonitorDevicePathCount (Index 6)
	hr, _, _ := syscall.SyscallN(
		wallpaper.LpVtbl.GetMonitorDevicePathCount,
		uintptr(unsafe.Pointer(wallpaper)),
		uintptr(unsafe.Pointer(&count)),
	)
	if hr != 0 {
		return nil
	}

	var monitors []Monitor
	for i := uint32(0); i < count; i++ {
		var rect image.Rectangle
		var winRect struct {
			Left, Top, Right, Bottom int32
		}

		// Call GetMonitorDevicePathAt (Index 5)
		// Signature: HRESULT GetMonitorDevicePathAt([in] UINT monitorIndex, [out] LPWSTR *monitorID)
		var monitorIDPtr unsafe.Pointer
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
		// Signature: HRESULT GetMonitorRECT([in] LPCWSTR monitorID, [out] RECT *displayRect)
		hr, _, _ = syscall.SyscallN(
			wallpaper.LpVtbl.GetMonitorRECT,
			uintptr(unsafe.Pointer(wallpaper)),
			uintptr(monitorIDPtr),
			uintptr(unsafe.Pointer(&winRect)),
		)

		devicePath := ""
		if monitorIDPtr != nil {
			// Converting uintptr from COM call to unsafe.Pointer for string conversion.
			// This is safe here because we just received the pointer from Windows.
			devicePath = windows.UTF16PtrToString((*uint16)(monitorIDPtr))
		}

		// Free the string returned by GetMonitorDevicePathAt
		_, _, _ = procCoTaskMemFree.Call(uintptr(monitorIDPtr))

		if hr == 0 {
			rect = image.Rect(int(winRect.Left), int(winRect.Top), int(winRect.Right), int(winRect.Bottom))
			if rect.Dx() > 0 && rect.Dy() > 0 {
				monitors = append(monitors, Monitor{
					ID:         int(i),
					DevicePath: devicePath,
					Rect:       rect,
					Bounds:     rect,
					Primary:    rect.Min.X == 0 && rect.Min.Y == 0, // Windows primary is always at virtual-desktop origin
				})
			}
		}
	}

	return monitors
}

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

	monitors := w.enumerateMonitors(wallpaper)
	if len(monitors) == 0 {
		return w.getPrimaryMonitorFallback()
	}
	ensurePrimary(monitors)

	names := queryMonitorFriendlyNames()
	for i := range monitors {
		monitors[i].Name = names[strings.ToLower(monitors[i].DevicePath)]
	}
	return sortAndIndexMonitors(monitors), nil
}

func (w *windowsOS) getPrimaryMonitorFallback() ([]Monitor, error) {
	width, height, err := w.GetDesktopDimension()
	if err != nil {
		return nil, err
	}
	return []Monitor{{ID: 0, Name: "Primary", Primary: true, Rect: image.Rect(0, 0, width, height), Bounds: image.Rect(0, 0, width, height)}}, nil
}

// SetWallpaper sets the desktop wallpaper for the given sorted monitorID.
func (w *windowsOS) SetWallpaper(imagePath string, monitorID int) error {
	if monitorID < 0 {
		return fmt.Errorf("invalid monitor ID: %d", monitorID)
	}

	// Resolve MSIX-virtualized path so explorer.exe (outside the container)
	// can find the file when setting it via IDesktopWallpaper COM API.
	imagePath = resolveMSIXPicturePath(imagePath)

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

	monitors := w.enumerateMonitors(wallpaper)
	ensurePrimary(monitors)
	monitors = sortAndIndexMonitors(monitors)
	if monitorID >= len(monitors) {
		return fmt.Errorf("monitor index %d out of range (%d monitors)", monitorID, len(monitors))
	}
	devPathUTF16, err := syscall.UTF16PtrFromString(monitors[monitorID].DevicePath)
	if err != nil {
		return err
	}

	// SetWallpaper (Index 3)
	// Signature: HRESULT SetWallpaper([in] LPCWSTR monitorID, [in] LPCWSTR wallpaper)
	hr, _, _ = syscall.SyscallN(
		wallpaper.LpVtbl.SetWallpaper,
		uintptr(unsafe.Pointer(wallpaper)),
		uintptr(unsafe.Pointer(devPathUTF16)),
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

// Stat returns file info for the given path.
func (w *windowsOS) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// getOS returns a new instance of the windowsOS struct.
func getOS() OS {
	return &windowsOS{}
}
