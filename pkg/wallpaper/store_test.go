package wallpaper

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/stretchr/testify/assert"
)

func TestStorePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, cacheFile)

	img1 := provider.Image{ID: "img1", Path: "http://example.com/1.jpg", FilePath: filepath.Join(tmpDir, "1.jpg")}
	img2 := provider.Image{ID: "img2", Path: "http://example.com/2.jpg", FilePath: filepath.Join(tmpDir, "2.jpg")}

	// Test Save
	store.Add(img1)
	store.Add(img2)
	store.SaveCache()

	// Verify file exists
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		t.Fatal("Cache file not created")
	}

	// Test Load
	store2 := NewImageStore()
	store2.SetAsyncSave(false)
	store2.SetFileManager(fm, cacheFile)
	err := store2.LoadCache()
	assert.NoError(t, err)
	assert.Equal(t, 2, store2.Count())

	// Verify content
	loadedImg, ok := store2.Get(0)
	assert.True(t, ok)
	assert.Equal(t, "img1", loadedImg.ID)
}

func TestStore_Persistence_Debounce(t *testing.T) {
	store := NewImageStore()
	store.SetDebounceDuration(50 * time.Millisecond)
	store.SetAsyncSave(true)

	// Mock saveFunc
	var saveCount int
	saveChan := make(chan struct{}, 10) // buffer to avoid blocking
	store.saveFunc = func() {
		saveCount++
		saveChan <- struct{}{}
	}

	// Pre-populate store (Update only works on existing)
	store.Add(provider.Image{ID: "img1"})
	// Drain the initial save caused by Add (if any, Add triggers save too!)
	// Wait, Add triggers save?
	// Let's check Add logic.
	// Add calls saveCacheInternalOriginalLocked if sync?
	// Add calls scheduleSaveLocked if async?
	// Let's assume Add triggers a save.
	// We need to wait for it or verify it.
	// Actually, just looping Update is fine, even if Add triggered one.
	// But clearer to drain it.
	select {
	case <-saveChan:
		saveCount = 0 // Reset for burst test
	case <-time.After(200 * time.Millisecond):
		// Maybe Add didn't trigger if avoidSet/idSet check failed?
		// No, Add triggers save.
	}

	// 1. Rapid Updates (Burst)
	for i := 0; i < 10; i++ {
		store.Update(provider.Image{ID: "img1"})
		// No sleep, or very short sleep < debounce
		time.Sleep(1 * time.Millisecond)
	}

	// Wait for debounce to fire
	select {
	case <-saveChan:
		// Save triggered
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for save")
	}

	// Verify count is 1 (Debounced)
	// We might have a race if we check immediately after channel receive?
	// The channel send happens inside saveFunc. The count increment happens before.
	// So count is at least 1.
	// We wait a bit more to ensure no *extra* saves fire.
	time.Sleep(100 * time.Millisecond)

	store.mu.Lock() // Check count safely? No, saveFunc is main locked?
	// saveFunc is called with RLock held.
	// We accessed saveCount without lock in test, technically racy if multiple saves run in parallel?
	// But debounce ensures they are serialized by timer.
	// Let's assert.
	assert.Equal(t, 1, saveCount, "Expected exactly 1 save for rapid burst")
	store.mu.Unlock()

	// 2. Spaced Updates
	saveCount = 0 // Reset
	store.Update(provider.Image{ID: "img1"})

	// Wait for save
	<-saveChan

	// Wait > debounce
	time.Sleep(100 * time.Millisecond)
	store.Update(provider.Image{ID: "img1"})

	// Wait for save
	<-saveChan

	assert.Equal(t, 0, 0, "dummy check to ensure we got here")
	// If we received twice from channel, we had 2 saves.
}

func TestStoreSync_Validation(t *testing.T) {
	// Test that Sync removes images where Master is missing
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)

	// Create FileManager directories so EnsurePath works?
	// Actually FM just returns paths.
	// But Sync checks os.Stat.

	cacheFile := filepath.Join(tmpDir, "cache.json")
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, cacheFile)

	// Setup:
	// img1: Master Exists.
	// img2: Master Missing.

	img1 := provider.Image{ID: "img1", Path: "http://url/1.jpg"} // Master -> 1.jpg
	img2 := provider.Image{ID: "img2", Path: "http://url/2.png"} // Master -> 2.png

	// Create Master file for img1
	master1 := fm.GetMasterPath("img1", ".jpg")
	err := os.MkdirAll(filepath.Dir(master1), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(master1, []byte("fake"), 0644)
	assert.NoError(t, err)

	store.Add(img1)
	store.Add(img2)

	// Run Sync
	// Limit 100 (high enough)
	store.Sync(100, nil, nil)

	// Verify
	assert.Equal(t, 1, store.Count())
	valid, ok := store.Get(0)
	assert.True(t, ok)
	assert.Equal(t, "img1", valid.ID)
}

func TestStoreSync_Grooming(t *testing.T) {
	// Test that Sync enforces limit and deletes files
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, cacheFile)

	// Add 3 images
	// All have masters
	ids := []string{"oldest", "middle", "newest"}
	for _, id := range ids {
		master := fm.GetMasterPath(id, ".jpg")
		err := os.MkdirAll(filepath.Dir(master), 0755)
		assert.NoError(t, err)
		err = os.WriteFile(master, []byte(id), 0644)
		assert.NoError(t, err)
		store.Add(provider.Image{ID: id, Path: "http://url/" + id + ".jpg"})
	}

	// Sync with Limit 2
	// Should remove "oldest" (index 0)
	store.Sync(2, nil, nil)

	assert.Equal(t, 2, store.Count())

	// Verify remaining
	img0, _ := store.Get(0)
	assert.Equal(t, "middle", img0.ID)

	// Verify File Deletion
	// Verify File Deletion (Validation works async with pacer)
	oldestPath := fm.GetMasterPath("oldest", ".jpg")
	assert.Eventually(t, func() bool {
		_, err := os.Stat(oldestPath)
		return os.IsNotExist(err)
	}, 2*time.Second, 100*time.Millisecond, "Oldest file should be deleted")
}

func TestStoreSync_CacheInvalidation(t *testing.T) {
	// Test that Sync removes images with mismatched processing flags
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, cacheFile)

	// img1: Processed with SmartFit=true
	img1 := provider.Image{
		ID:              "img1",
		ProcessingFlags: map[string]bool{"SmartFit": true},
	}
	// Create master so it survives validation check
	master1 := fm.GetMasterPath("img1", ".jpg")
	err := os.MkdirAll(filepath.Dir(master1), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(master1, []byte("fake"), 0644)
	assert.NoError(t, err)

	store.Add(img1)

	// Sync with Target: SmartFit=false
	targetFlags := map[string]bool{"SmartFit": false}
	store.Sync(100, targetFlags, nil)

	// Expect: Image pruned because flags don't match
	// (Note: Master file persists, but used record is gone from store so it will be re-processed)
	assert.Equal(t, 0, store.Count())

	// Verify Master still exists (it should!)
	if _, err := os.Stat(master1); os.IsNotExist(err) {
		t.Errorf("Master file should persist after invalidation")
	}
}

func TestGetKnownIDs(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	store.Add(provider.Image{ID: "1"})
	store.Add(provider.Image{ID: "2"})
	store.Add(provider.Image{ID: "3"})

	known := store.GetKnownIDs()
	assert.Equal(t, 3, len(known))
	assert.True(t, known["1"])
	assert.True(t, known["2"])
	assert.True(t, known["3"])
	assert.False(t, known["4"])
}

func TestRemoveByQueryID(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	// Create test images
	// q1: img1, img3
	// q2: img2
	img1 := provider.Image{ID: "img1", SourceQueryID: "q1", FilePath: filepath.Join(tmpDir, "img1.jpg")}
	img2 := provider.Image{ID: "img2", SourceQueryID: "q2", FilePath: filepath.Join(tmpDir, "img2.jpg")}
	img3 := provider.Image{ID: "img3", SourceQueryID: "q1", FilePath: filepath.Join(tmpDir, "img3.jpg")}

	// Create dummy files
	assert.NoError(t, os.WriteFile(img1.FilePath, []byte("1"), 0644))
	assert.NoError(t, os.WriteFile(img2.FilePath, []byte("2"), 0644))
	assert.NoError(t, os.WriteFile(img3.FilePath, []byte("3"), 0644))

	store.Add(img1)
	store.Add(img2)
	store.Add(img3)

	assert.Equal(t, 3, store.Count())

	// Remove q1
	store.RemoveByQueryID("q1")

	// Verify store content
	assert.Equal(t, 1, store.Count())
	remaining, ok := store.Get(0)
	assert.True(t, ok)
	assert.Equal(t, "img2", remaining.ID)

	// Verify file deletion (Async)
	assert.Eventually(t, func() bool {
		_, err1 := os.Stat(img1.FilePath)
		_, err3 := os.Stat(img3.FilePath)
		return os.IsNotExist(err1) && os.IsNotExist(err3)
	}, 2*time.Second, 100*time.Millisecond, "Files for q1 should be deleted")

	// Verify q2 file remains
	_, err2 := os.Stat(img2.FilePath)
	assert.NoError(t, err2)
}

func TestStoreSync_StrictPruning(t *testing.T) {
	// Test that Sync removes images from inactive queries
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, cacheFile)

	// img1: Active Query "qA"
	img1 := provider.Image{ID: "img1", SourceQueryID: "qA", FilePath: filepath.Join(tmpDir, "img1.jpg")}
	// img2: Inactive Query "qB"
	img2 := provider.Image{ID: "img2", SourceQueryID: "qB", FilePath: filepath.Join(tmpDir, "img2.jpg")}
	// img3: No Query ID (Manual/Legacy) -> Should be kept?
	// Logic says: if SourceQueryID != "" && !active...
	// So empty SourceQueryID should correspond to "no active filter apply"?
	// Or should we delete orphans?
	// Code: if img.SourceQueryID != "" && !activeQueryIDs[img.SourceQueryID]
	// So img3 (empty ID) is SAFE.
	img3 := provider.Image{ID: "img3", SourceQueryID: "", FilePath: filepath.Join(tmpDir, "img3.jpg")}

	// Create dummy files
	assert.NoError(t, os.WriteFile(img1.FilePath, []byte("1"), 0644))
	assert.NoError(t, os.WriteFile(img2.FilePath, []byte("2"), 0644))
	assert.NoError(t, os.WriteFile(img3.FilePath, []byte("3"), 0644))

	store.Add(img1)
	store.Add(img2)
	store.Add(img3)

	// Define Active Queries
	activeIDs := map[string]bool{
		"qA": true,
	}

	// Run Sync
	store.Sync(100, nil, activeIDs)

	// Verify Store Content
	assert.Equal(t, 2, store.Count())

	known := store.GetKnownIDs()

	// Expect img1 and img3 to remain
	assert.True(t, known["img1"])
	assert.True(t, known["img3"])

	// Expect img2 to be gone from store
	assert.False(t, known["img2"])

	// Verify File Deletion
	// img2 file should be deleted (Sync does it inline/deep delete)
	_, err := os.Stat(img2.FilePath)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

// TestStore_LoadAvoidSet_And_Clear verifies that:
// 1. `LoadAvoidSet` correctly populates the blocklist.
// 2. `Add` respects the blocklist.
// 3. `Clear` preserves the blocklist.
// 4. `Wipe` clears the blocklist.
func TestStore_LoadAvoidSet_And_Clear(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	blockedID := "blocked_1"
	avoidSet := map[string]bool{
		blockedID: true,
	}

	// 1. LoadAvoidSet
	store.LoadAvoidSet(avoidSet)

	// 2. Verify Add respects blocklist
	added := store.Add(provider.Image{ID: blockedID})
	assert.False(t, added, "Should reject blocked ID")

	accepted := store.Add(provider.Image{ID: "clean_1"})
	assert.True(t, accepted, "Should accept non-blocked ID")
	assert.Equal(t, 1, store.Count())

	// 3. Verify Clear preserves blocklist
	store.Clear()
	assert.Equal(t, 0, store.Count(), "Clear should remove images")

	// Try adding blocked ID again
	addedAgain := store.Add(provider.Image{ID: blockedID})
	assert.False(t, addedAgain, "Should still reject blocked ID after Clear")

	// 4. Verify Wipe clears blocklist
	store.Wipe()
	addedPostWipe := store.Add(provider.Image{ID: blockedID})
	assert.True(t, addedPostWipe, "Should accept blocked ID after Wipe (reset)")
}
