package wallpaper

import (
	"fmt"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
)

// CreateTrayMenuItems creates the menu items for the tray menu
func (wp *wallpaperPlugin) CreateTrayMenuItems() []*fyne.MenuItem {
	items := []*fyne.MenuItem{}
	items = append(items, wp.manager.CreateMenuItem("Next Wallpaper", func() {
		go wp.SetNextWallpaper()
	}, "next.png"))
	items = append(items, wp.manager.CreateMenuItem("Prev Wallpaper", func() {
		go wp.SetPreviousWallpaper()
	}, "prev.png"))
	items = append(items, wp.manager.CreateToggleMenuItem("Shuffle Wallpapers", wp.SetShuffleImage, "shuffle.png", wp.cfg.GetImgShuffle()))
	items = append(items, fyne.NewMenuItemSeparator())
	items = append(items, wp.manager.CreateMenuItem("Image Source", func() {
		go wp.ViewCurrentImageOnWeb()
	}, "view.png"))
	items = append(items, wp.manager.CreateMenuItem("Delete and Block Image", func() {
		go wp.DeleteCurrentImage()
	}, "delete.png"))
	return items
}

// createSettingEntry creates a widget for a setting entry
func (wp *wallpaperPlugin) createImgQueryList(sm setting.SettingsManager) *widget.List {

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
				urlLink.SetURLFromString(query.URL)
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
							wp.cfg.EnableImageQuery(query.ID)
						} else {
							wp.cfg.DisableImageQuery(query.ID)
						}
						initialActive = b // Update the closure's captured variable
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
						wp.cfg.RemoveImageQuery(query.ID)
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
func (wp *wallpaperPlugin) createImageQueryPanel(sm setting.SettingsManager) *fyne.Container {

	imgQueryList := wp.createImgQueryList(sm)
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

// CreateWallpaperPreferences creates a preferences widget for wallpaper settings
func (wp *wallpaperPlugin) CreatePrefsPanel(sm setting.SettingsManager) *fyne.Container {
	header := container.NewVBox()
	footer := container.NewVBox()
	prefsPanel := container.NewBorder(header, footer, nil, nil)

	header.Add(sm.CreateSectionTitleLabel("Wallpaper Preferences"))

	// Change Frequency
	var frequencyConfig setting.SelectConfig
	frequencyConfig = setting.SelectConfig{
		Name:         "changeFrequency",
		Options:      setting.StringOptions(GetFrequencies()),
		InitialValue: int(wp.cfg.GetWallpaperChangeFrequency()),
		Label:        sm.CreateSettingTitleLabel("Wallpaper Change Frequency:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Set how often the wallpaper changes. Set to never to disable wallpaper changes."),
		ApplyFunc: func(val interface{}) {
			selectedFrequency := Frequency(val.(int))
			wp.cfg.SetWallpaperChangeFrequency(selectedFrequency) // Persists new frequency in configuration
			wp.ChangeWallpaperFrequency(selectedFrequency)        // Activate the new frequency in wallpaper plugin
			frequencyConfig.InitialValue = int(selectedFrequency) // Update initial value for frequency
		},
	}
	sm.CreateSelectSetting(&frequencyConfig, header)

	// Cache Size
	var cacheSizeConfig setting.SelectConfig
	cacheSizeConfig = setting.SelectConfig{
		Name:         "cacheSize",
		Options:      setting.StringOptions(GetCacheSizes()), // Correctly calling GetCacheSizes
		InitialValue: int(wp.cfg.GetCacheSize()),
		Label:        sm.CreateSettingTitleLabel("Cache Size:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Set how many images to cache for faster startup and less network usage. Set to none to disable caching."),
		ApplyFunc: func(val interface{}) {
			selectedCacheSize := CacheSize(val.(int))
			wp.cfg.SetCacheSize(selectedCacheSize)                // Persists new cache size in configuration
			cacheSizeConfig.InitialValue = int(selectedCacheSize) // Update initial value for cache size
		},
	}
	sm.CreateSelectSetting(&cacheSizeConfig, header)

	// Smart Fit
	var smartFitConfig setting.BoolConfig
	smartFitConfig = setting.BoolConfig{
		Name:         "smartFit",
		InitialValue: wp.cfg.GetSmartFit(),
		Label:        sm.CreateSettingTitleLabel("Scale Wallpaper to Fit Screen:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Smart Fit analizes wallpapers to find best way to scale and crop them to fit your screen."),
		ApplyFunc: func(b bool) {
			wp.cfg.SetSmartFit(b)           // Persists the setting in wp.cfg and updates the UI
			wp.SetSmartFit(b)               // Activates smart fit in the wallpaper engine
			smartFitConfig.InitialValue = b // Update the initial value to reflect the new state of smart fit
		},
		NeedsRefresh: true,
	}
	sm.CreateBoolSetting(&smartFitConfig, header) // Use the SettingsManager

	// Change Wallpaper on Start
	var chgImgOnStartConfig setting.BoolConfig
	chgImgOnStartConfig = setting.BoolConfig{
		Name:         "chgImgOnStart",
		InitialValue: wp.cfg.GetChgImgOnStart(),
		Label:        sm.CreateSettingTitleLabel("Change wallpaper on start:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Disable if you prefer the wallpaper to change only based on its timer or a manual refresh."),
		ApplyFunc: func(b bool) {
			wp.cfg.SetChgImgOnStart(b)           // Persists the setting in wp.cfg and updates the UI
			chgImgOnStartConfig.InitialValue = b // Update the initial value to reflect the new state of change wallpaper on start
		},
		NeedsRefresh: false,
	}
	sm.CreateBoolSetting(&chgImgOnStartConfig, header) // Use the SettingsManager

	// Nightly Refresh
	var nightlyRefreshConfig setting.BoolConfig
	nightlyRefreshConfig = setting.BoolConfig{
		Name:         "nightlyRefresh",
		InitialValue: wp.cfg.GetNightlyRefresh(),
		Label:        sm.CreateSettingTitleLabel("Refresh wallpapers nightly:"),
		HelpContent:  sm.CreateSettingDescriptionLabel("Useful when using image queries with random elements. Requires application restart to take effect."),
		ApplyFunc: func(b bool) {
			wp.cfg.SetNightlyRefresh(b)           // Persists the setting in wp.cfg and updates the UI
			nightlyRefreshConfig.InitialValue = b // Update the initial value to reflect the new state of nightly refresh
		},
		NeedsRefresh: false,
	}
	sm.CreateBoolSetting(&nightlyRefreshConfig, header) // Use the SettingsManager

	// Reset Blocked Images
	resetButtonConfig := setting.ButtonWithConfirmationConfig{
		Label:          sm.CreateSettingTitleLabel("Blocked Images:"),
		HelpContent:    sm.CreateSettingDescriptionLabel("Clear the blocked images list. Blocked images may be downloaded next time wallpapers are refreshed."),
		ButtonText:     "Reset",
		ConfirmTitle:   "Please Confirm",
		ConfirmMessage: "This cannot be undone. Are you sure?",
		OnPressed:      wp.cfg.ResetAvoidSet,
	}
	sm.CreateButtonWithConfirmationSetting(&resetButtonConfig, header) // Use the SettingsManager

	// wallhaven service section
	header.Add(widget.NewSeparator())
	header.Add(sm.CreateSectionTitleLabel("wallhaven.cc Image Service Preferences"))

	// Wallhaven API Key
	whURL, _ := url.Parse("https://wallhaven.cc/settings/account")
	var wallhavenAPIKeyConfig setting.TextEntrySettingConfig
	wallhavenAPIKeyConfig = setting.TextEntrySettingConfig{
		Name:         "wallhavenAPIKey",
		InitialValue: wp.cfg.GetWallhavenAPIKey(),
		PlaceHolder:  "Enter your wallhaven.cc API Key",
		Label:        sm.CreateSettingTitleLabel("wallhaven API Key:"),
		HelpContent:  widget.NewHyperlink("Restricted content requires an API key. Get one here.", whURL),
		Validator:    validation.NewRegexp(WallhavenAPIKeyRegexp, "32 alphanumeric characters required"),
		ApplyFunc: func(s string) {
			wp.cfg.SetWallhavenAPIKey(s)           // Update the wallpaper configuration with the new API key
			wallhavenAPIKeyConfig.InitialValue = s // Update the initial value of the text entry setting with the new API key
		},
		NeedsRefresh:      true,
		DisplayStatus:     true,
		PostValidateCheck: CheckWallhavenAPIKey,
	}
	sm.CreateTextEntrySetting(&wallhavenAPIKeyConfig, header) // Use the SettingsManager

	qp := wp.createImageQueryPanel(sm) // Create image query panel
	prefsPanel.Add(qp)                 // Add image query panel to preferences panel

	footer.Add(widget.NewSeparator())
	sm.RegisterRefreshFunc(func() {
		wp.RefreshImagesAndPulse()
	})

	return prefsPanel
}
