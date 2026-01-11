package wallpaper

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"time"

	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/util/log"
	pigo "github.com/esimov/pigo/core"
	"github.com/muesli/smartcrop"
)

// SmartImageProcessor is an image processor that uses smart cropping.
type SmartImageProcessor struct {
	os              OS
	aspectThreshold float64                // Image size comparison threshold
	resampler       imaging.ResampleFilter //moved to struct level
	pigo            *pigo.Pigo
	config          *Config
	// Diagnostics
	lastStats FaceDetectionStats
}

// FaceDetectionStats holds diagnostic data about the last face detection run.
type FaceDetectionStats struct {
	Found      bool
	Q          float32
	Rect       image.Rectangle
	Scale      int
	Processing time.Duration
}

// GetLastStats returns the diagnostics from the last fit operation.
func (c *SmartImageProcessor) GetLastStats() FaceDetectionStats {
	return c.lastStats
}

// NewSmartImageProcessor creates a new processor instance.
func NewSmartImageProcessor(os OS, config *Config, pigo *pigo.Pigo) *SmartImageProcessor {
	return &SmartImageProcessor{
		os:              os,
		config:          config,
		pigo:            pigo,
		aspectThreshold: 0.9,
		resampler:       imaging.Lanczos,
	}
}

// DecodeImage decodes an image from a byte slice with context awareness.
func (c *SmartImageProcessor) DecodeImage(ctx context.Context, imgBytes []byte, contentType string) (image.Image, string, error) {
	var img image.Image
	var err error
	var ext string

	if err := checkContext(ctx); err != nil { //keep the checkContext helper
		return nil, "", err
	}

	switch contentType {
	case "image/png":
		img, err = png.Decode(bytes.NewReader(imgBytes))
		ext = "png"
	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(imgBytes))
		ext = "jpg"
	default:
		img, ext, err = image.Decode(bytes.NewReader(imgBytes))
	}
	if err != nil {
		return nil, ext, fmt.Errorf("decoding image: %w", err)
	}

	if err := checkContext(ctx); err != nil { // Keep context checks
		return nil, "", err
	}
	return img, ext, nil
}

// EncodeImage encodes an image to a byte slice with context awareness.
func (c *SmartImageProcessor) EncodeImage(ctx context.Context, img image.Image, contentType string) ([]byte, error) {
	var buf bytes.Buffer
	var err error

	if err := checkContext(ctx); err != nil { // Keep context checks
		return nil, err
	}

	switch contentType {
	case "image/png":
		err = png.Encode(&buf, img)
	case "image/jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})
	default:
		return nil, fmt.Errorf("unsupported format: %s", contentType)
	}

	if err != nil {
		return nil, fmt.Errorf("encoding image: %w", err)
	}

	if err := checkContext(ctx); err != nil { // Keep context checks
		return nil, err
	}

	return buf.Bytes(), nil
}

// CheckCompatibility checks if an image of given dimensions is compatible with Smart Fit settings.
// CheckCompatibility checks if an image of given dimensions is compatible with Smart Fit settings.
func (c *SmartImageProcessor) CheckCompatibility(width, height int) error {
	mode := c.config.GetSmartFitMode()

	if mode == SmartFitOff {
		return nil
	}

	systemWidth, systemHeight, err := c.os.GetDesktopDimension()
	if err != nil {
		return fmt.Errorf("getting desktop dimensions: %w", err)
	}

	// 1. Strict Resolution Floor
	// User Requirement: Both width and height must be >= desktop resolution.
	if width < systemWidth || height < systemHeight {
		log.Debugf("SmartFit: Image too small (%dx%d vs %dx%d)", width, height, systemWidth, systemHeight)
		return fmt.Errorf("image resolution too low (must be at least desktop size)")
	}

	// 2. Aspect Ratio Tolerance
	// "Quality" Mode:
	// Normally we reject if Diff > 0.9.
	// HOWEVER, we now support "Face Rescue" (Accepting > 0.9 IF Face Q > 20).
	// Since usage in downloader.go cannot see faces/pixels, we MUST accept here to allow the download proceed.
	// We will enforce the strict rejection in FitImage if no face is found.
	if mode == SmartFitNormal {
		return nil
	}

	// "Flexibility" Mode (SmartFitAggressive):
	// Relaxed adherence based on resolution.
	imageAspect := float64(width) / float64(height)
	systemAspect := float64(systemWidth) / float64(systemHeight)
	aspectDiff := math.Abs(systemAspect - imageAspect)

	if mode == SmartFitAggressive {
		// Calculate how much "surplus" resolution we have relative to the screen.
		scaleX := float64(width) / float64(systemWidth)
		scaleY := float64(height) / float64(systemHeight)
		surplus := math.Min(scaleX, scaleY)

		// Dynamic Formula: Base * Surplus * AggressiveMultiplier (1.9)
		effectiveThreshold := c.aspectThreshold * surplus * 1.9
		log.Debugf("SmartFit [Flexibility]: Check (Surplus: %.2f, DynamicThreshold: %.2f, Diff: %.2f)", surplus, effectiveThreshold, aspectDiff)

		if aspectDiff > effectiveThreshold {
			return fmt.Errorf("image aspect ratio not compatible (Diff: %.2f > Limit: %.2f)", aspectDiff, effectiveThreshold)
		}
	}

	return nil
}

// FitImage fits an image with context awareness.
func (c *SmartImageProcessor) FitImage(ctx context.Context, img image.Image) (image.Image, error) {
	if c.config.GetSmartFitMode() == SmartFitOff {
		c.lastStats = FaceDetectionStats{}
		return img, nil
	}

	systemWidth, systemHeight, err := c.os.GetDesktopDimension() // No context here (TEMP)
	if err != nil {
		return nil, fmt.Errorf("getting desktop dimensions: %w", err)
	}

	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	imageWidth := img.Bounds().Dx()
	imageHeight := img.Bounds().Dy()
	systemAspect := float64(systemWidth) / float64(systemHeight)
	imageAspect := float64(imageWidth) / float64(imageHeight)
	aspectDiff := math.Abs(systemAspect - imageAspect)

	r := &resizer{resampler: c.resampler}

	// Pre-check basic compatibility (Resolution mainly)
	if err := c.CheckCompatibility(imageWidth, imageHeight); err != nil {
		log.Debugf("FitImage: %v", err)
		return nil, err
	}

	// Perfect fits
	if imageWidth == systemWidth && imageHeight == systemHeight {
		return img, nil
	}
	if imageAspect == systemAspect {
		resizedImg := r.resizeWithContext(ctx, img, uint(systemWidth), uint(systemHeight)) //nolint:gosec
		if resizedImg == nil {
			return nil, ctx.Err()
		}
		return resizedImg, nil
	}

	// --- CROP LOGIC START ---
	c.lastStats = FaceDetectionStats{}
	start := time.Now()
	defer func() {
		c.lastStats.Processing = time.Since(start)
	}()

	// 1. Face Detection Strategy
	// We run this early because Quality Mode relies on it for the "Rescue" decision.
	var faceBox image.Rectangle
	faceFound := false
	var faceQ float32

	if (c.config.GetFaceCropEnabled() || c.config.GetFaceBoostEnabled()) && c.pigo != nil {
		// Variable to hold the image used for analysis
		// (We use original img here, unlike prev code which shadowed it? prev code used imgForAnalysis = img)
		fb, err := c.findBestFace(img)
		if err == nil {
			faceFound = true
			faceBox = fb
			faceQ = c.lastStats.Q
			c.lastStats.Found = true
			c.lastStats.Rect = faceBox
			log.Debugf("Face Logic: Found face (Q:%.1f)", faceQ)
		} else {
			log.Debugf("Face Logic: No face found.")
		}
	} else if c.pigo == nil {
		log.Debugf("Face Logic: Skipped (Pigo model not loaded or disabled).")
	}

	// 2. Logic Branching: Quality vs Flexibility

	// MODE: QUALITY (SmartFitNormal)
	if c.config.GetSmartFitMode() == SmartFitNormal {
		// Strict Aspect Check (0.9)
		if aspectDiff > c.aspectThreshold {
			// RETRY: Check for "Rescue" (Strong Face)
			if faceFound && faceQ > 20.0 {
				log.Debugf("SmartFit [Quality]: EXCEPTION! Image preserved despite Aspect Diff %.2f (> %.2f) due to Strong Face (Q=%.1f)", aspectDiff, c.aspectThreshold, faceQ)
				// Proceed to use the face!
			} else {
				// REJECT
				return nil, fmt.Errorf("quality mode rejected: aspect diff %.2f > %.2f and no strong face (Q>20) to rescue", aspectDiff, c.aspectThreshold)
			}
		} else {
			log.Debugf("SmartFit [Quality]: Accepted (Diff %.2f <= %.2f)", aspectDiff, c.aspectThreshold)
		}
	}

	// MODE: FLEXIBILITY (SmartFitAggressive)
	if c.config.GetSmartFitMode() == SmartFitAggressive {
		// Validation was already done in CheckCompatibility (Dynamic Threshold).
		// Here we just handle the "Feet Crop" Safety Fallback.
		// If No Face is found, and image is "Unsafe" (Diff > 0.4), use Center.
		if !faceFound {
			safeThreshold := 0.4 // Tuned down from 0.5
			if aspectDiff > safeThreshold {
				log.Debugf("SmartFit [Flexibility]: Fallback to Center. Diff %.2f > %.2f (Unsafe for Entropy)", aspectDiff, safeThreshold)
				center := image.Point{X: img.Bounds().Dx() / 2, Y: img.Bounds().Dy() / 2}
				return c.smartPanAndResize(ctx, img, center, systemWidth, systemHeight)
			}
		}
	}

	// 3. Execution (Face or Smart)

	// If Face Found (and we are still here), use it.
	if faceFound {
		center := image.Point{X: faceBox.Min.X + faceBox.Dx()/2, Y: faceBox.Min.Y + faceBox.Dy()/2}

		// Priority: Face Crop (Hard)
		if c.config.GetFaceCropEnabled() {
			cropRect := c.cropAroundFace(img.Bounds(), faceBox, systemWidth, systemHeight)
			type SubImager interface {
				SubImage(r image.Rectangle) image.Image
			}
			img = img.(SubImager).SubImage(cropRect)
			resizedImg := r.resizeWithContext(ctx, img, uint(systemWidth), uint(systemHeight)) //nolint:gosec
			if resizedImg == nil {
				return nil, ctx.Err()
			}
			return resizedImg, nil
		}

		// Priority: Face Boost / Smart Pan
		return c.smartPanAndResize(ctx, img, center, systemWidth, systemHeight)
	}

	// 4. Fallback to SmartCrop (Entropy)
	// If we are here:
	// - Quality Mode: Diff was <= 0.9 (Safe-ish) OR Rescued (but FaceCrop disabled? Unlikely logic path if Face found but FaceCrop off... wait.
	//   If Quality Mode + Rescue (FaceFound) + FaceCrop OFF -> We fall here?
	//   CHECK ABOVE: "If Face Found... use it".
	//   So if we were rescued, FaceFound is true, so we used `smartPanAndResize` with face center. Correct.

	// - Flexibility Mode: Diff was <= 0.4 (Very Safe).
	log.Debugf("SmartFit: Using Entropy Crop.")
	analyzer := smartcrop.NewAnalyzer(r)

	type cropResult struct {
		crop image.Rectangle
		err  error
	}
	resultChan := make(chan cropResult)

	go func() {
		topCrop, err := analyzer.FindBestCrop(img, systemWidth, systemHeight)
		resultChan <- cropResult{crop: topCrop, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChan:
		if result.err != nil {
			return nil, fmt.Errorf("finding best crop: %w", result.err)
		}
		smartCenter := result.crop.Min.Add(result.crop.Size().Div(2))
		return c.smartPanAndResize(ctx, img, smartCenter, systemWidth, systemHeight)
	}
}

// cropAroundFace calculates the crop rectangle centered on the face.
func (c *SmartImageProcessor) cropAroundFace(imgBounds image.Rectangle, faceBox image.Rectangle, targetWidth, targetHeight int) image.Rectangle {
	faceCenter := faceBox.Min.Add(faceBox.Size().Div(2))

	targetAspect := float64(targetWidth) / float64(targetHeight)

	// Calculate crop size based on image bounds and target aspect
	var cropWidth, cropHeight int

	if float64(imgBounds.Dx())/float64(imgBounds.Dy()) > targetAspect {
		// Image is wider than target
		cropHeight = imgBounds.Dy()
		cropWidth = int(float64(cropHeight) * targetAspect)
	} else {
		// Image is taller than target
		cropWidth = imgBounds.Dx()
		cropHeight = int(float64(cropWidth) / targetAspect)
	}

	// Center crop on face
	minX := faceCenter.X - cropWidth/2
	minY := faceCenter.Y - cropHeight/2
	maxX := minX + cropWidth
	maxY := minY + cropHeight

	// Adjust to stay within bounds
	if minX < imgBounds.Min.X {
		diff := imgBounds.Min.X - minX
		minX += diff
		maxX += diff
	}
	if minY < imgBounds.Min.Y {
		diff := imgBounds.Min.Y - minY
		minY += diff
		maxY += diff
	}
	if maxX > imgBounds.Max.X {
		diff := maxX - imgBounds.Max.X
		minX -= diff
		maxX -= diff
	}
	if maxY > imgBounds.Max.Y {
		diff := maxY - imgBounds.Max.Y
		minY -= diff
		maxY -= diff
	}

	return image.Rect(minX, minY, maxX, maxY)
}

// resizer implements the smartcrop.Resizer interface and adds context awareness.
type resizer struct {
	resampler imaging.ResampleFilter
}

// Resize *doesn't* take a context here.  The smartcrop.Resizer interface doesn't
// support contexts.  We handle cancellation in ResizeWithContext.
func (r *resizer) Resize(img image.Image, width, height uint) image.Image {
	//nolint:gosec // G115: integer overflow conversion (uint -> int). Images > 2B pixels unlikely.
	return imaging.Resize(img, int(width), int(height), r.resampler)
}

// resizeWithContext performs the resize operation with context awareness.
func (r *resizer) resizeWithContext(ctx context.Context, img image.Image, width, height uint) image.Image {
	resultChan := make(chan image.Image)

	go func() {
		//nolint:gosec // G115: integer overflow conversion (uint -> int). Images > 2B pixels unlikely.
		resultChan <- imaging.Resize(img, int(width), int(height), r.resampler)
	}()

	select {
	case <-ctx.Done():
		return nil // Return nil if context is canceled.
	case result := <-resultChan:
		return result
	}
}

func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// findBestFace runs pigo and returns the largest, most confident face.
func (c *SmartImageProcessor) findBestFace(img image.Image) (image.Rectangle, error) {
	// pigo needs grayscale image data.
	// We convert to NRGBA first to ensure consistent pixel access across different image formats (YCbCr, etc.)
	nrgba := imaging.Clone(img)
	pixels := pigo.RgbToGrayscale(nrgba)

	// We also need the image dimensions.
	// We also need the image dimensions.
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Run the detection
	// Determine dynamic MaxSize based on image dimensions
	minDimension := width
	if height < width {
		minDimension = height
	}

	params := pigo.CascadeParams{
		MinSize:     int(float64(minDimension) * (float64(c.config.GetFaceDetectMinSizePct()) / 100.0)), // Configurable min size
		MaxSize:     minDimension,                                                                       // Allow faces up to the full image size
		ShiftFactor: c.config.GetFaceDetectShiftFactor(),
		ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{
			Pixels: pixels,
			Rows:   height,
			Cols:   width,
			Dim:    width,
		},
	}

	dets := c.pigo.RunCascade(params, 0.0)

	// Now, cluster the results
	dets = c.pigo.ClusterDetections(dets, 0.2) // 0.2 is the IoU threshold

	var bestDet pigo.Detection
	var maxScore float32 = -1.0
	found := false

	bottomEdgeThreshold := int(float64(height) * 0.9)

	confThreshold := float32(c.config.GetFaceDetectConfidence())

	for _, det := range dets {
		// 1. Confidence Floor: Filter out clear noise (phantom faces/shadows)
		if det.Q < confThreshold {
			continue
		}

		// 2. Edge Safety: Discard low-confidence detections in the bottom 10% of the frame.
		// Real subject faces are rarely at the literal bottom edge of a high-quality wallpaper.
		if det.Row > bottomEdgeThreshold && det.Q < 20.0 {
			log.Debugf("Face Logic: Discarded bottom-edge detection with low confidence (Q: %.2f)", det.Q)
			continue
		}

		// 3. Confidence-Weighted Selection (Q * Scale):
		// This ensures a high-confidence mid-sized face wins over a low-confidence large blob.
		score := det.Q * float32(det.Scale)
		if score > maxScore {
			maxScore = score
			bestDet = det
			found = true
		}
	}

	if !found {
		return image.Rectangle{}, fmt.Errorf("no face found")
	}

	// Determine boost parameters based on strength (0, 1, 2)
	// Default is 0 (Standard)
	strength := c.config.GetFaceBoostStrength()

	scaleFactor := 1.5 // Default (Strength 0)
	if strength == 1 {
		scaleFactor = 2.0
	} else if strength >= 2 {
		scaleFactor = 2.5
	}

	// Expand the box by ensuring we cover the whole face (forehead, chin, etc.)
	// pigo often detects just the "core" (eyes/nose/mouth)
	expandedScale := int(float64(bestDet.Scale) * scaleFactor)

	// Convert pigo's detection (col, row, scale) to a standard image.Rectangle
	faceBox := image.Rect(
		bestDet.Col-expandedScale/2,
		bestDet.Row-expandedScale/2,
		bestDet.Col+expandedScale/2,
		bestDet.Row+expandedScale/2,
	)

	c.lastStats.Q = bestDet.Q
	c.lastStats.Scale = bestDet.Scale

	// Clamp to image bounds
	faceBox = faceBox.Intersect(img.Bounds())

	return faceBox, nil
}

// smartPanAndResize crops the image to the maximum size that fits the target aspect ratio,
// centered on the given point, and then resizes it to the target dimensions.
func (c *SmartImageProcessor) smartPanAndResize(ctx context.Context, img image.Image, center image.Point, targetWidth, targetHeight int) (image.Image, error) {
	// 1. Calculate the maximum possible crop size for the target aspect ratio
	imgBounds := img.Bounds()
	targetAspect := float64(targetWidth) / float64(targetHeight)
	var cropWidth, cropHeight int

	if float64(imgBounds.Dx())/float64(imgBounds.Dy()) > targetAspect {
		// Image is wider than target: Height is the limiting factor
		cropHeight = imgBounds.Dy()
		cropWidth = int(float64(cropHeight) * targetAspect)
	} else {
		// Image is taller than target: Width is the limiting factor
		cropWidth = imgBounds.Dx()
		cropHeight = int(float64(cropWidth) / targetAspect)
	}

	// 2. Center this max crop on the smart center
	minX := center.X - cropWidth/2
	minY := center.Y - cropHeight/2
	maxX := minX + cropWidth
	maxY := minY + cropHeight

	// 3. Adjust to stay within bounds (clamp)
	if minX < imgBounds.Min.X {
		diff := imgBounds.Min.X - minX
		minX += diff
		maxX += diff
	}
	if minY < imgBounds.Min.Y {
		diff := imgBounds.Min.Y - minY
		minY += diff
		maxY += diff
	}
	if maxX > imgBounds.Max.X {
		diff := maxX - imgBounds.Max.X
		minX -= diff
		maxX -= diff
	}
	if maxY > imgBounds.Max.Y {
		diff := maxY - imgBounds.Max.Y
		minY -= diff
		maxY -= diff
	}

	finalCrop := image.Rect(minX, minY, maxX, maxY)
	log.Debugf("Smart Pan: ImgBounds %v, TargetAspect %.2f, SmartCenter %v", imgBounds, targetAspect, center)
	log.Debugf("Smart Pan: Calculated Max Crop %dx%d", cropWidth, cropHeight)
	log.Debugf("Smart Pan: Final Crop %v (Size: %dx%d)", finalCrop, finalCrop.Dx(), finalCrop.Dy())

	// Crop and resize the image.
	type SubImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	img = img.(SubImager).SubImage(finalCrop)

	r := &resizer{resampler: c.resampler}
	// Use the context-aware resize.
	//nolint:gosec // G115: integer overflow conversion (uint -> int). Images > 2B pixels unlikely.
	resizedImg := r.resizeWithContext(ctx, img, uint(targetWidth), uint(targetHeight))

	if resizedImg == nil {
		return nil, ctx.Err() // Context was canceled during resize.
	}

	return resizedImg, nil
}
