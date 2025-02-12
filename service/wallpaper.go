package service

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
	FitImage(img image.Image) (image.Image, error)              // Fit the image to the desktop resolution.
	DecodeImage(imgBytes []byte) (image.Image, string, error)   // Decode an image from a byte slice.
	EncodeImage(img image.Image, format string) ([]byte, error) // Encode an image to a byte slice.
}

// wallpaperService manages wallpaper rotation.
type wallpaperService struct {
	os              OS
	imgProcessor    ImageProcessor
	cfg             *config.Config
	prefs           fyne.Preferences
	ticker          *time.Ticker
	downloadMutex   sync.Mutex // Protects currentPage, downloading, and download operations
	currentImage    ImgSrvcImage
	imageIndex      int
	downloadedDir   string
	downloadHistory map[string]ImgSrvcImage
	currentPage     int
	fitImage        bool
	interrupt       bool
}

// ImgSrvcImage represents an image from the image service.
type ImgSrvcImage struct {
	Path     string `json:"path"`
	ID       string `json:"id"`
	ShortURL string `json:"short_url"`
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
	once       sync.Once
)

// RefreshImages downloads all images from the configured URLs.
func (ws *wallpaperService) refreshImages() {
	// Clear the downloaded images directory
	ws.downloadMutex.Lock()
	ws.interrupt = true
	err := ws.clearDownloadedImagesDir()
	if err != nil {
		log.Printf("Error clearing downloaded images directory: %v", err)
	}
	clear(ws.downloadHistory) // Clear the download history
	ws.currentPage = 1        // Reset to the first page
	ws.imageIndex = -1        // Reset the image index
	ws.downloadMutex.Unlock()

	time.Sleep(time.Second * 2) // Sleep for 2 seconds to allow the download history to clear)
	ws.downloadMutex.Lock()
	ws.interrupt = false
	ws.downloadMutex.Unlock()

	ws.downloadAllImages(ws.currentPage)
}

// downloadAllImages downloads images from all active URLs for the specified page.
func (ws *wallpaperService) downloadAllImages(page int) {
	for _, url := range ws.cfg.ImageQueries {
		if url.Active {
			go ws.downloadImagesForURL(url.URL, page)
		}
	}
}

// downloadImagesForURL downloads images from the given URL for the specified page.
func (ws *wallpaperService) downloadImagesForURL(imgSrvcURL string, page int) {
	// Construct the API URL
	u, err := url.Parse(imgSrvcURL)
	if err != nil {
		log.Printf("Invalid Image URL: %v", err)
		return
	}

	q := u.Query()
	q.Set("apikey", ws.prefs.StringWithFallback(WallhavenAPIKeyPrefKey, "")) // Add the API key
	q.Set("page", fmt.Sprint(page))                                          // Add the page number

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
	for _, img := range response.Data {
		ws.downloadMutex.Lock()
		if ws.interrupt {
			ws.downloadMutex.Unlock()
			log.Printf("Download interrupted returning func")
			return // Interrupt download
		}
		ws.downloadImage(img)
		ws.downloadMutex.Unlock()
		time.Sleep(500 * time.Millisecond)
	}
}

// downloadImage downloads a single image.
func (ws *wallpaperService) downloadImage(img ImgSrvcImage) (string, error) {
	resp, err := http.Get(img.Path)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %v", err)
	}
	defer resp.Body.Close()

	// Read the image bytes
	imgBytes, err := io.ReadAll(resp.Body) // Read all bytes at once
	if err != nil {
		return "", fmt.Errorf("failed to read image bytes: %w", err)
	}

	// Create a file name using the timestamp and the image ID
	timestamp := time.Now().UnixNano()
	tempFile := filepath.Join(ws.downloadedDir, fmt.Sprintf("%d_%s.jpg", timestamp, img.ID))

	if ws.fitImage {
		// Decode the image
		img, ext, err := ws.imgProcessor.DecodeImage(imgBytes)
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
		processedImgBytes, err := ws.imgProcessor.EncodeImage(processedImg, ext)
		if err != nil {
			log.Printf("Failed to encode image: %v", err)
			return "", fmt.Errorf("failed to encode image: %v", err)
		}
		imgBytes = processedImgBytes
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

	// Store the downloaded image in the history
	//ws.downloadMutex.Lock()
	//defer ws.downloadMutex.Unlock()
	ws.downloadHistory[outFile.Name()] = img
	return tempFile, nil
}

// setWallpaperAt sets the wallpaper at the specified index.
func (ws *wallpaperService) setWallpaperAt(dirIndex int) {
	// Get a list of all downloaded image files
	imageFiles, err := os.ReadDir(ws.downloadedDir)
	if err != nil {
		log.Printf("Failed to read downloaded images directory: %v", err)
		return
	}

	// Check if we need to download the next page
	if dirIndex >= len(imageFiles)-1 {
		ws.currentPage++
		currentPageToDownload := ws.currentPage
		ws.downloadAllImages(currentPageToDownload)
		time.Sleep(3 * time.Second)

		// Reload imageFiles after potential download
		imageFiles, err = os.ReadDir(ws.downloadedDir)
		if err != nil {
			log.Printf("Failed to read downloaded images directory: %v", err)
			return
		}
	}

	retries := 3
	for retries > 0 {
		if len(imageFiles) == 0 {
			log.Println("No downloaded images found. Retrying...")
			time.Sleep(3 * time.Second)                    // Wait 3 seconds before trying again
			imageFiles, err = os.ReadDir(ws.downloadedDir) // Reload imageFiles
			if err != nil {
				log.Printf("Failed to read downloaded images directory: %v", err)
				return
			}
			retries--
		} else {
			break
		}
	}

	if len(imageFiles) == 0 {
		log.Println("No downloaded images found after retries.")
		return
	}

	// Get the image file at the specified index
	safeIndex := (dirIndex + len(imageFiles)) % len(imageFiles)
	imageFile := imageFiles[safeIndex]
	imagePath := filepath.Join(ws.downloadedDir, imageFile.Name())

	// Set the wallpaper
	ws.downloadMutex.Lock()
	defer ws.downloadMutex.Unlock()

	if err = ws.os.setWallpaper(imagePath); err != nil {
		log.Printf("Failed to set wallpaper: %v", err)
		return // Or handle the error in a way that makes sense for your application
	}

	// Update current image and index under lock using temporary variables
	ws.currentImage = ws.downloadHistory[imagePath]
	ws.imageIndex = safeIndex
}

// clearDownloadedImagesDir clears the downloaded images directory.
func (ws *wallpaperService) clearDownloadedImagesDir() error {
	files, err := os.ReadDir(ws.downloadedDir)
	if err != nil {
		return fmt.Errorf("failed to read downloaded images directory: %v", err)
	}

	for _, file := range files {
		err = os.Remove(filepath.Join(ws.downloadedDir, file.Name()))
		if err != nil {
			return fmt.Errorf("failed to remove image file: %v", err)
		}
	}

	return nil
}

// Start starts the wallpaper rotation service.
func (ws *wallpaperService) Start() {
	ws.os.getDesktopDimension()

	// Create the downloaded images directory if it doesn't exist
	ws.downloadedDir = filepath.Join(getTempDir(), strings.ToLower(config.ServiceName)+"_downloads")
	err := os.MkdirAll(ws.downloadedDir, 0755)
	if err != nil {
		log.Fatalf("Error creating downloaded images directory: %v", err)
	}

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

	// Refresh images and set the first wallpaper
	go ws.refreshImages()
	time.Sleep(5 * time.Second)
	ws.SetNextWallpaper()

	// Start the wallpaper rotation ticker
	freqInt := ws.prefs.IntWithFallback(WallpaperChgFreqPrefKey, int(FrequencyHourly))
	frequency := Frequency(freqInt)
	ws.ticker = time.NewTicker(frequency.Duration())
	for range ws.ticker.C {
		ws.SetNextWallpaper()
	}
}

// changeFrequency changes the wallpaper change frequency.
func (ws *wallpaperService) changeFrequency(newFrequency time.Duration) {
	ws.downloadMutex.Lock()
	defer ws.downloadMutex.Unlock()

	if ws.ticker != nil {
		ws.ticker.Stop()
		ws.ticker = time.NewTicker(newFrequency)

		// Reset the ticker channel to immediately trigger
		go func() {
			for range ws.ticker.C {
				ws.SetNextWallpaper()
			}
		}()

		log.Printf("Wallpaper change frequency updated to: %v", newFrequency)
	} else {
		log.Println("Ticker not initialized. Start the service before changing frequency.")
	}
}

// Stop stops the wallpaper rotation service and triggers necessary cleanup.
func (ws *wallpaperService) Stop() {
	if ws.ticker != nil {
		ws.ticker.Stop()
	}
}

// GetCurrentImage returns the current image.
func (ws *wallpaperService) getCurrentImage() ImgSrvcImage {
	ws.downloadMutex.Lock()
	defer ws.downloadMutex.Unlock()

	return ws.currentImage
}

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
	q.Set("apikey", ws.prefs.StringWithFallback(WallhavenAPIKeyPrefKey, "")) // Add the API key

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

// clear clears the given map.
func clear(m map[string]ImgSrvcImage) {
	for k := range m {
		delete(m, k)
	}
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
	ws.downloadMutex.Unlock()

	ws.setWallpaperAt(tempIndex)
}

// SetPreviousWallpaper sets the previous wallpaper in the list.
func (ws *wallpaperService) SetPreviousWallpaper() {
	ws.downloadMutex.Lock()
	ws.imageIndex-- // Decrement the image index
	tempIndex := ws.imageIndex
	ws.downloadMutex.Unlock()

	ws.setWallpaperAt(tempIndex)
}

// SetRandomWallpaper sets a random wallpaper from the list.
func (ws *wallpaperService) SetRandomWallpaper() {
	ws.downloadMutex.Lock()
	// Get a list of all downloaded image files
	imageFiles, err := os.ReadDir(ws.downloadedDir)
	if err != nil {
		log.Printf("Failed to read downloaded images directory: %v", err)
		ws.downloadMutex.Unlock()
		return
	}

	if len(imageFiles) == 0 {
		log.Println("No downloaded images found.")
		ws.downloadMutex.Unlock()
		return
	}
	randomIndex := rand.Intn(len(imageFiles) - 1)
	ws.downloadMutex.Unlock()

	ws.setWallpaperAt(randomIndex)
}

// StartWallpaperService starts the wallpaper service.
func StartWallpaperService(cfg *config.Config, prefs fyne.Preferences) {
	ws := getWallpaperService(cfg, prefs)
	ws.Start()
}

// ChangeWallpaperFrequency changes the wallpaper frequency.
func ChangeWallpaperFrequency(newFrequency time.Duration) {
	ws := getWallpaperService(nil, nil)
	ws.changeFrequency(newFrequency)
}

// SetNextWallpaper sets the next wallpaper.
func SetNextWallpaper() {
	ws := getWallpaperService(nil, nil) // Might not need config here
	ws.SetNextWallpaper()
}

// SetPreviousWallpaper sets the previous wallpaper.
func SetPreviousWallpaper() {
	ws := getWallpaperService(nil, nil)
	ws.SetPreviousWallpaper()
}

// SetRandomWallpaper sets a random wallpaper.
func SetRandomWallpaper() {
	ws := getWallpaperService(nil, nil)
	ws.SetRandomWallpaper()
}

// StopWallpaperService stops the wallpaper service.
func StopWallpaperService() {
	ws := getWallpaperService(nil, nil)
	ws.Stop()
}

// GetCurrentImage returns the current wallpaper image information.
func GetCurrentImage() ImgSrvcImage {
	ws := getWallpaperService(nil, nil)
	return ws.getCurrentImage()
}

// ViewCurrentImageOnWeb opens the current wallpaper image in the default web browser.
func ViewCurrentImageOnWeb(app fyne.App) {
	ws := getWallpaperService(nil, nil)
	url, err := url.Parse(ws.getCurrentImage().ShortURL)
	if err != nil {
		log.Printf("Failed to parse URL: %v", err)
		return
	}
	app.OpenURL(url)
}

// RefreshImages discards all downloaded images and fetches new ones.
func RefreshImages() {
	ws := getWallpaperService(nil, nil)
	ws.refreshImages()
	go func() {
		time.Sleep(5 * time.Second)
		ws.SetNextWallpaper()
	}()
}

// SetSmartFitEnabled enables or disables smart cropping.
func SetSmartFitEnabled(enabled bool) {
	ws := getWallpaperService(nil, nil)
	ws.prefs.SetBool(SmartFitPrefKey, enabled)
	ws.fitImage = enabled
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
// Returns true if the URL is valid, false otherwise.
// Also returns an error if any.
func CheckWallhavenURL(queryURL string) error {
	ws := getWallpaperService(nil, nil)
	return ws.checkWallhavenURL(CovertToAPIURL(queryURL))
}

// GetWallhavenURL retrieves the Wallhaven URL using the provided API URL.
// It initializes the wallpaper service and calls its getWallhavenURL method.
//
// Parameters:
//   - apiURL: A string representing the API URL to be used.
//
// Returns:
//   - A pointer to a url.URL object containing the Wallhaven URL.
func GetWallhavenURL(apiURL string) *url.URL {
	ws := getWallpaperService(nil, nil)
	return ws.getWallhavenURL(apiURL)
}
