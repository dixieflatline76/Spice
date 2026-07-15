package wallpaper

import (
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
)

func newZombieTestStore(t *testing.T, tmpDir string) (*ImageStore, *FileManager) {
	t.Helper()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))
	return store, fm
}

func defaultTargetFlags() map[string]bool {
	return map[string]bool{
		"SmartFit":  true,
		"FaceCrop":  false,
		"FaceBoost": false,
		"Upscale":   false,
		"Fill":      false,
	}
}

func newHealthyImage(t *testing.T, fm *FileManager, id, queryID, resolution string, flags map[string]bool) provider.Image {
	t.Helper()
	createMasterFile(t, fm, id)
	derivPath := filepath.Join(fm.rootDir, "fitted", id+"_"+resolution+".jpg")
	return provider.Image{
		ID:              id,
		SourceQueryID:   queryID,
		ProcessingFlags: copyFlags(flags),
		DerivativePaths: map[string]string{resolution: derivPath},
	}
}

func newZombieImage(t *testing.T, fm *FileManager, id, queryID string, flags map[string]bool) provider.Image {
	t.Helper()
	createMasterFile(t, fm, id)
	return provider.Image{
		ID:              id,
		SourceQueryID:   queryID,
		ProcessingFlags: copyFlags(flags),
		DerivativePaths: map[string]string{},
	}
}
