package wallpaper

import (
	"fmt"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
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
	wp.sourceMenuItem = wp.manager.CreateMenuItem("Image Source", func() {
		go wp.ViewCurrentImageOnWeb()
	}, "view.png")
	items = append(items, wp.sourceMenuItem)
	items = append(items, wp.manager.CreateMenuItem("Delete and Block Image", func() {
		go wp.DeleteCurrentImage()
	}, "delete.png"))

	return items
}

// createSettingEntry creates a widget for a setting entry
func (wp *Plugin) createImgQueryList(sm setting.SettingsManager) *widget.List {

	pendingState := make(map[string]bool) // holds pending active state changes
	var queryList *widget.List
	queryList = widget.NewList(
		func() int {
			return len(wp.cfg.ImageQueries)
		},
		func() fyne.CanvasObject {
			urlLink := widget.NewHyperlink("Placeholder", nil) // Placeholder text
			activeCheck := widget.NewCheck("Active", nil)
			deleteButton := widget.NewButton("Delete", nil)

			return container.NewHBox(urlLink, layout.NewSpacer(), activeCheck, deleteButton)
		},
		func(i int, o fyne.CanvasObject) {
			if i >= len(wp.cfg.ImageQueries) {
				return // Safety check
			}
			// Capture the query object itself, NOT the index 'i'.
			query := wp.cfg.ImageQueries[i]
			queryKey := query.ID // Use stable ID for refresh key

			c := o.(*fyne.Container)

			urlLink := c.Objects[0].(*widget.Hyperlink)
			urlLink.SetText(query.Description)

			siteURL := wp.GetWallhavenURL(query.URL)
			if siteURL != nil {
				urlLink.SetURL(siteURL)
			} else {
				// this should never happen
				// TODO refactor later
				if err := urlLink.SetURLFromString(query.URL); err != nil {
					utilLog.Printf("Failed to set URL from string: %v", err)
				}
			}

			activeCheck := c.Objects[2].(*widget.Check)
			deleteButton := c.Objects[3].(*widget.Button)

			initialActive := query.Active
			activeCheck.OnChanged = nil

			if val, ok := pendingState[queryKey]; ok {
				activeCheck.SetChecked(val)
			} else {
				activeCheck.SetChecked(initialActive)
			}

			activeCheck.OnChanged = func(b bool) {
				if b != initialActive {
					pendingState[queryKey] = b // Store the pending change
					sm.SetSettingChangedCallback(queryKey, func() {
						if b {
							if err := wp.cfg.EnableImageQuery(query.ID); err != nil {
								utilLog.Printf("Failed to enable image query: %v", err)
							}
						} else {
							if err := wp.cfg.DisableImageQuery(query.ID); err != nil {
								utilLog.Printf("Failed to disable image query: %v", err)
							}
						}
						delete(pendingState, queryKey) // Clean up the pending state on apply
					})
					sm.SetRefreshFlag(queryKey)
				} else {
					// User toggled back to the original state, so no change is pending
					delete(pendingState, queryKey)
					sm.RemoveSettingChangedCallback(queryKey)
					sm.UnsetRefreshFlag(queryKey)
				}
				sm.GetCheckAndEnableApplyFunc()()
			}

			deleteButton.OnTapped = func() {
				d := dialog.NewConfirm("Please Confirm", fmt.Sprintf("Are you sure you want to delete %s?", query.Description), func(b bool) {
					if b {
						if query.Active {
							sm.SetRefreshFlag(queryKey)
							sm.GetCheckAndEnableApplyFunc()()
						}
						// Clear any pending state for a deleted item
						delete(pendingState, queryKey)
						if err := wp.cfg.RemoveImageQuery(query.ID); err != nil {
							utilLog.Printf("Failed to remove image query: %v", err)
						}
						queryList.Refresh()
					}

				}, sm.GetSettingsWindow())
				d.Show()
			}
		},
	)
	return queryList
}

// createImgQueryList creates a list of image queries
func (wp *Plugin) createImageQueryPanel(sm setting.SettingsManager) *fyne.Container {

	imgQueryList := wp.createImgQueryList(sm)
	sm.RegisterRefreshFunc(imgQueryList.Refresh)

	var addButton *widget.Button

	addButton = widget.NewButton("Add wallhaven URL", func() {

		urlEntry := widget.NewEntry()
		urlEntry.SetPlaceHolder("Cut and paste a wallhaven search or collection URL from your browser to here")

		descEntry := widget.NewEntry()
		descEntry.SetPlaceHolder("Add a description for these images")

		formStatus := widget.NewLabel("")
		activeBool := widget.NewCheck("Active", nil)

		cancelButton := widget.NewButton("Cancel", nil)
		saveButton := widget.NewButton("Save", nil)
		saveButton.Disable() // Save button is only enabled when the URL is valid and min desc has been added

		formValidator := func(who *widget.Entry) bool {
			urlStrErr := urlEntry.Validate()
			descStrErr := descEntry.Validate()

			if urlStrErr != nil {
				if who == urlEntry {
					formStatus.SetText(urlStrErr.Error())
					formStatus.Importance = widget.DangerImportance
				}
				formStatus.Refresh()
				return false // URL syntax is wrong
			}

			if descStrErr != nil {
				if who == descEntry {
					formStatus.SetText(descStrErr.Error())
					formStatus.Importance = widget.DangerImportance
				}
				formStatus.Refresh()
				return false // Description is wrong
			}

			apiURL, _, err := CovertWebToAPIURL(urlEntry.Text)
			if err != nil {
				if who == urlEntry {
					formStatus.SetText(fmt.Sprintf("URL conversion error: %v", err))
					formStatus.Importance = widget.DangerImportance
				}
				formStatus.Refresh()
				return false // URL is not convertible
			}

			queryID := GenerateQueryID(apiURL) // Using our new exported function
			if wp.cfg.IsDuplicateID(queryID) {
				if who == urlEntry || (who == descEntry && urlEntry.Text != "") {
					formStatus.SetText("Duplicate query: this URL already exists")
					formStatus.Importance = widget.DangerImportance
				}
				formStatus.Refresh()
				return false // It's a duplicate!
			}

			formStatus.SetText("Everything looks good")
			formStatus.Importance = widget.SuccessImportance
			formStatus.Refresh()
			return true
		}

		urlEntry.Validator = validation.NewRegexp(WallhavenURLRegexp, "Invalid wallhaven image query URL pattern")
		descEntry.Validator = validation.NewRegexp(WallhavenDescRegexp, fmt.Sprintf("Description must be between 5 and %d alpha numeric characters long", MaxDescLength))

		newEntryLengthChecker := func(entry *widget.Entry, maxLen int) func(string) {
			{
				return func(s string) {
					if len(s) > maxLen {
						entry.SetText(s[:maxLen]) // Truncate to max length
						return                    // Stop further processing
					}

					if formValidator(entry) {
						saveButton.Enable()
					} else {
						saveButton.Disable()
					}
				}
			}
		}
		urlEntry.OnChanged = newEntryLengthChecker(urlEntry, MaxURLLength)
		descEntry.OnChanged = newEntryLengthChecker(descEntry, MaxDescLength)

		c := container.NewVBox()
		c.Add(sm.CreateSettingTitleLabel("wallhaven Search or Collection (Favorites) URL:"))
		c.Add(urlEntry)
		c.Add(sm.CreateSettingTitleLabel("Description:"))
		c.Add(descEntry)
		c.Add(formStatus)
		c.Add(activeBool)
		c.Add(widget.NewSeparator())
		c.Add(container.NewHBox(cancelButton, layout.NewSpacer(), saveButton))

		d := dialog.NewCustomWithoutButtons("New Image Query", c, sm.GetSettingsWindow())
		d.Resize(fyne.NewSize(800, 200))

		saveButton.OnTapped = func() {

			apiURL, queryType, err := CovertWebToAPIURL(urlEntry.Text) // Convert web URL to API URL
			if err != nil {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
				return
			}

			err = wp.CheckWallhavenURL(apiURL, queryType)
			if err != nil {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
				return
			}

			// We already checked for duplicates, but we check err just in case.
			newID, err := wp.cfg.AddImageQuery(descEntry.Text, apiURL, activeBool.Checked)
			if err != nil {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
				return // Don't close the dialog
			}

			addButton.Enable()
			imgQueryList.Refresh()

			if activeBool.Checked {
				sm.SetRefreshFlag(newID)
				sm.GetCheckAndEnableApplyFunc()()
			}
			d.Hide()
			addButton.Enable()
		}

		cancelButton.OnTapped = func() {
			d.Hide()
			addButton.Enable()
		}

		d.Show()
		addButton.Disable()
	})

	header := container.NewVBox()
	header.Add(sm.CreateSettingTitleLabel("wallhaven Queries and Collections (Favorites)"))
	header.Add(sm.CreateSettingDescriptionLabel("Manage your wallhaven.cc image queries and collections here. Paste your image search or collection URL and Spice will take care of the rest."))
	header.Add(addButton)
	qpContainer := container.NewBorder(header, nil, nil, nil, imgQueryList)
	return qpContainer
}

// createUnsplashQueryList creates a widget for an Unsplash query entry
func (wp *Plugin) createUnsplashQueryList(sm setting.SettingsManager) *widget.List {

	pendingState := make(map[string]bool) // holds pending active state changes
	var queryList *widget.List
	queryList = widget.NewList(
		func() int {
			return len(wp.cfg.UnsplashQueries)
		},
		func() fyne.CanvasObject {
			urlLink := widget.NewHyperlink("Placeholder", nil) // Placeholder text
			activeCheck := widget.NewCheck("Active", nil)
			deleteButton := widget.NewButton("Delete", nil)

			return container.NewHBox(urlLink, layout.NewSpacer(), activeCheck, deleteButton)
		},
		func(i int, o fyne.CanvasObject) {
			if i >= len(wp.cfg.UnsplashQueries) {
				return // Safety check
			}
			// Capture the query object itself, NOT the index 'i'.
			query := wp.cfg.UnsplashQueries[i]
			queryKey := query.ID // Use stable ID for refresh key

			c := o.(*fyne.Container)

			urlLink := c.Objects[0].(*widget.Hyperlink)
			urlLink.SetText(query.Description)

			// For Unsplash, we can just use the URL as is for now, or parse it if needed.
			// Since we store the web URL, we can use it.
			if u, err := url.Parse(query.URL); err == nil {
				urlLink.SetURL(u)
			} else {
				if err := urlLink.SetURLFromString(query.URL); err != nil {
					utilLog.Printf("Failed to set URL from string: %v", err)
				}
			}

			activeCheck := c.Objects[2].(*widget.Check)
			deleteButton := c.Objects[3].(*widget.Button)

			initialActive := query.Active
			activeCheck.OnChanged = nil

			if val, ok := pendingState[queryKey]; ok {
				activeCheck.SetChecked(val)
			} else {
				activeCheck.SetChecked(initialActive)
			}

			activeCheck.OnChanged = func(b bool) {
				if b != initialActive {
					pendingState[queryKey] = b // Store the pending change
					sm.SetSettingChangedCallback(queryKey, func() {
						if b {
							if err := wp.cfg.EnableUnsplashQuery(query.ID); err != nil {
								utilLog.Printf("Failed to enable Unsplash query: %v", err)
							}
						} else {
							if err := wp.cfg.DisableUnsplashQuery(query.ID); err != nil {
								utilLog.Printf("Failed to disable Unsplash query: %v", err)
							}
						}
						delete(pendingState, queryKey) // Clean up the pending state on apply
					})
					sm.SetRefreshFlag(queryKey)
				} else {
					// User toggled back to the original state, so no change is pending
					delete(pendingState, queryKey)
					sm.RemoveSettingChangedCallback(queryKey)
					sm.UnsetRefreshFlag(queryKey)
				}
				sm.GetCheckAndEnableApplyFunc()()
			}

			deleteButton.OnTapped = func() {
				d := dialog.NewConfirm("Please Confirm", fmt.Sprintf("Are you sure you want to delete %s?", query.Description), func(b bool) {
					if b {
						if query.Active {
							sm.SetRefreshFlag(queryKey)
							sm.GetCheckAndEnableApplyFunc()()
						}
						// Clear any pending state for a deleted item
						delete(pendingState, queryKey)
						if err := wp.cfg.RemoveUnsplashQuery(query.ID); err != nil {
							utilLog.Printf("Failed to remove Unsplash query: %v", err)
						}
						queryList.Refresh()
					}

				}, sm.GetSettingsWindow())
				d.Show()
			}
		},
	)
	return queryList
}

// createUnsplashQueryPanel creates a list of Unsplash image queries
func (wp *Plugin) createUnsplashQueryPanel(sm setting.SettingsManager) *fyne.Container {

	imgQueryList := wp.createUnsplashQueryList(sm)
	sm.RegisterRefreshFunc(imgQueryList.Refresh)

	var addButton *widget.Button

	addButton = widget.NewButton("Add Unsplash URL", func() {

		urlEntry := widget.NewEntry()
		urlEntry.SetPlaceHolder("Paste an Unsplash search or collection URL")

		descEntry := widget.NewEntry()
		descEntry.SetPlaceHolder("Add a description")

		formStatus := widget.NewLabel("")
		activeBool := widget.NewCheck("Active", nil)

		cancelButton := widget.NewButton("Cancel", nil)
		saveButton := widget.NewButton("Save", nil)
		saveButton.Disable()

		formValidator := func(who *widget.Entry) bool {
			// Basic validation
			if urlEntry.Text == "" {
				return false
			}
			if descEntry.Text == "" {
				return false
			}

			// Validate URL format using UnsplashProvider logic
			// We need an instance of UnsplashProvider or access to its logic.
			// Since we are in `wallpaper` package, we can create a temporary one or use a helper.
			// But `UnsplashProvider` needs config and client.
			// We can just use `wp.providers["Unsplash"]` if available.
			provider, ok := wp.providers["Unsplash"]
			if !ok {
				formStatus.SetText("Unsplash provider not initialized")
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
				return false
			}

			_, err := provider.ParseURL(urlEntry.Text)
			if err != nil {
				if who == urlEntry {
					formStatus.SetText(fmt.Sprintf("Invalid Unsplash URL: %v", err))
					formStatus.Importance = widget.DangerImportance
				}
				formStatus.Refresh()
				return false
			}

			// Check for duplicates
			// We need to generate ID first.
			// UnsplashProvider doesn't export GenerateID, but Config uses GenerateQueryID(url).
			// We can use that.
			queryID := GenerateQueryID(urlEntry.Text)
			if wp.cfg.IsDuplicateID(queryID) {
				if who == urlEntry || (who == descEntry && urlEntry.Text != "") {
					formStatus.SetText("Duplicate query: this URL already exists")
					formStatus.Importance = widget.DangerImportance
				}
				formStatus.Refresh()
				return false
			}

			formStatus.SetText("Everything looks good")
			formStatus.Importance = widget.SuccessImportance
			formStatus.Refresh()
			return true
		}

		urlEntry.OnChanged = func(s string) {
			if formValidator(urlEntry) {
				saveButton.Enable()
			} else {
				saveButton.Disable()
			}
		}
		descEntry.OnChanged = func(s string) {
			if formValidator(descEntry) {
				saveButton.Enable()
			} else {
				saveButton.Disable()
			}
		}

		c := container.NewVBox()
		c.Add(sm.CreateSettingTitleLabel("Unsplash Search or Collection URL:"))
		c.Add(urlEntry)
		c.Add(sm.CreateSettingTitleLabel("Description:"))
		c.Add(descEntry)
		c.Add(formStatus)
		c.Add(activeBool)
		c.Add(widget.NewSeparator())
		c.Add(container.NewHBox(cancelButton, layout.NewSpacer(), saveButton))

		d := dialog.NewCustomWithoutButtons("New Unsplash Query", c, sm.GetSettingsWindow())
		d.Resize(fyne.NewSize(800, 200))

		saveButton.OnTapped = func() {
			newID, err := wp.cfg.AddUnsplashQuery(descEntry.Text, urlEntry.Text, activeBool.Checked)
			if err != nil {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
				return
			}

			addButton.Enable()
			imgQueryList.Refresh()

			if activeBool.Checked {
				sm.SetRefreshFlag(newID)
				sm.GetCheckAndEnableApplyFunc()()
			}
			d.Hide()
			addButton.Enable()
		}

		cancelButton.OnTapped = func() {
			d.Hide()
			addButton.Enable()
		}

		d.Show()
		addButton.Disable()
	})

	header := container.NewVBox()
	header.Add(sm.CreateSettingTitleLabel("Unsplash Queries"))
	header.Add(sm.CreateSettingDescriptionLabel("Manage your Unsplash image queries here."))
	header.Add(addButton)
	qpContainer := container.NewBorder(header, nil, nil, nil, imgQueryList)
	return qpContainer
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

	// --- Wallhaven Settings Container ---
	whHeader := container.NewVBox()

	// Wallhaven API Key
	whURL, _ := url.Parse("https://wallhaven.cc/settings/account")
	wallhavenAPIKeyConfig := setting.TextEntrySettingConfig{
		Name:              "wallhavenAPIKey",
		InitialValue:      wp.cfg.GetWallhavenAPIKey(),
		PlaceHolder:       "Enter your wallhaven.cc API Key",
		Label:             sm.CreateSettingTitleLabel("wallhaven API Key:"),
		HelpContent:       widget.NewHyperlink("Restricted content requires an API key. Get one here.", whURL),
		Validator:         validation.NewRegexp(WallhavenAPIKeyRegexp, "32 alphanumeric characters required"),
		NeedsRefresh:      true,
		DisplayStatus:     true,
		PostValidateCheck: CheckWallhavenAPIKey,
	}
	wallhavenAPIKeyConfig.ApplyFunc = func(s string) {
		wp.cfg.SetWallhavenAPIKey(s)           // Update the wallpaper configuration with the new API key
		wallhavenAPIKeyConfig.InitialValue = s // Update the initial value of the text entry setting with the new API key
	}
	sm.CreateTextEntrySetting(&wallhavenAPIKeyConfig, whHeader) // Use the SettingsManager

	// Clear API Key Button
	clearKeyBtn := widget.NewButton("Clear API Key", func() {
		dialog.NewConfirm("Clear API Key", "Are you sure you want to clear the Wallhaven API Key?", func(b bool) {
			if b {
				wp.cfg.SetWallhavenAPIKey("")
				// Since we can't easily clear the entry widget directly without refactoring SettingsManager,
				// and the config has NeedsRefresh: true, we can try to trigger a refresh or just notify the user.
				// Ideally, we would update the entry text.
				// For now, we rely on the user restarting or reopening settings if they want to see it gone immediately,
				// or we can try to force a refresh if we had access to it.
				// Actually, we can just update the config and let the user know.
				// A better UX might be to just set the text entry to empty if we had it.
				// Given the constraints, we'll just clear the config.
				// To make it visible, we can ask the user to reopen settings or just accept it's cleared in backend.
				wallhavenAPIKeyConfig.InitialValue = "" // Update for next render
				wp.manager.NotifyUser("Settings", "Wallhaven API Key cleared")
			}
		}, sm.GetSettingsWindow()).Show()
	})
	whHeader.Add(clearKeyBtn)

	qp := wp.createImageQueryPanel(sm) // Create image query panel
	// qp is a Border layout. We want whHeader at the top.
	// We can wrap qp in another Border layout with whHeader at the top.
	wallhavenContainer := container.NewBorder(whHeader, nil, nil, nil, qp)

	// --- Unsplash Settings Container ---
	usHeader := container.NewVBox()

	// Unsplash Settings
	// usHeader is already declared above

	// Connect Button and Status
	var connectBtn *widget.Button
	statusLabel := widget.NewLabel("")

	updateStatus := func() {
		if wp.cfg.GetUnsplashToken() != "" {
			statusLabel.SetText("Status: Connected")
			statusLabel.Importance = widget.SuccessImportance
			connectBtn.SetText("Reconnect Unsplash")
		} else {
			statusLabel.SetText("Status: Not Connected")
			statusLabel.Importance = widget.DangerImportance
			connectBtn.SetText("Connect Unsplash")
		}
	}

	connectBtn = widget.NewButton("Connect Unsplash", func() {
		connectBtn.Disable()
		statusLabel.SetText("Status: Connecting... Check your browser.")

		go func() {
			authenticator := NewUnsplashAuthenticator(wp.cfg, wp.httpClient)
			err := authenticator.StartOAuthFlow(wp.manager.OpenURL)

			fyne.Do(func() {
				if err != nil {
					utilLog.Printf("OAuth failed: %v", err)
					statusLabel.SetText("Status: Failed. Check logs.")
					statusLabel.Importance = widget.DangerImportance

					errWin := fyne.CurrentApp().NewWindow("Authentication Failed")
					errWin.SetContent(widget.NewLabel(fmt.Sprintf("Error: %v", err)))
					errWin.Resize(fyne.NewSize(400, 150))
					errWin.Show()
				} else {
					statusLabel.SetText("Status: Connected")
					statusLabel.Importance = widget.SuccessImportance
				}
				connectBtn.Enable()
				updateStatus()
			})
		}()
	})

	disconnectBtn := widget.NewButton("Disconnect", func() {
		dialog.NewConfirm("Disconnect Unsplash", "Are you sure you want to disconnect? This will remove the local access token.", func(b bool) {
			if b {
				wp.cfg.SetUnsplashToken("")
				updateStatus()
			}
		}, sm.GetSettingsWindow()).Show()
	})
	disconnectBtn.Importance = widget.DangerImportance

	// Update the updateStatus function to handle visibility
	// We need to re-define updateStatus to capture disconnectBtn
	updateStatus = func() {
		if wp.cfg.GetUnsplashToken() != "" {
			statusLabel.SetText("Status: Connected")
			statusLabel.Importance = widget.SuccessImportance
			connectBtn.SetText("Reconnect Unsplash")
			connectBtn.Hide()
			disconnectBtn.Show()
		} else {
			statusLabel.SetText("Status: Not Connected")
			statusLabel.Importance = widget.DangerImportance
			connectBtn.SetText("Connect Unsplash")
			connectBtn.Show()
			disconnectBtn.Hide()
		}
	}

	updateStatus() // Initial state

	usHeader.Add(sm.CreateSettingTitleLabel("Unsplash Account:"))
	usHeader.Add(sm.CreateSettingDescriptionLabel("Connect your Unsplash account to access your collections and higher rate limits."))
	usHeader.Add(container.NewHBox(connectBtn, disconnectBtn, statusLabel))

	usQp := wp.createUnsplashQueryPanel(sm)
	unsplashContainer := container.NewBorder(usHeader, nil, nil, nil, usQp)

	// --- Accordion ---
	// Wrap generalContainer in VScroll because it might be tall
	generalScroll := container.NewVScroll(generalContainer)

	// --- Smart Accordion Logic ---
	// We build a custom accordion to support the "auto-expand next" behavior requested by the user.

	// Accordion Items
	items := []struct {
		Title   string
		Content fyne.CanvasObject
		Open    bool
	}{
		{"General Settings", generalScroll, true},
		{"Image Sources (Wallhaven)", wallhavenContainer, false},
		{"Image Sources (Unsplash)", unsplashContainer, false},
	}

	// Container to hold the accordion
	accordionContainer := container.NewStack() // Use Stack layout to hold the Border layout

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
			// No error check needed for theme icons

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
			// headerBtn.Importance = widget.LowImportance // Removed to improve visibility (default background)

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
		// Top: topHeaders
		// Bottom: bottomHeaders
		// Center: centerContent
		borderLayout := container.NewBorder(topHeaders, bottomHeaders, nil, nil, centerContent)

		accordionContainer.Objects = []fyne.CanvasObject{borderLayout}
		accordionContainer.Refresh()
	}

	// Initial Render
	refreshAccordion()

	// Add "Manage Image Sources" button to General Settings
	// Now we can access the 'items' and 'refreshAccordion'
	manageSourcesBtn := widget.NewButton("Manage Image Sources", func() {
		// Open the second item (Image Sources)
		for j := range items {
			items[j].Open = (j == 1)
		}
		refreshAccordion()
	})
	// Add a separator and the button to the end of the general container
	generalContainer.Add(widget.NewSeparator())
	generalContainer.Add(manageSourcesBtn)

	// Wrap accordion in a container to return.
	// Since accordion items expand, we just return the accordion.
	// However, CreatePrefsPanel returns *fyne.Container. widget.Accordion is a Widget.
	// We need to wrap it.
	return container.NewStack(accordionContainer)
}
