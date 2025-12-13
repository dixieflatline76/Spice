package wallpaper

import (
	"testing"

	"github.com/dixieflatline76/Spice/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSetShuffleImage_Persists(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	wp := &Plugin{
		cfg:              cfg,
		store:            NewImageStore(),
		shuffleImageFlag: util.NewSafeBool(),
	}

	// Initial State: False (Default)
	assert.False(t, cfg.GetImgShuffle())

	// Enable
	wp.SetShuffleImage(true)

	// Verify Runtime State
	assert.True(t, cfg.GetImgShuffle(), "Runtime config should be true")

	// Verify Persistence (Simulate App Restart)
	ResetConfig()
	newCfg := GetConfig(prefs)
	assert.True(t, newCfg.GetImgShuffle(), "Persisted value should be loaded on restart")

	// Disable
	// Re-inject new config into a pseudo-plugin state or just use the config directly if we trusted the setter.
	// But we want to test the Plugin method.
	wp.cfg = newCfg
	wp.SetShuffleImage(false)

	assert.False(t, newCfg.GetImgShuffle())

	// Check persistence again
	ResetConfig()
	finalCfg := GetConfig(prefs)
	assert.False(t, finalCfg.GetImgShuffle(), "Persisted value should be false after disable")
}

func TestTogglePause_Persists(t *testing.T) {
	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	mockPM := new(MockPluginManager)
	mockPM.On("NotifyUser", mock.Anything, mock.Anything).Return()

	wp := &Plugin{
		cfg:     cfg,
		manager: mockPM,
	}

	// Initial State: Hourly (Default)
	assert.Equal(t, FrequencyHourly, cfg.GetWallpaperChangeFrequency())
	assert.False(t, wp.IsPaused())

	// Pause
	wp.TogglePause()

	// Verify Runtime State
	assert.Equal(t, FrequencyNever, cfg.GetWallpaperChangeFrequency())
	assert.True(t, wp.IsPaused())

	// Verify Persistence
	ResetConfig()
	newCfg := GetConfig(prefs)
	assert.Equal(t, FrequencyNever, newCfg.GetWallpaperChangeFrequency(), "Paused state (Never) should persist")

	// Resume
	// We need to setup a Plugin with the new config to resume correctly (it needs to read the persisted state)
	wp2 := &Plugin{
		cfg:               newCfg,
		manager:           mockPM,
		prePauseFrequency: FrequencyHourly, // Simulate restored state or default
	}
	// Note: In real app, prePauseFrequency is in-memory only.
	// If we restart the app while paused, prePauseFrequency is lost (reset to 0/default).
	// The TogglePause logic handles this: if prePause is 0 (Never?), it defaults to Hourly.
	// Let's verify that flow.

	wp2.TogglePause()

	// Should be Hourly (default/fallback)
	assert.Equal(t, FrequencyHourly, newCfg.GetWallpaperChangeFrequency())
	assert.False(t, wp2.IsPaused())

	// Verify Persistence Again
	ResetConfig()
	finalCfg := GetConfig(prefs)
	assert.Equal(t, FrequencyHourly, finalCfg.GetWallpaperChangeFrequency(), "Resumed state should persist")
}
