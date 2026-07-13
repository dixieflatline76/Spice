package wallpaper

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"

	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
)

type FrameStyle int

const (
	FrameStyleWhite FrameStyle = iota
	FrameStyleBlack
	FrameStyleWood
	FrameStyleGold
)

// VirtualFramer is a Decorator for ImageProcessor that dynamically adds
// virtual gallery frames to art pieces that don't fit the monitor aspect ratio.
type VirtualFramer struct {
	next ImageProcessor
	cfg  *Config
}

// NewVirtualFramer creates a new VirtualFramer decorator.
func NewVirtualFramer(next ImageProcessor, cfg *Config) *VirtualFramer {
	return &VirtualFramer{
		next: next,
		cfg:  cfg,
	}
}

// DecodeImage delegates to the underlying processor
func (v *VirtualFramer) DecodeImage(ctx context.Context, imgBytes []byte, contentType string) (image.Image, string, error) {
	return v.next.DecodeImage(ctx, imgBytes, contentType)
}

// EncodeImage delegates to the underlying processor
func (v *VirtualFramer) EncodeImage(ctx context.Context, img image.Image, contentType string) ([]byte, error) {
	return v.next.EncodeImage(ctx, img, contentType)
}

// CheckCompatibility intercepts the compatibility check to "rescue" images that would be rejected
// by SmartFit Quality mode, IF those images are eligible for Virtual Framing.
func (v *VirtualFramer) CheckCompatibility(imgWidth, imgHeight, targetWidth, targetHeight int) error {
	err := v.next.CheckCompatibility(imgWidth, imgHeight, targetWidth, targetHeight)

	if err != nil {
		if v.cfg.VirtualFramingFallback {
			return nil // Rescue
		}

		v.cfg.mu.RLock()
		hasMuseumMode := false
		for _, enabled := range v.cfg.MuseumFraming {
			if enabled {
				hasMuseumMode = true
				break
			}
		}
		v.cfg.mu.RUnlock()

		if hasMuseumMode {
			return nil
		}
	}

	return err
}

// FitImage intercepts the fit process to apply virtual framing if necessary
func (v *VirtualFramer) FitImage(ctx context.Context, img image.Image, targetWidth, targetHeight int, opts provider.TuningOptions) (image.Image, error) {
	if targetWidth <= 0 || targetHeight <= 0 || img.Bounds().Dx() <= 10 || img.Bounds().Dy() <= 10 {
		return v.next.FitImage(ctx, img, targetWidth, targetHeight, opts)
	}

	shouldFrame := false

	// 1. User Override (Tune Image Popup)
	if opts.FrameOverride == provider.FrameOverrideForceOn {
		shouldFrame = true
	} else if opts.FrameOverride == provider.FrameOverrideForceOff {
		shouldFrame = false
	} else {
		// 2. Museum Mode (Always Frame for specific providers)
		if providerID, ok := ctx.Value(provider.ProviderIDKey).(string); ok && providerID != "" {
			if v.cfg.GetMuseumFraming(providerID) {
				shouldFrame = true
			}
		}

		// 3. Fallback Mode (Rescue Misfit Images)
		if !shouldFrame && v.cfg.VirtualFramingFallback {
			// Ask the next processor (SmartFit) if it WOULD reject this image.
			err := v.next.CheckCompatibility(img.Bounds().Dx(), img.Bounds().Dy(), targetWidth, targetHeight)
			if err != nil {
				shouldFrame = true
			}
		}
	}

	if !shouldFrame {
		return v.next.FitImage(ctx, img, targetWidth, targetHeight, opts)
	}

	if v.isStudioObject(img) && opts.FrameOverride != provider.FrameOverrideForceOn {
		return v.next.FitImage(ctx, img, targetWidth, targetHeight, opts)
	}

	if ptr, ok := ctx.Value(provider.VirtualFramedKey).(*bool); ok && ptr != nil {
		*ptr = true
	}
	return v.renderGalleryWall(img, targetWidth, targetHeight, opts)
}

func (v *VirtualFramer) shouldFrameDims(srcWidth, srcHeight, targetWidth, targetHeight int, setting VirtualFramingMode) bool {
	srcAspect := float64(srcWidth) / float64(srcHeight)
	targetAspect := float64(targetWidth) / float64(targetHeight)

	// Calculate mismatch ratio (how much larger one aspect is than the other)
	mismatch := targetAspect / srcAspect
	if mismatch < 1.0 {
		mismatch = 1.0 / mismatch // Handle inverted mismatch (e.g. panorama on portrait)
	}

	if setting == FramingExtreme && mismatch > 2.5 {
		return true
	}
	if setting == FramingSignificant && mismatch > 2.0 {
		return true
	}

	return false
}

func (v *VirtualFramer) isStudioObject(img image.Image) bool {
	// Simple edge variance heuristic
	// Sample pixels along the border. If they are almost identical, it's a studio object.
	bounds := img.Bounds()
	if bounds.Dx() < 10 || bounds.Dy() < 10 {
		return false
	}

	// Downsample massively to get a quick average of the border
	thumb := imaging.Resize(img, 10, 10, imaging.NearestNeighbor)

	// Sample outer pixels
	var rSum, gSum, bSum uint32
	var rSq, gSq, bSq float64
	count := 0

	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			if x == 0 || x == 9 || y == 0 || y == 9 {
				r, g, b, _ := thumb.At(x, y).RGBA()
				r8, g8, b8 := r>>8, g>>8, b>>8
				rSum += r8
				gSum += g8
				bSum += b8
				count++
			}
		}
	}

	rAvg := float64(rSum) / float64(count)
	gAvg := float64(gSum) / float64(count)
	bAvg := float64(bSum) / float64(count)

	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			if x == 0 || x == 9 || y == 0 || y == 9 {
				r, g, b, _ := thumb.At(x, y).RGBA()
				r8, g8, b8 := r>>8, g>>8, b>>8
				rSq += math.Pow(float64(r8)-rAvg, 2)
				gSq += math.Pow(float64(g8)-gAvg, 2)
				bSq += math.Pow(float64(b8)-bAvg, 2)
			}
		}
	}

	variance := (rSq + gSq + bSq) / float64(count*3)
	return variance < 100.0 // Very low variance indicates a solid studio background
}

func (v *VirtualFramer) determineFrameStyle(avgColor color.Color) FrameStyle {
	r, g, b, _ := avgColor.RGBA()
	r8, g8, b8 := float64(r>>8), float64(g>>8), float64(b>>8)

	// Calculate Luminance
	luminance := (0.299*r8 + 0.587*g8 + 0.114*b8) / 255.0

	// Very dark image gets a white frame to pop
	if luminance < 0.25 {
		return FrameStyleWhite
	}

	// Very bright image gets a black frame for contrast
	if luminance > 0.8 {
		return FrameStyleBlack
	}

	// Calculate basic color temperature (red vs blue)
	if r8 > b8*1.2 && r8 > g8*1.1 {
		return FrameStyleWood // Warm colors get wood/gold
	}

	return FrameStyleBlack // Cool or neutral gets black
}

func (v *VirtualFramer) calculateWallColor(img image.Image, opts provider.TuningOptions) color.Color {
	setting := v.cfg.VirtualWallColor
	if opts.WallColor == provider.WallColorOverrideNeutral {
		setting = WallNeutral
	} else if opts.WallColor == provider.WallColorOverrideAlgorithmic {
		setting = WallAlgorithmic
	}

	avgPixel := imaging.Resize(img, 1, 1, imaging.Linear)
	r, g, b, _ := avgPixel.At(0, 0).RGBA()

	if setting == WallNeutral {
		luminance := (0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)) / 255.0
		if luminance > 0.5 {
			return color.RGBA{40, 40, 45, 255} // Charcoal
		}
		return color.RGBA{220, 220, 225, 255} // Slate White
	}

	// Algorithmic (Dominant Muted Hue)
	// Extract the most colorful pixels to avoid "mud" average, then scale brightness to a moody gallery tone.
	small := imaging.Resize(img, 32, 32, imaging.Linear)
	bounds := small.Bounds()

	var sumR, sumG, sumB, count uint32
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pr, pg, pb, _ := small.At(x, y).RGBA()
			pr >>= 8
			pg >>= 8
			pb >>= 8

			maxC := pr
			if pg > maxC {
				maxC = pg
			}
			if pb > maxC {
				maxC = pb
			}

			minC := pr
			if pg < minC {
				minC = pg
			}
			if pb < minC {
				minC = pb
			}

			// If it's somewhat colorful, add to average
			if (maxC - minC) > 20 {
				sumR += pr
				sumG += pg
				sumB += pb
				count++
			}
		}
	}

	if count == 0 {
		// Fallback to overall average if the image is mostly greyscale
		sumR = r >> 8
		sumG = g >> 8
		sumB = b >> 8
		count = 1
	}

	domR := float64(sumR / count)
	domG := float64(sumG / count)
	domB := float64(sumB / count)

	// Dim the color to a "wall" brightness (e.g. moody gallery feel)
	lum := 0.299*domR + 0.587*domG + 0.114*domB
	targetLum := 50.0 // Out of 255, this is a dark, moody value

	var scale float64
	if lum > 0 {
		scale = targetLum / lum
	} else {
		scale = 1.0
	}

	// Cap scale so we don't turn dark images into blinding neon noise
	if scale > 3.0 {
		scale = 3.0
	}

	valR := uint8(math.Min(255, domR*scale))
	valG := uint8(math.Min(255, domG*scale))
	valB := uint8(math.Min(255, domB*scale))

	return color.RGBA{valR, valG, valB, 255}
}

func (v *VirtualFramer) renderGalleryWall(img image.Image, targetW, targetH int, opts provider.TuningOptions) (image.Image, error) {
	wallColor := v.calculateWallColor(img, opts)
	canvas := imaging.New(targetW, targetH, wallColor)

	frameSize := v.cfg.VirtualFrameSize
	if frameSize == 0 {
		frameSize = 0.85
	}
	if opts.FrameSize > 0 {
		frameSize = opts.FrameSize
	}

	useMatting := v.cfg.VirtualPaperMatting
	if opts.Matting == provider.MattingOverrideOn {
		useMatting = true
	} else if opts.Matting == provider.MattingOverrideOff {
		useMatting = false
	}

	imgW := float64(img.Bounds().Dx())
	imgH := float64(img.Bounds().Dy())

	// Golden Ratio Proportions: 2% Frame, 5% Matting
	frameProp := 0.02
	matProp := 0.05

	C := frameProp
	if useMatting {
		C += matProp
	}

	maxDim := math.Max(imgW, imgH)

	totalW := imgW + 2*C*maxDim
	totalH := imgH + 2*C*maxDim

	maxAllowedW := float64(targetW) * frameSize
	maxAllowedH := float64(targetH) * frameSize

	scaleW := maxAllowedW / totalW
	scaleH := maxAllowedH / totalH
	scale := math.Min(scaleW, scaleH)

	artW := int(imgW * scale)
	artH := int(imgH * scale)

	if artW <= 0 || artH <= 0 {
		artW, artH = 1, 1
	}

	artResized := imaging.Resize(img, artW, artH, imaging.Lanczos)

	// Determine dominant color for frame styling
	avgPixel := imaging.Resize(img, 1, 1, imaging.Linear)
	style := v.determineFrameStyle(avgPixel.At(0, 0))

	frameThickness := int(maxDim * scale * frameProp)
	if frameThickness < 15 {
		frameThickness = 15
	}

	var frameColor color.Color
	var highlightColor color.Color
	var shadowColor color.Color
	switch style {
	case FrameStyleWhite:
		frameColor = color.RGBA{240, 240, 240, 255}
		highlightColor = color.RGBA{255, 255, 255, 255}
		shadowColor = color.RGBA{210, 210, 210, 255}
	case FrameStyleBlack:
		frameColor = color.RGBA{30, 30, 30, 255}
		highlightColor = color.RGBA{60, 60, 60, 255}
		shadowColor = color.RGBA{10, 10, 10, 255}
	case FrameStyleGold:
		frameColor = color.RGBA{218, 165, 32, 255}
		highlightColor = color.RGBA{255, 215, 0, 255}
		shadowColor = color.RGBA{160, 110, 15, 255}
	case FrameStyleWood:
		frameColor = color.RGBA{101, 67, 33, 255} // Dark walnut
		highlightColor = color.RGBA{139, 90, 43, 255}
		shadowColor = color.RGBA{60, 40, 20, 255}
	}

	// Matting
	matW := artW
	matH := artH
	var mat *image.RGBA
	if useMatting {
		matThickness := int(maxDim * scale * matProp)
		if matThickness < 40 {
			matThickness = 40
		}
		matW = artW + (matThickness * 2)
		matH = artH + (matThickness * 2)

		// Base mat color (warm off-white / archival paper)
		matBaseColor := color.RGBA{248, 245, 238, 255}
		mat = image.NewRGBA(image.Rect(0, 0, matW, matH))
		draw.Draw(mat, mat.Bounds(), &image.Uniform{matBaseColor}, image.Point{}, draw.Src)

		// Paper Texture (Noise)
		for y := 0; y < matH; y++ {
			for x := 0; x < matW; x++ {
				// Don't draw noise inside the art area
				if x >= matThickness && x < matW-matThickness && y >= matThickness && y < matH-matThickness {
					continue
				}
				//nolint:gosec // intentional weak rand for visual noise
				noise := uint8(rand.Intn(10)) // subtle noise
				//nolint:gosec // intentional weak rand for visual noise
				if rand.Intn(2) == 0 {
					c := mat.RGBAAt(x, y)
					mat.SetRGBA(x, y, color.RGBA{
						R: uint8(math.Min(255, float64(c.R)+float64(noise))),
						G: uint8(math.Min(255, float64(c.G)+float64(noise))),
						B: uint8(math.Min(255, float64(c.B)+float64(noise))),
						A: 255,
					})
				} else {
					c := mat.RGBAAt(x, y)
					mat.SetRGBA(x, y, color.RGBA{
						R: uint8(math.Max(0, float64(c.R)-float64(noise))),
						G: uint8(math.Max(0, float64(c.G)-float64(noise))),
						B: uint8(math.Max(0, float64(c.B)-float64(noise))),
						A: 255,
					})
				}
			}
		}

		// Inner cut bevel (45-degree cut showing core of the paper)
		bevelCoreColor := color.RGBA{255, 252, 245, 255}
		bevelShadowColor := color.RGBA{200, 195, 185, 255}
		bevelSize := 3

		for i := 0; i < bevelSize; i++ {
			rect := image.Rect(matThickness-i-1, matThickness-i-1, matW-matThickness+i+1, matH-matThickness+i+1)
			draw.Draw(mat, image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+1), &image.Uniform{bevelCoreColor}, image.Point{}, draw.Src)
			draw.Draw(mat, image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+1, rect.Max.Y), &image.Uniform{bevelCoreColor}, image.Point{}, draw.Src)
			draw.Draw(mat, image.Rect(rect.Min.X, rect.Max.Y-1, rect.Max.X, rect.Max.Y), &image.Uniform{bevelShadowColor}, image.Point{}, draw.Src)
			draw.Draw(mat, image.Rect(rect.Max.X-1, rect.Min.Y, rect.Max.X, rect.Max.Y), &image.Uniform{bevelShadowColor}, image.Point{}, draw.Src)
		}
	}

	// Frame
	frameW := matW + (frameThickness * 2)
	frameH := matH + (frameThickness * 2)
	frame := image.NewRGBA(image.Rect(0, 0, frameW, frameH))
	draw.Draw(frame, frame.Bounds(), &image.Uniform{frameColor}, image.Point{}, draw.Src)

	// Frame Bevels (3D effect)
	bevelWidth := int(math.Max(1, float64(frameThickness)*0.3)) // 30% of frame thickness is bevel

	for i := 0; i < bevelWidth; i++ {
		rect := image.Rect(i, i, frameW-i, frameH-i)
		draw.Draw(frame, image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+1), &image.Uniform{highlightColor}, image.Point{}, draw.Src)
		draw.Draw(frame, image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+1, rect.Max.Y), &image.Uniform{highlightColor}, image.Point{}, draw.Src)
		draw.Draw(frame, image.Rect(rect.Min.X, rect.Max.Y-1, rect.Max.X, rect.Max.Y), &image.Uniform{shadowColor}, image.Point{}, draw.Src)
		draw.Draw(frame, image.Rect(rect.Max.X-1, rect.Min.Y, rect.Max.X, rect.Max.Y), &image.Uniform{shadowColor}, image.Point{}, draw.Src)

		innerI := frameThickness - bevelWidth + i
		innerRect := image.Rect(innerI, innerI, frameW-innerI, frameH-innerI)
		draw.Draw(frame, image.Rect(innerRect.Min.X, innerRect.Min.Y, innerRect.Max.X, innerRect.Min.Y+1), &image.Uniform{shadowColor}, image.Point{}, draw.Src)
		draw.Draw(frame, image.Rect(innerRect.Min.X, innerRect.Min.Y, innerRect.Min.X+1, innerRect.Max.Y), &image.Uniform{shadowColor}, image.Point{}, draw.Src)
		draw.Draw(frame, image.Rect(innerRect.Min.X, innerRect.Max.Y-1, innerRect.Max.X, innerRect.Max.Y), &image.Uniform{highlightColor}, image.Point{}, draw.Src)
		draw.Draw(frame, image.Rect(innerRect.Max.X-1, innerRect.Min.Y, innerRect.Max.X, innerRect.Max.Y), &image.Uniform{highlightColor}, image.Point{}, draw.Src)
	}

	// Create Drop Shadow
	shadowBlurRadius := float64(math.Max(10, float64(frameW)*0.015))
	shadowOffset := int(shadowBlurRadius * 0.8)

	shadowPad := int(shadowBlurRadius * 3)
	shadowW := frameW + shadowPad*2
	shadowH := frameH + shadowPad*2
	shadow := image.NewRGBA(image.Rect(0, 0, shadowW, shadowH))

	black := color.RGBA{0, 0, 0, 180}
	draw.Draw(shadow, image.Rect(shadowPad, shadowPad, shadowPad+frameW, shadowPad+frameH), &image.Uniform{black}, image.Point{}, draw.Src)

	shadowBlurred := imaging.Blur(shadow, shadowBlurRadius)

	// Composite
	canvas = imaging.Overlay(canvas, shadowBlurred, image.Pt((targetW-shadowW)/2+shadowOffset, (targetH-shadowH)/2+shadowOffset), 1.0)
	canvas = imaging.OverlayCenter(canvas, frame, 1.0)
	if useMatting {
		canvas = imaging.OverlayCenter(canvas, mat, 1.0)
	}
	canvas = imaging.OverlayCenter(canvas, artResized, 1.0)

	return canvas, nil
}
