// Package activity provides cross-platform detection of "user busy" session
// state (fullscreen games, presentations) and battery status for Spice's
// Activity Gate (Milestone 3.0 — Engine Intelligence).
//
// Design (see plans/activity-gate-design.md):
//
//	Inner ring (this file): pure Go — Detector ports, enums, and ALL
//	  detection decision logic as exported pure functions (100% unit-testable
//	  in CI, no OS required).
//	Outer ring (build-tagged files): thin OS shells (Win32 via LazyDLL,
//	  macOS via CGWindowList/NSWorkspace cgo + pmset) that delegate every decision to the pure
//	  functions below.
//
// Detection signals (combined by CombineBusy):
//
//	S1  Shell notification state / native fullscreen   (Windows QUNS / macOS AXFullScreen)
//	S2  Foreground borderless fullscreen geometry      (Win32 GetWindowRect / macOS SE frame)
//	S3  Foreground process allowlist                   (user-maintained game list)
//	S4  Manual Game Mode toggle                        (engine-side pref, not a detector concern)
//	S5  Sustained 3D-engine GPU load of foreground pid (Windows PDH only; optional GpuDetector)
//
// All functions in this file are safe for concurrent use and never touch the
// OS: the outer-ring detectors own the syscall boundaries.
package activity

import (
	"fmt"
	"regexp"
	"strings"
)

// ---------------------------------------------------------------------------
// Ports
// ---------------------------------------------------------------------------

// Detector is the platform port for session-state and power probing.
// Implementations must be cheap (<10ms) and side-effect free; callers may
// poll them on a slow ticker (60s). They must never panic on unexpected OS
// results — return zero values instead.
type Detector interface {
	// UserBusy reports whether the session currently looks busy
	// (fullscreen game, presentation, fullscreen video). Signals S1 and S2
	// are evaluated here; S3/S5 votes are combined by the engine gate.
	UserBusy() BusyState

	// Battery returns the power source and charge level. Returns the zero
	// value (NoBattery semantics) when the machine has no battery or the
	// state is unknown — never an error.
	Battery() BatteryState

	// ForegroundProcess returns the executable/app name of the frontmost
	// window (e.g. "cs2.exe", "Keynote") for allowlist matching (S3).
	// ok is false when the foreground process cannot be determined.
	ForegroundProcess() (name string, ok bool)
}

// GpuDetector is an optional capability port (S5) implemented only where a
// public per-process GPU utilization API exists (Windows PDH today). The
// engine type-asserts; stubs simply omit it.
type GpuDetector interface {
	// GpuLoad3D returns 3D-engine GPU utilization percentage per process id
	// (sampled over a short window). ok is false when GPU metrics are
	// unavailable (unsupported platform, missing counters, driver quirks).
	GpuLoad3D() (pidPct map[int]float64, ok bool)

	// ForegroundPID returns the OS process ID of the frontmost window so S5
	// can attribute 3D engine utilization to the foreground application.
	ForegroundPID() (pid int, ok bool)
}

// BusyState classifies how confidently the session looks busy.
type BusyState int

const (
	BusyNone         BusyState = iota // No busy signal detected
	BusyPresentation                  // Presentation/busy shell state (F11, slideshow, video)
	BusyFullscreen                    // Fullscreen app/game detected
)

// String returns a stable lowercase name (used in logs and the probe tool).
func (b BusyState) String() string {
	switch b {
	case BusyPresentation:
		return "presentation"
	case BusyFullscreen:
		return "fullscreen"
	default:
		return "none"
	}
}

// BatteryState describes the machine's power situation. The zero value means
// "no battery present / unknown" (desktops, VMs).
type BatteryState struct {
	OnAC       bool // Plugged into external power
	HasBattery bool // A battery exists and its percentage is known
	Percent    int  // 0..100 charge level (meaningful only when HasBattery)
}

// String returns a compact representation for logs and the probe tool.
func (b BatteryState) String() string {
	switch {
	case !b.HasBattery && !b.OnAC:
		return "none"
	case b.HasBattery && b.OnAC:
		return fmt.Sprintf("ac:%d%%", b.Percent)
	case b.OnAC:
		return "ac"
	default:
		return fmt.Sprintf("battery:%d%%", b.Percent)
	}
}

// Low reports whether the battery is discharging at or below the given
// percentage threshold. Always false on AC power or without a battery.
func (b BatteryState) Low(threshold int) bool {
	return b.HasBattery && !b.OnAC && b.Percent <= threshold
}

// ---------------------------------------------------------------------------
// Rect — minimal pure-Go rectangle used by geometry heuristics.
// ---------------------------------------------------------------------------

// Rect is an axis-aligned rectangle in virtual-screen coordinates.
type Rect struct {
	X, Y, W, H int
}

func (r Rect) area() int {
	if r.W <= 0 || r.H <= 0 {
		return 0
	}
	return r.W * r.H
}

func intersectArea(a, b Rect) int {
	x0, y0 := maxInt(a.X, b.X), maxInt(a.Y, b.Y)
	x1, y1 := minInt(a.X+a.W, b.X+b.W), minInt(a.Y+a.H, b.Y+b.H)
	if x1 <= x0 || y1 <= y0 {
		return 0
	}
	return (x1 - x0) * (y1 - y0)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// S1 — Windows shell notification state (QUNS_*)
// ---------------------------------------------------------------------------

// SHQueryUserNotificationState return codes.
const (
	qunsNotPresent           = 2 // Screensaver running / session locked — NOT busy
	qunsBusy                 = 1 // Full-screen (F11) mode — busy
	qunsD3DFullscreen        = 3 // Exclusive-mode D3D fullscreen — busy (game)
	qunsPresentationMode     = 4 // Presenter mode (PowerPoint etc.) — busy
	qunsAcceptsNotifications = 5 // Normal desktop — not busy
	qunsQuietTime            = 6 // Scheduled quiet hours — NOT busy (not activity)
	qunsApp                  = 7 // Windows Store app fullscreen — busy (game)
)

// ParseUserNotificationState maps a QUNS_* code to a BusyState.
//
// Deliberate exclusions: QUNS_NOT_PRESENT (2) means locked/screensaver — the
// user is away and rotating is harmless; QUNS_QUIET_TIME (6) is scheduled
// do-not-disturb, not activity. Both must never pause rotation.
func ParseUserNotificationState(v int) BusyState {
	switch v {
	case qunsBusy, qunsPresentationMode:
		return BusyPresentation
	case qunsD3DFullscreen, qunsApp:
		return BusyFullscreen
	default:
		return BusyNone
	}
}

// QUNSName returns a human-readable name for a QUNS code (probe/logging).
func QUNSName(v int) string {
	switch v {
	case qunsBusy:
		return "BUSY"
	case qunsNotPresent:
		return "NOT_PRESENT"
	case qunsD3DFullscreen:
		return "D3D_FULL_SCREEN"
	case qunsPresentationMode:
		return "PRESENTATION_MODE"
	case qunsAcceptsNotifications:
		return "ACCEPTS_NOTIFICATIONS"
	case qunsQuietTime:
		return "QUIET_TIME"
	case qunsApp:
		return "APP"
	default:
		return fmt.Sprintf("UNKNOWN_%d", v)
	}
}

// ---------------------------------------------------------------------------
// Battery mapping
// ---------------------------------------------------------------------------

// ParsePowerStatus maps Win32 SYSTEM_POWER_STATUS fields to a BatteryState.
// acLine: 1 = on AC, 0 = on battery, 255 = unknown. lifePercent: 0..100, or
// 255 when unknown (VMs, desktops without driver-reported battery).
func ParsePowerStatus(acLine, lifePercent int) BatteryState {
	s := BatteryState{OnAC: acLine == 1}
	if lifePercent != 255 {
		s.HasBattery = true
		s.Percent = lifePercent
	}
	return s
}

var pmsetPercentRe = regexp.MustCompile(`(\d+)%`)

// ParsePmsetOutput parses `pmset -g batt` output on macOS.
// Examples:
//
//	"Now drawing from 'AC Power' ... 98%; charging"  → {OnAC, battery, 98}
//	"Now drawing from 'Battery Power' ... 42%; ..."  → {battery, 42}
//	"Now drawing from 'AC Power'"                    → {OnAC, no battery}
func ParsePmsetOutput(out string) BatteryState {
	s := BatteryState{OnAC: strings.Contains(out, "AC Power")}
	if m := pmsetPercentRe.FindStringSubmatch(out); m != nil {
		s.HasBattery = true
		s.Percent = parseDecimal(m[1])
	}
	return s
}

func parseDecimal(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// ---------------------------------------------------------------------------
// S2 — Borderless fullscreen geometry heuristic
// ---------------------------------------------------------------------------

// Win32 window style bits relevant to the heuristic.
const (
	wsPopup   uint32 = 0x80000000 // WS_POPUP — typical borderless game window
	wsCaption uint32 = 0x00C00000 // WS_CAPTION — titled/maximized window
)

// fullscreenCoverFraction is the minimum fraction of a monitor a window must
// cover to count as fullscreen. 98% tolerates 1px shadows/borders while
// rejecting maximized-but-not-fullscreen windows on scaled displays.
const fullscreenCoverFraction = 0.98

// IsFullscreenGeometry reports whether a borderless (WS_POPUP-style, no
// caption) foreground window covers >=98% of any monitor. A captioned window
// can never qualify — a maximized browser must not pause rotation.
func IsFullscreenGeometry(win Rect, monitors []Rect, style uint32) bool {
	if (style & wsCaption) == wsCaption {
		return false
	}
	winArea := win.area()
	if winArea <= 0 {
		return false
	}
	for _, m := range monitors {
		if m.area() <= 0 {
			continue
		}
		covered := float64(intersectArea(win, m)) / float64(m.area())
		if covered >= fullscreenCoverFraction {
			return true
		}
	}
	return false
}

// winShellClasses lists Windows desktop and taskbar window classes that cover
// the screen or use WS_POPUP but are never user fullscreen applications.
var winShellClasses = map[string]bool{
	"progman":                true,
	"workerw":                true,
	"shell_traywnd":          true,
	"shell_secondarytraywnd": true,
}

// IsWindowsShellClass reports whether className is a Windows desktop/shell
// window class that must be excluded from the S2 fullscreen geometry check.
func IsWindowsShellClass(className string) bool {
	return winShellClasses[strings.ToLower(strings.TrimSpace(className))]
}

// ---------------------------------------------------------------------------
// S3 — Foreground process allowlist
// ---------------------------------------------------------------------------

// MatchesProcessList reports whether procName matches any entry in the
// user-maintained, comma-separated allowlist (S3). Matching is
// case-insensitive, ignores surrounding whitespace, strips `.exe`/`.app`
// suffixes on both sides, and compares basenames — never substrings — so
// "csnotepad.exe" cannot match "notepad".
func MatchesProcessList(procName, allowlist string) bool {
	norm := normalizeProcessName(procName)
	if norm == "" {
		return false
	}
	for _, entry := range strings.Split(allowlist, ",") {
		e := normalizeProcessName(entry)
		if e != "" && e == norm {
			return true
		}
	}
	return false
}

func normalizeProcessName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	if s == "" {
		return ""
	}
	for _, suffix := range []string{".exe", ".app"} {
		s = strings.TrimSuffix(s, suffix)
	}
	// Basename across both path separators.
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// macOS fullscreen classification (S1/S2 — pure decision part)
// ---------------------------------------------------------------------------

// macOS system-owned processes that own full-screen-sized layers but are
// never user activity. Excluded case-insensitively.
var macSystemOwners = map[string]bool{
	"dock":            true,
	"windowserver":    true,
	"systemuiserver":  true,
	"control center":  true,
	"spotlight":       true,
	"notification c":  true, // NotificationCenter legacy name fragment guard
	"notifications":   true,
	"screensaver":     true,
	"loginwindow":     true,
	"desktop manager": true, // WindowServer's spaces/Stage Manager helper name
}

// ClassifyWindow is the macOS window-level decision used by the CGWindowList
// path (Phase 2 cgo upgrade). Today's osascript detector relies on the
// AXFullScreen attribute plus frame geometry, but this pure classifier is
// shared by both paths.
//
// Rules: only layer 0 windows count; Spice itself and known system owners are
// excluded; the frame must cover >=98% of the display.
func ClassifyWindow(owner string, layer int, frame, display Rect, spiceOwner string) BusyState {
	if layer != 0 {
		return BusyNone
	}
	if strings.EqualFold(strings.TrimSpace(owner), strings.TrimSpace(spiceOwner)) {
		return BusyNone
	}
	if macSystemOwners[strings.ToLower(strings.TrimSpace(owner))] {
		return BusyNone
	}
	if display.area() <= 0 || frame.area() <= 0 {
		return BusyNone
	}
	// On macOS, normal zoomed/maximized windows sit below the ~24pt menu bar
	// (frame.Y > display.Y) yet still cover >98% of a 1440p/4K screen. True
	// fullscreen spaces and borderless games cover the top edge of the display.
	if frame.Y > display.Y {
		return BusyNone
	}
	if float64(intersectArea(frame, display))/float64(display.area()) >= fullscreenCoverFraction {
		return BusyFullscreen
	}
	return BusyNone
}

// ---------------------------------------------------------------------------
// Signal combiner
// ---------------------------------------------------------------------------

// CombineBusy merges all detection votes into a final BusyState. Any
// fullscreen-class vote (S2 geometry, S3 allowlist, S5 GPU) promotes the
// result to BusyFullscreen; a presentation-only shell state stays
// BusyPresentation; no votes means BusyNone.
func CombineBusy(shell BusyState, geometry, allowlist, gpu bool) BusyState {
	if geometry || allowlist || gpu || shell == BusyFullscreen {
		return BusyFullscreen
	}
	if shell == BusyPresentation {
		return BusyPresentation
	}
	return BusyNone
}

// ---------------------------------------------------------------------------
// S5 — GPU 3D-engine load (Windows PDH)
// ---------------------------------------------------------------------------

// GpuEngineRow is one formatted PDH counter value for a "GPU Engine"
// instance, e.g. InstanceName
// "pid_1234_luid_..._phys_0_eng_0_engtype_3D", Value 45.5 (%).
type GpuEngineRow struct {
	InstanceName string
	Value        float64
}

var gpuPidRe = regexp.MustCompile(`pid_(\d+)`)

// ParseGpuEngineInstances reduces raw PDH rows to per-pid 3D-engine
// utilization. Only rows tagged engtype_3D are kept (VideoDecode/Copy/etc.
// are excluded so video playback never trips the signal); when a pid has
// multiple 3D engines the maximum is kept. Rows without a parseable pid or
// engine type are discarded.
func ParseGpuEngineInstances(rows []GpuEngineRow) map[int]float64 {
	out := make(map[int]float64)
	for _, r := range rows {
		if !strings.Contains(r.InstanceName, "engtype_3D") {
			continue
		}
		m := gpuPidRe.FindStringSubmatch(r.InstanceName)
		if m == nil {
			continue
		}
		pid := parseDecimal(m[1])
		if cur, ok := out[pid]; !ok || r.Value > cur {
			out[pid] = r.Value
		}
	}
	return out
}

// IsSustained3DLoad reports whether the given pid showed 3D-engine
// utilization at or above thresholdPct in each of the last minSamples
// samples (minSamples <= 0 is treated as 1). A single dip in the tail
// window disqualifies the pid, preventing one-off WebGL spikes from
// engaging the gate.
func IsSustained3DLoad(samples []map[int]float64, pid int, thresholdPct float64, minSamples int) bool {
	if minSamples < 1 {
		minSamples = 1
	}
	if len(samples) < minSamples {
		return false
	}
	tail := samples[len(samples)-minSamples:]
	for _, s := range tail {
		v, ok := s[pid]
		if !ok || v < thresholdPct {
			return false
		}
	}
	return true
}
