package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyMapValidity(t *testing.T) {
	// Replicating the keyMap from main.go
	keyMap := map[string]string{
		"GOOGLE_PHOTOS_CLIENT_ID":     "github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/googlephotos.GoogleClientID",
		"GOOGLE_PHOTOS_CLIENT_SECRET": "github.com/dixieflatline76/Spice/v2/pkg/wallpaper/providers/googlephotos.GoogleClientSecret",
	}

	modulePrefix := "github.com/dixieflatline76/Spice/v2/"

	for envKey, fullPath := range keyMap {
		t.Run(envKey, func(t *testing.T) {
			if !strings.HasPrefix(fullPath, modulePrefix) {
				t.Fatalf("Path %s does not start with module prefix %s", fullPath, modulePrefix)
			}

			// Extract relative path and variable name
			// Example: pkg/wallpaper/providers/googlephotos.GoogleClientID
			relWithVar := strings.TrimPrefix(fullPath, modulePrefix)
			lastDot := strings.LastIndex(relWithVar, ".")
			if lastDot == -1 {
				t.Fatalf("Invalid path format: %s (missing dot)", fullPath)
			}

			relDir := relWithVar[:lastDot]
			varName := relWithVar[lastDot+1:]

			// Find the physical directory
			// Note: The test runs in cmd/util/load_secrets, so we need to go up 3 levels to reach root
			absDir, err := filepath.Abs(filepath.Join("..", "..", "..", relDir))
			if err != nil {
				t.Fatalf("Failed to resolve directory: %v", err)
			}

			if _, err := os.Stat(absDir); os.IsNotExist(err) {
				t.Fatalf("Directory does not exist: %s", absDir)
			}

			// Parse all Go files in the directory
			fset := token.NewFileSet()
			pkgs, err := parser.ParseDir(fset, absDir, nil, 0)
			if err != nil {
				t.Fatalf("Failed to parse directory %s: %v", absDir, err)
			}

			found := false
			for _, pkg := range pkgs {
				for _, file := range pkg.Files {
					for _, decl := range file.Decls {
						genDecl, ok := decl.(*ast.GenDecl)
						if !ok || genDecl.Tok != token.VAR {
							continue
						}
						for _, spec := range genDecl.Specs {
							valueSpec, ok := spec.(*ast.ValueSpec)
							if !ok {
								continue
							}
							for _, id := range valueSpec.Names {
								if id.Name == varName {
									found = true
									break
								}
							}
							if found {
								break
							}
						}
						if found {
							break
						}
					}
					if found {
						break
					}
				}
				if found {
					break
				}
			}

			if !found {
				t.Errorf("Variable %s NOT FOUND in package %s (path: %s). Regression detected!", varName, relDir, fullPath)
			} else {
				fmt.Printf("✓ Validated mapping: %s -> %s\n", envKey, fullPath)
			}
		})
	}
}
