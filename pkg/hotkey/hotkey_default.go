//go:build !darwin && !windows

package hotkey

import "golang.design/x/hotkey"

const (
	modCtrl = hotkey.Modifier(0) // Dummy for default
	modAlt  = hotkey.Modifier(0)

	keyRight = hotkey.Key(0)
	keyLeft  = hotkey.Key(0)
	keyUp    = hotkey.Key(0)
	keyDown  = hotkey.Key(0)
	keyP     = hotkey.Key(0)
	keyO     = hotkey.Key(0)
)

func GetMonitorIDFromKey() int {
	return -1
}

func HasAccessibility() bool {
	return true
}
