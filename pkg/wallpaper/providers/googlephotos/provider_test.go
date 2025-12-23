package googlephotos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider(t *testing.T) {
	cfg := &wallpaper.Config{}
	client := &http.Client{}
	p := NewProvider(cfg, client)
	assert.NotNil(t, p)
	assert.Equal(t, "GooglePhotos", p.Name())
	assert.Equal(t, "Google Photos", p.Title())
}

func TestParseURL(t *testing.T) {
	p := NewProvider(&wallpaper.Config{}, &http.Client{})

	valid := "googlephotos://some-guid-123"
	url, err := p.ParseURL(valid)
	assert.NoError(t, err)
	assert.Equal(t, valid, url)

	_, err = p.ParseURL("http://google.com")
	assert.Error(t, err)

	_, err = p.ParseURL("googlephotos://")
	// URL parse usually succeeds but we might want to check hostname presence if we were strict
	// The current implementation just checks scheme.
	assert.NoError(t, err)
}

func TestFetchImages(t *testing.T) {
	mockGUID := "test-guid-123"

	// Mock Local API
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := fmt.Sprintf("/local/google_photos/%s/images", mockGUID)
		if r.URL.Path != expectedPath {
			http.NotFound(w, r)
			return
		}

		resp := []map[string]string{
			{
				"id":          "img1",
				"url":         "/path/to/img1.jpg",
				"attribution": "Author",
				"product_url": "http://google.com/view",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	host := ts.URL[7:] // Strip http://
	p := NewProvider(&wallpaper.Config{}, ts.Client())
	p.SetTestConfig(host, t.TempDir())

	apiURL := "googlephotos://" + mockGUID
	images, err := p.FetchImages(context.Background(), apiURL, 1)
	assert.NoError(t, err)
	require.Len(t, images, 1)
	assert.Equal(t, "img1", images[0].ID)
	assert.Equal(t, "/path/to/img1.jpg", images[0].Path)
	assert.Equal(t, "Author", images[0].Attribution)
}

func TestMetadataOperations(t *testing.T) {
	rootDir := t.TempDir()
	p := NewProvider(&wallpaper.Config{}, &http.Client{})
	p.SetTestConfig("localhost:0", rootDir)

	guid := "meta-test-guid"
	targetDir := filepath.Join(rootDir, guid)
	err := os.MkdirAll(targetDir, 0755)
	require.NoError(t, err)

	links := map[string]string{
		"file1.jpg": "http://link1",
	}

	// 1. Save Initial Metadata
	err = p.saveInitialMetadata(guid, links)
	assert.NoError(t, err)

	metaPath := filepath.Join(targetDir, "metadata.json")
	assert.FileExists(t, metaPath)

	content, _ := os.ReadFile(metaPath)
	var data map[string]interface{}
	err = json.Unmarshal(content, &data)
	assert.NoError(t, err)
	assert.Equal(t, guid, data["id"])

	// 2. Update Metadata
	newDesc := "Updated Description"
	err = p.updateMetadata(guid, newDesc)
	assert.NoError(t, err)

	content, _ = os.ReadFile(metaPath)
	err = json.Unmarshal(content, &data)
	assert.NoError(t, err)
	assert.Equal(t, newDesc, data["description"])
	assert.Equal(t, guid, data["id"])
}

func TestFetchImages_Error(t *testing.T) {
	mockGUID := "error-guid"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	host := ts.URL[7:]
	p := NewProvider(&wallpaper.Config{}, ts.Client())
	p.SetTestConfig(host, t.TempDir())

	apiURL := "googlephotos://" + mockGUID
	_, err := p.FetchImages(context.Background(), apiURL, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
