package main

import (
	"fmt"

	"github.com/dixieflatline76/Spice/util/log"

	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/ui"

	"github.com/dixieflatline76/Spice/pkg/hotkey"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
)

var version = "0.0.0"

func init() {
	config.AppVersion = version
}

func main() {
	// Create a mutex to ensure only one instance is running
	ok, err := acquireLock()
	if err != nil {
		log.Fatalf("Failed to launch: %v", err)
	}
	if !ok {
		fmt.Println("Another instance of Wallhavener is already running.")
		return
	}
	defer releaseLock() // Make sure to release the lock when done

	spiceApp := ui.GetApplication() // Create a new Fyne application
	pm := ui.GetPluginManager()     // Get the plugin manager
	wallpaper.LoadPlugin(pm)        // Initialize the wallpaper plugin

	// Start global hotkey listeners (Register on main thread after app start)
	spiceApp.Lifecycle().SetOnStarted(func() {
		hotkey.StartListeners()
	})

	spiceApp.Start() // Run the application
}
