package wallhaven

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
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

	proc := &WallhavenProvider{
		httpClient: client,
		cfg:        &wallpaper.Config{Preferences: NewMockPreferences()},
	}

	// Test case
	img := provider.Image{ID: "test_id", Attribution: ""}
	enriched, err := proc.EnrichImage(context.Background(), img)

	assert.NoError(t, err)
	assert.Equal(t, "TestUser", enriched.Attribution)

	// Test case: Already has attribution
	img2 := provider.Image{ID: "test_id_2", Attribution: "Existing"}
	enriched2, err := proc.EnrichImage(context.Background(), img2)
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
	providerError := &WallhavenProvider{httpClient: clientError, cfg: &wallpaper.Config{Preferences: NewMockPreferences()}}

	img3 := provider.Image{ID: "error_id", Attribution: ""}
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
	providerBadJSON := &WallhavenProvider{httpClient: clientBadJSON, cfg: &wallpaper.Config{Preferences: NewMockPreferences()}}

	img4 := provider.Image{ID: "bad_json_id", Attribution: ""}
	enriched4, err := providerBadJSON.EnrichImage(context.Background(), img4)
	assert.Error(t, err) // Should error on decode failure
	assert.Contains(t, err.Error(), "failed to decode")
	assert.Equal(t, "", enriched4.Attribution)
}

func TestWallhavenProvider_WithResolution(t *testing.T) {
	p := &WallhavenProvider{}
	width, height := 1920, 1080

	tests := []struct {
		name     string
		inputURL string
		expected string
	}{
		{
			name:     "No existing resolution params",
			inputURL: "https://wallhaven.cc/api/v1/search?q=anime",
			expected: "https://wallhaven.cc/api/v1/search?atleast=1920x1080&q=anime",
		},
		{
			name:     "Existing atleast param",
			inputURL: "https://wallhaven.cc/api/v1/search?q=anime&atleast=2560x1440",
			expected: "https://wallhaven.cc/api/v1/search?q=anime&atleast=2560x1440",
		},
		{
			name:     "Existing resolutions param",
			inputURL: "https://wallhaven.cc/api/v1/search?q=anime&resolutions=1920x1080",
			expected: "https://wallhaven.cc/api/v1/search?q=anime&resolutions=1920x1080",
		},
		{
			name:     "Existing ratios param",
			inputURL: "https://wallhaven.cc/api/v1/search?q=anime&ratios=16x9",
			expected: "https://wallhaven.cc/api/v1/search?q=anime&ratios=16x9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.WithResolution(tt.inputURL, width, height)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestWallhavenProvider_ParseURL(t *testing.T) {
	p := &WallhavenProvider{}

	tests := []struct {
		name      string
		input     string
		want      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:  "User Favorites URL",
			input: "https://wallhaven.cc/user/TestUser/favorites/123",
			want:  "https://wallhaven.cc/api/v1/collections/TestUser/123",
		},
		{
			name:  "Search URL Simple",
			input: "https://wallhaven.cc/search",
			want:  "https://wallhaven.cc/api/v1/search",
		},
		{
			name:  "Search URL With Query",
			input: "https://wallhaven.cc/search?q=anime",
			want:  "https://wallhaven.cc/api/v1/search?q=anime",
		},
		{
			name:  "API Collection URL (Pass-through)",
			input: "https://wallhaven.cc/api/v1/collections/TestUser/123",
			want:  "https://wallhaven.cc/api/v1/collections/TestUser/123",
		},
		{
			name:  "API Search URL (Pass-through)",
			input: "https://wallhaven.cc/api/v1/search?q=nature",
			want:  "https://wallhaven.cc/api/v1/search?q=nature",
		},
		{
			name:  "URL with API Key (Should be stripped)",
			input: "https://wallhaven.cc/api/v1/search?q=cats&apikey=secret123",
			want:  "https://wallhaven.cc/api/v1/search?q=cats",
		},
		{
			name:  "URL with Page (Should be stripped)",
			input: "https://wallhaven.cc/api/v1/search?q=cats&page=2",
			want:  "https://wallhaven.cc/api/v1/search?q=cats",
		},
		{
			name:      "Unsupported URL",
			input:     "https://google.com",
			wantErr:   true,
			errSubstr: "not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.ParseURL(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestWallhavenProvider_FetchImages(t *testing.T) {
	// Setup mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Basic validation of injected params
		query := r.URL.Query()
		if query.Get("apikey") != "test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if query.Get("page") != "1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"id": "img1",
					"path": "https://w.wallhaven.cc/full/img1.jpg",
					"short_url": "https://wh.cc/img1",
					"file_type": "image/jpeg",
					"uploader": { "username": "User1" }
				},
				{
					"id": "img2",
					"path": "https://w.wallhaven.cc/full/img2.png",
					"short_url": "https://wh.cc/img2",
					"file_type": "image/png",
					"uploader": { "username": "User2" }
				}
			]
		}`))
	}))
	defer ts.Close()

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

	// Create Provider
	p := NewWallhavenProvider(&wallpaper.Config{}, client)
	p.SetAPIKeyForTesting("test-api-key")

	// Test Success
	images, err := p.FetchImages(context.Background(), "https://wallhaven.cc/api/v1/search", 1)
	assert.NoError(t, err)
	assert.Len(t, images, 2)
	assert.Equal(t, "img1", images[0].ID)
	assert.Equal(t, "User1", images[0].Attribution)

	// Test Error Case: Bad API Key (401)
	pBad := NewWallhavenProvider(&wallpaper.Config{}, client)
	pBad.SetAPIKeyForTesting("wrong-key")
	_, err = pBad.FetchImages(context.Background(), "https://wallhaven.cc/api/v1/search", 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status: 401")
}

// mockTransport allows mocking HTTP responses
type mockTransport struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

// MockPreferences implements fyne.Preferences for testing
type MockPreferences struct {
	strings map[string]string
	ints    map[string]int
	bools   map[string]bool
	floats  map[string]float64
}

func NewMockPreferences() fyne.Preferences {
	return &MockPreferences{
		strings: make(map[string]string),
		ints:    make(map[string]int),
		bools:   make(map[string]bool),
		floats:  make(map[string]float64),
	}
}

func (m *MockPreferences) BoolList(key string) []bool                              { return nil }
func (m *MockPreferences) BoolListWithFallback(key string, fallback []bool) []bool { return fallback }
func (m *MockPreferences) SetBoolList(key string, value []bool)                    {}
func (m *MockPreferences) FloatList(key string) []float64                          { return nil }
func (m *MockPreferences) FloatListWithFallback(key string, fallback []float64) []float64 {
	return fallback
}
func (m *MockPreferences) SetFloatList(key string, value []float64)             {}
func (m *MockPreferences) IntList(key string) []int                             { return nil }
func (m *MockPreferences) IntListWithFallback(key string, fallback []int) []int { return fallback }
func (m *MockPreferences) SetIntList(key string, value []int)                   {}
func (m *MockPreferences) StringList(key string) []string                       { return nil }
func (m *MockPreferences) StringListWithFallback(key string, fallback []string) []string {
	return fallback
}
func (m *MockPreferences) SetStringList(key string, value []string) {}
func (m *MockPreferences) Bool(key string) bool                     { return m.bools[key] }
func (m *MockPreferences) BoolWithFallback(key string, fallback bool) bool {
	if val, ok := m.bools[key]; ok {
		return val
	}
	return fallback
}
func (m *MockPreferences) SetBool(key string, value bool) { m.bools[key] = value }
func (m *MockPreferences) Float(key string) float64       { return m.floats[key] }
func (m *MockPreferences) FloatWithFallback(key string, fallback float64) float64 {
	if val, ok := m.floats[key]; ok {
		return val
	}
	return fallback
}
func (m *MockPreferences) SetFloat(key string, value float64) { m.floats[key] = value }
func (m *MockPreferences) Int(key string) int                 { return m.ints[key] }
func (m *MockPreferences) IntWithFallback(key string, fallback int) int {
	if val, ok := m.ints[key]; ok {
		return val
	}
	return fallback
}
func (m *MockPreferences) SetInt(key string, value int) { m.ints[key] = value }
func (m *MockPreferences) String(key string) string     { return m.strings[key] }
func (m *MockPreferences) StringWithFallback(key string, fallback string) string {
	if val, ok := m.strings[key]; ok {
		return val
	}
	return fallback
}
func (m *MockPreferences) SetString(key string, value string) { m.strings[key] = value }
func (m *MockPreferences) RemoveValue(key string) {
	delete(m.strings, key)
	delete(m.ints, key)
	delete(m.bools, key)
	delete(m.floats, key)
}
func (m *MockPreferences) AddChangeListener(callback func()) {}
func (m *MockPreferences) ChangeListeners() []func()         { return nil }
