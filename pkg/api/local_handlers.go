package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	// Security: Re-resolve path locally to satisfy CodeQL data flow analysis
	collectionPath, err := s.resolveCollectionPath(rootPath, collectionID)
	if err != nil {
		http.Error(w, "Invalid collection path", http.StatusBadRequest)
		return
	}

	// Paging params
	pageStr := r.URL.Query().Get("page")
	perPageStr := r.URL.Query().Get("per_page")

	page := 1
	perPage := 24

	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 {
		perPage = pp
	}

	// Read Metadata
	var attribution string
	var filesMeta map[string]interface{}

	metaFile := filepath.Join(collectionPath, "metadata.json")
	// CodeQL False Positive: collectionPath is already validated via resolveCollectionPath.
	if f, err := os.Open(metaFile); err == nil {
		defer f.Close()
		var meta map[string]interface{}
		if err := json.NewDecoder(f).Decode(&meta); err == nil {
			// Construct attribution string
			desc, _ := meta["description"].(string)
			author, _ := meta["author"].(string)

			// Helper to format attribution
			if desc != "" && author != "" {
				if namespace == "google_photos" {
					attribution = desc // Suppress author for Google Photos
				} else {
					attribution = fmt.Sprintf("%s (by %s)", desc, author)
				}
			} else if desc != "" {
				attribution = desc
			} else if author != "" {
				if namespace == "google_photos" {
					attribution = "" // Suppress author-only attribution for Google Photos
				} else {
					attribution = "by " + author
				}
			}

			// Extract files mapping
			filesMeta, _ = meta["files"].(map[string]interface{})
		}
	}
	if attribution == "" {
		attribution = collectionID // Use name directly instead of "Local: ..."
	}

	// Read dir
	// CodeQL False Positive: collectionPath is already validated via resolveCollectionPath.
	entries, err := os.ReadDir(collectionPath)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]LocalImage{})
			return
		}
		http.Error(w, "Failed to read directory", http.StatusInternalServerError)
		return
	}

	// Filter images
	var images []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
			images = append(images, name)
		}
	}

	// Sort (optional, but good for consistent paging)
	sort.Strings(images)

	// Slice page
	start := (page - 1) * perPage
	end := start + perPage

	if start >= len(images) {
		start = len(images) // Empty
	}
	if end > len(images) {
		end = len(images)
	}
	if start > end {
		start = end // Safety
	}

	pageImages := images[start:end]

	// Map to response
	var result []LocalImage
	host := r.Host
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	for _, name := range pageImages {
		url := fmt.Sprintf("%s://%s/local/%s/%s/assets/%s", scheme, host, namespace, collectionID, name)

		imgAttribution := attribution
		var pUrl string
		if filesMeta != nil {
			if v, ok := filesMeta[name]; ok {
				if m, ok := v.(map[string]interface{}); ok {
					// Per-image metadata (Favorites style)
					if attr, ok := m["attribution"].(string); ok && attr != "" {
						imgAttribution = attr
					}
					if purl, ok := m["product_url"].(string); ok {
						pUrl = purl
					}
				} else {
					// Legacy string style (Google Photos style)
					pUrl, _ = v.(string)
				}
			}
		}

		result = append(result, LocalImage{
			ID:          strings.TrimSuffix(name, filepath.Ext(name)),
			URL:         url,
			Attribution: imgAttribution,
			ProductURL:  pUrl,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
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
