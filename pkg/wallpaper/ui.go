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

// actionBinder wires tray actions to a monitor ID.
//
// On per-screen tray platforms (live=true), the menu was built for the monitor
// under the cursor at build time, but the user may flick to another display and
// click before the rebuild lands. At click time the cursor is on the menu, which
// is on the display whose tray icon was clicked, so the live cursor monitor is
// the true target.
//
//	follow: stateless actions (Next, Prev, Shuffle, Tune, View on Web) retarget
//	        to the live monitor.
//	guard:  actions whose label reflects snapshot state (Pause/Resume,
//	        Add/Remove Favorite, Delete And Block) fail closed on mismatch and
//	        trigger an immediate rebuild instead of acting on the wrong image.
//
// When live=false both simply use the snapshot ID.
type actionBinder struct {
	wp   *Plugin
	id   int
	live bool
}

func (wp *Plugin) newActionBinder(id int, live bool) actionBinder {
	return actionBinder{wp: wp, id: id, live: live}
}

func (b actionBinder) follow(fn func(id int)) func() {
	return func() {
		if b.live {
			if cur, ok := b.wp.cursorMonitorID(); ok {
				fn(cur)
				return
			}
		}
		fn(b.id)
	}
}

func (b actionBinder) guard(fn func(id int)) func() {
	return func() {
		if b.live {
			if cur, ok := b.wp.cursorMonitorID(); ok && cur != b.id {
				utilLog.Printf("[Tray] Ignoring action for monitor %d: cursor is on monitor %d. Rebuilding menu.", b.id, cur)
				b.wp.activeTrayMonitorID.Store(int64(cur))
				if b.wp.manager != nil {
					b.wp.manager.RebuildTrayMenuImmediate()
				}
				return
			}
		}
		fn(b.id)
	}
}

// CreateTrayMenu creates the declarative schema for the tray menu.
// All monMu usage is confined to a fast snapshot at the top; the rest is lock-free.
func (wp *Plugin) CreateTrayMenu() *schema.MenuSchema {
	// ── Snapshot phase: read all needed monitor data under monMu ──
	type monSnap struct {
		id          int
		mon         Monitor
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
			mon:         mc.Monitor,
			mc:          mc,
			image:       mc.State.CurrentImage,
			initialized: mc.State.CurrentID != "" || mc.State.CurrentImage.ID != "",
			paused:      mc.State.Paused,
		}
		mc.mu.RUnlock()

		// Build display name while we have the monitor reference
		s.displayName = i18n.Tf("Display {{.ID}}", map[string]any{"ID": id + 1})
		if s.mon.Name != "" && s.mon.Name != "Primary" && !strings.HasPrefix(s.mon.Name, "Monitor ") {
			s.displayName = i18n.Tf("Display {{.ID}} ({{.Name}})", map[string]any{"ID": id + 1, "Name": s.mon.Name})
		}

		snaps = append(snaps, s)
	}
	wp.monMu.RUnlock()
	// ── monMu released. Everything below is lock-free. ──

	sort.Slice(snaps, func(i, j int) bool { return snaps[i].id < snaps[j].id })

	var items []schema.MenuItemSchema

	// --- HELPER: Create Monitor Section Items ---
	createMonitorItems := func(snap monSnap, live bool) []schema.MenuItemSchema {
		b := wp.newActionBinder(snap.id, live)
		currentImage := snap.image
		isInitialized := snap.initialized
		isPaused := snap.paused

		// Actions
		nextItem := schema.MenuItemSchema{
			Label:    i18n.T("Next Wallpaper"),
			IconName: "next.png",
			Action:   b.follow(func(id int) { go wp.SetNextWallpaper(id, true) }),
		}
		prevItem := schema.MenuItemSchema{
			Label:    i18n.T("Prev Wallpaper"),
			IconName: "prev.png",
			Action:   b.follow(func(id int) { go wp.SetPreviousWallpaper(id, true) }),
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

		if isPaused || wp.IsGateSuppressed() {
			pauseLabel = i18n.T("Resume Play")
			pauseIcon = "play.png"
		}

		pauseItem := schema.MenuItemSchema{
			Label:    pauseLabel,
			IconName: pauseIcon,
			Action: b.guard(func(id int) {
				wp.TogglePauseMonitorAction(id)
			}),
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
			Action: b.follow(func(id int) {
				go wp.ViewCurrentImageOnWeb(id)
			}),
		}

		deleteItem := schema.MenuItemSchema{
			Label:    i18n.T("Delete And Block"),
			IconName: "delete.png",
			Action: b.guard(func(id int) {
				go wp.DeleteCurrentImage(id)
			}),
		}

		shuffleItem := schema.MenuItemSchema{
			Label:    i18n.T("Shuffle"),
			IconName: "shuffle.png",
			Action: b.follow(func(id int) {
				go wp.TriggerShuffle(id)
			}),
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
				Action: b.guard(func(id int) {
					go wp.TriggerFavorite(id)
				}),
			})
		}
		if wp.cfg.GetSmartFitMode() != SmartFitOff {
			anchorItem := schema.MenuItemSchema{
				Label:    i18n.T("Tune Image"),
				IconName: "anchor.png",
				Action: b.follow(func(id int) {
					wp.showTuneImagePopup(id)
				}),
			}
			res = append(res, anchorItem)
		}
		res = append(res, deleteItem)

		return res
	}

	if len(snaps) == 0 {
		return &schema.MenuSchema{Items: items}
	}

	monitors := make([]Monitor, len(snaps))
	byID := make(map[int]monSnap, len(snaps))
	for i, s := range snaps {
		monitors[i] = s.mon
		byID[s.id] = s
	}

	perScreen := wp.hasPerScreenTray()
	rootID, submenuMonitors := PlanTrayMonitors(monitors, perScreen, int(wp.activeTrayMonitorID.Load()))

	if root, ok := byID[rootID]; ok {
		items = append(items, createMonitorItems(root, perScreen)...)
	}

	if len(submenuMonitors) > 0 {
		items = append(items, schema.MenuItemSchema{IsSeparator: true})
		for _, m := range submenuMonitors {
			snap, ok := byID[m.ID]
			if !ok {
				continue
			}
			items = append(items, schema.MenuItemSchema{
				Label:    snap.displayName,
				IconName: "display.png",
				SubMenu: &schema.MenuSchema{
					Label: snap.displayName,
					Items: createMonitorItems(snap, false),
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
