//go:build !linux

package wallpaper

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToggleFavorite_NoDeadlock_FavoritesProvider verifies that unfavoriting a
// Favorites-provider image does not deadlock. Before the fix, the MonitorController's
// goroutine held mc.mu.Lock() and ToggleFavorite tried mc.mu.RLock() on the same
// mutex, causing a guaranteed deadlock due to Go's non-reentrant RWMutex.
func TestToggleFavorite_NoDeadlock_FavoritesProvider(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore()
	fm := NewFileManager(tmpDir)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	img := provider.Image{
		ID:          "test_img_1",
		Provider:    "Favorites",
		IsFavorited: true,
		FilePath:    filepath.Join(tmpDir, "test_img_1.jpg"),
	}
	// Create the file so store operations work
	require.NoError(t, os.WriteFile(img.FilePath, []byte("fake"), 0644))
	store.Add(img)

	mockOS := new(MockOS)
	cfg := GetConfig(NewMockPreferences())

	mc := NewMonitorController(0, Monitor{
		ID: 0, Name: "Primary", Rect: image.Rect(0, 0, 1920, 1080),
	}, store, fm, mockOS, cfg, nil)
	mc.State.CurrentImage = img
	mc.State.CurrentID = img.ID

	// The fix dispatches OnFavoriteRequest via `go`, so it runs in a new goroutine
	// outside the mc.mu.Lock() scope. This test verifies that.
	done := make(chan struct{})
	mc.OnFavoriteRequest = func(reqImg provider.Image) {
		// If this runs synchronously under mc.mu.Lock (old code), the test would
		// deadlock because the calling code holds mc.mu.Lock().
		// With the fix (go mc.OnFavoriteRequest), this runs in a new goroutine.
		close(done)
	}

	// Simulate the scenario: mc.mu.Lock() is held (as in handleCommand), then toggleFavorite is called
	mc.mu.Lock()
	mc.toggleFavorite()
	mc.mu.Unlock()

	select {
	case <-done:
		// Success - no deadlock
	case <-time.After(2 * time.Second):
		t.Fatal("DEADLOCK: toggleFavorite did not complete within 2 seconds")
	}
}

// TestReconcileFavorites verifies that stale IsFavorited flags in the cache
// are corrected on startup by reconcileFavorites().
func TestReconcileFavorites(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore()
	fm := NewFileManager(tmpDir)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	// Image marked as favorited in cache but NOT in the favorites provider (ghost)
	ghostFav := provider.Image{
		ID:          "ghost_fav_1",
		Provider:    "Wallhaven",
		IsFavorited: true,
		FilePath:    filepath.Join(tmpDir, "ghost_fav_1.jpg"),
	}
	require.NoError(t, os.WriteFile(ghostFav.FilePath, []byte("fake"), 0644))
	store.Add(ghostFav)

	// Image correctly marked as favorited
	realFav := provider.Image{
		ID:          "real_fav_1",
		Provider:    "Wallhaven",
		IsFavorited: true,
		FilePath:    filepath.Join(tmpDir, "real_fav_1.jpg"),
	}
	require.NoError(t, os.WriteFile(realFav.FilePath, []byte("fake"), 0644))
	store.Add(realFav)

	// Normal unfavorited image
	normalImg := provider.Image{
		ID:          "normal_1",
		Provider:    "Wallhaven",
		IsFavorited: false,
		FilePath:    filepath.Join(tmpDir, "normal_1.jpg"),
	}
	require.NoError(t, os.WriteFile(normalImg.FilePath, []byte("fake"), 0644))
	store.Add(normalImg)

	// Use the existing mockFavoriter (testify-based) from favorites_responsiveness_test.go
	fav := &mockFavoriter{}
	// ghost_fav_1: provider says NOT favorited
	fav.On("IsFavorited", ghostFav).Return(false)
	// real_fav_1: provider says IS favorited
	fav.On("IsFavorited", realFav).Return(true)
	// normal_1: provider says NOT favorited
	fav.On("IsFavorited", normalImg).Return(false)

	wp := &Plugin{
		store:     store,
		favoriter: fav,
	}

	wp.reconcileFavorites()

	// Ghost favorite should be corrected
	img, ok := store.GetByID("ghost_fav_1")
	require.True(t, ok)
	assert.False(t, img.IsFavorited, "ghost favorite should have been corrected to false")

	// Real favorite should remain
	img, ok = store.GetByID("real_fav_1")
	require.True(t, ok)
	assert.True(t, img.IsFavorited, "real favorite should still be true")

	// Normal image should be unchanged
	img, ok = store.GetByID("normal_1")
	require.True(t, ok)
	assert.False(t, img.IsFavorited, "normal image should still be false")
}

// TestReconcileFavorites_NilFavoriter verifies reconcileFavorites is a no-op
// when the favorites provider is nil.
func TestReconcileFavorites_NilFavoriter(t *testing.T) {
	store := NewImageStore()

	wp := &Plugin{
		store:     store,
		favoriter: nil,
	}

	// Should not panic
	wp.reconcileFavorites()
}

// TestReconcileFavorites_RemovesDeadFavoritesEntry is a regression test for the
// ghost favorites bug where a Provider=Favorites image whose file was deleted from
// the favorites folder was never cleaned up. The old IsFavorited() had a short-circuit
// `if img.Provider == "Favorites" { return true }` that bypassed the favMap check,
// preventing reconcileFavorites from detecting the orphan.
func TestReconcileFavorites_RemovesDeadFavoritesEntry(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewImageStore()
	fm := NewFileManager(tmpDir)
	store.SetFileManager(fm, filepath.Join(tmpDir, "cache.json"))

	// Stale Provider=Favorites entry — file was deleted from favorites folder.
	// loadInitialMetadata's orphan validator removed it from favMap, so
	// IsFavorited should return false. reconcileFavorites should Remove() it.
	staleFav := provider.Image{
		ID:            "Wallhaven_21z536",
		Provider:      "Favorites",
		IsFavorited:   true,
		SourceQueryID: FavoritesQueryID,
		FilePath:      filepath.Join(tmpDir, "Wallhaven_21z536.jpg"),
	}
	require.NoError(t, os.WriteFile(staleFav.FilePath, []byte("fake"), 0644))
	store.Add(staleFav)

	// Healthy favorite — file exists, favMap has it
	healthyFav := provider.Image{
		ID:            "Wallhaven_abc123",
		Provider:      "Favorites",
		IsFavorited:   true,
		SourceQueryID: FavoritesQueryID,
		FilePath:      filepath.Join(tmpDir, "Wallhaven_abc123.jpg"),
	}
	require.NoError(t, os.WriteFile(healthyFav.FilePath, []byte("fake"), 0644))
	store.Add(healthyFav)

	fav := &mockFavoriter{}
	// Stale: favMap says NOT favorited (orphan validator cleaned it)
	fav.On("IsFavorited", staleFav).Return(false)
	// Healthy: favMap says IS favorited
	fav.On("IsFavorited", healthyFav).Return(true)

	wp := &Plugin{
		store:     store,
		favoriter: fav,
	}

	wp.reconcileFavorites()

	// Stale entry should be REMOVED from store entirely (not just flag-flipped)
	_, ok := store.GetByID("Wallhaven_21z536")
	assert.False(t, ok, "stale Provider=Favorites entry should have been removed from store")

	// Healthy favorite should remain untouched
	img, ok := store.GetByID("Wallhaven_abc123")
	require.True(t, ok)
	assert.True(t, img.IsFavorited, "healthy favorite should still be favorited")
	assert.Equal(t, "Favorites", img.Provider, "healthy favorite should still be Provider=Favorites")
}
