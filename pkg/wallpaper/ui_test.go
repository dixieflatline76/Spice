//go:build !linux

package wallpaper

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type managedMockWidget struct {
	widget    fyne.Disableable
	enabledIf func() bool
}

// MockSettingsManager implements setting.SettingsManager for testing
type MockSettingsManager struct {
	mock.Mock
	// We store the created widgets to inspect them
	selectWidgets  map[string]*setting.SelectConfig
	checkWidgets   map[string]*widget.Check
	managedWidgets []managedMockWidget
}

func NewMockSettingsManager() *MockSettingsManager {
	return &MockSettingsManager{
		selectWidgets:  make(map[string]*setting.SelectConfig),
		checkWidgets:   make(map[string]*widget.Check),
		managedWidgets: make([]managedMockWidget, 0),
	}
}

func (m *MockSettingsManager) CreateSectionTitleLabel(desc string) *widget.Label {
	return widget.NewLabel(desc)
}

func (m *MockSettingsManager) CreateSettingTitleLabel(desc string) *widget.Label {
	return widget.NewLabel(desc)
}

func (m *MockSettingsManager) CreateSettingDescriptionLabel(desc string) fyne.CanvasObject {
	return widget.NewLabel(desc)
}

func (m *MockSettingsManager) CreateSelectSetting(cfg *setting.SelectConfig, header *fyne.Container) {
	m.selectWidgets[cfg.Name] = cfg
	// We don't have a real select widget in the mock yet, but let's track the EnabledIf
	if cfg.EnabledIf != nil {
		// Mock select widget just to have something to Disable/Enable
		w := widget.NewSelect(cfg.Options, nil)
		m.managedWidgets = append(m.managedWidgets, managedMockWidget{
			widget:    w,
			enabledIf: cfg.EnabledIf,
		})
	}
}

func (m *MockSettingsManager) CreateBoolSetting(cfg *setting.BoolConfig, header *fyne.Container) *widget.Check {
	// Create a real widget so we can test its behavior
	check := widget.NewCheck(cfg.Name, cfg.OnChanged)
	check.Checked = cfg.InitialValue

	// We need to store the check widget associated with the config name to retrieve it later
	m.checkWidgets[cfg.Name] = check

	if cfg.EnabledIf != nil {
		m.managedWidgets = append(m.managedWidgets, managedMockWidget{
			widget:    check,
			enabledIf: cfg.EnabledIf,
		})
	}
	return check
}

func (m *MockSettingsManager) CreateTextEntrySetting(cfg *setting.TextEntrySettingConfig, header *fyne.Container) *widget.Entry {
	// Return a stub entry for tests
	entry := widget.NewEntry()
	if cfg.EnabledIf != nil {
		m.managedWidgets = append(m.managedWidgets, managedMockWidget{
			widget:    entry,
			enabledIf: cfg.EnabledIf,
		})
	}
	return entry
}

func (m *MockSettingsManager) CreateButtonWithConfirmationSetting(cfg *setting.ButtonWithConfirmationConfig, header *fyne.Container) {
	// No-op for regression test
}

func (m *MockSettingsManager) GetApplySettingsButton() *widget.Button {
	return widget.NewButton("Apply", nil)
}

func (m *MockSettingsManager) SetSettingChangedCallback(settingName string, callback func()) {
	// No-op for now
}

func (m *MockSettingsManager) RemoveSettingChangedCallback(settingName string) {
	// No-op
}

func (m *MockSettingsManager) SetRefreshFlag(settingName string) {
	// No-op
}

func (m *MockSettingsManager) UnsetRefreshFlag(settingName string) {
	// No-op
}

func (m *MockSettingsManager) RegisterRefreshFunc(refreshFunc func()) {
	// No-op
}

func (m *MockSettingsManager) RegisterOnSettingsSaved(callback func()) {
	// No-op
}

func (m *MockSettingsManager) GetSettingsWindow() fyne.Window {
	return nil
}

func (m *MockSettingsManager) GetCheckAndEnableApplyFunc() func() {
	return func() {}
}

func (m *MockSettingsManager) RebuildTrayMenu() {
	// No-op
}

func (m *MockSettingsManager) SeedBaseline(name string, val interface{}) {
	// No-op
}

func (m *MockSettingsManager) GetBaseline(name string) interface{} {
	return nil
}

func (m *MockSettingsManager) GetValue(name string) interface{} {
	// Attempt to return current value from select/check widgets if they exist
	if cfg, ok := m.selectWidgets[name]; ok {
		return cfg.InitialValue // Mock current as initial for simplicity in tests unless we track more
	}
	if check, ok := m.checkWidgets[name]; ok {
		return check.Checked
	}
	return nil
}

func (m *MockSettingsManager) SetValue(name string, val interface{}) {
	if cfg, ok := m.selectWidgets[name]; ok {
		cfg.InitialValue = val
		m.refreshWidgetStates()
		return
	}
	if check, ok := m.checkWidgets[name]; ok {
		if b, ok := val.(bool); ok {
			check.SetChecked(b)
			m.refreshWidgetStates()
		}
	}
}

func (m *MockSettingsManager) HasPendingChange(name string) bool {
	return false
}

func (m *MockSettingsManager) Refresh() {
	// No-op for mock
}

func (m *MockSettingsManager) refreshWidgetStates() {
	for _, mw := range m.managedWidgets {
		if mw.enabledIf() {
			mw.widget.Enable()
		} else {
			mw.widget.Disable()
		}
	}
}

func TestSmartFitEnablesFaceOptions(t *testing.T) {
	// Initialize Fyne test app
	test.NewApp()

	// Setup
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	// Ensure we start with SmartFit Disabled
	cfg.SetSmartFitMode(SmartFitOff)
	cfg.SetFaceBoostEnabled(false)
	cfg.SetFaceCropEnabled(false)

	// Create Plugin and Mock Manager
	wp := &Plugin{cfg: cfg}
	sm := NewMockSettingsManager()

	// Execute: Create Panel (this builds the UI and wires the logic)
	wp.CreatePrefsPanel(sm)

	// Verify Check Widgets exist
	faceCrop := sm.checkWidgets["faceCrop"]
	faceBoost := sm.checkWidgets["faceBoost"]
	assert.NotNil(t, faceCrop, "Face Crop Check should be created")
	assert.NotNil(t, faceBoost, "Face Boost Check should be created")

	// Trigger initial state evaluation (in real SM this happens on create)
	sm.refreshWidgetStates()

	// Initial State: Should be Disabled
	assert.True(t, faceCrop.Disabled(), "Face Crop should be disabled initially (SmartFitOff)")
	assert.True(t, faceBoost.Disabled(), "Face Boost should be disabled initially (SmartFitOff)")

	// Action: Change Smart Fit to Quality (Normal) via SetValue (simulates UI change)
	sm.SetValue("smartFitMode", int(SmartFitNormal))

	// Assertions: Should be Enabled
	assert.False(t, faceCrop.Disabled(), "Face Crop should be enabled after switching to Quality")
	assert.False(t, faceBoost.Disabled(), "Face Boost should be enabled after switching to Quality")

	// Action: Change back to Disabled
	sm.SetValue("smartFitMode", int(SmartFitOff))

	// Assertions: Should be Disabled again
	assert.True(t, faceCrop.Disabled(), "Face Crop should be disabled after switching to Disabled")
	assert.True(t, faceBoost.Disabled(), "Face Boost should be disabled after switching to Disabled")
}
