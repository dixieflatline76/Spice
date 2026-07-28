// Package autostart manages whether the application launches automatically
// when the user logs in.
//
// On macOS this wraps SMAppService (macOS 13+), which registers the running
// .app bundle itself as a login item. There is deliberately no LaunchAgent
// plist fallback: writing to ~/Library/LaunchAgents is forbidden under the
// App Store sandbox, and the legacy SMLoginItemSetEnabled API requires a
// separate helper bundle and is deprecated.
//
// On every other platform the package compiles to no-ops that report
// ErrUnsupportedOS. Windows autostart is handled by installer metadata
// (msix/AppxManifest.xml and Spice.iss), not at runtime.
package autostart

import (
	"errors"
	"path/filepath"
	"strings"
)

// Errors returned by this package.
var (
	// ErrUnsupportedOS is returned when the platform has no runtime
	// login-item mechanism, or the OS version is too old for one.
	ErrUnsupportedOS = errors.New("autostart: not supported on this platform")

	// ErrNotBundled is returned when the executable is not running from
	// inside a .app bundle. SMAppService registers a bundle, so a bare
	// binary (go run, ./bin/Spice-darwin-arm64) can never be registered.
	ErrNotBundled = errors.New("autostart: not running from an application bundle")
)

// Status describes the login-item state as reported by the operating system.
type Status int

// Login-item states. These mirror SMAppServiceStatus on macOS.
const (
	// StatusUnknown means the state could not be determined.
	StatusUnknown Status = iota
	// StatusNotRegistered means no login item exists for this bundle.
	StatusNotRegistered
	// StatusEnabled means the login item exists and will launch at login.
	StatusEnabled
	// StatusRequiresApproval means a login item exists but the user has
	// disabled it in System Settings. Treat this as a deliberate opt-out
	// and never silently re-register over it.
	StatusRequiresApproval
	// StatusNotFound means the OS has no record of the service.
	StatusNotFound
	// StatusUnsupported means this platform or OS version cannot report a status.
	StatusUnsupported
)

// String renders the status for logs and diagnostics.
func (s Status) String() string {
	switch s {
	case StatusNotRegistered:
		return "not registered"
	case StatusEnabled:
		return "enabled"
	case StatusRequiresApproval:
		return "requires approval"
	case StatusNotFound:
		return "not found"
	case StatusUnsupported:
		return "unsupported"
	default:
		return "unknown"
	}
}

// Diagnostic reports why registering a login item may not behave as expected.
// It is surfaced in the UI alongside the raw OS error so that an environment
// problem is not mistaken for a broken feature.
type Diagnostic struct {
	// Bundled is true when the executable lives inside a .app bundle.
	Bundled bool
	// BundlePath is the resolved .app path, empty when not bundled.
	BundlePath string
	// InApplications is true when the bundle sits in a standard Applications
	// directory. SMAppService records the bundle at its current path, so an
	// app registered from Downloads breaks the moment it is moved.
	InApplications bool
	// AdhocSigned is true when the bundle carries no Team Identifier.
	// SMAppService registration on such a build fails, or registers an item
	// that macOS then refuses to launch.
	AdhocSigned bool
	// OSVersionOK is true when the OS is new enough for the login-item API.
	OSVersionOK bool
}

// Hints returns human-readable explanations for every condition that would
// prevent a login item from working. An empty slice means nothing is wrong.
func (d Diagnostic) Hints() []string {
	var hints []string
	if !d.OSVersionOK {
		hints = append(hints, "This version of macOS is too old to register a login item automatically (macOS 13 or later is required).")
	}
	if !d.Bundled {
		hints = append(hints, "Spice is not running from an application bundle, so it cannot register itself to start at login.")
	} else if !d.InApplications {
		hints = append(hints, "Spice is not installed in the Applications folder. Move Spice.app to /Applications and try again, otherwise the login item breaks as soon as the app is moved.")
	}
	if d.AdhocSigned {
		hints = append(hints, "This build of Spice is not signed with a Developer ID. macOS will not reliably launch an unsigned login item.")
	}
	return hints
}

// bundlePathFromExecutable derives the enclosing .app bundle path from an
// executable path. It expects the macOS layout <Name>.app/Contents/MacOS/<bin>
// and returns ErrNotBundled for anything else.
//
// The caller is responsible for resolving symlinks first; this function is
// pure string handling so it stays unit-testable on every platform.
func bundlePathFromExecutable(exe string) (string, error) {
	if exe == "" {
		return "", ErrNotBundled
	}

	// Walk up from the executable looking for the Contents/MacOS pair sitting
	// directly under a .app directory. Walking (rather than slicing three
	// levels off) tolerates the nested-helper layout and any future change to
	// the binary's depth inside Contents/MacOS.
	dir := filepath.Dir(exe)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root without a match.
			return "", ErrNotBundled
		}
		if filepath.Base(dir) == "MacOS" && filepath.Base(parent) == "Contents" {
			bundle := filepath.Dir(parent)
			if strings.EqualFold(filepath.Ext(bundle), ".app") {
				return bundle, nil
			}
			return "", ErrNotBundled
		}
		dir = parent
	}
}

// isInApplications reports whether a bundle path lives in a standard
// Applications directory. home may be empty when the home directory is
// unknown, in which case only the system location is considered.
func isInApplications(bundle, home string) bool {
	if bundle == "" {
		return false
	}
	clean := filepath.Clean(bundle)
	if strings.HasPrefix(clean, "/Applications/") {
		return true
	}
	if home != "" {
		userApps := filepath.Join(filepath.Clean(home), "Applications") + string(filepath.Separator)
		if strings.HasPrefix(clean, userApps) {
			return true
		}
	}
	return false
}
