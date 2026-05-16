//go:generate go run ../util/gen_providers/main.go

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dixieflatline76/Spice/v2/util/log"

	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/ui"

	"fyne.io/fyne/v2/app"

	"github.com/dixieflatline76/Spice/v2/pkg/api"
	"github.com/dixieflatline76/Spice/v2/pkg/hotkey"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
)

var version = "0.0.0"

// breadcrumb writes a diagnostic message to stderr for sysdiagnose capture.
// These bypass the file logger and appear in Console.app, allowing DTS engineers
// to see exactly where the app dies during App Store review.
func breadcrumb(msg string) {
	fmt.Fprintf(os.Stderr, "[Spice:%s] %s\n", version, msg)
}

func init() {
	config.AppVersion = version
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-probe-gl" {
		// Suppress Windows Error Reporting crash dialogs for this subprocess.
		// When OpenGL is broken, Fyne calls os.Exit(1) — we want that to happen
		// silently without showing "Spice has stopped working" to the user.
		suppressCrashDialogs()

		// Run a bare minimum Fyne window creation.
		// If OpenGL is unavailable, Fyne will log the error and call os.Exit(1).
		// If it succeeds, it will proceed to os.Exit(0).
		// This protects the main app from Fyne's hard crash.
		a := app.New()
		a.NewWindow("Probe")
		os.Exit(0)
	}

	breadcrumb("main() entered")

	// Create a mutex to ensure only one instance is running
	ok, err := acquireLock()
	if err != nil {
		breadcrumb(fmt.Sprintf("acquireLock failed: %v", err))
		log.Fatalf("Failed to launch: %v", err)
	}
	if !ok {
		breadcrumb("acquireLock returned false — another instance detected")
		fmt.Println("Another instance of Spice is already running.")
		return
	}
	defer releaseLock() // Make sure to release the lock when done
	breadcrumb("lock acquired")

	spiceApp := ui.GetApplication() // Create a new Fyne application
	breadcrumb("UI application initialized")

	pm := ui.GetPluginManager() // Get the plugin manager
	wallpaper.LoadPlugin(pm)    // Initialize the wallpaper plugin
	breadcrumb("wallpaper plugin loaded")

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
		appDir := config.GetAppDir()
		apiServer.RegisterNamespace("google_photos", filepath.Join(appDir, "google_photos")) // Point directly to google_photos subfolder

		// Register favorites namespace to point to spice folder,
		// allowing favorite_images to be the collection ID.
		apiServer.RegisterNamespace(wallpaper.FavoritesNamespace, appDir)

		// Wire up dynamic resolver for Local Folder provider
		// This allows user-selected folders to be served via the local API
		// without needing static namespace registration per folder.
		apiServer.SetDynamicResolver(func(namespace, collectionID string) (string, bool) {
			if namespace != wallpaper.LocalFolderNamespace {
				return "", false
			}
			for _, q := range wallpaper.GetConfigInstance().GetLocalFolderQueries() {
				if wallpaper.HashFolderPath(q.URL) == collectionID {
					return q.URL, true
				}
			}
			return "", false
		})

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
		hotkey.StartListeners(ui.GetPluginManager())
	}()

	breadcrumb("entering Fyne event loop")
	spiceApp.Start() // Run the application
}
