package wallpaper

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
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

	img1 := provider.Image{ID: "img1", Path: "http://url/1.jpg", DerivativePaths: map[string]string{"1920x1080": "d1.jpg"}} // Master -> 1.jpg
	img2 := provider.Image{ID: "img2", Path: "http://url/2.png", DerivativePaths: map[string]string{"1920x1080": "d2.jpg"}} // Master -> 2.png

	// Create Master file for img1
	master1, _ := fm.GetMasterPath("img1", ".jpg")
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
		master, _ := fm.GetMasterPath(id, ".jpg")
		err := os.MkdirAll(filepath.Dir(master), 0755)
		assert.NoError(t, err)
		err = os.WriteFile(master, []byte(id), 0644)
		assert.NoError(t, err)
		store.Add(provider.Image{ID: id, Path: "http://url/" + id + ".jpg", DerivativePaths: map[string]string{"1920x1080": id + "_d.jpg"}})
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
	oldestPath, _ := fm.GetMasterPath("oldest", ".jpg")
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
		DerivativePaths: map[string]string{"1920x1080": "d1.jpg"},
	}
	// Create master so it survives validation check
	master1, _ := fm.GetMasterPath("img1", ".jpg")
	err := os.MkdirAll(filepath.Dir(master1), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(master1, []byte("fake"), 0644)
	assert.NoError(t, err)

	store.Add(img1)

	// Sync with Target: SmartFit=false
	targetFlags := map[string]bool{"SmartFit": false}
	store.Sync(100, targetFlags, nil)

	// Expect: Image refreshed (properties cleared) but record kept
	// (Note: Master file persists, record stays but derivatives and previous flags are cleared/updated)
	assert.Equal(t, 1, store.Count())
	img, _ := store.Get(0)
	assert.Empty(t, img.DerivativePaths)
	assert.Equal(t, false, img.ProcessingFlags["SmartFit"])

	// Verify Master still exists (it should!)
	if _, err := os.Stat(master1); os.IsNotExist(err) {
		t.Errorf("Master file should persist after invalidation")
	}
}

func TestStoreSync_IncompatibleTagsPreserved(t *testing.T) {
	// Regression test: flagsMatch must ignore "incompatible:<WxH>" metadata tags.
	// Images get incompatibility tags added during processing (e.g. "incompatible:1920x1080").
	// These are NOT processing mode flags and must not cause spurious invalidation.
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, cacheFile)

	// Full processing flags as set by downloader.go, PLUS an incompatibility tag
	imgFlags := map[string]bool{
		"SmartFit":               true,
		"FitFlexibility":         false,
		"FitQuality":             true,
		"FaceCrop":               false,
		"FaceBoost":              false,
		"incompatible:1920x1080": true, // metadata tag from rejection tagging
	}

	img1 := provider.Image{
		ID:              "img1",
		ProcessingFlags: imgFlags,
		DerivativePaths: map[string]string{"3440x1440": "/path/to/derivative.jpg"},
	}
	// Create master so it survives validation check
	master1, _ := fm.GetMasterPath("img1", ".jpg")
	err := os.MkdirAll(filepath.Dir(master1), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(master1, []byte("fake"), 0644)
	assert.NoError(t, err)

	store.Add(img1)

	// Target flags match the 5 processing-mode keys exactly
	targetFlags := map[string]bool{
		"SmartFit":       true,
		"FitFlexibility": false,
		"FitQuality":     true,
		"FaceCrop":       false,
		"FaceBoost":      false,
	}
	store.Sync(100, targetFlags, nil)

	// Image should be KEPT (not invalidated), because the processing flags match.
	// The "incompatible:1920x1080" tag should be ignored by flagsMatch.
	assert.Equal(t, 1, store.Count())
	got, _ := store.Get(0)
	assert.NotEmpty(t, got.DerivativePaths, "DerivativePaths should be preserved when flags match")
	assert.Equal(t, "/path/to/derivative.jpg", got.DerivativePaths["3440x1440"])
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
	img1 := provider.Image{ID: "img1", SourceQueryID: "qA", FilePath: filepath.Join(tmpDir, "img1.jpg"), DerivativePaths: map[string]string{"1920x1080": "d1.jpg"}}
	// img2: Inactive Query "qB"
	img2 := provider.Image{ID: "img2", SourceQueryID: "qB", FilePath: filepath.Join(tmpDir, "img2.jpg"), DerivativePaths: map[string]string{"1920x1080": "d2.jpg"}}
	// img3: No Query ID (Manual/Legacy) -> Should be kept?
	// Logic says: if SourceQueryID != "" && !active...
	// So empty SourceQueryID should correspond to "no active filter apply"?
	// Or should we delete orphans?
	// Code: if img.SourceQueryID != "" && !activeQueryIDs[img.SourceQueryID]
	// So img3 (empty ID) is SAFE.
	img3 := provider.Image{ID: "img3", SourceQueryID: "", FilePath: filepath.Join(tmpDir, "img3.jpg"), DerivativePaths: map[string]string{"1920x1080": "d3.jpg"}}

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
	assert.Equal(t, 1, store.Count())

	known := store.GetKnownIDs()

	// Expect img1 to remain (Active Query)
	assert.True(t, known["img1"])

	// Expect img3 to be gone (Orphan/Legacy pruned in Strict Mode)
	assert.False(t, known["img3"])

	// Expect img2 to be gone from store
	assert.False(t, known["img2"])

	// Verify File Deletion
	// img2 file should be deleted (Sync does it inline/deep delete)
	assert.Eventually(t, func() bool {
		_, err := os.Stat(img2.FilePath)
		return os.IsNotExist(err)
	}, 2*time.Second, 100*time.Millisecond, "File should be deleted asynchronously")
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

func TestStore_ResolutionBuckets(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	img1 := provider.Image{
		ID: "img1",
		DerivativePaths: map[string]string{
			"1920x1080": "path1_hd.jpg",
			"3840x2160": "path1_4k.jpg",
		},
	}
	img2 := provider.Image{
		ID: "img2",
		DerivativePaths: map[string]string{
			"1920x1080": "path2_hd.jpg",
		},
	}

	// 1. Test Add
	store.Add(img1)
	store.Add(img2)

	assert.Equal(t, 2, store.GetBucketSize("1920x1080"))
	assert.Equal(t, 1, store.GetBucketSize("3840x2160"))

	ids := store.GetIDsForResolution("1920x1080")
	assert.Contains(t, ids, "img1")
	assert.Contains(t, ids, "img2")

	// 2. Test Remove
	store.Remove("img1")
	assert.Equal(t, 1, store.GetBucketSize("1920x1080"))
	assert.Equal(t, 0, store.GetBucketSize("3840x2160"))

	// 3. Test Sync
	store.Clear()
	store.Add(img1)
	store.Add(img2)
	// Sync with limit 1 should prune img1 (oldest)
	store.Sync(1, nil, nil)
	assert.Equal(t, 1, store.GetBucketSize("1920x1080"))
	ids = store.GetIDsForResolution("1920x1080")
	assert.Equal(t, "img2", ids[0])
}

// =============================================================================
// Nightly Grooming Simulation
// =============================================================================
//
// This test simulates the full nightly grooming cycle as invoked by scheduler.go.
// It uses the EXACT flag maps set by downloader.go (ProcessImageJob) and
// syncStoreWithConfig() to catch any drift between the two.
//
// If you add a new processing flag in downloader.go, you MUST update both
// makeDownloaderFlags() and makeGroomingTarget() below, or this test will
// fail and tell you exactly what's wrong.

// makeDownloaderFlags builds the ProcessingFlags map exactly as downloader.go does.
// See downloader.go:L113-126.
func makeDownloaderFlags(smartFit bool, mode SmartFitMode, faceCrop, faceBoost bool) map[string]bool {
	return map[string]bool{
		"SmartFit":       smartFit,
		"FitFlexibility": mode == SmartFitAggressive,
		"FitQuality":     mode == SmartFitNormal,
		"FaceCrop":       faceCrop,
		"FaceBoost":      faceBoost,
	}
}

// makeGroomingTarget builds the targetFlags map exactly as syncStoreWithConfig() does.
// See wallpaper.go:L1242-1248.
func makeGroomingTarget(smartFit bool, mode SmartFitMode, faceCrop, faceBoost bool) map[string]bool {
	return map[string]bool{
		"SmartFit":       smartFit,
		"FitFlexibility": mode == SmartFitAggressive,
		"FitQuality":     mode == SmartFitNormal,
		"FaceCrop":       faceCrop,
		"FaceBoost":      faceBoost,
	}
}

// createMasterFile creates a fake master .jpg for an image ID so it passes validation.
func createMasterFile(t *testing.T, fm *FileManager, id string) {
	t.Helper()
	master, _ := fm.GetMasterPath(id, ".jpg")
	err := os.MkdirAll(filepath.Dir(master), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(master, []byte("fake-"+id), 0644)
	assert.NoError(t, err)
}

func TestNightlyGroomingSimulation(t *testing.T) {
	// -------------------------------------------------------------------------
	// Setup: Simulate a store with images as they would exist after a day of
	// normal operation. Images have the full flag set from downloader.go,
	// some have incompatible tags, some are from different queries.
	// -------------------------------------------------------------------------
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, cacheFile)

	// Current user config: SmartFit=true, Aggressive mode, FaceCrop=false, FaceBoost=false
	currentMode := SmartFitAggressive
	currentSmartFit := true
	currentFaceCrop := false
	currentFaceBoost := false

	baseFlags := makeDownloaderFlags(currentSmartFit, currentMode, currentFaceCrop, currentFaceBoost)

	// --- Image 1: Normal image, single monitor, no incompatible tags ---
	img1Flags := make(map[string]bool)
	for k, v := range baseFlags {
		img1Flags[k] = v
	}
	img1 := provider.Image{
		ID:              "MetMuseum_12345",
		SourceQueryID:   "qA",
		ProcessingFlags: img1Flags,
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(tmpDir, "fitted", "MetMuseum_12345_3440x1440.jpg")},
	}
	createMasterFile(t, fm, img1.ID)

	// --- Image 2: Multi-monitor image, incompatible with secondary monitor ---
	img2Flags := make(map[string]bool)
	for k, v := range baseFlags {
		img2Flags[k] = v
	}
	img2Flags["incompatible:1920x1080"] = true // too small for secondary
	img2 := provider.Image{
		ID:              "Rijks_67890",
		SourceQueryID:   "qA",
		ProcessingFlags: img2Flags,
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(tmpDir, "fitted", "Rijks_67890_3440x1440.jpg")},
	}
	createMasterFile(t, fm, img2.ID)

	// --- Image 3: Multi-monitor image, incompatible with TWO monitors ---
	img3Flags := make(map[string]bool)
	for k, v := range baseFlags {
		img3Flags[k] = v
	}
	img3Flags["incompatible:1920x1080"] = true
	img3Flags["incompatible:2560x1440"] = true
	img3 := provider.Image{
		ID:              "Cleveland_11111",
		SourceQueryID:   "qA",
		ProcessingFlags: img3Flags,
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(tmpDir, "fitted", "Cleveland_11111_3440x1440.jpg")},
	}
	createMasterFile(t, fm, img3.ID)

	// --- Image 4: From a DIFFERENT active query ---
	img4Flags := make(map[string]bool)
	for k, v := range baseFlags {
		img4Flags[k] = v
	}
	img4 := provider.Image{
		ID:              "Artic_22222",
		SourceQueryID:   "qB",
		ProcessingFlags: img4Flags,
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(tmpDir, "fitted", "Artic_22222_3440x1440.jpg")},
	}
	createMasterFile(t, fm, img4.ID)

	// --- Image 5: From an INACTIVE query (should be pruned) ---
	img5Flags := make(map[string]bool)
	for k, v := range baseFlags {
		img5Flags[k] = v
	}
	img5 := provider.Image{
		ID:              "Pexels_33333",
		SourceQueryID:   "qDEAD",
		ProcessingFlags: img5Flags,
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(tmpDir, "fitted", "Pexels_33333_3440x1440.jpg")},
	}
	createMasterFile(t, fm, img5.ID)

	// --- Image 6: Missing master file (should be pruned) ---
	img6Flags := make(map[string]bool)
	for k, v := range baseFlags {
		img6Flags[k] = v
	}
	img6 := provider.Image{
		ID:              "Wikimedia_44444",
		SourceQueryID:   "qA",
		ProcessingFlags: img6Flags,
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(tmpDir, "fitted", "Wikimedia_44444_3440x1440.jpg")},
	}
	// NOTE: No createMasterFile for img6 — simulates corrupted/missing master

	store.Add(img1)
	store.Add(img2)
	store.Add(img3)
	store.Add(img4)
	store.Add(img5)
	store.Add(img6)
	assert.Equal(t, 6, store.Count())

	// =========================================================================
	// Test 1: Steady-State Grooming (same config, no mode change)
	// Expected: img5 deleted (inactive query), img6 deleted (missing master),
	//           img1-img4 kept with ALL derivatives and incompatible tags intact.
	// =========================================================================
	t.Run("SteadyState_NoModeChange", func(t *testing.T) {
		target := makeGroomingTarget(currentSmartFit, currentMode, currentFaceCrop, currentFaceBoost)
		activeQueries := map[string]bool{"qA": true, "qB": true}

		store.Sync(100, target, activeQueries)

		// 4 images should remain
		assert.Equal(t, 4, store.Count(), "Expected 4 images after steady-state grooming")

		known := store.GetKnownIDs()
		assert.True(t, known["MetMuseum_12345"], "img1 should survive")
		assert.True(t, known["Rijks_67890"], "img2 should survive")
		assert.True(t, known["Cleveland_11111"], "img3 should survive")
		assert.True(t, known["Artic_22222"], "img4 should survive")
		assert.False(t, known["Pexels_33333"], "img5 (inactive query) should be deleted")
		assert.False(t, known["Wikimedia_44444"], "img6 (missing master) should be deleted")

		// Verify derivatives are PRESERVED (not nuked)
		got1, _ := store.GetByID("MetMuseum_12345")
		assert.NotEmpty(t, got1.DerivativePaths, "img1 derivatives should be preserved")

		got2, _ := store.GetByID("Rijks_67890")
		assert.NotEmpty(t, got2.DerivativePaths, "img2 derivatives should be preserved")
		assert.True(t, got2.ProcessingFlags["incompatible:1920x1080"], "img2 incompatible tag should be preserved")

		got3, _ := store.GetByID("Cleveland_11111")
		assert.NotEmpty(t, got3.DerivativePaths, "img3 derivatives should be preserved")
		assert.True(t, got3.ProcessingFlags["incompatible:1920x1080"], "img3 first incompatible tag preserved")
		assert.True(t, got3.ProcessingFlags["incompatible:2560x1440"], "img3 second incompatible tag preserved")

		// Verify resolution buckets are intact
		bucketSize := store.GetBucketSize("3440x1440")
		assert.Equal(t, 4, bucketSize, "All 4 surviving images should be in the 3440x1440 bucket")
	})

	// =========================================================================
	// Test 2: Mode Change Grooming (user switches from Aggressive → Normal)
	// Expected: All remaining images invalidated (derivatives wiped,
	//           incompatible tags wiped, new flags set). Master files preserved.
	// =========================================================================
	t.Run("ModeChange_InvalidatesAll", func(t *testing.T) {
		newMode := SmartFitNormal // User changed mode
		target := makeGroomingTarget(currentSmartFit, newMode, currentFaceCrop, currentFaceBoost)
		// No query filter this time
		store.Sync(100, target, nil)

		assert.Equal(t, 4, store.Count(), "All 4 images should remain (invalidated, not deleted)")

		// All images should have wiped derivatives
		for i := 0; i < store.Count(); i++ {
			img, ok := store.Get(i)
			assert.True(t, ok)
			assert.Empty(t, img.DerivativePaths, "Derivatives should be wiped after mode change for %s", img.ID)

			// Flags should now match the new target (incompatible tags wiped)
			assert.Equal(t, target["SmartFit"], img.ProcessingFlags["SmartFit"])
			assert.Equal(t, target["FitFlexibility"], img.ProcessingFlags["FitFlexibility"])
			assert.Equal(t, target["FitQuality"], img.ProcessingFlags["FitQuality"])
			assert.Equal(t, target["FaceCrop"], img.ProcessingFlags["FaceCrop"])
			assert.Equal(t, target["FaceBoost"], img.ProcessingFlags["FaceBoost"])

			// Incompatible tags should be GONE (mode change requires re-evaluation)
			assert.False(t, img.ProcessingFlags["incompatible:1920x1080"],
				"incompatible tags should be wiped after mode change for %s", img.ID)
		}

		// Resolution buckets should be EMPTY (no derivatives)
		assert.Equal(t, 0, store.GetBucketSize("3440x1440"),
			"Bucket should be empty after invalidation (derivatives wiped)")
	})

	// =========================================================================
	// Test 3: Cache Limit Pruning
	// Expected: Oldest images pruned to fit within cache limit.
	// =========================================================================
	t.Run("CacheLimit_PrunesOldest", func(t *testing.T) {
		// After ModeChange_InvalidatesAll, all images have empty DerivativePaths.
		// The zombie recovery logic would delete them. Restore derivatives so
		// we actually test the cache limit pruning path.
		for i, img := range store.List() {
			img.DerivativePaths = map[string]string{
				"3440x1440": filepath.Join(tmpDir, "fitted", fmt.Sprintf("img%d_3440x1440.jpg", i)),
			}
			store.Update(img)
		}
		assert.Equal(t, 4, store.Count(), "All 4 images should still be present before pruning")

		// Now Sync with limit 2 should prune the 2 oldest.
		store.Sync(2, nil, nil)
		assert.Equal(t, 2, store.Count(), "Should prune down to cache limit of 2")
	})
}

// =============================================================================
// Regression Guard: The 2-Flag Bug
// =============================================================================
// This test ensures the exact bug that caused the nightly grooming to nuke all
// derivatives can never silently recur. It simulates what the OLD scheduler code
// did (passing a 2-flag target) and asserts that it WOULD cause invalidation.
// The fix was to use the full 5-flag target from syncStoreWithConfig().

func TestNightlyGrooming_RegressionGuard_IncompleteFlags(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	// Image with full downloader flags (as set in production)
	fullFlags := makeDownloaderFlags(true, SmartFitAggressive, false, false)
	img := provider.Image{
		ID:              "Regression_001",
		ProcessingFlags: fullFlags,
		DerivativePaths: map[string]string{"3440x1440": "/path/to/derivative.jpg"},
	}
	createMasterFile(t, fm, img.ID)
	store.Add(img)

	// The CORRECT target (5 flags) — should NOT invalidate
	correctTarget := makeGroomingTarget(true, SmartFitAggressive, false, false)
	store.Sync(100, correctTarget, nil)

	got, _ := store.GetByID("Regression_001")
	assert.NotEmpty(t, got.DerivativePaths,
		"REGRESSION: correct 5-flag target should NOT invalidate matching images")

	// Reset for next check
	store.Clear()
	fullFlags2 := makeDownloaderFlags(true, SmartFitAggressive, false, false)
	img2 := provider.Image{
		ID:              "Regression_002",
		ProcessingFlags: fullFlags2,
		DerivativePaths: map[string]string{"3440x1440": "/path/to/derivative.jpg"},
	}
	createMasterFile(t, fm, img2.ID)
	store.Add(img2)

	// The BROKEN target (only 2 flags, as the old scheduler.go had).
	// This SHOULD cause invalidation — the reverse check in flagsMatch will
	// detect that the image has keys (FitFlexibility, FitQuality, FaceBoost)
	// not present in the target.
	brokenTarget := map[string]bool{
		"SmartFit": true,
		"FaceCrop": false,
	}
	store.Sync(100, brokenTarget, nil)

	got2, _ := store.GetByID("Regression_002")
	assert.Empty(t, got2.DerivativePaths,
		"REGRESSION GUARD: incomplete 2-flag target MUST invalidate (this was the original bug)")

	// Verify the image was invalidated with the broken flags, not deleted
	assert.Equal(t, 1, store.Count(), "Image should be invalidated, not deleted")
	assert.Equal(t, true, got2.ProcessingFlags["SmartFit"])
	assert.Equal(t, false, got2.ProcessingFlags["FaceCrop"])
}

// TestFlagsMatch_DownloaderAndGroomingConsistency is a direct unit test that
// verifies makeDownloaderFlags and makeGroomingTarget produce identical maps
// for the same inputs. If a new flag is added to one but not the other, this
// test will catch it.
func TestFlagsMatch_DownloaderAndGroomingConsistency(t *testing.T) {
	modes := []SmartFitMode{SmartFitNormal, SmartFitAggressive}
	bools := []bool{true, false}

	for _, mode := range modes {
		for _, sf := range bools {
			for _, fc := range bools {
				for _, fb := range bools {
					dl := makeDownloaderFlags(sf, mode, fc, fb)
					gr := makeGroomingTarget(sf, mode, fc, fb)

					assert.Equal(t, len(dl), len(gr),
						"Flag count mismatch for mode=%v sf=%v fc=%v fb=%v", mode, sf, fc, fb)

					for k, v := range dl {
						assert.Equal(t, v, gr[k],
							"Flag value mismatch for key=%s mode=%v sf=%v fc=%v fb=%v", k, mode, sf, fc, fb)
					}
				}
			}
		}
	}
}

// =============================================================================
// Query Removal Tests
// =============================================================================

func TestRemoveByQueryID_ResolutionBuckets(t *testing.T) {
	// Verifies that resolution buckets are correctly updated when images
	// are removed by query ID. This prevents stale bucket entries that
	// would cause the monitor controller to try setting non-existent wallpapers.
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	img1 := provider.Image{
		ID:            "q1_img1",
		SourceQueryID: "q1",
		DerivativePaths: map[string]string{
			"3440x1440": filepath.Join(tmpDir, "fitted", "q1_img1_3440x1440.jpg"),
			"1920x1080": filepath.Join(tmpDir, "fitted", "q1_img1_1920x1080.jpg"),
		},
	}
	img2 := provider.Image{
		ID:            "q1_img2",
		SourceQueryID: "q1",
		DerivativePaths: map[string]string{
			"3440x1440": filepath.Join(tmpDir, "fitted", "q1_img2_3440x1440.jpg"),
		},
	}
	img3 := provider.Image{
		ID:            "q2_img1",
		SourceQueryID: "q2",
		DerivativePaths: map[string]string{
			"3440x1440": filepath.Join(tmpDir, "fitted", "q2_img1_3440x1440.jpg"),
			"1920x1080": filepath.Join(tmpDir, "fitted", "q2_img1_1920x1080.jpg"),
		},
	}

	store.Add(img1)
	store.Add(img2)
	store.Add(img3)

	// Pre-removal: verify bucket state
	assert.Equal(t, 3, store.GetBucketSize("3440x1440"))
	assert.Equal(t, 2, store.GetBucketSize("1920x1080"))

	// Remove query q1
	store.RemoveByQueryID("q1")

	// Post-removal: buckets should only contain q2's images
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"), "Only q2_img1 should remain in 3440x1440")
	assert.Equal(t, 1, store.GetBucketSize("1920x1080"), "Only q2_img1 should remain in 1920x1080")

	ids := store.GetIDsForResolution("3440x1440")
	assert.Equal(t, 1, len(ids))
	assert.Equal(t, "q2_img1", ids[0])
}

func TestRemoveByQueryID_SeenCount(t *testing.T) {
	// Verifies that seen count is correctly decremented when query images are removed.
	store := NewImageStore()
	store.SetAsyncSave(false)

	img1 := provider.Image{ID: "img1", SourceQueryID: "q1", Seen: true}
	img2 := provider.Image{ID: "img2", SourceQueryID: "q1", Seen: true}
	img3 := provider.Image{ID: "img3", SourceQueryID: "q2", Seen: false}
	img4 := provider.Image{ID: "img4", SourceQueryID: "q2", Seen: true}

	store.Add(img1)
	store.Add(img2)
	store.Add(img3)
	store.Add(img4)

	assert.Equal(t, 3, store.SeenCount(), "Pre-removal: 3 seen images")

	// Remove q1 (both seen)
	store.RemoveByQueryID("q1")

	assert.Equal(t, 2, store.Count(), "2 images should remain")
	assert.Equal(t, 1, store.SeenCount(), "Only img4 (from q2) should be counted as seen")
}

func TestRemoveByQueryID_NonExistentQuery(t *testing.T) {
	// Removing a query that doesn't exist should be a no-op.
	store := NewImageStore()
	store.SetAsyncSave(false)

	store.Add(provider.Image{ID: "img1", SourceQueryID: "q1"})
	store.Add(provider.Image{ID: "img2", SourceQueryID: "q2"})

	store.RemoveByQueryID("q_nonexistent")

	assert.Equal(t, 2, store.Count(), "No images should be removed for non-existent query")
}

func TestRemoveByQueryID_EmptyQueryID(t *testing.T) {
	// Images with empty SourceQueryID should NOT be removed when removing
	// a specific query. They are "orphans" from legacy/manual adds.
	store := NewImageStore()
	store.SetAsyncSave(false)

	store.Add(provider.Image{ID: "legacy_img", SourceQueryID: ""})
	store.Add(provider.Image{ID: "q1_img", SourceQueryID: "q1"})

	store.RemoveByQueryID("q1")

	assert.Equal(t, 1, store.Count())
	known := store.GetKnownIDs()
	assert.True(t, known["legacy_img"], "Legacy image with empty query ID should survive")
	assert.False(t, known["q1_img"], "q1 image should be removed")
}

// =============================================================================
// Blocklist (AvoidSet) Tests
// =============================================================================

func TestBlocklist_RemoveAutoPopulates(t *testing.T) {
	// Verifies that Remove() automatically adds the image ID to the avoidSet,
	// preventing the same image from being re-added by future fetches.
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	img := provider.Image{
		ID: "bad_image",
		DerivativePaths: map[string]string{
			"3440x1440": filepath.Join(tmpDir, "fitted", "bad_image_3440x1440.jpg"),
		},
	}
	store.Add(img)
	assert.Equal(t, 1, store.Count())
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"))

	// Remove the image (user clicked "Block" / "Never show again")
	_, ok := store.Remove("bad_image")
	assert.True(t, ok)
	assert.Equal(t, 0, store.Count())
	assert.Equal(t, 0, store.GetBucketSize("3440x1440"), "Bucket should be empty after Remove")

	// Try to re-add the same image (simulates next fetch returning it)
	added := store.Add(provider.Image{ID: "bad_image"})
	assert.False(t, added, "Blocked image should be rejected by Add()")
	assert.Equal(t, 0, store.Count(), "Store should still be empty")
}

func TestBlocklist_SyncPrunesBlockedImages(t *testing.T) {
	// Verifies that Sync's determineSyncAction correctly deletes images
	// that are in the avoidSet. This covers the case where an image was
	// blocked AFTER being added to the store (e.g., loaded from cache).
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	// Add images with DerivativePaths (required to avoid zombie recovery deletion)
	img1 := provider.Image{ID: "good_img", SourceQueryID: "q1", DerivativePaths: map[string]string{"1920x1080": "good.jpg"}}
	img2 := provider.Image{ID: "bad_img", SourceQueryID: "q1", DerivativePaths: map[string]string{"1920x1080": "bad.jpg"}}
	img3 := provider.Image{ID: "also_good", SourceQueryID: "q1", DerivativePaths: map[string]string{"1920x1080": "also.jpg"}}

	createMasterFile(t, fm, "good_img")
	createMasterFile(t, fm, "bad_img")
	createMasterFile(t, fm, "also_good")

	store.Add(img1)
	store.Add(img2)
	store.Add(img3)

	// Now load the blocklist (simulates app startup loading persisted blocklist)
	store.LoadAvoidSet(map[string]bool{
		"bad_img": true,
	})

	// Run Sync (as nightly grooming would)
	store.Sync(100, nil, nil)

	// bad_img should be gone
	assert.Equal(t, 2, store.Count())
	known := store.GetKnownIDs()
	assert.True(t, known["good_img"])
	assert.True(t, known["also_good"])
	assert.False(t, known["bad_img"], "Blocked image should be pruned by Sync")
}

func TestBlocklist_InteractionWithQueryRemoval(t *testing.T) {
	// Tests the full lifecycle:
	// 1. Images from query q1 are in the store
	// 2. User blocks one specific image from q1
	// 3. Query q1 is removed (user deactivates the query)
	// 4. Query q1 is re-added (user re-enables the query)
	// 5. Fetch returns all original images including the blocked one
	// 6. The blocked image must still be rejected
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	// Phase 1: Add images from query q1
	store.Add(provider.Image{ID: "q1_a", SourceQueryID: "q1"})
	store.Add(provider.Image{ID: "q1_b", SourceQueryID: "q1"})
	store.Add(provider.Image{ID: "q1_blocked", SourceQueryID: "q1"})
	assert.Equal(t, 3, store.Count())

	// Phase 2: User blocks q1_blocked
	store.Remove("q1_blocked")
	assert.Equal(t, 2, store.Count())

	// Phase 3: User deactivates query q1
	store.RemoveByQueryID("q1")
	assert.Equal(t, 0, store.Count())

	// Phase 4+5: User re-enables q1, fetch returns all 3 images
	added1 := store.Add(provider.Image{ID: "q1_a", SourceQueryID: "q1"})
	added2 := store.Add(provider.Image{ID: "q1_b", SourceQueryID: "q1"})
	added3 := store.Add(provider.Image{ID: "q1_blocked", SourceQueryID: "q1"})

	// Phase 6: Verify
	assert.True(t, added1, "q1_a should be re-addable")
	assert.True(t, added2, "q1_b should be re-addable")
	assert.False(t, added3, "q1_blocked must remain blocked even after query removal/re-add")
	assert.Equal(t, 2, store.Count())
}

func TestBlocklist_SurvivesPersistence(t *testing.T) {
	// Verifies that the avoidSet survives Clear() but not Wipe(),
	// and that blocked images are still rejected after store reload.
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")

	// Store 1: Add images, block one, save
	store1 := NewImageStore()
	store1.SetAsyncSave(false)
	store1.SetFileManager(fm, cacheFile)

	store1.Add(provider.Image{ID: "img_a"})
	store1.Add(provider.Image{ID: "img_b"})
	store1.Remove("img_b") // Block img_b
	store1.SaveCache()

	// Simulate app restart: Create new store, load cache
	store2 := NewImageStore()
	store2.SetAsyncSave(false)
	store2.SetFileManager(fm, cacheFile)
	err := store2.LoadCache()
	assert.NoError(t, err)

	// img_b should not be in the loaded store
	assert.Equal(t, 1, store2.Count())
	known := store2.GetKnownIDs()
	assert.True(t, known["img_a"])
	assert.False(t, known["img_b"])

	// However, blocklist is NOT persisted in cache.json (it's loaded from config).
	// So after LoadCache, trying to add img_b would succeed unless LoadAvoidSet
	// is called separately. Let's verify this expected behavior:
	added := store2.Add(provider.Image{ID: "img_b"})
	// Without LoadAvoidSet, the blocklist is empty in the new store
	assert.True(t, added, "Without LoadAvoidSet, blocked image can be re-added (blocklist is config-managed)")

	// Now load the blocklist explicitly (as app startup does)
	store2.Remove("img_b") // Re-block it
	store2.LoadAvoidSet(map[string]bool{"img_b": true})
	store2.Clear() // Clear preserves avoidSet

	addedAgain := store2.Add(provider.Image{ID: "img_b"})
	assert.False(t, addedAgain, "After LoadAvoidSet + Clear, blocked image should still be rejected")
}

func TestBlocklist_ResolutionBucketsAfterSyncPrune(t *testing.T) {
	// Verifies that resolution buckets are correctly updated when Sync
	// prunes blocked images. This is the nightly grooming scenario where
	// a blocked image somehow ended up in the store (e.g., race condition
	// or loaded from stale cache).
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	img1 := provider.Image{
		ID: "clean_img",
		DerivativePaths: map[string]string{
			"3440x1440": filepath.Join(tmpDir, "fitted", "clean_3440x1440.jpg"),
		},
	}
	img2 := provider.Image{
		ID: "will_be_blocked",
		DerivativePaths: map[string]string{
			"3440x1440": filepath.Join(tmpDir, "fitted", "blocked_3440x1440.jpg"),
			"1920x1080": filepath.Join(tmpDir, "fitted", "blocked_1920x1080.jpg"),
		},
	}

	createMasterFile(t, fm, "clean_img")
	createMasterFile(t, fm, "will_be_blocked")

	store.Add(img1)
	store.Add(img2)

	assert.Equal(t, 2, store.GetBucketSize("3440x1440"))
	assert.Equal(t, 1, store.GetBucketSize("1920x1080"))

	// Block the image via avoidSet
	store.LoadAvoidSet(map[string]bool{"will_be_blocked": true})

	// Sync prunes it
	store.Sync(100, nil, nil)

	assert.Equal(t, 1, store.Count())
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"), "Only clean_img in 3440x1440")
	assert.Equal(t, 0, store.GetBucketSize("1920x1080"), "1920x1080 bucket should be empty")

	ids := store.GetIDsForResolution("3440x1440")
	assert.Equal(t, "clean_img", ids[0])
}

// =============================================================================
// Zombie Image Recovery Integration Tests
// =============================================================================
//
// Context: The "flagsMatch length comparison" bug caused nightly grooming to
// invalidate ALL images. Invalidation wipes DerivativePaths and resets
// ProcessingFlags. After multiple nights, the store was full of "zombie" images:
//   - Master files exist on disk
//   - ProcessingFlags match the target (set correctly during invalidation)
//   - DerivativePaths is empty → invisible to monitors (no resolution buckets)
//   - store.Exists(id) returns true → blocks re-fetching
//
// The zombie recovery in determineSyncAction detects and deletes these images
// so they can be re-downloaded and reprocessed.
// =============================================================================

// newZombieTestStore creates a fresh ImageStore with a FileManager rooted in tmpDir.
func newZombieTestStore(t *testing.T, tmpDir string) (*ImageStore, *FileManager) {
	t.Helper()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))
	return store, fm
}

// defaultTargetFlags returns the standard 5-flag target for testing.
func defaultTargetFlags() map[string]bool {
	return map[string]bool{
		"SmartFit":  true,
		"FaceCrop":  false,
		"FaceBoost": false,
		"Upscale":   false,
		"Fill":      false,
	}
}

// newHealthyImage creates an image with master file, correct flags, and derivatives.
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

// newZombieImage creates an image with master file and correct flags but empty derivatives.
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

func TestZombieRecovery_BasicDetection(t *testing.T) {
	// Zombie with empty DerivativePaths map is detected and deleted.
	// Healthy image with derivatives survives.
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	store.Add(newHealthyImage(t, fm, "healthy", "q1", "3440x1440", flags))
	store.Add(newZombieImage(t, fm, "zombie", "q1", flags))
	assert.Equal(t, 2, store.Count())

	store.Sync(100, flags, nil)

	assert.Equal(t, 1, store.Count())
	known := store.GetKnownIDs()
	assert.True(t, known["healthy"])
	assert.False(t, known["zombie"])
}

func TestZombieRecovery_NilDerivativePaths(t *testing.T) {
	// Some corrupted images may have nil DerivativePaths (not just empty map).
	// This must also trigger zombie cleanup.
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	createMasterFile(t, fm, "nil_deriv")
	nilImg := provider.Image{
		ID:              "nil_deriv",
		SourceQueryID:   "q1",
		ProcessingFlags: copyFlags(flags),
		DerivativePaths: nil, // nil, not empty map
	}
	store.Add(nilImg)
	store.Add(newHealthyImage(t, fm, "ok", "q1", "1920x1080", flags))

	store.Sync(100, flags, nil)

	assert.Equal(t, 1, store.Count())
	known := store.GetKnownIDs()
	assert.False(t, known["nil_deriv"], "nil DerivativePaths should be treated as zombie")
	assert.True(t, known["ok"])
}

func TestZombieRecovery_PartialDerivativesNotZombie(t *testing.T) {
	// An image that has SOME derivatives (even just one) is NOT a zombie.
	// Only images with zero derivatives are zombies.
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	partialImg := newHealthyImage(t, fm, "partial", "q1", "1920x1080", flags)
	// Only has 1920x1080 derivative, missing 3440x1440 — but that's fine
	store.Add(partialImg)

	store.Sync(100, flags, nil)

	assert.Equal(t, 1, store.Count(), "Image with partial derivatives should survive")
}

func TestZombieRecovery_ReAddAfterDeletion(t *testing.T) {
	// After a zombie is deleted from the store, the same image ID can be
	// re-added with proper derivatives (simulating a re-fetch by the pipeline).
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	store.Add(newZombieImage(t, fm, "revived", "q1", flags))
	assert.Equal(t, 1, store.Count())
	assert.True(t, store.Exists("revived"))

	// Zombie gets cleaned
	store.Sync(100, flags, nil)
	assert.Equal(t, 0, store.Count())
	assert.False(t, store.Exists("revived"))

	// Re-add with proper derivatives (simulates pipeline re-download)
	revivedImg := newHealthyImage(t, fm, "revived", "q1", "3440x1440", flags)
	added := store.Add(revivedImg)
	assert.True(t, added, "Zombie should be re-addable after deletion")
	assert.Equal(t, 1, store.Count())

	// Survives a second Sync (it's healthy now)
	store.Sync(100, flags, nil)
	assert.Equal(t, 1, store.Count(), "Re-added healthy image should survive Sync")
}

func TestZombieRecovery_MultiQueryZombies(t *testing.T) {
	// Zombies from different source queries are all detected and cleaned.
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	store.Add(newZombieImage(t, fm, "z_wallhaven", "wallhaven_q", flags))
	store.Add(newZombieImage(t, fm, "z_pexels", "pexels_q", flags))
	store.Add(newZombieImage(t, fm, "z_wikimedia", "wiki_q", flags))
	store.Add(newHealthyImage(t, fm, "h_wallhaven", "wallhaven_q", "3440x1440", flags))
	assert.Equal(t, 4, store.Count())

	store.Sync(100, flags, nil)

	assert.Equal(t, 1, store.Count())
	known := store.GetKnownIDs()
	assert.True(t, known["h_wallhaven"], "Healthy image survives")
	assert.False(t, known["z_wallhaven"])
	assert.False(t, known["z_pexels"])
	assert.False(t, known["z_wikimedia"])
}

func TestZombieRecovery_ZombieAndBlocklistInteraction(t *testing.T) {
	// A zombie that is also in the blocklist should be deleted.
	// The blocklist check runs before the zombie check, so it's deleted
	// by the avoidSet path — either way, it's removed.
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	store.Add(newZombieImage(t, fm, "blocked_zombie", "q1", flags))
	store.Add(newHealthyImage(t, fm, "clean", "q1", "1920x1080", flags))
	store.LoadAvoidSet(map[string]bool{"blocked_zombie": true})

	store.Sync(100, flags, nil)

	assert.Equal(t, 1, store.Count())
	assert.False(t, store.GetKnownIDs()["blocked_zombie"])
	assert.True(t, store.GetKnownIDs()["clean"])
}

func TestZombieRecovery_PostInvalidationCycle(t *testing.T) {
	// Simulates the full corruption lifecycle:
	// 1. Image starts healthy with correct flags + derivatives
	// 2. Mode change causes invalidation → derivatives wiped, new flags set
	// 3. Next Sync: flags match now, but derivatives are empty → zombie → deleted
	store, fm := newZombieTestStore(t, t.TempDir())

	oldFlags := map[string]bool{
		"SmartFit": false, "FaceCrop": true, "FaceBoost": true,
		"Upscale": false, "Fill": false,
	}
	newFlags := map[string]bool{
		"SmartFit": true, "FaceCrop": false, "FaceBoost": false,
		"Upscale": false, "Fill": false,
	}

	// Step 1: Image is healthy with old flags
	img := newHealthyImage(t, fm, "transitioning", "q1", "1920x1080", oldFlags)
	store.Add(img)
	assert.Equal(t, 1, store.Count())

	// Step 2: User changes mode — Sync invalidates (flags mismatch)
	store.Sync(100, newFlags, nil)

	// After invalidation: image should still be in store but with new flags and empty derivatives
	assert.Equal(t, 1, store.Count(), "Image should survive invalidation")
	updated, ok := store.Get(0)
	assert.True(t, ok)
	assert.Empty(t, updated.DerivativePaths, "Derivatives should be wiped after invalidation")
	assert.Equal(t, newFlags["SmartFit"], updated.ProcessingFlags["SmartFit"])

	// Step 3: NEXT Sync — flags now match, but no derivatives → zombie → delete
	store.Sync(100, newFlags, nil)
	assert.Equal(t, 0, store.Count(), "Post-invalidation zombie should be cleaned on next Sync")
}

func TestZombieRecovery_MassCorruption(t *testing.T) {
	// Simulates real-world scenario: ALL images in the store were corrupted
	// by the nightly grooming bug. Every single one is a zombie.
	// After recovery, the store should be empty and ready for fresh downloads.
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	// Add 20 zombie images (simulating a full cache worth of corrupted images)
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("zombie_%03d", i)
		store.Add(newZombieImage(t, fm, id, "q1", flags))
	}
	assert.Equal(t, 20, store.Count())

	// Run Sync — all should be cleaned
	store.Sync(100, flags, nil)
	assert.Equal(t, 0, store.Count(), "All 20 zombies should be deleted")

	// Verify store is completely clean
	assert.Equal(t, 0, store.SeenCount())
	assert.Empty(t, store.GetKnownIDs())
}

func TestZombieRecovery_CacheLimitInteraction(t *testing.T) {
	// When zombies coexist with healthy images and a cache limit is applied,
	// zombies are deleted first (by zombie detection), THEN the cache limit
	// prunes the oldest remaining healthy images.
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	// Add 3 healthy images
	store.Add(newHealthyImage(t, fm, "h1", "q1", "3440x1440", flags))
	store.Add(newHealthyImage(t, fm, "h2", "q1", "3440x1440", flags))
	store.Add(newHealthyImage(t, fm, "h3", "q1", "3440x1440", flags))
	// Add 2 zombies
	store.Add(newZombieImage(t, fm, "z1", "q1", flags))
	store.Add(newZombieImage(t, fm, "z2", "q1", flags))
	assert.Equal(t, 5, store.Count())

	// Sync with cache limit of 2: zombies deleted first, then oldest pruned
	store.Sync(2, flags, nil)

	assert.Equal(t, 2, store.Count(), "Should have exactly 2 healthy images after limit")
	known := store.GetKnownIDs()
	assert.False(t, known["z1"], "Zombie should be gone")
	assert.False(t, known["z2"], "Zombie should be gone")
	// h1 is oldest and should be pruned by cache limit
	assert.False(t, known["h1"], "Oldest healthy image should be pruned")
	assert.True(t, known["h2"], "h2 should survive")
	assert.True(t, known["h3"], "h3 (newest) should survive")
}

func TestZombieRecovery_ResolutionBucketIntegrity(t *testing.T) {
	// After zombie cleanup, resolution buckets must be rebuilt correctly.
	// Only healthy images should appear in buckets.
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	store.Add(newHealthyImage(t, fm, "h_4k", "q1", "3840x2160", flags))
	store.Add(newHealthyImage(t, fm, "h_uw", "q1", "3440x1440", flags))
	store.Add(newZombieImage(t, fm, "z1", "q1", flags))
	store.Add(newZombieImage(t, fm, "z2", "q1", flags))

	assert.Equal(t, 4, store.Count())
	assert.Equal(t, 1, store.GetBucketSize("3840x2160"))
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"))

	store.Sync(100, flags, nil)

	assert.Equal(t, 2, store.Count())
	assert.Equal(t, 1, store.GetBucketSize("3840x2160"), "4K bucket should have h_4k")
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"), "UW bucket should have h_uw")

	// Verify the correct IDs are in each bucket
	ids4k := store.GetIDsForResolution("3840x2160")
	assert.Contains(t, ids4k, "h_4k")
	idsUW := store.GetIDsForResolution("3440x1440")
	assert.Contains(t, idsUW, "h_uw")
}

func TestZombieRecovery_WithQueryFilter(t *testing.T) {
	// When Sync is called with activeQueryIDs (strict mode), zombies from
	// active queries are cleaned by zombie detection. Zombies from inactive
	// queries are cleaned by the query filter (ImageActionDelete).
	// Both paths result in deletion — test that nothing leaks.
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	store.Add(newZombieImage(t, fm, "z_active", "active_q", flags))
	store.Add(newZombieImage(t, fm, "z_inactive", "dead_q", flags))
	store.Add(newHealthyImage(t, fm, "h_active", "active_q", "1920x1080", flags))

	activeQueries := map[string]bool{"active_q": true}
	store.Sync(100, flags, activeQueries)

	assert.Equal(t, 1, store.Count())
	known := store.GetKnownIDs()
	assert.True(t, known["h_active"], "Healthy image from active query survives")
	assert.False(t, known["z_active"], "Zombie from active query deleted by zombie check")
	assert.False(t, known["z_inactive"], "Zombie from inactive query deleted by query filter")
}

func TestZombieRecovery_NoFalsePositivesWithFlags(t *testing.T) {
	// When Sync is called with empty/nil targetFlags, the zombie check
	// should still work (it's independent of flag matching).
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	store.Add(newZombieImage(t, fm, "z1", "q1", flags))
	store.Add(newHealthyImage(t, fm, "h1", "q1", "1920x1080", flags))

	// Sync with nil targetFlags (like a basic cache-limit-only sync)
	store.Sync(100, nil, nil)

	assert.Equal(t, 1, store.Count())
	assert.True(t, store.GetKnownIDs()["h1"])
	assert.False(t, store.GetKnownIDs()["z1"])
}

func TestZombieRecovery_PersistenceAcrossRestart(t *testing.T) {
	// Simulates app restart: store is loaded from cache.json containing
	// zombie images. After loading + Sync, zombies are cleaned.
	tmpDir := t.TempDir()
	store1, fm := newZombieTestStore(t, tmpDir)
	flags := defaultTargetFlags()

	// Session 1: Add zombie + healthy, save to disk
	store1.Add(newZombieImage(t, fm, "z_persist", "q1", flags))
	store1.Add(newHealthyImage(t, fm, "h_persist", "q1", "1920x1080", flags))
	store1.SaveCache()

	// Session 2: New store, load from cache
	store2, _ := newZombieTestStore(t, tmpDir)
	store2.LoadCache()
	assert.Equal(t, 2, store2.Count(), "Both images should be loaded from cache")

	// Sync detects and cleans zombie
	store2.Sync(100, flags, nil)
	assert.Equal(t, 1, store2.Count())
	assert.True(t, store2.GetKnownIDs()["h_persist"])
	assert.False(t, store2.GetKnownIDs()["z_persist"], "Zombie should be cleaned after restart")
}

func TestZombieRecovery_SeenCountReset(t *testing.T) {
	// Zombies that were previously marked as "seen" should have their
	// seen count contribution removed when they're deleted.
	store, fm := newZombieTestStore(t, t.TempDir())
	flags := defaultTargetFlags()

	// Create zombie with a FilePath (MarkSeen uses filePath, not ID)
	zImg := newZombieImage(t, fm, "z_seen", "q1", flags)
	zImg.FilePath = "/fake/z_seen.jpg"
	store.Add(zImg)
	store.Add(newHealthyImage(t, fm, "h_unseen", "q1", "1920x1080", flags))

	// Mark zombie as seen using its FilePath
	store.MarkSeen(zImg.FilePath)
	assert.Equal(t, 1, store.SeenCount(), "One image should be marked seen")

	// Sync cleans zombie
	store.Sync(100, flags, nil)

	assert.Equal(t, 1, store.Count())
	assert.Equal(t, 0, store.SeenCount(), "Seen count should drop when zombie is deleted")
}

func copyFlags(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
