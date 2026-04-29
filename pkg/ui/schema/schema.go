package schema

import (
	"context"
	"net/url"
	"time"
)

// ItemSchema is the common interface for all declarative UI elements.
type ItemSchema interface {
	isItemSchema() // Private marker method to ensure only our types implement the interface
}

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

// PanelSchema is the root of the declarative UI definition.
type PanelSchema struct {
	Sections []SectionSchema
}

// SectionSchema represents a logical grouping of settings.
type SectionSchema struct {
	ID          string // Unique identifier for this section (e.g., "met", "aic")
	Title       string
	Description string
	Compact     bool // If true, rendering will use high-density layouts
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

func (BoolItem) isItemSchema() {}

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
	SkipApply          bool // When true, changes won't register with the Apply button (e.g. verify-first fields)
	EnabledIf          func() bool
	VisibleIf          func() bool
}

func (TextItem) isItemSchema() {}

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

func (SelectItem) isItemSchema() {}

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
	IconName        string
	EnabledIf       func() bool
	VisibleIf       func() bool
}

func (AsyncButtonItem) isItemSchema() {}

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
	IconName       string
	EnabledIf      func() bool
	VisibleIf      func() bool
}

func (ConfirmButtonItem) isItemSchema() {}

// ButtonItem represents a standard action button.
type ButtonItem struct {
	Name       string
	Label      string
	Help       string
	ButtonText string
	Importance Importance
	OnPressed  func()
	IconName   string
	EnabledIf  func() bool
	VisibleIf  func() bool
}

func (ButtonItem) isItemSchema() {}

// HyperlinkItem represents a clickable URL.
type HyperlinkItem struct {
	ID   string
	Text string
	URL  string
}

func (HyperlinkItem) isItemSchema() {}

// LabelItem represents static text or a sub-title.
type LabelItem struct {
	ID         string
	Text       string
	IsTitle    bool
	Importance Importance // Low importance maps to muted description style
}

func (LabelItem) isItemSchema() {}

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

func (OAuthPickerItem) isItemSchema() {}

// FolderPickerItem represents a button that launches a directory selection dialog.
type FolderPickerItem struct {
	Name       string
	Label      string
	Help       string
	ButtonText string
	// OnFolderSelected is called when a folder is picked.
	// It is up to the provider logic to then add the query.
	OnFolderSelected func(path string) error
	EnabledIf        func() bool
	VisibleIf        func() bool
}

func (FolderPickerItem) isItemSchema() {}

// Query represents a generic abstract query data record to break dependency chains.
type Query struct {
	ID          string
	URL         string
	Description string
	Active      bool
	Managed     bool // Whether this query is managed by sync (cannot be deleted)
}

// QueryListItem represents the abstraction for the list of queries/collections.
type QueryListItem struct {
	ID                   string
	GetQueries           func() []Query
	EnableQuery          func(id string) error
	DisableQuery         func(id string) error
	RemoveQuery          func(id string) error
	GetDisplayText       func(Query) string // Optional: converts a query to its display label
	GetDisplayURL        func(Query) *url.URL
	DeleteLabel          string // Text for the action button. Default is "Delete".
	ForceActionEnabled   bool   // Enable the action button even for managed queries.
	DeleteConfirmMessage string // Custom message for the confirmation dialog.
}

func (QueryListItem) isItemSchema() {}

// HorizontalRowItem groups multiple items horizontally.
type HorizontalRowItem struct {
	ID    string
	Items []ItemSchema
}

func (HorizontalRowItem) isItemSchema() {}

// AddQueryConfig defines the configuration for a standardized Add Query modal.
type AddQueryConfig struct {
	Title           string
	Description     string // Optional description for the modal
	URLPlaceholder  string
	URLValidator    string
	URLErrorMsg     string
	DescPlaceholder string
	DescValidator   string
	DescErrorMsg    string

	// ValidateFunc is an optional custom validation logic.
	ValidateFunc func(url, desc string) error

	// AddHandler performs the actual addition of the query.
	// It returns the new query ID and an error if it fails.
	AddHandler func(desc, url string, active bool) (string, error)
}

// SecretItem represents a sensitive credential (like an API key).
type SecretItem struct {
	Name         string
	Label        string
	Help         string
	InitialValue string
	Placeholder  string
	OnVerify     func(key string) error
	OnClear      func()
	ApplyFunc    func(string)
}

func (SecretItem) isItemSchema() {}
