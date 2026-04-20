package metmuseum

import (
	"fmt"

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
	return fyne.NewStaticResource("MetMuseum.png", iconData)
}

// CreateSettingsPanel satisfies the ImageProvider interface.
// It is intentionally left as a stub because the framework uses CreateSettingsSchema instead.
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}

// CreateQueryPanel creates the image query management panel.
func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, pendingUrl string) fyne.CanvasObject {
	// This provider uses the "Museum Template"

	header := wallpaper.CreateMuseumHeader(
		"The Metropolitan Museum of Art",
		"New York City, USA",
		i18n.T("Open Access (CC0)"),
		"https://www.metmuseum.org/about-the-met/policies-and-documents/open-access",
		i18n.T("The crown jewel of New York City. From ancient Egyptian temples to modern masterpieces, The Met houses 5,000 years of humanity's greatest creative achievements."),
		"https://www.google.com/maps/search/?api=1&query=The+Metropolitan+Museum+of+Art",
		"https://www.metmuseum.org",
		"https://www.metmuseum.org/donate",
		sm,
	)

	// Fixed List of Collections
	collections := []struct {
		Name string
		Key  string
	}{
		{i18n.T("Director's Cut: Essential Masterpieces"), CollectionSpiceMelange},
		{i18n.T("American Wing"), CollectionAmerican},
		{i18n.T("European Paintings"), CollectionEuropean},
		{i18n.T("Arts of Asia"), CollectionAsian},
		{i18n.T("Egyptian Art"), CollectionEgyptian},
	}

	// Helper to find existing query state
	getDetails := func(key string) (bool, string) {
		for _, q := range p.cfg.GetMetMuseumQueries() {
			if q.URL == key {
				return q.Active, q.ID
			}
		}
		return false, "" // Not added yet
	}

	// Create Checkboxes
	var checks []fyne.CanvasObject
	for _, col := range collections {
		col := col // capture
		active, _ := getDetails(col.Key)
		dirtyKey := fmt.Sprintf("met_%s", col.Key)
		callbackKey := fmt.Sprintf("met_cb_%s", col.Key)

		sm.SeedBaseline(dirtyKey, active)
		chk := widget.NewCheck(col.Name, func(on bool) {
			if on != sm.GetBaseline(dirtyKey).(bool) {
				sm.SetSettingChangedCallback(callbackKey, func() {
					// Actual Save Logic (Deferred)
					// Fetch fresh ID from config to ensure we target correctly
					_, cid := getDetails(col.Key)

					if on {
						if cid != "" {
							if err := p.cfg.EnableMetMuseumQuery(cid); err != nil {
								log.Printf("MET: Failed to enable %s: %v", col.Name, err)
							}
						} else {
							desc := fmt.Sprintf(i18n.T("The Met: %s"), col.Name)
							if _, err := p.cfg.AddMetMuseumQuery(desc, col.Key, true); err != nil {
								log.Printf("MET: Failed to add %s: %v", col.Name, err)
							}
						}
					} else {
						if cid != "" {
							if err := p.cfg.DisableMetMuseumQuery(cid); err != nil {
								log.Printf("MET: Failed to disable %s: %v", col.Name, err)
							}
						}
					}
				})
				// Enable Apply Button
				sm.SetRefreshFlag(dirtyKey)
			} else {
				// Reverted to original state
				sm.RemoveSettingChangedCallback(callbackKey)
				sm.UnsetRefreshFlag(dirtyKey)
			}
			sm.GetCheckAndEnableApplyFunc()()
		})
		chk.Checked = active
		checks = append(checks, chk)
	}

	listContainer := container.NewVBox(checks...)

	return container.NewVBox(
		header,
		widget.NewSeparator(),
		widget.NewLabelWithStyle(i18n.T("Collections"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		listContainer,
	)
}
