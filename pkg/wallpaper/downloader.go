package wallpaper

import (
	"context"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// ProcessImageJob processes a single image download job.
// It implements the Source + Derivative architecture:
// 1. Ensure Master (Raw) exists.
// 2. Ensure Derivative (Processed) exists (generating from Master if needed).
func (wp *Plugin) ProcessImageJob(ctx context.Context, job DownloadJob) (resultImg provider.Image, finalErr error) {
	img := job.Image
	downloadProvider := job.Provider

	// Prevent orphaned files from leaking by ensuring partial processing artifacts are scrubbed.
	// If context is cancelled midway through downloading/processing, we aggressively clean up
	// any master or derivative files we just wrote instead of waiting for the nightly sweep.
	defer func() {
		if finalErr != nil && wp.fm != nil {
			log.Debugf("Job failed for %s (Err: %v), cleaning up artifacts...", img.ID, finalErr)
			_ = wp.fm.DeepDelete(img.ID)
		}
	}()

	if wp.cfg.InAvoidSet(img.ID) {
		return provider.Image{}, fmt.Errorf("image %s is in avoid set", img.ID)
	}

	// 0. Early Filtering (Optimization)
	if err := wp.checkImageCompatibility(img); err != nil {
		return provider.Image{}, err
	}

	// 1. Lazy Enrichment (Soft Fail)
	// (API pacing is automatically handled upstream by the Fair Scheduler Dispatcher)
	if downloadProvider != nil {
		log.Debugf("Enriching image %s...", img.ID)
	}
	img = wp.enrichImage(ctx, img, downloadProvider)

	// 2. Ensure Master (Raw Image)
	// (Download pacing is automatically handled upstream by the Fair Scheduler Dispatcher)
	if downloadProvider != nil {
		log.Debugf("Downloading raw master image %s...", img.ID)
	}
	masterPath, err := wp.ensureMaster(ctx, img, downloadProvider)
	if err != nil {
		providerName := "Unknown"
		if downloadProvider != nil {
			providerName = downloadProvider.ID()
		}
		return provider.Image{}, fmt.Errorf("failed to ensure master (%s): %w", providerName, err)
	}

	// 3. Ensure Derivative (Processed Image)
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

func (wp *Plugin) checkImageCompatibility(img provider.Image) error {
	if img.Width <= 0 || img.Height <= 0 {
		return nil // Cannot check without dimensions
	}

	var resolutions []Resolution
	monitors, err := wp.os.GetMonitors()
	if err == nil && len(monitors) > 0 {
		resolutions = GetUniqueResolutions(monitors)
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
		}
	}

	if incompatibleCount == len(resolutions) {
		return fmt.Errorf("incompatible image skipped (fits zero monitors)")
	}
	return nil
}

func (wp *Plugin) enrichImage(ctx context.Context, img provider.Image, p provider.ImageProvider) provider.Image {
	if p == nil {
		return img
	}

	// *** NAMESPACING Middleware ***
	// Strip prefix so provider sees raw ID
	originalID := img.ID
	namespaced := false
	if p.Type() == provider.TypeOnline {
		prefix := p.ID() + "_"
		if strings.HasPrefix(img.ID, prefix) {
			img.ID = strings.TrimPrefix(img.ID, prefix)
			namespaced = true
		}
	}

	enrichedImg, err := p.EnrichImage(ctx, img)
	if err != nil {
		// SOFT FAIL: Log warning but proceed.
		log.Debugf("Lazy enrichment failed for %s (will try later): %v", originalID, err)
		// Restore ID if we stripped it, just in case
		if namespaced {
			img.ID = originalID
		}
		return img
	}

	// Restore Namespaced ID
	if namespaced {
		enrichedImg.ID = originalID
	}

	return enrichedImg
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

	return wp.downloadMasterFile(ctx, client, reqUrl, masterPath, imgProvider)
}

func (wp *Plugin) downloadMasterFile(ctx context.Context, client *http.Client, reqUrl, masterPath string, imgProvider provider.ImageProvider) (string, error) {
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
			providerName = imgProvider.ID()
		}
		return "", fmt.Errorf("failed to ensure master (%s): status %d", providerName, resp.StatusCode)
	}

	file, err := os.Create(masterPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		file.Close() // Close before attempting removal
		if err := os.Remove(masterPath); err != nil && !os.IsNotExist(err) {
			log.Printf("Failed to clean up aborted download %s: %v", masterPath, err)
		}
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
	resolutions := wp.getResolutionsForDerivatives()

	// 1. Check if all exist
	paths, allExist := wp.checkExistingDerivatives(img.ID, ext, derivativeDir, resolutions)
	if allExist && len(paths) > 0 {
		return wp.ensurePrimaryPath(paths), nil
	}

	// 2. Generate Missing
	if err := wp.generateMissingDerivatives(ctx, img, masterPath, derivativeDir, ext, resolutions, paths); err != nil {
		return nil, err
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("incompatible: failed to generate any derivatives")
	}

	return wp.ensurePrimaryPath(paths), nil
}

func (wp *Plugin) getResolutionsForDerivatives() []Resolution {
	monitors, err := wp.os.GetMonitors()
	if err != nil || len(monitors) == 0 {
		log.Printf("Warning: No monitors found (or error: %v). Using Safe Fallback (1920x1080).", err)
		monitors = []Monitor{{ID: 0, Name: "Fallback", Rect: image.Rect(0, 0, 1920, 1080)}}
	}
	return GetUniqueResolutions(monitors)
}

func (wp *Plugin) checkExistingDerivatives(imgID, ext, derivativeDir string, resolutions []Resolution) (map[string]string, bool) {
	paths := make(map[string]string)
	allExist := true

	for _, res := range resolutions {
		resDir := fmt.Sprintf("%dx%d", res.Width, res.Height)
		fullDir := filepath.Join(derivativeDir, resDir)
		targetPath, _ := wp.fm.GetDerivativePath(imgID, ext, fullDir)

		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			allExist = false
			// We continue to populate paths for those that DO exist, or just break?
			// The original logic broke on first missing, but we need to know WHICH exist if we want to be partial.
			// However, original logic restarted generation for ALL if one was missing (inefficient but simple).
			// Let's stick to "if any missing, we proceed to generation phase".
			// But wait, the generation phase might re-check existence.
		} else {
			paths[resDir] = targetPath
			// Mark primary
			for _, mid := range res.Monitors {
				if mid == 0 {
					paths["primary"] = targetPath
				}
			}
		}
	}
	return paths, allExist
}

func (wp *Plugin) generateMissingDerivatives(ctx context.Context, img provider.Image, masterPath, derivativeDir, ext string, resolutions []Resolution, paths map[string]string) error {
	srcImg, err := imaging.Open(masterPath)
	if err != nil {
		return fmt.Errorf("failed to open master %s: %w", masterPath, err)
	}

	for _, res := range resolutions {
		if err := ctx.Err(); err != nil {
			return err
		}
		resDir := fmt.Sprintf("%dx%d", res.Width, res.Height)

		// Skip if we already found it in the check phase?
		// The check phase populated 'paths' even if allExist was false.
		if _, ok := paths[resDir]; ok {
			continue
		}

		fullDir := filepath.Join(derivativeDir, resDir)
		targetPath, err := wp.fm.GetDerivativePath(img.ID, ext, fullDir)
		if err != nil {
			log.Printf("Error getting derivative path for %s: %v", resDir, err)
			continue
		}

		// Double check existence (persistence race or just safety)
		if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
			paths[resDir] = targetPath
			continue
		}

		// Compatibility check
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

		paths[resDir] = targetPath
		wp.updatePrimaryPath(paths, res, targetPath)
	}
	return nil
}

func (wp *Plugin) updatePrimaryPath(paths map[string]string, res Resolution, targetPath string) {
	for _, mid := range res.Monitors {
		if mid == 0 {
			paths["primary"] = targetPath
		}
	}
}

func (wp *Plugin) ensurePrimaryPath(paths map[string]string) map[string]string {
	if _, ok := paths["primary"]; !ok {
		for _, p := range paths {
			paths["primary"] = p
			break
		}
	}
	return paths
}
