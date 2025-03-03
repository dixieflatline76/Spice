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
	"github.com/dixieflatline76/Spice/ui"
)

// TODO: Major refactor needed, this is more of a proof of concept, optimally we move the apply logic and the status maps to the plugin manager
// TODO: Should make use of structs to minimize parameter passing

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
func (wp *wallpaperPlugin) createImgQueryList(prefsWindow fyne.Window, chgPrefsCallbacks *map[string]func(), refresh *map[string]bool, checkAndEnableApply func()) *widget.List {

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

			queryKey := fmt.Sprintf("img_query_%d", i)
			c := o.(*fyne.Container)

			urlLink := c.Objects[0].(*widget.Hyperlink)
			urlLink.SetText(wp.cfg.ImageQueries[i].Description)

			siteURL := wp.GetWallhavenURL(wp.cfg.ImageQueries[i].URL)
			if siteURL != nil {
				urlLink.SetURL(siteURL)
			} else {
				// this should never happen
				// TODO refactor later
				urlLink.SetURLFromString(wp.cfg.ImageQueries[i].URL)
			}

			activeCheck := c.Objects[2].(*widget.Check)
			deleteButton := c.Objects[3].(*widget.Button)

			initialActive := wp.cfg.ImageQueries[i].Active
			activeCheck.SetChecked(initialActive)
			activeCheck.OnChanged = func(b bool) {
				if b != initialActive { // only add callback if the value has changed
					(*chgPrefsCallbacks)[queryKey] = func() {
						if b {
							wp.cfg.EnableImageQuery(i)
						} else {
							wp.cfg.DisableImageQuery(i)
						}
						initialActive = b // update initial value
					}
					(*refresh)[queryKey] = true // mark for refresh
				} else {
					delete(*chgPrefsCallbacks, queryKey) // remove callback
					delete(*refresh, queryKey)           // remove refresh
				}
				checkAndEnableApply()
			}

			deleteButton.OnTapped = func() {
				d := dialog.NewConfirm("Please Confirm", fmt.Sprintf("Are you sure you want to delete %s?", wp.cfg.ImageQueries[i].Description), func(b bool) {
					if b {
						if wp.cfg.ImageQueries[i].Active {
							(*refresh)[queryKey] = true
							checkAndEnableApply()
						}

						wp.cfg.RemoveImageQuery(i)
						queryList.Refresh()
					}

				}, prefsWindow)
				d.Show()
			}
		},
	)
	return queryList
}

// createImgQueryList creates a list of image queries
func (wp *wallpaperPlugin) createImageQueryPanel(prefsWindow fyne.Window, parent *fyne.Container, chgPrefsCallbacks *map[string]func(), refresh *map[string]bool, checkAndEnableApply func()) {

	imgQueryList := wp.createImgQueryList(prefsWindow, chgPrefsCallbacks, refresh, checkAndEnableApply)
	var addButton *widget.Button
	addButton = widget.NewButton("Add Image Query", func() {

		urlEntry := widget.NewEntry()
		urlEntry.SetPlaceHolder("Cut and paste your wallhaven image query URL here")

		descEntry := widget.NewEntry()
		descEntry.SetPlaceHolder("Add a description for this query")

		formStatus := widget.NewLabel("")
		activeBool := widget.NewCheck("Active", nil)

		cancelButton := widget.NewButton("Cancel", nil)
		saveButton := widget.NewButton("Save", nil)
		saveButton.Disable() // Save button is only enabled when the URL is valid and min desc has been added

		formValidator := func(who *widget.Entry) bool {
			urlStrErr := urlEntry.Validate()
			descStrErr := descEntry.Validate()

			if urlStrErr == nil && descStrErr == nil {
				formStatus.SetText("Everything looks good")
				formStatus.Importance = widget.SuccessImportance
				formStatus.Refresh()
				return true
			}

			if who == urlEntry {
				if urlStrErr != nil {
					formStatus.SetText(urlStrErr.Error())
					formStatus.Importance = widget.DangerImportance
				} else {
					formStatus.SetText("URL OK")
					formStatus.Importance = widget.SuccessImportance
				}
			}

			if who == descEntry {
				if descStrErr != nil {
					formStatus.SetText(descStrErr.Error())
					formStatus.Importance = widget.DangerImportance
				} else {
					formStatus.SetText("Description OK")
					formStatus.Importance = widget.SuccessImportance
				}
			}

			formStatus.Refresh()
			return false
		}

		urlEntry.Validator = validation.NewRegexp(WallhavenURLRegexp, "Invalid wallhaven image query URL pattern")
		descEntry.Validator = validation.NewRegexp(WallhavenDescRegexp, fmt.Sprintf("Description must be between 5 and %d alpha numeric characters long", MaxDescLength))

		newEntryLengthChecker := func(entry *widget.Entry, maxLen int) func(string) {
			{
				return func(s string) {
					if len(s) > maxLen {
						entry.SetText(s[:maxLen]) // Truncate to max length
						return                    // Very important! Stop further processing
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
		c.Add(ui.CreateSettingTitleLabel("wallhaven Image Query URL:"))
		c.Add(urlEntry)
		c.Add(ui.CreateSettingTitleLabel("Description:"))
		c.Add(descEntry)
		c.Add(formStatus)
		c.Add(activeBool)
		c.Add(widget.NewSeparator())
		c.Add(container.NewHBox(cancelButton, layout.NewSpacer(), saveButton))

		d := dialog.NewCustomWithoutButtons("New Image Query", c, prefsWindow)
		d.Resize(fyne.NewSize(800, 200))

		saveButton.OnTapped = func() {
			newQueryKey := "newQuery"

			apiURL := CovertToAPIURL(urlEntry.Text)
			err := wp.CheckWallhavenURL(apiURL)
			if err != nil {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
				return
			}

			wp.cfg.AddImageQuery(descEntry.Text, apiURL, activeBool.Checked)

			addButton.Enable()
			imgQueryList.Refresh()

			if activeBool.Checked {
				(*refresh)[newQueryKey] = true
				checkAndEnableApply()
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
		parent.Refresh()
	})

	header := container.NewVBox()
	header.Add(ui.CreateSettingTitleLabel("wallhaven Image Queries"))
	header.Add(ui.CreateSettingDescriptionLabel("Manage your wallhaven.cc image queries here. Spice will convert query URL into wallhaven API format."))
	header.Add(addButton)
	qpContainer := container.NewBorder(header, nil, nil, nil, imgQueryList)
	parent.Add(qpContainer)
	parent.Refresh()
}

// CreateWallpaperPreferences creates a preferences widget for wallpaper settings
func (wp *wallpaperPlugin) CreatePrefsPanel(prefsWindow fyne.Window) *fyne.Container {
	header := container.NewVBox()
	footer := container.NewVBox()

	prefsPanel := container.NewBorder(header, footer, nil, nil)
	header.Add(ui.CreateSectionTitleLabel("Wallpaper Preferences"))

	var checkAndEnableApply func()               // Function to check if any setting has changed and enable/disable the apply button
	chgPrefsCallbacks := make(map[string]func()) // Map to store the changed settings and their corresponding apply functions
	refreshNeeded := make(map[string]bool)       // Map to track if a refresh is needed after applying settings

	// Change Frequency (using the enum)
	frequencyOptions := []string{}
	frequencyKey := "changeFrequency"
	for _, f := range GetFrequencies() {
		frequencyOptions = append(frequencyOptions, f.String())
	}
	initialFrequency := wp.cfg.GetWallpaperChangeFrequency()
	frequencySelect := widget.NewSelect(frequencyOptions, func(selected string) {})
	frequencySelect.SetSelectedIndex(int(initialFrequency))
	header.Add(ui.NewSplitRow(ui.CreateSettingTitleLabel("Wallpaper Change Frequency:"), frequencySelect, ui.SplitProportion.OneThird))
	header.Add(ui.CreateSettingDescriptionLabel("Set how often the wallpaper changes. The default is hourly. Set to never to disable wallpaper changes."))
	frequencySelect.OnChanged = func(s string) {
		for _, f := range GetFrequencies() {
			if f.String() == s && f != initialFrequency {
				chgPrefsCallbacks[frequencyKey] = func() {
					selectedFrequency := Frequency(frequencySelect.SelectedIndex())
					if selectedFrequency != initialFrequency {
						wp.cfg.SetWallpaperChangeFrequency(selectedFrequency)
						wp.ChangeWallpaperFrequency(selectedFrequency) // Change the frequency
						initialFrequency = selectedFrequency           // Update the initial frequency
					}
				}
				break
			} else {
				delete(chgPrefsCallbacks, frequencyKey)
			}
		}
		checkAndEnableApply()
	}

	// Cache Size
	cacheSizeOptions := []string{}
	cacheSizeKey := "cacheSize"
	for _, f := range GetCacheSizes() {
		cacheSizeOptions = append(cacheSizeOptions, f.String())
	}
	initialCacheSize := wp.cfg.GetCacheSize()
	cacheSizeSelect := widget.NewSelect(cacheSizeOptions, func(selected string) {})
	cacheSizeSelect.SetSelectedIndex(int(initialCacheSize))
	header.Add(ui.NewSplitRow(ui.CreateSettingTitleLabel("Cache Size:"), cacheSizeSelect, ui.SplitProportion.OneThird))
	header.Add(ui.CreateSettingDescriptionLabel("Set how many images to cache for faster startup and less network usage. The default is 200. Set to none to disable caching."))
	cacheSizeSelect.OnChanged = func(s string) {
		for _, f := range GetCacheSizes() {
			if f.String() == s && f != initialCacheSize {
				chgPrefsCallbacks[cacheSizeKey] = func() {
					selectedCacheSize := CacheSize(cacheSizeSelect.SelectedIndex())
					if selectedCacheSize != initialCacheSize {
						wp.cfg.SetCacheSize(selectedCacheSize) // Save the cache size
						initialCacheSize = selectedCacheSize   // Update the initial cache size
					}
				}
				break
			} else {
				delete(chgPrefsCallbacks, cacheSizeKey) // Remove the selected frequency and duration from the map
			}
		}
		checkAndEnableApply()
	}

	// Smart Fit
	initialSmartFit := wp.cfg.GetSmartFit()
	smartFitKey := "smartFit"
	smartFitCheck := widget.NewCheck("Enable Smart Fit", func(b bool) {})
	smartFitCheck.SetChecked(initialSmartFit)
	header.Add(ui.NewSplitRow(ui.CreateSettingTitleLabel("Scale Wallpaper to Fit Screen:"), smartFitCheck, ui.SplitProportion.OneThird))
	header.Add(ui.CreateSettingDescriptionLabel("Smart Fit analizes wallpapers to find best way to scale and crop them to fit your screen. This is disabled by default."))
	smartFitCheck.OnChanged = func(b bool) {
		if initialSmartFit != b {
			chgPrefsCallbacks[smartFitKey] = func() {
				selectedSmartFit := smartFitCheck.Checked
				if selectedSmartFit != initialSmartFit {
					wp.cfg.SetSmartFit(selectedSmartFit)  // Save the smart fit flag
					wp.SetSmartFit(smartFitCheck.Checked) // Set the smart fit flag
					initialSmartFit = selectedSmartFit    // Update the initial smart fit flag
				}
			}
			refreshNeeded[smartFitKey] = true
		} else {
			delete(chgPrefsCallbacks, smartFitKey)
			delete(refreshNeeded, smartFitKey)
		}
		checkAndEnableApply()
	}

	// Reset Blocked Images
	clearBlckLstBtn := widget.NewButton("Reset", func() {
		d := dialog.NewConfirm("Please Confirm", "This cannot be undone. Are you sure? ", func(b bool) {
			if b {
				wp.cfg.ResetAvoidSet()
			}
		}, prefsWindow)
		d.Show()
	})
	header.Add(
		ui.NewSplitRow(
			ui.CreateSettingTitleLabel("Blocked Images:"),
			clearBlckLstBtn,
			ui.SplitProportion.OneThird))
	header.Add(
		ui.CreateSettingDescriptionLabel("Clear the blocked images list. Blocked images may be downloaded next time wallpapers are refreshed."))

	//wallhaven service section
	header.Add(widget.NewSeparator())
	header.Add(ui.CreateSectionTitleLabel("wallhaven.cc Image Service Preferences"))

	// wallhaven API Key
	wallhavenAPIKeyKey := "wallhavenAPIKey"
	wallhavenKeyEntry := widget.NewEntry()
	wallhavenKeyEntry.SetPlaceHolder("Enter your wallhaven.cc API Key")
	initialKey := wp.cfg.GetWallhavenAPIKey()
	wallhavenKeyEntry.SetText(initialKey)
	apiKeyStatus := widget.NewLabel("")
	wallhavenKeyEntry.Validator = validation.NewRegexp(WallhavenAPIKeyRegexp, "32 alphanumeric characters required")
	header.Add(
		ui.NewSplitRow(
			ui.CreateSettingTitleLabel("wallhaven API Key:"),
			wallhavenKeyEntry,
			ui.SplitProportion.OneThird))
	whURL, _ := url.Parse("https://wallhaven.cc/settings/account")
	header.Add(
		ui.NewSplitRowWithAlignment(
			widget.NewHyperlink("Restricted content requires an API key. Get one here.", whURL),
			apiKeyStatus,
			ui.SplitProportion.TwoThirds,
			ui.SplitAlign.Opposed))
	wallhavenKeyEntry.OnChanged = func(s string) {
		entryErr := wallhavenKeyEntry.Validate()
		if entryErr != nil {
			apiKeyStatus.SetText(entryErr.Error())
			apiKeyStatus.Importance = widget.DangerImportance
			delete(chgPrefsCallbacks, wallhavenAPIKeyKey)
			delete(refreshNeeded, wallhavenAPIKeyKey)
			checkAndEnableApply()
		} else {
			keyErr := CheckWallhavenAPIKey(s)
			if keyErr != nil {
				apiKeyStatus.SetText(keyErr.Error())
				apiKeyStatus.Importance = widget.DangerImportance
				delete(chgPrefsCallbacks, wallhavenAPIKeyKey)
				delete(refreshNeeded, wallhavenAPIKeyKey)
				checkAndEnableApply()
			} else {
				apiKeyStatus.SetText("API Key OK")
				apiKeyStatus.Importance = widget.SuccessImportance
				if initialKey != s {
					chgPrefsCallbacks[wallhavenAPIKeyKey] = func() {
						enteredAPIKey := wallhavenKeyEntry.Text
						if enteredAPIKey != initialKey {
							wp.cfg.SetWallhavenAPIKey(enteredAPIKey) // Save the API key
							initialKey = enteredAPIKey               // Update the initial key
						}
					}
					refreshNeeded[wallhavenAPIKeyKey] = true
					checkAndEnableApply()
				}
			}
		}
		apiKeyStatus.Refresh()
	}

	// Apply Button (Initially Disabled)
	var applyButton *widget.Button
	applyButton = widget.NewButton("Apply Changes", func() {

		originalText := applyButton.Text
		applyButton.Disable()
		applyButton.SetText("Applying changes, please wait...")
		applyButton.Refresh()
		go func() {
			// Change wallpaper frequency
			if len(chgPrefsCallbacks) > 0 {
				for _, callback := range chgPrefsCallbacks {
					callback()
				}
				chgPrefsCallbacks = make(map[string]func()) // Reset the flag
			}
			// Refresh images if API Key has changed or smart fit has been toggled
			if len(refreshNeeded) > 0 {
				wp.RefreshImagesAndPulse()
				refreshNeeded = map[string]bool{}
			}
			applyButton.SetText(originalText)
			applyButton.Refresh()
		}()
	})
	applyButton.Disable() // Disable the apply button again after the changes have been applied

	// Function to check if any setting has changed and enable/disable the apply button
	checkAndEnableApply = func() {
		if len(refreshNeeded) > 0 || len(chgPrefsCallbacks) > 0 {
			applyButton.Enable()
		} else {
			applyButton.Disable()
		}
		applyButton.Refresh()
	}

	wp.createImageQueryPanel(prefsWindow, prefsPanel, &chgPrefsCallbacks, &refreshNeeded, checkAndEnableApply)

	footer.Add(widget.NewSeparator())
	footer.Add(applyButton)

	//return wallpaperPrefs
	return prefsPanel
}
