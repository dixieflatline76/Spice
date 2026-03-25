package localfolder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider(t *testing.T) {
	cfg := &wallpaper.Config{}
	p := NewProvider(cfg)
	assert.NotNil(t, p)
	assert.Equal(t, "LocalFolder", p.ID())
	assert.Equal(t, "Local Folders", p.Title())
	assert.Equal(t, provider.TypeLocal, p.Type())
	assert.True(t, p.SupportsUserQueries())
}

func TestParseURL_ValidDirectory(t *testing.T) {
	tempDir := t.TempDir()
	p := NewProvider(&wallpaper.Config{})

	result, err := p.ParseURL(tempDir)
	assert.NoError(t, err)
	assert.Equal(t, tempDir, result)
}

func TestParseURL_InvalidPath(t *testing.T) {
	p := NewProvider(&wallpaper.Config{})

	_, err := p.ParseURL("/nonexistent/path/that/does/not/exist")
	assert.Error(t, err)
}

func TestParseURL_NotADirectory(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "not_a_dir.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("hello"), 0644))

	p := NewProvider(&wallpaper.Config{})

	_, err := p.ParseURL(filePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

func TestHashFolderPath(t *testing.T) {
	// Same path should always produce same hash
	hash1 := wallpaper.HashFolderPath("/home/user/photos")
	hash2 := wallpaper.HashFolderPath("/home/user/photos")
	assert.Equal(t, hash1, hash2)

	// Different paths produce different hashes
	hash3 := wallpaper.HashFolderPath("/home/user/other")
	assert.NotEqual(t, hash1, hash3)

	// Hash should be 16 hex chars (8 bytes)
	assert.Len(t, hash1, 16)
}

func TestHasLocalImages(t *testing.T) {
	tempDir := t.TempDir()

	// Empty directory
	assert.False(t, hasLocalImages(tempDir))

	// Add a non-image file
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "readme.txt"), []byte("text"), 0644))
	assert.False(t, hasLocalImages(tempDir))

	// Add an image file
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "photo.jpg"), []byte("image"), 0644))
	assert.True(t, hasLocalImages(tempDir))
}

func TestFetchImages(t *testing.T) {
	// Create a temp dir with a test image
	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "image1.jpg"), []byte("img"), 0644))

	// Mock local API server
	collectionID := wallpaper.HashFolderPath(tempDir)
	expectPath := fmt.Sprintf("/local/%s/%s/images", wallpaper.LocalFolderNamespace, collectionID)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectPath {
			http.NotFound(w, r)
			return
		}
		resp := []map[string]string{
			{
				"id":          "image1",
				"url":         "http://localhost/local/local_folders/abc/assets/image1.jpg",
				"attribution": "User",
				"product_url": "",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	host := ts.URL[7:] // Strip http://

	p := NewProvider(&wallpaper.Config{})
	p.SetTestConfig(host)

	images, err := p.FetchImages(context.Background(), tempDir, 1)
	assert.NoError(t, err)
	require.Len(t, images, 1)
	assert.Equal(t, fmt.Sprintf("LocalFolder_%s_image1", collectionID), images[0].ID)
	assert.Equal(t, ProviderName, images[0].Provider)
}

func TestFetchImages_EmptyFolder(t *testing.T) {
	tempDir := t.TempDir()

	p := NewProvider(&wallpaper.Config{})
	p.SetTestConfig("localhost:0")

	images, err := p.FetchImages(context.Background(), tempDir, 1)
	assert.NoError(t, err)
	assert.Empty(t, images)
}

func TestEnrichImage(t *testing.T) {
	p := NewProvider(&wallpaper.Config{})
	img := provider.Image{ID: "test"}
	result, err := p.EnrichImage(context.Background(), img)
	assert.NoError(t, err)
	assert.Equal(t, img, result)
}

func TestDynamicNamespaceResolver(t *testing.T) {
	// This tests the ResolveNamespace method directly
	// In production, the resolver is wired via main.go inline closure
	// but we test the localfolder's own resolver for correctness

	p := NewProvider(&wallpaper.Config{})

	// Wrong namespace
	_, ok := p.ResolveNamespace("wrong_namespace", "abc123")
	assert.False(t, ok)

	// Correct namespace but no matching queries (empty config)
	_, ok = p.ResolveNamespace(wallpaper.LocalFolderNamespace, "abc123")
	assert.False(t, ok)
}
