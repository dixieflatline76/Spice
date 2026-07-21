package artic

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"

	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/dixieflatline76/Spice/v2/pkg/curation"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

//go:embed artic.png
var iconData []byte

// Config interface for the provider
type Config interface {
	GetImgShuffle() bool
	AddArtInstituteChicagoQuery(desc, url string, active bool) (string, error)
	GetArtInstituteChicagoQueries() []wallpaper.ImageQuery
	EnableArtInstituteChicagoQuery(id string) error
	DisableArtInstituteChicagoQuery(id string) error
	EnableImageQuery(id string) error
	DisableImageQuery(id string) error
	RemoveImageQuery(id string) error
	GetMuseumFraming(providerID string) bool
	SetMuseumFraming(providerID string, enabled bool)
}

type Provider struct {
	cfg        *wallpaper.Config
	httpClient *http.Client

	idCache   map[string][]int
	idCacheMu sync.RWMutex
}

var aicRateLimitMu sync.Mutex // AIC requires single-threaded access for scraping

type aicSerializedRoundTripper struct {
	next http.RoundTripper
}

func (t *aicSerializedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	aicRateLimitMu.Lock()
	// No defer Unlock here! We unlock when the body is closed.

	// Politeness delay
	select {
	case <-req.Context().Done():
		aicRateLimitMu.Unlock()
		return nil, req.Context().Err()
	case <-time.After(1500 * time.Millisecond):
	}

	resp, err := t.next.RoundTrip(req)
	if err != nil {
		aicRateLimitMu.Unlock()
		return nil, err
	}

	// Wrap body to unlock on Close
	resp.Body = &aicLockedBody{
		ReadCloser: resp.Body,
		mu:         &aicRateLimitMu,
	}

	return resp, nil
}

type aicLockedBody struct {
	io.ReadCloser
	mu     *sync.Mutex
	closed bool
}

func (b *aicLockedBody) Close() error {
	if b.closed {
		return b.ReadCloser.Close()
	}
	err := b.ReadCloser.Close()
	b.mu.Unlock()
	b.closed = true
	return err
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

// NewProvider creates a new AIC provider.
func NewProvider(cfg *wallpaper.Config, httpClient *http.Client) *Provider {
	// We wrap the provided client with our strict serialization logic
	next := httpClient.Transport
	if next == nil {
		next = http.DefaultTransport
	}

	serializedClient := &http.Client{
		// Increase timeout to 5 minutes (300s) to allow for queueing delays.
		// Since requests are serialized and delayed by 1.5s, a backed-up queue can take minutes.
		Timeout: 300 * time.Second,
		Transport: &aicSerializedRoundTripper{
			next: next,
		},
	}

	p := &Provider{
		cfg:        cfg,
		httpClient: serializedClient,
		idCache:    make(map[string][]int),
	}
	return p
}

func (p *Provider) ID() string {
	return "ArtInstituteChicago"
}

func (p *Provider) Name() string {
	return i18n.T("Art Institute of Chicago")
}

func (p *Provider) Title() string {
	return ProviderTitle
}

// GetAPIPacing implements the PacedProvider interface.
func (p *Provider) GetAPIPacing() time.Duration {
	return 1500 * time.Millisecond // Aligning with aicSerializedRoundTripper delay
}

// GetProcessPacing implements the PacedProvider interface.
func (p *Provider) GetProcessPacing() time.Duration {
	return 1500 * time.Millisecond
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

func (p *Provider) SupportsUserQueries() bool {
	return false
}

func (p *Provider) HomeURL() string {
	return "https://www.artic.edu"
}

// WithResolution implements ResolutionAwareProvider to dynamically scale IIIF images.
func (p *Provider) WithResolution(apiURL string, width, height int) string {
	// If it's a IIIF URL, we can adjust the size parameter
	// Format: .../full/!1920,1080/0/default.jpg -> .../full/!W,H/0/default.jpg
	if strings.Contains(apiURL, "/iiif/2/") && strings.Contains(apiURL, "/full/!") {
		// Replace the size part
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

// ParseURL transforms a web URL into an internal identifier.
func (p *Provider) ParseURL(webURL string) (string, error) {
	matches := ObjectURLRegex.FindStringSubmatch(webURL)
	if len(matches) > 1 {
		return fmt.Sprintf("object:%s", matches[1]), nil
	}
	return "", fmt.Errorf("URL not supported: %s", webURL)
}

func (p *Provider) FetchImages(ctx context.Context, query string, page int) ([]provider.Image, error) {
	ids, err := p.resolveQueryToIDs(ctx, query)
	if err != nil {
		return nil, err
	}

	// Pagination logic
	const pageSize = 10
	start := (page - 1) * pageSize
	if start >= len(ids) {
		return nil, nil // End of list
	}
	end := start + pageSize
	if end > len(ids) {
		end = len(ids)
	}
	batch := ids[start:end]

	var images []provider.Image

	for _, id := range batch {
		img, err := p.fetchArtworkDetails(ctx, id)
		if err != nil {
			log.Printf("AIC: Error fetching artwork %d: %v", id, err)
			continue // Don't fail the whole batch
		}
		if img != nil {
			images = append(images, *img)
		}
	}

	return images, nil
}

func (p *Provider) resolveQueryToIDs(ctx context.Context, query string) ([]int, error) {
	p.idCacheMu.RLock()
	if cached, ok := p.idCache[query]; ok {
		p.idCacheMu.RUnlock()
		return cached, nil
	}
	p.idCacheMu.RUnlock()

	var ids []int

	// Case 1: Curated Tour
	found := false
	if entry := curation.GetManager().GetEntry(p.ID(), query); entry != nil && entry.Type == "curated" {
		for _, strID := range entry.IDs {
			if id, err := strconv.Atoi(strID); err == nil {
				ids = append(ids, id)
			}
		}
		found = true
	}

	if found {
		// Processed
	} else if strings.HasPrefix(query, "object:") {
		// Case 2: Direct Object
		idStr := strings.TrimPrefix(query, "object:")
		var id int
		if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
			ids = []int{id}
		}
	} else {
		// Case 3: Search
		var err error
		ids, err = p.searchArtworkIDs(ctx, query)
		if err != nil {
			log.Printf("AIC: Search failed for %s: %v, falling back to highlights", query, err)
			if entry := curation.GetManager().GetEntry(p.ID(), CollectionHighlights); entry != nil {
				for _, strID := range entry.IDs {
					if id, err := strconv.Atoi(strID); err == nil {
						ids = append(ids, id)
					}
				}
			}
		}
	}

	// Stability: Sort -> Shuffle -> Cache
	if len(ids) > 0 {
		idsCopy := make([]int, len(ids))
		copy(idsCopy, ids)

		ids = idsCopy
	}

	p.idCacheMu.Lock()
	p.idCache[query] = ids
	p.idCacheMu.Unlock()

	return ids, nil
}

func (p *Provider) fetchArtworkDetails(ctx context.Context, id int) (*provider.Image, error) {
	url := fmt.Sprintf("%s/artworks/%d?fields=id,title,artist_display,image_id,thumbnail", APIBaseURL, id)
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
		Data struct {
			ID            int    `json:"id"`
			Title         string `json:"title"`
			ArtistDisplay string `json:"artist_display"`
			DateDisplay   string `json:"date_display"`
			ImageID       string `json:"image_id"`
			Thumbnail     struct {
				Width  int `json:"width"`
				Height int `json:"height"`
			} `json:"thumbnail"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Data.ImageID == "" {
		return nil, nil // No image available
	}

	// Use IIIF for high-res image
	// We use 4K (4096x4096) as our "Ultra-Premium" target to ensure it looks great on all monitors.
	// The downloader can further refine this via WithResolution if a specific screen size is known.
	imgURL := getIIIFURL(result.Data.ImageID, 4096, 4096)

	return &provider.Image{
		ID:          fmt.Sprintf("%d", result.Data.ID),
		Path:        imgURL,
		ViewURL:     fmt.Sprintf("https://www.artic.edu/artworks/%d", result.Data.ID),
		Attribution: result.Data.ArtistDisplay,
		Title:       result.Data.Title,
		Artist:      result.Data.ArtistDisplay,
		Year:        result.Data.DateDisplay,
		Provider:    p.ID(),
		FileType:    "image/jpeg",
	}, nil
}

func (p *Provider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil // No enrichment needed for AIC currently
}

func (p *Provider) GetDownloadHeaders() map[string]string {
	return map[string]string{
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"AIC-User-Agent":  "SpiceWallpaper (spice@dixieflatline76.com)",
		"Accept":          "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.9",
		"Referer":         "https://www.artic.edu/",
		"Sec-Fetch-Dest":  "image",
		"Sec-Fetch-Mode":  "no-cors",
		"Sec-Fetch-Site":  "same-site",
	}
}

func (p *Provider) searchArtworkIDs(ctx context.Context, query string) ([]int, error) {
	searchURL := fmt.Sprintf("%s/artworks/search?q=%s&fields=id,thumbnail&limit=100", APIBaseURL, strings.ReplaceAll(query, " ", "%20"))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search API failed: %s", resp.Status)
	}

	var result struct {
		Data []struct {
			ID        int `json:"id"`
			Thumbnail struct {
				Width  int `json:"width"`
				Height int `json:"height"`
			} `json:"thumbnail"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var ids []int
	for _, item := range result.Data {
		ids = append(ids, item.ID)
	}

	return ids, nil
}

// getIIIFURL constructs a dynamic resizing URL using the IIIF Image API.
func getIIIFURL(imageID string, width, height int) string {
	// Format: {scheme}://{server}{/prefix}/{identifier}/{region}/{size}/{rotation}/{quality}.{format}
	// We use full region, and !w,h for "fit within"
	return fmt.Sprintf("https://www.artic.edu/iiif/2/%s/full/!%d,%d/0/default.jpg", imageID, width, height)
}

// FetchThumbnails implements provider.ThumbnailProvider.
func (p *Provider) FetchThumbnails(ctx context.Context, ids []string) ([]provider.Thumbnail, error) {
	thumbnails := make([]provider.Thumbnail, len(ids))
	var wg sync.WaitGroup
	limiter := rate.NewLimiter(rate.Every(p.GetAPIPacing()), 1)

	for i, id := range ids {
		wg.Add(1)
		go func(index int, artworkID string) {
			defer wg.Done()
			_ = limiter.Wait(ctx)
			var artID int
			if _, err := fmt.Sscanf(artworkID, "%d", &artID); err != nil {
				return
			}
			img, err := p.fetchArtworkDetails(ctx, artID)
			if err != nil {
				log.Printf("AIC: Failed to fetch %s for thumbnails: %v", artworkID, err)
				return
			}
			if img != nil && img.Path != "" {
				thumbnails[index] = provider.Thumbnail{
					ID:      artworkID,
					URL:     p.WithResolution(img.Path, 800, 800),
					ViewURL: img.ViewURL,
					Title:   img.Title,
					Artist:  img.Artist,
					Year:    img.Year,
				}
			}
		}(i, id)
	}
	wg.Wait()

	var validThumbnails []provider.Thumbnail
	for _, t := range thumbnails {
		if t.URL != "" {
			validThumbnails = append(validThumbnails, t)
		}
	}
	return validThumbnails, nil
}

// --- UI Implementation (Pure Go) ---

// CreateSettingsPanel returns the declarative UI for ArtIC settings.
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
	return schema.CreateMuseumSettingsPanel(schema.MuseumSettingsConfig{
		MuseumFramingGetFunc: func() bool { return p.cfg.GetMuseumFraming(p.ID()) },
		MuseumFramingSetFunc: func(val bool) { p.cfg.SetMuseumFraming(p.ID(), val) },
		ID:                   "AIC",
		Title:                i18n.T("Art Institute of Chicago"),
		Location:             i18n.T("Chicago, IL, USA"),
		LicenseURL:           "https://www.artic.edu/open-access/open-access-images",
		Description:          i18n.T("One of the world's great art museums, housing icons like Nighthawks and American Gothic."),
		MapQuery:             "Art Institute of Chicago",
		WebsiteURL:           "https://www.artic.edu",
		DonateURL:            "https://www.artic.edu/support-us",
	}, sm.OpenURL)
}

func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, _ string) *schema.PanelSchema {
	return wallpaper.CreateCuratedQueryPanel(p, sm, p.cfg)
}
