package main

import (
	"fmt"
	"log"

	"github.com/dixieflatline76/Spice/config"
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

	a := ui.GetInstance() // Get the Fyne application instance
	a.Run()               // Run the Fyne application
}
