package wallpaper

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChromeOS_SetWallpaper_NoBridge(t *testing.T) {
	os := &ChromeOS{}
	err := os.SetWallpaper("test.jpg")
	// If no bridge, it should probably error or handle gracefully.
	// Current stub returns nil. TDD: We should define behavior.
	// Let's expect an error "bridge not connected" in implementation.
	// For now, let's just assert it behaves as currently implemented (nil)
	// until we define the logic.
	// Actually, TDD says define Desired Behavior.
	// Desired: Error.
	// So this test will FAIL until we implement the error.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bridge")
}

func TestChromeOS_SetWallpaper_WithBridge(t *testing.T) {
	os := &ChromeOS{}

	// Mock Bridge
	called := false
	var capturedPath string
	os.RegisterBridge(func(path string) error {
		called = true
		capturedPath = path
		return nil
	})

	err := os.SetWallpaper("/tmp/image.jpg")
	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "/tmp/image.jpg", capturedPath)
}

func TestChromeOS_SetWallpaper_BridgeError(t *testing.T) {
	os := &ChromeOS{}

	os.RegisterBridge(func(path string) error {
		return errors.New("connection failed")
	})

	err := os.SetWallpaper("test.jpg")
	assert.Error(t, err)
	assert.Equal(t, "connection failed", err.Error())
}

func TestChromeOS_GetDesktopDimension(t *testing.T) {
	os := &ChromeOS{}
	w, h, err := os.GetDesktopDimension()
	assert.NoError(t, err)
	assert.Greater(t, w, 0)
	assert.Greater(t, h, 0)
}
