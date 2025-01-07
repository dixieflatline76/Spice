package wallpaper

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

	"github.com/dixieflatline76/wallhavener/config"
	"golang.org/x/sys/windows"
)

var (
	ticker          *time.Ticker
	currentImage    imgSrvcImage
	imageMX         sync.Mutex              // Mutex to protect image data
	downloadCond    *sync.Cond              // Condition variable to signal download completion
	imageIndex      int                     // Index of the current image
	downloadedDir   string                  // Directory to store downloaded images
	downloadHistory map[string]imgSrvcImage // Map to store downloaded images (URL -> Image)
)

func init() {
	downloadCond = sync.NewCond(&imageMX)
	downloadHistory = make(map[string]imgSrvcImage)
}

type imgSrvcImage struct {
	Path     string `json:"path"`
	Id       string `json:"id"`
	ShortURL string `json:"short_url"`
}

type imgSrvcResponse struct {
	Data []imgSrvcImage `json:"data"`
	Meta struct {
		LastPage int `json:"last_page"`
	} `json:"meta"`
}

// Windows API constants (defined manually)
const (
	SPI_SETDESKWALLPAPER  = 0x0014
	SPIF_UPDATEINIFILE    = 0x01
	SPIF_SENDCHANGE       = 0x02
	SPIF_SENDWININICHANGE = 0x02
)

// setWallpaper sets the wallpaper to the given image file path.
func setWallpaper(imagePath string) error {
	imagePathUTF16, err := syscall.UTF16PtrFromString(imagePath) // Convert the image path to UTF-16
	if err != nil {
		return err
	}

	user32 := windows.NewLazySystemDLL("user32.dll")
	systemParametersInfo := user32.NewProc("SystemParametersInfoW")
	ret, _, err := systemParametersInfo.Call(
		uintptr(SPI_SETDESKWALLPAPER),
		uintptr(0),
		uintptr(unsafe.Pointer(imagePathUTF16)),
		uintptr(SPIF_UPDATEINIFILE|SPIF_SENDCHANGE),
	)
	if ret == 0 {
		return err
	}

	return nil
}

// RotateWallpapers rotates the wallpapers at the configured frequency.
func RotateWallpapers() {
	// Create the downloaded images directory if it doesn't exist
	downloadedDir = filepath.Join(getTempDir(), strings.ToLower(config.ServiceName)+"_downloads")
	err := os.MkdirAll(downloadedDir, 0755)
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
			refreshImages()
		}
	}()

	// Download all images from the configured URLs
	refreshImages()

	ticker = time.NewTicker(config.Cfg.Frequency)

	time.Sleep(5 * time.Second) // Wait a bit for the wallpapers to load
	SetNextWallpaper()          // Set the initial wallpaper

	for range ticker.C {
		SetNextWallpaper()
	}
}

func refreshImages() {
	// Clear the downloaded images directory
	imageMX.Lock()
	err := clearDownloadedImagesDir()
	if err != nil {
		log.Printf("Error clearing downloaded images directory: %v", err)
	}
	clear(downloadHistory)
	imageMX.Unlock()

	for _, url := range config.Cfg.ImageURLs {
		if url.Active {
			go downloadImagesForURL(url.URL)
		}
	}

	imageIndex = -1 // Reset the image index
}

func downloadImagesForURL(imgSrvcURL string) {
	// Construct the API URL
	u, err := url.Parse(imgSrvcURL)
	if err != nil {
		log.Printf("Invalid Image URL: %v", err)
		return
	}

	q := u.Query()
	q.Set("apikey", config.Cfg.APIKey)
	u.RawQuery = q.Encode()
	log.Println(u.String())
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
			_, err := downloadImage(img)
			if err != nil {
				log.Printf("Failed to download image: %v", err)
				continue
			}
			time.Sleep(500 * time.Millisecond) // Be nice to the API
		}

		imageMX.Lock()
		// Check if there are more pages
		if page >= response.Meta.LastPage {
			imageMX.Unlock()
			break
		}
		downloadCond.Wait() // Wait for the next page
		imageMX.Unlock()
	}
}

func SetNextWallpaper() {
	imageMX.Lock()
	defer imageMX.Unlock()

	imageIndex++ // Increment the image index
	setWallpaperAt(imageIndex)
}

func SetPreviousWallpaper() {
	imageMX.Lock()
	defer imageMX.Unlock()

	imageIndex-- // Decrement the image index
	setWallpaperAt(imageIndex)
}

func SetRandomWallpaper() {
	imageMX.Lock()
	defer imageMX.Unlock()

	// Get a list of all downloaded image files
	imageFiles, err := os.ReadDir(downloadedDir)
	if err != nil {
		log.Printf("Failed to read downloaded images directory: %v", err)
		return
	}

	if len(imageFiles) == 0 {
		log.Println("No downloaded images found.")
		return
	}

	randomIndex := rand.Intn(len(imageFiles) - 1)
	setWallpaperAt(randomIndex)
}

func setWallpaperAt(index int) {
	// Get a list of all downloaded image files
	imageFiles, err := os.ReadDir(downloadedDir)
	if err != nil {
		log.Printf("Failed to read downloaded images directory: %v", err)
		return
	}

	if len(imageFiles) == 0 {
		log.Println("No downloaded images found.")
		return
	}

	if index > len(imageFiles)-1 {
		downloadCond.Broadcast() // Signal to download more images
		downloadCond.Wait()      // Wait for more images to be downloaded
	}

	// Get the image file at the specified index
	safeIndex := (index + len(imageFiles)) % len(imageFiles) // Ensure the index is within bounds
	imageFile := imageFiles[safeIndex]
	imagePath := filepath.Join(downloadedDir, imageFile.Name())

	// Set the wallpaper
	err = setWallpaper(imagePath)
	if err != nil {
		log.Printf("Failed to set wallpaper: %v", err)
		return
	}

	// Update current image and index (for tracking/debugging)
	currentImage = downloadHistory[imageFile.Name()]
	imageIndex = safeIndex
}

// StopRotation stops the wallpaper rotation ticker.
func StopRotation() {
	if ticker != nil {
		ticker.Stop()
	}
}

func downloadImage(img imgSrvcImage) (string, error) {
	resp, err := http.Get(img.Path)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %v", err)
	}
	defer resp.Body.Close()

	// Create a file name using the timestamp and the image ID
	timestamp := time.Now().UnixNano()
	tempFile := filepath.Join(downloadedDir, fmt.Sprintf("%d_%s.jpg", timestamp, img.Id))
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
	imageMX.Lock()
	downloadHistory[tempFile] = img
	imageMX.Unlock()
	return tempFile, nil
}

func getTempDir() string {
	tempDir := os.Getenv("TEMP")
	if tempDir == "" {
		tempDir = os.Getenv("TMP")
	}
	if tempDir == "" {
		tempDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local", "Temp")
	}
	return tempDir
}

func clearDownloadedImagesDir() error {
	files, err := os.ReadDir(downloadedDir)
	if err != nil {
		return fmt.Errorf("failed to read downloaded images directory: %v", err)
	}

	for _, file := range files {
		err = os.Remove(filepath.Join(downloadedDir, file.Name()))
		if err != nil {
			return fmt.Errorf("failed to remove image file: %v", err)
		}
	}

	return nil
}

func GetCurrentImage() imgSrvcImage {
	imageMX.Lock()
	defer imageMX.Unlock()

	return currentImage
}
