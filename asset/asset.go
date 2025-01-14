package asset

import (
	"bytes"
	"embed"
	"image"
	_ "image/png" // Register PNG decoder
	"log"

	"fyne.io/fyne/v2"
)

//go:embed images/* icons/* text/*
var assets embed.FS

// Manager manages the loading of UI assets.
type Manager struct{}

// NewManager creates a new asset manager.
func NewManager() *Manager {
	return &Manager{}
}

// GetImage loads and returns embedded image asset by name.
func (am *Manager) GetImage(name string) (image.Image, error) {
	splashData, err := assets.ReadFile("images/" + name)
	if err != nil {
		log.Println("Error loading image:", err)
		return nil, err
	}

	img, _, err := image.Decode(bytes.NewReader(splashData))
	if err != nil {
		log.Println("Error decoding image:", err)
		return nil, err
	}

	return img, nil
}

// GetIcon loads and returns embedded icon asset by name.
func (am *Manager) GetIcon(name string) (fyne.Resource, error) {
	iconData, err := assets.ReadFile("icons/" + name)
	if err != nil {
		log.Println("Error loading icon:", err)
		return nil, err
	}

	return fyne.NewStaticResource(name, iconData), nil
}

// GetText loads and returns embedded text asset by name.
func (am *Manager) GetText(name string) (string, error) {
	textBytes, err := assets.ReadFile("text/" + name)
	if err != nil {
		log.Println("Error loading text:", err)
		return "", err
	}
	return string(textBytes), nil
}
