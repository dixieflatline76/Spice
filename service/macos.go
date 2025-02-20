//go:build darwin
// +build darwin

package service

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/dixieflatline76/Spice/config"
)

// macOSOS implements the OS interface for macOS.
type macOSOS struct{}

// setWallpaper sets the desktop wallpaper on macOS.
func (m *macOSOS) setWallpaper(imagePath string) error {
	// Use AppleScript to set the wallpaper
	script := fmt.Sprintf(`
                tell application "Finder"
                        set desktop picture to POSIX file "%s"
                end tell
        `, imagePath)

	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set wallpaper: %w", err)
	}

	return nil
}

// getDesktopDimension returns the desktop dimensions on macOS.
func (m *macOSOS) getDesktopDimension() (int, int, error) {
	// Use `system_profiler` to get screen resolution
	cmd := exec.Command("system_profiler", "SPDisplaysDataType")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get screen resolution: %w", err)
	}

	// Parse the output to extract the resolution
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Resolution:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				resolution := strings.TrimSpace(parts[1])
				dimensions := strings.Split(resolution, " x ")
				if len(dimensions) == 2 {
					width, _ := strconv.Atoi(dimensions[0])
					height, _ := strconv.Atoi(dimensions[1])
					return width, height, nil
				}
			}
		}
	}

	return 0, 0, fmt.Errorf("failed to parse screen resolution")
}

// getWallpaperService returns the singleton instance of wallpaperService.
func getWallpaperService(cfg *config.Config) *wallpaperService {
	once.Do(func() {
		// Initialize the wallpaper service for macOS
		currentOS := &macOSOS{}

		// Initialize the wallpaper service
		wsInstance = &wallpaperService{
			os:              currentOS,                                                                             // Initialize with macOS OS
			imgProcessor:    &smartImageProcessor{os: currentOS, aspectThreshold: 0.9, resampler: imaging.Lanczos}, // Initialize with smartCropper with a lenient threshold
			cfg:             cfg,
			downloadMutex:   sync.Mutex{},
			downloadHistory: make(map[string]ImgSrvcImage),
			seenHistory:     make(map[string]bool),
			currentPage:     1,                                        // Start with the first page,
			fitImage:        cfg.BoolWithFallback("Smart Fit", false), // Initialize with smart fit preference
		}
	})
	return wsInstance
}
