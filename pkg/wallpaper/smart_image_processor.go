package wallpaper

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"

	"github.com/dixieflatline76/Spice/util/log"

	"github.com/disintegration/imaging"
	"github.com/muesli/smartcrop"
)

// smartImageProcessor is an image processor that uses smart cropping.
// It uses the OS to get the desktop resolution and uses smart cropping to
// crop the image to the desktop resolution.
type smartImageProcessor struct {
	os              OS
	aspectThreshold float64 // If image is this percentage larger or smaller than the system resolution, it will be scaled
	resampler       imaging.ResampleFilter
}

// DecodeImage decodes an image from a byte slice, detecting the format.
func (c *smartImageProcessor) DecodeImage(imgBytes []byte, contentType string) (image.Image, string, error) {
	var img image.Image
	var err error
	var ext string

	switch contentType {
	case "image/png":
		img, err = png.Decode(bytes.NewReader(imgBytes))
		ext = "png"

	case "image/jpeg":
		img, err = jpeg.Decode(bytes.NewReader(imgBytes))
		ext = "jpg"
	default:
		// Try to detect the image format and decode with best effort
		img, ext, err = image.Decode(bytes.NewReader(imgBytes))
	}
	return img, ext, err
}

// EncodeImage encodes an image to a byte slice in the specified contentType.
func (c *smartImageProcessor) EncodeImage(img image.Image, contentType string) ([]byte, error) {
	var buf bytes.Buffer
	var err error

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

	return buf.Bytes(), nil
}

// FitImage fits an image to the system native resolution using smart cropping.
func (c *smartImageProcessor) FitImage(img image.Image) (image.Image, error) {

	// Get the desktop dimensions
	systemWidth, systemHeight, err := c.os.getDesktopDimension()
	if err != nil {
		log.Printf("failed to get desktop dimensions: %v", err)
		return nil, err
	}
	imageWidth := img.Bounds().Dx()
	imageHeight := img.Bounds().Dy()
	systemAspect := float64(systemWidth) / float64(systemHeight)
	imageAspect := float64(imageWidth) / float64(imageHeight)
	aspectDiff := math.Abs(systemAspect - imageAspect)

	// Check if the image is compatible with smart fit for the current desktop resolution

	switch {
	case imageWidth < systemWidth || imageHeight < systemHeight || aspectDiff > c.aspectThreshold:
		img = nil
		err = fmt.Errorf("image not compatible with smart fit for current desktop resolution")
	case imageWidth == systemWidth && imageHeight == systemHeight:
		// Perfect fit, no scaling needed
	case imageAspect == systemAspect:
		// Perfect aspect ratio match, use standard scaling
		img = imaging.Resize(img, systemWidth, systemHeight, c.resampler)
	default:
		img, err = c.cropImage(img)
	}
	return img, err
}

// CropImage crops an image to the system native resolution using smart cropping.
func (c *smartImageProcessor) cropImage(img image.Image) (image.Image, error) {

	// Get the desktop dimensions
	systemWidth, systemHeight, err := c.os.getDesktopDimension()
	if err != nil {
		return nil, fmt.Errorf("failed to get desktop dimensions: %w", err)
	}

	// Create the analyzer with the option:
	r := &resizer{resampler: c.resampler}
	analyzer := smartcrop.NewAnalyzer(r)

	topCrop, err := analyzer.FindBestCrop(img, systemWidth, systemHeight)
	if err != nil {
		return nil, fmt.Errorf("finding best crop: %w", err)
	}

	// crop the image
	type SubImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	img = img.(SubImager).SubImage(topCrop)
	img = imaging.Resize(img, systemWidth, systemHeight, c.resampler)
	return img, nil
}

type resizer struct {
	resampler imaging.ResampleFilter
}

func (r *resizer) Resize(img image.Image, width, height uint) image.Image {
	return imaging.Resize(img, int(width), int(height), r.resampler)
}
