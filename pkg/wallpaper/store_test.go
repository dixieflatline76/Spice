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
		store.replace(provider.Image{ID: "img1"})
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
	store.replace(provider.Image{ID: "img1"})

	// Wait for save
	<-saveChan

	// Wait > debounce
	time.Sleep(100 * time.Millisecond)
	store.replace(provider.Image{ID: "img1"})

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
			store.replace(img)
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
// Regression: Pipeline Add-or-Update Fallback
// =============================================================================
// Verifies that the Add-or-Update pattern in stateManagerLoop correctly handles
// backlog healing: when an image already exists in the store but is missing
// derivatives, the pipeline re-processes it and the fully-populated result
// must land in the store via Update() after Add() is rejected.

func TestPipeline_AddFallbackToUpdate(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Step 1: Image in store with partial data (no derivatives — zombie state)
	partial := provider.Image{
		ID:              "img1",
		Width:           4000,
		Height:          3000,
		ProcessingFlags: map[string]bool{"SmartFit": true},
		// No DerivativePaths — this is the corrupted state
	}
	store.Add(partial)
	assert.Equal(t, 0, store.GetBucketSize("3440x1440"), "No derivatives → no bucket entry")

	// Step 2: Pipeline produces the fully-processed result
	fullResult := provider.Image{
		ID:       "img1",
		Width:    4000,
		Height:   3000,
		FilePath: "/path/to/file.jpg",
		DerivativePaths: map[string]string{
			"3440x1440": "/path/to/fitted/img1.jpg",
		},
		ProcessingFlags: map[string]bool{"SmartFit": true, "FaceCrop": true},
	}

	// Step 3: Simulate the fixed stateManagerLoop pattern
	if !store.Add(fullResult) {
		store.replace(fullResult)
	}

	// Step 4: Verify the fully-processed result landed
	got, ok := store.GetByID("img1")
	assert.True(t, ok)
	assert.NotEmpty(t, got.DerivativePaths, "Fully-processed DerivativePaths must be in store")
	assert.Equal(t, "/path/to/fitted/img1.jpg", got.DerivativePaths["3440x1440"])
	assert.Equal(t, "/path/to/file.jpg", got.FilePath)
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"), "Bucket should now have the image")
}

// =============================================================================
// Regression: Nightly Cycle Does Not Clobber
// =============================================================================
// Full lifecycle test simulating the exact scenario that caused the production bug:
// 1. Store has healthy images with derivatives
// 2. Nightly grooming runs (syncStoreWithConfig → store.Sync)
// 3. Pipeline re-processes images (backlog healing)
// 4. Verify derivatives survive the entire cycle

func TestNightlyCycle_ProcessImageJobDoesNotClobber(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	flags := makeDownloaderFlags(true, SmartFitAggressive, true, false)
	target := makeGroomingTarget(true, SmartFitAggressive, true, false)

	// Step 1: Store has 5 healthy images
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("img_%d", i)
		img := newHealthyImage(t, fm, id, "q1", "3440x1440", flags)
		img.Width = 4000
		img.Height = 3000
		img.FilePath = filepath.Join(tmpDir, id+".jpg")
		store.Add(img)
	}
	assert.Equal(t, 5, store.GetBucketSize("3440x1440"))

	// Step 2: Grooming pass
	activeQueries := map[string]bool{"q1": true}
	store.Sync(100, target, activeQueries)
	assert.Equal(t, 5, store.Count())

	// Step 3: Simulate pipeline re-processing
	// With the intermediate updates removed, ProcessImageJob is purely functional.
	// The stateManagerLoop uses Add-or-Update to persist the final result.
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("img_%d", i)

		fullResult := provider.Image{
			ID:            id,
			SourceQueryID: "q1",
			Width:         5000, // Probed width updated
			Height:        3000,
			FilePath:      filepath.Join(tmpDir, id+".jpg"),
			DerivativePaths: map[string]string{
				"3440x1440": filepath.Join(tmpDir, "fitted", id+"_3440x1440.jpg"),
			},
			ProcessingFlags: copyFlags(flags),
		}
		fullResult.ProcessingFlags["incompatible:1920x1080"] = true // Tagged

		// Pipeline stateManagerLoop Add-or-Update
		if !store.Add(fullResult) {
			store.replace(fullResult)
		}
	}

	// Step 4: Verify everything survived and was updated
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("img_%d", i)
		got, _ := store.GetByID(id)
		assert.NotEmpty(t, got.DerivativePaths, "%s should have derivatives", id)
		assert.Equal(t, 5000, got.Width, "%s should have updated width", id)
		assert.True(t, got.ProcessingFlags["incompatible:1920x1080"])
	}

	assert.Equal(t, 5, store.GetBucketSize("3440x1440"), "All images should still be in the bucket")
}

func copyFlags(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// === Issue 1: Purpose-Built Method Tests ===

func TestSetFavorited_OnlyChangesFlag(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	img := provider.Image{
		ID:              "test1",
		Width:           4000,
		Height:          3000,
		DerivativePaths: map[string]string{"3440x1440": "/path/to/deriv.jpg"},
		ProcessingFlags: map[string]bool{"SmartFit": true},
		IsFavorited:     false,
	}
	store.Add(img)

	// Set favorited
	ok := store.SetFavorited("test1", true)
	assert.True(t, ok)

	got, found := store.GetByID("test1")
	assert.True(t, found)
	assert.True(t, got.IsFavorited, "IsFavorited should be true")

	// All other fields must be untouched
	assert.Equal(t, 4000, got.Width, "Width should be untouched")
	assert.Equal(t, 3000, got.Height, "Height should be untouched")
	assert.Equal(t, "/path/to/deriv.jpg", got.DerivativePaths["3440x1440"], "DerivativePaths should be untouched")
	assert.True(t, got.ProcessingFlags["SmartFit"], "ProcessingFlags should be untouched")

	// Unfavorite
	store.SetFavorited("test1", false)
	got, _ = store.GetByID("test1")
	assert.False(t, got.IsFavorited, "IsFavorited should be false")
}

func TestSetFavorited_NotFound(t *testing.T) {
	store := NewImageStore()
	ok := store.SetFavorited("nonexistent", true)
	assert.False(t, ok)
}

func TestClearDerivatives_RemovesFromBuckets(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	img := provider.Image{
		ID:              "test1",
		DerivativePaths: map[string]string{"3440x1440": "/path/to/deriv.jpg"},
		ProcessingFlags: map[string]bool{"SmartFit": true},
	}
	store.Add(img)

	// Verify image is in the resolution bucket
	ids := store.GetIDsForResolution("3440x1440")
	assert.Contains(t, ids, "test1")

	// Clear derivatives
	ok := store.ClearDerivatives("test1")
	assert.True(t, ok)

	// Verify image is removed from the bucket
	ids = store.GetIDsForResolution("3440x1440")
	assert.NotContains(t, ids, "test1", "Image should be removed from bucket after ClearDerivatives")

	// Verify the image still exists but has empty maps
	got, found := store.GetByID("test1")
	assert.True(t, found, "Image should still be in the store")
	assert.Empty(t, got.DerivativePaths, "DerivativePaths should be empty")
	assert.Empty(t, got.ProcessingFlags, "ProcessingFlags should be empty")
}

func TestClearDerivatives_NotFound(t *testing.T) {
	store := NewImageStore()
	ok := store.ClearDerivatives("nonexistent")
	assert.False(t, ok)
}

func TestReplace_StillWorks(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Add an initial image (like from pipeline first Add)
	initial := provider.Image{
		ID:    "test1",
		Width: 4000,
	}
	store.Add(initial)

	// Replace with fully-processed result (like stateManagerLoop does)
	full := provider.Image{
		ID:              "test1",
		Width:           4000,
		Height:          3000,
		DerivativePaths: map[string]string{"3440x1440": "/path/to/deriv.jpg"},
		ProcessingFlags: map[string]bool{"SmartFit": true},
	}
	ok := store.replace(full)
	assert.True(t, ok)

	got, found := store.GetByID("test1")
	assert.True(t, found)
	assert.Equal(t, 3000, got.Height, "Height should be updated by replace")
	assert.Equal(t, "/path/to/deriv.jpg", got.DerivativePaths["3440x1440"], "DerivativePaths should be set by replace")

	// Verify bucket is updated
	ids := store.GetIDsForResolution("3440x1440")
	assert.Contains(t, ids, "test1")
}

// =============================================================================
// Favorites Upsert Tests (Issue 2)
// =============================================================================

// TestFavoritesUpsert_PreservesStoreMetadata verifies that when Add() receives
// a FavoritesQueryID image that already exists, it selectively updates only
// provider-authoritative fields and preserves store-managed metadata.
func TestFavoritesUpsert_PreservesStoreMetadata(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Existing image with full store-managed metadata (already processed by pipeline)
	existing := provider.Image{
		ID:              "Wallhaven_4yd58l",
		Provider:        "Wallhaven",
		Path:            "https://wallhaven.cc/4yd58l.jpg",
		Attribution:     "nexed",
		ViewURL:         "https://whvn.cc/4yd58l",
		IsFavorited:     true,
		SourceQueryID:   "wallhaven_query_1",
		FilePath:        "/downloads/Wallhaven_4yd58l.jpg",
		DerivativePaths: map[string]string{"3440x1440": "/fitted/wh.jpg"},
		ProcessingFlags: map[string]bool{"SmartFit": true, "FaceCrop": true},
		Tuning:          map[string]provider.TuningOptions{"3440x1440": {Anchor: provider.AnchorTopCenter}},
		Width:           3840,
		Height:          2160,
		Seen:            true,
	}
	store.Add(existing)

	// Favorites provider fetches the same image — has only provider-level fields
	incoming := provider.Image{
		ID:            "Wallhaven_4yd58l",
		Provider:      "Favorites",
		Path:          "http://127.0.0.1:49452/local/favorites/favorite_images/assets/Wallhaven_4yd58l.jpg",
		Attribution:   "nexed",
		ViewURL:       "https://whvn.cc/4yd58l",
		IsFavorited:   true,
		SourceQueryID: FavoritesQueryID,
		// No DerivativePaths, ProcessingFlags, CropAnchors, Width, Height, FilePath
	}
	result := store.Add(incoming)
	assert.True(t, result, "Upsert should return true")

	got, ok := store.GetByID("Wallhaven_4yd58l")
	assert.True(t, ok)

	// Provider-authoritative fields should be updated
	assert.Equal(t, "Favorites", got.Provider, "Provider should be updated to Favorites")
	assert.Equal(t, FavoritesQueryID, got.SourceQueryID, "SourceQueryID should be updated")
	assert.True(t, got.IsFavorited, "IsFavorited should be true")
	assert.Equal(t, incoming.Path, got.Path, "Path should be updated to local API URL")

	// Store-managed fields should be PRESERVED
	assert.Equal(t, map[string]string{"3440x1440": "/fitted/wh.jpg"}, got.DerivativePaths, "DerivativePaths must be preserved")
	assert.Equal(t, map[string]bool{"SmartFit": true, "FaceCrop": true}, got.ProcessingFlags, "ProcessingFlags must be preserved")
	assert.Equal(t, provider.AnchorTopCenter, got.Tuning["3440x1440"].Anchor, "Tuning must be preserved")
	assert.Equal(t, 3840, got.Width, "Width must be preserved")
	assert.Equal(t, 2160, got.Height, "Height must be preserved")
	assert.Equal(t, "/downloads/Wallhaven_4yd58l.jpg", got.FilePath, "FilePath must be preserved when incoming is empty")
	assert.True(t, got.Seen, "Seen must be preserved")
}

// TestFavoritesUpsert_NonEmptyWins_Attribution verifies the race condition
// protection: when the Favorites provider sends an image with empty Attribution
// (because metadata.json hasn't been written yet), the existing Attribution is
// preserved. When it sends a non-empty Attribution, it overwrites.
func TestFavoritesUpsert_NonEmptyWins_Attribution(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Existing image with attribution
	existing := provider.Image{
		ID:            "MetMuseum_436528",
		Provider:      "MetMuseum",
		Attribution:   "Vincent van Gogh - Irises",
		ViewURL:       "https://www.metmuseum.org/art/collection/search/436528",
		SourceQueryID: "met_query_1",
	}
	store.Add(existing)

	// Race scenario: Favorites fetch arrives with EMPTY attribution
	raceImg := provider.Image{
		ID:            "MetMuseum_436528",
		Provider:      "Favorites",
		Attribution:   "", // Empty due to race with metadata.json write
		ViewURL:       "", // Also empty
		SourceQueryID: FavoritesQueryID,
		IsFavorited:   true,
	}
	store.Add(raceImg)

	got, _ := store.GetByID("MetMuseum_436528")
	assert.Equal(t, "Vincent van Gogh - Irises", got.Attribution, "Attribution must be preserved when incoming is empty")
	assert.Equal(t, "https://www.metmuseum.org/art/collection/search/436528", got.ViewURL, "ViewURL must be preserved when incoming is empty")

	// Normal scenario: Favorites fetch arrives WITH attribution (metadata was written)
	normalImg := provider.Image{
		ID:            "MetMuseum_436528",
		Provider:      "Favorites",
		Attribution:   "Vincent van Gogh - Irises (Updated)",
		ViewURL:       "https://updated.url",
		SourceQueryID: FavoritesQueryID,
		IsFavorited:   true,
	}
	store.Add(normalImg)

	got, _ = store.GetByID("MetMuseum_436528")
	assert.Equal(t, "Vincent van Gogh - Irises (Updated)", got.Attribution, "Attribution should be updated when incoming is non-empty")
	assert.Equal(t, "https://updated.url", got.ViewURL, "ViewURL should be updated when incoming is non-empty")
}

// TestFavoritesUpsert_NewImageStillAdded verifies that completely new
// Favorites images (not yet in store) are still added normally.
func TestFavoritesUpsert_NewImageStillAdded(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	newFav := provider.Image{
		ID:            "LocalFolder_favorite_images_Wallhaven_new",
		Provider:      "Favorites",
		Attribution:   "artist",
		SourceQueryID: FavoritesQueryID,
		IsFavorited:   true,
	}
	result := store.Add(newFav)
	assert.True(t, result)
	assert.Equal(t, 1, store.Count())

	got, ok := store.GetByID("LocalFolder_favorite_images_Wallhaven_new")
	assert.True(t, ok)
	assert.Equal(t, "artist", got.Attribution)
}

// TestFavoritesUpsert_NonFavoritesQueryStillRejected verifies that non-Favorites
// duplicate images are still rejected by Add() (existing behavior preserved).
func TestFavoritesUpsert_NonFavoritesQueryStillRejected(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	original := provider.Image{
		ID:            "Wallhaven_abc",
		Provider:      "Wallhaven",
		Attribution:   "original artist",
		SourceQueryID: "wallhaven_query_1",
	}
	store.Add(original)

	// Same ID, different query (not Favorites) — should be rejected
	duplicate := provider.Image{
		ID:            "Wallhaven_abc",
		Provider:      "Wallhaven",
		Attribution:   "different artist",
		SourceQueryID: "wallhaven_query_2",
	}
	result := store.Add(duplicate)
	assert.False(t, result, "Non-Favorites duplicate should be rejected")

	got, _ := store.GetByID("Wallhaven_abc")
	assert.Equal(t, "original artist", got.Attribution, "Original attribution should be unchanged")
}

// =============================================================================
// Incremental Bucket Operation Tests (Issue 4)
//
// These tests verify that addToBucketsLocked/removeFromBucketsLocked produce
// identical results to the old rebuildBucketsLocked for every mutation path.
// =============================================================================

// TestBuckets_Replace_SameResolutions verifies that replace() with the same
// resolutions keeps buckets correct (remove old + add new = no change).
func TestBuckets_Replace_SameResolutions(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	img := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"1920x1080": "/old.jpg", "3440x1440": "/old_uw.jpg"},
	}
	store.Add(img)

	assert.Equal(t, 1, store.GetBucketSize("1920x1080"))
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"))

	// Replace with same resolutions, different paths
	updated := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"1920x1080": "/new.jpg", "3440x1440": "/new_uw.jpg"},
	}
	store.replace(updated)

	assert.Equal(t, 1, store.GetBucketSize("1920x1080"), "Bucket size should remain 1")
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"), "Bucket size should remain 1")
	assert.Contains(t, store.GetIDsForResolution("1920x1080"), "img1")
	assert.Contains(t, store.GetIDsForResolution("3440x1440"), "img1")
}

// TestBuckets_Replace_DifferentResolutions verifies that replace() correctly
// handles resolution changes — old resolutions removed, new ones added.
func TestBuckets_Replace_DifferentResolutions(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	img := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"1920x1080": "/hd.jpg"},
	}
	store.Add(img)
	assert.Equal(t, 1, store.GetBucketSize("1920x1080"))
	assert.Equal(t, 0, store.GetBucketSize("3440x1440"))

	// Replace with DIFFERENT resolution
	updated := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"3440x1440": "/uw.jpg"},
	}
	store.replace(updated)

	assert.Equal(t, 0, store.GetBucketSize("1920x1080"), "Old resolution should be removed from buckets")
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"), "New resolution should be added to buckets")
	assert.Contains(t, store.GetIDsForResolution("3440x1440"), "img1")
}

// TestBuckets_Replace_AddResolution verifies that replace() adding a new
// resolution to an existing image updates buckets correctly.
func TestBuckets_Replace_AddResolution(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	img := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"1920x1080": "/hd.jpg"},
	}
	store.Add(img)

	// Replace adding a second resolution
	updated := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"1920x1080": "/hd.jpg", "3440x1440": "/uw.jpg"},
	}
	store.replace(updated)

	assert.Equal(t, 1, store.GetBucketSize("1920x1080"), "Existing resolution should remain")
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"), "New resolution should be added")
}

// TestBuckets_Replace_ClearAllDerivatives verifies that replace() with empty
// DerivativePaths removes all bucket entries for that image.
func TestBuckets_Replace_ClearAllDerivatives(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	img := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"1920x1080": "/hd.jpg", "3440x1440": "/uw.jpg"},
	}
	store.Add(img)
	assert.Equal(t, 1, store.GetBucketSize("1920x1080"))
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"))

	// Replace with empty DerivativePaths
	updated := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{},
	}
	store.replace(updated)

	assert.Equal(t, 0, store.GetBucketSize("1920x1080"), "Old resolution buckets should be empty")
	assert.Equal(t, 0, store.GetBucketSize("3440x1440"), "Old resolution buckets should be empty")
}

// TestBuckets_ClearDerivatives verifies that ClearDerivatives() removes
// all bucket entries for the cleared image without affecting others.
func TestBuckets_ClearDerivatives(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	img1 := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"1920x1080": "/img1_hd.jpg", "3440x1440": "/img1_uw.jpg"},
	}
	img2 := provider.Image{
		ID:              "img2",
		DerivativePaths: map[string]string{"1920x1080": "/img2_hd.jpg"},
	}
	store.Add(img1)
	store.Add(img2)

	assert.Equal(t, 2, store.GetBucketSize("1920x1080"))
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"))

	// Clear derivatives for img1 only
	store.ClearDerivatives("img1")

	assert.Equal(t, 1, store.GetBucketSize("1920x1080"), "Only img2 should remain in 1920x1080 bucket")
	assert.Equal(t, 0, store.GetBucketSize("3440x1440"), "3440x1440 bucket should be empty")
	ids := store.GetIDsForResolution("1920x1080")
	assert.Equal(t, []string{"img2"}, ids, "Only img2 should be in bucket")
}

// TestBuckets_Remove_MultiImage verifies that Remove() correctly removes
// one image's bucket entries while preserving other images' entries.
func TestBuckets_Remove_MultiImage(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	img1 := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"1920x1080": "/1.jpg", "3440x1440": "/1_uw.jpg"},
	}
	img2 := provider.Image{
		ID:              "img2",
		DerivativePaths: map[string]string{"1920x1080": "/2.jpg", "3440x1440": "/2_uw.jpg"},
	}
	img3 := provider.Image{
		ID:              "img3",
		DerivativePaths: map[string]string{"1920x1080": "/3.jpg"},
	}
	store.Add(img1)
	store.Add(img2)
	store.Add(img3)

	assert.Equal(t, 3, store.GetBucketSize("1920x1080"))
	assert.Equal(t, 2, store.GetBucketSize("3440x1440"))

	// Remove img2 (middle)
	store.Remove("img2")

	assert.Equal(t, 2, store.GetBucketSize("1920x1080"), "Should have img1+img3")
	assert.Equal(t, 1, store.GetBucketSize("3440x1440"), "Should have img1 only")

	ids1080 := store.GetIDsForResolution("1920x1080")
	assert.Contains(t, ids1080, "img1")
	assert.NotContains(t, ids1080, "img2")
	assert.Contains(t, ids1080, "img3")

	ids1440 := store.GetIDsForResolution("3440x1440")
	assert.Contains(t, ids1440, "img1")
	assert.NotContains(t, ids1440, "img2")
}

// TestBuckets_Replace_OtherImagesUnaffected verifies that replace() on one
// image does NOT corrupt bucket entries for other images sharing the same
// resolution bucket.
func TestBuckets_Replace_OtherImagesUnaffected(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	img1 := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"1920x1080": "/1.jpg"},
	}
	img2 := provider.Image{
		ID:              "img2",
		DerivativePaths: map[string]string{"1920x1080": "/2.jpg"},
	}
	img3 := provider.Image{
		ID:              "img3",
		DerivativePaths: map[string]string{"1920x1080": "/3.jpg"},
	}
	store.Add(img1)
	store.Add(img2)
	store.Add(img3)
	assert.Equal(t, 3, store.GetBucketSize("1920x1080"))

	// Replace img2 (middle of the bucket slice) — resolution stays the same
	updated := provider.Image{
		ID:              "img2",
		DerivativePaths: map[string]string{"1920x1080": "/2_new.jpg"},
	}
	store.replace(updated)

	// All 3 should still be in the bucket
	assert.Equal(t, 3, store.GetBucketSize("1920x1080"))
	ids := store.GetIDsForResolution("1920x1080")
	assert.Contains(t, ids, "img1")
	assert.Contains(t, ids, "img2")
	assert.Contains(t, ids, "img3")
}

// TestBuckets_NoDerivatives_NoBucketEntry verifies that images with no
// DerivativePaths don't pollute buckets on Add, replace, or ClearDerivatives.
func TestBuckets_NoDerivatives_NoBucketEntry(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Add image with no derivatives
	img := provider.Image{ID: "img1"}
	store.Add(img)
	assert.Equal(t, 0, store.GetBucketSize("1920x1080"))

	// Replace with no derivatives
	store.replace(provider.Image{ID: "img1"})
	assert.Equal(t, 0, store.GetBucketSize("1920x1080"))

	// ClearDerivatives on image with no derivatives — should not panic
	store.ClearDerivatives("img1")
	assert.Equal(t, 0, store.GetBucketSize("1920x1080"))
}

// TestBuckets_IncrementalMatchesFullRebuild verifies that the incremental
// bucket operations produce IDENTICAL state to a full rebuildBucketsLocked().
// This is the definitive correctness test: run a complex mutation sequence,
// snapshot the incremental result, force a full rebuild, and compare.
func TestBuckets_IncrementalMatchesFullRebuild(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Build up a complex state with many mutations
	store.Add(provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"1920x1080": "/1_hd.jpg", "3440x1440": "/1_uw.jpg"},
	})
	store.Add(provider.Image{
		ID:              "img2",
		DerivativePaths: map[string]string{"1920x1080": "/2_hd.jpg", "2560x1440": "/2_2k.jpg"},
	})
	store.Add(provider.Image{
		ID:              "img3",
		DerivativePaths: map[string]string{"1920x1080": "/3_hd.jpg", "3440x1440": "/3_uw.jpg", "2560x1440": "/3_2k.jpg"},
	})
	store.Add(provider.Image{
		ID:              "img4",
		DerivativePaths: map[string]string{"3840x2160": "/4_4k.jpg"},
	})
	store.Add(provider.Image{ID: "img5"}) // No derivatives

	// Mutation 1: replace img1 — swap resolutions
	store.replace(provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"2560x1440": "/1_2k.jpg"}, // lost 1920x1080 and 3440x1440, gained 2560x1440
	})

	// Mutation 2: ClearDerivatives on img3
	store.ClearDerivatives("img3")

	// Mutation 3: Remove img2
	store.Remove("img2")

	// Mutation 4: replace img4 — add a resolution
	store.replace(provider.Image{
		ID:              "img4",
		DerivativePaths: map[string]string{"3840x2160": "/4_4k.jpg", "1920x1080": "/4_hd.jpg"},
	})

	// Mutation 5: replace img5 — from nothing to something
	store.replace(provider.Image{
		ID:              "img5",
		DerivativePaths: map[string]string{"3440x1440": "/5_uw.jpg"},
	})

	// Snapshot the incremental result
	incrementalBuckets := snapshotBuckets(store)

	// Force a full rebuild and snapshot
	store.mu.Lock()
	store.rebuildBucketsLocked()
	store.mu.Unlock()
	rebuildBuckets := snapshotBuckets(store)

	// Compare: every resolution should have the same sorted ID list
	assert.Equal(t, len(rebuildBuckets), len(incrementalBuckets),
		"Bucket count mismatch: rebuild=%d, incremental=%d", len(rebuildBuckets), len(incrementalBuckets))

	for res, rebuildIDs := range rebuildBuckets {
		incrementalIDs, exists := incrementalBuckets[res]
		assert.True(t, exists, "Resolution %s exists in rebuild but not in incremental", res)
		assert.ElementsMatch(t, rebuildIDs, incrementalIDs,
			"Resolution %s: rebuild=%v, incremental=%v", res, rebuildIDs, incrementalIDs)
	}
	for res := range incrementalBuckets {
		_, exists := rebuildBuckets[res]
		assert.True(t, exists, "Resolution %s exists in incremental but not in rebuild", res)
	}
}

// snapshotBuckets returns a deep copy of the resolution buckets for comparison.
func snapshotBuckets(store *ImageStore) map[string][]string {
	store.mu.RLock()
	defer store.mu.RUnlock()
	result := make(map[string][]string)
	for res, ids := range store.resolutionBuckets {
		cp := make([]string, len(ids))
		copy(cp, ids)
		result[res] = cp
	}
	return result
}

// TestBuckets_ConcurrentMutations runs Add, replace, Remove, and
// ClearDerivatives from multiple goroutines simultaneously.
// Run with `go test -race` to detect data races.
func TestBuckets_ConcurrentMutations(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Pre-populate
	for i := 0; i < 50; i++ {
		store.Add(provider.Image{
			ID:              fmt.Sprintf("img_%d", i),
			DerivativePaths: map[string]string{"1920x1080": fmt.Sprintf("/%d_hd.jpg", i)},
		})
	}

	done := make(chan bool)

	// Writer 1: Add new images
	go func() {
		for i := 50; i < 100; i++ {
			store.Add(provider.Image{
				ID: fmt.Sprintf("img_%d", i),
				DerivativePaths: map[string]string{
					"1920x1080": fmt.Sprintf("/%d_hd.jpg", i),
					"3440x1440": fmt.Sprintf("/%d_uw.jpg", i),
				},
			})
		}
		done <- true
	}()

	// Writer 2: Replace existing images with different resolutions
	go func() {
		for i := 0; i < 25; i++ {
			store.replace(provider.Image{
				ID:              fmt.Sprintf("img_%d", i),
				DerivativePaths: map[string]string{"3440x1440": fmt.Sprintf("/%d_uw_new.jpg", i)},
			})
		}
		done <- true
	}()

	// Writer 3: ClearDerivatives on some images
	go func() {
		for i := 25; i < 40; i++ {
			store.ClearDerivatives(fmt.Sprintf("img_%d", i))
		}
		done <- true
	}()

	// Writer 4: Remove some images
	go func() {
		for i := 40; i < 50; i++ {
			store.Remove(fmt.Sprintf("img_%d", i))
		}
		done <- true
	}()

	// Reader: Continuously read bucket sizes
	go func() {
		for i := 0; i < 200; i++ {
			store.GetBucketSize("1920x1080")
			store.GetBucketSize("3440x1440")
			store.GetIDsForResolution("1920x1080")
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify consistency: force rebuild and compare
	incrementalBuckets := snapshotBuckets(store)
	store.mu.Lock()
	store.rebuildBucketsLocked()
	store.mu.Unlock()
	rebuildBuckets := snapshotBuckets(store)

	assert.Equal(t, len(rebuildBuckets), len(incrementalBuckets),
		"After concurrent mutations, bucket count should match rebuild")

	for res, rebuildIDs := range rebuildBuckets {
		incrementalIDs := incrementalBuckets[res]
		assert.ElementsMatch(t, rebuildIDs, incrementalIDs,
			"After concurrent mutations, resolution %s should match: rebuild=%v, incremental=%v",
			res, rebuildIDs, incrementalIDs)
	}
}

func TestStore_MigrateLegacyCropAnchorsToTuning(t *testing.T) {
	// Simulate legacy JSON where "CropAnchors" is defined but "Tuning" is not.
	legacyJSON := `[{
		"ID": "legacy1",
		"CropAnchors": {
			"1920x1080": 2,
			"3440x1440": 8
		}
	}]`

	// Create a temp file and write legacy json
	tmpFile, err := os.CreateTemp("", "spice_legacy_test_*.json")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.Write([]byte(legacyJSON))
	assert.NoError(t, err)
	tmpFile.Close()

	tmpDir := t.TempDir()
	store := NewImageStore()
	store.SetFileManager(NewFileManager(tmpDir), tmpFile.Name())
	err = store.LoadCache()
	assert.NoError(t, err)

	// Ensure it loaded correctly into the new struct format
	img, ok := store.GetByID("legacy1")
	assert.True(t, ok)
	assert.Equal(t, "legacy1", img.ID)

	// Verify it auto-migrated via unmarshalling (Go json package natively supports this if we do it in UnmarshalJSON,
	// OR we can do it explicitly in store load. Wait, standard unmarshaling doesn't auto-migrate if we removed the field!
	// Oh! We removed CropAnchors from the struct! So standard json.Unmarshal will just silently drop it if we don't have custom logic!)

	// We MUST assert the Tuning is populated, otherwise we fail.
	assert.NotNil(t, img.Tuning)
	assert.Equal(t, provider.AnchorTopRight, img.GetTuning("1920x1080").Anchor)              // 2 is AnchorTopRight
	assert.Equal(t, provider.AnchorBottomRight, img.GetTuning("3440x1440").Anchor)           // 8 is AnchorBottomRight
	assert.Equal(t, provider.FrameOverrideInherit, img.GetTuning("1920x1080").FrameOverride) // Inherit is zero-value
}

func TestStore_SetTuningOptions(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.Add(provider.Image{ID: "test1"})

	// Set tuning for 1920x1080
	opts := provider.TuningOptions{
		Anchor:        provider.AnchorTopCenter,
		FrameOverride: provider.FrameOverrideForceOn,
		WallColor:     provider.WallColorOverrideAlgorithmic,
		Matting:       provider.MattingOverrideOff,
		FrameSize:     0.9,
	}
	ok := store.SetTuningOptions("test1", "1920x1080", opts)
	assert.True(t, ok)

	img, ok := store.GetByID("test1")
	assert.True(t, ok)
	assert.Equal(t, provider.AnchorTopCenter, img.GetTuning("1920x1080").Anchor)
	assert.Equal(t, provider.FrameOverrideForceOn, img.GetTuning("1920x1080").FrameOverride)
	assert.Equal(t, provider.WallColorOverrideAlgorithmic, img.GetTuning("1920x1080").WallColor)
	assert.Equal(t, provider.MattingOverrideOff, img.GetTuning("1920x1080").Matting)
	assert.Equal(t, 0.9, img.GetTuning("1920x1080").FrameSize)

	// Test clearing
	ok = store.SetTuningOptions("test1", "1920x1080", provider.TuningOptions{})
	assert.True(t, ok)
	img, ok = store.GetByID("test1")
	assert.True(t, ok)
	assert.Equal(t, provider.AnchorAuto, img.GetTuning("1920x1080").Anchor)
	assert.Equal(t, provider.FrameOverrideInherit, img.GetTuning("1920x1080").FrameOverride)
}

func TestStore_PersistenceAcrossRestarts(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "spice_persist_test_*.json")
	assert.NoError(t, err)
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Step 1: Create store, save complex struct
	store1 := NewImageStore()
	store1.SetAsyncSave(false)
	store1.SetFileManager(NewFileManager(t.TempDir()), tmpPath)
	store1.Add(provider.Image{ID: "persist_test"})

	opts := provider.TuningOptions{
		Anchor:        provider.AnchorMiddleRight,
		FrameOverride: provider.FrameOverrideForceOff,
		WallColor:     provider.WallColorOverrideNeutral,
		Matting:       provider.MattingOverrideOn,
		FrameSize:     0.75,
	}
	store1.SetTuningOptions("persist_test", "3440x1440", opts)

	// Step 2: "Restart" - read from disk into a fresh store
	store2 := NewImageStore()
	store2.SetFileManager(NewFileManager(t.TempDir()), tmpPath)
	err = store2.LoadCache()
	assert.NoError(t, err)

	img, ok := store2.GetByID("persist_test")
	assert.True(t, ok)
	assert.Equal(t, "persist_test", img.ID)

	loadedOpts := img.GetTuning("3440x1440")
	assert.Equal(t, provider.AnchorMiddleRight, loadedOpts.Anchor, "Anchor should persist")
	assert.Equal(t, provider.FrameOverrideForceOff, loadedOpts.FrameOverride, "FrameOverride should persist")
	assert.Equal(t, provider.WallColorOverrideNeutral, loadedOpts.WallColor, "WallColor should persist")
	assert.Equal(t, provider.MattingOverrideOn, loadedOpts.Matting, "Matting should persist")
	assert.Equal(t, 0.75, loadedOpts.FrameSize, "FrameSize should persist")
}

func TestStore_DetermineSyncAction(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	// Create a dummy master file so masterFileExists returns true
	dummyID := "test_image"
	masterPath, _ := fm.GetMasterPath(dummyID, ".jpg")
	os.WriteFile(masterPath, []byte("fake"), 0644)

	t.Run("Protect Favorites from Strict Mode", func(t *testing.T) {
		img := provider.Image{
			ID:            dummyID,
			SourceQueryID: "OldProviderQuery",
			IsFavorited:   true,
		}
		// Strict mode: activeQueryIDs does NOT contain "OldProviderQuery"
		activeQueryIDs := map[string]bool{"NewProviderQuery": true}
		targetFlags := map[string]bool{}

		action := store.determineSyncAction(img, activeQueryIDs, targetFlags)
		assert.Equal(t, ImageActionKeep, action, "Favorites should be protected from strict mode deletion")
	})

	t.Run("Protect New Unprocessed Images from Zombie Deletion", func(t *testing.T) {
		img := provider.Image{
			ID:              dummyID,
			SourceQueryID:   "NewProviderQuery",
			ProcessingFlags: map[string]bool{"SmartFit": true}, // Pretend Sync just invalidated it
			DerivativePaths: nil,                               // No derivatives yet (waiting in pipeline)
		}
		activeQueryIDs := map[string]bool{"NewProviderQuery": true}
		targetFlags := map[string]bool{"SmartFit": true}

		action := store.determineSyncAction(img, activeQueryIDs, targetFlags)
		assert.Equal(t, ImageActionKeep, action, "New unprocessed images with matching flags should not be deleted as zombies")
	})
}
