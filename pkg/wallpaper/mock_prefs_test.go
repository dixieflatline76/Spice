package wallpaper

import "fyne.io/fyne/v2"

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

// ... List methods ...

func (m *MockPreferences) BoolList(key string) []bool {
	// Not implemented for this mock
	return nil
}

func (m *MockPreferences) BoolListWithFallback(key string, fallback []bool) []bool {
	return fallback
}

func (m *MockPreferences) SetBoolList(key string, value []bool) {
	// Not implemented
}

func (m *MockPreferences) FloatList(key string) []float64 {
	return nil
}

func (m *MockPreferences) FloatListWithFallback(key string, fallback []float64) []float64 {
	return fallback
}

func (m *MockPreferences) SetFloatList(key string, value []float64) {
}

func (m *MockPreferences) IntList(key string) []int {
	return nil
}

func (m *MockPreferences) IntListWithFallback(key string, fallback []int) []int {
	return fallback
}

func (m *MockPreferences) SetIntList(key string, value []int) {
}

func (m *MockPreferences) StringList(key string) []string {
	return nil
}

func (m *MockPreferences) StringListWithFallback(key string, fallback []string) []string {
	return fallback
}

func (m *MockPreferences) SetStringList(key string, value []string) {
}

func (m *MockPreferences) Bool(key string) bool {
	return m.bools[key]
}

func (m *MockPreferences) BoolWithFallback(key string, fallback bool) bool {
	if val, ok := m.bools[key]; ok {
		return val
	}
	return fallback
}

func (m *MockPreferences) SetBool(key string, value bool) {
	m.bools[key] = value
}

func (m *MockPreferences) Float(key string) float64 {
	return m.floats[key]
}

func (m *MockPreferences) FloatWithFallback(key string, fallback float64) float64 {
	if val, ok := m.floats[key]; ok {
		return val
	}
	return fallback
}

func (m *MockPreferences) SetFloat(key string, value float64) {
	m.floats[key] = value
}

func (m *MockPreferences) Int(key string) int {
	return m.ints[key]
}

func (m *MockPreferences) IntWithFallback(key string, fallback int) int {
	if val, ok := m.ints[key]; ok {
		return val
	}
	return fallback
}

func (m *MockPreferences) SetInt(key string, value int) {
	m.ints[key] = value
}

func (m *MockPreferences) String(key string) string {
	return m.strings[key]
}

func (m *MockPreferences) StringWithFallback(key string, fallback string) string {
	if val, ok := m.strings[key]; ok {
		return val
	}
	return fallback
}

func (m *MockPreferences) SetString(key string, value string) {
	m.strings[key] = value
}

func (m *MockPreferences) RemoveValue(key string) {
	delete(m.strings, key)
	delete(m.ints, key)
	delete(m.bools, key)
	delete(m.floats, key)
}

func (m *MockPreferences) AddChangeListener(callback func()) {
	// No-op for mock
}

func (m *MockPreferences) ChangeListeners() []func() {
	return nil
}
