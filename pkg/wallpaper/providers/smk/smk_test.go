package smk

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseURL(t *testing.T) {
	p := &Provider{}

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "Any URL",
			url:     "https://www.smk.dk/en/",
			wantErr: true, // we don't support arbitrary URLs yet since everything is curated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.ParseURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWithResolution(t *testing.T) {
	p := &Provider{}

	tests := []struct {
		name     string
		url      string
		width    int
		height   int
		expected string
	}{
		{
			name:     "IIIF Full Region",
			url:      "https://iip.smk.dk/iiif/jp2/KMS1621.tif.jp2/full/!2048,2048/0/default.jpg",
			width:    1920,
			height:   1080,
			expected: "https://iip.smk.dk/iiif/jp2/KMS1621.tif.jp2/full/!1920,1080/0/default.jpg",
		},
		{
			name:     "Non IIIF URL",
			url:      "https://example.com/image.jpg",
			width:    1920,
			height:   1080,
			expected: "https://example.com/image.jpg", // unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.WithResolution(tt.url, tt.width, tt.height)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestEmbeddedJSON(t *testing.T) {
	var col struct {
		Version     string `json:"version"`
		Description string `json:"description"`
		Entries     []struct {
			Key  string   `json:"key"`
			Name string   `json:"name"`
			IDs  []string `json:"ids"`
		} `json:"collections"`
	}
	err := json.Unmarshal(embeddedCollection, &col)
	assert.NoError(t, err)
	assert.NotEmpty(t, col.Entries, "Embedded JSON should have parsed Collections successfully")
}
