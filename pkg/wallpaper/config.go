package wallpaper

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/user"
	"path/filepath"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/util/log"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/v2/asset"
	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/zalando/go-keyring"
)

// Package config provides configuration management for the Wallpaper Downloader service
// TODO: Explore moving all preferences Set/Get into this file
// TODO: Explore tradeoffs of using a single JSON string preference vs multiple preferences

// Config struct to hold all configuration data
type Config struct {
	fyne.Preferences  `json:"-"`      // DO NOT serialize the Fyne interface to JSON (prevents "null" unmarshal panics)
	WallhavenAPIKey   string          `json:"wallhaven_api_key"`
	Queries           []ImageQuery    `json:"queries"`            // Unified list of image queries
	ImageQueries      []ImageQuery    `json:"query_urls"`         // Legacy: List of image queries (Wallhaven)
	PexelsQueries     []ImageQuery    `json:"pexels_queries"`     // Legacy: List of Pexels image queries
	WallhavenUsername string          `json:"wallhaven_username"` // Wallhaven username for sync
	assetMgr          *asset.Manager  // Asset manager
	AvoidSet          map[string]bool `json:"avoid_set"` // Set of image URLs to avoid
	avoidMap          sync.Map        // Thread-safe map for InAvoidSet checks
	userid            string
	mu                sync.RWMutex // Mutex for thread-safe access
	// Advanced
	LogLevel                string       `json:"logLevel"`
	MaxConcurrentProcessors int          `json:"maxConcurrentProcessors"`
	Tuning                  TuningConfig `json:"tuning"`

	// Callbacks
	QueryRemovedCallback     func(queryID string) `json:"-"`
	QueryDisabledCallback    func(queryID string) `json:"-"`
	FavoritesClearedCallback func()               `json:"-"`
	ShortcutsDisabled        bool                 `json:"shortcuts_disabled"`
	WallhavenSyncEnabled     bool                 `json:"wallhaven_sync_enabled"`
	MonitorPauseStates       map[string]bool      `json:"monitor_pause_states"`
}

// ImageQuery struct to hold the URL of an image and whether it is active
type ImageQuery struct {
	ID          string `json:"id"`
	Description string `json:"desc"`
	URL         string `json:"url"`
	Active      bool   `json:"active"`
	Provider    string `json:"provider"` // Provider name (e.g., "Wallhaven", "Unsplash", "Pexels")
	Managed     bool   `json:"managed"`  // Whether this query is managed by sync
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
		return i18n.T("Disabled")
	case SmartFitNormal:
		return i18n.T("Quality")
	case SmartFitAggressive:
		return i18n.T("Flexibility")
	default:
		return i18n.T("Unknown")
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
			Preferences:        p,
			Queries:            make([]ImageQuery, 0),
			ImageQueries:       make([]ImageQuery, 0),
			PexelsQueries:      make([]ImageQuery, 0),
			assetMgr:           asset.NewManager(),
			AvoidSet:           make(map[string]bool),
			MonitorPauseStates: make(map[string]bool),
			userid:             u.Uid,
			Tuning:             DefaultTuningConfig(),
		}
		// Load config from file
		if err := cfgInstance.loadFromPrefs(); err != nil {
			// Handle error, e.g., log, use defaults
			log.Printf("Error loading config: %v", err)
		}
	})
	return cfgInstance
}

// GetConfigInstance returns the already-initialized Config singleton.
// This panics if GetConfig has not been called yet.
func GetConfigInstance() *Config {
	if cfgInstance == nil {
		panic("wallpaper.GetConfigInstance called before GetConfig initialization")
	}
	return cfgInstance
}

// GetPath returns the path to the user's config directory
func GetPath() string {
	return config.GetAppDir()
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

	// Execute Migration Chain
	migrations := NewMigrationChain()
	if err := migrations.Execute(c); err != nil {
		log.Printf("Error running configuration migrations: %v", err)
		// We continue even if migration fails, as we have a partial config
	}

	// Sync AvoidSet from JSON into the thread-safe avoidMap (Post-Migration)
	for id := range c.AvoidSet {
		c.avoidMap.Store(id, true)
	}

	return nil
}

// AddProviderQuery is the unified method for adding a query for ANY provider.
func (c *Config) AddProviderQuery(description, url, provider string, active, managed bool) (string, error) {
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
	log.Debugf("[Config] AddProviderQuery: provider=%s, url=%s, generatedID=%s", provider, url, id)

	if c.isDuplicateID(id) {
		log.Debugf("[Config] AddProviderQuery: duplicate detected for ID=%s", id)
		// If it's favorites and already exists, just return it (no error)
		if provider == "Favorites" {
			// Ensure it's marked as managed if it was a legacy query
			for i := range c.Queries {
				if c.Queries[i].ID == id {
					if !c.Queries[i].Managed {
						log.Printf("AddProviderQuery: upgrading legacy Favorites query to managed")
						c.Queries[i].Managed = true
						c.save()
					}
					break
				}
			}
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
		Managed:     managed,
	}
	log.Debugf("[Config] AddProviderQuery: appending new query: %+v", newQuery)

	c.Queries = append([]ImageQuery{newQuery}, c.Queries...)
	c.save()
	return id, nil
}

// AddImageQuery adds a new Wallhaven image query.
func (c *Config) AddImageQuery(desc, url string, active bool) (string, error) {
	return c.AddProviderQuery(desc, url, "Wallhaven", active, false)
}

// AddPexelsQuery adds a new Pexels query.
func (c *Config) AddPexelsQuery(description, url string, active bool) (string, error) {
	return c.AddProviderQuery(description, url, "Pexels", active, false)
}

// AddWikimediaQuery adds a new Wikimedia query.
func (c *Config) AddWikimediaQuery(description, url string, active bool) (string, error) {
	return c.AddProviderQuery(description, url, "Wikimedia", active, false)
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

// SyncManagedQueries reconciles managed queries for a specific provider.
func (c *Config) SyncManagedQueries(provider string, remoteQueries []ImageQuery) {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Debugf("SyncManagedQueries starting for provider: %s. Remote count: %d", provider, len(remoteQueries))

	remoteMap := make(map[string]ImageQuery)
	for _, q := range remoteQueries {
		remoteMap[q.ID] = q
	}

	newQueries := make([]ImageQuery, 0, len(c.Queries))
	foundRemoteIDs := make(map[string]bool)

	// Update existing managed queries or keep non-managed ones
	for _, q := range c.Queries {
		if q.Provider == provider && q.Managed {
			if remoteQ, found := remoteMap[q.ID]; found {
				// Keep existing activation state, but update metadata if needed
				q.Description = remoteQ.Description
				newQueries = append(newQueries, q)
				foundRemoteIDs[q.ID] = true
				log.Debugf("SyncManagedQueries: Keeping existing managed query: %s", q.ID)
			} else {
				log.Debugf("SyncManagedQueries: Removing managed query no longer on remote: %s", q.ID)
			}
		} else {
			newQueries = append(newQueries, q)
		}
	}

	// Add new managed queries
	for _, remoteQ := range remoteQueries {
		if !foundRemoteIDs[remoteQ.ID] {
			log.Debugf("SyncManagedQueries: Adding new managed query: %s (%s)", remoteQ.ID, remoteQ.Description)
			newQueries = append(newQueries, remoteQ)
		}
	}

	c.Queries = newQueries
	log.Debugf("SyncManagedQueries finished. Total Queries now: %d", len(c.Queries))
	c.save()
}

func (c *Config) RemoveImageQuery(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	index, err := c.findQueryIndex(id)
	if err != nil {
		return err
	}

	if c.Queries[index].Managed {
		return fmt.Errorf("cannot remove a managed system query: %s", id)
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

// SetQueryDisabledCallback sets the callback for when a query is disabled.
func (c *Config) SetQueryDisabledCallback(callback func(queryID string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.QueryDisabledCallback = callback
}

// SetFavoritesClearedCallback sets the callback for when all favorites are cleared.
func (c *Config) SetFavoritesClearedCallback(callback func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.FavoritesClearedCallback = callback
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

	// Trigger callback
	if c.QueryDisabledCallback != nil {
		go c.QueryDisabledCallback(id)
	}
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

// InAvoidSet checks if the given ID is in the avoid set (exact match only).
func (c *Config) InAvoidSet(id string) bool {
	_, found := c.avoidMap.Load(id)
	return found
}

// AddToAvoidSet adds the given ID to the avoid set
func (c *Config) AddToAvoidSet(id string) {
	c.avoidMap.Store(id, true)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.AvoidSet == nil {
		c.AvoidSet = make(map[string]bool)
	}
	c.AvoidSet[id] = true
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
	c.AvoidSet = make(map[string]bool)
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
	return true // Permanently enabled
}

// SetImgShuffle sets the image shuffle preference.
func (c *Config) SetImgShuffle(enabled bool) {
	// No-op: Permanent Shuffle is active
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

// GetWikimediaPersonalToken returns the Wikimedia Personal API Token from the keyring.
func (c *Config) GetWikimediaPersonalToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	token, err := keyring.Get(WikimediaTokenPrefKey, c.userid)
	if err != nil {
		if !errors.Is(err, keyring.ErrNotFound) {
			log.Printf("failed to retrieve Wikimedia personal token from keyring: %v", err)
		}
		return ""
	}
	return token
}

// SetWikimediaPersonalToken sets the Wikimedia Personal API Token in the keyring.
func (c *Config) SetWikimediaPersonalToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if token == "" {
		_ = keyring.Delete(WikimediaTokenPrefKey, c.userid)
		return
	}
	err := keyring.Set(WikimediaTokenPrefKey, c.userid, token)
	if err != nil {
		log.Printf("failed to save Wikimedia personal token to keyring: %v", err)
	}
}

// GetWikimediaToken returns the Wikimedia OAuth Access Token from the keyring.
func (c *Config) GetWikimediaToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	token, err := keyring.Get(WikimediaAccessTokenPrefKey, c.userid)
	if err != nil {
		if !errors.Is(err, keyring.ErrNotFound) {
			log.Printf("failed to retrieve Wikimedia access token from keyring: %v", err)
		}
		return ""
	}
	return token
}

// SetWikimediaToken sets the Wikimedia OAuth Access Token in the keyring.
func (c *Config) SetWikimediaToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if token == "" {
		_ = keyring.Delete(WikimediaAccessTokenPrefKey, c.userid)
		return
	}
	err := keyring.Set(WikimediaAccessTokenPrefKey, c.userid, token)
	if err != nil {
		log.Printf("failed to save Wikimedia access token to keyring: %v", err)
	}
}

// GetWikimediaRefreshToken returns the Wikimedia OAuth Refresh Token from the keyring.
func (c *Config) GetWikimediaRefreshToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	token, err := keyring.Get(WikimediaRefreshTokenPrefKey, c.userid)
	if err != nil {
		if !errors.Is(err, keyring.ErrNotFound) {
			log.Printf("failed to retrieve Wikimedia refresh token from keyring: %v", err)
		}
		return ""
	}
	return token
}

// SetWikimediaRefreshToken sets the Wikimedia OAuth Refresh Token in the keyring.
func (c *Config) SetWikimediaRefreshToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if token == "" {
		_ = keyring.Delete(WikimediaRefreshTokenPrefKey, c.userid)
		return
	}
	err := keyring.Set(WikimediaRefreshTokenPrefKey, c.userid, token)
	if err != nil {
		log.Printf("failed to save Wikimedia refresh token to keyring: %v", err)
	}
}

// GetWikimediaTokenExpiry returns the Wikimedia OAuth Token Expiry from the keyring.
func (c *Config) GetWikimediaTokenExpiry() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	expiryStr := c.StringWithFallback(WikimediaTokenExpiryPrefKey, "")
	if expiryStr == "" {
		return time.Time{}
	}
	expiry, err := time.Parse(time.RFC3339, expiryStr)
	if err != nil {
		log.Printf("failed to parse Wikimedia token expiry: %v", err)
		return time.Time{}
	}
	return expiry
}

// SetWikimediaTokenExpiry sets the Wikimedia OAuth Token Expiry in the keyring.
func (c *Config) SetWikimediaTokenExpiry(expiry time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetString(WikimediaTokenExpiryPrefKey, expiry.Format(time.RFC3339))
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
	return c.AddProviderQuery(description, url, "Favorites", active, true)
}

// GetFavoritesQueries returns only the Favorites query.
func (c *Config) GetFavoritesQueries() []ImageQuery {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var queries []ImageQuery
	for i := range c.Queries {
		q := c.Queries[i]
		if q.Provider == "Favorites" {
			q.Managed = true // Force managed flag in memory for UI consistency
			queries = append(queries, q)
		}
	}
	return queries
}

// AddLocalFolderQuery adds a new Local Folder query.
// The url parameter should be the absolute path to the user-selected folder.
func (c *Config) AddLocalFolderQuery(description, url string, active bool) (string, error) {
	log.Debugf("[Config] AddLocalFolderQuery: desc=%q, path=%q, active=%v", description, url, active)

	// Robust path normalization to catch duplicates even with trailing slashes or relative paths.
	cleanPath := filepath.Clean(url)
	if abs, err := filepath.Abs(cleanPath); err == nil {
		cleanPath = abs
	}

	return c.AddProviderQuery(description, cleanPath, LocalFolderProviderID, active, false)
}

// RemoveLocalFolderQuery removes a Local Folder query from the unified list.
func (c *Config) RemoveLocalFolderQuery(id string) error {
	return c.RemoveImageQuery(id)
}

// GetLocalFolderQueries returns a copy of the Local Folder queries in a thread-safe manner.
func (c *Config) GetLocalFolderQueries() []ImageQuery {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var queries []ImageQuery
	for _, q := range c.Queries {
		if q.Provider == LocalFolderProviderID {
			queries = append(queries, q)
		}
	}
	return queries
}

// AddGooglePhotosQuery adds a new Google Photos query.
func (c *Config) AddGooglePhotosQuery(description, url string, active bool) (string, error) {
	return c.AddProviderQuery(description, url, "GooglePhotos", active, false)
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

// GetShortcutsDisabled returns the hotkey disabled preference.
func (c *Config) GetShortcutsDisabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BoolWithFallback(ShortcutsDisabledPrefKey, false) // Default: active
}

// SetShortcutsDisabled sets the hotkey disabled preference.
func (c *Config) SetShortcutsDisabled(disabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetBool(ShortcutsDisabledPrefKey, disabled)
}

// GetTargetedShortcutsDisabled returns the targeted hotkey disabled preference.
func (c *Config) GetTargetedShortcutsDisabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BoolWithFallback(TargetedShortcutsDisabledPrefKey, true) // Default: disabled
}

// SetTargetedShortcutsDisabled sets the targeted hotkey disabled preference.
func (c *Config) SetTargetedShortcutsDisabled(disabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SetBool(TargetedShortcutsDisabledPrefKey, disabled)
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
	// However, we MUST NOT block the calling functions (and Fyne's RLock polling) for the duration
	// of a massive JSON marshal. So we build a serialization object while under the current lock,
	// and then marshal that object.

	// Build a clean clone for serialization to avoid copying sync primitives (copylocks)
	clone := &Config{
		WallhavenAPIKey:         c.WallhavenAPIKey,
		WallhavenUsername:       c.WallhavenUsername,
		LogLevel:                c.LogLevel,
		MaxConcurrentProcessors: c.MaxConcurrentProcessors,
		Tuning:                  c.Tuning,
		ShortcutsDisabled:       c.ShortcutsDisabled,
		WallhavenSyncEnabled:    c.WallhavenSyncEnabled,
	}

	// Sync avoidMap to AvoidSet for JSON marshaling
	clone.AvoidSet = make(map[string]bool)
	c.avoidMap.Range(func(key, value interface{}) bool {
		clone.AvoidSet[key.(string)] = true
		return true
	})

	// Deep copy slices to avoid data races when marshaling in the background
	if c.Queries != nil {
		clone.Queries = make([]ImageQuery, len(c.Queries))
		copy(clone.Queries, c.Queries)
	}

	if c.ImageQueries != nil {
		clone.ImageQueries = make([]ImageQuery, len(c.ImageQueries))
		copy(clone.ImageQueries, c.ImageQueries)
	}

	if c.PexelsQueries != nil {
		clone.PexelsQueries = make([]ImageQuery, len(c.PexelsQueries))
		copy(clone.PexelsQueries, c.PexelsQueries)
	}

	// Deep copy MonitorPauseStates map
	if c.MonitorPauseStates != nil {
		clone.MonitorPauseStates = make(map[string]bool)
		for k, v := range c.MonitorPauseStates {
			clone.MonitorPauseStates[k] = v
		}
	}

	// Fast-path: spin off the actual marshaling/saving to a goroutine so the
	// caller's defer c.mu.Unlock() executes instantly and Fyne isn't blocked!
	// UPDATE: Removing goroutine to prevent "Stale Overwrite" race conditions where
	// multiple saves race to Preferences. Synchronous save is safe and fast here.
	data, err := json.MarshalIndent(clone, "", "  ") // Use indentation for readability
	if err != nil {
		log.Printf("Error encoding config data: %v", err)
		return
	}

	// SetString might internally lock Fyne preferences, but it's safe outside our own c.mu
	c.SetString(wallhavenConfigPrefKey, string(data))
}

// IsMonitorPaused returns true if the specified monitor is paused.
func (c *Config) IsMonitorPaused(devicePath string) bool {
	if devicePath == "" {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.MonitorPauseStates == nil {
		return false
	}
	return c.MonitorPauseStates[devicePath]
}

// SetMonitorPaused sets the pause state for a specific monitor.
func (c *Config) SetMonitorPaused(devicePath string, paused bool) {
	if devicePath == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.MonitorPauseStates == nil {
		c.MonitorPauseStates = make(map[string]bool)
	}
	c.MonitorPauseStates[devicePath] = paused
	c.save()
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
	return c.AddProviderQuery(description, url, "MetMuseum", active, false)
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
	return c.AddProviderQuery(description, url, "ArtInstituteChicago", active, false)
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

// SetWallhavenSyncEnabled sets whether Wallhaven sync is enabled.
func (c *Config) SetWallhavenSyncEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	log.Debugf("Config: Setting WallhavenSyncEnabled to: %v", enabled)
	c.WallhavenSyncEnabled = enabled
	c.save()
}

// GetWallhavenSyncEnabled returns whether Wallhaven sync is enabled.
func (c *Config) GetWallhavenSyncEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.WallhavenSyncEnabled
}

// SetWallhavenUsername sets the Wallhaven username for sync.
func (c *Config) SetWallhavenUsername(username string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	log.Debugf("Config: Setting WallhavenUsername to: '%s'", username)
	c.WallhavenUsername = username
	c.save()
}

// GetWallhavenUsername returns the Wallhaven username for sync.
func (c *Config) GetWallhavenUsername() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.WallhavenUsername
}
