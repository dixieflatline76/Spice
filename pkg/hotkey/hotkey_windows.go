//go:build windows

package hotkey

import (
	"syscall"
	"time"

	"github.com/dixieflatline76/Spice/util/log"
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
	// Poll for up to 100ms to catch "chorded" key presses
	for retry := 0; retry < 5; retry++ {
		for i := 0; i < 9; i++ {
			vk := 0x31 + i
			ret, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
			// If the most significant bit is set, the key is down
			if ret&0x8000 != 0 {
				log.Debugf("[Hotkey] Windows detected monitor key %d on retry %d", i+1, retry)
				return i
			}
		}
		if retry < 4 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	return -1
}

func HasAccessibility() bool {
	return true
}
