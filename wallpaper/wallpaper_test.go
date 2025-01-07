package wallpaper

import (
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/wallhavener/config"
)

func TestSetWallpaper(t *testing.T) {
	// This test might be tricky to implement directly, as it interacts with the Windows API.
	// You might need to mock the SystemParametersInfo function or skip this test for now.
	t.Skip("Skipping SetWallpaper test for now")
}

func TestRotateWallpapers(t *testing.T) {
	// Mock the config
	config.Cfg = config.Config{
		Frequency: 100 * time.Millisecond, // Set a very short frequency for testing
		ImageURLs: []config.ImageURL{
			{URL: "https://example.com/valid_url", Active: true},
		},
	}

	// Create a mock HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve a sample JSON response with a valid image URL
		sampleResponse := `{"data": [{"path": "https://example.com/image1.jpg"}, {"path": "https://example.com/image2.png"}], "meta": {"last_page": 1}}`
		fmt.Fprintln(w, sampleResponse)
	}))
	defer ts.Close()

	// Replace the actual URL with the mock server's URL
	config.Cfg.ImageURLs[0].URL = ts.URL

	// Start the rotation in a separate goroutine
	go RotateWallpapers()

	// Wait for a short time to allow the rotation to happen
	time.Sleep(200 * time.Millisecond)

	// Stop the rotation
	StopRotation()

	// Check if the downloaded images directory exists
	downloadedDir := filepath.Join(getTempDir(), "wallhavener_downloads")
	if _, err := os.Stat(downloadedDir); os.IsNotExist(err) {
		t.Error("Downloaded images directory does not exist")
	}

	// Check if the images were downloaded
	files, err := os.ReadDir(downloadedDir)
	if err != nil {
		t.Errorf("Failed to read downloaded images directory: %v", err)
	}
	if len(files) != 2 { // Expecting 2 images
		t.Errorf("Expected 2 downloaded images, found %d", len(files))
	}
}

func TestDownloadImage(t *testing.T) {
	// Create a mock HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve a sample image
		img := image.NewRGBA(image.Rect(0, 0, 100, 100))
		png.Encode(w, img)
	}))
	defer ts.Close()

	// Call downloadImage with the mock server's URL
	filePath, err := downloadImage(ts.URL)
	if err != nil {
		t.Errorf("downloadImage() returned an error: %v", err)
	}

	// Check if the file was created
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Image file was not created")
	}

	// Clean up the temporary file
	os.Remove(filePath)
}

func TestGetTempDir(t *testing.T) {
	tempDir := getTempDir()

	// Check if the returned directory exists
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Errorf("Temporary directory does not exist: %s", tempDir)
	}
}

func TestDeleteTempImages(t *testing.T) {
	// This test is no longer relevant as we're not deleting images
}
