package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// CreateSectionTitleLabel creates a label for a setting title
func (sm *SettingsManager) CreateSectionTitleLabel(desc string) *widget.Label {
	label := widget.NewLabel(desc)
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.HighImportance
	label.TextStyle = fyne.TextStyle{Bold: true}
	return label
}

// CreateSettingTitleLabel creates a label for a setting title
func (sm *SettingsManager) CreateSettingTitleLabel(desc string) *widget.Label {
	label := widget.NewLabel(desc)
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.MediumImportance
	label.TextStyle = fyne.TextStyle{Bold: true}
	return label
}

// CreateSettingDescriptionLabel creates a label for a setting description
func (sm *SettingsManager) CreateSettingDescriptionLabel(desc string) *widget.Label {
	label := widget.NewLabel(desc)
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.LowImportance
	label.TextStyle = fyne.TextStyle{Italic: true}
	return label
}
