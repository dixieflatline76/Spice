package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/getty"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Println("Starting Virtual Gallery Generator...")

	// Determine project root to set the absolute destination directory
	wd, _ := os.Getwd()
	// If run via go generate from a provider directory, wd is that dir. If run from root, wd is root.
	// We'll walk up until we find go.mod to find the root.
	root := wd
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}

	baseDestDir := filepath.Join(root, "asset", "galleries")
	if err := os.MkdirAll(baseDestDir, 0755); err != nil {
		fmt.Printf("Error creating destination directory: %v\n", err)
		os.Exit(1)
	}

	cfg := &wallpaper.Config{}
	client := &http.Client{Timeout: 15 * time.Second}

	// Wait a moment for providers to initialize their embedded JSONs via goroutines
	time.Sleep(500 * time.Millisecond)

	fmt.Println("Generating Getty galleries...")
	gettyProvider := getty.NewProvider(cfg, client)
	
	// Wait a moment for getty to load embedded collection
	time.Sleep(1 * time.Second)

	gettyDest := filepath.Join(baseDestDir, gettyProvider.ID())
	if err := gettyProvider.GenerateGalleries(ctx, gettyDest); err != nil {
		fmt.Printf("Getty generator error: %v\n", err)
	} else {
		fmt.Println("Getty galleries generated successfully.")
	}

	fmt.Println("Done!")
}
