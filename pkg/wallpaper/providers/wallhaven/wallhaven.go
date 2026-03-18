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
		ApplyFunc: func(b bool) {
			p.cfg.SetWallhavenSyncEnabled(b)

			// Perform sync/cleanup on Apply
			if b && p.cfg.GetWallhavenUsername() == "" {
				dialog.ShowError(errors.New(i18n.T("Please enter your wallhaven.cc username")), sm.GetSettingsWindow())
				// we don't return here so the setting is still saved, but sync is skipped
			}

			// Trigger sync in background
			go func() {
				_ = p.Sync(context.Background())
				fyne.Do(func() {
					sm.SetRefreshFlag("queries") // This will trigger refresh of all registered refresh funcs
				})
			}()
		},
	}
	sm.CreateBoolSetting(&syncConfig, whHeader)

	return whHeader
}

func (p *WallhavenProvider) buildAPIKeySection(sm setting.SettingsManager, whHeader *fyne.Container) {
	var apiKeyBtn *widget.Button

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
		EnabledIf: func() bool {
			currentValue := sm.GetValue("wallhavenAPIKey")
			if currentValue == nil {
				return true
			}
			baselineValue := sm.GetBaseline("wallhavenAPIKey")
			if baselineValue == nil {
				baselineValue = ""
			}

			curr := currentValue.(string)
			base := baselineValue.(string)

			return curr == "" || curr != base
		},
		OnChanged: func(s string) {
			if apiKeyBtn == nil {
				return
			}
			base := sm.GetBaseline("wallhavenAPIKey").(string)
			if s == base {
				if s == "" {
					apiKeyBtn.Hide()
				} else {
					apiKeyBtn.SetText(i18n.T("Clear API Key"))
					apiKeyBtn.Importance = widget.DangerImportance
					apiKeyBtn.Show()
				}
			} else {
				apiKeyBtn.SetText(i18n.T("Verify & Connect"))
				apiKeyBtn.Importance = widget.HighImportance
				if s == "" {
					apiKeyBtn.Hide()
				} else {
					apiKeyBtn.Show()
				}
			}
			apiKeyBtn.Refresh()
		},
	}

	refreshAPIKeyUI := func() {
		curr := sm.GetValue("wallhavenAPIKey").(string)
		wallhavenAPIKeyConfig.OnChanged(curr)
	}

	sm.CreateTextEntrySetting(&wallhavenAPIKeyConfig, whHeader)

	apiKeyBtn = widget.NewButton(i18n.T("Verify & Connect"), func() {
		currKey := sm.GetValue("wallhavenAPIKey").(string)
		baseKey := sm.GetBaseline("wallhavenAPIKey").(string)

		if currKey == baseKey && currKey != "" {
			dialog.NewConfirm(i18n.T("Clear API Key"), i18n.T("Are you sure you want to clear the Wallhaven API Key, Username, and all synced collections?"), func(b bool) {
				if b {
					p.validatedUsername = ""
					sm.SetValue("wallhavenAPIKey", "")
					sm.SetValue("Wallhaven Username", "")
					sm.SetValue("WallhavenSyncEnabled", false)
					p.cfg.SetWallhavenAPIKey("")
					p.cfg.SetWallhavenUsername("")
					p.cfg.SetWallhavenSyncEnabled(false)
					p.cfg.SyncManagedQueries("Wallhaven", nil)
					sm.SeedBaseline("wallhavenAPIKey", "")
					sm.SeedBaseline("Wallhaven Username", "")
					sm.SeedBaseline("WallhavenSyncEnabled", false)
					sm.GetCheckAndEnableApplyFunc()()
					sm.Refresh()
				}
			}, sm.GetSettingsWindow()).Show()
			return
		}

		apiKeyBtn.Disable()
		apiKeyBtn.SetText(i18n.T("Verifying..."))
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := CheckWallhavenAPIKeyWithContext(ctx, currKey)
			fyne.Do(func() {
				apiKeyBtn.Enable()
				if err != nil {
					dialog.ShowError(err, sm.GetSettingsWindow())
					apiKeyBtn.SetText(i18n.T("Verify & Connect"))
					return
				}
				p.cfg.SetWallhavenAPIKey(currKey)
				sm.SeedBaseline("wallhavenAPIKey", currKey)
				sm.Refresh()
				refreshAPIKeyUI()
			})
		}()
	})
	apiKeyBtn.Importance = widget.HighImportance
	initialKey := p.cfg.GetWallhavenAPIKey()
	if initialKey == "" {
		apiKeyBtn.Hide()
	} else {
		apiKeyBtn.SetText(i18n.T("Clear API Key"))
		apiKeyBtn.Importance = widget.DangerImportance
	}
	whHeader.Add(apiKeyBtn)
}

func (p *WallhavenProvider) buildUsernameSection(sm setting.SettingsManager, whHeader *fyne.Container) {
	var usernameBtn *widget.Button

	whUsernameConfig := setting.TextEntrySettingConfig{
		Name:          "Wallhaven Username",
		InitialValue:  p.cfg.GetWallhavenUsername(),
		PlaceHolder:   i18n.T("Please enter your wallhaven.cc username"),
		Label:         sm.CreateSettingTitleLabel(i18n.T("Wallhaven Username:")),
		Validator:     validation.NewRegexp(WallhavenUsernameRegexp, i18n.T("3 to 20 alphanumeric characters (or -_) required")),
		NeedsRefresh:  true,
		DisplayStatus: false,
		EnabledIf: func() bool {
			val := sm.GetValue("WallhavenSyncEnabled")
			if val == nil {
				return true
			}
			return !val.(bool)
		},
		OnChanged: func(s string) {
			if usernameBtn == nil {
				return
			}
			if s == "" || s == p.validatedUsername {
				usernameBtn.Hide()
			} else {
				usernameBtn.Show()
			}
			usernameBtn.Refresh()
		},
	}
	sm.CreateTextEntrySetting(&whUsernameConfig, whHeader)

	usernameBtn = widget.NewButton(i18n.T("Verify Username"), func() {
		currUser := sm.GetValue("Wallhaven Username").(string)
		apiKeyVal := sm.GetValue("wallhavenAPIKey")
		var apiKey string
		if apiKeyVal != nil {
			apiKey = apiKeyVal.(string)
		}

		if apiKey == "" {
			dialog.ShowError(errors.New(i18n.T("API Key required for verification")), sm.GetSettingsWindow())
			return
		}

		usernameBtn.Disable()
		usernameBtn.SetText(i18n.T("Verifying..."))
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := CheckWallhavenUsername(ctx, currUser, apiKey)
			fyne.Do(func() {
				usernameBtn.Enable()
				usernameBtn.SetText(i18n.T("Verify Username"))
				if err != nil {
					dialog.ShowError(err, sm.GetSettingsWindow())
					p.validatedUsername = ""
					sm.Refresh()
					return
				}
				p.validatedUsername = currUser
				p.cfg.SetWallhavenUsername(currUser)
				sm.SeedBaseline("Wallhaven Username", currUser)
				sm.Refresh()
				usernameBtn.Hide()
			})
		}()
	})
	usernameBtn.Importance = widget.HighImportance
	if p.cfg.GetWallhavenUsername() == "" || p.validatedUsername == p.cfg.GetWallhavenUsername() {
		usernameBtn.Hide()
	}
	whHeader.Add(usernameBtn)
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

func (p *WallhavenProvider) getWebURL(apiURL string) *url.URL {
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
	return 1000 * time.Millisecond // 1 second per API call
}

// GetProcessPacing implements the PacedProvider interface to space out image downloads.
func (p *WallhavenProvider) GetProcessPacing() time.Duration {
	return 1000 * time.Millisecond // 1 second per image download
}
