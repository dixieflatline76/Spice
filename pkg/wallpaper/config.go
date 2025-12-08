package wallpaper

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dixieflatline76/Spice/util/log"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/asset"
	"github.com/dixieflatline76/Spice/config"
	"github.com/zalando/go-keyring"
)

// Package config provides configuration management for the Wallpaper Downloader service
// TODO: Explore moving all preferences Set/Get into this file
// TODO: Explore tradeoffs of using a single JSON string preference vs multiple preferences

// Config struct to hold all configuration data
type Config struct {
	fyne.Preferences
	WallhavenAPIKey string          `json:"wallhaven_api_key"`
	Queries         []ImageQuery    `json:"queries"`          // Unified list of image queries
	ImageQueries    []ImageQuery    `json:"query_urls"`       // Legacy: List of image queries (Wallhaven)
	UnsplashQueries []ImageQuery    `json:"unsplash_queries"` // Legacy: List of Unsplash image queries
	PexelsQueries   []ImageQuery    `json:"pexels_queries"`   // Legacy: List of Pexels image queries
	assetMgr        *asset.Manager  // Asset manager
	AvoidSet        map[string]bool `json:"avoid_set"` // Set of image URLs to avoid
	userid          string
	mu              sync.RWMutex // Mutex for thread-safe access
}

// ImageQuery struct to hold the URL of an image and whether it is active
type ImageQuery struct {
	ID          string `json:"id"`
	Description string `json:"desc"`
	URL         string `json:"url"`
	Active      bool   `json:"active"`
	Provider    string `json:"provider"` // Provider name (e.g., "Wallhaven", "Unsplash", "Pexels")
}

var (
	cfgInstance *Config
	cfgOnce     sync.Once
)

// GetConfig returns the singleton instance of Config.
func GetConfig(p fyne.Preferences) *Config {
	cfgOnce.Do(func() {
		u, e := user.Current()
		if e != nil {
			log.Fatalf("failed to initialize %s: %s", config.AppName, e)
		}
		cfgInstance = &Config{
			Preferences:     p,
			Queries:         make([]ImageQuery, 0),
			ImageQueries:    make([]ImageQuery, 0),
			UnsplashQueries: make([]ImageQuery, 0),
			PexelsQueries:   make([]ImageQuery, 0), // Initialize PexelsQueries
			assetMgr:        asset.NewManager(),
			AvoidSet:        make(map[string]bool),
			userid:          u.Uid,
		}
		// Load config from file
		if err := cfgInstance.loadFromPrefs(); err != nil {
			// Handle error, e.g., log, use defaults
			fmt.Println("Error loading config:", err)
		}
	})
	return cfgInstance
}

// GetPath returns the path to the user's config directory
func GetPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Error getting user home directory: %v", err)
	}
	return filepath.Join(homeDir, "."+strings.ToLower(config.AppName))
}

// loadFromPrefs loads configuration from the specified file
func (c *Config) loadFromPrefs() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	defaultCfg, err := c.assetMgr.GetText("default_config.json")
	if err != nil {
		return err
	}
	cfgText := c.StringWithFallback(wallhavenConfigPrefKey, defaultCfg)

	if err := json.Unmarshal([]byte(cfgText), c); err != nil {
		return err
	}

	// Data migration: Iterate and backfill missing IDs for Wallhaven queries
	queriesChanged := false
	for i, q := range c.ImageQueries {
		if q.ID == "" {
			c.ImageQueries[i].ID = GenerateQueryID(q.URL)
			queriesChanged = true
		}
	}

	// Data migration: Iterate and backfill missing IDs for Unsplash queries
	for i, q := range c.UnsplashQueries {
		if q.ID == "" {
			c.UnsplashQueries[i].ID = GenerateQueryID(q.URL)
			queriesChanged = true
		}
	}

	// Data migration: Iterate and backfill missing IDs for Pexels queries
	for i, q := range c.PexelsQueries {
		if q.ID == "" {
			c.PexelsQueries[i].ID = GenerateQueryID(q.URL)
			queriesChanged = true
		}
	}

	// Data migration: Unify queries into single list
	// Only migrate if Queries is empty and we have legacy data
	if len(c.Queries) == 0 && (len(c.ImageQueries) > 0 || len(c.UnsplashQueries) > 0 || len(c.PexelsQueries) > 0) {
		log.Print("Migrating legacy queries to unified list...")
		for _, q := range c.ImageQueries {
			q.Provider = "Wallhaven"
			c.Queries = append(c.Queries, q)
		}
		for _, q := range c.UnsplashQueries {
			q.Provider = "Unsplash"
			c.Queries = append(c.Queries, q)
		}
		for _, q := range c.PexelsQueries {
			q.Provider = "Pexels"
			c.Queries = append(c.Queries, q)
		}

		// Clear legacy lists
		c.ImageQueries = make([]ImageQuery, 0)
		c.UnsplashQueries = make([]ImageQuery, 0)
		c.PexelsQueries = make([]ImageQuery, 0)
		queriesChanged = true
	}

	// Data migration: Iterate and backfill missing IDs and Providers for unified queries
	// This covers fresh loads from default_config.json where IDs/Providers might be implicit
	for i, q := range c.Queries {
		if q.ID == "" {
			c.Queries[i].ID = GenerateQueryID(q.URL)
			queriesChanged = true
		}
		// Default to Wallhaven if provider is missing (legacy compat or partial config)
		if q.Provider == "" {
			c.Queries[i].Provider = "Wallhaven"
			queriesChanged = true
		}
	}

	// Sanitize Face Boost/Crop settings (Mutual Exclusivity)
	// We check the Fyne preferences directly to avoid recursive locking (deadlock)
	faceCrop := c.BoolWithFallback(FaceCropPrefKey, false)
	faceBoost := c.BoolWithFallback(FaceBoostPrefKey, false)

	if faceCrop && faceBoost {
		c.SetBool(FaceBoostPrefKey, false)
	}

	if queriesChanged {
		// Re-save the config with the new IDs/Settings immediately
		c.save()
	}

	return nil
}

// AddImageQuery adds a new image query to the list and returns its new ID.
func (c *Config) AddImageQuery(desc, url string, active bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	newID := GenerateQueryID(url)
	// Check for duplicates across unified list
	if c.isDuplicateID(newID) {
		return "", errors.New("duplicate query: this URL already exists")
	}

	newItem := ImageQuery{
		ID:          newID,
		Description: desc,
		URL:         url,
		Active:      active,
		Provider:    "Wallhaven",
	}
	c.Queries = append([]ImageQuery{newItem}, c.Queries...)
	c.save()
	return newID, nil
}

// AddUnsplashQuery adds a new Unsplash query to the list.
func (c *Config) AddUnsplashQuery(description, url string, active bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := GenerateQueryID(url)
	if c.isDuplicateID(id) {
		return "", fmt.Errorf("duplicate query: this URL already exists")
	}

	newQuery := ImageQuery{
		ID:          id,
		Description: description,
		URL:         url,
		Active:      active,
		Provider:    "Unsplash",
	}

	c.Queries = append([]ImageQuery{newQuery}, c.Queries...)
	c.save()
	return id, nil
}

// AddPexelsQuery adds a new Pexels query to the list.
func (c *Config) AddPexelsQuery(description, url string, active bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := GenerateQueryID(url)
	if c.isDuplicateID(id) {
		return "", fmt.Errorf("duplicate query: this URL already exists")
	}

	newQuery := ImageQuery{
		ID:          id,
		Description: description,
		URL:         url,
		Active:      active,
		Provider:    "Pexels",
	}

	c.Queries = append([]ImageQuery{newQuery}, c.Queries...)
	c.save()
	return id, nil
}

// AddWikimediaQuery adds a new Wikimedia query to the list.
func (c *Config) AddWikimediaQuery(description, url string, active bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := GenerateQueryID(url)
	if c.isDuplicateID(id) {
		return "", fmt.Errorf("duplicate query: this URL already exists")
	}

	newQuery := ImageQuery{
		ID:          id,
		Description: description,
		URL:         url,
		Active:      active,
		Provider:    "Wikimedia",
	}

	c.Queries = append([]ImageQuery{newQuery}, c.Queries...)
	c.save()
	return id, nil
}

// isDuplicateID checks if a query ID already exists in the unified list.
func (c *Config) isDuplicateID(id string) bool {
	for _, q := range c.Queries {
		if q.ID == id {
			return true
		}
	}
	return false
}

// IsDuplicateID checks if a query ID already exists (exported version).
func (c *Config) IsDuplicateID(id string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isDuplicateID(id)
}

// findQueryIndex is a helper to find a query by its stable ID in the unified slice
func (c *Config) findQueryIndex(id string) (int, error) {
	for i, q := range c.Queries {
		if q.ID == id {
			return i, nil
		}
	}
	return -1, fmt.Errorf("query with ID %s not found", id)
}

// GetQuery returns a query by its ID. uniqueID is required.
func (c *Config) GetQuery(id string) (ImageQuery, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	idx, err := c.findQueryIndex(id)
	if err != nil {
		return ImageQuery{}, false
	}
	return c.Queries[idx], true
}

// RemoveImageQuery removes the image query with the specified ID
func (c *Config) RemoveImageQuery(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	index, err := c.findQueryIndex(id)
	if err != nil {
		return err
	}

	c.Queries = append(c.Queries[:index], c.Queries[index+1:]...)
	c.save()
	return nil
}

// RemoveUnsplashQuery removes an Unsplash query from the unified list.
func (c *Config) RemoveUnsplashQuery(id string) error {
	return c.RemoveImageQuery(id) // Reuse generic remove
}

// RemovePexelsQuery removes a Pexels query from the unified list.
func (c *Config) RemovePexelsQuery(id string) error {
	return c.RemoveImageQuery(id) // Reuse generic remove
}

// EnableImageQuery enables the image query with the specified ID
func (c *Config) EnableImageQuery(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	index, err := c.findQueryIndex(id)
	if err != nil {
		return err
	}

	c.Queries[index].Active = true
	c.save()
	return nil
}

// EnableUnsplashQuery enables an Unsplash query.
func (c *Config) EnableUnsplashQuery(id string) error {
	return c.EnableImageQuery(id)
}

// EnablePexelsQuery enables a Pexels query.
func (c *Config) EnablePexelsQuery(id string) error {
	return c.EnableImageQuery(id)
}

// DisableImageQuery disables the image query with the specified ID
func (c *Config) DisableImageQuery(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	index, err := c.findQueryIndex(id)
	if err != nil {
		return err
	}

	c.Queries[index].Active = false
	c.save()
	return nil
}

// DisableUnsplashQuery disables an Unsplash query.
func (c *Config) DisableUnsplashQuery(id string) error {
	return c.DisableImageQuery(id)
}

// DisablePexelsQuery disables a Pexels query.
func (c *Config) DisablePexelsQuery(id string) error {
	return c.DisableImageQuery(id)
}

// InAvoidSet checks if the given ID is in the avoid set
func (c *Config) InAvoidSet(id string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, found := c.AvoidSet[id]
	return found
}

// AddToAvoidSet adds the given ID to the avoid set
func (c *Config) AddToAvoidSet(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AvoidSet[id] = true
	c.save()
}

// ResetAvoidSet clears the avoid set
func (c *Config) ResetAvoidSet() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AvoidSet = make(map[string]bool)
	c.save()
}

// GetCacheSize returns the cache size enumeration from the config, or the default value if not set or invalid
func (c *Config) GetCacheSize() CacheSize {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CacheSize(c.IntWithFallback(CacheSizePrefKey, int(Cache200Images))) // Default to 200 images
}

// SetCacheSize sets the cache size enumeration and saves it
func (c *Config) SetCacheSize(size CacheSize) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetInt(CacheSizePrefKey, int(size))
}

// GetSmartFit returns the smart fit preference from the config.
func (c *Config) GetSmartFit() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BoolWithFallback(SmartFitPrefKey, true) // Default to true
}

// SetSmartFit sets the smart fit preference.
func (c *Config) SetSmartFit(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetBool(SmartFitPrefKey, enabled) // Save the preference to the config file
}

// GetWallpaperChangeFrequency returns the wallpaper change frequency enumeration from the config, or the default value if not set or invalid
func (c *Config) GetWallpaperChangeFrequency() Frequency {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Frequency(c.IntWithFallback(WallpaperChgFreqPrefKey, int(FrequencyHourly))) // Default to hourly
}

// SetWallpaperChangeFrequency sets the frequency enumeration for wallpaper changes and saves it
func (c *Config) SetWallpaperChangeFrequency(frequency Frequency) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetInt(WallpaperChgFreqPrefKey, int(frequency))
}

// GetImgShuffle returns the image shuffle preference from the config.
func (c *Config) GetImgShuffle() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BoolWithFallback(ImgShufflePrefKey, false)
}

// SetImgShuffle sets the image shuffle preference.
func (c *Config) SetImgShuffle(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetBool(ImgShufflePrefKey, enabled)
}

// GetUnsplashToken returns the Unsplash Access Token from the keyring.
func (c *Config) GetUnsplashToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	token, err := keyring.Get(UnsplashTokenPrefKey, c.userid)
	if err != nil {
		// Log only if it's not a "not found" error to avoid noise on first run
		if !errors.Is(err, keyring.ErrNotFound) {
			log.Printf("failed to retrieve Unsplash token from keyring: %v", err)
		}
		return ""
	}
	return token
}

// SetUnsplashToken sets the Unsplash Access Token in the keyring.
func (c *Config) SetUnsplashToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := keyring.Set(UnsplashTokenPrefKey, c.userid, token)
	if err != nil {
		log.Printf("failed to save Unsplash token to keyring: %v", err)
	}
}

// GetWallhavenAPIKey returns the Wallhaven API key from the config.
func (c *Config) GetWallhavenAPIKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	apiKey, err := keyring.Get(WallhavenAPIKeyPrefKey, c.userid) // Try to get the API key from the keyring
	if err != nil {
		log.Printf("failed to retrieve Wallhaven API key from keyring: %v", err)
		return "" // Return an empty string if the keyring lookup fails
	}
	return apiKey // Return the API key from the keyring
}

// SetWallhavenAPIKey sets the Wallhaven API key.
func (c *Config) SetWallhavenAPIKey(apiKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := keyring.Set(WallhavenAPIKeyPrefKey, c.userid, apiKey) // Save the API key to the keyring
	if err != nil {
		log.Printf("failed to save Wallhaven API key to keyring: %v", err)
	}
}

// GetPexelsAPIKey returns the Pexels API key from the config.
func (c *Config) GetPexelsAPIKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	apiKey, err := keyring.Get(PexelsAPIKeyPrefKey, c.userid)
	if err != nil {
		if !errors.Is(err, keyring.ErrNotFound) {
			log.Printf("failed to retrieve Pexels API key from keyring: %v", err)
		}
		return ""
	}
	return apiKey
}

// SetPexelsAPIKey sets the Pexels API key.
func (c *Config) SetPexelsAPIKey(apiKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := keyring.Set(PexelsAPIKeyPrefKey, c.userid, apiKey)
	if err != nil {
		log.Printf("failed to save Pexels API key to keyring: %v", err)
	}
}

// SetChgImgOnStart returns the change image on start preference.
func (c *Config) SetChgImgOnStart(enable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetBool(ChgImgOnStartPrefKey, enable)
}

// GetChgImgOnStart returns the change image on start preference.
func (c *Config) GetChgImgOnStart() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BoolWithFallback(ChgImgOnStartPrefKey, true) // Return the change image on start preference with a fallback value of true if not set
}

// SetNightlyRefresh sets the nightly refresh preference.
func (c *Config) SetNightlyRefresh(enable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetBool(NightlyRefreshPrefKey, enable)
}

// GetNightlyRefresh returns the nightly refresh preference.
func (c *Config) GetNightlyRefresh() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BoolWithFallback(NightlyRefreshPrefKey, true) // Return the change image on start preference with a fallback value of true if not set
}

// SetFaceBoostEnabled sets the face boost preference.
func (c *Config) SetFaceBoostEnabled(enable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetBool(FaceBoostPrefKey, enable)
}

// GetFaceBoostEnabled returns the face boost preference.
func (c *Config) GetFaceBoostEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BoolWithFallback(FaceBoostPrefKey, false)
}

// SetFaceCropEnabled sets the face crop preference.
func (c *Config) SetFaceCropEnabled(enable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetBool(FaceCropPrefKey, enable)
}

// GetFaceCropEnabled returns the face crop preference.
func (c *Config) GetFaceCropEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BoolWithFallback(FaceCropPrefKey, false)
}

// GetAssetManager returns the asset manager
func (c *Config) GetAssetManager() *asset.Manager {
	return c.assetMgr
}

// Save saves the current configuration to the user's config file
func (c *Config) save() {
	// Don't lock the mutex here because we're already holding it in all calling functions
	data, err := json.MarshalIndent(c, "", "  ") // Use indentation for readability
	if err != nil {
		log.Fatalf("Error encoding config data: %v", err)
	}

	c.SetString(wallhavenConfigPrefKey, string(data))
}

// GetImageQueries returns a copy of the Wallhaven queries in a thread-safe manner.
func (c *Config) GetImageQueries() []ImageQuery {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var queries []ImageQuery
	for _, q := range c.Queries {
		if q.Provider == "Wallhaven" {
			queries = append(queries, q)
		}
	}
	return queries
}

// GetUnsplashQueries returns a copy of the Unsplash queries in a thread-safe manner.
func (c *Config) GetUnsplashQueries() []ImageQuery {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var queries []ImageQuery
	for _, q := range c.Queries {
		if q.Provider == "Unsplash" {
			queries = append(queries, q)
		}
	}
	return queries
}

// GetPexelsQueries returns a copy of the Pexels queries in a thread-safe manner.
func (c *Config) GetPexelsQueries() []ImageQuery {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var queries []ImageQuery
	for _, q := range c.Queries {
		if q.Provider == "Pexels" {
			queries = append(queries, q)
		}
	}
	return queries
}

// GetWikimediaQueries returns a copy of the Wikimedia queries in a thread-safe manner.
func (c *Config) GetWikimediaQueries() []ImageQuery {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var queries []ImageQuery
	for _, q := range c.Queries {
		if q.Provider == "Wikimedia" {
			queries = append(queries, q)
		}
	}
	return queries
}

// GetQueries returns a copy of all queries in a thread-safe manner.
func (c *Config) GetQueries() []ImageQuery {
	c.mu.RLock()
	defer c.mu.RUnlock()
	queries := make([]ImageQuery, len(c.Queries))
	copy(queries, c.Queries)
	return queries
}

// GetActiveQueries returns a copy of all active queries in a thread-safe manner.
func (c *Config) GetActiveQueries() []ImageQuery {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var active []ImageQuery
	for _, q := range c.Queries {
		if q.Active {
			active = append(active, q)
		}
	}
	return active
}
