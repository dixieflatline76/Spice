package wallpaper

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSettingsManager implements setting.SettingsManager for testing
type MockSettingsManager struct {
	mock.Mock
	// We store the created widgets to inspect them
	selectWidgets map[string]*setting.SelectConfig
	checkWidgets  map[string]*widget.Check
}

func NewMockSettingsManager() *MockSettingsManager {
	return &MockSettingsManager{
		selectWidgets: make(map[string]*setting.SelectConfig),
		checkWidgets:  make(map[string]*widget.Check),
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
}

func (m *MockSettingsManager) CreateBoolSetting(cfg *setting.BoolConfig, header *fyne.Container) *widget.Check {
	// Create a real widget so we can test its behavior
	check := widget.NewCheck(cfg.Name, cfg.OnChanged)
	check.Checked = cfg.InitialValue

	// We need to store the check widget associated with the config name to retrieve it later
	m.checkWidgets[cfg.Name] = check
	return check
}

func (m *MockSettingsManager) CreateTextEntrySetting(cfg *setting.TextEntrySettingConfig, header *fyne.Container) {
	// No-op for regression test
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

func (m *MockSettingsManager) GetSettingsWindow() fyne.Window {
	return nil
}

func (m *MockSettingsManager) GetCheckAndEnableApplyFunc() func() {
	return func() {}
}

func (m *MockSettingsManager) RebuildTrayMenu() {}

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
	// We mock the manager enough to pass CreatePrefsPanel
	// We don't need full Plugin initialization for this UI logic test

	sm := NewMockSettingsManager()

	// Execute: Create Panel (this builds the UI and wires the logic)
	wp.CreatePrefsPanel(sm)

	// Verify Select Widget exists
	smartFit := sm.selectWidgets["smartFitMode"]
	assert.NotNil(t, smartFit, "Smart Fit Mode Select should be created")

	// Verify Check Widgets exist
	faceCrop := sm.checkWidgets["faceCrop"]
	faceBoost := sm.checkWidgets["faceBoost"]
	assert.NotNil(t, faceCrop, "Face Crop Check should be created")
	assert.NotNil(t, faceBoost, "Face Boost Check should be created")

	// Initial State: Should be Disabled
	assert.True(t, faceCrop.Disabled(), "Face Crop should be disabled initially (SmartFitOff)")
	assert.True(t, faceBoost.Disabled(), "Face Boost should be disabled initially (SmartFitOff)")

	// Action: Change Smart Fit to Quality (Normal)
	// We simulate the UI event "OnChanged", not "ApplyFunc" (which is deferred)
	if smartFit.OnChanged != nil {
		smartFit.OnChanged("Quality", int(SmartFitNormal))
	} else {
		t.Fatal("OnChanged handler not assigned for Smart Fit Mode")
	}

	// Assertions: Should be Enabled
	assert.False(t, faceCrop.Disabled(), "Face Crop should be enabled after switching to Quality")
	assert.False(t, faceBoost.Disabled(), "Face Boost should be enabled after switching to Quality")

	// Action: Change back to Disabled
	if smartFit.OnChanged != nil {
		smartFit.OnChanged("Disabled", int(SmartFitOff))
	}

	// Assertions: Should be Disabled again
	assert.True(t, faceCrop.Disabled(), "Face Crop should be disabled after switching to Disabled")
	assert.True(t, faceBoost.Disabled(), "Face Boost should be disabled after switching to Disabled")
}
