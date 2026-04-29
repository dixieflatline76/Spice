package setting

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
)

// SettingReset holds the payload for an atomic state reset.
type SettingReset struct {
	Name  string
	Value interface{}
}

// StringOptions converts a slice of fmt.Stringer to a slice of strings.
func StringOptions(options []fmt.Stringer) []string {
	stringOptions := []string{}
	for _, option := range options {
		stringOptions = append(stringOptions, option.String())
	}
	return stringOptions
}

// SettingsManager is an interface for managing settings. It provides methods to create various types of settings widgets.
// The interface is designed so that consumers in pkg/ never need to import Fyne directly.
type SettingsManager interface {
	GetApplySettingsButton() *widget.Button                        //GetApplySettingsButton returns the Apply Changes button from the SettingsManager to be used in the UI.
	SetSettingChangedCallback(settingName string, callback func()) // Set a callback function to be called when a setting changes.
	RemoveSettingChangedCallback(settingName string)               // Remove a callback function associated with a specific setting.
	SetRefreshFlag(settingName string)                             // Set a flag to indicate that a specific setting needs a refresh.
	UnsetRefreshFlag(settingName string)                           // Unset the refresh flag for a specific setting.

	RegisterRefreshFunc(refreshFunc func())  // Register a function to be called when the settings need to be refreshed.
	RegisterOnSettingsSaved(callback func()) // Register a function to be called after settings are saved.
	GetCheckAndEnableApplyFunc() func()      // GetCheckAndEnableApplyFunction returns the check and enable apply function for the SettingsManager.
	// SeedBaseline seeds the initial state for a setting to track changes.
	SeedBaseline(name string, val interface{})
	// GetBaseline returns the initial state for a setting.
	GetBaseline(name string) interface{}
	// GetValue returns the live/current value for a setting from its valueGetter.
	GetValue(name string) interface{}
	// SetValue programmatically updates the live value of a setting.
	SetValue(name string, val interface{})
	// HasPendingChange returns true if the user has toggled a setting but not yet applied.
	HasPendingChange(name string) bool
	// RefreshUI performs a UI-ONLY refresh: state evaluation and widget repaints,
	// WITHOUT running registered callbacks. Safe for interactive handlers (checkbox toggles,
	// text edits) — will not trigger wallpaper changes or other engine-level side effects.
	// This is the ONLY refresh method available to providers. Engine-internal code uses
	// fullRefresh() for committed state changes that need registered callbacks.
	RefreshUI()
	// CommitSetting atomically reads the current UI value, applies it to the native setter, and updates the baseline.
	CommitSetting(name string)
	// ResetSettings atomically clears multiple settings, updates native getters, and resyncs baselines.
	ResetSettings(resets ...SettingReset)

	// SetSettingStatus programmatically updates a setting's status label (thread-safe).
	SetSettingStatus(name string, message string, importance schema.Importance)

	// RenderSchema takes a pure Go UI definition and renders it to a Fyne container.
	RenderSchema(p schema.PanelSchema) fyne.CanvasObject

	// OpenURL opens the specified URL in the system's default browser.
	OpenURL(u string)

	// ShowAddQueryDialog opens the modal for adding image queries.
	ShowAddQueryDialog(cfg schema.AddQueryConfig, initialURL, initialDesc string, onAdded func())

	// ShowError displays a modal error dialog to the user.
	ShowError(err error)

	// ShowConfirm displays a modal confirmation dialog to the user.
	ShowConfirm(title, message string, callback func(bool))
}
