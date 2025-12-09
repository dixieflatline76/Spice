package wallpaper

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetConfig_Defaults(t *testing.T) {
	ResetConfig()
	p := NewMockPreferences()
	cfg := GetConfig(p)

	assert.NotNil(t, cfg)
	assert.NotNil(t, cfg.Queries)
	assert.NotNil(t, cfg.AvoidSet)

	// Check default values
	assert.True(t, cfg.GetSmartFit())
	assert.False(t, cfg.GetFaceBoostEnabled())
	assert.False(t, cfg.GetFaceCropEnabled())
	assert.Equal(t, FrequencyHourly, cfg.GetWallpaperChangeFrequency())
}

func TestQueryManagement(t *testing.T) {
	ResetConfig()
	p := NewMockPreferences()
	cfg := GetConfig(p)

	// Add Image Query
	cfg.Queries = []ImageQuery{} // Clear default queries
	cfg.ImageQueries = []ImageQuery{}
	cfg.UnsplashQueries = []ImageQuery{}
	cfg.PexelsQueries = []ImageQuery{}

	id1, err := cfg.AddImageQuery("Test Wallhaven", "http://example.com/1.jpg", true)
	assert.NoError(t, err)
	assert.NotEmpty(t, id1)

	// Add Unsplash Query
	id2, err := cfg.AddUnsplashQuery("Test Unsplash", fmt.Sprintf("http://example.com/u-%d.jpg", time.Now().UnixNano()), true)
	if err != nil {
		t.Fatalf("AddUnsplashQuery failed: %v", err)
	}
	if id2 == "" {
		t.Fatal("AddUnsplashQuery returned empty ID")
	}

	// Add Pexels Query
	id3, err := cfg.AddPexelsQuery("Test Pexels", fmt.Sprintf("http://example.com/p-%d.jpg", time.Now().UnixNano()), true)
	if err != nil {
		t.Fatalf("AddPexelsQuery failed: %v", err)
	}
	if id3 == "" {
		t.Fatal("AddPexelsQuery returned empty ID")
	}

	// Verify counts
	assert.Equal(t, 3, len(cfg.GetQueries()))
	assert.Equal(t, 1, len(cfg.GetImageQueries()))
	assert.Equal(t, 1, len(cfg.GetUnsplashQueries()))
	assert.Equal(t, 1, len(cfg.GetPexelsQueries()))

	// Test Duplication
	_, err = cfg.AddImageQuery("Duplicate", "http://example.com/1.jpg", true)
	assert.Error(t, err)
	assert.True(t, cfg.IsDuplicateID(id1))

	// Test GetQuery
	q, found := cfg.GetQuery(id1)
	assert.True(t, found)
	assert.Equal(t, "Test Wallhaven", q.Description)

	// Test Disable/Enable
	err = cfg.DisableImageQuery(id1)
	assert.NoError(t, err)
	q, _ = cfg.GetQuery(id1)
	assert.False(t, q.Active)

	err = cfg.EnableImageQuery(id1)
	assert.NoError(t, err)
	q, _ = cfg.GetQuery(id1)
	assert.True(t, q.Active)

	// Test Remove
	err = cfg.RemoveImageQuery(id1)
	assert.NoError(t, err)
	_, found = cfg.GetQuery(id1)
	assert.False(t, found)
	assert.Equal(t, 2, len(cfg.GetQueries()))
}

func TestConfigPreferences(t *testing.T) {
	ResetConfig()
	p := NewMockPreferences()
	cfg := GetConfig(p)

	// Smart Fit
	cfg.SetSmartFit(false)
	assert.False(t, cfg.GetSmartFit())

	// Cache Size
	cfg.SetCacheSize(Cache500Images)
	assert.Equal(t, Cache500Images, cfg.GetCacheSize())

	// Wallpaper Change Frequency
	cfg.SetWallpaperChangeFrequency(FrequencyDaily)
	assert.Equal(t, FrequencyDaily, cfg.GetWallpaperChangeFrequency())

	// Image Shuffle
	cfg.SetImgShuffle(true)
	assert.True(t, cfg.GetImgShuffle())

	// Face Boost/Crop
	cfg.SetFaceBoostEnabled(true)
	assert.True(t, cfg.GetFaceBoostEnabled())

	cfg.SetFaceCropEnabled(true)
	assert.True(t, cfg.GetFaceCropEnabled())
}

func TestAvoidSet(t *testing.T) {
	ResetConfig()
	p := NewMockPreferences()
	cfg := GetConfig(p)

	id := "some-image-url"
	assert.False(t, cfg.InAvoidSet(id))

	cfg.AddToAvoidSet(id)
	assert.True(t, cfg.InAvoidSet(id))

	cfg.ResetAvoidSet()
	assert.False(t, cfg.InAvoidSet(id))
}
