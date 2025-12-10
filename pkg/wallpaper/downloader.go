package wallpaper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util/log"
)

// ProcessImageJob processes a single image download job.
func (wp *Plugin) ProcessImageJob(ctx context.Context, job DownloadJob) (provider.Image, error) {
	img := job.Image
	downloadProvider := job.Provider

	if wp.cfg.InAvoidSet(img.ID) {
		return provider.Image{}, fmt.Errorf("image %s is in avoid set", img.ID)
	}

	// Enrich image metadata
	if downloadProvider != nil {
		enrichedImg, err := downloadProvider.EnrichImage(ctx, img)
		if err != nil {
			log.Printf("Failed to enrich image %s: %v", img.ID, err)
			// Continue with original image
		} else {
			img = enrichedImg
		}
	}

	filename := extractFilenameFromURL(img.Path)
	if filepath.Ext(filename) == "" {
		switch img.FileType {
		case "image/jpeg":
			filename += ".jpg"
		case "image/png":
			filename += ".png"
		}
	}

	// Determine paths
	// Note: using wp.getDownloadedDir() which might depend on fitImageFlag.
	// We need to ensure thread safety of getDownloadedDir or the flags it uses.
	// safeGetDownloadedDir() would be better.
	imagePath := filepath.Join(wp.getDownloadedDir(), filename)

	// Check if already exists on disk
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		log.Debugf("Image %s already exists at %s. Skipping download.", img.ID, imagePath)
		img.FilePath = imagePath
		return img, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, img.Path, nil)
	if err != nil {
		return provider.Image{}, fmt.Errorf("failed to create request for %s: %w", img.ID, err)
	}

	// Apply provider-specific headers if available
	if downloadProvider != nil {
		if hp, ok := downloadProvider.(provider.HeaderProvider); ok {
			for k, v := range hp.GetDownloadHeaders() {
				req.Header.Set(k, v)
			}
		}
	}

	resp, err := wp.httpClient.Do(req)
	if err != nil {
		return provider.Image{}, fmt.Errorf("failed to download image %s: %w", img.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return provider.Image{}, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	imgBytes, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024*1024))
	if err != nil {
		return provider.Image{}, fmt.Errorf("failed to read image bytes for %s: %w", img.ID, err)
	}

	if wp.fitImageFlag.Value() {
		log.Debugf("SmartFit enabled. Processing image %s...", img.ID)
		decodedImg, _, err := wp.imgProcessor.DecodeImage(ctx, imgBytes, img.FileType)
		if err != nil {
			return provider.Image{}, fmt.Errorf("failed to decode image %s: %w", img.ID, err)
		}

		processedImg, err := wp.imgProcessor.FitImage(ctx, decodedImg)
		if err != nil {
			return provider.Image{}, fmt.Errorf("failed to fit image %s: %w", img.ID, err)
		}

		processedImgBytes, err := wp.imgProcessor.EncodeImage(ctx, processedImg, img.FileType)
		if err != nil {
			return provider.Image{}, fmt.Errorf("failed to encode image %s: %w", img.ID, err)
		}
		imgBytes = processedImgBytes
	}

	outFile, err := os.Create(imagePath)
	if err != nil {
		return provider.Image{}, fmt.Errorf("failed to create file for %s: %w", imagePath, err)
	}
	defer outFile.Close()

	if _, err := outFile.Write(imgBytes); err != nil {
		return provider.Image{}, fmt.Errorf("failed to save image to file %s: %w", imagePath, err)
	}

	img.FilePath = imagePath
	return img, nil
}

// getDownloadedDir returns the downloaded images directory.
func (wp *Plugin) getDownloadedDir() string {
	if wp.fitImageFlag.Value() {
		if wp.cfg.GetFaceCropEnabled() {
			return filepath.Join(wp.downloadedDir, FittedFaceCropImgDir)
		}
		if wp.cfg.GetFaceBoostEnabled() {
			return filepath.Join(wp.downloadedDir, FittedFaceBoostImgDir)
		}
		return filepath.Join(wp.downloadedDir, FittedImgDir)
	}
	return wp.downloadedDir
}
