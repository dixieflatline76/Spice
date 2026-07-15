package artic

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestEmbeddedJSON(t *testing.T) {
	var col CuratedList
	err := json.Unmarshal(embeddedJSON, &col)
	assert.NoError(t, err)
	assert.NotEmpty(t, col.Entries, "Embedded JSON should have parsed Collections successfully")
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
		curatedList: CuratedList{
			Entries: []CollectionEntry{
				{
					Key:  "test_collection",
					Name: "Test Collection",
					IDs:  []int{101, 102, 103, 104, 105, 106, 107, 108, 109, 110},
				},
			},
		},
	}

	images, err := p.FetchImages(context.Background(), "test_collection", 1)
	assert.NoError(t, err)
	assert.Len(t, images, 10)

	expectedIDs := []string{"101", "102", "103", "104", "105", "106", "107", "108", "109", "110"}
	actualIDs := make([]string, len(images))
	for i, img := range images {
		actualIDs[i] = img.ID
	}

	assert.Equal(t, expectedIDs, actualIDs, "FetchImages should return images in the exact sequential order of the underlying query, despite network jitter.")
}
