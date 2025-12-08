package wallhaven

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
)

// WallhavenProvider implements ImageProvider for Wallhaven.
type WallhavenProvider struct {
	cfg        *wallpaper.Config
	httpClient *http.Client
	testAPIKey string
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

func (p *WallhavenProvider) Name() string {
	return "Wallhaven"
}

func (p *WallhavenProvider) HomeURL() string {
	return "https://wallhaven.cc"
}

// ParseURL converts a web URL to an API URL.
func (p *WallhavenProvider) ParseURL(webURL string) (string, error) {
	trimmedURL := strings.TrimSpace(webURL)
	var baseURL string

	// --- Transformation and Type Detection using constants ---
	switch {
	case UserFavoritesRegex.MatchString(trimmedURL):
		matches := UserFavoritesRegex.FindStringSubmatch(trimmedURL)
		// matches[0]=Full, matches[1]=Username, matches[2]=ID
		baseURL = fmt.Sprintf(WallhavenAPICollectionURL, matches[1], matches[2])

	case SearchRegex.MatchString(trimmedURL):
		matches := SearchRegex.FindStringSubmatch(trimmedURL)
		// matches[0]=Full match, matches[1]=Base ("https://wallhaven.cc/"), matches[2]=Query part ("?q=...") or empty string
		apiSearchBase := WallhavenAPISearchURL
		if len(matches) == 3 && matches[2] != "" { // Check if query part was captured
			baseURL = apiSearchBase + matches[2] // Append the captured query part
		} else {
			baseURL = apiSearchBase // No query part found
		}

	case APICollectionRegex.MatchString(trimmedURL):
		baseURL = trimmedURL // Already API format

	case APISearchRegex.MatchString(trimmedURL):
		baseURL = trimmedURL // Already API format

	default:
		return "", fmt.Errorf("entered URL is currently not supported: %s", trimmedURL)
	}

	// --- Parameter Cleaning (for the URL to be *saved*) ---
	parsedURL, parseErr := url.Parse(baseURL)
	if parseErr != nil {
		return "", fmt.Errorf("internal error parsing transformed URL '%s': %w", baseURL, parseErr)
	}
	q := parsedURL.Query()
	paramsChanged := false
	// Clean API key (don't store in saved query)
	if q.Has("apikey") {
		q.Del("apikey")
		paramsChanged = true
	}
	// Clean page (don't store pagination in base query)
	if q.Has("page") {
		q.Del("page")
		paramsChanged = true
	}

	if paramsChanged {
		parsedURL.RawQuery = q.Encode()
	}
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

	var images []provider.Image
	for _, item := range response.Data {
		log.Debugf("Wallhaven Image ID: %s, Uploader: '%s'", item.ID, item.Uploader.Username)
		images = append(images, provider.Image{
			ID:          item.ID,
			Path:        item.Path,
			ViewURL:     item.ShortURL,
			Attribution: item.Uploader.Username,
			Provider:    p.Name(),
			FileType:    item.FileType,
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

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return img, fmt.Errorf("failed to fetch enrichment data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Log but don't fail hard, just return original image
		log.Printf("Wallhaven enrichment failed for %s: status %d", img.ID, resp.StatusCode)
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
	Path     string   `json:"path"`
	ID       string   `json:"id"`
	ShortURL string   `json:"short_url"`
	FileType string   `json:"file_type"`
	Ratio    string   `json:"ratio"`
	Thumbs   Thumbs   `json:"thumbs"`
	Uploader Uploader `json:"uploader"`
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

	// Wallhaven API Key
	whURL, _ := url.Parse("https://wallhaven.cc/settings/account")
	wallhavenAPIKeyConfig := setting.TextEntrySettingConfig{
		Name:          "wallhavenAPIKey",
		InitialValue:  p.cfg.GetWallhavenAPIKey(),
		PlaceHolder:   "Enter your wallhaven.cc API Key",
		Label:         sm.CreateSettingTitleLabel("wallhaven API Key:"),
		HelpContent:   widget.NewHyperlink("Restricted content requires an API key. Get one here.", whURL),
		Validator:     validation.NewRegexp(WallhavenAPIKeyRegexp, "32 alphanumeric characters required"),
		NeedsRefresh:  true,
		DisplayStatus: true,
		PostValidateCheck: func(s string) error {
			// Skip check if empty (clearing key)
			if s == "" {
				return nil
			}
			return CheckWallhavenAPIKey(s)
		},
	}
	wallhavenAPIKeyConfig.ApplyFunc = func(s string) {
		p.cfg.SetWallhavenAPIKey(s)
		wallhavenAPIKeyConfig.InitialValue = s
	}
	sm.CreateTextEntrySetting(&wallhavenAPIKeyConfig, whHeader)

	// Clear API Key Button
	clearKeyBtn := widget.NewButton("Clear API Key", func() {
		dialog.NewConfirm("Clear API Key", "Are you sure you want to clear the Wallhaven API Key?", func(b bool) {
			if b {
				p.cfg.SetWallhavenAPIKey("")
				wallhavenAPIKeyConfig.InitialValue = ""
			}
		}, sm.GetSettingsWindow()).Show()
	})
	whHeader.Add(clearKeyBtn)

	return whHeader
}

func (p *WallhavenProvider) CreateQueryPanel(sm setting.SettingsManager) fyne.CanvasObject {
	imgQueryList := p.createImgQueryList(sm)
	sm.RegisterRefreshFunc(imgQueryList.Refresh)

	var addButton *widget.Button

	addButton = widget.NewButton("Add wallhaven URL", func() {

		urlEntry := widget.NewEntry()
		urlEntry.SetPlaceHolder("Cut and paste a wallhaven search or collection URL from your browser to here")

		descEntry := widget.NewEntry()
		descEntry.SetPlaceHolder("Add a description for these images")

		formStatus := widget.NewLabel("")
		activeBool := widget.NewCheck("Active", nil)

		cancelButton := widget.NewButton("Cancel", nil)
		saveButton := widget.NewButton("Save", nil)
		saveButton.Disable() // Save button is only enabled when the URL is valid and min desc has been added

		formValidator := func(who *widget.Entry) bool {
			urlStrErr := urlEntry.Validate()
			descStrErr := descEntry.Validate()

			if urlStrErr != nil {
				if who == urlEntry {
					formStatus.SetText(urlStrErr.Error())
					formStatus.Importance = widget.DangerImportance
				}
				formStatus.Refresh()
				return false // URL syntax is wrong
			}

			if descStrErr != nil {
				if who == descEntry {
					formStatus.SetText(descStrErr.Error())
					formStatus.Importance = widget.DangerImportance
				}
				formStatus.Refresh()
				return false // Description is wrong
			}

			apiURL, _, err := CovertWebToAPIURL(urlEntry.Text)
			if err != nil {
				if who == urlEntry {
					formStatus.SetText(fmt.Sprintf("URL conversion error: %v", err))
					formStatus.Importance = widget.DangerImportance
				}
				formStatus.Refresh()
				return false // URL is not convertible
			}

			queryID := wallpaper.GenerateQueryID(apiURL)
			if p.cfg.IsDuplicateID(queryID) {
				if who == urlEntry || (who == descEntry && urlEntry.Text != "") {
					formStatus.SetText("Duplicate query: this URL already exists")
					formStatus.Importance = widget.DangerImportance
				}
				formStatus.Refresh()
				return false // It's a duplicate!
			}

			formStatus.SetText("Everything looks good")
			formStatus.Importance = widget.SuccessImportance
			formStatus.Refresh()
			return true
		}

		urlEntry.Validator = validation.NewRegexp(WallhavenURLRegexp, "Invalid wallhaven image query URL pattern")
		descEntry.Validator = validation.NewRegexp(WallhavenDescRegexp, fmt.Sprintf("Description must be between 5 and %d alpha numeric characters long", wallpaper.MaxDescLength))

		newEntryLengthChecker := func(entry *widget.Entry, maxLen int) func(string) {
			{
				return func(s string) {
					if len(s) > maxLen {
						entry.SetText(s[:maxLen]) // Truncate to max length
						return                    // Stop further processing
					}

					if formValidator(entry) {
						saveButton.Enable()
					} else {
						saveButton.Disable()
					}
				}
			}
		}
		urlEntry.OnChanged = newEntryLengthChecker(urlEntry, wallpaper.MaxURLLength)
		descEntry.OnChanged = newEntryLengthChecker(descEntry, wallpaper.MaxDescLength)

		c := container.NewVBox()
		c.Add(sm.CreateSettingTitleLabel("wallhaven Search or Collection (Favorites) URL:"))
		c.Add(urlEntry)
		c.Add(sm.CreateSettingTitleLabel("Description:"))
		c.Add(descEntry)
		c.Add(formStatus)
		c.Add(activeBool)
		c.Add(widget.NewSeparator())
		c.Add(container.NewHBox(cancelButton, layout.NewSpacer(), saveButton))

		d := dialog.NewCustomWithoutButtons("New Image Query", c, sm.GetSettingsWindow())
		d.Resize(fyne.NewSize(800, 200))

		saveButton.OnTapped = func() {

			apiURL, _, err := CovertWebToAPIURL(urlEntry.Text)
			if err != nil {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
				return
			}

			// We skip CheckWallhavenURL for now as it requires OS context.
			// We can reimplement a simpler check here if needed later.

			// We already checked for duplicates, but we check err just in case.
			newID, err := p.cfg.AddImageQuery(descEntry.Text, apiURL, activeBool.Checked)
			if err != nil {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
				return // Don't close the dialog
			}

			addButton.Enable()
			imgQueryList.Refresh()

			if activeBool.Checked {
				sm.SetRefreshFlag(newID)
				sm.GetCheckAndEnableApplyFunc()()
			}
			d.Hide()
			addButton.Enable()
		}

		cancelButton.OnTapped = func() {
			d.Hide()
			addButton.Enable()
		}

		d.Show()
		addButton.Disable()
	})

	header := container.NewVBox()
	header.Add(sm.CreateSettingTitleLabel("wallhaven Queries and Collections (Favorites)"))
	header.Add(sm.CreateSettingDescriptionLabel("Manage your wallhaven.cc image queries and collections here. Paste your image search or collection URL and Spice will take care of the rest."))
	header.Add(addButton)
	qpContainer := container.NewBorder(header, nil, nil, nil, imgQueryList)
	return qpContainer
}

func (p *WallhavenProvider) createImgQueryList(sm setting.SettingsManager) *widget.List {
	pendingState := make(map[string]bool)
	var queryList *widget.List
	queryList = widget.NewList(
		func() int {
			return len(p.cfg.GetImageQueries())
		},
		func() fyne.CanvasObject {
			urlLink := widget.NewHyperlink("Placeholder", nil)
			activeCheck := widget.NewCheck("Active", nil)
			deleteButton := widget.NewButton("Delete", nil)

			return container.NewHBox(urlLink, layout.NewSpacer(), activeCheck, deleteButton)
		},
		func(i int, o fyne.CanvasObject) {
			queries := p.cfg.GetImageQueries()
			if i >= len(queries) {
				return
			}
			query := queries[i]
			queryKey := query.ID

			c := o.(*fyne.Container)

			urlLink := c.Objects[0].(*widget.Hyperlink)
			urlLink.SetText(query.Description)

			siteURL := p.getWebURL(query.URL)
			if siteURL != nil {
				urlLink.SetURL(siteURL)
			} else {
				if err := urlLink.SetURLFromString(query.URL); err != nil {
					log.Printf("Failed to set URL from string: %v", err)
				}
			}

			activeCheck := c.Objects[2].(*widget.Check)
			deleteButton := c.Objects[3].(*widget.Button)

			initialActive := query.Active
			activeCheck.OnChanged = nil

			if val, ok := pendingState[queryKey]; ok {
				activeCheck.SetChecked(val)
			} else {
				activeCheck.SetChecked(initialActive)
			}

			activeCheck.OnChanged = func(b bool) {
				// Fetch latest status to ensure we compare against current config, not stale UI state
				currentQ, found := p.cfg.GetQuery(queryKey)
				currentActive := initialActive
				if found {
					currentActive = currentQ.Active
				}

				if b != currentActive {
					pendingState[queryKey] = b
					sm.SetSettingChangedCallback(queryKey, func() {
						var err error
						if b {
							err = p.cfg.EnableImageQuery(query.ID)
						} else {
							err = p.cfg.DisableImageQuery(query.ID)
						}
						if err != nil {
							log.Printf("Failed to update query status: %v", err)
						}
						delete(pendingState, queryKey)
					})
					sm.SetRefreshFlag(queryKey)
				} else {
					delete(pendingState, queryKey)
					sm.RemoveSettingChangedCallback(queryKey)
					sm.UnsetRefreshFlag(queryKey)
				}
				sm.GetCheckAndEnableApplyFunc()()
			}

			deleteButton.OnTapped = func() {
				d := dialog.NewConfirm("Please Confirm", fmt.Sprintf("Are you sure you want to delete %s?", query.Description), func(b bool) {
					if b {
						if query.Active {
							sm.SetRefreshFlag(queryKey)
							// Trigger apply check if needed
							sm.GetCheckAndEnableApplyFunc()()
						}
						delete(pendingState, queryKey)
						if err := p.cfg.RemoveImageQuery(query.ID); err != nil {
							log.Printf("Failed to remove image query: %v", err)
						}
						queryList.Refresh()
					}
				}, sm.GetSettingsWindow())
				d.Show()
			}
		},
	)
	return queryList
}

// GetProviderIcon returns the provider's icon for the tray menu.
func (p *WallhavenProvider) GetProviderIcon() fyne.Resource {
	return nil // Use default for now
}

func (p *WallhavenProvider) getWebURL(apiURL string) *url.URL {
	urlStr := strings.Replace(apiURL, "https://wallhaven.cc/api/v1/search?", "https://wallhaven.cc/search?", 1)
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}
	return u
}
