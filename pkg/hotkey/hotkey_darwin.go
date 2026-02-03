//go:build darwin

package hotkey

import (
	"time"

	"golang.design/x/hotkey"
)

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices
#include <CoreGraphics/CoreGraphics.h>
#include <ApplicationServices/ApplicationServices.h>

int isKeyPressedNative(int state, int keyCode) {
    return CGEventSourceKeyState((CGEventSourceStateID)state, (CGKeyCode)keyCode) ? 1 : 0;
}

int checkAccessibilityNative() {
    return AXIsProcessTrusted() ? 1 : 0;
}
*/
import "C"

func HasAccessibility() bool {
	return C.checkAccessibilityNative() != 0
}

const (
	modCtrl = hotkey.ModCmd
	modAlt  = hotkey.ModOption

	// macOS Virtual Key Codes
	kVK_Option      = 58
	kVK_RightOption = 61
	kVK_Cmd         = 55

	keyRight = hotkey.KeyRight
	keyLeft  = hotkey.KeyLeft
	keyUp    = hotkey.KeyUp
	keyDown  = hotkey.KeyDown
	keyP     = hotkey.KeyP
	keyO     = hotkey.KeyO
)

// GetMonitorIDFromKey checks if any number key 1-9 is currently pressed.
// Returns monitor ID (0-based) or -1 if none.
func GetMonitorIDFromKey() int {
	// Virtual Key Codes for macOS (1-9 Top Row):
	codes := []int{18, 19, 20, 21, 23, 22, 26, 28, 25}
	// Numpad codes (1-9):
	numpadCodes := []int{83, 84, 85, 86, 87, 88, 89, 91, 92}

	// Main detection with retry loop
	for retry := 0; retry < 3; retry++ {
		// Check top row
		for i, code := range codes {
			if C.isKeyPressedNative(C.kCGEventSourceStateCombinedSessionState, C.int(code)) != 0 ||
				C.isKeyPressedNative(C.kCGEventSourceStateHIDSystemState, C.int(code)) != 0 {
				return i
			}
		}
		// Check numpad
		for i, code := range numpadCodes {
			if C.isKeyPressedNative(C.kCGEventSourceStateCombinedSessionState, C.int(code)) != 0 ||
				C.isKeyPressedNative(C.kCGEventSourceStateHIDSystemState, C.int(code)) != 0 {
				return i
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	return -1
}
