package wallpaper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"

	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/pkg/ui"
	"github.com/dixieflatline76/Spice/util"
	"github.com/dixieflatline76/Spice/util/log"
	pigo "github.com/esimov/pigo/core"
)

// OS interface defines the operating system specific operations.
type OS interface {
	getDesktopDimension() (int, int, error)
	setWallpaper(path string) error
}

// ImageProcessor interface defines the image processing operations.
type ImageProcessor interface {
	DecodeImage(ctx context.Context, imgBytes []byte, contentType string) (image.Image, string, error)
	EncodeImage(ctx context.Context, img image.Image, contentType string) ([]byte, error)
	FitImage(ctx context.Context, img image.Image) (image.Image, error)
}

// Plugin is the main struct for the wallpaper downloader plugin.
type Plugin struct {
	os                   OS
	imgProcessor         ImageProcessor
	cfg                  *Config
	httpClient           *http.Client
	manager              ui.PluginManager
	downloadMutex        sync.Mutex
	currentDownloadPage  *util.SafeCounter
	downloadedDir        string
	localImgRecs         []Image
	interrupt            *util.SafeFlag
	currentImage         Image
	localImgIndex        util.SafeCounter
	randomizedIndexes    []int
	randomizedIndexesPos int
	seenImages           map[string]bool
	prevLocalImgs        []int
	imgPulseOp           func()
	fitImageFlag         *util.SafeFlag
	shuffleImageFlag     *util.SafeFlag
	stopNightlyRefresh   chan struct{}
	ticker               *time.Ticker
	cancel               context.CancelFunc
	downloadWaitGroup    *sync.WaitGroup
	prePauseFrequency    Frequency
	pauseChangeCallback  func(bool)
	pauseMenuItem        *fyne.MenuItem
	sourceMenuItem       *fyne.MenuItem
	providers            map[string]ImageProvider
}

type fileInfo struct {
	path    string
	modTime time.Time
}

var (
	wpInstance *Plugin
	wpOnce     sync.Once
)

// getPlugin returns the singleton instance of the wallpaper plugin.
func getPlugin() *Plugin {
	wpOnce.Do(func() {
		// Initialize the wallpaper service for right OS
		currentOS := getOS()

		robustClient := &http.Client{
			Timeout: HTTPClientRequestTimeout,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   HTTPClientDialerTimeout,
					KeepAlive: HTTPClientKeepAlive,
				}).DialContext,
				ResponseHeaderTimeout: HTTPClientResponseHeaderTimeout,
				TLSHandshakeTimeout:   HTTPClientTLSHandshakeTimeout,
			},
		}

		// Initialize pigo
		var pigoInstance *pigo.Pigo
		am := asset.NewManager()
		modelData, err := am.GetModel("facefinder")
		if err != nil {
			log.Printf("Warning: Failed to load face detection model: %v. Face Boost will be disabled.", err)
		} else {
			p := pigo.NewPigo()
			pigoInstance, err = p.Unpack(modelData)
			if err != nil {
				log.Printf("Warning: Failed to unpack face detection model: %v. Face Boost will be disabled.", err)
				pigoInstance = nil
			}
		}

		// Initialize the wallpaper service
		wpInstance = &Plugin{
			os: currentOS, // Initialize with right OS
			imgProcessor: &smartImageProcessor{
				os:              currentOS,
				aspectThreshold: 0.9,
				resampler:       imaging.Lanczos,
				pigo:            pigoInstance,
				config:          nil, // Will be set in Init
			},
			cfg:        nil,
			httpClient: robustClient,

			downloadMutex:       sync.Mutex{},
			currentDownloadPage: util.NewSafeIntWithValue(1), // Start with the first page,
			downloadedDir:       "",
			localImgRecs:        []Image{},
			interrupt:           util.NewSafeBoolWithValue(false),

			localImgIndex:        *util.NewSafeIntWithValue(-1),
			randomizedIndexes:    []int{},
			randomizedIndexesPos: 0,
			seenImages:           make(map[string]bool),
			prevLocalImgs:        []int{},
			imgPulseOp:           nil,
			fitImageFlag:         util.NewSafeBoolWithValue(false),
			shuffleImageFlag:     util.NewSafeBoolWithValue(false),
			stopNightlyRefresh:   make(chan struct{}), // Initialize the channel for nightly refresh
			providers:            make(map[string]ImageProvider),
		}
		wpInstance.imgPulseOp = wpInstance.setNextWallpaper
	})
	return wpInstance
}

// Init initializes the wallpaper plugin with the given PluginManager.
func (wp *Plugin) Init(manager ui.PluginManager) {
	wp.manager = manager
	wp.cfg = GetConfig(manager.GetPreferences())

	// Initialize providers
	wallhavenProvider := NewWallhavenProvider(wp.cfg, wp.httpClient)
	unsplashProvider := NewUnsplashProvider(wp.cfg, wp.httpClient)

	wp.providers = map[string]ImageProvider{
		wallhavenProvider.Name(): wallhavenProvider,
		unsplashProvider.Name():  unsplashProvider,
	}
	// Inject config into smartImageProcessor
	if sip, ok := wp.imgProcessor.(*smartImageProcessor); ok {
		sip.config = wp.cfg
	}

	log.Debugf("Wallpaper Plugin Initialized. Config: FaceBoostEnabled=%v, SmartFit=%v", wp.cfg.GetFaceBoostEnabled(), wp.cfg.GetSmartFit())
}

// Name returns the name of the plugin.
func (wp *Plugin) Name() string {
	return "Wallpaper"
}

// stopAllWorkers stops all workers and cancels any ongoing downloads. It blocks until all workers have stopped.
func (wp *Plugin) stopAllWorkers() {

	log.Print("Stopping all workers...")
	wp.interrupt.Set(true)
	wp.downloadMutex.Lock()
	if wp.cancel != nil {
		wp.cancel()
		wp.cancel = nil
	}
	wg := wp.downloadWaitGroup
	wp.downloadMutex.Unlock()

	if wg != nil {
		log.Print("Waiting for running downloads to stop...")
		wg.Wait()
		log.Print("All running downloads stopped.")
	}
}

// resetPluginState clears all state related to image downloads and resets the plugin. It also cleans up any downloaded images from the cache directory.
func (wp *Plugin) resetPluginState() {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	wp.localImgRecs = []Image{}   // Clear the download history.
	clear(wp.seenImages)          // Clear the seen history.
	wp.prevLocalImgs = []int{}    // Clear the previous history.
	wp.currentDownloadPage.Set(1) // Reset to the first page.
	wp.localImgIndex.Set(-1)      // Reset the current image index.

	err := wp.cleanupImageCache()
	if err != nil {
		log.Printf("error clearing downloaded images directory: %v", err)
	}
}

// startNightlyRefresher runs a goroutine that periodically checks if a nightly refresh is due.
func (wp *Plugin) startNightlyRefresher() {
	log.Print("Starting nightly refresh checker...")

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	var lastRefreshDay int = -1 // Initialize to -1 to ensure the first check works correctly

	runCheckWithTimeout := func(now time.Time, lastDay int, isStartup bool) int {
		done := make(chan int)
		timeoutDuration := 5 * time.Minute

		go func() {
			result := wp.checkAndRunRefresh(now, lastDay, isStartup)
			select {
			case done <- result:
			default:
				log.Print("checkAndRunRefresh completed, but the call had already timed out.")
			}
		}()

		select {
		case res := <-done:
			return res
		case <-time.After(timeoutDuration):
			log.Printf("!!! HANG DETECTED !!! Timeout of %v reached while waiting for refresh check.", timeoutDuration)
			return lastDay
		}
	}

	initialTime := time.Now()
	lastRefreshDay = runCheckWithTimeout(initialTime, lastRefreshDay, true) // Force check on startup

	for {
		select {
		case now := <-ticker.C:
			lastRefreshDay = runCheckWithTimeout(now, lastRefreshDay, false) // Normal periodic check
		case <-wp.stopNightlyRefresh:
			log.Print("Stopping nightly refresh checker.")
			return // Exit the goroutine
		}
	}
}

// checkAndRunRefresh determines if a nightly refresh should be performed based on the current day and time.
func (wp *Plugin) checkAndRunRefresh(now time.Time, lastRefreshDay int, isInitialCheck bool) int {
	today := now.Day()
	shouldRun := false
	reason := "" // For logging clarity

	if isInitialCheck {
		log.Printf("Initial refresh check at %s", now.Format(time.RFC3339))

		if lastRefreshDay == -1 && now.Hour() == 0 && now.Minute() < 6 {
			shouldRun = true
			reason = "Initial check detected start/wake-up shortly after midnight."
		} else if lastRefreshDay == -1 {
			reason = fmt.Sprintf("Initial check: Current time (%s) is not post-midnight. Setting last refresh day to %d.", now.Format(time.Kitchen), today)
			log.Print(reason)
			lastRefreshDay = today // IMPORTANT: Set lastRefreshDay here for non-midnight starts
		}
	}

	if today != lastRefreshDay {
		if !shouldRun {
			shouldRun = true
			reason = fmt.Sprintf("Detected day change (%d -> %d at %s).", lastRefreshDay, today, now.Format(time.RFC3339))
		}
	}

	if shouldRun {
		log.Printf("Decision: Refresh needed. Reason: %s", reason) // Log why it's running

		// Network Check
		if !wp.isNetworkAvailable() {
			log.Print("Nightly refresh check: Network appears to be unavailable. Skipping refresh cycle.")
			return lastRefreshDay
		}
		log.Print("Nightly refresh check: Network available. Proceeding with refresh...")

		updatedLastRefreshDay := today

		log.Print("Running nightly refresh action...") // Clarify log message
		wp.currentDownloadPage.Set(1)
		wp.downloadAllImages() // This calls stopAllWorkers internally

		log.Print("Nightly refresh action finished.")
		return updatedLastRefreshDay // Return the new day
	}

	return lastRefreshDay
}

// isNetworkAvailable checks if the device has a stable internet connection by attempting to connect to a public endpoint.
func (wp *Plugin) isNetworkAvailable() bool {
	checkURL := "https://connectivitycheck.gstatic.com/generate_204"

	ctx, cancel := context.WithTimeout(context.Background(), NetworkConnectivityCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, checkURL, nil)
	if err != nil {
		log.Printf("isNetworkAvailable: Error creating request: %v", err)
		return false
	}

	resp, err := wp.httpClient.Do(req)
	if err != nil {
		log.Printf("isNetworkAvailable: Network check failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}

	log.Printf("isNetworkAvailable: Network check returned non-success status: %d", resp.StatusCode)
	return false
}

// downloadAllImages downloads images from all active URLs for the specified page.
func (wp *Plugin) downloadAllImages() {
	wp.stopAllWorkers()     // Stop all workers before starting new ones. Blocks until all workers are stopped.
	wp.interrupt.Set(false) // Reset interrupt flag so new downloads can proceed.
	if wp.currentDownloadPage.Value() <= 1 {
		wp.resetPluginState()
	}

	ctx, cancel := context.WithTimeout(context.Background(), HTTPClientRequestTimeout)
	defer cancel()

	wg := &sync.WaitGroup{}
	wp.downloadMutex.Lock()
	wp.cancel = cancel
	wp.downloadWaitGroup = wg
	// Combine queries from all sources
	queries := append([]ImageQuery{}, wp.cfg.ImageQueries...)
	queries = append(queries, wp.cfg.UnsplashQueries...)
	wp.downloadMutex.Unlock()
	wp.interrupt.Set(false)

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

	wg.Wait() // Wait for all goroutines to finish
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
	// Find the provider for this URL
	var provider ImageProvider
	var apiURL string
	var err error

	for _, p := range wp.providers {
		apiURL, err = p.ParseURL(query.URL)
		if err == nil {
			provider = p
			log.Debugf("Found provider %s for URL %s", p.Name(), query.URL)
			break
		}
	}

	if provider == nil {
		log.Printf("No provider found for URL: %s", query.URL)
		return
	}

	// Apply resolution constraints if supported
	if rap, ok := provider.(ResolutionAwareProvider); ok {
		width, height, err := wp.os.getDesktopDimension()
		if err == nil {
			apiURL = rap.WithResolution(apiURL, width, height)
			log.Debugf("Applied resolution constraints for %s: %s", provider.Name(), apiURL)
		} else {
			log.Printf("Failed to get desktop dimension for resolution filtering: %v", err)
		}
	}

	images, err := provider.FetchImages(ctx, apiURL, page)
	if err != nil {
		if ctx.Err() != nil {
			log.Printf("Request canceled: %v", ctx.Err())
			return
		}
		log.Printf("Failed to fetch from %s: %v", provider.Name(), err)
		return
	}

	for _, img := range images {
		if wp.interrupt.Value() || ctx.Err() != nil {
			log.Printf("Download of '%s' interrupted", query.Description)
			return // Interrupt download
		}
		wp.downloadImage(ctx, img)
	}
}

// downloadImage downloads a single image, processes it if needed, and saves it to the cache.
func (wp *Plugin) downloadImage(ctx context.Context, img Image) {
	if wp.cfg.InAvoidSet(img.ID) {
		return // Skip this image
	}

	// Enrich image metadata (e.g. fetch attribution for Wallhaven)
	// We do this before checking for file existence so that even if the file exists,
	// we have the correct metadata in memory.
	// Note: If this becomes too slow for existing images, we could move it inside the !exists block,
	// but then we risk having missing attribution for cached images until a restart/refetch.
	// For now, we assume enrichment is fast or necessary.
	// Actually, if the file exists, we might not need to enrich if we persist metadata?
	// But we don't persist metadata to disk yet (only in memory struct).
	// So we should enrich.

	// Find the provider for this image
	// We don't have the provider instance easily available here, it's passed to FetchImages but not downloadImage.
	// We need to look it up or pass it.
	// downloadImage is called from downloadAllImages which iterates over queries.
	// But downloadAllImages calls FetchImages which returns images.
	// The images have a Provider string field.
	// We can look up the provider by name.

	var provider ImageProvider
	if p, ok := wp.providers[img.Provider]; ok {
		provider = p
	}

	if provider != nil {
		enrichedImg, err := provider.EnrichImage(ctx, img)
		if err != nil {
			log.Printf("Failed to enrich image %s: %v", img.ID, err)
			// Continue with original image
		} else {
			img = enrichedImg
		}
	}

	filename := extractFilenameFromURL(img.Path)
	if filepath.Ext(filename) == "" {
		if img.FileType == "image/jpeg" {
			filename += ".jpg"
		} else if img.FileType == "image/png" {
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

// setWallpaperAt sets the wallpaper at the specified index.
func (wp *Plugin) setWallpaperAt(imageIndex int) {
	var shouldDownloadNextPage bool

	wp.downloadMutex.Lock()
	if len(wp.localImgRecs) == 0 {
		log.Println("no downloaded images found.")
		wp.downloadMutex.Unlock()
		return
	}

	seenCount := len(wp.seenImages)
	localRecsCount := float64(len(wp.localImgRecs))
	threshold := int(math.Round(PrcntSeenTillDownload * localRecsCount))

	if seenCount > MinSeenImagesForDownload && seenCount >= threshold {
		shouldDownloadNextPage = true
	}

	safeIndex := (imageIndex + len(wp.localImgRecs)) % len(wp.localImgRecs)
	isi := wp.localImgRecs[safeIndex]
	imagePath := isi.FilePath
	wp.downloadMutex.Unlock()

	if shouldDownloadNextPage {
		wp.currentDownloadPage.Increment()
		go wp.downloadAllImages()
	}

	// Update internal state and UI *before* setting the wallpaper to ensure responsiveness
	// and avoid race conditions where the menu update happens long after the user action.
	wp.downloadMutex.Lock()
	wp.currentImage = isi
	wp.localImgIndex.Set(safeIndex)
	wp.seenImages[imagePath] = true

	if wp.sourceMenuItem != nil {
		attribution := isi.Attribution
		if attribution == "" {
			attribution = "Unknown"
		}
		if len(attribution) > 25 {
			attribution = attribution[:22] + "..."
		}
		wp.sourceMenuItem.Label = fmt.Sprintf("%s: %s", isi.Provider, attribution)
		wp.manager.RefreshTrayMenu()
	}
	wp.downloadMutex.Unlock()

	if err := wp.os.setWallpaper(imagePath); err != nil {
		log.Printf("failed to set wallpaper: %v", err)
		// Note: We don't revert the UI state here because it would be jarring.
		// The next successful wallpaper change will correct it.
		return
	}

	// Trigger download event for Unsplash (or other providers requiring it)
	if isi.DownloadLocation != "" {
		go wp.triggerDownload(isi.DownloadLocation)
	}
}

// DeleteCurrentImage deletes the current wallpaper image from the filesystem and updates the history.
func (wp *Plugin) DeleteCurrentImage() {
	if wp.localImgIndex.Value() == -1 {
		log.Println("no current image to delete.")
		return
	}

	imagePath := wp.currentImage.FilePath

	wp.downloadMutex.Lock()
	currentPos := wp.localImgIndex.Value()
	wp.localImgRecs = append(wp.localImgRecs[:currentPos], wp.localImgRecs[currentPos+1:]...)
	wp.prevLocalImgs = wp.prevLocalImgs[:len(wp.prevLocalImgs)-1]
	delete(wp.seenImages, imagePath)
	wp.cfg.AddToAvoidSet(wp.currentImage.ID)
	wp.localImgIndex.Decrement()
	wp.downloadMutex.Unlock()

	wp.manager.NotifyUser("Wallpaper Blocked", "Image deleted and added to blocklist.")

	wp.SetNextWallpaper()

	if err := os.Remove(imagePath); err != nil {
		log.Printf("failed to delete blocked image: %v", err)
	}
}

// cleanupImageCache clears the downloaded images directory.
func (wp *Plugin) cleanupImageCache() error {
	var files []fileInfo
	dirs := []string{
		wp.downloadedDir,
		filepath.Join(wp.downloadedDir, FittedImgDir),
		filepath.Join(wp.downloadedDir, FittedFaceBoostImgDir),
		filepath.Join(wp.downloadedDir, FittedFaceCropImgDir),
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			// If the directory doesn't exist, just skip it
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("error reading directory %s: %w", dir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && isImageFile(entry.Name()) {
				info, err := entry.Info()
				if err != nil {
					return err
				}
				files = append(files, fileInfo{filepath.Join(dir, entry.Name()), info.ModTime()})
			}
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	excess := len(files) - wp.cfg.GetCacheSize().Size()
	if excess > 0 {
		for i := 0; i < excess; i++ {
			err := os.Remove(files[i].path)
			if err != nil {
				return fmt.Errorf("error deleting file %s: %w", files[i].path, err)
			}
		}
	}
	return nil
}

// setupImageDirs sets up the downloaded images directories.
func (wp *Plugin) setupImageDirs() {
	wp.downloadedDir = filepath.Join(config.GetWorkingDir(), strings.ToLower(pluginName)+"_downloads")
	fittedDir := filepath.Join(wp.downloadedDir, FittedImgDir)
	fittedFaceBoostDir := filepath.Join(wp.downloadedDir, FittedFaceBoostImgDir)
	fittedFaceCropDir := filepath.Join(wp.downloadedDir, FittedFaceCropImgDir)

	err := os.MkdirAll(wp.downloadedDir, 0755)
	if err != nil {
		log.Fatalf("error creating downloaded images directory: %v", err)
	}
	err = os.MkdirAll(fittedDir, 0755)
	if err != nil {
		log.Fatalf("error creating fitted images directory: %v", err)
	}
	err = os.MkdirAll(fittedFaceBoostDir, 0755)
	if err != nil {
		log.Fatalf("error creating fitted face boost images directory: %v", err)
	}
	err = os.MkdirAll(fittedFaceCropDir, 0755)
	if err != nil {
		log.Fatalf("error creating fitted face crop images directory: %v", err)
	}
}

// Activate starts the wallpaper rotation.
func (wp *Plugin) Activate() {
	wp.setupImageDirs()

	if wp.cfg.GetNightlyRefresh() {
		wp.downloadMutex.Lock()
		if wp.stopNightlyRefresh == nil {
			wp.stopNightlyRefresh = make(chan struct{})
			log.Print("Recreated nightly refresh stop channel.")
		}
		go wp.startNightlyRefresher()
		wp.downloadMutex.Unlock()
	}

	wp.SetShuffleImage(wp.cfg.GetImgShuffle())
	wp.SetSmartFit(wp.cfg.GetSmartFit())

	if wp.cfg.GetChgImgOnStart() {
		wp.RefreshImagesAndPulse()
	} else {
		wp.currentDownloadPage.Set(1)
		wp.downloadAllImages()
	}
	wp.ChangeWallpaperFrequency(wp.cfg.GetWallpaperChangeFrequency())
}

// changeFrequency changes the wallpaper change frequency.
func (wp *Plugin) changeFrequency(newFrequency Frequency) {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	if wp.ticker != nil {
		wp.ticker.Stop()
	}

	if newFrequency == FrequencyNever {
		wp.manager.NotifyUser("Wallpaper Change", "Disabled")
		return
	}

	wp.ticker = time.NewTicker(newFrequency.Duration())

	go func() {
		for range wp.ticker.C {
			wp.imgPulseOp()
		}
	}()
	wp.manager.NotifyUser("Wallpaper Change", newFrequency.String())
}

// Deactivate stops the wallpaper rotation, any active downloads, and cleans up.
func (wp *Plugin) Deactivate() {
	log.Print("Deactivating wallpaper plugin...")

	wp.downloadMutex.Lock()
	if wp.stopNightlyRefresh != nil {
		close(wp.stopNightlyRefresh)
		wp.stopNightlyRefresh = nil
		log.Print("Nightly refresh stop signal sent and channel cleared.")
	}
	if wp.ticker != nil {
		wp.ticker.Stop()
		log.Print("Wallpaper change ticker stopped.")
	}
	wp.interrupt.Set(true)
	wp.downloadMutex.Unlock()

	wp.stopAllWorkers()

	log.Print("Wallpaper plugin deactivated.")
}

// getCurrentImage returns the current image.
func (wp *Plugin) getCurrentImage() Image {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()
	return wp.currentImage
}

// getWallhavenURL returns the wallhaven URL for the given API URL.
func (wp *Plugin) getWallhavenURL(apiURL string) *url.URL {
	urlStr := strings.Replace(apiURL, "https://wallhaven.cc/api/v1/search?", "https://wallhaven.cc/search?", 1)
	url, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}

	q := url.Query()
	if !q.Has("resolutions") && !q.Has("atleast") {
		if width, height, err := wp.os.getDesktopDimension(); err == nil {
			q.Set("atleast", fmt.Sprintf("%dx%d", width, height))
		}
	}
	url.RawQuery = q.Encode()
	return url
}

// setNextWallpaper sets the next wallpaper in the list.
func (wp *Plugin) setNextWallpaper() {
	wp.prevLocalImgs = append(wp.prevLocalImgs, wp.localImgIndex.Increment())
	wp.setWallpaperAt(wp.localImgIndex.Value())
}

// setRandomWallpaper sets a random wallpaper from the list.
func (wp *Plugin) setRandomWallpaper() {
	wp.downloadMutex.Lock()
	if len(wp.localImgRecs) == 0 {
		log.Println("no downloaded images found.")
		wp.downloadMutex.Unlock()
		return
	}
	if len(wp.randomizedIndexes) != len(wp.localImgRecs) || wp.randomizedIndexesPos >= len(wp.randomizedIndexes) {
		wp.randomizedIndexes = rand.Perm(len(wp.localImgRecs))
		wp.randomizedIndexesPos = 0
	}
	randomIndex := wp.randomizedIndexes[wp.randomizedIndexesPos]
	wp.randomizedIndexesPos++
	wp.prevLocalImgs = append(wp.prevLocalImgs, randomIndex)
	wp.downloadMutex.Unlock()
	wp.setWallpaperAt(randomIndex)
}

// SetPreviousWallpaper sets the previous wallpaper in the list.
func (wp *Plugin) SetPreviousWallpaper() {
	wp.downloadMutex.Lock()
	if len(wp.prevLocalImgs) <= 1 {
		wp.downloadMutex.Unlock()
		wp.manager.NotifyUser("No Previous Wallpaper", "You are at the beginning.")
		return
	}
	wp.prevLocalImgs = wp.prevLocalImgs[:len(wp.prevLocalImgs)-1]
	tempIndex := wp.prevLocalImgs[len(wp.prevLocalImgs)-1]
	if wp.shuffleImageFlag.Value() {
		wp.randomizedIndexesPos--
	}
	wp.downloadMutex.Unlock()
	wp.setWallpaperAt(tempIndex)
}

// SetNextWallpaper sets the next wallpaper, will respect shuffle toggle
func (wp *Plugin) SetNextWallpaper() {
	log.Debugf("[Timing] SetNextWallpaper: User triggered action")
	wp.imgPulseOp()
}

// GetInstance returns the singleton instance of the wallpaper plugin.
func GetInstance() *Plugin {
	return getPlugin()
}

// TogglePause toggles the wallpaper change frequency between paused (Never) and the previous frequency.
func (wp *Plugin) TogglePause() {
	wp.downloadMutex.Lock()
	currentFreq := wp.cfg.GetWallpaperChangeFrequency()
	wp.downloadMutex.Unlock()

	if currentFreq == FrequencyNever {
		// Resume
		if wp.prePauseFrequency != FrequencyNever {
			wp.cfg.SetWallpaperChangeFrequency(wp.prePauseFrequency)
			wp.ChangeWallpaperFrequency(wp.prePauseFrequency)
		} else {
			wp.cfg.SetWallpaperChangeFrequency(FrequencyHourly)
			wp.ChangeWallpaperFrequency(FrequencyHourly)
		}
		if wp.pauseChangeCallback != nil {
			wp.pauseChangeCallback(false)
		}
	} else {
		// Pause
		wp.prePauseFrequency = currentFreq
		wp.cfg.SetWallpaperChangeFrequency(FrequencyNever)
		wp.ChangeWallpaperFrequency(FrequencyNever)
		if wp.pauseChangeCallback != nil {
			wp.pauseChangeCallback(true)
		}
	}
}

// TogglePauseAction triggers the UI action for pausing/resuming if available, otherwise toggles logic directly.
func (wp *Plugin) TogglePauseAction() {
	if wp.pauseMenuItem != nil {
		wp.pauseMenuItem.Action()
	} else {
		wp.TogglePause()
	}
}

// SetPauseChangeCallback sets the callback function to be called when pause state changes.
func (wp *Plugin) SetPauseChangeCallback(callback func(bool)) {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()
	wp.pauseChangeCallback = callback
}

// IsPaused returns true if the wallpaper change frequency is set to Never.
func (wp *Plugin) IsPaused() bool {
	return wp.cfg.GetWallpaperChangeFrequency() == FrequencyNever
}

// SetRandomWallpaper sets a random wallpaper.
func (wp *Plugin) SetRandomWallpaper() {
	wp.setRandomWallpaper()
}

// GetCurrentImage returns the current wallpaper image information.
func (wp *Plugin) GetCurrentImage() Image {
	return wp.getCurrentImage()
}

// ChangeWallpaperFrequency changes the wallpaper frequency.
func (wp *Plugin) ChangeWallpaperFrequency(newFrequency Frequency) {
	wp.changeFrequency(newFrequency)
}

// ViewCurrentImageOnWeb opens the current image in the default browser.
func (wp *Plugin) ViewCurrentImageOnWeb() {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()
	if wp.currentImage.ViewURL == "" {
		log.Println("no current image to view.")
		return
	}
	url, err := url.Parse(wp.currentImage.ViewURL)
	if err != nil {
		log.Printf("failed to parse URL: %v", err)
		return
	}
	if err := wp.manager.OpenURL(url); err != nil {
		log.Printf("failed to open URL: %v", err)
	}
}

// RefreshImagesAndPulse refreshes the list of images and pulses the image.
func (wp *Plugin) RefreshImagesAndPulse() {
	go func() {
		wp.currentDownloadPage.Set(1)
		wp.downloadAllImages()

		for i := 0; len(wp.localImgRecs) < MinLocalImageBeforePulse && i < MaxImageWaitRetry; i++ {
			time.Sleep(ImageWaitRetryDelay)
		}
		wp.imgPulseOp()
	}()
}

// SetSmartFit enables or disables smart cropping.
func (wp *Plugin) SetSmartFit(enabled bool) {
	wp.fitImageFlag.Set(enabled)
}

// SetShuffleImage enables or disables image shuffling.
func (wp *Plugin) SetShuffleImage(enabled bool) {
	wp.shuffleImageFlag.Set(enabled)
	wp.cfg.SetImgShuffle(enabled)

	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	if wp.shuffleImageFlag.Value() {
		wp.imgPulseOp = wp.SetRandomWallpaper
		wp.manager.NotifyUser("Wallpaper Shuffling", "Enabled")
	} else {
		wp.imgPulseOp = wp.setNextWallpaper
		wp.manager.NotifyUser("Wallpaper Shuffling", "Disabled")
	}
}

// checkWallhavenURL takes a transformed API URL and its type, performs a network check
func (wp *Plugin) checkWallhavenURL(apiURL string, queryType URLType) error {
	checkURLParsed, err := url.Parse(apiURL)
	if err != nil {
		return fmt.Errorf("internal error parsing API URL for check '%s': %w", apiURL, err)
	}
	checkQuery := checkURLParsed.Query()

	apiKey := wp.cfg.GetWallhavenAPIKey()
	if apiKey != "" {
		checkQuery.Set("apikey", apiKey)
	} else {
		log.Println("Warning: Checking Wallhaven URL without an API key.")
	}

	if queryType == Search {
		if !checkQuery.Has("resolutions") && !checkQuery.Has("atleast") {
			if width, height, dimErr := wp.os.getDesktopDimension(); dimErr == nil && width > 0 && height > 0 {
				checkQuery.Set("atleast", fmt.Sprintf("%dx%d", width, height))
				log.Printf("checkWallhavenURL: Added 'atleast=%dx%d' parameter for check.", width, height)
			}
		}
	}
	checkURLParsed.RawQuery = checkQuery.Encode()
	urlToCheck := checkURLParsed.String()

	ctx, cancel := context.WithTimeout(context.Background(), URLValidationTimeout)
	defer cancel()

	req, reqErr := http.NewRequestWithContext(ctx, "GET", urlToCheck, nil)
	if reqErr != nil {
		return fmt.Errorf("failed to create check request: %w", reqErr)
	}
	req.Header.Set("User-Agent", "SpiceWallpaperManager/v0.1.0")

	resp, doErr := wp.httpClient.Do(req)
	if doErr != nil {
		if errors.Is(doErr, context.DeadlineExceeded) {
			return fmt.Errorf("checking URL timed out after %v: %w", URLValidationTimeout, doErr)
		}
		return fmt.Errorf("failed to fetch URL for check: %w", doErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Wallhaven API check failed [status %d]. URL: %s | Response: %s", resp.StatusCode, urlToCheck, string(bodyBytes))
		switch resp.StatusCode {
		case 401:
			return fmt.Errorf("check failed: Unauthorized (API key invalid or missing?) [status %d]", resp.StatusCode)
		case 404:
			return fmt.Errorf("check failed: Resource not found (invalid query/collection ID?) [status %d]", resp.StatusCode)
		case 429:
			return fmt.Errorf("check failed: Too many requests (rate limited) [status %d]", resp.StatusCode)
		default:
			return fmt.Errorf("check failed with HTTP status: %d", resp.StatusCode)
		}
	}

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("failed to read check response body: %w", readErr)
	}

	var responseData imgSrvcResponse
	jsonErr := json.Unmarshal(body, &responseData)
	if jsonErr != nil {
		return fmt.Errorf("failed to parse API check response: %w", jsonErr)
	}

	if len(responseData.Data) == 0 {
		return fmt.Errorf("query is valid but returned no images (check filters/permissions?)")
	}

	log.Printf("Wallhaven URL check successful for %s (Type: %s)", apiURL, queryType)
	return nil
}

// CheckWallhavenURL checks if the given URL is a valid Wallhaven URL.
func (wp *Plugin) CheckWallhavenURL(queryURL string, queryType URLType) error {
	return wp.checkWallhavenURL(queryURL, queryType)
}

// GetWallhavenURL returns the Wallhaven URL for the given API URL.
func (wp *Plugin) GetWallhavenURL(apiURL string) *url.URL {
	return wp.getWallhavenURL(apiURL)
}

// StopNightlyRefresh signals the nightly refresh goroutine to stop.
func (wp *Plugin) StopNightlyRefresh() {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	if wp.stopNightlyRefresh != nil {
		close(wp.stopNightlyRefresh) // Signal the goroutine to stop
		wp.stopNightlyRefresh = nil  // Set to nil so we don't close it twice
		log.Print("Nightly refresh stop signal sent and channel cleared.")
	}
}

// StartNightlyRefresh starts the goroutine for nightly wallpaper refresh.
func (wp *Plugin) StartNightlyRefresh() {
	// Stop any existing goroutine before starting a new one.
	wp.StopNightlyRefresh()

	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	// Create a new stop channel and start the goroutine
	wp.stopNightlyRefresh = make(chan struct{})
	log.Print("Created new nightly refresh stop channel.")
	go wp.startNightlyRefresher()
}

// LoadPlugin loads the wallpaper plugin.
func LoadPlugin(pm ui.PluginManager) {
	pm.Register(getPlugin())
}

// triggerDownload sends a request to the provider's download location endpoint to register the download.
func (wp *Plugin) triggerDownload(url string) {
	ctx, cancel := context.WithTimeout(context.Background(), NetworkConnectivityCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("Failed to create download trigger request: %v", err)
		return
	}

	// Unsplash requires the Client-ID to be sent with the download trigger request
	if strings.Contains(url, "unsplash.com") {
		req.Header.Set("Authorization", "Client-ID "+UnsplashClientID)
	}

	resp, err := wp.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to trigger download event: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Download trigger returned status: %d", resp.StatusCode)
	} else {
		log.Debugf("Download event triggered successfully for %s", url)
	}
}
