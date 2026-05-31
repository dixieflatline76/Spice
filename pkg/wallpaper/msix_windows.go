//go:build windows
// +build windows

package wallpaper

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	modKernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentPackageFamilyName = modKernel32.NewProc("GetCurrentPackageFamilyName")
)

const (
	appmodelErrorNoPackage = 15700
)

// msixPackageFamilyName holds the cached package family name.
// Empty string means "not running in MSIX" (or detection failed).
var msixPackageFamilyName string

// msixWallpaperStaging is the directory where wallpapers are copied
// before being passed to IDesktopWallpaper::SetWallpaper.
// This ensures explorer.exe can always access the file.
var msixWallpaperStaging string

func init() {
	msixPackageFamilyName = detectMSIXFamilyName()
	if msixPackageFamilyName != "" {
		// Use %USERPROFILE%\Pictures\SpiceWallpapers as the staging area.
		// This is always accessible by explorer.exe and not subject to
		// any MSIX container virtualization.
		home, err := os.UserHomeDir()
		if err == nil {
			msixWallpaperStaging = filepath.Join(home, "Pictures", "SpiceWallpapers")
			_ = os.MkdirAll(msixWallpaperStaging, 0755)
		}
		log.Printf("MSIX: Running in package %s", msixPackageFamilyName)
		log.Printf("MSIX: Wallpaper staging dir: %s", msixWallpaperStaging)
	}
}

// detectMSIXFamilyName calls GetCurrentPackageFamilyName to determine
// if this process is running inside an MSIX/AppX container. Returns the
// package family name, or "" if not packaged.
func detectMSIXFamilyName() string {
	// First call: get required buffer length
	var length uint32
	r, _, _ := procGetCurrentPackageFamilyName.Call(
		uintptr(unsafe.Pointer(&length)),
		0, // nil buffer
	)

	// APPMODEL_ERROR_NO_PACKAGE means we are NOT in an MSIX container
	if r == appmodelErrorNoPackage {
		return ""
	}

	// ERROR_INSUFFICIENT_BUFFER (122) is expected — length now contains the required size
	if length == 0 {
		return ""
	}

	// Second call: retrieve the family name
	buf := make([]uint16, length)
	r, _, _ = procGetCurrentPackageFamilyName.Call(
		uintptr(unsafe.Pointer(&length)),
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if r != 0 {
		return ""
	}

	return syscall.UTF16ToString(buf)
}

// resolveMSIXPath ensures the wallpaper file is accessible by explorer.exe
// when running inside an MSIX package.
//
// MSIX container boundaries can prevent explorer.exe (which runs outside the
// container) from accessing files written by the packaged app, even when they
// appear to exist at a normal path. This function copies the wallpaper to a
// staging directory in %USERPROFILE%\Pictures that is always accessible.
func resolveMSIXPath(imagePath string) string {
	if msixPackageFamilyName == "" || msixWallpaperStaging == "" {
		return imagePath // Not in MSIX, nothing to do
	}

	// Verify the source file exists before copying
	if _, err := os.Stat(imagePath); err != nil {
		log.Printf("MSIX: Source file does not exist: %s", imagePath)
		return imagePath
	}

	// Copy to staging directory, preserving just the filename
	filename := filepath.Base(imagePath)
	stagedPath := filepath.Join(msixWallpaperStaging, filename)

	// Skip if already staged and same size
	srcInfo, _ := os.Stat(imagePath)
	if dstInfo, err := os.Stat(stagedPath); err == nil {
		if srcInfo != nil && dstInfo.Size() == srcInfo.Size() {
			return stagedPath
		}
	}

	if err := copyFile(imagePath, stagedPath); err != nil {
		log.Printf("MSIX: Failed to stage wallpaper %s -> %s: %v", imagePath, stagedPath, err)
		return imagePath // Fall back to original path
	}

	return stagedPath
}

// copyFile copies src to dst atomically using a temp file + rename.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	// Write to a temp file in the same directory, then rename
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}

	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("copy: %w", err)
	}
	if closeErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("close: %w", closeErr)
	}

	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}
