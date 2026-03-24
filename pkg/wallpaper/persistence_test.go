//go:build !linux

package wallpaper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFrequencyNever_Persists(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	// Initial State: Hourly (Default)
	assert.Equal(t, FrequencyHourly, cfg.GetWallpaperChangeFrequency())

	// Set to Never
	cfg.SetWallpaperChangeFrequency(FrequencyNever)

	// Verify Persistence
	ResetConfig()
	newCfg := GetConfig(prefs)
	assert.Equal(t, FrequencyNever, newCfg.GetWallpaperChangeFrequency(), "Never frequency should persist")

	// Set back to Hourly
	newCfg.SetWallpaperChangeFrequency(FrequencyHourly)

	// Verify Persistence Again
	ResetConfig()
	finalCfg := GetConfig(prefs)
	assert.Equal(t, FrequencyHourly, finalCfg.GetWallpaperChangeFrequency(), "Hourly frequency should persist")
}

func TestMonitorPausePersistence(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	devicePath := "MONITOR_1"

	// Initial State: Unpaused
	assert.False(t, cfg.IsMonitorPaused(devicePath))

	// Set to Paused
	cfg.SetMonitorPaused(devicePath, true)
	time.Sleep(1 * time.Second) // Force a long wait for background save
	assert.True(t, cfg.IsMonitorPaused(devicePath))

	// Verify Persistence
	ResetConfig()
	newCfg := GetConfig(prefs)

	// Debug: Check if map is loaded
	assert.NotNil(t, newCfg.MonitorPauseStates, "MonitorPauseStates map should be initialized after reload")
	assert.True(t, newCfg.IsMonitorPaused(devicePath), "Monitor pause state should persist")

	// Unpause
	newCfg.SetMonitorPaused(devicePath, false)
	time.Sleep(1 * time.Second) // Force a long wait for background save
	assert.False(t, newCfg.IsMonitorPaused(devicePath))

	// Verify Persistence Again
	ResetConfig()
	finalCfg := GetConfig(prefs)
	assert.False(t, finalCfg.IsMonitorPaused(devicePath), "Monitor unpause state should persist")
}
