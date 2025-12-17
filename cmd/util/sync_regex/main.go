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

// Config map: Provider Name -> Path to const.go
var providers = map[string]string{
	"Wallhaven": "pkg/wallpaper/providers/wallhaven/const.go",
	"Pexels":    "pkg/wallpaper/providers/pexels/const.go",
	"Unsplash":  "pkg/wallpaper/providers/unsplash/const.go",
	"Wikimedia": "pkg/wallpaper/providers/wikimedia/const.go",
}

// Config map: Provider Name -> Constant Name to extract
var constNames = map[string]string{
	"Wallhaven": "WallhavenURLRegexp",
	"Pexels":    "PexelsURLRegexp",
	"Unsplash":  "UnsplashURLRegexp",
	"Wikimedia": "WikimediaURLRegexp",
}

func main() {
	log.Println("Syncing regex constants from Go to extension...")

	projectRoot, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	regexMap := make(map[string]string)

	for provider, relPath := range providers {
		fullPath := filepath.Join(projectRoot, relPath)
		regex, err := extractConstant(fullPath, constNames[provider])
		if err != nil {
			log.Fatalf("Failed to extract %s from %s: %v", constNames[provider], provider, err)
		}
		// Convert Go backticks to JS compatible string if needed
		// Usually Go raw strings `...` need to be converted to /.../ in JS if they are regexes.
		// But in JS, `new RegExp("string")` requires escaping backslashes.
		// Literal /.../ handles backslashes differently.
		// Our Go regexes are `^...$`.
		// To make it a JS regex literal: /pattern/
		// We need to ensure forward slashes are escaped?
		// e.g. https:// -> https:\/\/ (Go regex usually has strict escaping too).

		// Let's assume the Go regex string content is safe to put inside /.../
		// EXCEPT we must ensure forward slashes are escaped if we use / delimiter.

		// Wait, `extractConstant` returns the CONTENT of the string literal.
		// e.g. `^https:\/\/wallhaven\.cc\/...`
		// If we wrap it in /.../, it works IF Go string already escaped /.
		// Wallhaven URL regex in Go: `^https:\/\/...` -> Yes, it escapes /.

		regexMap[provider] = regex
		log.Printf("Found %s: %s...", provider, regex[:30])
	}

	newBlock := generateJSBlock(regexMap)

	// Update background.js
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

func generateJSBlock(regexMap map[string]string) string {
	var jsBuilder strings.Builder
	jsBuilder.WriteString("// REGEX_START\n")
	jsBuilder.WriteString("const SUPPORTED_PATTERNS = [\n")

	// Order matters? Not really, but let's be consistent.
	ordered := []string{"Wallhaven", "Pexels", "Unsplash", "Wikimedia"}

	for i, p := range ordered {
		regex := regexMap[p]
		if regex == "" {
			continue
		}
		jsBuilder.WriteString(fmt.Sprintf("    // %s\n", p))
		jsBuilder.WriteString(fmt.Sprintf("    /%s/", regex))
		if i < len(ordered)-1 {
			jsBuilder.WriteString(",")
		}
		jsBuilder.WriteString("\n")
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
							// Unescape double quotes if needed, but usually raw strings are used for regex
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
