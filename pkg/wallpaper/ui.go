package wallpaper

import (
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	utilLog "github.com/dixieflatline76/Spice/util/log"
)

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
		// Actions
		nextItem := wp.manager.CreateMenuItem("Next Wallpaper", func() { go wp.SetNextWallpaper(mID, true) }, "next.png")
		prevItem := wp.manager.CreateMenuItem("Prev Wallpaper", func() { go wp.SetPreviousWallpaper(mID, true) }, "prev.png")

		// Global (but requested in root and submenus?)
		// User instruction said: "sub menu with items starting from Next Wallpaper to Delete And Block"
		pauseItem := wp.manager.CreateMenuItem("Pause Play", func() {
			wp.TogglePauseMonitorAction(mID)
		}, "pause.png")

		// Info Items (Store in monitorMenu for updates)
		mItems := &MonitorMenuItems{
			ProviderMenuItem: wp.manager.CreateMenuItem("Source: Initializing...", nil, ""),
			ArtistMenuItem: wp.manager.CreateMenuItem("By: Unknown", func() {
				go wp.ViewCurrentImageOnWeb(mID)
			}, "view.png"),
		}
		if q, exists := wp.cfg.GetQuery(FavoritesQueryID); exists && q.Active {
			mItems.FavoriteMenuItem = wp.manager.CreateMenuItem("Add to Favorites", func() {
				go wp.TriggerFavorite(mID)
			}, "favorite.png")
		}

		deleteItem := wp.manager.CreateMenuItem("Delete And Block", func() {
			go wp.DeleteCurrentImage(mID)
		}, "delete.png")

		wp.monMu.Lock()
		wp.monitorMenu[mID] = mItems
		wp.monMu.Unlock()

		mItems.PauseMenuItem = pauseItem

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

			displayName := fmt.Sprintf("Display %d", mID+1)
			wp.monMu.RLock()
			if m, ok := wp.Monitors[mID]; ok && m.Monitor.Name != "" {
				// Only append the name if it's a real device name (not a generic "Monitor N" index)
				if m.Monitor.Name != "Primary" && !strings.HasPrefix(m.Monitor.Name, "Monitor ") {
					displayName = fmt.Sprintf("Display %d (%s)", mID+1, m.Monitor.Name)
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
	aiTab := container.NewStack(widget.NewLabelWithStyle("AI features coming soon...", fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))

	// 4. Assemble Tabs
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("General", theme.SettingsIcon(), generalTab),
		container.NewTabItemWithIcon("Online", theme.GridIcon(), onlineTab),
		container.NewTabItemWithIcon("Local", theme.FolderIcon(), localTab),
		container.NewTabItemWithIcon("AI", theme.ComputerIcon(), aiTab),
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
