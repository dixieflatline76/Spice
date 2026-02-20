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

// LocalListingHandler encapsulates the logic for listing local images.
// It follows a Template Method style where the ServeHTTP method orchestrates the steps:
// 1. Parse Request
// 2. Resolve Path
// 3. Read Metadata
// 4. Scan Directory
// 5. Filter & Sort
// 6. Paginate
// 7. Format Response
type LocalListingHandler struct {
	server       *Server
	w            http.ResponseWriter
	r            *http.Request
	rootPath     string
	namespace    string
	collectionID string

	// State populated during processing
	collectionPath string
	page           int
	perPage        int
	attribution    string
	filesMeta      map[string]interface{}
	allImages      []string
	pagedImages    []string
}

func NewLocalListingHandler(s *Server, w http.ResponseWriter, r *http.Request, rootPath, namespace, collectionID string) *LocalListingHandler {
	return &LocalListingHandler{
		server:       s,
		w:            w,
		r:            r,
		rootPath:     rootPath,
		namespace:    namespace,
		collectionID: collectionID,
		page:         1,
		perPage:      24,
	}
}

func (h *LocalListingHandler) Handle() {
	if !h.resolvePath() {
		return
	}
	h.parsePagination()
	h.readMetadata()
	if !h.scanDirectory() {
		return // scanDirectory handles error response
	}
	h.filterAndSort()
	h.paginate()
	h.sendResponse()
}

func (h *LocalListingHandler) resolvePath() bool {
	var err error
	h.collectionPath, err = h.server.resolveCollectionPath(h.rootPath, h.collectionID)
	if err != nil {
		http.Error(h.w, "Invalid collection path", http.StatusBadRequest)
		return false
	}
	return true
}

func (h *LocalListingHandler) parsePagination() {
	pageStr := h.r.URL.Query().Get("page")
	perPageStr := h.r.URL.Query().Get("per_page")

	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		h.page = p
	}
	if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 {
		h.perPage = pp
	}
}

func (h *LocalListingHandler) readMetadata() {
	metaFile := filepath.Join(h.collectionPath, "metadata.json")
	if f, err := os.Open(metaFile); err == nil {
		defer f.Close()
		var meta map[string]interface{}
		if err := json.NewDecoder(f).Decode(&meta); err == nil {
			desc, _ := meta["description"].(string)
			author, _ := meta["author"].(string)

			if desc != "" && author != "" {
				if h.namespace == "google_photos" {
					h.attribution = desc
				} else {
					h.attribution = fmt.Sprintf("%s (by %s)", desc, author)
				}
			} else if desc != "" {
				h.attribution = desc
			} else if author != "" {
				if h.namespace == "google_photos" {
					h.attribution = ""
				} else {
					h.attribution = "by " + author
				}
			}

			h.filesMeta, _ = meta["files"].(map[string]interface{})
		}
	}
	if h.attribution == "" && h.namespace != "favorites" {
		h.attribution = h.collectionID
	}
}

func (h *LocalListingHandler) scanDirectory() bool {
	entries, err := os.ReadDir(h.collectionPath)
	if err != nil {
		if os.IsNotExist(err) {
			h.w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(h.w).Encode([]LocalImage{})
			return false
		}
		http.Error(h.w, "Failed to read directory", http.StatusInternalServerError)
		return false
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		h.allImages = append(h.allImages, e.Name())
	}
	return true
}

func (h *LocalListingHandler) filterAndSort() {
	var filtered []string
	for _, name := range h.allImages {
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
			filtered = append(filtered, name)
		}
	}
	h.allImages = filtered
	sort.Strings(h.allImages)
}

func (h *LocalListingHandler) paginate() {
	start := (h.page - 1) * h.perPage
	end := start + h.perPage

	if start >= len(h.allImages) {
		start = len(h.allImages)
	}
	if end > len(h.allImages) {
		end = len(h.allImages)
	}
	if start > end {
		start = end
	}

	h.pagedImages = h.allImages[start:end]
}

func (h *LocalListingHandler) sendResponse() {
	var result []LocalImage
	host := h.r.Host
	scheme := "http"
	if h.r.TLS != nil {
		scheme = "https"
	}

	for _, name := range h.pagedImages {
		url := fmt.Sprintf("%s://%s/local/%s/%s/assets/%s", scheme, host, h.namespace, h.collectionID, name)

		imgAttribution := h.attribution
		var pUrl string
		if h.filesMeta != nil {
			if v, ok := h.filesMeta[name]; ok {
				if m, ok := v.(map[string]interface{}); ok {
					if attr, ok := m["attribution"].(string); ok && attr != "" {
						imgAttribution = attr
					}
					if purl, ok := m["product_url"].(string); ok {
						pUrl = purl
					}
				} else {
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

	h.w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(h.w).Encode(result)
}
