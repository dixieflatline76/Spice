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

// WallhavenProvider implements ImageProvider for Wallhaven.
type WallhavenProvider struct {
	cfg        *Config
	httpClient *http.Client
}

// NewWallhavenProvider creates a new WallhavenProvider.
func NewWallhavenProvider(cfg *Config, client *http.Client) *WallhavenProvider {
	return &WallhavenProvider{
		cfg:        cfg,
		httpClient: client,
	}
}

func (p *WallhavenProvider) Name() string {
	return "Wallhaven"
}

// ParseURL converts a web URL to an API URL.
func (p *WallhavenProvider) ParseURL(webURL string) (string, error) {
	trimmedURL := strings.TrimSpace(webURL)
	var baseURL string

	// --- Transformation and Type Detection using constants ---
	switch {
	case UserFavoritesRegex.MatchString(trimmedURL):
		matches := UserFavoritesRegex.FindStringSubmatch(trimmedURL)
		// matches[0]=Full, matches[1]=Username, matches[2]=ID
		baseURL = fmt.Sprintf(WallhavenAPICollectionURL, matches[1], matches[2])

	case SearchRegex.MatchString(trimmedURL):
		matches := SearchRegex.FindStringSubmatch(trimmedURL)
		// matches[0]=Full match, matches[1]=Base ("https://wallhaven.cc/"), matches[2]=Query part ("?q=...") or empty string
		apiSearchBase := WallhavenAPISearchURL
		if len(matches) == 3 && matches[2] != "" { // Check if query part was captured
			baseURL = apiSearchBase + matches[2] // Append the captured query part
		} else {
			baseURL = apiSearchBase // No query part found
		}

	case APICollectionRegex.MatchString(trimmedURL):
		baseURL = trimmedURL // Already API format

	case APISearchRegex.MatchString(trimmedURL):
		baseURL = trimmedURL // Already API format

	default:
		return "", fmt.Errorf("entered URL is currently not supported: %s", trimmedURL)
	}

	// --- Parameter Cleaning (for the URL to be *saved*) ---
	parsedURL, parseErr := url.Parse(baseURL)
	if parseErr != nil {
		return "", fmt.Errorf("internal error parsing transformed URL '%s': %w", baseURL, parseErr)
	}
	q := parsedURL.Query()
	paramsChanged := false
	// Clean API key (don't store in saved query)
	if q.Has("apikey") {
		q.Del("apikey")
		paramsChanged = true
	}
	// Clean page (don't store pagination in base query)
	if q.Has("page") {
		q.Del("page")
		paramsChanged = true
	}

	if paramsChanged {
		parsedURL.RawQuery = q.Encode()
	}
	return parsedURL.String(), nil
}

// WithResolution adds resolution constraints to the API URL if missing.
func (p *WallhavenProvider) WithResolution(apiURL string, width, height int) string {
	u, err := url.Parse(apiURL)
	if err != nil {
		return apiURL // Return original if parsing fails
	}

	q := u.Query()
	// Only add 'atleast' if no other resolution constraints are present
	if !q.Has("atleast") && !q.Has("resolutions") && !q.Has("ratios") {
		q.Set("atleast", fmt.Sprintf("%dx%d", width, height))
		u.RawQuery = q.Encode()
		return u.String()
	}

	return apiURL
}

// FetchImages fetches images from Wallhaven.
func (p *WallhavenProvider) FetchImages(ctx context.Context, apiURL string, page int) ([]Image, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL: %w", err)
	}

	q := u.Query()
	q.Set("apikey", p.cfg.GetWallhavenAPIKey())
	q.Set("page", fmt.Sprint(page))

	// We need desktop dimensions for default query if no resolutions/atleast specified.
	// WallhavenProvider doesn't have direct access to OS anymore.
	// We can skip this optimization or add it back if critical.
	// For now, I'll skip it to keep it simple and consistent with interface.
	// If needed, I can add `os` to `WallhavenProvider` struct.

	// Actually, let's try to keep it if possible.
	// But I don't have access to `wp.os` here.
	// I'll skip it for now. Users can specify resolutions in query if they want.
	// Or I can add `os` to `NewWallhavenProvider` later.

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from Wallhaven: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wallhaven API returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var response imgSrvcResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	var images []Image
	for _, item := range response.Data {
		log.Debugf("Wallhaven Image ID: %s, Uploader: '%s'", item.ID, item.Uploader.Username)
		images = append(images, Image{
			ID:          item.ID,
			Path:        item.Path,
			ViewURL:     item.ShortURL,
			Attribution: item.Uploader.Username,
			Provider:    p.Name(),
			FileType:    item.FileType,
		})
	}

	return images, nil
}

// EnrichImage fetches additional details for the image (e.g. attribution) if missing.
func (p *WallhavenProvider) EnrichImage(ctx context.Context, img Image) (Image, error) {
	if img.Attribution != "" {
		return img, nil // Already has attribution
	}

	// Wallhaven ID is usually the last part of the path or available in the struct
	// We use the ID from the image struct
	apiURL := fmt.Sprintf("https://wallhaven.cc/api/v1/w/%s", img.ID)

	apiKey := p.cfg.GetWallhavenAPIKey()
	if apiKey != "" {
		apiURL += "?apikey=" + apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return img, fmt.Errorf("failed to create enrichment request: %w", err)
	}
	req.Header.Set("User-Agent", "SpiceWallpaperManager/v0.1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return img, fmt.Errorf("failed to fetch enrichment data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Log but don't fail hard, just return original image
		log.Printf("Wallhaven enrichment failed for %s: status %d", img.ID, resp.StatusCode)
		return img, nil
	}

	var response struct {
		Data struct {
			Uploader struct {
				Username string `json:"username"`
			} `json:"uploader"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return img, fmt.Errorf("failed to decode enrichment response: %w", err)
	}

	if response.Data.Uploader.Username != "" {
		img.Attribution = response.Data.Uploader.Username
		log.Debugf("Enriched Wallhaven image %s with uploader: %s", img.ID, img.Attribution)
	}

	return img, nil
}

// --- Wallhaven JSON Structs ---

// imgSrvcResponse is the response from the Wallhaven API
type imgSrvcResponse struct {
	Data []ImgSrvcImage `json:"data"`
	Meta struct {
		LastPage int `json:"meta"`
	} `json:"meta"`
}

// ImgSrvcImage represents an image from the image service.
type ImgSrvcImage struct {
	Path     string   `json:"path"`
	ID       string   `json:"id"`
	ShortURL string   `json:"short_url"`
	FileType string   `json:"file_type"`
	Ratio    string   `json:"ratio"`
	Thumbs   Thumbs   `json:"thumbs"`
	Uploader Uploader `json:"uploader"`
}

// Uploader represents the user who uploaded the image.
type Uploader struct {
	Username string `json:"username"`
}

// Thumbs represents the different sizes of the image.
type Thumbs struct {
	Large    string `json:"large"`
	Original string `json:"original"`
	Small    string `json:"small"`
}

// CollectionResponse is the response from the Wallhaven API
type CollectionResponse struct {
	Data []Collection `json:"data"`
}

// Collection is the wallpaper collection
type Collection struct {
	ID     int    `json:"id"`
	Label  string `json:"label"`
	Views  int    `json:"views"`
	Public int    `json:"public"`
	Count  int    `json:"count"`
}
