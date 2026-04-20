package favorites

import (
	"log"
	"net/url"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
)

// GetProviderIcon returns the provider's icon for the tray menu.
func (p *Provider) GetProviderIcon() fyne.Resource {
	return fyne.NewStaticResource(ProviderName, iconData)
}

// CreateSettingsPanel satisfies the ImageProvider interface.
// It is intentionally left as a stub because the framework uses CreateSettingsSchema instead.
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject {
	return nil
}

// CreateQueryPanel creates the favorites management and clear-all panel.
func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, _ string) fyne.CanvasObject {
	// Ensure the favorites query exists in config
	if len(p.cfg.GetFavoritesQueries()) == 0 {
		_, _ = p.cfg.AddFavoritesQuery("Favorite Images", wallpaper.FavoritesQueryID, true)
	}

	queryList := wallpaper.CreateQueryList(sm, wallpaper.QueryListConfig{
		GetQueries:   p.cfg.GetFavoritesQueries,
		EnableQuery:  p.cfg.EnableImageQuery,
		DisableQuery: p.cfg.DisableImageQuery,
		RemoveQuery: func(id string) error {
			// Instead of removing the query from config, we wipe the folder (Clear All)
			path := p.rootDir
			_ = os.RemoveAll(path)
			_ = os.MkdirAll(path, 0755)
			log.Println("[Favorites] Wipe requested via UI: folder cleared.")

			p.mu.Lock()
			p.favMap = make(map[string]bool)
			p.mu.Unlock()

			if p.cfg.FavoritesClearedCallback != nil {
				go p.cfg.FavoritesClearedCallback()
			}
			return nil
		},
		GetDisplayText: func(q wallpaper.ImageQuery) string {
			return p.GetFavoritesDisplayName()
		},
		GetDisplayURL: func(q wallpaper.ImageQuery) *url.URL {
			if u, err := url.Parse(p.HomeURL()); err == nil {
				return u
			}
			return nil
		},
		DeleteLabel:          i18n.T("Clear"),
		ForceActionEnabled:   true,
		DeleteConfirmMessage: i18n.T("Are you sure you want to delete all saved favorites?"),
	})
	sm.RegisterRefreshFunc(queryList.Refresh)

	return container.NewVBox(
		widget.NewLabelWithStyle(i18n.T("Favorites:"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		queryList,
	)
}
