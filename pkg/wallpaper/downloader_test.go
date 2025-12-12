package wallpaper

import (
	"context"
	"testing"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/stretchr/testify/assert"
)

func TestProcessImageJob_BlockedImage(t *testing.T) {
	// Setup Config
	ResetConfig()
	p := NewMockPreferences()
	cfg := GetConfig(p)

	// Add an image to the AvoidSet
	blockedID := "blocked_image_123"
	cfg.AddToAvoidSet(blockedID)

	// Create a dummy plugin with this config
	wp := &Plugin{
		cfg: cfg,
	}

	// Create a job for the blocked image
	job := DownloadJob{
		Image: provider.Image{
			ID:          blockedID,
			Path:        "http://example.com/blocked.jpg",
			Attribution: "Blocked Artist",
		},
	}

	// Execute ProcessImageJob
	// Note: ProcessImageJob is a method on *Plugin: wp.ProcessImageJob(...)
	_, err := wp.ProcessImageJob(context.Background(), job)

	// Expect error indicating it was skipped/blocked
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "avoid set")
}
