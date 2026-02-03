package wallpaper

import (
	"fmt"
	"image"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockMonitor is a simple struct for testing resolution logic
// We can use the real Monitor struct now

// Ensure GetUniqueResolutions returns unique Rectangles
func TestGetUniqueResolutions(t *testing.T) {
	// Setup: 2 monitors with same 1080p res, 1 monitor with 4K
	mon1 := Monitor{ID: 0, Rect: image.Rect(0, 0, 1920, 1080)}
	mon2 := Monitor{ID: 1, Rect: image.Rect(0, 0, 1920, 1080)}
	mon3 := Monitor{ID: 2, Rect: image.Rect(0, 0, 3840, 2160)}

	monitors := []Monitor{mon1, mon2, mon3}

	// This function doesn't exist yet, we are TDD-ing it.
	// We will implement it in resolution.go
	resolutions := GetUniqueResolutions(monitors)

	// Check results
	if len(resolutions) != 2 {
		t.Errorf("expected 2 unique resolutions, got %d", len(resolutions))
	}

	found1080p := false
	found4K := false

	for _, res := range resolutions {
		key := fmt.Sprintf("%dx%d", res.Width, res.Height)
		switch key {
		case "1920x1080":
			found1080p = true
			if len(res.Monitors) != 2 { // ID 0 and 1
				t.Errorf("expected 2 monitors for 1920x1080, got %d", len(res.Monitors))
			}
		case "3840x2160":
			found4K = true
			if len(res.Monitors) != 1 { // ID 2
				t.Errorf("expected 1 monitor for 3840x2160, got %d", len(res.Monitors))
			}
		default:
			t.Errorf("unexpected resolution: %s", key)
		}
	}

	assert.True(t, found1080p, "Should contain 1920x1080")
	assert.True(t, found4K, "Should contain 3840x2160")
}

// Ensure GetDerivativePath generates correct resolution-based paths
func TestGetDerivativePath(t *testing.T) {
	// Setup
	// Setup
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)

	wp := &Plugin{
		fm: fm,
	}
	// Note: We might need to abstract the base path or mock the FileManager
	// For now let's assume GetDerivativePath logic is pure or we create a helper.

	id := "image_123"
	rect := image.Rect(0, 0, 2560, 1440)

	// Expected: .../fitted/2560x1440/image_123.jpg
	// We need to implement GetDerivativePath to accept a Rect or Width/Height

	// Let's assume the signature: GetResolutionPath(id string, width, height int) string
	path := wp.GetDerivativePath(id, rect.Dx(), rect.Dy())

	assert.Contains(t, path, "fitted", "Path should contain fitted dir")
	assert.Contains(t, path, "2560x1440", "Path should contain resolution folder")
	assert.Contains(t, path, "image_123.jpg", "Path should contain filename")
	assert.Equal(t, ".jpg", filepath.Ext(path), "Should default to .jpg")
}
