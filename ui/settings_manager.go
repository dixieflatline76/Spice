package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
)

// SettingsManager handles UI elements for settings.
type SettingsManager struct {
	chgPrefsCallbacks   map[string]func()
	refreshFlags        map[string]bool
	refreshFuncs        []func()
	registry            map[string]interface{}
	valueGetters        map[string]func() interface{}
	checkAndEnableApply func()
	applyButton         *widget.Button
	prefsWindow         fyne.Window
	onSettingsSaved     []func()
	managedWidgets      []managedWidget // Track widgets with EnabledIf conditions
	widgets             map[string]fyne.CanvasObject
}

type managedWidget struct {
	widget    fyne.Disableable
	enabledIf func() bool
}

// NewSettingsManager creates a new SettingsManager.
func NewSettingsManager(window fyne.Window) setting.SettingsManager {
	cpcs := make(map[string]func())
	rns := make(map[string]bool)
	rfs := make([]func(), 0)

	sm := &SettingsManager{
		chgPrefsCallbacks:   cpcs,
		refreshFlags:        rns,
		refreshFuncs:        rfs,
		registry:            make(map[string]interface{}),
		valueGetters:        make(map[string]func() interface{}),
		checkAndEnableApply: nil,
		applyButton:         nil,
		prefsWindow:         window,
		managedWidgets:      make([]managedWidget, 0),
		widgets:             make(map[string]fyne.CanvasObject),
	}

	sm.applyButton = createApplyButton(sm)
	sm.checkAndEnableApply = func() {
		// Evaluate UI dependencies first
		sm.refreshWidgetStates()

		if len(sm.refreshFlags) > 0 || len(sm.chgPrefsCallbacks) > 0 {
			sm.applyButton.Enable() // Enable if changes or refresh needed
		} else {
			sm.applyButton.Disable() // Otherwise, disable
		}
		sm.applyButton.Refresh()
	}

	return sm
}

// SeedBaseline seeds the initial state for a setting to track changes.
func (sm *SettingsManager) SeedBaseline(name string, val interface{}) {
	sm.registry[name] = val
}

// refreshWidgetStates evaluates all EnabledIf conditions and updates widget states.
func (sm *SettingsManager) refreshWidgetStates() {
	for _, mw := range sm.managedWidgets {
		if mw.enabledIf != nil {
			if mw.enabledIf() {
				mw.widget.Enable()
			} else {
				mw.widget.Disable()
			}
		}
	}
}

// GetValue returns the live/current value for a setting from its valueGetter.
func (sm *SettingsManager) GetValue(name string) interface{} {
	val, ok := sm.valueGetters[name]
	if !ok || val == nil {
		return nil
	}
	return val()
}

// SetValue programmatically updates the live value of a setting.
func (sm *SettingsManager) SetValue(name string, val interface{}) {
	w, ok := sm.widgets[name]
	if !ok {
		return
	}

	switch v := w.(type) {
	case *widget.Check:
		if b, ok := val.(bool); ok {
			v.SetChecked(b)
		}
	case *widget.Entry:
		if s, ok := val.(string); ok {
			v.SetText(s)
		}
	case *widget.Select:
		if i, ok := val.(int); ok {
			v.SetSelectedIndex(i)
		}
	}

	sm.refreshWidgetStates()
}

// GetBaseline returns the initial state for a setting.
func (sm *SettingsManager) GetBaseline(name string) interface{} {
	return sm.registry[name]
}

// HasPendingChange returns true if the user has toggled a setting but not yet applied.
func (sm *SettingsManager) HasPendingChange(name string) bool {
	_, exists := sm.chgPrefsCallbacks[name]
	return exists
}

// CreateApplyButton is a helper function that creates and sets up the Apply Changes button.
func createApplyButton(sm *SettingsManager) *widget.Button {
	var applyButton *widget.Button
	applyButton = widget.NewButton("Apply Changes", func() {
		originalText := applyButton.Text
		sm.applyButton.Disable()
		sm.applyButton.SetText("Applying changes, please wait...")
		sm.applyButton.Refresh()
		fyne.Do(func() {
			defer func() {
				// Rebuild tray menu (if needed)
				// sm.RebuildTrayMenu() // This function does not exist in the provided code, commenting out.

				// Notify listeners that settings have been saved (e.g. refresh UI counters)
				for _, cb := range sm.onSettingsSaved {
					cb()
				}

				// Disable the Apply button after saving
				if sm.applyButton != nil {
					sm.applyButton.Disable()
				}
				sm.applyButton.SetText(originalText)
				sm.applyButton.Refresh()
			}()

			if len(sm.chgPrefsCallbacks) > 0 {
				for name, callback := range sm.chgPrefsCallbacks {
					callback()
					// Update registry baseline so subsequent changes are compared to NEW state
					if getter, ok := sm.valueGetters[name]; ok {
						sm.registry[name] = getter()
					}
				}
				sm.chgPrefsCallbacks = make(map[string]func())
			}

			if len(sm.refreshFlags) > 0 && len(sm.refreshFuncs) > 0 {
				for _, rf := range sm.refreshFuncs {
					rf()
				}
				sm.refreshFlags = make(map[string]bool)
			}
		})
	})
	applyButton.Disable()
	return applyButton
}

// GetApplySettingsButton returns the Apply Changes button from the SettingsManager to be used in the UI.
func (sm *SettingsManager) GetApplySettingsButton() *widget.Button {
	return sm.applyButton
}

// CreateSelectSetting creates a reusable select widget.
func (sm *SettingsManager) CreateSelectSetting(cfg *setting.SelectConfig, header *fyne.Container) {
	selectWidget := widget.NewSelect(cfg.Options, func(selected string) {})
	selectWidget.SetSelectedIndex(cfg.InitialValue.(int))
	sm.registry[cfg.Name] = cfg.InitialValue.(int)
	sm.valueGetters[cfg.Name] = func() interface{} {
		return selectWidget.SelectedIndex()
	}

	header.Add(NewSplitRow(cfg.Label, selectWidget, SplitProportion.OneThird))
	if cfg.HelpContent != nil {
		header.Add(cfg.HelpContent)
	}

	selectWidget.OnChanged = func(s string) {
		options := cfg.Options
		selectedIndex := -1
		for i, opt := range options {
			if opt == s { // Assuming cfg.Options are strings
				selectedIndex = i
				break
			}
		}

		if selectedIndex != sm.registry[cfg.Name].(int) {
			sm.chgPrefsCallbacks[cfg.Name] = func() {
				cfg.ApplyFunc(selectedIndex)
				if cfg.NeedsRefresh {
					sm.refreshFlags[cfg.Name] = true
				}
			}
		} else {
			delete(sm.chgPrefsCallbacks, cfg.Name)
		}

		if cfg.OnChanged != nil {
			cfg.OnChanged(s, selectedIndex)
		}

		sm.checkAndEnableApply()
	}

	// Register value getter for dependency tracking
	sm.valueGetters[cfg.Name] = func() interface{} {
		s := selectWidget.Selected
		options := cfg.Options
		for i, opt := range options {
			if opt == s { // Assuming cfg.Options are strings
				return i
			}
		}
		return -1
	}

	// Track if it has an EnabledIf condition
	if cfg.EnabledIf != nil {
		sm.managedWidgets = append(sm.managedWidgets, managedWidget{
			widget:    selectWidget,
			enabledIf: cfg.EnabledIf,
		})
	}

	sm.widgets[cfg.Name] = selectWidget
}

// CreateBoolSetting creates a reusable boolean check setting.
func (sm *SettingsManager) CreateBoolSetting(cfg *setting.BoolConfig, header *fyne.Container) *widget.Check {
	check := widget.NewCheck("", nil) // Use empty string, label is CanvasObject
	check.SetChecked(cfg.InitialValue)
	sm.registry[cfg.Name] = cfg.InitialValue

	label := cfg.Label
	if label == nil {
		label = widget.NewLabel("")
	}

	header.Add(NewSplitRow(label, check, SplitProportion.OneThird))
	if cfg.HelpContent != nil {
		header.Add(cfg.HelpContent)
	}

	check.OnChanged = func(b bool) {
		if b != sm.registry[cfg.Name].(bool) {
			sm.chgPrefsCallbacks[cfg.Name] = func() {
				cfg.ApplyFunc(b)
				if cfg.NeedsRefresh {
					sm.refreshFlags[cfg.Name] = true
				}
			}
		} else {
			delete(sm.chgPrefsCallbacks, cfg.Name)
		}

		if cfg.OnChanged != nil {
			cfg.OnChanged(b)
		}

		sm.checkAndEnableApply()
	}

	// Register value getter for dependency tracking
	sm.valueGetters[cfg.Name] = func() interface{} {
		return check.Checked
	}

	// Track if it has an EnabledIf condition
	if cfg.EnabledIf != nil {
		sm.managedWidgets = append(sm.managedWidgets, managedWidget{
			widget:    check,
			enabledIf: cfg.EnabledIf,
		})
	}

	sm.widgets[cfg.Name] = check
	return check
}

// CreateTextEntrySetting creates a reusable text entry setting.
func (sm *SettingsManager) CreateTextEntrySetting(cfg *setting.TextEntrySettingConfig, header *fyne.Container) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(cfg.PlaceHolder)
	entry.SetText(cfg.InitialValue)
	sm.registry[cfg.Name] = cfg.InitialValue
	sm.valueGetters[cfg.Name] = func() interface{} {
		return entry.Text
	}
	sm.widgets[cfg.Name] = entry

	if cfg.IsPassword {
		entry.Password = true
	}

	if cfg.Validator != nil {
		entry.Validator = cfg.Validator
	}

	label := cfg.Label
	if label == nil {
		label = widget.NewLabel("")
	}

	statusLabel := widget.NewLabel("")

	header.Add(NewSplitRow(label, entry, SplitProportion.OneThird))
	if cfg.HelpContent != nil {
		header.Add(NewSplitRowWithAlignment(cfg.HelpContent, statusLabel, SplitProportion.TwoThirds, SplitAlign.Opposed))
	} else {
		header.Add(NewSplitRow(widget.NewLabel(""), statusLabel, SplitProportion.TwoThirds))
	}

	var debounceTimer *time.Timer
	entry.OnChanged = func(s string) {
		sm.handleTextEntryChanged(s, cfg, entry, statusLabel, &debounceTimer)
	}

	// Track if it has an EnabledIf condition
	if cfg.EnabledIf != nil {
		sm.managedWidgets = append(sm.managedWidgets, managedWidget{
			widget:    entry,
			enabledIf: cfg.EnabledIf,
		})
	}

	return entry
}

func (sm *SettingsManager) handleTextEntryChanged(s string, cfg *setting.TextEntrySettingConfig, entry *widget.Entry, statusLabel *widget.Label, debounceTimer **time.Timer) {
	if *debounceTimer != nil {
		(*debounceTimer).Stop()
	}

	var entryErr error
	if cfg.Validator != nil {
		entryErr = entry.Validate()
	}

	if entryErr != nil {
		statusLabel.SetText(entryErr.Error())
		statusLabel.Importance = widget.DangerImportance
		sm.RemoveSettingChangedCallback(cfg.Name)
		if cfg.NeedsRefresh {
			sm.UnsetRefreshFlag(cfg.Name)
		}
		sm.GetCheckAndEnableApplyFunc()()
		statusLabel.Refresh()
		if cfg.OnChanged != nil {
			cfg.OnChanged(s)
		}
		return
	}

	// Validation passed (or none), clear status and importance while waiting/running post-check
	statusLabel.SetText("")
	statusLabel.Importance = widget.LowImportance
	statusLabel.Refresh()

	runPostCheck := func(val string) {
		sm.runTextEntryPostCheck(val, cfg, entry, statusLabel)
	}

	if cfg.ValidationDebounce > 0 {
		*debounceTimer = time.AfterFunc(cfg.ValidationDebounce, func() {
			runPostCheck(s)
		})
	} else {
		runPostCheck(s)
	}

	if cfg.OnChanged != nil {
		cfg.OnChanged(s)
	}
}

func (sm *SettingsManager) runTextEntryPostCheck(val string, cfg *setting.TextEntrySettingConfig, entry *widget.Entry, statusLabel *widget.Label) {
	var postErr error
	if cfg.PostValidateCheck != nil {
		postErr = cfg.PostValidateCheck(val)
	}

	fyne.Do(func() {
		// Ensure the value hasn't changed since we started this check
		if entry.Text != val {
			return
		}

		if postErr != nil {
			statusLabel.SetText(postErr.Error())
			statusLabel.Importance = widget.DangerImportance
			sm.RemoveSettingChangedCallback(cfg.Name)
			if cfg.NeedsRefresh {
				sm.UnsetRefreshFlag(cfg.Name)
			}
		} else {
			if cfg.DisplayStatus && val != "" {
				statusLabel.SetText(fmt.Sprintf("%s OK", cfg.Name))
				statusLabel.Importance = widget.SuccessImportance
			} else {
				statusLabel.SetText("")
				statusLabel.Importance = widget.LowImportance
			}

			if val != sm.registry[cfg.Name].(string) {
				sm.SetSettingChangedCallback(cfg.Name, func() {
					enteredTxt := entry.Text
					if enteredTxt != sm.registry[cfg.Name].(string) {
						cfg.ApplyFunc(enteredTxt)
					}
				})
				if cfg.NeedsRefresh {
					sm.SetRefreshFlag(cfg.Name)
				}
			} else {
				sm.RemoveSettingChangedCallback(cfg.Name)
				if cfg.NeedsRefresh {
					sm.UnsetRefreshFlag(cfg.Name)
				}
			}
		}
		statusLabel.Refresh()
		sm.GetCheckAndEnableApplyFunc()()
	})
}

// CreateButtonWithConfirmationSetting creates a reusable button setting with confirmation dialog.
func (sm *SettingsManager) CreateButtonWithConfirmationSetting(cfg *setting.ButtonWithConfirmationConfig, header *fyne.Container) {
	button := widget.NewButton(cfg.ButtonText, func() {
		if cfg.ConfirmTitle != "" && cfg.ConfirmMessage != "" {
			d := dialog.NewConfirm(cfg.ConfirmTitle, cfg.ConfirmMessage, func(b bool) {
				if b {
					cfg.OnPressed()
				}
			}, sm.prefsWindow)
			d.Show()
		} else {
			cfg.OnPressed()
		}
	})

	if cfg.Label != nil {
		header.Add(NewSplitRow(cfg.Label, button, SplitProportion.OneThird))
	} else {
		header.Add(button)
	}

	if cfg.HelpContent != nil {
		header.Add(cfg.HelpContent)
	}
}

// SetSettingChangedCallback sets a callback function to be called when a setting changes.
func (sm *SettingsManager) SetSettingChangedCallback(settingName string, callback func()) {
	sm.chgPrefsCallbacks[settingName] = callback
}

// RemoveSettingChangedCallback removes a callback function associated with a specific setting.
func (sm *SettingsManager) RemoveSettingChangedCallback(settingName string) {
	delete(sm.chgPrefsCallbacks, settingName)
}

// SetRefreshFlag sets a flag to indicate that a specific setting needs a refresh.
func (sm *SettingsManager) SetRefreshFlag(settingName string) {
	sm.refreshFlags[settingName] = true
}

// UnsetRefreshFlag removes the refresh flag for a specific setting.
func (sm *SettingsManager) UnsetRefreshFlag(settingName string) {
	delete(sm.refreshFlags, settingName)
}

// RegisterOnSettingsSaved registers a function to be called after settings are applied.
func (sm *SettingsManager) RegisterOnSettingsSaved(callback func()) {
	sm.onSettingsSaved = append(sm.onSettingsSaved, callback)
}

// RegisterRefreshFunc registers a function to be called when the settings need to be refreshed.
// This should be done by each plugin that needs to perform some action after settings changes.
// Like refreshing all wallpaper images for example.
func (sm *SettingsManager) RegisterRefreshFunc(refreshFunc func()) {
	sm.refreshFuncs = append(sm.refreshFuncs, refreshFunc)
}

// GetSettingsWindow returns the window associated with the SettingsManager.
func (sm *SettingsManager) GetSettingsWindow() fyne.Window {
	return sm.prefsWindow
}

// GetCheckAndEnableApplyFunc returns the check and enable apply function for the SettingsManager.
func (sm *SettingsManager) GetCheckAndEnableApplyFunc() func() {
	return sm.checkAndEnableApply
}

// RebuildTrayMenu rebuilds the tray menu list from scratch.
func (sm *SettingsManager) RebuildTrayMenu() {
	getInstance().RebuildTrayMenu()
}

// Refresh triggers all registered refresh functions immediately.
func (sm *SettingsManager) Refresh() {
	// 1. Evaluate all UI dependencies (EnabledIf)
	sm.refreshWidgetStates()

	// 2. Trigger registered manual refresh functions (e.g. table refreshes)
	for _, rf := range sm.refreshFuncs {
		rf()
	}

	// 3. Refresh all managed widget objects themselves
	for _, w := range sm.widgets {
		w.Refresh()
	}
}
