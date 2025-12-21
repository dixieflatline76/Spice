package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	srcDir     = "extension"
	distDir    = "dist"
	chromeZip  = "spice-extension-chrome.zip"
	firefoxZip = "spice-extension-firefox.zip"
	firefoxID  = "spice-extension@dixieflatline76.github.io" // Placeholder ID
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Create dist directory
	if err := os.MkdirAll(distDir, 0755); err != nil {
		return fmt.Errorf("creating dist dir: %w", err)
	}

	// 1. Pack for Chrome (As Is)
	fmt.Println("Packing for Chrome...")
	if err := zipDirectory(srcDir, filepath.Join(distDir, chromeZip), nil); err != nil {
		return fmt.Errorf("packing chrome: %w", err)
	}
	fmt.Printf("Created %s\n", chromeZip)

	// 2. Pack for Firefox (With Manifest Injection)
	fmt.Println("Packing for Firefox...")
	manifestModifier := func(path string, content []byte) ([]byte, error) {
		if filepath.Base(path) == "manifest.json" {
			return adaptManifestForFirefox(content)
		}
		return content, nil
	}
	if err := zipDirectory(srcDir, filepath.Join(distDir, firefoxZip), manifestModifier); err != nil {
		return fmt.Errorf("packing firefox: %w", err)
	}
	fmt.Printf("Created %s\n", firefoxZip)

	return nil
}

// zipDirectory zips the contents of src into dest.
// modifier is an optional function to modify file content on the fly.
func zipDirectory(src, dest string, modifier func(path string, content []byte) ([]byte, error)) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	w := zip.NewWriter(out)
	defer w.Close()

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Get relative path for zip
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// Ensure forward slashes for zip
		relPath = filepath.ToSlash(relPath)

		// Create zip entry
		f, err := w.Create(relPath)
		if err != nil {
			return err
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Modify content if needed
		if modifier != nil {
			content, err = modifier(path, content)
			if err != nil {
				return fmt.Errorf("modifying %s: %w", path, err)
			}
		}

		_, err = f.Write(content)
		return err
	})
}

// adaptManifestForFirefox adds browser_specific_settings and converts
// background.service_worker to background.scripts for Firefox compatibility.
func adaptManifestForFirefox(content []byte) ([]byte, error) {
	var manifest map[string]interface{}
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, err
	}

	// 1. Inject Firefox ID
	manifest["browser_specific_settings"] = map[string]interface{}{
		"gecko": map[string]interface{}{
			"id":                 firefoxID,
			"strict_min_version": "109.0",
		},
	}
	// Add data_collection_permissions to gecko settings (Mozilla requirement Nov 2025)
	if gecko, ok := manifest["browser_specific_settings"].(map[string]interface{})["gecko"].(map[string]interface{}); ok {
		gecko["data_collection_permissions"] = map[string]interface{}{
			"data_collection": false,
		}
	}

	// 2. Convert service_worker to scripts (Firefox MV3 compatibility)
	if bg, ok := manifest["background"].(map[string]interface{}); ok {
		if sw, ok := bg["service_worker"].(string); ok {
			delete(bg, "service_worker")
			bg["scripts"] = []string{sw}
			manifest["background"] = bg
		}
	}

	return json.MarshalIndent(manifest, "", "  ")
}
