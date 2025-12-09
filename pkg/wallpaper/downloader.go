package wallpaper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/util/log"
)

// downloadAllImages downloads images from all active URLs for the specified page.
// If doneChan is provided, it performs the download asynchronously and closes the channel when finished.
// Otherwise, it blocks until completion.
func (wp *Plugin) downloadAllImages(doneChan chan struct{}) {
	wp.stopAllWorkers()     // Stop all workers before starting new ones. Blocks until all workers are stopped.
	wp.interrupt.Set(false) // Reset interrupt flag so new downloads can proceed.
	if wp.currentDownloadPage.Value() <= 1 {
		wp.resetPluginState()
	}

	ctx, cancel := context.WithTimeout(context.Background(), HTTPClientRequestTimeout)
	// Do not defer cancel() here, as it will cancel the context immediately for async calls.
	// We must ensure cancel() is called when operations complete.

	wg := &sync.WaitGroup{}
	wp.downloadMutex.Lock()
	wp.cancel = cancel
	wp.downloadWaitGroup = wg
	// Ensure isDownloading is set, in case we were called directly.
	wp.isDownloading = true
	queries := wp.cfg.GetQueries()

	wp.downloadMutex.Unlock()
	wp.interrupt.Set(false)

	// Ensure isDownloading is cleared when we return
	defer func() {
		wp.downloadMutex.Lock()
		wp.isDownloading = false
		wp.downloadMutex.Unlock()
	}()

	var message string
	log.Debugf("Processing %d queries...", len(queries))
	for _, query := range queries {
		log.Debugf("Checking query: %s (Active: %v)", query.URL, query.Active)
		if query.Active {
			wg.Add(1)
			go func(q ImageQuery) {
				defer wg.Done()
				wp.downloadImagesForURL(ctx, q, wp.currentDownloadPage.Value()) // Pass the derived context.
			}(query) // Pass the query to the closure.

			message += fmt.Sprintf("[%s]\n", query.Description)
		}
	}
	wp.manager.NotifyUser("Downloading: ", message)

	// If async mode is requested
	if doneChan != nil {
		go func() {
			defer cancel() // Ensure context is cancelled when done
			wg.Wait()
			close(doneChan)
			log.Print("All downloads for this batch have completed (Async).")

			wp.downloadMutex.Lock()
			wp.cancel = nil
			wp.downloadWaitGroup = nil
			wp.downloadMutex.Unlock()
		}()
		return
	}

	wg.Wait() // Wait for all goroutines to finish
	cancel()  // Ensure context is cancelled when done (Sync)
	log.Print("All downloads for this batch have completed.")

	wp.downloadMutex.Lock()
	wp.cancel = nil
	wp.downloadWaitGroup = nil
	wp.downloadMutex.Unlock()
}

// downloadImagesForURL downloads images from the given URL for the specified page. This function purposely serialize the download process per query
// and per page to prevent overwhelming the API server. This is a design choice as there's no need to maximize download speed.
// downloadImagesForURL downloads images from the given URL for the specified page.
func (wp *Plugin) downloadImagesForURL(ctx context.Context, query ImageQuery, page int) {
	log.Debugf("Starting download for query: '%s' (Page: %d)", query.Description, page)
	// Find the provider for this URL
	var downloadProvider provider.ImageProvider
	var apiURL string
	var err error

	for _, p := range wp.providers {
		apiURL, err = p.ParseURL(query.URL)
		if err == nil {
			downloadProvider = p
			log.Debugf("Found provider %s for URL %s", p.Name(), query.URL)
			break
		}
	}

	if downloadProvider == nil {
		log.Printf("No provider found for URL: %s", query.URL)
		return
	}

	// Apply resolution constraints if supported
	if rap, ok := downloadProvider.(provider.ResolutionAwareProvider); ok {
		width, height, err := wp.os.getDesktopDimension()
		if err == nil {
			apiURL = rap.WithResolution(apiURL, width, height)
			log.Debugf("Applied resolution constraints for %s: %s", downloadProvider.Name(), apiURL)
		} else {
			log.Printf("Failed to get desktop dimension for resolution filtering: %v", err)
		}
	}

	images, err := downloadProvider.FetchImages(ctx, apiURL, page)
	if err != nil {
		if ctx.Err() != nil {
			log.Printf("Request canceled: %v", ctx.Err())
			return
		}
		log.Printf("Failed to fetch from %s: %v", downloadProvider.Name(), err)
		return
	}
	log.Debugf("Fetched %d images from %s for query '%s'", len(images), downloadProvider.Name(), query.Description)

	for _, img := range images {
		if wp.interrupt.Value() || ctx.Err() != nil {
			log.Printf("Download of '%s' interrupted", query.Description)
			return // Interrupt download
		}
		wp.downloadImage(ctx, img)
	}
}

// downloadImage downloads a single image, processes it if needed, and saves it to the cache.
func (wp *Plugin) downloadImage(ctx context.Context, img provider.Image) {
	if wp.cfg.InAvoidSet(img.ID) {
		return // Skip this image
	}

	// Enrich image metadata (e.g. fetch attribution for Wallhaven)
	var downloadProvider provider.ImageProvider
	if p, ok := wp.providers[img.Provider]; ok {
		downloadProvider = p
	}

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
	imagePath := filepath.Join(wp.getDownloadedDir(), filename)
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		log.Debugf("Image %s already exists at %s. Skipping download/processing.", img.ID, imagePath)
		wp.downloadMutex.Lock()
		// Update local path in the struct
		img.FilePath = imagePath
		wp.localImgRecs = append(wp.localImgRecs, img)
		wp.downloadMutex.Unlock()
		return // Image already exists
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, img.Path, nil)
	if err != nil {
		log.Printf("failed to create request for %s: %v", img.ID, err)
		return
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
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Printf("request for image %s canceled: %v", img.ID, err)
		} else {
			log.Printf("failed to download image %s: %v", img.ID, err)
		}
		return
	}
	defer resp.Body.Close()

	imgBytes, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024*1024))
	if err != nil {
		log.Printf("failed to read image bytes for %s: %v", img.ID, err)
		return
	}

	if wp.fitImageFlag.Value() {
		log.Debugf("SmartFit enabled. Processing image %s...", img.ID)
		decodedImg, _, err := wp.imgProcessor.DecodeImage(ctx, imgBytes, img.FileType)
		if err != nil {
			log.Printf("failed to decode image %s: %v", img.ID, err)
			return
		}

		processedImg, err := wp.imgProcessor.FitImage(ctx, decodedImg)
		if err != nil {
			log.Printf("failed to fit image %s: %v", img.ID, err)
			return
		}

		processedImgBytes, err := wp.imgProcessor.EncodeImage(ctx, processedImg, img.FileType)
		if err != nil {
			log.Printf("failed to encode image %s: %v", img.ID, err)
			return
		}
		imgBytes = processedImgBytes
	} else {
		log.Debugf("SmartFit disabled. Skipping processing for image %s.", img.ID)
	}

	outFile, err := os.Create(imagePath)
	if err != nil {
		log.Printf("failed to create file for %s: %v", imagePath, err)
		return
	}
	defer outFile.Close()

	if _, err := outFile.Write(imgBytes); err != nil {
		log.Printf("failed to save image to file %s: %v", imagePath, err)
		return
	}

	wp.downloadMutex.Lock()
	img.FilePath = imagePath
	wp.localImgRecs = append(wp.localImgRecs, img)
	wp.downloadMutex.Unlock()
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
