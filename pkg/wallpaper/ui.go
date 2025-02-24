package wallpaper

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func (wp *wallpaperPlugin) CreateTrayMenuItems() []*fyne.MenuItem {
	items := []*fyne.MenuItem{}
	items = append(items, wp.manager.CreateMenuItem("Next Wallpaper", func() {
		go wp.SetNextWallpaper()
	}, "next.png"))
	items = append(items, wp.manager.CreateMenuItem("Prev Wallpaper", func() {
		go wp.SetPreviousWallpaper()
	}, "prev.png"))
	items = append(items, wp.manager.CreateToggleMenuItem("Shuffle Wallpapers", wp.SetShuffleImage, "shuffle.png", wp.cfg.BoolWithFallback(ImgShufflePrefKey, false)))
	items = append(items, fyne.NewMenuItemSeparator())
	items = append(items, wp.manager.CreateMenuItem("Image Source", func() {
		go wp.ViewCurrentImageOnWeb()
	}, "view.png"))
	return items
}

// createSectionTitleLabel creates a label for a setting title
func createSectionTitleLabel(desc string) *widget.Label {
	label := widget.NewLabel(desc)
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.HighImportance
	label.TextStyle = fyne.TextStyle{Bold: true}
	return label
}

// createSettingTitleLabel creates a label for a setting title
func createSettingTitleLabel(desc string) *widget.Label {
	label := widget.NewLabel(desc)
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.MediumImportance
	label.TextStyle = fyne.TextStyle{Bold: true}
	return label
}

// createSettingDescriptionLabel creates a label for a setting description
func createSettingDescriptionLabel(desc string) *widget.Label {
	label := widget.NewLabel(desc)
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.LowImportance
	label.TextStyle = fyne.TextStyle{Italic: true}
	return label
}

// createSettingEntry creates a widget for a setting entry
func (wp *wallpaperPlugin) createImgQueryList(prefsWindow fyne.Window, refresh *bool, checkAndEnableApply func()) *widget.List {

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

			activeCheck.SetChecked(wp.cfg.ImageQueries[i].Active)
			activeCheck.OnChanged = func(b bool) {
				if b == wp.cfg.ImageQueries[i].Active {
					return // no change, just return
				}

				if b {
					wp.cfg.EnableImageQuery(i)
				} else {
					wp.cfg.DisableImageQuery(i)
				}

				*refresh = true
				checkAndEnableApply()
			}

			deleteButton.OnTapped = func() {
				d := dialog.NewConfirm("Please Confirm", fmt.Sprintf("Are you sure you want to delete %s?", wp.cfg.ImageQueries[i].Description), func(b bool) {
					if b {
						if wp.cfg.ImageQueries[i].Active {
							*refresh = true
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
func (wp *wallpaperPlugin) createImageQueryPanel(prefsWindow fyne.Window, parent *fyne.Container, refresh *bool, checkAndEnableApply func()) {

	imgQueryList := wp.createImgQueryList(prefsWindow, refresh, checkAndEnableApply)
	var addButton *widget.Button
	addButton = widget.NewButton("Add Image Query", func() {

		urlEntry := widget.NewEntry()
		urlEntry.SetPlaceHolder("Cut and paste your wallhaven image query URL here")

		descEntry := widget.NewEntry()
		descEntry.SetPlaceHolder("Add a description for this query")

		formStatus := widget.NewLabel("")
		activeBool := widget.NewCheck("Active", nil)

		cancelButton := widget.NewButton("Cancel", nil)
		actionButton := widget.NewButton("Save", nil)
		actionButton.Disable() // Save button is only enabled when the URL is valid and min desc has been added

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
						actionButton.Enable()
					} else {
						actionButton.Disable()
					}
				}
			}
		}
		urlEntry.OnChanged = newEntryLengthChecker(urlEntry, MaxURLLength)
		descEntry.OnChanged = newEntryLengthChecker(descEntry, MaxDescLength)

		c := container.NewVBox()
		c.Add(createSettingTitleLabel("wallhaven Image Query URL:"))
		c.Add(urlEntry)
		c.Add(createSettingTitleLabel("Description:"))
		c.Add(descEntry)
		c.Add(formStatus)
		c.Add(activeBool)
		c.Add(widget.NewSeparator())
		c.Add(container.NewHBox(cancelButton, layout.NewSpacer(), actionButton))

		d := dialog.NewCustomWithoutButtons("New Image Query", c, prefsWindow)
		d.Resize(fyne.NewSize(800, 200))

		actionButton.OnTapped = func() {

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
				*refresh = true
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
	header.Add(createSettingTitleLabel("wallhaven Image Queries"))
	header.Add(createSettingDescriptionLabel("Manage your wallhaven.cc image queries here. Spice will convert query URL into wallhaven API format."))
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

	header.Add(createSectionTitleLabel("Wallpaper Preferences"))
	header.Add(createSettingDescriptionLabel("Following settings control the general behavior of wallpaper across all image services."))

	var checkAndEnableApply func()  // Function to check if any setting has changed and enable/disable the apply button
	refresh, chgFrq := false, false // Flags to track if the wallpaper settings have changed

	// Change Frequency (using the enum)
	frequencyOptions := []string{}
	for _, f := range GetFrequencies() {
		frequencyOptions = append(frequencyOptions, f.String())
	}

	initialFrequencyInt := wp.cfg.IntWithFallback(WallpaperChgFreqPrefKey, int(FrequencyHourly)) // Default to hourly
	intialFrequency := Frequency(initialFrequencyInt)

	frequencySelect := widget.NewSelect(frequencyOptions, func(selected string) {})
	frequencySelect.SetSelectedIndex(initialFrequencyInt)

	header.Add(createSettingTitleLabel("Change Frequency:"))
	header.Add(createSettingDescriptionLabel("Select how often you want your wallpaper to change."))
	header.Add(frequencySelect)

	// 3. Smart Fit
	initialSmartFit := wp.cfg.BoolWithFallback(SmartFitPrefKey, false)

	smartFitCheck := widget.NewCheck("Enable Smart Fit", func(b bool) {})
	smartFitCheck.SetChecked(initialSmartFit)

	header.Add(createSettingTitleLabel("Smart Fit:"))
	header.Add(createSettingDescriptionLabel("Enable Smart Fit to automatically scale and crop the wallpaper to fit your screen resolution."))
	header.Add(smartFitCheck)

	//wallhaven service section
	header.Add(widget.NewSeparator())
	header.Add(createSectionTitleLabel("wallhaven Service Preferences"))
	header.Add(createSettingDescriptionLabel("Following settings are only used for wallhaven.cc image service."))

	// wallhaven API Key
	wallhavenKeyEntry := widget.NewEntry()
	wallhavenKeyEntry.SetPlaceHolder("Enter your wallhaven.cc API Key")
	initialKey := wp.cfg.StringWithFallback(WallhavenAPIKeyPrefKey, "")
	wallhavenKeyEntry.SetText(initialKey)
	statusLabel := widget.NewLabel("")

	wallhavenKeyEntry.Validator = validation.NewRegexp(WallhavenAPIKeyRegexp, "wallhaven API keys are 32 alpha numerics characters")
	wallhavenKeyEntry.OnChanged = func(s string) {
		entryErr := wallhavenKeyEntry.Validate()
		if entryErr != nil {
			statusLabel.SetText(entryErr.Error())
			statusLabel.Importance = widget.DangerImportance
		} else {
			keyErr := CheckWallhavenAPIKey(s)
			if keyErr != nil {
				statusLabel.SetText(keyErr.Error())
				statusLabel.Importance = widget.DangerImportance
			} else {
				statusLabel.SetText("API Key OK")
				statusLabel.Importance = widget.SuccessImportance
				if initialKey != s {
					refresh = true
					checkAndEnableApply()
				}
			}
		}
		statusLabel.Refresh()
	}

	header.Add(createSettingTitleLabel("wallhaven API Key:"))
	header.Add(createSettingDescriptionLabel("Enter your API Key from wallhaven.cc to enable wallpaper downloads from this source."))
	header.Add(wallhavenKeyEntry)
	header.Add(statusLabel)

	// Apply Button (Initially Disabled)
	var applyButton *widget.Button
	applyButton = widget.NewButton("Apply Changes", func() {

		originalText := applyButton.Text
		applyButton.Disable()
		applyButton.SetText("Applying changes, please wait...")
		applyButton.Refresh()
		go func() {
			// Change wallpaper frequency
			if chgFrq {
				selectedFrequency := Frequency(frequencySelect.SelectedIndex())
				wp.ChangeWallpaperFrequency(selectedFrequency)
				chgFrq = false
			}

			// Refresh images if API Key has changed or smart fit has been toggled
			if refresh {
				wp.SetWallhavenAPIKey(wallhavenKeyEntry.Text) // Set the API key
				initialKey = wallhavenKeyEntry.Text           // Update the initial key

				wp.SetSmartFit(smartFitCheck.Checked)   // Set the smart fit flag
				initialSmartFit = smartFitCheck.Checked // Update the initial smart fit flag

				wp.RefreshImages()
				refresh = false
			}

			applyButton.SetText(originalText)
			applyButton.Refresh()
		}()
	})
	applyButton.Disable() // Start as disabled

	// Function to check if any setting has changed and enable/disable the apply button
	checkAndEnableApply = func() {
		if refresh || chgFrq {
			applyButton.Enable()
		} else {
			applyButton.Disable()
		}
		applyButton.Refresh()
	}

	frequencySelect.OnChanged = func(s string) {
		for _, f := range GetFrequencies() {
			if f.String() == s && f != intialFrequency {
				wp.cfg.SetInt(WallpaperChgFreqPrefKey, int(f))
				intialFrequency = f
				// Log the selected frequency and duration
				chgFrq = true
				break
			}
		}
		checkAndEnableApply()
	}

	smartFitCheck.OnChanged = func(b bool) {
		if initialSmartFit != b {
			refresh = true
		} else {
			refresh = false
		}
		checkAndEnableApply()
	}

	wp.createImageQueryPanel(prefsWindow, prefsPanel, &refresh, checkAndEnableApply)

	footer.Add(widget.NewSeparator())
	footer.Add(applyButton)

	//return wallpaperPrefs
	return prefsPanel
}
