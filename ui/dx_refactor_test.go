package ui

import (
	"fmt"
	"image"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/stretchr/testify/assert"
)

func TestCommitSetting(t *testing.T) {
	// Initialize test app and window
	app := test.NewApp()
	defer test.NewApp() // cleanup
	testWin := app.NewWindow("Test")

	sm := NewSettingsManager(testWin)

	// 1. Create a setting via schema
	initialVal := "Original Baseline"
	appliedVal := ""

	p := schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Items: []schema.ItemSchema{
					schema.TextItem{
						Name:         "testKey",
						InitialValue: initialVal,
						ApplyFunc: func(s string) {
							appliedVal = s
						},
					},
				},
			},
		},
	}
	sm.RenderSchema(p)

	// 2. Modify value in UI
	newVal := "New Value to Commit"
	sm.SetValue("testKey", newVal)

	// Since SetValue doesn't trigger the change callback (it's for programmatic sync),
	// we need to simulate the UI callback.
	entry := sm.(*SettingsManager).allWidgets["testKey"].(*widget.Entry)
	entry.OnChanged(newVal)

	assert.True(t, sm.HasPendingChange("testKey"), "Should have pending change after typing")
	assert.Equal(t, initialVal, sm.GetBaseline("testKey"), "Baseline should still be original")

	// 4. Call CommitSetting
	sm.CommitSetting("testKey")

	// 5. Final Assertions
	assert.False(t, sm.HasPendingChange("testKey"), "Pending change should be cleared after CommitSetting")
	assert.Equal(t, newVal, sm.GetBaseline("testKey"), "Baseline should be updated to new value")
	assert.Equal(t, newVal, appliedVal, "Native setter (ApplyFunc) should have been invoked")
}

func TestResetSettings(t *testing.T) {
	app := test.NewApp()
	testWin := app.NewWindow("Test")
	sm := NewSettingsManager(testWin)

	appliedVal := "Initial"
	p := schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Items: []schema.ItemSchema{
					schema.TextItem{
						Name:         "testKey",
						InitialValue: "Initial",
						ApplyFunc: func(s string) {
							appliedVal = s
						},
					},
				},
			},
		},
	}
	sm.RenderSchema(p)

	// 1. Atomic Reset
	resetVal := "Reset Value"
	sm.ResetSettings(setting.SettingReset{Name: "testKey", Value: resetVal})

	// 2. Verify
	assert.Equal(t, resetVal, sm.GetBaseline("testKey"), "Baseline should be updated")
	assert.Equal(t, resetVal, sm.GetValue("testKey"), "UI value should be updated")
	assert.Equal(t, resetVal, appliedVal, "Native preference should be updated")
	assert.False(t, sm.HasPendingChange("testKey"), "Should have no pending change after reset")
}

func TestAsyncButtonReentrancy(t *testing.T) {
	app := test.NewApp()
	testWin := app.NewWindow("Test Reentrancy")
	sm := NewSettingsManager(testWin)

	var callCount int32
	blocker := make(chan bool)

	p := schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Items: []schema.ItemSchema{
					schema.AsyncButtonItem{
						Name:        "asyncTest",
						ButtonText:  "Execute",
						LoadingText: "Loading...",
						OnPressed: func() error {
							atomic.AddInt32(&callCount, 1)
							<-blocker // Wait for manual release
							return nil
						},
						OnCompleted: func(err error) {},
					},
				},
			},
		},
	}
	sm.RenderSchema(p)
	btn := sm.(*SettingsManager).allWidgets["asyncTest"].(*widget.Button)

	// Verify initial state
	assert.False(t, btn.Disabled())

	// Trigger first tap
	go test.Tap(btn)

	// Brief sleep to ensure goroutine starts and enters OnPressed
	time.Sleep(50 * time.Millisecond)

	// Assert button is now disabled (Loading state)
	assert.True(t, btn.Disabled(), "Button should be disabled while processing")
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "Should have called OnPressed once")

	// Trigger second tap while still blocked
	test.Tap(btn)

	// Release and wait for completion
	blocker <- true
	time.Sleep(50 * time.Millisecond)

	// Final verification
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "Should NOT have called OnPressed a second time (Reentrancy protection failed)")
	assert.False(t, btn.Disabled(), "Button should be re-enabled after completion")
}

func TestVisibleIfStateTransitions(t *testing.T) {
	app := test.NewApp()
	testWin := app.NewWindow("Test Visibility")
	sm := NewSettingsManager(testWin)

	baseline := "ValidKey"
	p := schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Items: []schema.ItemSchema{
					schema.TextItem{
						Name:         "wallhavenAPIKey",
						InitialValue: baseline,
						ApplyFunc:    func(s string) {},
					},
					schema.AsyncButtonItem{
						Name:       "verify",
						ButtonText: "Verify",
						VisibleIf: func() bool {
							curr := sm.GetValue("wallhavenAPIKey").(string)
							base := sm.GetBaseline("wallhavenAPIKey").(string)
							return curr != base || curr == ""
						},
					},
					schema.AsyncButtonItem{
						Name:       "clear",
						ButtonText: "Clear",
						VisibleIf: func() bool {
							curr := sm.GetValue("wallhavenAPIKey").(string)
							base := sm.GetBaseline("wallhavenAPIKey").(string)
							return curr == base && curr != ""
						},
					},
				},
			},
		},
	}
	sm.RenderSchema(p)

	verifyBtn := sm.(*SettingsManager).allWidgets["verify"].(*widget.Button)
	clearBtn := sm.(*SettingsManager).allWidgets["clear"].(*widget.Button)

	// Initial State: baseline matches live -> Clear visible, Verify hidden
	sm.(*SettingsManager).fullRefresh()
	assert.True(t, clearBtn.Visible(), "Clear button should be visible initially")
	assert.False(t, verifyBtn.Visible(), "Verify button should be hidden initially")

	// Transition 1: User types (live != baseline)
	sm.SetValue("wallhavenAPIKey", "ValidKeyX")
	sm.(*SettingsManager).fullRefresh()
	assert.False(t, clearBtn.Visible(), "Clear button should hide when editing")
	assert.True(t, verifyBtn.Visible(), "Verify button should show when editing")

	// Transition 2: User reverts (live == baseline)
	sm.SetValue("wallhavenAPIKey", "ValidKey")
	sm.(*SettingsManager).fullRefresh()
	assert.True(t, clearBtn.Visible(), "Clear button should reappear on revert")
	assert.False(t, verifyBtn.Visible(), "Verify button should hide on revert")
}

func TestApplyTriggersRefresh(t *testing.T) {
	app := test.NewApp()
	testWin := app.NewWindow("Test Apply Refresh")
	sm := NewSettingsManager(testWin)

	refreshCount := 0
	sm.RegisterRefreshFunc(func() {
		refreshCount++
	})

	// Create a setting with NeedsRefresh: true via schema
	p := schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Items: []schema.ItemSchema{
					schema.BoolItem{
						Name:         "triggerRefresh",
						InitialValue: false,
						NeedsRefresh: true,
						ApplyFunc:    func(b bool) {},
					},
				},
			},
		},
	}
	sm.RenderSchema(p)

	// Verify initial state: apply button should be disabled, no refreshes yet
	applyBtn := sm.GetApplySettingsButton()
	assert.True(t, applyBtn.Disabled())
	assert.Equal(t, 0, refreshCount)

	// Simulate user toggling the setting
	sm.SetValue("triggerRefresh", true)
	assert.False(t, applyBtn.Disabled(), "Apply button should enable after change")

	// Trigger Apply
	test.Tap(applyBtn)

	// Assert: The refresh function MUST have been called
	assert.Equal(t, 1, refreshCount, "Registered refresh functions should fire after Apply if NeedsRefresh is true")
}

func TestAsyncButtonStatusAutoUpdate(t *testing.T) {
	app := test.NewApp()
	testWin := app.NewWindow("Test Async Status")
	sm := NewSettingsManager(testWin)

	p := schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Items: []schema.ItemSchema{
					schema.TextItem{
						Name:          "targetField",
						InitialValue:  "Initial",
						DisplayStatus: true,
					},
					schema.AsyncButtonItem{
						Name:            "testBtn",
						ButtonText:      "Verify",
						TargetStatusKey: "targetField",
						OnPressed: func() error {
							time.Sleep(50 * time.Millisecond)
							return nil // Signal success
						},
						OnCompleted: func(err error) {},
					},
				},
			},
		},
	}
	sm.RenderSchema(p)

	// Verify initial status: Empty
	statusLabel := sm.(*SettingsManager).statusLabels["targetField"]
	assert.Equal(t, "", statusLabel.Text)

	// Simulate Button Click
	// Tapping our async button specifically
	testBtn, ok := sm.(*SettingsManager).allWidgets["testBtn"].(*widget.Button)
	assert.True(t, ok)
	assert.NotNil(t, testBtn)

	test.Tap(testBtn)

	// Wait for async completion
	// In a real app we'd use fyne.Do, in tests we can wait slightly
	time.Sleep(200 * time.Millisecond)

	// Assert: Framework should have automatically updated the status to "Success" (via i18n translation or hardcoded default)
	// Since we are using i18n.T("Success"), it might be "Success" in English.
	assert.Contains(t, statusLabel.Text, "Success")
	assert.Equal(t, widget.SuccessImportance, statusLabel.Importance)
}

func TestSetSettingStatusThreadSafety(t *testing.T) {
	app := test.NewApp()
	testWin := app.NewWindow("Test Thread Safety")
	sm := NewSettingsManager(testWin)

	p := schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Items: []schema.ItemSchema{
					schema.TextItem{
						Name:          "safetyTarget",
						InitialValue:  "",
						DisplayStatus: true,
					},
				},
			},
		},
	}
	sm.RenderSchema(p)

	statusLabel := sm.(*SettingsManager).statusLabels["safetyTarget"]

	// Launch background status update
	done := make(chan bool)
	go func() {
		sm.SetSettingStatus("safetyTarget", "External Update", schema.ImportanceHigh)
		done <- true
	}()

	<-done
	// Give Fyne's event loop a tiny bit of time
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, "External Update", statusLabel.Text)
	assert.Equal(t, widget.HighImportance, statusLabel.Importance)
}

func TestSchemaRendering(t *testing.T) {
	app := test.NewApp()
	testWin := app.NewWindow("Test Schema Rendering")
	sm := NewSettingsManager(testWin)

	// 1. Define a pure Go schema (No Fyne dependencies here!)
	p := schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title:       "General Settings",
				Description: "Configure basic app behavior",
				Items: []schema.ItemSchema{
					schema.BoolItem{
						Name:         "enableNotifications",
						Label:        "Enable Notifications",
						InitialValue: true,
					},
					schema.TextItem{
						Name:         "username",
						Label:        "Username",
						InitialValue: "Karl",
						Validator: func(s string) error {
							if len(s) < 3 {
								return fmt.Errorf("too short")
							}
							return nil
						},
					},
					schema.HyperlinkItem{
						Text: "Help",
						URL:  "https://example.com",
					},
					schema.LabelItem{
						Text: "Static description",
					},
					schema.LabelItem{
						Text:    "Sub Header",
						IsTitle: true,
					},
				},
			},
			{
				Title: "Advanced",
				Items: []schema.ItemSchema{
					schema.AsyncButtonItem{
						Name:       "resetDatabase",
						ButtonText: "Reset DB",
						Style:      schema.ButtonStyleDanger,
					},
				},
			},
		},
	}

	// 2. Render the schema
	rendered := sm.RenderSchema(p)

	// 3. Verify the generated tree
	assert.NotNil(t, rendered)

	// For panels without an expanding list (App tab, General sections, Museums),
	// we expect a VScroll wrapping the VBox so short/HiDPI windows scroll cleanly.
	scroll, ok := rendered.(*container.Scroll)
	assert.True(t, ok, "Rendered output should be a VScroll container")
	mainBox, ok := scroll.Content.(*fyne.Container)
	assert.True(t, ok, "VScroll content should be a VBox container")
	assert.Equal(t, 2, len(mainBox.Objects), "VBox should have 2 sections")

	// Check registry: The RenderSchema should have registered the widgets
	assert.NotNil(t, sm.(*SettingsManager).allWidgets["enableNotifications"])
	assert.NotNil(t, sm.(*SettingsManager).allWidgets["username"])
	assert.NotNil(t, sm.(*SettingsManager).allWidgets["resetDatabase"])

	// Check widget types
	check := sm.(*SettingsManager).allWidgets["enableNotifications"].(*widget.Check)
	assert.True(t, check.Checked)

	entry := sm.(*SettingsManager).allWidgets["username"].(*widget.Entry)
	assert.Equal(t, "Karl", entry.Text)

	// Verify Validator translation
	assert.NoError(t, entry.Validator("ValidName"))
	assert.Error(t, entry.Validator("Hi")) // Too short

	btn := sm.(*SettingsManager).allWidgets["resetDatabase"].(*widget.Button)
	assert.Equal(t, widget.DangerImportance, btn.Importance)
}

func TestSecretItemStateTransitions(t *testing.T) {
	app := test.NewApp()
	testWin := app.NewWindow("SecretTest")
	sm := NewSettingsManager(testWin)

	apiKey := ""
	panel := schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title: "Security",
				Items: []schema.ItemSchema{
					schema.SecretItem{
						Name:  "myKey",
						Label: "API Key",
						OnVerify: func(key string) error {
							if key == "valid" {
								apiKey = key
								return nil
							}
							return fmt.Errorf("invalid key")
						},
						OnClear: func() {
							apiKey = ""
							sm.ResetSettings(setting.SettingReset{Name: "myKey", Value: ""})
						},
					},
				},
			},
		},
	}

	// 1. Initial Render (Empty State)
	_ = sm.RenderSchema(panel)
	assert.Equal(t, "", sm.GetBaseline("myKey"), "Baseline should be seeded as empty initially")

	// 2. Mock a valid Save
	// In the real UI, the user types "valid" and hits "Save", which calls OnVerify then CommitSetting.
	err := panel.Sections[0].Items[0].(schema.SecretItem).OnVerify("valid")
	assert.NoError(t, err)

	sm.(*SettingsManager).valueGetters["myKey"] = func() interface{} { return "valid" }
	sm.CommitSetting("myKey")

	assert.Equal(t, "valid", sm.GetBaseline("myKey"), "Baseline should be 'valid' after commit")
	assert.Equal(t, "valid", apiKey, "Provider variable should be updated via OnVerify side-effect")

	// 3. Clear the key
	// This would be triggered by the "Clear" button in the SAVED state branch.
	// Since RenderSchema is declarative, calling sm.RefreshUI() and re-rendering would show the Save UI again.
	sm.ResetSettings(setting.SettingReset{Name: "myKey", Value: ""})
	assert.Equal(t, "", sm.GetBaseline("myKey"), "Baseline should be empty after reset")
}

// spyDesktopWindow wraps a test window and records RequestPosition calls.
// desktop.Window requires all three methods below.
type spyDesktopWindow struct {
	fyne.Window
	posCalled  bool
	posX, posY int
}

func (s *spyDesktopWindow) RequestPosition(x, y int)    { s.posCalled, s.posX, s.posY = true, x, y }
func (s *spyDesktopWindow) RequestFullScreenSecondary() {}
func (s *spyDesktopWindow) RequestAlwaysOnTop()         {}

var _ desktop.Window = (*spyDesktopWindow)(nil)

func TestCenterWindowOnMonitor(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	spy := &spyDesktopWindow{Window: app.NewWindow("t")}
	centerWindowOnMonitor(spy, image.Rect(1920, 0, 4480, 1440))
	assert.True(t, spy.posCalled)
	assert.Equal(t, 1984, spy.posX)
	assert.Equal(t, 64, spy.posY)

	spy2 := &spyDesktopWindow{Window: app.NewWindow("t2")}
	centerWindowOnMonitor(spy2, image.Rectangle{})
	assert.False(t, spy2.posCalled)
}

func TestQueryListRendering_FullHeightInSingleVScroll(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	testWin := app.NewWindow("QueryListTest")
	sm := NewSettingsManager(testWin)

	queries := []schema.Query{
		{ID: "q1", Description: "Query 1", URL: "https://wallhaven.cc/1", Active: true},
		{ID: "q2", Description: "Query 2", URL: "https://wallhaven.cc/2", Active: false},
		{ID: "q3", Description: "Query 3", URL: "https://wallhaven.cc/3", Active: true},
		{ID: "q4", Description: "Query 4", URL: "https://wallhaven.cc/4", Active: true},
	}

	buildPanel := func() schema.PanelSchema {
		return schema.PanelSchema{
			Sections: []schema.SectionSchema{
				{
					Title: "Wallhaven Settings",
					Items: []schema.ItemSchema{
						schema.BoolItem{
							Name:         "sketchy",
							Label:        "Include Sketchy",
							InitialValue: false,
						},
						schema.QueryListItem{
							ID: "queries",
							GetQueries: func() []schema.Query {
								return queries
							},
							EnableQuery: func(id string) error {
								for i := range queries {
									if queries[i].ID == id {
										queries[i].Active = true
									}
								}
								return nil
							},
							DisableQuery: func(id string) error {
								for i := range queries {
									if queries[i].ID == id {
										queries[i].Active = false
									}
								}
								return nil
							},
							RemoveQuery: func(id string) error {
								for i := range queries {
									if queries[i].ID == id {
										queries = append(queries[:i], queries[i+1:]...)
										break
									}
								}
								return nil
							},
						},
					},
				},
			},
		}
	}

	rendered4 := sm.RenderSchema(buildPanel())
	scroll4, ok := rendered4.(*container.Scroll)
	assert.True(t, ok, "Panel with QueryListItem must be wrapped in a single outer VScroll")

	heightWith4 := scroll4.Content.MinSize().Height

	// Compare against 1 query row: 4 query rows must have a substantially larger content MinSize.Height
	queries = queries[:1]
	sm1 := NewSettingsManager(testWin)
	rendered1 := sm1.RenderSchema(buildPanel())
	scroll1 := rendered1.(*container.Scroll)
	heightWith1 := scroll1.Content.MinSize().Height

	assert.Greater(t, heightWith4, heightWith1+60, "QueryListWidget MinSize.Height must scale with all rows instead of collapsing to 1 row")
}

// newTestQueryPanel builds a single QueryListItem panel backed by an in-memory query slice.
func newTestQueryPanel(queries *[]schema.Query, removed *[]string, enabled, disabled *[]string) schema.PanelSchema {
	return schema.PanelSchema{Sections: []schema.SectionSchema{{Items: []schema.ItemSchema{
		schema.QueryListItem{
			ID:         "queries",
			GetQueries: func() []schema.Query { return append([]schema.Query(nil), (*queries)...) },
			EnableQuery: func(id string) error {
				*enabled = append(*enabled, id)
				return nil
			},
			DisableQuery: func(id string) error {
				*disabled = append(*disabled, id)
				return nil
			},
			RemoveQuery: func(id string) error {
				*removed = append(*removed, id)
				for i := range *queries {
					if (*queries)[i].ID == id {
						*queries = append((*queries)[:i], (*queries)[i+1:]...)
						break
					}
				}
				return nil
			},
		},
	}}}}
}

func TestQueryListRendering_LongDescriptionDoesNotWidenPanel(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	w := app.NewWindow("QueryListWidth")

	measure := func(desc string) float32 {
		queries := []schema.Query{{ID: "q1", Description: desc, URL: "https://wallhaven.cc/1", Active: true}}
		var removed, enabled, disabled []string
		return NewSettingsManager(w).RenderSchema(newTestQueryPanel(&queries, &removed, &enabled, &disabled)).MinSize().Width
	}

	short := measure("Cats")
	long := measure(strings.Repeat("W", 100)) // 100 chars == max description length
	assert.InDelta(t, short, long, 1, "Long query descriptions must be truncated, not inflate the panel/window minimum width")
}

func TestQueryListRendering_ToggleStagesAndDeleteRebuilds(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	w := app.NewWindow("QueryListInteract")
	smi := NewSettingsManager(w)
	sm := smi.(*SettingsManager)

	queries := []schema.Query{
		{ID: "q1", Description: "Query 1", URL: "https://wallhaven.cc/1", Active: true},
		{ID: "q2", Description: "Query 2", URL: "https://wallhaven.cc/2", Active: false},
	}
	var removed, enabled, disabled []string
	smi.RenderSchema(newTestQueryPanel(&queries, &removed, &enabled, &disabled))

	ql := sm.allWidgets["queries"].(*queryListWidget)
	assert.Len(t, ql.rows, 2)
	assert.True(t, ql.rows[0].activeCheck.Checked)
	assert.False(t, ql.rows[1].activeCheck.Checked)

	// Toggle q2 on: must be STAGED (no domain call) and light up Apply.
	firstCheck := ql.rows[1].activeCheck
	test.Tap(firstCheck)
	assert.True(t, sm.HasPendingChange("q2"), "Toggling Active must stage a pending change")
	assert.Empty(t, enabled, "EnableQuery must not be called before Apply")
	assert.False(t, sm.GetApplySettingsButton().Disabled(), "Apply must be enabled after staging a toggle")

	// A UI refresh with unchanged query IDs must update rows in place (same widget instances)
	// and preserve the staged checkbox state.
	sm.RefreshUI()
	assert.Same(t, firstCheck, ql.rows[1].activeCheck, "Rows must be rebound in place when query IDs are unchanged")
	assert.True(t, ql.rows[1].activeCheck.Checked, "Staged toggle must survive RefreshUI")

	// Apply commits the staged toggle.
	test.Tap(sm.GetApplySettingsButton())
	assert.Equal(t, []string{"q2"}, enabled)

	// Delete q1 via its button + confirm dialog: list must rebuild to 1 row.
	test.Tap(ql.rows[0].deleteButton)
	overlays := w.Canvas().Overlays().List()
	assert.NotEmpty(t, overlays, "Delete must show a confirmation dialog")
	confirmBtn := findButtonByText(overlays[len(overlays)-1], "Yes")
	if assert.NotNil(t, confirmBtn, "Confirmation dialog must have a Yes button") {
		test.Tap(confirmBtn)
	}
	assert.Equal(t, []string{"q1"}, removed)
	assert.Len(t, ql.rows, 1)
	assert.Equal(t, "q2", ql.rows[0].id)
}

// findButtonByText walks a canvas object tree and returns the first button with the given text.
func findButtonByText(obj fyne.CanvasObject, text string) *widget.Button {
	if b, ok := obj.(*widget.Button); ok && b.Text == text {
		return b
	}
	var children []fyne.CanvasObject
	switch o := obj.(type) {
	case *fyne.Container:
		children = o.Objects
	case fyne.Widget:
		children = test.WidgetRenderer(o).Objects()
	}
	for _, c := range children {
		if b := findButtonByText(c, text); b != nil {
			return b
		}
	}
	return nil
}
