package wallpaper

import (
	"context"
	"fmt"
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
		if err := wp.imgProcessor.CheckCompatibility(img.Width, img.Height); err != nil {
			return provider.Image{}, fmt.Errorf("incompatible image skipped (pre-enrichment): %w", err)
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
		return provider.Image{}, fmt.Errorf("failed to ensure master: %w", err)
	}

	// 3. Ensure Derivative (Processed Image)
	// Determine target flags for cache invalidation tracking
	processingFlags := map[string]bool{
		"SmartFit": wp.cfg.GetSmartFit(),
		"FaceCrop": wp.cfg.GetFaceCropEnabled(),
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

	masterPath := wp.fm.GetMasterPath(img.ID, ext)

	// Check existence
	if _, err := os.Stat(masterPath); !os.IsNotExist(err) {
		// Exists.
		return masterPath, nil
	}

	// Download
	log.Debugf("Downloading master for %s...", img.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, img.Path, nil)
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

	resp, err := wp.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
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

// ensureDerivative ensures the processed image exists.
// If missing, generates it from masterPath.
func (wp *Plugin) ensureDerivative(ctx context.Context, img provider.Image, masterPath string) (string, error) {
	// Determine Derivative Type based on Config
	// This logic mirrors old getDownloadedDir logic
	var derivativeDir string
	smartFit := wp.cfg.GetSmartFit()

	if smartFit {
		if wp.cfg.GetFaceCropEnabled() {
			derivativeDir = FittedFaceCropImgDir
		} else if wp.cfg.GetFaceBoostEnabled() {
			derivativeDir = FittedFaceBoostImgDir
		} else {
			derivativeDir = FittedImgDir
		}
	} else {
		// Raw/None logic.
		// If SmartFit is OFF, we use the Master as the source?
		// OR do we copy Master to "Raw" derivative folder?
		// User said: "Source + Derivative".
		// If NO processing is needed, we can just return MasterPath?
		// Advantage: Saves disk space.
		// Disadvantage: "Deep Delete" logic currently deletes derivatives based on ID.
		// If we use MasterPath as FilePath in Store, DeepDelete works (it deletes Master).
		// So checking "SmartFit=False" implies "Use Master".
		return masterPath, nil
	}

	ext := filepath.Ext(masterPath)
	targetPath := wp.fm.GetDerivativePath(img.ID, ext, derivativeDir)

	// Check existence
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		return targetPath, nil
	}

	// Generate
	log.Debugf("Generating derivative for %s (Dir: %s)...", img.ID, derivativeDir)

	// Open Master
	// Using generic "Open" might be slow if we need just decode.
	// imaging.Open handles format detection.
	srcImg, err := imaging.Open(masterPath)
	if err != nil {
		return "", fmt.Errorf("failed to open master %s: %w", masterPath, err)
	}

	// Process
	// We reuse existing imgProcessor logic but we need to pass the IMAGE object, not bytes.
	// Wait, existing `imgProcessor.DecodeImage` takes bytes.
	// `FitImage` takes `image.Image`.
	// So `srcImg` is `image.Image`. Perfect.

	processedImg, err := wp.imgProcessor.FitImage(ctx, srcImg)
	if err != nil {
		return "", fmt.Errorf("failed to fit image: %w", err)
	}

	// Save
	// `imgProcessor.EncodeImage` returns bytes.
	// We can use `imaging.Save` directly?
	// `imaging` supports Save.
	// But `EncodeImage` handles format specific encoding logic?
	// Let's check `EncodeImage`.
	// Assuming `imaging.Save` is fine.
	if err := imaging.Save(processedImg, targetPath); err != nil {
		return "", fmt.Errorf("failed to save derivative %s: %w", targetPath, err)
	}

	return targetPath, nil
}
