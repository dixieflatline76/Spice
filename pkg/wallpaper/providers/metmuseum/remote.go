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

	"github.com/dixieflatline76/Spice/config"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
)

// Collection represents the structure of the remote JSON file
type Collection struct {
	Version     int    `json:"version"`
	Description string `json:"description"`
	IDs         []int  `json:"ids"`
}

const (
	remoteURL     = "https://raw.githubusercontent.com/dixieflatline76/Spice/feature/provider-ux-overhaul/docs/collections/met.json"
	cacheFileName = "met_cache.json"
)

//go:embed met.json
var embeddedJSON []byte

// Global cache fallback in case everything fails
var embeddedCollection Collection

// Essential landscape masterpieces to hook new users
var SpiceMelangeIDs = []int{
	11417,  // Washington Crossing the Delaware (Leutze) - Iconic American history
	436535, // Wheat Field with Cypresses (Van Gogh) - Vibrant, recognizable texture
	45434,  // The Great Wave (Hokusai) - Globally recognized icon
	437133, // Garden at Sainte-Adresse (Monet) - Bright, colorful Impressionism
	10497,  // The Oxbow (Cole) - Dramatic storm vs. pastoral landscape
	435809, // The Harvesters (Bruegel) - Detailed, immersive window into history
	437853, // Venice (Turner) - Dreamy, atmospheric light
	10154,  // Rocky Mountains (Bierstadt) - Epic scale nature
	438817, // Rehearsal of the Ballet (Degas) - Movement and culture
	436534, // Starry Night Drawing (Van Gogh) - Rare, interesting ink sketch
	11122,  // The Gulf Stream (Homer) - Dramatic narrative
	437658, // Road from Versailles (Pissarro) - Classic village scenery
}

// InitRemoteCollection initializes the embedded collection from assets
// and attempts to refresh the cache from the remote URL.
// It returns the most up-to-date collection available (Remote > Cache > Embedded > Fallback).
func InitRemoteCollection(cfg *wallpaper.Config) (*Collection, error) {
	// 0. Initialize with Hardcoded Fallback (Ground Zero)
	embeddedCollection = Collection{
		Version:     1,
		Description: "Spice Melange (Essential Fallback)",
		IDs:         SpiceMelangeIDs,
	}

	// 1. Load Embedded (Always succeed fallback)
	if err := json.Unmarshal(embeddedJSON, &embeddedCollection); err != nil {
		log.Printf("MET: Failed to parse embedded collection, using hardcoded fallback: %v", err)
	}

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
		fetchedCollection = col
		// Update Cache
		if err := saveCache(cachePath, col); err != nil {
			log.Printf("MET: Failed to save cache: %v", err)
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
			log.Printf("MET: Successfully loaded remote collection (v%d)", fetchedCollection.Version)
			return fetchedCollection, nil
		}
		log.Printf("MET: Remote fetch failed: %v", <-fetchErr)
	case <-time.After(3 * time.Second): // 3s timeout usually enough for raw.github
		log.Printf("MET: Remote fetch timed out, falling back to cache")
	}

	// 4. Fallback to Cache
	if cacheCol, err := loadCache(cachePath); err == nil {
		log.Printf("MET: Loaded cached collection (v%d)", cacheCol.Version)
		return cacheCol, nil
	} else {
		log.Printf("MET: Failed to load cache: %v", err)
	}

	// 5. Fallback to Embedded
	log.Printf("MET: Using embedded collection (v%d)", embeddedCollection.Version)
	return &embeddedCollection, nil
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
