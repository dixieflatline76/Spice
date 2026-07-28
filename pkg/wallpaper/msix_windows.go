//go:build windows
// +build windows

package wallpaper

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/dixieflatline76/Spice/v2/config"
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

// Staging directories outside the MSIX container boundary.
var (
	msixPictureStaging  string // %USERPROFILE%\Pictures\SpiceWallpapers
	msixDocumentStaging string // %USERPROFILE%\Documents\Spice
)

func init() {
	msixPackageFamilyName = detectMSIXFamilyName()
	if msixPackageFamilyName != "" {
		home, err := os.UserHomeDir()
		if err == nil {
			msixPictureStaging = filepath.Join(home, "Pictures", "SpiceWallpapers")
			_ = os.MkdirAll(msixPictureStaging, 0755)

			msixDocumentStaging = filepath.Join(home, "Documents", "Spice")
			_ = os.MkdirAll(msixDocumentStaging, 0755)
		}
		log.Printf("MSIX: Running in package %s", msixPackageFamilyName)
		log.Printf("MSIX: Picture staging dir: %s", msixPictureStaging)
		log.Printf("MSIX: Document staging dir: %s", msixDocumentStaging)
	}
}

// detectMSIXFamilyName calls GetCurrentPackageFamilyName to determine
// if this process is running inside an MSIX/AppX container. Returns the
// package family name, or "" if not packaged.
func detectMSIXFamilyName() string {
	var length uint32
	r, _, _ := procGetCurrentPackageFamilyName.Call(
		uintptr(unsafe.Pointer(&length)),
		0,
	)

	if r == appmodelErrorNoPackage {
		return ""
	}

	if length == 0 {
		return ""
	}

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

// resolveMSIXPicturePath ensures image/wallpaper files are accessible by explorer.exe or Windows APIs
// outside the MSIX container by staging them into %USERPROFILE%\Pictures\SpiceWallpapers.
func resolveMSIXPicturePath(imagePath string) string {
	return resolveMSIXStagedPath(imagePath, msixPictureStaging)
}

// resolveMSIXDocumentPath ensures HTML/document files are accessible by external applications (e.g. web browsers)
// outside the MSIX container by staging them into %USERPROFILE%\Documents\Spice.
func resolveMSIXDocumentPath(docPath string) string {
	return resolveMSIXStagedPath(docPath, msixDocumentStaging)
}


// resolveMSIXStagedPath performs the generic staging copy logic relative to baseStagingDir.
func resolveMSIXStagedPath(srcPath, baseStagingDir string) string {
	if msixPackageFamilyName == "" || baseStagingDir == "" {
		return srcPath // Not in MSIX, nothing to do
	}

	// Verify the source file exists before copying
	if _, err := os.Stat(srcPath); err != nil {
		log.Printf("MSIX: Source file does not exist: %s", srcPath)
		return srcPath
	}

	// Preserve directory structure relative to WorkingDir if applicable
	var stagedPath string
	workingDir := config.GetWorkingDir()
	cleanSrc := filepath.Clean(srcPath)
	cleanWork := filepath.Clean(workingDir)
	if strings.HasPrefix(cleanSrc, cleanWork) {
		relPath, err := filepath.Rel(cleanWork, cleanSrc)
		if err == nil {
			stagedPath = filepath.Join(baseStagingDir, relPath)
		} else {
			stagedPath = filepath.Join(baseStagingDir, filepath.Base(srcPath))
		}
	} else {
		stagedPath = filepath.Join(baseStagingDir, filepath.Base(srcPath))
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(stagedPath), 0755); err != nil {
		log.Printf("MSIX: Failed to create parent directory for %s: %v", stagedPath, err)
		return srcPath
	}

	// Skip if already staged and same size
	srcInfo, _ := os.Stat(srcPath)
	if dstInfo, err := os.Stat(stagedPath); err == nil {
		if srcInfo != nil && dstInfo.Size() == srcInfo.Size() {
			return stagedPath
		}
	}

	if err := copyFile(srcPath, stagedPath); err != nil {
		log.Printf("MSIX: Failed to stage file %s -> %s: %v", srcPath, stagedPath, err)
		return srcPath
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
