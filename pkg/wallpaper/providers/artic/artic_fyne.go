package artic

import (
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// GetProviderIcon returns the provider's icon for the tray menu.
func (p *Provider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource("artic.png", iconData)
}

// CreateSettingsPanel satisfies the ImageProvider interface.
// It is intentionally left as a stub because the framework uses CreateSettingsSchema instead.
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}

// CreateQueryPanel creates the image query management panel.
func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	header := wallpaper.CreateMuseumHeader(
		"Art Institute of Chicago",
		"Chicago, IL • USA",
		i18n.T("CC0 - Public Domain"),
		"https://www.artic.edu/open-access/open-access-images",
		i18n.T("One of the world's great art museums, housing icons like Nighthawks and American Gothic."),
		"https://www.google.com/maps/search/?api=1&query=Art+Institute+of+Chicago",
		"https://www.artic.edu",
		"https://sales.artic.edu/donate",
		sm,
	)

	// Collection List (Deferred Save Model)
	listContainer := container.NewVBox()

	// We iterate through our curated tours
	keys := make([]string, 0, len(p.curatedList.Tours))
	for k := range p.curatedList.Tours {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Helper to find existing query state
	getDetails := func(key string) (bool, string) {
		for _, q := range p.cfg.GetArtInstituteChicagoQueries() {
			if q.URL == key {
				return q.Active, q.ID
			}
		}
		return false, ""
	}

	for _, key := range keys {
		tour := p.curatedList.Tours[key]
		key := key // shadow for closure
		active, _ := getDetails(key)

		sm.SeedBaseline(ProviderName+"_"+key, active)
		check := widget.NewCheck(tour.Name, func(b bool) {
			if b != sm.GetBaseline(ProviderName+"_"+key).(bool) {
				// Deferred save logic
				sm.SetSettingChangedCallback(ProviderName+"_"+key, func() {
					_, cid := getDetails(key)
					if b {
						if cid != "" {
							if err := p.cfg.EnableArtInstituteChicagoQuery(cid); err != nil {
								log.Printf("AIC: Failed to enable query %s: %v", key, err)
							}
						} else {
							if _, err := p.cfg.AddArtInstituteChicagoQuery(tour.Name, key, true); err != nil {
								log.Printf("AIC: Failed to add query %s: %v", tour.Name, err)
							}
						}
					} else {
						if cid != "" {
							if err := p.cfg.DisableArtInstituteChicagoQuery(cid); err != nil {
								log.Printf("AIC: Failed to disable query %s: %v", key, err)
							}
						}
					}
				})
			} else {
				sm.RemoveSettingChangedCallback(ProviderName + "_" + key)
			}
			sm.GetCheckAndEnableApplyFunc()()
		})
		check.Checked = active
		listContainer.Add(check)
	}

	return container.NewVBox(
		header,
		widget.NewSeparator(),
		widget.NewLabelWithStyle(i18n.T("Curated Tours"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		listContainer,
	)
}
