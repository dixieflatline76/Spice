// Package activity tests pin the Detector port contract and the pure
// decision logic for the Activity Gate (TDD: tests are written before the
// implementation; see plans/activity-gate-implementation.md §1).
//
// Platform syscall shells are intentionally NOT covered here — they are the
// documented uncovered lines (§7 of the implementation plan). All mapping,
// classification, matching, and reduction logic is exercised through the
// table-driven tests below without any OS dependency.
package activity

import "testing"

// ---------------------------------------------------------------------------
// Port contract (TDD step 1.1)
// ---------------------------------------------------------------------------

// fakeDetector is the test double reused by the gate tests in pkg/wallpaper.
// Defined here to pin the exact port contract.
type fakeDetector struct {
	busy   BusyState
	batt   BatteryState
	fgName string
	fgOK   bool
	fgPID  int
	gpu    map[int]float64
	gpuOK  bool
}

func (f *fakeDetector) UserBusy() BusyState                { return f.busy }
func (f *fakeDetector) Battery() BatteryState              { return f.batt }
func (f *fakeDetector) ForegroundProcess() (string, bool)  { return f.fgName, f.fgOK }
func (f *fakeDetector) ForegroundPID() (int, bool)         { return f.fgPID, f.fgPID > 0 }
func (f *fakeDetector) GpuLoad3D() (map[int]float64, bool) { return f.gpu, f.gpuOK }

// Compile-time contract: the fake satisfies both ports.
var (
	_ Detector    = (*fakeDetector)(nil)
	_ GpuDetector = (*fakeDetector)(nil)
)

// TestNewReturnsDetector verifies the platform hook yields a usable port on
// every build (no syscalls involved).
func TestNewReturnsDetector(t *testing.T) {
	d := New()
	if d == nil {
		t.Fatal("New() returned nil; platform hook must provide a Detector")
	}
}

func TestBusyStateString(t *testing.T) {
	cases := map[BusyState]string{
		BusyNone:         "none",
		BusyPresentation: "presentation",
		BusyFullscreen:   "fullscreen",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("BusyState(%d).String() = %q, want %q", int(state), got, want)
		}
	}
}

func TestBatteryStateString(t *testing.T) {
	cases := []struct {
		name string
		in   BatteryState
		want string
	}{
		{"no battery", BatteryState{}, "none"},
		{"on ac", BatteryState{OnAC: true, HasBattery: true, Percent: 80}, "ac:80%"},
		{"on battery", BatteryState{HasBattery: true, Percent: 42}, "battery:42%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBatteryLow(t *testing.T) {
	cases := []struct {
		name      string
		in        BatteryState
		threshold int
		want      bool
	}{
		{"below threshold on battery", BatteryState{HasBattery: true, Percent: 15}, 20, true},
		{"equal threshold", BatteryState{HasBattery: true, Percent: 20}, 20, true},
		{"above threshold", BatteryState{HasBattery: true, Percent: 25}, 20, false},
		{"on ac never low", BatteryState{OnAC: true, HasBattery: true, Percent: 5}, 20, false},
		{"no battery never low", BatteryState{}, 20, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Low(tc.threshold); got != tc.want {
				t.Errorf("Low(%d) = %v, want %v", tc.threshold, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// S1: shell notification state mapping (TDD step 1.2)
// ---------------------------------------------------------------------------

func TestParseUserNotificationState(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want BusyState
	}{
		{"F11 busy is presentation", qunsBusy, BusyPresentation},
		{"locked/screensaver ignored", qunsNotPresent, BusyNone},
		{"D3D exclusive fullscreen", qunsD3DFullscreen, BusyFullscreen},
		{"presentation mode", qunsPresentationMode, BusyPresentation},
		{"accepts notifications", qunsAcceptsNotifications, BusyNone},
		{"quiet time ignored", qunsQuietTime, BusyNone},
		{"full-screen app", qunsApp, BusyFullscreen},
		{"unknown zero", 0, BusyNone},
		{"out of range", 99, BusyNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseUserNotificationState(tc.in); got != tc.want {
				t.Fatalf("ParseUserNotificationState(%d) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestQUNSName(t *testing.T) {
	cases := map[int]string{
		qunsBusy:                 "BUSY",
		qunsNotPresent:           "NOT_PRESENT",
		qunsD3DFullscreen:        "D3D_FULL_SCREEN",
		qunsPresentationMode:     "PRESENTATION_MODE",
		qunsAcceptsNotifications: "ACCEPTS_NOTIFICATIONS",
		qunsQuietTime:            "QUIET_TIME",
		qunsApp:                  "APP",
		123:                      "UNKNOWN_123",
	}
	for in, want := range cases {
		if got := QUNSName(in); got != want {
			t.Errorf("QUNSName(%d) = %q, want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Battery mapping (TDD steps 1.3/1.4)
// ---------------------------------------------------------------------------

func TestParsePowerStatus(t *testing.T) {
	cases := []struct {
		name        string
		acLine      int
		lifePercent int
		want        BatteryState
	}{
		{"on ac with battery", 1, 80, BatteryState{OnAC: true, HasBattery: true, Percent: 80}},
		{"on battery", 0, 42, BatteryState{OnAC: false, HasBattery: true, Percent: 42}},
		{"unknown percent on battery", 0, 255, BatteryState{OnAC: false, HasBattery: false}},
		{"unknown percent on ac", 1, 255, BatteryState{OnAC: true, HasBattery: false}},
		{"unknown ac line treated as battery", 255, 60, BatteryState{OnAC: false, HasBattery: true, Percent: 60}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParsePowerStatus(tc.acLine, tc.lifePercent); got != tc.want {
				t.Fatalf("ParsePowerStatus(%d, %d) = %+v, want %+v", tc.acLine, tc.lifePercent, got, tc.want)
			}
		})
	}
}

func TestParsePmsetOutput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want BatteryState
	}{
		{
			"ac power charging",
			"Now drawing from 'AC Power'\n -InternalBattery-0 (id=536875091)\t98%; charging; 0:12 remaining present: true\n",
			BatteryState{OnAC: true, HasBattery: true, Percent: 98},
		},
		{
			"on battery discharging",
			"Now drawing from 'Battery Power'\n -InternalBattery-0 (id=536875091)\t42%; discharging; 3:45 remaining present: true\n",
			BatteryState{OnAC: false, HasBattery: true, Percent: 42},
		},
		{
			"desktop no battery",
			"Now drawing from 'AC Power'\n",
			BatteryState{OnAC: true, HasBattery: false},
		},
		{"empty", "", BatteryState{}},
		{"garbage", "hello world", BatteryState{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParsePmsetOutput(tc.in); got != tc.want {
				t.Fatalf("ParsePmsetOutput(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// S2: borderless fullscreen geometry heuristic (TDD step 1.5)
// ---------------------------------------------------------------------------

func TestIsFullscreenGeometry(t *testing.T) {
	primary := Rect{X: 0, Y: 0, W: 1920, H: 1080}
	secondary := Rect{X: 1920, Y: 0, W: 2560, H: 1440}
	monitors := []Rect{primary, secondary}

	const wsPopup = uint32(0x80000000)

	cases := []struct {
		name  string
		win   Rect
		mons  []Rect
		style uint32
		want  bool
	}{
		{"exact cover popup", Rect{X: 0, Y: 0, W: 1920, H: 1080}, monitors, wsPopup, true},
		{"popup with ws_border accepted", Rect{X: 0, Y: 0, W: 1920, H: 1080}, monitors, wsPopup | 0x00800000, true},
		{"exact cover zero style", Rect{X: 0, Y: 0, W: 1920, H: 1080}, monitors, 0, true},
		{"97.9 percent rejected", Rect{X: 0, Y: 0, W: 1881, H: 1080}, monitors, wsPopup, false},
		{"98.1 percent accepted", Rect{X: 0, Y: 0, W: 1882, H: 1080}, monitors, wsPopup, true},
		{"captioned maximized rejected", Rect{X: 0, Y: 0, W: 1920, H: 1080}, monitors, wsCaption, false},
		{"secondary monitor cover", Rect{X: 1920, Y: 0, W: 2560, H: 1440}, monitors, wsPopup, true},
		{"small window", Rect{X: 100, Y: 100, W: 800, H: 600}, monitors, wsPopup, false},
		{"spanning both monitors", Rect{X: 0, Y: 0, W: 4480, H: 1440}, monitors, wsPopup, true},
		{"no monitors", Rect{X: 0, Y: 0, W: 1920, H: 1080}, nil, wsPopup, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsFullscreenGeometry(tc.win, tc.mons, tc.style); got != tc.want {
				t.Fatalf("IsFullscreenGeometry(%+v, %d monitors, 0x%08X) = %v, want %v",
					tc.win, len(tc.mons), tc.style, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// S3: process allowlist matching (TDD step 1.6)
// ---------------------------------------------------------------------------

func TestMatchesProcessList(t *testing.T) {
	cases := []struct {
		name      string
		procName  string
		allowlist string
		want      bool
	}{
		{"exact match", "cs2.exe", "cs2.exe", true},
		{"basename of full path", `C:\Games\CS2\cs2.exe`, "cs2.exe", true},
		{"case insensitive", "CS2.EXE", "cs2.exe", true},
		{"list entry missing extension", "eldenring", "eldenring.exe", true},
		{"name missing extension", "eldenring.exe", "eldenring", true},
		{"no substring guard", "csnotepad.exe", "notepad", false},
		{"not in list", "notepad.exe", "cs2.exe, eldenring.exe", false},
		{"empty allowlist", "anything.exe", "", false},
		{"empty name", "", "cs2.exe", false},
		{"macOS bundle name", "Keynote.app", "keynote.app", true},
		{"macOS frontmost name", "Keynote", "keynote", true},
		{"whitespace tolerated", " keynote ", "keynote", true},
		{"full path equality", `C:\Tools\run.exe`, `c:\tools\run.exe`, true},
		{"second entry in list", "eldenring.exe", "cs2.exe, eldenring.exe", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchesProcessList(tc.procName, tc.allowlist); got != tc.want {
				t.Fatalf("MatchesProcessList(%q, %q) = %v, want %v", tc.procName, tc.allowlist, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// macOS S1 window classifier (TDD step 1.7)
// ---------------------------------------------------------------------------

func TestClassifyWindow(t *testing.T) {
	display := Rect{X: 0, Y: 0, W: 2560, H: 1440}
	full := display

	cases := []struct {
		name       string
		owner      string
		layer      int
		frame      Rect
		display    Rect
		spiceOwner string
		want       BusyState
	}{
		{"fullscreen game", "Steam", 0, full, display, "Spice", BusyFullscreen},
		{"spice itself ignored", "Spice", 0, full, display, "Spice", BusyNone},
		{"menu layer ignored", "AnyApp", 25, full, display, "Spice", BusyNone},
		{"negative layer ignored", "AnyApp", -1, full, display, "Spice", BusyNone},
		{"dock ignored", "Dock", 0, full, display, "Spice", BusyNone},
		{"window server ignored", "WindowServer", 0, full, display, "Spice", BusyNone},
		{"system ui ignored", "SystemUIServer", 0, full, display, "Spice", BusyNone},
		{"control center ignored", "Control Center", 0, full, display, "Spice", BusyNone},
		{"half cover ignored", "YouTube", 0, Rect{X: 0, Y: 0, W: 1280, H: 1440}, display, "Spice", BusyNone},
		{"zoomed below 24pt menu bar ignored", "Safari", 0, Rect{X: 0, Y: 24, W: 2560, H: 1416}, display, "Spice", BusyNone},
		{"secondary display fullscreen", "Steam", 0, Rect{X: 2560, Y: 0, W: 2560, H: 1440}, Rect{X: 2560, Y: 0, W: 2560, H: 1440}, "Spice", BusyFullscreen},
		{"case-insensitive owner", "STEAM", 0, full, display, "spice", BusyFullscreen},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyWindow(tc.owner, tc.layer, tc.frame, tc.display, tc.spiceOwner)
			if got != tc.want {
				t.Fatalf("ClassifyWindow(%q, %d, ...) = %v, want %v", tc.owner, tc.layer, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Signal combiner (TDD step 1.8)
// ---------------------------------------------------------------------------

func TestCombineBusy(t *testing.T) {
	cases := []struct {
		name      string
		shell     BusyState
		geometry  bool
		allowlist bool
		gpu       bool
		want      BusyState
	}{
		{"shell fullscreen", BusyFullscreen, false, false, false, BusyFullscreen},
		{"geometry vote", BusyNone, true, false, false, BusyFullscreen},
		{"allowlist vote", BusyNone, false, true, false, BusyFullscreen},
		{"gpu vote", BusyNone, false, false, true, BusyFullscreen},
		{"shell presentation only", BusyPresentation, false, false, false, BusyPresentation},
		{"nothing", BusyNone, false, false, false, BusyNone},
		{"any fullscreen wins over presentation", BusyPresentation, true, false, false, BusyFullscreen},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CombineBusy(tc.shell, tc.geometry, tc.allowlist, tc.gpu)
			if got != tc.want {
				t.Fatalf("CombineBusy(%v, %v, %v, %v) = %v, want %v",
					tc.shell, tc.geometry, tc.allowlist, tc.gpu, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// S5: GPU engine reduction + sustained-load hysteresis (TDD step 1.9)
// ---------------------------------------------------------------------------

func TestParseGpuEngineInstances(t *testing.T) {
	rows := []GpuEngineRow{
		{InstanceName: "pid_1234_luid_0x00000000_0x0000C822_phys_0_eng_0_engtype_3D", Value: 45.5},
		{InstanceName: "pid_1234_luid_0x00000000_0x0000C822_phys_0_eng_1_engtype_3D", Value: 12},
		{InstanceName: "pid_1234_luid_0x00000000_0x0000C822_phys_0_eng_0_engtype_VideoDecode", Value: 80},
		{InstanceName: "pid_999_luid_0x00000000_0x0000C822_phys_0_eng_0_engtype_3D", Value: 0},
		{InstanceName: "garbage", Value: 50},
	}
	want := map[int]float64{1234: 45.5, 999: 0}
	got := ParseGpuEngineInstances(rows)
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%+v)", len(got), len(want), got)
	}
	for pid, wantVal := range want {
		if gotVal, ok := got[pid]; !ok || gotVal != wantVal {
			t.Errorf("pid %d = %f (ok=%t), want %f", pid, gotVal, ok, wantVal)
		}
	}
}

func TestIsSustained3DLoad(t *testing.T) {
	cases := []struct {
		name       string
		samples    []map[int]float64
		pid        int
		threshold  float64
		minSamples int
		want       bool
	}{
		{"sustained", []map[int]float64{{1234: 45}, {1234: 50}}, 1234, 30, 2, true},
		{"not enough samples", []map[int]float64{{1234: 45}}, 1234, 30, 2, false},
		{"one sample below threshold", []map[int]float64{{1234: 10}, {1234: 50}}, 1234, 30, 2, false},
		{"pid missing from a sample", []map[int]float64{{1234: 45}, {999: 50}}, 1234, 30, 2, false},
		{"three sustained samples", []map[int]float64{{1234: 45}, {1234: 46}, {1234: 47}}, 1234, 30, 3, true},
		{"dip in tail window", []map[int]float64{{1234: 50}, {1234: 10}, {1234: 45}}, 1234, 30, 3, false},
		{"zero min samples treated as one", []map[int]float64{{1234: 45}}, 1234, 30, 0, true},
		{"empty samples", nil, 1234, 30, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSustained3DLoad(tc.samples, tc.pid, tc.threshold, tc.minSamples); got != tc.want {
				t.Fatalf("IsSustained3DLoad(..., pid=%d, thr=%f, min=%d) = %v, want %v",
					tc.pid, tc.threshold, tc.minSamples, got, tc.want)
			}
		})
	}
}

func TestIsWindowsShellClass(t *testing.T) {
	cases := []struct {
		cls  string
		want bool
	}{
		{"Progman", true},
		{"progman", true},
		{"WorkerW", true},
		{"Shell_TrayWnd", true},
		{"Shell_SecondaryTrayWnd", true},
		{"  WorkerW  ", true},
		{"UnrealWindow", false},
		{"SDL_app", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsWindowsShellClass(tc.cls); got != tc.want {
			t.Errorf("IsWindowsShellClass(%q) = %v, want %v", tc.cls, got, tc.want)
		}
	}
}
