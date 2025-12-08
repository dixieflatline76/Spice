package unsplash

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/stretchr/testify/assert"
)

func TestUnsplashProvider_EnrichImage(t *testing.T) {
	proc := &UnsplashProvider{}
	img := provider.Image{ID: "test", Attribution: "Original"}

	enriched, err := proc.EnrichImage(context.Background(), img)

	assert.NoError(t, err)
	assert.Equal(t, "Original", enriched.Attribution)
	assert.Equal(t, img, enriched)
}

func TestUnsplashProvider_ParseURL(t *testing.T) {
	proc := &UnsplashProvider{}

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "Search URL",
			input:    "https://unsplash.com/s/photos/nature",
			expected: "https://api.unsplash.com/search/photos?query=nature",
			wantErr:  false,
		},
		{
			name:     "Search URL with Encoded Characters",
			input:    "https://unsplash.com/s/photos/nature%20forest",
			expected: "https://api.unsplash.com/search/photos?query=nature+forest",
			wantErr:  false,
		},
		{
			name:     "Collection URL",
			input:    "https://unsplash.com/collections/123456/my-collection",
			expected: "https://api.unsplash.com/collections/123456/photos",
			wantErr:  false,
		},
		{
			name:     "Invalid URL",
			input:    "https://example.com",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "Empty URL",
			input:    "",
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := proc.ParseURL(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestUnsplashProvider_FetchImages(t *testing.T) {
	// Mock server response
	mockResponse := `{
		"results": [
			{
				"id": "img1",
				"urls": {
					"full": "https://images.unsplash.com/photo-1",
					"regular": "https://images.unsplash.com/photo-1-reg"
				},
				"links": {
					"html": "https://unsplash.com/photos/img1",
					"download_location": "https://api.unsplash.com/photos/img1/download"
				},
				"user": {
					"name": "John Doe",
					"username": "johndoe"
				}
			}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Received Authorization header: %s", r.Header.Get("Authorization"))

		// Verify Pagination
		query := r.URL.Query()
		assert.Equal(t, "1", query.Get("page"))
		assert.Equal(t, "30", query.Get("per_page"))

		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(mockResponse)); err != nil {
			t.Errorf("Failed to write mock response: %v", err)
		}
	}))
	defer server.Close()

	cfg := &wallpaper.Config{}

	proc := NewUnsplashProvider(cfg, server.Client())
	proc.SetTokenForTesting("test-token")

	// Use a URL that triggers search response parsing
	testURL := server.URL + "/search/photos"
	ctx := context.Background()
	images, err := proc.FetchImages(ctx, testURL, 1)

	assert.NoError(t, err)
	if assert.Len(t, images, 1) {
		assert.Equal(t, "img1", images[0].ID)
		assert.Equal(t, "https://images.unsplash.com/photo-1", images[0].Path)
		assert.Equal(t, "https://unsplash.com/photos/img1", images[0].ViewURL)
		assert.Equal(t, "John Doe", images[0].Attribution)
		assert.Equal(t, "Unsplash", images[0].Provider)
		assert.Equal(t, "https://api.unsplash.com/photos/img1/download", images[0].DownloadLocation)
	}
}

func TestUnsplashProvider_WithResolution(t *testing.T) {
	p := &UnsplashProvider{}

	tests := []struct {
		name     string
		inputURL string
		width    int
		height   int
		expected string
	}{
		{
			name:     "Search URL - Landscape Screen",
			inputURL: "https://api.unsplash.com/search/photos?query=nature",
			width:    1920,
			height:   1080,
			expected: "https://api.unsplash.com/search/photos?orientation=landscape&query=nature",
		},
		{
			name:     "Search URL - Portrait Screen",
			inputURL: "https://api.unsplash.com/search/photos?query=nature",
			width:    1080,
			height:   1920,
			expected: "https://api.unsplash.com/search/photos?orientation=portrait&query=nature",
		},
		{
			name:     "Search URL - Existing Orientation",
			inputURL: "https://api.unsplash.com/search/photos?query=nature&orientation=squarish",
			width:    1920,
			height:   1080,
			expected: "https://api.unsplash.com/search/photos?query=nature&orientation=squarish",
		},
		{
			name:     "Collection URL (No change expected)",
			inputURL: "https://api.unsplash.com/collections/123/photos",
			width:    1920,
			height:   1080,
			expected: "https://api.unsplash.com/collections/123/photos",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.WithResolution(tt.inputURL, tt.width, tt.height)
			assert.Equal(t, tt.expected, got)
		})
	}
}
