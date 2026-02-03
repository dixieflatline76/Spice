//go:build darwin
// +build darwin

package wallpaper

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/dixieflatline76/Spice/pkg/sysinfo"
	"github.com/dixieflatline76/Spice/util/log"
)

// macOSOS implements the OS interface for macOS.
type macOSOS struct{}

type spDisplay struct {
	Name       string `json:"_name"`
	Resolution string `json:"_spdisplays_pixels"` // Changed from _spdisplays_resolution to _spdisplays_pixels for actual resolution
}

type spGPU struct {
	Displays []spDisplay `json:"spdisplays_ndrvs"`
}

type spDataType struct {
	GPUs []spGPU `json:"SPDisplaysDataType"`
}

// GetMonitors returns information about all connected monitors on macOS.
func (m *macOSOS) GetMonitors() ([]Monitor, error) {
	// 1. Mock Support for Windows Testing
	if output := os.Getenv("MOCK_MACOS_OUTPUT"); output != "" {
		return m.parseSystemProfiler(output)
	}

	// 2. Real Implementation
	cmd := exec.Command("system_profiler", "SPDisplaysDataType", "-json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting display info: %w", err)
	}

	return m.parseSystemProfiler(string(out))
}

var (
	// resolutionRegex matches strings like "3456 x 2234"
	resolutionRegex = regexp.MustCompile(`(\d+)\s*x\s*(\d+)`)
)

func (m *macOSOS) parseSystemProfiler(jsonOutput string) ([]Monitor, error) {
	var data spDataType
	if err := json.Unmarshal([]byte(jsonOutput), &data); err != nil {
		return nil, fmt.Errorf("parsing system_profiler: %w", err)
	}

	var monitors []Monitor
	monitorIdx := 0

	for _, gpu := range data.GPUs {
		for _, d := range gpu.Displays {
			matches := resolutionRegex.FindStringSubmatch(d.Resolution)
			if len(matches) < 3 {
				continue
			}

			w, _ := strconv.Atoi(matches[1])
			h, _ := strconv.Atoi(matches[2])

			monitors = append(monitors, Monitor{
				ID:   monitorIdx,
				Name: d.Name,
				Rect: image.Rect(0, 0, w, h),
			})
			monitorIdx++
		}
	}

	if len(monitors) == 0 {
		return nil, fmt.Errorf("no displays found in output")
	}

	return monitors, nil
}

// SetWallpaper sets the desktop wallpaper on macOS.
// Uses AppleScript via osascript to target specific desktops.
func (m *macOSOS) SetWallpaper(imagePath string, monitorID int) error {
	// 1. Mock Support
	if os.Getenv("MOCK_MACOS_OUTPUT") != "" {
		log.Printf("[MOCK] Setting Wallpaper for Monitor %d: %s", monitorID, imagePath)
		return nil
	}

	// 2. Real Implementation
	// AppleScript desktops are 1-indexed.
	// "tell application \"System Events\" to set picture of desktop %d to \"%s\""
	script := fmt.Sprintf(`
		tell application "System Events"
			set picture of desktop %d to "%s"
		end tell
	`, monitorID+1, imagePath)

	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript failed: %v, output: %s", err, string(out))
	}

	return nil
}

// GetDesktopDimension returns the primary desktop dimensions on macOS.
func (m *macOSOS) GetDesktopDimension() (int, int, error) {
	return sysinfo.GetScreenDimensions()
}

// getOS returns a new instance of the macOSOS struct.
func getOS() OS {
	return &macOSOS{}
}
