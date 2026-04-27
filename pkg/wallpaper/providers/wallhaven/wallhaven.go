package wallhaven

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "embed"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

//go:embed wallhaven.png
var iconData []byte

// Provider implements ImageProvider for Wallhaven.
type Provider struct {
	cfg               *wallpaper.Config
	httpClient        *http.Client
	testAPIKey        string
	validatedUsername string // Successfully verified username via API
}

// SetAPIKeyForTesting sets an API key for testing purposes.
func (p *Provider) SetAPIKeyForTesting(key string) {
	p.testAPIKey = key
}

func init() {
	wallpaper.RegisterProvider("Wallhaven", func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

// NewProvider creates a new Provider.
func NewProvider(cfg *wallpaper.Config, client *http.Client) *Provider {
	return &Provider{
		cfg:        cfg,
		httpClient: client,
	}
}

func (p *Provider) ID() string {
	return "Wallhaven"
}

func (p *Provider) Name() string {
	return i18n.T("Wallhaven")
}

func (p *Provider) Type() provider.ProviderType {
	return provider.TypeOnline
}

func (p *Provider) GetAttributionType() provider.AttributionType {
	return provider.AttributionBy
}

func (p *Provider) SupportsUserQueries() bool {
	return true
}

func (p *Provider) HomeURL() string {
	return "https://wallhaven.cc"
}

func (p *Provider) GetProviderIcon() interface{} {
	return iconData
}

// ParseURL converts a web URL to an API URL.
func (p *Provider) ParseURL(webURL string) (string, error) {
	log.Debugf("Wallhaven Parsing URL: %s", webURL)
	trimmedURL := strings.TrimSpace(webURL)

	baseURL, err := determineBaseAPIURL(trimmedURL)
	if err != nil {
		return "", err
	}

	return cleanQueryParams(baseURL)
}

func determineBaseAPIURL(trimmedURL string) (string, error) {
	switch {
	case UserFavoritesRegex.MatchString(trimmedURL):
		matches := UserFavoritesRegex.FindStringSubmatch(trimmedURL)
		return fmt.Sprintf(WallhavenAPICollectionURL, matches[1], matches[2]), nil

	case MainCategoryRegex.MatchString(trimmedURL):
		matches := MainCategoryRegex.FindStringSubmatch(trimmedURL)
		category := matches[1] // "latest", "toplist", etc.
		baseURL := WallhavenAPISearchURL + "?" + mapCategoryToParams(category)
		if len(matches) > 2 && matches[2] != "" {
			baseURL += "&" + strings.TrimPrefix(matches[2], "?")
		}
		return baseURL, nil

	case SearchRegex.MatchString(trimmedURL):
		matches := SearchRegex.FindStringSubmatch(trimmedURL)
		apiSearchBase := WallhavenAPISearchURL
		if len(matches) == 3 && matches[2] != "" {
			return apiSearchBase + matches[2], nil
		}
		return apiSearchBase, nil

	case APICollectionRegex.MatchString(trimmedURL), APISearchRegex.MatchString(trimmedURL):
		return trimmedURL, nil

	default:
		return "", fmt.Errorf("entered URL is currently not supported: %s", trimmedURL)
	}
}

func cleanQueryParams(baseURL string) (string, error) {
	parsedURL, parseErr := url.Parse(baseURL)
	if parseErr != nil {
		return "", fmt.Errorf("internal error parsing transformed URL '%s': %w", baseURL, parseErr)
	}
	q := parsedURL.Query()
	paramsChanged := false

	if q.Has("apikey") {
		q.Del("apikey")
		paramsChanged = true
	}
	if q.Has("page") {
		q.Del("page")
		paramsChanged = true
	}
	if q.Get("sorting") == "random" && q.Has("seed") {
		q.Del("seed")
		paramsChanged = true
	}

	if paramsChanged {
		parsedURL.RawQuery = q.Encode()
	}
	log.Debugf("Wallhaven Parsed URL: %s", parsedURL.String())
	return parsedURL.String(), nil
}

// WithResolution adds resolution constraints to the API URL if missing.
func (p *Provider) WithResolution(apiURL string, width, height int) string {
	u, err := url.Parse(apiURL)
	if err != nil {
		return apiURL // Return original if parsing fails
	}

	q := u.Query()
	// Only add 'atleast' if no other resolution constraints are present
	if !q.Has("atleast") && !q.Has("resolutions") && !q.Has("ratios") {
		q.Set("atleast", fmt.Sprintf("%dx%d", width, height))
		u.RawQuery = q.Encode()
		return u.String()
	}

	return apiURL
}

// DiscoverCollections fetches all public collections for a given username.
func (p *Provider) DiscoverCollections(ctx context.Context, username string) ([]wallpaper.ImageQuery, error) {
	if username == "" {
		log.Printf("[ERROR] Wallhaven: DiscoverCollections called with empty username")
		return nil, errors.New("username cannot be empty")
	}

	apiKey := p.cfg.GetWallhavenAPIKey()
	if p.testAPIKey != "" {
		apiKey = p.testAPIKey
	}

	log.Debugf("Wallhaven: Discovering collections for user: %s (using API Key: %v)", username, apiKey != "")

	url := fmt.Sprintf(WallhavenAPICollectionsRootURL, username)
	if apiKey != "" {
		url += "?apikey=" + apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Printf("[ERROR] Wallhaven: HTTP request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	log.Debugf("Wallhaven: API response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			return nil, errors.New("access denied: account might be private or API key invalid")
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, errors.New("user not found on Wallhaven")
		}
		return nil, fmt.Errorf("wallhaven API error: status %d", resp.StatusCode)
	}

	var result CollectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[ERROR] Wallhaven: Failed to decode collection JSON: %v", err)
		return nil, err
	}

	log.Debugf("Wallhaven: Found %d remote collections", len(result.Data))

	queries := make([]wallpaper.ImageQuery, 0, len(result.Data))
	for _, col := range result.Data {
		apiURL := fmt.Sprintf(WallhavenAPICollectionURL, username, fmt.Sprint(col.ID))
		privacy := "Private"
		if col.Public == 1 {
			privacy = "Public"
		}
		queries = append(queries, wallpaper.ImageQuery{
			ID:          wallpaper.GenerateQueryID(p.ID() + ":" + apiURL),
			Description: fmt.Sprintf("❤ Collection: %s - %s - %d images", col.Label, privacy, col.Count),
			URL:         apiURL,
			Active:      false, // Default to inactive for new synced collections
			Provider:    "Wallhaven",
			Managed:     true,
		})
	}

	return queries, nil
}

// Sync performs the actual sync of collections into the config.
func (p *Provider) Sync(ctx context.Context) error {
	syncEnabled := p.cfg.GetWallhavenSyncEnabled()
	log.Debugf("Wallhaven: Sync starting. Enabled: %v", syncEnabled)

	if !syncEnabled {
		log.Debugf("Wallhaven: Sync disabled, clearing managed queries.")
		p.cfg.SyncManagedQueries("Wallhaven", nil)
		return nil
	}

	username := p.cfg.GetWallhavenUsername()
	log.Debugf("Wallhaven: Sync using username: '%s'", username)

	if username == "" {
		log.Debugf("Wallhaven: Sync skipped due to empty username.")
		return nil
	}
	remoteQueries, err := p.DiscoverCollections(ctx, username)
	if err != nil {
		log.Printf("[ERROR] Wallhaven: Sync discovery failed: %v", err)
		return err
	}

	p.cfg.SyncManagedQueries("Wallhaven", remoteQueries)
	return nil
}

// SyncCollections is a legacy wrapper (optional cleanup)
func (p *Provider) SyncCollections(ctx context.Context, username string) error {
	return p.Sync(ctx)
}

// mapCategoryToParams maps a Wallhaven Main Category to API query parameters reference in alphabetical order.
func mapCategoryToParams(category string) string {
	switch category {
	case "latest":
		return "order=desc&sorting=date_added"
	case "toplist":
		return "order=desc&sorting=toplist&topRange=1M"
	case "hot":
		return "order=desc&sorting=hot"
	case "random":
		// Random sorting needs a seed to stay consistent during paging, but for a "wallpaper changer"
		// usually we want freshness. Here we set sorting=random.
		// If page=2 is requested later, the API handles it if using same seed, but here we are generating the *base* query.
		return "order=desc&sorting=random"
	default:
		return ""
	}
}

// FetchImages fetches images from Wallhaven.
func (p *Provider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL: %w", err)
	}

	q := u.Query()
	if p.testAPIKey != "" {
		q.Set("apikey", p.testAPIKey)
	} else {
		q.Set("apikey", p.cfg.GetWallhavenAPIKey())
	}
	q.Set("page", fmt.Sprint(page))

	// We need desktop dimensions for default query if no resolutions/atleast specified.
	// Provider doesn't have direct access to OS anymore.
	// We can skip this optimization or add it back if critical.
	// For now, I'll skip it to keep it simple and consistent with interface.
	// If needed, I can add `os` to `Provider` struct.

	// Actually, let's try to keep it if possible.
	// But I don't have access to `wp.os` here.
	// I'll skip it for now. Users can specify resolutions in query if they want.
	// Or I can add `os` to `NewProvider` later.

	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from Wallhaven: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wallhaven API returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var response imgSrvcResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	log.Debugf("Wallhaven API returned %d images", len(response.Data))

	var images []provider.Image
	for _, item := range response.Data {
		log.Debugf("Wallhaven Image ID: %s, Uploader: '%s'", item.ID, item.Uploader.Username)
		images = append(images, provider.Image{
			ID:          item.ID,
			Path:        item.Path,
			ViewURL:     item.ShortURL,
			Attribution: item.Uploader.Username,
			Provider:    p.ID(),
			FileType:    item.FileType,
			Width:       item.DimensionX,
			Height:      item.DimensionY,
		})
	}

	return images, nil
}

// EnrichImage fetches additional details for the image (e.g. attribution) if missing.
func (p *Provider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	if img.Attribution != "" {
		return img, nil // Already has attribution
	}

	// Wallhaven ID is usually the last part of the path or available in the struct
	// We use the ID from the image struct
	apiURL := fmt.Sprintf("https://wallhaven.cc/api/v1/w/%s", img.ID)

	apiKey := p.cfg.GetWallhavenAPIKey()
	if p.testAPIKey != "" {
		apiKey = p.testAPIKey
	}
	if apiKey != "" {
		apiURL += "?apikey=" + apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return img, fmt.Errorf("failed to create enrichment request: %w", err)
	}
	req.Header.Set("User-Agent", "SpiceWallpaperManager/v0.1.0")

	start := time.Now()
	resp, err := p.httpClient.Do(req)
	log.Debugf("EnrichImage HTTP Request took %v", time.Since(start))
	if err != nil {
		return img, fmt.Errorf("failed to fetch enrichment data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Log but don't fail hard, just return original image
		log.Debugf("Wallhaven enrichment failed for %s: status %d", img.ID, resp.StatusCode)
		return img, nil
	}

	var response struct {
		Data struct {
			Uploader struct {
				Username string `json:"username"`
			} `json:"uploader"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return img, fmt.Errorf("failed to decode enrichment response: %w", err)
	}

	if response.Data.Uploader.Username != "" {
		img.Attribution = response.Data.Uploader.Username
		log.Debugf("Enriched Wallhaven image %s with uploader: %s", img.ID, img.Attribution)
	}

	return img, nil
}

// --- Wallhaven JSON Structs ---

// imgSrvcResponse is the response from the Wallhaven API
type imgSrvcResponse struct {
	Data []ImgSrvcImage `json:"data"`
	Meta struct {
		LastPage int `json:"meta"`
	} `json:"meta"`
}

// ImgSrvcImage represents an image from the image service.
type ImgSrvcImage struct {
	Path       string   `json:"path"`
	ID         string   `json:"id"`
	ShortURL   string   `json:"short_url"`
	FileType   string   `json:"file_type"`
	Ratio      string   `json:"ratio"`
	DimensionX int      `json:"dimension_x"`
	DimensionY int      `json:"dimension_y"`
	Thumbs     Thumbs   `json:"thumbs"`
	Uploader   Uploader `json:"uploader"`
}

// Uploader represents the user who uploaded the image.
type Uploader struct {
	Username string `json:"username"`
}

// Thumbs represents the different sizes of the image.
type Thumbs struct {
	Large    string `json:"large"`
	Original string `json:"original"`
	Small    string `json:"small"`
}

// CollectionResponse is the response from the Wallhaven API
type CollectionResponse struct {
	Data []Collection `json:"data"`
}

// Collection is the wallpaper collection
type Collection struct {
	ID     int    `json:"id"`
	Label  string `json:"label"`
	Views  int    `json:"views"`
	Public int    `json:"public"`
	Count  int    `json:"count"`
}

// --- UI Integration ---

// --- UI Implementation (Pure Go) ---

func (p *Provider) Title() string {
	return "Wallhaven"
}

// CreateSettingsPanel returns the declarative UI for Wallhaven settings.
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
	const apiKeyIdent = "wallhavenAPIKey"
	const userIdent = "Wallhaven Username"
	const syncIdent = "WallhavenSyncEnabled"

	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title:       i18n.T("wallhaven API Key"),
				Description: i18n.T("Restricted content requires an API key."),
				Items: []schema.ItemSchema{
					schema.TextItem{
						Name:         apiKeyIdent,
						Label:        i18n.T("wallhaven API Key:"),
						InitialValue: p.cfg.GetWallhavenAPIKey(),
						PlaceHolder:  i18n.T("Enter your wallhaven.cc API Key"),
						IsPassword:   true,
						NeedsRefresh: true,
						EnabledIf: func() bool {
							currKey, _ := sm.GetValue(apiKeyIdent).(string)
							baseKey, _ := sm.GetBaseline(apiKeyIdent).(string)
							return currKey != baseKey || currKey == ""
						},
						ApplyFunc: func(s string) {
							p.cfg.SetWallhavenAPIKey(s)
						},
					},
					schema.HyperlinkItem{
						Text: i18n.T("Restricted content requires an API key. Get one here."),
						URL:  "https://wallhaven.cc/settings/account",
					},
					schema.AsyncButtonItem{
						Name:        "wallhavenVerifyBtn",
						ButtonText:  i18n.T("Verify & Connect"),
						LoadingText: i18n.T("Verifying..."),
						Style:       schema.ButtonStylePrimary,
						VisibleIf: func() bool {
							currKey, _ := sm.GetValue(apiKeyIdent).(string)
							baseKey, _ := sm.GetBaseline(apiKeyIdent).(string)
							return currKey != baseKey || currKey == ""
						},
						OnPressed: func() error {
							currKey, _ := sm.GetValue(apiKeyIdent).(string)
							ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
							defer cancel()
							return CheckWallhavenAPIKeyWithContext(ctx, currKey)
						},
						OnCompleted: func(err error) {
							if err == nil {
								sm.CommitSetting(apiKeyIdent)
							}
						},
					},
					schema.ConfirmButtonItem{
						Name:           "wallhavenClearBtn",
						ButtonText:     i18n.T("Clear API Key"),
						ConfirmTitle:   i18n.T("Clear API Key"),
						ConfirmMessage: i18n.T("Are you sure you want to clear the Wallhaven API Key, Username, and all synced collections?"),
						Importance:     schema.ImportanceDanger,
						VisibleIf: func() bool {
							currKey, _ := sm.GetValue(apiKeyIdent).(string)
							baseKey, _ := sm.GetBaseline(apiKeyIdent).(string)
							return currKey == baseKey && currKey != ""
						},
						OnPressed: func() {
							p.validatedUsername = ""
							sm.ResetSettings(
								setting.SettingReset{Name: apiKeyIdent, Value: ""},
								setting.SettingReset{Name: userIdent, Value: ""},
								setting.SettingReset{Name: syncIdent, Value: false},
							)
							p.cfg.SyncManagedQueries("Wallhaven", nil)
						},
					},
				},
			},
			{
				Title:       i18n.T("Wallhaven Sync"),
				Description: i18n.T("Synchronize your Wallhaven collections with Spice."),
				Items: []schema.ItemSchema{
					schema.TextItem{
						Name:          userIdent,
						Label:         i18n.T("Wallhaven Username:"),
						InitialValue:  p.cfg.GetWallhavenUsername(),
						PlaceHolder:   i18n.T("Please enter your wallhaven.cc username"),
						DisplayStatus: true,
						NeedsRefresh:  true,
						EnabledIf: func() bool {
							val := sm.GetValue(syncIdent)
							if val == nil {
								return true
							}
							return !val.(bool)
						},
						ApplyFunc: func(s string) {
							p.cfg.SetWallhavenUsername(s)
						},
					},
					schema.AsyncButtonItem{
						Name:            "VerifyUsername",
						ButtonText:      i18n.T("Verify Username"),
						LoadingText:     i18n.T("Verifying..."),
						Style:           schema.ButtonStylePrimary,
						TargetStatusKey: userIdent,
						VisibleIf: func() bool {
							curr := sm.GetValue(userIdent)
							base := sm.GetBaseline(userIdent)
							if curr == nil || base == nil {
								return false
							}
							currStr := curr.(string)
							baseStr := base.(string)
							return currStr != "" && currStr != p.validatedUsername && currStr != baseStr
						},
						OnPressed: func() error {
							currUser, _ := sm.GetValue(userIdent).(string)
							apiKeyVal, _ := sm.GetValue(apiKeyIdent).(string)
							if apiKeyVal == "" {
								return errors.New(i18n.T("API Key required for verification"))
							}
							ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
							defer cancel()
							return CheckWallhavenUsername(ctx, currUser, apiKeyVal)
						},
						OnCompleted: func(err error) {
							if err == nil {
								p.validatedUsername, _ = sm.GetValue(userIdent).(string)
								sm.CommitSetting(userIdent)
							}
						},
					},
					schema.BoolItem{
						Name:         syncIdent,
						Label:        i18n.T("Keep Favorites (collections) Synced:"),
						Help:         i18n.T("Automatically synchronize your Wallhaven collections with Spice. New collections will be added as inactive queries."),
						InitialValue: p.cfg.GetWallhavenSyncEnabled(),
						NeedsRefresh: true,
						EnabledIf: func() bool {
							curr := sm.GetValue(userIdent)
							base := sm.GetBaseline(userIdent)
							if curr == nil || base == nil {
								return false
							}
							currStr := curr.(string)
							baseStr := base.(string)
							return currStr != "" && (currStr == p.validatedUsername || currStr == baseStr)
						},
						ApplyFunc: func(b bool) {
							p.cfg.SetWallhavenSyncEnabled(b)
							go func() {
								_ = p.Sync(context.Background())
								sm.SetSettingStatus(syncIdent, i18n.T("Favorites Synced"), schema.ImportanceSuccess)
								sm.Refresh()
							}()
						},
					},
				},
			},
		},
	}
}

// CreateQueryPanel creates the image query management panel.
func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, _ string) *schema.PanelSchema {
	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title:       i18n.T("Wallhaven Queries"),
				Description: i18n.T("Manage your Wallhaven image queries here."),
				Items: []schema.ItemSchema{
					schema.QueryListItem{
						GetQueries: func() []schema.Query {
							queries := p.cfg.GetQueries()
							var abstracts []schema.Query
							for _, q := range queries {
								if q.Provider == p.ID() {
									abstracts = append(abstracts, schema.Query{
										ID:          q.ID,
										URL:         q.URL,
										Description: q.Description,
										Active:      q.Active,
									})
								}
							}
							return abstracts
						},
						EnableQuery:  p.cfg.EnableImageQuery,
						DisableQuery: p.cfg.DisableImageQuery,
						RemoveQuery:  p.cfg.RemoveImageQuery,
					},
				},
			},
		},
	}
}


// GetAPIPacing implements the PacedProvider interface to space out API calls.
func (p *Provider) GetAPIPacing() time.Duration {
	return 1500 * time.Millisecond // 1.5 seconds per API call (40 RPM max)
}

// GetProcessPacing implements the PacedProvider interface to space out image downloads.
func (p *Provider) GetProcessPacing() time.Duration {
	return 1500 * time.Millisecond // 1.5 seconds per image download
}
