//go:build !linux

package wallpaper

import (
	"testing"

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
