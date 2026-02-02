package wallpaper

import (
	"context"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util/log"
)

// ProcessImageJob processes a single image download job.
// It implements the Source + Derivative architecture:
// 1. Ensure Master (Raw) exists.
// 2. Ensure Derivative (Processed) exists (generating from Master if needed).
func (wp *Plugin) ProcessImageJob(ctx context.Context, job DownloadJob) (provider.Image, error) {
	img := job.Image
	downloadProvider := job.Provider

	if wp.cfg.InAvoidSet(img.ID) {
		return provider.Image{}, fmt.Errorf("image %s is in avoid set", img.ID)
	}

	// 0. Early Filtering (Optimization)
	// Check if we can reject the image based on dimensions BEFORE paying the 'Enrichment Tax'.
	if img.Width > 0 && img.Height > 0 {
		var resolutions []Resolution
		if wp.cfg.GetSyncMonitors() {
			monitors, err := wp.os.GetMonitors()
			if err == nil && len(monitors) > 0 {
				resolutions = GetUniqueResolutions(monitors)
			}
		}

		// Fallback to primary if Sync is OFF or GetMonitors failed
		if len(resolutions) == 0 {
			w, h, err := wp.os.GetDesktopDimension()
			if err == nil {
				resolutions = append(resolutions, Resolution{Width: w, Height: h})
			}
		}

		// Check all candidate resolutions.
		// Synced Mode: Fail if ANY are incompatible.
		// Independent Mode: Fail only if ALL are incompatible.
		incompatibleCount := 0
		for _, res := range resolutions {
			if err := wp.imgProcessor.CheckCompatibility(img.Width, img.Height, res.Width, res.Height); err != nil {
				incompatibleCount++
				if wp.cfg.GetSyncMonitors() {
					return provider.Image{}, fmt.Errorf("incompatible image skipped for resolution %dx%d: %w", res.Width, res.Height, err)
				}
			}
		}

		if !wp.cfg.GetSyncMonitors() && incompatibleCount == len(resolutions) {
			return provider.Image{}, fmt.Errorf("incompatible image skipped (fits zero monitors)")
		}
	}

	// 1. Lazy Enrichment (Soft Fail)
	// Try to enrich, but if it fails (limit reached/network error), allow the image anyway.
	// We will try to patch the metadata later via background process.
	if downloadProvider != nil {
		enrichedImg, err := downloadProvider.EnrichImage(ctx, img)
		if err != nil {
			// SOFT FAIL: Log warning but proceed.
			log.Printf("Warning: Lazy enrichment failed for %s (will try later): %v", img.ID, err)
		} else {
			img = enrichedImg
		}
	}

	// 2. Ensure Master (Raw Image)
	masterPath, err := wp.ensureMaster(ctx, img, downloadProvider)
	if err != nil {
		providerName := "Unknown"
		if downloadProvider != nil {
			providerName = downloadProvider.Name()
		}
		return provider.Image{}, fmt.Errorf("failed to ensure master (%s): %w", providerName, err)
	}

	// 3. Ensure Derivative (Processed Image)
	// Determine target flags for cache invalidation tracking
	// Determine mode flags for cleaner invalidation
	mode := wp.cfg.GetSmartFitMode()
	isFlex := mode == SmartFitAggressive
	isQuality := mode == SmartFitNormal

	processingFlags := map[string]bool{
		"SmartFit":       wp.cfg.GetSmartFit(),
		"FitFlexibility": isFlex,
		"FitQuality":     isQuality,
		"FaceCrop":       wp.cfg.GetFaceCropEnabled(),
		"FaceBoost":      wp.cfg.GetFaceBoostEnabled(),
	}

	derivativePath, err := wp.ensureDerivative(ctx, img, masterPath)
	if err != nil {
		return provider.Image{}, fmt.Errorf("failed to ensure derivative: %w", err)
	}

	// Return updated image pointing to the derivative (for display)
	// but flagged with how it was processed.
	img.FilePath = derivativePath
	img.ProcessingFlags = processingFlags

	// We also might want to store the MasterPath in the struct?
	// provider.Image doesn't have it, but we can resolve it via ID/FM later if needed.
	// The important part is FilePath points to what we show.

	if wp.favoriter != nil && wp.favoriter.IsFavorited(img) {
		img.IsFavorited = true
	}

	// log.Debugf("ProcessImageJob Finished: ID=%s, FilePath=%s, IsFav=%v", img.ID, derivativePath, img.IsFavorited)
	return img, nil
}

// ensureMaster ensures the raw image is on disk.
// Returns partial path or absolute path? Absolute.
func (wp *Plugin) ensureMaster(ctx context.Context, img provider.Image, imgProvider provider.ImageProvider) (string, error) {
	// Determine extension. We prefer what's in URL or Content-Type.
	ext := filepath.Ext(extractFilenameFromURL(img.Path))
	if ext == "" {
		if img.FileType == "image/png" {
			ext = ".png"
		} else {
			ext = ".jpg" // Default
		}
	}

	masterPath, err := wp.fm.GetMasterPath(img.ID, ext)
	if err != nil {
		return "", fmt.Errorf("security check failed for master path: %w", err)
	}

	// Check existence
	if _, err := os.Stat(masterPath); !os.IsNotExist(err) {
		// Exists.
		return masterPath, nil
	}

	// Download Remote URL
	// log.Debugf("Downloading master for %s...", img.ID)
	client := wp.httpClient
	if cp, ok := imgProvider.(provider.CustomClientProvider); ok {
		client = cp.GetClient()
	}

	reqUrl := img.Path
	if rap, ok := imgProvider.(provider.ResolutionAwareProvider); ok {
		// Attempt to get screen resolution to request a perfectly sized image
		prefs := wp.manager.GetPreferences()
		w, h := prefs.Int("screen_width"), prefs.Int("screen_height")
		if w > 0 && h > 0 {
			reqUrl = rap.WithResolution(reqUrl, w, h)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqUrl, nil)
	if err != nil {
		return "", err
	}

	if imgProvider != nil {
		if hp, ok := imgProvider.(provider.HeaderProvider); ok {
			for k, v := range hp.GetDownloadHeaders() {
				req.Header.Set(k, v)
			}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Improved error message to include provider name
		providerName := "Unknown"
		if imgProvider != nil {
			providerName = imgProvider.Name()
		}
		return "", fmt.Errorf("failed to ensure master (%s): status %d", providerName, resp.StatusCode)
	}

	file, err := os.Create(masterPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return "", err
	}

	return masterPath, nil
}

// getBaseDerivativeDir returns the relative path segment for derivatives based on current settings.
// Format: fitted / [quality|flexibility] / [standard|faceboost|facecrop]
func (wp *Plugin) getBaseDerivativeDir() string {
	mode := wp.cfg.GetSmartFitMode()
	if mode == SmartFitOff {
		return ""
	}

	// 1. Determine Mode Segment
	var modeDir string
	if mode == SmartFitAggressive {
		modeDir = FlexibilityDir
	} else {
		modeDir = QualityDir
	}

	// 2. Determine Type Segment
	var typeDir string
	if wp.cfg.GetFaceCropEnabled() {
		typeDir = FaceCropDir
	} else if wp.cfg.GetFaceBoostEnabled() {
		typeDir = FaceBoostDir
	} else {
		typeDir = StandardDir
	}

	return filepath.Join(FittedRootDir, modeDir, typeDir)
}

// ensureDerivative ensures the processed image exists.
// If missing, generates it from masterPath.
func (wp *Plugin) ensureDerivative(ctx context.Context, img provider.Image, masterPath string) (string, error) {
	derivativeDir := wp.getBaseDerivativeDir()
	if derivativeDir == "" {
		return masterPath, nil
	}

	ext := filepath.Ext(masterPath)
	targetPath, err := wp.fm.GetDerivativePath(img.ID, ext, derivativeDir)
	if err != nil {
		return "", fmt.Errorf("security check failed for derivative path: %w", err)
	}

	// Check existence
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		return targetPath, nil
	}

	// Generate
	// log.Debugf("Generating derivative for %s (Dir: %s)...", img.ID, derivativeDir)

	// Open Master
	// Using generic "Open" might be slow if we need just decode.
	// imaging.Open handles format detection.
	srcImg, err := imaging.Open(masterPath)
	if err != nil {
		return "", fmt.Errorf("failed to open master %s: %w", masterPath, err)
	}

	// Stage 3: Eager Resolution Loop
	// Fetch Monitors
	monitors, err := wp.os.GetMonitors()
	if err != nil || len(monitors) == 0 {
		log.Printf("Warning: No monitors failed (or error: %v). Using Safe Fallback (1920x1080).", err)
		monitors = []Monitor{{ID: 0, Name: "Fallback", Rect: image.Rect(0, 0, 1920, 1080)}}
	}

	resolutions := GetUniqueResolutions(monitors)
	var primaryPath string

	for _, res := range resolutions {
		// Construct path: fitted/{Settings}/{WxH}/{ID}.ext
		resDir := fmt.Sprintf("%dx%d", res.Width, res.Height)
		fullDir := filepath.Join(derivativeDir, resDir)

		targetPath, err := wp.fm.GetDerivativePath(img.ID, ext, fullDir)
		if err != nil {
			log.Printf("Error getting derivative path for %s: %v", resDir, err)
			continue
		}

		// Check existence
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			// Compatibility check: In Independent Mode, we might skip this specific resolution
			if !wp.cfg.GetSyncMonitors() {
				if err := wp.imgProcessor.CheckCompatibility(srcImg.Bounds().Dx(), srcImg.Bounds().Dy(), res.Width, res.Height); err != nil {
					log.Debugf("Skipping derivative for %s: incompatible: %v", resDir, err)
					continue
				}
			}

			// Generate
			processedImg, err := wp.imgProcessor.FitImage(ctx, srcImg, res.Width, res.Height)
			if err != nil {
				log.Printf("Error fitting image for %s: %v", resDir, err)
				continue
			}

			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				log.Printf("Error creating directory for %s: %v", targetPath, err)
				continue
			}

			if err := imaging.Save(processedImg, targetPath); err != nil {
				log.Printf("Error saving derivative %s: %v", targetPath, err)
				continue
			}
		}

		// Capture primary path (Monitor 0)
		for _, mid := range res.Monitors {
			if mid == 0 {
				primaryPath = targetPath
			}
		}
		// Fallback: capture first one if primary not set
		if primaryPath == "" {
			primaryPath = targetPath
		}
	}

	if primaryPath == "" {
		return "", fmt.Errorf("failed to generate any derivatives")
	}

	return primaryPath, nil
}
