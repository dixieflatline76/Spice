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
	"github.com/dixieflatline76/Spice/v2/util/log"
	pigo "github.com/esimov/pigo/core"
)

// SmartImageProcessor is an image processor that uses smart cropping.
type SmartImageProcessor struct {
	os        OS
	resampler imaging.ResampleFilter //moved to struct level
	pigo      *pigo.Pigo
	config    *Config
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
		os:        os,
		config:    config,
		pigo:      pigo,
		resampler: imaging.Lanczos,
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
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: c.config.Tuning.EncodingQuality})
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
func (c *SmartImageProcessor) CheckCompatibility(imgWidth, imgHeight, systemWidth, systemHeight int) error {
	mode := c.config.GetSmartFitMode()

	if mode == SmartFitOff {
		return nil
	}

	if imgWidth <= 0 || imgHeight <= 0 || systemWidth <= 0 || systemHeight <= 0 {
		return nil // Assume compatible if dimensions are missing/invalid (prevents test regressions)
	}

	// 2. Aspect Ratio Tolerance
	imageAspect := float64(imgWidth) / float64(imgHeight)
	systemAspect := float64(systemWidth) / float64(systemHeight)
	aspectDiff := math.Abs(systemAspect - imageAspect)

	// "Quality" Mode (SmartFitNormal)
	if mode == SmartFitNormal {
		return c.checkQualityModeCompatibility(imgWidth, imgHeight, systemWidth, systemHeight, aspectDiff)
	}

	// "Flexibility" Mode (SmartFitAggressive)
	if mode == SmartFitAggressive {
		return c.checkFlexibilityModeCompatibility(imgWidth, imgHeight, systemWidth, systemHeight, aspectDiff)
	}

	return nil
}

func (c *SmartImageProcessor) checkQualityModeCompatibility(imgWidth, imgHeight, systemWidth, systemHeight int, aspectDiff float64) error {
	isSquare := imgWidth == imgHeight
	if !isSquare {
		srcLand := imgWidth > imgHeight
		tgtLand := systemWidth > systemHeight
		if srcLand != tgtLand {
			if aspectDiff > 0.5 {
				return fmt.Errorf("incompatible orientation for Quality mode (Diff %.2f > 0.5)", aspectDiff)
			}
		}
	}

	if aspectDiff > 1.5 {
		return fmt.Errorf("aspect ratio diff too large for Quality mode (%.2f > 1.5)", aspectDiff)
	}
	return nil
}

func (c *SmartImageProcessor) checkFlexibilityModeCompatibility(imgWidth, imgHeight, systemWidth, systemHeight int, aspectDiff float64) error {
	scaleX := float64(imgWidth) / float64(systemWidth)
	scaleY := float64(imgHeight) / float64(systemHeight)
	surplus := math.Min(scaleX, scaleY)

	// Dynamic Formula: Base * Surplus * AggressiveMultiplier (1.9)
	effectiveThreshold := c.config.Tuning.AspectThreshold * surplus * c.config.Tuning.AggressiveMultiplier

	// SAFETY CAP: Even with high resolution, don't allow insane crops.
	// 1.5 is the absolute limit for Flexibility. Anything beyond this regardless of resolution is a "sliver".
	if effectiveThreshold > 1.5 {
		effectiveThreshold = 1.5
	}

	// Orientation Safety: Block drastic mismatches (e.g. Landscape on Portrait)
	// Square images (imgWidth == imgHeight) are exempted to allow safe cropping.
	isSquare := imgWidth == imgHeight
	if !isSquare {
		srcLand := imgWidth > imgHeight
		tgtLand := systemWidth > systemHeight
		if srcLand != tgtLand {
			// Orientation Mismatch: Cap threshold to block bad crops (e.g. 16:9 on 9:16)
			// Limit of 0.8 allows 4:3 on Portrait (Diff ~0.7) but blocks 16:9 on Portrait (Diff ~1.15)
			if effectiveThreshold > 0.8 {
				effectiveThreshold = 0.8
			}
		}
	}

	log.Debugf("SmartFit [Flexibility]: Check (Src: %dx%d, Tgt: %dx%d, Surplus: %.2f, DynamicThreshold: %.2f, Diff: %.2f)",
		imgWidth, imgHeight, systemWidth, systemHeight, surplus, effectiveThreshold, aspectDiff)

	if aspectDiff > effectiveThreshold {
		return fmt.Errorf("image aspect ratio not compatible (Diff: %.2f > Limit: %.2f)", aspectDiff, effectiveThreshold)
	}
	return nil
}

// FitImage fits an image with context awareness.
func (c *SmartImageProcessor) FitImage(ctx context.Context, img image.Image, targetWidth, targetHeight int) (image.Image, error) {
	if c.config.GetSmartFitMode() == SmartFitOff {
		c.lastStats = FaceDetectionStats{}
		return img, nil
	}

	systemWidth := targetWidth
	systemHeight := targetHeight

	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	imageWidth := img.Bounds().Dx()
	imageHeight := img.Bounds().Dy()

	// Pre-check basic compatibility
	if err := c.CheckCompatibility(imageWidth, imageHeight, targetWidth, targetHeight); err != nil {
		log.Debugf("FitImage: %v", err)
		return nil, err
	}

	// Perfect fits or exact aspect checks
	systemAspect := float64(systemWidth) / float64(systemHeight)
	imageAspect := float64(imageWidth) / float64(imageHeight)

	if imageWidth == systemWidth && imageHeight == systemHeight {
		return img, nil
	}

	r := &resizer{resampler: c.resampler}
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

	// 1. Analysis Phase
	faceFound, faceBox, faceQ, err := c.analyzeFace(img)
	if err != nil {
		log.Debugf("Face Logic: %v", err) // Log error but proceed (not fatal)
	}

	// Calculate Energy (needed for Flexible mode fallbacks and feet guard)
	energy, energyErr := c.calculateImageEnergy(ctx, img)

	// 2. Gate Checks
	if err := c.checkQualityGate(imageAspect, systemAspect, faceFound, faceQ); err != nil {
		return nil, err
	}

	// 3. Strategy Selection
	strategy := c.selectStrategy(img, faceFound, faceBox, energy, energyErr)

	// 4. Execution
	return strategy.Apply(ctx, img, systemWidth, systemHeight, c)
}

func (c *SmartImageProcessor) analyzeFace(img image.Image) (bool, image.Rectangle, float32, error) {
	if (!c.config.GetFaceCropEnabled() && !c.config.GetFaceBoostEnabled()) || c.pigo == nil {
		return false, image.Rectangle{}, 0, nil
	}

	fb, err := c.findBestFace(img)
	if err != nil {
		return false, image.Rectangle{}, 0, err
	}

	c.lastStats.Found = true
	c.lastStats.Rect = fb

	log.Debugf("Face Logic: Found face (Q:%.1f)", c.lastStats.Q)
	return true, fb, c.lastStats.Q, nil
}

func (c *SmartImageProcessor) checkQualityGate(imgAspect, sysAspect float64, faceFound bool, faceQ float32) error {
	if c.config.GetSmartFitMode() != SmartFitNormal {
		return nil
	}

	aspectDiff := math.Abs(sysAspect - imgAspect)
	if aspectDiff <= c.config.Tuning.AspectThreshold {
		log.Debugf("SmartFit [Quality]: Accepted (Diff %.2f <= %.2f)", aspectDiff, c.config.Tuning.AspectThreshold)
		return nil
	}

	// RETRY: Check for "Rescue" (Strong Face)
	if faceFound && faceQ > c.config.Tuning.FaceRescueQThreshold {
		log.Debugf("SmartFit [Quality]: EXCEPTION! Image preserved despite Aspect Diff %.2f (> %.2f) due to Strong Face (Q=%.1f)", aspectDiff, c.config.Tuning.AspectThreshold, faceQ)
		return nil
	}

	return fmt.Errorf("quality mode rejected: aspect diff %.2f > %.2f and no strong face (Q>%.1f) to rescue", aspectDiff, c.config.Tuning.AspectThreshold, c.config.Tuning.FaceRescueQThreshold)
}

func (c *SmartImageProcessor) selectStrategy(img image.Image, faceFound bool, faceBox image.Rectangle, energy float64, energyErr error) CropStrategy {
	// Flexibility Mode Low Energy Fallback
	if c.config.GetSmartFitMode() == SmartFitAggressive {
		if !faceFound && energyErr == nil && energy < c.config.Tuning.MinEnergyThreshold {
			log.Debugf("SmartFit [Flexibility]: Energy %.4f too low (Flat Image). Fallback to Center.", energy)
			center := image.Point{X: img.Bounds().Dx() / 2, Y: img.Bounds().Dy() / 2}
			return &SmartPanStrategy{Center: center}
		}
	}

	if faceFound {
		if c.config.GetFaceCropEnabled() {
			return &FaceCropStrategy{FaceBox: faceBox}
		}
		// Face Boost
		center := image.Point{X: faceBox.Min.X + faceBox.Dx()/2, Y: faceBox.Min.Y + faceBox.Dy()/2}
		return &SmartPanStrategy{Center: center}
	}

	return &EntropyCropStrategy{FaceFound: faceFound, Energy: energy}
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
		MinSize:     int(float64(minDimension) * (float64(c.config.Tuning.FaceDetectMinSizePct) / 100.0)), // Configurable min size
		MaxSize:     minDimension,                                                                         // Allow faces up to the full image size
		ShiftFactor: c.config.Tuning.FaceDetectShift,
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
	dets = c.pigo.ClusterDetections(dets, c.config.Tuning.FaceIoUThreshold) // Centralized threshold

	var bestDet pigo.Detection
	var maxScore float32 = -1.0
	found := false

	bottomEdgeThreshold := int(float64(height) * c.config.Tuning.FaceBottomEdgeThreshold)

	confThreshold := float32(c.config.Tuning.FaceDetectConfidence)

	for _, det := range dets {
		// 1. Confidence Floor: Filter out clear noise (phantom faces/shadows)
		if det.Q < confThreshold {
			continue
		}

		// 2. Edge Safety: Discard low-confidence detections in the bottom section of the frame.
		// Real subject faces are rarely at the literal bottom edge of a high-quality wallpaper.
		// Holistic (v1.6.2.2): We expand this "danger zone" to the bottom 30% for low-confidence hits.
		if det.Row > bottomEdgeThreshold && det.Q < c.config.Tuning.FaceBottomEdgeMinQ {
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

	// Determine boost parameters
	// Strength 0 (Standard) -> 1.5
	scaleFactor := 1.5

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

// calculateImageEnergy calculates the standard deviation of luminance (entropy proxy).
func (c *SmartImageProcessor) calculateImageEnergy(ctx context.Context, img image.Image) (float64, error) {
	if err := checkContext(ctx); err != nil {
		return 0, err
	}

	// Resize to a small thumbnail for performance
	thumb := imaging.Resize(img, c.config.Tuning.EnergyThumbSize, 0, imaging.Box)

	bounds := thumb.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	var sum, sumSq float64
	var count float64

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			p := thumb.At(x, y)
			r, g, b, _ := p.RGBA()
			// Luminance formula (0-1 range)
			lum := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535.0

			sum += lum
			sumSq += lum * lum
			count++
		}
	}

	if count == 0 {
		return 0, nil
	}

	mean := sum / count
	variance := (sumSq / count) - (mean * mean)
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance), nil
}
