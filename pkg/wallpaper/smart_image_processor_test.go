package wallpaper

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTestImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{255, 0, 0, 255}}, image.Point{}, draw.Src)
	return img
}

func TestSmartImageProcessor_FitImage(t *testing.T) {
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	// Enable SmartFit
	cfg.SetSmartFit(true)

	t.Run("FitImage_Resize", func(t *testing.T) {
		mockOS := new(MockOS)
		processor := &smartImageProcessor{
			os:              mockOS,
			config:          cfg,
			aspectThreshold: 2.0,
		}

		// Desktop: 1920x1080
		mockOS.On("getDesktopDimension").Return(1920, 1080, nil)

		// Input: 3840x2160 (16:9, same aspect ratio)
		inputImg := createTestImage(3840, 2160)

		outputImg, err := processor.FitImage(context.Background(), inputImg)
		assert.NoError(t, err)
		assert.NotNil(t, outputImg)

		bounds := outputImg.Bounds()
		assert.Equal(t, 1920, bounds.Dx())
		assert.Equal(t, 1080, bounds.Dy())
	})

	t.Run("FitImage_Crop", func(t *testing.T) {
		mockOS := new(MockOS)
		processor := &smartImageProcessor{
			os:              mockOS,
			config:          cfg,
			aspectThreshold: 2.0,
		}

		// Desktop: 1920x1080 (16:9)
		mockOS.On("getDesktopDimension").Return(1920, 1080, nil)

		// Input: 2000x2000 (1:1)
		inputImg := createTestImage(2000, 2000)

		outputImg, err := processor.FitImage(context.Background(), inputImg)
		assert.NoError(t, err)
		assert.NotNil(t, outputImg)

		bounds := outputImg.Bounds()
		if bounds.Dx() != 1920 {
			t.Errorf("Expected width 1920, got %d", bounds.Dx())
		}
		if bounds.Dy() != 1080 {
			t.Errorf("Expected height 1080, got %d", bounds.Dy())
		}
	})

	t.Run("FitImage_SmartFitDisabled", func(t *testing.T) {
		cfg.SetSmartFit(false)
		mockOS := new(MockOS)
		processor := &smartImageProcessor{
			os:              mockOS,
			config:          cfg,
			aspectThreshold: 2.0,
		}

		// Input: 100x100
		inputImg := createTestImage(100, 100)

		// Should return original image without calling getDesktopDimension
		outputImg, err := processor.FitImage(context.Background(), inputImg)
		assert.NoError(t, err)
		assert.Equal(t, inputImg, outputImg)

		mockOS.AssertNotCalled(t, "getDesktopDimension")
	})
}
