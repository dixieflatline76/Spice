package pexels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	_ "embed"

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

//go:embed Pexels.png
var iconData []byte

// PexelsProvider implements ImageProvider for Pexels.
type PexelsProvider struct {
	cfg        *wallpaper.Config
	httpClient *http.Client
	testToken  string
}

// SetTokenForTesting sets a token for testing purposes, overriding the config.
func (p *PexelsProvider) SetTokenForTesting(token string) {
	p.testToken = token
}

func init() {
	wallpaper.RegisterProvider("Pexels", func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewPexelsProvider(cfg, client)
	})
}

// NewPexelsProvider creates a new PexelsProvider.
func NewPexelsProvider(cfg *wallpaper.Config, client *http.Client) *PexelsProvider {
	return &PexelsProvider{
		cfg:        cfg,
		httpClient: client,
	}
}

func (p *PexelsProvider) Name() string {
	return "Pexels"
}

func (p *PexelsProvider) Type() provider.ProviderType {
	return provider.TypeOnline
}

func (p *PexelsProvider) HomeURL() string {
	return "https://www.pexels.com"
}

// Regex patterns for Pexels URLs
var (
	// Matches /search/{query}/
	pexelsSearchRegex = regexp.MustCompile(`^https?://(?:www\.)?pexels\.com/search/([^/?#]+)/?`)
	// Matches /collections/{slug}-{id}/
	pexelsCollectionRegex = regexp.MustCompile(`^https?://(?:www\.)?pexels\.com/collections/.*-([a-zA-Z0-9]+)/?$`)
)

// ParseURL converts a web URL to an API URL.
func (p *PexelsProvider) ParseURL(webURL string) (string, error) {
	u, err := url.Parse(webURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if !strings.Contains(u.Host, "pexels.com") {
		return "", fmt.Errorf("not a Pexels URL")
	}

	// Clean up path
	// Pexels web URLs often look like https://www.pexels.com/search/nature/

	if pexelsSearchRegex.MatchString(webURL) {
		matches := pexelsSearchRegex.FindStringSubmatch(webURL)
		if len(matches) < 2 {
			return "", fmt.Errorf("could not extract query from search URL")
		}
		query, err := url.QueryUnescape(matches[1])
		if err != nil {
			return "", fmt.Errorf("failed to decode query from URL: %w", err)
		}

		apiURL, _ := url.Parse(PexelsAPISearchURL)
		q := apiURL.Query()
		q.Set("query", query)

		// Preserve filters from the original web URL
		originalQuery := u.Query()
		if val := originalQuery.Get("orientation"); val != "" {
			q.Set("orientation", val)
		}
		if val := originalQuery.Get("size"); val != "" {
			q.Set("size", val)
		}
		if val := originalQuery.Get("color"); val != "" {
			q.Set("color", val)
		}

		apiURL.RawQuery = q.Encode()
		return apiURL.String(), nil
	}

	if pexelsCollectionRegex.MatchString(webURL) {
		matches := pexelsCollectionRegex.FindStringSubmatch(webURL)
		if len(matches) < 2 {
			return "", fmt.Errorf("could not extract collection ID from URL")
		}
		collectionID := matches[1]
		return fmt.Sprintf(PexelsAPICollectionURL, collectionID), nil
	}

	return "", fmt.Errorf("unsupported Pexels URL format")
}

// FetchImages fetches images from the Pexels API.
func (p *PexelsProvider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL: %w", err)
	}

	q := u.Query()
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", "30") // Default to 30 images
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	apiKey := p.testToken
	if apiKey == "" {
		apiKey = p.cfg.GetPexelsAPIKey()
	}
	if apiKey == "" {
		return nil, fmt.Errorf("pexels API key is missing")
	}
	req.Header.Set("Authorization", apiKey)

	log.Debugf("Fetching Pexels images from: %s", u.String())

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Pexels API Error: %s", string(body))
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var images []provider.Image

	// Determine response type based on URL path
	if strings.Contains(u.Path, "/search") {
		var searchResp PexelsSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
			return nil, fmt.Errorf("failed to decode search response: %w", err)
		}
		for _, photo := range searchResp.Photos {
			images = append(images, p.mapPexelsImage(photo))
		}
	} else if strings.Contains(u.Path, "/collections") {
		var collectionResp PexelsCollectionResponse
		if err := json.NewDecoder(resp.Body).Decode(&collectionResp); err != nil {
			return nil, fmt.Errorf("failed to decode collection response: %w", err)
		}
		for _, photo := range collectionResp.Media {
			if photo.Type == "Photo" { // Ensure we only get photos, though 'media' suggests mixed
				// Wait, Pexels 'media' field in collections might have videos too?
				// The API docs say "media": [ ... ]. Checking documentation or assumption.
				// For safety, let's assume it matches PexelsPhoto structure if type is Photo.
				// Actually, PexelsCollectionResponse usually returns 'media' array.
				// Let's implement robustly.
				images = append(images, p.mapPexelsImage(photo))
			}
		}
	} else {
		return nil, fmt.Errorf("unknown Pexels API endpoint")
	}

	if len(images) == 0 {
		log.Printf("Pexels query returned 0 images for URL: %s", u.String())
	} else {
		log.Debugf("Found %d images from Pexels", len(images))
	}

	return images, nil
}

// EnrichImage is a no-op for Pexels as attribution is included in search results.
func (p *PexelsProvider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

func (p *PexelsProvider) mapPexelsImage(photo PexelsPhoto) provider.Image {
	// Prefer 'original' or 'large2x' depending on need. 'original' is best for wallpapers.
	imagePath := photo.Src.Original
	if imagePath == "" {
		imagePath = photo.Src.Large2x
	}

	return provider.Image{
		ID:          strconv.Itoa(photo.ID),
		Path:        imagePath,
		ViewURL:     photo.URL,
		Attribution: photo.Photographer,
		Provider:    p.Name(),
		FileType:    "image/jpeg", // Pexels primarily serves JPEGs
		// Pexels doesn't require a specific download trigger URL like Unsplash
	}
}

// Pexels JSON Structures

type PexelsSearchResponse struct {
	TotalResults int           `json:"total_results"`
	Page         int           `json:"page"`
	PerPage      int           `json:"per_page"`
	Photos       []PexelsPhoto `json:"photos"`
	NextPage     string        `json:"next_page"`
}

type PexelsCollectionResponse struct {
	ID    string        `json:"id"`
	Media []PexelsPhoto `json:"media"` // Collections return 'media' list
}

type PexelsPhoto struct {
	ID           int       `json:"id"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	URL          string    `json:"url"`
	Photographer string    `json:"photographer"`
	Src          PexelsSrc `json:"src"`
	Type         string    `json:"type"` // "Photo" or "Video"
}

type PexelsSrc struct {
	Original  string `json:"original"`
	Large2x   string `json:"large2x"`
	Large     string `json:"large"`
	Medium    string `json:"medium"`
	Small     string `json:"small"`
	Portrait  string `json:"portrait"`
	Landscape string `json:"landscape"`
	Tiny      string `json:"tiny"`
}

// --- UI Integration ---

func (p *PexelsProvider) Title() string {
	return "Pexels"
}

func (p *PexelsProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	pexHeader := container.NewVBox()

	// Pexels API Key
	pexURL, _ := url.Parse("https://www.pexels.com/api/key/")
	pexelsAPIKeyConfig := setting.TextEntrySettingConfig{
		Name:          "pexelsAPIKey",
		InitialValue:  p.cfg.GetPexelsAPIKey(),
		PlaceHolder:   "Enter your Pexels API Key",
		Label:         sm.CreateSettingTitleLabel("Pexels API Key:"),
		HelpContent:   widget.NewHyperlink("Get a free API key from Pexels.", pexURL),
		Validator:     validation.NewRegexp(PexelsAPIKeyRegexp, "Invalid API Key format (56 characters)"),
		NeedsRefresh:  true,
		DisplayStatus: true,
	}
	pexelsAPIKeyConfig.ApplyFunc = func(s string) {
		p.cfg.SetPexelsAPIKey(s)
		pexelsAPIKeyConfig.InitialValue = s
	}
	sm.CreateTextEntrySetting(&pexelsAPIKeyConfig, pexHeader)

	// Clear Pexels Key Button
	clearPexKeyBtn := widget.NewButton("Clear API Key", func() {
		dialog.NewConfirm("Clear API Key", "Are you sure you want to clear the Pexels API Key?", func(b bool) {
			if b {
				p.cfg.SetPexelsAPIKey("")
				pexelsAPIKeyConfig.InitialValue = ""
				// NotifyUser logic skipped as we don't have access to manager
			}
		}, sm.GetSettingsWindow()).Show()
	})
	pexHeader.Add(clearPexKeyBtn)

	return pexHeader
}

func (p *PexelsProvider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	imgQueryList := p.createImgQueryList(sm)
	sm.RegisterRefreshFunc(imgQueryList.Refresh)

	// Create standardized Add Query Config
	onAdded := func() {
		imgQueryList.Refresh()
	}

	addQueryCfg := wallpaper.AddQueryConfig{
		Title:           "New Pexels Query",
		URLPlaceholder:  "Pexels Search URL (e.g. https://www.pexels.com/search/nature/)",
		URLValidator:    PexelsURLRegexp,
		URLErrorMsg:     "Invalid Pexels URL (search or collection)",
		DescPlaceholder: "Add a description",
		DescValidator:   PexelsDescRegexp,
		DescErrorMsg:    fmt.Sprintf("Description must be between 5 and %d alpha numeric characters long", wallpaper.MaxDescLength),
		ValidateFunc: func(url, desc string) error {
			// Validate string using Pexels specific logic
			// Pexels regex validation happens via InputValidator, so we just check duplicates here

			// Check for duplicates
			queryID := wallpaper.GenerateQueryID(url)
			if p.cfg.IsDuplicateID(queryID) {
				return errors.New("duplicate query: this URL already exists")
			}
			return nil
		},
		AddHandler: func(desc, url string, active bool) (string, error) {
			return p.cfg.AddPexelsQuery(desc, url, active)
		},
	}

	// Create "Add" Button using standardized helper
	addButton := wallpaper.CreateAddQueryButton(
		"Add Pexels Search",
		sm,
		addQueryCfg,
		onAdded,
	)

	header := container.NewVBox()
	header.Add(sm.CreateSettingTitleLabel("Pexels Queries"))
	header.Add(sm.CreateSettingDescriptionLabel("Manage your Pexels image queries here."))
	// Auto-open if pending URL exists
	if pendingUrl != "" {
		// Open dialog with pre-filled URL and empty description
		wallpaper.OpenAddQueryDialog(sm, addQueryCfg, pendingUrl, "", onAdded)
	}

	header.Add(addButton)
	qpContainer := container.NewBorder(header, nil, nil, nil, imgQueryList)
	return qpContainer
}

func (p *PexelsProvider) createImgQueryList(sm setting.SettingsManager) *widget.List {
	pendingState := make(map[string]bool)
	var queryList *widget.List
	queryList = widget.NewList(
		func() int {
			return len(p.cfg.GetPexelsQueries())
		},
		func() fyne.CanvasObject {
			urlLink := widget.NewHyperlink("Placeholder", nil)
			activeCheck := widget.NewCheck("Active", nil)
			deleteButton := widget.NewButton("Delete", nil)

			return container.NewHBox(urlLink, layout.NewSpacer(), activeCheck, deleteButton)
		},
		func(i int, o fyne.CanvasObject) {
			queries := p.cfg.GetPexelsQueries()
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
							err = p.cfg.EnablePexelsQuery(query.ID)
						} else {
							err = p.cfg.DisablePexelsQuery(query.ID)
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
						if err := p.cfg.RemovePexelsQuery(query.ID); err != nil {
							log.Printf("Failed to remove Pexels query: %v", err)
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
func (p *PexelsProvider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("Pexels", iconData)
}
