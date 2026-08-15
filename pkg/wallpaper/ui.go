package wallpaper

import (
	"sort"
	"strings"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/schema"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	utilLog "github.com/dixieflatline76/Spice/v2/util/log"
)

// CreateTrayMenu creates the declarative schema for the tray menu.
// All monMu usage is confined to a fast snapshot at the top; the rest is lock-free.
func (wp *Plugin) CreateTrayMenu() *schema.MenuSchema {
	// ── Snapshot phase: read all needed monitor data under monMu ──
	type monSnap struct {
		id          int
		mc          *MonitorController
		image       provider.Image
		initialized bool
		paused      bool
		displayName string
	}

	wp.monMu.RLock()
	snaps := make([]monSnap, 0, len(wp.Monitors))
	for id, mc := range wp.Monitors {
		mc.mu.RLock()
		s := monSnap{
			id:          id,
			mc:          mc,
			image:       mc.State.CurrentImage,
			initialized: mc.State.CurrentID != "" || mc.State.CurrentImage.ID != "",
			paused:      mc.State.Paused,
		}
		mc.mu.RUnlock()

		// Build display name while we have the monitor reference
		s.displayName = i18n.Tf("Display {{.ID}}", map[string]any{"ID": id + 1})
		if mc.Monitor.Name != "" && mc.Monitor.Name != "Primary" && !strings.HasPrefix(mc.Monitor.Name, "Monitor ") {
			s.displayName = i18n.Tf("Display {{.ID}} ({{.Name}})", map[string]any{"ID": id + 1, "Name": mc.Monitor.Name})
		}

		snaps = append(snaps, s)
	}
	wp.monMu.RUnlock()
	// ── monMu released. Everything below is lock-free. ──

	sort.Slice(snaps, func(i, j int) bool { return snaps[i].id < snaps[j].id })

	var items []schema.MenuItemSchema

	// --- HELPER: Create Monitor Section Items ---
	createMonitorItems := func(snap monSnap) []schema.MenuItemSchema {
		mID := snap.id
		currentImage := snap.image
		isInitialized := snap.initialized
		isPaused := snap.paused

		// Actions
		nextItem := schema.MenuItemSchema{
			Label:    i18n.T("Next Wallpaper"),
			IconName: "next.png",
			Action:   func() { go wp.SetNextWallpaper(mID, true) },
		}
		prevItem := schema.MenuItemSchema{
			Label:    i18n.T("Prev Wallpaper"),
			IconName: "prev.png",
			Action:   func() { go wp.SetPreviousWallpaper(mID, true) },
		}

		// Initial Labels with state awareness
		providerLabel := i18n.T("Source: Initializing...")
		artistLabel := i18n.T("By: Unknown")
		favoriteLabel := i18n.T("Add to Favorites")
		favoriteIcon := "favorite.png"
		pauseLabel := i18n.T("Pause Play")
		pauseIcon := "pause.png"

		providerIcon := ""
		var providerIconBytes []byte
		if isInitialized {
			attribution := SanitizeMenuString(currentImage.Attribution)
			runes := []rune(attribution)
			if len(runes) > 20 {
				attribution = string(runes[:17]) + "..."
			}
			providerLabel = i18n.Tf("Source: {{.Provider}}", map[string]any{"Provider": wp.GetProviderTitle(currentImage.Provider)})

			attrType := provider.AttributionBy
			if p, exists := wp.providers[currentImage.Provider]; exists {
				attrType = p.GetAttributionType()
				if iconStr, ok := p.GetProviderIcon().(string); ok {
					providerIcon = iconStr
				} else if b, ok := p.GetProviderIcon().([]byte); ok {
					providerIconBytes = b
					providerIcon = currentImage.Provider
				} else {
					providerIcon = currentImage.Provider
				}
			}

			if currentImage.Attribution == "" {
				artistLabel = i18n.T("By: Unknown")
			} else {
				key := "attribution_by"
				if attrType == provider.AttributionIn {
					key = "attribution_in"
				}
				artistLabel = i18n.Tf(key, map[string]any{"Attribution": attribution})
			}
			if currentImage.IsFavorited || currentImage.Provider == "Favorites" {
				favoriteLabel = i18n.T("Remove from Favorites")
				favoriteIcon = "unfavorite.png"
			}
		}

		if isPaused {
			pauseLabel = i18n.T("Resume Play")
			pauseIcon = "play.png"
		}

		pauseItem := schema.MenuItemSchema{
			Label:    pauseLabel,
			IconName: pauseIcon,
			Action: func() {
				wp.TogglePauseMonitorAction(mID)
			},
		}

		var providerAction func()
		if isInitialized {
			providerAction = func() {
				wp.focusProviderName = currentImage.Provider
				wp.manager.OpenPreferences("Wallpaper")
			}
		}

		providerMenuItem := schema.MenuItemSchema{
			Label:     providerLabel,
			IconName:  providerIcon,
			IconBytes: providerIconBytes,
			Action:    providerAction,
		}

		artistMenuItem := schema.MenuItemSchema{
			Label:    artistLabel,
			IconName: "view.png",
			Action: func() {
				go wp.ViewCurrentImageOnWeb(mID)
			},
		}

		deleteItem := schema.MenuItemSchema{
			Label:    i18n.T("Delete And Block"),
			IconName: "delete.png",
			Action: func() {
				go wp.DeleteCurrentImage(mID)
			},
		}

		shuffleItem := schema.MenuItemSchema{
			Label:    i18n.T("Shuffle"),
			IconName: "shuffle.png",
			Action: func() {
				go wp.TriggerShuffle(mID)
			},
		}

		res := []schema.MenuItemSchema{
			nextItem,
			prevItem,
		}
		if wp.cfg.GetWallpaperChangeFrequency() != FrequencyNever {
			res = append(res, pauseItem)
		}
		res = append(res, shuffleItem)
		res = append(res, schema.MenuItemSchema{IsSeparator: true})
		res = append(res, providerMenuItem)
		res = append(res, artistMenuItem)
		if q, exists := wp.cfg.GetQuery(FavoritesQueryID); exists && q.Active {
			res = append(res, schema.MenuItemSchema{
				Label:    favoriteLabel,
				IconName: favoriteIcon,
				Action: func() {
					go wp.TriggerFavorite(mID)
				},
			})
		}
		if wp.cfg.GetSmartFitMode() != SmartFitOff {
			anchorItem := schema.MenuItemSchema{
				Label:    i18n.T("Tune Image"),
				IconName: "anchor.png",
				Action: func() {
					wp.showTuneImagePopup(mID)
				},
			}
			res = append(res, anchorItem)
		}
		res = append(res, deleteItem)

		return res
	}

	// --- 1. Primary Monitor (Monitor 0) ---
	for _, snap := range snaps {
		if snap.id == 0 {
			items = append(items, createMonitorItems(snap)...)
			break
		}
	}

	// --- 2. Other Monitors (Submenus) ---
	if len(snaps) > 1 {
		items = append(items, schema.MenuItemSchema{IsSeparator: true})
		for _, snap := range snaps {
			if snap.id == 0 {
				continue // Skip primary
			}

			subMenuItems := createMonitorItems(snap)
			items = append(items, schema.MenuItemSchema{
				Label:    snap.displayName,
				IconName: "display.png",
				SubMenu: &schema.MenuSchema{
					Label: snap.displayName,
					Items: subMenuItems,
				},
			})
		}
	}

	utilLog.Debugf("Finished Generating Tray Menu Schema for %d monitors.", len(snaps))
	return &schema.MenuSchema{
		Items: items,
	}
}

// CreatePrefsSchema creates a declarative preferences tabs schema for wallpaper settings.
func (wp *Plugin) CreatePrefsSchema(sm setting.SettingsManager) *schema.TabsSchema {
	builder := NewPrefsPanelBuilder(wp, sm)

	// Register the wallpaper refresh function
	sm.RegisterRefreshFunc(wp.RefreshImagesAndPulse)

	return builder.BuildPrefsTabsSchema()
}
