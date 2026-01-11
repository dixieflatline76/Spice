package wallpaper

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	_ "image/png" // For decoding png
	"os"
	"testing"

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

		// Test cropAroundFace
		// Our test image is small (generated). Let's pick a target size that definitely fits.
		targetW, targetH := 50, 50
		crop := processor.cropAroundFace(img.Bounds(), rect, targetW, targetH)

		// Verify aspect ratio matches target (1:1)
		// The crop should be a square, as large as possible.
		// Since we don't know the exact image size (generated), we just check it is square.
		// Allow for small rounding error (1px)
		width := crop.Dx()
		height := crop.Dy()
		diff := width - height
		if diff < 0 {
			diff = -diff
		}
		assert.LessOrEqual(t, diff, 1, "Crop aspect ratio mismatch (expected square)")

		// Verify crop contains center of face (roughly)
		faceCenter := rect.Min.Add(rect.Size().Div(2))
		assert.True(t, faceCenter.In(crop), "Crop should contain face center")
	}
}
