package rijksmuseum

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
	"golang.org/x/sync/errgroup"
)

// Provider implements the Rijksmuseum wallpaper provider.
// It uses the Linked Art API at data.rijksmuseum.nl (no authentication required).
type Provider struct {
	cfg    *wallpaper.Config
	client *http.Client
	mu     sync.RWMutex

	collection *Collection // In-memory curated collection

	// Cache for search results (stable pagination across pages)
	searchCache   map[string][]string // query key → object IDs
	searchCacheMu sync.RWMutex

	// Cache for resolved image details (avoid redundant 3-step resolution)
	poolCache   map[string]*provider.Image
	poolCacheMu sync.RWMutex
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

// NewProvider creates a new Rijksmuseum provider.
func NewProvider(cfg *wallpaper.Config, client *http.Client) *Provider {
	p := &Provider{
		cfg:         cfg,
		client:      client,
		searchCache: make(map[string][]string),
		poolCache:   make(map[string]*provider.Image),
	}
	// Async init remote collection
	go func() {
		col, err := InitRemoteCollection(cfg)
		if err != nil {
			log.Printf("Rijksmuseum: Failed to init remote collection: %v", err)
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

// Ensure interface compliance
var _ provider.RemoteConfigSyncer = (*Provider)(nil)

func (p *Provider) ID() string {
	return ProviderName
}

func (p *Provider) HomeURL() string {
	return WebBaseURL
}

func (p *Provider) Name() string {
	return i18n.T("Rijksmuseum")
}

func (p *Provider) Title() string {
	return ProviderTitle
}

func (p *Provider) GetProviderIcon() interface{} {
	return iconData
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

//go:embed Rijksmuseum.png
var iconData []byte

// GetAPIPacing implements the PacedProvider interface.
func (p *Provider) GetAPIPacing() time.Duration {
	return 1 * time.Second
}

// GetProcessPacing implements the PacedProvider interface.
func (p *Provider) GetProcessPacing() time.Duration {
	return 2 * time.Second
}

// ParseURL checks if a URL matches a Rijksmuseum object page.
func (p *Provider) ParseURL(url string) (string, error) {
	matches := ObjectURLRegex.FindStringSubmatch(url)
	if len(matches) > 1 {
		return "object:" + matches[1], nil
	}

	lower := strings.ToLower(url)
	if strings.Contains(lower, "://") || strings.HasPrefix(lower, "www.") {
		if !strings.Contains(lower, "rijksmuseum.nl") {
			return "", fmt.Errorf("invalid Rijksmuseum URL")
		}
	}

	return url, nil
}

// FetchImages fetches wallpaper candidates for the given query.
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

	entry := col.FindEntry(query)
	if entry == nil {
		log.Printf("Rijksmuseum: Unknown collection key %q, falling back to masterpieces", query)
		entry = col.FindEntry(CollectionMasterpieces)
		if entry == nil {
			return nil, fmt.Errorf("no collection entries available")
		}
	}

	switch entry.Type {
	case "curated":
		return p.fetchCurated(entry, page)
	case "search":
		return p.fetchSearch(ctx, entry, page)
	default:
		return nil, fmt.Errorf("unknown collection type: %s", entry.Type)
	}
}

// fetchCurated returns images from the pre-resolved curated collection.
func (p *Provider) fetchCurated(entry *CollectionEntry, page int) ([]provider.Image, error) {
	items := entry.Items
	if len(items) == 0 {
		return nil, nil
	}

	// Paginate
	const pageSize = 20
	start := (page - 1) * pageSize
	if start >= len(items) {
		return nil, nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}

	var images []provider.Image
	for _, item := range items[start:end] {
		if item.ImageURL == "" {
			continue
		}
		images = append(images, provider.Image{
			ID:          item.ObjectID,
			Path:        item.ImageURL,
			ViewURL:     BuildObjectURL(item.ObjectNumber),
			Attribution: fmt.Sprintf("%s - %s", item.Artist, item.Title),
			Provider:    p.ID(),
		})
	}

	log.Debugf("Rijksmuseum: Curated page %d: %d images", page, len(images))
	return images, nil
}

// fetchSearch queries the Linked Art API and resolves images via the 3-step chain.
func (p *Provider) fetchSearch(ctx context.Context, entry *CollectionEntry, page int) ([]provider.Image, error) {
	// 1. Get object IDs from search (cached)
	objectIDs, err := p.resolveSearchIDs(ctx, entry)
	if err != nil {
		return nil, err
	}

	// 2. Paginate and resolve
	const targetCount = 20
	const batchSize = 40
	const maxBatches = 5
	stride := targetCount
	startIndex := (page - 1) * stride

	if startIndex >= len(objectIDs) {
		return nil, nil
	}

	var images []provider.Image
	var mu sync.Mutex

	for b := 0; b < maxBatches; b++ {
		mu.Lock()
		got := len(images)
		mu.Unlock()
		if got >= targetCount {
			break
		}

		currentStart := startIndex + (b * batchSize)
		if currentStart >= len(objectIDs) {
			break
		}
		currentEnd := currentStart + batchSize
		if currentEnd > len(objectIDs) {
			currentEnd = len(objectIDs)
		}

		batch := objectIDs[currentStart:currentEnd]
		if len(batch) == 0 {
			break
		}

		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(3) // Conservative: 3 concurrent resolution chains

		for _, objID := range batch {
			objID := objID
			g.Go(func() error {
				img, err := p.resolveObjectToImage(gctx, objID)
				if err != nil || img == nil {
					return nil
				}
				mu.Lock()
				if len(images) < targetCount {
					images = append(images, *img)
				}
				mu.Unlock()
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			log.Printf("Rijksmuseum: Batch resolve error: %v", err)
		}

		time.Sleep(300 * time.Millisecond)
	}

	log.Debugf("Rijksmuseum: Search page %d: %d images from %d candidates", page, len(images), len(objectIDs))
	return images, nil
}

// resolveSearchIDs fetches and caches the list of object IDs for a search collection.
func (p *Provider) resolveSearchIDs(ctx context.Context, entry *CollectionEntry) ([]string, error) {
	// Check cache
	p.searchCacheMu.RLock()
	if cached, ok := p.searchCache[entry.Key]; ok {
		p.searchCacheMu.RUnlock()
		return cached, nil
	}
	p.searchCacheMu.RUnlock()

	// Fetch from API
	url := fmt.Sprintf("%s?%s", SearchBaseURL, entry.SearchParams)
	log.Printf("Rijksmuseum: Searching: %s", url)

	var allIDs []string
	maxPages := 5 // Cap at 500 objects (5 pages × 100 per page)

	for i := 0; i < maxPages; i++ {
		ids, nextURL, err := p.fetchSearchPage(ctx, url)
		if err != nil {
			if len(allIDs) > 0 {
				break // Use what we have
			}
			return nil, err
		}
		allIDs = append(allIDs, ids...)

		if nextURL == "" {
			break
		}
		url = nextURL
	}

	// Sort for stable pagination
	sort.Strings(allIDs)

	// Cache
	p.searchCacheMu.Lock()
	p.searchCache[entry.Key] = allIDs
	p.searchCacheMu.Unlock()

	log.Printf("Rijksmuseum: Search %q resolved %d object IDs", entry.Key, len(allIDs))
	return allIDs, nil
}

// fetchSearchPage fetches one page of search results.
func (p *Provider) fetchSearchPage(ctx context.Context, searchURL string) ([]string, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/ld+json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("search API returned %s", resp.Status)
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", err
	}

	var ids []string
	for _, item := range result.OrderedItems {
		if item.ID != "" {
			// Extract numeric ID from URL (e.g., "https://id.rijksmuseum.nl/200107928" → "200107928")
			parts := strings.Split(item.ID, "/")
			if len(parts) > 0 {
				ids = append(ids, parts[len(parts)-1])
			}
		}
	}

	var nextURL string
	if result.Next != nil && result.Next.ID != "" {
		nextURL = result.Next.ID
	}

	return ids, nextURL, nil
}

// resolveObjectToImage performs the 3-step Linked Art resolution chain:
//
//	Object → VisualItem → DigitalObject → IIIF URL
func (p *Provider) resolveObjectToImage(ctx context.Context, objectID string) (*provider.Image, error) {
	// Check pool cache
	p.poolCacheMu.RLock()
	if cached, ok := p.poolCache[objectID]; ok {
		p.poolCacheMu.RUnlock()
		return cached, nil
	}
	p.poolCacheMu.RUnlock()

	// Step 1: Resolve object
	objURL := fmt.Sprintf("%s/%s", ObjectBaseURL, objectID)
	obj, err := p.fetchLinkedArtObject(ctx, objURL)
	if err != nil {
		p.cacheResult(objectID, nil)
		return nil, err
	}

	// Extract metadata
	title := ExtractTitle(obj.IdentifiedBy)
	objectNumber := ExtractObjectNumber(obj.IdentifiedBy)
	artist := ExtractArtist(obj.ProducedBy)

	// Check dimensions for landscape orientation
	width, height := ExtractDimensions(obj.ReferredToBy)
	if width > 0 && height > 0 {
		ratio := width / height
		if ratio < 1.2 {
			// Portrait or near-square — skip
			p.cacheResult(objectID, nil)
			return nil, nil
		}
	}

	// Step 2: Get VisualItem reference
	if len(obj.Shows) == 0 {
		p.cacheResult(objectID, nil)
		return nil, nil
	}
	visualItemURL := obj.Shows[0].ID

	vi, err := p.fetchVisualItem(ctx, visualItemURL)
	if err != nil {
		p.cacheResult(objectID, nil)
		return nil, err
	}

	// Step 3: Get DigitalObject reference
	if len(vi.DigitallyShownBy) == 0 {
		p.cacheResult(objectID, nil)
		return nil, nil
	}
	digitalObjectURL := vi.DigitallyShownBy[0].ID

	dig, err := p.fetchDigitalObject(ctx, digitalObjectURL)
	if err != nil {
		p.cacheResult(objectID, nil)
		return nil, err
	}

	// Extract IIIF URL
	if len(dig.AccessPoint) == 0 {
		p.cacheResult(objectID, nil)
		return nil, nil
	}
	imageURL := dig.AccessPoint[0].ID

	if imageURL == "" {
		p.cacheResult(objectID, nil)
		return nil, nil
	}

	attribution := title
	if artist != "" {
		attribution = fmt.Sprintf("%s - %s", artist, title)
	}

	img := &provider.Image{
		ID:          objectID,
		Path:        imageURL,
		ViewURL:     BuildObjectURL(objectNumber),
		Attribution: attribution,
		Provider:    p.ID(),
	}

	p.cacheResult(objectID, img)
	return img, nil
}

func (p *Provider) fetchLinkedArtObject(ctx context.Context, url string) (*ObjectResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/ld+json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("object API returned %s for %s", resp.Status, url)
	}

	var obj ObjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return nil, err
	}
	return &obj, nil
}

func (p *Provider) fetchVisualItem(ctx context.Context, url string) (*VisualItemResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/ld+json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("visual item API returned %s", resp.Status)
	}

	var vi VisualItemResponse
	if err := json.NewDecoder(resp.Body).Decode(&vi); err != nil {
		return nil, err
	}
	return &vi, nil
}

func (p *Provider) fetchDigitalObject(ctx context.Context, url string) (*DigitalObjectResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/ld+json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("digital object API returned %s", resp.Status)
	}

	var dig DigitalObjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&dig); err != nil {
		return nil, err
	}
	return &dig, nil
}

func (p *Provider) cacheResult(id string, img *provider.Image) {
	p.poolCacheMu.Lock()
	defer p.poolCacheMu.Unlock()
	p.poolCache[id] = img
}

// EnrichImage is a no-op — all metadata is resolved during FetchImages.
func (p *Provider) EnrichImage(_ context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

// --- UI Implementation ---

// CreateSettingsPanel returns the museum info panel.
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
	return schema.CreateMuseumSettingsPanel(schema.MuseumSettingsConfig{
		MuseumFramingGetFunc: func() bool { return p.cfg.GetMuseumFraming("Rijks") },
		MuseumFramingSetFunc: func(val bool) { p.cfg.SetMuseumFraming("Rijks", val) },
		ID:                   "Rijks",
		Title:                i18n.T("Rijksmuseum"),
		Location:             i18n.T("Amsterdam, Netherlands"),
		LicenseURL:           "https://www.rijksmuseum.nl/en/research/conduct-research/data/policy",
		Description:          i18n.T("The national museum of the Netherlands, home to Rembrandt's Night Watch, Vermeer's Milkmaid, and the finest collection of Dutch Golden Age masterpieces in the world."),
		MapQuery:             "Rijksmuseum Amsterdam",
		WebsiteURL:           WebBaseURL,
		DonateURL:            "https://www.rijksmuseum.nl/en/support",
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
					_, _ = p.cfg.AddRijksmuseumQuery(label, key, true)
				}
			} else {
				if cid != "" {
					_ = p.cfg.DisableImageQuery(cid)
				}
			}
		},
	}
}
