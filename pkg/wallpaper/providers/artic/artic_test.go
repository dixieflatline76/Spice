package artic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseURL(t *testing.T) {
	p := &Provider{} // We'll implement this soon

	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "Direct Artwork URL",
			url:  "https://www.artic.edu/artworks/27992/a-sunday-on-la-grande-jatte-1884",
			want: "object:27992",
		},
		{
			name: "Direct Artwork URL Simple",
			url:  "https://www.artic.edu/artworks/111628",
			want: "object:111628",
		},
		{
			name:    "Invalid Domain",
			url:     "https://example.com/artworks/27992",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ParseURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestGetIIIFURL(t *testing.T) {
	// This tests the dynamic aspect ratio URL construction
	imageID := "e9667990-97ee-173f-29f9-55d9aff5d918"

	// We want a high-res landscape URL
	got := getIIIFURL(imageID, 1920, 1080)

	// Expected format using the ! notation for "fit within"
	expected := "https://www.artic.edu/iiif/2/e9667990-97ee-173f-29f9-55d9aff5d918/full/!1920,1080/0/default.jpg"
	assert.Equal(t, expected, got)
}
func TestIsLandscape(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		want   bool
	}{
		{"Perfect Landscape 16:9", 1920, 1080, true},
		{"Square", 1000, 1000, false},
		{"Portrait", 1000, 1500, false},
		{"Near Square (1.05)", 1050, 1000, false},
		{"Edge Case Landscape (1.1)", 1100, 1000, true},
		{"Zero Dims", 0, 0, false},
		{"Negative Dims", -1, 100, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isLandscape(tt.width, tt.height))
		})
	}
}
