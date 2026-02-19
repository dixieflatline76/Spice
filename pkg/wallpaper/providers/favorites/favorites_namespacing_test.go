package favorites

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/stretchr/testify/assert"
)

func TestFavorites_HandlesNamespacedIDs(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	cfg := &wallpaper.Config{} // Mock or minimal config
	favProv := NewProvider(cfg)
	favProv.SetTestConfig("localhost", tmpDir)

	// Create a dummy image file to "favorite"
	// In the real app, this file comes from the cache/download dir.
	cacheDir := t.TempDir()
	namespacedID := "Pexels_12345"
	filename := namespacedID + ".jpg"
	srcPath := filepath.Join(cacheDir, filename)
	err := os.WriteFile(srcPath, []byte("dummy image content"), 0644)
	assert.NoError(t, err)

	img := provider.Image{
		ID:          namespacedID,
		Provider:    "Pexels",
		FilePath:    srcPath,
		Attribution: "Photographer X",
		ViewURL:     "http://pexels.com/12345",
	}

	// 1. Verify Initial State
	assert.False(t, favProv.IsFavorited(img), "Image should not be favorited initially")

	// 2. Add Favorite
	err = favProv.AddFavorite(img)
	assert.NoError(t, err)

	// Wait for async worker to process the add job
	assert.Eventually(t, func() bool {
		return favProv.IsFavorited(img)
	}, 1*time.Second, 50*time.Millisecond, "Image should be favorited after AddFavorite")

	// 3. Verify File Storage
	// The file should be copied to tmpDir/Pexels_12345.jpg
	destPath := filepath.Join(tmpDir, filename)
	assert.Eventually(t, func() bool {
		_, err := os.Stat(destPath)
		return err == nil
	}, 1*time.Second, 50*time.Millisecond, "Favorite file should exist in favorites directory")

	// Wait for metadata to be updated (since NewProvider loads it once)
	metaFile := filepath.Join(tmpDir, "metadata.json")
	assert.Eventually(t, func() bool {
		content, err := os.ReadFile(metaFile)
		return err == nil && string(content) != "" // meaningful content check if possible, but at least exists
	}, 1*time.Second, 50*time.Millisecond, "Metadata file should exist")

	// Better: wait until metadata contains our ID
	assert.Eventually(t, func() bool {
		content, err := os.ReadFile(metaFile)
		return err == nil && string(content) != "" && strings.Contains(string(content), namespacedID)
	}, 2*time.Second, 100*time.Millisecond, "Metadata should contain the ID")

	// Small buffer for Windows FS stability after write
	time.Sleep(100 * time.Millisecond)

	// 4. Verify Persistence (Reload Provider)
	// Create a new provider instance pointing to the same dir to simulate app restart
	favProv2 := NewProvider(cfg)
	favProv2.SetTestConfig("localhost", tmpDir)
	// Trigger load (usually happens in NewProvider but we need to ensure loadInitialMetadata ran)
	// loadInitialMetadata is called in NewProvider.

	assert.True(t, favProv2.IsFavorited(img), "Image should be recognized as favorite after reload")

	// 5. Remove Favorite
	err = favProv.RemoveFavorite(img)
	assert.NoError(t, err)

	// Wait for async worker to process remove
	assert.Eventually(t, func() bool {
		return !favProv.IsFavorited(img)
	}, 1*time.Second, 50*time.Millisecond, "Image should NOT be favorited after RemoveFavorite")

	// 6. Verify File Removal
	assert.Eventually(t, func() bool {
		_, err := os.Stat(destPath)
		return os.IsNotExist(err)
	}, 1*time.Second, 50*time.Millisecond, "Favorite file should be removed")
}
