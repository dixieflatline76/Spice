package provider

import (
	"context"

	"fyne.io/fyne/v2"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
)

// Image represents a generic wallpaper image.
type Image struct {
	ID               string
	Path             string // URL to download the image
	ViewURL          string // URL to view the image in browser
	FilePath         string // Local path after download (optional/computed)
	Attribution      string // Photographer or Uploader name
	Provider         string // Source provider name
	FileType         string // Content type (e.g., "image/jpeg")
	DownloadLocation string // URL to trigger download event (Unsplash requirement)
}

// ImageProvider defines the interface for an image service.
type ImageProvider interface {
	// Name returns the provider name.
	Name() string
	// HomeURL returns the home URL of the provider service.
	HomeURL() string
	// ParseURL checks if the given web URL is valid for this provider and returns the API URL.
	// It returns an error if the URL is invalid.
	ParseURL(webURL string) (string, error)
	// FetchImages fetches images from the provider using the given API URL and page number.
	FetchImages(ctx context.Context, apiURL string, page int) ([]Image, error)
	// EnrichImage fetches additional details for the image (e.g. attribution) if missing.
	EnrichImage(ctx context.Context, img Image) (Image, error)

	// --- UI Integration ---

	// Title returns the display title for the provider section (e.g., "Image Sources (Unsplash)").
	Title() string

	// GetProviderIcon returns the provider's icon for UI display (e.g. tray menu, settings header).
	// It should return a high-quality, recognizable icon, preferably 64x64 or larger.
	// Returns nil if no icon is available.
	GetProviderIcon() fyne.Resource

	// CreateSettingsPanel creates the general configuration panel (e.g., API Keys).
	// Returns nil if the provider has no general settings.
	CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject

	// CreateQueryPanel creates the image query management panel.
	// Returns nil if the provider does not support custom queries.
	CreateQueryPanel(sm setting.SettingsManager) fyne.CanvasObject
}

// ResolutionAwareProvider is an optional interface for providers that can filter images based on screen resolution.
type ResolutionAwareProvider interface {
	ImageProvider
	// WithResolution returns a new API URL with resolution constraints added if they are missing.
	WithResolution(apiURL string, width, height int) string
}

// HeaderProvider is an optional interface for providers that need custom headers for image downloads.
type HeaderProvider interface {
	GetDownloadHeaders() map[string]string
}
