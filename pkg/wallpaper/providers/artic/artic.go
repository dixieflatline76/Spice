package artic

import (
	"context"
	_ "embed" // For go:embed
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
	"golang.org/x/sync/errgroup"
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
}

// Provider implements the Art Institute of Chicago image provider.
type Provider struct {
	cfg        Config
	httpClient *http.Client

	idCache   map[string][]int
	idCacheMu sync.RWMutex

	curatedList CuratedList
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

type CuratedList struct {
	Version     int                 `json:"version"`
	Description string              `json:"description"`
	Tours       map[string]TourData `json:"tours"`
}

type TourData struct {
	Name string `json:"name"`
	IDs  []int  `json:"ids"`
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewArtInstituteChicagoProvider(cfg, client)
	})
}

// NewArtInstituteChicagoProvider creates a new AIC provider.
func NewArtInstituteChicagoProvider(cfg Config, httpClient *http.Client) *Provider {
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
	// Async init remote collection
	go func() {
		col, err := InitRemoteCollection()
		if err != nil {
			log.Printf("AIC: Failed to init remote collection: %v", err)
		} else {
			p.curatedList = *col
		}
	}()
	return p
}

func (p *Provider) Name() string {
	return ProviderName
}

func (p *Provider) Title() string {
	return ProviderTitle
}

func (p *Provider) GetClient() *http.Client {
	return p.httpClient
}

func (p *Provider) Type() provider.ProviderType {
	return provider.TypeOnline
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
	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)

	for _, id := range batch {
		id := id
		g.Go(func() error {
			img, err := p.fetchArtworkDetails(ctx, id)
			if err != nil {
				log.Printf("AIC: Error fetching artwork %d: %v", id, err)
				return nil // Don't fail the whole batch
			}
			if img != nil {
				mu.Lock()
				images = append(images, *img)
				mu.Unlock()
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
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
	if tour, ok := p.curatedList.Tours[query]; ok {
		ids = make([]int, len(tour.IDs))
		copy(ids, tour.IDs)
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
			ids = make([]int, len(p.curatedList.Tours[CollectionHighlights].IDs))
			copy(ids, p.curatedList.Tours[CollectionHighlights].IDs)
		}
	}

	// Stability: Sort -> Shuffle -> Cache
	sort.Ints(ids)
	if p.cfg.GetImgShuffle() {
		rand.Shuffle(len(ids), func(i, j int) {
			ids[i], ids[j] = ids[j], ids[i]
		})
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

	// Filter for landscape
	if !isLandscape(result.Data.Thumbnail.Width, result.Data.Thumbnail.Height) {
		return nil, nil
	}

	// Use IIIF for high-res landscape
	// We use 4K (4096x4096) as our "Ultra-Premium" target to ensure it looks great on all monitors.
	// The downloader can further refine this via WithResolution if a specific screen size is known.
	imgURL := getIIIFURL(result.Data.ImageID, 4096, 4096)

	return &provider.Image{
		ID:          fmt.Sprintf("%d", result.Data.ID),
		Path:        imgURL,
		ViewURL:     fmt.Sprintf("https://www.artic.edu/artworks/%d", result.Data.ID),
		Attribution: result.Data.ArtistDisplay,
		Provider:    ProviderName,
		FileType:    "image/jpeg",
	}, nil
}

func (p *Provider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("artic.png", iconData)
}

func (p *Provider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil // No enrichment needed for AIC currently
}

func (p *Provider) GetDownloadHeaders() map[string]string {
	return map[string]string{
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Accept":          "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.9",
		"Referer":         "https://www.artic.edu/",
		"Sec-Fetch-Dest":  "image",
		"Sec-Fetch-Mode":  "no-cors",
		"Sec-Fetch-Site":  "same-site",
	}
}

func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	header := wallpaper.CreateMuseumHeader(
		"Art Institute of Chicago",
		"Chicago, IL • USA",
		"CC0 - Public Domain",
		"https://www.artic.edu/open-access/open-access-images",
		"One of the world's great art museums, housing icons like Nighthawks and American Gothic.",
		"https://www.google.com/maps/search/?api=1&query=Art+Institute+of+Chicago",
		"https://www.artic.edu",
		"https://sales.artic.edu/donate",
		sm,
	)

	// Collection List (Deferred Save Model)
	listContainer := container.NewVBox()

	// We iterate through our curated tours
	keys := make([]string, 0, len(p.curatedList.Tours))
	for k := range p.curatedList.Tours {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Helper to find existing query state
	getDetails := func(key string) (bool, string) {
		for _, q := range p.cfg.GetArtInstituteChicagoQueries() {
			if q.URL == key {
				return q.Active, q.ID
			}
		}
		return false, ""
	}

	for _, key := range keys {
		tour := p.curatedList.Tours[key]
		key := key // shadow for closure
		active, _ := getDetails(key)

		check := widget.NewCheck(tour.Name, func(b bool) {
			// Deferred save logic
			sm.SetSettingChangedCallback(ProviderName+"_"+key, func() {
				_, cid := getDetails(key)
				if b {
					if cid != "" {
						if err := p.cfg.EnableArtInstituteChicagoQuery(cid); err != nil {
							log.Printf("AIC: Failed to enable query %s: %v", key, err)
						}
					} else {
						if _, err := p.cfg.AddArtInstituteChicagoQuery(tour.Name, key, true); err != nil {
							log.Printf("AIC: Failed to add query %s: %v", tour.Name, err)
						}
					}
				} else {
					if cid != "" {
						if err := p.cfg.DisableArtInstituteChicagoQuery(cid); err != nil {
							log.Printf("AIC: Failed to disable query %s: %v", key, err)
						}
					}
				}
			})
			sm.GetCheckAndEnableApplyFunc()()
		})
		check.Checked = active
		listContainer.Add(check)
	}

	return container.NewVBox(
		header,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Curated Tours", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		listContainer,
	)
}

func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil // No specific settings like API keys needed for AIC
}

func (p *Provider) searchArtworkIDs(ctx context.Context, query string) ([]int, error) {
	searchURL := fmt.Sprintf("%s/artworks/search?q=%s&fields=id,thumbnail&limit=100", APIBaseURL, url.QueryEscape(query))

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
		// Pre-filter for landscape if possible
		if !isLandscape(item.Thumbnail.Width, item.Thumbnail.Height) {
			continue
		}
		ids = append(ids, item.ID)
	}

	return ids, nil
}

func isLandscape(width, height int) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	ratio := float64(width) / float64(height)
	return ratio >= 1.1
}

// getIIIFURL constructs a dynamic resizing URL using the IIIF Image API.
func getIIIFURL(imageID string, width, height int) string {
	// Format: {scheme}://{server}{/prefix}/{identifier}/{region}/{size}/{rotation}/{quality}.{format}
	// We use full region, and !w,h for "fit within"
	return fmt.Sprintf("https://www.artic.edu/iiif/2/%s/full/!%d,%d/0/default.jpg", imageID, width, height)
}
