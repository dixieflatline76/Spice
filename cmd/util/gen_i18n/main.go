package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Language struct {
	Code string
	Name string
}

// allowIdenticalToEnglish is a set of keys that are legitimately
// the same string in English and in other languages (proper nouns,
// international loanwords, etc.). These are excluded from the
// "untranslated" check.
var allowIdenticalToEnglish = map[string]bool{
	// Proper nouns — museum names used internationally in English
	"Art Institute of Chicago":       true,
	"The Metropolitan Museum of Art": true,
	"Rijksmuseum":                    true,

	// International loanwords / identical across many languages
	"App":       true,
	"Community": true,
	"General":   true,
	"Images":    true,
	"Museums":   true,
	"Personal":  true,
	"System":    true,
	"Actions":   true,

	// Brand names / tech terms kept in English
	"Pexels":                  true,
	"Wikimedia":               true,
	"Wikimedia Commons":       true,
	"Google Photos":           true,
	"Google Photos Extension": true,
	"wallhaven":               true,
	"Open Access (CC0)":       true,
	"CC0 - Public Domain":     true,

	// Locations kept in their international form
	"Chicago, IL, USA":       true,
	"New York City, USA":     true,
	"Amsterdam, Netherlands": true,

	// Short functional strings identical in many languages
	"Error: ": true,

	// Attribution keys where some languages use the same prefix as English
	"attribution_in": true,

	// Internal keys that use the key as the value
	"_meta_name": true,
}

// dynamicI18nKeys are translation keys that exist in the JSON files but
// are used at runtime via variables (e.g. i18n.Tf(key, ...)) rather than
// direct i18n.T("literal") calls. The static regex extractor cannot detect
// these, so we explicitly register them to prevent false "stale" warnings.
var dynamicI18nKeys = map[string]bool{
	// Attribution format strings — selected dynamically based on provider.AttributionType
	"attribution_by": true,
	"attribution_in": true,

	// Museum collection names — used as data-driven labels from curated JSON collections
	"Egyptian Art":       true,
	"European Paintings": true,
}

func main() {
	translationsDir := "pkg/i18n/translations"
	outputFile := "pkg/i18n/zz_generated_languages.go"
	codeRoot := "."

	// If we are running inside pkg/i18n (e.g. via go generate), adjust the paths
	if _, err := os.Stat("translations"); err == nil {
		translationsDir = "translations"
		outputFile = "zz_generated_languages.go"
		codeRoot = "../.." // Navigate up to project root
	}

	// Parse --check flag
	checkMode := false
	for _, arg := range os.Args[1:] {
		if arg == "--check" {
			checkMode = true
		}
	}

	files, err := os.ReadDir(translationsDir)
	if err != nil {
		log.Fatalf("failed to read translations dir (%s): %v", translationsDir, err)
	}

	// ──────────────────────────────────────────────────────────────────
	// Phase 1: Extract all i18n keys from Go source code
	// ──────────────────────────────────────────────────────────────────
	codeKeys := extractKeysFromCode(codeRoot)

	// Merge in dynamic keys that can't be detected by static regex analysis
	for k := range dynamicI18nKeys {
		codeKeys[k] = true
	}
	fmt.Printf("Extracted %d unique i18n keys from source code (%d static + %d dynamic)\n",
		len(codeKeys), len(codeKeys)-len(dynamicI18nKeys), len(dynamicI18nKeys))

	// ──────────────────────────────────────────────────────────────────
	// Phase 2: Load en.json as the reference
	// ──────────────────────────────────────────────────────────────────
	enPath := filepath.Join(translationsDir, "en.json")
	enData, err := os.ReadFile(enPath)
	if err != nil {
		log.Fatalf("failed to read %s: %v", enPath, err)
	}

	var enMap map[string]interface{}
	if err := json.Unmarshal(enData, &enMap); err != nil {
		log.Fatalf("failed to unmarshal %s: %v", enPath, err)
	}

	// Preserve and remove metadata
	metaName := enMap["_meta_name"]
	delete(enMap, "_meta_name")

	existingKeyCount := len(enMap)
	fmt.Printf("Existing en.json has %d keys\n", existingKeyCount)

	// ──────────────────────────────────────────────────────────────────
	// Phase 3: Merge — detect new keys and stale keys
	// ──────────────────────────────────────────────────────────────────
	var violations []string

	// Detect new keys (in code but not in en.json)
	addedCount := 0
	for key := range codeKeys {
		if _, exists := enMap[key]; !exists {
			if checkMode {
				violations = append(violations, fmt.Sprintf("MISSING from en.json: %q (found in code but not in translations)", key))
			} else {
				enMap[key] = key
				addedCount++
				fmt.Printf("  ADDED: %q\n", key)
			}
		}
	}

	// Detect stale keys (in en.json but not found in code)
	var staleKeys []string
	for key := range enMap {
		if _, inCode := codeKeys[key]; !inCode {
			staleKeys = append(staleKeys, key)
		}
	}
	sort.Strings(staleKeys)

	if len(staleKeys) > 0 {
		for _, key := range staleKeys {
			if checkMode {
				violations = append(violations, fmt.Sprintf("STALE in en.json: %q (not found in any i18n.T/Tf/N call)", key))
			} else {
				fmt.Printf("  STALE (not found in code): %q\n", key)
			}
		}
	}

	if !checkMode {
		if addedCount > 0 {
			fmt.Printf("→ Added %d new keys to en.json\n", addedCount)
		}
		if len(staleKeys) > 0 {
			fmt.Printf("→ WARNING: %d stale keys in en.json (review manually, not auto-deleted)\n", len(staleKeys))
		}
		if addedCount == 0 && len(staleKeys) == 0 {
			fmt.Println("→ en.json is in sync with source code")
		}
	}

	// ──────────────────────────────────────────────────────────────────
	// Phase 4: Sort and rewrite en.json (skip in check mode)
	// ──────────────────────────────────────────────────────────────────
	if !checkMode {
		if metaName != nil {
			enMap["_meta_name"] = metaName
		}
		if enOut, err := json.MarshalIndent(enMap, "", "  "); err == nil {
			if writeErr := os.WriteFile(enPath, enOut, 0600); writeErr != nil {
				log.Printf("warning: failed to write %s: %v", enPath, writeErr)
			}
		}
		delete(enMap, "_meta_name")
		fmt.Printf("Wrote en.json with %d keys\n", len(enMap))
	}

	// ──────────────────────────────────────────────────────────────────
	// Phase 5: Generate pseudo.json for layout testing (skip in check mode)
	// ──────────────────────────────────────────────────────────────────
	if !checkMode {
		pseudoMap := make(map[string]string)
		pseudoMap["_meta_name"] = "[!! Pseudo-Loc !!]"
		for k, v := range enMap {
			if str, ok := v.(string); ok {
				pseudoMap[k] = pseudolocalize(str)
			}
		}

		pseudoPath := filepath.Join(translationsDir, "pseudo.json")
		pseudoData, err := json.MarshalIndent(pseudoMap, "", "  ")
		if err != nil {
			log.Fatalf("failed to marshal pseudo.json: %v", err)
		}
		if err := os.WriteFile(pseudoPath, pseudoData, 0600); err != nil {
			log.Fatalf("failed to write pseudo.json: %v", err)
		}
		fmt.Println("Generated pseudo.json for layout testing")
	}

	// ──────────────────────────────────────────────────────────────────
	// Phase 6: Check all language files for issues
	// ──────────────────────────────────────────────────────────────────
	var languages []Language
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}

		code := strings.TrimSuffix(f.Name(), ".json")
		if code == "pseudo" {
			continue
		}
		path := filepath.Join(translationsDir, f.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("warning: failed to read %s: %v", path, err)
			continue
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			log.Printf("warning: failed to unmarshal %s: %v", path, err)
			continue
		}

		name, ok := m["_meta_name"].(string)
		if !ok {
			log.Printf("warning: %s missing _meta_name", path)
			continue
		}

		if code != "en" {
			// Check for stale keys in this language file (keys not in en.json)
			for k := range m {
				if k == "_meta_name" {
					continue
				}
				if _, inEn := enMap[k]; !inEn {
					if checkMode {
						violations = append(violations, fmt.Sprintf("STALE in %s: %q (not in en.json)", f.Name(), k))
					}
				}
			}

			// Check for untranslated keys (value == English value and not allowlisted)
			for k, v := range enMap {
				if langVal, exists := m[k]; exists {
					enStr, enOk := v.(string)
					langStr, langOk := langVal.(string)
					if enOk && langOk && enStr == langStr && !allowIdenticalToEnglish[k] {
						if checkMode {
							violations = append(violations, fmt.Sprintf("UNTRANSLATED in %s: %q (still English)", f.Name(), k))
						}
					}
				}
			}
		}

		if !checkMode {
			// Auto-fill any missing keys from en.json
			changed := false
			for k, v := range enMap {
				if _, ok := m[k]; !ok {
					m[k] = v
					changed = true
				}
			}

			if changed {
				log.Printf("info: auto-filled missing keys in %s.json", code)
			}

			// Always rewrite the file to guarantee alphabetical sorting
			if outData, err := json.MarshalIndent(m, "", "  "); err == nil {
				if writeErr := os.WriteFile(path, outData, 0600); writeErr != nil {
					log.Printf("warning: failed to write %s: %v", path, writeErr)
				}
			} else {
				log.Printf("warning: failed to rewrite %s: %v", path, err)
			}
		}

		languages = append(languages, Language{
			Code: code,
			Name: name,
		})
	}

	// ──────────────────────────────────────────────────────────────────
	// Phase 7: Generate Go language registry (skip in check mode)
	// ──────────────────────────────────────────────────────────────────
	if !checkMode {
		// Sort languages by code for deterministic output, but keep English first if it exists
		sort.Slice(languages, func(i, j int) bool {
			if languages[i].Code == "en" {
				return true
			}
			if languages[j].Code == "en" {
				return false
			}
			return languages[i].Code < languages[j].Code
		})

		goFile, err := os.Create(outputFile)
		if err != nil {
			log.Fatalf("failed to create output file: %v", err)
		}
		defer goFile.Close()

		fmt.Fprintln(goFile, "// Code generated by gen_i18n. DO NOT EDIT.")
		fmt.Fprintln(goFile, "package i18n")
		fmt.Fprintln(goFile)
		fmt.Fprintln(goFile, "type Language struct {")
		fmt.Fprintln(goFile, "\tCode string")
		fmt.Fprintln(goFile, "\tName string")
		fmt.Fprintln(goFile, "}")
		fmt.Fprintln(goFile)
		fmt.Fprintln(goFile, "var SupportedLanguages = []Language{")
		for _, lang := range languages {
			fmt.Fprintf(goFile, "\t{Code: %q, Name: %q},\n", lang.Code, lang.Name)
		}
		fmt.Fprintln(goFile, "}")
		fmt.Fprintln(goFile)
		fmt.Fprintln(goFile, "func GetLanguageNames() []string {")
		fmt.Fprintln(goFile, "\tnames := []string{\"System Default\"}")
		fmt.Fprintln(goFile, "\tfor _, lang := range SupportedLanguages {")
		fmt.Fprintln(goFile, "\t\tnames = append(names, lang.Name)")
		fmt.Fprintln(goFile, "\t}")
		fmt.Fprintln(goFile, "\treturn names")
		fmt.Fprintln(goFile, "}")

		// Generate zz_generated_pseudoloc.go
		pseudoFile := "pkg/i18n/zz_generated_pseudoloc.go"
		if _, err := os.Stat("translations"); err == nil {
			pseudoFile = "zz_generated_pseudoloc.go"
		}

		pf, err := os.Create(pseudoFile)
		if err != nil {
			log.Fatalf("failed to create %s: %v", pseudoFile, err)
		}
		defer pf.Close()

		fmt.Fprintln(pf, "// Code generated by gen_i18n. DO NOT EDIT.")
		fmt.Fprintln(pf, "//go:build !release")
		fmt.Fprintln(pf)
		fmt.Fprintln(pf, "package i18n")
		fmt.Fprintln(pf)
		fmt.Fprintln(pf, "func init() {")
		fmt.Fprintln(pf, "\tSupportedLanguages = append(SupportedLanguages, Language{Code: \"pseudo\", Name: \"[!! Pseudo-Loc !!]\"})")
		fmt.Fprintln(pf, "}")
	}

	// ──────────────────────────────────────────────────────────────────
	// Final: Report results
	// ──────────────────────────────────────────────────────────────────
	if checkMode {
		if len(violations) > 0 {
			sort.Strings(violations)
			fmt.Printf("\n✗ i18n check FAILED with %d violation(s):\n\n", len(violations))
			for _, v := range violations {
				fmt.Printf("  • %s\n", v)
			}
			fmt.Println()
			fmt.Println("To fix:")
			fmt.Println("  1. Run 'make gen-i18n' to sync en.json with code")
			fmt.Println("  2. Remove stale keys from all language files")
			fmt.Println("  3. Translate new strings in all language files")
			fmt.Println("  4. Add legitimately-identical strings to the allowlist in gen_i18n/main.go")
			os.Exit(1)
		}
		fmt.Println("\n✓ i18n check passed — all translations in sync")
		return
	}

	fmt.Println("Done.")
}

// extractKeysFromCode walks the Go source tree and extracts all string
// arguments from i18n.T(), i18n.Tf(), and i18n.N() calls.
func extractKeysFromCode(root string) map[string]bool {
	// Matches: i18n.T("..."), i18n.Tf("...", ...), i18n.N("...", ...)
	// Captures the double-quoted string argument including escape sequences.
	i18nRegex := regexp.MustCompile(`i18n\.(?:T|Tf|N)\(\s*"((?:[^"\\]|\\.)*)"`)

	keys := make(map[string]bool)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			name := d.Name()
			// Skip directories that shouldn't be scanned
			if name == "vendor" || name == ".git" || name == "bin" || name == "node_modules" {
				return filepath.SkipDir
			}
			// Skip cmd/util (tool noise — these files reference i18n in regex patterns, not real calls)
			if name == "util" && strings.Contains(filepath.ToSlash(path), "cmd/util") {
				return filepath.SkipDir
			}
			return nil
		}

		// Only scan Go source files (skip tests, generated files)
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.Contains(d.Name(), "zz_generated") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			log.Printf("warning: failed to read %s: %v", path, err)
			return nil
		}

		matches := i18nRegex.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			rawKey := match[1]
			// Interpret escape sequences (e.g., \n, \t, \")
			key, err := strconv.Unquote("\"" + rawKey + "\"")
			if err != nil {
				key = rawKey
			}
			keys[key] = true
		}

		return nil
	})

	if err != nil {
		log.Printf("warning: error walking source tree: %v", err)
	}

	return keys
}

func pseudolocalize(s string) string {
	var sb strings.Builder
	sb.WriteString("[!! ")

	inPlaceholder := false
	for i := 0; i < len(s); i++ {
		char := s[i]
		if char == '{' && i+1 < len(s) && s[i+1] == '{' {
			inPlaceholder = true
		}
		if char == '}' && i-1 >= 0 && s[i-1] == '}' {
			inPlaceholder = false
		}

		sb.WriteByte(char)

		if !inPlaceholder {
			// Double vowels to expand string
			if strings.ContainsRune("aeiouAEIOU", rune(char)) {
				sb.WriteByte(char)
			}
		}
	}

	sb.WriteString(" !!]")
	return sb.String()
}
