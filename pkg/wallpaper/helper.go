package wallpaper

import (
	"encoding/hex"
	"hash/fnv"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/dixieflatline76/Spice/v2/util/log"
)

// extractFilenameFromURL extracts the filename from a URL, ignoring query parameters.
func extractFilenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		// Fallback to simple string manipulation if parsing fails
		return extractFilenameSimple(rawURL)
	}
	filename := filepath.Base(u.Path)
	if filename == "." || filename == "/" || filename == "\\" {
		return ""
	}
	return filename
}

func extractFilenameSimple(urlStr string) string {
	// Strip query params if present
	if idx := strings.Index(urlStr, "?"); idx != -1 {
		urlStr = urlStr[:idx]
	}
	lastSlashIndex := strings.LastIndex(urlStr, "/")
	if lastSlashIndex == -1 {
		return urlStr
	}
	if lastSlashIndex == len(urlStr)-1 {
		return ""
	}
	return urlStr[lastSlashIndex+1:]
}

// GenerateQueryID creates a stable hash ID from a URL string.
func GenerateQueryID(url string) string {
	h := fnv.New64a()
	h.Write([]byte(url))
	id := hex.EncodeToString(h.Sum(nil))
	log.Debugf("[Helper] GenerateQueryID: url=%s -> id=%s", url, id)
	return id
}

// SanitizeMenuString collapses all whitespace (newlines, tabs, multiple spaces)
// into a single space and trims the result. This prevents tray menu layout issues.
func SanitizeMenuString(s string) string {
	// Simple whitespace collapsing logic
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
