package wallpaper

import (
	"fmt"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/activity"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

const (
	activityGateInterval = 15 * time.Second
	gateEngageStreak     = 2 // Consecutive busy samples to engage (~30s)
	gateReleaseStreak    = 3 // Consecutive clear samples to release (~45s)
	batteryReleaseMargin = 10
	gpu3DThresholdPct    = 30.0
	gpuMinSustained      = 2
)

// ActivityGate manages session-busy and power gating for the Plugin.
// It inhibits automatic rotation and background fetches via wp.gateSuppressed
// without mutating per-monitor user pause state (mc.State.Paused).
type ActivityGate struct {
	wp       *Plugin
	detector activity.Detector

	mu             sync.Mutex
	busyStreak     int
	clearStreak    int
	engaged        bool
	batteryLatched bool
	engageReason   string
	graceUntil     time.Time
	gpuSamples     []map[int]float64

	wakeCh   chan struct{}
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewActivityGate constructs a gate with the injected detector.
func NewActivityGate(wp *Plugin, det activity.Detector) *ActivityGate {
	if det == nil {
		det = activity.New()
	}
	if wp != nil && wp.gateSuppressed != nil {
		wp.gateSuppressed.Set(false)
	}
	return &ActivityGate{
		wp:       wp,
		detector: det,
		wakeCh:   make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
	}
}

// Start launches the 60-second polling loop.
func (g *ActivityGate) Start() {
	go g.runLoop(activityGateInterval)
}

func (g *ActivityGate) runLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			g.tick()
		case <-g.wakeCh:
			g.tick()
		case <-g.stopCh:
			return
		}
	}
}

// Wakeup triggers an immediate out-of-band evaluation tick (e.g. when the user
// toggles Game Mode in the tray or applies Activity settings).
func (g *ActivityGate) Wakeup() {
	select {
	case g.wakeCh <- struct{}{}:
	default:
	}
}

// Stop terminates the polling loop. Idempotent.
func (g *ActivityGate) Stop() {
	g.stopOnce.Do(func() {
		close(g.stopCh)
		g.mu.Lock()
		if g.wp != nil && g.wp.gateSuppressed != nil {
			g.wp.gateSuppressed.Set(false)
		}
		g.engaged = false
		g.engageReason = ""
		g.mu.Unlock()
	})
}

// IsEngaged reports whether the gate is currently inhibiting rotation.
func (g *ActivityGate) IsEngaged() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.engaged
}

// NotifyUserUnpaused records a manual user override, immediately releasing the
// gate if engaged and starting the grace period.
func (g *ActivityGate) NotifyUserUnpaused() {
	g.mu.Lock()
	defer g.mu.Unlock()
	mins := 2
	if g.wp != nil && g.wp.cfg != nil {
		mins = g.wp.cfg.GetGateGraceMinutes()
		if g.wp.cfg.GetGameMode() {
			g.wp.cfg.SetGameMode(false)
		}
	}
	g.graceUntil = time.Now().Add(time.Duration(mins) * time.Minute)
	g.busyStreak = 0
	g.gpuSamples = nil
	if g.engaged {
		g.releaseLocked()
	}
}

func (g *ActivityGate) tick() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.wp == nil || g.wp.cfg == nil {
		return
	}

	busyVote, busyReason := g.evaluateBusyLocked()
	battVote, battReason := g.evaluateBatteryLocked()
	gameMode := g.wp.cfg.GetGameMode()

	anyEngageCandidate := gameMode || battVote || busyVote
	candidateReason := ""
	switch {
	case gameMode:
		candidateReason = "game_mode"
	case battVote:
		candidateReason = battReason
	case busyVote:
		candidateReason = busyReason
	}

	if g.engaged {
		if anyEngageCandidate {
			g.engageReason = candidateReason
			g.clearStreak = 0
			return
		}
		// If the gate's engagement condition is no longer enabled in preferences,
		// release immediately without waiting for the clear countdown.
		if g.isReasonDisabledByConfigLocked() {
			g.releaseLocked()
			return
		}
		g.clearStreak++
		if g.clearStreak >= gateReleaseStreak {
			g.releaseLocked()
		}
		return
	}

	if !anyEngageCandidate {
		g.busyStreak = 0
		return
	}

	immediate := gameMode || battVote || candidateReason == "allowlist" || candidateReason == "gpu_3d"
	if immediate {
		g.engageLocked(candidateReason)
		return
	}

	g.busyStreak++
	if g.busyStreak >= gateEngageStreak {
		g.engageLocked(candidateReason)
	}
}

func (g *ActivityGate) isReasonDisabledByConfigLocked() bool {
	if g.wp == nil || g.wp.cfg == nil {
		return false
	}
	switch g.engageReason {
	case "game_mode":
		return !g.wp.cfg.GetGameMode()
	case "battery":
		return !g.wp.cfg.GetBatterySaver()
	default: // "busy", "allowlist", "gpu_3d"
		return !g.wp.cfg.GetPauseWhenBusy()
	}
}

func (g *ActivityGate) evaluateBusyLocked() (bool, string) {
	if !g.wp.cfg.GetPauseWhenBusy() {
		g.gpuSamples = nil
		return false, ""
	}

	// S3 allowlist check (deterministic; bypasses grace period).
	if fg, ok := g.detector.ForegroundProcess(); ok && fg != "" {
		allowlist := g.wp.cfg.GetGateGameProcesses()
		if activity.MatchesProcessList(fg, allowlist) {
			return true, "allowlist"
		}
	}

	// Grace period suppresses heuristic busy signals (S1/S2/S5).
	if time.Now().Before(g.graceUntil) {
		return false, ""
	}

	busy := g.detector.UserBusy()
	if busy != activity.BusyNone {
		return true, "busy"
	}

	// S5 optional GPU 3D load check attributed to the foreground PID.
	if gpuDet, ok := g.detector.(activity.GpuDetector); ok {
		fgPID, pidOK := gpuDet.ForegroundPID()
		pidPct, gpuOK := gpuDet.GpuLoad3D()
		if pidOK && gpuOK && len(pidPct) > 0 {
			g.gpuSamples = append(g.gpuSamples, pidPct)
			if len(g.gpuSamples) > gpuMinSustained {
				g.gpuSamples = g.gpuSamples[len(g.gpuSamples)-gpuMinSustained:]
			}
			if activity.IsSustained3DLoad(g.gpuSamples, fgPID, gpu3DThresholdPct, gpuMinSustained) {
				return true, "gpu_3d"
			}
		} else {
			g.gpuSamples = nil
		}
	}

	return false, ""
}

func (g *ActivityGate) evaluateBatteryLocked() (bool, string) {
	if !g.wp.cfg.GetBatterySaver() {
		g.batteryLatched = false
		return false, ""
	}
	batt := g.detector.Battery()
	thr := g.wp.cfg.BatterySaverThresholdPct()

	if g.batteryLatched {
		if batt.OnAC || !batt.HasBattery || batt.Percent >= thr+batteryReleaseMargin {
			g.batteryLatched = false
			return false, ""
		}
		return true, "battery"
	}

	if batt.Low(thr) {
		g.batteryLatched = true
		return true, "battery"
	}
	return false, ""
}

func (g *ActivityGate) engageLocked(reason string) {
	g.engaged = true
	g.engageReason = reason
	g.busyStreak = 0
	g.clearStreak = 0

	if g.wp.gateSuppressed != nil {
		g.wp.gateSuppressed.Set(true)
	}

	log.Debugf("[ActivityGate] Engaging (%s); inhibiting auto-rotation and background fetches", reason)

	title := i18n.T("Wallpaper Rotation Paused")
	msg := ""
	switch reason {
	case "game_mode":
		msg = i18n.T("Game Mode enabled: rotation paused.")
	case "battery":
		msg = i18n.T("Battery low: paused to save power.")
	case "allowlist":
		msg = i18n.T("Game running: rotation paused.")
	default:
		msg = i18n.T("Fullscreen app detected: rotation paused.")
	}
	if g.wp.manager != nil {
		g.wp.manager.RebuildTrayMenu()
		g.wp.manager.NotifyUser(title, msg)
	}
}

func (g *ActivityGate) releaseLocked() {
	g.engaged = false
	g.engageReason = ""
	g.clearStreak = 0
	g.busyStreak = 0
	g.batteryLatched = false

	if g.wp.gateSuppressed != nil {
		g.wp.gateSuppressed.Set(false)
	}

	log.Debugf("[ActivityGate] Releasing; resuming normal rotation")

	if g.wp.manager != nil {
		g.wp.manager.RebuildTrayMenu()
		g.wp.manager.NotifyUser(
			i18n.T("Wallpaper Rotation Resumed"),
			i18n.T("Activity cleared: resuming normal rotation."),
		)
	}
}

func (g *ActivityGate) String() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return fmt.Sprintf("engaged=%t reason=%s", g.engaged, g.engageReason)
}
