package wallpaper

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// createColoredTestImage generates a blank image of a specific size and uniform color
func createColoredTestImage(width, height int, col color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, col)
		}
	}
	return img
}

func TestVirtualFramer_Passthrough(t *testing.T) {
	mockProcessor := new(MockImageProcessor)
	cfg := &Config{
		VirtualFramingFallback: false,
	}

	framer := NewVirtualFramer(mockProcessor, cfg)

	srcImg := createColoredTestImage(100, 100, color.Black)
	expectedImg := createColoredTestImage(1920, 1080, color.Black)

	// We expect the mock processor to be called because framing is off
	mockProcessor.On("FitImage", mock.Anything, srcImg, 1920, 1080, provider.TuningOptions{Anchor: provider.AnchorAuto}).Return(expectedImg, nil)

	outImg, err := framer.FitImage(context.Background(), srcImg, 1920, 1080, provider.TuningOptions{Anchor: provider.AnchorAuto})
	assert.NoError(t, err)
	assert.Equal(t, expectedImg, outImg)
	mockProcessor.AssertExpectations(t)
}

func TestVirtualFramer_Passthrough_TinyImage(t *testing.T) {
	mockProcessor := new(MockImageProcessor)
	cfg := &Config{VirtualFramingFallback: true}

	framer := NewVirtualFramer(mockProcessor, cfg)

	srcImg := createColoredTestImage(5, 5, color.Black) // Impossibly small image
	expectedImg := createColoredTestImage(1920, 1080, color.Black)

	// Even though framing is on, the image is too small to frame, so it should passthrough
	mockProcessor.On("FitImage", mock.Anything, srcImg, 1920, 1080, provider.TuningOptions{Anchor: provider.AnchorAuto}).Return(expectedImg, nil)

	outImg, err := framer.FitImage(context.Background(), srcImg, 1920, 1080, provider.TuningOptions{Anchor: provider.AnchorAuto})
	assert.NoError(t, err)
	assert.Equal(t, expectedImg, outImg)
	mockProcessor.AssertExpectations(t)
}

func TestVirtualFramer_ThresholdLogic(t *testing.T) {
	tests := []struct {
		name             string
		aspectThreshold  float64
		srcW, srcH       int
		targetW, targetH int
		shouldFrame      bool
	}{
		{
			name:            "Square on Ultrawide with 0.9 Threshold -> Frames",
			aspectThreshold: 0.9,
			srcW:            1000, srcH: 1000, // 1.0 ratio
			targetW: 3440, targetH: 1440, // 2.38 ratio (mismatch = 1.38)
			shouldFrame: true,
		},
		{
			name:            "Square on Ultrawide with 1.5 Threshold -> Bypasses",
			aspectThreshold: 1.5,
			srcW:            1000, srcH: 1000, // 1.0 ratio
			targetW: 3440, targetH: 1440, // 2.38 ratio. mismatch is 1.38, < 1.5
			shouldFrame: false,
		},
		{
			name:            "Landscape on Ultrawide with 0.9 Threshold -> Bypasses",
			aspectThreshold: 0.9,
			srcW:            1920, srcH: 1080, // 1.77 ratio
			targetW: 3440, targetH: 1440, // 2.38 ratio (mismatch = 0.61) -> < 0.9 -> Bypasses
			shouldFrame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProcessor := new(MockImageProcessor)
			cfg := &Config{
				VirtualFramingFallback: true,
				Tuning: TuningConfig{
					AspectThreshold: tt.aspectThreshold,
				},
			}

			framer := NewVirtualFramer(mockProcessor, cfg)

			srcImg := createNoisyImage(tt.srcW, tt.srcH)

			if !tt.shouldFrame {
				mockProcessor.On("CheckCompatibility", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockProcessor.On("FitImage", mock.Anything, srcImg, tt.targetW, tt.targetH, provider.TuningOptions{Anchor: provider.AnchorAuto}).Return(createColoredTestImage(tt.targetW, tt.targetH, color.Black), nil)
			} else {
				// With the new logic, CheckCompatibility might not even be called if aspectDiff > threshold!
				// We don't mock CheckCompatibility here, or we use mock.Anything.
			}

			outImg, err := framer.FitImage(context.Background(), srcImg, tt.targetW, tt.targetH, provider.TuningOptions{Anchor: provider.AnchorAuto})
			assert.NoError(t, err)
			assert.NotNil(t, outImg)
			assert.Equal(t, tt.targetW, outImg.Bounds().Dx())
			assert.Equal(t, tt.targetH, outImg.Bounds().Dy())

			if !tt.shouldFrame {
				mockProcessor.AssertExpectations(t)
			} else {
				mockProcessor.AssertNotCalled(t, "FitImage", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			}
		})
	}
}

// createNoisyImage generates an image with high pixel variance (like a painting)
// using large blocks of color to avoid downsampling aliasing to 0 variance.
func createNoisyImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if x < width/2 && y < height/2 {
				img.Set(x, y, color.RGBA{255, 0, 0, 255})
			} else if x >= width/2 && y < height/2 {
				img.Set(x, y, color.RGBA{0, 255, 0, 255})
			} else if x < width/2 && y >= height/2 {
				img.Set(x, y, color.RGBA{0, 0, 255, 255})
			} else {
				img.Set(x, y, color.RGBA{255, 255, 0, 255})
			}
		}
	}
	return img
}

func TestVirtualFramer_ObjectHeuristic(t *testing.T) {
	// Set threshold to 1.5 so that AspectDiff doesn't force framing automatically (1000/2000 vs 1920/1080 diff is 1.27)
	cfg := &Config{
		VirtualFramingFallback: true,
		Tuning: TuningConfig{
			AspectThreshold: 1.5,
		},
	}

	t.Run("Solid Grey Border (simulating studio object) -> Bypasses", func(t *testing.T) {
		mockProcessor := new(MockImageProcessor)
		framer := NewVirtualFramer(mockProcessor, cfg)

		studioImg := createColoredTestImage(1000, 2000, color.RGBA{128, 128, 128, 255})
		mockProcessor.On("CheckCompatibility", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockProcessor.On("FitImage", mock.Anything, studioImg, 1920, 1080, provider.TuningOptions{Anchor: provider.AnchorAuto}).Return(createColoredTestImage(1920, 1080, color.Black), nil)

		_, err := framer.FitImage(context.Background(), studioImg, 1920, 1080, provider.TuningOptions{Anchor: provider.AnchorAuto})
		assert.NoError(t, err)
		mockProcessor.AssertExpectations(t)
	})

	t.Run("Noisy Paint Strokes -> Frames", func(t *testing.T) {
		mockProcessor := new(MockImageProcessor)
		framer := NewVirtualFramer(mockProcessor, cfg)

		noisyImg := createNoisyImage(1000, 2000)
		mockProcessor.On("CheckCompatibility", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("incompatible"))

		outImg, err := framer.FitImage(context.Background(), noisyImg, 1920, 1080, provider.TuningOptions{Anchor: provider.AnchorAuto})
		assert.NoError(t, err)
		assert.NotNil(t, outImg)
		mockProcessor.AssertNotCalled(t, "FitImage", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestVirtualFramer_FrameStyleSelection(t *testing.T) {
	framer := &VirtualFramer{}

	// Test pure greyscale (dark)
	style := framer.determineFrameStyle(color.RGBA{50, 50, 50, 255})
	assert.Equal(t, FrameStyleWhite, style)

	// Test pure greyscale (bright)
	style = framer.determineFrameStyle(color.RGBA{220, 220, 220, 255})
	assert.Equal(t, FrameStyleBlack, style)

	// Test warm saturated (gold)
	style = framer.determineFrameStyle(color.RGBA{200, 100, 50, 255})
	assert.Equal(t, FrameStyleWood, style)

	// Test cool/blue
	style = framer.determineFrameStyle(color.RGBA{50, 100, 200, 255})
	assert.Equal(t, FrameStyleBlack, style)
}

func TestVirtualFramer_Integration(t *testing.T) {
	mockProcessor := new(MockImageProcessor)
	cfg := &Config{
		VirtualFramingFallback: true,
		VirtualWallColor:       WallAlgorithmic,
	}

	framer := NewVirtualFramer(mockProcessor, cfg)

	srcImg := createNoisyImage(1000, 2000) // Portrait noisy image
	mockProcessor.On("CheckCompatibility", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("incompatible"))

	// Should frame.
	outImg, err := framer.FitImage(context.Background(), srcImg, 1920, 1080, provider.TuningOptions{Anchor: provider.AnchorAuto})
	assert.NoError(t, err)
	assert.NotNil(t, outImg)
	assert.Equal(t, 1920, outImg.Bounds().Dx())
	assert.Equal(t, 1080, outImg.Bounds().Dy())
}

func TestVirtualFramer_CheckCompatibility_ReturnsSentinelError(t *testing.T) {
	mockProcessor := new(MockImageProcessor)
	cfg := &Config{VirtualFramingFallback: true}
	framer := NewVirtualFramer(mockProcessor, cfg)

	// SmartFit says incompatible
	mockProcessor.On("CheckCompatibility", 1000, 2000, 1920, 1080).Return(fmt.Errorf("incompatible aspect ratio"))

	err := framer.CheckCompatibility(1000, 2000, 1920, 1080)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrRequiresVirtualFraming)
	mockProcessor.AssertExpectations(t)
}

func TestVirtualFramer_CheckCompatibility_PassesThroughSizeError(t *testing.T) {
	mockProcessor := new(MockImageProcessor)
	cfg := &Config{VirtualFramingFallback: true}
	framer := NewVirtualFramer(mockProcessor, cfg)

	// SmartFit says too small
	originalErr := fmt.Errorf("insufficient size")
	mockProcessor.On("CheckCompatibility", 5, 5, 1920, 1080).Return(originalErr)

	err := framer.CheckCompatibility(5, 5, 1920, 1080)
	assert.Error(t, err)
	assert.Equal(t, originalErr, err) // Should be original error, NOT sentinel
	mockProcessor.AssertExpectations(t)
}

func TestVirtualFramer_CheckCompatibility_PassesThroughCompatible(t *testing.T) {
	mockProcessor := new(MockImageProcessor)
	cfg := &Config{VirtualFramingFallback: true}
	framer := NewVirtualFramer(mockProcessor, cfg)

	mockProcessor.On("CheckCompatibility", 1920, 1080, 1920, 1080).Return(nil)

	err := framer.CheckCompatibility(1920, 1080, 1920, 1080)
	assert.NoError(t, err)
	mockProcessor.AssertExpectations(t)
}
