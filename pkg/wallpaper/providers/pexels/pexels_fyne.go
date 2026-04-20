package pexels

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
)

// GetProviderIcon returns the provider's icon for the tray menu.
func (p *PexelsProvider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("Pexels", iconData)
}

// CreateSettingsPanel satisfies the ImageProvider interface.
// It is intentionally left as a stub because the framework uses CreateSettingsSchema instead.
func (p *PexelsProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}

// CreateQueryPanel creates the image query management panel.
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
			// Check for duplicates
			queryID := wallpaper.GenerateQueryID(p.ID() + ":" + url)
			if p.cfg.IsDuplicateID(queryID) {
				return fmt.Errorf("%s", i18n.T("duplicate query: this URL already exists"))
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
