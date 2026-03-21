package favorites

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider(t *testing.T) {
	cfg := &wallpaper.Config{}
	p := NewProvider(cfg)
	assert.NotNil(t, p)
	assert.Equal(t, ProviderName, p.Name())
	assert.Equal(t, "Favorites", p.Title())
}

func TestParseURL(t *testing.T) {
	p := NewProvider(&wallpaper.Config{})

	url, err := p.ParseURL(wallpaper.FavoritesQueryID)
	assert.NoError(t, err)
	assert.Equal(t, wallpaper.FavoritesQueryID, url)

	_, err = p.ParseURL("http://google.com")
	assert.Error(t, err)
}

func TestAddAndRemoveFavorite(t *testing.T) {
	// Setup temporary directories
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "favorites_root")
	sourceDir := filepath.Join(tempDir, "source")
	err := os.MkdirAll(sourceDir, 0755)
	require.NoError(t, err)

	// Create a dummy source image
	sourceImgPath := filepath.Join(sourceDir, "test_image.jpg")
	err = os.WriteFile(sourceImgPath, []byte("fake image content"), 0644)
	require.NoError(t, err)

	p := NewProvider(&wallpaper.Config{})
	p.SetTestConfig("localhost:0", rootDir) // Host irrelevant for this test

	img := provider.Image{
		ID:          "test_image",
		FilePath:    sourceImgPath,
		Attribution: "Test Author",
		ViewURL:     "http://example.com/view",
		Provider:    "Unsplash",
	}

	// 1. Test AddFavorite
	err = p.AddFavorite(img)
	assert.NoError(t, err)

	// Verify file exists in rootDir (Eventually, as it is async)
	destPath := filepath.Join(rootDir, "test_image.jpg")
	assert.Eventually(t, func() bool {
		_, err := os.Stat(destPath)
		return err == nil
	}, time.Second*2, time.Millisecond*50)

	// Verify content matches
	content, _ := os.ReadFile(destPath)
	assert.Equal(t, "fake image content", string(content))

	// Verify metadata
	metaPath := filepath.Join(rootDir, "metadata.json")
	assert.Eventually(t, func() bool {
		_, err := os.Stat(metaPath)
		return err == nil
	}, time.Second*2, time.Millisecond*50)

	// 2. Test IsFavorited
	assert.True(t, p.IsFavorited(img))

	nonFav := provider.Image{FilePath: filepath.Join(sourceDir, "other.jpg")}
	assert.False(t, p.IsFavorited(nonFav))

	// 3. Test RemoveFavorite
	err = p.RemoveFavorite(img)
	assert.NoError(t, err)

	assert.Eventually(t, func() bool {
		_, err := os.Stat(destPath)
		return os.IsNotExist(err)
	}, time.Second*2, time.Millisecond*50)

	assert.False(t, p.IsFavorited(img))
}

func TestHomeURL(t *testing.T) {
	tempDir := t.TempDir()
	p := NewProvider(&wallpaper.Config{})
	p.SetTestConfig("", tempDir)

	url := p.HomeURL()
	assert.NotEmpty(t, url)
	assert.Contains(t, url, "file://")

	// Basic format check
	// Should contain the tempDir path, but sanitized
	normalizedTemp := filepath.ToSlash(tempDir)
	assert.Contains(t, url, normalizedTemp)
}

func TestFetchImages(t *testing.T) {
	// Mock local API server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify expected URL path
		expectPath := fmt.Sprintf("/local/%s/%s/images", wallpaper.FavoritesNamespace, wallpaper.FavoritesCollection)
		if r.URL.Path != expectPath {
			http.NotFound(w, r)
			return
		}

		// Return mock JSON response
		resp := []map[string]string{
			{
				"id":          "img1",
				"url":         "/path/to/img1.jpg",
				"attribution": "Author 1",
				"product_url": "http://link1",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	// Parse host from ts.URL (e.g., http://127.0.0.1:12345)
	host := ts.URL[7:] // Strip http://

	p := NewProvider(&wallpaper.Config{})
	tempDir := t.TempDir()
	p.SetTestConfig(host, tempDir)

	// Create a dummy file to bypass empty-folder optimization
	// The optimization checks for file existence but relies on API for actual data in this test
	dummy := filepath.Join(tempDir, "dummy.jpg")
	_ = os.WriteFile(dummy, []byte(""), 0644)

	images, err := p.FetchImages(context.Background(), "", 1)
	assert.NoError(t, err)
	require.Len(t, images, 1)
	assert.Equal(t, "img1", images[0].ID)
	assert.Equal(t, "/path/to/img1.jpg", images[0].Path)
	assert.Equal(t, "Author 1", images[0].Attribution)
	assert.Equal(t, ProviderName, images[0].Provider)
}

func TestFIFOEviction(t *testing.T) {
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "favorites_root")
	sourceDir := filepath.Join(tempDir, "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))

	p := NewProvider(&wallpaper.Config{})
	p.SetTestConfig("localhost:0", rootDir)
	p.SetTestMaxFavorites(3) // Low limit for fast testing
	defer p.Close()

	// Create and add 3 favorites with staggered times
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("img_%d", i)
		srcPath := filepath.Join(sourceDir, name+".jpg")
		require.NoError(t, os.WriteFile(srcPath, []byte(fmt.Sprintf("content_%d", i)), 0644))

		img := provider.Image{
			ID:       name,
			FilePath: srcPath,
		}
		require.NoError(t, p.AddFavorite(img))

		// Wait for async worker to process
		destPath := filepath.Join(rootDir, name+".jpg")
		assert.Eventually(t, func() bool {
			_, err := os.Stat(destPath)
			return err == nil
		}, 2*time.Second, 50*time.Millisecond, "File %s should exist", name)

		// Stagger modtimes so oldest is deterministic
		time.Sleep(50 * time.Millisecond)
	}

	// Verify all 3 exist
	assert.True(t, p.IsFavorited(provider.Image{ID: "img_0"}))
	assert.True(t, p.IsFavorited(provider.Image{ID: "img_1"}))
	assert.True(t, p.IsFavorited(provider.Image{ID: "img_2"}))

	// Add a 4th — should trigger FIFO eviction of img_0 (oldest)
	srcPath4 := filepath.Join(sourceDir, "img_3.jpg")
	require.NoError(t, os.WriteFile(srcPath4, []byte("content_3"), 0644))

	img4 := provider.Image{ID: "img_3", FilePath: srcPath4}
	require.NoError(t, p.AddFavorite(img4))

	// Wait for the 4th to be written
	assert.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(rootDir, "img_3.jpg"))
		return err == nil
	}, 2*time.Second, 50*time.Millisecond)

	// img_0 should have been evicted
	assert.Eventually(t, func() bool {
		return !p.IsFavorited(provider.Image{ID: "img_0"})
	}, 2*time.Second, 50*time.Millisecond, "img_0 should be evicted from favMap")

	assert.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(rootDir, "img_0.jpg"))
		return os.IsNotExist(err)
	}, 2*time.Second, 50*time.Millisecond, "img_0.jpg should be deleted from disk")

	// Others should still exist
	assert.True(t, p.IsFavorited(provider.Image{ID: "img_1"}))
	assert.True(t, p.IsFavorited(provider.Image{ID: "img_2"}))
	assert.True(t, p.IsFavorited(provider.Image{ID: "img_3"}))
}

func TestOrphanCleanup(t *testing.T) {
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "favorites_root")
	require.NoError(t, os.MkdirAll(rootDir, 0755))

	// Create metadata.json with entries for files that don't exist on disk
	meta := map[string]interface{}{
		"files": map[string]interface{}{
			"real_image.jpg": map[string]string{
				"attribution": "Author A",
				"product_url": "http://example.com/a",
			},
			"orphan_image.jpg": map[string]string{
				"attribution": "Author B",
				"product_url": "http://example.com/b",
			},
			"another_orphan.png": map[string]string{
				"attribution": "Author C",
				"product_url": "http://example.com/c",
			},
		},
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "metadata.json"), metaData, 0644))

	// Only create the "real" file on disk
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "real_image.jpg"), []byte("real"), 0644))

	// Create provider — loadInitialMetadata will run cleanOrphanMetadata
	p := NewProvider(&wallpaper.Config{})
	p.SetTestConfig("localhost:0", rootDir)
	defer p.Close()

	// real_image should be in favMap
	assert.True(t, p.IsFavorited(provider.Image{ID: "real_image"}))

	// orphans should be cleaned from favMap
	assert.False(t, p.IsFavorited(provider.Image{ID: "orphan_image"}))
	assert.False(t, p.IsFavorited(provider.Image{ID: "another_orphan"}))

	// Verify metadata.json was also cleaned
	updatedMeta, err := os.ReadFile(filepath.Join(rootDir, "metadata.json"))
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(updatedMeta, &parsed))

	filesMeta := parsed["files"].(map[string]interface{})
	assert.Contains(t, filesMeta, "real_image.jpg", "real_image.jpg should remain in metadata")
	assert.NotContains(t, filesMeta, "orphan_image.jpg", "orphan_image.jpg should be removed from metadata")
	assert.NotContains(t, filesMeta, "another_orphan.png", "another_orphan.png should be removed from metadata")
}

func TestClearFavorites(t *testing.T) {
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "favorites_root")
	sourceDir := filepath.Join(tempDir, "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0755))

	p := NewProvider(&wallpaper.Config{})
	p.SetTestConfig("localhost:0", rootDir)
	defer p.Close()

	// Add a couple of favorites
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("clear_img_%d", i)
		srcPath := filepath.Join(sourceDir, name+".jpg")
		require.NoError(t, os.WriteFile(srcPath, []byte("data"), 0644))
		require.NoError(t, p.AddFavorite(provider.Image{ID: name, FilePath: srcPath, Attribution: "Author"}))
	}

	// Wait for all to be written
	assert.Eventually(t, func() bool {
		entries, _ := os.ReadDir(rootDir)
		imageCount := 0
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) != ".json" {
				imageCount++
			}
		}
		return imageCount >= 3
	}, 3*time.Second, 50*time.Millisecond, "All 3 files should be written")

	// Now clear everything (extracted logic from CreateSettingsPanel)
	os.RemoveAll(rootDir)
	require.NoError(t, os.MkdirAll(rootDir, 0755))

	p.mu.Lock()
	p.favMap = make(map[string]bool)
	p.mu.Unlock()

	// Verify everything is wiped
	assert.False(t, p.IsFavorited(provider.Image{ID: "clear_img_0"}))
	assert.False(t, p.IsFavorited(provider.Image{ID: "clear_img_1"}))
	assert.False(t, p.IsFavorited(provider.Image{ID: "clear_img_2"}))

	entries, err := os.ReadDir(rootDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "Favorites directory should be empty after clear")
}

func TestWorkerShutdown(t *testing.T) {
	p := NewProvider(&wallpaper.Config{})
	p.SetTestConfig("localhost:0", t.TempDir())

	// Close should not panic and should be idempotent
	p.Close()
	p.Close() // Second call should be safe
}
