package wallpaper

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
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
	// Root dirs
	if err := os.MkdirAll(fm.rootDir, 0755); err != nil {
		return fmt.Errorf("failed to create root directory %s: %w", fm.rootDir, err)
	}

	fittedRoot := filepath.Join(fm.rootDir, FittedRootDir)

	// Create structure: fitted/{quality,flexibility}/{standard,faceboost,facecrop}
	modes := []string{QualityDir, FlexibilityDir}
	types := []string{StandardDir, FaceBoostDir, FaceCropDir}

	for _, mode := range modes {
		for _, t := range types {
			dir := filepath.Join(fittedRoot, mode, t)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
	}
	return nil
}

// validateID ensures the ID does not contain path traversal characters.
func (fm *FileManager) validateID(id string) error {
	if strings.Contains(id, "..") || strings.Contains(id, string(filepath.Separator)) {
		return fmt.Errorf("invalid id: contains illegal characters")
	}
	return nil
}

// GetMasterPath returns the absolute path for the Master (Raw) image.
// Master images are stored directly in the root directory.
func (fm *FileManager) GetMasterPath(id string, ext string) (string, error) {
	if err := fm.validateID(id); err != nil {
		return "", err
	}
	if strings.Contains(ext, "..") || strings.Contains(ext, string(filepath.Separator)) {
		return "", fmt.Errorf("invalid extension")
	}
	return filepath.Join(fm.rootDir, id+ext), nil
}

// GetDerivativePath returns the path for a processed image based on the type.
// derivativeType is now a relative path segment (e.g., "fitted/quality/standard")
func (fm *FileManager) GetDerivativePath(id string, ext string, derivativeType string) (string, error) {
	if err := fm.validateID(id); err != nil {
		return "", err
	}
	// Basic sanitation for derivative path segments
	if strings.Contains(derivativeType, "..") {
		return "", fmt.Errorf("invalid derivative type")
	}
	return filepath.Join(fm.rootDir, derivativeType, id+ext), nil
}

// DerivativeExists checks if a specific derivative exists on disk.
// derivativeDir should be the resolution folder name (e.g. "1920x1080")
func (fm *FileManager) DerivativeExists(id string, ext string, derivativeDir string) bool {
	path, err := fm.GetDerivativePath(id, ext, derivativeDir)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// DeepDelete removes the Master image and ALL its derivatives.
// It searches for files with the given ID in all known directories.
func (fm *FileManager) DeepDelete(id string) error {
	filesToDelete := []string{}

	// Helper to find file by ID in a dir
	findFile := func(dir string) string {
		matches, _ := filepath.Glob(filepath.Join(dir, id+".*"))
		if len(matches) > 0 {
			return matches[0] // Assume one file per ID per folder
		}
		return ""
	}

	// 1. Master (Root)
	if f := findFile(fm.rootDir); f != "" {
		filesToDelete = append(filesToDelete, f)
		log.Debugf("DeepDelete: Found Master file %s", f)
	}

	// 2. Derivatives (Recursive in FittedRoot)
	fittedRoot := filepath.Join(fm.rootDir, FittedRootDir)
	log.Debugf("DeepDelete: Scanning fitted root %s for ID %s", fittedRoot, id)

	err := filepath.Walk(fittedRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("DeepDelete: Error accessing path %s: %v", path, err)
			return nil // Skip access errors
		}
		if !info.IsDir() {
			name := info.Name()
			ext := filepath.Ext(name)
			fileID := strings.TrimSuffix(name, ext)
			if fileID == id {
				filesToDelete = append(filesToDelete, path)
				log.Debugf("DeepDelete: Found Derivative file %s", path)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("DeepDelete: Error walking fitted dir: %v", err)
	}

	log.Debugf("DeepDelete: Total files to delete for %s: %d", id, len(filesToDelete))

	for _, f := range filesToDelete {
		if err := os.Remove(f); err != nil {
			// Suppress benign errors
			if os.IsNotExist(err) {
				continue
			}
			// Suppress "used by another process" errors (benign race with active download/usage)
			if strings.Contains(err.Error(), "used by another process") || strings.Contains(err.Error(), "access is denied") {
				log.Debugf("DeepDelete: Skipped locked file %s: %v", f, err)
			} else {
				log.Printf("DeepDelete: Failed to delete %s: %v", f, err)
			}
		} else {
			log.Debugf("DeepDelete: Successfully deleted %s", f)
		}
	}

	return nil
}

// CleanupOrphans removes files from the root directory and subdirectories
// that are NOT present in the knownIDs map.
func (fm *FileManager) CleanupOrphans(knownIDs map[string]bool) {
	log.Print("FileManager: Starting orphan cleanup...")
	deletedCount := 0

	// 1. Clean Root (Masters)
	entries, err := os.ReadDir(fm.rootDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := filepath.Ext(entry.Name())
			id := strings.TrimSuffix(entry.Name(), ext)

			if !knownIDs[id] {
				fullPath := filepath.Join(fm.rootDir, entry.Name())
				time.Sleep(50 * time.Millisecond) // Pacer
				if err := os.Remove(fullPath); err == nil {
					deletedCount++
				}
			}
		}
	}

	// 2. Clean Derivatives (Recursive in FittedRoot)
	fittedRoot := filepath.Join(fm.rootDir, FittedRootDir)
	err = filepath.Walk(fittedRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			name := info.Name()
			ext := filepath.Ext(name)
			id := strings.TrimSuffix(name, ext)

			if !knownIDs[id] {
				// Orphan
				time.Sleep(50 * time.Millisecond) // Pacer
				if err := os.Remove(path); err == nil {
					deletedCount++
				}
			}
		}
		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		log.Printf("FileManager: Error walking fitted dir during cleanup: %v", err)
	}

	log.Printf("FileManager: Orphan cleanup finished. Removed %d files.", deletedCount)
}

// DeleteDerivatives removes ONLY the processed versions of an image, keeping the Master.
// This is used when invalidating cache due to settings changes (Smart Fit, etc.).
func (fm *FileManager) DeleteDerivatives(id string) error {
	filesToDelete := []string{}

	// Recursive walk in fitted directory
	fittedRoot := filepath.Join(fm.rootDir, FittedRootDir)
	err := filepath.Walk(fittedRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip access errors
		}
		if !info.IsDir() {
			name := info.Name()
			ext := filepath.Ext(name)
			fileID := strings.TrimSuffix(name, ext)
			if fileID == id {
				filesToDelete = append(filesToDelete, path)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("DeleteDerivatives: Error walking fitted dir: %v", err)
	}

	for _, f := range filesToDelete {
		if err := os.Remove(f); err != nil {
			// Suppress benign errors
			if strings.Contains(err.Error(), "used by another process") || strings.Contains(err.Error(), "access is denied") {
				log.Debugf("DeleteDerivatives: Skipped locked file %s: %v", f, err)
			} else {
				log.Printf("DeleteDerivatives: Failed to delete %s: %v", f, err)
			}
		}
	}

	return nil
}

// GetDimensions returns the width and height of an image file on disk.
func (fm *FileManager) GetDimensions(path string) (int, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	img, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, err
	}
	return img.Width, img.Height, nil
}
