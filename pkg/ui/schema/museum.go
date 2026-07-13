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

	// Optional: API Key/Token support (leave empty for zero-auth providers)
	APIKeyLabel     string        // e.g. "API Key:" or "API Token:" — empty means no key needed
	APIKeyHelp      string        // Help text for the key input
	RegistrationURL string        // URL where users can register for a key
	APIKeyGetFunc   func() string // Read current key value
	APIKeySetFunc   func(string)  // Save new key value

	MuseumFramingGetFunc func() bool // Read current museum framing value
	MuseumFramingSetFunc func(bool)  // Save new museum framing value
}

// CreateMuseumSettingsPanel generates a standard PanelSchema for museum providers.
func CreateMuseumSettingsPanel(cfg MuseumSettingsConfig, openURL func(string)) *PanelSchema {
	prefix := strings.ToLower(cfg.ID)

	// Build the info items (always present)
	infoItems := []ItemSchema{}

	infoItems = append(infoItems,
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
	)

	if cfg.MuseumFramingGetFunc != nil && cfg.MuseumFramingSetFunc != nil {
		infoItems = append(infoItems, BoolItem{
			Name:         prefix + "_museum_framing",
			Label:        i18n.T("Display as Framed Gallery"),
			Help:         i18n.T("Present all artwork from this collection inside a virtual museum frame with a dynamic background, regardless of its original dimensions."),
			InitialValue: cfg.MuseumFramingGetFunc(),
			ApplyFunc:    cfg.MuseumFramingSetFunc,
			NeedsRefresh: false,
		})
	}

	sections := []SectionSchema{
		{
			ID:      cfg.ID,
			Title:   cfg.Title,
			Compact: true,
			Items:   infoItems,
		},
	}

	// Optional: API Key section (only if APIKeyLabel is set)
	if cfg.APIKeyLabel != "" {
		apiItems := []ItemSchema{
			TextItem{
				Name:         prefix + "_apikey",
				Label:        cfg.APIKeyLabel,
				Help:         cfg.APIKeyHelp,
				InitialValue: cfg.APIKeyGetFunc(),
				IsPassword:   true,
				ApplyFunc: func(val string) {
					cfg.APIKeySetFunc(val)
				},
			},
		}
		if cfg.RegistrationURL != "" {
			apiItems = append(apiItems, HyperlinkItem{
				Text: i18n.T("Get a free API key"),
				URL:  cfg.RegistrationURL,
			})
		}
		sections = append(sections, SectionSchema{
			Title:   i18n.T("Authentication"),
			Compact: true,
			Items:   apiItems,
		})
	}

	return &PanelSchema{
		Sections: sections,
	}
}
