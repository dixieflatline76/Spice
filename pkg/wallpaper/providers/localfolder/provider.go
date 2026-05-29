package localfolder

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
)

//go:embed localfolder.png
var iconData []byte

const ProviderName = "LocalFolder"

type Provider struct {
	cfg     *wallpaper.Config
	apiHost string
}

func init() {
	wallpaper.RegisterProvider(ProviderName, func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg)
	})
}

func NewProvider(cfg *wallpaper.Config) *Provider {
	return &Provider{
		cfg:     cfg,
		apiHost: "127.0.0.1:49452",
	}
}

// SetTestConfig allows tests to override internal settings.
func (p *Provider) SetTestConfig(host string) {
	p.apiHost = host
}

func (p *Provider) ID() string {
	return wallpaper.LocalFolderProviderID
}

func (p *Provider) Name() string {
	return i18n.T("Local Folders")
}

func (p *Provider) Type() provider.ProviderType {
	return provider.TypePersonal
}

func (p *Provider) GetAttributionType() provider.AttributionType {
	return provider.AttributionIn
}

func (p *Provider) SupportsUserQueries() bool {
	return true
}

func (p *Provider) Title() string {
	return "Local Folders"
}

func (p *Provider) HomeURL() string {
	return ""
}

func (p *Provider) GetProviderIcon() interface{} {
	return iconData
}

func (p *Provider) ParseURL(webURL string) (string, error) {
	// For Local Folder, the "URL" is actually an absolute folder path
	info, err := os.Stat(webURL)
	if err != nil {
		return "", fmt.Errorf("invalid folder path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", webURL)
	}
	return webURL, nil
}

func (p *Provider) EnrichImage(_ context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

// hasLocalImages checks if the given directory contains at least one image file.
// It uses os.Open and Readdir to stop searching as soon as it finds a single valid image,
// making it much faster for large directories.
func hasLocalImages(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()

	for {
		entries, err := f.Readdir(100) // Process in small batches
		if err != nil {
			break
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
				return true
			}
		}
	}
	return false
}

func (p *Provider) FetchImages(ctx context.Context, folderPath string, page int) ([]provider.Image, error) {
	// Short-circuit if folder is empty
	if !hasLocalImages(folderPath) {
		return []provider.Image{}, nil
	}

	collectionID := wallpaper.HashFolderPath(folderPath)
	host := p.apiHost
	u := fmt.Sprintf("http://%s/local/%s/%s/images?page=%d", host, wallpaper.LocalFolderNamespace, collectionID, page)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local API returned status: %d", resp.StatusCode)
	}

	var respData []struct {
		ID          string `json:"id"`
		URL         string `json:"url"`
		Attribution string `json:"attribution"`
		ProductURL  string `json:"product_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return nil, err
	}

	images := make([]provider.Image, len(respData))
	for i, d := range respData {
		images[i] = provider.Image{
			ID:            fmt.Sprintf("LocalFolder_%s_%s", collectionID, d.ID),
			Path:          d.URL,
			Attribution:   d.Attribution,
			ViewURL:       d.ProductURL,
			Provider:      ProviderName,
			SourceQueryID: wallpaper.GenerateQueryID(wallpaper.LocalFolderProviderID + ":" + folderPath),
		}
	}
	return images, nil
}

// CreateSettingsPanel returns the declarative UI for Local Folder settings.
func (p *Provider) CreateSettingsPanel(_ setting.SettingsManager) *schema.PanelSchema {
	items := []schema.ItemSchema{
		schema.LabelItem{
			Text:       i18n.T("Browse to a folder on your computer containing wallpaper images."),
			Importance: schema.ImportanceLow,
		},
	}

	if runtime.GOOS == "windows" {
		items = append(items, schema.LabelItem{
			Text:       i18n.T("Note (Windows): Due to OS limitations, to select a folder you must click on any image file inside the desired folder and then click 'Open'. The entire folder containing that image will be added."),
			Importance: schema.ImportanceLow,
		})
	}

	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title:   i18n.T("Local Folders"),
				Compact: true,
				Items:   items,
			},
		},
	}
}

// CreateQueryPanel creates the image query management panel.
func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, _ string) *schema.PanelSchema {
	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title: i18n.T("Local Folder Sources"),
				Items: []schema.ItemSchema{
					schema.FolderPickerItem{
						Name:       "local_folder_add",
						ButtonText: i18n.T("Add Folder"),
						OnFolderSelected: func(path string) (string, error) {
							desc := path
							if len(desc) > 100 {
								desc = desc[:100]
							}
							return p.cfg.AddLocalFolderQuery(desc, path, false)
						},
						EnableQuery: p.cfg.EnableImageQuery,
					},
					schema.QueryListItem{
						GetQueries: func() []schema.Query {
							queries := p.cfg.GetLocalFolderQueries()
							abstracts := make([]schema.Query, len(queries))
							for i, q := range queries {
								abstracts[i] = schema.Query{
									ID:          q.ID,
									URL:         q.URL,
									Description: q.URL, // Show full path
									Active:      q.Active,
									Managed:     q.Managed,
								}
							}
							return abstracts
						},
						EnableQuery:  p.cfg.EnableImageQuery,
						DisableQuery: p.cfg.DisableImageQuery,
						RemoveQuery:  p.cfg.RemoveLocalFolderQuery,
						GetDisplayURL: func(q schema.Query) *url.URL {
							// Copy logic from Favorites provider for compatible file URIs
							absPath, err := filepath.Abs(q.URL)
							if err != nil {
								return nil
							}
							slashPath := filepath.ToSlash(absPath)
							if runtime.GOOS == "windows" && !strings.HasPrefix(slashPath, "/") {
								slashPath = "/" + slashPath
							}
							return &url.URL{
								Scheme: "file",
								Path:   slashPath,
							}
						},
					},
				},
			},
		},
	}
}

// ResolveNamespace is the DynamicNamespaceResolver callback for the API server.
// It maps collectionIDs (path hashes) to the actual folder paths from config.
func (p *Provider) ResolveNamespace(namespace, collectionID string) (string, bool) {
	if namespace != wallpaper.LocalFolderNamespace {
		return "", false
	}
	for _, q := range p.cfg.GetLocalFolderQueries() {
		if wallpaper.HashFolderPath(q.URL) == collectionID {
			return q.URL, true
		}
	}
	return "", false
}
