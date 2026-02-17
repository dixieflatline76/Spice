package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// resolveCollectionPath resolves a collection path relative to a namespace root and
// enforces that the resulting absolute path is contained within the root.
func (s *Server) resolveCollectionPath(rootPath, collectionID string) (string, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return "", fmt.Errorf("invalid namespace root: %w", err)
	}
	absRoot = filepath.Clean(absRoot)

	absCollection, err := filepath.Abs(filepath.Join(absRoot, collectionID))
	if err != nil {
		return "", fmt.Errorf("invalid collection path: %w", err)
	}
	absCollection = filepath.Clean(absCollection)

	// Ensure the collection path is strictly within the namespace root.
	if absCollection != absRoot && !strings.HasPrefix(absCollection, absRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("path traversal detected")
	}

	return absCollection, nil
}

// handleLocal routes requests to local namespace handlers
// Path format: /local/{namespace}/{action}/...
// Supported patterns:
// 1. list: /local/{namespace}/{collectionID}/images?page=1&per_page=20
// 2. asset: /local/{namespace}/{collectionID}/assets/{filename}
func (s *Server) handleLocal(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/local/")
	parts := strings.Split(path, "/")

	if len(parts) < 3 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	namespace := parts[0]
	collectionID := parts[1]
	actionOrType := parts[2] // "images" or "assets"

	// Security: Prevent path traversal in collectionID
	if collectionID == "." || collectionID == ".." || strings.ContainsAny(collectionID, `/\`) {
		http.Error(w, "Invalid collection ID", http.StatusBadRequest)
		return
	}

	rootPath, ok := s.namespaces[namespace]
	if !ok {
		http.Error(w, "Namespace not found", http.StatusNotFound)
		return
	}

	// Construct the collection path
	// Use absolute, cleaned paths and enforce strict containment within rootPath.
	collectionPath, err := s.resolveCollectionPath(rootPath, collectionID)
	if err != nil {
		http.Error(w, "Invalid collection path or traversal detected", http.StatusBadRequest)
		return
	}

	switch actionOrType {
	case "images":
		s.handleLocalListing(w, r, rootPath, namespace, collectionID)
	case "assets":
		if len(parts) < 4 {
			http.Error(w, "Missing filename", http.StatusBadRequest)
			return
		}
		filename := parts[3]
		s.handleLocalAsset(w, r, collectionPath, filename)
	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

type LocalImage struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Attribution string `json:"attribution,omitempty"`
	ProductURL  string `json:"product_url,omitempty"`
}

func (s *Server) handleLocalListing(w http.ResponseWriter, r *http.Request, rootPath, namespace, collectionID string) {
	handler := NewLocalListingHandler(s, w, r, rootPath, namespace, collectionID)
	handler.Handle()
}

func (s *Server) handleLocalAsset(w http.ResponseWriter, r *http.Request, collectionPath, filename string) {
	// Security: validate filename - must be a single path component with no traversal
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}
	cleanParams := filepath.Base(filename)
	if cleanParams != filename {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	// Double-check containment using absolute, cleaned paths to satisfy CodeQL and prevent traversal
	absCollectionPath, err := filepath.Abs(collectionPath)
	if err != nil {
		http.Error(w, "Invalid asset path", http.StatusBadRequest)
		return
	}
	absCollectionPath = filepath.Clean(absCollectionPath)

	// Build the asset path relative to the normalized collection root
	fullPath := filepath.Join(absCollectionPath, filename)
	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		http.Error(w, "Invalid asset path", http.StatusBadRequest)
		return
	}
	absFullPath = filepath.Clean(absFullPath)

	// Ensure prefix check includes separator to prevent partial name matching
	prefix := absCollectionPath
	if !strings.HasSuffix(prefix, string(os.PathSeparator)) {
		prefix += string(os.PathSeparator)
	}

	if !strings.HasPrefix(absFullPath, prefix) {
		http.Error(w, "Invalid asset path", http.StatusBadRequest)
		return
	}

	http.ServeFile(w, r, absFullPath)
}
