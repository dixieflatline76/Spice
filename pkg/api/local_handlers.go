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

	rootPath, ok := s.namespaces[namespace]
	if !ok {
		http.Error(w, "Namespace not found", http.StatusNotFound)
		return
	}

	// Construct the collection path
	// e.g. C:\...Temp\spice\google_photos\GUID
	collectionPath := filepath.Join(rootPath, collectionID)

	switch actionOrType {
	case "images":
		s.handleLocalListing(w, r, collectionPath, namespace, collectionID)
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

func (s *Server) handleLocalListing(w http.ResponseWriter, r *http.Request, collectionPath, namespace, collectionID string) {
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
		attribution = "Local: " + collectionID // Fallback
	}

	// Read dir
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

		var pUrl string
		if filesMeta != nil {
			if v, ok := filesMeta[name]; ok {
				pUrl, _ = v.(string)
			}
		}

		result = append(result, LocalImage{
			ID:          name,
			URL:         url,
			Attribution: attribution,
			ProductURL:  pUrl,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) handleLocalAsset(w http.ResponseWriter, r *http.Request, collectionPath, filename string) {
	// Security: validate filename (no ..)
	cleanParams := filepath.Base(filename)
	if cleanParams != filename {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(collectionPath, filename)
	http.ServeFile(w, r, fullPath)
}
