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
	"time"

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
	avoidMap        sync.Map        // Thread-safe map for InAvoidSet checks
	userid          string
	mu              sync.RWMutex // Mutex for thread-safe access
	// Advanced
	LogLevel                string       `json:"logLevel"`
	MaxConcurrentProcessors int          `json:"maxConcurrentProcessors"`
	Tuning                  TuningConfig `json:"tuning"`

	// Callbacks
	QueryRemovedCallback func(queryID string) `json:"-"`
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

// SmartFitMode defines the behavior of the Smart Fit algorithm
type SmartFitMode int

const (
	SmartFitOff        SmartFitMode = 0 // Disabled (Legacy: SmartFit=false)
	SmartFitNormal     SmartFitMode = 1 // Strict Aspect Ratio (Legacy: SmartFit=true, Unlock=false)
	SmartFitAggressive SmartFitMode = 2 // Relaxed Aspect Ratio (Legacy: SmartFit=true, Unlock=true)
)

func (m SmartFitMode) String() string {
	switch m {
	case SmartFitOff:
		return "Disabled"
	case SmartFitNormal:
		return "Quality"
	case SmartFitAggressive:
		return "Flexibility"
	default:
		return "Unknown"
	}
}

// GetSmartFitModes returns a list of available smart fit modes as strings
func GetSmartFitModes() []string {
	return []string{
		SmartFitOff.String(),
		SmartFitNormal.String(),
		SmartFitAggressive.String(),
	}
}

// ParseSmartFitMode parses a string into a SmartFitMode
func ParseSmartFitMode(s string) SmartFitMode {
	switch s {
	case "Disabled":
		return SmartFitOff
	case "Quality":
		return SmartFitNormal
	case "Flexibility":
		return SmartFitAggressive
	default:
		return SmartFitNormal
	}
}

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
			Tuning:          DefaultTuningConfig(),
		}
		// Load config from file
		if err := cfgInstance.loadFromPrefs(); err != nil {
			// Handle error, e.g., log, use defaults
			log.Printf("Error loading config: %v", err)
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

	// Migration: Ensure Favorites query exists (for existing users upgrading)
	hasFavorites := false
	for _, q := range c.Queries {
		if q.ID == FavoritesQueryID {
			hasFavorites = true
			break
		}
	}
	if !hasFavorites {
		log.Println("Migration: Adding missing Favorites query to configuration.")
		favQuery := ImageQuery{
			ID:          FavoritesQueryID,
			Description: "Favorite Images",
			URL:         FavoritesQueryID,
			Active:      true,
			Provider:    "Favorites",
		}
		c.Queries = append(c.Queries, favQuery)
		// We don't save immediately here to avoid writing to disk on boot unless necessary,
		// but since this is an in-memory fix for the session, it works.
		// Use c.save() if we want it persisted immediately.
		c.save()
	}

	// Populate sync.Map from loaded AvoidSet
	if c.AvoidSet != nil {
		for k, v := range c.AvoidSet {
			c.avoidMap.Store(k, v)
		}
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

	// Data migration: Prune stale Favorites queries
	// During development we changed from "favorites://default" to "favorites://favorite_images"
	var finalQueries []ImageQuery

	for _, q := range c.Queries {

		if q.Provider == "Favorites" && q.ID != FavoritesQueryID {
			log.Printf("Migration: Pruning stale Favorites query: %s", q.ID)
			queriesChanged = true
			continue
		}
		finalQueries = append(finalQueries, q)
	}

	// Data migration: Auto-Enable Favorites

	// Data migration: Auto-Enable Wikimedia Featured
	// User requested removal of this logic.
	// New installs get it via default_config.json.
	// Existing users who deleted it stay deleted.

	// Data migration: Update query IDs to prevent collisions (Provider:URL)
	for i := range finalQueries {
		if finalQueries[i].Provider == "Favorites" {
			continue
		}
		// Generate new ID using provider + URL to prevent object:ID collisions between museums
		newID := GenerateQueryID(finalQueries[i].Provider + ":" + finalQueries[i].URL)
		if finalQueries[i].ID != newID {
			log.Printf("Migration: Updating query ID for %s: %s -> %s", finalQueries[i].Provider, finalQueries[i].ID, newID)
			finalQueries[i].ID = newID
			queriesChanged = true
		}
	}

	c.Queries = finalQueries

	if queriesChanged {
		// Re-save the config with the new IDs/Settings immediately
		c.save()
	}

	return nil
}

// AddProviderQuery is the unified method for adding a query for ANY provider.
func (c *Config) AddProviderQuery(description, url, provider string, active bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var id string
	if provider == "Favorites" {
		// Favorites uses the URL itself as the ID (legacy behavior)
		id = url
	} else {
		// Include provider in ID to prevent collisions across different providers using same IDs (e.g. object:123)
		id = GenerateQueryID(provider + ":" + url)
	}

	if c.isDuplicateID(id) {
		// If it's favorites and already exists, just return it (no error)
		if provider == "Favorites" {
			return id, nil
		}
		return "", fmt.Errorf("duplicate query: this URL already exists")
	}

	newQuery := ImageQuery{
		ID:          id,
		Description: description,
		URL:         url,
		Active:      active,
		Provider:    provider,
	}

	c.Queries = append([]ImageQuery{newQuery}, c.Queries...)
	c.save()
	return id, nil
}

// AddImageQuery adds a new Wallhaven image query.
func (c *Config) AddImageQuery(desc, url string, active bool) (string, error) {
	return c.AddProviderQuery(desc, url, "Wallhaven", active)
}

// AddUnsplashQuery adds a new Unsplash query.
func (c *Config) AddUnsplashQuery(description, url string, active bool) (string, error) {
	return c.AddProviderQuery(description, url, "Unsplash", active)
}

// AddPexelsQuery adds a new Pexels query.
func (c *Config) AddPexelsQuery(description, url string, active bool) (string, error) {
	return c.AddProviderQuery(description, url, "Pexels", active)
}

// AddWikimediaQuery adds a new Wikimedia query.
func (c *Config) AddWikimediaQuery(description, url string, active bool) (string, error) {
	return c.AddProviderQuery(description, url, "Wikimedia", active)
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

	// Trigger callback
	if c.QueryRemovedCallback != nil {
		go c.QueryRemovedCallback(id)
	}
	return nil
}

// SetQueryRemovedCallback sets the callback for when a query is removed.
func (c *Config) SetQueryRemovedCallback(callback func(queryID string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.QueryRemovedCallback = callback
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

// GetActiveQueryIDs returns a map of all currently active query IDs.
func (c *Config) GetActiveQueryIDs() map[string]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	active := make(map[string]bool)
	for _, q := range c.Queries {
		if q.Active {
			active[q.ID] = true
		}
	}
	return active
}

// InAvoidSet checks if the given ID is in the avoid set
func (c *Config) InAvoidSet(id string) bool {
	_, found := c.avoidMap.Load(id)
	return found
}

// AddToAvoidSet adds the given ID to the avoid set
func (c *Config) AddToAvoidSet(id string) {
	c.avoidMap.Store(id, true)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.save()
}

// ResetAvoidSet clears the avoid set
func (c *Config) ResetAvoidSet() {
	c.avoidMap.Range(func(key, value interface{}) bool {
		c.avoidMap.Delete(key)
		return true
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	c.save()
}

// GetAvoidSet returns a copy of the avoid set.
func (c *Config) GetAvoidSet() map[string]bool {
	avoidSet := make(map[string]bool)
	c.avoidMap.Range(func(key, value interface{}) bool {
		avoidSet[key.(string)] = true
		return true
	})
	return avoidSet
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

// GetGooglePhotosToken returns the Google Photos Access Token from the keyring.
func (c *Config) GetGooglePhotosToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	token, err := keyring.Get(GooglePhotosTokenPrefKey, c.userid)
	if err != nil {
		if !errors.Is(err, keyring.ErrNotFound) {
			log.Printf("failed to retrieve Google Photos token from keyring: %v", err)
		}
		return ""
	}
	return token
}

// SetGooglePhotosToken sets the Google Photos Access Token in the keyring.
func (c *Config) SetGooglePhotosToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := keyring.Set(GooglePhotosTokenPrefKey, c.userid, token)
	if err != nil {
		log.Printf("failed to save Google Photos token to keyring: %v", err)
	}
}

// GetGooglePhotosRefreshToken returns the Google Photos Refresh Token from the keyring.
func (c *Config) GetGooglePhotosRefreshToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	token, err := keyring.Get(GooglePhotosRefreshTokenPrefKey, c.userid)
	if err != nil {
		if !errors.Is(err, keyring.ErrNotFound) {
			log.Printf("failed to retrieve Google Photos refresh token from keyring: %v", err)
		}
		return ""
	}
	return token
}

// SetGooglePhotosRefreshToken sets the Google Photos Refresh Token in the keyring.
func (c *Config) SetGooglePhotosRefreshToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := keyring.Set(GooglePhotosRefreshTokenPrefKey, c.userid, token)
	if err != nil {
		log.Printf("failed to save Google Photos refresh token to keyring: %v", err)
	}
}

// GetGooglePhotosTokenExpiry returns the Google Photos Token Expiry from the keyring.
func (c *Config) GetGooglePhotosTokenExpiry() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	expiryStr := c.StringWithFallback(GooglePhotosTokenExpiryPrefKey, "")
	if expiryStr == "" {
		return time.Time{}
	}
	expiry, err := time.Parse(time.RFC3339, expiryStr)
	if err != nil {
		log.Printf("failed to parse Google Photos token expiry: %v", err)
		return time.Time{}
	}
	return expiry
}

// SetGooglePhotosTokenExpiry sets the Google Photos Token Expiry in the keyring.
func (c *Config) SetGooglePhotosTokenExpiry(expiry time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetString(GooglePhotosTokenExpiryPrefKey, expiry.Format(time.RFC3339))
}

// AddFavoritesQuery adds a new Favorites query.
func (c *Config) AddFavoritesQuery(description, url string, active bool) (string, error) {
	return c.AddProviderQuery(description, url, "Favorites", active)
}

// AddGooglePhotosQuery adds a new Google Photos query.
func (c *Config) AddGooglePhotosQuery(description, url string, active bool) (string, error) {
	return c.AddProviderQuery(description, url, "GooglePhotos", active)
}

// RemoveGooglePhotosQuery removes a Google Photos query from the unified list.
func (c *Config) RemoveGooglePhotosQuery(id string) error {
	return c.RemoveImageQuery(id) // Reuse generic remove
}

// EnableGooglePhotosQuery enables a Google Photos query.
func (c *Config) EnableGooglePhotosQuery(id string) error {
	return c.EnableImageQuery(id)
}

// DisableGooglePhotosQuery disables a Google Photos query.
func (c *Config) DisableGooglePhotosQuery(id string) error {
	return c.DisableImageQuery(id)
}

// GetGooglePhotosQueries returns a copy of the Google Photos queries in a thread-safe manner.
func (c *Config) GetGooglePhotosQueries() []ImageQuery {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var queries []ImageQuery
	for _, q := range c.Queries {
		if q.Provider == "GooglePhotos" {
			queries = append(queries, q)
		}
	}
	return queries
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

// GetSyncMonitors returns the sync monitors preference.
func (c *Config) GetSyncMonitors() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BoolWithFallback(SyncMonitorsPrefKey, true) // Default to true
}

// SetSyncMonitors sets the sync monitors preference.
func (c *Config) SetSyncMonitors(enable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetBool(SyncMonitorsPrefKey, enable)
}

// GetStaggerMonitorChanges returns the stagger monitor changes preference.
func (c *Config) GetStaggerMonitorChanges() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BoolWithFallback(StaggerMonitorChangesPrefKey, false) // Default to false
}

// SetStaggerMonitorChanges sets the stagger monitor changes preference.
func (c *Config) SetStaggerMonitorChanges(enable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetBool(StaggerMonitorChangesPrefKey, enable)
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
	return c.BoolWithFallback(FaceCropPrefKey, true) // Default: true
}

// GetAssetManager returns the asset manager
func (c *Config) GetAssetManager() *asset.Manager {
	return c.assetMgr
}

// SetUnlockAspectRatio sets the unlock aspect ratio flag.
func (c *Config) SetUnlockAspectRatio(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetBool("UnlockAspectRatio", enabled)
}

// SetSmartFitMode sets the smart fit mode.
func (c *Config) SetSmartFitMode(mode SmartFitMode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetInt(SmartFitModePrefKey, int(mode))
}

func (c *Config) GetSmartFitMode() SmartFitMode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check if the new preference is set (check if key exists? Fyne prefs don't easily allow "exists" check without fallback)
	// We use -1 as a sentinel for "not set"
	val := c.IntWithFallback(SmartFitModePrefKey, -1)
	if val != -1 {
		return SmartFitMode(val)
	}

	// Migration Logic: Use the central SmartFitPrefKey constant
	smartFit := c.BoolWithFallback(SmartFitPrefKey, true)
	// Let's assume default "On" as per user experience.

	if !smartFit {
		return SmartFitOff
	}

	// Default to Flexibility (Aggressive) if not explicitly set to unlock=false
	unlock := c.BoolWithFallback("UnlockAspectRatio", true) // Default: true for Flexibility
	if unlock {
		return SmartFitAggressive
	}
	return SmartFitNormal
}

// GetUnlockAspectRatio returns the unlock aspect ratio flag.
func (c *Config) GetUnlockAspectRatio() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BoolWithFallback("UnlockAspectRatio", false)
}

// Save saves the current configuration to the user's config file
func (c *Config) save() {
	// Don't lock the mutex here because we're already holding it in all calling functions

	// Sync avoidMap to AvoidSet for JSON marshaling
	c.AvoidSet = make(map[string]bool)
	c.avoidMap.Range(func(key, value interface{}) bool {
		c.AvoidSet[key.(string)] = true
		return true
	})

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

// AddMetMuseumQuery adds a new Met Museum query.
func (c *Config) AddMetMuseumQuery(description, url string, active bool) (string, error) {
	return c.AddProviderQuery(description, url, "MetMuseum", active)
}

// RemoveMetMuseumQuery removes a Met Museum query.
func (c *Config) RemoveMetMuseumQuery(id string) error {
	return c.RemoveImageQuery(id)
}

// EnableMetMuseumQuery enables a Met Museum query.
func (c *Config) EnableMetMuseumQuery(id string) error {
	return c.EnableImageQuery(id)
}

// DisableMetMuseumQuery disables a Met Museum query.
func (c *Config) DisableMetMuseumQuery(id string) error {
	return c.DisableImageQuery(id)
}

// GetMetMuseumQueries returns a copy of the Met Museum queries.
func (c *Config) GetMetMuseumQueries() []ImageQuery {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var queries []ImageQuery
	for _, q := range c.Queries {
		if q.Provider == "MetMuseum" {
			queries = append(queries, q)
		}
	}
	return queries
}

// AddArtInstituteChicagoQuery adds a new AIC query.
func (c *Config) AddArtInstituteChicagoQuery(description, url string, active bool) (string, error) {
	return c.AddProviderQuery(description, url, "ArtInstituteChicago", active)
}

// EnableArtInstituteChicagoQuery enables an AIC query.
func (c *Config) EnableArtInstituteChicagoQuery(id string) error {
	return c.EnableImageQuery(id)
}

// DisableArtInstituteChicagoQuery disables an AIC query.
func (c *Config) DisableArtInstituteChicagoQuery(id string) error {
	return c.DisableImageQuery(id)
}

// GetArtInstituteChicagoQueries returns a copy of the AIC queries.
func (c *Config) GetArtInstituteChicagoQueries() []ImageQuery {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var queries []ImageQuery
	for _, q := range c.Queries {
		if q.Provider == "ArtInstituteChicago" {
			queries = append(queries, q)
		}
	}
	return queries
}
