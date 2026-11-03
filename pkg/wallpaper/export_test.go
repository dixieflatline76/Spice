package wallpaper

import "sync"

// ResetConfig resets the config singleton for testing purposes.
func ResetConfig() {
	cfgInstance = nil
	cfgOnce = sync.Once{}
}
