package wallpaper

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
)

// AddQueryConfig defines the configuration for the standardized Add Query modal.
type AddQueryConfig struct {
	Title           string
	Description     string // Optional description for the modal
	URLPlaceholder  string
	URLValidator    string
	URLErrorMsg     string
	DescPlaceholder string
	DescValidator   string
	DescErrorMsg    string

	// ValidateFunc is an optional custom validation logic (e.g., API URL conversion check).
	// It returns an error if validation fails.
	ValidateFunc func(url, desc string) error

	// AddHandler performs the actual addition of the query.
	// It returns the new query ID and an error if it fails.
	AddHandler func(desc, url string, active bool) (string, error)
}

// CreateAddQueryButton creates a standardized button that opens a modal for adding image queries.
// It handles input validation, UI feedback, and correctly triggering the SettingsManager apply lifecycle.
func CreateAddQueryButton(label string, sm setting.SettingsManager, cfg AddQueryConfig, onAdded func()) *widget.Button {
	return widget.NewButton(label, func() {
		OpenAddQueryDialog(sm, cfg, "", "", onAdded)
	})
}

// OpenAddQueryDialog opens the modal for adding image queries.
// It allows pre-filling the URL and Description.
func OpenAddQueryDialog(sm setting.SettingsManager, cfg AddQueryConfig, initialURL, initialDesc string, onAdded func()) {
	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder(cfg.URLPlaceholder)
	urlEntry.SetText(initialURL)

	descEntry := widget.NewEntry()
	descEntry.SetPlaceHolder(cfg.DescPlaceholder)
	descEntry.SetText(initialDesc)

	formStatus := widget.NewLabel("")
	activeBool := widget.NewCheck("Active", nil)
	activeBool.SetChecked(true) // Default to active

	cancelButton := widget.NewButton("Cancel", nil)
	saveButton := widget.NewButton("Save", nil)
	if initialURL == "" {
		saveButton.Disable()
	} else {
		// If pre-filled, we might enable it, but validation triggers on change.
		// Let's force validation check if pre-filled.
		// For now simple disable until edited/validated logic runs?
		// Actually, let's enable it if it looks valid?
		// Simplest: Enable if prefilled. User can click and see error if invalid.
		saveButton.Enable()
	}

	// Internal validator function used by entry callbacks
	formValidator := func(who *widget.Entry) bool {
		// 1. Basic empty checks
		if urlEntry.Text == "" || descEntry.Text == "" {
			return false
		}

		// 2. Regex Validation Logic (Key-by-Key)
		if err := urlEntry.Validate(); err != nil {
			if who == urlEntry {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
			}
			return false
		}
		if err := descEntry.Validate(); err != nil {
			if who == descEntry {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
			}
			return false
		}

		// 3. Custom Validation Logic (e.g. Duplicates)
		if cfg.ValidateFunc != nil {
			if err := cfg.ValidateFunc(urlEntry.Text, descEntry.Text); err != nil {
				if who == urlEntry || (who == descEntry && urlEntry.Text != "") {
					// Only show error if we are editing the relevant field or if URL is set
					formStatus.SetText(err.Error())
					formStatus.Importance = widget.DangerImportance
					formStatus.Refresh()
				}
				return false
			}
		}

		formStatus.SetText("Everything looks good")
		formStatus.Importance = widget.SuccessImportance
		formStatus.Refresh()
		return true
	}

	// Input Validators (Regex & Length)
	if cfg.URLValidator != "" {
		urlEntry.Validator = validation.NewRegexp(cfg.URLValidator, cfg.URLErrorMsg)
	}
	if cfg.DescValidator != "" {
		descEntry.Validator = validation.NewRegexp(cfg.DescValidator, cfg.DescErrorMsg)
	}

	// Length checker factory
	newEntryLengthChecker := func(entry *widget.Entry, maxLen int) func(string) {
		return func(s string) {
			if len(s) > maxLen {
				entry.SetText(s[:maxLen])
				return
			}
			if formValidator(entry) {
				saveButton.Enable()
			} else {
				saveButton.Disable()
			}
		}
	}

	urlEntry.OnChanged = newEntryLengthChecker(urlEntry, MaxURLLength)
	descEntry.OnChanged = newEntryLengthChecker(descEntry, MaxDescLength)

	// Layout
	c := container.NewVBox()
	if cfg.Description != "" {
		c.Add(widget.NewLabel(cfg.Description))
	}
	c.Add(sm.CreateSettingTitleLabel("URL / Search Term:"))
	c.Add(urlEntry)
	c.Add(sm.CreateSettingTitleLabel("Description:"))
	c.Add(descEntry)
	c.Add(formStatus)
	c.Add(activeBool)
	c.Add(widget.NewSeparator())
	c.Add(container.NewHBox(cancelButton, layout.NewSpacer(), saveButton))

	d := dialog.NewCustomWithoutButtons(cfg.Title, c, sm.GetSettingsWindow())
	d.Resize(fyne.NewSize(600, 200)) // Standard size

	saveButton.OnTapped = func() {
		// Final safe-guard validation
		if cfg.ValidateFunc != nil {
			if err := cfg.ValidateFunc(urlEntry.Text, descEntry.Text); err != nil {
				formStatus.SetText(err.Error())
				formStatus.Importance = widget.DangerImportance
				formStatus.Refresh()
				return
			}
		}

		// Perform Add
		newID, err := cfg.AddHandler(descEntry.Text, urlEntry.Text, activeBool.Checked)
		if err != nil {
			formStatus.SetText(err.Error())
			formStatus.Importance = widget.DangerImportance
			formStatus.Refresh()
			return
		}

		// Success! Trigger updates
		if onAdded != nil {
			onAdded()
		}

		// Critical: Trigger Apply Button
		if activeBool.Checked {
			sm.SetRefreshFlag(newID)
			sm.GetCheckAndEnableApplyFunc()()
		}

		d.Hide()
	}

	cancelButton.OnTapped = func() {
		d.Hide()
	}

	d.Show()
}
