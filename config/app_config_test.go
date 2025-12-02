package config

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

func TestAppConfig(t *testing.T) {
	prefs := NewMockPreferences()
	cfg := NewAppConfig(prefs)

	t.Run("Notifications", func(t *testing.T) {
		// Default should be true
		assert.True(t, cfg.GetAppNotificationsEnabled())

		cfg.SetAppNotificationsEnabled(false)
		assert.False(t, cfg.GetAppNotificationsEnabled())

		cfg.SetAppNotificationsEnabled(true)
		assert.True(t, cfg.GetAppNotificationsEnabled())
	})

	t.Run("UpdateCheck", func(t *testing.T) {
		// Default should be true
		assert.True(t, cfg.GetUpdateCheckEnabled())

		cfg.SetUpdateCheckEnabled(false)
		assert.False(t, cfg.GetUpdateCheckEnabled())

		cfg.SetUpdateCheckEnabled(true)
		assert.True(t, cfg.GetUpdateCheckEnabled())
	})

	t.Run("Theme", func(t *testing.T) {
		// Default should be "System"
		assert.Equal(t, "System", cfg.GetTheme())

		cfg.SetTheme("Dark")
		assert.Equal(t, "Dark", cfg.GetTheme())

		cfg.SetTheme("Light")
		assert.Equal(t, "Light", cfg.GetTheme())
	})
}
