package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dixieflatline76/Spice/util/log"

	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/ui"

	"github.com/dixieflatline76/Spice/pkg/api"
	"github.com/dixieflatline76/Spice/pkg/hotkey"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	_ "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/favorites"
	_ "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/googlephotos"
	_ "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/pexels"

	// _ "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/unsplash"
	_ "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/metmuseum"
	_ "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/wallhaven"
	_ "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/wikimedia"
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

	// --- BROWSER INTEGRATION START ---
	// Start the Local API Server
	apiServer := api.NewServer()

	// Register Add Handler
	apiServer.SetAddCollectionHandler(func(url string) error {
		// Use the singleton instance to open UI
		return wallpaper.GetInstance().OpenAddCollectionUI(url)
	})

	go func() {
		// Register Namespaces for Local Assets
		tempDir := os.TempDir()
		gpPath := filepath.Join(tempDir, "spice", "google_photos")
		apiServer.RegisterNamespace("google_photos", gpPath)

		// Register favorites namespace to point to spice folder,
		// allowing favorite_images to be the collection ID.
		apiServer.RegisterNamespace(wallpaper.FavoritesNamespace, filepath.Join(tempDir, "spice"))

		favPath := filepath.Join(tempDir, "spice", wallpaper.FavoritesCollection)
		if err := os.MkdirAll(favPath, 0755); err != nil {
			log.Printf("Warning: Failed to create favorites directory: %v", err)
		}

		log.Printf("Starting Local API Server on :49452...")
		if err := apiServer.Start(); err != nil {
			log.Printf("Failed to start API Server: %v", err)
		}
	}()

	// Wire up Chrome OS Bridge if active
	wp := wallpaper.GetInstance()
	if bridge, ok := wp.GetOS().(*wallpaper.ChromeOS); ok {
		log.Printf("Chrome OS Bridge Activated. Wiring WebSocket Broadcaster...")
		bridge.RegisterBridge(apiServer.BroadcastWallpaper)
	}
	// --- BROWSER INTEGRATION END ---

	// Start the listener in a separate goroutine
	go func() {
		time.Sleep(500 * time.Millisecond)
		hotkey.StartListeners()
	}()

	spiceApp.Start() // Run the application
}
