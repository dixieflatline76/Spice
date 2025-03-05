package wallpaper

import (
	"encoding/json"
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
	"github.com/dixieflatline76/Spice/pkg"
	"github.com/dixieflatline76/Spice/ui"
	"github.com/dixieflatline76/Spice/util"
)

var (
	wpInstance *wallpaperPlugin
	wpOnce     sync.Once
)

// LoadPlugin creates a new instance of the wallpaper plugin and registers it with the plugin manager.
func LoadPlugin() {
	wp := getWallpaperPlugin() // Get the wallpaper plugin instance
	ui.GetPluginManager().Register(wp)
}

// OS is an interface for abstracting OS-specific operations.
type OS interface {
	setWallpaper(imagePath string) error    // Set the desktop wallpaper.
	getDesktopDimension() (int, int, error) // Get the desktop dimensions.
}

// ImageProcessor is an interface for image cropping.
type ImageProcessor interface {
	FitImage(img image.Image) (image.Image, error)                                // Fit the image to the desktop resolution.
	DecodeImage(imgBytes []byte, contentType string) (image.Image, string, error) // Decode an image from a byte slice.
	EncodeImage(img image.Image, contentType string) ([]byte, error)              // Encode an image to a byte slice.
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
	downloadMutex       sync.Mutex     // Protects currentPage, downloading, and download operations
	currentDownloadPage *util.SafeInt  // Current page of images
	downloadedDir       string         // Directory where downloaded images are stored
	localImgRecs        []ImgSrvcImage // Keep track of downloaded images to quickly access info like image web path
	interrupt           *util.SafeBool // Whether to interrupt the image download
	workerCount         *util.SafeInt  // Number of concurrent image download workers

	// Display related fields
	currentImage     ImgSrvcImage    // Current image being displayed
	localImgIndex    util.SafeInt    // Index of the current image in the download history
	seenImages       map[string]bool // Keep track of images that have been seen to trigger download of next page
	prevLocalImgs    []int           // Keep track of every image set to support the previous wallpaper action
	imgPulseOp       func()          // Function to call to pulse the image
	fitImageFlag     *util.SafeBool  // Whether to fit the image to the desktop resolution
	shuffleImageFlag *util.SafeBool  // Whether to shuffle the images

	// Plugin related fields
	manager pkg.UIPluginManager // Plugin manager
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
			workerCount:         util.NewSafeIntWithValue(0),

			localImgIndex:    *util.NewSafeIntWithValue(-1),
			seenImages:       make(map[string]bool),
			prevLocalImgs:    []int{},
			imgPulseOp:       wpInstance.SetNextWallpaper,
			fitImageFlag:     util.NewSafeBoolWithValue(false), // Initialize with smart fit preference
			shuffleImageFlag: util.NewSafeBoolWithValue(false),
		}
	})
	return wpInstance
}

// InitPlugin initializes the wallpaper plugin.
func (wp *wallpaperPlugin) Init(manager pkg.UIPluginManager) {
	wp.manager = manager
	wp.cfg = GetConfig(manager.GetPreferences())
}

// Name returns the name of the plugin.
func (wp *wallpaperPlugin) Name() string {
	return pluginName
}

// RefreshImages downloads all images from the configured URLs.
func (wp *wallpaperPlugin) refreshImages() {
	// Signal all download goroutines to stop
	wp.interrupt.Set(true)

	// Wait for all download goroutines to finish
	for wp.workerCount.Value() > 0 {
		log.Print("Waiting for download goroutines to finish...")
		time.Sleep(time.Second * 1)
	}

	wp.downloadMutex.Lock()
	err := wp.cleanupImageCache()
	if err != nil {
		log.Printf("Error clearing downloaded images directory: %v", err)
	}
	wp.localImgRecs = []ImgSrvcImage{} // Clear the download history
	clear(wp.seenImages)               // Clear the seen history)
	wp.prevLocalImgs = []int{}         // Clear the previous history
	wp.currentDownloadPage.Set(1)      // Reset to the first page
	wp.localImgIndex.Set(-1)
	wp.interrupt.Set(false)
	wp.downloadMutex.Unlock()

	wp.downloadAllImages(wp.currentDownloadPage.Value())
}

// downloadAllImages downloads images from all active URLs for the specified page.
func (wp *wallpaperPlugin) downloadAllImages(page int) {
	var message string
	for _, query := range wp.cfg.ImageQueries {
		if query.Active {
			go wp.downloadImagesForURL(query, page)
			message += fmt.Sprintf("[%s]\n", query.Description)
		}
	}
	wp.manager.NotifyUser("Downloading Images From:", message)
}

// downloadImagesForURL downloads images from the given URL for the specified page.
func (wp *wallpaperPlugin) downloadImagesForURL(query ImageQuery, page int) {
	wp.workerCount.Increment()
	defer wp.workerCount.Decrement()

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

	// Fetch the JSON response
	resp, err := http.Get(u.String())
	if err != nil {
		log.Printf("Failed to fetch from image service: %v", err)
		return
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read image service response: %v", err)
		return
	}

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
		if wp.interrupt.Value() {
			wp.downloadMutex.Unlock()
			log.Printf("Download of '%s' interrupted", query.Description)
			return // Interrupt download
		}
		wp.downloadImage(isi)
		wp.downloadMutex.Unlock()
	}
}

// getDownloadedDir returns the downloaded images directory.
func (wp *wallpaperPlugin) getDownloadedDir() string {
	if wp.fitImageFlag.Value() {
		return filepath.Join(wp.downloadedDir, FittedImgDir) // Use a sub directory for fitted images
	}
	return wp.downloadedDir
}

// downloadImage downloads a single image.
func (wp *wallpaperPlugin) downloadImage(isi ImgSrvcImage) (string, error) {

	if wp.cfg.InAvoidSet(isi.ID) {
		log.Printf("Skipping download, image '%s' in avoid set", isi.ID)
		return "", nil // Skip this image
	}

	// Check if the image has already been downloaded
	tempFile := filepath.Join(wp.getDownloadedDir(), extractFilenameFromURL(isi.Path))
	_, err := os.Stat(tempFile)
	if !os.IsNotExist(err) {
		wp.localImgRecs = append(wp.localImgRecs, isi)
		log.Printf("Skipping download, image '%s' in cache", isi.ID)
		return tempFile, nil // Image already exists
	}

	resp, err := http.Get(isi.Path)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %v", err)
	}
	defer resp.Body.Close()

	// Read the image bytes
	imgBytes, err := io.ReadAll(resp.Body) // Read all bytes at once
	if err != nil {
		return "", fmt.Errorf("failed to read image bytes: %w", err)
	}

	if wp.fitImageFlag.Value() {
		// Decode the image
		wp.downloadMutex.Unlock() // Unlock before fitting, decoding and encoding
		img, _, err := wp.imgProcessor.DecodeImage(imgBytes, isi.FileType)
		if err != nil {
			log.Printf("Failed to decode image: %v", err)
			wp.downloadMutex.Lock() // Lock again before returning
			return "", fmt.Errorf("failed to decode image: %v", err)
		}
		processedImg, err := wp.imgProcessor.FitImage(img)
		if err != nil {
			// Failed to fit image, return the error and continue
			log.Printf("Failed to fit image: %v", err)
			wp.downloadMutex.Lock() // Lock again before returning
			return "", err
		}

		// Encode the processed image
		processedImgBytes, err := wp.imgProcessor.EncodeImage(processedImg, isi.FileType)
		if err != nil {
			log.Printf("Failed to encode image: %v", err)
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
	log.Printf("Image '%s' dowwnloaded. downloadHistory length: %d", isi.ID, len(wp.localImgRecs))
	return tempFile, nil
}

// setWallpaperAt sets the wallpaper at the specified index.
func (wp *wallpaperPlugin) setWallpaperAt(imageIndex int) {

	// Check if we need to download the next page
	seenCount := len(wp.seenImages)
	localRecsCount := float64(len(wp.localImgRecs))
	threshold := int(math.Round(PrcntSeenTillDownload * localRecsCount))
	log.Printf("Seen count: %d, threshold: %d", seenCount, threshold)
	if seenCount > MinSeenImagesForDownload && seenCount >= threshold {
		wp.currentDownloadPage.Increment()
		wp.downloadAllImages(wp.currentDownloadPage.Value())
	}

	// Check if we have any downloaded images
	if len(wp.localImgRecs) == 0 {
		log.Println("No downloaded images found.")
		return
	}

	// Get the image file at the specified index
	safeIndex := (imageIndex + len(wp.localImgRecs)) % len(wp.localImgRecs)
	isi := wp.localImgRecs[safeIndex]
	imagePath := filepath.Join(wp.getDownloadedDir(), extractFilenameFromURL(isi.Path))

	// Set the wallpaper
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	if err := wp.os.setWallpaper(imagePath); err != nil {
		log.Printf("Failed to set wallpaper: %v", err)
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
		log.Println("No current image to delete.")
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
		log.Printf("Failed to delete blocked image: %v", err)
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
		log.Fatalf("Error creating downloaded images directory: %v", err)
	}
	err = os.MkdirAll(fittedDir, 0755)
	if err != nil {
		log.Fatalf("Error creating downloaded images directory: %v", err)
	}
}

// Activate starts the wallpaper rotation.
func (wp *wallpaperPlugin) Activate() {

	// Setup the downloaded images directories
	wp.setupImageDirs()

	// Goroutine to refresh images daily at midnight
	go func() {
		for {
			// Calculate time until next midnight
			now := time.Now()
			nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
			timeUntilMidnight := time.Until(nextMidnight)

			// Wait until midnight
			time.Sleep(timeUntilMidnight)

			// Refresh all images
			wp.refreshImages()
		}
	}()

	wp.SetShuffleImage(wp.cfg.GetImgShuffle()) // Set shuffle image preference
	wp.SetSmartFit(wp.cfg.GetSmartFit())       // Set smart fit preference

	// Refresh images and set the first wallpaper
	go wp.refreshImages()

	// Set the first wallpaper
	for i := 0; len(wp.localImgRecs) < MinLocalImageBeforePulse && i < MaxImageWaitRetry; i++ {
		// Wait for images to be downloaded
		time.Sleep(ImageWaitRetryDelay)
	}
	wp.imgPulseOp()

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
	if wp.ticker != nil {
		wp.ticker.Stop() // Stop the ticker
	}
	wp.interrupt.Set(true) // Interrupt any ongoing downloads
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
			log.Printf("Error getting desktop dimensions: %v", err)
			// Do NOT set a default resolution. Let the API handle it.
		} else {
			q.Set("atleast", fmt.Sprintf("%dx%d", width, height))
		}
	}
	url.RawQuery = q.Encode()
	return url
}

// checkWallhavenURL checks if the given URL is a valid wallhaven URL.
// Returns true if the URL is valid, false otherwise.
// Also returns an error if any.
func (wp *wallpaperPlugin) checkWallhavenURL(queryURL string) error {
	// Construct the API URL
	u, err := url.Parse(queryURL)
	if err != nil {
		return err
	}

	q := u.Query()
	q.Set("apikey", wp.cfg.GetWallhavenAPIKey()) // Add the API key

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

	// Fetch the JSON response
	resp, err := http.Get(u.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Parse the JSON response
	var response imgSrvcResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return err
	}

	if len(response.Data) == 0 {
		return fmt.Errorf("no suitable images found for your current resolution")
	}

	//
	return nil // success
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
		log.Println("No downloaded images found.")
		wp.downloadMutex.Unlock()
		return
	}

	randomIndex := 0
	if len(wp.localImgRecs) > 1 {
		randomIndex = rand.Intn(len(wp.localImgRecs) - 1)
		if randomIndex == wp.localImgIndex.Value() { // Ensure the new index is different from the current one
			randomIndex = rand.Intn(len(wp.localImgRecs) - 1)
		}
	}

	wp.prevLocalImgs = append(wp.prevLocalImgs, randomIndex)
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
	url, err := url.Parse(wp.getCurrentImage().ShortURL)
	if err != nil {
		log.Printf("Failed to parse URL: %v", err)
		return
	}
	wp.manager.OpenURL(url)
}

// RefreshImagesAndPulse refreshes the list of images and pulses the image.
func (wp *wallpaperPlugin) RefreshImagesAndPulse() {
	wp.refreshImages()
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

// CheckWallhavenURL checks if the given URL is a valid Wallhaven URL.
func (wp *wallpaperPlugin) CheckWallhavenURL(queryURL string) error {
	return wp.checkWallhavenURL(CovertToAPIURL(queryURL))
}

// GetWallhavenURL returns the Wallhaven URL for the given API URL.
func (wp *wallpaperPlugin) GetWallhavenURL(apiURL string) *url.URL {
	return wp.getWallhavenURL(apiURL)
}
