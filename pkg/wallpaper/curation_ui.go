package wallpaper

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/dixieflatline76/Spice/v2/config"
	"github.com/dixieflatline76/Spice/v2/pkg/curation"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
)

// CreateCuratedQueryPanel generates a standard panel schema for any museum provider
// that uses the central curation.Manager.
func CreateCuratedQueryPanel(p provider.ImageProvider, sm setting.SettingsManager, cfg *Config) *schema.PanelSchema {
	mgr := curation.GetManager()

	// Get the collection from the manager using the provider's ID.
	col := mgr.GetCollection(p.ID())
	if col == nil {
		return &schema.PanelSchema{}
	}

	var items []schema.ItemSchema
	for _, entry := range col.Entries {
		items = append(items, buildCuratedUIItem(p, sm, cfg, entry))
	}

	return &schema.PanelSchema{
		Sections: []schema.SectionSchema{
			{
				Title: i18n.T("Curated Collections"),
				Items: items,
			},
		},
	}
}

func buildCuratedUIItem(p provider.ImageProvider, sm setting.SettingsManager, cfg *Config, entry curation.CollectionEntry) schema.BoolItem {
	// Helper to find existing query state
	getQuery := func(key string) (bool, string) {
		for _, q := range cfg.GetQueries() {
			if q.Provider == p.ID() && q.URL == key {
				return q.Active, q.ID
			}
		}
		return false, ""
	}

	active, _ := getQuery(entry.Key)

	actionText := ""
	var actionFunc func()
	label := i18n.TMap(entry.Name, entry.NameTranslations)

	if entry.Type == "curated" && len(entry.IDs) > 0 {
		actionText = i18n.T("Preview")
		label = fmt.Sprintf("%s (%d Pieces)", label, len(entry.IDs))

		actionFunc = func() {
			safeName := strings.ReplaceAll(strings.ToLower(entry.Key), " ", "_")
			fileName := fmt.Sprintf("%s.html", safeName)
			providerCacheDir := filepath.Join(config.GetWorkingDir(), "cache", strings.ToLower(p.ID()))
			outPath := filepath.Join(providerCacheDir, fileName)

			langParam := i18n.GetLanguage()
			localeJSPath := filepath.Join(providerCacheDir, "current_locale.js")
			_ = os.WriteFile(localeJSPath, fmt.Appendf(nil, "window.spiceAppLocale = '%s';", langParam), 0600)

			fileURL := fmt.Sprintf("file:///%s", filepath.ToSlash(outPath))
			if u, err := url.Parse(fileURL); err == nil {
				_ = u
				sm.OpenURL(fileURL)
			}
		}
	} else if entry.Type == "curated" && len(entry.Items) > 0 {
		// Pre-resolved items (e.g. Rijksmuseum)
		actionText = i18n.T("Preview")
		label = fmt.Sprintf("%s (%d Pieces)", label, len(entry.Items))

		actionFunc = func() {
			safeName := strings.ReplaceAll(strings.ToLower(entry.Key), " ", "_")
			fileName := fmt.Sprintf("%s.html", safeName)
			providerCacheDir := filepath.Join(config.GetWorkingDir(), "cache", strings.ToLower(p.ID()))
			outPath := filepath.Join(providerCacheDir, fileName)

			langParam := i18n.GetLanguage()
			localeJSPath := filepath.Join(providerCacheDir, "current_locale.js")
			_ = os.WriteFile(localeJSPath, fmt.Appendf(nil, "window.spiceAppLocale = '%s';", langParam), 0600)

			fileURL := fmt.Sprintf("file:///%s", filepath.ToSlash(outPath))
			if u, err := url.Parse(fileURL); err == nil {
				_ = u
				sm.OpenURL(fileURL)
			}
		}
	}

	return schema.BoolItem{
		Name:         p.ID() + "_" + entry.Key,
		Label:        label,
		ActionText:   actionText,
		ActionFunc:   actionFunc,
		InitialValue: active,
		NeedsRefresh: true,
		ApplyFunc: func(b bool) {
			_, cid := getQuery(entry.Key)
			if b {
				if cid != "" {
					_ = cfg.EnableImageQuery(cid)
				} else {
					_, _ = cfg.AddProviderQuery(entry.Name, entry.Key, p.ID(), true, false)
				}
			} else {
				if cid != "" {
					_ = cfg.DisableImageQuery(cid)
				}
			}
		},
	}
}
