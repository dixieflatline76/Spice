package wallpaper

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dixieflatline76/Spice/util/log"
)

// FileManager handles all file system operations for wallpaper images.
// It enforces the directory structure for Source + Derivative architecture.
type FileManager struct {
	rootDir string
}

// NewFileManager creates a new FileManager with the given root directory.
// The rootDir is typically ".../wallpaper_downloads".
func NewFileManager(rootDir string) *FileManager {
	return &FileManager{
		rootDir: rootDir,
	}
}

// GetDownloadDir returns the root directory where images are downloaded.
func (fm *FileManager) GetDownloadDir() string {
	return fm.rootDir
}

// EnsureDirs creates necessary subdirectories for derivatives.
func (fm *FileManager) EnsureDirs() error {
	dirs := []string{
		fm.rootDir,
		filepath.Join(fm.rootDir, FittedImgDir),
		filepath.Join(fm.rootDir, FittedFaceBoostImgDir),
		filepath.Join(fm.rootDir, FittedFaceCropImgDir),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// validateID ensures the ID does not contain path traversal characters.
func (fm *FileManager) validateID(id string) error {
	if strings.Contains(id, "..") || strings.Contains(id, string(filepath.Separator)) {
		return fmt.Errorf("invalid id: contains illegal characters")
	}
	// Strict: Alphanumeric + standard symbols (-_.) only?
	// For now, blocking traversal is the main goal.
	return nil
}

// GetMasterPath returns the absolute path for the Master (Raw) image.
// Master images are stored directly in the root directory.
func (fm *FileManager) GetMasterPath(id string, ext string) (string, error) {
	if err := fm.validateID(id); err != nil {
		return "", err
	}
	// Sanitize extension just in case
	if strings.Contains(ext, "..") || strings.Contains(ext, string(filepath.Separator)) {
		return "", fmt.Errorf("invalid extension")
	}
	return filepath.Join(fm.rootDir, id+ext), nil
}

// GetDerivativePath returns the path for a processed image based on the type.
func (fm *FileManager) GetDerivativePath(id string, ext string, derivativeType string) (string, error) {
	if err := fm.validateID(id); err != nil {
		return "", err
	}
	// Validate derivative type via whitelist? Or just directory check?
	// derivativeType should strictly be one of the known folders.
	// But as long as it doesn't have "..", it stays in root.
	if strings.Contains(derivativeType, "..") {
		return "", fmt.Errorf("invalid derivative type")
	}
	return filepath.Join(fm.rootDir, derivativeType, id+ext), nil
}

// DeepDelete removes the Master image and ALL its derivatives.
// It searches for files with the given ID in all known directories.
func (fm *FileManager) DeepDelete(id string) error {
	// 1. Delete Master (try common extensions if not provided, or search glob?)
	// Since we don't store extension in ID, we need to find the file.
	// Helper to find and delete by ID key.

	filesToDelete := []string{}

	// Helper to find file by ID in a dir
	findFile := func(dir string) string {
		matches, _ := filepath.Glob(filepath.Join(dir, id+".*"))
		if len(matches) > 0 {
			return matches[0] // Assume one file per ID per folder
		}
		return ""
	}

	// Master
	if f := findFile(fm.rootDir); f != "" {
		filesToDelete = append(filesToDelete, f)
	}

	// Derivatives
	subDirs := []string{FittedImgDir, FittedFaceBoostImgDir, FittedFaceCropImgDir}
	for _, sub := range subDirs {
		dir := filepath.Join(fm.rootDir, sub)
		if f := findFile(dir); f != "" {
			filesToDelete = append(filesToDelete, f)
		}
	}

	for _, f := range filesToDelete {
		if err := os.Remove(f); err != nil {
			// Log error but continue? Or return?
			// For deep clean, we want to try deleting everything.
			return fmt.Errorf("failed to delete %s: %v", f, err)
		}
	}

	return nil
}

// CleanupOrphans removes files from the root directory and subdirectories
// that are NOT present in the knownIDs map.
// It includes a pacer (sleep) to run as a low-priority background task.
func (fm *FileManager) CleanupOrphans(knownIDs map[string]bool) {
	log.Print("FileManager: Starting orphan cleanup...")
	deletedCount := 0

	// Helper to walk and clean
	cleanDir := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			log.Printf("FileManager: Failed to read dir %s: %v", dir, err)
			return
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			// Extract ID from filename (remove extension)
			ext := filepath.Ext(entry.Name())
			id := strings.TrimSuffix(entry.Name(), ext)

			if !knownIDs[id] {
				// ORPHAN! Matches no known ID.
				// (Assuming no other files exist in these folders except images)
				// Safeguard: Check if it looks like an image? NO, we control the folders.

				fullPath := filepath.Join(dir, entry.Name())
				// Pacer
				time.Sleep(100 * time.Millisecond)

				if err := os.Remove(fullPath); err != nil {
					log.Printf("FileManager: Failed to delete orphan %s: %v", fullPath, err)
				} else {
					// log.Debugf("FileManager: Deleted orphan %s", fullPath) // Verbose
					deletedCount++
				}
			}
		}
	}

	// Clean Root (Masters)
	cleanDir(fm.rootDir)

	// Clean Derivatives
	subDirs := []string{FittedImgDir, FittedFaceBoostImgDir, FittedFaceCropImgDir}
	for _, sub := range subDirs {
		cleanDir(filepath.Join(fm.rootDir, sub))
	}

	log.Printf("FileManager: Orphan cleanup finished. Removed %d files.", deletedCount)
}
