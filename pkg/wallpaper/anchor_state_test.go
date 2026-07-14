//go:build !linux

package wallpaper

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestReprocessWithAnchor_StateSync verifies that after reprocessing with
// a new crop anchor, mc.State.CurrentImage reflects the updated anchor.
// This prevents regressions where the anchor popup shows a stale selection.
func TestReprocessWithAnchor_StateSync(t *testing.T) {
	// --- Setup temp dirs for FileManager ---
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "wallpaper_downloads")
	resDir := filepath.Join(rootDir, "fitted", "flexibility", "facecrop", "1920x1080")
	require.NoError(t, os.MkdirAll(rootDir, 0755))
	require.NoError(t, os.MkdirAll(resDir, 0755))

	// Write a minimal JPEG as the master (stored in rootDir)
	masterPath := filepath.Join(rootDir, "test_img.jpg")
	writeTestJPEG(t, masterPath, 200, 200)

	// Write a derivative so reprocessWithAnchor has something to overwrite
	derivPath := filepath.Join(resDir, "test_img.jpg")
	writeTestJPEG(t, derivPath, 1920, 1080)

	// --- Create FileManager ---
	fm := NewFileManager(rootDir)

	// --- Mock dependencies ---
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	mockIP := new(MockImageProcessor)
	cfg := GetConfig(NewMockPreferences())

	monitor := Monitor{
		ID:   0,
		Rect: image.Rect(0, 0, 1920, 1080),
	}

	mc := NewMonitorController(0, monitor, mockStore, fm, mockOS, cfg, mockIP)

	// Set the current image — initially no crop anchors
	imgID := "test_img"
	currentImg := provider.Image{
		ID:       imgID,
		FilePath: masterPath,
		DerivativePaths: map[string]string{
			"1920x1080": derivPath,
		},
	}
	mc.State.CurrentImage = currentImg
	mc.State.CurrentID = imgID

	// --- Mock expectations ---
	mockStore.On("SetTuningOptions", imgID, "1920x1080", provider.TuningOptions{Anchor: provider.AnchorTopCenter}).Return(true)

	// FitImage should return a valid image (1x1 is fine for test)
	dummyResult := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	mockIP.On("FitImage", mock.Anything, mock.Anything, 1920, 1080, provider.TuningOptions{Anchor: provider.AnchorTopCenter}).Return(dummyResult, nil)

	// SetWallpaper should succeed
	mockOS.On("SetWallpaper", mock.AnythingOfType("string"), 0).Return(nil)
	mockStore.On("GetUpdateChannel").Return((<-chan struct{})(nil))

	// --- Execute: send anchor command through the Run loop ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mc.Run(ctx)

	mc.Commands <- CmdAnchorTC // TopCenter = 202
	time.Sleep(300 * time.Millisecond)

	// --- Assert: CurrentImage must reflect the new anchor ---
	mc.mu.RLock()
	resultAnchor := mc.State.CurrentImage.GetAnchor("1920x1080")
	mc.mu.RUnlock()

	assert.Equal(t, provider.AnchorTopCenter, resultAnchor,
		"CurrentImage.Tuning should reflect the new anchor after reprocessing")

	mockStore.AssertExpectations(t)
	mockIP.AssertExpectations(t)
}

// TestReprocessWithAnchor_AnchorAuto_ClearsMap verifies that setting
// AnchorAuto removes the resolution entry from the Tuning map.
func TestReprocessWithAnchor_AnchorAuto_ClearsMap(t *testing.T) {
	tmpDir := t.TempDir()
	rootDir := filepath.Join(tmpDir, "wallpaper_downloads")
	resDir := filepath.Join(rootDir, "fitted", "flexibility", "facecrop", "1920x1080")
	require.NoError(t, os.MkdirAll(rootDir, 0755))
	require.NoError(t, os.MkdirAll(resDir, 0755))

	masterPath := filepath.Join(rootDir, "test_img.jpg")
	writeTestJPEG(t, masterPath, 200, 200)
	derivPath := filepath.Join(resDir, "test_img.jpg")
	writeTestJPEG(t, derivPath, 1920, 1080)

	fm := NewFileManager(rootDir)
	mockStore := new(MockImageStore)
	mockOS := new(MockOS)
	mockIP := new(MockImageProcessor)
	cfg := GetConfig(NewMockPreferences())

	monitor := Monitor{ID: 0, Rect: image.Rect(0, 0, 1920, 1080)}
	mc := NewMonitorController(0, monitor, mockStore, fm, mockOS, cfg, mockIP)

	// Start with an existing anchor
	mc.State.CurrentImage = provider.Image{
		ID:       "test_img",
		FilePath: masterPath,
		Tuning: map[string]provider.TuningOptions{
			"1920x1080": {Anchor: provider.AnchorTopCenter},
		},
		DerivativePaths: map[string]string{
			"1920x1080": derivPath,
		},
	}
	mc.State.CurrentID = "test_img"

	mockStore.On("SetTuningOptions", "test_img", "1920x1080", provider.TuningOptions{Anchor: provider.AnchorAuto}).Return(true)
	dummyResult := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	mockIP.On("FitImage", mock.Anything, mock.Anything, 1920, 1080, provider.TuningOptions{Anchor: provider.AnchorAuto}).Return(dummyResult, nil)
	mockOS.On("SetWallpaper", mock.AnythingOfType("string"), 0).Return(nil)
	mockStore.On("GetUpdateChannel").Return((<-chan struct{})(nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mc.Run(ctx)

	mc.Commands <- CmdAnchorAuto // Auto = 200
	time.Sleep(300 * time.Millisecond)

	mc.mu.RLock()
	_, exists := mc.State.CurrentImage.Tuning["1920x1080"]
	mc.mu.RUnlock()

	assert.False(t, exists, "AnchorAuto should clear the resolution entry from Tuning")
	mockStore.AssertExpectations(t)
}

// writeTestJPEG creates a minimal JPEG file for testing.
func writeTestJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Fill with a non-transparent color so imaging.Open doesn't fail
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, jpeg.Encode(f, img, &jpeg.Options{Quality: 50}))
}
