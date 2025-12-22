package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Known Providers Config: Map from Import Path Suffix -> Path to const.go
var knownProvidersConfig = map[string]struct {
	ConstPath string
	ConstName string
}{
	"wallhaven": {
		ConstPath: "pkg/wallpaper/providers/wallhaven/const.go",
		ConstName: "WallhavenURLRegexp",
	},
	"pexels": {
		ConstPath: "pkg/wallpaper/providers/pexels/const.go",
		ConstName: "PexelsURLRegexp",
	},
	"unsplash": {
		ConstPath: "pkg/wallpaper/providers/unsplash/const.go",
		ConstName: "UnsplashURLRegexp",
	},
	"wikimedia": {
		ConstPath: "pkg/wallpaper/providers/wikimedia/const.go",
		ConstName: "WikimediaURLRegexp",
	},
}

const mainAppPath = "cmd/spice/main.go"

func main() {
	log.Println("Syncing regex constants from Go to extension...")

	projectRoot, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// 1. Parse main.go to find enabled providers
	enabledProviders, err := getEnabledProviders(filepath.Join(projectRoot, mainAppPath))
	if err != nil {
		log.Fatalf("Failed to parse main.go: %v", err)
	}
	log.Printf("Detected enabled providers: %v", enabledProviders)

	regexMap := make(map[string]string)

	// 2. Extract regex for each enabled provider
	for _, providerKey := range enabledProviders {
		config, exists := knownProvidersConfig[providerKey]
		if !exists {
			log.Printf("Warning: Provider '%s' enabled but not configured for regex sync. Skipping.", providerKey)
			continue
		}

		fullPath := filepath.Join(projectRoot, config.ConstPath)
		regex, err := extractConstant(fullPath, config.ConstName)
		if err != nil {
			log.Fatalf("Failed to extract %s from %s: %v", config.ConstName, providerKey, err)
		}

		regexMap[providerKey] = regex
		log.Printf("Found Regex for %s: %s...", providerKey, regex[:30])
	}

	// 3. Generate JS Block
	newBlock := generateJSBlock(regexMap)

	// 4. Update background.js
	extPath := filepath.Join(projectRoot, "extension", "background.js")
	content, err := os.ReadFile(extPath)
	if err != nil {
		log.Fatal(err)
	}

	finalContent, err := updateJSContent(string(content), newBlock)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(extPath, []byte(finalContent), 0644)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Successfully synced regex constants to background.js")
}

// getEnabledProviders parses main.go imports to find enabled providers
func getEnabledProviders(mainPath string) ([]string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, mainPath, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	var providers []string
	prefix := "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/"

	for _, imp := range node.Imports {
		// imp.Path.Value includes quotes, e.g. "github.com/..."
		path := strings.Trim(imp.Path.Value, "\"")

		if strings.HasPrefix(path, prefix) {
			providerName := strings.TrimPrefix(path, prefix)
			providers = append(providers, providerName)
		}
	}
	return providers, nil
}

func generateJSBlock(regexMap map[string]string) string {
	var jsBuilder strings.Builder
	jsBuilder.WriteString("// REGEX_START\n")
	jsBuilder.WriteString("const SUPPORTED_PATTERNS = [\n")

	// Iterate over the regexMap (order is random, but that's fine for JS array)
	// Or we can sort keys for stability.
	// Let's rely on standard map iteration, but maybe better to enable stability.
	// Actually, iterating map is random. Let's iterate over knownProvidersConfig keys in fixed order
	// and check if they exist in regexMap.

	orderedKeys := []string{"wallhaven", "pexels", "unsplash", "wikimedia"}

	for i, key := range orderedKeys {
		regex, exists := regexMap[key]
		if !exists {
			continue
		}

		// Capitalize for display comment
		display := strings.Title(key)
		jsBuilder.WriteString(fmt.Sprintf("    // %s\n", display))
		jsBuilder.WriteString(fmt.Sprintf("    /%s/", regex))

		// Add comma if not the last item?
		// Simpler: Add comma always, trailing comma is valid in JS arrays.
		jsBuilder.WriteString(",\n")
		_ = i
	}
	jsBuilder.WriteString("];\n")
	jsBuilder.WriteString("// REGEX_END")
	return jsBuilder.String()
}

func updateJSContent(original, newBlock string) (string, error) {
	startMarker := "// REGEX_START"
	endMarker := "// REGEX_END"

	startIdx := strings.Index(original, startMarker)
	endIdx := strings.Index(original, endMarker)

	if startIdx == -1 || endIdx == -1 {
		return "", fmt.Errorf("could not find REGEX markers in background.js")
	}

	return original[:startIdx] + newBlock + original[endIdx+len(endMarker):], nil
}

func extractConstant(filePath string, constName string) (string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return "", err
	}

	var value string
	found := false

	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		if genDecl, ok := n.(*ast.GenDecl); ok && genDecl.Tok == token.CONST {
			for _, spec := range genDecl.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				for i, name := range valueSpec.Names {
					if name.Name == constName {
						// Found the constant
						basicLit, ok := valueSpec.Values[i].(*ast.BasicLit)
						if !ok {
							continue
						}
						// Strip quotes/backticks
						val := basicLit.Value
						if strings.HasPrefix(val, "`") && strings.HasSuffix(val, "`") {
							value = val[1 : len(val)-1]
						} else if strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"") {
							value = val[1 : len(val)-1]
						} else {
							value = val
						}
						found = true
						return false
					}
				}
			}
		}
		return true
	})

	if !found {
		return "", fmt.Errorf("constant not found")
	}
	return value, nil
}
