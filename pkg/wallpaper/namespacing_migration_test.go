package wallpaper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_NamespacingMigration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "spice-migration-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	fm := NewFileManager(tmpDir)
	err = fm.EnsureDirs()
	require.NoError(t, err)

	// 1. Create a legacy image on disk
	oldID := "123"
	ext := ".jpg"
	masterPath := filepath.Join(tmpDir, oldID+ext)
	err = os.WriteFile(masterPath, []byte("fake-image-data"), 0644)
	require.NoError(t, err)

	// 2. Create a derivative image on disk
	derivDir := filepath.Join(tmpDir, FittedRootDir, QualityDir, StandardDir)
	derivPath := filepath.Join(derivDir, oldID+ext)
	err = os.WriteFile(derivPath, []byte("fake-deriv-data"), 0644)
	require.NoError(t, err)

	// 3. Create a legacy cache file
	legacyImage := provider.Image{
		ID:       oldID,
		Provider: "Wallhaven",
		FilePath: masterPath,
		DerivativePaths: map[string]string{
			"standard": derivPath,
		},
	}
	cacheFile := filepath.Join(tmpDir, "image_cache_map.json")
	cacheData, _ := json.Marshal([]provider.Image{legacyImage})
	err = os.WriteFile(cacheFile, cacheData, 0644)
	require.NoError(t, err)

	// 4. Initialize store and load cache
	store := NewImageStore()
	store.SetFileManager(fm, cacheFile)
	err = store.LoadCache()
	require.NoError(t, err)

	// 5. Verify Migration
	newID := "Wallhaven_123"

	// Check memory state
	img, ok := store.GetByID(newID)
	assert.True(t, ok, "Image should be found by new ID")
	assert.Equal(t, newID, img.ID)

	// Check filesystem (Master)
	newMasterPath := filepath.Join(tmpDir, newID+ext)
	_, err = os.Stat(newMasterPath)
	assert.NoError(t, err, "Master file should be renamed")
	_, err = os.Stat(masterPath)
	assert.Error(t, err, "Old master file should be gone")

	// Check filesystem (Derivative)
	newDerivPath := filepath.Join(derivDir, newID+ext)
	_, err = os.Stat(newDerivPath)
	assert.NoError(t, err, "Derivative file should be renamed")

	// Check updated metadata
	assert.Contains(t, img.FilePath, newID)
	assert.Contains(t, img.DerivativePaths["standard"], newID)
}

func TestConfig_AvoidSetFuzzyMatch(t *testing.T) {
	cfg := &Config{
		Preferences: NewMockPreferences(),
	}
	cfg.AddToAvoidSet("123") // Legacy ID

	assert.True(t, cfg.InAvoidSet("123"), "Exact legacy match should work")
	assert.True(t, cfg.InAvoidSet("Wallhaven_123"), "Fuzzy namespaced match should work")
	assert.False(t, cfg.InAvoidSet("Other_1234"), "Non-matching ID should fail")
	assert.True(t, cfg.InAvoidSet("WrongPrefix_123"), "Fuzzy match should work regardless of prefix for legacy IDs")
}
