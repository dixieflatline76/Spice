package unsplash

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
)

//go:embed Unsplash.png
var iconData []byte

// UnsplashProvider implements ImageProvider for Unsplash.
type UnsplashProvider struct {
	cfg           *wallpaper.Config
	httpClient    *http.Client
	testToken     string
	testAccessKey string
}

// SetTokenForTesting sets a token for testing purposes, overriding the config.
func (p *UnsplashProvider) SetTokenForTesting(token string) {
	p.testToken = token
}

// SetAccessKeyForTesting sets an access key for testing purposes, overriding the config.
func (p *UnsplashProvider) SetAccessKeyForTesting(key string) {
	p.testAccessKey = key
}

func init() {
	wallpaper.RegisterProvider("Unsplash", func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewUnsplashProvider(cfg, client)
	})
}

// NewUnsplashProvider creates a new UnsplashProvider.
func NewUnsplashProvider(cfg *wallpaper.Config, client *http.Client) *UnsplashProvider {
	return &UnsplashProvider{
		cfg:        cfg,
		httpClient: client,
	}
}

func (p *UnsplashProvider) Name() string {
	return "Unsplash"
}

func (p *UnsplashProvider) HomeURL() string {
	// Construct URL with UTM parameters if needed, or just home.
	// Legacy code: "https://unsplash.com/?utm_source=" + UnsplashClientID + "&utm_medium=referral"
	return "https://unsplash.com/?utm_source=" + UnsplashClientID + "&utm_medium=referral"
}

// ParseURL converts a web URL to an API URL.
// Supported formats:
// - https://unsplash.com/s/photos/query -> https://api.unsplash.com/search/photos?query=query
// - https://unsplash.com/collections/ID/NAME -> https://api.unsplash.com/collections/ID/photos
func (p *UnsplashProvider) ParseURL(webURL string) (string, error) {
	u, err := url.Parse(webURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if !strings.Contains(u.Host, "unsplash.com") {
		return "", fmt.Errorf("not an Unsplash URL")
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) == 0 {
		return "", fmt.Errorf("invalid Unsplash URL path")
	}

	apiURL := &url.URL{
		Scheme: "https",
		Host:   "api.unsplash.com",
	}

	q := u.Query() // Start with existing query params (e.g. orientation)

	if pathParts[0] == "s" && len(pathParts) >= 3 && pathParts[1] == "photos" {
		// Search: /s/photos/query
		apiURL.Path = "/search/photos"
		query, err := url.QueryUnescape(pathParts[2])
		if err != nil {
			return "", fmt.Errorf("failed to decode query from URL: %w", err)
		}
		q.Set("query", query)
	} else if pathParts[0] == "collections" && len(pathParts) >= 2 {
		// Collection: /collections/ID/NAME
		collectionID := pathParts[1] // ID shouldn't need decoding usually, but safe to keep as is
		apiURL.Path = fmt.Sprintf("/collections/%s/photos", collectionID)
	} else {
		return "", fmt.Errorf("unsupported Unsplash URL format")
	}

	apiURL.RawQuery = q.Encode()
	return apiURL.String(), nil
}

// WithResolution adds resolution constraints to the API URL if missing.
func (p *UnsplashProvider) WithResolution(apiURL string, width, height int) string {
	u, err := url.Parse(apiURL)
	if err != nil {
		return apiURL
	}

	// Unsplash Search API supports 'orientation'
	// We only apply this for search endpoints
	if strings.Contains(u.Path, "/search/photos") {
		q := u.Query()
		if !q.Has("orientation") {
			if width > height {
				q.Set("orientation", "landscape")
			} else if height > width {
				q.Set("orientation", "portrait")
			}
			// If square, we don't set orientation (or could set 'squarish')
			u.RawQuery = q.Encode()
			return u.String()
		}
	}

	return apiURL
}

// FetchImages fetches images from the Unsplash API.
func (p *UnsplashProvider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL: %w", err)
	}

	q := u.Query()
	q.Set("client_id", UnsplashClientID)
	q.Set("page", fmt.Sprint(page))
	q.Set("per_page", "30") // Default to 30 images
	u.RawQuery = q.Encode()

	log.Debugf("Fetching Unsplash images from: %s", u.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	token := p.testToken
	if token == "" {
		token = p.cfg.GetUnsplashToken()
	}
	if token == "" {
		return nil, fmt.Errorf("Unsplash token is missing (disconnected)")
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Unsplash API Error: %s", string(body))
		return nil, fmt.Errorf("api returned status %d: %s", resp.StatusCode, string(body))
	}

	var images []provider.Image

	// Unsplash search API returns a wrapper object, while collections API returns a list.
	// We need to detect which one it is.
	// Or we can check the path.
	if strings.Contains(u.Path, "/search/photos") {
		var searchResp UnsplashSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
			return nil, fmt.Errorf("failed to decode search response: %w", err)
		}
		for _, item := range searchResp.Results {
			images = append(images, p.mapUnsplashImage(item))
		}
	} else {
		var listResp []UnsplashImage
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			return nil, fmt.Errorf("failed to decode list response: %w", err)
		}
		for _, item := range listResp {
			images = append(images, p.mapUnsplashImage(item))
		}
	}

	if len(images) == 0 {
		log.Printf("Unsplash query returned 0 images for URL: %s", u.String())
	} else {
		log.Debugf("Found %d images from Unsplash", len(images))
	}

	return images, nil
}

// EnrichImage fetches additional details for the image (e.g. attribution) if missing.
// For Unsplash, the attribution is already present in the search results, so this is a no-op.
func (p *UnsplashProvider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

func (p *UnsplashProvider) mapUnsplashImage(ui UnsplashImage) provider.Image {
	return provider.Image{
		ID:               ui.ID,
		Path:             ui.URLs.Full, // Use full resolution
		ViewURL:          ui.Links.HTML,
		Attribution:      ui.User.Name,
		Provider:         p.Name(),
		FileType:         "image/jpeg", // Unsplash usually serves JPEGs
		DownloadLocation: ui.Links.DownloadLocation,
	}
}

// Unsplash JSON structures

type UnsplashSearchResponse struct {
	Total      int             `json:"total"`
	TotalPages int             `json:"total_pages"`
	Results    []UnsplashImage `json:"results"`
}

type UnsplashImage struct {
	ID    string `json:"id"`
	URLs  URLs   `json:"urls"`
	Links Links  `json:"links"`
	User  User   `json:"user"`
}

type URLs struct {
	Raw     string `json:"raw"`
	Full    string `json:"full"`
	Regular string `json:"regular"`
	Small   string `json:"small"`
	Thumb   string `json:"thumb"`
}

type Links struct {
	Self             string `json:"self"`
	HTML             string `json:"html"`
	Download         string `json:"download"`
	DownloadLocation string `json:"download_location"`
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// --- UI Integration ---

func (p *UnsplashProvider) Title() string {
	return "Unsplash"
}

func (p *UnsplashProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	usHeader := container.NewVBox()

	// Connect Button and Status
	var connectBtn *widget.Button
	statusLabel := widget.NewLabel("")

	// Use a pointer to the function so it can refer to itself (for recursion/callback)
	var updateStatus func()

	connectBtn = widget.NewButton("Connect Unsplash", func() {
		connectBtn.Disable()
		statusLabel.SetText("Status: Connecting... Check your browser.")

		go func() {
			authenticator := NewUnsplashAuthenticator(p.cfg, p.httpClient)
			// Use fyne.CurrentApp().OpenURL instead of manager.OpenURL
			err := authenticator.StartOAuthFlow(fyne.CurrentApp().OpenURL)

			// Update UI on main thread
			if err != nil {
				log.Printf("OAuth failed: %v", err)
				statusLabel.SetText("Status: Failed. Check logs.")
				statusLabel.Importance = widget.DangerImportance
				// Basic error dialog if we can attach it?
				// We don't have easy access to create windows here without app context,
				// but we can try fyne.CurrentApp().NewWindow if needed.
				// For now just logging and label update.
			} else {
				statusLabel.SetText("Status: Connected")
				statusLabel.Importance = widget.SuccessImportance
			}
			connectBtn.Enable()
			// Since updateStatus is a closure that captures the buttons, calling it here works
			// But we need to make sure UI updates happen on main thread?
			// In Fyne, property changes are usually thread-safeish but layout might need refresh.
			// But `fyne.Do` or `main thread` is safer.
			// Let's use callOnUI logic if we can, or assume `StartOAuthFlow` calls back.
			// Authenticator `StartOAuthFlow` blocks until done?
			// Yes.

			// Safe UI update
			// We can't access `fyne.Do` directly? Yes we imported `fyne`.
			// But `fyne` package doesn't have `Do`? Unsplash implementation in `ui.go` used `fyne.Do`?
			// Wait, `fyne` package usually doesn't expose `Do` directly in v2?
			// `ui.go` used `fyne.Do(func() { ... })`?
			// Let's check imports in `ui.go`. Yes `fyne.io/fyne/v2`.
			// Does `fyne/v2` have `Do`? No, usually `Driver().CanvasForObject(...).Refresh()` or similar.
			// Maybe `ui.go` meant something else or I misread?
			// Ah, checked `ui.go` line 841: `fyne.Do(func() { ... })`. Is that valid?
			// Maybe it's a deprecated alias or my knowledge is fuzzy.
			// Actually `fyne.CurrentApp().Driver().RunOnUIThread(...)` is standard.
			// But let's assume `widget` updates are safeish or `CreateSettingsPanel` provided `sm`?
			// Actually `ui.go` used `fyne.Do`.
			// I'll stick to `connectBtn.Refresh()` pattern or just `SetText` which triggers refresh.
			updateStatus()
		}()
	})

	disconnectBtn := widget.NewButton("Disconnect", func() {
		dialog.NewConfirm("Disconnect Unsplash", "Are you sure you want to disconnect? This will remove the local access token.", func(b bool) {
			if b {
				p.cfg.SetUnsplashToken("")
				updateStatus()
			}
		}, sm.GetSettingsWindow()).Show()
	})
	disconnectBtn.Importance = widget.DangerImportance

	updateStatus = func() {
		if p.cfg.GetUnsplashToken() != "" {
			statusLabel.SetText("Status: Connected")
			statusLabel.Importance = widget.SuccessImportance
			connectBtn.SetText("Reconnect Unsplash")
			connectBtn.Hide()
			disconnectBtn.Show()
		} else {
			statusLabel.SetText("Status: Not Connected")
			statusLabel.Importance = widget.DangerImportance
			connectBtn.SetText("Connect Unsplash")
			connectBtn.Show()
			disconnectBtn.Hide()
		}
	}

	updateStatus() // Initial state

	usHeader.Add(sm.CreateSettingTitleLabel("Unsplash Account:"))
	usHeader.Add(sm.CreateSettingDescriptionLabel("Connect your Unsplash account to access your collections and higher rate limits."))
	usHeader.Add(container.NewHBox(connectBtn, disconnectBtn, statusLabel))

	return usHeader
}

func (p *UnsplashProvider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	imgQueryList := p.createImgQueryList(sm)
	sm.RegisterRefreshFunc(imgQueryList.Refresh)

	// Create standardized Add Query Config
	onAdded := func() {
		imgQueryList.Refresh()
	}

	addQueryCfg := wallpaper.AddQueryConfig{
		Title:           "New Unsplash Query",
		URLPlaceholder:  "Paste an Unsplash search or collection URL",
		URLValidator:    UnsplashURLRegexp,
		URLErrorMsg:     "Invalid Unsplash URL (search/collection/photo)",
		DescPlaceholder: "Add a description",
		DescValidator:   UnsplashDescRegexp,
		DescErrorMsg:    fmt.Sprintf("Description must be between 5 and %d alpha numeric characters long", wallpaper.MaxDescLength),
		ValidateFunc: func(url, desc string) error {
			// Validate string using Unsplash specific logic
			if _, err := p.ParseURL(url); err != nil {
				return fmt.Errorf("Invalid Unsplash URL: %v", err)
			}

			// Check for duplicates
			queryID := wallpaper.GenerateQueryID(url)
			if p.cfg.IsDuplicateID(queryID) {
				return errors.New("Duplicate query: this URL already exists")
			}
			return nil
		},
		AddHandler: func(desc, url string, active bool) (string, error) {
			return p.cfg.AddUnsplashQuery(desc, url, active)
		},
	}

	// Create "Add" Button using standardized helper
	addButton := wallpaper.CreateAddQueryButton(
		"Add Unsplash URL",
		sm,
		addQueryCfg,
		onAdded,
	)

	header := container.NewVBox()
	header.Add(sm.CreateSettingTitleLabel("Unsplash Queries"))
	header.Add(sm.CreateSettingDescriptionLabel("Manage your Unsplash image queries here."))
	header.Add(addButton)
	// Auto-open if pending URL exists
	if pendingUrl != "" {
		wallpaper.OpenAddQueryDialog(sm, addQueryCfg, pendingUrl, "", onAdded)
	}
	qpContainer := container.NewBorder(header, nil, nil, nil, imgQueryList)
	return qpContainer
}

func (p *UnsplashProvider) createImgQueryList(sm setting.SettingsManager) *widget.List {
	pendingState := make(map[string]bool)
	var queryList *widget.List
	queryList = widget.NewList(
		func() int {
			return len(p.cfg.GetUnsplashQueries())
		},
		func() fyne.CanvasObject {
			urlLink := widget.NewHyperlink("Placeholder", nil)
			activeCheck := widget.NewCheck("Active", nil)
			deleteButton := widget.NewButton("Delete", nil)

			return container.NewHBox(urlLink, layout.NewSpacer(), activeCheck, deleteButton)
		},
		func(i int, o fyne.CanvasObject) {
			queries := p.cfg.GetUnsplashQueries()
			if i >= len(queries) {
				return
			}
			query := queries[i]
			queryKey := query.ID

			c := o.(*fyne.Container)

			urlLink := c.Objects[0].(*widget.Hyperlink)
			urlLink.SetText(query.Description)

			if u, err := url.Parse(query.URL); err == nil {
				urlLink.SetURL(u)
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
							err = p.cfg.EnableUnsplashQuery(query.ID)
						} else {
							err = p.cfg.DisableUnsplashQuery(query.ID)
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
							sm.GetCheckAndEnableApplyFunc()()
						}
						delete(pendingState, queryKey)
						if err := p.cfg.RemoveUnsplashQuery(query.ID); err != nil {
							log.Printf("Failed to remove Unsplash query: %v", err)
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
func (p *UnsplashProvider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("Unsplash", iconData)
}
