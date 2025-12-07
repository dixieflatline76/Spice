package wallpaper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWallhavenProvider_WithResolution(t *testing.T) {
	p := &WallhavenProvider{}
	width, height := 1920, 1080

	tests := []struct {
		name     string
		inputURL string
		expected string
	}{
		{
			name:     "No existing resolution params",
			inputURL: "https://wallhaven.cc/api/v1/search?q=anime",
			expected: "https://wallhaven.cc/api/v1/search?atleast=1920x1080&q=anime",
		},
		{
			name:     "Existing atleast param",
			inputURL: "https://wallhaven.cc/api/v1/search?q=anime&atleast=2560x1440",
			expected: "https://wallhaven.cc/api/v1/search?q=anime&atleast=2560x1440",
		},
		{
			name:     "Existing resolutions param",
			inputURL: "https://wallhaven.cc/api/v1/search?q=anime&resolutions=1920x1080",
			expected: "https://wallhaven.cc/api/v1/search?q=anime&resolutions=1920x1080",
		},
		{
			name:     "Existing ratios param",
			inputURL: "https://wallhaven.cc/api/v1/search?q=anime&ratios=16x9",
			expected: "https://wallhaven.cc/api/v1/search?q=anime&ratios=16x9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.WithResolution(tt.inputURL, width, height)
			assert.Equal(t, tt.expected, got)
		})
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
