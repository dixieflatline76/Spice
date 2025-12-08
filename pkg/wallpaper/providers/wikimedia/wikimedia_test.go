package wikimedia

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/stretchr/testify/assert"
)

func TestWikimediaParseURL(t *testing.T) {
	wp := &WikimediaProvider{}

	tests := []struct {
		input    string
		expected string
		hasError bool
	}{
		{"Category:Space", "category:Space", false},
		{"Category:Featured_pictures", "category:Featured_pictures", false},
		{"category:lowercase", "category:lowercase", false},
		{"https://commons.wikimedia.org/wiki/Category:Nature", "category:Nature", false},
		{"mountain", "search:mountain", false},
		{"Space Exploration", "search:Space Exploration", false},
		{"http://google.com", "", true}, // Invalid domain
		{"", "", true},                  // Empty
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			res, err := wp.ParseURL(tc.input)
			if tc.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, res)
			}
		})
	}
}

func TestWikimediaWithResolution(t *testing.T) {
	wp := &WikimediaProvider{}

	tests := []struct {
		name     string
		query    string
		w, h     int
		expected string
	}{
		{
			name:     "Search Query",
			query:    "search:Space",
			w:        1920,
			h:        1080,
			expected: "search:Space filew:>1920 fileh:>1080",
		},
		{
			name:     "Category Query (Simple)",
			query:    "category:Nature",
			w:        2560,
			h:        1440,
			expected: "search:incategory:\"Nature\" filew:>2560 fileh:>1440",
		},
		{
			name:     "Category Query (Spaces)",
			query:    "category:Featured pictures",
			w:        3840,
			h:        2160,
			expected: "search:incategory:\"Featured pictures\" filew:>3840 fileh:>2160",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := wp.WithResolution(tc.query, tc.w, tc.h)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestWikimediaFetchImages_Category(t *testing.T) {
	// Mock Server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "categorymembers", r.URL.Query().Get("generator"))
		assert.Equal(t, "Category:Nature", r.URL.Query().Get("gcmtitle"))
		assert.Equal(t, "file", r.URL.Query().Get("gcmtype"))
		assert.Equal(t, "json", r.URL.Query().Get("format"))

		response := `{
			"query": {
				"pages": {
					"123": {
						"pageid": 123,
						"title": "File:Nature.jpg",
						"imageinfo": [{
							"url": "https://upload.wikimedia.org/Nature.jpg",
							"extmetadata": {
								"ObjectName": {"value": "Beautiful Nature"},
								"Artist": {"value": "Photographer X"},
								"LicenseShortName": {"value": "CC-BY-SA"}
							}
						}]
					}
				}
			}
		}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer ts.Close()

	cfg := &wallpaper.Config{}
	client := ts.Client()
	wp := NewWikimediaProvider(cfg, client)
	wp.baseURL = ts.URL // Override Base URL

	images, err := wp.FetchImages(context.Background(), "category:Nature", 1)
	assert.NoError(t, err)
	assert.Len(t, images, 1)
	assert.Equal(t, "123", images[0].ID)
	assert.Equal(t, "https://upload.wikimedia.org/Nature.jpg", images[0].Path)
	assert.Equal(t, "Photographer X (CC-BY-SA)", images[0].Attribution)
}

func TestWikimediaProvider_Structure(t *testing.T) {
	cfg := &wallpaper.Config{}
	client := &http.Client{}
	p := NewWikimediaProvider(cfg, client)

	assert.Equal(t, "Wikimedia", p.Name())
	assert.Equal(t, "Wikimedia Commons", p.Title())
}
