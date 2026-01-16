package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	secretFile := ".spice_secrets"

	// Mapping from .spice_secrets keys to Go package paths
	keyMap := map[string]string{
		"GOOGLE_PHOTOS_CLIENT_ID":     "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/googlephotos.GoogleClientID",
		"GOOGLE_PHOTOS_CLIENT_SECRET": "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/googlephotos.GoogleClientSecret",
		"UNSPLASH_CLIENT_ID":          "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/unsplash.UnsplashClientID",
		"UNSPLASH_CLIENT_SECRET":      "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/unsplash.UnsplashClientSecret",
	}

	collected := make(map[string]string)

	// 1. Load from environment first (CI path)
	for key, pkgPath := range keyMap {
		if val := os.Getenv(key); val != "" {
			collected[pkgPath] = trimValue(val)
		}
	}

	// 2. Overlay from .spice_secrets (Local path)
	if _, err := os.Stat(secretFile); err == nil {
		file, err := os.Open(secretFile)
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}

				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					continue
				}

				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				if pkgPath, ok := keyMap[key]; ok {
					collected[pkgPath] = trimValue(value)
				}
			}
		}
	}

	var ldflags []string
	for pkgPath, value := range collected {
		// Output format: -X pkg.path=value
		// We avoid internal quotes because the Makefile joins this into a larger quoted string.
		// Secrets (hex/base64) don't have spaces, so this is safe and shell-portable.
		ldflags = append(ldflags, fmt.Sprintf("-X %s=%s", pkgPath, value))
	}

	if len(ldflags) > 0 {
		fmt.Print(strings.Join(ldflags, " "))
	}
}

func trimValue(s string) string {
	s = strings.TrimSpace(s)
	// Remove surrounding quotes if present
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		s = s[1 : len(s)-1]
	}
	return s
}
