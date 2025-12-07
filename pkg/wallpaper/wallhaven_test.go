package wallpaper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWallhavenProvider_EnrichImage(t *testing.T) {
	// Setup mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check URL
		if r.URL.Path != "/api/v1/w/test_id" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"uploader": {
					"username": "TestUser"
				}
			}
		}`))
	}))
	defer ts.Close()

	// Create provider with mock client
	// We need to override the base URL or intercept the request.
	// Since the code uses absolute URL "https://wallhaven.cc/api/v1/w/%s", we can't easily swap the base URL
	// without refactoring or using a custom transport.
	// However, we can use a custom Transport in the http.Client to redirect requests to our mock server.

	client := &http.Client{
		Transport: &mockTransport{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				// Redirect to mock server
				u, _ := req.URL.Parse(ts.URL)
				req.URL.Scheme = u.Scheme
				req.URL.Host = u.Host
				return http.DefaultTransport.RoundTrip(req)
			},
		},
	}

	provider := &WallhavenProvider{
		httpClient: client,
		cfg:        &Config{},
	}

	// Test case
	img := Image{ID: "test_id", Attribution: ""}
	enriched, err := provider.EnrichImage(context.Background(), img)

	assert.NoError(t, err)
	assert.Equal(t, "TestUser", enriched.Attribution)

	// Test case: Already has attribution
	img2 := Image{ID: "test_id_2", Attribution: "Existing"}
	enriched2, err := provider.EnrichImage(context.Background(), img2)
	assert.NoError(t, err)
	assert.Equal(t, "Existing", enriched2.Attribution)

	// Test case: API Error (Non-200)
	tsError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tsError.Close()

	clientError := &http.Client{
		Transport: &mockTransport{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				u, _ := req.URL.Parse(tsError.URL)
				req.URL.Scheme = u.Scheme
				req.URL.Host = u.Host
				return http.DefaultTransport.RoundTrip(req)
			},
		},
	}
	providerError := &WallhavenProvider{httpClient: clientError, cfg: &Config{}}

	img3 := Image{ID: "error_id", Attribution: ""}
	enriched3, err := providerError.EnrichImage(context.Background(), img3)
	assert.NoError(t, err) // Should not error, just return original
	assert.Equal(t, "", enriched3.Attribution)

	// Test case: Malformed JSON
	tsBadJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid_json`))
	}))
	defer tsBadJSON.Close()

	clientBadJSON := &http.Client{
		Transport: &mockTransport{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				u, _ := req.URL.Parse(tsBadJSON.URL)
				req.URL.Scheme = u.Scheme
				req.URL.Host = u.Host
				return http.DefaultTransport.RoundTrip(req)
			},
		},
	}
	providerBadJSON := &WallhavenProvider{httpClient: clientBadJSON, cfg: &Config{}}

	img4 := Image{ID: "bad_json_id", Attribution: ""}
	enriched4, err := providerBadJSON.EnrichImage(context.Background(), img4)
	assert.Error(t, err) // Should error on decode failure
	assert.Contains(t, err.Error(), "failed to decode")
	assert.Equal(t, "", enriched4.Attribution)
}

// mockTransport allows mocking HTTP responses
type mockTransport struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}
