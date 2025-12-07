package wallpaper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/dixieflatline76/Spice/util/log"
)

// UnsplashProvider implements ImageProvider for Unsplash.
type UnsplashProvider struct {
	cfg        *Config
	httpClient *http.Client
	testToken  string
}

// SetTokenForTesting sets a token for testing purposes, overriding the config.
func (p *UnsplashProvider) SetTokenForTesting(token string) {
	p.testToken = token
}

// NewUnsplashProvider creates a new UnsplashProvider.
func NewUnsplashProvider(cfg *Config, client *http.Client) *UnsplashProvider {
	return &UnsplashProvider{
		cfg:        cfg,
		httpClient: client,
	}
}

func (p *UnsplashProvider) Name() string {
	return "Unsplash"
}

// ParseURL converts a web URL to an API URL.
// Supported formats:
// - https://unsplash.com/s/photos/query -> https://api.unsplash.com/search/photos?query=query
// - https://unsplash.com/collections/ID/NAME -> https://api.unsplash.com/collections/ID/photos
func (p *UnsplashProvider) ParseURL(webURL string) (string, error) {
	u, err := url.Parse(webURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if !strings.Contains(u.Host, "unsplash.com") {
		return "", fmt.Errorf("not an Unsplash URL")
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) == 0 {
		return "", fmt.Errorf("invalid Unsplash URL path")
	}

	apiURL := &url.URL{
		Scheme: "https",
		Host:   "api.unsplash.com",
	}

	q := u.Query() // Start with existing query params (e.g. orientation)

	if pathParts[0] == "s" && len(pathParts) >= 3 && pathParts[1] == "photos" {
		// Search: /s/photos/query
		apiURL.Path = "/search/photos"
		q.Set("query", pathParts[2])
	} else if pathParts[0] == "collections" && len(pathParts) >= 2 {
		// Collection: /collections/ID/NAME
		collectionID := pathParts[1]
		apiURL.Path = fmt.Sprintf("/collections/%s/photos", collectionID)
	} else {
		return "", fmt.Errorf("unsupported Unsplash URL format")
	}

	apiURL.RawQuery = q.Encode()
	return apiURL.String(), nil
}

// WithResolution adds resolution constraints to the API URL if missing.
func (p *UnsplashProvider) WithResolution(apiURL string, width, height int) string {
	u, err := url.Parse(apiURL)
	if err != nil {
		return apiURL
	}

	// Unsplash Search API supports 'orientation'
	// We only apply this for search endpoints
	if strings.Contains(u.Path, "/search/photos") {
		q := u.Query()
		if !q.Has("orientation") {
			if width > height {
				q.Set("orientation", "landscape")
			} else if height > width {
				q.Set("orientation", "portrait")
			}
			// If square, we don't set orientation (or could set 'squarish')
			u.RawQuery = q.Encode()
			return u.String()
		}
	}

	return apiURL
}

// FetchImages fetches images from the Unsplash API.
func (p *UnsplashProvider) FetchImages(ctx context.Context, apiURL string, page int) ([]Image, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL: %w", err)
	}

	q := u.Query()
	q.Set("client_id", UnsplashClientID)
	q.Set("page", fmt.Sprint(page))
	q.Set("per_page", "30") // Default to 30 images
	u.RawQuery = q.Encode()

	log.Debugf("Fetching Unsplash images from: %s", u.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	token := p.testToken
	if token == "" {
		token = p.cfg.GetUnsplashToken()
	}
	if token == "" {
		return nil, fmt.Errorf("Unsplash token is missing (disconnected)")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Unsplash API Error: %s", string(body))
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var images []Image

	// Unsplash search API returns a wrapper object, while collections API returns a list.
	// We need to detect which one it is.
	// Or we can check the path.
	if strings.Contains(u.Path, "/search/photos") {
		var searchResp UnsplashSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
			return nil, fmt.Errorf("failed to decode search response: %w", err)
		}
		for _, item := range searchResp.Results {
			images = append(images, p.mapUnsplashImage(item))
		}
	} else {
		var listResp []UnsplashImage
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			return nil, fmt.Errorf("failed to decode list response: %w", err)
		}
		for _, item := range listResp {
			images = append(images, p.mapUnsplashImage(item))
		}
	}

	if len(images) == 0 {
		log.Printf("Unsplash query returned 0 images for URL: %s", u.String())
	} else {
		log.Debugf("Found %d images from Unsplash", len(images))
	}

	return images, nil
}

// EnrichImage fetches additional details for the image (e.g. attribution) if missing.
// For Unsplash, the attribution is already present in the search results, so this is a no-op.
func (p *UnsplashProvider) EnrichImage(ctx context.Context, img Image) (Image, error) {
	return img, nil
}

func (p *UnsplashProvider) mapUnsplashImage(ui UnsplashImage) Image {
	return Image{
		ID:               ui.ID,
		Path:             ui.URLs.Full, // Use full resolution
		ViewURL:          ui.Links.HTML,
		Attribution:      ui.User.Name,
		Provider:         p.Name(),
		FileType:         "image/jpeg", // Unsplash usually serves JPEGs
		DownloadLocation: ui.Links.DownloadLocation,
	}
}

// Unsplash JSON structures

type UnsplashSearchResponse struct {
	Total      int             `json:"total"`
	TotalPages int             `json:"total_pages"`
	Results    []UnsplashImage `json:"results"`
}

type UnsplashImage struct {
	ID    string `json:"id"`
	URLs  URLs   `json:"urls"`
	Links Links  `json:"links"`
	User  User   `json:"user"`
}

type URLs struct {
	Raw     string `json:"raw"`
	Full    string `json:"full"`
	Regular string `json:"regular"`
	Small   string `json:"small"`
	Thumb   string `json:"thumb"`
}

type Links struct {
	Self             string `json:"self"`
	HTML             string `json:"html"`
	Download         string `json:"download"`
	DownloadLocation string `json:"download_location"`
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}
