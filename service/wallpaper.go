package service

import (
	"encoding/json"
	"fmt"
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
)

// OS is an interface for abstracting OS-specific operations.
type OS interface {
	setWallpaper(imagePath string) error // Not exported
	getTempDir() string                  // Not exported
	showNotification(title, message string) error
}

// wallpaperService manages wallpaper rotation.
type wallpaperService struct {
	os              OS
	cfg             *config.Config
	ticker          *time.Ticker
	downloadMutex   sync.Mutex // Protects currentPage, downloading, and download operations
	currentImage    ImgSrvcImage
	imageIndex      int
	downloadedDir   string
	downloadHistory map[string]ImgSrvcImage
	currentPage     int
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
	instance *wallpaperService
	once     sync.Once
)

// getWallpaperService returns the singleton instance of wallpaperService.
func getWallpaperService(cfg *config.Config) *wallpaperService {
	once.Do(func() {
		instance = &wallpaperService{
			os:              &windowsOS{}, // Initialize with Windows OS
			cfg:             cfg,
			downloadMutex:   sync.Mutex{},
			downloadHistory: make(map[string]ImgSrvcImage),
			currentPage:     1, // Start with the first page
		}
	})
	return instance
}

// RefreshImages downloads all images from the configured URLs.
func (ws *wallpaperService) refreshImages() {
	// Clear the downloaded images directory
	ws.downloadMutex.Lock()
	err := ws.clearDownloadedImagesDir()
	if err != nil {
		log.Printf("Error clearing downloaded images directory: %v", err)
	}
	clear(ws.downloadHistory) // Clear the download history
	ws.currentPage = 1        // Reset to the first page
	ws.imageIndex = -1        // Reset the image index
	ws.downloadMutex.Unlock()

	ws.downloadAllImages(ws.currentPage)
}

// downloadAllImages downloads images from all active URLs for the specified page.
func (ws *wallpaperService) downloadAllImages(page int) {
	for _, url := range ws.cfg.ImageURLs {
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
	q.Set("apikey", ws.cfg.APIKey)
	q.Set("page", fmt.Sprint(page))
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
		ws.downloadImage(img)
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

	// Create a file name using the timestamp and the image ID
	timestamp := time.Now().UnixNano()
	tempFile := filepath.Join(ws.downloadedDir, fmt.Sprintf("%d_%s.jpg", timestamp, img.ID))
	outFile, err := os.Create(tempFile)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer outFile.Close()

	// Save the image to the temporary file
	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save image to temporary file: %v", err)
	}

	// Store the downloaded image in the history
	ws.downloadMutex.Lock()
	defer ws.downloadMutex.Unlock()
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

	if len(imageFiles) == 0 {
		log.Println("No downloaded images found.")
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
	// Create the downloaded images directory if it doesn't exist
	ws.downloadedDir = filepath.Join(ws.os.getTempDir(), strings.ToLower(config.ServiceName)+"_downloads")
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
	ws.ticker = time.NewTicker(ws.cfg.Frequency)
	for range ws.ticker.C {
		ws.SetNextWallpaper()
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

// clear clears the given map.
func clear(m map[string]ImgSrvcImage) {
	for k := range m {
		delete(m, k)
	}
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
func StartWallpaperService(cfg *config.Config) {
	ws := getWallpaperService(cfg)
	ws.Start()
}

// SetNextWallpaper sets the next wallpaper.
func SetNextWallpaper() {
	ws := getWallpaperService(nil) // Might not need config here
	ws.SetNextWallpaper()
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
