package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/asset"
)

// Package config provides configuration management for the Wallpaper Downloader service

// wallhavenConfigPrefKey is the string key use for saving and retrieving wallhaven image queries to fyne preferences
const wallhavenConfigPrefKey = "wallhaven_image_queries"

// Config struct to hold all configuration data
type Config struct { //nolint:golint"
	fyne.Preferences
	ImageQueries []ImageQuery `json:"query_urls"`
	assetMgr     *asset.Manager
}

// ImageQuery struct to hold the URL of an image and whether it is active
type ImageQuery struct {
	Description string `json:"desc"`
	URL         string `json:"url"`
	Active      bool   `json:"active"`
}

var (
	instance *Config
	once     sync.Once
)

// GetConfig returns the singleton instance of Config.
func GetConfig(p fyne.Preferences) *Config {
	once.Do(func() {
		instance = &Config{
			Preferences: p,
		}
		// Load config from file
		if err := instance.loadFromPrefs(); err != nil {
			// Handle error, e.g., log, use defaults
			fmt.Println("Error loading config:", err)
		}
	})
	return instance
}

// GetPath returns the path to the user's config directory
func GetPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Error getting user home directory: %v", err)
	}
	return filepath.Join(homeDir, "."+strings.ToLower(ServiceName))
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

// Save saves the current configuration to the user's config file
func (c *Config) save() {
	data, err := json.MarshalIndent(c, "", "  ") // Use indentation for readability
	if err != nil {
		log.Fatalf("Error encoding config data: %v", err)
	}

	c.SetString(wallhavenConfigPrefKey, string(data))
}
