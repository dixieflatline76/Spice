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
