package wallpaper

import (
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	utilLog "github.com/dixieflatline76/Spice/v2/util/log"
)

// asResource converts a provider's abstract icon (bytes or resource) back to a Fyne resource.
func asResource(icon interface{}, name string) fyne.Resource {
	if icon == nil {
		return nil
	}
	switch v := icon.(type) {
	case fyne.Resource:
		return v
	case []byte:
		return fyne.NewStaticResource(name, v)
	}
	return nil
}

// CreateTrayMenuItems creates the menu items for the tray menu.
// THREADING: This function runs exclusively on the Fyne main thread (via fyne.Do).
// All monMu usage is confined to a fast snapshot at the top; the rest is lock-free.
func (wp *Plugin) CreateTrayMenuItems() []*fyne.MenuItem {
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
			initialized: mc.State.CurrentID != "",
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

	// Reset menu map — no lock needed (main thread affinity)
	wp.monitorMenu = make(map[int]*MonitorMenuItems)

	items := []*fyne.MenuItem{}

	// --- HELPER: Create Monitor Section Items ---
	createMonitorItems := func(snap monSnap) []*fyne.MenuItem {
		mID := snap.id
		currentImage := snap.image
		isInitialized := snap.initialized
		isPaused := snap.paused

		// Actions
		nextItem := wp.manager.CreateMenuItem(i18n.T("Next Wallpaper"), func() { go wp.SetNextWallpaper(mID, true) }, "next.png")
		prevItem := wp.manager.CreateMenuItem(i18n.T("Prev Wallpaper"), func() { go wp.SetPreviousWallpaper(mID, true) }, "prev.png")

		// Initial Labels with state awareness
		providerLabel := i18n.T("Source: Initializing...")
		artistLabel := i18n.T("By: Unknown")
		favoriteLabel := i18n.T("Add to Favorites")
		favoriteIcon := "favorite.png"
		pauseLabel := i18n.T("Pause Play")
		pauseIcon := "pause.png"

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
			if currentImage.IsFavorited {
				favoriteLabel = i18n.T("Remove from Favorites")
				favoriteIcon = "unfavorite.png"
			}
		}

		if isPaused {
			pauseLabel = i18n.T("Resume Play")
			pauseIcon = "play.png"
		}

		pauseItem := wp.manager.CreateMenuItem(pauseLabel, func() {
			wp.TogglePauseMonitorAction(mID)
		}, pauseIcon)

		var providerAction func()
		if isInitialized {
			providerAction = func() {
				wp.focusProviderName = currentImage.Provider
				wp.manager.OpenPreferences("Wallpaper")
			}
		}

		// Info Items (Store in monitorMenu for updates)
		mItems := &MonitorMenuItems{
			ProviderMenuItem: wp.manager.CreateMenuItem(providerLabel, providerAction, ""),
			ArtistMenuItem: wp.manager.CreateMenuItem(artistLabel, func() {
				go wp.ViewCurrentImageOnWeb(mID)
			}, "view.png"),
			PauseMenuItem: pauseItem,
		}

		// Provider Icon
		if isInitialized {
			if p, exists := wp.providers[currentImage.Provider]; exists {
				mItems.ProviderMenuItem.Icon = asResource(p.GetProviderIcon(), currentImage.Provider)
			}
		}

		if q, exists := wp.cfg.GetQuery(FavoritesQueryID); exists && q.Active {
			mItems.FavoriteMenuItem = wp.manager.CreateMenuItem(favoriteLabel, func() {
				go wp.TriggerFavorite(mID)
			}, favoriteIcon)
		}

		deleteItem := wp.manager.CreateMenuItem(i18n.T("Delete And Block"), func() {
			go wp.DeleteCurrentImage(mID)
		}, "delete.png")

		shuffleItem := wp.manager.CreateMenuItem(i18n.T("Shuffle"), func() {
			go wp.TriggerShuffle(mID)
		}, "shuffle.png")
		mItems.ShuffleMenuItem = shuffleItem

		// No lock needed — main thread affinity
		wp.monitorMenu[mID] = mItems

		res := []*fyne.MenuItem{
			nextItem,
			prevItem,
		}
		if wp.cfg.GetWallpaperChangeFrequency() != FrequencyNever {
			res = append(res, pauseItem)
		}
		res = append(res, shuffleItem)
		res = append(res, fyne.NewMenuItemSeparator())
		res = append(res, mItems.ProviderMenuItem)
		res = append(res, mItems.ArtistMenuItem)
		if mItems.FavoriteMenuItem != nil {
			res = append(res, mItems.FavoriteMenuItem)
		}
		if wp.cfg.GetSmartFitMode() != SmartFitOff {
			anchorItem := wp.manager.CreateMenuItem(i18n.T("Crop Anchor"), func() {
				wp.showAnchorPopup(mID)
			}, "anchor.png")
			res = append(res, anchorItem)
		}
		res = append(res, deleteItem)

		return res
	}

	// --- 1. Primary Monitor (Monitor 0) ---
	// Find the primary monitor snapshot
	for _, snap := range snaps {
		if snap.id == 0 {
			items = append(items, createMonitorItems(snap)...)
			break
		}
	}

	// --- 2. Other Monitors (Submenus) ---
	if len(snaps) > 1 {
		items = append(items, fyne.NewMenuItemSeparator())
		for _, snap := range snaps {
			if snap.id == 0 {
				continue // Skip primary
			}

			subMenu := wp.manager.CreateMenuItem(snap.displayName, nil, "display.png")
			subMenu.ChildMenu = fyne.NewMenu(snap.displayName, createMonitorItems(snap)...)
			items = append(items, subMenu)
		}
	}

	utilLog.Debugf("Finished Generating Tray Menu Items for %d monitors.", len(snaps))
	return items
}

// CreatePrefsPanel creates a preferences widget for wallpaper settings
func (wp *Plugin) CreatePrefsPanel(sm setting.SettingsManager) *fyne.Container {
	builder := NewPrefsPanelBuilder(wp, sm)

	// Register the wallpaper refresh function
	sm.RegisterRefreshFunc(wp.RefreshImagesAndPulse)

	// 1. Build General Settings as accordion (one section per accordion item)
	generalItems := builder.BuildGeneralTabAccordion(sm)
	generalTab, refreshGeneral := createAccordion(generalItems)
	sm.RegisterOnSettingsSaved(func() {
		if refreshGeneral != nil {
			refreshGeneral()
		}
	})

	// 2. Build Provider Tabs
	onlineTab, localTab, museumTab, targetTabIndex := builder.BuildProviderTabs()

	// 3. Assemble Tabs
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon(i18n.T("General"), theme.SettingsIcon(), generalTab),
		container.NewTabItemWithIcon(i18n.T("Community"), theme.GridIcon(), onlineTab),
		container.NewTabItemWithIcon(i18n.T("Personal"), theme.FolderIcon(), localTab),
		container.NewTabItemWithIcon(i18n.T("Museums"), theme.ColorPaletteIcon(), museumTab),
	)
	wp.settingsTabs = tabs
	tabs.SetTabLocation(container.TabLocationLeading)

	if targetTabIndex > 0 && targetTabIndex < len(tabs.Items) {
		tabs.SelectIndex(targetTabIndex)
	}

	return container.NewStack(tabs)
}

// Helper struct for accordion items
type accordionItem struct {
	Title     string
	TitleFunc func() string // Optional: Function to generate title dynamically

	Content fyne.CanvasObject
	Open    bool
	Icon    fyne.Resource
}

func createAccordion(items []accordionItem) (fyne.CanvasObject, func()) {
	// Container to hold the accordion
	accordionContainer := container.NewStack()

	// Function to refresh the accordion UI
	var refreshAccordion func()

	refreshAccordion = func() {
		// Use fyne.Do to ensure this runs on the main thread
		fyne.Do(func() {
			topHeaders := container.NewVBox()
			bottomHeaders := container.NewVBox()
			var centerContent fyne.CanvasObject

			foundOpen := false

			// If no items, show a placeholder or empty
			if len(items) == 0 {
				accordionContainer.Objects = []fyne.CanvasObject{widget.NewLabel("No providers in this category.")}
				accordionContainer.Refresh()
				return
			}

			for i := range items {
				index := i // Capture loop variable
				item := &items[index]

				// State Icon (Arrow)
				var arrowIcon fyne.Resource
				if item.Open {
					arrowIcon = theme.MoveDownIcon()
				} else {
					arrowIcon = theme.NavigateNextIcon()
				}

				// Header Action
				onTapped := func() {
					if item.Open {
						// If closing, open the next one (wrapping around)
						item.Open = false
						nextIndex := (index + 1) % len(items)
						items[nextIndex].Open = true
					} else {
						// If opening, close all others
						for j := range items {
							items[j].Open = (j == index)
						}
					}
					refreshAccordion()
				}

				// --- Complex Header Layout ---
				bgBtn := widget.NewButton("", onTapped)
				bgBtn.Alignment = widget.ButtonAlignLeading

				// Dynamic Title Support
				// If TitleFunc is provided, use it to fetch the latest title (e.g. updated counts)
				title := item.Title
				if item.TitleFunc != nil {
					title = item.TitleFunc()
				}

				titleLabel := widget.NewLabel(title)
				titleLabel.TextStyle = fyne.TextStyle{Bold: item.Open}

				headerContent := container.NewHBox(
					widget.NewIcon(arrowIcon),
				)
				if item.Icon != nil {
					providerIcon := widget.NewIcon(item.Icon)
					headerContent.Add(providerIcon)
				}
				headerContent.Add(titleLabel)

				headerStack := container.NewStack(bgBtn, container.NewPadded(headerContent))

				if item.Open {
					topHeaders.Add(headerStack)
					centerContent = item.Content
					foundOpen = true
				} else {
					if foundOpen {
						bottomHeaders.Add(headerStack)
					} else {
						topHeaders.Add(headerStack)
					}
				}
			}

			// Use Border Layout: Top headers | Bottom headers | Center Content
			// This ensures the Center Content (Provider UI) expands to fill available space.
			content := container.NewBorder(topHeaders, bottomHeaders, nil, nil, centerContent)
			accordionContainer.Objects = []fyne.CanvasObject{content}
			accordionContainer.Refresh()
		})
	}

	// EXPORTED via return closure? No, we simply register this closure if we had access to SM.
	// But createAccordion is generic.
	// HACK: We attach a "Refresh" method to the container? No.
	// Better: We return the refreshFunc as a second return value, OR we inject it into the items?
	// Actually, we need to call refreshAccordion from OUTSIDE when settings change.

	// Since we can't easily change the signature of createAccordion locally without refactoring,
	// checking if we can attach a callback to the returned container or rely on the caller to rebuild?
	// Caller (CreatePrefsPanel) builds it once.

	// Let's modify createAccordion signature to return (CanvasObject, func())
	// and update the caller.

	refreshAccordion()
	return accordionContainer, refreshAccordion
}
