package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
)

// SettingsManager handles UI elements for settings.
type SettingsManager struct {
	chgPrefsCallbacks   map[string]func()
	refreshFlags        map[string]bool
	refreshFuncs        []func()
	checkAndEnableApply func()
	applyButton         *widget.Button
	prefsWindow         fyne.Window
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
		checkAndEnableApply: nil,
		applyButton:         nil,
		prefsWindow:         window,
	}

	sm.applyButton = createApplyButton(sm)
	sm.checkAndEnableApply = func() {
		if len(sm.refreshFlags) > 0 || len(sm.chgPrefsCallbacks) > 0 {
			sm.applyButton.Enable() // Enable if changes or refresh needed
		} else {
			sm.applyButton.Disable() // Otherwise, disable
		}
		sm.applyButton.Refresh()
	}

	return sm
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
				sm.applyButton.SetText(originalText)
				sm.applyButton.Refresh()
			}()

			if len(sm.chgPrefsCallbacks) > 0 {
				for _, callback := range sm.chgPrefsCallbacks {
					callback()
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

	header.Add(NewSplitRow(cfg.Label, selectWidget, SplitProportion.OneThird))
	if cfg.HelpContent != nil {
		header.Add(cfg.HelpContent)
	}

	selectWidget.OnChanged = func(s string) {
		selectedIndex := selectWidget.SelectedIndex()
		if selectedIndex != cfg.InitialValue.(int) {
			sm.SetSettingChangedCallback(cfg.Name, func() {
				cfg.ApplyFunc(selectedIndex)
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
		if cfg.OnChanged != nil {
			cfg.OnChanged(s, selectedIndex)
		}
		sm.GetCheckAndEnableApplyFunc()()
	}
}

// CreateBoolSetting creates a reusable boolean check setting.
func (sm *SettingsManager) CreateBoolSetting(cfg *setting.BoolConfig, header *fyne.Container) *widget.Check {
	check := widget.NewCheck("", func(b bool) {}) // Use empty string, label is CanvasObject
	check.SetChecked(cfg.InitialValue)

	header.Add(NewSplitRow(cfg.Label, check, SplitProportion.OneThird))
	if cfg.HelpContent != nil {
		header.Add(cfg.HelpContent)
	}

	check.OnChanged = func(b bool) {
		if b != cfg.InitialValue {
			sm.SetSettingChangedCallback(cfg.Name, func() {
				cfg.ApplyFunc(b)
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
		if cfg.OnChanged != nil {
			cfg.OnChanged(b)
		}
		sm.GetCheckAndEnableApplyFunc()()
		sm.GetCheckAndEnableApplyFunc()()
	}
	return check
}

// CreateTextEntrySetting creates a reusable text entry setting.
func (sm *SettingsManager) CreateTextEntrySetting(cfg *setting.TextEntrySettingConfig, header *fyne.Container) {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(cfg.PlaceHolder)
	entry.SetText(cfg.InitialValue)

	if cfg.Validator != nil {
		entry.Validator = cfg.Validator
	}

	statusLabel := widget.NewLabel("")

	header.Add(NewSplitRow(cfg.Label, entry, SplitProportion.OneThird))
	if cfg.HelpContent != nil {
		header.Add(NewSplitRowWithAlignment(cfg.HelpContent, statusLabel, SplitProportion.TwoThirds, SplitAlign.Opposed))
	} else {
		header.Add(NewSplitRow(widget.NewLabel(""), statusLabel, SplitProportion.TwoThirds))
	}

	entry.OnChanged = func(s string) {
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
		} else {
			var postValidationCheckErr error
			if cfg.PostValidateCheck != nil {
				postValidationCheckErr = cfg.PostValidateCheck(s) // Post validation check is provided, run it and handle errors.
			}

			if postValidationCheckErr != nil {
				// Post validation check failed, handle the error.
				statusLabel.SetText(postValidationCheckErr.Error())
				statusLabel.Importance = widget.DangerImportance
				sm.RemoveSettingChangedCallback(cfg.Name)
				if cfg.NeedsRefresh {
					sm.UnsetRefreshFlag(cfg.Name)
				}
			} else {
				// Post validation check passed, update the status label and apply the function.
				statusLabel.SetText(fmt.Sprintf("%s OK", cfg.Name))
				statusLabel.Importance = widget.SuccessImportance
				if s != cfg.InitialValue {
					// Value has changed, set up the callback and refresh flag.
					sm.SetSettingChangedCallback(cfg.Name, func() {
						enteredTxt := entry.Text
						if enteredTxt != cfg.InitialValue {
							cfg.ApplyFunc(enteredTxt)     // Correctly use enteredTxt instead of s
							cfg.InitialValue = enteredTxt // Update InitialValue with the new value
						}
					})
					if cfg.NeedsRefresh {
						sm.SetRefreshFlag(cfg.Name)
					}
				}
			}
		}
		statusLabel.Refresh()             // Refresh the status label after processing all settings.
		sm.GetCheckAndEnableApplyFunc()() // Check and enable the apply button if necessary.
	}
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
