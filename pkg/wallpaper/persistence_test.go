//go:build !linux

package wallpaper

import (
	"strings"
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

	// Wait for async save (Polling is more robust than fixed sleep)
	var prefJSON string
	success := false
	for i := 0; i < 20; i++ {
		prefJSON = prefs.String(wallhavenConfigPrefKey)
		// Specifically look for the monitor ID followed by true
		if strings.Contains(prefJSON, "\""+devicePath+"\": true") || strings.Contains(prefJSON, "\""+devicePath+"\":true") {
			success = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.True(t, success, "Monitor pause state should be written to preferences. JSON: %s", prefJSON)
	assert.True(t, cfg.IsMonitorPaused(devicePath))

	// Verify Persistence
	ResetConfig()
	newCfg := GetConfig(prefs)

	// Debug: Check if map is loaded
	assert.NotNil(t, newCfg.MonitorPauseStates, "MonitorPauseStates map should be initialized after reload")
	assert.True(t, newCfg.IsMonitorPaused(devicePath), "Monitor pause state should persist")

	// Unpause
	newCfg.SetMonitorPaused(devicePath, false)

	// Wait for async save
	success = false
	for i := 0; i < 20; i++ {
		prefJSON = prefs.String(wallhavenConfigPrefKey)
		// Specifically look for the monitor ID followed by false
		if strings.Contains(prefJSON, "\""+devicePath+"\": false") || strings.Contains(prefJSON, "\""+devicePath+"\":false") {
			success = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.True(t, success, "Monitor unpause state should be written to preferences. JSON: %s", prefJSON)

	assert.False(t, newCfg.IsMonitorPaused(devicePath))

	// Verify Persistence Again
	ResetConfig()
	finalCfg := GetConfig(prefs)
	assert.False(t, finalCfg.IsMonitorPaused(devicePath), "Monitor unpause state should persist")
}
