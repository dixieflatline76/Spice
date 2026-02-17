package wallpaper

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Protection Tests for FitImage Refactoring
// These tests target specific logic branches that will be moved into Strategies.

func createSolidImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	return img
}

func TestFitImage_Protection_Strategies(t *testing.T) {
	// Setup generic dependencies
	mockOS := new(MockOS)
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	// Create a minimal processor (no pigo initially)
	processor := &SmartImageProcessor{
		os:        mockOS,
		config:    cfg,
		resampler: imaging.NearestNeighbor, // Fast
	}

	t.Run("Strategy_Off_ReturnsOriginal", func(t *testing.T) {
		cfg.SetSmartFitMode(SmartFitOff)
		img := createSolidImage(100, 100, color.White)

		out, err := processor.FitImage(context.Background(), img, 50, 50)
		require.NoError(t, err)
		assert.Equal(t, img, out, "Should return original image object when Off")
	})

	t.Run("Strategy_Quality_RejectsBadAspect", func(t *testing.T) {
		cfg.SetSmartFitMode(SmartFitNormal)
		cfg.Tuning.AspectThreshold = 0.1 // Strict threshold

		// Target: 100x100 (1.0)
		// Input: 120x100 (1.2) -> Diff 0.2.
		// Compass Check: Diff 0.2 < 0.5 (Orientation OK).
		// Fit Check: Diff 0.2 > 0.1 (Strict Limit). -> Expect Rejection.
		img := createSolidImage(120, 100, color.White)

		_, err := processor.FitImage(context.Background(), img, 100, 100)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "quality mode rejected")
	})

	t.Run("Strategy_Flexibility_LowEnergy_Fallback", func(t *testing.T) {
		cfg.SetSmartFitMode(SmartFitAggressive)
		cfg.Tuning.MinEnergyThreshold = 0.1 // High enough to fail a solid color

		// Solid color = 0 Entropy
		img := createSolidImage(200, 200, color.White)

		// Target 100x100
		out, err := processor.FitImage(context.Background(), img, 100, 100)
		require.NoError(t, err)

		// We can't easily verify WHICH logic ran without logs/mocks,
		// but we can verify it succeeded and resized.
		assert.Equal(t, 100, out.Bounds().Dx())
	})
}

// MockOS is already defined in mocks_test.go
