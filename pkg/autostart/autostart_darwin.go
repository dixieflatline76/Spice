//go:build darwin
// +build darwin

package autostart

/*
#cgo CFLAGS: -x objective-c
// ServiceManagement.framework itself exists back to macOS 10.6, so it links
// normally at our 12.0 deployment target. Only the SMAppService class is 13+,
// and every use of it is guarded by @available in the Objective-C layer.
#cgo LDFLAGS: -framework AppKit -framework Foundation -framework ServiceManagement
#include "autostart_native.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"
)

// errBufSize bounds the NSError description copied back from Objective-C.
const errBufSize = 512

// osAvailable reports whether SMAppService exists on the running OS.
// The deployment target is macOS 12 while SMAppService is 13+, so every call
// site must gate on this.
func osAvailable() bool {
	return C.spiceLoginItemAvailable() == 1
}

// bundlePath resolves the .app bundle enclosing the running executable.
func bundlePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolving executable: %w", err)
	}
	// Resolve symlinks so a linked binary still maps to its real bundle.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return bundlePathFromExecutable(exe)
}

// Supported reports whether this build can register a login item: macOS 13+
// and running from inside a .app bundle.
func Supported() bool {
	if !osAvailable() {
		return false
	}
	_, err := bundlePath()
	return err == nil
}

// Get returns the login-item status reported by the operating system.
func Get() (Status, error) {
	if !osAvailable() {
		return StatusUnsupported, ErrUnsupportedOS
	}
	if _, err := bundlePath(); err != nil {
		return StatusUnsupported, err
	}

	switch C.spiceLoginItemStatus() {
	case C.SPICE_LOGIN_NOTREGISTERED:
		return StatusNotRegistered, nil
	case C.SPICE_LOGIN_ENABLED:
		return StatusEnabled, nil
	case C.SPICE_LOGIN_REQUIRESAPPROVAL:
		return StatusRequiresApproval, nil
	case C.SPICE_LOGIN_NOTFOUND:
		return StatusNotFound, nil
	case C.SPICE_LOGIN_UNSUPPORTED:
		return StatusUnsupported, ErrUnsupportedOS
	default:
		return StatusUnknown, nil
	}
}

// call invokes one of the register/unregister entry points and turns a
// non-zero result into an error carrying the verbatim NSError text.
func call(fn func(*C.char, C.int) C.int, action string) error {
	if !osAvailable() {
		return ErrUnsupportedOS
	}
	if _, err := bundlePath(); err != nil {
		return err
	}

	buf := (*C.char)(C.calloc(errBufSize, 1))
	defer C.free(unsafe.Pointer(buf))

	code := fn(buf, C.int(errBufSize))
	if code == 0 {
		return nil
	}

	msg := C.GoString(buf)
	if msg == "" {
		msg = "unknown error"
	}
	return fmt.Errorf("autostart: %s login item failed (code %d): %s", action, int(code), msg)
}

// Enable registers the running app bundle as a login item.
func Enable() error {
	return call(func(b *C.char, n C.int) C.int { return C.spiceLoginItemRegister(b, n) }, "registering")
}

// Disable removes the login item.
func Disable() error {
	return call(func(b *C.char, n C.int) C.int { return C.spiceLoginItemUnregister(b, n) }, "removing")
}

// OpenLoginItemsSettings opens System Settings at the Login Items pane so the
// user can add Spice by hand when automatic registration is unavailable.
func OpenLoginItemsSettings() error {
	if C.spiceOpenLoginItemsSettings() != 0 {
		return errors.New("autostart: could not open Login Items settings")
	}
	return nil
}

// Diagnose reports the environmental conditions that determine whether a
// login item will actually work. It never fails; unknown conditions are
// reported as their pessimistic value.
func Diagnose() Diagnostic {
	d := Diagnostic{OSVersionOK: osAvailable()}

	bundle, err := bundlePath()
	if err != nil {
		return d
	}
	d.Bundled = true
	d.BundlePath = bundle

	home, _ := os.UserHomeDir()
	d.InApplications = isInApplications(bundle, home)
	d.AdhocSigned = isAdhocSigned(bundle)

	return d
}

// isAdhocSigned reports whether a bundle carries no Team Identifier, which is
// the case for local ad-hoc signed builds. macOS will not reliably launch a
// login item for such a bundle.
//
// Failure to run codesign is reported as "not ad-hoc" so a diagnostic problem
// never manifests as a spurious warning.
func isAdhocSigned(bundle string) bool {
	cmd := exec.Command("codesign", "-dv", "--verbose=4", bundle)
	// codesign writes its report to stderr.
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "TeamIdentifier=") {
			value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "TeamIdentifier="))
			return value == "" || value == "not set"
		}
	}
	// No TeamIdentifier line at all means the bundle is ad-hoc signed.
	return true
}
