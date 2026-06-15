package wallpaper

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Crop Anchor Persistence — Regression Test Suite
// =============================================================================
//
// These tests guard against the guaranteed-clobber bug where CropAnchors were
// deterministically destroyed during settings changes (double-Sync pattern)
// and pipeline replacements.
//
// Run with: go test -v -run "TestCropAnchor_" ./pkg/wallpaper
//
// If ANY of these tests fail, user-set crop anchors are being silently lost.

// =============================================================================
// Category 1: Fix Mechanics (Unit-Level)
// =============================================================================

// TestCropAnchor_Replace_PreservesExisting verifies that replace() with a
// pipeline result (no CropAnchors) does NOT overwrite existing store anchors.
// This is the secondary bug: the pipeline never writes CropAnchors, so the
// store's values must survive a full struct replacement.
func TestCropAnchor_Replace_PreservesExisting(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Step 1: Image enters store with derivatives (normal pipeline completion)
	initial := provider.Image{
		ID:              "img1",
		Width:           4000,
		Height:          3000,
		DerivativePaths: map[string]string{"3440x1440": "/path/to/deriv.jpg"},
		ProcessingFlags: map[string]bool{"SmartFit": true},
	}
	store.Add(initial)

	// Step 2: User sets crop anchor via UI
	store.SetCropAnchor("img1", "3440x1440", provider.AnchorTopCenter)

	// Verify anchor was stored
	got, _ := store.GetByID("img1")
	assert.Equal(t, provider.AnchorTopCenter, got.CropAnchors["3440x1440"])

	// Step 3: Pipeline re-processes image (backlog healing) — result has NO CropAnchors
	pipelineResult := provider.Image{
		ID:              "img1",
		Width:           4000,
		Height:          3000,
		DerivativePaths: map[string]string{"3440x1440": "/path/to/new_deriv.jpg"},
		ProcessingFlags: map[string]bool{"SmartFit": true},
		// CropAnchors: nil — pipeline never sets this
	}
	store.replace(pipelineResult)

	// Step 4: Verify anchor survived the replace
	got, ok := store.GetByID("img1")
	assert.True(t, ok)
	assert.Equal(t, provider.AnchorTopCenter, got.CropAnchors["3440x1440"],
		"CropAnchors must survive replace() — the store is authoritative for user metadata")
	assert.Equal(t, "/path/to/new_deriv.jpg", got.DerivativePaths["3440x1440"],
		"DerivativePaths should still be updated by replace")
}

// TestCropAnchor_Replace_HonorsRemoval verifies that if the user removed all
// anchors (via AnchorAuto) while the pipeline was processing, replace() does
// NOT resurrect stale anchors from MergeExistingMetadata.
func TestCropAnchor_Replace_HonorsRemoval(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Step 1: Image with an anchor
	img := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"3440x1440": "/deriv.jpg"},
		CropAnchors:     map[string]provider.CropAnchor{"3440x1440": provider.AnchorTopCenter},
	}
	store.Add(img)

	// Step 2: User removes the anchor (SetCropAnchor with AnchorAuto deletes the key)
	store.SetCropAnchor("img1", "3440x1440", provider.AnchorAuto)

	// Verify anchor is gone from store
	got, _ := store.GetByID("img1")
	_, exists := got.CropAnchors["3440x1440"]
	assert.False(t, exists, "Anchor should be removed after AnchorAuto")

	// Step 3: Pipeline result arrives with stale anchor from MergeExistingMetadata
	// (this happens when MergeExistingMetadata ran before the user cleared the anchor)
	pipelineResult := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"3440x1440": "/new_deriv.jpg"},
		CropAnchors:     map[string]provider.CropAnchor{"3440x1440": provider.AnchorTopCenter}, // stale!
	}
	store.replace(pipelineResult)

	// Step 4: The store's empty state must win — anchor must NOT be resurrected
	got, _ = store.GetByID("img1")
	assert.Nil(t, got.CropAnchors,
		"replace() must NOT resurrect anchors the user explicitly removed")
}

// TestCropAnchor_Replace_MidFlight verifies that if the user changes an anchor
// WHILE the pipeline is processing, the user's newer choice wins.
func TestCropAnchor_Replace_MidFlight(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Step 1: Image with initial anchor
	img := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"3440x1440": "/deriv.jpg"},
		CropAnchors:     map[string]provider.CropAnchor{"3440x1440": provider.AnchorTopCenter},
	}
	store.Add(img)

	// Step 2: Pipeline picks up image (via MergeExistingMetadata — gets TopCenter)
	// ... pipeline is processing ...

	// Step 3: User changes anchor to BottomRight while pipeline runs
	store.SetCropAnchor("img1", "3440x1440", provider.AnchorBottomRight)

	// Step 4: Pipeline finishes — its struct has the OLD anchor from MergeExistingMetadata
	pipelineResult := provider.Image{
		ID:              "img1",
		DerivativePaths: map[string]string{"3440x1440": "/new_deriv.jpg"},
		CropAnchors:     map[string]provider.CropAnchor{"3440x1440": provider.AnchorTopCenter}, // stale!
	}
	store.replace(pipelineResult)

	// Step 5: User's BottomRight must win
	got, _ := store.GetByID("img1")
	assert.Equal(t, provider.AnchorBottomRight, got.CropAnchors["3440x1440"],
		"User's latest anchor choice must win over the pipeline's stale copy")
}

// TestCropAnchor_ZombieExemption verifies that determineSyncAction() returns
// ImageActionKeep (not ImageActionDelete) for invalidated images that have
// user-set CropAnchors.
func TestCropAnchor_ZombieExemption(t *testing.T) {
	tmpDir := t.TempDir()
	store, fm := newZombieTestStore(t, tmpDir)
	flags := defaultTargetFlags()

	// Create an image with correct flags, master file, but NO derivatives
	// (this is the state after Sync #1 invalidation)
	img := newZombieImage(t, fm, "anchored_zombie", "q1", flags)
	img.CropAnchors = map[string]provider.CropAnchor{"3440x1440": provider.AnchorTopCenter}
	store.Add(img)

	// Also add a true zombie (no anchors) to verify it's still deleted
	trueZombie := newZombieImage(t, fm, "true_zombie", "q1", flags)
	store.Add(trueZombie)

	// Sync — should keep the anchored image, delete the true zombie
	store.Sync(100, flags, nil)

	known := store.GetKnownIDs()
	assert.True(t, known["anchored_zombie"],
		"Image with CropAnchors must survive zombie detection")
	assert.False(t, known["true_zombie"],
		"True zombie (no CropAnchors) must still be deleted")

	// Verify anchors are intact
	got, _ := store.GetByID("anchored_zombie")
	assert.Equal(t, provider.AnchorTopCenter, got.CropAnchors["3440x1440"])
}

// TestCropAnchor_ZombieWithoutAnchors_StillDeleted verifies that the zombie
// recovery path still functions correctly for images without CropAnchors.
// This is a guard against accidental over-protection.
func TestCropAnchor_ZombieWithoutAnchors_StillDeleted(t *testing.T) {
	tmpDir := t.TempDir()
	store, fm := newZombieTestStore(t, tmpDir)
	flags := defaultTargetFlags()

	// Three zombies: nil anchors, empty map, and zero-length map
	zombie1 := newZombieImage(t, fm, "z_nil", "q1", flags)
	zombie1.CropAnchors = nil
	store.Add(zombie1)

	zombie2 := newZombieImage(t, fm, "z_empty", "q1", flags)
	zombie2.CropAnchors = map[string]provider.CropAnchor{}
	store.Add(zombie2)

	zombie3 := newZombieImage(t, fm, "z_default", "q1", flags)
	// CropAnchors not set (zero value)
	store.Add(zombie3)

	// A healthy image to keep the store non-empty
	healthy := newHealthyImage(t, fm, "healthy", "q1", "3440x1440", flags)
	store.Add(healthy)

	store.Sync(100, flags, nil)

	assert.Equal(t, 1, store.Count(), "Only healthy image should remain")
	assert.True(t, store.GetKnownIDs()["healthy"])
	assert.False(t, store.GetKnownIDs()["z_nil"], "Zombie with nil CropAnchors must be deleted")
	assert.False(t, store.GetKnownIDs()["z_empty"], "Zombie with empty CropAnchors must be deleted")
	assert.False(t, store.GetKnownIDs()["z_default"], "Zombie with default CropAnchors must be deleted")
}

// =============================================================================
// Category 2: Lifecycle Scenarios (Integration-Level)
// =============================================================================

// TestCropAnchor_SurviveDoubleSyncModeChange reproduces the EXACT production
// bug: user sets a crop anchor, then changes SmartFit mode. The double-Sync
// pattern (invalidation → zombie check) must NOT destroy the anchor.
//
// Kill chain without fix:
//
//	Sync #1: flag mismatch → ImageActionInvalidate → DerivativePaths wiped
//	Sync #2: flags match, DerivativePaths=0 → ImageActionDelete → CropAnchors GONE
//
// With fix:
//
//	Sync #2: flags match, DerivativePaths=0, CropAnchors>0 → ImageActionKeep
func TestCropAnchor_SurviveDoubleSyncModeChange(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, cacheFile)

	// === SETUP: Image processed with SmartFit Aggressive mode ===
	originalFlags := makeDownloaderFlags(true, SmartFitAggressive, false, false)
	img := provider.Image{
		ID:              "victim_img",
		SourceQueryID:   "q1",
		ProcessingFlags: copyFlags(originalFlags),
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(tmpDir, "fitted", "victim.jpg")},
	}
	createMasterFile(t, fm, "victim_img")
	store.Add(img)

	// User sets a crop anchor
	store.SetCropAnchor("victim_img", "3440x1440", provider.AnchorBottomCenter)

	// Verify setup
	got, _ := store.GetByID("victim_img")
	assert.Equal(t, provider.AnchorBottomCenter, got.CropAnchors["3440x1440"],
		"Setup: anchor should be set")
	assert.NotEmpty(t, got.DerivativePaths, "Setup: derivatives should exist")

	// === SYNC #1: User changes SmartFit to Normal mode ===
	// This is the mode change that triggers invalidation
	newTarget := makeGroomingTarget(true, SmartFitNormal, false, false)
	store.Sync(100, newTarget, nil)

	// After Sync #1: image should be invalidated (derivatives wiped) but alive
	got, ok := store.GetByID("victim_img")
	assert.True(t, ok, "Image must survive invalidation")
	assert.Empty(t, got.DerivativePaths, "Derivatives should be wiped by invalidation")
	assert.Equal(t, provider.AnchorBottomCenter, got.CropAnchors["3440x1440"],
		"CropAnchors must survive invalidation")

	// === SYNC #2: The killer — same flags, zero derivatives ===
	// Without the fix, this zombie-deletes the image and CropAnchors are gone forever.
	store.Sync(100, newTarget, nil)

	// === ASSERTION: Image and anchors MUST survive ===
	got, ok = store.GetByID("victim_img")
	assert.True(t, ok,
		"CRITICAL: Image must survive the double-Sync pattern — zombie exemption must fire")
	assert.Equal(t, provider.AnchorBottomCenter, got.CropAnchors["3440x1440"],
		"CRITICAL: CropAnchors must survive the double-Sync pattern")
}

// TestCropAnchor_SurviveTripleSyncStartup verifies that anchors survive 3
// consecutive Sync calls, which can happen during worst-case startup:
// Activate() + RefreshImagesAndPulse() + scheduler.
func TestCropAnchor_SurviveTripleSyncStartup(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	flags := makeDownloaderFlags(true, SmartFitAggressive, false, false)
	target := makeGroomingTarget(true, SmartFitAggressive, false, false)

	// Image with matching flags and anchor
	img := newHealthyImage(t, fm, "triple_sync", "q1", "3440x1440", flags)
	img.CropAnchors = map[string]provider.CropAnchor{"3440x1440": provider.AnchorTopLeft}
	store.Add(img)

	// Three consecutive syncs with identical flags (steady-state startup)
	store.Sync(100, target, nil)
	store.Sync(100, target, nil)
	store.Sync(100, target, nil)

	got, ok := store.GetByID("triple_sync")
	assert.True(t, ok, "Image must survive three consecutive Syncs")
	assert.Equal(t, provider.AnchorTopLeft, got.CropAnchors["3440x1440"],
		"CropAnchors must survive three consecutive Syncs")
	assert.NotEmpty(t, got.DerivativePaths, "Derivatives should be preserved (flags matched)")
}

// TestCropAnchor_SurviveSaveLoadCycle verifies that CropAnchors persist
// across cache serialization/deserialization (JSON round-trip).
func TestCropAnchor_SurviveSaveLoadCycle(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	cacheFile := filepath.Join(tmpDir, "cache.json")

	// Session 1: Create image, set anchor, save
	store1 := NewImageStore()
	store1.SetAsyncSave(false)
	store1.SetFileManager(fm, cacheFile)

	store1.Add(provider.Image{
		ID:              "persist_test",
		DerivativePaths: map[string]string{"3440x1440": "/deriv.jpg"},
	})
	store1.SetCropAnchor("persist_test", "3440x1440", provider.AnchorMiddleRight)
	store1.SetCropAnchor("persist_test", "1920x1080", provider.AnchorBottomLeft)
	store1.SaveCache()

	// Verify cache file was written
	_, err := os.Stat(cacheFile)
	assert.NoError(t, err, "Cache file should exist")

	// Session 2: New store, load from disk
	store2 := NewImageStore()
	store2.SetAsyncSave(false)
	store2.SetFileManager(fm, cacheFile)
	err = store2.LoadCache()
	assert.NoError(t, err)

	// Verify anchors survived the round-trip
	got, ok := store2.GetByID("persist_test")
	assert.True(t, ok)
	assert.Equal(t, provider.AnchorMiddleRight, got.CropAnchors["3440x1440"],
		"3440x1440 anchor must survive Save/Load cycle")
	assert.Equal(t, provider.AnchorBottomLeft, got.CropAnchors["1920x1080"],
		"1920x1080 anchor must survive Save/Load cycle")
}

// TestCropAnchor_NotExemptFromQueryPruning verifies that CropAnchors do NOT
// grant immunity from query-based pruning. If the image's source query is
// inactive, it should still be deleted — anchors are not a reason to keep
// orphaned images.
func TestCropAnchor_NotExemptFromQueryPruning(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	flags := makeDownloaderFlags(true, SmartFitAggressive, false, false)

	// Image from query "qDEAD" with a crop anchor
	img := newHealthyImage(t, fm, "orphan_anchored", "qDEAD", "3440x1440", flags)
	img.CropAnchors = map[string]provider.CropAnchor{"3440x1440": provider.AnchorTopCenter}
	store.Add(img)

	// Image from active query (for comparison)
	active := newHealthyImage(t, fm, "active_img", "qLIVE", "3440x1440", flags)
	store.Add(active)

	// Sync with strict query filtering — qDEAD is not in active set
	activeQueries := map[string]bool{"qLIVE": true}
	target := makeGroomingTarget(true, SmartFitAggressive, false, false)
	store.Sync(100, target, activeQueries)

	known := store.GetKnownIDs()
	assert.False(t, known["orphan_anchored"],
		"CropAnchors must NOT protect images from inactive-query pruning")
	assert.True(t, known["active_img"],
		"Active query image should survive")
}

// TestCropAnchor_NotExemptFromLRUPruning verifies that CropAnchors do NOT
// grant immunity from cache-limit (LRU) pruning. If the store exceeds its
// limit, older images with anchors are still evicted.
func TestCropAnchor_NotExemptFromLRUPruning(t *testing.T) {
	tmpDir := t.TempDir()
	fm := NewFileManager(tmpDir)
	store := NewImageStore()
	store.SetAsyncSave(false)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	flags := makeDownloaderFlags(true, SmartFitAggressive, false, false)

	// Add 5 images, all with crop anchors
	for i := 0; i < 5; i++ {
		id := "lru_" + string(rune('A'+i))
		img := newHealthyImage(t, fm, id, "q1", "3440x1440", flags)
		img.CropAnchors = map[string]provider.CropAnchor{"3440x1440": provider.AnchorTopCenter}
		store.Add(img)
	}
	assert.Equal(t, 5, store.Count())

	// Sync with limit 2 — oldest 3 should be pruned despite having anchors
	target := makeGroomingTarget(true, SmartFitAggressive, false, false)
	store.Sync(2, target, nil)

	assert.Equal(t, 2, store.Count(),
		"Cache limit must be enforced even for images with CropAnchors")
}
