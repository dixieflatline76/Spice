//go:build !linux

package wallpaper

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
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
	pendingChanges map[string]bool
	refreshFuncs   []func()
}

func NewMockSettingsManager() *MockSettingsManager {
	return &MockSettingsManager{
		selectWidgets:  make(map[string]*setting.SelectConfig),
		checkWidgets:   make(map[string]*widget.Check),
		managedWidgets: make([]managedMockWidget, 0),
		pendingChanges: make(map[string]bool),
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
	m.refreshFuncs = append(m.refreshFuncs, refreshFunc)
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
	return m.pendingChanges[name]
}

func (m *MockSettingsManager) SetPendingChange(name string, pending bool) {
	m.pendingChanges[name] = pending
}

func (m *MockSettingsManager) Refresh() {
	for _, f := range m.refreshFuncs {
		f()
	}
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

func TestReactiveAccordionTitle(t *testing.T) {
	ResetConfig()
	prefs := NewMockPreferences()
	cfg := GetConfig(prefs)

	// Setup: 1 active query for Wallhaven
	cfg.Queries = []ImageQuery{}
	id, _ := cfg.AddImageQuery("Wallhaven Default", "http://wallhaven.cc/1", true)

	wp := &Plugin{cfg: cfg, providers: make(map[string]provider.ImageProvider)}
	// Mock provider
	p := &MockProvider{IDVal: "Wallhaven", TitleVal: "Wallhaven"}
	wp.providers["Wallhaven"] = p

	sm := NewMockSettingsManager()
	builder := NewPrefsPanelBuilder(wp, sm)

	// Execute: Create Title Func
	titleFunc := builder.createTitleFunc(p)

	// Initial State: Should show (1 active)
	assert.Contains(t, titleFunc(), "(1 active)")

	// Action: Simulate user untoggling the query in the UI (pending change)
	sm.SetPendingChange(id, true) // baseline was 'true', so pending change makes it 'false' in the title logic

	// Verify Reactivity: Title should update immediately when func is called (provided Refresh was triggered)
	// In the real app, sm.Refresh() is called on toggle, which calls the accordion refreshers, which call the titleFunc.
	assert.NotContains(t, titleFunc(), "(1 active)", "Title should no longer show active count after pending untoggle")
	assert.Equal(t, "Wallhaven", titleFunc())

	// Action: Reset pending change
	sm.SetPendingChange(id, false)
	assert.Contains(t, titleFunc(), "(1 active)")
}

// Minimal MockProvider for testing
type MockProvider struct {
	mock.Mock
	IDVal    string
	TitleVal string
}

func (m *MockProvider) ID() string                                                       { return m.IDVal }
func (m *MockProvider) Name() string                                                     { return m.IDVal }
func (m *MockProvider) Title() string                                                    { return m.TitleVal }
func (m *MockProvider) Type() provider.ProviderType                                      { return provider.TypeOnline }
func (m *MockProvider) GetProviderIcon() fyne.Resource                                   { return nil }
func (m *MockProvider) GetAttributionType() provider.AttributionType                     { return provider.AttributionBy }
func (m *MockProvider) ParseURL(url string) (string, error)                              { return "", nil }
func (m *MockProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject { return nil }
func (m *MockProvider) CreateQueryPanel(sm setting.SettingsManager, pendingURL string) fyne.CanvasObject {
	return nil
}
func (m *MockProvider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	return nil, nil
}
func (m *MockProvider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}
func (m *MockProvider) SupportsUserQueries() bool { return true }
func (m *MockProvider) HomeURL() string           { return "" }
