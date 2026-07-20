package curation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/docs/collections"
	"github.com/dixieflatline76/Spice/v2/util/log"
	"golang.org/x/mod/semver"
)

// CuratedCollection represents the unified JSON structure for all curated museum collections.
type CuratedCollection struct {
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Entries     []CollectionEntry `json:"collections"`
}

// PreResolvedItem defines an artwork with a pre-fetched image URL.
// This allows providers to bypass complex fetching APIs entirely.
type PreResolvedItem struct {
	ID          string `json:"id"`                     // Unique identifier (e.g., "200107928")
	AccessionID string `json:"accession_id,omitempty"` // Museum specific object number (e.g., "SK-C-5")
	Title       string `json:"title"`
	Artist      string `json:"artist,omitempty"`
	Year        string `json:"year,omitempty"`
	ImageURL    string `json:"image_url"`
	ViewURL     string `json:"view_url,omitempty"`
}

// CollectionEntry defines a single browsable collection.
type CollectionEntry struct {
	Name             string            `json:"name"`
	NameTranslations map[string]string `json:"name_translations,omitempty"`
	Key              string            `json:"key"`
	Type             string            `json:"type"`          // "curated", "query", "search", "department"
	IDs              []string          `json:"ids,omitempty"` // Standard IDs

	// Extension fields for advanced providers
	Query        string            `json:"query,omitempty"`         // Metmuseum
	DeptID       int               `json:"deptId,omitempty"`        // Metmuseum
	SearchParams string            `json:"search_params,omitempty"` // Rijksmuseum
	Items        []PreResolvedItem `json:"items,omitempty"`         // Rijksmuseum pre-resolved metadata
}

// Manager orchestrates the loading and parsing for curated collections.
type Manager struct {
	mu            sync.RWMutex
	collections   map[string]*CuratedCollection
	embeddedData  map[string][]byte
	RemoteBaseURL string
	CacheDir      string
	httpClient    *http.Client
}

var (
	GlobalManager *Manager
	once          sync.Once
)

func GetManager() *Manager {
	once.Do(func() {
		GlobalManager = NewManager()
	})
	return GlobalManager
}

func NewManager() *Manager {
	return &Manager{
		collections:   make(map[string]*CuratedCollection),
		embeddedData:  make(map[string][]byte),
		RemoteBaseURL: "https://raw.githubusercontent.com/dixieflatline76/Spice/main/docs/collections/",
		CacheDir:      filepath.Join(config.GetWorkingDir(), "cache", "curation"),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

// GetCollection ensures the collection is parsed and cached in memory.
func (m *Manager) GetCollection(providerID string) *CuratedCollection {
	m.mu.RLock()
	col, exists := m.collections[providerID]
	m.mu.RUnlock()

	if exists {
		return col
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if col, exists := m.collections[providerID]; exists {
		return col
	}

	data, ok := m.embeddedData[providerID]
	if !ok {
		filename := fmt.Sprintf("%s.json", strings.ToLower(providerID))
		if mapped, ok := ProviderIDToFilename[providerID]; ok {
			filename = mapped
		}

		// 1. Try Cache First
		cachePath := filepath.Join(m.CacheDir, filename)
		cacheData, err := os.ReadFile(cachePath)
		var cacheCol CuratedCollection
		cacheValid := false
		if err == nil {
			if json.Unmarshal(cacheData, &cacheCol) == nil {
				cacheValid = true
			}
		}

		// 2. Load Embedded
		embeddedData, err := collections.FS.ReadFile(filename)
		var embedCol CuratedCollection
		embedValid := false
		if err == nil {
			if json.Unmarshal(embeddedData, &embedCol) == nil {
				embedValid = true
			}
		}

		// 3. Compare Versions
		if cacheValid && embedValid {
			if semver.Compare(cacheCol.Version, embedCol.Version) >= 0 {
				data = cacheData // Cache is equal or newer
			} else {
				data = embeddedData // Embedded is newer (e.g. app just updated)
			}
		} else if cacheValid {
			data = cacheData
		} else if embedValid {
			data = embeddedData
		}
	}

	if data != nil {
		var c CuratedCollection
		if err := json.Unmarshal(data, &c); err == nil {
			m.collections[providerID] = &c
			return &c
		}
	}

	return nil
}

// GetEntry returns the full collection entry data for a given provider and key.
func (m *Manager) GetEntry(providerID, key string) *CollectionEntry {
	col := m.GetCollection(providerID)
	if col == nil {
		return nil
	}
	for _, e := range col.Entries {
		if e.Key == key {
			// Return a copy so the caller can't mutate the cached state
			c := e
			return &c
		}
	}
	return nil
}

// ClearCache removes a collection from the in-memory cache, forcing a reload on next access.
func (m *Manager) ClearCache(providerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.collections, providerID)
}

// SyncAll iterates through all known providers, downloads their remote JSON, compares versions,
// and saves updates to the local cache. Returns a list of providerIDs that were updated.
func (m *Manager) SyncAll(ctx context.Context) ([]string, error) {
	if err := os.MkdirAll(m.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create curation cache dir: %w", err)
	}

	var updatedProviders []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	for id, filename := range ProviderIDToFilename {
		wg.Add(1)
		go func(providerID, fname string) {
			defer wg.Done()
			updated, err := m.syncSingle(ctx, providerID, fname)
			if err != nil {
				log.Printf("Curation OTA: failed to sync %s: %v", providerID, err)
				return
			}
			if updated {
				mu.Lock()
				updatedProviders = append(updatedProviders, providerID)
				mu.Unlock()
			}
		}(id, filename)
	}

	wg.Wait()
	return updatedProviders, nil
}

func (m *Manager) syncSingle(ctx context.Context, providerID, filename string) (bool, error) {
	url := m.RemoteBaseURL + filename
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	remoteData, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var remoteCol CuratedCollection
	if err := json.Unmarshal(remoteData, &remoteCol); err != nil {
		return false, fmt.Errorf("invalid json payload: %w", err)
	}
	if !semver.IsValid(remoteCol.Version) {
		return false, fmt.Errorf("invalid semantic version: %s", remoteCol.Version)
	}

	localCol := m.GetCollection(providerID)
	if localCol != nil && semver.Compare(remoteCol.Version, localCol.Version) <= 0 {
		return false, nil // Local is equal or newer, no update needed
	}

	// Remote is newer, write to cache
	cachePath := filepath.Join(m.CacheDir, filename)
	if err := os.WriteFile(cachePath, remoteData, 0600); err != nil {
		return false, fmt.Errorf("failed to write cache: %w", err)
	}

	// Update in-memory map
	m.mu.Lock()
	m.collections[providerID] = &remoteCol
	m.mu.Unlock()

	log.Printf("Curation OTA: Updated %s to version %s", providerID, remoteCol.Version)
	return true, nil
}
