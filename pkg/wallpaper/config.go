package wallpaper

import (
	"encoding/json"
	"fmt"
	"os"
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
type Config struct { //nolint:golint"
	fyne.Preferences
	ImageQueries []ImageQuery    `json:"query_urls"` // List of image queries
	assetMgr     *asset.Manager  // Asset manager
	AvoidSet     map[string]bool `json:"avoid_set"` // Set of image URLs to avoid
}

// ImageQuery struct to hold the URL of an image and whether it is active
type ImageQuery struct {
	Description string `json:"desc"`
	URL         string `json:"url"`
	Active      bool   `json:"active"`
}

var (
	cfgInstance *Config
	cfgOnce     sync.Once
)

// GetConfig returns the singleton instance of Config.
func GetConfig(p fyne.Preferences) *Config {
	cfgOnce.Do(func() {
		cfgInstance = &Config{
			Preferences:  p,
			ImageQueries: make([]ImageQuery, 0),
			assetMgr:     asset.NewManager(),
			AvoidSet:     make(map[string]bool),
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
	defaultCfg, err := c.assetMgr.GetText("default_config.json")
	if err != nil {
		return err
	}
	cfgText := c.StringWithFallback(wallhavenConfigPrefKey, defaultCfg)

	err = json.Unmarshal([]byte(cfgText), c)
	if err != nil {
		return err
	}

	return nil
}

// AddImageQuery adds a new image query to the end of the list
func (c *Config) AddImageQuery(desc, url string, active bool) {
	newItem := ImageQuery{desc, url, active}
	c.ImageQueries = append([]ImageQuery{newItem}, c.ImageQueries...)
	c.save()
}

// RemoveImageQuery removes the image query with the specified description
func (c *Config) RemoveImageQuery(index int) error {
	if index < 0 || index >= len(c.ImageQueries) {
		return fmt.Errorf("invalid query index: %d", index)
	}

	c.ImageQueries = append(c.ImageQueries[:index], c.ImageQueries[index+1:]...)
	c.save()
	return nil
}

// EnableImageQuery enables the image query with the specified description
func (c *Config) EnableImageQuery(index int) error {
	if index < 0 || index >= len(c.ImageQueries) {
		return fmt.Errorf("invalid query index: %d", index)
	}

	c.ImageQueries[index].Active = true
	c.save()
	return nil
}

// DisableImageQuery disables the image query with the specified description
func (c *Config) DisableImageQuery(index int) error {
	if index < 0 || index >= len(c.ImageQueries) {
		return fmt.Errorf("invalid query index: %d", index)
	}

	c.ImageQueries[index].Active = false
	c.save()
	return nil
}

// InAvoidSet checks if the given ID is in the avoid set
func (c *Config) InAvoidSet(id string) bool {
	_, found := c.AvoidSet[id]
	return found
}

// AddToAvoidSet adds the given ID to the avoid set
func (c *Config) AddToAvoidSet(id string) {
	c.AvoidSet[id] = true
	c.save()
}

// ResetAvoidSet clears the avoid set
func (c *Config) ResetAvoidSet() {
	c.AvoidSet = make(map[string]bool)
	c.save()
}

// GetCacheSize returns the cache size enumeration from the config, or the default value if not set or invalid
func (c *Config) GetCacheSize() CacheSize {
	return CacheSize(c.IntWithFallback(CacheSizePrefKey, int(Cache200Images))) // Default to 200 images
}

// SetCacheSize sets the cache size enumeration and saves it
func (c *Config) SetCacheSize(size CacheSize) {
	c.SetInt(CacheSizePrefKey, int(size))
	c.save()
}

// GetSmartFit returns the smart fit preference from the config.
func (c *Config) GetSmartFit() bool {
	return c.BoolWithFallback(SmartFitPrefKey, false) // Default to false
}

// SetSmartFit sets the smart fit preference.
func (c *Config) SetSmartFit(enabled bool) {
	c.SetBool(SmartFitPrefKey, enabled) // Save the preference to the config file
	c.save()
}

// GetWallpaperChangeFrequency returns the wallpaper change frequency enumeration from the config, or the default value if not set or invalid
func (c *Config) GetWallpaperChangeFrequency() Frequency {
	return Frequency(c.IntWithFallback(WallpaperChgFreqPrefKey, int(FrequencyHourly))) // Default to hourly
}

// SetWallpaperChangeFrequency sets the frequency enumeration for wallpaper changes and saves it
func (c *Config) SetWallpaperChangeFrequency(frequency Frequency) {
	c.SetInt(WallpaperChgFreqPrefKey, int(frequency))
	c.save()
}

// GetImgShuffle returns the image shuffle preference from the config.
func (c *Config) GetImgShuffle() bool {
	return c.BoolWithFallback(ImgShufflePrefKey, false)
}

// SetImgShuffle sets the image shuffle preference.
func (c *Config) SetImgShuffle(enabled bool) {
	c.SetBool(ImgShufflePrefKey, enabled)
	c.save()
}

// GetWallhavenAPIKey returns the Wallhaven API key from the config.
func (c *Config) GetWallhavenAPIKey() string {
	apiKey, err := keyring.Get(serviceName, WallhavenAPIKeyPrefKey) // Try to get the API key from the keyring
	if err != nil {
		log.Printf("failed to retrieve Wallhaven API key from keyring: %v", err)
		return "" // Return an empty string if the keyring lookup fails
	}
	return apiKey // Return the API key from the keyring
}

// SetWallhavenAPIKey sets the Wallhaven API key.
func (c *Config) SetWallhavenAPIKey(apiKey string) {
	keyring.Set(serviceName, WallhavenAPIKeyPrefKey, apiKey) // Save the API key to the keyring
	c.save()
}

// Save saves the current configuration to the user's config file
func (c *Config) save() {
	data, err := json.MarshalIndent(c, "", "  ") // Use indentation for readability
	if err != nil {
		log.Fatalf("Error encoding config data: %v", err)
	}

	c.SetString(wallhavenConfigPrefKey, string(data))
}
