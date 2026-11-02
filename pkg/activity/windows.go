//go:build windows
// +build windows

package activity

import (
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---------------------------------------------------------------------------
// Win32 shell (LazyDLL pattern — mirrors pkg/sysinfo/windows.go)
// ---------------------------------------------------------------------------

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	shell32                = syscall.NewLazyDLL("shell32.dll")
	getForegroundFP        = user32.NewProc("GetForegroundWindow")
	getWindowRect          = user32.NewProc("GetWindowRect")
	getWindowStyle         = user32.NewProc("GetWindowLongW")
	getClassNameW          = user32.NewProc("GetClassNameW")
	getWindowThread        = user32.NewProc("GetWindowThreadProcessId")
	enumDisplayMonitors    = user32.NewProc("EnumDisplayMonitors")
	getMonitorInfoW        = user32.NewProc("GetMonitorInfoW")
	getSysMetrics          = user32.NewProc("GetSystemMetrics")
	getPowerStatus         = kernel32.NewProc("GetSystemPowerStatus")
	queryFullProcImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	shellQueryUNS          = shell32.NewProc("SHQueryUserNotificationState")
)

func queryNotificationState() int {
	var state uint32
	r, _, _ := shellQueryUNS.Call(uintptr(unsafe.Pointer(&state)))
	if r != 0 { // S_OK == 0
		return 0
	}
	return int(state)
}

const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79
)

const gwlStyle = uintptr(0xFFFFFFFFFFFFFFF0) // GWL_STYLE = -16 as two's complement

type rectStruct struct {
	Left, Top, Right, Bottom int32
}

type monitorInfoStruct struct {
	CbSize    uint32
	RcMonitor rectStruct
	RcWork    rectStruct
	DwFlags   uint32
}

type powerStatusStruct struct {
	ACLineStatus    byte
	BatteryFlag     byte
	BatteryLifePct  byte
	Reserved        byte
	BatteryLifetime uint32
	MaxBatteryLife  uint32
}

func windowClassName(hwnd uintptr) string {
	buf := make([]uint16, 128)
	n, _, _ := getClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	if int(n) > len(buf) {
		n = uintptr(len(buf))
	}
	return windows.UTF16ToString(buf[:n])
}

// foregroundWindowRectAndStyle fetches the foreground window's rect + style
// (S2 input), excluding Windows desktop/taskbar shell classes.
func foregroundWindowRectAndStyle() (win Rect, style uint32, ok bool) {
	hwnd, _, _ := getForegroundFP.Call()
	if hwnd == 0 {
		return Rect{}, 0, false
	}
	if IsWindowsShellClass(windowClassName(hwnd)) {
		return Rect{}, 0, false
	}
	var r rectStruct
	if ret, _, _ := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ret == 0 {
		return Rect{}, 0, false
	}
	st, _, _ := getWindowStyle.Call(hwnd, gwlStyle)
	return Rect{
			X: int(r.Left),
			Y: int(r.Top),
			W: int(r.Right - r.Left),
			H: int(r.Bottom - r.Top),
		},
		uint32(st), true // #nosec G115 //nolint:gosec // G115: Win32 GWL_STYLE is a 32-bit bitmask
}

func foregroundPIDRaw() (uint32, bool) {
	hwnd, _, _ := getForegroundFP.Call()
	if hwnd == 0 {
		return 0, false
	}
	var pid uint32
	_, _, _ = getWindowThread.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return 0, false
	}
	return pid, true
}

func foregroundPID() (int, bool) {
	pid, ok := foregroundPIDRaw()
	if !ok {
		return 0, false
	}
	return int(pid), true
}

func foregroundProcessName() (string, bool) {
	pid, ok := foregroundPIDRaw()
	if !ok {
		return "", false
	}
	if name, ok := processImageNameByOpen(pid); ok {
		return name, true
	}
	return processExeNameBySnapshot(pid)
}

func processImageNameByOpen(pid uint32) (string, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer func() { _ = windows.CloseHandle(h) }()

	var size uint32 = 512
	buf := make([]uint16, size)
	ret, _, _ := queryFullProcImageName.Call(
		uintptr(h), 0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret == 0 || size == 0 {
		return "", false
	}
	n := int(size)
	if n > len(buf) {
		n = len(buf)
	}
	return windows.UTF16ToString(buf[:n]), true
}

// processExeNameBySnapshot resolves a PID's executable basename via
// CreateToolhelp32Snapshot when OpenProcess is denied (e.g. elevated or
// anti-cheat protected games).
func processExeNameBySnapshot(pid uint32) (string, bool) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", false
	}
	defer func() { _ = windows.CloseHandle(snap) }()

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return "", false
	}
	for {
		if pe.ProcessID == pid {
			name := windows.UTF16ToString(pe.ExeFile[:])
			if name != "" {
				return name, true
			}
			return "", false
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			break
		}
	}
	return "", false
}

var (
	enumMonMu   sync.Mutex
	enumMonOnce sync.Once
	enumMonCB   uintptr
	enumMonBuf  []Rect
)

func enumMonitorCallback(hMonitor uintptr, hdc uintptr, lpRect *rectStruct, lParam uintptr) uintptr {
	var mi monitorInfoStruct
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	if ret, _, _ := getMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi))); ret != 0 {
		r := mi.RcMonitor
		enumMonBuf = append(enumMonBuf, Rect{
			X: int(r.Left),
			Y: int(r.Top),
			W: int(r.Right - r.Left),
			H: int(r.Bottom - r.Top),
		})
	}
	return 1 // continue enumeration
}

// monitorRects enumerates individual display bounds so S2 works across
// multi-monitor setups. Falls back to virtualScreenRect if enumeration fails.
// Uses a single registered Win32 callback slot for the process lifetime.
func monitorRects() []Rect {
	enumMonMu.Lock()
	defer enumMonMu.Unlock()

	enumMonOnce.Do(func() {
		enumMonCB = syscall.NewCallback(enumMonitorCallback)
	})

	enumMonBuf = enumMonBuf[:0]
	_, _, _ = enumDisplayMonitors.Call(0, 0, enumMonCB, 0)
	if len(enumMonBuf) == 0 {
		return []Rect{virtualScreenRect()}
	}
	out := make([]Rect, len(enumMonBuf))
	copy(out, enumMonBuf)
	return out
}

func virtualScreenRect() Rect {
	x, _, _ := getSysMetrics.Call(smXVirtualScreen)
	y, _, _ := getSysMetrics.Call(smYVirtualScreen)
	w, _, _ := getSysMetrics.Call(smCXVirtualScreen)
	h, _, _ := getSysMetrics.Call(smCYVirtualScreen)
	return Rect{X: int(int32(x)), Y: int(int32(y)), W: int(w), H: int(h)} // #nosec G115 //nolint:gosec // G115: GetSystemMetrics returns signed 32-bit int
}

func queryPowerStatus() powerStatusStruct {
	var ps powerStatusStruct
	r, _, _ := getPowerStatus.Call(uintptr(unsafe.Pointer(&ps)))
	if r == 0 {
		return powerStatusStruct{}
	}
	return ps
}

// ---------------------------------------------------------------------------
// Detector implementation
// ---------------------------------------------------------------------------

type windowsDetector struct {
	pdhMu      sync.Mutex
	pdhQuery   syscall.Handle
	pdhCounter syscall.Handle
	pdhInit    bool
	pdhFailed  bool
}

// New returns the platform Detector hook (Win32-backed on Windows).
func New() Detector { return &windowsDetector{} }

func (d *windowsDetector) UserBusy() BusyState {
	s1 := ParseUserNotificationState(queryNotificationState())

	var s2 bool
	if win, style, ok := foregroundWindowRectAndStyle(); ok {
		s2 = IsFullscreenGeometry(win, monitorRects(), style)
	}
	return CombineBusy(s1, s2, false, false)
}

func (d *windowsDetector) Battery() BatteryState {
	ps := queryPowerStatus()
	return ParsePowerStatus(int(ps.ACLineStatus), int(ps.BatteryLifePct))
}

func (d *windowsDetector) ForegroundProcess() (string, bool) {
	return foregroundProcessName()
}

func (d *windowsDetector) ForegroundPID() (int, bool) {
	return foregroundPID()
}

// ---------------------------------------------------------------------------
// S5 (GPU 3D-engine load) — Persistent PDH query handle
// ---------------------------------------------------------------------------

var (
	pdh             = syscall.NewLazyDLL("pdh.dll")
	pdhOpenQuery    = pdh.NewProc("PdhOpenQueryW")
	pdhAddCounter   = pdh.NewProc("PdhAddEnglishCounterW")
	pdhCollectData  = pdh.NewProc("PdhCollectQueryData")
	pdhGetFormatted = pdh.NewProc("PdhGetFormattedCounterArrayW")
	pdhCloseQuery   = pdh.NewProc("PdhCloseQuery")
)

const (
	pdhFmtDouble = 0x00000200
	pdhMoreData  = 0x800007D2
)

// GpuLoad3D reuses a persistent PDH query handle across ticks so rate counters
// are computed between consecutive calls without sleeping on subsequent ticks.
func (d *windowsDetector) GpuLoad3D() (map[int]float64, bool) {
	d.pdhMu.Lock()
	defer d.pdhMu.Unlock()

	if d.pdhFailed {
		return nil, false
	}

	if !d.pdhInit {
		if err := pdhOpenQuery.Find(); err != nil {
			d.pdhFailed = true
			return nil, false
		}
		if err := pdhAddCounter.Find(); err != nil {
			d.pdhFailed = true
			return nil, false
		}
		if err := pdhCollectData.Find(); err != nil {
			d.pdhFailed = true
			return nil, false
		}
		if err := pdhGetFormatted.Find(); err != nil {
			d.pdhFailed = true
			return nil, false
		}

		const counterPath = `\GPU Engine(*)\Utilization Percentage`
		pathPtr, err := windows.UTF16PtrFromString(counterPath)
		if err != nil {
			d.pdhFailed = true
			return nil, false
		}

		var query syscall.Handle
		if ret, _, _ := pdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&query))); ret != 0 {
			d.pdhFailed = true
			return nil, false
		}
		var counter syscall.Handle
		if ret, _, _ := pdhAddCounter.Call(
			uintptr(query),
			uintptr(unsafe.Pointer(pathPtr)),
			0,
			uintptr(unsafe.Pointer(&counter)),
		); ret != 0 {
			if pdhCloseQuery.Find() == nil {
				_, _, _ = pdhCloseQuery.Call(uintptr(query))
			}
			d.pdhFailed = true
			return nil, false
		}
		// Prime the baseline sample once on initialization and wait briefly so
		// the first collection has a valid rate delta.
		_, _, _ = pdhCollectData.Call(uintptr(query))
		time.Sleep(100 * time.Millisecond)
		d.pdhQuery = query
		d.pdhCounter = counter
		d.pdhInit = true
	}

	if ret, _, _ := pdhCollectData.Call(uintptr(d.pdhQuery)); ret != 0 {
		return nil, false
	}

	rows := readGpuCounterRows(d.pdhCounter)
	if len(rows) == 0 {
		return nil, false
	}
	return ParseGpuEngineInstances(rows), true
}

func readGpuCounterRows(counter syscall.Handle) []GpuEngineRow {
	bufferSize := uint32(64 * 1024)
	for attempts := 0; attempts < 4; attempts++ {
		buf := make([]byte, bufferSize)
		size := bufferSize
		var itemCount uint32
		ret, _, _ := pdhGetFormatted.Call(
			uintptr(counter),
			pdhFmtDouble,
			uintptr(unsafe.Pointer(&size)),
			uintptr(unsafe.Pointer(&itemCount)),
			uintptr(unsafe.Pointer(&buf[0])),
		)
		if ret == 0 {
			if itemCount == 0 {
				return nil
			}
			return parseGpuItems(buf, itemCount)
		}
		if (ret & 0xFFFFFFFF) != pdhMoreData {
			return nil
		}
		if size > bufferSize {
			bufferSize = size * 2
		} else {
			bufferSize *= 2
		}
		if bufferSize > 16*1024*1024 {
			break
		}
	}
	return nil
}

// parseGpuItems decodes the 64-bit PDH_FMT_COUNTERVALUE_ITEM_W array:
// offset 0: SzName (*uint16, 8 bytes)
// offset 8: FmtValue.CStatus (uint32, 4 bytes) + padding (4 bytes)
// offset 16: FmtValue.doubleValue (float64, 8 bytes)
func parseGpuItems(buf []byte, itemCount uint32) []GpuEngineRow {
	const itemSize = 24
	const pdhCStatusNewData = 1 // PDH_CSTATUS_VALID_DATA (0) or PDH_CSTATUS_NEW_DATA (1)
	rows := make([]GpuEngineRow, 0, int(itemCount))
	for i := 0; i < int(itemCount); i++ {
		off := i * itemSize
		if off+itemSize > len(buf) {
			break
		}
		cStatus := *(*uint32)(unsafe.Pointer(&buf[off+8]))
		if cStatus > pdhCStatusNewData {
			continue
		}
		namePtr := *(**uint16)(unsafe.Pointer(&buf[off]))
		if namePtr == nil {
			continue
		}
		name := windows.UTF16PtrToString(namePtr)
		val := *(*float64)(unsafe.Pointer(&buf[off+16]))
		rows = append(rows, GpuEngineRow{InstanceName: name, Value: val})
	}
	return rows
}
