package pkg

import (
	"fyne.io/fyne/v2"
)

// Plugin is the interface that must be implemented by all plugins.
type Plugin interface {
	Name() string                                 // Returns the plugin's name.
	Activate()                                    // Called to activate the plugin.
	Deactivate()                                  // Called to deactivate the plugin.
	CreateTrayMenuItems() []*fyne.MenuItem        // Returns menu items to add to the tray.
	CreatePrefsPanel(fyne.Window) *fyne.Container // Returns a UI panel for preferences.
	Init(UIPluginManager)                         // Injects the preferences.
}

// Notifier is a function that notifies the user.
type Notifier func(title, message string)
