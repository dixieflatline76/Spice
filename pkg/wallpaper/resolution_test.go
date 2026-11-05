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

func TestParseXRandrMonitors(t *testing.T) {
	raw := `Monitors: 2
 0: +HDMI-1 1920/531x1080/299+0+0  HDMI-1
 1: +*DP-1 2560/597x1440/336+1920+0  DP-1`

	mons, err := ParseXRandrMonitors(raw)
	assert.NoError(t, err)
	if assert.Len(t, mons, 2) {
		assert.Equal(t, 0, mons[0].ID)
		assert.Equal(t, "HDMI-1", mons[0].Name)
		assert.False(t, mons[0].Primary)
		assert.Equal(t, image.Rect(0, 0, 1920, 1080), mons[0].Rect)
		assert.Equal(t, image.Rect(0, 0, 1920, 1080), mons[0].Bounds)

		assert.Equal(t, 1, mons[1].ID)
		assert.Equal(t, "DP-1", mons[1].Name)
		assert.True(t, mons[1].Primary)
		assert.Equal(t, image.Rect(0, 0, 2560, 1440), mons[1].Rect)
		assert.Equal(t, image.Rect(1920, 0, 4480, 1440), mons[1].Bounds)
	}
}

func TestParseXRandrMonitors_NoStarMarksFirstPrimary(t *testing.T) {
	raw := ` 0: +HDMI-1 1920/531x1080/299+0+0  HDMI-1`
	mons, err := ParseXRandrMonitors(raw)
	assert.NoError(t, err)
	if assert.Len(t, mons, 1) {
		assert.True(t, mons[0].Primary)
	}
}

func TestMonitorAtPoint(t *testing.T) {
	monitors := []Monitor{
		{ID: 0, Primary: true, Bounds: image.Rect(0, 0, 1728, 1117)},
		{ID: 1, Primary: false, Bounds: image.Rect(1728, 0, 4288, 1440)},
	}

	id, hit := MonitorAtPoint(image.Pt(500, 20), monitors)
	assert.Equal(t, 0, id)
	assert.True(t, hit)

	id, hit = MonitorAtPoint(image.Pt(2000, 10), monitors)
	assert.Equal(t, 1, id)
	assert.True(t, hit)

	// Outside all bounds: falls back to Primary and reports a miss.
	id, hit = MonitorAtPoint(image.Pt(-500, -500), monitors)
	assert.Equal(t, 0, id)
	assert.False(t, hit)

	// Empty Bounds falls back to Rect.
	id, hit = MonitorAtPoint(image.Pt(10, 10), []Monitor{{ID: 7, Rect: image.Rect(0, 0, 100, 100)}})
	assert.Equal(t, 7, id)
	assert.True(t, hit)

	id, hit = MonitorAtPoint(image.Pt(0, 0), nil)
	assert.Equal(t, 0, id)
	assert.False(t, hit)
}

func TestPlanTrayMonitors(t *testing.T) {
	monitors := []Monitor{
		{ID: 0, Name: "Secondary", Primary: false, Bounds: image.Rect(2560, 0, 4480, 1080)},
		{ID: 1, Name: "Primary", Primary: true, Bounds: image.Rect(0, 0, 2560, 1440)},
	}

	// Single-tray OS (Windows/Linux): Primary at root, others as submenus.
	rootID, subs := PlanTrayMonitors(monitors, false, 0)
	assert.Equal(t, 1, rootID)
	if assert.Len(t, subs, 1) {
		assert.Equal(t, 0, subs[0].ID)
	}

	// Per-screen tray OS (macOS): active monitor at root, zero submenus.
	rootID, subs = PlanTrayMonitors(monitors, true, 0)
	assert.Equal(t, 0, rootID)
	assert.Empty(t, subs)

	// Per-screen with unknown activeID falls back to Primary.
	rootID, subs = PlanTrayMonitors(monitors, true, 42)
	assert.Equal(t, 1, rootID)
	assert.Empty(t, subs)

	// No monitors.
	rootID, subs = PlanTrayMonitors(nil, false, 0)
	assert.Equal(t, 0, rootID)
	assert.Empty(t, subs)
}

func TestSortAndIndexMonitors(t *testing.T) {
	raw := []Monitor{
		{ID: 0, Name: "DELL P2419H", DevicePath: "path-right", Bounds: image.Rect(2560, 0, 4480, 1080)},
		{ID: 1, Name: "LG UltraGear", DevicePath: "path-primary", Bounds: image.Rect(0, 0, 2560, 1440), Primary: true},
		{ID: 2, Name: "BenQ", DevicePath: "path-left", Bounds: image.Rect(-1920, 0, 0, 1080)},
	}
	sorted := sortAndIndexMonitors(raw)
	if assert.Len(t, sorted, 3) {
		assert.Equal(t, "path-primary", sorted[0].DevicePath)
		assert.Equal(t, 0, sorted[0].ID)
		assert.Equal(t, "path-left", sorted[1].DevicePath)
		assert.Equal(t, 1, sorted[1].ID)
		assert.Equal(t, "path-right", sorted[2].DevicePath)
		assert.Equal(t, 2, sorted[2].ID)
	}
}
