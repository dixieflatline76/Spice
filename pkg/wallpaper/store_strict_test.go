package wallpaper

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/stretchr/testify/assert"
)

// TestStrictSync_LegacySafety ensures that passing a nil active map (legacy behavior)
// does NOT prune any images, even if they have source IDs.
func TestStrictSync_LegacySafety(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, cacheFile)

	// Setup: Image with a SourceID
	img1 := provider.Image{ID: "img1", SourceQueryID: "q1", FilePath: filepath.Join(tmpDir, "img1.jpg")}
	assert.NoError(t, os.WriteFile(img1.FilePath, []byte("data"), 0644))
	store.Add(img1)

	// Run Sync with nil active map
	store.Sync(100, nil, nil)

	// Verify: Image should still exist
	assert.Equal(t, 1, store.Count())
	known := store.GetKnownIDs()
	assert.True(t, known["img1"])

	// Verify file exists
	_, err := os.Stat(img1.FilePath)
	assert.NoError(t, err)
}

// TestStrictSync_UncheckAll verifies that passing an empty map (simulating "Uncheck All Queries")
// prunes ALL images that have specific SourceQueryIDs, but leaves untagged images safe.
func TestStrictSync_UncheckAll(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, cacheFile)

	// img1: From q1
	img1 := provider.Image{ID: "img1", SourceQueryID: "q1", FilePath: filepath.Join(tmpDir, "img1.jpg")}
	// img2: Manual/Untagged (should remain)
	img2 := provider.Image{ID: "img2", SourceQueryID: "", FilePath: filepath.Join(tmpDir, "img2.jpg")}

	assert.NoError(t, os.WriteFile(img1.FilePath, []byte("1"), 0644))
	assert.NoError(t, os.WriteFile(img2.FilePath, []byte("2"), 0644))
	store.Add(img1)
	store.Add(img2)

	// Run Sync with EMPTY map (Active = None)
	activeIDs := make(map[string]bool)
	store.Sync(100, nil, activeIDs)

	// Verify: img1 gone, img2 stays
	assert.Equal(t, 1, store.Count())
	known := store.GetKnownIDs()
	assert.False(t, known["img1"], "img1 should be pruned")
	assert.True(t, known["img2"], "img2 (untagged) should remain")

	// Verify files
	assert.Eventually(t, func() bool {
		_, err1 := os.Stat(img1.FilePath)
		_, err2 := os.Stat(img2.FilePath)
		return os.IsNotExist(err1) && err2 == nil
	}, 2*time.Second, 100*time.Millisecond)
}

// TestStrictSync_MixedState verifies a complex scenario with active, inactive, and untagged images.
func TestStrictSync_MixedState(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, cacheFile)

	// Setup 4 images
	// 1. qA (Active) -> Keep
	// 2. qB (Inactive) -> Delete
	// 3. qA (Active, but file missing) -> Delete (Validation logic)
	// 4. Untagged -> Keep

	img1 := provider.Image{ID: "img1", SourceQueryID: "qA", FilePath: filepath.Join(tmpDir, "img1.jpg")}
	img2 := provider.Image{ID: "img2", SourceQueryID: "qB", FilePath: filepath.Join(tmpDir, "img2.jpg")}
	img3 := provider.Image{ID: "img3", SourceQueryID: "qA", FilePath: filepath.Join(tmpDir, "img3.jpg")}
	img4 := provider.Image{ID: "img4", SourceQueryID: "", FilePath: filepath.Join(tmpDir, "img4.jpg")}

	// Create files for 1, 2, 4. Skip 3.
	assert.NoError(t, os.WriteFile(img1.FilePath, []byte("1"), 0644))
	assert.NoError(t, os.WriteFile(img2.FilePath, []byte("2"), 0644))
	// img3 file missing
	assert.NoError(t, os.WriteFile(img4.FilePath, []byte("4"), 0644))

	store.Add(img1)
	store.Add(img2)
	store.Add(img3)
	store.Add(img4)

	// Active Set: Only qA
	activeIDs := map[string]bool{"qA": true}

	store.Sync(100, nil, activeIDs)

	// Verification
	known := store.GetKnownIDs()

	// 1. img1: Active Query & File Exists -> KEEP
	assert.True(t, known["img1"])
	_, err := os.Stat(img1.FilePath)
	assert.NoError(t, err)

	// 2. img2: Inactive Query -> DELETE
	assert.False(t, known["img2"])

	// 3. img3: Active Query but File Missing -> DELETE (Validation)
	assert.False(t, known["img3"])

	// 4. img4: Untagged -> KEEP
	assert.True(t, known["img4"])
	_, err = os.Stat(img4.FilePath)
	assert.NoError(t, err)

	// Verify async file deletion
	assert.Eventually(t, func() bool {
		_, err2 := os.Stat(img2.FilePath)
		return os.IsNotExist(err2)
	}, 2*time.Second, 100*time.Millisecond)
}
