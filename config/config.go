package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Package config provides configuration management for the Wallpaper Downloader service

// Config struct to hold all configuration data
type Config struct {
	APIKey         string        `json:"api_key"`
	Frequency      time.Duration `json:"change_frequency"`
	ImageURLs      []ImageURL    `json:"query_urls"`
	WallpaperStyle int32         `json:"wallpaper_style"`
}

// ImageURL struct to hold the URL of an image and whether it is active
type ImageURL struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

// Cfg is the global configuration variable
var Cfg Config

// configFile returns the path to the user's config file
func configFile() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Error getting user home directory: %v", err)
	}
	return filepath.Join(homeDir, "."+strings.ToLower(ServiceName), "config.json")
}

// LoadConfig loads configuration from the user's config file
func LoadConfig() {
	cfgFile := configFile()
	log.Printf("Config file: %v", cfgFile)

	data, err := os.ReadFile(cfgFile)
	if err != nil {
		log.Printf("Error reading config file: %v", err)
		if os.IsNotExist(err) {
			// Config file doesn't exist, use defaults
			log.Println("Config file not found, using defaults")
			Cfg = Config{
				APIKey:         "",
				Frequency:      30 * time.Minute, // Default frequency
				WallpaperStyle: 10,               // Default to "fill" (Span)
			}
			SaveConfig() // Save the default config to the user's config file
			return
		}
		log.Fatalf("Error reading config file: %v", err)
	}

	err = json.Unmarshal(data, &Cfg)
	if err != nil {
		log.Fatalf("Error parsing config file: %v", err)
	}

	// If WallpaperStyle is not set in the config file, set it to the default value
	if Cfg.WallpaperStyle == 0 {
		Cfg.WallpaperStyle = 10 // Default to "fill" (Span)
		log.Printf("Failed to get wallpaper style: %v", err)
	}
}

// SaveConfig saves the current configuration to the user's config file
func SaveConfig() {
	cfgFile := configFile()
	err := os.MkdirAll(filepath.Dir(cfgFile), 0700) // Ensure the directory exists
	if err != nil {
		log.Fatalf("Error creating config directory: %v", err)
	}

	data, err := json.MarshalIndent(Cfg, "", "  ") // Use indentation for readability
	if err != nil {
		log.Fatalf("Error encoding config data: %v", err)
	}

	err = os.WriteFile(cfgFile, data, 0644) // Use appropriate file permissions
	if err != nil {
		log.Fatalf("Error writing config file: %v", err)
	}
}
