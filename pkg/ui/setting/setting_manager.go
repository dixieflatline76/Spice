package setting

import (
	"context"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// ButtonStyle represents the visual priority of a button in a pure Go way.
type ButtonStyle string

const (
	ButtonStyleDefault ButtonStyle = "default"
	ButtonStylePrimary ButtonStyle = "primary"
	ButtonStyleDanger  ButtonStyle = "danger"
	ButtonStyleSuccess ButtonStyle = "success"
)

// Importance defines the visual weight of a UI element.
type Importance string

const (
	ImportanceHigh    Importance = "high"
	ImportanceMedium  Importance = "medium"
	ImportanceLow     Importance = "low"
	ImportanceSuccess Importance = "success"
	ImportanceDanger  Importance = "danger"
)

// ItemSchema is the common interface for all declarative UI elements.
type ItemSchema interface {
	isItemSchema() // Private marker method to ensure only our types implement the interface
}

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
	VisibleIf    func() bool // Optional: function to determine if the widget should be visible
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
	VisibleIf    func() bool // Optional: function to determine if the widget should be visible
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
	VisibleIf          func() bool   // Optional: function to determine if the widget should be visible
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
	Importance     widget.Importance
	OnPressed      func()
	EnabledIf      func() bool // Optional: function to determine if the widget should be enabled
	VisibleIf      func() bool // Optional: function to determine if the widget should be visible
}

// AsyncButtonConfig holds configuration for a background/network task button.
type AsyncButtonConfig struct {
	Name            string
	ButtonText      string
	LoadingText     string
	Importance      widget.Importance
	OnPressed       func() error // Executed in background thread
	OnCompleted     func(error)  // Executed in UI thread after OnPressed completes
	TargetStatusKey string       // Optional: name of a setting whose status label should be updated on completion
	NeedsRefresh    bool         // Whether the UI needs a full refresh after completion
	EnabledIf       func() bool  // Optional: function to determine if the widget should be enabled
	VisibleIf       func() bool  // Optional: function to determine if the widget should be visible
}

// ButtonConfig holds configuration for a standard action button.
type ButtonConfig struct {
	Name        string
	Label       fyne.CanvasObject
	HelpContent fyne.CanvasObject
	ButtonText  string
	Importance  widget.Importance
	OnPressed   func()
	EnabledIf   func() bool // Optional: function to determine if the widget should be enabled
	VisibleIf   func() bool // Optional: function to determine if the widget should be visible
}

// OAuthPickerItem defines the contract for complex OAuth sessions and picker workflows.
type OAuthPickerItem struct {
	Name  string
	Label string
	Help  string

	// Pure Domain Core Logic
	CheckAuthStatus func() (isAuth bool, isExpired bool)
	OnAuthorize     func() error
	OnDisconnect    func() error

	// Async Polling & Download (Takes a pure string callback for UI stream updates)
	OnLaunchPicker func(ctx context.Context, updateStatus func(string)) (itemCount int, guid string, err error)

	// Collection Persistence Hooks
	OnSaveCollection   func(guid string, description string, active bool) error
	OnCancelCollection func(guid string)
}

func (*OAuthPickerItem) isItemSchema() {}

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
type SettingsManager interface {
	SettingsHelper

	CreateSelectSetting(cfg *SelectConfig, header *fyne.Container)                                 // Create a select setting widget.
	CreateBoolSetting(cfg *BoolConfig, header *fyne.Container) *widget.Check                       // Create a boolean setting widget.
	CreateTextEntrySetting(cfg *TextEntrySettingConfig, header *fyne.Container) *widget.Entry      // Create a text entry setting widget.
	CreateButtonWithConfirmationSetting(cfg *ButtonWithConfirmationConfig, header *fyne.Container) // Create a button setting with confirmation dialog widget.
	CreateButtonSetting(cfg *ButtonConfig, header *fyne.Container)                                 // Create a standard button setting widget.
	CreateAsyncButton(cfg *AsyncButtonConfig, header *fyne.Container) *widget.Button               // Create a button that handles background tasks and UI thread transitions.

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
	// HasPendingChange returns true if the user has toggled a setting but not yet applied.
	HasPendingChange(name string) bool
	// Refresh triggers all registered refresh functions immediately.
	Refresh()
	// CommitSetting atomically reads the current UI value, applies it to the native setter, and updates the baseline.
	CommitSetting(name string)
	// ResetSettings atomically clears multiple settings, updates native getters, and resyncs baselines.
	ResetSettings(resets ...SettingReset)

	// SetSettingStatus programmatically updates a setting's status label (thread-safe).
	SetSettingStatus(name string, message string, importance Importance)

	// RenderSchema takes a pure Go UI definition and renders it to a Fyne container.
	RenderSchema(schema PanelSchema) fyne.CanvasObject
}

// PanelSchema is the root of the declarative UI definition.
type PanelSchema struct {
	Sections []SectionSchema
}

// SectionSchema represents a logical grouping of settings.
type SectionSchema struct {
	Title       string
	Description string
	Items       []ItemSchema
}

// BoolItem represents a checkbox/toggle.
type BoolItem struct {
	Name         string
	Label        string
	Help         string
	InitialValue bool
	OnChanged    func(bool)
	ApplyFunc    func(bool)
	NeedsRefresh bool
	EnabledIf    func() bool
	VisibleIf    func() bool
}

func (b BoolItem) isItemSchema() {}

// TextItem represents a text entry field.
type TextItem struct {
	Name               string
	Label              string
	Help               string
	InitialValue       string
	PlaceHolder        string
	IsPassword         bool
	DisplayStatus      bool
	ValidationDebounce time.Duration
	Validator          func(string) error // Pure Go validator
	PostValidateCheck  func(string) error
	OnChanged          func(string)
	ApplyFunc          func(string)
	NeedsRefresh       bool
	EnabledIf          func() bool
	VisibleIf          func() bool
}

func (t TextItem) isItemSchema() {}

// SelectItem represents a dropdown/select field.
type SelectItem struct {
	Name         string
	Label        string
	Help         string
	Options      []string
	InitialValue interface{}
	OnChanged    func(string, interface{})
	ApplyFunc    func(interface{})
	NeedsRefresh bool
	EnabledIf    func() bool
	VisibleIf    func() bool
}

func (s SelectItem) isItemSchema() {}

// AsyncButtonItem represents a button that performs a background task.
type AsyncButtonItem struct {
	Name            string
	ButtonText      string
	LoadingText     string
	Style           ButtonStyle
	TargetStatusKey string
	OnPressed       func() error
	OnCompleted     func(error)
	NeedsRefresh    bool
	EnabledIf       func() bool
	VisibleIf       func() bool
}

func (a AsyncButtonItem) isItemSchema() {}

// ConfirmButtonItem represents a button that requires confirmation before execution.
type ConfirmButtonItem struct {
	Name           string
	Label          string
	Help           string
	ButtonText     string
	ConfirmTitle   string
	ConfirmMessage string
	Importance     Importance
	OnPressed      func()
	EnabledIf      func() bool
	VisibleIf      func() bool
}

func (c ConfirmButtonItem) isItemSchema() {}

// ButtonItem represents a standard action button.
type ButtonItem struct {
	Name       string
	Label      string
	Help       string
	ButtonText string
	Importance Importance
	OnPressed  func()
	EnabledIf  func() bool
	VisibleIf  func() bool
}

func (b ButtonItem) isItemSchema() {}

// HyperlinkItem represents a clickable URL.
type HyperlinkItem struct {
	Text string
	URL  string
}

func (h HyperlinkItem) isItemSchema() {}

// LabelItem represents static text or a sub-title.
type LabelItem struct {
	Text       string
	IsTitle    bool
	Importance Importance // Low importance maps to muted description style
}

func (l LabelItem) isItemSchema() {}
