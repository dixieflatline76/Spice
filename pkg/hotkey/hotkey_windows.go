//go:build windows

package hotkey

import (
	"syscall"

	"golang.design/x/hotkey"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

const (
	modCtrl = hotkey.ModCtrl
	modAlt  = hotkey.ModAlt

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
	// Virtual Key Codes for Windows: '1' through '9' are 0x31 through 0x39
	for i := 0; i < 9; i++ {
		vk := 0x31 + i
		ret, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
		// If the most significant bit is set, the key is down
		if ret&0x8000 != 0 {
			return i
		}
	}
	return -1
}

func HasAccessibility() bool {
	return true
}
