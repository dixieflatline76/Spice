package ui

import (
	"bytes"
	"embed"
	"image"
	_ "image/png" // Register PNG decoder

	"fyne.io/fyne/v2"
)

//go:embed assets/images/* assets/icons/*
var assets embed.FS

// AssetManager manages the loading of UI assets.
type AssetManager struct{}

// NewAssetManager creates a new AssetManager instance.
func NewAssetManager() *AssetManager {
	return &AssetManager{}
}

// GetImage loads and returns an image.
func (am *AssetManager) GetImage(name string) (image.Image, error) {
	splashData, err := assets.ReadFile("assets/images/" + name)
	if err != nil {
		return nil, err
	}

	img, _, err := image.Decode(bytes.NewReader(splashData))
	if err != nil {
		return nil, err
	}

	return img, nil
}

// GetIcon loads and returns an icon.
func (am *AssetManager) GetIcon(name string) (fyne.Resource, error) {
	iconData, err := assets.ReadFile("assets/icons/" + name)
	if err != nil {
		return nil, err
	}

	return fyne.NewStaticResource("app-icon.png", iconData), nil
}
