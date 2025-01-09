package ui

import (
	"embed"
	"bytes"
	"image"

	"fyne.io/fyne/v2"
)

//go:embed assets/images/*.png
var assets embed.FS

// AssetManager manages the loading of UI assets.
type AssetManager struct{}

// NewAssetManager creates a new AssetManager instance.
func NewAssetManager() *AssetManager {
	return &AssetManager{}
}

// GetSplashImage loads and returns the splash image.
func (am *AssetManager) GetSplashImage() (image.Image, error) {
	splashData, err := assets.ReadFile("assets/images/splash.png")
	if err != nil {
		return nil, err
	}

	img, _, err := image.Decode(bytes.NewReader(splashData))
	if err != nil {
		return nil, err
	}

	return img, nil
}

// GetAppIcon loads and returns the application icon.
func (am *AssetManager) GetAppIcon() (fyne.Resource, error) {
	iconData, err := assets.ReadFile("assets/icons/app-icon.png")
	if err != nil {
		return nil, err
	}

	return fyne.NewStaticResource("app-icon.png", iconData), nil
}
