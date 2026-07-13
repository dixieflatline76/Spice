package ui

import (
	"net/url"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/v2/asset"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
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
	OpenPreferences(tab string)                                           // OpenPreferences opens the preferences window, optionally navigating to a specific tab.
	GetPreferences() fyne.Preferences                                     // Returns the preferences.
	GetAssetManager() *asset.Manager                                      // Returns the asset manager.
	RefreshTrayMenu()                                                     // Refreshes the tray menu.
	RebuildTrayMenu()                                                     // Rebuilds the tray menu from scratch.

	// ShowTuneImagePopup displays the popup for tuning images (crop anchor, framing overrides).
	// The outer ring (UI layer) owns window lifecycle and OpenGL error recovery.
	// onSelect receives the chosen tuning options and a done callback to invoke after processing.
	ShowTuneImagePopup(monitorID int, currentOpts provider.TuningOptions, effectiveOpts provider.TuningOptions, labels [9]string, values [9]provider.CropAnchor, onSelect func(opts provider.TuningOptions, onDone func()))
}

// App is the interface that must be implemented by all applications.
type App interface {
	Start()                    // Bam starts the Spice application.
	Lifecycle() fyne.Lifecycle // Lifecycle returns the application lifecycle.
}
