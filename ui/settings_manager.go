package ui

import (
	"fmt"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
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
	statusLabels        map[string]*widget.Label     // NEW: Maps setting names to their status labels
	applyFuncs          map[string]func(interface{}) // Type-safe wrappers for native apply functions
}

type managedWidget struct {
	widget    fyne.CanvasObject
	enabledIf func() bool
	visibleIf func() bool
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
		onSettingsSaved:     make([]func(), 0),
		managedWidgets:      make([]managedWidget, 0),
		widgets:             make(map[string]fyne.CanvasObject),
		statusLabels:        make(map[string]*widget.Label),
		applyFuncs:          make(map[string]func(interface{})),
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

// refreshWidgetStates evaluates all EnabledIf and VisibleIf conditions and updates widget states.
func (sm *SettingsManager) refreshWidgetStates() {
	for _, mw := range sm.managedWidgets {
		// Handle Visibility
		if mw.visibleIf != nil {
			if mw.visibleIf() {
				mw.widget.Show()
			} else {
				mw.widget.Hide()
			}
		}

		// Handle Enabling (requires fyne.Disableable)
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

	fyne.Do(func() {
		switch v := w.(type) {
		case *widget.Check:
			if b, ok := val.(bool); ok {
				v.SetChecked(b)
				v.Refresh()
			}
		case *widget.Entry:
			if s, ok := val.(string); ok {
				v.SetText(s)
				v.Refresh()
			}
		case *widget.Select:
			if i, ok := val.(int); ok {
				v.SetSelectedIndex(i)
				v.Refresh()
			} else if s, ok := val.(string); ok {
				v.SetSelected(s)
				v.Refresh()
			}
		}

		sm.refreshWidgetStates()
	})
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
	applyButton = widget.NewButton(i18n.T("Apply Changes"), func() {
		originalText := applyButton.Text
		sm.applyButton.Disable()
		sm.applyButton.SetText(i18n.T("Applying changes, please wait..."))
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

			// Conditional refresh based on dirty flags (evaluated during ApplyFuncs above)
			if len(sm.refreshFlags) > 0 {
				sm.Refresh()
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
	sm.applyFuncs[cfg.Name] = func(val interface{}) {
		switch v := val.(type) {
		case int:
			cfg.ApplyFunc(v)
		case string:
			for i, opt := range cfg.Options {
				if opt == v {
					cfg.ApplyFunc(i)
					return
				}
			}
		}
	}
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

	// Track if it has an EnabledIf or VisibleIf condition
	if cfg.EnabledIf != nil || cfg.VisibleIf != nil {
		sm.managedWidgets = append(sm.managedWidgets, managedWidget{
			widget:    selectWidget,
			enabledIf: cfg.EnabledIf,
			visibleIf: cfg.VisibleIf,
		})
	}

	sm.widgets[cfg.Name] = selectWidget
}

// CreateBoolSetting creates a reusable boolean check setting.
func (sm *SettingsManager) CreateBoolSetting(cfg *setting.BoolConfig, header *fyne.Container) *widget.Check {
	check := widget.NewCheck("", nil) // Use empty string, label is CanvasObject
	check.SetChecked(cfg.InitialValue)
	sm.registry[cfg.Name] = cfg.InitialValue
	sm.applyFuncs[cfg.Name] = func(val interface{}) {
		cfg.ApplyFunc(val.(bool))
	}

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

	// Track if it has an EnabledIf or VisibleIf condition
	if cfg.EnabledIf != nil || cfg.VisibleIf != nil {
		sm.managedWidgets = append(sm.managedWidgets, managedWidget{
			widget:    check,
			enabledIf: cfg.EnabledIf,
			visibleIf: cfg.VisibleIf,
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
	sm.applyFuncs[cfg.Name] = func(val interface{}) {
		cfg.ApplyFunc(val.(string))
	}
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
	sm.statusLabels[cfg.Name] = statusLabel

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

	// Track if it has an EnabledIf or VisibleIf condition
	if cfg.EnabledIf != nil || cfg.VisibleIf != nil {
		sm.managedWidgets = append(sm.managedWidgets, managedWidget{
			widget:    entry,
			enabledIf: cfg.EnabledIf,
			visibleIf: cfg.VisibleIf,
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

			baseline, _ := sm.registry[cfg.Name].(string)
			if val != baseline {
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
	button.Importance = cfg.Importance

	if cfg.Label != nil {
		header.Add(NewSplitRow(cfg.Label, button, SplitProportion.OneThird))
	} else {
		header.Add(button)
	}

	if cfg.HelpContent != nil {
		header.Add(cfg.HelpContent)
	}

	// Track if it has an EnabledIf or VisibleIf condition
	if cfg.EnabledIf != nil || cfg.VisibleIf != nil {
		sm.managedWidgets = append(sm.managedWidgets, managedWidget{
			widget:    button,
			enabledIf: cfg.EnabledIf,
			visibleIf: cfg.VisibleIf,
		})
	}
}

// CreateButtonSetting creates a standard button setting widget.
func (sm *SettingsManager) CreateButtonSetting(cfg *setting.ButtonConfig, header *fyne.Container) {
	button := widget.NewButton(cfg.ButtonText, cfg.OnPressed)
	button.Importance = cfg.Importance

	if cfg.Label != nil {
		header.Add(NewSplitRow(cfg.Label, button, SplitProportion.OneThird))
	} else {
		header.Add(button)
	}

	if cfg.HelpContent != nil {
		header.Add(cfg.HelpContent)
	}

	// Track if it has an EnabledIf or VisibleIf condition
	if cfg.EnabledIf != nil || cfg.VisibleIf != nil {
		sm.managedWidgets = append(sm.managedWidgets, managedWidget{
			widget:    button,
			enabledIf: cfg.EnabledIf,
			visibleIf: cfg.VisibleIf,
		})
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

// CreateAsyncButton creates a button that handles background tasks and UI thread transitions.
func (sm *SettingsManager) CreateAsyncButton(cfg *setting.AsyncButtonConfig, header *fyne.Container) *widget.Button {
	btn := widget.NewButton(cfg.ButtonText, nil)
	btn.Importance = cfg.Importance

	// Define the handler to capture 'btn' properly
	btn.OnTapped = func() {
		originalText := btn.Text
		btn.Disable()
		sm.applyButton.Disable()
		btn.SetText(cfg.LoadingText)
		btn.Refresh()

		go func() {
			err := cfg.OnPressed()
			fyne.Do(func() {
				btn.Enable()
				btn.SetText(originalText)
				sm.applyButton.Enable()

				// NEW: Automatic framework-managed status update
				if cfg.TargetStatusKey != "" {
					if err != nil {
						sm.SetSettingStatus(cfg.TargetStatusKey, err.Error(), setting.ImportanceDanger)
					} else {
						sm.SetSettingStatus(cfg.TargetStatusKey, i18n.T("Success"), setting.ImportanceSuccess)
					}
				}

				cfg.OnCompleted(err)
				if cfg.NeedsRefresh {
					sm.Refresh()
				} else {
					sm.refreshWidgetStates()
				}
				sm.checkAndEnableApply()
			})
		}()
	}

	header.Add(btn)
	sm.widgets[cfg.Name] = btn

	// Track if it has an EnabledIf or VisibleIf condition
	if cfg.EnabledIf != nil || cfg.VisibleIf != nil {
		sm.managedWidgets = append(sm.managedWidgets, managedWidget{
			widget:    btn,
			enabledIf: cfg.EnabledIf,
			visibleIf: cfg.VisibleIf,
		})
	}

	return btn
}

// CommitSetting atomically reads the current UI value, applies it to the native setter, and updates the baseline.
func (sm *SettingsManager) CommitSetting(name string) {
	val := sm.GetValue(name)
	if val == nil {
		return
	}

	if apply, ok := sm.applyFuncs[name]; ok {
		apply(val)
	}

	sm.SeedBaseline(name, val)
	sm.RemoveSettingChangedCallback(name)

	// Since Commit is usually triggered by a successful verification/on-press,
	// we always refresh to catch the visibility state change.
	sm.Refresh()
	sm.checkAndEnableApply()
}

// ResetSettings atomically clears multiple settings, updates native getters, and resyncs baselines.
func (sm *SettingsManager) ResetSettings(resets ...setting.SettingReset) {
	for _, r := range resets {
		// 1. Update native Fyne preference
		if apply, ok := sm.applyFuncs[r.Name]; ok {
			apply(r.Value)
		}
		// 2. Update Baseline
		sm.SeedBaseline(r.Name, r.Value)
		// 3. Update UI
		sm.SetValue(r.Name, r.Value)
		// 4. Remove pending
		sm.RemoveSettingChangedCallback(r.Name)
	}

	sm.Refresh()
	sm.checkAndEnableApply()
}

// SetSettingStatus programmatically updates a setting's status label (thread-safe).
func (sm *SettingsManager) SetSettingStatus(name string, message string, importance setting.Importance) {
	label, ok := sm.statusLabels[name]
	if !ok || label == nil {
		return
	}

	fyneImportance := widget.LowImportance
	switch importance {
	case setting.ImportanceHigh:
		fyneImportance = widget.HighImportance
	case setting.ImportanceMedium:
		fyneImportance = widget.MediumImportance
	case setting.ImportanceLow:
		fyneImportance = widget.LowImportance
	case setting.ImportanceSuccess:
		fyneImportance = widget.SuccessImportance
	case setting.ImportanceDanger:
		fyneImportance = widget.DangerImportance
	}

	fyne.Do(func() {
		label.SetText(message)
		label.Importance = fyneImportance
		label.Refresh()
	})
}

// RenderSchema takes a pure Go UI definition and renders it to a Fyne container.
func (sm *SettingsManager) RenderSchema(schema setting.PanelSchema) fyne.CanvasObject {
	mainBox := container.NewVBox()

	for _, section := range schema.Sections {
		sectionContainer := container.NewVBox()
		if section.Title != "" {
			sectionContainer.Add(sm.CreateSectionTitleLabel(section.Title))
		}
		if section.Description != "" {
			sectionContainer.Add(sm.CreateSettingDescriptionLabel(section.Description))
		}

		for _, item := range section.Items {
			switch v := item.(type) {
			case setting.BoolItem:
				sm.CreateBoolSetting(&setting.BoolConfig{
					Name:         v.Name,
					InitialValue: v.InitialValue,
					Label:        sm.CreateSettingTitleLabel(v.Label),
					HelpContent:  sm.CreateSettingDescriptionLabel(v.Help),
					OnChanged:    v.OnChanged,
					ApplyFunc:    v.ApplyFunc,
					NeedsRefresh: v.NeedsRefresh,
					EnabledIf:    v.EnabledIf,
					VisibleIf:    v.VisibleIf,
				}, sectionContainer)

			case setting.TextItem:
				var fyneValidator fyne.StringValidator
				if v.Validator != nil {
					fyneValidator = v.Validator
				}

				sm.CreateTextEntrySetting(&setting.TextEntrySettingConfig{
					Name:               v.Name,
					InitialValue:       v.InitialValue,
					PlaceHolder:        v.PlaceHolder,
					Label:              sm.CreateSettingTitleLabel(v.Label),
					HelpContent:        sm.CreateSettingDescriptionLabel(v.Help),
					Validator:          fyneValidator,
					OnChanged:          v.OnChanged,
					PostValidateCheck:  v.PostValidateCheck,
					ApplyFunc:          v.ApplyFunc,
					NeedsRefresh:       v.NeedsRefresh,
					DisplayStatus:      v.DisplayStatus,
					IsPassword:         v.IsPassword,
					EnabledIf:          v.EnabledIf,
					VisibleIf:          v.VisibleIf,
					ValidationDebounce: v.ValidationDebounce,
				}, sectionContainer)

			case setting.SelectItem:
				sm.CreateSelectSetting(&setting.SelectConfig{
					Name:         v.Name,
					Options:      v.Options,
					InitialValue: v.InitialValue,
					Label:        sm.CreateSettingTitleLabel(v.Label),
					HelpContent:  sm.CreateSettingDescriptionLabel(v.Help),
					OnChanged:    v.OnChanged,
					ApplyFunc:    v.ApplyFunc,
					NeedsRefresh: v.NeedsRefresh,
					EnabledIf:    v.EnabledIf,
					VisibleIf:    v.VisibleIf,
				}, sectionContainer)

			case setting.AsyncButtonItem:
				importance := widget.LowImportance
				switch v.Style {
				case setting.ButtonStylePrimary:
					importance = widget.HighImportance
				case setting.ButtonStyleDanger:
					importance = widget.DangerImportance
				case setting.ButtonStyleSuccess:
					importance = widget.SuccessImportance
				}

				sm.CreateAsyncButton(&setting.AsyncButtonConfig{
					Name:            v.Name,
					ButtonText:      v.ButtonText,
					LoadingText:     v.LoadingText,
					Importance:      importance,
					OnPressed:       v.OnPressed,
					OnCompleted:     v.OnCompleted,
					TargetStatusKey: v.TargetStatusKey,
					NeedsRefresh:    v.NeedsRefresh,
					EnabledIf:       v.EnabledIf,
					VisibleIf:       v.VisibleIf,
				}, sectionContainer)

			case setting.ConfirmButtonItem:
				fyneImportance := widget.LowImportance
				switch v.Importance {
				case setting.ImportanceHigh:
					fyneImportance = widget.HighImportance
				case setting.ImportanceMedium:
					fyneImportance = widget.MediumImportance
				case setting.ImportanceLow:
					fyneImportance = widget.LowImportance
				case setting.ImportanceSuccess:
					fyneImportance = widget.SuccessImportance
				case setting.ImportanceDanger:
					fyneImportance = widget.DangerImportance
				}

				sm.CreateButtonWithConfirmationSetting(&setting.ButtonWithConfirmationConfig{
					Name:           v.Name,
					ButtonText:     v.ButtonText,
					ConfirmTitle:   v.ConfirmTitle,
					ConfirmMessage: v.ConfirmMessage,
					Importance:     fyneImportance,
					OnPressed:      v.OnPressed,
					EnabledIf:      v.EnabledIf,
					VisibleIf:      v.VisibleIf,
					Label:          sm.CreateSettingTitleLabel(v.Label),
					HelpContent:    sm.CreateSettingDescriptionLabel(v.Help),
				}, sectionContainer)

			case setting.ButtonItem:
				fyneImportance := widget.MediumImportance
				switch v.Importance {
				case setting.ImportanceHigh:
					fyneImportance = widget.HighImportance
				case setting.ImportanceMedium:
					fyneImportance = widget.MediumImportance
				case setting.ImportanceLow:
					fyneImportance = widget.LowImportance
				case setting.ImportanceSuccess:
					fyneImportance = widget.SuccessImportance
				case setting.ImportanceDanger:
					fyneImportance = widget.DangerImportance
				}

				sm.CreateButtonSetting(&setting.ButtonConfig{
					Name:        v.Name,
					ButtonText:  v.ButtonText,
					Importance:  fyneImportance,
					OnPressed:   v.OnPressed,
					EnabledIf:   v.EnabledIf,
					VisibleIf:   v.VisibleIf,
					Label:       sm.CreateSettingTitleLabel(v.Label),
					HelpContent: sm.CreateSettingDescriptionLabel(v.Help),
				}, sectionContainer)

			case setting.HyperlinkItem:
				u, err := url.Parse(v.URL)
				if err == nil {
					sectionContainer.Add(widget.NewHyperlink(v.Text, u))
				} else {
					// Fallback for invalid URLs in schema: just show text
					sectionContainer.Add(widget.NewLabel(v.Text + " (" + v.URL + ")"))
				}

			case setting.LabelItem:
				if v.IsTitle {
					sectionContainer.Add(widget.NewLabelWithStyle(v.Text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
				} else {
					var content fyne.CanvasObject
					if v.Importance == setting.ImportanceLow {
						// Description style: Muted color using RichText
						rich := widget.NewRichTextWithText(v.Text)
						rich.Segments[0].(*widget.TextSegment).Style.ColorName = theme.ColorNamePlaceHolder
						rich.Wrapping = fyne.TextWrapWord
						content = rich
					} else {
						lbl := widget.NewLabel(v.Text)
						lbl.Wrapping = fyne.TextWrapWord
						content = lbl
					}

					// Standardized Indentation: matching Karl's preference for consistency
					padding := theme.Padding() * 3 // Normalized to 3x for consistency with legacy descriptions
					spacer := canvas.NewRectangle(nil)
					spacer.SetMinSize(fyne.NewSize(padding, 0))

					sectionContainer.Add(container.NewBorder(nil, nil, spacer, nil, content))
				}
			}
		}

		mainBox.Add(sectionContainer)
	}

	return mainBox
}
