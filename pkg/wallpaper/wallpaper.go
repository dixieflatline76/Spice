package wallpaper

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/pkg"
	"github.com/dixieflatline76/Spice/ui"
	"golang.org/x/oauth2"
)

var (
	wpInstance *wallpaperPlugin
	wpOnce     sync.Once
)

// LoadPlugin loads the wallpaper plugin.
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

// wallpaperPlugin manages wallpaper rotation.
type wallpaperPlugin struct {
	os              OS
	imgProcessor    ImageProcessor
	cfg             *Config
	ticker          *time.Ticker
	downloadMutex   sync.Mutex // Protects currentPage, downloading, and download operations
	currentImage    ImgSrvcImage
	imageIndex      int
	downloadedDir   string
	downloadHistory []ImgSrvcImage      // Keep track of downloaded images to quickly access info like image web path
	seenHistory     map[string]bool     // Keep track of images that have been seen to trigger download of next page
	prevHistory     []int               // Keep track of every image set to support the previous wallpaper action
	imgPulseOp      func()              // Function to call to pulse the image
	currentPage     int                 //	Current page of images
	fitImage        bool                // Whether to fit the image to the desktop resolution
	shuffleImage    bool                // Whether to shuffle the images
	interrupt       bool                // Whether to interrupt the image download
	manager         pkg.UIPluginManager // Plugin manager
}

// InitPlugin initializes the wallpaper plugin.
func (wp *wallpaperPlugin) Init(manager pkg.UIPluginManager) {
	wp.manager = manager
	wp.cfg = GetConfig(manager.GetPreferences())
}

// Name returns the name of the plugin.
func (wp *wallpaperPlugin) Name() string {
	return "Wallpaper"
}

// RefreshImages downloads all images from the configured URLs.
func (wp *wallpaperPlugin) refreshImages() {
	// Clear the downloaded images directory
	wp.downloadMutex.Lock()
	wp.interrupt = true
	err := wp.cleanupImageCache()
	if err != nil {
		log.Printf("Error clearing downloaded images directory: %v", err)
	}
	wp.downloadHistory = []ImgSrvcImage{} // Clear the download history
	clear(wp.seenHistory)                 // Clear the seen history)
	wp.prevHistory = []int{}              // Clear the previous history
	wp.currentPage = 1                    // Reset to the first page
	wp.imageIndex = -1                    // Reset the image index
	wp.downloadMutex.Unlock()

	time.Sleep(time.Second * 3) // Sleep for 2 seconds to allow the download history to clear)
	wp.downloadMutex.Lock()
	wp.interrupt = false
	wp.downloadMutex.Unlock()

	wp.downloadAllImages(wp.currentPage)
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
	// Construct the API URL
	u, err := url.Parse(query.URL)
	if err != nil {
		log.Printf("Invalid Image URL: %v", err)
		return
	}

	q := u.Query()
	q.Set("apikey", wp.cfg.StringWithFallback(WallhavenAPIKeyPrefKey, "")) // Add the API key
	q.Set("page", fmt.Sprint(page))                                        // Add the page number

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
		if wp.interrupt {
			wp.downloadMutex.Unlock()
			log.Printf("Download of '%s' interrupted", query.Description)
			return // Interrupt download
		}
		wp.downloadImage(isi)
		wp.downloadMutex.Unlock()
		time.Sleep(500 * time.Millisecond)
	}
}

// extractFilenameFromURL extracts the filename from a URL.
func extractFilenameFromURL(url string) string {
	lastSlashIndex := strings.LastIndex(url, "/")
	if lastSlashIndex == -1 || lastSlashIndex == len(url)-1 {
		return "" // Handle cases where there's no slash or it's at the end
	}
	return url[lastSlashIndex+1:]
}

// getDownloadedDir returns the downloaded images directory.
func (wp *wallpaperPlugin) getDownloadedDir() string {
	if wp.fitImage {
		return filepath.Join(wp.downloadedDir, FittedImgDir) // Use a sub directory for fitted images
	}
	return wp.downloadedDir
}

// downloadImage downloads a single image.
func (wp *wallpaperPlugin) downloadImage(isi ImgSrvcImage) (string, error) {

	// Check if the image has already been downloaded
	tempFile := filepath.Join(wp.getDownloadedDir(), extractFilenameFromURL(isi.Path))
	_, err := os.Stat(tempFile)
	if !os.IsNotExist(err) {
		wp.downloadHistory = append(wp.downloadHistory, isi)
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

	if wp.fitImage {
		// Decode the image
		wp.downloadMutex.Unlock() // Unlock before fitting, decoding and encoding
		img, _, err := wp.imgProcessor.DecodeImage(imgBytes, isi.FileType)
		if err != nil {
			log.Printf("Failed to decode image: %v", err)
			return "", fmt.Errorf("failed to decode image: %v", err)
		}
		processedImg, err := wp.imgProcessor.FitImage(img)
		if err != nil {
			// Failed to fit image, return the error and continue
			log.Printf("Failed to fit image: %v", err)
			return "", err
		}

		// Encode the processed image
		processedImgBytes, err := wp.imgProcessor.EncodeImage(processedImg, isi.FileType)
		if err != nil {
			log.Printf("Failed to encode image: %v", err)
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

	wp.downloadHistory = append(wp.downloadHistory, isi)
	return tempFile, nil
}

// setWallpaperAt sets the wallpaper at the specified index.
func (wp *wallpaperPlugin) setWallpaperAt(dirIndex int) {

	// Check if we need to download the next page
	if len(wp.seenHistory) > PageDownloadOffset && len(wp.seenHistory) >= (len(wp.downloadHistory)-PageDownloadOffset) {
		wp.currentPage++
		currentPageToDownload := wp.currentPage

		wp.downloadAllImages(currentPageToDownload)
		time.Sleep(3 * time.Second) // Wait 3 seconds before setting the wallpaper
	}

	retries := 2
	for retries > 0 {
		if len(wp.downloadHistory) == 0 {
			log.Println("No downloaded images found. Retrying...")
			time.Sleep(3 * time.Second) // Wait 3 seconds before trying again
			retries--
		} else {
			break
		}
	}

	if len(wp.downloadHistory) == 0 {
		log.Println("No downloaded images found after retries.")
		return
	}

	// Get the image file at the specified index
	safeIndex := (dirIndex + len(wp.downloadHistory)) % len(wp.downloadHistory)
	isi := wp.downloadHistory[safeIndex]
	imagePath := filepath.Join(wp.getDownloadedDir(), extractFilenameFromURL(isi.Path))

	// Set the wallpaper
	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()

	if err := wp.os.setWallpaper(imagePath); err != nil {
		log.Printf("Failed to set wallpaper: %v", err)
		return // Or handle the error in a way that makes sense for your application
	}

	// Update current image and index under lock using temporary variables
	wp.currentImage = wp.downloadHistory[safeIndex]
	wp.imageIndex = safeIndex
	wp.seenHistory[imagePath] = true
}

// fileInfo struct to store file path and modification time.
type fileInfo struct {
	path    string
	modTime time.Time
}

// isImageFile checks if a file has a common image extension.
func isImageFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif"
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
	excess := len(files) - DefaultWallpaperCacheSize
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
	wp.downloadedDir = filepath.Join(getTempDir(), strings.ToLower(config.AppName)+"_downloads")
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

	wp.SetShuffleImage(wp.cfg.BoolWithFallback(ImgShufflePrefKey, false)) // Set shuffle image preference
	wp.SetSmartFit(wp.cfg.BoolWithFallback(SmartFitPrefKey, false))       // Set smart fit preference

	// Refresh images and set the first wallpaper
	go wp.refreshImages()
	time.Sleep(5 * time.Second)
	wp.imgPulseOp()

	// Start the wallpaper rotation ticker
	wp.ChangeWallpaperFrequency(Frequency(wp.cfg.IntWithFallback(WallpaperChgFreqPrefKey, int(FrequencyHourly)))) // Set wallpaper change frequency preference
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
	wp.interrupt = true // Interrupt any ongoing downloads
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
	q.Set("apikey", wp.cfg.StringWithFallback(WallhavenAPIKeyPrefKey, "")) // Add the API key

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

// getTempDir returns the temporary directory.
func getTempDir() string {
	tempDir := os.TempDir()
	return tempDir
}

// SetNextWallpaper sets the next wallpaper in the list.
func (wp *wallpaperPlugin) setNextWallpaper() {
	wp.downloadMutex.Lock()
	wp.imageIndex++ // Increment the image index
	tempIndex := wp.imageIndex
	wp.prevHistory = append(wp.prevHistory, tempIndex)
	wp.downloadMutex.Unlock()

	wp.setWallpaperAt(tempIndex)
}

// SetPreviousWallpaper sets the previous wallpaper in the list.
func (wp *wallpaperPlugin) SetPreviousWallpaper() {
	wp.downloadMutex.Lock()
	if len(wp.prevHistory) <= 1 {
		wp.downloadMutex.Unlock()
		wp.manager.NotifyUser("No Previous Wallpaper", "You are at the beginning.")
		return // No previous history
	}
	wp.prevHistory = wp.prevHistory[:len(wp.prevHistory)-1] // Remove the last element
	tempIndex := wp.prevHistory[len(wp.prevHistory)-1]      // Get the last element
	wp.downloadMutex.Unlock()

	wp.setWallpaperAt(tempIndex)
}

// SetRandomWallpaper sets a random wallpaper from the list.
func (wp *wallpaperPlugin) setRandomWallpaper() {
	wp.downloadMutex.Lock()
	if len(wp.downloadHistory) == 0 {
		log.Println("No downloaded images found.")
		wp.downloadMutex.Unlock()
		return
	}

	randomIndex := 0
	if len(wp.downloadHistory) > 1 {
		randomIndex = rand.Intn(len(wp.downloadHistory) - 1)
	}

	wp.prevHistory = append(wp.prevHistory, randomIndex)
	wp.downloadMutex.Unlock()

	wp.setWallpaperAt(randomIndex)
}

// ChangeWallpaperFrequency changes the wallpaper frequency.
func (wp *wallpaperPlugin) ChangeWallpaperFrequency(newFrequency Frequency) {
	wp.changeFrequency(newFrequency)
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

// ViewCurrentImageOnWeb opens the current wallpaper image in the default web browser.
func (wp *wallpaperPlugin) ViewCurrentImageOnWeb() {
	url, err := url.Parse(wp.getCurrentImage().ShortURL)
	if err != nil {
		log.Printf("Failed to parse URL: %v", err)
		return
	}
	wp.manager.OpenURL(url)
}

// RefreshImages discards all downloaded images and fetches new ones.
func (wp *wallpaperPlugin) RefreshImages() {
	wp.refreshImages()
	go func() {
		time.Sleep(3 * time.Second)
		wp.SetNextWallpaper()
	}()
}

// SetWallhavenAPIKey sets the Wallhaven API key.
func (wp *wallpaperPlugin) SetWallhavenAPIKey(apiKey string) {
	wp.cfg.SetString(WallhavenAPIKeyPrefKey, apiKey)
}

// SetSmartFit enables or disables smart cropping.
func (wp *wallpaperPlugin) SetSmartFit(enabled bool) {
	wp.cfg.SetBool(SmartFitPrefKey, enabled)
	wp.fitImage = enabled
}

// SetShuffleImage enables or disables image shuffling.
func (wp *wallpaperPlugin) SetShuffleImage(enabled bool) {
	// Set the shuffle image preference and update the image pulse operation
	wp.shuffleImage = enabled
	wp.cfg.SetBool(ImgShufflePrefKey, enabled)

	wp.downloadMutex.Lock()
	defer wp.downloadMutex.Unlock()
	log.Print("Shuffle Image called")
	if wp.shuffleImage {
		wp.imgPulseOp = wp.SetRandomWallpaper
		wp.manager.NotifyUser("Wallpaper Shuffling", "Enabled")
	} else {
		wp.imgPulseOp = wp.setNextWallpaper
		wp.manager.NotifyUser("Wallpaper Shuffling", "Disabled")
	}
}

// CheckWallhavenAPIKey checks if the given API key is valid.
func CheckWallhavenAPIKey(apiKey string) error {
	// 1. Configure the OAuth2 HTTP Client
	// Wallhaven uses API keys as Bearer tokens, which OAuth2 handles nicely.
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: apiKey},
	)
	client := oauth2.NewClient(context.Background(), ts)

	// 2. Make a Request to a Protected Endpoint
	// Choose an endpoint that requires authentication.  The 'account' endpoint is a good option.
	req, err := http.NewRequestWithContext(context.Background(), "GET", "https://wallhaven.cc/api/v1/settings?apikey="+apiKey, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// 3. Execute the Request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	// 4. Check the Response Status Code
	if resp.StatusCode == http.StatusOK {
		return nil // Success!
	}
	return fmt.Errorf("API key is invalid")
}

// CovertToAPIURL converts the given URL to a Wallhaven API URL.
func CovertToAPIURL(queryURL string) string {

	// Convert to API URL
	queryURL = strings.Replace(queryURL, "https://wallhaven.cc/search?", "https://wallhaven.cc/api/v1/search?", 1)

	u, err := url.Parse(queryURL)
	if err != nil {
		// Not a valid URL
		return queryURL
	}

	q := u.Query()

	// Remove API key
	if q.Has("apikey") {
		q.Del("apikey")
	}

	// Remove page
	if q.Has("page") {
		q.Del("page")
	}

	u.RawQuery = q.Encode()
	return u.String()
}

// CheckWallhavenURL checks if the given URL is a valid Wallhaven URL.
func (wp *wallpaperPlugin) CheckWallhavenURL(queryURL string) error {
	return wp.checkWallhavenURL(CovertToAPIURL(queryURL))
}

// GetWallhavenURL returns the Wallhaven URL for the given API URL.
func (wp *wallpaperPlugin) GetWallhavenURL(apiURL string) *url.URL {
	return wp.getWallhavenURL(apiURL)
}
