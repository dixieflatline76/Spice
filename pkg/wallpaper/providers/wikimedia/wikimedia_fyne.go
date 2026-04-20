package wikimedia

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
)

// GetProviderIcon returns the provider's icon for the tray menu.
// This is hosted in a separate Fyne-aware file to keep the core provider logic pure Go.
func (p *WikimediaProvider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("Wikimedia", iconData)
}

// CreateSettingsPanel satisfies the ImageProvider interface.
// It is intentionally left as a stub because the framework uses CreateSettingsSchema instead.
func (p *WikimediaProvider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}

// CreateQueryPanel creates the image query management panel.
// This remains in the Fyne-aware shim for Phase 2.
func (p *WikimediaProvider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	imgQueryList := buidQueryList(p, sm)
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
			return p.validateQueryInternal(term, desc)
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

// Helper to bridge to pure-go config methods
func buidQueryList(p *WikimediaProvider, sm setting.SettingsManager) fyne.CanvasObject {
	return wallpaper.CreateQueryList(sm, wallpaper.QueryListConfig{
		GetQueries:    p.cfg.GetWikimediaQueries,
		EnableQuery:   p.cfg.EnableImageQuery,
		DisableQuery:  p.cfg.DisableImageQuery,
		RemoveQuery:   p.cfg.RemoveImageQuery,
		GetDisplayURL: p.getDisplayURL,
	})
}
