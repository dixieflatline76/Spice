package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Package config provides configuration management for the Wallpaper Downloader service

// Config struct to hold all configuration data
type Config struct { // You can add "//nolint:golint" here if you prefer
	APIKey    string        `json:"api_key"`
	Frequency time.Duration `json:"change_frequency"`
	ImageURLs []ImageURL    `json:"query_urls"`
}

// ImageURL struct to hold the URL of an image and whether it is active
type ImageURL struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

var (
	instance *Config
	once     sync.Once
)

// GetConfig returns the singleton instance of Config.
func GetConfig() *Config {
	once.Do(func() {
		instance = &Config{}
		// Load config from file
		if err := instance.loadFromFile(GetFilename()); err != nil {
			// Handle error, e.g., log, use defaults
			fmt.Println("Error loading config:", err)
			instance.setDefaultValues()
		}
	})
	return instance
}

// GetFilename returns the path to the user's config file
func GetFilename() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Error getting user home directory: %v", err)
	}
	return filepath.Join(homeDir, "."+strings.ToLower(ServiceName), "config.json")
}

// GetPath returns the path to the user's config directory
func GetPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Error getting user home directory: %v", err)
	}
	return filepath.Join(homeDir, "."+strings.ToLower(ServiceName))
}

// loadFromFile loads configuration from the specified file
func (c *Config) loadFromFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err // Return the error for handling in GetConfig()
	}

	err = json.Unmarshal(data, c)
	if err != nil {
		return err
	}

	return nil
}

// setDefaultValues sets default values for the configuration
func (c *Config) setDefaultValues() {
	c.APIKey = ""
	c.Frequency = 30 * time.Minute
	// ... set other defaults
}

// Save saves the current configuration to the user's config file
func (c *Config) Save() {
	cfgFile := GetFilename()
	err := os.MkdirAll(filepath.Dir(cfgFile), 0700) // Ensure the directory exists
	if err != nil {
		log.Fatalf("Error creating config directory: %v", err)
	}

	data, err := json.MarshalIndent(c, "", "  ") // Use indentation for readability
	if err != nil {
		log.Fatalf("Error encoding config data: %v", err)
	}

	err = os.WriteFile(cfgFile, data, 0644) // Use appropriate file permissions
	if err != nil {
		log.Fatalf("Error writing config file: %v", err)
	}
}
