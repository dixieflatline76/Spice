package getty

import (
	_ "embed"
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

// Collection represents the curated collections for the Getty Museum.
type Collection struct {
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Entries     []CollectionEntry `json:"collections"`
}

// CollectionEntry defines a single browsable collection.
type CollectionEntry struct {
	Name             string            `json:"name"`
	NameTranslations map[string]string `json:"name_translations,omitempty"`
	Key              string            `json:"key"`
	Type             string            `json:"type"`          // "curated"
	IDs              []string          `json:"ids,omitempty"` // Getty UUIDs
}

const (
	cacheFileName = "getty_cache.json"
)

//go:embed getty.json
var embeddedJSON []byte

var embeddedCollection Collection

// InitRemoteCollection initializes the collection from assets
// and attempts to refresh from the remote URL.
// Priority: Remote > Cache > Embedded
func InitRemoteCollection(cfg *wallpaper.Config) (*Collection, error) {
	if err := json.Unmarshal(embeddedJSON, &embeddedCollection); err != nil {
		return nil, fmt.Errorf("failed to parse embedded getty.json: %w", err)
	}

	cacheDir := filepath.Join(config.GetWorkingDir(), "cache", "getty")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return &embeddedCollection, fmt.Errorf("failed to create cache dir: %w", err)
	}

	cachePath := filepath.Join(cacheDir, cacheFileName)

	var wg sync.WaitGroup
	var fetchedCollection *Collection
	fetchErr := make(chan error, 1)

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
				log.Printf("Getty: Failed to save cache: %v", err)
			}
		} else {
			log.Printf("Getty: Remote collection (%s) is older than embedded (%s), ignoring remote", col.Version, embeddedCollection.Version)
			fetchErr <- fmt.Errorf("remote version older than embedded")
		}
	}()

	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()

	select {
	case <-c:
		if fetchedCollection != nil {
			log.Printf("Getty: Successfully loaded remote collection (%s, %d entries)", fetchedCollection.Version, len(fetchedCollection.Entries))
			return fetchedCollection, nil
		}
		log.Printf("Getty: Remote fetch failed: %v", <-fetchErr)
	case <-time.After(3 * time.Second):
		log.Printf("Getty: Remote fetch timed out, falling back to cache")
	}

	if cacheCol, err := loadCache(cachePath); err == nil {
		if semver.Compare(cacheCol.Version, embeddedCollection.Version) >= 0 {
			log.Printf("Getty: Loaded cached collection (%s)", cacheCol.Version)
			return cacheCol, nil
		}
	} else {
		log.Printf("Getty: Failed to load cache: %v", err)
	}

	log.Printf("Getty: Using embedded collection (%s)", embeddedCollection.Version)
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

// RefreshRemoteCollection forces a fetch from GitHub.
func RefreshRemoteCollection() (*Collection, error) {
	col, err := fetchRemote()
	if err != nil {
		return nil, err
	}
	if semver.Compare(col.Version, embeddedCollection.Version) >= 0 {
		cacheDir := filepath.Join(config.GetWorkingDir(), "cache", "getty")
		cachePath := filepath.Join(cacheDir, cacheFileName)
		_ = os.MkdirAll(cacheDir, 0755)
		if err := saveCache(cachePath, col); err != nil {
			log.Printf("Getty: Failed to save cache during refresh: %v", err)
		}
		return col, nil
	}
	log.Printf("Getty: Remote collection (%s) is not newer than embedded (%s), no update needed", col.Version, embeddedCollection.Version)
	return nil, nil
}

func fetchRemote() (*Collection, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(cdnUrl)
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
