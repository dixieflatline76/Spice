//go:build !windows && !darwin
// +build !windows,!darwin

package activity

// stubDetector is the silent no-op detector used on unsupported platforms
// (Linux today — mirrors pkg/hotkey/hotkey_linux.go precedent until the v3.2
// Wayland milestone adds native D-Bus / compositing hooks).
type stubDetector struct{}

// New returns the platform Detector hook (stub on non-Windows/non-macOS).
func New() Detector { return &stubDetector{} }

func (s *stubDetector) UserBusy() BusyState               { return BusyNone }
func (s *stubDetector) Battery() BatteryState             { return BatteryState{} }
func (s *stubDetector) ForegroundProcess() (string, bool) { return "", false }
