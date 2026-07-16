package curation

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/dixieflatline76/Spice/v2/docs/collections"
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
	mu           sync.RWMutex
	collections  map[string]*CuratedCollection
	embeddedData map[string][]byte
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
		collections:  make(map[string]*CuratedCollection),
		embeddedData: make(map[string][]byte),
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
		d, err := collections.FS.ReadFile(filename)
		if err == nil {
			data = d
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
