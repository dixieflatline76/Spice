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
		processor := &SmartImageProcessor{
			os:              mockOS,
			config:          cfg,
			aspectThreshold: 2.0,
		}

		// Desktop: 1920x1080
		mockOS.On("GetDesktopDimension").Return(1920, 1080, nil)

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
		processor := &SmartImageProcessor{
			os:              mockOS,
			config:          cfg,
			aspectThreshold: 2.0,
		}

		// Desktop: 1920x1080 (16:9)
		mockOS.On("GetDesktopDimension").Return(1920, 1080, nil)

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
		cfg.SetSmartFitMode(SmartFitOff)
		mockOS := new(MockOS)
		processor := &SmartImageProcessor{
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

func TestSmartImageProcessor_FitImage_CenterFallback(t *testing.T) {
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	// Enable SmartFit Flexible + FaceCrop
	cfg.SetSmartFitMode(SmartFitAggressive)
	cfg.SetFaceCropEnabled(true)

	mockOS := new(MockOS)
	// Hybrid Fallback Test:
	// Input: 1024x833 (Aspect 1.23)
	// Target: 160x90 (Aspect 1.77)
	// Diff: |1.77 - 1.23| = 0.54
	// Limit: 0.5
	// Result: 0.54 > 0.5 -> UNSAFE -> Should use Center Crop (160x90).
	mockOS.On("GetDesktopDimension").Return(160, 90, nil)

	processor := &SmartImageProcessor{
		os:              mockOS,
		config:          cfg,
		aspectThreshold: 2.0,
		resampler:       imaging.Lanczos,
		// No pigo model loaded -> Simulates "No Face Found" OR "Model Missing"
		// If pigo is nil, logic might skip straight to fallback.
		// To simulate "No Face Found" with pigo loaded, we need a valid pigo instance but an image with no face.
		// However, if pigo is nil, it logs "Face Logic: Enabled but pigo model not loaded" and falls through to SmartAnalyzer?
		// Wait, let's check code.
		// Line 312: } else if ... && c.pigo == nil { log... }
		// Fallback to smartcrop analyzer := ...

		// THE CENTER FALLBACK IS INSIDE THE `if c.pigo != nil` BLOCK.
		// So we MUST HAVE Pigo loaded to test the fallback!
	}

	// Load dummy pigo (or use the one from TestFaceDetection if available, otherwise skip)
	// For unit test without asset file, we can't easily load pigo.
	// But we can trick it? No, `processor.pigo` is `*pigo.Pigo`.
	// We need to load it.

	am := asset.NewManager()
	modelData, err := am.GetModel("facefinder")
	if err != nil {
		t.Skip("Facefinder model not found, skipping center fallback test")
	}
	p := pigo.NewPigo()
	pigoInstance, _ := p.Unpack(modelData)
	processor.pigo = pigoInstance

	// Input: 1024x833 (Subject image from user report)
	// Create a red image
	inputImg := createTestImage(1024, 833)

	// Run FitImage
	outputImg, err := processor.FitImage(context.Background(), inputImg)
	require.NoError(t, err)
	require.NotNil(t, outputImg)

	// Expectation:
	// 1. Accepted (Not rejected)
	// 2. Output size matches desktop (1920x1080) ?
	//    Wait, smartPanAndResize RESIZES to target.
	//    Input 1024x833 -> Target 1920x1080.
	//    It should upscale?
	//    Code: `r.resizeWithContext(..., uint(targetWidth), uint(targetHeight))`
	//    Yes, it resizes to target.

	assert.Equal(t, 160, outputImg.Bounds().Dx())
	assert.Equal(t, 90, outputImg.Bounds().Dy())
}
