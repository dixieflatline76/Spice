package wallpaper

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dixieflatline76/Spice/pkg/ui/setting"
	"github.com/dixieflatline76/Spice/util/log"
)

// CreateMuseumHeader creates a standardized header for museum providers.
func CreateMuseumHeader(name, location, licenseText, licenseURL, description, mapURL, webURL, donateURL string, sm setting.SettingsManager) fyne.CanvasObject {
	// Title
	title := widget.NewLabelWithStyle(name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// Subtitle Row: Location • License (Clickable)
	locLabel := widget.NewLabelWithStyle(location, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	usageRow := container.NewHBox(locLabel)

	if licenseText != "" && licenseURL != "" {
		usageRow.Add(widget.NewLabel(" • "))

		u, _ := url.Parse(licenseURL)
		link := widget.NewHyperlink(licenseText, u)
		link.Alignment = fyne.TextAlignLeading
		usageRow.Add(link)
	} else if licenseText != "" {
		// Text only if no URL
		usageRow.Add(widget.NewLabel(" • " + licenseText))
	}

	// Create a wrapper to ensure left alignment in the Border layout context
	// actually HBox packs left by default.

	// Descriptions
	descLabel := widget.NewLabel(description)
	descLabel.Wrapping = fyne.TextWrapWord

	// Action Buttons
	openURL := func(u string) {
		urlObj, err := url.Parse(u)
		if err != nil {
			log.Printf("Invalid URL %s: %v", u, err)
			return
		}
		if err := fyne.CurrentApp().OpenURL(urlObj); err != nil {
			log.Printf("Failed to open URL: %v", err)
		}
	}

	var mapBtn *widget.Button
	if mapURL != "" {
		mapBtn = widget.NewButtonWithIcon("📍 Plan a Visit", theme.InfoIcon(), func() {
			openURL(mapURL)
		})
	} else {
		// Fallback for digital-only archives
		mapBtn = widget.NewButtonWithIcon("License: CC0", theme.InfoIcon(), func() {
			openURL("https://www.metmuseum.org/about-the-met/policies-and-documents/open-access")
		})
	}

	webBtn := widget.NewButtonWithIcon("Visit Website", theme.HomeIcon(), func() {
		openURL(webURL)
	})
	donateBtn := widget.NewButtonWithIcon("Donate", theme.ContentAddIcon(), func() {
		openURL(donateURL)
	})
	// Highlight Donate button
	donateBtn.Importance = widget.HighImportance

	actions := container.NewHBox(
		layout.NewSpacer(),
		mapBtn,
		webBtn,
		donateBtn,
		layout.NewSpacer(),
	)

	return container.NewVBox(
		container.NewBorder(nil, nil, nil, nil, title),
		container.NewBorder(nil, nil, nil, nil, usageRow),
		descLabel,
		actions,
	)
}
