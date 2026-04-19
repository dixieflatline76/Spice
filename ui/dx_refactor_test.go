package ui

import (
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/stretchr/testify/assert"
)

func TestCommitSetting(t *testing.T) {
	// Initialize test app and window
	app := test.NewApp()
	defer test.NewApp() // cleanup
	testWin := app.NewWindow("Test")

	sm := NewSettingsManager(testWin)
	header := container.NewVBox()

	// 1. Create a setting
	initialVal := "Original Baseline"
	appliedVal := ""

	sm.CreateTextEntrySetting(&setting.TextEntrySettingConfig{
		Name:         "testKey",
		InitialValue: initialVal,
		ApplyFunc: func(s string) {
			appliedVal = s
		},
	}, header)

	// 2. Modify value in UI
	newVal := "New Value to Commit"
	sm.SetValue("testKey", newVal)

	// Since SetValue doesn't trigger the change callback (it's for programmatic sync),
	// we need to simulate the UI callback.
	entry := sm.(*SettingsManager).widgets["testKey"].(*widget.Entry)
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
	header := container.NewVBox()

	appliedVal := "Initial"
	sm.CreateTextEntrySetting(&setting.TextEntrySettingConfig{
		Name:         "testKey",
		InitialValue: "Initial",
		ApplyFunc: func(s string) {
			appliedVal = s
		},
	}, header)

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
	header := container.NewVBox()

	var callCount int32
	blocker := make(chan bool)

	btn := sm.CreateAsyncButton(&setting.AsyncButtonConfig{
		Name:        "asyncTest",
		ButtonText:  "Execute",
		LoadingText: "Loading...",
		OnPressed: func() error {
			atomic.AddInt32(&callCount, 1)
			<-blocker // Wait for manual release
			return nil
		},
		OnCompleted: func(err error) {},
	}, header)

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
	header := container.NewVBox()

	baseline := "ValidKey"
	sm.CreateTextEntrySetting(&setting.TextEntrySettingConfig{
		Name:         "wallhavenAPIKey",
		InitialValue: baseline,
		ApplyFunc:    func(s string) {},
	}, header)

	verifyBtn := sm.CreateAsyncButton(&setting.AsyncButtonConfig{
		Name:       "verify",
		ButtonText: "Verify",
		VisibleIf: func() bool {
			curr := sm.GetValue("wallhavenAPIKey").(string)
			base := sm.GetBaseline("wallhavenAPIKey").(string)
			return curr != base || curr == ""
		},
	}, header)

	clearBtn := sm.CreateAsyncButton(&setting.AsyncButtonConfig{
		Name:       "clear",
		ButtonText: "Clear",
		VisibleIf: func() bool {
			curr := sm.GetValue("wallhavenAPIKey").(string)
			base := sm.GetBaseline("wallhavenAPIKey").(string)
			return curr == base && curr != ""
		},
	}, header)

	// Initial State: baseline matches live -> Clear visible, Verify hidden
	sm.Refresh()
	assert.True(t, clearBtn.Visible(), "Clear button should be visible initially")
	assert.False(t, verifyBtn.Visible(), "Verify button should be hidden initially")

	// Transition 1: User types (live != baseline)
	sm.SetValue("wallhavenAPIKey", "ValidKeyX")
	sm.Refresh()
	assert.False(t, clearBtn.Visible(), "Clear button should hide when editing")
	assert.True(t, verifyBtn.Visible(), "Verify button should show when editing")

	// Transition 2: User reverts (live == baseline)
	sm.SetValue("wallhavenAPIKey", "ValidKey")
	sm.Refresh()
	assert.True(t, clearBtn.Visible(), "Clear button should reappear on revert")
	assert.False(t, verifyBtn.Visible(), "Verify button should hide on revert")
}

func TestApplyTriggersRefresh(t *testing.T) {
	app := test.NewApp()
	testWin := app.NewWindow("Test Apply Refresh")
	sm := NewSettingsManager(testWin)
	header := container.NewVBox()

	refreshCount := 0
	sm.RegisterRefreshFunc(func() {
		refreshCount++
	})

	// Create a setting with NeedsRefresh: true
	sm.CreateBoolSetting(&setting.BoolConfig{
		Name:         "triggerRefresh",
		InitialValue: false,
		NeedsRefresh: true,
		ApplyFunc:    func(b bool) {},
	}, header)

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
	header := container.NewVBox()

	// 1. Create a text setting with status enabled
	sm.CreateTextEntrySetting(&setting.TextEntrySettingConfig{
		Name:          "targetField",
		InitialValue:  "Initial",
		DisplayStatus: true,
	}, header)

	// 2. Create an async button targeting that field
	sm.CreateAsyncButton(&setting.AsyncButtonConfig{
		Name:            "testBtn",
		ButtonText:      "Verify",
		TargetStatusKey: "targetField",
		OnPressed: func() error {
			time.Sleep(50 * time.Millisecond)
			return nil // Signal success
		},
		OnCompleted: func(err error) {},
	}, header)

	// Verify initial status: Empty
	statusLabel := sm.(*SettingsManager).statusLabels["targetField"]
	assert.Equal(t, "", statusLabel.Text)

	// Simulate Button Click
	// Tapping our async button specifically
	testBtn, ok := sm.(*SettingsManager).widgets["testBtn"].(*widget.Button)
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
	header := container.NewVBox()

	sm.CreateTextEntrySetting(&setting.TextEntrySettingConfig{
		Name:          "safetyTarget",
		InitialValue:  "",
		DisplayStatus: true,
	}, header)

	statusLabel := sm.(*SettingsManager).statusLabels["safetyTarget"]

	// Launch background status update
	done := make(chan bool)
	go func() {
		sm.SetSettingStatus("safetyTarget", "External Update", widget.WarningImportance)
		done <- true
	}()

	<-done
	// Give Fyne's event loop a tiny bit of time
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, "External Update", statusLabel.Text)
	assert.Equal(t, widget.WarningImportance, statusLabel.Importance)
}
