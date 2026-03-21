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

func (p *PexelsProvider) ID() string {
	return "Pexels"
}

func (p *PexelsProvider) Name() string {
	return i18n.T("Pexels")
}

func (p *PexelsProvider) Type() provider.ProviderType {
	return provider.TypeOnline
}

func (p *PexelsProvider) GetAttributionType() provider.AttributionType {
	return provider.AttributionBy
}

func (p *PexelsProvider) SupportsUserQueries() bool {
	return true
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

	// Idempotency: if it's already an API URL, return as is (but clean up if needed)
	if strings.Contains(u.Host, "api.pexels.com") {
		return webURL, nil
	}

	if pexelsSearchRegex.MatchString(webURL) {
		return parsePexelsSearchURL(webURL, u)
	}

	if pexelsCollectionRegex.MatchString(webURL) {
		matches := pexelsCollectionRegex.FindStringSubmatch(webURL)
		if len(matches) < 2 {
			return "", fmt.Errorf("could not extract collection ID from URL")
		}
		collectionID := matches[1]
		return fmt.Sprintf(wallpaper.PexelsAPICollectionURL, collectionID), nil
	}

	return "", fmt.Errorf("unsupported Pexels URL format")
}

func parsePexelsSearchURL(webURL string, u *url.URL) (string, error) {
	matches := pexelsSearchRegex.FindStringSubmatch(webURL)
	if len(matches) < 2 {
		return "", fmt.Errorf("could not extract query from search URL")
	}
	query, err := url.QueryUnescape(matches[1])
	if err != nil {
		return "", fmt.Errorf("failed to decode query from URL: %w", err)
	}

	apiURL, _ := url.Parse(wallpaper.PexelsAPISearchURL)
	q := u.Query()
	q.Set("query", query)

	// Map resolution info to Pexels 'size'
	// We support both browser (min_width/height) and normalized (res/resolution) formats
	res := q.Get("res")
	if res == "" {
		res = q.Get("resolution")
	}

	var minWidth, minHeight int
	if res != "" {
		parts := strings.Split(res, "x")
		if len(parts) == 2 {
			minWidth, _ = strconv.Atoi(parts[0])
			minHeight, _ = strconv.Atoi(parts[1])
		}
	} else {
		minWidth, _ = strconv.Atoi(q.Get("min_width"))
		minHeight, _ = strconv.Atoi(q.Get("min_height"))
	}

	if minWidth > 0 && minHeight > 0 {
		// size can be: large (24MP), medium (12MP), small (4MP)
		mp := (float64(minWidth) * float64(minHeight)) / 1000000.0
		size := "small"
		if mp > 12 {
			size = "large"
		} else if mp > 4 {
			size = "medium"
		}
		q.Set("size", size)

		// Store normalized resolution for Spice internal use
		q.Set("res", fmt.Sprintf("%dx%d", minWidth, minHeight))
	}

	// Remove pagination and web-only parameters
	q.Del("page")
	q.Del("per_page")
	q.Del("min_width")
	q.Del("min_height")
	q.Del("resolution") // Use normalized 'res'

	apiURL.RawQuery = q.Encode()
	return apiURL.String(), nil
}

// NormalizeURL converts a Pexels web URL to a more compact/standardized format for Spice.
// It combines min_width and min_height into a single 'res' parameter.
func (p *PexelsProvider) NormalizeURL(webURL string) string {
	u, err := url.Parse(webURL)
	if err != nil {
		return webURL
	}

	if !strings.Contains(u.Host, "pexels.com") || strings.Contains(u.Host, "api.pexels.com") {
		return webURL
	}

	q := u.Query()
	minW := q.Get("min_width")
	minH := q.Get("min_height")
	if minW != "" && minH != "" {
		q.Set("res", minW+"x"+minH)
		q.Del("min_width")
		q.Del("min_height")
		u.RawQuery = q.Encode()
		return u.String()
	}

	return webURL
}

// WithResolution adds resolution constraints to the Pexels API URL.
func (p *PexelsProvider) WithResolution(apiURL string, width, height int) string {
	u, err := url.Parse(apiURL)
	if err != nil {
		return apiURL
	}
	q := u.Query()

	// Only set if not already present or if we want to override
	q.Set("res", fmt.Sprintf("%dx%d", width, height))

	// Re-map size based on new resolution
	mp := (float64(width) * float64(height)) / 1000000.0
	size := "small"
	if mp > 12 {
		size = "large"
	} else if mp > 4 {
		size = "medium"
	}
	q.Set("size", size)

	u.RawQuery = q.Encode()
	return u.String()
}

// FetchImages fetches images from the Pexels API.
func (p *PexelsProvider) FetchImages(ctx context.Context, apiURL string, page int) ([]provider.Image, error) {
	// Robustness: Check if we have a web URL and convert it on the fly
	if strings.Contains(apiURL, "www.pexels.com") || strings.Contains(apiURL, "pexels.com/search") || strings.Contains(apiURL, "pexels.com/collections") {
		converted, err := p.ParseURL(apiURL)
		if err == nil {
			log.Printf("Pexels: Converted legacy Web URL to API URL: %s", converted)
			apiURL = converted
		} else {
			log.Printf("Pexels: Warning - Failed to convert URL %s: %v", apiURL, err)
		}
	}

	fullURL, err := p.buildAPIURL(apiURL, page)
	if err != nil {
		return nil, err
	}

	resp, err := p.executeRequest(ctx, fullURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 500 {
			log.Printf("Pexels API Error (%d): %s...", resp.StatusCode, string(body[:500]))
		} else {
			log.Printf("Pexels API Error (%d): %s", resp.StatusCode, string(body))
		}
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	images, err := p.parseResponse(resp.Body, fullURL)
	if err != nil {
		return nil, err
	}

	if len(images) == 0 {
		log.Printf("Pexels query returned 0 images for URL: %s", fullURL)
	} else {
		log.Debugf("Found %d images from Pexels", len(images))
	}

	return images, nil
}

func (p *PexelsProvider) buildAPIURL(apiURL string, page int) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", fmt.Errorf("invalid API URL: %w", err)
	}

	q := u.Query()
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", "30") // Default to 30 images
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func (p *PexelsProvider) executeRequest(ctx context.Context, fullURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
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

	log.Debugf("Fetching Pexels images from: %s", fullURL)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

func (p *PexelsProvider) parseResponse(body io.Reader, urlStr string) ([]provider.Image, error) {
	var images []provider.Image

	// Determine response type based on URL path
	if strings.Contains(urlStr, "/search") {
		var searchResp PexelsSearchResponse
		if err := json.NewDecoder(body).Decode(&searchResp); err != nil {
			return nil, fmt.Errorf("failed to decode search response: %w", err)
		}
		for _, photo := range searchResp.Photos {
			images = append(images, p.mapPexelsImage(photo))
		}
	} else if strings.Contains(urlStr, "/collections") {
		var collectionResp PexelsCollectionResponse
		if err := json.NewDecoder(body).Decode(&collectionResp); err != nil {
			return nil, fmt.Errorf("failed to decode collection response: %w", err)
		}
		for _, photo := range collectionResp.Media {
			if photo.Type == "Photo" { // Ensure we only get photos
				images = append(images, p.mapPexelsImage(photo))
			}
		}
	} else {
		return nil, fmt.Errorf("unknown Pexels API endpoint")
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
		Provider:    p.ID(),
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

	var pexKeyBtn *widget.Button

	// Pexels API Key
	pexURL, _ := url.Parse("https://www.pexels.com/api/key/")
	pexelsAPIKeyConfig := setting.TextEntrySettingConfig{
		Name:          "pexelsAPIKey",
		InitialValue:  p.cfg.GetPexelsAPIKey(),
		PlaceHolder:   i18n.T("Enter your Pexels API Key"),
		Label:         sm.CreateSettingTitleLabel(i18n.T("Pexels API Key:")),
		HelpContent:   widget.NewHyperlink(i18n.T("Get a free API key from Pexels."), pexURL),
		Validator:     validation.NewRegexp(wallpaper.PexelsAPIKeyRegexp, i18n.T("Invalid API Key format (56 characters)")),
		NeedsRefresh:  true,
		DisplayStatus: true,
		IsPassword:    true,
		EnabledIf: func() bool {
			currentValue := sm.GetValue("pexelsAPIKey")
			if currentValue == nil {
				return true
			}
			baselineValue := sm.GetBaseline("pexelsAPIKey")
			if baselineValue == nil {
				baselineValue = ""
			}

			curr := currentValue.(string)
			base := baselineValue.(string)

			// Enabled if empty OR if we're currently editing/clearing (diff from baseline)
			return curr == "" || curr != base
		},
		OnChanged: func(s string) {
			if pexKeyBtn == nil {
				return
			}
			base := sm.GetBaseline("pexelsAPIKey").(string)
			if s == base {
				if s == "" {
					pexKeyBtn.Hide()
				} else {
					pexKeyBtn.SetText(i18n.T("Clear API Key"))
					pexKeyBtn.Importance = widget.DangerImportance
					pexKeyBtn.Show()
				}
			} else {
				pexKeyBtn.SetText(i18n.T("Verify & Connect"))
				pexKeyBtn.Importance = widget.HighImportance
				if s == "" {
					pexKeyBtn.Hide()
				} else {
					pexKeyBtn.Show()
				}
			}
			pexKeyBtn.Refresh()
		},
	}

	// Dynamic update helper for API key button
	refreshPexelsUI := func() {
		curr := sm.GetValue("pexelsAPIKey").(string)
		pexelsAPIKeyConfig.OnChanged(curr)
	}

	sm.CreateTextEntrySetting(&pexelsAPIKeyConfig, pexHeader)

	// Pexels API Key Action Button
	pexKeyBtn = widget.NewButton(i18n.T("Verify & Connect"), func() {
		currKey := sm.GetValue("pexelsAPIKey").(string)
		baseKey := sm.GetBaseline("pexelsAPIKey").(string)

		if currKey == baseKey && currKey != "" {
			// State: Clear
			dialog.NewConfirm(i18n.T("Clear API Key"), i18n.T("Are you sure you want to clear the Pexels API Key?"), func(b bool) {
				if b {
					sm.SetValue("pexelsAPIKey", "")
					p.cfg.SetPexelsAPIKey("")
					sm.SeedBaseline("pexelsAPIKey", "")
					sm.GetCheckAndEnableApplyFunc()()
					sm.Refresh()
				}
			}, sm.GetSettingsWindow()).Show()
			return
		}

		// State: Verify & Connect
		pexKeyBtn.Disable()
		pexKeyBtn.SetText(i18n.T("Verifying..."))
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := CheckPexelsAPIKeyWithContext(ctx, currKey)
			fyne.Do(func() {
				pexKeyBtn.Enable()
				if err != nil {
					dialog.ShowError(err, sm.GetSettingsWindow())
					pexKeyBtn.SetText(i18n.T("Verify & Connect"))
					return
				}
				// Success! Save immediately and lock
				p.cfg.SetPexelsAPIKey(currKey)
				sm.SeedBaseline("pexelsAPIKey", currKey)
				sm.Refresh()
				refreshPexelsUI()
			})
		}()
	})
	pexKeyBtn.Importance = widget.HighImportance
	// Initial visibility
	initialKey := p.cfg.GetPexelsAPIKey()
	if initialKey == "" {
		pexKeyBtn.Hide()
	} else {
		pexKeyBtn.SetText(i18n.T("Clear API Key"))
		pexKeyBtn.Importance = widget.DangerImportance
	}
	pexHeader.Add(pexKeyBtn)

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
		Title:           i18n.T("New Pexels Query"),
		URLPlaceholder:  i18n.T("Pexels Search URL (e.g. https://www.pexels.com/search/nature/)"),
		URLValidator:    wallpaper.PexelsURLRegexp,
		URLErrorMsg:     i18n.T("Invalid Pexels URL (search or collection)"),
		DescPlaceholder: i18n.T("Add a description"),
		DescValidator:   wallpaper.PexelsDescRegexp,
		DescErrorMsg:    fmt.Sprintf(i18n.T("Description must be between 5 and %d alpha numeric characters long"), wallpaper.MaxDescLength),
		ValidateFunc: func(url, desc string) error {
			// Validate string using Pexels specific logic
			// Pexels regex validation happens via InputValidator, so we just check duplicates here

			// Check for duplicates
			queryID := wallpaper.GenerateQueryID(p.ID() + ":" + url)
			if p.cfg.IsDuplicateID(queryID) {
				return errors.New(i18n.T("duplicate query: this URL already exists"))
			}
			return nil
		},
		AddHandler: func(desc, url string, active bool) (string, error) {
			// Convert Web URL to API URL before saving
			apiURL, err := p.ParseURL(url)
			if err != nil {
				return "", fmt.Errorf("failed to convert URL: %w", err)
			}
			return p.cfg.AddPexelsQuery(desc, apiURL, active)
		},
	}

	// Create "Add" Button using standardized helper
	addButton := wallpaper.CreateAddQueryButton(
		i18n.T("Add Pexels Search"),
		sm,
		addQueryCfg,
		onAdded,
	)

	header := container.NewVBox()
	header.Add(sm.CreateSettingTitleLabel(i18n.T("Pexels Queries")))
	header.Add(sm.CreateSettingDescriptionLabel(i18n.T("Manage your Pexels image queries here.")))

	header.Add(addButton)

	// Auto-open if pending URL exists
	if pendingUrl != "" {
		// Normalize URL before showing in modal
		normalizedUrl := p.NormalizeURL(pendingUrl)

		fyne.Do(func() {
			// Delay slightly to ensure window is fully ready/shown
			time.Sleep(50 * time.Millisecond)
			// Open dialog with pre-filled URL and empty description
			wallpaper.OpenAddQueryDialog(sm, addQueryCfg, normalizedUrl, "", onAdded)
		})
	}

	return container.NewBorder(header, nil, nil, nil, imgQueryList)
}

func (p *PexelsProvider) createImgQueryList(sm setting.SettingsManager) *widget.List {
	return wallpaper.CreateQueryList(sm, wallpaper.QueryListConfig{
		GetQueries:   p.cfg.GetPexelsQueries,
		EnableQuery:  p.cfg.EnablePexelsQuery,
		DisableQuery: p.cfg.DisablePexelsQuery,
		RemoveQuery:  p.cfg.RemovePexelsQuery,
	})
}

// GetProviderIcon returns the provider's icon for the tray menu.
func (p *PexelsProvider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("Pexels", iconData)
}

// GetAPIPacing implements the PacedProvider interface to space out API calls.
func (p *PexelsProvider) GetAPIPacing() time.Duration {
	return 500 * time.Millisecond
}

// GetProcessPacing implements the PacedProvider interface to space out image downloads.
func (p *PexelsProvider) GetProcessPacing() time.Duration {
	return 800 * time.Millisecond
}
