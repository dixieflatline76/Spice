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

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/util/log"
)

//go:embed Wikimedia.png
var iconData []byte

// WikimediaProvider implements ImageProvider for Wikimedia Commons
type WikimediaProvider struct {
	cfg        *wallpaper.Config
	httpClient *http.Client
	baseURL    string
}

// NewWikimediaProvider creates a new instance of WikimediaProvider
func NewWikimediaProvider(cfg *wallpaper.Config, client *http.Client) *WikimediaProvider {
	return &WikimediaProvider{
		cfg:        cfg,
		httpClient: client,
		baseURL:    WikimediaBaseURL,
	}
}

// Name returns the provider name
func (p *WikimediaProvider) Name() string {
	return "Wikimedia"
}

func (p *WikimediaProvider) HomeURL() string {
	return "https://commons.wikimedia.org"
}

// ParseURL determines if the input is a Search term, a Category, or a direct URL.
// It normalizes it into a "search URL" for the API.
func (p *WikimediaProvider) ParseURL(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("input cannot be empty")
	}

	// 1. Check for Category Match (explicit "Category:" prefix)
	catRegex := regexp.MustCompile(WikimediaCategoryRegexp)
	if catRegex.MatchString(input) {
		// It's a category. Extract proper title if it's a full URL.
		if strings.HasPrefix(input, "http") {
			// e.g. https://commons.wikimedia.org/wiki/Category:Nature
			u, err := url.Parse(input)
			if err != nil {
				return "", err
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
			return "category:" + input[9:], nil
		}
		if strings.HasPrefix(lowerInput, "category%3a") {
			return "category:" + input[11:], nil
		}
		// Should not happen if regex matched but check just in case
		return "category:" + strings.TrimPrefix(input, "Category:"), nil
	}

	// 2. Check for Validation of commons.wikimedia.org domain if http is used
	if strings.HasPrefix(input, "http") {
		domainRegex := regexp.MustCompile(WikimediaDomainRegexp)
		if !domainRegex.MatchString(input) {
			return "", errors.New("invalid Wikimedia URL: must be commons.wikimedia.org")
		}
		// If it's a generic URL that isn't a category, we might treat it as a file?
		// For now, let's treat generic URLs as search terms derived from the title?
		// Simpler: Just fail if it's not a category URL, OR treat valid wiki URLs as categories?
		// Let's fallback to error for complex URLs for V1 unless they are categories.
		return "", errors.New("only 'Category:' URLs are currently supported directly")
	}

	// 3. Fallback: Treat as Search Term
	return "search:" + input, nil
}

type wikimediaResponse struct {
	Query struct {
		Pages map[string]struct {
			PageID    int    `json:"pageid"`
			Title     string `json:"title"`
			ImageInfo []struct {
				URL         string `json:"url"` // Original URL
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

	// Fallback (unknown format)
	return query
}

// FetchImages fetches images from the API based on the parsed URL (category:XXX or search:XXX)
func (p *WikimediaProvider) FetchImages(ctx context.Context, query string, page int) ([]provider.Image, error) {
	// Rate limiting check or just rely on global?
	// Note: page int is for "batch number" mostly.

	var apiURL string
	var params url.Values

	if strings.HasPrefix(query, "category:") {
		catTitle := strings.TrimPrefix(query, "category:")
		var limit = 50 // Fetch batch of 50
		apiURL = p.baseURL
		params = url.Values{}
		params.Set("action", "query")
		params.Set("generator", "categorymembers")
		params.Set("gcmtitle", "Category:"+catTitle)
		params.Set("gcmtype", "file")
		params.Set("gcmlimit", strconv.Itoa(limit))
		// Random sort is not reliable, so we fetch standard (sortkey) and shuffle client side.
		// To get variety, we could use gcmstart sortkey? For now, standard.

		params.Set("prop", "imageinfo")
		params.Set("iiprop", "url|extmetadata")
		params.Set("format", "json")
	} else if strings.HasPrefix(query, "search:") {
		searchTerm := strings.TrimPrefix(query, "search:")
		apiURL = p.baseURL
		var limit = 50
		params = url.Values{}
		params.Set("action", "query")
		params.Set("generator", "search")
		params.Set("gsrsearch", searchTerm)
		params.Set("gsrnamespace", "6") // File namespace
		params.Set("gsrlimit", strconv.Itoa(limit))
		params.Set("prop", "imageinfo")
		params.Set("iiprop", "url|extmetadata")
		params.Set("format", "json")
	} else {
		return nil, errors.New("unknown query format")
	}

	fullURL := apiURL + "?" + params.Encode()
	log.Debugf("Fetching Wikimedia: %s", fullURL)

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", WikimediaUserAgent)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result wikimediaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var images []provider.Image
	for _, page := range result.Query.Pages {
		if len(page.ImageInfo) == 0 {
			continue
		}
		info := page.ImageInfo[0]

		// Basic metadata cleanup
		// title := info.ExtMetadata.ObjectName.Value
		// if title == "" {
		// 	title = page.Title
		// }
		// Title is unused in current Image struct logic below, so we ignore it.

		// Attribution often contains HTML (links to user pages). We must strip it for the tray menu.
		artist := stripHTML(info.ExtMetadata.Artist.Value)
		if artist == "" {
			artist = "Unknown"
		}

		attribution := artist + " (" + info.ExtMetadata.LicenseShortName.Value + ")"

		img := provider.Image{
			ID:          strconv.Itoa(page.PageID), // Unique PageID
			Provider:    p.Name(),
			Path:        info.URL, // Original Full Res
			ViewURL:     info.URL, // Or page url? info.DescriptionUrl is better if available, but URL is fine.
			Attribution: attribution,
			FileType:    "image/jpeg", // Assume JPG mostly, or detect from ext?
		}

		// Simple extension check
		if strings.HasSuffix(strings.ToLower(info.URL), ".png") {
			img.FileType = "image/png"
		}

		images = append(images, img)
	}

	// Client-side Shuffle removed to respect global shuffle setting.
	// Images will be returned in API order (default).

	// Limit to reasonable batch size for the app
	if len(images) > 20 {
		images = images[:20]
	}

	return images, nil
}

// EnrichImage returns the image as is since we fetch full metadata in search
func (p *WikimediaProvider) EnrichImage(ctx context.Context, img provider.Image) (provider.Image, error) {
	return img, nil
}

// GetDownloadHeaders returns the necessary headers for downloading images.
func (p *WikimediaProvider) GetDownloadHeaders() map[string]string {
	return map[string]string{
		"User-Agent": WikimediaUserAgent,
	}
}

// --- UI Integration ---

func (p *WikimediaProvider) Title() string {
	return "Wikimedia Commons"
}

func (p *WikimediaProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	donationURL, _ := url.Parse("https://donate.wikimedia.org/")
	return container.NewVBox(
		widget.NewLabel("Wikimedia Commons is a media file repository making public domain and"),
		widget.NewLabel("freely-licensed educational media content available to everyone."),
		widget.NewHyperlink("Donate to Wikimedia", donationURL),
	)
}

func (p *WikimediaProvider) CreateQueryPanel(sm setting.SettingsManager) fyne.CanvasObject {
	pendingState := make(map[string]bool)
	var queryList *widget.List

	queryList = widget.NewList(
		func() int {
			return len(p.cfg.GetWikimediaQueries())
		},
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewCheck("Active", func(b bool) {}),
				widget.NewLabel("Template Description"),
				widget.NewLabel("Template URL"),
				widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {}),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			queries := p.cfg.GetWikimediaQueries()
			if id >= len(queries) {
				return
			}
			q := queries[id]
			queryKey := q.ID

			hbox := obj.(*fyne.Container)
			check := hbox.Objects[0].(*widget.Check)
			descLabel := hbox.Objects[1].(*widget.Label)
			urlLabel := hbox.Objects[2].(*widget.Label)
			deleteBtn := hbox.Objects[3].(*widget.Button)

			initialActive := q.Active
			check.OnChanged = nil // Detach to avoid triggering during setup

			if val, ok := pendingState[queryKey]; ok {
				check.SetChecked(val)
			} else {
				check.SetChecked(initialActive)
			}

			check.OnChanged = func(b bool) {
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
							err = p.cfg.EnableImageQuery(q.ID)
						} else {
							err = p.cfg.DisableImageQuery(q.ID)
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

			descLabel.SetText(q.Description)
			urlLabel.SetText(q.URL)

			deleteBtn.OnTapped = func() {
				dialog.NewConfirm("Delete Query", "Are you sure you want to delete this query?", func(b bool) {
					if b {
						if q.Active {
							sm.SetRefreshFlag(queryKey)
							// Trigger apply check if needed, though delete is usually immediate in this app's pattern
							// checking wallhaven logic: it just deletes immediately.
						}
						delete(pendingState, queryKey)
						if err := p.cfg.RemoveImageQuery(q.ID); err != nil {
							dialog.ShowError(err, sm.GetSettingsWindow())
						}
						sm.SetRefreshFlag("queries") // Refresh global query list tag
						queryList.Refresh()
					}
				}, sm.GetSettingsWindow()).Show()
			}
		},
	)

	// Create "Add" Box
	descEntry := widget.NewEntry()
	descEntry.PlaceHolder = "Description (e.g. Space)"
	urlEntry := widget.NewEntry()
	urlEntry.PlaceHolder = "Category:Space OR Search Term"

	addBtn := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		if descEntry.Text == "" || urlEntry.Text == "" {
			dialog.ShowError(errors.New("description and value are required"), sm.GetSettingsWindow())
			return
		}

		val := urlEntry.Text
		// Allow "Category: " with space
		// Pre-validation logic?
		// If it looks like a URL, validation happens in ParseURL during download?
		// We should probably validate input format here slightly?
		// Re-use ParseURL logic?

		_, err := p.ParseURL(val)
		if err != nil {
			dialog.ShowError(fmt.Errorf("invalid input: %v", err), sm.GetSettingsWindow())
			return
		}

		// If valid, add it
		// Note check for duplicates is inside Config
		_, err = p.cfg.AddWikimediaQuery(descEntry.Text, val, true)
		if err != nil {
			dialog.ShowError(err, sm.GetSettingsWindow())
			return
		}

		descEntry.SetText("")
		urlEntry.SetText("")
		queryList.Refresh()
		sm.SetRefreshFlag("queries")
	})

	addBox := container.NewBorder(nil, nil, nil, addBtn, container.NewGridWithColumns(2, descEntry, urlEntry))

	return container.NewBorder(nil, addBox, nil, nil, queryList)
}

func init() {
	wallpaper.RegisterProvider("Wikimedia", func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
		return NewWikimediaProvider(cfg, client)
	})
}

// stripHTML removes HTML tags from a string.
// It uses a simple regex to replace <...> with empty string.
func stripHTML(input string) string {
	re := regexp.MustCompile("<[^>]*>")
	return re.ReplaceAllString(input, "")
}

// GetProviderIcon returns the provider's icon for the tray menu.
func (p *WikimediaProvider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("Wikimedia", iconData)
}
