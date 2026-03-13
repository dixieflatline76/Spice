package wallpaper

import (
	"net/url"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
	"github.com/dixieflatline76/Spice/v2/pkg/provider"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	utilLog "github.com/dixieflatline76/Spice/v2/util/log"
)

// QueryListConfig defines the provider-specific callbacks for a scrollable query list.
// Providers pass this to CreateQueryList to get a scroll-safe widget.List that
// correctly handles Fyne's cell recycling without losing pending user toggles.
type QueryListConfig struct {
	// GetQueries returns the current list of queries from config.
	GetQueries func() []ImageQuery
	// EnableQuery is called on Apply when the user enables a query.
	EnableQuery func(id string) error
	// DisableQuery is called on Apply when the user disables a query.
	DisableQuery func(id string) error
	// RemoveQuery is called when the user confirms deletion.
	RemoveQuery func(id string) error
	// GetDisplayURL converts an API URL to a clickable web URL. Optional.
	GetDisplayURL func(apiURL string) *url.URL
}

// CreateQueryList builds a scroll-safe widget.List for query management.
// It handles baseline seeding, pending state preservation across Fyne cell recycling,
// and wires up the enable/disable/delete interactions with the SettingsManager.
func CreateQueryList(sm setting.SettingsManager, cfg QueryListConfig) *widget.List {
	var queryList *widget.List
	queryList = widget.NewList(
		// Length
		func() int {
			return len(cfg.GetQueries())
		},
		// CreateItem — builds the cell template (no data binding here)
		func() fyne.CanvasObject {
			urlLink := widget.NewHyperlink(i18n.T("Placeholder"), nil)
			activeCheck := widget.NewCheck(i18n.T("Active"), nil)
			deleteButton := widget.NewButton(i18n.T("Delete"), nil)
			return container.NewHBox(urlLink, layout.NewSpacer(), activeCheck, deleteButton)
		},
		// UpdateItem — binds data to a recycled cell (scroll-safe)
		func(i int, o fyne.CanvasObject) {
			queries := cfg.GetQueries()
			if i >= len(queries) {
				return
			}
			query := queries[i]
			queryKey := query.ID

			c := o.(*fyne.Container)
			urlLink := c.Objects[0].(*widget.Hyperlink)
			activeCheck := c.Objects[2].(*widget.Check)
			deleteButton := c.Objects[3].(*widget.Button)

			// Set display text and URL
			urlLink.SetText(query.Description)
			if cfg.GetDisplayURL != nil {
				if u := cfg.GetDisplayURL(query.URL); u != nil {
					urlLink.SetURL(u)
				}
			} else {
				if u, err := url.Parse(query.URL); err == nil {
					urlLink.SetURL(u)
				}
			}

			// --- Scroll-Safe State Management ---
			// Only seed baseline on first encounter to avoid overwriting pending toggles.
			if sm.GetBaseline(queryKey) == nil {
				sm.SeedBaseline(queryKey, query.Active)
			}

			// MUST clear OnChanged before SetChecked, otherwise recycling a cell
			// will trigger the previous query's OnChanged callback!
			activeCheck.OnChanged = nil

			// Restore pending state if user has toggled, otherwise use persisted state.
			if sm.HasPendingChange(queryKey) {
				activeCheck.SetChecked(!sm.GetBaseline(queryKey).(bool))
			} else {
				activeCheck.SetChecked(query.Active)
			}

			// Wire checkbox toggle
			activeCheck.OnChanged = func(b bool) {
				if b != sm.GetBaseline(queryKey).(bool) {
					sm.SetSettingChangedCallback(queryKey, func() {
						var err error
						if b {
							err = cfg.EnableQuery(query.ID)
						} else {
							err = cfg.DisableQuery(query.ID)
						}

						if err != nil {
							utilLog.Printf("Failed to update query status: %v", err)
						} else {
							// Update the baseline IF the change was successfully applied.
							// This prevents the "logic flip" bug if the user interacts
							// with the list again before reloading the panel.
							sm.SeedBaseline(queryKey, b)

							// Also update the underlying query data object in the local slice for this cell recycle loop,
							// just in case, though the config slice is usually re-fetched.
							query.Active = b
						}
					})
					sm.SetRefreshFlag(queryKey)
				} else {
					sm.RemoveSettingChangedCallback(queryKey)
					sm.UnsetRefreshFlag(queryKey)
				}
				sm.GetCheckAndEnableApplyFunc()()
			}

			// Wire delete button
			deleteButton.OnTapped = func() {
				d := dialog.NewConfirm(i18n.T("Please Confirm"), i18n.Tf("Are you sure you want to delete {{.Description}}?", map[string]any{"Description": query.Description}), func(b bool) {
					if b {
						if query.Active {
							sm.SetRefreshFlag(queryKey)
							sm.GetCheckAndEnableApplyFunc()()
						}
						if err := cfg.RemoveQuery(query.ID); err != nil {
							utilLog.Printf("Failed to remove query: %v", err)
						}
						queryList.Refresh()
					}
				}, sm.GetSettingsWindow())
				d.Show()
			}

			// Managed queries cannot be deleted
			if query.Managed {
				deleteButton.Disable()
			} else {
				deleteButton.Enable()
			}
		},
	)
	return queryList
}

// CreateTrayMenuItems creates the menu items for the tray menu
func (wp *Plugin) CreateTrayMenuItems() []*fyne.MenuItem {
	wp.monMu.RLock()
	monitorIDs := make([]int, 0, len(wp.Monitors))
	for id := range wp.Monitors {
		monitorIDs = append(monitorIDs, id)
	}
	wp.monMu.RUnlock()
	sort.Ints(monitorIDs)

	wp.monMu.Lock()
	wp.monitorMenu = make(map[int]*MonitorMenuItems)
	wp.monMu.Unlock()

	items := []*fyne.MenuItem{}

	// --- HELPER: Create Monitor Section Items ---
	createMonitorItems := func(mID int) []*fyne.MenuItem {
		wp.monMu.RLock()
		mc, hasMC := wp.Monitors[mID]
		wp.monMu.RUnlock()

		var currentImage provider.Image
		isInitialized := false
		isPaused := false
		if hasMC {
			mc.mu.RLock()
			currentImage = mc.State.CurrentImage
			isInitialized = mc.State.CurrentID != ""
			isPaused = mc.State.Paused
			mc.mu.RUnlock()
		}

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
			artistLabel = i18n.Tf("By: {{.Attribution}}", map[string]any{"Attribution": attribution})
			if currentImage.Attribution == "" {
				artistLabel = i18n.T("By: Unknown")
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
				mItems.ProviderMenuItem.Icon = p.GetProviderIcon()
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

		wp.monMu.Lock()
		wp.monitorMenu[mID] = mItems
		wp.monMu.Unlock()

		res := []*fyne.MenuItem{
			nextItem,
			prevItem,
		}
		if wp.cfg.GetWallpaperChangeFrequency() != FrequencyNever {
			res = append(res, pauseItem)
		}
		res = append(res, fyne.NewMenuItemSeparator())
		res = append(res, mItems.ProviderMenuItem)
		res = append(res, mItems.ArtistMenuItem)
		if mItems.FavoriteMenuItem != nil {
			res = append(res, mItems.FavoriteMenuItem)
		}
		res = append(res, deleteItem)

		return res
	}

	// --- 1. Primary Monitor (Monitor 0) ---
	items = append(items, createMonitorItems(0)...)

	// --- 2. Other Monitors (Submenus) ---
	if len(monitorIDs) > 1 {
		items = append(items, fyne.NewMenuItemSeparator())
		for _, mID := range monitorIDs {
			if mID == 0 {
				continue // Skip primary
			}

			displayName := i18n.Tf("Display {{.ID}}", map[string]any{"ID": mID + 1})
			wp.monMu.RLock()
			if m, ok := wp.Monitors[mID]; ok && m.Monitor.Name != "" {
				// Only append the name if it's a real device name (not a generic "Monitor N" index)
				if m.Monitor.Name != "Primary" && !strings.HasPrefix(m.Monitor.Name, "Monitor ") {
					displayName = i18n.Tf("Display {{.ID}} ({{.Name}})", map[string]any{"ID": mID + 1, "Name": m.Monitor.Name})
				}
			}
			wp.monMu.RUnlock()

			subMenu := wp.manager.CreateMenuItem(displayName, nil, "display.png")
			subMenu.ChildMenu = fyne.NewMenu(displayName, createMonitorItems(mID)...)
			items = append(items, subMenu)
		}
	}

	utilLog.Debugf("Finished Generating Tray Menu Items for %d monitors.", len(monitorIDs))
	return items
}

// CreatePrefsPanel creates a preferences widget for wallpaper settings
func (wp *Plugin) CreatePrefsPanel(sm setting.SettingsManager) *fyne.Container {
	builder := NewPrefsPanelBuilder(wp, sm)

	// Register the wallpaper refresh function
	sm.RegisterRefreshFunc(wp.RefreshImagesAndPulse)

	// 1. Build General Settings
	generalTab := builder.BuildGeneralTab()

	// 2. Build Provider Tabs
	onlineTab, localTab, targetTabIndex := builder.BuildProviderTabs()

	// 3. AI Tab Placeholder
	aiTab := container.NewStack(widget.NewLabelWithStyle(i18n.T("AI features coming soon..."), fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))

	// 4. Assemble Tabs
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon(i18n.T("General"), theme.SettingsIcon(), generalTab),
		container.NewTabItemWithIcon(i18n.T("Online"), theme.GridIcon(), onlineTab),
		container.NewTabItemWithIcon(i18n.T("Local"), theme.FolderIcon(), localTab),
		container.NewTabItemWithIcon(i18n.T("AI"), theme.ComputerIcon(), aiTab),
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
	Content   fyne.CanvasObject
	Open      bool
	Icon      fyne.Resource
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
