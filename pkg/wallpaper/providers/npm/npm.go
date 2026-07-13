package npm

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

const ProviderName = "NationalPalaceMuseum"
const WebBaseURL = "https://theme.npm.edu.tw/opendata/"

// Provider implements the National Palace Museum wallpaper provider.
type Provider struct {
	cfg    *wallpaper.Config
	client *http.Client
	mu     sync.RWMutex

	collection *Collection

	// Cache for resolved image details
	poolCache   map[int]*provider.Image
	poolCacheMu sync.RWMutex
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

// NewProvider creates a new NPM provider.
func NewProvider(cfg *wallpaper.Config, client *http.Client) *Provider {
	p := &Provider{
		cfg:       cfg,
		client:    client,
		poolCache: make(map[int]*provider.Image),
	}
	go func() {
		col, err := InitRemoteCollection(cfg)
		if err != nil {
			log.Printf("NPM: Failed to init remote collection: %v", err)
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
	return i18n.T("故宮 (NPM)")
}

func (p *Provider) Title() string { return "故宮 (NPM)" }

//go:embed npm.png
var iconData []byte

func (p *Provider) GetProviderIcon() interface{} { return iconData }

func (p *Provider) Type() provider.ProviderType {
	return provider.TypeMuseum
}

func (p *Provider) GetAttributionType() provider.AttributionType {
	return provider.AttributionBy
}

func (p *Provider) SupportsUserQueries() bool { return false }

// GetAPIPacing implements PacedProvider.
func (p *Provider) GetAPIPacing() time.Duration { return 1000 * time.Millisecond }

// GetProcessPacing implements PacedProvider.
func (p *Provider) GetProcessPacing() time.Duration { return 1500 * time.Millisecond }

// ParseURL checks if a URL matches.
func (p *Provider) ParseURL(url string) (string, error) {
	return url, nil
}

// FetchImages fetches wallpaper candidates.
func (p *Provider) FetchImages(ctx context.Context, query string, page int) ([]provider.Image, error) {
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
		log.Printf("NPM: Unknown collection key %q, falling back to masterpieces", query)
		entry = col.FindEntry("npm_masterpieces")
		if entry == nil {
			return nil, fmt.Errorf("no collection entries available")
		}
	}

	if entry.Type == "curated" {
		return p.fetchCurated(ctx, entry, page)
	}

	// search endpoints not yet implemented
	return nil, nil
}

func (p *Provider) fetchCurated(ctx context.Context, entry *CollectionEntry, page int) ([]provider.Image, error) {
	ids := make([]int, len(entry.IDs))
	copy(ids, entry.IDs)

	const pageSize = 10
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
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup

	for _, id := range pageIDs {
		wg.Add(1)
		go func(cid int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			img, err := p.fetchImageByCID(ctx, cid)
			if err != nil {
				log.Printf("NPM: Error fetching artwork %d: %v", cid, err)
				return
			}
			if img != nil {
				mu.Lock()
				images = append(images, *img)
				mu.Unlock()
			}
		}(id)
	}

	wg.Wait()
	return images, nil
}

func (p *Provider) fetchImageByCID(ctx context.Context, cid int) (*provider.Image, error) {
	p.poolCacheMu.RLock()
	cached, ok := p.poolCache[cid]
	p.poolCacheMu.RUnlock()
	if ok && cached != nil {
		return cached, nil
	}

	manifestURL := fmt.Sprintf("https://digitalarchive.npm.gov.tw/Integrate/GetJson?cid=%d&dept=U", cid)

	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SpiceWallpaper/2.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status from IIIF manifest: %s", resp.Status)
	}

	var manifest iiifManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}

	if len(manifest.Sequences) == 0 || len(manifest.Sequences[0].Canvases) == 0 || len(manifest.Sequences[0].Canvases[0].Images) == 0 {
		return nil, fmt.Errorf("invalid IIIF manifest structure for CID %d", cid)
	}

	var title, englishTitle string
	for _, m := range manifest.Metadata {
		if m.Label == "Chinese Title" {
			title = m.Value
		}
		if m.Label == "English Title" {
			englishTitle = m.Value
		}
	}
	if englishTitle != "" {
		title = englishTitle
	}

	serviceID := selectBestCanvasID(&manifest)
	if serviceID == "" {
		return nil, fmt.Errorf("could not find IIIF service ID for CID %d", cid)
	}

	imageURL := fmt.Sprintf("%s/full/max/0/default.jpg", serviceID)
	viewURL := fmt.Sprintf("https://digitalarchive.npm.gov.tw/Collection/Detail?id=%d&dep=U", cid)

	img := &provider.Image{
		ID:          strconv.Itoa(cid),
		Path:        imageURL,
		Attribution: title,
		ViewURL:     viewURL,
		Provider:    ProviderName,
	}

	p.poolCacheMu.Lock()
	p.poolCache[cid] = img
	p.poolCacheMu.Unlock()

	return img, nil
}

// EnrichImage is a no-op
func (p *Provider) EnrichImage(_ context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

// --- UI Implementation ---

func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
	return schema.CreateMuseumSettingsPanel(schema.MuseumSettingsConfig{
		MuseumFramingGetFunc: func() bool { return p.cfg.GetMuseumFraming("NPM") },
		MuseumFramingSetFunc: func(val bool) { p.cfg.SetMuseumFraming("NPM", val) },
		ID:                   "NPM",
		Title:                i18n.T("國立故宮博物院 - National Palace Museum"),
		Location:             i18n.T("Taipei, Taiwan"),
		LicenseURL:           "https://theme.npm.edu.tw/opendata/Article.aspx?sNo=03009210",
		Description:          i18n.T("The National Palace Museum houses one of the largest collections of Chinese imperial artifacts and artworks in the world."),
		MapQuery:             "National Palace Museum Taipei",
		WebsiteURL:           "https://www.npm.gov.tw/index.aspx?l=2",
		DonateURL:            "https://www.npm.gov.tw/Articles.aspx?sno=03009802&l=2",
	}, sm.OpenURL)
}

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
					_, _ = p.cfg.AddNationalPalaceMuseumQuery(label, key, true)
				}
			} else {
				if cid != "" {
					_ = p.cfg.DisableImageQuery(cid)
				}
			}
		},
	}
}

// selectBestCanvasID parses the IIIF manifest and returns the optimal canvas Service ID.
func selectBestCanvasID(manifest *iiifManifest) string {
	if len(manifest.Sequences) == 0 || len(manifest.Sequences[0].Canvases) == 0 {
		return ""
	}

	bestCanvasIdx := 0
	if len(manifest.Sequences[0].Canvases) > 1 {
		bestCanvasIdx = 1
		maxArea := 0
		for i := 1; i < len(manifest.Sequences[0].Canvases); i++ {
			c := manifest.Sequences[0].Canvases[i]
			if len(c.Images) == 0 || c.Images[0].Resource.Service.ID == "" {
				continue
			}
			w, _ := strconv.Atoi(c.Width)
			h, _ := strconv.Atoi(c.Height)
			area := w * h
			if area > maxArea {
				maxArea = area
				bestCanvasIdx = i
			}
			// 800,000 pixels is roughly 1000x800, a solid threshold for a "hero" shot
			if area >= 800000 {
				bestCanvasIdx = i
				break
			}
		}
	}

	if len(manifest.Sequences[0].Canvases[bestCanvasIdx].Images) == 0 {
		return ""
	}
	return manifest.Sequences[0].Canvases[bestCanvasIdx].Images[0].Resource.Service.ID
}

type iiifManifest struct {
	Metadata []struct {
		Label string `json:"label"`
		Value string `json:"value"`
	} `json:"metadata"`
	Sequences []struct {
		Canvases []struct {
			Width  string `json:"width"`
			Height string `json:"height"`
			Images []struct {
				Resource struct {
					Service struct {
						ID string `json:"@id"`
					} `json:"service"`
				} `json:"resource"`
			} `json:"images"`
		} `json:"canvases"`
	} `json:"sequences"`
}
