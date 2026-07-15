package rijksmuseum

import (
	_ "embed" // For go:embed
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/mod/semver"

	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// Collection represents the full set of curated collections for the Rijksmuseum.
// Follows the same polymorphic structure as The Met:
//   - "curated":  Pre-resolved image URLs for guaranteed masterpieces
//   - "search":   Live queries via the Linked Art search API
type Collection struct {
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Entries     []CollectionEntry `json:"collections"`
}

// CollectionEntry defines a single browsable collection.
// The Type field determines how resolveQueryToIDs operates:
//   - "curated": Uses the embedded Items list directly (pre-resolved URLs)
//   - "search":  Calls the Rijksmuseum Linked Art search API with SearchParams
type CollectionEntry struct {
	Name             string            `json:"name"`
	NameTranslations map[string]string `json:"name_translations,omitempty"`
	Key              string            `json:"key"`
	Type             string            `json:"type"`                    // "curated" or "search"
	Items            []CuratedItem     `json:"items,omitempty"`         // For "curated" type
	SearchParams     string            `json:"search_params,omitempty"` // For "search" type (URL query params)
}

// CuratedItem is a pre-resolved artwork with its IIIF image URL.
// Used in curated collections to avoid the 3-step Linked Art resolution chain.
type CuratedItem struct {
	ObjectID     string `json:"object_id"`     // Rijksmuseum numeric ID (e.g., "200109287")
	ObjectNumber string `json:"object_number"` // Accession number (e.g., "SK-C-5")
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	ImageURL     string `json:"image_url"` // Pre-resolved IIIF URL
}

const (
	remoteURL     = "https://raw.githubusercontent.com/dixieflatline76/Spice/main/docs/collections/rijksmuseum.json"
	cacheFileName = "rijksmuseum_cache.json"
)

//go:embed rijksmuseum.json
var embeddedJSON []byte

// Global cache fallback in case everything fails
var embeddedCollection Collection

// InitRemoteCollection initializes the collection from assets
// and attempts to refresh the cache from the remote URL.
// Priority: Remote > Cache > Embedded
func InitRemoteCollection(cfg *wallpaper.Config) (*Collection, error) {
	// 1. Load from embedded JSON (compiled into the binary — always available)
	if err := json.Unmarshal(embeddedJSON, &embeddedCollection); err != nil {
		return nil, fmt.Errorf("failed to parse embedded rijksmuseum.json: %w", err)
	}

	// 2. Determine Cache Path
	cacheDir := filepath.Join(config.GetWorkingDir(), "cache", "rijksmuseum")
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

		if semver.Compare(col.Version, embeddedCollection.Version) >= 0 {
			fetchedCollection = col
			if err := saveCache(cachePath, col); err != nil {
				log.Printf("Rijksmuseum: Failed to save cache: %v", err)
			}
		} else {
			log.Printf("RIJKS: Remote collection (%s) is older than embedded (%s), ignoring remote", col.Version, embeddedCollection.Version)
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
			log.Printf("RIJKS: Successfully loaded remote collection (%s, %d entries)", fetchedCollection.Version, len(fetchedCollection.Entries))
			return fetchedCollection, nil
		}
		log.Printf("Rijksmuseum: Remote fetch failed: %v", <-fetchErr)
	case <-time.After(3 * time.Second):
		log.Printf("Rijksmuseum: Remote fetch timed out, falling back to cache")
	}

	// 4. Fallback to Cache
	if cacheCol, err := loadCache(cachePath); err == nil {
		if semver.Compare(cacheCol.Version, embeddedCollection.Version) >= 0 {
			log.Printf("RIJKS: Loaded cached collection (%s)", cacheCol.Version)
			return cacheCol, nil
		}
		log.Printf("RIJKS: Cached collection (%s) is older than embedded (%s), ignoring cache", cacheCol.Version, embeddedCollection.Version)
	} else {
		log.Printf("Rijksmuseum: Failed to load cache: %v", err)
	}

	// 5. Fallback to Embedded
	log.Printf("RIJKS: Using embedded collection (%s)", embeddedCollection.Version)
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

// RefreshRemoteCollection forces a fetch from GitHub, and updates the local cache if successful.
func RefreshRemoteCollection() (*Collection, error) {
	col, err := fetchRemote()
	if err != nil {
		return nil, err
	}

	if semver.Compare(col.Version, embeddedCollection.Version) >= 0 {
		cacheDir := filepath.Join(config.GetWorkingDir(), "cache", "rijksmuseum")
		cachePath := filepath.Join(cacheDir, cacheFileName)
		_ = os.MkdirAll(cacheDir, 0755)
		if err := saveCache(cachePath, col); err != nil {
			log.Printf("Rijksmuseum: Failed to save cache during refresh: %v", err)
		}
		return col, nil
	}

	log.Printf("RIJKS: Remote collection (%s) is not newer than embedded (%s), no update needed", col.Version, embeddedCollection.Version)
	return nil, nil
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
	if len(col.Entries) == 0 {
		return nil, fmt.Errorf("remote collection is empty or malformed (schema mismatch)")
	}
	return &col, nil
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
	if len(col.Entries) == 0 {
		return nil, fmt.Errorf("remote collection is empty or malformed (schema mismatch)")
	}
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
