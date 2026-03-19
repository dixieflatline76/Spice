package wikimedia

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
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
		{"https://commons.wikimedia.org/wiki/Commons:Featured_pictures/Astronomy", "page:Commons:Featured_pictures/Astronomy", false},
		{"https://commons.wikimedia.org/wiki/Commons:Featured_pictures/Animals", "page:Commons:Featured_pictures/Animals", false},
		{"https://commons.wikimedia.org/wiki/Commons:Featured_pictures", "page:Commons:Featured_pictures", false},
		{"page:CustomPage", "page:CustomPage", false},
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

func TestWikimediaFetchImages_Gallery(t *testing.T) {
	// Mock Server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Commons:Featured_pictures/Astronomy", r.URL.Query().Get("titles"))
		assert.Equal(t, "images", r.URL.Query().Get("generator"))
		assert.Equal(t, "imageinfo", r.URL.Query().Get("prop"))

		response := `{
			"query": {
				"pages": {
					"456": {
						"pageid": 456,
						"title": "File:Galaxy.jpg",
						"imageinfo": [{
							"url": "https://upload.wikimedia.org/Galaxy.jpg",
							"extmetadata": {
								"ObjectName": {"value": "Andromeda"},
								"Artist": {"value": "Hubble"},
								"LicenseShortName": {"value": "Public Domain"}
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
	wp.baseURL = ts.URL

	images, err := wp.FetchImages(context.Background(), "page:Commons:Featured_pictures/Astronomy", 1)
	assert.NoError(t, err)
	assert.Len(t, images, 1)
	assert.Equal(t, "456", images[0].ID)
	assert.Equal(t, "Hubble (Public Domain)", images[0].Attribution)
}

func TestWikimediaProvider_Structure(t *testing.T) {
	cfg := &wallpaper.Config{}
	client := &http.Client{}
	p := NewWikimediaProvider(cfg, client)

	assert.Equal(t, "Wikimedia", p.Title())
}

func TestCircuitBreaker(t *testing.T) {
	cb := &CircuitBreaker{}

	// Initial State: Closed
	assert.False(t, cb.IsOpen())

	// Trip it for 100ms
	cb.Trip(100 * time.Millisecond)
	assert.True(t, cb.IsOpen())
	assert.Greater(t, cb.GetCooldownTime(), 0*time.Second)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)
	assert.False(t, cb.IsOpen())

	// Reset manually
	cb.Trip(5 * time.Minute)
	assert.True(t, cb.IsOpen())
	cb.Reset()
	assert.False(t, cb.IsOpen())
}

func TestCircuitBreakerRoundTripper_429(t *testing.T) {
	cb := &CircuitBreaker{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(429)
	}))
	defer server.Close()

	tripper := &CircuitBreakerRoundTripper{
		Base: server.Client().Transport,
		CB:   cb,
	}

	// Use context with timeout for safety
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/w/api.php", nil)

	resp, err := tripper.RoundTrip(req)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "rate limited (429)")

	// Check if CB is tripped
	assert.True(t, cb.IsOpen())
}

func TestWikimediaPaginationState(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		var response string
		switch callCount {
		case 1:
			// First call, no continue params. Returns page 1 and tokens for page 2.
			assert.Empty(t, r.URL.Query().Get("continue"))
			response = `{
				"query": {
					"pages": {
						"1": {
							"pageid": 1,
							"title": "File:1.jpg",
							"imageinfo": [{"url": "https://upload.wikimedia.org/1.jpg", "extmetadata": {}}]
						}
					}
				},
				"continue": {
					"continue": "-||",
					"gcmcontinue": "page|12345"
				}
			}`
		case 2:
			// Second call, should have the tokens injected from the cache.
			assert.Equal(t, "-||", r.URL.Query().Get("continue"))
			assert.Equal(t, "page|12345", r.URL.Query().Get("gcmcontinue"))
			response = `{
				"query": {
					"pages": {
						"2": {
							"pageid": 2,
							"title": "File:2.jpg",
							"imageinfo": [{"url": "https://upload.wikimedia.org/2.jpg", "extmetadata": {}}]
						}
					}
				}
			}`
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer ts.Close()

	cfg := &wallpaper.Config{}
	client := ts.Client()
	wp := NewWikimediaProvider(cfg, client)
	wp.baseURL = ts.URL

	// 1. Fetch Page 1
	images1, err := wp.FetchImages(context.Background(), "category:Nature", 1)
	assert.NoError(t, err)
	assert.Len(t, images1, 1)
	assert.Equal(t, "1", images1[0].ID)
	// Assert state was populated tightly mapped to subsequent page 2
	assert.NotNil(t, wp.queryTokens["category:Nature"])
	assert.NotNil(t, wp.queryTokens["category:Nature"][2])
	assert.Equal(t, "-||", wp.queryTokens["category:Nature"][2].Get("continue"))

	// 2. Fetch Page 2
	images2, err := wp.FetchImages(context.Background(), "category:Nature", 2)
	assert.NoError(t, err)
	assert.Len(t, images2, 1)
	assert.Equal(t, "2", images2[0].ID)

	// 3. Assert call count to ensure exactly 2 distinct HTTP hits occurred
	assert.Equal(t, 2, callCount)
}
