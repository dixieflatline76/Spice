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

	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/util/log"
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

// Hardcoded fallback (Ground Zero)
var fallbackCollection = CuratedList{
	Version:     1,
	Description: "AIC Highlights (Ground Zero Fallback)",
	Tours: map[string]TourData{
		CollectionHighlights: {
			Name: "AIC Highlights: The Big Picture",
			// Seurat, Hopper, Wood, Hokusai, Monet, Caillebotte, Van Gogh
			IDs: []int{27992, 111628, 117302, 24645, 16568, 20684, 16571, 16545, 28560, 20701, 76244},
		},
		CollectionImpression: {
			Name: "Impressionist Vistas",
			// Monet (St. Lazare, Bordighera, Water Lilies), Renoir, Caillebotte, Pissarro
			IDs: []int{16568, 20684, 16571, 16545, 16546, 6565, 81538, 27992},
		},
		CollectionModern: {
			Name: "Modern Landscapes",
			IDs:  []int{111628, 20701, 60072, 76244, 115206},
		},
		CollectionAsia: {
			Name: "Arts of Asia: Landscape Prints",
			IDs:  []int{24645, 59056, 199092, 199093, 199120},
		},
	},
}

// InitRemoteCollection initializes the curated list from remote, cache, or embedded sources.
func InitRemoteCollection() (*CuratedList, error) {
	// 1. Start with Hardcoded Fallback
	embeddedCollection = fallbackCollection

	// 2. Load Embedded
	if err := json.Unmarshal(embeddedJSON, &embeddedCollection); err != nil {
		log.Printf("AIC: Failed to parse embedded collection, using hardcoded fallback: %v", err)
	}

	// 3. Determine Cache Path
	cacheDir := filepath.Join(config.GetWorkingDir(), "cache", "artic")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Printf("AIC: Failed to create cache dir: %v", err)
		return &embeddedCollection, nil
	}
	cachePath := filepath.Join(cacheDir, cacheFileName)

	var wg sync.WaitGroup
	var fetchedCollection *CuratedList
	fetchErr := make(chan error, 1)

	// 4. Async Fetch
	wg.Add(1)
	go func() {
		defer wg.Done()
		col, err := fetchRemote()
		if err != nil {
			fetchErr <- err
			return
		}
		fetchedCollection = col
		// Update Cache
		if err := saveCache(cachePath, col); err != nil {
			log.Printf("AIC: Failed to save cache: %v", err)
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
			log.Printf("AIC: Successfully loaded remote collection (v%d)", fetchedCollection.Version)
			return fetchedCollection, nil
		}
		log.Printf("AIC: Remote fetch failed: %v", <-fetchErr)
	case <-time.After(3 * time.Second):
		log.Printf("AIC: Remote fetch timed out, falling back to cache")
	}

	// 5. Fallback to Cache
	if cacheCol, err := loadCache(cachePath); err == nil {
		log.Printf("AIC: Loaded cached collection (v%d)", cacheCol.Version)
		return cacheCol, nil
	}

	// 6. Fallback to Embedded
	log.Printf("AIC: Using embedded collection (v%d)", embeddedCollection.Version)
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
	return &col, nil
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
