package wikimedia

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
)

//go:embed Wikimedia.png
var iconData []byte

// CircuitBreaker manages a temporary "open" state when rate limits are hit.
type CircuitBreaker struct {
	mu        sync.RWMutex
	openUntil time.Time
}

func (cb *CircuitBreaker) Trip(duration time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.openUntil = time.Now().Add(duration)
}

func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return time.Now().Before(cb.openUntil)
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.openUntil = time.Time{}
}

func (cb *CircuitBreaker) GetCooldownTime() time.Duration {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	if time.Now().After(cb.openUntil) {
		return 0
	}
	return time.Until(cb.openUntil)
}

// Provider implements ImageProvider for Wikimedia Commons
type Provider struct {
	cfg        *wallpaper.Config
	httpClient *http.Client
	baseURL    string
	cb         *CircuitBreaker

	queryTokens map[string]map[int]url.Values
	mu          sync.Mutex
}

// NewProvider creates a new instance of Provider
func NewProvider(cfg *wallpaper.Config, client *http.Client) *Provider {
	cb := &CircuitBreaker{}

	p := &Provider{
		cfg:         cfg,
		baseURL:     WikimediaBaseURL,
		cb:          cb,
		queryTokens: make(map[string]map[int]url.Values),
	}
	// Wrap the default client with our 429 circuit breaker round tripper
	p.httpClient = &http.Client{
		Transport: &CircuitBreakerRoundTripper{
			Base: client.Transport,
			CB:   cb,
		},
		Timeout: 0, // Timeout managed per-request via context
	}
	return p
}

// ID returns the provider's unique identifier
func (p *Provider) ID() string {
	return "Wikimedia"
}

// Name returns the provider name
func (p *Provider) Name() string {
	return i18n.T("Wikimedia")
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
	return "https://commons.wikimedia.org"
}

func (p *Provider) Title() string {
	return "Wikimedia"
}

func (p *Provider) GetProviderIcon() interface{} {
	return iconData
}

func (p *Provider) IsThrottled() bool {
	return p.cb.IsOpen()
}

// ParseURL determines if the input is a Search term, a Category, or a direct URL.
func (p *Provider) ParseURL(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("input cannot be empty")
	}

	if normalized, ok := p.checkPrefixNormalization(input); ok {
		return normalized, nil
	}

	if normalized, ok, err := p.handleCategoryInput(input); ok || err != nil {
		return normalized, err
	}

	if strings.HasPrefix(input, "http") {
		return p.handleHttpInput(input)
	}

	return "search:" + input, nil
}

func (p *Provider) checkPrefixNormalization(input string) (string, bool) {
	lowerInput := strings.ToLower(input)
	if strings.HasPrefix(lowerInput, "search:") {
		return "search:" + input[7:], true
	}
	if strings.HasPrefix(lowerInput, "category:") {
		return "category:" + input[9:], true
	}
	if strings.HasPrefix(lowerInput, "page:") {
		return "page:" + input[5:], true
	}
	if strings.HasPrefix(lowerInput, "file:") {
		if strings.HasPrefix(lowerInput, "file:file:") {
			return input, true
		}
		return "file:" + input, true
	}
	return "", false
}

func (p *Provider) handleCategoryInput(input string) (string, bool, error) {
	catRegex := regexp.MustCompile(WikimediaCategoryRegexp)
	if !catRegex.MatchString(input) {
		return "", false, nil
	}

	if strings.HasPrefix(input, "http") {
		u, err := url.Parse(input)
		if err != nil {
			return "", false, err
		}
		parts := strings.Split(u.Path, "/")
		if len(parts) > 0 {
			input = parts[len(parts)-1]
		}
	}
	lowerInput := strings.ToLower(input)
	if strings.HasPrefix(lowerInput, "category:") {
		return "category:" + input[9:], true, nil
	}
	if strings.HasPrefix(lowerInput, "category%3a") {
		return "category:" + input[11:], true, nil
	}
	return "category:" + strings.TrimPrefix(input, "Category:"), true, nil
}

func (p *Provider) handleHttpInput(input string) (string, error) {
	domainRegex := regexp.MustCompile(WikimediaDomainRegexp)
	if !domainRegex.MatchString(input) {
		return "", errors.New("invalid Wikimedia URL: must be commons.wikimedia.org")
	}

	u, err := url.Parse(input)
	if err != nil {
		return "", err
	}

	if u.Path == "/w/index.php" || strings.Contains(u.Path, "Special:MediaSearch") {
		searchParam := u.Query().Get("search")
		if searchParam != "" {
			return "search:" + searchParam, nil
		}
	}

	if strings.Contains(u.Path, "/wiki/File:") {
		parts := strings.Split(u.Path, "/wiki/")
		if len(parts) > 1 {
			return "file:" + parts[1], nil
		}
	}

	if strings.Contains(u.Path, "/wiki/") {
		parts := strings.Split(u.Path, "/wiki/")
		if len(parts) > 1 {
			return "page:" + parts[1], nil
		}
	}

	return "", errors.New(i18n.T("only 'Category:', 'File:' or component Search URLs are currently supported directly"))
}

type wikimediaResponse struct {
	BatchComplete string `json:"batchcomplete"`
	Continue      struct {
		GimContinue string `json:"gimcontinue"`
		GcmContinue string `json:"gcmcontinue"`
		GsrOffset   int    `json:"gsroffset"`
		Continue    string `json:"continue"`
	} `json:"continue"`
	Query struct {
		Pages map[string]struct {
			PageID    int    `json:"pageid"`
			Title     string `json:"title"`
			ImageInfo []struct {
				URL         string `json:"url"`
				Width       int    `json:"width"`
				Height      int    `json:"height"`
				ExtMetadata struct {
					ObjectName struct {
						Value string `json:"value"`
					} `json:"ObjectName"`
					Artist struct {
						Value string `json:"value"`
					} `json:"Artist"`
					LicenseShortName struct {
						Value string `json:"value"`
					} `json:"LicenseShortName"`
				} `json:"extmetadata"`
			} `json:"imageinfo"`
		} `json:"pages"`
	} `json:"query"`
	Error *struct {
		Code string `json:"code"`
		Info string `json:"info"`
	} `json:"error"`
}

func (p *Provider) WithResolution(query string, width, height int) string {
	constraint := fmt.Sprintf(" filew:>%d fileh:>%d", width, height)
	if strings.HasPrefix(query, "search:") {
		return query + constraint
	}
	if strings.HasPrefix(query, "category:") {
		catName := strings.TrimPrefix(query, "category:")
		return fmt.Sprintf("search:incategory:\"%s\"%s", catName, constraint)
	}
	return query
}

func (p *Provider) FetchImages(ctx context.Context, query string, page int) ([]provider.Image, error) {
	var allImages []provider.Image
	isPageQuery := strings.HasPrefix(query, "page:")

	p.mu.Lock()
	if p.queryTokens[query] == nil {
		p.queryTokens[query] = make(map[int]url.Values)
	}

	var continueParams url.Values
	if page > 1 {
		tokens, ok := p.queryTokens[query][page]
		if !ok {
			p.mu.Unlock()
			return nil, nil
		}
		continueParams = tokens
	}
	p.mu.Unlock()

	var apiURL string
	var params url.Values

	if strings.HasPrefix(query, "category:") {
		catTitle := strings.TrimPrefix(query, "category:")
		apiURL = p.baseURL
		params = url.Values{}
		params.Set("action", "query")
		params.Set("generator", "categorymembers")
		params.Set("gcmtitle", "Category:"+catTitle)
		params.Set("gcmtype", "file")
		params.Set("gcmlimit", "100")
		params.Set("prop", "imageinfo")
		params.Set("iiprop", "url|size|extmetadata")
		params.Set("format", "json")
	} else if strings.HasPrefix(query, "search:") {
		searchTerm := strings.TrimPrefix(query, "search:")
		apiURL = p.baseURL
		params = url.Values{}
		params.Set("action", "query")
		params.Set("generator", "search")
		params.Set("gsrsearch", searchTerm)
		params.Set("gsrnamespace", "6")
		params.Set("gsrlimit", "100")
		params.Set("prop", "imageinfo")
		params.Set("iiprop", "url|size|extmetadata")
		params.Set("format", "json")
	} else if strings.HasPrefix(query, "file:") {
		fileTitle := strings.TrimPrefix(query, "file:")
		apiURL = p.baseURL
		params = url.Values{}
		params.Set("action", "query")
		params.Set("titles", fileTitle)
		params.Set("prop", "imageinfo")
		params.Set("iiprop", "url|size|extmetadata")
		params.Set("format", "json")
	} else if strings.HasPrefix(query, "page:") {
		pageTitle := strings.TrimPrefix(query, "page:")
		apiURL = p.baseURL
		params = url.Values{}
		params.Set("action", "query")
		params.Set("titles", pageTitle)
		params.Set("generator", "images")
		params.Set("gimlimit", "200")
		params.Set("prop", "imageinfo")
		params.Set("iiprop", "url|size|extmetadata")
		params.Set("format", "json")
	} else {
		return nil, errors.New("unknown query format")
	}

	for k, v := range continueParams {
		params.Set(k, v[0])
	}

	fullURL := apiURL + "?" + params.Encode()
	var result wikimediaResponse
	err := p.doWithRetry(ctx, fullURL, &result)
	if err != nil {
		return nil, err
	}

	for _, pageData := range result.Query.Pages {
		if len(pageData.ImageInfo) == 0 {
			continue
		}
		info := pageData.ImageInfo[0]
		lowerURL := strings.ToLower(info.URL)
		if strings.HasSuffix(lowerURL, ".svg") || strings.HasSuffix(lowerURL, ".gif") ||
			strings.HasSuffix(lowerURL, ".pdf") || strings.HasSuffix(lowerURL, ".ogg") ||
			strings.HasSuffix(lowerURL, ".ogv") || strings.HasSuffix(lowerURL, ".webm") ||
			strings.HasSuffix(lowerURL, ".tif") || strings.HasSuffix(lowerURL, ".tiff") {
			continue
		}

		if isPageQuery && info.Width > 0 && info.Height > 0 {
			aspect := float64(info.Width) / float64(info.Height)
			if aspect < 1.1 {
				continue
			}
		}

		artist := sanitizeAttribution(info.ExtMetadata.Artist.Value)
		if artist == "" {
			artist = "Unknown"
		}
		attribution := artist + " (" + sanitizeAttribution(info.ExtMetadata.LicenseShortName.Value) + ")"

		allImages = append(allImages, provider.Image{
			ID:          strconv.Itoa(pageData.PageID),
			Provider:    p.ID(),
			Path:        info.URL,
			ViewURL:     info.URL,
			Attribution: attribution,
			FileType:    "image/jpeg",
			Width:       info.Width,
			Height:      info.Height,
		})
	}

	if result.Continue.Continue != "" {
		nextParams := url.Values{}
		nextParams.Set("continue", result.Continue.Continue)
		if result.Continue.GimContinue != "" {
			nextParams.Set("gimcontinue", result.Continue.GimContinue)
		}
		if result.Continue.GcmContinue != "" {
			nextParams.Set("gcmcontinue", result.Continue.GcmContinue)
		}
		if result.Continue.GsrOffset > 0 {
			nextParams.Set("gsroffset", strconv.Itoa(result.Continue.GsrOffset))
		}

		p.mu.Lock()
		p.queryTokens[query][page+1] = nextParams
		p.mu.Unlock()
	}

	return allImages, nil
}

func (p *Provider) doWithRetry(ctx context.Context, fullURL string, target interface{}) error {
	return p.doRequest(ctx, fullURL, target)
}

func (p *Provider) doRequest(ctx context.Context, fullURL string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", WikimediaUserAgent)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return err
	}
	return nil
}

func (p *Provider) GetClient() *http.Client {
	return p.httpClient
}

const (
	WikimediaAPIPacing       = 5 * time.Second
	WikimediaMediaPacing     = 60 * time.Second
	WikimediaDefaultCooldown = 15 * time.Minute
)

type CircuitBreakerRoundTripper struct {
	Base http.RoundTripper
	CB   *CircuitBreaker
}

func (t *CircuitBreakerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.CB.IsOpen() {
		return nil, fmt.Errorf("wikimedia circuit breaker: open")
	}

	var resp *http.Response
	var err error

	if t.Base != nil {
		resp, err = t.Base.RoundTrip(req)
	} else {
		resp, err = http.DefaultTransport.RoundTrip(req)
	}

	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 429 {
		resp.Body.Close()
		coolDown := WikimediaDefaultCooldown
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if seconds, err := strconv.Atoi(ra); err == nil {
				coolDown = time.Duration(seconds) * time.Second
			}
		}
		t.CB.Trip(coolDown)
		return nil, fmt.Errorf("wikimedia circuit breaker: rate limited (429)")
	}

	return resp, nil
}

func (p *Provider) GetAPIPacing() time.Duration {
	return WikimediaAPIPacing
}

func (p *Provider) GetProcessPacing() time.Duration {
	return WikimediaMediaPacing
}

func (p *Provider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

func (p *Provider) GetDownloadHeaders() map[string]string {
	return map[string]string{
		"User-Agent": WikimediaUserAgent,
		"Referer":    "https://github.com/dixieflatline76/Spice",
	}
}

// --- UI Integration (Pure Go) ---

// CreateSettingsPanel returns the declarative UI for Wikimedia settings.
func (p *Provider) CreateSettingsPanel(_ setting.SettingsManager) *schema.PanelSchema {
	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title: i18n.T("Wikimedia Commons"),
				Items: []schema.ItemSchema{
					schema.LabelItem{
						Text:       i18n.T("Wikimedia Commons is a media file repository making public domain and freely-licensed educational media content available to everyone."),
						Importance: schema.ImportanceLow,
					},
					schema.HyperlinkItem{
						Text: i18n.T("Donate to Wikimedia"),
						URL:  "https://donate.wikimedia.org/",
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
				Title:       i18n.T("Wikimedia Queries"),
				Description: i18n.T("Manage your Wikimedia image queries here."),
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
						GetDisplayURL: func(q schema.Query) *url.URL {
							return p.getDisplayURL(wallpaper.ImageQuery{URL: q.URL})
						},
					},
				},
			},
		},
	}
}


// Internal helper for use by the Fyne shim
func (p *Provider) validateQueryInternal(term, desc string) error {
	if len(desc) < 5 {
		return errors.New(i18n.T("description too short"))
	}
	if len(desc) > wallpaper.MaxDescLength {
		return errors.New(i18n.T("description too long"))
	}
	normalized, err := p.ParseURL(term)
	if err != nil {
		return err
	}
	id := wallpaper.GenerateQueryID(p.ID() + ":" + normalized)
	if p.cfg.IsDuplicateID(id) {
		return errors.New(i18n.T("duplicate query"))
	}
	return nil
}

func (p *Provider) getDisplayURL(q wallpaper.ImageQuery) *url.URL {
	queryURL := q.URL
	lowerURL := strings.ToLower(queryURL)
	var displayURL string
	if strings.HasPrefix(lowerURL, "category:") {
		catName := queryURL[9:]
		displayURL = "https://commons.wikimedia.org/wiki/Category:" + url.PathEscape(catName)
	} else if strings.HasPrefix(lowerURL, "search:") {
		displayURL = "https://commons.wikimedia.org/w/index.php?search=" + url.QueryEscape(queryURL[7:])
	} else if strings.HasPrefix(lowerURL, "page:") {
		pageName := queryURL[5:]
		displayURL = "https://commons.wikimedia.org/wiki/" + pageName
	} else {
		displayURL = queryURL
	}
	if u, err := url.Parse(displayURL); err == nil && u.Scheme != "" {
		return u
	}
	return nil
}

func init() {
	wallpaper.RegisterProvider("Wikimedia", func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewProvider(cfg, client)
	})
}

func sanitizeAttribution(input string) string {
	re := regexp.MustCompile("<[^>]*>")
	output := re.ReplaceAllString(input, "")
	reSpace := regexp.MustCompile(`\s+`)
	output = reSpace.ReplaceAllString(output, " ")
	return strings.TrimSpace(output)
}
