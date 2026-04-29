package metmuseum

import (
	_ "embed" // For go:embed
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// Collection represents the full set of curated collections for The Met.
// All collection definitions (curated ID lists, search queries, department filters)
// are driven from this structure, which is loaded from JSON.
type Collection struct {
	Version     int               `json:"version"`
	Description string            `json:"description"`
	Entries     []CollectionEntry `json:"collections"`

	// LegacyIDs supports the old JSON format (v1–v4) where only curated IDs
	// were stored at the top level. Automatically migrated on load.
	LegacyIDs []int `json:"ids,omitempty"`
}

// migrate converts old-format JSON (top-level "ids" only) into the new
// entries-based structure. This ensures remote/cached v3/v4 JSON files
// continue to work until they're updated to the new format.
func (c *Collection) migrate() {
	if len(c.Entries) > 0 || len(c.LegacyIDs) == 0 {
		return // Already new format, or nothing to migrate
	}

	log.Printf("MET: Migrating legacy collection v%d to entries format (%d curated IDs)", c.Version, len(c.LegacyIDs))
	c.Entries = []CollectionEntry{
		{Name: "Best of The Met", Key: CollectionSpiceMelange, Type: "curated", IDs: c.LegacyIDs},
		{Name: "American Paintings", Key: CollectionAmerican, Type: "search", Query: "American Paintings"},
		{Name: "European Paintings", Key: CollectionEuropean, Type: "department", DeptID: DeptEuropeanPaintings},
		{Name: "Asian Art", Key: CollectionAsian, Type: "department", DeptID: DeptAsianArt},
		{Name: "Egyptian Art", Key: CollectionEgyptian, Type: "department", DeptID: DeptEgyptianArt},
	}
	c.LegacyIDs = nil // Clear after migration
}

// CollectionEntry defines a single browsable collection.
// The Type field determines how resolveQueryToIDs fetches artwork IDs:
//   - "curated":    Uses the embedded IDs list directly
//   - "search":     Calls the Met search API with Query
//   - "department": Calls the Met department API with DeptID
type CollectionEntry struct {
	Name   string `json:"name"`
	Key    string `json:"key"`
	Type   string `json:"type"`              // "curated", "search", "department"
	IDs    []int  `json:"ids,omitempty"`     // For "curated" type
	Query  string `json:"query,omitempty"`   // For "search" type
	DeptID int    `json:"dept_id,omitempty"` // For "department" type
}

const (
	remoteURL     = "https://raw.githubusercontent.com/dixieflatline76/Spice/main/docs/collections/met.json"
	cacheFileName = "met_cache.json"
)

//go:embed met.json
var embeddedJSON []byte

// Global cache fallback in case everything fails
var embeddedCollection Collection

// InitRemoteCollection initializes the collection from assets
// and attempts to refresh the cache from the remote URL.
// It returns the most up-to-date collection available (Remote > Cache > Embedded > Fallback).
func InitRemoteCollection(cfg *wallpaper.Config) (*Collection, error) {
	// 1. Load from embedded JSON (compiled into the binary — always available)
	if err := json.Unmarshal(embeddedJSON, &embeddedCollection); err != nil {
		return nil, fmt.Errorf("failed to parse embedded met.json: %w", err)
	}
	embeddedCollection.migrate()

	// 2. Determine Cache Path
	cacheDir := filepath.Join(config.GetWorkingDir(), "cache", "met")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return &embeddedCollection, fmt.Errorf("failed to create cache dir: %w", err)
	}
	cachePath := filepath.Join(cacheDir, cacheFileName)

	var wg sync.WaitGroup
	var fetchedCollection *Collection
	fetchErr := make(chan error, 1)

	// 3. Async Fetch
	wg.Add(1)
	go func() {
		defer wg.Done()
		col, err := fetchRemote()
		if err != nil {
			fetchErr <- err
			return
		}

		if col.Version >= embeddedCollection.Version {
			fetchedCollection = col
			// Update Cache
			if err := saveCache(cachePath, col); err != nil {
				log.Printf("MET: Failed to save cache: %v", err)
			}
		} else {
			log.Printf("MET: Remote collection (v%d) is older than embedded (v%d), ignoring remote", col.Version, embeddedCollection.Version)
			fetchErr <- fmt.Errorf("remote version older than embedded")
		}
	}()

	// Wait with timeout
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()

	select {
	case <-c:
		if fetchedCollection != nil {
			log.Printf("MET: Successfully loaded remote collection (v%d, %d entries)", fetchedCollection.Version, len(fetchedCollection.Entries))
			return fetchedCollection, nil
		}
		log.Printf("MET: Remote fetch failed: %v", <-fetchErr)
	case <-time.After(3 * time.Second):
		log.Printf("MET: Remote fetch timed out, falling back to cache")
	}

	// 4. Fallback to Cache
	if cacheCol, err := loadCache(cachePath); err == nil {
		if cacheCol.Version >= embeddedCollection.Version {
			log.Printf("MET: Loaded cached collection (v%d)", cacheCol.Version)
			return cacheCol, nil
		}
		log.Printf("MET: Cached collection (v%d) is older than embedded (v%d), ignoring cache", cacheCol.Version, embeddedCollection.Version)
	} else {
		log.Printf("MET: Failed to load cache: %v", err)
	}

	// 5. Fallback to Embedded
	log.Printf("MET: Using embedded collection (v%d)", embeddedCollection.Version)
	return &embeddedCollection, nil
}

// FindEntry looks up a collection entry by its key.
func (c *Collection) FindEntry(key string) *CollectionEntry {
	for i := range c.Entries {
		if c.Entries[i].Key == key {
			return &c.Entries[i]
		}
	}
	return nil
}

func fetchRemote() (*Collection, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(remoteURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	var col Collection
	if err := json.NewDecoder(resp.Body).Decode(&col); err != nil {
		return nil, err
	}
	col.migrate()
	return &col, nil
}

// RefreshRemoteCollection forces a fetch from GitHub, and updates the local cache if successful.
func RefreshRemoteCollection() (*Collection, error) {
	col, err := fetchRemote()
	if err != nil {
		return nil, err
	}

	if col.Version >= embeddedCollection.Version {
		cacheDir := filepath.Join(config.GetWorkingDir(), "cache", "met")
		cachePath := filepath.Join(cacheDir, cacheFileName)
		_ = os.MkdirAll(cacheDir, 0755)
		if err := saveCache(cachePath, col); err != nil {
			log.Printf("MET: Failed to save cache during refresh: %v", err)
		}
		return col, nil
	}

	log.Printf("MET: Remote collection (v%d) is not newer than embedded (v%d), no update needed", col.Version, embeddedCollection.Version)
	return nil, nil
}

func loadCache(path string) (*Collection, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var col Collection
	if err := json.NewDecoder(f).Decode(&col); err != nil {
		return nil, err
	}
	col.migrate()
	return &col, nil
}

func saveCache(path string, col *Collection) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(col)
}
