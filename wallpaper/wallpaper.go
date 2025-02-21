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

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/config"
	"golang.org/x/oauth2"
)

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

// NotifierFunc is a function that notifies the user.
type NotifierFunc func(title, message string)

// wallpaperService manages wallpaper rotation.
type wallpaperService struct {
	os              OS
	imgProcessor    ImageProcessor
	cfg             *Config
	ticker          *time.Ticker
	downloadMutex   sync.Mutex // Protects currentPage, downloading, and download operations
	currentImage    ImgSrvcImage
	imageIndex      int
	downloadedDir   string
	downloadHistory []ImgSrvcImage  // Keep track of downloaded images to quickly access info like image web path
	seenHistory     map[string]bool // Keep track of images that have been seen to trigger download of next page
	prevHistory     []int           // Keep track of every image set to support the previous wallpaper action
	imgPulseOp      func()          // Function to call to pulse the image
	currentPage     int             //	Current page of images
	fitImage        bool            // Whether to fit the image to the desktop resolution
	shuffleImage    bool            // Whether to shuffle the images
	interrupt       bool            // Whether to interrupt the image download
	notifier        NotifierFunc    // Notifier function to show alerts or log events
}

// ImgSrvcImage represents an image from the image service.
type ImgSrvcImage struct {
	Path     string `json:"path"`
	ID       string `json:"id"`
	ShortURL string `json:"short_url"`
	FileType string `json:"file_type"`
	Ratio    string `json:"ratio"`
	Thumbs   Thumbs `json:"thumbs"`
}

// Thumbs represents the different sizes of thumbnails for an image.
type Thumbs struct {
	Large    string `json:"large"`
	Original string `json:"original"`
	Small    string `json:"small"`
}

// imgSrvcResponse represents a response from the image service.
type imgSrvcResponse struct {
	Data []ImgSrvcImage `json:"data"`
	Meta struct {
		LastPage int `json:"meta"`
	} `json:"meta"`
}

var (
	wsInstance *wallpaperService
	wsOnce     sync.Once
)

// RefreshImages downloads all images from the configured URLs.
func (ws *wallpaperService) refreshImages() {
	// Clear the downloaded images directory
	ws.downloadMutex.Lock()
	ws.interrupt = true
	err := ws.cleanupImageCache()
	if err != nil {
		log.Printf("Error clearing downloaded images directory: %v", err)
	}
	ws.downloadHistory = []ImgSrvcImage{} // Clear the download history
	clear(ws.seenHistory)                 // Clear the seen history)
	ws.prevHistory = []int{}              // Clear the previous history
	ws.currentPage = 1                    // Reset to the first page
	ws.imageIndex = -1                    // Reset the image index
	ws.downloadMutex.Unlock()

	time.Sleep(time.Second * 3) // Sleep for 2 seconds to allow the download history to clear)
	ws.downloadMutex.Lock()
	ws.interrupt = false
	ws.downloadMutex.Unlock()

	ws.downloadAllImages(ws.currentPage)
}

// downloadAllImages downloads images from all active URLs for the specified page.
func (ws *wallpaperService) downloadAllImages(page int) {
	var message string
	for _, query := range ws.cfg.ImageQueries {
		if query.Active {
			go ws.downloadImagesForURL(query, page)
			message += fmt.Sprintf("[%s]\n", query.Description)
		}
	}
	ws.notifier("Downloading Images From:", message)
}

// downloadImagesForURL downloads images from the given URL for the specified page.
func (ws *wallpaperService) downloadImagesForURL(query ImageQuery, page int) {
	// Construct the API URL
	u, err := url.Parse(query.URL)
	if err != nil {
		log.Printf("Invalid Image URL: %v", err)
		return
	}

	q := u.Query()
	q.Set("apikey", ws.cfg.StringWithFallback(WallhavenAPIKeyPrefKey, "")) // Add the API key
	q.Set("page", fmt.Sprint(page))                                        // Add the page number

	// Check for resolutions or atleast parameters
	if !q.Has("resolutions") && !q.Has("atleast") {
		width, height, err := ws.os.getDesktopDimension()
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
		ws.downloadMutex.Lock()
		if ws.interrupt {
			ws.downloadMutex.Unlock()
			log.Printf("Download of '%s' interrupted", query.Description)
			return // Interrupt download
		}
		ws.downloadImage(isi)
		ws.downloadMutex.Unlock()
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
func (ws *wallpaperService) getDownloadedDir() string {
	if ws.fitImage {
		return filepath.Join(ws.downloadedDir, FittedImgDir) // Use a sub directory for fitted images
	}
	return ws.downloadedDir
}

// downloadImage downloads a single image.
func (ws *wallpaperService) downloadImage(isi ImgSrvcImage) (string, error) {

	// Check if the image has already been downloaded
	tempFile := filepath.Join(ws.getDownloadedDir(), extractFilenameFromURL(isi.Path))
	_, err := os.Stat(tempFile)
	if !os.IsNotExist(err) {
		ws.downloadHistory = append(ws.downloadHistory, isi)
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

	if ws.fitImage {
		// Decode the image
		ws.downloadMutex.Unlock() // Unlock before fitting, decoding and encoding
		img, _, err := ws.imgProcessor.DecodeImage(imgBytes, isi.FileType)
		if err != nil {
			log.Printf("Failed to decode image: %v", err)
			return "", fmt.Errorf("failed to decode image: %v", err)
		}
		processedImg, err := ws.imgProcessor.FitImage(img)
		if err != nil {
			// Failed to fit image, return the error and continue
			log.Printf("Failed to fit image: %v", err)
			return "", err
		}

		// Encode the processed image
		processedImgBytes, err := ws.imgProcessor.EncodeImage(processedImg, isi.FileType)
		if err != nil {
			log.Printf("Failed to encode image: %v", err)
			return "", fmt.Errorf("failed to encode image: %v", err)
		}
		imgBytes = processedImgBytes
		ws.downloadMutex.Lock() // Lock before saving the image
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

	ws.downloadHistory = append(ws.downloadHistory, isi)
	return tempFile, nil
}

// setWallpaperAt sets the wallpaper at the specified index.
func (ws *wallpaperService) setWallpaperAt(dirIndex int) {

	// Check if we need to download the next page
	if len(ws.seenHistory) > PageDownloadOffset && len(ws.seenHistory) >= (len(ws.downloadHistory)-PageDownloadOffset) {
		ws.currentPage++
		currentPageToDownload := ws.currentPage

		ws.downloadAllImages(currentPageToDownload)
		time.Sleep(3 * time.Second) // Wait 3 seconds before setting the wallpaper
	}

	retries := 2
	for retries > 0 {
		if len(ws.downloadHistory) == 0 {
			log.Println("No downloaded images found. Retrying...")
			time.Sleep(3 * time.Second) // Wait 3 seconds before trying again
			retries--
		} else {
			break
		}
	}

	if len(ws.downloadHistory) == 0 {
		log.Println("No downloaded images found after retries.")
		return
	}

	// Get the image file at the specified index
	safeIndex := (dirIndex + len(ws.downloadHistory)) % len(ws.downloadHistory)
	isi := ws.downloadHistory[safeIndex]
	imagePath := filepath.Join(ws.getDownloadedDir(), extractFilenameFromURL(isi.Path))

	// Set the wallpaper
	ws.downloadMutex.Lock()
	defer ws.downloadMutex.Unlock()

	if err := ws.os.setWallpaper(imagePath); err != nil {
		log.Printf("Failed to set wallpaper: %v", err)
		return // Or handle the error in a way that makes sense for your application
	}

	// Update current image and index under lock using temporary variables
	ws.currentImage = ws.downloadHistory[safeIndex]
	ws.imageIndex = safeIndex
	ws.seenHistory[imagePath] = true
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
func (ws *wallpaperService) cleanupImageCache() error {
	// 1. Collect all image files with their modification times.
	var files []fileInfo
	for _, dir := range []string{ws.downloadedDir, filepath.Join(ws.downloadedDir, FittedImgDir)} {
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
func (ws *wallpaperService) setupImageDirs() {
	// Create the downloaded images directory if it doesn't exist
	ws.downloadedDir = filepath.Join(getTempDir(), strings.ToLower(config.AppName)+"_downloads")
	fittedDir := filepath.Join(ws.downloadedDir, FittedImgDir)
	err := os.MkdirAll(ws.downloadedDir, 0755)
	if err != nil {
		log.Fatalf("Error creating downloaded images directory: %v", err)
	}
	err = os.MkdirAll(fittedDir, 0755)
	if err != nil {
		log.Fatalf("Error creating downloaded images directory: %v", err)
	}
}

// Start starts the wallpaper rotation service.
func (ws *wallpaperService) Start() {

	// Setup the downloaded images directories
	ws.setupImageDirs()

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
			ws.refreshImages()
		}
	}()

	SetShuffleImage(ws.cfg.BoolWithFallback(ImgShufflePrefKey, false)) // Set shuffle image preference
	SetSmartFit(ws.cfg.BoolWithFallback(SmartFitPrefKey, false))       // Set smart fit preference

	// Refresh images and set the first wallpaper
	go ws.refreshImages()
	time.Sleep(5 * time.Second)
	ws.imgPulseOp()

	// Start the wallpaper rotation ticker
	ChangeWallpaperFrequency(Frequency(ws.cfg.IntWithFallback(WallpaperChgFreqPrefKey, int(FrequencyHourly)))) // Set wallpaper change frequency preference
}

// changeFrequency changes the wallpaper change frequency.
func (ws *wallpaperService) changeFrequency(newFrequency Frequency) {
	ws.downloadMutex.Lock()
	defer ws.downloadMutex.Unlock()

	// Stop the ticker
	if ws.ticker != nil {
		ws.ticker.Stop()
	}

	// Check if the frequency is set to never
	if newFrequency == FrequencyNever {
		ws.notifier("Wallpaper Change", "Disabled")
		return
	}

	ws.ticker = time.NewTicker(newFrequency.Duration())

	// Reset the ticker channel to immediately trigger
	go func() {
		for range ws.ticker.C {
			ws.imgPulseOp()
		}
	}()
	ws.notifier("Wallpaper Change", newFrequency.String())
}

// Stop stops the wallpaper rotation service and triggers necessary cleanup.
func (ws *wallpaperService) Stop() {
	if ws.ticker != nil {
		ws.ticker.Stop() // Stop the ticker
	}
	ws.interrupt = true // Interrupt any ongoing downloads
}

// GetCurrentImage returns the current image.
func (ws *wallpaperService) getCurrentImage() ImgSrvcImage {
	ws.downloadMutex.Lock()
	defer ws.downloadMutex.Unlock()

	return ws.currentImage
}

// getWallhavenURL returns the wallhaven URL for the given API URL.
func (ws *wallpaperService) getWallhavenURL(apiURL string) *url.URL {
	// Convert to API URL
	urlStr := strings.Replace(apiURL, "https://wallhaven.cc/api/v1/search?", "https://wallhaven.cc/search?", 1)
	url, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}

	q := url.Query()

	// Check for resolutions or atleast parameters
	if !q.Has("resolutions") && !q.Has("atleast") {
		width, height, err := ws.os.getDesktopDimension()
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
func (ws *wallpaperService) checkWallhavenURL(queryURL string) error {
	// Construct the API URL
	u, err := url.Parse(queryURL)
	if err != nil {
		return err
	}

	q := u.Query()
	q.Set("apikey", ws.cfg.StringWithFallback(WallhavenAPIKeyPrefKey, "")) // Add the API key

	// Check for resolutions or atleast parameters
	if !q.Has("resolutions") && !q.Has("atleast") {
		width, height, err := ws.os.getDesktopDimension()
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
func (ws *wallpaperService) SetNextWallpaper() {
	ws.downloadMutex.Lock()
	ws.imageIndex++ // Increment the image index
	tempIndex := ws.imageIndex
	ws.prevHistory = append(ws.prevHistory, tempIndex)
	ws.downloadMutex.Unlock()

	ws.setWallpaperAt(tempIndex)
}

// SetPreviousWallpaper sets the previous wallpaper in the list.
func (ws *wallpaperService) SetPreviousWallpaper() {
	ws.downloadMutex.Lock()
	if len(ws.prevHistory) <= 1 {
		ws.downloadMutex.Unlock()
		ws.notifier("No Previous Wallpaper", "You are at the beginning.")
		return // No previous history
	}
	ws.prevHistory = ws.prevHistory[:len(ws.prevHistory)-1] // Remove the last element
	tempIndex := ws.prevHistory[len(ws.prevHistory)-1]      // Get the last element
	ws.downloadMutex.Unlock()

	ws.setWallpaperAt(tempIndex)
}

// SetRandomWallpaper sets a random wallpaper from the list.
func (ws *wallpaperService) SetRandomWallpaper() {
	ws.downloadMutex.Lock()
	if len(ws.downloadHistory) == 0 {
		log.Println("No downloaded images found.")
		ws.downloadMutex.Unlock()
		return
	}

	randomIndex := 0
	if len(ws.downloadHistory) > 1 {
		randomIndex = rand.Intn(len(ws.downloadHistory) - 1)
	}

	ws.prevHistory = append(ws.prevHistory, randomIndex)
	ws.downloadMutex.Unlock()

	ws.setWallpaperAt(randomIndex)
}

// StartWallpaperService starts the wallpaper service.
func StartWallpaperService(prefs fyne.Preferences, notifiers ...NotifierFunc) {
	ws := getWallpaperService(GetConfig(prefs))
	if len(notifiers) > 0 {
		ws.notifier = notifiers[0] // TODO - Support multiple notifiers
	}
	ws.Start()
}

// ChangeWallpaperFrequency changes the wallpaper frequency.
func ChangeWallpaperFrequency(newFrequency Frequency) {
	ws := getWallpaperService(nil)
	ws.changeFrequency(newFrequency)
}

// SetNextWallpaper sets the next wallpaper, will respect shuffle toggle
func SetNextWallpaper() {
	ws := getWallpaperService(nil)
	ws.imgPulseOp()
}

// SetPreviousWallpaper sets the previous wallpaper.
func SetPreviousWallpaper() {
	ws := getWallpaperService(nil)
	ws.SetPreviousWallpaper()
}

// SetRandomWallpaper sets a random wallpaper.
func SetRandomWallpaper() {
	ws := getWallpaperService(nil)
	ws.SetRandomWallpaper()
}

// StopWallpaperService stops the wallpaper service.
func StopWallpaperService() {
	ws := getWallpaperService(nil)
	ws.Stop()
}

// GetCurrentImage returns the current wallpaper image information.
func GetCurrentImage() ImgSrvcImage {
	ws := getWallpaperService(nil)
	return ws.getCurrentImage()
}

// ViewCurrentImageOnWeb opens the current wallpaper image in the default web browser.
func ViewCurrentImageOnWeb(app fyne.App) {
	ws := getWallpaperService(nil)
	url, err := url.Parse(ws.getCurrentImage().ShortURL)
	if err != nil {
		log.Printf("Failed to parse URL: %v", err)
		return
	}
	app.OpenURL(url)
}

// RefreshImages discards all downloaded images and fetches new ones.
func RefreshImages() {
	ws := getWallpaperService(nil)
	ws.refreshImages()
	go func() {
		time.Sleep(3 * time.Second)
		ws.SetNextWallpaper()
	}()
}

// SetWallhavenAPIKey sets the Wallhaven API key.
func SetWallhavenAPIKey(apiKey string) {
	ws := getWallpaperService(nil)
	ws.cfg.SetString(WallhavenAPIKeyPrefKey, apiKey)
}

// SetSmartFit enables or disables smart cropping.
func SetSmartFit(enabled bool) {
	ws := getWallpaperService(nil)
	ws.cfg.SetBool(SmartFitPrefKey, enabled)
	ws.fitImage = enabled
}

// SetShuffleImage enables or disables image shuffling.
func SetShuffleImage(enabled bool) {
	ws := getWallpaperService(nil)

	ws.shuffleImage = enabled
	ws.cfg.SetBool(ImgShufflePrefKey, enabled)

	ws.downloadMutex.Lock()
	defer ws.downloadMutex.Unlock()
	if ws.shuffleImage {
		ws.imgPulseOp = ws.SetRandomWallpaper
		ws.notifier("Wallpaper Shuffling", "Enabled")
	} else {
		ws.imgPulseOp = ws.SetNextWallpaper
		ws.notifier("Wallpaper Shuffling", "Disabled")
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
func CheckWallhavenURL(queryURL string) error {
	ws := getWallpaperService(nil)
	return ws.checkWallhavenURL(CovertToAPIURL(queryURL))
}

// GetWallhavenURL returns the Wallhaven URL for the given API URL.
func GetWallhavenURL(apiURL string) *url.URL {
	ws := getWallpaperService(nil)
	return ws.getWallhavenURL(apiURL)
}
