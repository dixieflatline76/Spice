package pexels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "embed"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

//go:embed Pexels.png
var iconData []byte

// Provider implements ImageProvider for Pexels.
type Provider struct {
	cfg        *wallpaper.Config
	httpClient *http.Client
	testToken  string
}

// SetTokenForTesting sets a token for testing purposes, overriding the config.
func (p *Provider) SetTokenForTesting(token string) {
	p.testToken = token
}

func init() {
	wallpaper.RegisterProvider("Pexels", func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

// NewProvider creates a new Pexels Provider.
func NewProvider(cfg *wallpaper.Config, client *http.Client) *Provider {
	return &Provider{
		cfg:        cfg,
		httpClient: client,
	}
}

func (p *Provider) ID() string {
	return "Pexels"
}

func (p *Provider) Name() string {
	return i18n.T("Pexels")
}

func (p *Provider) Title() string {
	return "Pexels"
}

func (p *Provider) GetProviderIcon() interface{} {
	return iconData
}

func (p *Provider) Type() provider.ProviderType {
	return provider.TypeCommunity
}

func (p *Provider) HomeURL() string {
	return "https://www.pexels.com"
}

func (p *Provider) GetAttributionType() provider.AttributionType {
	return provider.AttributionBy
}

func (p *Provider) SupportsUserQueries() bool {
	return true
}

// Regex patterns for Pexels URLs
var (
	// Matches /search/{query}/
	pexelsSearchRegex = regexp.MustCompile(`^https?://(?:www\.)?pexels\.com/search/([^/?#]+)/?`)
	// Matches /collections/{slug}-{id}/
	pexelsCollectionRegex = regexp.MustCompile(`^https?://(?:www\.)?pexels\.com/collections/.*-([a-zA-Z0-9]+)/?$`)
)

// ParseURL converts a web URL to an API URL.
func (p *Provider) ParseURL(webURL string) (string, error) {
	u, err := url.Parse(webURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if !strings.Contains(u.Host, "pexels.com") {
		return "", fmt.Errorf("not a Pexels URL")
	}

	// Idempotency: if it's already an API URL, return as is (but clean up if needed)
	if strings.Contains(u.Host, "api.pexels.com") {
		return webURL, nil
	}

	if pexelsSearchRegex.MatchString(webURL) {
		return parsePexelsSearchURL(webURL, u)
	}

	if pexelsCollectionRegex.MatchString(webURL) {
		matches := pexelsCollectionRegex.FindStringSubmatch(webURL)
		if len(matches) < 2 {
			return "", fmt.Errorf("could not extract collection ID from URL")
		}
		collectionID := matches[1]
		return fmt.Sprintf(wallpaper.PexelsAPICollectionURL, collectionID), nil
	}

	return "", fmt.Errorf("unsupported Pexels URL format")
}

func parsePexelsSearchURL(webURL string, u *url.URL) (string, error) {
	matches := pexelsSearchRegex.FindStringSubmatch(webURL)
	if len(matches) < 2 {
		return "", fmt.Errorf("could not extract query from search URL")
	}
	query, err := url.QueryUnescape(matches[1])
	if err != nil {
		return "", fmt.Errorf("failed to decode query from URL: %w", err)
	}

	apiURL, _ := url.Parse(wallpaper.PexelsAPISearchURL)
	q := u.Query()
	q.Set("query", query)

	// Map resolution info to Pexels 'size'
	// We support both browser (min_width/height) and normalized (res/resolution) formats
	res := q.Get("res")
	if res == "" {
		res = q.Get("resolution")
	}

	var minWidth, minHeight int
	if res != "" {
		parts := strings.Split(res, "x")
		if len(parts) == 2 {
			minWidth, _ = strconv.Atoi(parts[0])
			minHeight, _ = strconv.Atoi(parts[1])
		}
	} else {
		minWidth, _ = strconv.Atoi(q.Get("min_width"))
		minHeight, _ = strconv.Atoi(q.Get("min_height"))
	}

	if minWidth > 0 && minHeight > 0 {
		// size can be: large (24MP), medium (12MP), small (4MP)
		mp := (float64(minWidth) * float64(minHeight)) / 1000000.0
		size := "small"
		if mp > 12 {
			size = "large"
		} else if mp > 4 {
			size = "medium"
		}
		q.Set("size", size)

		// Store normalized resolution for Spice internal use
		q.Set("res", fmt.Sprintf("%dx%d", minWidth, minHeight))
	}

	// Remove pagination and web-only parameters
	q.Del("page")
	q.Del("per_page")
	q.Del("min_width")
	q.Del("min_height")
	q.Del("resolution") // Use normalized 'res'

	apiURL.RawQuery = q.Encode()
	return apiURL.String(), nil
}

// NormalizeURL converts a Pexels web URL to a more compact/standardized format for Spice.
// It combines min_width and min_height into a single 'res' parameter.
func (p *Provider) NormalizeURL(webURL string) string {
	u, err := url.Parse(webURL)
	if err != nil {
		return webURL
	}

	if !strings.Contains(u.Host, "pexels.com") || strings.Contains(u.Host, "api.pexels.com") {
		return webURL
	}

	q := u.Query()
	minW := q.Get("min_width")
	minH := q.Get("min_height")
	if minW != "" && minH != "" {
		q.Set("res", minW+"x"+minH)
		q.Del("min_width")
		q.Del("min_height")
		u.RawQuery = q.Encode()
		return u.String()
	}

	return webURL
}

// WithResolution adds resolution constraints to the Pexels API URL.
func (p *Provider) WithResolution(apiURL string, width, height int) string {
	u, err := url.Parse(apiURL)
	if err != nil {
		return apiURL
	}
	q := u.Query()

	// Only set if not already present or if we want to override
	q.Set("res", fmt.Sprintf("%dx%d", width, height))

	// Re-map size based on new resolution
	mp := (float64(width) * float64(height)) / 1000000.0
	size := "small"
	if mp > 12 {
		size = "large"
	} else if mp > 4 {
		size = "medium"
	}
	q.Set("size", size)

	u.RawQuery = q.Encode()
	return u.String()
}

// FetchImages fetches images from the Pexels API.
func (p *Provider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	// Robustness: Check if we have a web URL and convert it on the fly
	if strings.Contains(apiURL, "www.pexels.com") || strings.Contains(apiURL, "pexels.com/search") || strings.Contains(apiURL, "pexels.com/collections") {
		converted, err := p.ParseURL(apiURL)
		if err == nil {
			log.Printf("Pexels: Converted legacy Web URL to API URL: %s", converted)
			apiURL = converted
		} else {
			log.Printf("Pexels: Warning - Failed to convert URL %s: %v", apiURL, err)
		}
	}

	fullURL, err := p.buildAPIURL(apiURL, page)
	if err != nil {
		return nil, err
	}

	resp, err := p.executeRequest(ctx, fullURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 500 {
			log.Printf("Pexels API Error (%d): %s...", resp.StatusCode, string(body[:500]))
		} else {
			log.Printf("Pexels API Error (%d): %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	images, err := p.parseResponse(resp.Body, fullURL)
	if err != nil {
		return nil, err
	}

	if len(images) == 0 {
		log.Printf("Pexels query returned 0 images for URL: %s", fullURL)
	} else {
		log.Debugf("Found %d images from Pexels", len(images))
	}

	return images, nil
}

func (p *Provider) buildAPIURL(apiURL string, page int) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", fmt.Errorf("invalid API URL: %w", err)
	}

	q := u.Query()
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", "30") // Default to 30 images
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func (p *Provider) executeRequest(ctx context.Context, fullURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	apiKey := p.testToken
	if apiKey == "" {
		apiKey = p.cfg.GetPexelsAPIKey()
	}
	if apiKey == "" {
		return nil, fmt.Errorf("pexels API key is missing")
	}
	req.Header.Set("Authorization", apiKey)

	log.Debugf("Fetching Pexels images from: %s", fullURL)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

func (p *Provider) parseResponse(body io.Reader, urlStr string) ([]provider.Image, error) {
	var images []provider.Image

	// Determine response type based on URL path
	if strings.Contains(urlStr, "/search") {
		var searchResp PexelsSearchResponse
		if err := json.NewDecoder(body).Decode(&searchResp); err != nil {
			return nil, fmt.Errorf("failed to decode search response: %w", err)
		}
		for _, photo := range searchResp.Photos {
			images = append(images, p.mapPexelsImage(photo))
		}
	} else if strings.Contains(urlStr, "/collections") {
		var collectionResp PexelsCollectionResponse
		if err := json.NewDecoder(body).Decode(&collectionResp); err != nil {
			return nil, fmt.Errorf("failed to decode collection response: %w", err)
		}
		for _, photo := range collectionResp.Media {
			if photo.Type == "Photo" { // Ensure we only get photos
				images = append(images, p.mapPexelsImage(photo))
			}
		}
	} else {
		return nil, fmt.Errorf("unknown Pexels API endpoint")
	}

	return images, nil
}

// EnrichImage is a no-op for Pexels as attribution is included in search results.
func (p *Provider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

func (p *Provider) mapPexelsImage(photo PexelsPhoto) provider.Image {
	// Prefer 'original' or 'large2x' depending on need. 'original' is best for wallpapers.
	imagePath := photo.Src.Original
	if imagePath == "" {
		imagePath = photo.Src.Large2x
	}

	return provider.Image{
		ID:          strconv.Itoa(photo.ID),
		Path:        imagePath,
		ViewURL:     photo.URL,
		Attribution: photo.Photographer,
		Provider:    p.ID(),
		FileType:    "image/jpeg", // Pexels primarily serves JPEGs
	}
}

// Pexels JSON Structures

type PexelsSearchResponse struct {
	TotalResults int           `json:"total_results"`
	Page         int           `json:"page"`
	PerPage      int           `json:"per_page"`
	Photos       []PexelsPhoto `json:"photos"`
	NextPage     string        `json:"next_page"`
}

type PexelsCollectionResponse struct {
	ID    string        `json:"id"`
	Media []PexelsPhoto `json:"media"` // Collections return 'media' list
}

type PexelsPhoto struct {
	ID           int       `json:"id"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	URL          string    `json:"url"`
	Photographer string    `json:"photographer"`
	Src          PexelsSrc `json:"src"`
	Type         string    `json:"type"` // "Photo" or "Video"
}

type PexelsSrc struct {
	Original  string `json:"original"`
	Large2x   string `json:"large2x"`
	Large     string `json:"large"`
	Medium    string `json:"medium"`
	Small     string `json:"small"`
	Portrait  string `json:"portrait"`
	Landscape string `json:"landscape"`
	Tiny      string `json:"tiny"`
}

// --- UI Implementation (Pure Go) ---

const pexAPIIdent = "pexelsAPIKey"

// CreateSettingsPanel returns the declarative UI for Pexels settings.
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title:   i18n.T("Pexels"),
				Compact: true,
				Items: []schema.ItemSchema{
					schema.LabelItem{
						Text:       i18n.T("Pexels provides high quality and completely free stock photos licensed under the Pexels license."),
						Importance: schema.ImportanceLow,
					},
					schema.SecretItem{
						Name:         pexAPIIdent,
						Label:        i18n.T("pexels API Key:"),
						InitialValue: p.cfg.GetPexelsAPIKey(),
						Placeholder:  i18n.T("Enter your Pexels API Key"),
						OnVerify: func(key string) error {
							ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
							defer cancel()
							return CheckPexelsAPIKeyWithContext(ctx, key)
						},
						ApplyFunc: func(key string) {
							p.cfg.SetPexelsAPIKey(key)
						},
						OnClear: func() {
							p.cfg.SetPexelsAPIKey("")
							sm.ResetSettings(
								setting.SettingReset{Name: pexAPIIdent, Value: ""},
							)
						},
					},
					schema.HyperlinkItem{
						Text: i18n.T("Get a free API key from Pexels."),
						URL:  "https://www.pexels.com/api/key/",
					},
				},
			},
		},
	}
}

// CreateQueryPanel creates the image query management panel.
func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) *schema.PanelSchema {
	addCfg := schema.AddQueryConfig{
		Title:           i18n.T("Add Pexels Collection"),
		URLPlaceholder:  "https://www.pexels.com/search/...",
		URLValidator:    PexelsURLRegexp,
		URLErrorMsg:     i18n.T("Invalid Pexels URL"),
		DescPlaceholder: i18n.T("Collection Description (e.g. Nature)"),
		AddHandler: func(desc, url string, active bool) (string, error) {
			apiURL, err := p.ParseURL(url)
			if err != nil {
				return "", err
			}
			return p.cfg.AddPexelsQuery(desc, apiURL, active)
		},
	}

	if pendingUrl != "" {
		sm.ShowAddQueryDialog(addCfg, pendingUrl, "", sm.RefreshUI)
	}

	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title:       i18n.T("Pexels Queries"),
				Description: i18n.T("Manage your Pexels image queries here."),
				Items: []schema.ItemSchema{
					schema.ButtonItem{
						Name:       "pexels_add",
						ButtonText: i18n.T("Add New Collection"),
						IconName:   "add",
						OnPressed: func() {
							sm.ShowAddQueryDialog(addCfg, "", "", sm.RefreshUI)
						},
					},
					schema.QueryListItem{
						GetQueries: func() []schema.Query {
							queries := p.cfg.GetPexelsQueries()
							abstracts := make([]schema.Query, len(queries))
							for i, q := range queries {
								abstracts[i] = schema.Query{
									ID:          q.ID,
									URL:         q.URL,
									Description: q.Description,
									Active:      q.Active,
									Managed:     q.Managed,
								}
							}
							return abstracts
						},
						EnableQuery:  p.cfg.EnablePexelsQuery,
						DisableQuery: p.cfg.DisablePexelsQuery,
						RemoveQuery:  p.cfg.RemovePexelsQuery,
						GetDisplayURL: func(q schema.Query) *url.URL {
							return p.getDisplayURL(q)
						},
					},
				},
			},
		},
	}
}

func (p *Provider) getDisplayURL(q schema.Query) *url.URL {
	apiURL := q.URL
	if strings.Contains(apiURL, "api.pexels.com/v1/search") {
		u, err := url.Parse(apiURL)
		if err != nil {
			return nil
		}
		query := u.Query().Get("query")
		displayURL := fmt.Sprintf("https://www.pexels.com/search/%s/", url.PathEscape(query))
		res, _ := url.Parse(displayURL)
		return res
	}
	if strings.Contains(apiURL, "api.pexels.com/v1/collections") {
		re := regexp.MustCompile(`^https?://api\.pexels\.com/v1/collections/([a-zA-Z0-9]+)`)
		matches := re.FindStringSubmatch(apiURL)
		if len(matches) >= 2 {
			displayURL := fmt.Sprintf("https://www.pexels.com/collections/%s/", matches[1])
			res, _ := url.Parse(displayURL)
			return res
		}
	}
	res, _ := url.Parse(apiURL)
	return res
}
