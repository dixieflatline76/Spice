package wallpaper

import (
	"fmt"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/provider"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	utilLog "github.com/dixieflatline76/Spice/util/log"
)

// CreateTrayMenuItems creates the menu items for the tray menu
func (wp *Plugin) CreateTrayMenuItems() []*fyne.MenuItem {
	items := []*fyne.MenuItem{}
	items = append(items, wp.manager.CreateMenuItem("Next Wallpaper", func() {
		go wp.SetNextWallpaper()
	}, "next.png"))
	items = append(items, wp.manager.CreateMenuItem("Prev Wallpaper", func() {
		go wp.SetPreviousWallpaper()
	}, "prev.png"))

	// Pause/Resume Item (Using ToggleMenuItem to leverage built-in refresh)
	updatePauseVisuals := func() {
		if wp.IsPaused() {
			wp.pauseMenuItem.Label = "Resume Wallpaper"
			wp.pauseMenuItem.Icon, _ = wp.manager.GetAssetManager().GetIcon("play.png")
		} else {
			wp.pauseMenuItem.Label = "Pause Wallpaper"
			wp.pauseMenuItem.Icon, _ = wp.manager.GetAssetManager().GetIcon("pause.png")
		}
	}

	wp.pauseMenuItem = wp.manager.CreateToggleMenuItem("Pause Wallpaper", func(b bool) {
		wp.TogglePause()
		updatePauseVisuals()
	}, "pause.png", wp.IsPaused())

	// Initial visual update (to override default checkmark if needed, though CreateToggleMenuItem sets label first)
	updatePauseVisuals()

	items = append(items, wp.pauseMenuItem)

	items = append(items, wp.manager.CreateToggleMenuItem("Shuffle Wallpapers", wp.SetShuffleImage, "shuffle.png", wp.cfg.GetImgShuffle()))
	// Relax Smart Fit removed from tray (Use Preferences)
	items = append(items, fyne.NewMenuItemSeparator())
	// Provider Info (Static)
	wp.providerMenuItem = wp.manager.CreateMenuItem("Initializing...", nil, "")
	items = append(items, wp.providerMenuItem)

	// Artist/Source Link (Clickable)
	wp.artistMenuItem = wp.manager.CreateMenuItem("Unknown", func() {
		go wp.ViewCurrentImageOnWeb()
	}, "view.png")
	items = append(items, wp.artistMenuItem)
	// Favorites Item
	q, exists := wp.cfg.GetQuery(FavoritesQueryID)
	if exists && q.Active {
		wp.favoriteMenuItem = wp.manager.CreateMenuItem("Add to Favorites", func() {
			go wp.ToggleFavorite()
		}, "favorite.png")
		items = append(items, wp.favoriteMenuItem)
		wp.updateFavoriteMenuItem(false) // Initialize label/icon
	}

	items = append(items, wp.manager.CreateMenuItem("Delete and Block Image", func() {
		go wp.DeleteCurrentImage()
	}, "delete.png"))

	return items
}

// CreatePrefsPanel creates a preferences widget for wallpaper settings
func (wp *Plugin) CreatePrefsPanel(sm setting.SettingsManager) *fyne.Container {
	// --- General Settings Container ---
	generalContainer := container.NewVBox()

	// Register the wallpaper refresh function
	sm.RegisterRefreshFunc(wp.RefreshImagesAndPulse)

	// Change Frequency
	frequencyConfig := setting.SelectConfig{
		Name:         "changeFrequency",
		Options:      setting.StringOptions(GetFrequencies()),
		InitialValue: int(wp.cfg.GetWallpaperChangeFrequency()),
		Label:        sm.CreateSettingTitleLabel("Wallpaper Change Frequency:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Set how often the wallpaper changes. Set to \"Never\" to disable wallpaper changes."),
	}
	frequencyConfig.ApplyFunc = func(val interface{}) {
		selectedFrequency := Frequency(val.(int))
		wp.cfg.SetWallpaperChangeFrequency(selectedFrequency) // Persists new frequency in configuration
		wp.ChangeWallpaperFrequency(selectedFrequency)        // Activate the new frequency in wallpaper plugin
		frequencyConfig.InitialValue = int(selectedFrequency) // Update initial value for frequency
	}
	sm.CreateSelectSetting(&frequencyConfig, generalContainer)

	// Cache Size
	cacheSizeConfig := setting.SelectConfig{
		Name:         "cacheSize",
		Options:      setting.StringOptions(GetCacheSizes()), // Correctly calling GetCacheSizes
		InitialValue: int(wp.cfg.GetCacheSize()),
		Label:        sm.CreateSettingTitleLabel("Cache Size:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Set how many images to cache for faster startup and less network usage. Set to \"None\" to disable caching."),
	}
	cacheSizeConfig.ApplyFunc = func(val interface{}) {
		selectedCacheSize := CacheSize(val.(int))
		wp.cfg.SetCacheSize(selectedCacheSize)                // Persists new cache size in configuration
		cacheSizeConfig.InitialValue = int(selectedCacheSize) // Update initial value for cache size
	}
	sm.CreateSelectSetting(&cacheSizeConfig, generalContainer)

	// Declare check widgets early for usage in closures
	var faceCropCheck *widget.Check
	var faceBoostCheck *widget.Check

	// Smart Fit Mode
	// 0: Disabled
	// 1: Standard (Strict)
	// 2: Relaxed (Aggressive)
	smartFitModeConfig := setting.SelectConfig{
		Name:         "smartFitMode",
		Options:      GetSmartFitModes(), // Pass string slice directly
		InitialValue: int(wp.cfg.GetSmartFitMode()),
		Label:        sm.CreateSettingTitleLabel("Smart Fit Mode:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Control how images are fitted to your screen:\n- Disabled: Original image.\n- Quality: Rejects images with mismatched aspect ratio.\n- Flexibility: Allows high-res images to crop aggressively."),
	}
	smartFitModeConfig.ApplyFunc = func(val interface{}) {
		mode := SmartFitMode(val.(int))
		wp.cfg.SetSmartFitMode(mode)
		smartFitModeConfig.InitialValue = int(mode)
	}

	smartFitModeConfig.OnChanged = func(s string, val interface{}) {
		mode := SmartFitMode(val.(int))
		// Link to Face Crop/Boost logic
		if faceCropCheck != nil && faceBoostCheck != nil {
			if mode == SmartFitOff {
				faceCropCheck.SetChecked(false)
				faceCropCheck.Disable()
				faceBoostCheck.SetChecked(false)
				faceBoostCheck.Disable()
			} else {
				faceCropCheck.Enable()
				faceBoostCheck.Enable()
			}
			// Force redraw of the widgets to reflect enabled/disabled state
			faceCropCheck.Refresh()
			faceBoostCheck.Refresh()
		}
	}
	sm.CreateSelectSetting(&smartFitModeConfig, generalContainer)

	// Face Crop and Face Boost configs
	var faceCropConfig setting.BoolConfig
	var faceBoostConfig setting.BoolConfig

	// Face Crop (formerly Face Boost)
	faceCropConfig = setting.BoolConfig{
		Name:         "faceCrop",
		InitialValue: wp.cfg.GetFaceCropEnabled(),
		Label:        sm.CreateSettingTitleLabel("Enable Face Crop:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Aggressively crops the image to center on the largest face found. Good for portraits."),
		ApplyFunc: func(b bool) {
			wp.cfg.SetFaceCropEnabled(b)
			if b {
				wp.cfg.SetFaceBoostEnabled(false)
				faceBoostConfig.InitialValue = false // Sync the other setting's initial value
			}
			faceCropConfig.InitialValue = b
		},
		NeedsRefresh: true,
	}

	// Face Boost (new hinting mode)
	faceBoostConfig = setting.BoolConfig{
		Name:         "faceBoost",
		InitialValue: wp.cfg.GetFaceBoostEnabled(),
		Label:        sm.CreateSettingTitleLabel("Enable Face Boost:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Uses face detection to hint the smart cropper. Keeps faces in frame but balances with other image details."),
		ApplyFunc: func(b bool) {
			wp.cfg.SetFaceBoostEnabled(b)
			if b {
				wp.cfg.SetFaceCropEnabled(false)
				faceCropConfig.InitialValue = false // Sync the other setting's initial value
			}
			faceBoostConfig.InitialValue = b
		},
		NeedsRefresh: true,
	}

	// Create checkboxes in a sub-container for indentation
	subSettingsContainer := container.NewVBox()
	faceCropCheck = sm.CreateBoolSetting(&faceCropConfig, subSettingsContainer)
	faceBoostCheck = sm.CreateBoolSetting(&faceBoostConfig, subSettingsContainer)

	// Add indentation
	indentation := widget.NewLabel("      ") // Simple spacer
	indentedWrapper := container.NewBorder(nil, nil, indentation, nil, subSettingsContainer)
	generalContainer.Add(indentedWrapper)

	// Mutual exclusion logic
	// We need to hook into the OnChanged of the widgets to update the UI state of the other checkbox.
	// CreateBoolSetting returns *widget.Check, so we can access it directly.

	// Store original handlers to chain them
	originalFaceCropHandler := faceCropCheck.OnChanged
	originalFaceBoostHandler := faceBoostCheck.OnChanged

	faceCropCheck.OnChanged = func(b bool) {
		utilLog.Debugf("UI: Face Crop Toggled: %v", b)
		if b {
			utilLog.Debugf("UI: Unchecking Face Boost")
			faceBoostCheck.SetChecked(false) // Uncheck Boost if Crop is checked
		}
		if originalFaceCropHandler != nil {
			originalFaceCropHandler(b)
		}
	}

	faceBoostCheck.OnChanged = func(b bool) {
		utilLog.Debugf("UI: Face Boost Toggled: %v", b)
		if b {
			utilLog.Debugf("UI: Unchecking Face Crop")
			faceCropCheck.SetChecked(false) // Uncheck Crop if Boost is checked
		}
		if originalFaceBoostHandler != nil {
			originalFaceBoostHandler(b)
		}
	}

	// Link both to Smart Fit Mode (Initial State)
	if wp.cfg.GetSmartFitMode() == SmartFitOff {
		faceCropCheck.Disable()
		faceBoostCheck.Disable()
	}

	// Change Wallpaper on Start
	chgImgOnStartConfig := setting.BoolConfig{
		Name:         "chgImgOnStart",
		InitialValue: wp.cfg.GetChgImgOnStart(),
		Label:        sm.CreateSettingTitleLabel("Change wallpaper on start:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Disable if you prefer the wallpaper to change only based on its timer or a manual refresh."),
		NeedsRefresh: false,
	}
	chgImgOnStartConfig.ApplyFunc = func(b bool) {
		wp.cfg.SetChgImgOnStart(b)           // Persists the setting in wp.cfg and updates the UI
		chgImgOnStartConfig.InitialValue = b // Update the initial value to reflect the new state of change wallpaper on start
	}
	sm.CreateBoolSetting(&chgImgOnStartConfig, generalContainer) // Use the SettingsManager

	// Nightly Refresh
	nightlyRefreshConfig := setting.BoolConfig{
		Name:         "nightlyRefresh",
		InitialValue: wp.cfg.GetNightlyRefresh(),
		Label:        sm.CreateSettingTitleLabel("Refresh wallpapers nightly:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Useful when using image queries with random elements. Toggling this will start or stop the nightly refresh process."),
		NeedsRefresh: false,
	}
	nightlyRefreshConfig.ApplyFunc = func(b bool) {
		wp.cfg.SetNightlyRefresh(b) // Persists the setting
		nightlyRefreshConfig.InitialValue = b
		if b {
			wp.StartNightlyRefresh()
		} else {
			wp.StopNightlyRefresh()
		}
	}
	sm.CreateBoolSetting(&nightlyRefreshConfig, generalContainer) // Use the SettingsManager

	// Clear Cache
	clearCacheButtonConfig := setting.ButtonWithConfirmationConfig{
		Label:          sm.CreateSettingTitleLabel("Clear Wallpaper Cache:"),
		HelpContent:    sm.CreateSettingDescriptionLabel("Delete all downloaded wallpapers (Source and Derivatives). This is a safety feature."),
		ButtonText:     "Clear Cache",
		ConfirmTitle:   "Clear Cache?",
		ConfirmMessage: "Are you sure? This will delete ALL downloaded images from disk. You will need internet to see new wallpapers.",
		OnPressed:      wp.ClearCache,
	}
	sm.CreateButtonWithConfirmationSetting(&clearCacheButtonConfig, generalContainer) // Use the SettingsManager

	// Reset Blocked Images
	resetButtonConfig := setting.ButtonWithConfirmationConfig{
		Label:          sm.CreateSettingTitleLabel("Blocked Images:"),
		HelpContent:    sm.CreateSettingDescriptionLabel("Clear the blocked images list. Blocked images may be downloaded next time wallpapers are refreshed."),
		ButtonText:     "Reset",
		ConfirmTitle:   "Please Confirm",
		ConfirmMessage: "This cannot be undone. Are you sure?",
		OnPressed:      wp.cfg.ResetAvoidSet,
	}
	sm.CreateButtonWithConfirmationSetting(&resetButtonConfig, generalContainer)

	// --- Dynamic Provider Settings ---

	// Wrap generalContainer in VScroll because it might be tall
	generalScroll := container.NewVScroll(generalContainer)

	// Accordion Items
	var onlineItems []accordionItem
	var localItems []accordionItem

	// Collect all provider names
	var orderedNames []string
	for name := range wp.providers {
		orderedNames = append(orderedNames, name)
	}
	// Sort all alphabetically for deterministic order
	sort.Strings(orderedNames)

	// Logic to auto-select tab if a pending provider is found
	targetTabIndex := 0 // Default to General loop

	for _, name := range orderedNames {
		p, ok := wp.providers[name]
		if !ok {
			continue
		}

		// Check if this provider handles the pending add URL
		providerPendingUrl := ""
		isPendingProvider := false
		if wp.pendingAddUrl != "" {
			if _, err := p.ParseURL(wp.pendingAddUrl); err == nil {
				providerPendingUrl = wp.pendingAddUrl
				isPendingProvider = true

				// Determine target tab index
				switch p.Type() {
				case provider.TypeLocal:
					targetTabIndex = 2 // Local
				case provider.TypeAI:
					targetTabIndex = 3 // AI
				default:
					targetTabIndex = 1 // Online
				}

				// Consume pending URL
				wp.pendingAddUrl = ""
			}
		}

		settingsPanel := p.CreateSettingsPanel(sm)
		queryPanel := p.CreateQueryPanel(sm, providerPendingUrl)

		if settingsPanel == nil && queryPanel == nil {
			continue // Nothing to show for this provider
		}

		var content fyne.CanvasObject
		if settingsPanel != nil && queryPanel != nil {
			// If both exist, put settings on top of queries.
			content = container.NewBorder(settingsPanel, nil, nil, nil, queryPanel)
		} else if settingsPanel != nil {
			content = settingsPanel
		} else {
			content = queryPanel
		}

		// Define Title Generator
		titleFunc := func() string {
			title := p.Title()
			if title == "" {
				title = "Image Sources (" + p.Name() + ")"
			}

			// Count active queries for this provider
			activeCount := 0
			for _, q := range wp.cfg.GetQueries() {
				if q.Provider == p.Name() && q.Active {
					activeCount++
				}
			}
			if activeCount > 0 {
				if activeCount == 1 {
					return fmt.Sprintf("%s (1 active)", title)
				}
				return fmt.Sprintf("%s (%d active)", title, activeCount)
			}
			return title
		}

		item := accordionItem{
			Title:     titleFunc(), // Initial title
			TitleFunc: titleFunc,   // Dynamic title generator
			Content:   content,
			Open:      isPendingProvider, // Auto-open if matched
			Icon:      p.GetProviderIcon(),
		}

		switch p.Type() {
		case provider.TypeLocal:
			localItems = append(localItems, item)
		case provider.TypeAI:
			// aiItems = append(aiItems, item) // TODO: Implement AI tab logic when ready
			continue
		default: // TypeOnline
			onlineItems = append(onlineItems, item)
		}
	}

	onlineTab, refreshOnline := createAccordion(onlineItems)
	localTab, refreshLocal := createAccordion(localItems)
	// Placeholder for AI tab
	aiTab := container.NewStack(widget.NewLabelWithStyle("AI features coming soon...", fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))

	// Register Refresh Callbacks with SettingsManager
	sm.RegisterOnSettingsSaved(func() {
		if refreshOnline != nil {
			refreshOnline()
		}
		if refreshLocal != nil {
			refreshLocal()
		}
	})

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("General", theme.SettingsIcon(), generalScroll),
		container.NewTabItemWithIcon("Online", theme.GridIcon(), onlineTab),
		container.NewTabItemWithIcon("Local", theme.FolderIcon(), localTab),
		container.NewTabItemWithIcon("AI", theme.ComputerIcon(), aiTab),
	)
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
