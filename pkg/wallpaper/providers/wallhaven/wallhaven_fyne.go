package wallhaven

import (
	"fmt"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
)

// CreateSettingsPanel satisfies the legacy ImageProvider interface.
// Migrated providers return nil to signal they use CreateSettingsSchema.
func (p *WallhavenProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}

// CreateQueryPanel creates the Fyne-specific UI for managing Wallhaven queries.
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
				return fmt.Errorf("%s", i18n.T("Duplicate query: this URL already exists"))
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
