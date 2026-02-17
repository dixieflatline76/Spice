//go:build !linux

package wallpaper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigMigration_LegacyToUnified(t *testing.T) {
	// 1. Setup Legacy JSON
	legacyJSON := `{
		"query_urls": [
			{"url": "http://wallhaven.cc/legacy1", "active": true}
		],
		"unsplash_queries": [
			{"url": "http://unsplash.com/legacy2", "active": true}
		]
	}`

	// 2. Setup Mock Preferences
	p := NewMockPreferences()
	p.SetString("wallhaven_image_queries", legacyJSON)

	// 3. Load Config
	// GetConfig calls loadFromPrefs internally
	ResetConfig() // Ensure fresh instance
	cfg := GetConfig(p)

	// 4. Verify Migration
	assert.NotEmpty(t, cfg.Queries)

	// Check content
	foundWallhaven := false
	foundUnsplash := false

	for _, q := range cfg.Queries {
		if q.URL == "http://wallhaven.cc/legacy1" {
			foundWallhaven = true
			assert.Equal(t, "Wallhaven", q.Provider)
			assert.NotEmpty(t, q.ID)
		}
		if q.URL == "http://unsplash.com/legacy2" {
			foundUnsplash = true
			assert.Equal(t, "Unsplash", q.Provider)
			assert.NotEmpty(t, q.ID)
		}
	}

	assert.True(t, foundWallhaven, "Legacy Wallhaven query not migrated")
	assert.True(t, foundUnsplash, "Legacy Unsplash query not migrated")

	// Verify Favorites was added
	foundFav := false
	for _, q := range cfg.Queries {
		if q.ID == FavoritesQueryID {
			foundFav = true
			break
		}
	}
	assert.True(t, foundFav, "Favorites query should be auto-added")
}
