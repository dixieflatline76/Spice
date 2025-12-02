package wallpaper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractFilenameFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"http://example.com/image.jpg", "image.jpg"},
		{"http://example.com/path/to/image.png", "image.png"},
		{"image.gif", "image.gif"},
		{"http://example.com/", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractFilenameFromURL(tt.url)
		assert.Equal(t, tt.expected, result, "URL: %s", tt.url)
	}
}

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"image.jpg", true},
		{"image.jpeg", true},
		{"image.png", true},
		{"image.gif", true},
		{"image.txt", false},
		{"image", false},
		{"path/to/image.JPG", false}, // Case sensitivity check (implementation uses filepath.Ext which is OS dependent but usually case sensitive on Linux/Mac, Windows might be case insensitive but Go implementation is simple string check?)
		// Actually filepath.Ext returns extension as is. The implementation checks == ".jpg". So it is case sensitive.
	}

	for _, tt := range tests {
		result := isImageFile(tt.path)
		assert.Equal(t, tt.expected, result, "Path: %s", tt.path)
	}
}

func TestGenerateQueryID(t *testing.T) {
	url1 := "https://wallhaven.cc/search?q=anime"
	url2 := "https://wallhaven.cc/search?q=anime"
	url3 := "https://wallhaven.cc/search?q=landscape"

	id1 := GenerateQueryID(url1)
	id2 := GenerateQueryID(url2)
	id3 := GenerateQueryID(url3)

	assert.Equal(t, id1, id2)
	assert.NotEqual(t, id1, id3)
	assert.NotEmpty(t, id1)
}

func TestCovertWebToAPIURL(t *testing.T) {
	tests := []struct {
		name          string
		inputURL      string
		expectedURL   string
		expectedType  URLType
		expectedError bool
	}{
		{
			name:         "Search URL",
			inputURL:     "https://wallhaven.cc/search?q=anime",
			expectedURL:  "https://wallhaven.cc/api/v1/search?q=anime",
			expectedType: Search,
		},
		{
			name:         "Collection URL",
			inputURL:     "https://wallhaven.cc/user/username/favorites/12345",
			expectedURL:  "https://wallhaven.cc/api/v1/collections/username/12345",
			expectedType: Favorites,
		},
		{
			name:         "API Search URL",
			inputURL:     "https://wallhaven.cc/api/v1/search?q=anime",
			expectedURL:  "https://wallhaven.cc/api/v1/search?q=anime",
			expectedType: Search,
		},
		{
			name:          "Invalid URL",
			inputURL:      "https://google.com",
			expectedError: true,
		},
		{
			name:         "URL with API Key (should be removed)",
			inputURL:     "https://wallhaven.cc/api/v1/search?q=anime&apikey=123",
			expectedURL:  "https://wallhaven.cc/api/v1/search?q=anime",
			expectedType: Search,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, qType, err := CovertWebToAPIURL(tt.inputURL)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURL, url)
				assert.Equal(t, tt.expectedType, qType)
			}
		})
	}
}
