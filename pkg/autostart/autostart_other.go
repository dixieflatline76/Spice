//go:build !darwin
// +build !darwin

package autostart

// Supported always reports false off macOS. Windows autostart is configured by
// the installer (msix/AppxManifest.xml, Spice.iss), not at runtime.
func Supported() bool { return false }

// Get reports that no runtime login-item mechanism exists.
func Get() (Status, error) { return StatusUnsupported, ErrUnsupportedOS }

// Enable is a no-op that reports the platform has no login-item API.
func Enable() error { return ErrUnsupportedOS }

// Disable is a no-op that reports the platform has no login-item API.
func Disable() error { return ErrUnsupportedOS }

// OpenLoginItemsSettings is a no-op off macOS.
func OpenLoginItemsSettings() error { return ErrUnsupportedOS }

// Diagnose reports the zero diagnostic; no condition is satisfiable here.
func Diagnose() Diagnostic { return Diagnostic{} }
