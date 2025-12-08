package wallpaper

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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
	items = append(items, fyne.NewMenuItemSeparator())
	// Provider Info (Static)
	wp.providerMenuItem = wp.manager.CreateMenuItem("Initializing...", nil, "")
	items = append(items, wp.providerMenuItem)

	// Artist/Source Link (Clickable)
	wp.artistMenuItem = wp.manager.CreateMenuItem("Unknown", func() {
		go wp.ViewCurrentImageOnWeb()
	}, "view.png")
	items = append(items, wp.artistMenuItem)
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
		HelpContent:  sm.CreateSettingDescriptionLabel("Set how often the wallpaper changes. Set to never to disable wallpaper changes."),
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
		HelpContent:  sm.CreateSettingDescriptionLabel("Set how many images to cache for faster startup and less network usage. Set to none to disable caching."),
	}
	cacheSizeConfig.ApplyFunc = func(val interface{}) {
		selectedCacheSize := CacheSize(val.(int))
		wp.cfg.SetCacheSize(selectedCacheSize)                // Persists new cache size in configuration
		cacheSizeConfig.InitialValue = int(selectedCacheSize) // Update initial value for cache size
	}
	sm.CreateSelectSetting(&cacheSizeConfig, generalContainer)

	// Smart Fit
	smartFitConfig := setting.BoolConfig{
		Name:         "smartFit",
		InitialValue: wp.cfg.GetSmartFit(),
		Label:        sm.CreateSettingTitleLabel("Scale Wallpaper to Fit Screen:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Smart Fit analizes wallpapers to find best way to scale and crop them to fit your screen."),
		NeedsRefresh: true,
	}
	smartFitConfig.ApplyFunc = func(b bool) {
		wp.cfg.SetSmartFit(b)           // Persists the setting in wp.cfg and updates the UI
		wp.SetSmartFit(b)               // Activates smart fit in the wallpaper engine
		smartFitConfig.InitialValue = b // Update the initial value to reflect the new state of smart fit
	}
	sm.CreateBoolSetting(&smartFitConfig, generalContainer) // Use the SettingsManager

	// Face Crop and Face Boost configs (pre-declared for mutual access)
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
	faceCropCheck := sm.CreateBoolSetting(&faceCropConfig, subSettingsContainer)
	faceBoostCheck := sm.CreateBoolSetting(&faceBoostConfig, subSettingsContainer)

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

	// Link both to Smart Fit
	if !wp.cfg.GetSmartFit() {
		faceCropCheck.Disable()
		faceBoostCheck.Disable()
	}

	smartFitConfig.OnChanged = func(b bool) {
		if b {
			faceCropCheck.Enable()
			faceBoostCheck.Enable()
		} else {
			faceCropCheck.SetChecked(false)
			faceCropCheck.Disable()
			faceBoostCheck.SetChecked(false)
			faceBoostCheck.Disable()
		}
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

	// Reset Blocked Images
	resetButtonConfig := setting.ButtonWithConfirmationConfig{
		Label:          sm.CreateSettingTitleLabel("Blocked Images:"),
		HelpContent:    sm.CreateSettingDescriptionLabel("Clear the blocked images list. Blocked images may be downloaded next time wallpapers are refreshed."),
		ButtonText:     "Reset",
		ConfirmTitle:   "Please Confirm",
		ConfirmMessage: "This cannot be undone. Are you sure?",
		OnPressed:      wp.cfg.ResetAvoidSet,
	}
	sm.CreateButtonWithConfirmationSetting(&resetButtonConfig, generalContainer) // Use the SettingsManager

	// --- Dynamic Provider Settings ---

	// Wrap generalContainer in VScroll because it might be tall
	generalScroll := container.NewVScroll(generalContainer)

	// Accordion Items
	items := []struct {
		Title   string
		Content fyne.CanvasObject
		Open    bool
	}{
		{"General Settings", generalScroll, true},
	}

	// We want Wallhaven, Unsplash, Pexels order
	orderedNames := []string{"Wallhaven", "Unsplash", "Pexels"}
	// Add other providers found in map but not in orderedNames
	for name := range wp.providers {
		found := false
		for _, on := range orderedNames {
			if on == name {
				found = true
				break
			}
		}
		if !found {
			orderedNames = append(orderedNames, name)
		}
	}

	for _, name := range orderedNames {
		p, ok := wp.providers[name]
		if !ok {
			continue
		}

		settingsPanel := p.CreateSettingsPanel(sm)
		queryPanel := p.CreateQueryPanel(sm)

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

		title := p.Title()
		if title == "" {
			title = "Image Sources (" + p.Name() + ")"
		}

		items = append(items, struct {
			Title   string
			Content fyne.CanvasObject
			Open    bool
		}{
			Title:   title,
			Content: content,
			Open:    false,
		})
	}

	// Container to hold the accordion
	accordionContainer := container.NewStack()

	// Function to refresh the accordion UI
	var refreshAccordion func()

	refreshAccordion = func() {
		topHeaders := container.NewVBox()
		bottomHeaders := container.NewVBox()
		var centerContent fyne.CanvasObject

		foundOpen := false

		for i := range items {
			index := i // Capture loop variable
			item := &items[index]

			// Header Button
			var icon fyne.Resource
			if item.Open {
				icon = theme.MoveDownIcon()
			} else {
				icon = theme.NavigateNextIcon()
			}

			// If we don't have icons, we can use text arrows
			titleText := item.Title

			headerBtn := widget.NewButton(titleText, func() {
				if item.Open {
					// If closing, open the next one (wrapping around)
					item.Open = false
					nextIndex := (index + 1) % len(items)
					items[nextIndex].Open = true
				} else {
					// If opening, close all others (Single Expansion)
					for j := range items {
						items[j].Open = (j == index)
					}
				}
				refreshAccordion()
			})
			headerBtn.Icon = icon
			headerBtn.Alignment = widget.ButtonAlignLeading

			if item.Open {
				topHeaders.Add(headerBtn)
				centerContent = item.Content
				foundOpen = true
			} else {
				if foundOpen {
					bottomHeaders.Add(headerBtn)
				} else {
					topHeaders.Add(headerBtn)
				}
			}
		}

		// Create the border layout
		borderLayout := container.NewBorder(topHeaders, bottomHeaders, nil, nil, centerContent)

		accordionContainer.Objects = []fyne.CanvasObject{borderLayout}
		accordionContainer.Refresh()
	}

	// Initial Render
	refreshAccordion()

	// Add "Manage Image Sources" button to General Settings
	manageSourcesBtn := widget.NewButton("Manage Image Sources", func() {
		// Open the second item (First Image Source)
		if len(items) > 1 {
			for j := range items {
				items[j].Open = (j == 1)
			}
			refreshAccordion()
		}
	})
	// Add a separator and the button to the end of the general container
	generalContainer.Add(widget.NewSeparator())
	generalContainer.Add(manageSourcesBtn)

	// Wrap accordion in a container to return.
	return container.NewStack(accordionContainer)
}
