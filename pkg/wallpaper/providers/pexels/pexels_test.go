package pexels

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/stretchr/testify/assert"
)

// mockTransport allows mocking HTTP responses (Duplicated for availability)
type mockPexelsTransport struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockPexelsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

func TestPexelsProvider_ParseURL(t *testing.T) {
	provider := &PexelsProvider{}

	tests := []struct {
		name    string
		webURL  string
		want    string
		wantErr bool
	}{
		{
			name:   "Simple Search",
			webURL: "https://www.pexels.com/search/nature/",
			want:   "https://api.pexels.com/v1/search?query=nature",
		},
		{
			name:   "Search with Orientation Filter",
			webURL: "https://www.pexels.com/search/nature/?orientation=portrait",
			want:   "https://api.pexels.com/v1/search?orientation=portrait&query=nature",
		},
		{
			name:   "Search with Multiple Filters",
			webURL: "https://www.pexels.com/search/abstract/?orientation=landscape&color=red&size=large",
			// Query params order can vary, but url.Encode usually sorts by key.
			// color=red, orientation=landscape, query=abstract, size=large
			want: "https://api.pexels.com/v1/search?color=red&orientation=landscape&query=abstract&size=large",
		},
		{
			name:   "Collection URL",
			webURL: "https://www.pexels.com/collections/cool-wallpapers-123456/",
			want:   "https://api.pexels.com/v1/collections/123456",
		},
		{
			name:   "Collection URL with odd slug",
			webURL: "https://www.pexels.com/collections/my-super-collection-987654321/",
			want:   "https://api.pexels.com/v1/collections/987654321",
		},
		{
			name:    "Invalid Domain",
			webURL:  "https://www.google.com/search/nature",
			wantErr: true,
		},
		{
			name:    "Unsupported Pexels URL",
			webURL:  "https://www.pexels.com/license/",
			wantErr: true,
		},
		{
			name:   "Search with Encoded Characters",
			webURL: "https://www.pexels.com/search/wirehair%20dachshund/",
			want:   "https://api.pexels.com/v1/search?query=wirehair%20dachshund",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := provider.ParseURL(tt.webURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("PexelsProvider.ParseURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Parse resulting URL to compare query params robustly
				uGot, _ := url.Parse(got)
				uWant, _ := url.Parse(tt.want)

				if uGot.Scheme != uWant.Scheme || uGot.Host != uWant.Host || uGot.Path != uWant.Path {
					t.Errorf("PexelsProvider.ParseURL() = %v, want %v", got, tt.want)
				}

				qGot := uGot.Query()
				qWant := uWant.Query()

				if len(qGot) != len(qWant) {
					t.Errorf("Query params mismatch: got %v, want %v", qGot, qWant)
				}
				for k, v := range qWant {
					if qGot.Get(k) != v[0] {
						t.Errorf("Query param %s mismatch: got %s, want %s", k, qGot.Get(k), v[0])
					}
				}
			}
		})
	}
}

func TestPexelsProvider_FetchImages(t *testing.T) {
	// Mock Search Response
	mockSearchJSON := `{
		"page": 1,
		"per_page": 15,
		"photos": [
			{
				"id": 2014422,
				"width": 3024,
				"height": 3024,
				"url": "https://www.pexels.com/photo/brown-rocks-during-golden-hour-2014422/",
				"photographer": "Pok Rie",
				"src": {
					"original": "https://images.pexels.com/photos/2014422/pexels-photo-2014422.jpeg",
					"large2x": "https://images.pexels.com/photos/2014422/pexels-photo-2014422.jpeg?auto=compress&cs=tinysrgb&dpr=2&h=650&w=940"
				}
			}
		],
		"total_results": 8000
	}`

	// Mock Collection Response
	mockCollectionJSON := `{
		"id": "12345",
		"media": [
			{
				"type": "Photo",
				"id": 3573351,
				"width": 3024,
				"height": 4032,
				"url": "https://www.pexels.com/photo/trees-during-day-3573351/",
				"photographer": "Lukas Rodriguez",
				"src": {
					"original": "https://images.pexels.com/photos/3573351/pexels-photo-3573351.jpeg",
					"large2x": "https://images.pexels.com/photos/3573351/pexels-photo-3573351.jpeg"
				}
			}
		]
	}`

	tests := []struct {
		name         string
		apiURL       string
		mockResponse string
		mockStatus   int
		wantErr      bool
		wantCount    int
	}{
		{
			name:         "Success Search",
			apiURL:       "https://api.pexels.com/v1/search?query=nature",
			mockResponse: mockSearchJSON,
			mockStatus:   http.StatusOK,
			wantCount:    1,
		},
		{
			name:         "Success Collection",
			apiURL:       "https://api.pexels.com/v1/collections/12345",
			mockResponse: mockCollectionJSON,
			mockStatus:   http.StatusOK,
			wantCount:    1,
		},
		{
			name:         "API Error 500",
			apiURL:       "https://api.pexels.com/v1/search?query=fail",
			mockResponse: `{"error": "Server Error"}`,
			mockStatus:   http.StatusInternalServerError,
			wantErr:      true,
		},
		{
			name:         "Empty Response",
			apiURL:       "https://api.pexels.com/v1/search?query=empty",
			mockResponse: `{"photos": [], "total_results": 0}`,
			mockStatus:   http.StatusOK,
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify Auth Header
				assert.Equal(t, "test-api-key", r.Header.Get("Authorization"))

				// Verify Pagination
				query := r.URL.Query()
				assert.Equal(t, "1", query.Get("page"))
				assert.Equal(t, "30", query.Get("per_page"))

				w.WriteHeader(tt.mockStatus)
				_, _ = w.Write([]byte(tt.mockResponse))
			}))
			defer ts.Close()

			client := &http.Client{
				Transport: &mockPexelsTransport{
					RoundTripFunc: func(req *http.Request) (*http.Response, error) {
						u, _ := req.URL.Parse(ts.URL)
						req.URL.Scheme = u.Scheme
						req.URL.Host = u.Host
						return http.DefaultTransport.RoundTrip(req)
					},
				},
			}

			// Mock Config - pass nil as we are bypassing it via SetTokenForTesting.
			provider := NewPexelsProvider(nil, client)
			provider.SetTokenForTesting("test-api-key")

			images, err := provider.FetchImages(context.Background(), tt.apiURL, 1)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCount, len(images))
			}
		})
	}
}

func TestPexelsProvider_FetchImages_NoAuth(t *testing.T) {
	client := &http.Client{}

	// Use wallpaper.Config
	cfg := &wallpaper.Config{}

	provider := NewPexelsProvider(cfg, client)

	_, err := provider.FetchImages(context.Background(), "https://api.pexels.com/v1/search", 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}
