package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func normalize(s string) string {
	// Let's just strip everything except letters and numbers to be 100% sure
	reg, _ := regexp.Compile("[^a-zA-Z0-9]+")
	return strings.ToLower(reg.ReplaceAllString(s, ""))
}

func main() {
	enPath := "pkg/i18n/translations/en.json"
	enData, err := os.ReadFile(enPath)
	if err != nil {
		log.Fatalf("failed to read en.json: %v", err)
	}

	var enMap map[string]interface{}
	if err := json.Unmarshal(enData, &enMap); err != nil {
		log.Fatalf("failed to unmarshal en.json: %v", err)
	}

	// Create normalized map for faster lookups
	enNormMap := make(map[string]bool)
	fmt.Println("DEBUG: LOADING EN.JSON KEYS")
	for k := range enMap {
		norm := normalize(k)
		enNormMap[norm] = true
		if strings.Contains(norm, "sethowoftenthe") {
			fmt.Printf("DEBUG: Found key in en.json: %q (Norm: %q)\n", k, norm)
		}
	}

	// Regex to find i18n.T("..."), i18n.Tf("...", ...), i18n.N("...", ...)
	// This one is more robust for escaped quotes in double-quoted strings
	i18nRegex := regexp.MustCompile(`i18n\.(?:T|Tf|N)\(\s*(?:"((?:[^"\\]|\\.)*)"|` + "`" + `([^` + "`" + `]+)` + "`" + `)`)

	foundMissing := false
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == ".git" || d.Name() == "bin" || d.Name() == "cmd" {
				// Skipping cmd/util itself to avoid self-reference or noise
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.Contains(path, "_test.go") || strings.Contains(path, "zz_generated") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		matches := i18nRegex.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			rawKey := match[1]
			var key string
			if rawKey != "" {
				// Interpet escape sequences like \n, \t, etc.
				var err error
				key, err = strconv.Unquote("\"" + rawKey + "\"")
				if err != nil {
					key = rawKey
				}
			} else {
				key = match[2]
			}

			normKey := normalize(key)

			if !enNormMap[normKey] {
				fmt.Printf("MISSING KEY in en.json (Norm: %q)\n   Original: %q\n   File: %s\n", normKey, key, path)
				foundMissing = true
			}
		}

		return nil
	})

	if err != nil {
		log.Fatalf("error walking codebase: %v", err)
	}

	if foundMissing {
		fmt.Println("\nERROR: Some i18n keys are missing from en.json (or formatted differently)!")
		os.Exit(1)
	}

	fmt.Println("All i18n keys in code are present in en.json.")
}
