package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot returns the repository root by walking up from the test file location.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file path")
	}
	// We're in cmd/util/sync_collections/, walk up 3 levels to repo root
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root not found at %s: %v", root, err)
	}
	return root
}

// providers lists all museum providers and their file paths relative to repo root.
var providers = []struct {
	Name    string
	DocsRel string
	PkgRel  string
}{
	{"artic", "docs/collections/artic.json", "pkg/wallpaper/providers/artic/artic.json"},
	{"cleveland", "docs/collections/cleveland.json", "pkg/wallpaper/providers/cleveland/cleveland.json"},
	{"metmuseum", "docs/collections/met.json", "pkg/wallpaper/providers/metmuseum/met.json"},
	{"rijksmuseum", "docs/collections/rijksmuseum.json", "pkg/wallpaper/providers/rijksmuseum/rijksmuseum.json"},
}

// TestDocsAndPkgInSync verifies that every docs/ JSON file is byte-identical to its pkg/ copy.
// The sync_collections utility is responsible for keeping these in sync.
func TestDocsAndPkgInSync(t *testing.T) {
	root := repoRoot(t)

	for _, p := range providers {
		t.Run(p.Name, func(t *testing.T) {
			docsPath := filepath.Join(root, filepath.FromSlash(p.DocsRel))
			pkgPath := filepath.Join(root, filepath.FromSlash(p.PkgRel))

			docsData, err := os.ReadFile(docsPath)
			if err != nil {
				t.Fatalf("failed to read docs file %s: %v", docsPath, err)
			}
			pkgData, err := os.ReadFile(pkgPath)
			if err != nil {
				t.Fatalf("failed to read pkg file %s: %v", pkgPath, err)
			}

			if string(docsData) != string(pkgData) {
				t.Errorf("docs/ and pkg/ files are out of sync for %s.\nRun: go run cmd/util/sync_collections/main.go", p.Name)
			}
		})
	}
}

// TestAllCollectionsHaveSemverVersion verifies the version field is a valid semver string.
func TestAllCollectionsHaveSemverVersion(t *testing.T) {
	root := repoRoot(t)

	for _, p := range providers {
		t.Run(p.Name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p.DocsRel)))
			if err != nil {
				t.Fatalf("failed to read %s: %v", p.DocsRel, err)
			}

			var doc map[string]interface{}
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatalf("failed to parse %s: %v", p.DocsRel, err)
			}

			version, ok := doc["version"]
			if !ok {
				t.Fatal("missing 'version' field")
			}

			vStr, ok := version.(string)
			if !ok {
				t.Fatalf("'version' field is not a string, got %T", version)
			}

			if !strings.HasPrefix(vStr, "v") {
				t.Errorf("version %q does not start with 'v' (expected semver like v2.5.5)", vStr)
			}
		})
	}
}

// TestAllCollectionsParseAsValidJSON validates that each JSON file is well-formed.
func TestAllCollectionsParseAsValidJSON(t *testing.T) {
	root := repoRoot(t)

	for _, p := range providers {
		t.Run(p.Name+"_docs", func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p.DocsRel)))
			if err != nil {
				t.Fatalf("failed to read: %v", err)
			}
			if !json.Valid(data) {
				t.Error("docs JSON is not valid")
			}
		})
		t.Run(p.Name+"_pkg", func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p.PkgRel)))
			if err != nil {
				t.Fatalf("failed to read: %v", err)
			}
			if !json.Valid(data) {
				t.Error("pkg JSON is not valid")
			}
		})
	}
}

// TestDefaultConfigIsValidJSON validates the default_config.json that ships with the binary.
func TestDefaultConfigIsValidJSON(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "asset", "text", "default_config.json"))
	if err != nil {
		t.Fatalf("failed to read default_config.json: %v", err)
	}
	if !json.Valid(data) {
		t.Fatal("default_config.json is not valid JSON")
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse default_config.json: %v", err)
	}

	queries, ok := cfg["queries"].([]interface{})
	if !ok {
		t.Fatal("'queries' field missing or not an array")
	}
	if len(queries) == 0 {
		t.Fatal("default_config.json has no queries")
	}

	// Verify each query has the required fields
	for i, q := range queries {
		qMap, ok := q.(map[string]interface{})
		if !ok {
			t.Errorf("query %d is not an object", i)
			continue
		}
		for _, field := range []string{"desc", "url", "provider"} {
			if _, exists := qMap[field]; !exists {
				t.Errorf("query %d (%v) missing '%s' field", i, qMap["desc"], field)
			}
		}
	}
}

// TestDefaultConfigMuseumQueryKeysExist validates that museum query URLs in default_config.json
// correspond to real collection keys in their respective JSON files.
func TestDefaultConfigMuseumQueryKeysExist(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "asset", "text", "default_config.json"))
	if err != nil {
		t.Fatalf("failed to read default_config.json: %v", err)
	}

	var cfg struct {
		Queries []struct {
			Desc     string `json:"desc"`
			URL      string `json:"url"`
			Provider string `json:"provider"`
		} `json:"queries"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse default_config.json: %v", err)
	}

	// Map provider names to their JSON file paths
	providerFiles := map[string]string{
		"MetMuseum":           "docs/collections/met.json",
		"ArtInstituteChicago": "docs/collections/artic.json",
		"Rijksmuseum":         "docs/collections/rijksmuseum.json",
		"ClevelandMuseum":     "docs/collections/cleveland.json",
	}

	for _, q := range cfg.Queries {
		jsonFile, isMuseum := providerFiles[q.Provider]
		if !isMuseum {
			continue // Skip non-museum providers (Wallhaven, Wikimedia, etc.)
		}

		t.Run(q.Provider+"/"+q.URL, func(t *testing.T) {
			collData, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(jsonFile)))
			if err != nil {
				t.Fatalf("failed to read %s: %v", jsonFile, err)
			}

			// AIC uses a different structure ("tours" map) vs others ("collections" array)
			if q.Provider == "ArtInstituteChicago" {
				var aic struct {
					Tours map[string]interface{} `json:"tours"`
				}
				if err := json.Unmarshal(collData, &aic); err != nil {
					t.Fatalf("failed to parse: %v", err)
				}
				if _, exists := aic.Tours[q.URL]; !exists {
					t.Errorf("collection key %q not found in %s tours", q.URL, jsonFile)
				}
			} else {
				var col struct {
					Collections []struct {
						Key string `json:"key"`
					} `json:"collections"`
				}
				if err := json.Unmarshal(collData, &col); err != nil {
					t.Fatalf("failed to parse: %v", err)
				}
				found := false
				for _, c := range col.Collections {
					if c.Key == q.URL {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("collection key %q not found in %s", q.URL, jsonFile)
				}
			}
		})
	}
}
