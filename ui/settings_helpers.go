package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
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
func (sm *SettingsManager) CreateSettingDescriptionLabel(desc string) fyne.CanvasObject {
	rich := widget.NewRichTextWithText(desc)
	rich.Wrapping = fyne.TextWrapWord
	// Set caption size for all text segments
	for i := range rich.Segments {
		if seg, ok := rich.Segments[i].(*widget.TextSegment); ok {
			seg.Style.SizeName = theme.SizeNameText          // Standard size
			seg.Style.ColorName = theme.ColorNamePlaceHolder // Muted color (Gray)
		}
	}

	// Indent the description
	indent := canvas.NewRectangle(color.Transparent)
	indent.SetMinSize(fyne.NewSize(20, 0)) // 20px indentation

	return container.NewBorder(nil, nil, indent, nil, rich)
}
