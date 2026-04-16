//go:build !windows

package ui

import (
	utilLog "github.com/dixieflatline76/Spice/v2/util/log"
)

// ShowNativeFallbackAlert is a stub for non-Windows platforms.
// On macOS/Linux, we currently just log the error since native message boxes
// are harder to trigger without a windowing context or extra dependencies.
func ShowNativeFallbackAlert(title, message string) {
	utilLog.Printf("NATIVE ALERT [%s]: %s", title, message)
}
