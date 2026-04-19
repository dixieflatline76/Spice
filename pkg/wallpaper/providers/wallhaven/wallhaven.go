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

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

//go:embed wallhaven.png
var iconData []byte

// WallhavenProvider implements ImageProvider for Wallhaven.
type WallhavenProvider struct {
	cfg               *wallpaper.Config
	httpClient        *http.Client
	testAPIKey        string
	validatedUsername string // Successfully verified username via API
}

// SetAPIKeyForTesting sets an API key for testing purposes.
func (p *WallhavenProvider) SetAPIKeyForTesting(key string) {
	p.testAPIKey = key
}

func init() {
	wallpaper.RegisterProvider("Wallhaven", func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewWallhavenProvider(cfg, client)
	})
}

// NewWallhavenProvider creates a new WallhavenProvider.
func NewWallhavenProvider(cfg *wallpaper.Config, client *http.Client) *WallhavenProvider {
	return &WallhavenProvider{
		cfg:        cfg,
		httpClient: client,
	}
}

func (p *WallhavenProvider) ID() string {
	return "Wallhaven"
}

func (p *WallhavenProvider) Name() string {
	return i18n.T("Wallhaven")
}

func (p *WallhavenProvider) Type() provider.ProviderType {
	return provider.TypeOnline
}

func (p *WallhavenProvider) GetAttributionType() provider.AttributionType {
	return provider.AttributionBy
}

func (p *WallhavenProvider) SupportsUserQueries() bool {
	return true
}

func (p *WallhavenProvider) HomeURL() string {
	return "https://wallhaven.cc"
}

// ParseURL converts a web URL to an API URL.
func (p *WallhavenProvider) ParseURL(webURL string) (string, error) {
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
func (p *WallhavenProvider) WithResolution(apiURL string, width, height int) string {
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
func (p *WallhavenProvider) DiscoverCollections(ctx context.Context, username string) ([]wallpaper.ImageQuery, error) {
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
func (p *WallhavenProvider) Sync(ctx context.Context) error {
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
func (p *WallhavenProvider) SyncCollections(ctx context.Context, username string) error {
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
func (p *WallhavenProvider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
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
	// WallhavenProvider doesn't have direct access to OS anymore.
	// We can skip this optimization or add it back if critical.
	// For now, I'll skip it to keep it simple and consistent with interface.
	// If needed, I can add `os` to `WallhavenProvider` struct.

	// Actually, let's try to keep it if possible.
	// But I don't have access to `wp.os` here.
	// I'll skip it for now. Users can specify resolutions in query if they want.
	// Or I can add `os` to `NewWallhavenProvider` later.

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
func (p *WallhavenProvider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
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

func (p *WallhavenProvider) Title() string {
	return "Wallhaven"
}

func (p *WallhavenProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	whHeader := container.NewVBox()

	p.buildAPIKeySection(sm, whHeader)
	p.buildUsernameSection(sm, whHeader)

	// Keep Synced Checkbox
	syncConfig := setting.BoolConfig{
		Name:         "WallhavenSyncEnabled",
		InitialValue: p.cfg.GetWallhavenSyncEnabled(),
		Label:        sm.CreateSettingTitleLabel(i18n.T("Keep Favorites (collections) Synced:")),
		HelpContent:  sm.CreateSettingDescriptionLabel(i18n.T("Automatically synchronize your Wallhaven collections with Spice. New collections will be added as inactive queries.")),
		EnabledIf: func() bool {
			currentUsername := sm.GetValue("Wallhaven Username")
			if currentUsername == nil {
				return false
			}
			// Only enable if the current text matches our successfully validated username
			return p.validatedUsername == currentUsername.(string) && p.validatedUsername != ""
		},
		NeedsRefresh: true,
		ApplyFunc: func(b bool) {
			p.cfg.SetWallhavenSyncEnabled(b)

			// Perform sync/cleanup on Apply
			if b && p.cfg.GetWallhavenUsername() == "" {
				dialog.ShowError(errors.New(i18n.T("Please enter your wallhaven.cc username")), sm.GetSettingsWindow())
			}

			// Trigger sync in background.
			go func() {
				_ = p.Sync(context.Background())
				sm.SetSettingStatus("WallhavenSyncEnabled", i18n.T("Favorites Synced"), widget.SuccessImportance)
				sm.Refresh()
			}()
		},
	}
	sm.CreateBoolSetting(&syncConfig, whHeader)

	return whHeader
}

func (p *WallhavenProvider) buildAPIKeySection(sm setting.SettingsManager, whHeader *fyne.Container) {
	whURL, _ := url.Parse("https://wallhaven.cc/settings/account")
	wallhavenAPIKeyConfig := setting.TextEntrySettingConfig{
		Name:          "wallhavenAPIKey",
		InitialValue:  p.cfg.GetWallhavenAPIKey(),
		PlaceHolder:   i18n.T("Enter your wallhaven.cc API Key"),
		Label:         sm.CreateSettingTitleLabel(i18n.T("wallhaven API Key:")),
		HelpContent:   widget.NewHyperlink(i18n.T("Restricted content requires an API key. Get one here."), whURL),
		Validator:     validation.NewRegexp(WallhavenAPIKeyRegexp, i18n.T("32 alphanumeric characters required")),
		NeedsRefresh:  true,
		DisplayStatus: true,
		IsPassword:    true,
		ApplyFunc: func(s string) {
			p.cfg.SetWallhavenAPIKey(s)
		},
	}
	sm.CreateTextEntrySetting(&wallhavenAPIKeyConfig, whHeader)

	// Button 1: The Async Verify Button
	sm.CreateAsyncButton(&setting.AsyncButtonConfig{
		Name:        "wallhavenVerifyBtn",
		ButtonText:  i18n.T("Verify & Connect"),
		LoadingText: i18n.T("Verifying..."),
		Importance:  widget.HighImportance,
		VisibleIf: func() bool {
			currKey := sm.GetValue("wallhavenAPIKey").(string)
			baseKey := sm.GetBaseline("wallhavenAPIKey").(string)
			// Visible if user typed something new OR if currently unconfigured
			return currKey != baseKey || currKey == ""
		},
		OnPressed: func() error {
			currKey := sm.GetValue("wallhavenAPIKey").(string)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return CheckWallhavenAPIKeyWithContext(ctx, currKey)
		},
		NeedsRefresh: true,
		OnCompleted: func(err error) {
			if err != nil {
				dialog.ShowError(err, sm.GetSettingsWindow())
			} else {
				dialog.ShowInformation(i18n.T("Success"), i18n.T("Wallhaven API Key verified and connected sample successfully."), sm.GetSettingsWindow())
				sm.CommitSetting("wallhavenAPIKey")
			}
		},
	}, whHeader)

	// Button 2: The Synchronous Clear Button
	sm.CreateButtonWithConfirmationSetting(&setting.ButtonWithConfirmationConfig{
		Name:           "wallhavenClearBtn",
		ButtonText:     i18n.T("Clear API Key"),
		ConfirmTitle:   i18n.T("Clear API Key"),
		ConfirmMessage: i18n.T("Are you sure you want to clear the Wallhaven API Key, Username, and all synced collections?"),
		VisibleIf: func() bool {
			currKey := sm.GetValue("wallhavenAPIKey").(string)
			baseKey := sm.GetBaseline("wallhavenAPIKey").(string)
			// Visible only if the current entry matches the verified baseline and is not empty
			return currKey == baseKey && currKey != ""
		},
		OnPressed: func() {
			p.validatedUsername = ""
			sm.ResetSettings(
				setting.SettingReset{Name: "wallhavenAPIKey", Value: ""},
				setting.SettingReset{Name: "Wallhaven Username", Value: ""},
				setting.SettingReset{Name: "WallhavenSyncEnabled", Value: false},
			)
			p.cfg.SyncManagedQueries("Wallhaven", nil)
		},
	}, whHeader)
}

func (p *WallhavenProvider) buildUsernameSection(sm setting.SettingsManager, whHeader *fyne.Container) {
	whUsernameConfig := setting.TextEntrySettingConfig{
		Name:          "Wallhaven Username",
		InitialValue:  p.cfg.GetWallhavenUsername(),
		PlaceHolder:   i18n.T("Please enter your wallhaven.cc username"),
		Label:         sm.CreateSettingTitleLabel(i18n.T("Wallhaven Username:")),
		Validator:     validation.NewRegexp(WallhavenUsernameRegexp, i18n.T("3 to 20 alphanumeric characters (or -_) required")),
		NeedsRefresh:  true,
		DisplayStatus: true,
		EnabledIf: func() bool {
			val := sm.GetValue("WallhavenSyncEnabled")
			if val == nil {
				return true
			}
			return !val.(bool)
		},
		ApplyFunc: func(s string) {
			p.cfg.SetWallhavenUsername(s)
		},
	}
	sm.CreateTextEntrySetting(&whUsernameConfig, whHeader)

	sm.CreateAsyncButton(&setting.AsyncButtonConfig{
		Name:            "VerifyUsername",
		ButtonText:      i18n.T("Verify Username"),
		LoadingText:     i18n.T("Verifying..."),
		Importance:      widget.HighImportance,
		TargetStatusKey: "Wallhaven Username",
		VisibleIf: func() bool {
			currUser, _ := sm.GetValue("Wallhaven Username").(string)
			// Hidden if empty OR matches the successfully validated username
			return currUser != "" && currUser != p.validatedUsername
		},
		OnPressed: func() error {
			currUser, _ := sm.GetValue("Wallhaven Username").(string)
			apiKeyVal, _ := sm.GetValue("wallhavenAPIKey").(string)

			if apiKeyVal == "" {
				return errors.New(i18n.T("API Key required for verification"))
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return CheckWallhavenUsername(ctx, currUser, apiKeyVal)
		},
		NeedsRefresh: true,
		OnCompleted: func(err error) {
			if err == nil {
				p.validatedUsername, _ = sm.GetValue("Wallhaven Username").(string)
				sm.CommitSetting("Wallhaven Username")
			}
		},
	}, whHeader)
}

func (p *WallhavenProvider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	imgQueryList := p.createImgQueryList(sm)
	sm.RegisterRefreshFunc(imgQueryList.Refresh)

	// Configuration for the Add Query Dialog
	addCfg := wallpaper.AddQueryConfig{
		Title:           i18n.T("New Image Query"),
		URLPlaceholder:  i18n.T("Paste your Wallhaven Search or Collection (Favorites) URL"),
		URLValidator:    WallhavenURLRegexp,
		URLErrorMsg:     i18n.T("Invalid wallhaven image query URL pattern"),
		DescPlaceholder: i18n.T("Add a description"),
		DescValidator:   WallhavenDescRegexp,
		DescErrorMsg:    fmt.Sprintf(i18n.T("Description must be between 5 and %d alpha numeric characters long"), wallpaper.MaxDescLength),
		ValidateFunc: func(url, desc string) error {
			// Wallhaven specific logic: we need to convert the Web URL to API URL
			apiURL, _, err := CovertWebToAPIURL(url)
			if err != nil {
				return fmt.Errorf("URL conversion error: %v", err)
			}

			// Check for duplicates
			queryID := wallpaper.GenerateQueryID(p.ID() + ":" + apiURL)
			if p.cfg.IsDuplicateID(queryID) {
				return errors.New(i18n.T("Duplicate query: this URL already exists"))
			}
			return nil
		},
		AddHandler: func(desc, url string, active bool) (string, error) {
			// Convert to API URL again (safe as validation passed)
			apiURL, _, _ := CovertWebToAPIURL(url)
			return p.cfg.AddImageQuery(desc, apiURL, active)
		},
	}

	// Create "Add" Button using standardized helper
	addButton := wallpaper.CreateAddQueryButton(
		i18n.T("Add Wallhaven URL"),
		sm,
		addCfg,
		func() {
			imgQueryList.Refresh()
		},
	)

	// Check for pending URL to auto-open dialog
	if pendingUrl != "" {
		// Verify URL is valid for this provider (Double check)
		if _, err := p.ParseURL(pendingUrl); err == nil {
			// Trigger the dialog
			// We delay slightly to ensure the parent container is effectively part of the window tree if that matters,
			// though Fyne usually handles dialogs fine if window exists.
			fyne.Do(func() {
				wallpaper.OpenAddQueryDialog(sm, addCfg, pendingUrl, "", func() {
					imgQueryList.Refresh()
				})
			})
		}
	}

	header := container.NewVBox()
	header.Add(sm.CreateSettingTitleLabel(i18n.T("Wallhaven Queries and Collections (Favorites)")))
	header.Add(sm.CreateSettingDescriptionLabel(i18n.T("Manage your wallhaven.cc image queries and collections here. Paste your image search or collection URL and Spice will take care of the rest.")))
	header.Add(addButton)
	qpContainer := container.NewBorder(header, nil, nil, nil, imgQueryList)
	return qpContainer
}

func (p *WallhavenProvider) createImgQueryList(sm setting.SettingsManager) *widget.List {
	return wallpaper.CreateQueryList(sm, wallpaper.QueryListConfig{
		GetQueries:    p.cfg.GetImageQueries,
		EnableQuery:   p.cfg.EnableImageQuery,
		DisableQuery:  p.cfg.DisableImageQuery,
		RemoveQuery:   p.cfg.RemoveImageQuery,
		GetDisplayURL: p.getWebURL,
	})
}

// GetProviderIcon returns the provider's icon for the tray menu.
func (p *WallhavenProvider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("Wallhaven", iconData)
}

func (p *WallhavenProvider) getWebURL(q wallpaper.ImageQuery) *url.URL {
	apiURL := q.URL
	// 1. Handle Search URLs
	if strings.Contains(apiURL, "/api/v1/search") {
		urlStr := strings.Replace(apiURL, "https://wallhaven.cc/api/v1/search", "https://wallhaven.cc/search", 1)
		u, err := url.Parse(urlStr)
		if err != nil {
			return nil
		}
		return u
	}

	// 2. Handle Collection URLs
	if APICollectionIDRegex.MatchString(apiURL) {
		matches := APICollectionIDRegex.FindStringSubmatch(apiURL)
		if len(matches) >= 3 {
			// matches[1]=Username, matches[2]=ID
			urlStr := fmt.Sprintf("https://wallhaven.cc/user/%s/favorites/%s", matches[1], matches[2])
			u, err := url.Parse(urlStr)
			if err != nil {
				return nil
			}
			return u
		}
	}

	// Fallback/Default
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil
	}
	return u
}

// GetAPIPacing implements the PacedProvider interface to space out API calls.
func (p *WallhavenProvider) GetAPIPacing() time.Duration {
	return 1500 * time.Millisecond // 1.5 seconds per API call (40 RPM max)
}

// GetProcessPacing implements the PacedProvider interface to space out image downloads.
func (p *WallhavenProvider) GetProcessPacing() time.Duration {
	return 1500 * time.Millisecond // 1.5 seconds per image download
}
