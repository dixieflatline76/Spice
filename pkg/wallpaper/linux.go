//go:build linux
// +build linux

package wallpaper

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dixieflatline76/Spice/pkg/sysinfo"
)

// linuxOS implements the OS interface for Linux.
type linuxOS struct{}

// getOS returns a new instance of the linuxOS struct.
// Note: This logic is usually at the bottom or separate, but we are overriding.
func getOS() OS {
	// Simple check for Chrome OS environment marker
	// Crostini usually has /dev/.cros_milestone
	if _, err := os.Stat("/dev/.cros_milestone"); err == nil {
		return &ChromeOS{}
	}
	return &linuxOS{}
}

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
