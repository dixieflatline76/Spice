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
	"github.com/muesli/smartcrop"
)

// smartImageProcessor is an image processor that uses smart cropping.
type smartImageProcessor struct {
	os              OS
	aspectThreshold float64                // Image size comparison threshold
	resampler       imaging.ResampleFilter //moved to struct level
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
		return nil, fmt.Errorf("image not compatible with smart fit")
	case imageWidth == systemWidth && imageHeight == systemHeight: // Perfect fit
		return img, nil
	case imageAspect == systemAspect: // Perfect aspect ratio
		resizedImg := r.resizeWithContext(ctx, img, uint(systemWidth), uint(systemHeight)) // Correct call
		if resizedImg == nil {
			return nil, ctx.Err() // Context was canceled during resize.
		}
		return resizedImg, nil
	default:
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
		//resizedImg := c.resizeWithContext(ctx, img, uint(systemWidth), uint(systemHeight))  // INCORRECT
		resizedImg := r.resizeWithContext(ctx, img, uint(systemWidth), uint(systemHeight)) // Correct call

		if resizedImg == nil {
			return nil, ctx.Err() // Context was canceled during resize.
		}

		return resizedImg, nil
	}
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
