package pkg

import (
	"net/url"

	"fyne.io/fyne/v2"
)

// PluginManager is the interface that must be implemented by all plugin managers.
type PluginManager interface {
	Register(Plugin)   // Registers a plugin.
	Deregister(Plugin) // Deregisters a plugin.
}

// UIPluginManager is the interface that must be implemented by all UI plugin managers.
type UIPluginManager interface {
	PluginManager
	NotifyUser(string, string)                                            // Notifies the user.
	RegisterNotifier(Notifier)                                            // Registers a notifier.
	CreateMenuItem(string, func(), string) *fyne.MenuItem                 // Creates a menu item.
	CreateToggleMenuItem(string, func(bool), string, bool) *fyne.MenuItem // Creates a toggle menu item.
	OpenURL(*url.URL) error                                               // Opens a URL.
	GetPreferences() fyne.Preferences                                     // Returns the preferences.
}
