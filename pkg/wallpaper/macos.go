//go:build darwin
// +build darwin

package wallpaper

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
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

// getDesktopDimension returns the primary desktop dimensions on macOS.
func (m *macOSOS) getDesktopDimension() (int, int, error) {
	// Use `system_profiler` to get display info
	cmd := exec.Command("system_profiler", "SPDisplaysDataType")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to run system_profiler SPDisplaysDataType: %w", err)
	}

	// Regex to find "Resolution: <width> x <height>", handling extra text like "Retina"
	// \s* = optional whitespace, (\d+) = capture digits (width/height)
	re := regexp.MustCompile(`Resolution:\s*(\d+)\s*x\s*(\d+)`)

	// Find the first match in the output (usually the primary display)
	matches := re.FindStringSubmatch(string(out))

	// matches slice: [0]=full match, [1]=width string, [2]=height string
	if len(matches) == 3 {
		width, errW := strconv.Atoi(matches[1])
		height, errH := strconv.Atoi(matches[2])

		// Check for Atoi conversion errors (should be unlikely with this regex)
		if errW != nil {
			return 0, 0, fmt.Errorf("failed to convert width '%s': %w", matches[1], errW)
		}
		if errH != nil {
			return 0, 0, fmt.Errorf("failed to convert height '%s': %w", matches[2], errH)
		}

		return width, height, nil
	}

	// If no resolution pattern was found
	return 0, 0, fmt.Errorf("failed to parse screen resolution from system_profiler output")
}

// getOS returns a new instance of the macOSOS struct.
func getOS() OS {
	return &macOSOS{}
}
