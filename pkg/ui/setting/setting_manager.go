package setting

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// SettingsHelper is the interface that must be implemented by all settings helpers.
type SettingsHelper interface {
	CreateSectionTitleLabel(desc string) *widget.Label           // Creates a section title label.
	CreateSettingTitleLabel(desc string) *widget.Label           // Creates a setting title label.
	CreateSettingDescriptionLabel(desc string) fyne.CanvasObject // Creates a setting description label.
}

// SelectConfig holds the configuration for a generic select widget.
type SelectConfig struct {
	Name         string
	Options      []string
	InitialValue interface{}
	Label        fyne.CanvasObject
	HelpContent  fyne.CanvasObject
	OnChanged    func(string, interface{})
	ApplyFunc    func(interface{})
	NeedsRefresh bool        // Whether the UI needs a full refresh after applying
	EnabledIf    func() bool // Optional: function to determine if the widget should be enabled
}

// BoolConfig holds configuration for a generic boolean check widget.
type BoolConfig struct {
	Name         string
	InitialValue bool
	Label        fyne.CanvasObject
	HelpContent  fyne.CanvasObject
	OnChanged    func(bool)
	ApplyFunc    func(bool)
	NeedsRefresh bool
	EnabledIf    func() bool // Optional: function to determine if the widget should be enabled
}

// TextEntrySettingConfig holds configuration for a generic text entry widget.
type TextEntrySettingConfig struct {
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
	DisplayStatus      bool          // Whether to display the value status next to the entry
	IsPassword         bool          // Whether to mask the input (e.g. for API keys)
	EnabledIf          func() bool   // Optional: function to determine if the widget should be enabled
	ValidationDebounce time.Duration // Optional: delay before running PostValidateCheck (0 = synchronous)
}

// ButtonWithConfirmationConfig holds configuration for a button with confirmation dialog.
type ButtonWithConfirmationConfig struct {
	Name           string
	Label          fyne.CanvasObject
	HelpContent    fyne.CanvasObject
	ButtonText     string
	ConfirmTitle   string
	ConfirmMessage string
	OnPressed      func()
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
type SettingsManager interface {
	SettingsHelper

	CreateSelectSetting(cfg *SelectConfig, header *fyne.Container)                                 // Create a select setting widget.
	CreateBoolSetting(cfg *BoolConfig, header *fyne.Container) *widget.Check                       // Create a boolean setting widget.
	CreateTextEntrySetting(cfg *TextEntrySettingConfig, header *fyne.Container) *widget.Entry      // Create a text entry setting widget.
	CreateButtonWithConfirmationSetting(cfg *ButtonWithConfirmationConfig, header *fyne.Container) // Create a button setting with confirmation dialog widget.

	GetApplySettingsButton() *widget.Button                        //GetApplySettingsButton returns the Apply Changes button from the SettingsManager to be used in the UI.
	SetSettingChangedCallback(settingName string, callback func()) // Set a callback function to be called when a setting changes.
	RemoveSettingChangedCallback(settingName string)               // Remove a callback function associated with a specific setting.
	SetRefreshFlag(settingName string)                             // Set a flag to indicate that a specific setting needs a refresh.
	UnsetRefreshFlag(settingName string)                           // Unset the refresh flag for a specific setting.

	RegisterRefreshFunc(refreshFunc func())  // Register a function to be called when the settings need to be refreshed.
	RegisterOnSettingsSaved(callback func()) // Register a function to be called after settings are saved.
	GetSettingsWindow() fyne.Window          // GetSettingsWindow returns the window associated with the SettingsManager.
	GetCheckAndEnableApplyFunc() func()      // GetCheckAndEnableApplyFunction returns the check and enable apply function for the SettingsManager.
	RebuildTrayMenu()                        // Rebuilds the tray menu from scratch.
	// SeedBaseline seeds the initial state for a setting to track changes.
	SeedBaseline(name string, val interface{})
	// GetBaseline returns the initial state for a setting.
	GetBaseline(name string) interface{}
	// GetValue returns the live/current value for a setting from its valueGetter.
	GetValue(name string) interface{}
	// SetValue programmatically updates the live value of a setting.
	SetValue(name string, val interface{})
	// Refresh triggers all registered refresh functions immediately.
	Refresh()
}
