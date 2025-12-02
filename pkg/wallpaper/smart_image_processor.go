package wallpaper

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"math/rand"

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

	// Variable to hold the image used for analysis (potentially boosted)
	var imgForAnalysis image.Image

	// Check for face crop/boost
	if (c.config.GetFaceCropEnabled() || c.config.GetFaceBoostEnabled()) && c.pigo != nil {
		faceBox, err := c.findBestFace(img)
		if err == nil {
			// Priority 1: Face Crop (Hard Crop)
			if c.config.GetFaceCropEnabled() {
				log.Debugf("Face Crop: Face found at %v. Cropping...", faceBox)
				cropRect := c.cropAroundFace(img.Bounds(), faceBox, systemWidth, systemHeight)
				log.Debugf("Face Crop: Cropping to %v (Size: %dx%d)", cropRect, cropRect.Dx(), cropRect.Dy())

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
			}

			// Priority 2: Face Boost (Hinting)
			if c.config.GetFaceBoostEnabled() {
				log.Debugf("Face Boost: Face found at %v. Applying boost hint...", faceBox)

				// Create a copy of the image for analysis
				bounds := img.Bounds()
				analysisImg := image.NewRGBA(bounds)

				// Draw original image onto the analysis image
				draw.Draw(analysisImg, bounds, img, bounds.Min, draw.Src)

				// Draw random noise over the face to boost edge detection energy
				// We use "Medium" noise (3px blocks) with RGB randomization to maximize variance.
				blockSize := 3
				for y := faceBox.Min.Y; y < faceBox.Max.Y; y += blockSize {
					for x := faceBox.Min.X; x < faceBox.Max.X; x += blockSize {
						// Generate high-contrast RGB value for the whole block
						r := uint8(0)
						if rand.Intn(2) == 1 {
							r = 255
						}
						g := uint8(0)
						if rand.Intn(2) == 1 {
							g = 255
						}
						b := uint8(0)
						if rand.Intn(2) == 1 {
							b = 255
						}
						c := color.RGBA{r, g, b, 255}

						// Fill the block
						for by := 0; by < blockSize; by++ {
							for bx := 0; bx < blockSize; bx++ {
								if x+bx < faceBox.Max.X && y+by < faceBox.Max.Y {
									analysisImg.Set(x+bx, y+by, c)
								}
							}
						}
					}
				}

				// Use this boosted image for analysis
				imgForAnalysis = analysisImg
			}
		} else {
			log.Debugf("Face Logic: No face found (%v). Falling back to smartcrop.", err)
		}
	} else if (c.config.GetFaceCropEnabled() || c.config.GetFaceBoostEnabled()) && c.pigo == nil {
		log.Debugf("Face Logic: Enabled but pigo model not loaded.")
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
		// Use imgForAnalysis if set, otherwise original img
		targetImg := img
		if imgForAnalysis != nil {
			targetImg = imgForAnalysis
		}
		topCrop, err := analyzer.FindBestCrop(targetImg, systemWidth, systemHeight)
		resultChan <- cropResult{crop: topCrop, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err() // Context was canceled.
	case result := <-resultChan:
		if result.err != nil {
			return nil, fmt.Errorf("finding best crop: %w", result.err)
		}

		// "Smart Pan" Logic:
		// The user wants to avoid "zooming" (cropping to a small area).
		// We want the MAXIMUM size crop that fits the aspect ratio, centered on the "smart" area.

		// 1. Calculate the center of the smart crop
		smartCenter := result.crop.Min.Add(result.crop.Size().Div(2))

		// 2. Calculate the maximum possible crop size for the target aspect ratio
		imgBounds := img.Bounds()
		targetAspect := float64(systemWidth) / float64(systemHeight)
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

		// 3. Center this max crop on the smart center
		minX := smartCenter.X - cropWidth/2
		minY := smartCenter.Y - cropHeight/2
		maxX := minX + cropWidth
		maxY := minY + cropHeight

		// 4. Adjust to stay within bounds (clamp)
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
		log.Debugf("Smart Pan: ImgBounds %v, TargetAspect %.2f, SmartCenter %v", imgBounds, targetAspect, smartCenter)
		log.Debugf("Smart Pan: Calculated Max Crop %dx%d", cropWidth, cropHeight)
		log.Debugf("Smart Pan: Final Crop %v (Size: %dx%d)", finalCrop, finalCrop.Dx(), finalCrop.Dy())

		// Crop and resize the image.
		type SubImager interface {
			SubImage(r image.Rectangle) image.Image
		}
		img = img.(SubImager).SubImage(finalCrop)

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
		MinSize:     int(float64(minDimension) * 0.05), // Dynamic min size: 5% of smallest dimension
		MaxSize:     minDimension,                      // Allow faces up to the full image size
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
		// We filter by a higher quality threshold to avoid false positives (like knees, elbows, or random textures).
		if det.Q > 20.0 {
			// If we found a valid face, we prioritize the LARGEST one (Scale).
			// Wallpapers usually feature the subject prominently.
			if !found || det.Scale > bestDet.Scale {
				bestDet = det
				found = true
			}
		}
	}

	if !found {
		return image.Rectangle{}, fmt.Errorf("no face found")
	}

	// Expand the box by 50% to ensure we cover the whole face (forehead, chin, etc.)
	// pigo often detects just the "core" (eyes/nose/mouth)
	expandedScale := int(float64(bestDet.Scale) * 1.5)

	// Convert pigo's detection (col, row, scale) to a standard image.Rectangle
	faceBox := image.Rect(
		bestDet.Col-expandedScale/2,
		bestDet.Row-expandedScale/2,
		bestDet.Col+expandedScale/2,
		bestDet.Row+expandedScale/2,
	)

	// Clamp to image bounds
	faceBox = faceBox.Intersect(img.Bounds())

	return faceBox, nil
}
