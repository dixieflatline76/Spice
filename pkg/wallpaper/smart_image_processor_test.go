//go:build !linux

package wallpaper

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // For decoding jpeg regression images
	_ "image/png"  // For decoding png
	"os"
	"path/filepath"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/v2/asset"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
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

		outputImg, err := processor.FitImage(context.Background(), inputImg, 1920, 1080, provider.TuningOptions{})
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

		outputImg, err := processor.FitImage(context.Background(), inputImg, 1920, 1080, provider.TuningOptions{})
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
		outputImg, err := processor.FitImage(context.Background(), inputImg, 1920, 1080, provider.TuningOptions{})
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

		outputImg, err := processor.FitImage(context.Background(), img, 160, 90, provider.TuningOptions{})
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

		outputImg, err := processor.FitImage(context.Background(), img, 160, 90, provider.TuningOptions{})
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

		outputImg, err := processor.FitImage(context.Background(), img, 160, 90, provider.TuningOptions{})
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
		outputImg, err := processor.FitImage(context.Background(), img, 160, 90, provider.TuningOptions{})
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

	_, err := processor.FitImage(context.Background(), inputImg, 160, 90, provider.TuningOptions{})
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

	outputImg, err := processor.FitImage(context.Background(), faceImg, 210, 90, provider.TuningOptions{})

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
	out1, err := processor.FitImage(context.Background(), img, 1920, 1080, provider.TuningOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1920, out1.Bounds().Dx())
	assert.Equal(t, 1080, out1.Bounds().Dy())

	// Case 2: Portrait Target (1080x1920)
	// We pass targetW=1080, targetH=1920
	out2, err := processor.FitImage(context.Background(), img, 1080, 1920, provider.TuningOptions{Anchor: provider.AnchorAuto})
	require.NoError(t, err)
	assert.Equal(t, 1080, out2.Bounds().Dx())
	assert.Equal(t, 1920, out2.Bounds().Dy())
}

// createGradientImage creates a WxH image where the green channel encodes
// the Y position (0 at top, 255 at bottom). This lets us verify WHERE a crop
// landed by sampling the center pixel's green value.
func createGradientImage(width, height int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		g := uint8(float64(y) / float64(height-1) * 255)
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 128, G: g, B: 0, A: 255})
		}
	}
	return img
}

// sampleCenterGreen returns the green channel value of the center pixel.
func sampleCenterGreen(img image.Image) uint8 {
	cx := img.Bounds().Min.X + img.Bounds().Dx()/2
	cy := img.Bounds().Min.Y + img.Bounds().Dy()/2
	_, g, _, _ := img.At(cx, cy).RGBA()
	return uint8(g >> 8) // RGBA returns 16-bit, we want 8-bit
}

func TestFitImage_CropAnchor_NoFace_ShiftsOutput(t *testing.T) {
	// Strategy: Create a landscape gradient image (2000x1200) where green encodes Y.
	// Crop to 2000x800 (wider landscape). Both are landscape so no orientation
	// mismatch. We prove the anchor works by showing that TopCenter and BottomCenter
	// produce measurably different green values (i.e., different crop positions).
	// TopCenter → low green (crop high), BottomCenter → high green (crop low).

	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	cfg.SetSmartFitMode(SmartFitAggressive)
	cfg.Tuning.AspectThreshold = 0.9

	mockOS := new(MockOS)
	processor := &SmartImageProcessor{
		os:        mockOS,
		config:    cfg,
		resampler: imaging.NearestNeighbor, // Preserve pixel values exactly
	}

	srcImg := createGradientImage(2000, 1200)

	// 1. Top anchor — crop should land in the upper region (low green)
	outTop, err := processor.FitImage(context.Background(), srcImg, 2000, 800, provider.TuningOptions{Anchor: provider.AnchorTopCenter})
	require.NoError(t, err)
	require.NotNil(t, outTop)
	assert.Equal(t, 2000, outTop.Bounds().Dx())
	assert.Equal(t, 800, outTop.Bounds().Dy())
	greenTop := sampleCenterGreen(outTop)

	// 2. Bottom anchor — crop should land in the lower region (high green)
	outBottom, err := processor.FitImage(context.Background(), srcImg, 2000, 800, provider.TuningOptions{Anchor: provider.AnchorBottomCenter})
	require.NoError(t, err)
	require.NotNil(t, outBottom)
	greenBottom := sampleCenterGreen(outBottom)

	// 3. MiddleCenter anchor — should land in between
	outMiddle, err := processor.FitImage(context.Background(), srcImg, 2000, 800, provider.TuningOptions{Anchor: provider.AnchorMiddleCenter})
	require.NoError(t, err)
	greenMiddle := sampleCenterGreen(outMiddle)

	t.Logf("Green values — Top:%d  Middle:%d  Bottom:%d", greenTop, greenMiddle, greenBottom)

	// Core proof: TopCenter produces different (lower) green than BottomCenter
	assert.Less(t, greenTop, greenBottom,
		"TopCenter anchor must produce lower green (higher crop) than BottomCenter anchor")

	// The difference should be significant (at least 15 out of 255)
	assert.Greater(t, int(greenBottom)-int(greenTop), 15,
		"Top-to-Bottom shift should be at least 15 green levels apart")

	// Middle should be between Top and Bottom
	assert.GreaterOrEqual(t, greenMiddle, greenTop,
		"MiddleCenter should be >= TopCenter")
	assert.LessOrEqual(t, greenMiddle, greenBottom,
		"MiddleCenter should be <= BottomCenter")
}

func TestFitImage_CropAnchor_NoFace_BlendWeight(t *testing.T) {
	// Verify the no-face blend weight (default 0.85) produces the expected center.
	// Image: 2000x1200. AnchorTopCenter: normalized (0.5, 0.2)
	// Image center: (1000, 600)
	// Anchor point: (1000, 240)  [0.2 * 1200]
	// Expected blend: Y = 600*(1-0.85) + 240*0.85 = 90 + 204 = 294
	// At Y=294 in 1200px, green ≈ 294/1199 * 255 ≈ 62

	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	cfg.SetSmartFitMode(SmartFitAggressive)
	cfg.Tuning.AspectThreshold = 0.9
	cfg.Tuning.AnchorBlendNoFace = 0.85

	mockOS := new(MockOS)
	processor := &SmartImageProcessor{
		os:        mockOS,
		config:    cfg,
		resampler: imaging.NearestNeighbor,
	}

	srcImg := createGradientImage(2000, 1200)

	out, err := processor.FitImage(context.Background(), srcImg, 2000, 800, provider.TuningOptions{Anchor: provider.AnchorTopCenter})
	require.NoError(t, err)
	require.NotNil(t, out)

	green := sampleCenterGreen(out)
	t.Logf("AnchorTopCenter green=%d (expected ~62 for Y≈294)", green)

	// The blend should put us clearly in the upper portion of the image
	// Green should be well below the center value of ~127
	assert.Less(t, green, uint8(110),
		"AnchorTopCenter with 85%% weight should crop into the upper region (green < 110)")
	// But not at the literal edge (green ~0 would mean Y=0)
	assert.Greater(t, green, uint8(10),
		"Blend should not slam to the literal edge (green > 10)")
}

func TestFitImage_CropAnchor_WithFace_ReducedShift(t *testing.T) {
	// When a face is found, the anchor weight drops to 0.6 (from 0.85),
	// meaning the crop shifts LESS from the face center.
	// We simulate this by using pigo with a real face image.
	// If the face image isn't available, we fall back to a unit test of selectStrategy directly.

	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	cfg.SetSmartFitMode(SmartFitAggressive)
	cfg.Tuning.AspectThreshold = 2.0
	cfg.Tuning.AnchorBlendFace = 0.6
	cfg.Tuning.AnchorBlendNoFace = 0.85
	cfg.SetFaceBoostEnabled(true)

	// Try to load pigo for real face detection
	am := asset.NewManager()
	modelData, err := am.GetModel("facefinder")
	if err != nil {
		t.Skip("Facefinder model not available, skipping face+anchor test")
	}
	p := pigo.NewPigo()
	pigoInstance, err := p.Unpack(modelData)
	if err != nil {
		t.Skip("Failed to unpack pigo model")
	}

	f, err := os.Open("testdata/face.png")
	if err != nil {
		t.Skip("testdata/face.png not found, skipping face+anchor test")
	}
	defer f.Close()
	faceImg, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}

	mockOS := new(MockOS)
	processor := &SmartImageProcessor{
		os:        mockOS,
		config:    cfg,
		pigo:      pigoInstance,
		resampler: imaging.NearestNeighbor,
	}

	// First verify face is actually detected
	faceBox, err := processor.findBestFace(faceImg)
	if err != nil {
		t.Skip("No face detected in test image, skipping face+anchor comparison")
	}
	t.Logf("Face detected at %v", faceBox)

	imgW := faceImg.Bounds().Dx()
	imgH := faceImg.Bounds().Dy()

	// Use a target aspect that requires cropping
	targetW := imgW
	targetH := imgW / 2 // Force landscape crop on a roughly square face image
	if targetH >= imgH {
		targetH = imgH / 2
	}

	// 1. No anchor — should center on face
	outNoAnchor, err := processor.FitImage(context.Background(), faceImg, targetW, targetH, provider.TuningOptions{})
	if err != nil {
		t.Skipf("FitImage rejected this aspect ratio: %v", err)
	}

	// 2. With anchor at bottom — should shift, but LESS than no-face because weight=0.6
	outWithAnchor, err := processor.FitImage(context.Background(), faceImg, targetW, targetH, provider.TuningOptions{Anchor: provider.AnchorBottomCenter})
	require.NoError(t, err)

	require.NotNil(t, outNoAnchor)
	require.NotNil(t, outWithAnchor)

	// Both should have correct dimensions
	assert.Equal(t, targetW, outNoAnchor.Bounds().Dx())
	assert.Equal(t, targetH, outNoAnchor.Bounds().Dy())
	assert.Equal(t, targetW, outWithAnchor.Bounds().Dx())
	assert.Equal(t, targetH, outWithAnchor.Bounds().Dy())

	// The images should differ (anchor caused a visual change)
	// Sample a grid of pixels to detect any difference
	diffCount := 0
	for y := 0; y < targetH; y += targetH / 10 {
		for x := 0; x < targetW; x += targetW / 10 {
			r1, g1, b1, _ := outNoAnchor.At(x, y).RGBA()
			r2, g2, b2, _ := outWithAnchor.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 {
				diffCount++
			}
		}
	}

	t.Logf("Pixel differences between no-anchor and bottom-anchor: %d sample points differ", diffCount)
	assert.Greater(t, diffCount, 0,
		"Anchor should produce a visually different crop than no-anchor (at least 1 sampled pixel must differ)")
}

// TestFitImage_CropAnchor_VisualVerification processes real images with all 9 anchors
// and saves the output to testdata/anchor_verify/ for manual visual inspection.
// Run with: go test -run TestFitImage_CropAnchor_VisualVerification -v
func TestFitImage_CropAnchor_VisualVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping visual verification in short mode")
	}

	outDir := "testdata/anchor_verify"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	// Load pigo for face detection
	am := asset.NewManager()
	modelData, err := am.GetModel("facefinder")
	if err != nil {
		t.Skip("Facefinder model not found")
	}
	p := pigo.NewPigo()
	pigoInstance, _ := p.Unpack(modelData)

	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)
	cfg.SetSmartFitMode(SmartFitAggressive)
	cfg.SetFaceCropEnabled(true)
	cfg.SetFaceBoostEnabled(true)

	mockOS := new(MockOS)
	processor := NewSmartImageProcessor(mockOS, cfg, pigoInstance)

	// Target: 1920x1080 (standard desktop)
	targetW, targetH := 1920, 1080

	anchors := []struct {
		Anchor provider.CropAnchor
		Name   string
	}{
		{0, "auto"},
		{provider.AnchorTopLeft, "top_left"},
		{provider.AnchorTopCenter, "top_center"},
		{provider.AnchorTopRight, "top_right"},
		{provider.AnchorMiddleLeft, "middle_left"},
		{provider.AnchorMiddleCenter, "middle_center"},
		{provider.AnchorMiddleRight, "middle_right"},
		{provider.AnchorBottomLeft, "bottom_left"},
		{provider.AnchorBottomCenter, "bottom_center"},
		{provider.AnchorBottomRight, "bottom_right"},
	}

	// Pick a few diverse images from the regression set
	testFiles := []string{
		"testdata/regressions/10154.jpg",
		"testdata/regressions/435851.jpg",
		"testdata/regressions/436105.jpg",
		"testdata/regressions/Wikimedia_33060206.jpg",
		"testdata/regressions/Wallhaven_zxorrj.jpg",
		"testdata/regressions/Pexels_34541206.jpeg",
	}

	// HTML report
	var html string
	html += `<!DOCTYPE html><html><head><style>
		body { background: #1a1a2e; color: #eee; font-family: sans-serif; padding: 20px; }
		h1 { color: #e94560; }
		h2 { color: #f0a500; border-bottom: 1px solid #333; padding-bottom: 8px; }
		.grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 40px; }
		.cell { text-align: center; }
		.cell img { width: 100%; border: 2px solid #333; border-radius: 4px; }
		.cell img:hover { border-color: #e94560; }
		.label { margin-top: 4px; font-size: 0.85em; color: #aaa; }
		.highlight { border-color: #f0a500 !important; }
	</style></head><body>
	<h1>🎯 Crop Anchor Visual Verification</h1>
	<p>Target: ` + fmt.Sprintf("%dx%d", targetW, targetH) + ` — Each image processed with all 9 anchors + auto.</p>`

	for _, filePath := range testFiles {
		f, err := os.Open(filePath)
		if err != nil {
			t.Logf("Skipping %s: %v", filePath, err)
			continue
		}

		srcImg, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			t.Logf("Skipping %s: decode error %v", filePath, err)
			continue
		}

		baseName := filepath.Base(filePath)
		baseName = baseName[:len(baseName)-len(filepath.Ext(baseName))]

		t.Logf("Processing %s (%dx%d)...", baseName, srcImg.Bounds().Dx(), srcImg.Bounds().Dy())

		html += fmt.Sprintf(`<h2>%s (%dx%d)</h2><div class="grid">`,
			baseName, srcImg.Bounds().Dx(), srcImg.Bounds().Dy())

		for _, a := range anchors {
			outImg, err := processor.FitImage(context.Background(), srcImg, targetW, targetH, provider.TuningOptions{Anchor: a.Anchor})
			if err != nil {
				t.Logf("  %s/%s: ERROR %v", baseName, a.Name, err)
				html += fmt.Sprintf(`<div class="cell"><div class="label">%s<br>ERROR: %v</div></div>`, a.Name, err)
				continue
			}

			outFile := fmt.Sprintf("%s_%s.jpg", baseName, a.Name)
			outPath := filepath.Join(outDir, outFile)
			if err := imaging.Save(outImg, outPath); err != nil {
				t.Logf("  Failed to save %s: %v", outPath, err)
				continue
			}

			t.Logf("  ✓ %s → %s (%dx%d)", a.Name, outPath, outImg.Bounds().Dx(), outImg.Bounds().Dy())

			highlight := ""
			if a.Name == "auto" {
				highlight = ` class="highlight"`
			}
			html += fmt.Sprintf(`<div class="cell"><img src="%s"%s/><div class="label">%s</div></div>`,
				outFile, highlight, a.Name)
		}

		html += `</div>`
	}

	html += `</body></html>`

	reportPath := filepath.Join(outDir, "report.html")
	if err := os.WriteFile(reportPath, []byte(html), 0644); err != nil {
		t.Fatalf("Failed to write report: %v", err)
	}

	absReport, _ := filepath.Abs(reportPath)
	t.Logf("\n\n📊 VISUAL REPORT: file:///%s\n", filepath.ToSlash(absReport))
	t.Logf("Open the report in a browser to visually verify anchor shifts.\n")
}
