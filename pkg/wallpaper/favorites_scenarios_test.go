//go:build !linux

package wallpaper

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Favorites Lifecycle Scenario Tests
//
// These tests cover the 6 scenarios identified in the favorites system analysis
// (favorites_analysis.md). They verify correctness despite the ID namespace
// mismatch between favMap (bare IDs like "Wikimedia_14921563") and the store
// (LocalFolder-prefixed IDs like "LocalFolder_favorite_images_Wikimedia_14921563").
// =============================================================================

// --- Scenario 1: User favorites a Wallhaven image currently displayed ---
// The original source image in the store should get IsFavorited=true.
// The tray menu should show "Remove from Favorites".
func TestFavScenario1_FavoriteSourceImage(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Original Wallhaven image in the store (not yet favorited)
	wallhavenImg := provider.Image{
		ID:            "Wallhaven_4yd58l",
		Provider:      "Wallhaven",
		IsFavorited:   false,
		SourceQueryID: "wallhaven_query_1",
	}
	store.Add(wallhavenImg)

	mockFav := &mockFavoriter{}
	mockMgr := &mockManager{}
	mockFav.On("AddFavorite", wallhavenImg).Return(nil)
	mockMgr.On("NotifyUser", mock.Anything, mock.Anything).Return()

	wp := &Plugin{
		store:              store,
		favoriter:          mockFav,
		manager:            mockMgr,
		cfg:                GetConfig(NewMockPreferences()),
		providers:          make(map[string]provider.ImageProvider),
		Monitors:           make(map[int]*MonitorController),
		downloadMutex:      sync.RWMutex{},
		queryPages:         make(map[string]*util.SafeCounter),
		fetchingInProgress: util.NewSafeBool(),
	}

	wp.ToggleFavorite(wallhavenImg)

	// Verify: the store entry should now have IsFavorited=true
	got, ok := store.GetByID("Wallhaven_4yd58l")
	require.True(t, ok)
	assert.True(t, got.IsFavorited, "Source image should be marked as favorited in store")
	assert.Equal(t, "Wallhaven", got.Provider, "Provider should remain Wallhaven")

	mockFav.AssertCalled(t, "AddFavorite", wallhavenImg)
}

// --- Scenario 2: Favorites provider re-downloads the same image ---
// The store should have TWO entries: the original Wallhaven entry and
// the Favorites-provider copy with a different (LocalFolder-prefixed) ID.
// Both should have IsFavorited=true.
func TestFavScenario2_DualEntryAfterFavoritesFetch(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Step 1: Original Wallhaven image, already favorited
	wallhavenImg := provider.Image{
		ID:            "Wallhaven_4yd58l",
		Provider:      "Wallhaven",
		IsFavorited:   true,
		SourceQueryID: "wallhaven_query_1",
	}
	store.Add(wallhavenImg)

	// Step 2: Favorites provider fetches the same image (via local API server).
	// It gets a different ID (LocalFolder-prefixed) and IsFavorited=true (our fix).
	favCopy := provider.Image{
		ID:            "LocalFolder_favorite_images_Wallhaven_4yd58l",
		Provider:      "Favorites",
		IsFavorited:   true,
		SourceQueryID: FavoritesQueryID,
	}
	store.Add(favCopy)

	// Verify: both entries exist with correct state
	assert.Equal(t, 2, store.Count(), "Store should have 2 entries")

	orig, ok := store.GetByID("Wallhaven_4yd58l")
	require.True(t, ok)
	assert.True(t, orig.IsFavorited)
	assert.Equal(t, "Wallhaven", orig.Provider)

	copy, ok := store.GetByID("LocalFolder_favorite_images_Wallhaven_4yd58l")
	require.True(t, ok)
	assert.True(t, copy.IsFavorited, "Favorites copy should have IsFavorited=true")
	assert.Equal(t, "Favorites", copy.Provider)
	assert.Equal(t, FavoritesQueryID, copy.SourceQueryID)
}

// --- Scenario 3: User removes the Wallhaven query but image was favorited ---
// The original Wallhaven entry should be pruned by Sync (inactive query).
// The Favorites copy should survive (FavoritesQueryID is always active).
// IsFavorited plays NO role in pruning — it's activeQueryIDs that protects.
func TestFavScenario3_SourceQueryRemoved_FavoritesSurvives(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore()
	fm := NewFileManager(tmpDir)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))
	store.SetAsyncSave(false)

	// Both entries exist
	wallhavenImg := provider.Image{
		ID:              "Wallhaven_4yd58l",
		Provider:        "Wallhaven",
		IsFavorited:     true,
		SourceQueryID:   "wallhaven_query_1",
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(tmpDir, "fitted", "w1.jpg")},
		ProcessingFlags: map[string]bool{"SmartFit": true},
	}
	favCopy := provider.Image{
		ID:              "LocalFolder_favorite_images_Wallhaven_4yd58l",
		Provider:        "Favorites",
		IsFavorited:     true,
		SourceQueryID:   FavoritesQueryID,
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(tmpDir, "fitted", "f1.jpg")},
		ProcessingFlags: map[string]bool{"SmartFit": true},
	}

	// Create master files (directly in rootDir) and derivative files
	fittedDir := filepath.Join(tmpDir, "fitted")
	require.NoError(t, os.MkdirAll(fittedDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Wallhaven_4yd58l.jpg"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "LocalFolder_favorite_images_Wallhaven_4yd58l.jpg"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(fittedDir, "w1.jpg"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(fittedDir, "f1.jpg"), []byte("fake"), 0644))

	store.Add(wallhavenImg)
	store.Add(favCopy)
	assert.Equal(t, 2, store.Count())

	// Sync with only FavoritesQueryID active (Wallhaven query removed)
	activeQueries := map[string]bool{
		FavoritesQueryID: true,
	}
	store.Sync(5000, map[string]bool{"SmartFit": true}, activeQueries)

	// Wallhaven entry should be deleted (inactive query)
	_, ok := store.GetByID("Wallhaven_4yd58l")
	assert.False(t, ok, "Wallhaven image should be pruned — its query is no longer active")

	// Favorites copy should survive (FavoritesQueryID is active)
	fav, ok := store.GetByID("LocalFolder_favorite_images_Wallhaven_4yd58l")
	assert.True(t, ok, "Favorites copy should survive — FavoritesQueryID is active")
	assert.True(t, fav.IsFavorited)
}

// --- Scenario 4: User unfavorites a Provider=Favorites image ---
// Even if IsFavorited was somehow false (pre-fix), ToggleFavorite should
// still take the "Remove" branch because Provider == "Favorites".
func TestFavScenario4_UnfavoriteFavoritesProviderImage(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Case A: IsFavorited=true (normal case with our fix)
	favImgTrue := provider.Image{
		ID:            "LocalFolder_favorite_images_Wikimedia_14921563",
		Provider:      "Favorites",
		IsFavorited:   true,
		SourceQueryID: FavoritesQueryID,
	}
	store.Add(favImgTrue)

	mockFav := &mockFavoriter{}
	mockMgr := &mockManager{}
	mockFav.On("RemoveFavorite", favImgTrue).Return(nil)
	mockMgr.On("NotifyUser", mock.Anything, mock.Anything).Return()

	wp := &Plugin{
		store:              store,
		favoriter:          mockFav,
		manager:            mockMgr,
		cfg:                GetConfig(NewMockPreferences()),
		providers:          make(map[string]provider.ImageProvider),
		Monitors:           make(map[int]*MonitorController),
		fetchingInProgress: util.NewSafeBool(),
	}

	wp.ToggleFavorite(favImgTrue)

	// Provider=Favorites images take the deep-delete path (store.Remove)
	_, ok := store.GetByID("LocalFolder_favorite_images_Wikimedia_14921563")
	assert.False(t, ok, "Provider=Favorites image should be deep-deleted from store on unfavorite")
	mockFav.AssertCalled(t, "RemoveFavorite", favImgTrue)
}

// --- Scenario 4b: Provider=Favorites image with IsFavorited=false (pre-fix data) ---
// Even with stale IsFavorited=false, the Provider field should drive the branch.
func TestFavScenario4b_UnfavoriteFavoritesProviderImage_StaleFalseFlag(t *testing.T) {
	store := NewImageStore()
	store.SetAsyncSave(false)

	// Case B: IsFavorited=false (stale pre-fix data, but Provider=Favorites)
	favImgFalse := provider.Image{
		ID:            "LocalFolder_favorite_images_Wikimedia_14921563",
		Provider:      "Favorites",
		IsFavorited:   false, // Stale pre-fix value
		SourceQueryID: FavoritesQueryID,
	}
	store.Add(favImgFalse)

	mockFav := &mockFavoriter{}
	mockMgr := &mockManager{}
	mockFav.On("RemoveFavorite", favImgFalse).Return(nil)
	mockMgr.On("NotifyUser", mock.Anything, mock.Anything).Return()

	wp := &Plugin{
		store:              store,
		favoriter:          mockFav,
		manager:            mockMgr,
		cfg:                GetConfig(NewMockPreferences()),
		providers:          make(map[string]provider.ImageProvider),
		Monitors:           make(map[int]*MonitorController),
		fetchingInProgress: util.NewSafeBool(),
	}

	// Even with IsFavorited=false, Provider=Favorites should drive the branch
	wp.ToggleFavorite(favImgFalse)

	// Should still take the "Remove" path (deep delete)
	_, ok := store.GetByID("LocalFolder_favorite_images_Wikimedia_14921563")
	assert.False(t, ok, "Provider=Favorites image should be deep-deleted even with IsFavorited=false")
	mockFav.AssertCalled(t, "RemoveFavorite", favImgFalse)
}

// --- Scenario 5: reconcileFavorites skips Provider=Favorites images ---
// Provider=Favorites images have LocalFolder-prefixed IDs that never match
// favMap keys. Reconcile must skip them to avoid spurious removals.
// Non-Favorites images with stale flags should still be corrected.
func TestFavScenario5_ReconcileSkipsFavoritesProvider(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore()
	fm := NewFileManager(tmpDir)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))
	store.SetAsyncSave(false)

	// Provider=Favorites image — reconcile should skip this entirely
	favImg := provider.Image{
		ID:            "LocalFolder_favorite_images_Wikimedia_14921563",
		Provider:      "Favorites",
		IsFavorited:   true,
		SourceQueryID: FavoritesQueryID,
		FilePath:      filepath.Join(tmpDir, "fav.jpg"),
	}
	require.NoError(t, os.WriteFile(favImg.FilePath, []byte("fake"), 0644))
	store.Add(favImg)

	// Wallhaven ghost favorite — flag says true but provider says false
	ghostFav := provider.Image{
		ID:          "Wallhaven_ghost",
		Provider:    "Wallhaven",
		IsFavorited: true,
		FilePath:    filepath.Join(tmpDir, "ghost.jpg"),
	}
	require.NoError(t, os.WriteFile(ghostFav.FilePath, []byte("fake"), 0644))
	store.Add(ghostFav)

	// Wallhaven real favorite — both agree
	realFav := provider.Image{
		ID:          "Wallhaven_real",
		Provider:    "Wallhaven",
		IsFavorited: true,
		FilePath:    filepath.Join(tmpDir, "real.jpg"),
	}
	require.NoError(t, os.WriteFile(realFav.FilePath, []byte("fake"), 0644))
	store.Add(realFav)

	// Normal unfavorited image
	normal := provider.Image{
		ID:          "MetMuseum_123",
		Provider:    "MetMuseum",
		IsFavorited: false,
		FilePath:    filepath.Join(tmpDir, "normal.jpg"),
	}
	require.NoError(t, os.WriteFile(normal.FilePath, []byte("fake"), 0644))
	store.Add(normal)

	mockFav := &mockFavoriter{}
	// IsFavorited should NOT be called for Provider=Favorites image
	// Only called for Wallhaven and MetMuseum images
	mockFav.On("IsFavorited", ghostFav).Return(false) // ghost: provider disagrees
	mockFav.On("IsFavorited", realFav).Return(true)    // real: provider agrees
	mockFav.On("IsFavorited", normal).Return(false)    // normal: both agree

	wp := &Plugin{
		store:     store,
		favoriter: mockFav,
	}

	wp.reconcileFavorites()

	// Provider=Favorites image: UNTOUCHED
	img, ok := store.GetByID("LocalFolder_favorite_images_Wikimedia_14921563")
	require.True(t, ok, "Favorites-provider image must not be removed by reconcile")
	assert.True(t, img.IsFavorited, "Favorites-provider IsFavorited must remain true")

	// Ghost favorite: flag corrected to false
	img, ok = store.GetByID("Wallhaven_ghost")
	require.True(t, ok)
	assert.False(t, img.IsFavorited, "Ghost favorite flag should be corrected to false")

	// Real favorite: unchanged
	img, ok = store.GetByID("Wallhaven_real")
	require.True(t, ok)
	assert.True(t, img.IsFavorited, "Real favorite should remain true")

	// Normal image: unchanged
	img, ok = store.GetByID("MetMuseum_123")
	require.True(t, ok)
	assert.False(t, img.IsFavorited, "Normal image should remain false")

	// Verify IsFavorited was NOT called for the Favorites-provider image
	mockFav.AssertNotCalled(t, "IsFavorited", favImg)
}

// --- Scenario 6: Favorites provider is disabled ---
// When the Favorites query is removed from activeQueryIDs, Sync should
// prune all Favorites-provider entries. Files remain on disk.
// When re-enabled, images re-enter the store via FetchImages.
func TestFavScenario6_FavoritesQueryDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore()
	fm := NewFileManager(tmpDir)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))
	store.SetAsyncSave(false)

	fittedDir := filepath.Join(tmpDir, "fitted")
	require.NoError(t, os.MkdirAll(fittedDir, 0755))

	// Favorites entry
	favImg := provider.Image{
		ID:              "LocalFolder_favorite_images_Wikimedia_14921563",
		Provider:        "Favorites",
		IsFavorited:     true,
		SourceQueryID:   FavoritesQueryID,
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(fittedDir, "fav.jpg")},
		ProcessingFlags: map[string]bool{"SmartFit": true},
	}
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "LocalFolder_favorite_images_Wikimedia_14921563.jpg"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(fittedDir, "fav.jpg"), []byte("fake"), 0644))
	store.Add(favImg)

	// Wallhaven entry (active query)
	wallhavenImg := provider.Image{
		ID:              "Wallhaven_4yd58l",
		Provider:        "Wallhaven",
		IsFavorited:     false,
		SourceQueryID:   "wallhaven_query_1",
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(fittedDir, "wh.jpg")},
		ProcessingFlags: map[string]bool{"SmartFit": true},
	}
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Wallhaven_4yd58l.jpg"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(fittedDir, "wh.jpg"), []byte("fake"), 0644))
	store.Add(wallhavenImg)

	assert.Equal(t, 2, store.Count())

	// Sync with Favorites query DISABLED (only Wallhaven active)
	activeQueries := map[string]bool{
		"wallhaven_query_1": true,
		// FavoritesQueryID is NOT included
	}
	store.Sync(5000, map[string]bool{"SmartFit": true}, activeQueries)

	// Favorites entry should be pruned
	_, ok := store.GetByID("LocalFolder_favorite_images_Wikimedia_14921563")
	assert.False(t, ok, "Favorites entry should be pruned when Favorites query is disabled")

	// Wallhaven entry should survive
	_, ok = store.GetByID("Wallhaven_4yd58l")
	assert.True(t, ok, "Wallhaven entry should survive — its query is active")

	// Re-enable: simulate Favorites provider fetching the image again
	favImgRefetched := provider.Image{
		ID:            "LocalFolder_favorite_images_Wikimedia_14921563",
		Provider:      "Favorites",
		IsFavorited:   true,
		SourceQueryID: FavoritesQueryID,
	}
	store.Add(favImgRefetched)

	// Should be back in the store
	got, ok := store.GetByID("LocalFolder_favorite_images_Wikimedia_14921563")
	assert.True(t, ok, "Favorites image should re-enter store after re-fetch")
	assert.True(t, got.IsFavorited)
}

// --- Cross-cutting: IsFavorited flag is defense-in-depth, NOT pruning protection ---
// Verify that determineSyncAction does NOT check IsFavorited.
// An unfavorited image with an active query should NOT be pruned.
func TestFavCrossCutting_IsFavoritedDoesNotAffectPruning(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore()
	fm := NewFileManager(tmpDir)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))
	store.SetAsyncSave(false)

	fittedDir := filepath.Join(tmpDir, "fitted")
	require.NoError(t, os.MkdirAll(fittedDir, 0755))

	// Unfavorited image with active query
	unfavImg := provider.Image{
		ID:              "Wallhaven_xyz",
		Provider:        "Wallhaven",
		IsFavorited:     false,
		SourceQueryID:   "wallhaven_query_1",
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(fittedDir, "wh.jpg")},
		ProcessingFlags: map[string]bool{"SmartFit": true},
	}
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Wallhaven_xyz.jpg"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(fittedDir, "wh.jpg"), []byte("fake"), 0644))
	store.Add(unfavImg)

	// Favorited image with active query
	favImg := provider.Image{
		ID:              "Wallhaven_abc",
		Provider:        "Wallhaven",
		IsFavorited:     true,
		SourceQueryID:   "wallhaven_query_1",
		DerivativePaths: map[string]string{"3440x1440": filepath.Join(fittedDir, "wh2.jpg")},
		ProcessingFlags: map[string]bool{"SmartFit": true},
	}
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Wallhaven_abc.jpg"), []byte("fake"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(fittedDir, "wh2.jpg"), []byte("fake"), 0644))
	store.Add(favImg)

	activeQueries := map[string]bool{
		"wallhaven_query_1": true,
	}
	store.Sync(5000, map[string]bool{"SmartFit": true}, activeQueries)

	// Both should survive — IsFavorited is irrelevant to Sync
	_, ok := store.GetByID("Wallhaven_xyz")
	assert.True(t, ok, "Unfavorited image with active query should NOT be pruned")

	_, ok = store.GetByID("Wallhaven_abc")
	assert.True(t, ok, "Favorited image with active query should NOT be pruned")
}
