package wallpaper

import (
	"fmt"
	"image"
	"net/url"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/v2/asset"
	"github.com/dixieflatline76/Spice/v2/pkg/activity"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/util"
)

// fakeGateDetector is a test double that lets unit tests script exact
// sequences of busy states, battery levels, and foreground processes.
type fakeGateDetector struct {
	mu     sync.Mutex
	busy   activity.BusyState
	batt   activity.BatteryState
	fgName string
	fgOK   bool
	fgPID  int
	gpu    map[int]float64
	gpuOK  bool
}

func (f *fakeGateDetector) UserBusy() activity.BusyState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.busy
}

func (f *fakeGateDetector) Battery() activity.BatteryState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.batt
}

func (f *fakeGateDetector) ForegroundProcess() (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fgName, f.fgOK
}

func (f *fakeGateDetector) ForegroundPID() (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fgPID <= 0 {
		return 0, false
	}
	return f.fgPID, true
}

func (f *fakeGateDetector) GpuLoad3D() (map[int]float64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gpu, f.gpuOK
}

func (f *fakeGateDetector) setBusy(b activity.BusyState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.busy = b
}

func (f *fakeGateDetector) setBattery(b activity.BatteryState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batt = b
}

func (f *fakeGateDetector) setForeground(name string, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fgName = name
	f.fgOK = ok
}

func (f *fakeGateDetector) setForegroundPID(pid int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fgPID = pid
}

func (f *fakeGateDetector) setGpu(m map[int]float64, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gpu = m
	f.gpuOK = ok
}

// recordingNotifier records notifications for assertion.
type recordingNotifier struct {
	mu     sync.Mutex
	titles []string
	msgs   []string
}

func (r *recordingNotifier) Notify(title, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.titles = append(r.titles, title)
	r.msgs = append(r.msgs, message)
}

func (r *recordingNotifier) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.msgs)
}

// helper: builds a minimal Plugin with mock monitor controllers for testing
// the gate without spinning up the entire wallpaper engine.
func newTestGatePlugin(t *testing.T, monitorCount int) (*Plugin, *recordingNotifier) {
	t.Helper()
	p := NewMockPreferences()
	cfg := &Config{
		Preferences:        p,
		MonitorPauseStates: make(map[string]bool),
	}
	notifier := &recordingNotifier{}

	wp := &Plugin{
		cfg:            cfg,
		gateSuppressed: util.NewSafeBool(),
		Monitors:       make(map[int]*MonitorController),
		manager:        &mockGatePluginManager{notifier: notifier.Notify},
	}

	for i := 0; i < monitorCount; i++ {
		mc := &MonitorController{
			ID:               i,
			Commands:         make(chan Command, 10),
			State:            &MonitorState{Paused: false},
			Monitor:          Monitor{DevicePath: fmt.Sprintf("monitor-%d", i)},
			IsGateSuppressed: wp.IsGateSuppressed,
		}
		wp.Monitors[i] = mc
	}

	return wp, notifier
}

// mockGatePluginManager is a minimal ui.PluginManager that records
// notifications; every other UI surface is inert.
type mockGatePluginManager struct {
	notifier func(title, message string)
}

func (m *mockGatePluginManager) Register(ui.Plugin)   {}
func (m *mockGatePluginManager) Deregister(ui.Plugin) {}

func (m *mockGatePluginManager) NotifyUser(title, message string) {
	if m.notifier != nil {
		m.notifier(title, message)
	}
}

func (m *mockGatePluginManager) RegisterNotifier(ui.Notifier) {}

func (m *mockGatePluginManager) CreateMenuItem(string, func(), string) *fyne.MenuItem {
	return nil
}

func (m *mockGatePluginManager) CreateToggleMenuItem(string, func(bool), string, bool) *fyne.MenuItem {
	return nil
}

func (m *mockGatePluginManager) OpenURL(*url.URL) error { return nil }

func (m *mockGatePluginManager) OpenPreferences(string)            {}
func (m *mockGatePluginManager) GetPreferences() fyne.Preferences  { return nil }
func (m *mockGatePluginManager) GetAssetManager() *asset.Manager   { return nil }
func (m *mockGatePluginManager) RefreshTrayMenu()                  {}
func (m *mockGatePluginManager) RebuildTrayMenu()                  {}
func (m *mockGatePluginManager) RebuildTrayMenuImmediate()         {}
func (m *mockGatePluginManager) ShowTuneImagePopup(int, image.Rectangle, provider.TuningOptions, provider.TuningOptions, [9]string, [9]provider.CropAnchor, bool, func(provider.TuningOptions, func()), func()) {
}

// ---------------------------------------------------------------------------
// TDD tests: Rules R1a–R9
// ---------------------------------------------------------------------------

func TestGate_Construction(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 2)
	det := &fakeGateDetector{}
	gate := NewActivityGate(wp, det)
	if gate == nil {
		t.Fatal("NewActivityGate returned nil")
	}
	if gate.IsEngaged() {
		t.Error("gate must not be engaged on construction")
	}
}

func TestGate_EngagesAfterTwoBusySamples(t *testing.T) {
	wp, notif := newTestGatePlugin(t, 2)
	det := &fakeGateDetector{}
	gate := NewActivityGate(wp, det)

	det.setBusy(activity.BusyFullscreen)

	// Sample 1: not yet engaged (hysteresis)
	gate.tick()
	if gate.IsEngaged() {
		t.Fatal("gate engaged after only 1 sample, want 2")
	}

	// Sample 2: should engage
	gate.tick()
	if !gate.IsEngaged() {
		t.Fatal("gate did not engage after 2 busy samples")
	}
	if !wp.gateSuppressed.Value() {
		t.Error("gateSuppressed must be true while engaged")
	}
	if notif.count() != 1 {
		t.Errorf("expected 1 notification on engage, got %d", notif.count())
	}

	// Verify gate sets gateSuppressed without mutating mc.State.Paused or sending CmdPause
	for id, mc := range wp.Monitors {
		if mc.State.Paused {
			t.Errorf("monitor %d State.Paused was mutated by gate", id)
		}
		if len(mc.Commands) != 0 {
			t.Errorf("monitor %d received unexpected command from gate", id)
		}
	}
}

func TestGate_EngagesOnLowBattery(t *testing.T) {
	wp, notif := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	gate := NewActivityGate(wp, det)

	det.setBattery(activity.BatteryState{OnAC: false, HasBattery: true, Percent: 15})
	// Low battery engages on first sample
	gate.tick()
	if !gate.IsEngaged() {
		t.Fatal("gate should engage on low battery")
	}
	if notif.count() != 1 {
		t.Errorf("expected 1 notification, got %d", notif.count())
	}
}

func TestGate_DoesNotEngageWhileOnAC(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	gate := NewActivityGate(wp, det)

	det.setBattery(activity.BatteryState{OnAC: true, HasBattery: true, Percent: 5})
	gate.tick()
	gate.tick()
	if gate.IsEngaged() {
		t.Error("gate must never engage on battery while plugged into AC")
	}
}

func TestGate_ReleasesAfterThreeClearSamples(t *testing.T) {
	wp, notif := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	gate := NewActivityGate(wp, det)

	// Engage
	det.setBusy(activity.BusyFullscreen)
	gate.tick()
	gate.tick()
	if !gate.IsEngaged() {
		t.Fatal("precondition failed: not engaged")
	}

	// Become clear
	det.setBusy(activity.BusyNone)

	// Clear 1
	gate.tick()
	if !gate.IsEngaged() {
		t.Fatal("released after 1 clear, want 3")
	}
	// Clear 2
	gate.tick()
	if !gate.IsEngaged() {
		t.Fatal("released after 2 clears, want 3")
	}
	// Clear 3: should release
	gate.tick()
	if gate.IsEngaged() {
		t.Fatal("still engaged after 3 clear samples")
	}
	if wp.gateSuppressed.Value() {
		t.Error("gateSuppressed must be false after release")
	}
	if notif.count() != 2 {
		t.Errorf("expected 2 notifications total (engage + release), got %d", notif.count())
	}
}

func TestGate_BatteryReleaseMargin(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	gate := NewActivityGate(wp, det)

	// Engage at 20%
	det.setBattery(activity.BatteryState{OnAC: false, HasBattery: true, Percent: 15})
	gate.tick()
	if !gate.IsEngaged() {
		t.Fatal("not engaged")
	}

	// Charge up to 24% (threshold 20 + margin 10 = 30% needed): should NOT release
	det.setBattery(activity.BatteryState{OnAC: false, HasBattery: true, Percent: 24})
	for i := 0; i < 5; i++ {
		gate.tick()
	}
	if !gate.IsEngaged() {
		t.Fatal("released before reaching threshold+margin")
	}

	// Reach 31%: release countdown begins
	det.setBattery(activity.BatteryState{OnAC: false, HasBattery: true, Percent: 31})
	for i := 0; i < 3; i++ {
		gate.tick()
	}
	if gate.IsEngaged() {
		t.Fatal("did not release after reaching threshold+margin")
	}
}

func TestGate_ACRestoresBatteryEngagedImmediately(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	gate := NewActivityGate(wp, det)

	det.setBattery(activity.BatteryState{OnAC: false, HasBattery: true, Percent: 15})
	gate.tick()
	if !gate.IsEngaged() {
		t.Fatal("not engaged")
	}

	// Plug into AC: clears battery busy condition immediately
	det.setBattery(activity.BatteryState{OnAC: true, HasBattery: true, Percent: 15})
	for i := 0; i < 3; i++ {
		gate.tick()
	}
	if gate.IsEngaged() {
		t.Fatal("plugging into AC should allow gate to release")
	}
}

func TestGate_PreservesUserPausedMonitorState(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 2)
	det := &fakeGateDetector{}
	gate := NewActivityGate(wp, det)

	// User pre-paused monitor 1, monitor 0 is playing
	wp.Monitors[0].State.Paused = false
	wp.Monitors[1].State.Paused = true

	// Engage
	det.setBusy(activity.BusyFullscreen)
	gate.tick()
	gate.tick()
	if !gate.IsEngaged() || !wp.IsGateSuppressed() {
		t.Fatal("expected gate to be engaged and suppressed")
	}

	// User-paused states must remain strictly untouched during engagement
	if wp.Monitors[0].State.Paused != false || wp.Monitors[1].State.Paused != true {
		t.Fatal("gate must never mutate per-monitor State.Paused")
	}

	// Release
	det.setBusy(activity.BusyNone)
	for i := 0; i < 3; i++ {
		gate.tick()
	}
	if gate.IsEngaged() || wp.IsGateSuppressed() {
		t.Fatal("expected gate to release")
	}
	if wp.Monitors[0].State.Paused != false || wp.Monitors[1].State.Paused != true {
		t.Fatal("user pause states must remain intact after release")
	}
}

func TestGate_AllowlistEngagesImmediately(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	wp.cfg.SetGateGameProcesses("cs2.exe, eldenring.exe")

	det := &fakeGateDetector{}
	det.setBusy(activity.BusyNone) // shell says not busy
	det.setForeground("cs2.exe", true)

	gate := NewActivityGate(wp, det)
	gate.tick()

	if !gate.IsEngaged() {
		t.Fatal("allowlist match must engage on first tick (deterministic)")
	}
}

func TestGate_GameModeToggleEngagesAndHolds(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	det.setBusy(activity.BusyNone)

	gate := NewActivityGate(wp, det)

	wp.cfg.SetGameMode(true)
	gate.tick()
	if !gate.IsEngaged() {
		t.Fatal("GameMode=true must engage the gate")
	}

	// Detector stays clear for 10 ticks: must NOT release while GameMode is on
	for i := 0; i < 10; i++ {
		gate.tick()
	}
	if !gate.IsEngaged() {
		t.Fatal("gate released while GameMode was still on")
	}

	// Turn GameMode off: releases immediately on next tick when no other signal is active
	wp.cfg.SetGameMode(false)
	gate.tick()
	if gate.IsEngaged() {
		t.Fatal("gate should release immediately when GameMode is toggled off")
	}
}

func TestGate_DisabledByPrefs(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	wp.cfg.SetPauseWhenBusy(false)
	wp.cfg.SetBatterySaver(false)

	det := &fakeGateDetector{}
	det.setBusy(activity.BusyFullscreen)
	det.setBattery(activity.BatteryState{OnAC: false, HasBattery: true, Percent: 5})

	gate := NewActivityGate(wp, det)
	for i := 0; i < 5; i++ {
		gate.tick()
	}
	if gate.IsEngaged() {
		t.Error("gate must remain inert when both features are disabled in prefs")
	}
}

func TestGate_GracePeriodAfterManualUnpause(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	det.setBusy(activity.BusyFullscreen)

	gate := NewActivityGate(wp, det)
	wp.cfg.SetGateGraceMinutes(2)

	// User explicitly unpauses
	gate.NotifyUserUnpaused()

	// Busy ticks during grace period must NOT engage
	gate.tick()
	gate.tick()
	if gate.IsEngaged() {
		t.Fatal("gate engaged during grace period")
	}

	// Advance clock past grace period
	gate.graceUntil = time.Now().Add(-1 * time.Second)
	gate.tick()
	gate.tick()
	if !gate.IsEngaged() {
		t.Fatal("gate should engage after grace period expires")
	}
}

func TestGate_NotifyUserUnpausedReleasesImmediatelyAndStartsGrace(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	det.setBusy(activity.BusyFullscreen)

	gate := NewActivityGate(wp, det)
	gate.tick()
	gate.tick()
	if !gate.IsEngaged() {
		t.Fatal("precondition: engaged")
	}

	// Manual user unpause releases immediately and starts grace window
	gate.NotifyUserUnpaused()
	if gate.IsEngaged() || wp.IsGateSuppressed() {
		t.Fatal("NotifyUserUnpaused must immediately release the gate")
	}
}

func TestGate_StopIsIdempotent(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	gate := NewActivityGate(wp, det)
	gate.Start()

	// Stopping twice must not panic
	gate.Stop()
	gate.Stop()
}

func TestGate_NilDetectorFallsBackToPlatformDefault(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	gate := NewActivityGate(wp, nil) // nil detector must be substituted
	if gate.detector == nil {
		t.Fatal("nil detector was not replaced with the platform default")
	}

	// A nil-cfg plugin must also be tolerated by tick (defensive guard).
	bare := NewActivityGate(&Plugin{}, &fakeGateDetector{})
	bare.tick() // must not panic
}

func TestGate_RunLoopTicksAndStops(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	det.setBusy(activity.BusyFullscreen)

	gate := NewActivityGate(wp, det)

	done := make(chan struct{})
	go func() {
		// Drive the full production loop body with a short interval.
		gate.runLoop(10 * time.Millisecond)
		close(done)
	}()

	// Wait for enough ticks to engage (needs 2 busy samples).
	deadline := time.After(2 * time.Second)
	engaged := false
	for !engaged {
		select {
		case <-deadline:
			t.Fatal("loop never engaged the gate within deadline")
		default:
		}
		if gate.IsEngaged() {
			engaged = true
		} else {
			time.Sleep(5 * time.Millisecond)
		}
	}

	// Stop must terminate the loop goroutine.
	gate.Stop()
	select {
	case <-done:
		// loop exited cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("runLoop did not exit after Stop")
	}
}

func TestGate_Gpu3DLoadEngages(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	det.setBusy(activity.BusyNone)
	det.setForegroundPID(1234)
	det.setGpu(map[int]float64{1234: 55.0}, true)

	gate := NewActivityGate(wp, det)
	// Sample 1: records 1st GPU sample (IsSustained3DLoad requires 2 samples)
	gate.tick()
	if gate.IsEngaged() {
		t.Fatal("gate engaged after only 1 GPU sample, want 2 sustained samples")
	}
	// Sample 2: records 2nd consecutive GPU sample -> sustained load true -> engages immediately
	gate.tick()

	if !gate.IsEngaged() {
		t.Fatal("sustained foreground 3D GPU load must engage the gate on 2nd sample")
	}
	if got := gate.String(); got != "engaged=true reason=gpu_3d" {
		t.Errorf("String() = %q, want engaged=true reason=gpu_3d", got)
	}
}

func TestGate_GpuBackgroundPIDIgnored(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	det.setBusy(activity.BusyNone)
	det.setForegroundPID(1234)                   // Foreground is PID 1234 (0% GPU)
	det.setGpu(map[int]float64{9999: 95.0}, true) // Background PID 9999 has 95% GPU

	gate := NewActivityGate(wp, det)
	for i := 0; i < 5; i++ {
		gate.tick()
	}
	if gate.IsEngaged() {
		t.Fatal("background 3D GPU load on non-foreground PID must not engage the gate")
	}
}

func TestGate_GpuLoadBelowThresholdDoesNotEngage(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	det.setGpu(map[int]float64{1234: 25.0}, true) // below 30% threshold

	gate := NewActivityGate(wp, det)
	for i := 0; i < 3; i++ {
		gate.tick()
	}
	if gate.IsEngaged() {
		t.Error("GPU load below threshold must not engage the gate")
	}
}

func TestGate_GpuUnavailableDoesNotEngage(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	det.setGpu(nil, false) // no GPU metrics (e.g. macOS / missing counters)

	gate := NewActivityGate(wp, det)
	for i := 0; i < 3; i++ {
		gate.tick()
	}
	if gate.IsEngaged() {
		t.Error("unavailable GPU metrics must not engage the gate")
	}
}

func TestGate_StringSnapshot(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 2)
	det := &fakeGateDetector{}
	det.setBusy(activity.BusyFullscreen)

	gate := NewActivityGate(wp, det)
	if got := gate.String(); got != "engaged=false reason=" {
		t.Errorf("String() before engage = %q", got)
	}

	gate.tick()
	gate.tick()

	want := "engaged=true reason=busy"
	if got := gate.String(); got != want {
		t.Errorf("String() after engage = %q, want %q", got, want)
	}
}

func TestUI_ActivityGateTrayAndAccordionIcons(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	wp.Monitors[0].State.CurrentID = "img-1"

	// 1. When gate is suppressed, tray menu must show "Resume Play" even though mc.State.Paused is false
	wp.gateSuppressed.Set(true)
	menu := wp.CreateTrayMenu()
	foundResume := false
	for _, item := range menu.Items {
		if item.Label == "Resume Play" {
			foundResume = true
			break
		}
	}
	if !foundResume {
		t.Error("CreateTrayMenu must show 'Resume Play' when gateSuppressed is true")
	}

	// 2. Accordion icon for "Activity & Power" (index 4) must be "pause" and "Actions" (index 5) must be "refresh"
	builder := NewPrefsPanelBuilder(wp, nil)
	acc := builder.BuildGeneralTabAccordion()
	if len(acc.Items) < 6 {
		t.Fatalf("expected 6 accordion sections, got %d", len(acc.Items))
	}
	if acc.Items[4].IconName != "pause" {
		t.Errorf("section 4 (%s) icon = %q, want \"pause\"", acc.Items[4].Title, acc.Items[4].IconName)
	}
	if acc.Items[5].IconName != "refresh" {
		t.Errorf("section 5 (%s) icon = %q, want \"refresh\"", acc.Items[5].Title, acc.Items[5].IconName)
	}

	// 3. gateGraceMinutes validator check
	generalSchema := builder.BuildGeneralTabSchema()
	var graceItem schema.TextItem
	foundGrace := false
	for _, section := range generalSchema.Sections {
		for _, item := range section.Items {
			if ti, ok := item.(schema.TextItem); ok && ti.Name == "gateGraceMinutes" {
				graceItem = ti
				foundGrace = true
				break
			}
		}
	}
	if !foundGrace || graceItem.Validator == nil {
		t.Fatal("expected gateGraceMinutes TextItem with Validator")
	}
	if err := graceItem.Validator("2"); err != nil {
		t.Errorf("validator rejected valid '2': %v", err)
	}
	if err := graceItem.Validator("-1"); err == nil {
		t.Error("validator accepted invalid '-1'")
	}
	if err := graceItem.Validator("abc"); err == nil {
		t.Error("validator accepted non-numeric 'abc'")
	}

	// 4. gateGameProcesses AppPickerItem add/remove/persistence check
	var pickerItem schema.AppPickerItem
	foundPicker := false
	for _, section := range generalSchema.Sections {
		for _, item := range section.Items {
			if ap, ok := item.(schema.AppPickerItem); ok && ap.Name == "gateGameProcesses" {
				pickerItem = ap
				foundPicker = true
				break
			}
		}
	}
	if !foundPicker || pickerItem.OnAdd == nil || pickerItem.OnRemove == nil || pickerItem.ApplyFunc == nil {
		t.Fatal("expected gateGameProcesses AppPickerItem with OnAdd, OnRemove, ApplyFunc")
	}
	if err := pickerItem.OnAdd("cs2.exe"); err != nil {
		t.Fatalf("OnAdd cs2.exe failed: %v", err)
	}
	if err := pickerItem.OnAdd("eldenring.exe"); err != nil {
		t.Fatalf("OnAdd eldenring.exe failed: %v", err)
	}
	if err := pickerItem.OnAdd("CS2.EXE"); err != nil {
		t.Fatalf("OnAdd duplicate CS2.EXE failed: %v", err)
	}
	apps := wp.cfg.GetGameProcesses()
	if len(apps) != 2 || apps[0] != "cs2.exe" || apps[1] != "eldenring.exe" {
		t.Fatalf("GetGameProcesses() after add = %v, want [cs2.exe eldenring.exe]", apps)
	}
	if err := pickerItem.OnRemove("cs2.exe"); err != nil {
		t.Fatalf("OnRemove cs2.exe failed: %v", err)
	}
	apps = wp.cfg.GetGameProcesses()
	if len(apps) != 1 || apps[0] != "eldenring.exe" {
		t.Fatalf("GetGameProcesses() after remove = %v, want [eldenring.exe]", apps)
	}
	pickerItem.ApplyFunc([]string{"Hades.exe"})
	apps = wp.cfg.GetGameProcesses()
	if len(apps) != 1 || apps[0] != "Hades.exe" {
		t.Fatalf("GetGameProcesses() after ApplyFunc = %v, want [Hades.exe]", apps)
	}
}

func TestGate_DisablingActiveFeatureReleasesImmediately(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	det.setBusy(activity.BusyFullscreen)

	gate := NewActivityGate(wp, det)
	gate.tick()
	gate.tick()
	if !gate.IsEngaged() || gate.engageReason != "busy" {
		t.Fatalf("precondition: expected gate engaged with reason busy, got %s", gate.engageReason)
	}

	// Disabling PauseWhenBusy while BatterySaver remains true must release immediately
	wp.cfg.SetPauseWhenBusy(false)
	gate.tick()
	if gate.IsEngaged() || wp.IsGateSuppressed() {
		t.Fatal("disabling PauseWhenBusy must release the gate immediately")
	}

	// Re-enable and engage on battery
	wp.cfg.SetPauseWhenBusy(true)
	det.setBusy(activity.BusyNone)
	det.setBattery(activity.BatteryState{OnAC: false, HasBattery: true, Percent: 15})
	gate.tick()
	if !gate.IsEngaged() || gate.engageReason != "battery" {
		t.Fatalf("precondition: expected gate engaged with reason battery, got %s", gate.engageReason)
	}

	// Disabling BatterySaver while PauseWhenBusy remains true must release immediately
	wp.cfg.SetBatterySaver(false)
	gate.tick()
	if gate.IsEngaged() || wp.IsGateSuppressed() {
		t.Fatal("disabling BatterySaver must release the gate immediately")
	}
}

func TestGate_StopClearsGateSuppressed(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	det.setBusy(activity.BusyFullscreen)

	gate := NewActivityGate(wp, det)
	gate.tick()
	gate.tick()
	if !gate.IsEngaged() || !wp.IsGateSuppressed() {
		t.Fatal("precondition failed: gate not suppressed")
	}

	gate.Stop()
	if wp.IsGateSuppressed() {
		t.Fatal("Stop() must reset gateSuppressed to false")
	}
}

func TestGate_WakeupTriggersImmediateTick(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	det := &fakeGateDetector{}
	gate := NewActivityGate(wp, det)

	done := make(chan struct{})
	go func() {
		gate.runLoop(10 * time.Second) // Very long ticker interval
		close(done)
	}()
	defer func() {
		gate.Stop()
		<-done
	}()

	wp.cfg.SetGameMode(true)
	gate.Wakeup()

	deadline := time.After(1 * time.Second)
	engaged := false
	for !engaged {
		select {
		case <-deadline:
			t.Fatal("Wakeup() did not trigger immediate evaluation within deadline")
		default:
		}
		if gate.IsEngaged() {
			engaged = true
		} else {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestGate_TogglePauseMonitorActionWhileEngaged(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 2)
	det := &fakeGateDetector{}
	wp.gate = NewActivityGate(wp, det)
	det.setBusy(activity.BusyFullscreen)
	wp.gate.tick()
	wp.gate.tick()
	wp.cfg.SetGameMode(true)

	if !wp.gate.IsEngaged() {
		t.Fatal("precondition failed: gate not engaged")
	}

	// 1. Resume specific monitor (Monitor 0)
	wp.TogglePauseMonitorAction(0)
	if wp.gate.IsEngaged() || wp.IsGateSuppressed() {
		t.Fatal("TogglePauseMonitorAction(0) while engaged must release gate")
	}
	if wp.cfg.GetGameMode() {
		t.Fatal("TogglePauseMonitorAction must clear GameMode")
	}
	if wp.Monitors[0].State.Paused {
		t.Fatal("Monitor 0 must remain unpaused")
	}

	// 2. Global resume (-1) when engaged
	wp.gate.graceUntil = time.Time{} // Clear grace period from step 1
	det.setBusy(activity.BusyFullscreen)
	wp.gate.tick()
	wp.gate.tick()
	if !wp.gate.IsEngaged() {
		t.Fatal("precondition failed: gate not re-engaged")
	}

	wp.TogglePauseMonitorAction(-1)
	if wp.gate.IsEngaged() || wp.IsGateSuppressed() {
		t.Fatal("TogglePauseMonitorAction(-1) while engaged must release gate")
	}
	if wp.Monitors[0].State.Paused || wp.Monitors[1].State.Paused {
		t.Fatal("monitors must remain unpaused after releasing gate via global resume")
	}
}

func TestScheduler_NightlyRefreshDeferredWhenGateSuppressed(t *testing.T) {
	wp, _ := newTestGatePlugin(t, 1)
	wp.gateSuppressed.Set(true)

	now := time.Now()
	// When gate is suppressed, checkAndRunRefresh must defer and return lastRefreshDay (-1)
	result := wp.checkAndRunRefresh(now, -1, false)
	if result != -1 {
		t.Fatalf("expected checkAndRunRefresh to return -1 (deferred), got %d", result)
	}
}

