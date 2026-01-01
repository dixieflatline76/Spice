package metmuseum

import (
	"context"
	_ "embed" // For go:embed
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
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

type Provider struct {
	cfg    *wallpaper.Config
	client *http.Client
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
		return NewMetMuseumProvider(cfg, client)
	})
}

func NewMetMuseumProvider(cfg *wallpaper.Config, client *http.Client) *Provider {
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
			p.collection = col
		}
	}()
	return p
}

func (p *Provider) HomeURL() string {
	return "https://www.metmuseum.org"
}

func (p *Provider) Name() string {
	return ProviderName
}

func (p *Provider) Title() string {
	return ProviderTitle
}

func (p *Provider) Type() provider.ProviderType {
	return provider.TypeOnline
}

//go:embed MetMuseum.png
var iconData []byte

func (p *Provider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("MetMuseum.png", iconData)
}

func (p *Provider) ParseURL(url string) (string, error) {
	// Check for direct object URL
	matches := ObjectURLRegex.FindStringSubmatch(url)
	if len(matches) > 1 {
		return "object:" + matches[1], nil
	}
	// TODO: Handle search URLs?
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
		// We use Limit(5) above, so we might hit ~20-50 req/s.
		// A set sleep ensures we don't burst too hard.
		time.Sleep(200 * time.Millisecond)
	}

	log.Printf("MET: FetchImages Page %d: Stride %d (Index %d). Scanning up to %d candidates. Yield: %d images.", page, stride, startIndex, maxBatches*batchSize, len(images))
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

	log.Printf("MET: ID Cache MISS for %s. Fetching...", query)

	var ids []int
	var err error

	// Case 1: Spice Melange (Already cached in p.collection, but needs to be handled uniformly for shuffle)
	if query == CollectionSpiceMelange || query == "metmuseum://curated" {
		if p.collection == nil {
			if col, err := InitRemoteCollection(p.cfg); err == nil {
				p.collection = col
			} else {
				return nil, fmt.Errorf("collection not loaded")
			}
		}
		// Copy IDs to avoid mutating the source collection
		ids = make([]int, len(p.collection.IDs))
		copy(ids, p.collection.IDs)
	} else if query == CollectionAmerican {
		// Case 2: American Art
		ids, err = p.fetchSearchHighlights(ctx, "American Paintings")
	} else {
		// Case 3: Department Highlights
		var deptID int
		switch query {
		case CollectionEuropean:
			deptID = DeptEuropeanPaintings
		case CollectionAsian:
			deptID = DeptAsianArt
		case CollectionEgyptian:
			deptID = DeptEgyptianArt
		default:
			deptID = DeptEuropeanPaintings
		}
		ids, err = p.fetchDepartmentHighlights(ctx, deptID)
	}

	if err != nil {
		return nil, err
	}

	// Logic: Stable Sort -> Optional Shuffle -> Cache
	// 1. Sort to ensure deterministic baseline (fix random API order)
	sort.Ints(ids)

	// 2. Shuffle if enabled (Stable per session)
	if p.cfg.GetImgShuffle() {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
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
		Provider:    ProviderName,
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

// UI Implementation

func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}

func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	// This provider uses the "Museum Template"

	header := wallpaper.CreateMuseumHeader(
		"The Metropolitan Museum of Art",
		"New York City, USA",
		"Open Access (CC0)",
		"https://www.metmuseum.org/about-the-met/policies-and-documents/open-access",
		"The crown jewel of New York City. From ancient Egyptian temples to modern masterpieces, The Met houses 5,000 years of humanity's greatest creative achievements.",
		"https://www.google.com/maps/search/?api=1&query=The+Metropolitan+Museum+of+Art",
		"https://www.metmuseum.org",
		"https://www.metmuseum.org/donate",
		sm,
	)

	// Fixed List of Collections
	collections := []struct {
		Name string
		Key  string
	}{
		{"Director's Cut: Essential Masterpieces", CollectionSpiceMelange},
		{"American Wing", CollectionAmerican},
		{"European Paintings", CollectionEuropean},
		{"Arts of Asia", CollectionAsian},
		{"Egyptian Art", CollectionEgyptian},
	}

	// Helper to find existing query state
	getDetails := func(key string) (bool, string) {
		for _, q := range p.cfg.GetMetMuseumQueries() {
			if q.URL == key {
				return q.Active, q.ID
			}
		}
		return false, "" // Not added yet
	}

	// Create Checkboxes
	var checks []fyne.CanvasObject
	for _, col := range collections {
		col := col // capture
		active, _ := getDetails(col.Key)

		chk := widget.NewCheck(col.Name, func(on bool) {
			// Defer Save Logic (Standard Pattern)
			// We verify against initial 'active' state captured at closure creation.
			// This works because "Apply" triggers callbacks to commit changes.

			isActive, _ := getDetails(col.Key)

			dirtyKey := fmt.Sprintf("met_%s", col.Key)
			callbackKey := fmt.Sprintf("met_cb_%s", col.Key)

			if on != isActive {
				sm.SetSettingChangedCallback(callbackKey, func() {
					// Actual Save Logic (Deferred)
					// Fetch fresh ID from config to ensure we target correctly
					_, cid := getDetails(col.Key)

					if on {
						if cid != "" {
							if err := p.cfg.EnableMetMuseumQuery(cid); err != nil {
								log.Printf("MET: Failed to enable %s: %v", col.Name, err)
							}
						} else {
							desc := fmt.Sprintf("The Met: %s", col.Name)
							if _, err := p.cfg.AddMetMuseumQuery(desc, col.Key, true); err != nil {
								log.Printf("MET: Failed to add %s: %v", col.Name, err)
							}
						}
					} else {
						if cid != "" {
							if err := p.cfg.DisableMetMuseumQuery(cid); err != nil {
								log.Printf("MET: Failed to disable %s: %v", col.Name, err)
							}
						}
					}
				})
				// Enable Apply Button
				sm.SetRefreshFlag(dirtyKey)

			} else {
				// Reverted to original state
				sm.RemoveSettingChangedCallback(callbackKey)
				sm.UnsetRefreshFlag(dirtyKey)
				// We don't unset "queries" (global), but without callback, no change occurs.
			}
			sm.GetCheckAndEnableApplyFunc()()
		})
		chk.Checked = active
		checks = append(checks, chk)
	}

	listContainer := container.NewVBox(checks...)

	return container.NewVBox(
		header,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Collections", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		listContainer,
	)
}
