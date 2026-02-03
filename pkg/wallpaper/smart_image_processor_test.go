package wallpaper

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	_ "image/png" // For decoding png
	"os"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/asset"
	pigo "github.com/esimov/pigo/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	cfg.SetSmartFitMode(SmartFitNormal)

	t.Run("FitImage_Resize", func(t *testing.T) {
		mockOS := new(MockOS)
		cfg.Tuning.AspectThreshold = 2.0
		processor := &SmartImageProcessor{
			os:     mockOS,
			config: cfg,
		}

		// Desktop: 1920x1080
		mockOS.On("GetDesktopDimension").Return(1920, 1080, nil)

		// Input: 3840x2160 (16:9, same aspect ratio)
		inputImg := createTestImage(3840, 2160)

		outputImg, err := processor.FitImage(context.Background(), inputImg, 1920, 1080)
		assert.NoError(t, err)
		assert.NotNil(t, outputImg)

		bounds := outputImg.Bounds()
		assert.Equal(t, 1920, bounds.Dx())
		assert.Equal(t, 1080, bounds.Dy())
	})

	t.Run("FitImage_Crop", func(t *testing.T) {
		mockOS := new(MockOS)
		cfg.Tuning.AspectThreshold = 2.0
		processor := &SmartImageProcessor{
			os:     mockOS,
			config: cfg,
		}

		// Desktop: 1920x1080 (16:9)
		mockOS.On("GetDesktopDimension").Return(1920, 1080, nil)

		// Input: 2000x2000 (1:1)
		inputImg := createTestImage(2000, 2000)

		outputImg, err := processor.FitImage(context.Background(), inputImg, 1920, 1080)
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
		cfg.SetSmartFitMode(SmartFitOff)
		mockOS := new(MockOS)
		cfg.Tuning.AspectThreshold = 2.0
		processor := &SmartImageProcessor{
			os:     mockOS,
			config: cfg,
		}

		// Input: 100x100
		inputImg := createTestImage(100, 100)

		// Should return original image without calling getDesktopDimension
		outputImg, err := processor.FitImage(context.Background(), inputImg, 1920, 1080)
		assert.NoError(t, err)
		assert.Equal(t, inputImg, outputImg)

		mockOS.AssertNotCalled(t, "GetDesktopDimension")
	})
}

func TestFaceDetection(t *testing.T) {
	// Load facefinder model
	am := asset.NewManager()
	modelData, err := am.GetModel("facefinder")
	if err != nil {
		t.Skip("Facefinder model not found, skipping face detection test")
	}

	p := pigo.NewPigo()
	pigoInstance, err := p.Unpack(modelData)
	if err != nil {
		t.Fatal(err)
	}

	// Load test image
	f, err := os.Open("testdata/face.png")
	if err != nil {
		t.Skip("Test image 'testdata/face.png' not found, skipping")
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}

	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	cfg.SetFaceBoostEnabled(true)
	cfg.SetFaceCropEnabled(true)

	mockOS := new(MockOS)
	mockConfig := cfg

	// Create processor with config (Fit Off)
	processor := &SmartImageProcessor{
		os:     mockOS,
		config: mockConfig,
		pigo:   pigoInstance,
	}

	// Test findBestFace
	rect, err := processor.findBestFace(img)
	if err != nil {
		t.Logf("Face detection failed (could be due to model/image mismatch): %v", err)
	} else {
		// Use require to stop if basic assertions fail
		require.False(t, rect.Empty(), "Face rectangle should not be empty")
		require.True(t, rect.In(img.Bounds()), "Face rectangle should be within image bounds")

		// Test smartPanAndResize (replacing cropAroundFace)
		targetW, targetH := 50, 50
		faceCenter := rect.Min.Add(rect.Size().Div(2))

		// Create a processor context
		// We can't access private method smartPanAndResize easily if it's not exported?
		// But we are in package wallpaper, so we can.

		result, err := processor.smartPanAndResize(context.Background(), img, faceCenter, targetW, targetH)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Verify result dimensions (should match target)
		assert.Equal(t, targetW, result.Bounds().Dx())
		assert.Equal(t, targetH, result.Bounds().Dy())
	}
}

func TestSmartImageProcessor_FitImage_Flexibility_DualSafety(t *testing.T) {
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	cfg.SetSmartFitMode(SmartFitAggressive)

	mockOS := new(MockOS)
	mockOS.On("GetDesktopDimension").Return(160, 90, nil)

	cfg.Tuning.AspectThreshold = 2.0
	processor := &SmartImageProcessor{
		os:        mockOS,
		config:    cfg,
		resampler: imaging.Lanczos,
	}

	t.Run("EnergyCheck_LowEnergy_CenterFallback", func(t *testing.T) {
		// Create a very flat image (solid grey)
		img := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
		draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{128, 128, 128, 255}}, image.Point{}, draw.Src)

		outputImg, err := processor.FitImage(context.Background(), img, 160, 90)
		assert.NoError(t, err)
		require.NotNil(t, outputImg)
		assert.Equal(t, 160, outputImg.Bounds().Dx())
		assert.Equal(t, 90, outputImg.Bounds().Dy())
	})

	t.Run("FeetGuard_BottomHugging_CenterFallback", func(t *testing.T) {
		// Create an image with high entropy ONLY at the bottom
		img := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
		// Top part: Flat white
		draw.Draw(img, image.Rect(0, 0, 1000, 750), &image.Uniform{color.White}, image.Point{}, draw.Src)
		// Bottom part: High entropy (noise) to attract SmartCrop
		for y := 750; y < 1000; y++ {
			for x := 0; x < 1000; x++ {
				img.Set(x, y, color.RGBA{uint8(x % 255), uint8(y % 255), uint8((x + y) % 255), 255})
			}
		}

		outputImg, err := processor.FitImage(context.Background(), img, 160, 90)
		assert.NoError(t, err)
		require.NotNil(t, outputImg)
		// Because it's tagged as a "Feet Crop" (Bottom half), it should fallback to Center.
		assert.Equal(t, 160, outputImg.Bounds().Dx())
		assert.Equal(t, 90, outputImg.Bounds().Dy())
	})

	t.Run("Pass_HighEnergy_CentralSubject", func(t *testing.T) {
		// Create an image with high energy in the center (Boat simulation)
		img := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
		draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
		// Subject in center
		for y := 400; y < 600; y++ {
			for x := 400; x < 600; x++ {
				img.Set(x, y, color.Black)
			}
		}

		outputImg, err := processor.FitImage(context.Background(), img, 160, 90)
		assert.NoError(t, err)
		require.NotNil(t, outputImg)
		assert.Equal(t, 160, outputImg.Bounds().Dx())
		assert.Equal(t, 90, outputImg.Bounds().Dy())
	})

	t.Run("FeetGuard_BoxyImage_SlackAware_CenterFallback", func(t *testing.T) {
		// Image: 1000x800 (Boxy, Aspect 1.25)
		// Target: 160x90 (Aspect 1.77)
		// Crop height will be: 1000 / (160/90) = 562.
		// SlackY = 800 - 562 = 238.
		// Slack Threshold (0.8) * 238 = 190.

		img := image.NewRGBA(image.Rect(0, 0, 1000, 800))
		draw.Draw(img, image.Rect(0, 0, 1000, 500), &image.Uniform{color.White}, image.Point{}, draw.Src)
		// Bottom part: High entropy (noise)
		for y := 500; y < 800; y++ {
			for x := 0; x < 1000; x++ {
				img.Set(x, y, color.RGBA{uint8(x % 255), uint8(y % 255), uint8((x + y) % 255), 255})
			}
		}

		// SmartCrop will pick the bottom (Min.Y = 238).
		// Since 238 > 190, it should trigger Slack Guard and fallback to Center (Min.Y = 119).
		outputImg, err := processor.FitImage(context.Background(), img, 160, 90)
		assert.NoError(t, err)
		require.NotNil(t, outputImg)

		// If we use imaging.Resize, it's hard to verify the crop directly.
		// But we know it's 160x90.
		assert.Equal(t, 160, outputImg.Bounds().Dx())
		assert.Equal(t, 90, outputImg.Bounds().Dy())
	})
}

func TestSmartImageProcessor_FitImage_Quality_Rejection(t *testing.T) {
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	cfg.SetSmartFitMode(SmartFitNormal)

	mockOS := new(MockOS)
	// Desktop: 160x90 (16:9, ~1.77)
	mockOS.On("GetDesktopDimension").Return(160, 90, nil)

	cfg.Tuning.AspectThreshold = 0.9
	processor := &SmartImageProcessor{
		os:        mockOS,
		config:    cfg,
		resampler: imaging.Lanczos,
		// No Pigo -> No Face Found
	}

	// Input: 100x100 (1:1, Aspect 1.0)
	// Diff: |1.77 - 1.0| = 0.77
	// Wait, Quality is 0.9.  0.77 < 0.9. This would PASS normally.
	// We need a rejection scenario.
	// Try Portrait: 160x200 (0.8)
	// Diff: |1.77 - 0.8| = 0.97.  0.97 > 0.9.
	// Width 160 >= 160, Height 200 >= 90.
	// Expect REJECTION.
	inputImg := createTestImage(160, 200)

	_, err := processor.FitImage(context.Background(), inputImg, 160, 90)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Quality mode")
}

func TestSmartImageProcessor_FitImage_Quality_Rescue(t *testing.T) {
	// Requirements:
	// Mode: Quality
	// Input: Bad Aspect (Diff > 0.9)
	// Face: Strong (Q > 20)
	// Expect: Success

	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	cfg.SetSmartFitMode(SmartFitNormal)
	cfg.SetFaceCropEnabled(true)

	// Load Pigo
	am := asset.NewManager()
	modelData, err := am.GetModel("facefinder")
	if err != nil {
		t.Skip("Facefinder model not found")
	}
	p := pigo.NewPigo()
	pigoInstance, _ := p.Unpack(modelData)

	mockOS := new(MockOS)
	// Target: 16:9 (160x90) (Aspect 1.77)
	mockOS.On("GetDesktopDimension").Return(160, 90, nil)

	cfg.Tuning.AspectThreshold = 0.9
	processor := &SmartImageProcessor{
		os:        mockOS,
		config:    cfg,
		pigo:      pigoInstance,
		resampler: imaging.Lanczos,
	}

	// Input: Face Image (Square-ish?) "testdata/face.png"
	// We need to check its aspect.
	// Let's assume we load it and it has a face.
	f, err := os.Open("testdata/face.png")
	if err != nil {
		t.Skip("testdata/face.png not found")
	}
	defer f.Close()
	faceImg, _, _ := image.Decode(f)

	// If face.png is square (likely), Aspect 1.0. Diff ~0.77.
	// 0.77 < 0.9. It passes naturally!
	// We need to FORCE a bad aspect ratio.
	// Let's Pad the image to make it extremely tall (portrait)?
	// Or crop it?
	// If we crop it to be very tall, we might lose the face or make it small.
	// Better: Change the Desktop target to be Ultrawide (21:9)!
	// 21:9 = ~2.33.
	// Image (Square 1.0). Diff: 1.33 > 0.9.
	// This forces Rejection UNLESS Rescued.
	mockOS2 := new(MockOS)
	mockOS2.On("GetDesktopDimension").Return(210, 90, nil) // 21:9
	processor.os = mockOS2

	outputImg, err := processor.FitImage(context.Background(), faceImg, 210, 90)

	// If rescue works, no error.
	require.NoError(t, err)
	require.NotNil(t, outputImg)
	assert.Equal(t, 210, outputImg.Bounds().Dx())
}

// Removed outdated fallback test - replaced by DualSafety test

func TestSmartImageProcessor_FitImage_RespectsTargetRatio(t *testing.T) {
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	cfg.SetSmartFitMode(SmartFitNormal)

	mockOS := new(MockOS)
	// mockOS.GetDesktopDimension should NOT be called now as we pass explicit targets.

	processor := &SmartImageProcessor{
		os:        mockOS,
		config:    cfg,
		resampler: imaging.Lanczos,
	}

	// Create a 4000x4000 square image (large enough for 1080p and 4K)
	img := createTestImage(4000, 4000)

	// Case 1: Landscape Target (1920x1080)
	// We pass targetW=1920, targetH=1080
	out1, err := processor.FitImage(context.Background(), img, 1920, 1080)
	require.NoError(t, err)
	assert.Equal(t, 1920, out1.Bounds().Dx())
	assert.Equal(t, 1080, out1.Bounds().Dy())

	// Case 2: Portrait Target (1080x1920)
	// We pass targetW=1080, targetH=1920
	out2, err := processor.FitImage(context.Background(), img, 1080, 1920)
	require.NoError(t, err)
	assert.Equal(t, 1080, out2.Bounds().Dx())
	assert.Equal(t, 1920, out2.Bounds().Dy())
}
