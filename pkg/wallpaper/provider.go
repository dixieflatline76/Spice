package wallpaper

import "context"

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
	// ParseURL checks if the given web URL is valid for this provider and returns the API URL.
	// It returns an error if the URL is invalid.
	ParseURL(webURL string) (string, error)
	// FetchImages fetches images from the provider using the given API URL and page number.
	FetchImages(ctx context.Context, apiURL string, page int) ([]Image, error)
	// EnrichImage fetches additional details for the image (e.g. attribution) if missing.
	EnrichImage(ctx context.Context, img Image) (Image, error)
}

// ResolutionAwareProvider is an optional interface for providers that can filter images based on screen resolution.
type ResolutionAwareProvider interface {
	ImageProvider
	// WithResolution returns a new API URL with resolution constraints added if they are missing.
	WithResolution(apiURL string, width, height int) string
}
