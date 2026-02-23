package wallpaper

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/v2/pkg/ui/setting"
	utilLog "github.com/dixieflatline76/Spice/v2/util/log"
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
	utilLog.Debugf("OpenAddQueryDialog: Triggered with URL: %s", initialURL)

	urlEntry, descEntry := createQueryEntries(cfg, initialURL, initialDesc)
	formStatus := widget.NewLabel("")
	activeBool := widget.NewCheck("Active", nil)
	activeBool.SetChecked(true)

	cancelButton := widget.NewButton("Cancel", nil)
	saveButton := widget.NewButton("Save", nil)

	if initialURL == "" {
		saveButton.Disable()
	} else {
		saveButton.Enable()
	}

	formValidator := createValidationLogic(cfg, urlEntry, descEntry, formStatus)
	configureEntryListeners(urlEntry, descEntry, saveButton, formValidator)

	inputContainer := createDialogLayout(sm, cfg, urlEntry, descEntry, formStatus, activeBool, cancelButton, saveButton)

	win := sm.GetSettingsWindow()
	utilLog.Debugf("OpenAddQueryDialog: Creating dialog with parent window: %v", win)
	d := dialog.NewCustomWithoutButtons(cfg.Title, inputContainer, win)
	d.Resize(fyne.NewSize(600, 200))

	saveButton.OnTapped = createSaveHandler(sm, cfg, d, urlEntry, descEntry, activeBool, formStatus, onAdded)

	cancelButton.OnTapped = func() {
		d.Hide()
	}

	utilLog.Debug("OpenAddQueryDialog: Calling d.Show()")
	d.Show()
}

func createQueryEntries(cfg AddQueryConfig, initialURL, initialDesc string) (*widget.Entry, *widget.Entry) {
	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder(cfg.URLPlaceholder)
	urlEntry.SetText(initialURL)
	if cfg.URLValidator != "" {
		urlEntry.Validator = validation.NewRegexp(cfg.URLValidator, cfg.URLErrorMsg)
	}

	descEntry := widget.NewEntry()
	descEntry.SetPlaceHolder(cfg.DescPlaceholder)
	descEntry.SetText(initialDesc)
	if cfg.DescValidator != "" {
		descEntry.Validator = validation.NewRegexp(cfg.DescValidator, cfg.DescErrorMsg)
	}

	return urlEntry, descEntry
}

func createValidationLogic(cfg AddQueryConfig, urlEntry, descEntry *widget.Entry, formStatus *widget.Label) func(*widget.Entry) bool {
	return func(who *widget.Entry) bool {
		if urlEntry.Text == "" || descEntry.Text == "" {
			return false
		}

		if err := urlEntry.Validate(); err != nil {
			if who == urlEntry {
				setStatusError(formStatus, err.Error())
			}
			return false
		}
		if err := descEntry.Validate(); err != nil {
			if who == descEntry {
				setStatusError(formStatus, err.Error())
			}
			return false
		}

		if cfg.ValidateFunc != nil {
			if err := cfg.ValidateFunc(urlEntry.Text, descEntry.Text); err != nil {
				if who == urlEntry || (who == descEntry && urlEntry.Text != "") {
					setStatusError(formStatus, err.Error())
				}
				return false
			}
		}

		formStatus.SetText("Everything looks good")
		formStatus.Importance = widget.SuccessImportance
		formStatus.Refresh()
		return true
	}
}

func setStatusError(l *widget.Label, msg string) {
	l.SetText(msg)
	l.Importance = widget.DangerImportance
	l.Refresh()
}

func configureEntryListeners(urlEntry, descEntry *widget.Entry, saveBtn *widget.Button, validator func(*widget.Entry) bool) {
	newChecker := func(entry *widget.Entry, maxLen int) func(string) {
		return func(s string) {
			if len(s) > maxLen {
				entry.SetText(s[:maxLen])
				return
			}
			if validator(entry) {
				saveBtn.Enable()
			} else {
				saveBtn.Disable()
			}
		}
	}
	urlEntry.OnChanged = newChecker(urlEntry, MaxURLLength)
	descEntry.OnChanged = newChecker(descEntry, MaxDescLength)
}

func createDialogLayout(sm setting.SettingsManager, cfg AddQueryConfig, urlEntry, descEntry *widget.Entry, status *widget.Label, active *widget.Check, cancel, save *widget.Button) *fyne.Container {
	c := container.NewVBox()
	if cfg.Description != "" {
		c.Add(widget.NewLabel(cfg.Description))
	}
	c.Add(sm.CreateSettingTitleLabel("URL / Search Term:"))
	c.Add(urlEntry)
	c.Add(sm.CreateSettingTitleLabel("Description:"))
	c.Add(descEntry)
	c.Add(status)
	c.Add(active)
	c.Add(widget.NewSeparator())
	c.Add(container.NewHBox(cancel, layout.NewSpacer(), save))
	return c
}

func createSaveHandler(sm setting.SettingsManager, cfg AddQueryConfig, d dialog.Dialog, urlEntry, descEntry *widget.Entry, active *widget.Check, status *widget.Label, onAdded func()) func() {
	return func() {
		if cfg.ValidateFunc != nil {
			if err := cfg.ValidateFunc(urlEntry.Text, descEntry.Text); err != nil {
				setStatusError(status, err.Error())
				return
			}
		}

		newID, err := cfg.AddHandler(descEntry.Text, urlEntry.Text, active.Checked)
		if err != nil {
			setStatusError(status, err.Error())
			return
		}

		if onAdded != nil {
			onAdded()
		}

		if active.Checked {
			sm.SetRefreshFlag(newID)
			sm.GetCheckAndEnableApplyFunc()()
		}

		d.Hide()
	}
}
