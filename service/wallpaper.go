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
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/config"

	"golang.org/x/sys/windows"
)

// OS is an interface for abstracting OS-specific operations.
type OS interface {
	setWallpaper(imagePath string) error // Not exported
	getTempDir() string                  // Not exported
	showNotification(title, message string) error
	openURL(url string) error
}

// windowsOS implements the OS interface for Windows.
type windowsOS struct{}

// setWallpaper sets the wallpaper to the given image file path.
func (w *windowsOS) setWallpaper(imagePath string) error {
	imagePathUTF16, err := syscall.UTF16PtrFromString(imagePath) // Convert the image path to UTF-16
	if err != nil {
		return err
	}

	user32 := windows.NewLazySystemDLL("user32.dll")
	systemParametersInfo := user32.NewProc("SystemParametersInfoW")
	ret, _, err := systemParametersInfo.Call(
		uintptr(SPISetDeskWallpaper),
		uintptr(0),
		uintptr(unsafe.Pointer(imagePathUTF16)),
		uintptr(SPIFUpdateIniFile|SPIFSendChange),
	)
	if ret == 0 {
		return err
	}

	return nil
}

// getTempDir returns the system's temporary directory.
func (w *windowsOS) getTempDir() string {
	tempDir := os.Getenv("TEMP")
	if tempDir == "" {
		tempDir = os.Getenv("TMP")
	}
	if tempDir == "" {
		tempDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local", "Temp")
	}
	return tempDir
}

func (w *windowsOS) showNotification(title, message string) error {
	// ... (Your Windows notification implementation using toast notifications or
	//      other Windows-specific methods)
	return nil
}

func (w *windowsOS) openURL(url string) error {
	// ... (Your Windows implementation for opening a URL, e.g., using
	//      `shell.Open()` or similar)
	return nil
}

// wallpaperService manages wallpaper rotation.
type wallpaperService struct {
	os              OS
	cfg             *config.Config
	ticker          *time.Ticker
	currentImage    ImgSrvcImage
	imageMX         sync.Mutex
	downloadCond    *sync.Cond
	imageIndex      int
	downloadedDir   string
	downloadHistory map[string]ImgSrvcImage
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
			imageMX:         sync.Mutex{},
			downloadCond:    sync.NewCond(&sync.Mutex{}),
			downloadHistory: make(map[string]ImgSrvcImage),
		}
	})
	return instance
}

// Start starts the wallpaper rotation service.
func (ws *wallpaperService) Start() {
	// Create the downloaded images directory if it doesn't exist
	ws.downloadedDir = filepath.Join(ws.os.getTempDir(), strings.ToLower(config.ServiceName)+"_downloads")
	err := os.MkdirAll(ws.downloadedDir, 0755)
	if err != nil {
		log.Fatalf("Error creating downloaded images directory: %v", err)
	}

	// Schedule daily image refresh
	go func() {
		for {
			now := time.Now()
			nextMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)
			time.Sleep(time.Until(nextMidnight))

			// Download all images again
			ws.refreshImages()
		}
	}()

	// Download all images from the configured URLs
	ws.refreshImages()

	time.Sleep(5 * time.Second) // Wait a bit for the wallpapers to load
	ws.SetNextWallpaper()       // Set the initial wallpaper

	ws.ticker = time.NewTicker(ws.cfg.Frequency)

	for range ws.ticker.C {
		ws.SetNextWallpaper()
	}
}

// RefreshImages downloads all images from the configured URLs.
func (ws *wallpaperService) refreshImages() {
	// Clear the downloaded images directory
	ws.imageMX.Lock()
	err := ws.clearDownloadedImagesDir()
	if err != nil {
		log.Printf("Error clearing downloaded images directory: %v", err)
	}
	clear(ws.downloadHistory)
	ws.imageMX.Unlock()

	for _, url := range ws.cfg.ImageURLs {
		if url.Active {
			go ws.downloadImagesForURL(url.URL)
		}
	}

	ws.imageIndex = -1 // Reset the image index
}

// clear clears the given map.
func (ws *wallpaperService) downloadImagesForURL(imgSrvcURL string) {
	// Construct the API URL
	u, err := url.Parse(imgSrvcURL)
	if err != nil {
		log.Printf("Invalid Image URL: %v", err)
		return
	}

	q := u.Query()
	q.Set("apikey", ws.cfg.APIKey)
	u.RawQuery = q.Encode()
	// Fetch and download images from each page
	for page := 1; ; page++ {

		log.Printf("Downloading from URL: %v (page %d)", u.String(), page)

		// Set the page number in the query parameters
		q.Set("page", fmt.Sprint(page))
		u.RawQuery = q.Encode()

		// Fetch the JSON response for the current page
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
			_, err := ws.downloadImage(img)
			if err != nil {
				log.Printf("Failed to download image: %v", err)
				continue
			}
			time.Sleep(500 * time.Millisecond) // Be nice to the API
		}

		ws.imageMX.Lock()
		// Check if there are more pages
		if page >= response.Meta.LastPage {
			ws.imageMX.Unlock()
			break
		}
		ws.downloadCond.Wait() // Wait for the next page
		ws.imageMX.Unlock()
	}
}

// SetNextWallpaper sets the next wallpaper in the list.
func (ws *wallpaperService) SetNextWallpaper() {
	ws.imageMX.Lock()
	defer ws.imageMX.Unlock()

	ws.imageIndex++ // Increment the image index
	ws.setWallpaperAt(ws.imageIndex)
}

// SetPreviousWallpaper sets the previous wallpaper in the list.
func (ws *wallpaperService) SetPreviousWallpaper() {
	ws.imageMX.Lock()
	defer ws.imageMX.Unlock()

	ws.imageIndex-- // Decrement the image index
	ws.setWallpaperAt(ws.imageIndex)
}

// SetRandomWallpaper sets a random wallpaper from the list.
func (ws *wallpaperService) SetRandomWallpaper() {
	ws.imageMX.Lock()
	defer ws.imageMX.Unlock()

	// Get a list of all downloaded image files
	imageFiles, err := os.ReadDir(ws.downloadedDir)
	if err != nil {
		log.Printf("Failed to read downloaded images directory: %v", err)
		return
	}

	if len(imageFiles) == 0 {
		log.Println("No downloaded images found.")
		return
	}

	randomIndex := rand.Intn(len(imageFiles) - 1)
	ws.setWallpaperAt(randomIndex)
}

// setWallpaperAt sets the wallpaper at the specified index.
func (ws *wallpaperService) setWallpaperAt(dirIndex int) {
	// Get a list of all downloaded image files
	imageFiles, err := os.ReadDir(ws.downloadedDir)
	if err != nil {
		log.Printf("Failed to read downloaded images directory: %v", err)
		return
	}

	if len(imageFiles) == 0 {
		log.Println("No downloaded images found.")
		return
	}

	if dirIndex > len(imageFiles)-1 {
		ws.downloadCond.Broadcast() // Signal to download more images
		ws.downloadCond.Wait()      // Wait for more images to be downloaded
	}

	// Get the image file at the specified index
	safeIndex := (dirIndex + len(imageFiles)) % len(imageFiles) // Ensure the index is within bounds
	imageFile := imageFiles[safeIndex]
	imagePath := filepath.Join(ws.downloadedDir, imageFile.Name())

	// Set the wallpaper
	err = ws.os.setWallpaper(imagePath)
	if err != nil {
		log.Printf("Failed to set wallpaper: %v", err)
		return
	}

	// Update current image and index (for tracking/debugging)
	ws.currentImage = ws.downloadHistory[imagePath]
	ws.imageIndex = safeIndex
}

// Stop stops the wallpaper rotation service and triggers necessary cleanup.
func (ws *wallpaperService) Stop() {
	if ws.ticker != nil {
		ws.ticker.Stop()
	}
}

// clear clears the given map.
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
	ws.imageMX.Lock()
	ws.downloadHistory[outFile.Name()] = img
	ws.imageMX.Unlock()
	return tempFile, nil
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

// GetCurrentImage returns the current image.
func (ws *wallpaperService) getCurrentImage() ImgSrvcImage {
	ws.imageMX.Lock()
	defer ws.imageMX.Unlock()

	return ws.currentImage
}

// clear clears the given map.
func clear(m map[string]ImgSrvcImage) {
	for k := range m {
		delete(m, k)
	}
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

// Windows API constants (defined manually)
const (
	SPISetDeskWallpaper  = 0x0014
	SPIFUpdateIniFile    = 0x01
	SPIFSendChange       = 0x02
	SPIFSendWinIniChange = 0x02
)
