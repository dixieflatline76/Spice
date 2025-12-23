package asset

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	_ "image/png" // Register PNG decoder

	"fyne.io/fyne/v2"

	"github.com/dixieflatline76/Spice/util/log"
)

//go:embed images/* icons/* text/* models/*
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

// GetRawImage loads and returns the raw bytes of an embedded image asset by name.
func (am *Manager) GetRawImage(name string) ([]byte, error) {
	return assets.ReadFile("images/" + name)
}

// GetIcon loads and returns embedded icon asset by name.
func (am *Manager) GetIcon(name string) (fyne.Resource, error) {
	if name == "" {
		return nil, fmt.Errorf("icon name is empty")
	}

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

// GetModel loads and returns embedded model asset by name.
func (am *Manager) GetModel(name string) ([]byte, error) {
	modelData, err := assets.ReadFile("models/" + name)
	if err != nil {
		log.Println("Error loading model:", err)
		return nil, err
	}
	return modelData, nil
}
