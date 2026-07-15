package artic

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
	"github.com/dixieflatline76/Spice/v2/util/log"
)

const (
	// remoteURL is where the curated list lives on GitHub.
	remoteURL     = "https://raw.githubusercontent.com/dixieflatline76/Spice/main/docs/collections/artic.json"
	cacheFileName = "artic_cache.json"
)

//go:embed artic.json
var embeddedJSON []byte

// Global cache fallback in case everything fails
var embeddedCollection CuratedList

// InitRemoteCollection initializes the curated list from remote, cache, or embedded sources.
func InitRemoteCollection() (*CuratedList, error) {
	// 1. Load from embedded JSON (compiled into the binary — always available)
	if err := json.Unmarshal(embeddedJSON, &embeddedCollection); err != nil {
		return nil, fmt.Errorf("failed to parse embedded artic.json: %w", err)
	}

	// 2. Determine Cache Path
	cacheDir := filepath.Join(config.GetWorkingDir(), "cache", "artic")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Printf("AIC: Failed to create cache dir: %v", err)
		return &embeddedCollection, nil
	}
	cachePath := filepath.Join(cacheDir, cacheFileName)

	var wg sync.WaitGroup
	var fetchedCollection *CuratedList
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
			// Update Cache
			if err := saveCache(cachePath, col); err != nil {
				log.Printf("AIC: Failed to save cache: %v", err)
			}
		} else {
			log.Printf("AIC: Remote collection (%s) is older than embedded (%s), ignoring remote", col.Version, embeddedCollection.Version)
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
			log.Printf("AIC: Successfully loaded remote collection (%s)", fetchedCollection.Version)
			return fetchedCollection, nil
		}
		log.Printf("AIC: Remote fetch failed: %v", <-fetchErr)
	case <-time.After(3 * time.Second):
		log.Printf("AIC: Remote fetch timed out, falling back to cache")
	}

	// 4. Fallback to Cache
	if cacheCol, err := loadCache(cachePath); err == nil {
		if semver.Compare(cacheCol.Version, embeddedCollection.Version) >= 0 {
			log.Printf("AIC: Loaded cached collection (%s)", cacheCol.Version)
			return cacheCol, nil
		}
		log.Printf("AIC: Cached collection (%s) is older than embedded (%s), ignoring cache", cacheCol.Version, embeddedCollection.Version)
	}

	// 5. Fallback to Embedded
	log.Printf("AIC: Using embedded collection (%s)", embeddedCollection.Version)
	return &embeddedCollection, nil
}

func fetchRemote() (*CuratedList, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(remoteURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	var col CuratedList
	if err := json.NewDecoder(resp.Body).Decode(&col); err != nil {
		return nil, err
	}
	if len(col.Entries) == 0 {
		return nil, fmt.Errorf("remote collection is empty or malformed (schema mismatch)")
	}
	return &col, nil
}

// RefreshRemoteCollection forces a fetch from GitHub, and updates the local cache if successful.
func RefreshRemoteCollection() (*CuratedList, error) {
	col, err := fetchRemote()
	if err != nil {
		return nil, err
	}

	if semver.Compare(col.Version, embeddedCollection.Version) >= 0 {
		cacheDir := filepath.Join(config.GetWorkingDir(), "cache", "artic")
		cachePath := filepath.Join(cacheDir, cacheFileName)
		_ = os.MkdirAll(cacheDir, 0755)
		if err := saveCache(cachePath, col); err != nil {
			log.Printf("AIC: Failed to save cache during refresh: %v", err)
		}
		return col, nil
	}

	log.Printf("AIC: Remote collection (%s) is not newer than embedded (%s), no update needed", col.Version, embeddedCollection.Version)
	return nil, nil
}

func loadCache(path string) (*CuratedList, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var col CuratedList
	if err := json.NewDecoder(f).Decode(&col); err != nil {
		return nil, err
	}
	if len(col.Entries) == 0 {
		return nil, fmt.Errorf("remote collection is empty or malformed (schema mismatch)")
	}
	return &col, nil
}

func saveCache(path string, col *CuratedList) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(col)
}
