package npm

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/curation"
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
	return p
}

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
	entry := curation.GetManager().GetEntry(p.ID(), query)
	if entry == nil {
		log.Printf("NPM: Unknown collection key %q, falling back to masterpieces", query)
		entry = curation.GetManager().GetEntry(p.ID(), "npm_masterpieces")
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

func (p *Provider) fetchCurated(ctx context.Context, entry *curation.CollectionEntry, page int) ([]provider.Image, error) {
	ids := make([]int, 0, len(entry.IDs))
	for _, strID := range entry.IDs {
		if id, err := strconv.Atoi(strID); err == nil {
			ids = append(ids, id)
		}
	}

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

	for _, id := range pageIDs {
		img, err := p.fetchImageByCID(ctx, id)
		if err != nil {
			log.Printf("NPM: Error fetching artwork %d: %v", id, err)
			continue
		}
		if img != nil {
			images = append(images, *img)
		}
	}

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

// FetchThumbnails implements provider.ThumbnailProvider.
func (p *Provider) FetchThumbnails(ctx context.Context, ids []string) ([]provider.Thumbnail, error) {
	var thumbnails []provider.Thumbnail
	for _, idStr := range ids {
		var id int
		if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
			log.Printf("NPM: invalid id format %s", idStr)
			continue
		}
		img, err := p.fetchImageByCID(ctx, id)
		if err != nil {
			log.Printf("NPM: Failed to fetch %d for thumbnails: %v", id, err)
			continue
		}
		if img.Path != "" {
			thumbURL := strings.ReplaceAll(img.Path, "/full/max/0/default.jpg", "/full/800,/0/default.jpg")
			thumbnails = append(thumbnails, provider.Thumbnail{
				ID:  idStr,
				URL: thumbURL,
			})
		}
	}
	return thumbnails, nil
}

// EnrichImage is a no-op
func (p *Provider) EnrichImage(_ context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

// --- UI Implementation ---

func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
	return schema.CreateMuseumSettingsPanel(schema.MuseumSettingsConfig{
		MuseumFramingGetFunc: func() bool { return p.cfg.GetMuseumFraming(p.ID()) },
		MuseumFramingSetFunc: func(val bool) { p.cfg.SetMuseumFraming(p.ID(), val) },
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
	return wallpaper.CreateCuratedQueryPanel(p, sm, p.cfg)
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
