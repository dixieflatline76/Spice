package main

import (
	"fmt"
	"log"

	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/service"
	"github.com/dixieflatline76/Spice/ui"
)

var version = "0.0.0"

func init() {
	config.AppVersion = version
}

func main() {
	// Create a mutex to ensure only one instance is running
	ok, err := acquireLock()
	if err != nil {
		log.Fatalf("Failed to acquire lock: %v", err)
	}
	if !ok {
		fmt.Println("Another instance of Wallhavener is already running.")
		return
	}
	defer releaseLock() // Make sure to release the lock when done

	// Create the Fyne application
	a := ui.GetInstance()
	cfg := config.GetConfig(a.Preferences())
	log.Printf("Version: %v", config.AppVersion)

	// Create a function to send notifications to the user
	notifier := func(title, message string) {
		a.SendNotification(title, message)
	}

	// Start the wallpaper service
	go service.StartWallpaperService(cfg, notifier)

	if a != nil {
		a.Run() // Run the Fyne application
	}
}
