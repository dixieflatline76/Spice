//go:build linux
// +build linux

package wallpaper

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/dixieflatline76/Spice/pkg/sysinfo"
	"github.com/dixieflatline76/Spice/util/log"
)

// linuxOS implements the OS interface for Linux.
type linuxOS struct{}

// getOS returns a new instance of the linuxOS struct.
func getOS() OS {
	// Simple check for Chrome OS environment marker
	if _, err := os.Stat("/dev/.cros_milestone"); err == nil {
		return &ChromeOS{}
	}
	return &linuxOS{}
}

// GetMonitors returns information about connected monitors on Linux.
func (l *linuxOS) GetMonitors() ([]Monitor, error) {
	// 1. Mock Support for Windows Testing
	if output := os.Getenv("MOCK_LINUX_OUTPUT"); output != "" {
		return l.parseXRandr(output)
	}

	// 2. Real Implementation
	cmd := exec.Command("xrandr", "--listmonitors")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("xrandr failed: %w", err)
	}

	return l.parseXRandr(string(out))
}

func (l *linuxOS) parseXRandr(output string) ([]Monitor, error) {
	lines := strings.Split(output, "\n")
	var monitors []Monitor

	// Regex: 0: +*HDMI-1 1920/531x1080/299+0+0  HDMI-1
	// Groups: 1=ID, 2=Name, 3=W, 4=H, 5=X, 6=Y
	re := regexp.MustCompile(`(\d+):\s+\+\*?(\S+)\s+(\d+)/\d+x(\d+)/\d+\+(\d+)\+(\d+)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		matches := re.FindStringSubmatch(line)
		if len(matches) < 7 {
			continue
		}

		id, _ := strconv.Atoi(matches[1])
		name := matches[2]
		w, _ := strconv.Atoi(matches[3])
		h, _ := strconv.Atoi(matches[4])
		// x, _ := strconv.Atoi(matches[5])
		// y, _ := strconv.Atoi(matches[6])

		monitors = append(monitors, Monitor{
			ID:   id,
			Name: name,
			Rect: image.Rect(0, 0, w, h),
		})
	}

	if len(monitors) == 0 {
		return l.GetPrimaryMonitorFallback()
	}

	return monitors, nil
}

func (l *linuxOS) GetPrimaryMonitorFallback() ([]Monitor, error) {
	width, height, err := l.GetDesktopDimension()
	if err != nil {
		return nil, err
	}
	return []Monitor{{ID: 0, Name: "Primary", Rect: image.Rect(0, 0, width, height)}}, nil
}

// SetWallpaper sets the desktop wallpaper on Linux.
// Note: Limited to simple implementation for this iteration.
func (l *linuxOS) SetWallpaper(imagePath string, monitorID int) error {
	// 1. Mock Support
	if os.Getenv("MOCK_LINUX_OUTPUT") != "" {
		log.Printf("[MOCK] Setting Wallpaper for Monitor %d: %s", monitorID, imagePath)
		return nil
	}

	// 2. Real Implementation (Example: nitrogen)
	// Nitrogen supports --head for specific monitors (X11)
	// nitrogen --head=0 --set-zoom-fill /path/to/img.jpg

	cmd := exec.Command("nitrogen", fmt.Sprintf("--head=%d", monitorID), "--set-zoom-fill", imagePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Fallback logging if tool missing
		log.Printf("Failed to set wallpaper with nitrogen (expected on X11): %v. Output: %s", err, string(out))
		return fmt.Errorf("nitrogen failed: %w", err)
	}

	return nil
}

// GetDesktopDimension returns the desktop dimensions on Linux.
func (l *linuxOS) GetDesktopDimension() (int, int, error) {
	return sysinfo.GetScreenDimensions()
}

// setWallpaperGNOME sets the wallpaper for GNOME-based desktop environments.
func (l *linuxOS) setWallpaperGNOME(imagePath string) error {
	cmd := exec.Command("gsettings", "set", "org.gnome.desktop.background", "picture-uri", fmt.Sprintf("file://%s", imagePath))
	return cmd.Run()
}

// setWallpaperKDE sets the wallpaper for KDE.
func (l *linuxOS) setWallpaperKDE(imagePath string) error {
	// Find the appropriate Plasma plugin
	plasmashellProc, err := exec.Command("pgrep", "-f", "plasmashell").Output()
	if err != nil {
		return fmt.Errorf("failed to find plasmashell process: %w", err)
	}

	plasmashellPID := strings.TrimSpace(string(plasmashellProc))

	dbusSendCmd := fmt.Sprintf(`dbus-send --session \
        --dest=org.kde.plasmashell \
        /PlasmaShell,%s \
        org.kde.PlasmaShell.evaluateScript \
        'string:
            var allDesktops = desktops();
            for (i=0;i<allDesktops.length;i++) {
                d = allDesktops[i];
                d.wallpaperPlugin = "org.kde.image";
                d.currentConfigGroup = Array("Wallpaper", "org.kde.image", "General");
                d.writeConfig("Image", "file://%s");
            }
        '`, plasmashellPID, imagePath)

	cmd := exec.Command("sh", "-c", dbusSendCmd)
	return cmd.Run()
}

// setWallpaperXFCE sets the wallpaper for XFCE.
func (l *linuxOS) setWallpaperXFCE(imagePath string) error {
	// Check if the XFCE configuration file exists
	if _, err := l.getXFCEDesktopConfigFile(); err != nil {
		return err
	}

	// Construct the command to update the configuration file
	cmd := exec.Command("xfconf-query",
		"--channel", "xfce4-desktop",
		"--property", "/backdrop/screen0/monitor0/workspace0/last-image",
		"--set", imagePath)

	// Run the command
	return cmd.Run()
}

// getXFCEDesktopConfigFile retrieves the path to the XFCE desktop configuration file.
func (l *linuxOS) getXFCEDesktopConfigFile() (string, error) {
	// Check if the file exists in the default location
	defaultConfigFile := filepath.Join(os.Getenv("HOME"), ".config", "xfce4", "xfconf", "xfce-perchannel-xml", "xfce4-desktop.xml")
	if _, err := os.Stat(defaultConfigFile); err == nil {
		return defaultConfigFile, nil
	}

	return "", fmt.Errorf("could not find XFCE desktop configuration file")
}

// setWallpaperSway sets the wallpaper for Sway.
func (l *linuxOS) setWallpaperSway(imagePath string) error {
	cmd := exec.Command("swaybg", imagePath) // Make sure swaybg is installed
	return cmd.Run()
}
