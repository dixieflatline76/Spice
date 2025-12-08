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
