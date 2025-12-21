package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAdaptManifestForFirefox(t *testing.T) {
	// Input manifest
	input := `{
  "name": "Test Extension",
  "manifest_version": 3,
  "background": {
    "service_worker": "bg.js"
  }
}`

	// Execute
	output, err := adaptManifestForFirefox([]byte(input))
	if err != nil {
		t.Fatalf("adaptManifestForFirefox failed: %v", err)
	}

	// Parse output to verify structure
	var manifest map[string]interface{}
	if err := json.Unmarshal(output, &manifest); err != nil {
		t.Fatalf("Failed to unmarshal output JSON: %v", err)
	}

	// Verify existing keys preserved
	if manifest["name"] != "Test Extension" {
		t.Errorf("Name field lost or changed")
	}

	// Verify injected keys
	bss, ok := manifest["browser_specific_settings"].(map[string]interface{})
	if !ok {
		t.Fatalf("browser_specific_settings not found or invalid type")
	}

	gecko, ok := bss["gecko"].(map[string]interface{})
	if !ok {
		t.Fatalf("gecko settings not found")
	}

	if gecko["id"] != firefoxID {
		t.Errorf("Expected Gecko ID %s, got %v", firefoxID, gecko["id"])
	}

	// Verify data_collection_permissions
	dcp, ok := gecko["data_collection_permissions"].(map[string]interface{})
	if !ok {
		t.Fatalf("data_collection_permissions not found")
	}
	req, ok := dcp["required"].([]interface{})
	if !ok {
		t.Errorf("required property missing or not an array")
	}
	if len(req) != 1 || req[0] != "none" {
		t.Errorf("required array should be [\"none\"], got %v", req)
	}

	// Verify background conversion
	bg, ok := manifest["background"].(map[string]interface{})
	if !ok {
		t.Fatalf("background key missing")
	}
	if _, exists := bg["service_worker"]; exists {
		t.Errorf("service_worker key still exists, should have been removed")
	}
	scripts, ok := bg["scripts"].([]interface{})
	if !ok {
		t.Fatalf("scripts key missing or not an array")
	}
	if len(scripts) != 1 || scripts[0] != "bg.js" {
		t.Errorf("scripts array incorrect: %v", scripts)
	}
}

func TestZipDirectory(t *testing.T) {
	// Setup temporary source directory
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.Mkdir(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create dummy files
	if err := os.WriteFile(filepath.Join(srcDir, "manifest.json"), []byte(`{"name":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "icon.png"), []byte("fake image data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Target zip file
	zipFile := filepath.Join(tmpDir, "output.zip")

	// Define a modifier that changes "manifest.json"
	modifier := func(path string, content []byte) ([]byte, error) {
		if filepath.Base(path) == "manifest.json" {
			return []byte(`{"name":"modified"}`), nil
		}
		return content, nil
	}

	// Run zipDirectory
	if err := zipDirectory(srcDir, zipFile, modifier); err != nil {
		t.Fatalf("zipDirectory failed: %v", err)
	}

	// Verify zip contents
	r, err := zip.OpenReader(zipFile)
	if err != nil {
		t.Fatalf("Failed to open zip: %v", err)
	}
	defer r.Close()

	files := make(map[string][]byte)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		rc.Close()
		files[f.Name] = buf.Bytes()
	}

	// Check manifest was modified
	if !bytes.Equal(files["manifest.json"], []byte(`{"name":"modified"}`)) {
		t.Errorf("manifest.json was not modified as expected. Got: %s", string(files["manifest.json"]))
	}

	// Check icon was NOT modified
	if !bytes.Equal(files["icon.png"], []byte("fake image data")) {
		t.Errorf("icon.png was improperly modified")
	}
}
