package smk

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

//go:embed smk.png
var iconData []byte

//go:embed smk.json
var embeddedCollection []byte

const (
	ProviderName         = "smk"
	ProviderTitle        = "SMK"
	APIBaseURL           = "https://api.smk.dk/api/v1/art"
	CollectionHighlights = "Best of SMK"
)

// Config interface for the provider
type Config interface {
	GetImgShuffle() bool
	AddStatensMuseumForKunstQuery(desc, url string, active bool) (string, error)
	GetStatensMuseumForKunstQueries() []wallpaper.ImageQuery
	EnableStatensMuseumForKunstQuery(id string) error
	DisableStatensMuseumForKunstQuery(id string) error
	EnableImageQuery(id string) error
	DisableImageQuery(id string) error
	RemoveImageQuery(id string) error
	GetMuseumFraming(providerID string) bool
	SetMuseumFraming(providerID string, enabled bool)
}

// Provider implements the Statens Museum for Kunst image provider.
type Provider struct {
	cfg         Config
	httpClient  *http.Client
	curatedList struct {
		Version     string `json:"version"`
		Description string `json:"description"`
		Entries     []struct {
			Key              string            `json:"key"`
			Name             string            `json:"name"`
			NameTranslations map[string]string `json:"name_translations,omitempty"`
			IDs              []string          `json:"ids"`
		} `json:"collections"`
	}
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

// NewProvider creates a new SMK provider.
func NewProvider(cfg Config, httpClient *http.Client) *Provider {
	p := &Provider{
		cfg:        cfg,
		httpClient: httpClient,
	}

	// Load the embedded JSON configuration
	if err := json.Unmarshal(embeddedCollection, &p.curatedList); err != nil {
		log.Printf("SMK: Failed to parse embedded collection: %v", err)
	}

	return p
}

func (p *Provider) ID() string {
	return "StatensMuseumForKunst"
}

func (p *Provider) Name() string {
	return i18n.T("Statens Museum for Kunst")
}

func (p *Provider) Title() string {
	return ProviderTitle
}

func (p *Provider) GetProviderIcon() interface{} {
	return iconData
}

func (p *Provider) GetClient() *http.Client {
	return p.httpClient
}

func (p *Provider) Type() provider.ProviderType {
	return provider.TypeMuseum
}

func (p *Provider) GetAttributionType() provider.AttributionType {
	return provider.AttributionBy
}

func (p *Provider) HomeURL() string {
	return "https://www.smk.dk/en/"
}

func (p *Provider) SupportsUserQueries() bool {
	return false
}

func (p *Provider) ParseURL(webURL string) (string, error) {
	return "", fmt.Errorf("user queries not supported for SMK")
}

func (p *Provider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	ids, err := p.resolveQueryToIDs(apiURL)
	if err != nil {
		return nil, err
	}

	// Calculate slice bounds
	start := (page - 1) * 50
	if start >= len(ids) {
		return nil, nil // No more images
	}

	end := start + 50
	if end > len(ids) {
		end = len(ids)
	}

	batch := ids[start:end]

	var images []provider.Image

	for _, id := range batch {
		img, err := p.fetchArtworkDetails(ctx, id)
		if err != nil {
			log.Printf("SMK: Error fetching artwork %s: %v", id, err)
			continue
		}
		if img != nil {
			images = append(images, *img)
		}
	}

	return images, nil
}

func (p *Provider) resolveQueryToIDs(query string) ([]string, error) {
	for _, entry := range p.curatedList.Entries {
		if entry.Key == query {
			ids := make([]string, len(entry.IDs))
			copy(ids, entry.IDs)
			return ids, nil
		}
	}

	// Fallback to highlights if query is not found
	for _, entry := range p.curatedList.Entries {
		if entry.Key == CollectionHighlights {
			ids := make([]string, len(entry.IDs))
			copy(ids, entry.IDs)
			return ids, nil
		}
	}

	return nil, fmt.Errorf("query not found and fallback unavailable: %s", query)
}

func (p *Provider) fetchArtworkDetails(ctx context.Context, id string) (*provider.Image, error) {
	url := fmt.Sprintf("%s/?object_number=%s", APIBaseURL, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Items []struct {
			Titles []struct {
				Title    string `json:"title"`
				Language string `json:"language"`
			} `json:"titles"`
			Artist         []string `json:"artist"`
			ImageThumbnail string   `json:"image_thumbnail"`
			ImageNative    string   `json:"image_native"`
			FrontendURL    string   `json:"frontend_url"`
			Colors         []string `json:"colors"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Items) == 0 {
		return nil, fmt.Errorf("no item found for id %s", id)
	}

	item := result.Items[0]

	if item.ImageNative == "" && item.ImageThumbnail == "" {
		return nil, nil // No image available
	}

	// Prefer native resolution, fallback to thumbnail URL.
	imgURL := item.ImageNative
	if imgURL == "" {
		imgURL = item.ImageThumbnail
	}

	title := "Unknown"
	if len(item.Titles) > 0 {
		title = item.Titles[0].Title
		for _, t := range item.Titles {
			if t.Language == "en" || t.Language == "engelsk" {
				title = t.Title
			}
		}
	}

	artist := "Unknown"
	if len(item.Artist) > 0 {
		artist = item.Artist[0]
	}

	attribution := fmt.Sprintf("%s, %s", title, artist)

	// Since SMK gives IIIF links for native resolution, they might look like:
	// https://iip.smk.dk/iiif/jp2/KMS...jp2/full/!2048,2048/0/default.jpg
	// We can leave it as is, or we can use WithResolution if needed later.

	viewURL := item.FrontendURL
	if viewURL == "" {
		viewURL = fmt.Sprintf("https://open.smk.dk/artwork/image/%s", id)
	}

	return &provider.Image{
		ID:          id,
		Path:        imgURL,
		ViewURL:     viewURL,
		Attribution: attribution,
		Provider:    p.ID(),
		FileType:    "image/jpeg",
	}, nil
}

// WithResolution implements ResolutionAwareProvider to dynamically scale IIIF images.
func (p *Provider) WithResolution(apiURL string, width, height int) string {
	if strings.Contains(apiURL, "/iiif/") && strings.Contains(apiURL, "/full/!") {
		parts := strings.Split(apiURL, "/full/!")
		if len(parts) == 2 {
			subParts := strings.Split(parts[1], "/0/default.jpg")
			if len(subParts) == 2 {
				return fmt.Sprintf("%s/full/!%d,%d/0/default.jpg", parts[0], width, height)
			}
		}
	}
	return apiURL
}

func (p *Provider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

func (p *Provider) GetDownloadHeaders() map[string]string {
	return map[string]string{
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Accept":          "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.9",
	}
}

// --- UI Implementation (Pure Go) ---

// CreateSettingsPanel returns the declarative UI for SMK settings.
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
	return schema.CreateMuseumSettingsPanel(schema.MuseumSettingsConfig{
		MuseumFramingGetFunc: func() bool { return p.cfg.GetMuseumFraming(p.ID()) },
		MuseumFramingSetFunc: func(val bool) { p.cfg.SetMuseumFraming(p.ID(), val) },
		ID:                   "StatensMuseumForKunst",
		Title:                ProviderTitle,
		Location:             i18n.T("Copenhagen, Denmark"),
		LicenseURL:           "https://www.smk.dk/en/article/smk-open/",
		Description:          i18n.T("Denmark's largest art museum, featuring outstanding collections of Danish and international art from the past seven centuries."),
		MapQuery:             "Statens Museum for Kunst",
		WebsiteURL:           "https://www.smk.dk/en/",
		DonateURL:            "https://www.smk.dk/en/article/donations/",
	}, sm.OpenURL)
}

// CreateQueryPanel creates the image query management panel.
func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, _ string) *schema.PanelSchema {
	// 1. Curated Tours Section
	var tourItems []schema.ItemSchema
	for _, entry := range p.curatedList.Entries {
		entry := entry // shadow for closure

		// Helper to find existing query state
		getQuery := func(key string) (bool, string) {
			for _, q := range p.cfg.GetStatensMuseumForKunstQueries() {
				if q.URL == key {
					return q.Active, q.ID
				}
			}
			return false, ""
		}

		active, _ := getQuery(entry.Key)

		tourItems = append(tourItems, schema.BoolItem{
			Name:         ProviderName + "_" + entry.Key,
			Label:        i18n.TMap(entry.Name, entry.NameTranslations),
			InitialValue: active,
			NeedsRefresh: true,
			ApplyFunc: func(b bool) {
				_, cid := getQuery(entry.Key)
				if b {
					if cid != "" {
						_ = p.cfg.EnableStatensMuseumForKunstQuery(cid)
					} else {
						_, _ = p.cfg.AddStatensMuseumForKunstQuery(entry.Name, entry.Key, true)
					}
				} else {
					if cid != "" {
						_ = p.cfg.DisableStatensMuseumForKunstQuery(cid)
					}
				}
			},
		})
	}

	toursSection := schema.SectionSchema{
		Title: i18n.T("Curated Tours"),
		Items: tourItems,
	}

	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{toursSection},
	}
}
