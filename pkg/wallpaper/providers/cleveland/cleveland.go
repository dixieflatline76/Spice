package cleveland

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// Provider implements the Cleveland Museum of Art wallpaper provider.
// Uses the Open Access API — no authentication required, direct CDN image URLs.
type Provider struct {
	cfg    *wallpaper.Config
	client *http.Client
	mu     sync.RWMutex

	collection *Collection

	// Cache for search result IDs (stable pagination)
	idCache   map[string][]int
	idCacheMu sync.RWMutex

	// Cache for resolved image details
	poolCache   map[int]*provider.Image
	poolCacheMu sync.RWMutex
}

// API response types

type apiResponse struct {
	Info apiInfo      `json:"info"`
	Data []apiArtwork `json:"data"`
}

type apiSingleResponse struct {
	Data apiArtwork `json:"data"`
}

type apiInfo struct {
	Total int `json:"total"`
}

type apiArtwork struct {
	ID              int          `json:"id"`
	AccessionNumber string       `json:"accession_number"`
	Title           string       `json:"title"`
	URL             string       `json:"url"`
	Images          *apiImages   `json:"images"`
	Creators        []apiCreator `json:"creators"`
}

type apiImages struct {
	Web   *apiImageSize `json:"web"`
	Print *apiImageSize `json:"print"`
}

type apiImageSize struct {
	URL    string `json:"url"`
	Width  string `json:"width"`
	Height string `json:"height"`
}

type apiCreator struct {
	Description string `json:"description"`
	Role        string `json:"role"`
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

// NewProvider creates a new Cleveland Museum provider.
func NewProvider(cfg *wallpaper.Config, client *http.Client) *Provider {
	p := &Provider{
		cfg:       cfg,
		client:    client,
		idCache:   make(map[string][]int),
		poolCache: make(map[int]*provider.Image),
	}
	go func() {
		col, err := InitRemoteCollection(cfg)
		if err != nil {
			log.Printf("Cleveland: Failed to init remote collection: %v", err)
		} else {
			p.mu.Lock()
			p.collection = col
			p.mu.Unlock()
		}
	}()
	return p
}

// SyncRemoteConfig fetches the latest curated collections from GitHub.
func (p *Provider) SyncRemoteConfig() error {
	col, err := RefreshRemoteCollection()
	if err != nil {
		return err
	}
	if col != nil {
		p.mu.Lock()
		p.collection = col
		p.mu.Unlock()
	}
	return nil
}

var _ provider.RemoteConfigSyncer = (*Provider)(nil)

func (p *Provider) ID() string      { return ProviderName }
func (p *Provider) HomeURL() string { return WebBaseURL }

func (p *Provider) Name() string {
	return i18n.T("Cleveland Museum of Art")
}

func (p *Provider) Title() string { return ProviderTitle }

func (p *Provider) GetProviderIcon() interface{} { return iconData }

func (p *Provider) Type() provider.ProviderType {
	return provider.TypeMuseum
}

func (p *Provider) GetAttributionType() provider.AttributionType {
	return provider.AttributionBy
}

func (p *Provider) SupportsUserQueries() bool { return false }

//go:embed Cleveland.png
var iconData []byte

// GetAPIPacing implements PacedProvider.
func (p *Provider) GetAPIPacing() time.Duration { return 500 * time.Millisecond }

// GetProcessPacing implements PacedProvider.
func (p *Provider) GetProcessPacing() time.Duration { return 1500 * time.Millisecond }

// ParseURL checks if a URL matches a Cleveland Museum object page.
func (p *Provider) ParseURL(url string) (string, error) {
	matches := ObjectURLRegex.FindStringSubmatch(url)
	if len(matches) > 1 {
		return "object:" + matches[1], nil
	}

	lower := strings.ToLower(url)
	if strings.Contains(lower, "://") || strings.HasPrefix(lower, "www.") {
		if !strings.Contains(lower, "clevelandart.org") {
			return "", fmt.Errorf("invalid Cleveland Museum URL")
		}
	}
	return url, nil
}

// FetchImages fetches wallpaper candidates.
func (p *Provider) FetchImages(ctx context.Context, query string, page int) ([]provider.Image, error) {
	// Ensure collection is loaded
	p.mu.RLock()
	col := p.collection
	p.mu.RUnlock()

	if col == nil {
		var err error
		col, err = InitRemoteCollection(p.cfg)
		if err != nil {
			return nil, fmt.Errorf("collection not loaded: %w", err)
		}
		p.mu.Lock()
		p.collection = col
		p.mu.Unlock()
	}

	// Handle direct object URL
	if strings.HasPrefix(query, "object:") {
		accNum := strings.TrimPrefix(query, "object:")
		return p.fetchByAccessionNumber(ctx, accNum)
	}

	entry := col.FindEntry(query)
	if entry == nil {
		log.Printf("Cleveland: Unknown collection key %q, falling back to masterpieces", query)
		entry = col.FindEntry(CollectionMasterpieces)
		if entry == nil {
			return nil, fmt.Errorf("no collection entries available")
		}
	}

	switch entry.Type {
	case "curated":
		return p.fetchCurated(ctx, entry, page)
	case "search":
		return p.fetchSearch(ctx, entry, page)
	default:
		return nil, fmt.Errorf("unknown collection type: %s", entry.Type)
	}
}

// fetchByAccessionNumber fetches a single artwork by its accession number.
func (p *Provider) fetchByAccessionNumber(ctx context.Context, accNum string) ([]provider.Image, error) {
	url := fmt.Sprintf("%s?accession_number=%s", APIBaseURL, accNum)
	artworks, err := p.fetchAPI(ctx, url)
	if err != nil {
		return nil, err
	}

	var images []provider.Image
	for _, art := range artworks {
		if img := p.artworkToImage(&art); img != nil {
			images = append(images, *img)
		}
	}
	return images, nil
}

// fetchCurated fetches images for curated (ID-based) collections.
// Each artwork is fetched individually via /api/artworks/{id} since the CMA API
// does not support batch ID lookups.
func (p *Provider) fetchCurated(ctx context.Context, entry *CollectionEntry, page int) ([]provider.Image, error) {
	ids := make([]int, len(entry.IDs))
	copy(ids, entry.IDs)

	if p.cfg.GetImgShuffle() {
		r := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // Not security-sensitive
		r.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	}

	const pageSize = 20
	start := (page - 1) * pageSize
	if start >= len(ids) {
		return nil, nil
	}
	end := start + pageSize
	if end > len(ids) {
		end = len(ids)
	}

	pageIDs := ids[start:end]

	var images []provider.Image
	var mu sync.Mutex

	// Fetch each artwork individually with bounded concurrency
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

	for _, id := range pageIDs {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			art, err := p.fetchSingleArtwork(ctx, id)
			if err != nil {
				log.Debugf("Cleveland: Error fetching artwork %d: %v", id, err)
				return
			}
			if img := p.artworkToImage(art); img != nil {
				mu.Lock()
				images = append(images, *img)
				mu.Unlock()
			}
		}(id)
	}

	wg.Wait()
	log.Debugf("Cleveland: Curated page %d: %d images from %d IDs", page, len(images), len(pageIDs))
	return images, nil
}

// fetchSearch queries the API with search parameters.
func (p *Provider) fetchSearch(ctx context.Context, entry *CollectionEntry, page int) ([]provider.Image, error) {
	// Check ID cache
	p.idCacheMu.RLock()
	cachedIDs, hasCached := p.idCache[entry.Key]
	p.idCacheMu.RUnlock()

	if !hasCached {
		// Fetch IDs by scanning multiple pages
		var allIDs []int
		maxPages := 5
		limit := 100

		for i := 0; i < maxPages; i++ {
			skip := i * limit
			url := fmt.Sprintf("%s?%s&limit=%d&skip=%d", APIBaseURL, entry.SearchParams, limit, skip)
			artworks, err := p.fetchAPI(ctx, url)
			if err != nil {
				if len(allIDs) > 0 {
					break
				}
				return nil, err
			}
			if len(artworks) == 0 {
				break
			}
			for _, art := range artworks {
				allIDs = append(allIDs, art.ID)
				// Pre-cache the image details while we have them
				if img := p.artworkToImage(&art); img != nil {
					p.poolCacheMu.Lock()
					p.poolCache[art.ID] = img
					p.poolCacheMu.Unlock()
				}
			}
			if len(artworks) < limit {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		sort.Ints(allIDs)
		if p.cfg.GetImgShuffle() {
			r := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // Not security-sensitive
			r.Shuffle(len(allIDs), func(i, j int) { allIDs[i], allIDs[j] = allIDs[j], allIDs[i] })
		}

		p.idCacheMu.Lock()
		p.idCache[entry.Key] = allIDs
		p.idCacheMu.Unlock()
		cachedIDs = allIDs

		log.Printf("Cleveland: Search %q resolved %d IDs", entry.Key, len(allIDs))
	}

	// Paginate from cached IDs
	const pageSize = 20
	start := (page - 1) * pageSize
	if start >= len(cachedIDs) {
		return nil, nil
	}
	end := start + pageSize
	if end > len(cachedIDs) {
		end = len(cachedIDs)
	}

	var images []provider.Image
	for _, id := range cachedIDs[start:end] {
		// Check pool cache first
		p.poolCacheMu.RLock()
		cached, ok := p.poolCache[id]
		p.poolCacheMu.RUnlock()
		if ok && cached != nil {
			images = append(images, *cached)
			continue
		}

		// Fetch individually if not cached
		art, err := p.fetchSingleArtwork(ctx, id)
		if err != nil {
			continue
		}
		if img := p.artworkToImage(art); img != nil {
			images = append(images, *img)
		}
	}

	log.Debugf("Cleveland: Search page %d: %d images from %d candidates", page, len(images), len(cachedIDs))
	return images, nil
}

// fetchSingleArtwork fetches a single artwork by its Athena ID via /api/artworks/{id}.
func (p *Provider) fetchSingleArtwork(ctx context.Context, id int) (*apiArtwork, error) {
	url := fmt.Sprintf("%s%d", APIBaseURL, id)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %s for artwork %d", resp.Status, id)
	}

	var result apiSingleResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result.Data, nil
}

// fetchAPI performs a GET request and returns the parsed artworks (list endpoint).
func (p *Provider) fetchAPI(ctx context.Context, url string) ([]apiArtwork, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %s", resp.Status)
	}

	var result apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data, nil
}

// artworkToImage converts an API artwork to a provider.Image, filtering non-landscape.
func (p *Provider) artworkToImage(art *apiArtwork) *provider.Image {
	if art.Images == nil {
		return nil
	}

	// Prefer print quality, fall back to web
	imgSize := art.Images.Print
	if imgSize == nil {
		imgSize = art.Images.Web
	}
	if imgSize == nil || imgSize.URL == "" {
		return nil
	}

	// Check dimensions for landscape orientation
	w, _ := strconv.Atoi(imgSize.Width)
	h, _ := strconv.Atoi(imgSize.Height)
	if w > 0 && h > 0 {
		ratio := float64(w) / float64(h)
		if ratio < 1.2 {
			return nil // Portrait or near-square
		}
	}

	// Build attribution
	artist := ""
	for _, c := range art.Creators {
		if c.Description != "" {
			artist = c.Description
			break
		}
	}

	attribution := art.Title
	if artist != "" {
		attribution = fmt.Sprintf("%s - %s", artist, art.Title)
	}

	viewURL := art.URL
	if viewURL == "" {
		viewURL = fmt.Sprintf("%s/art/%s", WebBaseURL, art.AccessionNumber)
	}

	img := &provider.Image{
		ID:          strconv.Itoa(art.ID),
		Path:        imgSize.URL,
		ViewURL:     viewURL,
		Attribution: attribution,
		Provider:    ProviderName,
	}

	// Cache the result
	p.poolCacheMu.Lock()
	p.poolCache[art.ID] = img
	p.poolCacheMu.Unlock()

	return img
}

// EnrichImage is a no-op — all metadata comes from the initial fetch.
func (p *Provider) EnrichImage(_ context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

// --- UI Implementation ---

// CreateSettingsPanel returns the museum info panel.
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
	return schema.CreateMuseumSettingsPanel(schema.MuseumSettingsConfig{
		ID:          "CMA",
		Title:       i18n.T("Cleveland Museum of Art"),
		Location:    i18n.T("Cleveland, OH, USA"),
		LicenseURL:  "https://www.clevelandart.org/open-access",
		Description: i18n.T("One of America's most distinguished comprehensive art museums. Its Open Access collection spans 6,000 years of achievement in art, all freely available for any use."),
		MapQuery:    "Cleveland Museum of Art",
		WebsiteURL:  WebBaseURL,
		DonateURL:   "https://www.clevelandart.org/give",
	}, sm.OpenURL)
}

// CreateQueryPanel creates the collection toggle panel.
func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, _ string) *schema.PanelSchema {
	p.mu.RLock()
	col := p.collection
	p.mu.RUnlock()

	if col == nil {
		if c, err := InitRemoteCollection(p.cfg); err == nil {
			p.mu.Lock()
			p.collection = c
			p.mu.Unlock()
			col = c
		}
	}

	var curatedItems []schema.ItemSchema
	if col != nil {
		for _, entry := range col.Entries {
			curatedItems = append(curatedItems, p.makeCollectionItem(entry.Name, entry.Key))
		}
	}

	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title: i18n.T("Museum Collections"),
				Items: curatedItems,
			},
		},
	}
}

func (p *Provider) makeCollectionItem(label, key string) schema.BoolItem {
	getQuery := func(key string) (bool, string) {
		for _, q := range p.cfg.GetQueries() {
			if q.Provider == p.ID() && q.URL == key {
				return q.Active, q.ID
			}
		}
		return false, ""
	}

	active, _ := getQuery(key)

	return schema.BoolItem{
		Name:         p.ID() + "_" + key,
		Label:        label,
		InitialValue: active,
		NeedsRefresh: true,
		ApplyFunc: func(b bool) {
			_, cid := getQuery(key)
			if b {
				if cid != "" {
					_ = p.cfg.EnableImageQuery(cid)
				} else {
					_, _ = p.cfg.AddClevelandMuseumQuery(label, key, true)
				}
			} else {
				if cid != "" {
					_ = p.cfg.DisableImageQuery(cid)
				}
			}
		},
	}
}
