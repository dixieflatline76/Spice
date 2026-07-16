package getty

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/piprate/json-gold/ld"

	"github.com/dixieflatline76/Spice/v2/pkg/curation"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// Provider implements the Getty Museum wallpaper provider.
type Provider struct {
	cfg    *wallpaper.Config
	client *http.Client
	mu     sync.RWMutex

	// JSON-LD Processor
	proc    *ld.JsonLdProcessor
	options *ld.JsonLdOptions
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

// NewProvider creates a new Getty provider.
func NewProvider(cfg *wallpaper.Config, client *http.Client) *Provider {
	p := &Provider{
		cfg:     cfg,
		client:  client,
		proc:    ld.NewJsonLdProcessor(),
		options: ld.NewJsonLdOptions(""),
	}
	return p
}

func (p *Provider) ID() string      { return ProviderName }
func (p *Provider) HomeURL() string { return "https://www.getty.edu/art/collection/" }

func (p *Provider) Name() string {
	return i18n.T("The Getty")
}

func (p *Provider) Title() string { return "The Getty" }

//go:embed getty.png
var iconData []byte

func (p *Provider) GetProviderIcon() interface{} { return iconData }

func (p *Provider) Type() provider.ProviderType {
	return provider.TypeMuseum
}

func (p *Provider) GetAttributionType() provider.AttributionType {
	return provider.AttributionBy
}

func (p *Provider) SupportsUserQueries() bool { return false }

func (p *Provider) ParseURL(url string) (string, error) {
	return url, nil
}

// FetchImages fetches images sequentially based on the page number and curated UUIDs.
func (p *Provider) FetchImages(ctx context.Context, query string, page int) ([]provider.Image, error) {
	entry := curation.GetManager().GetEntry(p.ID(), query)
	if entry == nil || entry.Type != "curated" || len(entry.IDs) == 0 {
		return nil, nil
	}
	ids := entry.IDs

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

	for _, uuid := range pageIDs {
		img, err := p.fetchObjectByUUID(ctx, uuid)
		if err != nil {
			log.Printf("Getty: Error fetching artwork %s: %v", uuid, err)
			continue
		}
		if img != nil {
			images = append(images, *img)
		}
	}
	return images, nil
}

func (p *Provider) fetchObjectByUUID(ctx context.Context, uuid string) (*provider.Image, error) {
	manifestURL := fmt.Sprintf(apiEndpoint, uuid)

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
		return nil, fmt.Errorf("bad status from API: %s", resp.Status)
	}

	var doc map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}

	img, err := p.parseGettyJSONLD(doc)
	if err != nil {
		return nil, err
	}

	img.ID = uuid
	if img.ViewURL == "" {
		img.ViewURL = fmt.Sprintf("https://www.getty.edu/art/collection/object/%s", uuid)
	}
	img.Provider = ProviderName

	return img, nil
}

// FetchThumbnails implements provider.ThumbnailProvider.
func (p *Provider) FetchThumbnails(ctx context.Context, ids []string) ([]provider.Thumbnail, error) {
	thumbnails := make([]provider.Thumbnail, len(ids))
	var wg sync.WaitGroup

	for i, id := range ids {
		wg.Add(1)
		go func(index int, artworkID string) {
			defer wg.Done()
			img, err := p.fetchObjectByUUID(ctx, artworkID)
			if err != nil {
				log.Printf("Getty: Failed to fetch %s for thumbnails: %v", artworkID, err)
				return
			}
			if img != nil && img.Path != "" {
				// Use a smaller image size for gallery preview instead of full res to make it fast
				thumbnails[index] = provider.Thumbnail{
					ID:  artworkID,
					URL: strings.ReplaceAll(img.Path, "/full/max/0/default.jpg", "/full/800,/0/default.jpg"),
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

func (p *Provider) parseGettyJSONLD(doc map[string]interface{}) (*provider.Image, error) {
	// Create a frame to extract exactly what we need
	frame := map[string]interface{}{
		"@context": "https://linked.art/ns/v1/linked-art.json",
		"@type":    "HumanMadeObject",
	}

	framed, err := p.proc.Frame(doc, frame, p.options)
	if err != nil {
		return nil, err
	}

	// Safely extract from framed graph
	graph, ok := framed["@graph"].([]interface{})
	if !ok || len(graph) == 0 {
		return nil, errors.New("no graph in framed json-ld")
	}

	obj, ok := graph[0].(map[string]interface{})
	if !ok {
		return nil, errors.New("invalid graph object")
	}

	title := "Unknown Title"
	if label, ok := obj["_label"].(string); ok {
		title = label
	}

	author := "Unknown Artist"
	if prodBy, ok := obj["produced_by"].(map[string]interface{}); ok {
		if carried, ok := prodBy["carried_out_by"].([]interface{}); ok && len(carried) > 0 {
			if artist, ok := carried[0].(map[string]interface{}); ok {
				if name, ok := artist["_label"].(string); ok {
					author = name
				}
			}
		}
	}

	imageURL := ""
	if rep, ok := obj["representation"].([]interface{}); ok && len(rep) > 0 {
		if rObj, ok := rep[0].(map[string]interface{}); ok {
			if id, ok := rObj["id"].(string); ok {
				// Convert IIIF default to max resolution
				imageURL = strings.ReplaceAll(id, "/full/full/0/default.jpg", "/full/max/0/default.jpg")
			}
		}
	}

	if imageURL == "" {
		return nil, errors.New("no image representation found")
	}

	viewURL := ""
	if subjectOf, ok := obj["subject_of"].([]interface{}); ok {
		for _, so := range subjectOf {
			if soMap, ok := so.(map[string]interface{}); ok {
				if format, ok := soMap["format"].(string); ok && format == "text/html" {
					if id, ok := soMap["id"].(string); ok {
						viewURL = id
						break
					}
				}
			}
		}
	}

	img := &provider.Image{
		Path:        imageURL,
		Attribution: fmt.Sprintf("%s - %s", author, title),
		ViewURL:     viewURL,
	}

	return img, nil
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
		ID:                   "Getty",
		Title:                i18n.T("The J. Paul Getty Museum"),
		Location:             i18n.T("Los Angeles, CA, USA"),
		LicenseURL:           "https://www.getty.edu/about/open-content-program/",
		Description:          i18n.T("The J. Paul Getty Museum features European paintings, drawings, sculpture, illuminated manuscripts, decorative arts, and photography from its beginnings to the present, gathered internationally."),
		MapQuery:             "J. Paul Getty Museum Los Angeles",
		WebsiteURL:           "https://www.getty.edu/visit/",
		DonateURL:            "https://www.getty.edu/about/join-and-give/",
	}, sm.OpenURL)
}

func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, _ string) *schema.PanelSchema {
	return wallpaper.CreateCuratedQueryPanel(p, sm, p.cfg)
}
