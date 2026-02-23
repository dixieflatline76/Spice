package wallpaper

import (
	"context"
	"fmt"
	"image"

	"github.com/dixieflatline76/Spice/v2/util/log"
	"github.com/muesli/smartcrop"
)

// CropStrategy defines the interface for different cropping algorithms.
type CropStrategy interface {
	// Apply executes the cropping logic.
	Apply(ctx context.Context, img image.Image, targetW, targetH int, proc *SmartImageProcessor) (image.Image, error)
	// Name returns the name of the strategy for logging.
	Name() string
}

// FaceCropStrategy performs a strict crop around a detected face.
type FaceCropStrategy struct {
	FaceBox image.Rectangle
}

func (s *FaceCropStrategy) Name() string { return "FaceCrop" }

func (s *FaceCropStrategy) Apply(ctx context.Context, img image.Image, targetW, targetH int, proc *SmartImageProcessor) (image.Image, error) {
	// 1. Calculate Crop Rect
	cropRect := proc.cropAroundFace(img.Bounds(), s.FaceBox, targetW, targetH)
	log.Debugf("Face Logic: Hard Crop Clean (Rect: %v)", cropRect)

	// 2. Sub-image
	type SubImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	subImg := img.(SubImager).SubImage(cropRect)

	// 3. Resize
	r := &resizer{resampler: proc.resampler}
	//nolint:gosec // G115
	resizedImg := r.resizeWithContext(ctx, subImg, uint(targetW), uint(targetH))
	if resizedImg == nil {
		return nil, ctx.Err()
	}
	return resizedImg, nil
}

// SmartPanStrategy pans the image to center on a specific point.
type SmartPanStrategy struct {
	Center image.Point
}

func (s *SmartPanStrategy) Name() string { return "SmartPan" }

func (s *SmartPanStrategy) Apply(ctx context.Context, img image.Image, targetW, targetH int, proc *SmartImageProcessor) (image.Image, error) {
	return proc.smartPanAndResize(ctx, img, s.Center, targetW, targetH)
}

// EntropyCropStrategy uses smartcrop logic with feet guards.
type EntropyCropStrategy struct {
	FaceFound bool
	Energy    float64
}

func (s *EntropyCropStrategy) Name() string { return "EntropyCrop" }

func (s *EntropyCropStrategy) Apply(ctx context.Context, img image.Image, targetW, targetH int, proc *SmartImageProcessor) (image.Image, error) {
	log.Debugf("SmartFit: Using Entropy Crop.")
	r := &resizer{resampler: proc.resampler}
	analyzer := smartcrop.NewAnalyzer(r)

	type cropResult struct {
		crop image.Rectangle
		err  error
	}
	resultChan := make(chan cropResult)

	go func() {
		topCrop, err := analyzer.FindBestCrop(img, targetW, targetH)
		resultChan <- cropResult{crop: topCrop, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChan:
		if result.err != nil {
			return nil, fmt.Errorf("finding best crop: %w", result.err)
		}

		// Feet Guard Logic
		if proc.config.GetSmartFitMode() == SmartFitAggressive && !s.FaceFound {
			if s.checkFeetGuard(img.Bounds(), result.crop, proc) {
				log.Debugf("SmartFit [Flexibility]: Feet Guard Triggered. Fallback to Center.")
				center := image.Point{X: img.Bounds().Dx() / 2, Y: img.Bounds().Dy() / 2}
				return proc.smartPanAndResize(ctx, img, center, targetW, targetH)
			}
		}

		smartCenter := result.crop.Min.Add(result.crop.Size().Div(2))
		return proc.smartPanAndResize(ctx, img, smartCenter, targetW, targetH)
	}
}

func (s *EntropyCropStrategy) checkFeetGuard(imgBounds, crop image.Rectangle, proc *SmartImageProcessor) bool {
	imgHeight := imgBounds.Dy()

	// Legacy Guard
	if float64(crop.Min.Y) > (float64(imgHeight) * proc.config.Tuning.FeetGuardRatio) {
		return true
	}

	// Slack Guard
	slackY := imgHeight - crop.Dy()
	if slackY > 0 {
		threshold := proc.config.Tuning.FeetGuardSlackThreshold
		if s.Energy > proc.config.Tuning.FeetGuardHighEnergyThreshold {
			threshold = proc.config.Tuning.FeetGuardSlackRelaxed
			log.Debugf("SmartFit [Flexibility]: Relaxing Slack Guard for High Energy (%.4f)", s.Energy)
		}

		if float64(crop.Min.Y) > (float64(slackY) * threshold) {
			return true
		}
	}
	return false
}
