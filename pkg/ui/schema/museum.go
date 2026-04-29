package schema

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/dixieflatline76/Spice/v2/pkg/i18n"
)

// MuseumSettingsConfig holds the configuration for a generic Museum settings panel.
type MuseumSettingsConfig struct {
	ID          string // Unique identifier (e.g., "AIC", "Met") — lowercased for control name prefixes
	Title       string
	Location    string
	LicenseURL  string
	Description string
	MapQuery    string
	WebsiteURL  string
	DonateURL   string
}

// CreateMuseumSettingsPanel generates a standard PanelSchema for museum providers.
func CreateMuseumSettingsPanel(cfg MuseumSettingsConfig, openURL func(string)) *PanelSchema {
	prefix := strings.ToLower(cfg.ID)
	return &PanelSchema{
		Sections: []SectionSchema{
			{
				ID:      cfg.ID,
				Title:   cfg.Title,
				Compact: true,
				Items: []ItemSchema{
					LabelItem{
						Text:       cfg.Description,
						Importance: ImportanceLow,
					},
					HorizontalRowItem{
						Items: []ItemSchema{
							LabelItem{Text: cfg.Location, Importance: ImportanceLow},
							HyperlinkItem{
								Text: i18n.T("Open Access (CC0)"),
								URL:  cfg.LicenseURL,
							},
						},
					},
					HorizontalRowItem{
						Items: []ItemSchema{
							ButtonItem{
								Name:       prefix + "_visit",
								ButtonText: i18n.T("Plan a Visit"),
								IconName:   "MapPin",
								OnPressed: func() {
									u := fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%s", url.QueryEscape(cfg.MapQuery))
									openURL(u)
								},
							},
							ButtonItem{
								Name:       prefix + "_web",
								ButtonText: i18n.T("Visit Website"),
								IconName:   "Home",
								OnPressed:  func() { openURL(cfg.WebsiteURL) },
							},
							ButtonItem{
								Name:       prefix + "_donate",
								ButtonText: i18n.T("Donate"),
								IconName:   "Heart",
								OnPressed:  func() { openURL(cfg.DonateURL) },
							},
						},
					},
				},
			},
		},
	}
}
