package wallpaper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockPreferences implements fyne.Preferences for testing
type MockPreferences struct {
	data map[string]interface{}
}

func NewMockPreferences() *MockPreferences {
	return &MockPreferences{
		data: make(map[string]interface{}),
	}
}

func (m *MockPreferences) Bool(key string) bool {
	val, ok := m.data[key]
	if !ok {
		return false
	}
	return val.(bool)
}

func (m *MockPreferences) BoolWithFallback(key string, fallback bool) bool {
	val, ok := m.data[key]
	if !ok {
		return fallback
	}
	return val.(bool)
}

func (m *MockPreferences) SetBool(key string, value bool) {
	m.data[key] = value
}

func (m *MockPreferences) Float(key string) float64 {
	val, ok := m.data[key]
	if !ok {
		return 0.0
	}
	return val.(float64)
}

func (m *MockPreferences) FloatWithFallback(key string, fallback float64) float64 {
	val, ok := m.data[key]
	if !ok {
		return fallback
	}
	return val.(float64)
}

func (m *MockPreferences) SetFloat(key string, value float64) {
	m.data[key] = value
}

func (m *MockPreferences) Int(key string) int {
	val, ok := m.data[key]
	if !ok {
		return 0
	}
	return val.(int)
}

func (m *MockPreferences) IntWithFallback(key string, fallback int) int {
	val, ok := m.data[key]
	if !ok {
		return fallback
	}
	return val.(int)
}

func (m *MockPreferences) SetInt(key string, value int) {
	m.data[key] = value
}

func (m *MockPreferences) String(key string) string {
	val, ok := m.data[key]
	if !ok {
		return ""
	}
	return val.(string)
}

func (m *MockPreferences) StringWithFallback(key string, fallback string) string {
	val, ok := m.data[key]
	if !ok {
		return fallback
	}
	return val.(string)
}

func (m *MockPreferences) SetString(key string, value string) {
	m.data[key] = value
}

func (m *MockPreferences) StringList(key string) []string {
	val, ok := m.data[key]
	if !ok {
		return []string{}
	}
	return val.([]string)
}

func (m *MockPreferences) StringListWithFallback(key string, fallback []string) []string {
	val, ok := m.data[key]
	if !ok {
		return fallback
	}
	return val.([]string)
}

func (m *MockPreferences) SetStringList(key string, value []string) {
	m.data[key] = value
}

func (m *MockPreferences) BoolList(key string) []bool {
	val, ok := m.data[key]
	if !ok {
		return []bool{}
	}
	return val.([]bool)
}

func (m *MockPreferences) BoolListWithFallback(key string, fallback []bool) []bool {
	val, ok := m.data[key]
	if !ok {
		return fallback
	}
	return val.([]bool)
}

func (m *MockPreferences) SetBoolList(key string, value []bool) {
	m.data[key] = value
}

func (m *MockPreferences) FloatList(key string) []float64 {
	val, ok := m.data[key]
	if !ok {
		return []float64{}
	}
	return val.([]float64)
}

func (m *MockPreferences) FloatListWithFallback(key string, fallback []float64) []float64 {
	val, ok := m.data[key]
	if !ok {
		return fallback
	}
	return val.([]float64)
}

func (m *MockPreferences) SetFloatList(key string, value []float64) {
	m.data[key] = value
}

func (m *MockPreferences) IntList(key string) []int {
	val, ok := m.data[key]
	if !ok {
		return []int{}
	}
	return val.([]int)
}

func (m *MockPreferences) IntListWithFallback(key string, fallback []int) []int {
	val, ok := m.data[key]
	if !ok {
		return fallback
	}
	return val.([]int)
}

func (m *MockPreferences) SetIntList(key string, value []int) {
	m.data[key] = value
}

func (m *MockPreferences) RemoveValue(key string) {
	delete(m.data, key)
}

func (m *MockPreferences) AddChangeListener(func()) {
	// No-op for now
}

func (m *MockPreferences) ChangeListeners() []func() {
	return []func(){}
}

func TestConfig(t *testing.T) {
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	t.Run("SmartFit", func(t *testing.T) {
		assert.True(t, cfg.GetSmartFit()) // Default true

		cfg.SetSmartFit(false)
		assert.False(t, cfg.GetSmartFit())
		assert.False(t, prefs.Bool(SmartFitPrefKey))
	})

	t.Run("FaceCrop", func(t *testing.T) {
		assert.False(t, cfg.GetFaceCropEnabled()) // Default false

		cfg.SetFaceCropEnabled(true)
		assert.True(t, cfg.GetFaceCropEnabled())
		assert.True(t, prefs.Bool(FaceCropPrefKey))
	})

	t.Run("FaceBoost", func(t *testing.T) {
		assert.False(t, cfg.GetFaceBoostEnabled()) // Default false

		cfg.SetFaceBoostEnabled(true)
		assert.True(t, cfg.GetFaceBoostEnabled())
		assert.True(t, prefs.Bool(FaceBoostPrefKey))
	})

	t.Run("ImageQueries", func(t *testing.T) {
		// Clear default queries loaded from asset
		cfg.Queries = []ImageQuery{}

		id, err := cfg.AddImageQuery("Test Query", "https://example.com", true)
		assert.NoError(t, err)
		assert.NotEmpty(t, id)

		assert.True(t, cfg.IsDuplicateID(id))

		// Verify it was added to the unified list with correct provider
		assert.Equal(t, 1, len(cfg.Queries))
		assert.Equal(t, "Wallhaven", cfg.Queries[0].Provider)

		err = cfg.DisableImageQuery(id)
		assert.NoError(t, err)
		// Verify it's disabled
		assert.False(t, cfg.Queries[0].Active)

		err = cfg.RemoveImageQuery(id)
		assert.NoError(t, err)
		assert.Empty(t, cfg.Queries)
	})
}
