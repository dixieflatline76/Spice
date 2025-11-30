package wallpaper

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"

	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/util/log"
	pigo "github.com/esimov/pigo/core"
	"github.com/muesli/smartcrop"
)

// smartImageProcessor is an image processor that uses smart cropping.
type smartImageProcessor struct {
	os              OS
	aspectThreshold float64                // Image size comparison threshold
	resampler       imaging.ResampleFilter //moved to struct level
	pigo            *pigo.Pigo
	config          *Config
}

// DecodeImage decodes an image from a byte slice with context awareness.
func (c *smartImageProcessor) DecodeImage(ctx context.Context, imgBytes []byte, contentType string) (image.Image, string, error) {
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
func (c *smartImageProcessor) EncodeImage(ctx context.Context, img image.Image, contentType string) ([]byte, error) {
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

// FitImage fits an image with context awareness.
func (c *smartImageProcessor) FitImage(ctx context.Context, img image.Image) (image.Image, error) {
	systemWidth, systemHeight, err := c.os.getDesktopDimension() // No context here (TEMP)
	if err != nil {
		return nil, fmt.Errorf("getting desktop dimensions: %w", err)
	}

	// Keep the context check, even though getDesktopDimension doesn't use it yet.
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	imageWidth := img.Bounds().Dx()
	imageHeight := img.Bounds().Dy()
	systemAspect := float64(systemWidth) / float64(systemHeight)
	imageAspect := float64(imageWidth) / float64(imageHeight)
	aspectDiff := math.Abs(systemAspect - imageAspect)

	r := &resizer{resampler: c.resampler} // Create resizer here

	switch {
	case imageWidth < systemWidth || imageHeight < systemHeight || aspectDiff > c.aspectThreshold:
		log.Debugf("FitImage: Image not compatible (too small or aspect diff too large)")
		return nil, fmt.Errorf("image not compatible with smart fit")
	case imageWidth == systemWidth && imageHeight == systemHeight: // Perfect fit
		log.Debugf("FitImage: Perfect fit, returning original")
		return img, nil
	case imageAspect == systemAspect: // Perfect aspect ratio
		log.Debugf("FitImage: Perfect aspect ratio, resizing only")
		resizedImg := r.resizeWithContext(ctx, img, uint(systemWidth), uint(systemHeight)) // Correct call
		if resizedImg == nil {
			return nil, ctx.Err() // Context was canceled during resize.
		}
		return resizedImg, nil
	default:
		log.Debugf("FitImage: Cropping needed, calling cropImage")
		croppedImg, err := c.cropImage(ctx, img) // Pass context
		if err != nil {
			return nil, fmt.Errorf("cropping image: %w", err)
		}
		return croppedImg, nil
	}
}

// cropImage crops an image with context awareness.
func (c *smartImageProcessor) cropImage(ctx context.Context, img image.Image) (image.Image, error) {
	systemWidth, systemHeight, err := c.os.getDesktopDimension() // No context here (TEMP)
	if err != nil {
		return nil, fmt.Errorf("getting desktop dimensions: %w", err)
	}

	// Keep the context check, even though getDesktopDimension doesn't use it yet.
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	r := &resizer{resampler: c.resampler} // Create resizer here

	// Check for face boost first

	if c.config.GetFaceBoostEnabled() && c.pigo != nil {
		faceBox, err := c.findBestFace(img)
		if err == nil {
			// Face found, crop around it
			log.Debugf("Face Boost: Face found at %v. Cropping...", faceBox)
			cropRect := c.cropAroundFace(img.Bounds(), faceBox, systemWidth, systemHeight)
			log.Debugf("Face Boost: Cropping to %v (Size: %dx%d)", cropRect, cropRect.Dx(), cropRect.Dy())

			// Crop and resize
			type SubImager interface {
				SubImage(r image.Rectangle) image.Image
			}
			img = img.(SubImager).SubImage(cropRect)

			resizedImg := r.resizeWithContext(ctx, img, uint(systemWidth), uint(systemHeight))
			if resizedImg == nil {
				return nil, ctx.Err()
			}
			return resizedImg, nil
		} else {
			log.Debugf("Face Boost: No face found (%v). Falling back to smartcrop.", err)
		}
	} else if c.config.GetFaceBoostEnabled() && c.pigo == nil {
		log.Debugf("Face Boost: Enabled but pigo model not loaded.")
	}

	// Fallback to smartcrop
	analyzer := smartcrop.NewAnalyzer(r)

	// Use a goroutine and channel to make FindBestCrop context-aware.
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
		return nil, ctx.Err() // Context was canceled.
	case result := <-resultChan:
		if result.err != nil {
			return nil, fmt.Errorf("finding best crop: %w", result.err)
		}

		// Crop and resize the image.
		type SubImager interface {
			SubImage(r image.Rectangle) image.Image
		}
		img = img.(SubImager).SubImage(result.crop)

		// Use the context-aware resize.
		resizedImg := r.resizeWithContext(ctx, img, uint(systemWidth), uint(systemHeight))

		if resizedImg == nil {
			return nil, ctx.Err() // Context was canceled during resize.
		}

		return resizedImg, nil
	}
}

// cropAroundFace calculates the crop rectangle centered on the face.
func (c *smartImageProcessor) cropAroundFace(imgBounds image.Rectangle, faceBox image.Rectangle, targetWidth, targetHeight int) image.Rectangle {
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
	return imaging.Resize(img, int(width), int(height), r.resampler)
}

// resizeWithContext performs the resize operation with context awareness.
func (r *resizer) resizeWithContext(ctx context.Context, img image.Image, width, height uint) image.Image {
	resultChan := make(chan image.Image)

	go func() {
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
func (c *smartImageProcessor) findBestFace(img image.Image) (image.Rectangle, error) {
	// pigo needs grayscale image data.
	// pigo needs grayscale image data (1 byte per pixel).
	pixels := pigo.RgbToGrayscale(img)

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
		MinSize:     20,           // Lower minimum size to catch smaller faces
		MaxSize:     minDimension, // Allow faces up to the full image size
		ShiftFactor: 0.1,
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
	found := false
	for _, det := range dets {
		// We're looking for the best *quality* detection.
		// You could also use `det.Scale` to find the *largest* face.

		if det.Q > bestDet.Q && det.Q > 5.0 { // 5.0 is a good minimum threshold
			bestDet = det
			found = true
		}
	}

	if !found {
		return image.Rectangle{}, fmt.Errorf("no face found")
	}

	// Convert pigo's detection (col, row, scale) to a standard image.Rectangle
	faceBox := image.Rect(
		bestDet.Col-bestDet.Scale/2,
		bestDet.Row-bestDet.Scale/2,
		bestDet.Col+bestDet.Scale/2,
		bestDet.Row+bestDet.Scale/2,
	)

	return faceBox, nil
}
