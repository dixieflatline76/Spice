package wallpaper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/util/log"

	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/pkg/ui"
	app "github.com/dixieflatline76/Spice/ui"
	"github.com/dixieflatline76/Spice/util"
)

var (
	wpInstance *wallpaperPlugin
	wpOnce     sync.Once
)

// LoadPlugin creates a new instance of the wallpaper plugin and registers it with the plugin manager.
func LoadPlugin() {
	wp := getWallpaperPlugin() // Get the wallpaper plugin instance
	app.GetPluginManager().Register(wp)
}

// OS is an interface for abstracting OS-specific operations.
type OS interface {
	setWallpaper(imagePath string) error    // Set the desktop wallpaper.
	getDesktopDimension() (int, int, error) // Get the desktop dimensions.
}

// ImageProcessor interface now includes context in all methods.
type ImageProcessor interface {
	FitImage(ctx context.Context, img image.Image) (image.Image, error)
	DecodeImage(ctx context.Context, imgBytes []byte, contentType string) (image.Image, string, error)
	EncodeImage(ctx context.Context, img image.Image, contentType string) ([]byte, error)
}

// fileInfo struct to store file path and modification time.
type fileInfo struct {
	path    string
	modTime time.Time
}

// wallpaperPlugin manages wallpaper rotation.
type wallpaperPlugin struct {
	os           OS
	imgProcessor ImageProcessor
	cfg          *Config
	ticker       *time.Ticker

	// Download related fields
	downloadMutex       sync.Mutex         // Protects currentPage, downloading, and download operations
	currentDownloadPage *util.SafeCounter  // Current page of images
	downloadedDir       string             // Directory where downloaded images are stored
	localImgRecs        []ImgSrvcImage     // Keep track of downloaded images to quickly access info like image web path
	interrupt           *util.SafeFlag     // Whether to interrupt the image download
	cancel              context.CancelFunc // Cancel function for the context
	downloadWaitGroup   *sync.WaitGroup    // Wait group for image download workers
	stopNightlyRefresh  chan struct{}      // Channel to signal nightly refresh goroutine to stop

	// Display related fields
	currentImage         ImgSrvcImage     // Current image being displayed
	localImgIndex        util.SafeCounter // Index of the current image in the download history
	randomizedIndexes    []int            // Keep track of randomized indexes for image selection
	randomizedIndexesPos int              // Position in the randomizedIndexes slice for image selection
	seenImages           map[string]bool  // Keep track of images that have been seen to trigger download of next page
	prevLocalImgs        []int            // Keep track of every image set to support the previous wallpaper action
	imgPulseOp           func()           // Function to call to pulse the image
	fitImageFlag         *util.SafeFlag   // Whether to fit the image to the desktop resolution
	shuffleImageFlag     *util.SafeFlag   // Whether to shuffle the images

	// Plugin related fields
	manager ui.PluginManager // Plugin manager
}

// getWallpaperPlugin returns the wallpaper plugin instance.
func getWallpaperPlugin() *wallpaperPlugin {
	wpOnce.Do(func() {
		// Initialize the wallpaper service for Windows
		currentOS := getOS()

		// Initialize the wallpaper service
		wpInstance = &wallpaperPlugin{
			os: currentOS, // Initialize with Windows OS
			imgProcessor: &smartImageProcessor{
				os:              currentOS,
				aspectThreshold: 0.9,
				resampler:       imaging.Lanczos}, // Initialize with smartCropper with a lenient threshold
			cfg: nil,

			downloadMutex:       sync.Mutex{},
			currentDownloadPage: util.NewSafeIntWithValue(1), // Start with the first page,
			downloadedDir:       "",
			localImgRecs:        []ImgSrvcImage{},
			interrupt:           util.NewSafeBoolWithValue(false),

			localImgIndex:        *util.NewSafeIntWithValue(-1),
			randomizedIndexes:    []int{},
			randomizedIndexesPos: 0,
			seenImages:           make(map[string]bool),
			prevLocalImgs:        []int{},
			imgPulseOp:           wpInstance.SetNextWallpaper,
			fitImageFlag:         util.NewSafeBoolWithValue(false),
			shuffleImageFlag:     util.NewSafeBoolWithValue(false),
			stopNightlyRefresh:   make(chan struct{}), // Initialize the channel for nightly refresh
		}
	})
	return wpInstance
}

// InitPlugin initializes the wallpaper plugin.
func (wp *wallpaperPlugin) Init(manager ui.PluginManager) {
	wp.manager = manager
	wp.cfg = GetConfig(manager.GetPreferences())
	if wp.cfg == nil {
		log.Fatal("Failed to initialize wallpaper configuration.")
	}
}

// Name returns the name of the plugin.
func (wp *wallpaperPlugin) Name() string {
	return pluginName
}

// stopAllWorkers stops all workers and cancels any ongoing downloads. It blocks until all workers have stopped.
// It also clears the download wait group, cancel func, and sets the interrupt flag to false.
func (wp *wallpaperPlugin) stopAllWorkers() {
	wp.downloadMutex.Lock()
	defer func() {
		wp.cancel = nil            // Cancel called, clearing it here for clarity
		wp.downloadWaitGroup = nil // All downloads finished, clear it here for clarity
		wp.interrupt.Set(false)    // Interrupt flag set to false, indicating no more downloads are needed.
		wp.downloadMutex.Unlock()  // Unlock after setting interrupt flag to false.
		log.Printf("Clean up completed. Ready to start new downloads.")
	}()

	log.Print("Stopping all all workers...")

	// Set interrupt flag to true to stop all workers.
	wp.interrupt.Set(true)

	// Cancel any ongoing downloads.
	if wp.cancel != nil {
		wp.cancel() // Cancel the existing context.

	}

	// Wait for all download goroutines to finish.
	if wp.downloadWaitGroup != nil {
		log.Print("Waiting for all downloads to stop...")
		wg := wp.downloadWaitGroup // Copy the wait group before unlocking.
		wp.downloadMutex.Unlock()  // Unlock before waiting.
		wg.Wait()                  // Wait for all download goroutines to finish.
		wp.downloadMutex.Lock()    // Lock before clearing.
		log.Printf("All downloads stopped.")
	}
}

// resetPluginState clears all state related to image downloads and resets the plugin. It also cleans up any downloaded images from the cache directory.
func (wp *wallpaperPlugin) resetPluginState() {
	wp.downloadMutex.Lock()
	wp.localImgRecs = []ImgSrvcImage{} // Clear the download history.
	clear(wp.seenImages)               // Clear the seen history.
	wp.prevLocalImgs = []int{}         // Clear the previous history.
	wp.currentDownloadPage.Set(1)      // Reset to the first page.
	wp.localImgIndex.Set(-1)           // Reset the current image index.
	wp.downloadMutex.Unlock()          // Unlock before clearing.
	err := wp.cleanupImageCache()
	if err != nil {
		log.Printf("error clearing downloaded images directory: %v", err)
	}
}

// startNightlyRefresher runs a goroutine that periodically checks if a nightly refresh is due.
// It should be run as a goroutine.
func (wp *wallpaperPlugin) startNightlyRefresher() {
	log.Print("Starting nightly refresh checker...")

	// Check roughly every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	var lastRefreshDay int = -1 // Initialize to -1 to ensure the first check works correctly

	// Perform an initial check immediately on start, in case we started/woke up after midnight
	now := time.Now()
	lastRefreshDay = wp.checkAndRunRefresh(now, lastRefreshDay, true) // Force check on startup

	for {
		select {
		case now = <-ticker.C:
			// Periodically check if the day changed
			lastRefreshDay = wp.checkAndRunRefresh(now, lastRefreshDay, false) // Normal periodic check

		case <-wp.stopNightlyRefresh:
			log.Print("Stopping nightly refresh checker.")
			return // Exit the goroutine
		}
	}
}

// checkAndRunRefresh determines if a nightly refresh should be performed based on the current day and time.
func (wp *wallpaperPlugin) checkAndRunRefresh(now time.Time, lastRefreshDay int, isInitialCheck bool) int {
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
		// Avoid running again if the initial check already decided to run
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
			// Return original day, allowing retry on the next tick
			return lastRefreshDay
		}
		log.Print("Nightly refresh check: Network available. Proceeding with refresh...")

		// It's crucial to update the last refresh day *before* the download starts.
		updatedLastRefreshDay := today

		log.Print("Running nightly refresh action...") // Clarify log message
		wp.currentDownloadPage.Set(1)
		wp.downloadAllImages() // This calls stopAllWorkers internally

		log.Print("Nightly refresh action finished.")
		return updatedLastRefreshDay // Return the new day
	}

	// No run needed, return the potentially updated lastRefreshDay (from initial check)
	return lastRefreshDay
}

// isNetworkAvailable checks if the device has a stable internet connection by attempting to connect to a public endpoint.
func (wp *wallpaperPlugin) isNetworkAvailable() bool {

	// These typically return HTTP 204 No Content quickly if successful.
	checkURL := "https://connectivitycheck.gstatic.com/generate_204"
	// Alternative public check endpoint (less standard): "https://httpbin.org/status/200"

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	// Create a new HTTP HEAD request. HEAD is lighter as it doesn't fetch the body.
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, checkURL, nil)
	if err != nil {
		// Should be rare for a valid URL and method, but handle defensively.
		log.Printf("isNetworkAvailable: Error creating request: %v", err)
		return false
	}

	// Execute the request using the default HTTP client.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("isNetworkAvailable: Network check failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("isNetworkAvailable: Network check successful (Status: %d)", resp.StatusCode)
		return true
	}

	log.Printf("isNetworkAvailable: Network check returned non-success status: %d", resp.StatusCode)
	return false
}

// downloadAllImages downloads images from all active URLs for the specified page.
func (wp *wallpaperPlugin) downloadAllImages() {
	wp.stopAllWorkers() // Stop all workers before starting new ones. Blocks until all workers are stopped.
	if wp.currentDownloadPage.Value() <= 1 {
		// If page is 1, reset plugin state to start fresh.
		wp.resetPluginState()
	}

	// Create a top-level context with a timeout (adjust as needed).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	wp.downloadMutex.Lock()
	wp.cancel = cancel
	wp.downloadWaitGroup = &sync.WaitGroup{}
	queries := wp.cfg.ImageQueries // Take a copy of the queries slice to avoid concurrent modification.
	wp.downloadMutex.Unlock()

	var message string
	for _, query := range queries {
		if query.Active {
			wp.downloadWaitGroup.Add(1)
			go func(q ImageQuery) {
				defer wp.downloadWaitGroup.Done()
				wp.downloadImagesForURL(ctx, q, wp.currentDownloadPage.Value()) // Pass the derived context.
			}(query) // Pass the query to the closure.

			message += fmt.Sprintf("[%s]\n", query.Description)
		}
	}
	wp.manager.NotifyUser("Downloading: ", message)

	// Goroutine to wait for workerCount and then cancel the context.
	go func() {
		wp.downloadMutex.Lock()
		defer wp.downloadMutex.Unlock()
		if wp.downloadWaitGroup != nil {
			wg := wp.downloadWaitGroup
			log.Print("Waiting for all downloads to finish...")
			wp.downloadMutex.Unlock()
			wg.Wait() // Wait for all goroutines to finish
			wp.downloadMutex.Lock()
			log.Print("All downloads finished.")
		}
		cancel()                   // Ensure cancellation on exit
		wp.cancel = nil            // Reset cancel *inside* the goroutine.
		wp.downloadWaitGroup = nil // Reset downloadWaitGroup *inside* the goroutine.
	}()
}

// downloadImagesForURL downloads images from the given URL for the specified page. This function purposely serialize the download process per query
// and per page to prevent overwhelming the API server. This is a design choice as there's no need to maximize download speed.
func (wp *wallpaperPlugin) downloadImagesForURL(ctx context.Context, query ImageQuery, page int) {

	// Construct the API URL
	u, err := url.Parse(query.URL)
	if err != nil {
		log.Printf("Invalid Image URL: %v", err)
		return
	}

	q := u.Query()
	q.Set("apikey", wp.cfg.GetWallhavenAPIKey()) // Add the API key
	q.Set("page", fmt.Sprint(page))              // Add the page number

	// Check for resolutions or atleast parameters
	if !q.Has("resolutions") && !q.Has("atleast") {
		width, height, err := wp.os.getDesktopDimension()
		if err != nil {
			log.Printf("Error getting desktop dimensions: %v", err)
			// Do NOT set a default resolution. Let the API handle it.
		} else {
			q.Set("atleast", fmt.Sprintf("%dx%d", width, height))
		}
	}

	u.RawQuery = q.Encode()

	// Fetch the JSON response (using context)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			log.Printf("Request canceled: %v", ctx.Err()) // Log context cancellation
			return
		}
		log.Printf("Failed to fetch from image service: %v", err)
		return
	}
	defer resp.Body.Close()

	// Read the response body (with context)
	var buf bytes.Buffer
	limitedReader := &io.LimitedReader{R: resp.Body, N: 1024 * 1024 * 100} // 100MB limit
	_, err = io.Copy(&buf, limitedReader)                                  // io.Copy respects context

	if err != nil {
		if ctx.Err() != nil {
			log.Printf("Response body read canceled: %v", ctx.Err()) //Log context cancellation
			return
		}
		log.Printf("Failed to read image service response: %v", err)
		return
	}
	body := buf.Bytes()

	// Parse the JSON response
	var response imgSrvcResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		log.Printf("Failed to parse image service JSON: %v", err)
		return
	}

	// Download images from the current page
	for _, isi := range response.Data {
		wp.downloadMutex.Lock()
		if wp.interrupt.Value() || ctx.Err() != nil { // Check both interrupt and context
			wp.downloadMutex.Unlock()
			if ctx.Err() != nil {
				log.Printf("Download of '%s' interrupted by context: %v", query.Description, ctx.Err())
			} else {
				log.Printf("Download of '%s' interrupted", query.Description)
			}
			return // Interrupt download
		}
		// Download the image and handle errors. Purposely serialize download calls to avoid rate limiting
		_, err := wp.downloadImage(ctx, isi)
		wp.downloadMutex.Unlock()
		if err != nil {
			log.Printf("Error downloading image %s: %v", isi.ID, err) // Log individual image errors
		}
	}
}

// getDownloadedDir returns the downloaded images directory.
func (wp *wallpaperPlugin) getDownloadedDir() string {
	if wp.fitImageFlag.Value() {
		return filepath.Join(wp.downloadedDir, FittedImgDir) // Use a sub directory for fitted images
	}
	return wp.downloadedDir
}

func (wp *wallpaperPlugin) downloadImage(ctx context.Context, isi ImgSrvcImage) (string, error) {

	if wp.cfg.InAvoidSet(isi.ID) {
		return "", nil // Skip this image
	}

	// Check if the image has already been downloaded
	tempFile := filepath.Join(wp.getDownloadedDir(), extractFilenameFromURL(isi.Path))
	_, err := os.Stat(tempFile)
	if !os.IsNotExist(err) {
		wp.localImgRecs = append(wp.localImgRecs, isi)
		return tempFile, nil // Image already exists
	}

	// 1. Create an HTTP request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, isi.Path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// 2. Perform the request using a client (you might have a client already)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Check for context cancellation
		if ctx.Err() != nil {
			return "", fmt.Errorf("request canceled: %w", ctx.Err())
		}
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	// 3. Read with context (using io.Copy with a limited reader)
	var buf bytes.Buffer
	limitedReader := &io.LimitedReader{R: resp.Body, N: 100 * 1024 * 1024} // Limit to 100MB, adjust as needed
	_, err = io.Copy(&buf, limitedReader)                                  // io.Copy checks for context cancellation internally!

	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("image read canceled: %w", ctx.Err())
		}
		return "", fmt.Errorf("failed to read image bytes: %w", err)
	}
	imgBytes := buf.Bytes()

	if wp.fitImageFlag.Value() {
		// Decode the image
		wp.downloadMutex.Unlock() // Unlock before fitting, decoding and encoding
		img, _, err := wp.imgProcessor.DecodeImage(ctx, imgBytes, isi.FileType)
		if err != nil {
			wp.downloadMutex.Lock() // Lock again before returning
			return "", fmt.Errorf("failed to decode image: %v", err)
		}

		// Fit the image
		processedImg, err := wp.imgProcessor.FitImage(ctx, img)
		if err != nil {
			wp.downloadMutex.Lock() // Lock again before returning
			return "", err
		}

		// Encode the processed image
		processedImgBytes, err := wp.imgProcessor.EncodeImage(ctx, processedImg, isi.FileType)
		if err != nil {
			wp.downloadMutex.Lock() // Lock again before returning
			return "", fmt.Errorf("failed to encode image: %v", err)
		}
		imgBytes = processedImgBytes
		wp.downloadMutex.Lock() // Lock before saving the image
	}

	outFile, err := os.Create(tempFile)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer outFile.Close()

	// Save the image to the temporary file
	_, err = outFile.Write(imgBytes)
	if err != nil {
		return "", fmt.Errorf("failed to save image to temporary file: %v", err)
	}

	wp.localImgRecs = append(wp.localImgRecs, isi)
	return tempFile, nil
}

// setWallpaperAt sets the wallpaper at the specified index.
func (wp *wallpaperPlugin) setWallpaperAt(imageIndex int) {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	// Check if we need to download the next page
	seenCount := len(wp.seenImages)
	localRecsCount := float64(len(wp.localImgRecs))
	threshold := int(math.Round(PrcntSeenTillDownload * localRecsCount))

	// If we have seen enough images and the threshold is met, start downloading the next page.
	if seenCount > MinSeenImagesForDownload && seenCount >= threshold {
		wp.downloadMutex.Unlock() // Unlock to give critical section control to download coordinator
		wp.currentDownloadPage.Increment()
		wp.downloadAllImages()
		wp.downloadMutex.Lock()
	}

	// Check if we have any downloaded images
	if len(wp.localImgRecs) == 0 {
		log.Println("no downloaded images found.")
		return
	}

	// Get the image file at the specified index
	safeIndex := (imageIndex + len(wp.localImgRecs)) % len(wp.localImgRecs)
	isi := wp.localImgRecs[safeIndex]
	imagePath := filepath.Join(wp.getDownloadedDir(), extractFilenameFromURL(isi.Path))

	// Set the wallpaper
	if err := wp.os.setWallpaper(imagePath); err != nil {
		log.Printf("failed to set wallpaper: %v", err)
		return // Or handle the error in a way that makes sense for your application
	}

	// Update current image and index under lock using temporary variables
	wp.currentImage = isi
	wp.localImgIndex.Set(safeIndex)
	wp.seenImages[imagePath] = true
}

// DeleteCurrentImage deletes the current wallpaper image from the filesystem and updates the history.
func (wp *wallpaperPlugin) DeleteCurrentImage() {
	// Check if there is a current image to delete
	if wp.localImgIndex.Value() == -1 {
		log.Println("no current image to delete.")
		return
	}

	imagePath := filepath.Join(wp.getDownloadedDir(), extractFilenameFromURL(wp.currentImage.Path))

	// Lock the critical section before remove the image info from all slices and maps
	wp.downloadMutex.Lock()
	currentPos := wp.localImgIndex.Value()

	// Remove the image from the slices and maps and add to avoid set
	wp.localImgRecs = append(wp.localImgRecs[:currentPos], wp.localImgRecs[currentPos+1:]...)
	wp.prevLocalImgs = wp.prevLocalImgs[:len(wp.prevLocalImgs)-1]
	delete(wp.seenImages, imagePath)
	wp.cfg.AddToAvoidSet(wp.currentImage.ID)

	wp.localImgIndex.Decrement() // Decrement the index to reflect the removal
	wp.downloadMutex.Unlock()    // Unlock the critical section as SetNextWallpaper will lock it again

	wp.SetNextWallpaper() // Set the next wallpaper immediately after deletion

	if err := os.Remove(imagePath); err != nil {
		log.Printf("failed to delete blocked image: %v", err)
	}
}

// cleanupImageCache clears the downloaded images directory.
func (wp *wallpaperPlugin) cleanupImageCache() error {
	// 1. Collect all image files with their modification times.
	var files []fileInfo
	for _, dir := range []string{wp.downloadedDir, filepath.Join(wp.downloadedDir, FittedImgDir)} {
		entries, err := os.ReadDir(dir)
		if err != nil {
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

	// 2. Sort files by modification time (oldest first).
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	// 3. Delete excess files.
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
func (wp *wallpaperPlugin) setupImageDirs() {
	// Create the downloaded images directory if it doesn't exist
	wp.downloadedDir = filepath.Join(config.GetWorkingDir(), strings.ToLower(pluginName)+"_downloads")
	fittedDir := filepath.Join(wp.downloadedDir, FittedImgDir)
	err := os.MkdirAll(wp.downloadedDir, 0755)
	if err != nil {
		log.Fatalf("error creating downloaded images directory: %v", err)
	}
	err = os.MkdirAll(fittedDir, 0755)
	if err != nil {
		log.Fatalf("error creating downloaded images directory: %v", err)
	}
}

// Activate starts the wallpaper rotation.
func (wp *wallpaperPlugin) Activate() {

	// Setup the downloaded images directories
	wp.setupImageDirs()

	// Setup nightly refresh if configured
	if wp.cfg.GetNightlyRefresh() {
		wp.downloadMutex.Lock() // Protect channel access/recreation
		if wp.stopNightlyRefresh == nil {
			wp.stopNightlyRefresh = make(chan struct{})
			log.Print("Recreated nightly refresh stop channel.")
		}
		go wp.startNightlyRefresher() // Start the checker in its own goroutine
		wp.downloadMutex.Unlock()
	}

	wp.SetShuffleImage(wp.cfg.GetImgShuffle()) // Set shuffle image preference
	wp.SetSmartFit(wp.cfg.GetSmartFit())       // Set smart fit preference

	if wp.cfg.GetChgImgOnStart() { // Check if change image on start preference is enabled
		wp.RefreshImagesAndPulse() // Refresh all images and set the first wallpaper
	} else {
		wp.currentDownloadPage.Set(1) // Reset the current download page to 1
		wp.downloadAllImages()
	}

	// Start the wallpaper rotation ticker
	wp.ChangeWallpaperFrequency(wp.cfg.GetWallpaperChangeFrequency()) // Set wallpaper change frequency preference
}

// changeFrequency changes the wallpaper change frequency.
func (wp *wallpaperPlugin) changeFrequency(newFrequency Frequency) {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	// Stop the ticker
	if wp.ticker != nil {
		wp.ticker.Stop()
	}

	// Check if the frequency is set to never
	if newFrequency == FrequencyNever {
		wp.manager.NotifyUser("Wallpaper Change", "Disabled")
		return
	}

	wp.ticker = time.NewTicker(newFrequency.Duration())

	// Reset the ticker channel to immediately trigger
	go func() {
		for range wp.ticker.C {
			wp.imgPulseOp()
		}
	}()
	wp.manager.NotifyUser("Wallpaper Change", newFrequency.String())
}

// Stop stops the wallpaper rotation, any active downloads, and cleans up.
func (wp *wallpaperPlugin) Deactivate() {
	log.Print("Deactivating wallpaper plugin...")

	// Stop Nightly Refresher
	wp.downloadMutex.Lock()
	if wp.stopNightlyRefresh != nil {
		close(wp.stopNightlyRefresh) // Close the channel to signal the goroutine to stop
		wp.stopNightlyRefresh = nil  // Set to nil so Activate knows to recreate it if needed, preventing double close
		log.Print("Nightly refresh stop signal sent and channel cleared.")
	}
	wp.downloadMutex.Unlock()

	if wp.ticker != nil {
		wp.ticker.Stop() // Stop the wallpaper change ticker
		log.Print("Wallpaper change ticker stopped.")
	}
	wp.interrupt.Set(true) // Interrupt any ongoing downloads via the existing flag
	wp.stopAllWorkers()

	log.Print("Wallpaper plugin deactivated.")
}

// GetCurrentImage returns the current image.
func (wp *wallpaperPlugin) getCurrentImage() ImgSrvcImage {
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	return wp.currentImage
}

// getWallhavenURL returns the wallhaven URL for the given API URL.
func (wp *wallpaperPlugin) getWallhavenURL(apiURL string) *url.URL {
	// Convert to API URL
	urlStr := strings.Replace(apiURL, "https://wallhaven.cc/api/v1/search?", "https://wallhaven.cc/search?", 1)
	url, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}

	q := url.Query()

	// Check for resolutions or atleast parameters
	if !q.Has("resolutions") && !q.Has("atleast") {
		width, height, err := wp.os.getDesktopDimension()
		if err != nil {
			log.Printf("error getting desktop dimensions: %v", err)
			// Do NOT set a default resolution. Let the API handle it.
		} else {
			q.Set("atleast", fmt.Sprintf("%dx%d", width, height))
		}
	}
	url.RawQuery = q.Encode()
	return url
}

// SetNextWallpaper sets the next wallpaper in the list.
func (wp *wallpaperPlugin) setNextWallpaper() {
	// Increment the index and add it to the history
	wp.prevLocalImgs = append(wp.prevLocalImgs, wp.localImgIndex.Increment())
	// Set the wallpaper
	wp.setWallpaperAt(wp.localImgIndex.Value())
}

// SetRandomWallpaper sets a random wallpaper from the list.
func (wp *wallpaperPlugin) setRandomWallpaper() {
	wp.downloadMutex.Lock()
	if len(wp.localImgRecs) == 0 {
		log.Println("no downloaded images found.")
		wp.downloadMutex.Unlock()
		return
	}

	// Generate a new randomized index set if it's not already generated or if more images were downloaded
	if len(wp.randomizedIndexes) != len(wp.localImgRecs) || wp.randomizedIndexesPos >= len(wp.randomizedIndexes) {
		wp.randomizedIndexes = rand.Perm(len(wp.localImgRecs))
		wp.randomizedIndexesPos = 0
	}

	randomIndex := wp.randomizedIndexes[wp.randomizedIndexesPos] // Get the next random index from the set
	wp.randomizedIndexesPos++                                    // Increment the position for the next random index
	wp.prevLocalImgs = append(wp.prevLocalImgs, randomIndex)     // Add the random index to the history
	wp.downloadMutex.Unlock()

	wp.setWallpaperAt(randomIndex)
}

// SetPreviousWallpaper sets the previous wallpaper in the list.
func (wp *wallpaperPlugin) SetPreviousWallpaper() {
	wp.downloadMutex.Lock()
	if len(wp.prevLocalImgs) <= 1 {
		wp.downloadMutex.Unlock()
		wp.manager.NotifyUser("No Previous Wallpaper", "You are at the beginning.")
		return // No previous history
	}
	wp.prevLocalImgs = wp.prevLocalImgs[:len(wp.prevLocalImgs)-1] // Remove the last element
	tempIndex := wp.prevLocalImgs[len(wp.prevLocalImgs)-1]        // Get the last element
	if wp.shuffleImageFlag.Value() {
		wp.randomizedIndexesPos--
	}
	wp.downloadMutex.Unlock()

	wp.setWallpaperAt(tempIndex)
}

// SetNextWallpaper sets the next wallpaper, will respect shuffle toggle
func (wp *wallpaperPlugin) SetNextWallpaper() {
	wp.imgPulseOp()
}

// SetRandomWallpaper sets a random wallpaper.
func (wp *wallpaperPlugin) SetRandomWallpaper() {
	wp.setRandomWallpaper()
}

// GetCurrentImage returns the current wallpaper image information.
func (wp *wallpaperPlugin) GetCurrentImage() ImgSrvcImage {
	return wp.getCurrentImage()
}

// ChangeWallpaperFrequency changes the wallpaper frequency.
func (wp *wallpaperPlugin) ChangeWallpaperFrequency(newFrequency Frequency) {
	wp.changeFrequency(newFrequency)
}

// ViewCurrentImageOnWeb opens the current wallpaper image in the default web browser.
func (wp *wallpaperPlugin) ViewCurrentImageOnWeb() {
	if wp.getCurrentImage().ShortURL == "" {
		wp.manager.NotifyUser("No Image Details", "Wallpaper not set during this session.")
		return
	}
	url, err := url.Parse(wp.getCurrentImage().ShortURL)
	if err != nil {
		log.Printf("failed to parse URL: %v", err)
		return
	}
	wp.manager.OpenURL(url)
}

// RefreshImagesAndPulse refreshes the list of images and pulses the image.
func (wp *wallpaperPlugin) RefreshImagesAndPulse() {
	wp.currentDownloadPage.Set(1) // Reset the current download page to 1
	wp.downloadAllImages()
	go func() {
		for i := 0; len(wp.localImgRecs) < MinLocalImageBeforePulse && i < MaxImageWaitRetry; i++ {
			time.Sleep(ImageWaitRetryDelay)
		}
		wp.imgPulseOp()
	}()
}

// SetSmartFit enables or disables smart cropping.
func (wp *wallpaperPlugin) SetSmartFit(enabled bool) {
	wp.fitImageFlag.Set(enabled) // Update the local smart fit flag
}

// SetShuffleImage enables or disables image shuffling.
func (wp *wallpaperPlugin) SetShuffleImage(enabled bool) {
	// Set the shuffle image preference and update the image pulse operation
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
// for reachability and results, returning an error on failure.
func (wp *wallpaperPlugin) checkWallhavenURL(apiURL string, queryType URLType) error {
	// --- Prepare URL specifically for the check ---
	checkURLParsed, err := url.Parse(apiURL) // Start from the clean API URL from transform step
	if err != nil {
		// Should not happen if transform succeeded, but check defensively
		return fmt.Errorf("internal error parsing API URL for check '%s': %w", apiURL, err)
	}
	checkQuery := checkURLParsed.Query()

	// Add API Key for the check itself
	apiKey := wp.cfg.GetWallhavenAPIKey() // TODO: Ensure this method exists and works
	if apiKey != "" {
		checkQuery.Set("apikey", apiKey)
	} else {
		log.Println("Warning: Checking Wallhaven URL without an API key.") // May fail for private collections etc.
	}

	// Conditionally add 'atleast' ONLY for Search type queries
	if queryType == Search {
		if !checkQuery.Has("resolutions") && !checkQuery.Has("atleast") {
			width, height, dimErr := wp.os.getDesktopDimension()
			if dimErr != nil {
				log.Printf("checkWallhavenURL: error getting dimensions: %v. Proceeding without 'atleast'.", dimErr)
			} else if width > 0 && height > 0 { // Basic sanity check on dimensions
				checkQuery.Set("atleast", fmt.Sprintf("%dx%d", width, height))
				log.Printf("checkWallhavenURL: Added 'atleast=%dx%d' parameter for check.", width, height)
			}
		}
	}
	checkURLParsed.RawQuery = checkQuery.Encode()
	urlToCheck := checkURLParsed.String()

	// --- Perform the HTTP Check ---
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Use timeout
	defer cancel()

	req, reqErr := http.NewRequestWithContext(ctx, "GET", urlToCheck, nil)
	if reqErr != nil {
		return fmt.Errorf("failed to create check request: %w", reqErr)
	}

	// Set a User-Agent (Good Practice!) - Replace YourVersion appropriately
	req.Header.Set("User-Agent", "SpiceWallpaperManager/v0.1.0") // Example

	resp, doErr := http.DefaultClient.Do(req)
	if doErr != nil {
		if errors.Is(doErr, context.DeadlineExceeded) {
			return fmt.Errorf("checking URL timed out after 15s: %w", doErr)
		}
		return fmt.Errorf("failed to fetch URL for check: %w", doErr)
	}
	defer resp.Body.Close()

	// Check HTTP Status Code for non-success (not 2xx)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try to read body for more info, but don't fail the check if reading fails
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)
		log.Printf("Wallhaven API check failed [status %d]. URL: %s | Response: %s", resp.StatusCode, urlToCheck, bodyStr)

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

	// Read the successful response body
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		log.Printf("Error reading response body after successful status: %v", readErr)
		return fmt.Errorf("failed to read check response body: %w", readErr)
	}

	// Parse JSON minimally to check for empty data
	var responseData imgSrvcResponse
	jsonErr := json.Unmarshal(body, &responseData)
	if jsonErr != nil {
		log.Printf("Failed to parse Wallhaven JSON check response. Status: %d, Body: %s", resp.StatusCode, string(body))
		return fmt.Errorf("failed to parse API check response: %w", jsonErr)
	}

	// Check if data array is empty
	if len(responseData.Data) == 0 {
		log.Printf("Wallhaven query returned no results. URL: %s | Response Meta: %+v", urlToCheck, responseData.Meta)
		return fmt.Errorf("query is valid but returned no images (check filters/permissions?)")
	}

	// If all checks passed
	log.Printf("Wallhaven URL check successful for %s (Type: %s)", apiURL, queryType)
	return nil // Success
}

// CheckWallhavenURL checks if the given URL is a valid Wallhaven URL.
func (wp *wallpaperPlugin) CheckWallhavenURL(queryURL string, queryType URLType) error {
	return wp.checkWallhavenURL(queryURL, queryType)
}

// GetWallhavenURL returns the Wallhaven URL for the given API URL.
func (wp *wallpaperPlugin) GetWallhavenURL(apiURL string) *url.URL {
	return wp.getWallhavenURL(apiURL)
}
