package metmuseum

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/dixieflatline76/Spice/pkg/wallpaper"
)

// MockPreferences implements fyne.Preferences for testing
type MockPreferences struct {
	store map[string]interface{}
}

func NewMockPreferences() *MockPreferences {
	return &MockPreferences{store: make(map[string]interface{})}
}

func (m *MockPreferences) Bool(key string) bool { return m.BoolWithFallback(key, false) }
func (m *MockPreferences) BoolWithFallback(key string, fallback bool) bool {
	if v, ok := m.store[key]; ok {
		return v.(bool)
	}
	return fallback
}
func (m *MockPreferences) SetBool(key string, value bool) { m.store[key] = value }

func (m *MockPreferences) Float(key string) float64                               { return 0.0 }
func (m *MockPreferences) FloatWithFallback(key string, fallback float64) float64 { return fallback }
func (m *MockPreferences) SetFloat(key string, value float64)                     {}

func (m *MockPreferences) Int(key string) int                           { return 0 }
func (m *MockPreferences) IntWithFallback(key string, fallback int) int { return fallback }
func (m *MockPreferences) SetInt(key string, value int)                 {}

func (m *MockPreferences) String(key string) string                              { return "" }
func (m *MockPreferences) StringWithFallback(key string, fallback string) string { return fallback }
func (m *MockPreferences) SetString(key string, value string)                    {}

func (m *MockPreferences) RemoveValue(key string)    {}
func (m *MockPreferences) AddChangeListener(func())  {}
func (m *MockPreferences) ChangeListeners() []func() { return nil } // Added missing method

// List methods required by interface
func (m *MockPreferences) BoolList(key string) []bool                              { return nil }
func (m *MockPreferences) BoolListWithFallback(key string, fallback []bool) []bool { return fallback }
func (m *MockPreferences) SetBoolList(key string, value []bool)                    {}

func (m *MockPreferences) FloatList(key string) []float64 { return nil }
func (m *MockPreferences) FloatListWithFallback(key string, fallback []float64) []float64 {
	return fallback
}
func (m *MockPreferences) SetFloatList(key string, value []float64) {}

func (m *MockPreferences) IntList(key string) []int                             { return nil }
func (m *MockPreferences) IntListWithFallback(key string, fallback []int) []int { return fallback }
func (m *MockPreferences) SetIntList(key string, value []int)                   {}

func (m *MockPreferences) StringList(key string) []string { return nil }
func (m *MockPreferences) StringListWithFallback(key string, fallback []string) []string {
	return fallback
}
func (m *MockPreferences) SetStringList(key string, value []string) {}

func (m *MockPreferences) ResultList() []string          { return nil } // Deprecated/Legacy
func (m *MockPreferences) QueryList(key string) []string { return nil } // Guessing signature?
// Note: Depending on Fyne version, other methods might exist.
// Ideally usage of "testing.NewPreferences()" from Fyne is better, but visibility issues exist.
// Let's rely on standard map.

func TestSpiceMelangeCollection(t *testing.T) {
	// 1. Setup Mock Config & Client
	cfg := &wallpaper.Config{
		Preferences: NewMockPreferences(),
	}
	client := &http.Client{Timeout: 5 * time.Second}

	// 2. Initialize Provider
	p := NewMetMuseumProvider(cfg, client)

	// Wait for async init to complete/fallback (embedded should load fast)
	time.Sleep(500 * time.Millisecond)

	// 3. Fetch Images (Spice Melange)
	ctx := context.Background()
	images, err := p.FetchImages(ctx, CollectionSpiceMelange, 1) // Page 1
	if err != nil {
		t.Fatalf("FetchImages failed: %v", err)
	}

	// 4. Assertions
	if len(images) == 0 {
		t.Error("Expected images from Spice Melange, got 0")
	}

	if len(images) > 20 {
		t.Errorf("Expected max 20 images per page, got %d", len(images))
	}

	// Check first image structure
	if len(images) > 0 {
		img := images[0]
		if img.Provider != ProviderName {
			t.Errorf("Expected provider %s, got %s", ProviderName, img.Provider)
		}
		if img.Path == "" {
			t.Error("Image Path is empty")
		}
		t.Logf("Successfully fetched image: %s by %s", img.ID, img.Attribution)
	}
}
