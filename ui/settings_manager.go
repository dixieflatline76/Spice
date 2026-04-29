package ui

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	utilLog "github.com/dixieflatline76/Spice/v2/util/log"
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
	allWidgets          map[string]fyne.CanvasObject
	statusLabels        map[string]*widget.Label     // NEW: Maps setting names to their status labels
	applyFuncs          map[string]func(interface{}) // Type-safe wrappers for native apply functions
	isRenderingCompact  bool                         // Tracks if the current section being rendered is compact
}

type managedWidget struct {
	widget    fyne.CanvasObject
	enabledIf func() bool
	visibleIf func() bool
}

type selectConfig struct {
	Name         string
	Options      []string
	InitialValue interface{}
	Label        fyne.CanvasObject
	HelpContent  fyne.CanvasObject
	OnChanged    func(string, interface{})
	ApplyFunc    func(interface{})
	NeedsRefresh bool
	EnabledIf    func() bool
	VisibleIf    func() bool
}

type boolConfig struct {
	Name         string
	InitialValue bool
	Label        fyne.CanvasObject
	HelpContent  fyne.CanvasObject
	OnChanged    func(bool)
	ApplyFunc    func(bool)
	NeedsRefresh bool
	EnabledIf    func() bool
	VisibleIf    func() bool
}

type textEntrySettingConfig struct {
	Name               string
	InitialValue       string
	PlaceHolder        string
	Label              fyne.CanvasObject
	HelpContent        fyne.CanvasObject
	Validator          fyne.StringValidator
	OnChanged          func(string)
	PostValidateCheck  func(string) error
	ApplyFunc          func(string)
	NeedsRefresh       bool
	SkipApply          bool
	DisplayStatus      bool
	IsPassword         bool
	EnabledIf          func() bool
	VisibleIf          func() bool
	ValidationDebounce time.Duration
}

type buttonWithConfirmationConfig struct {
	Name           string
	Label          fyne.CanvasObject
	HelpContent    fyne.CanvasObject
	ButtonText     string
	ConfirmTitle   string
	ConfirmMessage string
	Importance     widget.Importance
	OnPressed      func()
	IconName       string
	EnabledIf      func() bool
	VisibleIf      func() bool
}

type asyncButtonConfig struct {
	Name            string
	ButtonText      string
	LoadingText     string
	Importance      widget.Importance
	OnPressed       func() error
	OnCompleted     func(error)
	TargetStatusKey string
	NeedsRefresh    bool
	IconName        string
	EnabledIf       func() bool
	VisibleIf       func() bool
}

type buttonConfig struct {
	Name        string
	Label       fyne.CanvasObject
	HelpContent fyne.CanvasObject
	ButtonText  string
	Importance  widget.Importance
	OnPressed   func()
	IconName    string
	EnabledIf   func() bool
	VisibleIf   func() bool
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
		allWidgets:          make(map[string]fyne.CanvasObject),
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

// OpenURL opens the specified URL in the system's default browser.
func (sm *SettingsManager) OpenURL(u string) {
	if parsed, err := url.Parse(u); err == nil {
		_ = fyne.CurrentApp().OpenURL(parsed)
	}
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
	w, ok := sm.allWidgets[name]
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
				sm.fullRefresh()
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

// renderSelectSetting creates a reusable select widget.
func (sm *SettingsManager) renderSelectSetting(cfg *selectConfig, header *fyne.Container) {
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

	label := cfg.Label
	labelIsEmpty := true
	if label != nil {
		if l, ok := label.(*widget.Label); ok {
			if l.Text != "" {
				labelIsEmpty = false
			}
		} else {
			labelIsEmpty = false
		}
	}

	if !labelIsEmpty {
		header.Add(NewSplitRow(cfg.Label, selectWidget, SplitProportion.OneThird))
	} else {
		header.Add(selectWidget)
	}

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

	sm.allWidgets[cfg.Name] = selectWidget
}

// renderBoolSetting creates a reusable boolean check setting.
func (sm *SettingsManager) renderBoolSetting(cfg *boolConfig, header *fyne.Container) *widget.Check {
	check := widget.NewCheck("", nil) // Use empty string, label is CanvasObject
	check.SetChecked(cfg.InitialValue)
	sm.registry[cfg.Name] = cfg.InitialValue
	sm.applyFuncs[cfg.Name] = func(val interface{}) {
		cfg.ApplyFunc(val.(bool))
	}

	label := cfg.Label
	labelIsEmpty := true
	if label != nil {
		if l, ok := label.(*widget.Label); ok {
			if l.Text != "" {
				labelIsEmpty = false
			}
		} else {
			labelIsEmpty = false
		}
	}

	if labelIsEmpty {
		header.Add(check)
	} else {
		header.Add(NewSplitRow(label, check, SplitProportion.OneThird))
	}
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

	sm.allWidgets[cfg.Name] = check
	return check
}

// renderTextEntrySetting creates a reusable text entry setting.
func (sm *SettingsManager) renderTextEntrySetting(cfg *textEntrySettingConfig, header *fyne.Container) *widget.Entry {
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
	sm.allWidgets[cfg.Name] = entry

	if cfg.IsPassword {
		entry.Password = true
	}

	if cfg.Validator != nil {
		entry.Validator = cfg.Validator
	}

	label := cfg.Label
	// Density Optimization: If label is nil or a label with empty text, skip the SplitRow for the input.
	// We check for *widget.Label specifically to see if it's the standard label we created.
	labelIsEmpty := true
	if label != nil {
		if l, ok := label.(*widget.Label); ok {
			if l.Text != "" {
				labelIsEmpty = false
			}
		} else {
			labelIsEmpty = false
		}
	}

	statusLabel := widget.NewLabel("")
	sm.statusLabels[cfg.Name] = statusLabel

	if labelIsEmpty {
		header.Add(entry)
	} else {
		header.Add(NewSplitRow(label, entry, SplitProportion.OneThird))
	}

	if cfg.HelpContent != nil {
		header.Add(NewSplitRowWithAlignment(cfg.HelpContent, statusLabel, SplitProportion.TwoThirds, SplitAlign.Opposed))
	} else if !labelIsEmpty {
		// Only add empty status row if we have a label (to keep alignment)
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

func (sm *SettingsManager) handleTextEntryChanged(s string, cfg *textEntrySettingConfig, entry *widget.Entry, statusLabel *widget.Label, debounceTimer **time.Timer) {
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

func (sm *SettingsManager) runTextEntryPostCheck(val string, cfg *textEntrySettingConfig, entry *widget.Entry, statusLabel *widget.Label) {
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
			if val != baseline && !cfg.SkipApply {
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

// renderButtonWithConfirmationSetting creates a reusable button setting with confirmation dialog.
func (sm *SettingsManager) renderButtonWithConfirmationSetting(cfg *buttonWithConfirmationConfig, header *fyne.Container) {
	var button *widget.Button
	icon := sm.getIconResource(cfg.IconName)

	button = widget.NewButtonWithIcon(cfg.ButtonText, icon, func() {
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

	label := cfg.Label
	labelIsEmpty := true
	if label != nil {
		if l, ok := label.(*widget.Label); ok {
			if l.Text != "" {
				labelIsEmpty = false
			}
		} else {
			labelIsEmpty = false
		}
	}

	if !labelIsEmpty {
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

// renderButtonSetting creates a standard button setting widget.
func (sm *SettingsManager) renderButtonSetting(cfg *buttonConfig, header *fyne.Container) {
	icon := sm.getIconResource(cfg.IconName)
	button := widget.NewButtonWithIcon(cfg.ButtonText, icon, cfg.OnPressed)
	button.Importance = cfg.Importance

	label := cfg.Label
	labelIsEmpty := true
	if label != nil {
		if l, ok := label.(*widget.Label); ok {
			if l.Text != "" {
				labelIsEmpty = false
			}
		} else {
			labelIsEmpty = false
		}
	}

	if !labelIsEmpty {
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

// Refresh performs a FULL refresh cycle: UI state evaluation, registered callbacks, and widget repaints.
//
// Steps:
//  1. Evaluates all EnabledIf/VisibleIf predicates on managed widgets.
//  2. Runs ALL registered refresh functions (via RegisterRefreshFunc). These may include
//     wallpaper rotation triggers, collection sync operations, accordion title recalculations,
//     and other engine-level side effects.
//  3. Calls .Refresh() on every widget in the allWidgets registry.
//
// Use Refresh() when a state change has been COMMITTED (e.g., after Apply, after sync completes,
// after a username verification resets state). Do NOT use it for pending/uncommitted user interactions
// like checkbox toggles — use RefreshUI() instead to avoid triggering wallpaper changes.
func (sm *SettingsManager) fullRefresh() {
	// 1. Evaluate all UI dependencies (EnabledIf/VisibleIf)
	sm.refreshWidgetStates()

	// 2. Run registered refresh callbacks (may have engine-level side effects)
	for _, rf := range sm.refreshFuncs {
		rf()
	}

	// 3. Repaint all managed widgets
	for _, w := range sm.allWidgets {
		w.Refresh()
	}
}

// RefreshUI performs a UI-ONLY refresh: state evaluation and widget repaints,
// WITHOUT running registered refresh callbacks.
//
// Steps:
//  1. Evaluates all EnabledIf/VisibleIf predicates on managed widgets.
//  2. Calls .Refresh() on every widget in the allWidgets registry.
//
// This is safe to call from interactive handlers (checkbox toggles, text edits)
// because it will NOT trigger engine-level side effects like wallpaper rotation or
// collection syncs. It only updates the visual state of the UI (e.g., enabling/disabling
// dependent fields, updating accordion titles).
func (sm *SettingsManager) RefreshUI() {
	// 1. Evaluate all UI dependencies (EnabledIf/VisibleIf)
	sm.refreshWidgetStates()

	// 2. Repaint all managed widgets (no registered callbacks)
	for _, w := range sm.allWidgets {
		w.Refresh()
	}
}

// renderAsyncButton creates a button that handles background tasks and UI thread transitions.
func (sm *SettingsManager) renderAsyncButton(cfg *asyncButtonConfig, header *fyne.Container) *widget.Button {
	icon := sm.getIconResource(cfg.IconName)
	btn := widget.NewButtonWithIcon(cfg.ButtonText, icon, nil)
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
						sm.SetSettingStatus(cfg.TargetStatusKey, err.Error(), schema.ImportanceDanger)
					} else {
						sm.SetSettingStatus(cfg.TargetStatusKey, i18n.T("Success"), schema.ImportanceSuccess)
					}
				}

				cfg.OnCompleted(err)
				if cfg.NeedsRefresh {
					sm.fullRefresh()
				} else {
					sm.refreshWidgetStates()
				}
				sm.checkAndEnableApply()
			})
		}()
	}

	header.Add(btn)
	sm.allWidgets[cfg.Name] = btn

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
	sm.fullRefresh()
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

	sm.fullRefresh()
	sm.checkAndEnableApply()
}

// SetSettingStatus programmatically updates a setting's status label (thread-safe).
func (sm *SettingsManager) SetSettingStatus(name string, message string, importance schema.Importance) {
	label, ok := sm.statusLabels[name]
	if !ok || label == nil {
		return
	}

	fyneImportance := widget.LowImportance
	switch importance {
	case schema.ImportanceHigh:
		fyneImportance = widget.HighImportance
	case schema.ImportanceMedium:
		fyneImportance = widget.MediumImportance
	case schema.ImportanceLow:
		fyneImportance = widget.LowImportance
	case schema.ImportanceSuccess:
		fyneImportance = widget.SuccessImportance
	case schema.ImportanceDanger:
		fyneImportance = widget.DangerImportance
	}

	fyne.Do(func() {
		label.SetText(message)
		label.Importance = fyneImportance
		label.Refresh()
	})
}

// RenderSchema takes a pure Go UI definition and renders it to a Fyne container.
func (sm *SettingsManager) RenderSchema(p schema.PanelSchema) fyne.CanvasObject {
	var topItems []fyne.CanvasObject
	var expandingList fyne.CanvasObject

	createLabel := func(text string) fyne.CanvasObject {
		if text == "" || sm.isRenderingCompact {
			return nil
		}
		return sm.CreateSettingTitleLabel(text)
	}
	createHelp := func(text string) fyne.CanvasObject {
		if text == "" {
			return nil
		}
		return sm.CreateSettingDescriptionLabel(text)
	}

	for _, section := range p.Sections {
		sm.isRenderingCompact = section.Compact
		sectionContainer := container.NewVBox()
		sectionHasItems := false

		if section.Title != "" {
			sectionContainer.Add(sm.CreateSectionTitleLabel(section.Title))
			sectionHasItems = true
		}
		if section.Description != "" {
			sectionContainer.Add(sm.CreateSettingDescriptionLabel(section.Description))
			sectionHasItems = true
		}

		for _, item := range section.Items {
			switch v := item.(type) {
			case *schema.OAuthPickerItem:
				val := *v
				if lbl := createLabel(val.Label); lbl != nil {
					sectionContainer.Add(lbl)
				}
				if hlp := createHelp(val.Help); hlp != nil {
					sectionContainer.Add(hlp)
				}
				sectionContainer.Add(sm.renderOAuthPickerInline(val))
				sectionHasItems = true

			case schema.OAuthPickerItem:
				if lbl := createLabel(v.Label); lbl != nil {
					sectionContainer.Add(lbl)
				}
				if hlp := createHelp(v.Help); hlp != nil {
					sectionContainer.Add(hlp)
				}
				sectionContainer.Add(sm.renderOAuthPickerInline(v))
				sectionHasItems = true

			case schema.BoolItem:
				if v.Help == "" {
					check := widget.NewCheck(v.Label, nil)
					check.SetChecked(v.InitialValue)
					sm.registry[v.Name] = v.InitialValue
					if v.ApplyFunc != nil {
						sm.applyFuncs[v.Name] = func(val interface{}) {
							v.ApplyFunc(val.(bool))
						}
					}
					check.OnChanged = func(b bool) {
						if b != sm.registry[v.Name].(bool) {
							applyFunc := v.ApplyFunc
							needsRefresh := v.NeedsRefresh
							settingName := v.Name
							sm.chgPrefsCallbacks[settingName] = func() {
								if applyFunc != nil {
									applyFunc(b)
								}
								if needsRefresh {
									sm.refreshFlags[settingName] = true
								}
							}
						} else {
							delete(sm.chgPrefsCallbacks, v.Name)
						}
						if v.OnChanged != nil {
							v.OnChanged(b)
						}
						sm.checkAndEnableApply()
					}
					sm.valueGetters[v.Name] = func() interface{} {
						return check.Checked
					}
					if v.EnabledIf != nil || v.VisibleIf != nil {
						sm.managedWidgets = append(sm.managedWidgets, managedWidget{
							widget:    check,
							enabledIf: v.EnabledIf,
							visibleIf: v.VisibleIf,
						})
					}
					sm.allWidgets[v.Name] = check
					sectionContainer.Add(check)
				} else {
					sm.renderBoolSetting(&boolConfig{
						Name:         v.Name,
						InitialValue: v.InitialValue,
						Label:        createLabel(v.Label),
						HelpContent:  createHelp(v.Help),
						OnChanged:    v.OnChanged,
						ApplyFunc:    v.ApplyFunc,
						NeedsRefresh: v.NeedsRefresh,
						EnabledIf:    v.EnabledIf,
						VisibleIf:    v.VisibleIf,
					}, sectionContainer)
				}
				sectionHasItems = true

			case schema.TextItem:
				var fyneValidator fyne.StringValidator
				if v.Validator != nil {
					fyneValidator = v.Validator
				}

				sm.renderTextEntrySetting(&textEntrySettingConfig{
					Name:               v.Name,
					InitialValue:       v.InitialValue,
					PlaceHolder:        v.PlaceHolder,
					Label:              createLabel(v.Label),
					HelpContent:        createHelp(v.Help),
					Validator:          fyneValidator,
					OnChanged:          v.OnChanged,
					PostValidateCheck:  v.PostValidateCheck,
					ApplyFunc:          v.ApplyFunc,
					NeedsRefresh:       v.NeedsRefresh,
					SkipApply:          v.SkipApply,
					DisplayStatus:      v.DisplayStatus,
					IsPassword:         v.IsPassword,
					EnabledIf:          v.EnabledIf,
					VisibleIf:          v.VisibleIf,
					ValidationDebounce: v.ValidationDebounce,
				}, sectionContainer)
				sectionHasItems = true

			case schema.SelectItem:
				sm.renderSelectSetting(&selectConfig{
					Name:         v.Name,
					Options:      v.Options,
					InitialValue: v.InitialValue,
					Label:        createLabel(v.Label),
					HelpContent:  createHelp(v.Help),
					OnChanged:    v.OnChanged,
					ApplyFunc:    v.ApplyFunc,
					NeedsRefresh: v.NeedsRefresh,
					EnabledIf:    v.EnabledIf,
					VisibleIf:    v.VisibleIf,
				}, sectionContainer)
				sectionHasItems = true

			case schema.AsyncButtonItem:
				importance := widget.LowImportance
				switch v.Style {
				case schema.ButtonStylePrimary:
					importance = widget.HighImportance
				case schema.ButtonStyleDanger:
					importance = widget.DangerImportance
				case schema.ButtonStyleSuccess:
					importance = widget.SuccessImportance
				}

				sm.renderAsyncButton(&asyncButtonConfig{
					Name:            v.Name,
					ButtonText:      v.ButtonText,
					LoadingText:     v.LoadingText,
					Importance:      importance,
					OnPressed:       v.OnPressed,
					OnCompleted:     v.OnCompleted,
					TargetStatusKey: v.TargetStatusKey,
					NeedsRefresh:    v.NeedsRefresh,
					IconName:        v.IconName,
					EnabledIf:       v.EnabledIf,
					VisibleIf:       v.VisibleIf,
				}, sectionContainer)
				sectionHasItems = true

			case schema.ConfirmButtonItem:
				fyneImportance := widget.LowImportance
				switch v.Importance {
				case schema.ImportanceHigh:
					fyneImportance = widget.HighImportance
				case schema.ImportanceMedium:
					fyneImportance = widget.MediumImportance
				case schema.ImportanceLow:
					fyneImportance = widget.LowImportance
				case schema.ImportanceSuccess:
					fyneImportance = widget.SuccessImportance
				case schema.ImportanceDanger:
					fyneImportance = widget.DangerImportance
				}

				sm.renderButtonWithConfirmationSetting(&buttonWithConfirmationConfig{
					Name:           v.Name,
					ButtonText:     v.ButtonText,
					ConfirmTitle:   v.ConfirmTitle,
					ConfirmMessage: v.ConfirmMessage,
					Importance:     fyneImportance,
					OnPressed:      v.OnPressed,
					IconName:       v.IconName,
					EnabledIf:      v.EnabledIf,
					VisibleIf:      v.VisibleIf,
					Label:          createLabel(v.Label),
					HelpContent:    createHelp(v.Help),
				}, sectionContainer)
				sectionHasItems = true

			case schema.ButtonItem:
				fyneImportance := widget.MediumImportance
				switch v.Importance {
				case schema.ImportanceHigh:
					fyneImportance = widget.HighImportance
				case schema.ImportanceMedium:
					fyneImportance = widget.MediumImportance
				case schema.ImportanceLow:
					fyneImportance = widget.LowImportance
				case schema.ImportanceSuccess:
					fyneImportance = widget.SuccessImportance
				case schema.ImportanceDanger:
					fyneImportance = widget.DangerImportance
				}

				sm.renderButtonSetting(&buttonConfig{
					Name:        v.Name,
					ButtonText:  v.ButtonText,
					Importance:  fyneImportance,
					OnPressed:   v.OnPressed,
					IconName:    v.IconName,
					EnabledIf:   v.EnabledIf,
					VisibleIf:   v.VisibleIf,
					Label:       createLabel(v.Label),
					HelpContent: createHelp(v.Help),
				}, sectionContainer)
				sectionHasItems = true

			case schema.HyperlinkItem:
				u, err := url.Parse(v.URL)
				var hl fyne.CanvasObject
				if err == nil {
					hl = widget.NewHyperlink(v.Text, u)
				} else {
					hl = widget.NewLabel(v.Text + " (" + v.URL + ")")
				}
				if v.ID != "" {
					sm.allWidgets[v.ID] = hl
				}
				sectionContainer.Add(hl)
				sectionHasItems = true

			case schema.LabelItem:
				var labelObj fyne.CanvasObject
				if v.IsTitle {
					labelObj = sm.CreateSectionTitleLabel(v.Text)
				} else {
					var content fyne.CanvasObject
					if v.Importance == schema.ImportanceLow {
						rich := widget.NewRichTextWithText(v.Text)
						rich.Segments[0].(*widget.TextSegment).Style.ColorName = theme.ColorNamePlaceHolder
						rich.Wrapping = fyne.TextWrapWord
						content = rich
					} else {
						lbl := widget.NewLabel(v.Text)
						lbl.Wrapping = fyne.TextWrapWord
						content = lbl
					}
					padding := theme.Padding()
					spacer := canvas.NewRectangle(nil)
					spacer.SetMinSize(fyne.NewSize(padding, 0))
					labelObj = container.NewBorder(nil, nil, spacer, nil, content)
				}
				if v.ID != "" {
					sm.allWidgets[v.ID] = labelObj
				}
				sectionContainer.Add(labelObj)
				sectionHasItems = true

			case schema.QueryListItem:
				list := sm.renderQueryList(v)
				sm.RegisterRefreshFunc(func() { list.Refresh() })
				expandingList = list
				// Always register in allWidgets so RefreshUI() can repaint the list
				// (e.g., after an async sync operation completes in a provider goroutine).
				widgetID := v.ID
				if widgetID == "" {
					widgetID = fmt.Sprintf("__querylist_%d", len(sm.allWidgets))
				}
				sm.allWidgets[widgetID] = list

			case schema.FolderPickerItem:
				btn := widget.NewButtonWithIcon(v.ButtonText, theme.FolderOpenIcon(), func() {
					showOSFolderPicker(sm.GetSettingsWindow(), func(path string, err error) {
						if err != nil || path == "" {
							return
						}
						_ = v.OnFolderSelected(path)
						sm.fullRefresh()
					})
				})
				sectionContainer.Add(btn)
				sm.allWidgets[v.Name] = btn
				sectionHasItems = true

			case *schema.HorizontalRowItem:
				val := *v
				row := sm.renderHorizontalRow(val)
				if val.ID != "" {
					sm.allWidgets[val.ID] = row
				}
				sectionContainer.Add(row)
				sectionHasItems = true

			case schema.HorizontalRowItem:
				row := sm.renderHorizontalRow(v)
				if v.ID != "" {
					sm.allWidgets[v.ID] = row
				}
				sectionContainer.Add(row)
				sectionHasItems = true

			case schema.SecretItem:
				sectionContainer.Add(sm.renderSecretItem(v))
				sectionHasItems = true
			}
		}

		if sectionHasItems {
			topItems = append(topItems, sectionContainer)
		}
	}

	topContainer := container.NewVBox(topItems...)

	if expandingList != nil {
		// ONLY use Border + VScroll for providers with editable query tables (Pexels, Wallhaven, etc.)
		content := container.NewBorder(topContainer, nil, nil, nil, expandingList)
		return container.NewVScroll(content)
	}

	// For Museums (Met, ArtIC) and others without lists:
	// Return the VBox directly. Do NOT wrap in VScroll. Do NOT wrap in Border.
	return topContainer
}

// renderSecretItem renders a high-density API key/credential item.
func (sm *SettingsManager) renderSecretItem(v schema.SecretItem) fyne.CanvasObject {
	if sm.GetBaseline(v.Name) == nil {
		sm.SeedBaseline(v.Name, v.InitialValue)
	}

	// Register a value getter immediately so cross-item dependencies
	// (e.g. Wallhaven username verification needing the API key) can
	// read the current value via sm.GetValue(). The getter returns the
	// live registry value which is kept in sync by CommitSetting/SeedBaseline.
	sm.valueGetters[v.Name] = func() interface{} {
		return sm.registry[v.Name]
	}

	if v.ApplyFunc != nil {
		sm.applyFuncs[v.Name] = func(val interface{}) {
			v.ApplyFunc(val.(string))
		}
	}

	// --- Empty State: Label + Entry + Save Button ---
	entry := widget.NewEntry()
	entry.Password = true
	entry.SetPlaceHolder(v.Placeholder)

	saveBtn := widget.NewButtonWithIcon(i18n.T("Verify & Save"), theme.ConfirmIcon(), nil)
	saveBtn.Importance = widget.HighImportance
	saveBtn.OnTapped = func() {
		saveBtn.Disable()
		err := v.OnVerify(entry.Text)
		if err != nil {
			dialog.ShowError(err, sm.GetSettingsWindow())
			saveBtn.Enable()
		} else {
			// Temporarily override the value getter so CommitSetting picks up the
			// fresh entry text. After commit, restore the registry-based getter
			// so cross-item reads (e.g. Wallhaven username needing API key) work.
			sm.valueGetters[v.Name] = func() interface{} {
				return entry.Text
			}
			sm.CommitSetting(v.Name) // Writes entry.Text → applyFunc → registry
			sm.valueGetters[v.Name] = func() interface{} {
				return sm.registry[v.Name]
			}
		}
	}

	// Build empty state: SplitRow with label + entry on top, save button full-width below
	var emptyRows []fyne.CanvasObject
	if v.Label != "" {
		label := sm.CreateSettingTitleLabel(v.Label)
		emptyRows = append(emptyRows, NewSplitRow(label, entry, SplitProportion.OneThird))
	} else {
		emptyRows = append(emptyRows, entry)
	}
	emptyRows = append(emptyRows, saveBtn)
	emptyState := container.NewVBox(emptyRows...)

	// --- Saved State: Masked display + full-width Clear button ---
	display := widget.NewEntry()
	display.SetText("********************************")
	display.Disable()

	clearBtn := widget.NewButtonWithIcon(i18n.T("Clear API Key"), theme.DeleteIcon(), func() {
		v.OnClear() // OnClear implementations call ResetSettings which already calls Refresh()
	})
	clearBtn.Importance = widget.DangerImportance

	var savedRows []fyne.CanvasObject
	if v.Label != "" {
		label := sm.CreateSettingTitleLabel(v.Label)
		savedRows = append(savedRows, NewSplitRow(label, display, SplitProportion.OneThird))
	} else {
		savedRows = append(savedRows, display)
	}
	savedRows = append(savedRows, clearBtn)
	savedState := container.NewVBox(savedRows...)

	// Stack them so we can toggle visibility easily
	stack := container.NewStack(emptyState, savedState)

	refresh := func() {
		baseline, _ := sm.registry[v.Name].(string)
		if baseline == "" {
			emptyState.Show()
			savedState.Hide()
			entry.SetText("") // Clear sensitive input when returning to empty state
			saveBtn.Enable()
		} else {
			emptyState.Hide()
			savedState.Show()
		}
		stack.Refresh()
	}

	sm.RegisterRefreshFunc(refresh)
	refresh() // Initial state trigger

	sm.allWidgets[v.Name] = stack
	return stack
}

// renderOAuthPickerInline renders the full OAuth authorize/disconnect + picker UI for a provider.
func (sm *SettingsManager) renderOAuthPickerInline(v schema.OAuthPickerItem) fyne.CanvasObject {
	statusLabel := widget.NewLabel(i18n.T("Status: Checking..."))
	var connectBtn *widget.Button
	var safeFullRefresh func()

	updateUI := func() {
		isAuth, _ := v.CheckAuthStatus()
		if isAuth {
			statusLabel.SetText(i18n.T("Status: Authorized (Ready to Select)"))
			connectBtn.SetText(i18n.T("Disconnect Authorisation"))
			connectBtn.Importance = widget.DangerImportance
			connectBtn.OnTapped = func() {
				err := v.OnDisconnect()
				if err != nil {
					dialog.ShowError(err, sm.GetSettingsWindow())
				}
				if safeFullRefresh != nil {
					safeFullRefresh()
				}
			}
		} else {
			statusLabel.SetText(i18n.T("Status: Not Authorized"))
			connectBtn.SetText(i18n.T("Authorize Application"))
			connectBtn.Importance = widget.MediumImportance
			connectBtn.OnTapped = func() {
				err := v.OnAuthorize()
				if err != nil {
					dialog.ShowError(err, sm.GetSettingsWindow())
				} else {
					dialog.ShowInformation(i18n.T("Success"), i18n.T("Authorized!"), sm.GetSettingsWindow())
					if safeFullRefresh != nil {
						safeFullRefresh()
					}
				}
			}
		}
		connectBtn.Refresh()
	}

	connectBtn = widget.NewButton("", nil)

	authContainer := container.NewVBox(
		statusLabel,
		connectBtn,
		widget.NewSeparator(),
	)

	pickerContainer := container.NewVBox()

	progressBar := widget.NewProgressBarInfinite()
	progressBar.Hide()
	pickerStatus := widget.NewLabel("")

	addBtn := widget.NewButton(i18n.T("Select Photos via Web Picker"), nil)
	cancelBtn := widget.NewButton(i18n.T("Cancel"), nil)
	cancelBtn.Importance = widget.LowImportance
	cancelBtn.Hide()

	var cancelFunc context.CancelFunc

	updateAddBtn := func() {
		isAuth, _ := v.CheckAuthStatus()
		if isAuth {
			addBtn.Enable()
			pickerStatus.SetText("")
		} else {
			addBtn.Disable()
			pickerStatus.SetText(i18n.T("Please Authorize above first."))
		}
	}

	safeFullRefresh = func() {
		updateUI()
		updateAddBtn()
	}
	safeFullRefresh()

	cancelBtn.OnTapped = func() {
		if cancelFunc != nil {
			cancelFunc()
			pickerStatus.SetText(i18n.T("Operation cancelled."))
		}
		cancelBtn.Hide()
		progressBar.Hide()
		addBtn.Show()
		addBtn.Enable()
	}

	addBtn.OnTapped = func() {
		isAuth, _ := v.CheckAuthStatus()
		if !isAuth {
			dialog.ShowError(fmt.Errorf("please authorize first"), sm.GetSettingsWindow())
			return
		}

		addBtn.Disable()
		addBtn.Hide()
		cancelBtn.Show()
		progressBar.Show()

		updateStatus := func(msg string) {
			fyne.Do(func() {
				pickerStatus.SetText(msg)
			})
		}
		updateStatus(i18n.T("Creating Web Session..."))

		ctx, cancel := context.WithCancel(context.Background())
		cancelFunc = cancel

		go func() {
			count, guid, err := v.OnLaunchPicker(ctx, updateStatus)

			fyne.Do(func() {
				cancelBtn.Hide()
				addBtn.Show()
				addBtn.Enable()
				progressBar.Hide()
				pickerStatus.SetText("")
				cancelFunc = nil

				if err != nil {
					if ctx.Err() != context.Canceled {
						dialog.ShowError(err, sm.GetSettingsWindow())
						pickerStatus.SetText(i18n.T("Error: ") + err.Error())
					}
					return
				}

				if count == 0 {
					dialog.ShowError(fmt.Errorf("no photos selected"), sm.GetSettingsWindow())
					return
				}

				urlStr := "googlephotos://" + guid
				defaultDesc := fmt.Sprintf(i18n.T("Collection %s (%d items)"), time.Now().Format("Jan 02 15:04"), count)

				urlEntry := widget.NewEntry()
				urlEntry.SetText(urlStr)
				urlEntry.Disable()
				descEntry := widget.NewEntry()
				descEntry.SetText(defaultDesc)

				activeCheck := widget.NewCheck(i18n.T("Active"), nil)
				activeCheck.SetChecked(true)

				form := container.NewVBox(
					widget.NewLabel(i18n.T("Internal ID:")),
					urlEntry,
					widget.NewLabel(i18n.T("Description:")),
					descEntry,
					activeCheck,
				)

				d := dialog.NewCustomConfirm(
					i18n.T("Save Collection"),
					i18n.T("Save"),
					i18n.T("Cancel"),
					form,
					func(save bool) {
						if save {
							if err := v.OnSaveCollection(guid, descEntry.Text, activeCheck.Checked); err != nil {
								dialog.ShowError(err, sm.GetSettingsWindow())
							} else {
								sm.SetRefreshFlag("queries")
								sm.GetCheckAndEnableApplyFunc()()
								sm.fullRefresh()
							}
						} else {
							if v.OnCancelCollection != nil {
								v.OnCancelCollection(guid)
							}
						}
					},
					sm.GetSettingsWindow(),
				)
				d.Resize(fyne.NewSize(500, 350))
				d.Show()
			})
		}()
	}

	pickerContainer.Add(container.NewStack(addBtn, cancelBtn))
	pickerContainer.Add(progressBar)
	pickerContainer.Add(pickerStatus)

	mainStack := container.NewVBox(authContainer, pickerContainer)
	sm.allWidgets[v.Name] = mainStack
	return mainStack
}

// renderHorizontalRow groups multiple items horizontally.
func (sm *SettingsManager) renderHorizontalRow(v schema.HorizontalRowItem) fyne.CanvasObject {
	if len(v.Items) == 0 {
		return nil
	}
	row := container.NewAdaptiveGrid(len(v.Items))
	for _, item := range v.Items {
		// Create a temporary container for each item in the row
		temp := container.NewVBox()
		// Recursively render into temp, but we only expect simple items here
		// Actually, we should probably extract the widget creation logic
		// For simplicity, we just handle buttons/labels for now
		switch item := item.(type) {
		case schema.TextItem:
			sm.renderTextEntrySetting(&textEntrySettingConfig{
				Name:               item.Name,
				InitialValue:       item.InitialValue,
				PlaceHolder:        item.PlaceHolder,
				IsPassword:         item.IsPassword,
				NeedsRefresh:       item.NeedsRefresh,
				DisplayStatus:      item.DisplayStatus,
				ValidationDebounce: item.ValidationDebounce,
				OnChanged:          item.OnChanged,
				PostValidateCheck:  item.PostValidateCheck,
				ApplyFunc:          item.ApplyFunc,
				Validator:          item.Validator,
				EnabledIf:          item.EnabledIf,
				VisibleIf:          item.VisibleIf,
			}, temp)
		case schema.ButtonItem:
			sm.renderButtonSetting(&buttonConfig{
				Name:       item.Name,
				ButtonText: item.ButtonText,
				Importance: sm.mapImportance(item.Importance),
				OnPressed:  item.OnPressed,
				IconName:   item.IconName,
				EnabledIf:  item.EnabledIf,
				VisibleIf:  item.VisibleIf,
			}, temp)
		case schema.ConfirmButtonItem:
			sm.renderButtonWithConfirmationSetting(&buttonWithConfirmationConfig{
				Name:           item.Name,
				ButtonText:     item.ButtonText,
				ConfirmTitle:   item.ConfirmTitle,
				ConfirmMessage: item.ConfirmMessage,
				Importance:     sm.mapImportance(item.Importance),
				OnPressed:      item.OnPressed,
				IconName:       item.IconName,
				EnabledIf:      item.EnabledIf,
				VisibleIf:      item.VisibleIf,
			}, temp)
		case schema.LabelItem:
			if item.IsTitle {
				temp.Add(sm.CreateSettingTitleLabel(item.Text))
			} else {
				temp.Add(widget.NewLabel(item.Text))
			}
		}
		// Add all widgets from temp to row
		for _, w := range temp.Objects {
			row.Add(w)
		}
	}
	return row
}

func (sm *SettingsManager) mapImportance(i schema.Importance) widget.Importance {
	switch i {
	case schema.ImportanceHigh:
		return widget.HighImportance
	case schema.ImportanceMedium:
		return widget.MediumImportance
	case schema.ImportanceLow:
		return widget.LowImportance
	case schema.ImportanceSuccess:
		return widget.SuccessImportance
	case schema.ImportanceDanger:
		return widget.DangerImportance
	default:
		return widget.MediumImportance
	}
}

func (sm *SettingsManager) getIconResource(name string) fyne.Resource {
	switch strings.ToLower(name) {
	case "help", "question":
		return theme.HelpIcon()
	case "map", "mappin":
		return theme.InfoIcon()
	case "home", "browser":
		return theme.HomeIcon()
	case "heart", "favorite":
		return theme.ConfirmIcon()
	case "info":
		return theme.InfoIcon()
	case "add", "plus":
		return theme.ContentAddIcon()
	case "settings", "prefs":
		return theme.SettingsIcon()
	case "search":
		return theme.SearchIcon()
	default:
		return nil
	}
}

// ShowError displays a modal error dialog to the user.
func (sm *SettingsManager) ShowError(err error) {
	dialog.ShowError(err, sm.prefsWindow)
}

// ShowConfirm displays a modal confirmation dialog to the user.
func (sm *SettingsManager) ShowConfirm(title, message string, callback func(bool)) {
	d := dialog.NewConfirm(title, message, callback, sm.prefsWindow)
	d.Show()
}

// ShowAddQueryDialog opens the modal for adding image queries.
func (sm *SettingsManager) ShowAddQueryDialog(cfg schema.AddQueryConfig, initialURL, initialDesc string, onAdded func()) {
	utilLog.Debugf("ShowAddQueryDialog: Triggered with URL: %s", initialURL)

	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder(cfg.URLPlaceholder)
	urlEntry.SetText(initialURL)
	if cfg.URLValidator != "" {
		urlEntry.Validator = validation.NewRegexp(cfg.URLValidator, cfg.URLErrorMsg)
	}

	descEntry := widget.NewEntry()
	descEntry.SetPlaceHolder(cfg.DescPlaceholder)
	descEntry.SetText(initialDesc)
	if cfg.DescValidator != "" {
		descEntry.Validator = validation.NewRegexp(cfg.DescValidator, cfg.DescErrorMsg)
	}

	formStatus := widget.NewLabel("")
	activeBool := widget.NewCheck(i18n.T("Active"), nil)
	activeBool.SetChecked(true)

	cancelButton := widget.NewButton(i18n.T("Cancel"), nil)
	saveButton := widget.NewButton(i18n.T("Save"), nil)

	if initialURL == "" {
		saveButton.Disable()
	} else {
		saveButton.Enable()
	}

	// Validation logic
	formValidator := func(who *widget.Entry) bool {
		if urlEntry.Text == "" || descEntry.Text == "" {
			return false
		}
		if err := urlEntry.Validate(); err != nil {
			if who == urlEntry {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
			}
			return false
		}
		if err := descEntry.Validate(); err != nil {
			if who == descEntry {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
			}
			return false
		}
		if cfg.ValidateFunc != nil {
			if err := cfg.ValidateFunc(urlEntry.Text, descEntry.Text); err != nil {
				if who == urlEntry || (who == descEntry && urlEntry.Text != "") {
					formStatus.SetText(err.Error())
					formStatus.Importance = widget.DangerImportance
					formStatus.Refresh()
				}
				return false
			}
		}
		formStatus.SetText(i18n.T("Everything looks good"))
		formStatus.Importance = widget.SuccessImportance
		formStatus.Refresh()
		return true
	}

	// Entry listeners with max length enforcement
	const maxURLLen = 500
	const maxDescLen = 100
	newChecker := func(entry *widget.Entry, maxLen int) func(string) {
		return func(s string) {
			if len(s) > maxLen {
				entry.SetText(s[:maxLen])
				return
			}
			if formValidator(entry) {
				saveButton.Enable()
			} else {
				saveButton.Disable()
			}
		}
	}
	urlEntry.OnChanged = newChecker(urlEntry, maxURLLen)
	descEntry.OnChanged = newChecker(descEntry, maxDescLen)

	// Build dialog layout
	inputContainer := container.NewVBox()
	if cfg.Description != "" {
		inputContainer.Add(widget.NewLabel(cfg.Description))
	}
	inputContainer.Add(sm.CreateSettingTitleLabel(i18n.T("URL / Search Term:")))
	inputContainer.Add(urlEntry)
	inputContainer.Add(sm.CreateSettingTitleLabel(i18n.T("Description:")))
	inputContainer.Add(descEntry)
	inputContainer.Add(formStatus)
	inputContainer.Add(activeBool)
	inputContainer.Add(widget.NewSeparator())
	inputContainer.Add(container.NewHBox(cancelButton, layout.NewSpacer(), saveButton))

	d := dialog.NewCustomWithoutButtons(cfg.Title, inputContainer, sm.prefsWindow)
	d.Resize(fyne.NewSize(600, 200))

	saveButton.OnTapped = func() {
		if cfg.ValidateFunc != nil {
			if err := cfg.ValidateFunc(urlEntry.Text, descEntry.Text); err != nil {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
				return
			}
		}

		newID, err := cfg.AddHandler(descEntry.Text, urlEntry.Text, activeBool.Checked)
		if err != nil {
			formStatus.SetText(err.Error())
			formStatus.Importance = widget.DangerImportance
			formStatus.Refresh()
			return
		}

		if onAdded != nil {
			onAdded()
		}

		if activeBool.Checked {
			sm.SetRefreshFlag(newID)
			sm.checkAndEnableApply()
		}

		d.Hide()
	}

	cancelButton.OnTapped = func() {
		d.Hide()
	}

	utilLog.Debug("ShowAddQueryDialog: Calling d.Show()")
	d.Show()
}

// renderQueryList builds a scroll-safe widget.List for query management.
// It handles baseline seeding, pending state preservation across Fyne cell recycling,
// and wires up the enable/disable/delete interactions with the SettingsManager.
func (sm *SettingsManager) renderQueryList(v schema.QueryListItem) *widget.List {
	var queryList *widget.List
	queryList = widget.NewList(
		// Length
		func() int {
			return len(v.GetQueries())
		},
		// CreateItem — builds the cell template (no data binding here)
		func() fyne.CanvasObject {
			urlLink := widget.NewHyperlink(i18n.T("Placeholder"), nil)
			activeCheck := widget.NewCheck(i18n.T("Active"), nil)
			deleteButton := widget.NewButton(i18n.T("Delete"), func() {})
			return container.NewHBox(urlLink, layout.NewSpacer(), activeCheck, deleteButton)
		},
		// UpdateItem — binds data to a recycled cell (scroll-safe)
		func(i int, o fyne.CanvasObject) {
			queries := v.GetQueries()
			if i >= len(queries) {
				return
			}
			query := queries[i]
			queryKey := query.ID

			c := o.(*fyne.Container)
			urlLink := c.Objects[0].(*widget.Hyperlink)
			activeCheck := c.Objects[2].(*widget.Check)
			deleteButton := c.Objects[3].(*widget.Button)

			// Set display text
			if v.GetDisplayText != nil {
				urlLink.SetText(v.GetDisplayText(query))
			} else {
				urlLink.SetText(query.Description)
			}

			// Set clickable URL
			if v.GetDisplayURL != nil {
				if u := v.GetDisplayURL(query); u != nil {
					urlLink.SetURL(u)
				}
			} else {
				if u, err := url.Parse(query.URL); err == nil {
					urlLink.SetURL(u)
				}
			}

			// --- Scroll-Safe State Management ---
			if sm.GetBaseline(queryKey) == nil {
				sm.SeedBaseline(queryKey, query.Active)
			}

			// MUST clear OnChanged before SetChecked to prevent recycling bugs
			activeCheck.OnChanged = nil

			if sm.HasPendingChange(queryKey) {
				activeCheck.SetChecked(!sm.GetBaseline(queryKey).(bool))
			} else {
				activeCheck.SetChecked(query.Active)
			}

			// Wire checkbox toggle
			activeCheck.OnChanged = func(b bool) {
				if b != sm.GetBaseline(queryKey).(bool) {
					sm.SetSettingChangedCallback(queryKey, func() {
						var err error
						if b {
							err = v.EnableQuery(query.ID)
						} else {
							err = v.DisableQuery(query.ID)
						}
						if err != nil {
							utilLog.Printf("Failed to update query status: %v", err)
						} else {
							sm.SeedBaseline(queryKey, b)
							query.Active = b
						}
					})
					sm.SetRefreshFlag(queryKey)
				} else {
					sm.RemoveSettingChangedCallback(queryKey)
					sm.UnsetRefreshFlag(queryKey)
				}
				sm.checkAndEnableApply()
				sm.RefreshUI() // Update UI states (accordion titles) without triggering wallpaper changes
			}

			// Wire action button
			label := i18n.T("Delete")
			if v.DeleteLabel != "" {
				label = v.DeleteLabel
			}
			deleteButton.SetText(label)

			deleteButton.OnTapped = func() {
				confirmTitle := i18n.T("Please Confirm")
				confirmMsg := i18n.Tf("Are you sure you want to delete {{.Description}}?", map[string]any{"Description": query.Description})

				if v.DeleteConfirmMessage != "" {
					confirmMsg = v.DeleteConfirmMessage
				} else if query.Managed || v.ForceActionEnabled {
					if v.DeleteLabel == i18n.T("Clear") {
						confirmMsg = i18n.T("Are you sure you want to delete all saved favorites?")
					}
				}

				sm.ShowConfirm(confirmTitle, confirmMsg, func(b bool) {
					if b {
						if query.Active {
							sm.SetRefreshFlag(queryKey)
							sm.checkAndEnableApply()
						}
						if err := v.RemoveQuery(query.ID); err != nil {
							utilLog.Printf("Failed to remove query: %v", err)
						}
						queryList.Refresh()
					}
				})
			}

			// Managed/synced queries: show a disabled Delete button for visual consistency.
			// Non-managed queries: enable the Delete button.
			if query.Managed && !v.ForceActionEnabled {
				deleteButton.Disable()
			} else {
				deleteButton.Enable()
			}
			deleteButton.Refresh()
		},
	)
	return queryList
}
