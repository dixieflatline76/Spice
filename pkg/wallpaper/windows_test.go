//go:build windows

package wallpaper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWindowsGetMonitors_Integration runs a real call to the OS to get monitors.
// This is an integration test that requires a Windows environment.
func TestWindowsGetMonitors_Integration(t *testing.T) {
	osImpl := getOS()
	monitors, err := osImpl.GetMonitors()

	// On any valid Windows system, we should get at least one monitor (or fallback to primary)
	// and no error.
	assert.NoError(t, err)
	assert.NotEmpty(t, monitors)

	for _, m := range monitors {
		t.Logf("Detected Monitor: ID=%d, Name=%s, Rect=%v", m.ID, m.Name, m.Rect)
		assert.GreaterOrEqual(t, m.ID, 0)
		assert.True(t, m.Rect.Dx() > 0)
		assert.True(t, m.Rect.Dy() > 0)
	}
}

// TestWindowsSetWallpaper_InvalidID checks the safety guards in SetWallpaper.
func TestWindowsSetWallpaper_InvalidID(t *testing.T) {
	osImpl := getOS()

	// Usage of a ridiculously high ID should fail quickly
	// We use the math package constant we restored to verify that check
	err := osImpl.SetWallpaper("C:\\fake\\path.jpg", -1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid monitor ID")

	// Check boundary condition (MaxUint32 + 1 if we could pass it, but int is signed)
	// We can't easy pass overflow via int, but we can pass a valid int that is likely not a real monitor
	// This exercises the GetMonitorDevicePathAt failure path
	err = osImpl.SetWallpaper("C:\\fake\\path.jpg", 999999)
	assert.Error(t, err)
	// The error might be "invalid monitor ID" (from our check) or "GetMonitorDevicePathAt failed" depending on logic
	// Our code: checks if monitorID < 0 || uint64(monitorID) > math.MaxUint32
	// 999999 is valid int but invalid monitor index.
	// So it should pass the first check and fail at GetMonitorDevicePathAt
	assert.Contains(t, err.Error(), "GetMonitorDevicePathAt failed")
}
