package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	providerFlag := flag.String("provider", "all", "Provider JSON filename without extension to sync (or all)")
	releaseFlag := flag.String("release", "", "Release version to stamp (e.g., v2.5.5)")
	flag.Parse()

	// Find all embedded JSON files in the providers directory
	pkgFiles, err := filepath.Glob("pkg/wallpaper/providers/*/*.json")
	if err != nil || len(pkgFiles) == 0 {
		fmt.Printf("Failed to find any provider JSON files: %v\n", err)
		os.Exit(1)
	}

	for _, pkgPath := range pkgFiles {
		filename := filepath.Base(pkgPath)
		providerName := filename[:len(filename)-len(filepath.Ext(filename))]
		docsPath := filepath.Join("docs", "collections", filename)

		// Filter by provider flag if 'all' is not specified
		if *providerFlag != "all" && *providerFlag != providerName {
			continue
		}

		if err := syncProvider(providerName, docsPath, pkgPath, *releaseFlag); err != nil {
			fmt.Printf("Failed to sync %s: %v\n", providerName, err)
			os.Exit(1)
		}
	}

	fmt.Println("Successfully synced collections.")
}

func syncProvider(name, docsPath, pkgPath, release string) error {
	fmt.Printf("Syncing %s...\n", name)

	// Read from Docs (Source of truth)
	data, err := os.ReadFile(docsPath)
	if err != nil {
		return fmt.Errorf("failed to read docs file: %w", err)
	}

	// Update version if release flag is provided
	if release != "" {
		var doc map[string]interface{}
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("failed to unmarshal docs file: %w", err)
		}
		doc["version"] = release

		data, err = json.MarshalIndent(doc, "", "    ")
		if err != nil {
			return fmt.Errorf("failed to marshal updated json: %w", err)
		}

		// Write back to Docs
		if err := os.WriteFile(docsPath, data, 0600); err != nil {
			return fmt.Errorf("failed to write docs file: %w", err)
		}
	}

	// Ensure destination directory exists
	pkgDir := filepath.Dir(pkgPath)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return fmt.Errorf("failed to create pkg dir: %w", err)
	}

	// Copy to Pkg
	if err := os.WriteFile(pkgPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write pkg file: %w", err)
	}

	return nil
}
