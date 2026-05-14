package googlephotos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "embed"

	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
	"github.com/google/uuid"
)

// Provider implements ImageProvider for Google Photos via Picker & Download.
type Provider struct {
	cfg        *wallpaper.Config
	httpClient *http.Client
	auth       *Authenticator

	apiHost string
	rootDir string

	// Callback to update the query panel when auth state changes
	onAuthStatusChanged func()
}

func init() {
	wallpaper.RegisterProvider("GooglePhotos", func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

// NewProvider creates a new Google Photos Provider.
func NewProvider(cfg *wallpaper.Config, client *http.Client) *Provider {
	p := &Provider{
		cfg:        cfg,
		httpClient: client,
		auth:       NewAuthenticator(cfg, client),
		apiHost:    "127.0.0.1:49452",
		rootDir:    filepath.Join(config.GetAppDir(), "google_photos"),
	}
	p.migrateOldGooglePhotos()
	return p
}

// SetTestConfig allows tests to override internal paths and hosts
func (p *Provider) SetTestConfig(host, rootDir string) {
	p.apiHost = host
	p.rootDir = rootDir
}

func (p *Provider) ID() string {
	return "GooglePhotos"
}

func (p *Provider) Name() string {
	return i18n.T("Google Photos")
}

func (p *Provider) Type() provider.ProviderType {
	return provider.TypePersonal
}

func (p *Provider) GetAttributionType() provider.AttributionType {
	return provider.AttributionIn
}

func (p *Provider) SupportsUserQueries() bool {
	return false
}

func (p *Provider) Title() string {
	return "Google Photos"
}

func (p *Provider) HomeURL() string {
	return "https://photos.google.com"
}

//go:embed GooglePhotos.png
var iconData []byte

func (p *Provider) GetProviderIcon() interface{} {
	return iconData
}

// ParseURL handles internal Google Photos URLs.
// Scheme: googlephotos://<GUID>
func (p *Provider) ParseURL(webURL string) (string, error) {
	u, err := url.Parse(webURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "googlephotos" {
		return "", fmt.Errorf("invalid scheme: %s", u.Scheme)
	}
	return webURL, nil
}

func (p *Provider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	// Image path is already local or needs converting?
	// If path is absolute local path, it works.
	return img, nil
}

// FetchImages queries the local loopback API for images.
func (p *Provider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	// Parse URL to get GUID
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}
	guid := u.Host // googlephotos://<GUID> -> Host is GUID

	// Call Local API
	// Endpoint: /local/google_photos/{guid}/images?page={page}
	// Dynamic port for testing
	reqURL := fmt.Sprintf("http://%s/local/google_photos/%s/images?page=%d&per_page=24", p.apiHost, guid, page)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("local api error: %d", resp.StatusCode)
	}

	var items []struct {
		ID          string `json:"id"`
		URL         string `json:"url"`
		Attribution string `json:"attribution"`
		ProductURL  string `json:"product_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	var images []provider.Image
	for _, item := range items {
		viewURL := item.ProductURL
		if viewURL == "" {
			viewURL = item.URL
		}
		images = append(images, provider.Image{
			ID:          item.ID,
			Path:        item.URL,
			ViewURL:     viewURL,
			Attribution: item.Attribution,
			Provider:    p.ID(),
		})
	}

	return images, nil
}

func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title:   i18n.T("Google Photos"),
				Compact: true,
				Items: []schema.ItemSchema{
					schema.LabelItem{
						Text:       i18n.T("Google Photos is a photo sharing and storage service developed by Google."),
						Importance: schema.ImportanceLow,
					},
				},
			},
		},
	}
}

func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) *schema.PanelSchema {
	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Items: []schema.ItemSchema{
					&schema.OAuthPickerItem{
						Name:  "google_photos_picker",
						Label: i18n.T("Google Photos Extension"),
						Help:  i18n.T("Authorize your Google account to select and download your personal photo albums directly into Spice."),
						CheckAuthStatus: func() (bool, bool) {
							token := p.cfg.GetGooglePhotosToken()
							if token == "" {
								return false, false
							}
							expiry := p.cfg.GetGooglePhotosTokenExpiry()
							return true, time.Now().After(expiry)
						},
						OnAuthorize: func() error {
							return p.auth.StartOAuthFlow(func(u *url.URL) error {
								sm.OpenURL(u.String())
								return nil
							})
						},
						OnDisconnect: func() error {
							p.cfg.SetGooglePhotosToken("")
							return nil
						},
						OnLaunchPicker: func(ctx context.Context, updateStatus func(string)) (int, string, error) {
							// 1. Create Session
							updateStatus(i18n.T("Creating Web Session..."))
							session, err := p.CreatePickerSession(ctx)
							if err != nil {
								return 0, "", err
							}

							// 2. Open Browser
							updateStatus(i18n.T("Please select photos in your browser..."))
							sm.OpenURL(session.PickerURI)

							// 3. Poll
							updateStatus(i18n.T("Waiting for selection (check browser)..."))
							finalSession, err := p.PollSession(ctx, session.ID, session.PollingConfig.PollInterval)
							if err != nil {
								return 0, "", err
							}

							// 4. Get Items
							updateStatus(i18n.T("Retrieving items..."))
							items, err := p.GetSessionItems(ctx, finalSession.ID)
							if err != nil {
								return 0, "", err
							}
							if len(items) == 0 {
								return 0, "", nil
							}

							// 5. Download
							updateStatus(fmt.Sprintf(i18n.T("Downloading %d items..."), len(items)))

							guid := uuid.New().String()
							storageBase := p.rootDir
							targetDir := filepath.Join(storageBase, guid)

							urlMap, err := p.DownloadItems(ctx, items, targetDir)
							if err != nil {
								return 0, "", err
							}

							// Pre-save metadata with links
							if err := p.saveInitialMetadata(guid, urlMap); err != nil {
								log.Printf("Failed to save initial metadata: %v", err)
							}

							return len(items), guid, nil
						},
						OnSaveCollection: func(guid string, description string, active bool) error {
							urlStr := "googlephotos://" + guid
							_, err := p.cfg.AddGooglePhotosQuery(description, urlStr, active)
							if err != nil {
								return err
							}
							_ = p.updateMetadata(guid, description)
							return nil
						},
						OnCancelCollection: func(guid string) {
							p.cleanupDownload(guid)
						},
					},
					schema.QueryListItem{
						GetQueries: func() []schema.Query {
							queries := p.cfg.GetGooglePhotosQueries()
							abstracts := make([]schema.Query, len(queries))
							for i, q := range queries {
								abstracts[i] = schema.Query{
									ID:          q.ID,
									URL:         q.URL,
									Description: q.Description,
									Active:      q.Active,
									Managed:     q.Managed,
								}
							}
							return abstracts
						},
						EnableQuery:  p.cfg.EnableGooglePhotosQuery,
						DisableQuery: p.cfg.DisableGooglePhotosQuery,
						RemoveQuery: func(id string) error {
							// Cleanup files before removing query
							queries := p.cfg.GetGooglePhotosQueries()
							for _, q := range queries {
								if q.ID == id {
									u, _ := url.Parse(q.URL)
									if u != nil && u.Host != "" {
										p.cleanupDownload(u.Host)
									}
									break
								}
							}
							return p.cfg.RemoveGooglePhotosQuery(id)
						},
						GetDisplayURL: func(q schema.Query) *url.URL {
							u, err := url.Parse(q.URL)
							if err != nil || u.Scheme != "googlephotos" {
								return nil
							}
							guid := u.Host
							absPath := filepath.Join(p.rootDir, guid)

							// Copy logic from Favorites provider for compatible file URIs
							slashPath := filepath.ToSlash(absPath)
							if !strings.HasPrefix(slashPath, "/") {
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

// updateMetadata updates the description in metadata.json while preserving other fields.
func (p *Provider) updateMetadata(guid, description string) error {
	storageBase := p.rootDir
	targetDir := filepath.Join(storageBase, guid)
	metaFile := filepath.Join(targetDir, "metadata.json")

	// Ensure dir exists
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return nil
	}

	data := make(map[string]interface{})

	// 1. Read existing
	if f, err := os.Open(metaFile); err == nil {
		_ = json.NewDecoder(f).Decode(&data)
		f.Close()
	}

	// 2. Update
	data["id"] = guid
	data["description"] = description
	// Author omitted or empty
	data["author"] = ""

	// 3. Write
	f, err := os.Create(metaFile)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(data)
}

// saveInitialMetadata creates the metadata.json with file links.
func (p *Provider) saveInitialMetadata(guid string, fileLinks map[string]string) error {
	storageBase := p.rootDir
	targetDir := filepath.Join(storageBase, guid)
	metaFile := filepath.Join(targetDir, "metadata.json")

	// Ensure dir exists (it usually does after download)
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return err
	}

	data := map[string]interface{}{
		"id":     guid,
		"author": "",
		"files":  fileLinks,
	}

	f, err := os.Create(metaFile)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(data)
}

func (p *Provider) cleanupDownload(guid string) {
	path := filepath.Join(p.rootDir, guid)
	os.RemoveAll(path)
}

func (p *Provider) migrateOldGooglePhotos() {
	oldDir := filepath.Join(os.TempDir(), "spice", "google_photos")
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return
	}

	// Ensure new directory exists
	if err := os.MkdirAll(p.rootDir, 0755); err != nil {
		log.Printf("[GooglePhotos] Migration error: failed to create new directory %s: %v", p.rootDir, err)
		return
	}

	entries, err := os.ReadDir(oldDir)
	if err != nil {
		log.Printf("[GooglePhotos] Migration error: failed to read old directory %s: %v", oldDir, err)
		return
	}

	if len(entries) == 0 {
		return
	}

	log.Printf("[GooglePhotos] Migrating %d collections from %s to %s...", len(entries), oldDir, p.rootDir)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		oldPath := filepath.Join(oldDir, entry.Name())
		newPath := filepath.Join(p.rootDir, entry.Name())

		if _, err := os.Stat(newPath); err == nil {
			log.Debugf("[GooglePhotos] Migration: skipping collection %s as it already exists in target", entry.Name())
			continue
		}

		if err := os.Rename(oldPath, newPath); err != nil {
			log.Printf("[GooglePhotos] Migration error: failed to move collection %s: %v", entry.Name(), err)
		}
	}

	// Attempt to remove the old directory if empty
	if entries, err := os.ReadDir(oldDir); err == nil && len(entries) == 0 {
		_ = os.Remove(oldDir)
	}
}
