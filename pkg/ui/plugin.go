package ui

import (
	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
)

// Plugin is the interface that must be implemented by all plugins.
type Plugin interface {
	ID() string                                               // ID returns a stable, non-localized provider ID.
	Name() string                                             // Name returns the localized provider name.
	Activate()                                                // Called to activate the plugin.
	Deactivate()                                              // Called to deactivate the plugin.
	CreateTrayMenuItems() []*fyne.MenuItem                    // Returns menu items to add to the tray.
	CreatePrefsPanel(setting.SettingsManager) *fyne.Container // Returns a UI panel for preferences.
	Init(PluginManager)                                       // Injects the preferences.
}

// Notifier is a function that notifies the user.
type Notifier func(title, message string)
