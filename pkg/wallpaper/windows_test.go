//go:build windows

package wallpaper

import (
	"testing"
	"unsafe"

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

	// A positive index beyond the enumerated monitor list should fail the range check.
	err = osImpl.SetWallpaper("C:\\fake\\path.jpg", 999999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestWindowsDisplayConfigStructSizes(t *testing.T) {
	assert.Equal(t, uintptr(72), unsafe.Sizeof(displayConfigPathInfo{}))
	assert.Equal(t, uintptr(64), unsafe.Sizeof(displayConfigModeInfo{}))
	assert.Equal(t, uintptr(420), unsafe.Sizeof(displayConfigTargetDeviceName{}))
}

func TestWindowsGetMonitors_PrimaryIsIDZero(t *testing.T) {
	monitors, err := getOS().GetMonitors()
	assert.NoError(t, err)
	if assert.NotEmpty(t, monitors) {
		assert.Equal(t, 0, monitors[0].ID)
		assert.True(t, monitors[0].Primary)
		assert.False(t, monitors[0].Bounds.Empty())
	}
}
