//go:build darwin
// +build darwin

package wallpaper

import (
	"fmt"
	"os/exec"

	"github.com/dixieflatline76/Spice/pkg/sysinfo"
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
	return sysinfo.GetScreenDimensions()
}

// getOS returns a new instance of the macOSOS struct.
func getOS() OS {
	return &macOSOS{}
}
