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

	derivativePaths, err := wp.ensureDerivative(ctx, img, masterPath)
	if err != nil {
		return provider.Image{}, fmt.Errorf("failed to ensure derivative: %w", err)
	}

	// Return updated image pointing to the derivative (for display)
	// but flagged with how it was processed.
	img.DerivativePaths = derivativePaths
	img.ProcessingFlags = processingFlags

	// Set a default FilePath for legacy compatibility (Monitor 0)
	if path, ok := derivativePaths["primary"]; ok {
		img.FilePath = path
	} else {
		// Fallback to first available
		for _, path := range derivativePaths {
			img.FilePath = path
			break
		}
	}

	if wp.favoriter != nil && wp.favoriter.IsFavorited(img) {
		img.IsFavorited = true
	}

	return img, nil
}

// ensureMaster ensures the raw image is on disk.
// Returns absolute path.
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
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Standard User-Agent to prevent 403 Forbidden from providers like Pexels
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	if hp, ok := imgProvider.(provider.HeaderProvider); ok {
		headers := hp.GetDownloadHeaders()
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
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

// ensureDerivative ensures the processed image exists for all detected monitor resolutions.
// Returns a map of resolution "WxH" -> absolute path.
func (wp *Plugin) ensureDerivative(ctx context.Context, img provider.Image, masterPath string) (map[string]string, error) {
	derivativeDir := wp.getBaseDerivativeDir()
	if derivativeDir == "" {
		return map[string]string{"primary": masterPath}, nil
	}

	ext := filepath.Ext(masterPath)
	paths := make(map[string]string)

	// Fetch Monitors to determine which resolutions we need
	monitors, err := wp.os.GetMonitors()
	if err != nil || len(monitors) == 0 {
		log.Printf("Warning: No monitors found (or error: %v). Using Safe Fallback (1920x1080).", err)
		monitors = []Monitor{{ID: 0, Name: "Fallback", Rect: image.Rect(0, 0, 1920, 1080)}}
	}

	resolutions := GetUniqueResolutions(monitors)

	// Check if we already have all derivatives on disk (Performance Optimization)
	allExist := true
	for _, res := range resolutions {
		resDir := fmt.Sprintf("%dx%d", res.Width, res.Height)
		fullDir := filepath.Join(derivativeDir, resDir)
		targetPath, _ := wp.fm.GetDerivativePath(img.ID, ext, fullDir)
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			allExist = false
			break
		}
		paths[resDir] = targetPath
		// Mark primary
		for _, mid := range res.Monitors {
			if mid == 0 {
				paths["primary"] = targetPath
			}
		}
	}

	if allExist && len(paths) > 0 {
		if _, ok := paths["primary"]; !ok {
			// Ensure primary is always set
			for _, p := range paths {
				paths["primary"] = p
				break
			}
		}
		return paths, nil
	}

	// Generate Missing Derivatives
	srcImg, err := imaging.Open(masterPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open master %s: %w", masterPath, err)
	}

	for _, res := range resolutions {
		resDir := fmt.Sprintf("%dx%d", res.Width, res.Height)
		fullDir := filepath.Join(derivativeDir, resDir)

		targetPath, err := wp.fm.GetDerivativePath(img.ID, ext, fullDir)
		if err != nil {
			log.Printf("Error getting derivative path for %s: %v", resDir, err)
			continue
		}

		// Check existence (don't re-process if it exists)
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			// Compatibility check: We skip this specific resolution if it doesn't fit settings.
			if err := wp.imgProcessor.CheckCompatibility(srcImg.Bounds().Dx(), srcImg.Bounds().Dy(), res.Width, res.Height); err != nil {
				log.Debugf("Skipping derivative for %s: incompatible: %v", resDir, err)
				continue
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

		paths[resDir] = targetPath
		// Mark primary
		for _, mid := range res.Monitors {
			if mid == 0 {
				paths["primary"] = targetPath
			}
		}
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("failed to generate any derivatives")
	}

	if _, ok := paths["primary"]; !ok {
		for _, p := range paths {
			paths["primary"] = p
			break
		}
	}

	return paths, nil
}
