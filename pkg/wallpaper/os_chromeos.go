package wallpaper

import (
	"errors"
	"sync"
)

// ChromeOS implements the OS interface for Chrome OS via Bridge.
type ChromeOS struct {
	mu             sync.Mutex
	bridgeCallback func(string) error
}

// SetWallpaper delegates to the bridge callback.
func (c *ChromeOS) SetWallpaper(path string) error {
	c.mu.Lock()
	cb := c.bridgeCallback
	c.mu.Unlock()

	if cb == nil {
		return errors.New("chrome extension bridge not connected")
	}
	return cb(path)
}

// GetDesktopDimension returns the desktop dimensions.
func (c *ChromeOS) GetDesktopDimension() (int, int, error) {
	// Standard full HD default, or could query chrome.system.display via bridge too!
	// For now, static default is safe as we rely on 'smart fit' mostly.
	return 1920, 1080, nil
}

// RegisterBridge registers the callback function.
func (c *ChromeOS) RegisterBridge(cb func(string) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bridgeCallback = cb
}
