//go:build linux
// +build linux

package wallpaper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
)

// linuxOS implements the OS interface for Linux.
type linuxOS struct{}

// setWallpaper sets the desktop wallpaper on Linux, supporting X11 and some Wayland compositors.
func (l *linuxOS) setWallpaper(imagePath string) error {
	desktopEnv := os.Getenv("XDG_CURRENT_DESKTOP")
	if desktopEnv == "" {
		desktopEnv = os.Getenv("DESKTOP_SESSION")
	}
	desktopEnv = strings.ToLower(desktopEnv)

	if os.Getenv("WAYLAND_DISPLAY") != "" {
		// Wayland
		if strings.Contains(desktopEnv, "gnome") || strings.Contains(desktopEnv, "mutter") {
			return l.setWallpaperGNOME(imagePath)
		} else if strings.Contains(desktopEnv, "sway") {
			return l.setWallpaperSway(imagePath)
		} else {
			return fmt.Errorf("unsupported Wayland compositor: %s (KDE Wayland support dropped)", desktopEnv)
		}
	} else {
		// X11
		switch {
		case strings.Contains(desktopEnv, "gnome") || strings.Contains(desktopEnv, "unity") || strings.Contains(desktopEnv, "cinnamon"):
			return l.setWallpaperGNOME(imagePath)
		case strings.Contains(desktopEnv, "kde"):
			return l.setWallpaperKDE(imagePath)
		case strings.Contains(desktopEnv, "xfce"):
			return l.setWallpaperXFCE(imagePath)
		default:
			return fmt.Errorf("unsupported X11 desktop environment: %s", desktopEnv)
		}
	}
}

// getDesktopDimension returns the desktop dimensions on Linux.
func (l *linuxOS) getDesktopDimension() (int, int, error) {
	// Use `xdpyinfo` to get screen resolution
	cmd := exec.Command("xdpyinfo", "|", "grep", "dimensions")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get screen resolution: %w", err)
	}

	// Parse the output to extract the resolution
	parts := strings.Split(string(out), ":")
	if len(parts) >= 2 {
		resolution := strings.TrimSpace(parts[1])
		dimensions := strings.Split(resolution, "x")
		if len(dimensions) == 2 {
			width, _ := strconv.Atoi(dimensions[0])
			height, _ := strconv.Atoi(dimensions[1])
			return width, height, nil
		}
	}

	return 0, 0, fmt.Errorf("failed to parse screen resolution")
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

// getWallpaperService returns the singleton instance of wallpaperService.
func getWallpaperService(cfg *Config) *wallpaperService {
	wsOnce.Do(func() {
		// Initialize the wallpaper service for Linux
		currentOS := &linuxOS{}

		// Initialize the wallpaper service
		wsInstance = &wallpaperService{
			os:              currentOS,
			imgProcessor:    &smartImageProcessor{os: currentOS, aspectThreshold: 0.9, resampler: imaging.Lanczos}, // Initialize with smartCropper with a lenient threshold
			cfg:             cfg,
			downloadMutex:   sync.Mutex{},
			downloadHistory: []ImgSrvcImage{},
			seenHistory:     make(map[string]bool),
			currentPage:     1,                                        // Start with the first page,
			fitImage:        cfg.BoolWithFallback("Smart Fit", false), // Initialize with smart fit preference
		}
	})
	return wsInstance
}
