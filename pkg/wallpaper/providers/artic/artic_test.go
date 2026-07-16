package artic

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

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

type mockRoundTripper struct{}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add random delay to simulate concurrent jitter
	time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)

	// Extract ID from URL
	// https://api.artic.edu/api/v1/artworks/123?fields=...
	parts := strings.Split(req.URL.Path, "/")
	idStr := strings.Split(parts[len(parts)-1], "?")[0]

	body := fmt.Sprintf(`{
		"data": {
			"id": %s,
			"title": "Mock Artwork %s",
			"image_id": "img-%s"
		}
	}`, idStr, idStr, idStr)

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}, nil
}

func TestFetchImages_SequentialOrder(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	p := &Provider{
		httpClient: &http.Client{
			Transport: &mockRoundTripper{},
		},
		idCache: make(map[string][]int),
	}

	// Use real key from artic.json that has known IDs
	images, err := p.FetchImages(context.Background(), "artic_impressionism", 1)
	assert.NoError(t, err)

	// In artic_impressionism we have 30 IDs, so pageSize 10 will return 10
	assert.Len(t, images, 10)

	// Just check the first few to ensure sequential order is maintained
	expectedPrefix := []string{"28560", "20684", "14655"}
	for i, exp := range expectedPrefix {
		assert.Equal(t, exp, images[i].ID)
	}
}

func TestFetchThumbnails_TDD(t *testing.T) {
	p := &Provider{
		httpClient: &http.Client{
			Transport: &mockRoundTripper{},
		},
	}

	ids := []string{"123", "456"}
	thumbnails, err := p.FetchThumbnails(context.Background(), ids)
	assert.NoError(t, err)

	assert.Len(t, thumbnails, 2)
	assert.Equal(t, "123", thumbnails[0].ID)
	assert.Equal(t, "https://www.artic.edu/iiif/2/img-123/full/!800,800/0/default.jpg", thumbnails[0].URL)

	assert.Equal(t, "456", thumbnails[1].ID)
	assert.Equal(t, "https://www.artic.edu/iiif/2/img-456/full/!800,800/0/default.jpg", thumbnails[1].URL)
}
