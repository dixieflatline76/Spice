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
	EnabledIf  func() bool
	VisibleIf  func() bool
}

func (ButtonItem) isItemSchema() {}

// HyperlinkItem represents a clickable URL.
type HyperlinkItem struct {
	Text string
	URL  string
}

func (HyperlinkItem) isItemSchema() {}

// LabelItem represents static text or a sub-title.
type LabelItem struct {
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
}

// QueryListItem represents the abstraction for the list of queries/collections.
type QueryListItem struct {
	GetQueries    func() []Query
	EnableQuery   func(id string) error
	DisableQuery  func(id string) error
	RemoveQuery   func(id string) error
	GetDisplayURL func(Query) *url.URL
}

func (QueryListItem) isItemSchema() {}
