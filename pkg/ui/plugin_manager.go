package ui

import (
	"net/url"

	"fyne.io/fyne/v2"
)

// PluginManager is the interface that must be implemented by all UI plugin managers.
type PluginManager interface {
	Register(Plugin)                                                      // Registers a plugin.
	Deregister(Plugin)                                                    // Deregisters a plugin.
	NotifyUser(string, string)                                            // Notifies the user.
	RegisterNotifier(Notifier)                                            // Registers a notifier.
	CreateMenuItem(string, func(), string) *fyne.MenuItem                 // Creates a menu item.
	CreateToggleMenuItem(string, func(bool), string, bool) *fyne.MenuItem // Creates a toggle menu item.
	OpenURL(*url.URL) error                                               // Opens a URL.
	GetPreferences() fyne.Preferences                                     // Returns the preferences.
}

// App is the interface that must be implemented by all applications.
type App interface {
	Start() // Bam starts the Spice application.
}
