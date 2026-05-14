package metmuseum

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
	"golang.org/x/sync/errgroup"
)

type Provider struct {
	cfg    *wallpaper.Config
	client *http.Client
	mu     sync.RWMutex
	// Met Museum specific state
	collection *Collection // In-memory cache of Spice Melange

	// Cache for resolved IDs (to ensure stable pagination and shuffling)
	idCache   map[string][]int
	idCacheMu sync.RWMutex

	// Cache for object details (to support cheap overlap/re-scanning)
	poolCache   map[int]*provider.Image
	poolCacheMu sync.RWMutex
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

func NewProvider(cfg *wallpaper.Config, client *http.Client) *Provider {
	p := &Provider{
		cfg:       cfg,
		client:    client,
		idCache:   make(map[string][]int),
		poolCache: make(map[int]*provider.Image),
	}
	// Try to load embedded collection immediately if possible, or wait/lazy load
	// Async init remote collection
	go func() {
		col, err := InitRemoteCollection(cfg)
		if err != nil {
			log.Printf("MET: Failed to init remote collection: %v", err)
		} else {
			p.mu.Lock()
			p.collection = col
			p.mu.Unlock()
		}
	}()
	return p
}

// SyncRemoteConfig fetches the latest curated collections list from the remote repository.
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
	return "MetMuseum"
}

func (p *Provider) HomeURL() string {
	return "https://www.metmuseum.org"
}

func (p *Provider) Name() string {
	return i18n.T("The Metropolitan Museum of Art")
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

//go:embed MetMuseum.png
var iconData []byte

// GetAPIPacing implements the PacedProvider interface to space out API calls.
func (p *Provider) GetAPIPacing() time.Duration {
	return 1 * time.Second
}

// GetProcessPacing implements the PacedProvider interface to space out image downloads.
func (p *Provider) GetProcessPacing() time.Duration {
	return 1 * time.Second
}

func (p *Provider) ParseURL(url string) (string, error) {
	// Check for direct object URL
	matches := ObjectURLRegex.FindStringSubmatch(url)
	if len(matches) > 1 {
		return "object:" + matches[1], nil
	}

	// Reject foreign URLs (fix for deep linking theft)
	lower := strings.ToLower(url)
	if strings.Contains(lower, "://") || strings.HasPrefix(lower, "www.") {
		if !strings.Contains(lower, "metmuseum.org") {
			return "", fmt.Errorf("invalid Met Museum URL")
		}
	}

	// Treat remaining as search URLs
	return url, nil
}

func (p *Provider) FetchImages(ctx context.Context, query string, page int) ([]provider.Image, error) {
	// 1. Resolve IDs based on query
	ids, err := p.resolveQueryToIDs(ctx, query)
	if err != nil {
		return nil, err
	}

	// 2. Fetch/Scan loop until we satisfy targetCount
	// Since many items might be filtered out (portraits, no images), we act as a stream
	// consuming IDs until we find enough valid ones.
	targetCount := 20
	// Calculate start index based on page
	// FIX: Use a smaller stride (targetCount) to prevent Gaps.
	// We allow Overlap (re-scanning 20-300) because we now cache the results in poolCache.
	// This ensures Page 2 starts at offset 20 and finds items 21-300, even if Page 1 stopped at 25.
	const batchSize = 60
	const maxBatches = 5
	// Stride matches our output goal, ensuring we move forward but check everything
	stride := targetCount

	startIndex := (page - 1) * stride

	if startIndex >= len(ids) {
		return []provider.Image{}, nil
	}

	var images []provider.Image
	var mu sync.Mutex

	// We'll verify chunks until we get enough images or hit max attempts
	// Max scan: 5 batches (300 items) to prevent indefinite loading for a single page

	for b := 0; b < maxBatches; b++ {
		// Stop if we have enough
		mu.Lock()
		got := len(images)
		mu.Unlock()
		if got >= targetCount {
			break
		}

		// determine batch range
		currentStart := startIndex + (b * batchSize)
		if currentStart >= len(ids) {
			break
		}
		currentEnd := currentStart + batchSize
		if currentEnd > len(ids) {
			currentEnd = len(ids)
		}

		batchIDs := ids[currentStart:currentEnd]
		if len(batchIDs) == 0 {
			break
		}

		// Fetch batch in parallel
		g, ctx := errgroup.WithContext(ctx)
		g.SetLimit(5) // Conservative limit (Target: <20-30 req/s active)

		for _, id := range batchIDs {
			id := id // capture
			g.Go(func() error {
				img, err := p.fetchObjectDetails(ctx, id)
				if err != nil {
					return nil
				}
				if img != nil {
					mu.Lock()
					if len(images) < targetCount {
						images = append(images, *img)
					}
					mu.Unlock()
				}
				return nil
			})
		}
		// Wait for this batch
		if err := g.Wait(); err != nil {
			log.Printf("MET: Batch fetch error: %v", err)
		}

		// Throttle between batches to respect rate limits (80 req/s, but be gentle)
		// We use Limit(5) above, so we don't burst too hard.
		// A set sleep ensures we don't burst too hard.
		time.Sleep(200 * time.Millisecond)
	}

	log.Debugf("MET: FetchImages Page %d: Stride %d (Index %d). Scanning up to %d candidates. Yield: %d images.", page, stride, startIndex, maxBatches*batchSize, len(images))
	return images, nil
}

func (p *Provider) resolveQueryToIDs(ctx context.Context, query string) ([]int, error) {
	// Case 1: Direct Object ID
	if strings.HasPrefix(query, "object:") {
		idStr := strings.TrimPrefix(query, "object:")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, err
		}
		return []int{id}, nil
	}

	// Check Cache first (Critical for Shuffle + Pagination stability)
	p.idCacheMu.RLock()
	if cachedIDs, ok := p.idCache[query]; ok {
		p.idCacheMu.RUnlock()
		log.Printf("MET: ID Cache HIT for %s (%d IDs)", query, len(cachedIDs))
		return cachedIDs, nil
	}
	p.idCacheMu.RUnlock()

	log.Debugf("MET: ID Cache MISS for %s. Fetching...", query)

	var ids []int
	var err error

	// Ensure collection is loaded
	p.mu.RLock()
	col := p.collection
	p.mu.RUnlock()

	if col == nil {
		var initErr error
		col, initErr = InitRemoteCollection(p.cfg)
		if initErr != nil {
			return nil, fmt.Errorf("collection not loaded: %w", initErr)
		}
		p.mu.Lock()
		p.collection = col
		p.mu.Unlock()
	}

	// Look up the collection entry by key
	entry := col.FindEntry(query)
	if entry == nil {
		// Legacy fallback: treat unknown keys as the curated collection
		log.Printf("MET: Unknown collection key %q, falling back to curated", query)
		entry = col.FindEntry(CollectionSpiceMelange)
		if entry == nil {
			return nil, fmt.Errorf("no collection entries available")
		}
	}

	switch entry.Type {
	case "curated":
		// Copy IDs to avoid mutating the source collection
		ids = make([]int, len(entry.IDs))
		copy(ids, entry.IDs)
	case "search":
		ids, err = p.fetchSearchHighlights(ctx, entry.Query)
	case "department":
		ids, err = p.fetchDepartmentHighlights(ctx, entry.DeptID)
	default:
		return nil, fmt.Errorf("unknown collection type: %s", entry.Type)
	}

	if err != nil {
		return nil, err
	}

	// Logic: Stable Sort -> Optional Shuffle -> Cache
	// 1. Sort to ensure deterministic baseline (fix random API order)
	sort.Ints(ids)

	// 2. Shuffle if enabled (Stable per session)
	if p.cfg.GetImgShuffle() {
		r := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // Not security-sensitive (UI Shuffle)
		r.Shuffle(len(ids), func(i, j int) {
			ids[i], ids[j] = ids[j], ids[i]
		})
	}

	// 3. Cache it
	p.idCacheMu.Lock()
	p.idCache[query] = ids
	p.idCacheMu.Unlock()

	return ids, nil
}

func (p *Provider) fetchSearchHighlights(ctx context.Context, q string) ([]int, error) {
	// Search for highlights with images
	// e.g. "American Paintings"
	url := fmt.Sprintf("%s/search?isHighlight=true&hasImages=true&isPublicDomain=true&q=%s", APIBaseURL, strings.ReplaceAll(q, " ", "%20"))

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
		return []int{}, nil
	}

	var result struct {
		ObjectIDs []int `json:"objectIDs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.ObjectIDs, nil
}

func (p *Provider) fetchDepartmentHighlights(ctx context.Context, deptID int) ([]int, error) {
	// Base URL construction
	// Note: For European (11), we drop isHighlight to get more volume (~2600 vs 125)
	isHighlight := "true"
	if deptID == DeptEuropeanPaintings {
		isHighlight = "false"
	}

	url := fmt.Sprintf("%s/search?departmentId=%d&isHighlight=%s&hasImages=true&isPublicDomain=true&q=*", APIBaseURL, deptID, isHighlight)

	// Refine query based on department to improve results
	switch deptID {
	case DeptEuropeanPaintings:
		// Keep medium=Paintings for European to avoid clutter
		url += "&medium=Paintings"
	case DeptAsianArt:
		// Asian art includes scrolls, screens, diverse media.
	case DeptEgyptianArt:
		// Papyri, reliefs, jewelry.
	}

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
		return nil, fmt.Errorf("search API failed: %s", resp.Status)
	}

	var result struct {
		ObjectIDs []int `json:"objectIDs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// CRITICAL: Sort IDs to ensure stable pagination across requests.
	// This sort is now handled in resolveQueryToIDs for all ID sources.
	// sort.Ints(result.ObjectIDs)

	return result.ObjectIDs, nil
}

func (p *Provider) fetchObjectDetails(ctx context.Context, id int) (*provider.Image, error) {
	// 1. Check Pool Cache (Avoid redundancy during overlap scan)
	p.poolCacheMu.RLock()
	if cached, ok := p.poolCache[id]; ok {
		p.poolCacheMu.RUnlock()
		return cached, nil
	}
	p.poolCacheMu.RUnlock()

	url := fmt.Sprintf("%s/objects/%d", APIBaseURL, id)
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
		return nil, fmt.Errorf("object API failed: %s", resp.Status)
	}

	// Struct to capture measurements for filtering
	var obj struct {
		ObjectID      int    `json:"objectID"`
		Title         string `json:"title"`
		PrimaryImage  string `json:"primaryImage"`
		ArtistDisplay string `json:"artistDisplayName"`
		ObjectDate    string `json:"objectDate"`
		ObjectURL     string `json:"objectURL"`
		Measurements  []struct {
			ElementName         string `json:"elementName"`
			ElementMeasurements struct {
				Height float64 `json:"Height"`
				Width  float64 `json:"Width"`
			} `json:"elementMeasurements"`
		} `json:"measurements"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return nil, err
	}

	if obj.PrimaryImage == "" {
		p.cacheResult(id, nil) // Cache explicitly as nil (invalid) to skip next time
		return nil, nil        // Skip if no image
	}

	// Strict Filter: Skip Portrait and Square-ish images
	// We strictly require a "Wallpaper" aspect ratio.
	// 1. Check consistency of Length vs Width.
	//    Some objects (stelae) swap Length/Height. If Length is the dominant dimension, it's risky.
	//    But simpler heuristic: Enforce Width must be significantly larger than Height.

	isPortrait := false
	hasMeasurements := false

	if len(obj.Measurements) > 0 {
		for _, m := range obj.Measurements {
			h := m.ElementMeasurements.Height
			w := m.ElementMeasurements.Width
			// Some 3D objects use Length for the major dimension
			// But usually Height/Width tracks the "Face".

			if h > 0 && w > 0 {
				hasMeasurements = true

				// Calculate Aspect Ratio
				ratio := w / h

				// We require strict landscape (> 1.2)
				// This filters out:
				// - Portraits (ratio < 1)
				// - Squares (ratio ~ 1)
				// - Near-squares (ratio < 1.2)
				// This solves the "Egyptian Stela" issue (12.5 vs 12 -> ratio 1.04)
				// and ensures reliable wallpaper candidates.
				if ratio < 1.2 {
					isPortrait = true
				}
				break
			}
		}
	}

	if !hasMeasurements {
		// No dimensions logic? Skip to be safe, as API has mix of orientations.
		p.cacheResult(id, nil)
		return nil, nil
	}

	if isPortrait {
		// log.Printf("Skipping portrait: %s", obj.Title)
		p.cacheResult(id, nil)
		return nil, nil
	}

	img := provider.Image{
		ID:          strconv.Itoa(obj.ObjectID),
		Path:        obj.PrimaryImage,
		ViewURL:     obj.ObjectURL,
		Attribution: fmt.Sprintf("%s - %s", obj.ArtistDisplay, obj.Title),
		Provider:    p.ID(),
	}

	p.cacheResult(id, &img)
	return &img, nil
}

// Helper to cache safe results
func (p *Provider) cacheResult(id int, img *provider.Image) {
	p.poolCacheMu.Lock()
	defer p.poolCacheMu.Unlock()
	p.poolCache[id] = img
}

func (p *Provider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	// We fetch full details in FetchImages, so no enrichment needed
	return img, nil
}

// --- UI Implementation (Pure Go) ---

// CreateSettingsPanel returns the declarative UI for MetMuseum settings.
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
	return schema.CreateMuseumSettingsPanel(schema.MuseumSettingsConfig{
		ID:          "Met",
		Title:       i18n.T("The Metropolitan Museum of Art"),
		Location:    i18n.T("New York City, USA"),
		LicenseURL:  "https://www.metmuseum.org/about-the-met/policies-and-documents/open-access",
		Description: i18n.T("The crown jewel of New York City. From ancient Egyptian temples to modern masterpieces, The Met houses 5,000 years of humanity's greatest creative achievements."),
		MapQuery:    "The Metropolitan Museum of Art",
		WebsiteURL:  "https://www.metmuseum.org",
		DonateURL:   "https://www.metmuseum.org/donate",
	}, sm.OpenURL)
}

// CreateQueryPanel creates the image query management panel.
// Collection items are driven entirely from the JSON collection data.
func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, _ string) *schema.PanelSchema {
	// Ensure collection is loaded for dynamic rendering
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

	curatedSection := schema.SectionSchema{
		Title: i18n.T("Museum Collections"),
		Items: curatedItems,
	}

	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{curatedSection},
	}
}

func (p *Provider) makeCollectionItem(label, key string) schema.BoolItem {
	// Helper to find existing query state
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
					_, _ = p.cfg.AddMetMuseumQuery(label, key, true)
				}
			} else {
				if cid != "" {
					_ = p.cfg.DisableImageQuery(cid)
				}
			}
		},
	}
}
