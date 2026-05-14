//go:build !linux

package wallpaper

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type managedMockWidget struct {
	widget    fyne.CanvasObject
	enabledIf func() bool
	visibleIf func() bool
}

// MockSettingsManager implements setting.SettingsManager for testing
type MockSettingsManager struct {
	mock.Mock
	// We store the created widgets to inspect them
	allWidgets     map[string]fyne.CanvasObject
	managedWidgets []managedMockWidget
	pendingChanges map[string]bool
	statusLabels   map[string]string
	refreshFuncs   []func()
}

func NewMockSettingsManager() *MockSettingsManager {
	return &MockSettingsManager{
		allWidgets:     make(map[string]fyne.CanvasObject),
		managedWidgets: make([]managedMockWidget, 0),
		pendingChanges: make(map[string]bool),
		statusLabels:   make(map[string]string),
	}
}

func (m *MockSettingsManager) GetApplySettingsButton() *widget.Button {
	return widget.NewButton("Apply", nil)
}

func (m *MockSettingsManager) SetSettingChangedCallback(settingName string, callback func()) {
	// No-op
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

func (m *MockSettingsManager) ShowError(err error) {
	// No-op
}

func (m *MockSettingsManager) ShowConfirm(title, message string, callback func(bool)) {
	// No-op
}

func (m *MockSettingsManager) GetCheckAndEnableApplyFunc() func() {
	return func() {}
}

func (m *MockSettingsManager) SeedBaseline(name string, val interface{}) {
	// No-op
}

func (m *MockSettingsManager) GetBaseline(name string) interface{} {
	return nil
}

func (m *MockSettingsManager) GetValue(name string) interface{} {
	if w, ok := m.allWidgets[name]; ok {
		if c, ok := w.(*widget.Check); ok {
			return c.Checked
		}
		if s, ok := w.(*widget.Select); ok {
			for i, opt := range s.Options {
				if opt == s.Selected {
					return i
				}
			}
			return -1
		}
	}
	return nil
}

func (m *MockSettingsManager) SetValue(name string, val interface{}) {
	if w, ok := m.allWidgets[name]; ok {
		if c, ok := w.(*widget.Check); ok {
			if b, ok := val.(bool); ok {
				c.SetChecked(b)
				m.refreshWidgetStates()
			}
		}
		if s, ok := w.(*widget.Select); ok {
			if str, ok := val.(string); ok {
				s.SetSelected(str)
				m.refreshWidgetStates()
			} else if idx, ok := val.(int); ok {
				if idx >= 0 && idx < len(s.Options) {
					s.SetSelected(s.Options[idx])
					m.refreshWidgetStates()
				}
			}
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
	m.refreshWidgetStates()
	for _, f := range m.refreshFuncs {
		f()
	}
}

func (m *MockSettingsManager) RefreshUI() {
	m.refreshWidgetStates()
}

func (m *MockSettingsManager) CommitSetting(name string) {
	m.pendingChanges[name] = false
}

func (m *MockSettingsManager) ResetSettings(resets ...setting.SettingReset) {
	for _, r := range resets {
		m.SetValue(r.Name, r.Value)
		m.pendingChanges[r.Name] = false
	}
}

func (m *MockSettingsManager) SetSettingStatus(name string, message string, importance schema.Importance) {
	m.statusLabels[name] = message
}

func (m *MockSettingsManager) OpenURL(u string) {
	m.Called(u)
}

func (m *MockSettingsManager) ShowAddQueryDialog(cfg schema.AddQueryConfig, initialURL, initialDesc string, onAdded func()) {
	m.Called(cfg, initialURL, initialDesc, onAdded)
}

func (m *MockSettingsManager) RenderSchema(panel schema.PanelSchema) fyne.CanvasObject {
	box := container.NewVBox()
	for _, s := range panel.Sections {
		for _, item := range s.Items {
			switch v := item.(type) {
			case schema.BoolItem:
				check := widget.NewCheck(v.Label, v.OnChanged)
				check.SetChecked(v.InitialValue)
				m.allWidgets[v.Name] = check
				if v.EnabledIf != nil || v.VisibleIf != nil {
					m.managedWidgets = append(m.managedWidgets, managedMockWidget{
						widget:    check,
						enabledIf: v.EnabledIf,
						visibleIf: v.VisibleIf,
					})
				}
				box.Add(check)
			case schema.SelectItem:
				sel := widget.NewSelect(v.Options, nil)
				if idx, ok := v.InitialValue.(int); ok && idx >= 0 && idx < len(v.Options) {
					sel.SetSelected(v.Options[idx])
				}
				m.allWidgets[v.Name] = sel
				if v.EnabledIf != nil || v.VisibleIf != nil {
					m.managedWidgets = append(m.managedWidgets, managedMockWidget{
						widget:    sel,
						enabledIf: v.EnabledIf,
						visibleIf: v.VisibleIf,
					})
				}
				box.Add(sel)
			case schema.ButtonItem:
				btn := widget.NewButton(v.ButtonText, v.OnPressed)
				m.allWidgets[v.Name] = btn
				if v.EnabledIf != nil || v.VisibleIf != nil {
					m.managedWidgets = append(m.managedWidgets, managedMockWidget{
						widget:    btn,
						enabledIf: v.EnabledIf,
						visibleIf: v.VisibleIf,
					})
				}
				box.Add(btn)
			case schema.AsyncButtonItem:
				btn := widget.NewButton(v.ButtonText, nil)
				m.allWidgets[v.Name] = btn
				if v.EnabledIf != nil || v.VisibleIf != nil {
					m.managedWidgets = append(m.managedWidgets, managedMockWidget{
						widget:    btn,
						enabledIf: v.EnabledIf,
						visibleIf: v.VisibleIf,
					})
				}
				box.Add(btn)
			case schema.TextItem:
				entry := widget.NewEntry()
				entry.SetText(v.InitialValue)
				m.allWidgets[v.Name] = entry
				if v.EnabledIf != nil || v.VisibleIf != nil {
					m.managedWidgets = append(m.managedWidgets, managedMockWidget{
						widget:    entry,
						enabledIf: v.EnabledIf,
						visibleIf: v.VisibleIf,
					})
				}
				box.Add(entry)
			case schema.HorizontalRowItem:
				m.RenderSchema(schema.PanelSchema{
					Sections: []schema.SectionSchema{{Items: v.Items}},
				})
			case *schema.HorizontalRowItem:
				m.RenderSchema(schema.PanelSchema{
					Sections: []schema.SectionSchema{{Items: v.Items}},
				})
			}
		}
	}
	return box
}

func (m *MockSettingsManager) refreshWidgetStates() {
	for _, mw := range m.managedWidgets {
		if mw.visibleIf != nil {
			if mw.visibleIf() {
				mw.widget.Show()
			} else {
				mw.widget.Hide()
			}
		}
		if mw.enabledIf != nil {
			if d, ok := mw.widget.(fyne.Disableable); ok {
				if mw.enabledIf() {
					d.Enable()
				} else {
					d.Disable()
				}
			}
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
	faceCrop := sm.allWidgets["faceCrop"].(*widget.Check)
	faceBoost := sm.allWidgets["faceBoost"].(*widget.Check)
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
	// In the real app, sm.RefreshUI() is called on toggle, which calls the accordion refreshers, which call the titleFunc.
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

func (m *MockProvider) ID() string                                   { return m.IDVal }
func (m *MockProvider) Name() string                                 { return m.IDVal }
func (m *MockProvider) Title() string                                { return m.TitleVal }
func (m *MockProvider) Type() provider.ProviderType                  { return provider.TypeCommunity }
func (m *MockProvider) GetProviderIcon() interface{}                 { return nil }
func (m *MockProvider) GetAttributionType() provider.AttributionType { return provider.AttributionBy }
func (m *MockProvider) ParseURL(url string) (string, error)          { return "", nil }
func (m *MockProvider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
	return nil
}
func (m *MockProvider) CreateQueryPanel(sm setting.SettingsManager, pendingURL string) *schema.PanelSchema {
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
