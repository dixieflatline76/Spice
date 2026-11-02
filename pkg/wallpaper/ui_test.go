//go:build !linux

package wallpaper

import (
	"context"
	"image"
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

func (m *MockSettingsManager) RenderTabs(tabs schema.TabsSchema) fyne.CanvasObject {
	box := container.NewVBox()
	for _, item := range tabs.Items {
		if item.Accordion != nil {
			box.Add(m.RenderAccordion(*item.Accordion))
		}
		if item.Panel != nil {
			box.Add(m.RenderSchema(*item.Panel))
		}
	}
	return box
}

func (m *MockSettingsManager) RenderAccordion(acc schema.AccordionSchema) fyne.CanvasObject {
	box := container.NewVBox()
	for _, item := range acc.Items {
		if item.Content != nil {
			box.Add(m.RenderSchema(*item.Content))
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
	tabsSchema := wp.CreatePrefsSchema(sm)
	sm.RenderTabs(*tabsSchema)

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

func newTrayTestPlugin(t *testing.T, perScreen bool, cursor image.Point) (*Plugin, *mockTrayOS, *MockPluginManager) {
	wp := newTestPlugin(t)
	tos := &mockTrayOS{MockOS: &MockOS{}, perScreen: perScreen, cursor: cursor, cursorOK: true}
	mgr := &MockPluginManager{}
	wp.os = tos
	wp.manager = mgr
	wp.Monitors = map[int]*MonitorController{
		0: {Monitor: Monitor{ID: 0, Name: "Built-in", Primary: true, Rect: image.Rect(0, 0, 3456, 2234), Bounds: image.Rect(0, 0, 1728, 1117)}, State: &MonitorState{}},
		1: {Monitor: Monitor{ID: 1, Name: "Studio", Rect: image.Rect(0, 0, 5120, 2880), Bounds: image.Rect(1728, 0, 4288, 1440)}, State: &MonitorState{}},
	}
	return wp, tos, mgr
}

func TestCreateTrayMenu_EmptyMonitors_DoesNotPanic(t *testing.T) {
	wp := newTestPlugin(t)
	wp.Monitors = map[int]*MonitorController{}
	var s *schema.MenuSchema
	assert.NotPanics(t, func() { s = wp.CreateTrayMenu() })
	assert.Empty(t, s.Items)
}

func TestCreateTrayMenu_SingleTray_PrimaryRootOthersSubmenus(t *testing.T) {
	wp, _, _ := newTrayTestPlugin(t, false, image.Point{})
	s := wp.CreateTrayMenu()
	subs := 0
	for _, it := range s.Items {
		if it.SubMenu != nil {
			subs++
		}
	}
	assert.Equal(t, 1, subs)
}

func TestCreateTrayMenu_PerScreenTray_FlatMenu(t *testing.T) {
	wp, _, mgr := newTrayTestPlugin(t, true, image.Pt(2000, 10)) // cursor on Monitor 1
	mgr.On("RebuildTrayMenuImmediate").Return()
	assert.True(t, wp.syncActiveTrayMonitor())
	assert.Equal(t, int64(1), wp.activeTrayMonitorID.Load())

	s := wp.CreateTrayMenu()
	for _, it := range s.Items {
		assert.Nil(t, it.SubMenu, "per-screen tray must not render Display submenus")
	}
	mgr.AssertCalled(t, "RebuildTrayMenuImmediate")
}

func TestSyncActiveTrayMonitor_NoChange_NoRebuild(t *testing.T) {
	wp, _, mgr := newTrayTestPlugin(t, true, image.Pt(500, 10)) // Monitor 0 == initial value
	assert.False(t, wp.syncActiveTrayMonitor())
	mgr.AssertNotCalled(t, "RebuildTrayMenuImmediate")
}

func TestCursorMonitorID_SingleMonitor_SkipsPolling(t *testing.T) {
	wp, _, _ := newTrayTestPlugin(t, true, image.Pt(2000, 10))
	delete(wp.Monitors, 1)
	_, ok := wp.cursorMonitorID()
	assert.False(t, ok)
}

func TestActionBinder_Follow_RetargetsToLiveCursor(t *testing.T) {
	wp, tos, _ := newTrayTestPlugin(t, true, image.Pt(2000, 10))
	got := -1
	act := wp.newActionBinder(1, true).follow(func(id int) { got = id })
	tos.cursor = image.Pt(500, 10) // fast move to Monitor 0 before click
	act()
	assert.Equal(t, 0, got)
}

func TestActionBinder_Guard_FailsClosedOnMismatch(t *testing.T) {
	wp, tos, mgr := newTrayTestPlugin(t, true, image.Pt(2000, 10))
	mgr.On("RebuildTrayMenuImmediate").Return()
	called := false
	act := wp.newActionBinder(1, true).guard(func(int) { called = true })
	tos.cursor = image.Pt(500, 10)
	act()
	assert.False(t, called, "state-labelled action must not run on a different monitor")
	mgr.AssertCalled(t, "RebuildTrayMenuImmediate")
}

func TestActionBinder_Guard_RunsOnMatch(t *testing.T) {
	wp, _, _ := newTrayTestPlugin(t, true, image.Pt(2000, 10))
	got := -1
	wp.newActionBinder(1, true).guard(func(id int) { got = id })()
	assert.Equal(t, 1, got)
}

func TestActionBinder_NotLive_UsesSnapshotID(t *testing.T) {
	wp, tos, _ := newTrayTestPlugin(t, false, image.Pt(500, 10))
	tos.cursor = image.Pt(500, 10)
	got := -1
	wp.newActionBinder(1, false).follow(func(id int) { got = id })()
	assert.Equal(t, 1, got)
	got = -1
	wp.newActionBinder(1, false).guard(func(id int) { got = id })()
	assert.Equal(t, 1, got)
}
