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

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
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

// Removed global state to ensure test isolation.
// CircuitBreaker and semaphores are now managed per-provider instance.

// WikimediaProvider implements ImageProvider for Wikimedia Commons
type WikimediaProvider struct {
	cfg        *wallpaper.Config
	httpClient *http.Client
	baseURL    string
	cb         *CircuitBreaker
	apiSem     chan struct{}
	mediaSem   chan struct{}
}

// NewWikimediaProvider creates a new instance of WikimediaProvider
func NewWikimediaProvider(cfg *wallpaper.Config, client *http.Client) *WikimediaProvider {
	cb := &CircuitBreaker{}
	apiSem := make(chan struct{}, 1)
	mediaSem := make(chan struct{}, 1)

	p := &WikimediaProvider{
		cfg:      cfg,
		baseURL:  WikimediaBaseURL,
		cb:       cb,
		apiSem:   apiSem,
		mediaSem: mediaSem,
	}
	// Wrap the default client with our throttled round tripper
	p.httpClient = &http.Client{
		Transport: &ThrottledRoundTripper{
			Base:     client.Transport,
			Config:   cfg,
			CB:       cb,
			APISem:   apiSem,
			MediaSem: mediaSem,
		},
		Timeout: 0, // Timeout managed per-request via context
	}
	return p
}

// ID returns the provider's unique identifier
func (p *WikimediaProvider) ID() string {
	return "Wikimedia"
}

// Name returns the provider name
func (p *WikimediaProvider) Name() string {
	return i18n.T("Wikimedia")
}

func (p *WikimediaProvider) Type() provider.ProviderType {
	return provider.TypeOnline
}

func (p *WikimediaProvider) SupportsUserQueries() bool {
	return true
}

func (p *WikimediaProvider) HomeURL() string {
	return "https://commons.wikimedia.org"
}

func (p *WikimediaProvider) IsThrottled() bool {
	return p.cb.IsOpen()
}

// ParseURL determines if the input is a Search term, a Category, or a direct URL.
// It normalizes it into a "search URL" for the API.
func (p *WikimediaProvider) ParseURL(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("input cannot be empty")
	}

	if normalized, ok := p.checkPrefixNormalization(input); ok {
		return normalized, nil
	}

	// 1. Check for Category Match (explicit "Category:" prefix via regex)
	if normalized, ok, err := p.handleCategoryInput(input); ok || err != nil {
		return normalized, err
	}

	// 2. Check for Validation of commons.wikimedia.org domain if http is used
	if strings.HasPrefix(input, "http") {
		return p.handleHttpInput(input)
	}

	// 3. Fallback: Treat as Search Term
	return "search:" + input, nil
}

func (p *WikimediaProvider) checkPrefixNormalization(input string) (string, bool) {
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
	// Treat "File:" explicitly as a direct file lookup
	if strings.HasPrefix(lowerInput, "file:") {
		// Fix idempotency: If input is already normalized (e.g. "file:File:Name"), return as is.
		// "file:File:" becomes "file:file:" in lowerInput.
		if strings.HasPrefix(lowerInput, "file:file:") {
			return input, true
		}
		return "file:" + input, true
	}
	return "", false
}

func (p *WikimediaProvider) handleCategoryInput(input string) (string, bool, error) {
	catRegex := regexp.MustCompile(WikimediaCategoryRegexp)
	if !catRegex.MatchString(input) {
		return "", false, nil
	}

	// It's a category. Extract proper title if it's a full URL.
	if strings.HasPrefix(input, "http") {
		// e.g. https://commons.wikimedia.org/wiki/Category:Nature
		u, err := url.Parse(input)
		if err != nil {
			return "", false, err
		}
		// Path is usually /wiki/Category:Name
		parts := strings.Split(u.Path, "/")
		if len(parts) > 0 {
			input = parts[len(parts)-1] // Category:Nature
		}
	}
	// Return internal scheme for category
	// Handle case-insensitive removal of "Category:" or "Category%3A"
	lowerInput := strings.ToLower(input)
	if strings.HasPrefix(lowerInput, "category:") {
		return "category:" + input[9:], true, nil
	}
	if strings.HasPrefix(lowerInput, "category%3a") {
		return "category:" + input[11:], true, nil
	}
	// Should not happen if regex matched but check just in case
	return "category:" + strings.TrimPrefix(input, "Category:"), true, nil
}

func (p *WikimediaProvider) handleHttpInput(input string) (string, error) {
	domainRegex := regexp.MustCompile(WikimediaDomainRegexp)
	if !domainRegex.MatchString(input) {
		return "", errors.New("invalid Wikimedia URL: must be commons.wikimedia.org")
	}

	// Parse standard Search URLs
	// e.g. https://commons.wikimedia.org/w/index.php?search=dachshund&title=Special%3AMediaSearch&type=image
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

	// If the user pasted a direct file URL like "https://commons.wikimedia.org/wiki/File:Foo.jpg"
	if strings.Contains(u.Path, "/wiki/File:") {
		parts := strings.Split(u.Path, "/wiki/")
		if len(parts) > 1 {
			return "file:" + parts[1], nil
		}
	}

	// If the user pasted a generic wiki URL like "https://commons.wikimedia.org/wiki/Commons:Featured_pictures/Astronomy"
	if strings.Contains(u.Path, "/wiki/") {
		parts := strings.Split(u.Path, "/wiki/")
		if len(parts) > 1 {
			// For generic wiki pages, treat as a "Gallery" collection
			return "page:" + parts[1], nil
		}
	}

	// Fallback for unhandled full URLs
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
				URL         string `json:"url"` // Original URL
				Width       int    `json:"width"`
				Height      int    `json:"height"`
				ExtMetadata struct {
					ObjectName struct {
						Value string `json:"value"`
					} `json:"ObjectName"`
					Artist struct {
						Value string `json:"value"` // HTML often
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

// WithResolution adds resolution constraints to the query using CirrusSearch syntax.
// Reference: https://www.mediawiki.org/wiki/Help:CirrusSearch
func (p *WikimediaProvider) WithResolution(query string, width, height int) string {
	// Format constraints: filew:>WIDTH fileh:>HEIGHT
	constraint := fmt.Sprintf(" filew:>%d fileh:>%d", width, height)

	// Case 1: Search Query -> Just append
	if strings.HasPrefix(query, "search:") {
		return query + constraint
	}

	// Case 2: Category Query -> Convert to Search with incategory:""
	// Categories using "categorymembers" generator don't support filew/fileh filtering easily.
	// We convert to a search query which does.
	if strings.HasPrefix(query, "category:") {
		catName := strings.TrimPrefix(query, "category:")
		// Quote the category name to handle spaces safely
		return fmt.Sprintf("search:incategory:\"%s\"%s", catName, constraint)
	}

	// Case 3: File/Page Query -> No resolution constraint needed
	if strings.HasPrefix(query, "file:") || strings.HasPrefix(query, "page:") {
		return query
	}

	// Fallback (unknown format)
	return query
}

// FetchImages fetches images from the API based on the parsed URL.
// It now supports pagination, extension filtering, landscape orientation filtering, and order preservation.
func (p *WikimediaProvider) FetchImages(ctx context.Context, query string, page int) ([]provider.Image, error) {
	var allImages []provider.Image
	continueParams := url.Values{}
	isPageQuery := strings.HasPrefix(query, "page:")

	// Limit to 3 pages of results to avoid excessive API usage
	for i := 0; i < 3; i++ {
		// Respect Wikimedia Policy: Small delay between batches (Interruptible)
		if i > 0 {
			if err := contextSleep(ctx, 500*time.Millisecond); err != nil {
				return nil, err
			}
		}

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

		// Apply pagination tokens
		for k, v := range continueParams {
			params.Set(k, v[0])
		}

		fullURL := apiURL + "?" + params.Encode()
		log.Debugf("Fetching Wikimedia API (Page %d): %s", i+1, fullURL)

		var result wikimediaResponse
		err := p.doWithRetry(ctx, fullURL, &result)
		if err != nil {
			log.Printf("Wikimedia API Request Failed after retries: %v", err)
			return nil, err
		}

		// Extract pages into a stable order (API usually returns them in a map,
		// but generator=images generally follows page order in search results or list results).
		// We'll process them in the order they appear in the result map keys if sorted,
		// but more importantly, we keep the batch-to-batch order.

		// To be safe and mimic visual order, we process the pages.
		// Note: Go map iteration is random, so we should collect and sort by some key or rely on API.
		// However, for generator queries, 'index' or 'sortkey' is often available.
		// Let's at least keep this batch's images together.

		batchCount := 0
		for _, pageData := range result.Query.Pages {
			if len(pageData.ImageInfo) == 0 {
				continue
			}
			info := pageData.ImageInfo[0]

			// Extension Filtering
			lowerURL := strings.ToLower(info.URL)
			if strings.HasSuffix(lowerURL, ".svg") || strings.HasSuffix(lowerURL, ".gif") ||
				strings.HasSuffix(lowerURL, ".pdf") || strings.HasSuffix(lowerURL, ".ogg") ||
				strings.HasSuffix(lowerURL, ".ogv") || strings.HasSuffix(lowerURL, ".webm") ||
				strings.HasSuffix(lowerURL, ".tif") || strings.HasSuffix(lowerURL, ".tiff") {
				continue
			}

			// Orientation Filtering (Landscape only for Gallery Mode)
			if isPageQuery && info.Width > 0 && info.Height > 0 {
				aspect := float64(info.Width) / float64(info.Height)
				if aspect < 1.1 { // Portrait or Square: skip
					log.Debugf("Skipping non-landscape image: %s (Aspect: %.2f)", info.URL, aspect)
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
			batchCount++
		}

		log.Debugf("Page %d: Found %d valid images", i+1, batchCount)

		// Check if we have enough images or if there's no more data
		if len(allImages) >= 50 || result.Continue.Continue == "" {
			break
		}

		// Prepare next page parameters
		continueParams = url.Values{}
		if result.Continue.Continue != "" {
			continueParams.Set("continue", result.Continue.Continue)
		}
		if result.Continue.GimContinue != "" {
			continueParams.Set("gimcontinue", result.Continue.GimContinue)
		}
		if result.Continue.GcmContinue != "" {
			continueParams.Set("gcmcontinue", result.Continue.GcmContinue)
		}
		if result.Continue.GsrOffset > 0 {
			continueParams.Set("gsroffset", strconv.Itoa(result.Continue.GsrOffset))
		}
	}

	log.Debugf("Total valid images gathered: %d", len(allImages))

	if len(allImages) == 0 {
		return nil, nil
	}

	// Limit to reasonable batch size for the app
	if len(allImages) > 30 {
		allImages = allImages[:30]
	}

	return allImages, nil
}

func (p *WikimediaProvider) doWithRetry(ctx context.Context, fullURL string, target interface{}) error {
	return p.doRequest(ctx, fullURL, target)
}

func (p *WikimediaProvider) doRequest(ctx context.Context, fullURL string, target interface{}) error {

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

	// Check for Wikimedia API level errors
	if res, ok := target.(*wikimediaResponse); ok && res.Error != nil {
		log.Printf("Wikimedia API Error: %s - %s", res.Error.Code, res.Error.Info)
		return fmt.Errorf("wikimedia api error: %s", res.Error.Info)
	}

	return nil
}

// GetClient returns our specialized throttled client for downloads.
func (p *WikimediaProvider) GetClient() *http.Client {
	return p.httpClient
}

const (
	WikimediaAPIPacing       = 5 * time.Second
	WikimediaMediaPacing     = 60 * time.Second
	WikimediaDefaultCooldown = 15 * time.Minute
)

// ThrottledRoundTripper handles Wikimedia rate limiting and concurrency.
type ThrottledRoundTripper struct {
	Base         http.RoundTripper
	Config       *wallpaper.Config
	CB           *CircuitBreaker
	APISem       chan struct{}
	MediaSem     chan struct{}
	lastMediaReq time.Time
	lastMediaMu  sync.Mutex
	lastAPIReq   time.Time
	lastAPIMu    sync.Mutex
}

func (t *ThrottledRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// 1. Trace the path (API vs Media)
	reqType := "Media"
	if strings.Contains(req.URL.Path, "/w/api.php") {
		reqType = "API"
	}

	// 2. [DELETED] Inject Authentication (Removed due to inefficiency for media)

	var resp *http.Response
	var err error

	// 3. Acquire lane slot
	var sem chan struct{}
	if reqType == "API" {
		sem = t.APISem
	} else {
		sem = t.MediaSem
	}

	// Pattern: Circuit Breaker
	if t.CB.IsOpen() {
		cooldown := t.CB.GetCooldownTime().Round(time.Second)
		log.Printf("Wikimedia Throttler: Circuit Breaker is OPEN. Skipping request. (Cooldown: %v)", cooldown)
		return nil, fmt.Errorf("wikimedia throttler: circuit breaker is open (%v remaining)", cooldown)
	}

	log.Debugf("Wikimedia Throttler: [%s] Waiting for slot... (Current Load: %d/1)", reqType, len(sem))
	select {
	case sem <- struct{}{}:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}

	// Perform the request and pacing within a protected block
	resp, err = func() (*http.Response, error) {
		defer func() { <-sem }()
		log.Debugf("Wikimedia Throttler: [%s] Slot acquired. Starting request: %s", reqType, req.URL.String())

		// 4. Mandatory gap for requests to avoid triggering anti-scraping
		if reqType == "API" {
			t.lastAPIMu.Lock()
			elapsed := time.Since(t.lastAPIReq)
			t.lastAPIMu.Unlock()

			if elapsed < WikimediaAPIPacing {
				wait := WikimediaAPIPacing - elapsed
				log.Debugf("Wikimedia Throttler: [API] Pacing request. Sleeping %v...", wait)
				if err := contextSleep(req.Context(), wait); err != nil {
					return nil, err
				}
			}

			// Update last request time BEFORE releasing the slot
			defer func() {
				t.lastAPIMu.Lock()
				t.lastAPIReq = time.Now()
				t.lastAPIMu.Unlock()
			}()
		} else if reqType == "Media" {
			t.lastMediaMu.Lock()
			elapsed := time.Since(t.lastMediaReq)
			t.lastMediaMu.Unlock()

			if elapsed < WikimediaMediaPacing {
				wait := WikimediaMediaPacing - elapsed
				log.Debugf("Wikimedia Throttler: [Media] Pacing request. Sleeping %v...", wait)
				if err := contextSleep(req.Context(), wait); err != nil {
					return nil, err
				}
			}

			// Update last request time BEFORE releasing the slot
			defer func() {
				t.lastMediaMu.Lock()
				t.lastMediaReq = time.Now()
				t.lastMediaMu.Unlock()
			}()
		}

		if t.Base != nil {
			return t.Base.RoundTrip(req)
		}
		return http.DefaultTransport.RoundTrip(req)
	}()

	if err != nil {
		log.Debugf("Wikimedia Throttler: [%s] Request failed: %v", reqType, err)
		return nil, err
	}

	if resp.StatusCode == 429 {
		resp.Body.Close()

		// Pattern: Circuit Breaker Trip
		// Check for Retry-After header
		coolDown := WikimediaDefaultCooldown // Default long cooldown for 429
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if seconds, err := strconv.Atoi(ra); err == nil {
				coolDown = time.Duration(seconds) * time.Second
			}
		}

		log.Printf("Wikimedia Throttler: [%s] Rate limited (429). Tripping Circuit Breaker for %v.", reqType, coolDown)
		t.CB.Trip(coolDown)

		return nil, fmt.Errorf("wikimedia throttler: rate limited (429), circuit breaker tripped for %v", coolDown)
	}

	if resp.StatusCode != http.StatusOK {
		log.Debugf("Wikimedia Throttler: [%s] Request finished with status %d", reqType, resp.StatusCode)
	} else {
		log.Debugf("Wikimedia Throttler: [%s] Request successful", reqType)
	}

	return resp, nil
}

// EnrichImage returns the image as is since we fetch full metadata in search
func (p *WikimediaProvider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

// GetDownloadHeaders returns the necessary headers for downloading images.
func (p *WikimediaProvider) GetDownloadHeaders() map[string]string {
	return map[string]string{
		"User-Agent": WikimediaUserAgent,
		"Referer":    "https://github.com/dixieflatline76/Spice",
	}
}

// --- UI Integration ---

func (p *WikimediaProvider) Title() string {
	return "Wikimedia"
}

func (p *WikimediaProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	donationURL, _ := url.Parse("https://donate.wikimedia.org/")

	return container.NewVBox(
		sm.CreateSettingTitleLabel(i18n.T("Wikimedia Commons")),
		sm.CreateSettingDescriptionLabel(i18n.T("Wikimedia Commons is a media file repository making public domain and freely-licensed educational media content available to everyone.")),
		widget.NewHyperlink(i18n.T("Donate to Wikimedia"), donationURL),
	)
}

func (p *WikimediaProvider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	imgQueryList := p.createImgQueryList(sm)
	sm.RegisterRefreshFunc(imgQueryList.Refresh)

	// Create standardized Add Query Config
	onAdded := func() {
		imgQueryList.Refresh()
	}

	addQueryCfg := wallpaper.AddQueryConfig{
		Title:           i18n.T("New Wikimedia Query"),
		URLPlaceholder:  i18n.T("Enter Category URL, Search URL, or plain 'category:Name'"),
		URLValidator:    "", // Custom validation used in ValidateFunc
		URLErrorMsg:     "",
		DescPlaceholder: i18n.T("Add a description"),
		DescValidator:   "", // Basic length validation only
		DescErrorMsg:    "",
		ValidateFunc: func(term, desc string) error {
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
		},
		AddHandler: func(desc, term string, active bool) (string, error) {
			// Parse/Normalize again to be safe
			normalized, err := p.ParseURL(term)
			if err != nil {
				return "", err
			}
			return p.cfg.AddWikimediaQuery(desc, normalized, active)
		},
	}
	addButton := wallpaper.CreateAddQueryButton(
		i18n.T("Add Wikimedia Query"),
		sm,
		addQueryCfg,
		onAdded,
	)

	header := container.NewVBox()
	header.Add(sm.CreateSettingTitleLabel(i18n.T("Wikimedia Commons Queries")))
	header.Add(sm.CreateSettingDescriptionLabel(i18n.T("Add queries for Wikimedia Commons categories or search results.")))
	header.Add(addButton)

	// Auto-open if pending URL exists
	if pendingUrl != "" {
		fyne.Do(func() {
			// Delay slightly to ensure window is fully ready/shown
			time.Sleep(50 * time.Millisecond)
			wallpaper.OpenAddQueryDialog(sm, addQueryCfg, pendingUrl, "", onAdded)
		})
	}

	return container.NewBorder(header, nil, nil, nil, imgQueryList)
}

func (p *WikimediaProvider) createImgQueryList(sm setting.SettingsManager) *widget.List {
	return wallpaper.CreateQueryList(sm, wallpaper.QueryListConfig{
		GetQueries:    p.cfg.GetWikimediaQueries,
		EnableQuery:   p.cfg.EnableImageQuery,
		DisableQuery:  p.cfg.DisableImageQuery,
		RemoveQuery:   p.cfg.RemoveImageQuery,
		GetDisplayURL: p.getDisplayURL,
	})
}

func (p *WikimediaProvider) getDisplayURL(queryURL string) *url.URL {
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
		return NewWikimediaProvider(cfg, client)
	})
}

// sanitizeAttribution removes HTML tags and collapses whitespace.
// It ensures that weird source strings (like those with newlines or tabs) don't break the UI.
func sanitizeAttribution(input string) string {
	// 1. Strip HTML tags
	re := regexp.MustCompile("<[^>]*>")
	output := re.ReplaceAllString(input, "")

	// 2. Replace multiple whitespace characters (including newlines and tabs) with a single space
	reSpace := regexp.MustCompile(`\s+`)
	output = reSpace.ReplaceAllString(output, " ")

	return strings.TrimSpace(output)
}

// GetProviderIcon returns the provider's icon for the tray menu.
func (p *WikimediaProvider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("Wikimedia", iconData)
}

// contextSleep performs a sleep that is instantly interruptible by context cancellation.
func contextSleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
