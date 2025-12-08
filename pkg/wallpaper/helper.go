package wallpaper

import (
	"encoding/hex"
	"hash/fnv"
	"net/url"
	"path/filepath"
	"strings"
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

// isImageFile checks if a file has a common image extension.
func isImageFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif"
}

// GenerateQueryID creates a stable hash ID from a URL string.
func GenerateQueryID(url string) string {
	h := fnv.New64a()
	h.Write([]byte(url))
	return hex.EncodeToString(h.Sum(nil))
}
