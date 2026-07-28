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

	"github.com/dixieflatline76/Spice/v2/util/log"
)

// SpiceTmpSuffix marks the hardlink aliases created by the macOS wallpaper
// setter to defeat WindowServer's URL-keyed render cache (see
// wallpaper_native_darwin.m). They share an inode with the real derivative,
// so leaving them behind pins the image data on disk even after the original
// is deleted.
const SpiceTmpSuffix = ".spice_tmp"

// spiceTmpMaxAge bounds how long an unreferenced hardlink alias may survive.
// The grace period avoids racing a wallpaper change that is still in flight:
// nativeSetWallpaper creates the link asynchronously on the main thread.
const spiceTmpMaxAge = 10 * time.Minute

// baseIDFromFileName derives an image ID from a file name, transparently
// unwrapping the ".spice_tmp" hardlink suffix.
//
// Every scanner in this file must agree on this mapping. Using filepath.Ext
// directly on "Wikimedia_123.jpg.spice_tmp" yields ".spice_tmp", which is why
// those aliases used to be invisible to both the orphan sweep and the batch
// delete, and accumulated indefinitely.
func baseIDFromFileName(name string) (id string, isTmp bool) {
	if strings.HasSuffix(strings.ToLower(name), SpiceTmpSuffix) {
		name = name[:len(name)-len(SpiceTmpSuffix)]
		isTmp = true
	}
	return strings.TrimSuffix(name, filepath.Ext(name)), isTmp
}

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

// DeepDeleteBatch removes all physical files (Master and Derivatives) associated with a list of IDs.
// It is significantly more efficient than calling DeepDelete in a loop for multiple IDs.
func (fm *FileManager) DeepDeleteBatch(ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	idMap := make(map[string]bool)
	for _, id := range ids {
		idMap[id] = true
	}

	filesToDelete := []string{}

	// 1. Scan Master (Root)
	entries, err := os.ReadDir(fm.rootDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			fileID, _ := baseIDFromFileName(name)
			if idMap[fileID] {
				filesToDelete = append(filesToDelete, filepath.Join(fm.rootDir, name))
				log.Debugf("DeepDeleteBatch: Found Master file %s", name)
			}
		}
	} else {
		log.Printf("DeepDeleteBatch: Failed to read root dir: %v", err)
	}

	// 2. Scan Derivatives (Recursive in FittedRoot)
	fittedRoot := filepath.Join(fm.rootDir, FittedRootDir)
	log.Debugf("DeepDeleteBatch: Scanning fitted root %s for %d IDs", fittedRoot, len(ids))

	err = filepath.Walk(fittedRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("DeepDeleteBatch: Error accessing path %s: %v", path, err)
			return nil // Skip access errors
		}
		if !info.IsDir() {
			name := info.Name()
			fileID, _ := baseIDFromFileName(name)
			if idMap[fileID] {
				filesToDelete = append(filesToDelete, path)
				log.Debugf("DeepDeleteBatch: Found Derivative file %s", path)
			}
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		log.Printf("DeepDeleteBatch: Error walking fitted dir: %v", err)
	}

	log.Debugf("DeepDeleteBatch: Total files to delete: %d", len(filesToDelete))

	for _, f := range filesToDelete {
		if err := os.Remove(f); err != nil {
			if !os.IsNotExist(err) {
				log.Printf("DeepDeleteBatch: Failed to delete %s: %v", f, err)
			}
		} else {
			log.Debugf("DeepDeleteBatch: Successfully deleted %s", f)
		}
	}

	return nil
}

// DeepDelete removes the Master image and ALL its derivatives.
// It searches for files with the given ID in all known directories.
func (fm *FileManager) DeepDelete(id string) error {
	log.Printf("[DeepDelete] Requested for ID: %s", id)
	return fm.DeepDeleteBatch([]string{id})
}

// isImageOrTmp reports whether a file name is one this sweep owns: a real
// image, or a ".spice_tmp" hardlink alias of one.
func isImageOrTmp(name string) bool {
	trimmed := name
	if strings.HasSuffix(strings.ToLower(trimmed), SpiceTmpSuffix) {
		trimmed = trimmed[:len(trimmed)-len(SpiceTmpSuffix)]
	}
	switch strings.ToLower(filepath.Ext(trimmed)) {
	case ".jpg", ".jpeg", ".png":
		return true
	default:
		return false
	}
}

// shouldDeleteOrphan decides the fate of one file during the orphan sweep.
//
// Real images are removed when their ID is unknown. Hardlink aliases are
// additionally removed once their ID is known but their base file is gone, or
// once they age out — an alias with no owner keeps the underlying inode alive
// forever. Aliases for a protected ID are always kept: the desktop may be
// pointing at that very URL right now, and removing it blanks the wallpaper.
func shouldDeleteOrphan(path, name string, isTmp bool, id string, knownIDs, protectedIDs map[string]bool, info os.FileInfo) bool {
	if protectedIDs[id] {
		return false
	}
	if !knownIDs[id] {
		return true
	}
	if !isTmp {
		return false
	}

	base := strings.TrimSuffix(path, SpiceTmpSuffix)
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return true
	}
	if info != nil && time.Since(info.ModTime()) > spiceTmpMaxAge {
		return true
	}
	return false
}

// CleanupOrphans removes files from the root directory and subdirectories
// that are NOT present in the knownIDs map.
//
// protectedIDs lists images currently displayed on a monitor; they and their
// hardlink aliases are never removed. Pass nil when no monitor is live (for
// example on the Wipe path).
func (fm *FileManager) CleanupOrphans(knownIDs map[string]bool, protectedIDs map[string]bool) {
	log.Print("FileManager: Starting orphan cleanup...")
	deletedCount := 0

	// 1. Clean Root (Masters)
	entries, err := os.ReadDir(fm.rootDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !isImageOrTmp(name) {
				continue
			}

			id, isTmp := baseIDFromFileName(name)
			fullPath := filepath.Join(fm.rootDir, name)
			info, _ := entry.Info()

			if shouldDeleteOrphan(fullPath, name, isTmp, id, knownIDs, protectedIDs, info) {
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
			if !isImageOrTmp(name) {
				return nil
			}

			id, isTmp := baseIDFromFileName(name)

			if shouldDeleteOrphan(path, name, isTmp, id, knownIDs, protectedIDs, info) {
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
			// baseIDFromFileName also matches the ".spice_tmp" hardlink
			// aliases, which are derivatives of this ID and must go with it.
			if fileID, _ := baseIDFromFileName(name); fileID == id {
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

// RenameAllAssets renames the Master image and ALL its derivatives to a new ID.
// This is used for namespacing migrations.
func (fm *FileManager) RenameAllAssets(oldID, newID string) error {
	log.Printf("[FileManager] Renaming assets: %s -> %s", oldID, newID)

	// 1. Rename Master (Root)
	entries, err := os.ReadDir(fm.rootDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			fileID, isTmp := baseIDFromFileName(name)
			if isTmp {
				// Hardlink aliases are disposable. Leaving one behind under the
				// old ID makes it an orphan, which the cleanup sweep reclaims.
				continue
			}
			ext := filepath.Ext(name)
			if fileID == oldID {
				oldPath := filepath.Join(fm.rootDir, name)
				newPath := filepath.Join(fm.rootDir, newID+ext)
				log.Debugf("FileManager: Renaming Master %s -> %s", oldPath, newPath)
				if err := os.Rename(oldPath, newPath); err != nil {
					log.Printf("FileManager: Failed to rename Master %s: %v", oldPath, err)
				}
			}
		}
	}

	// 2. Rename Derivatives (Recursive in FittedRoot)
	fittedRoot := filepath.Join(fm.rootDir, FittedRootDir)
	err = filepath.Walk(fittedRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			name := info.Name()
			fileID, isTmp := baseIDFromFileName(name)
			if isTmp {
				return nil // Disposable alias; the cleanup sweep reclaims it.
			}
			ext := filepath.Ext(name)
			if fileID == oldID {
				newPath := filepath.Join(filepath.Dir(path), newID+ext)
				log.Debugf("FileManager: Renaming Derivative %s -> %s", path, newPath)
				if err := os.Rename(path, newPath); err != nil {
					log.Printf("FileManager: Failed to rename Derivative %s: %v", path, err)
				}
			}
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		log.Printf("FileManager: Error walking fitted dir during rename: %v", err)
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
