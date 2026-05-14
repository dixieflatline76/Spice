package rijksmuseum

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Linked Art API response types for the Rijksmuseum data.rijksmuseum.nl endpoint.
// These types parse the JSON-LD responses needed for the 3-step resolution chain:
//   Object (HumanMadeObject) → VisualItem → DigitalObject → IIIF URL

// SearchResponse represents the top-level search result from the collection API.
type SearchResponse struct {
	PartOf       SearchPartOf `json:"partOf"`
	Next         *SearchRef   `json:"next,omitempty"`
	OrderedItems []SearchItem `json:"orderedItems"`
}

// SearchPartOf contains pagination metadata.
type SearchPartOf struct {
	TotalItems int `json:"totalItems"`
}

// SearchRef is a typed reference with an ID.
type SearchRef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// SearchItem is a single item in the search results.
type SearchItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// ObjectResponse is the Linked Art representation of a HumanMadeObject.
// We parse only the fields needed for wallpaper extraction.
type ObjectResponse struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	IdentifiedBy json.RawMessage `json:"identified_by"`
	ProducedBy   *Production     `json:"produced_by,omitempty"`
	Shows        []TypedRef      `json:"shows,omitempty"`
	ReferredToBy json.RawMessage `json:"referred_to_by,omitempty"`
}

// Production holds the production event (for artist and date extraction).
type Production struct {
	ReferredToBy []LinguisticObject `json:"referred_to_by,omitempty"`
	Timespan     *Timespan          `json:"timespan,omitempty"`
}

// Timespan holds date information.
type Timespan struct {
	BeginOfTheBegin string `json:"begin_of_the_begin"`
	EndOfTheEnd     string `json:"end_of_the_end"`
}

// TypedRef is a generic {id, type} reference.
type TypedRef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// VisualItemResponse is the Linked Art representation of a VisualItem.
type VisualItemResponse struct {
	DigitallyShownBy []TypedRef `json:"digitally_shown_by,omitempty"`
}

// DigitalObjectResponse is the final step containing the IIIF access point.
type DigitalObjectResponse struct {
	AccessPoint []TypedRef `json:"access_point,omitempty"`
}

// LinguisticObject represents a text annotation in Linked Art.
type LinguisticObject struct {
	Type         string             `json:"type"`
	Content      string             `json:"content"`
	ClassifiedAs []ClassifiedAsItem `json:"classified_as,omitempty"`
	Language     []TypedRef         `json:"language,omitempty"`
}

// ClassifiedAsItem is a type classification reference.
type ClassifiedAsItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// IdentifiedByItem represents a name or identifier in Linked Art.
type IdentifiedByItem struct {
	Type         string             `json:"type"`
	Content      string             `json:"content"`
	ClassifiedAs []ClassifiedAsItem `json:"classified_as,omitempty"`
	Language     []TypedRef         `json:"language,omitempty"`
}

// Getty AAT vocabulary URIs used for classification.
const (
	// Language codes
	aatEnglish = "http://vocab.getty.edu/aat/300388277"

	// Identifier types
	aatAccessionNumber = "http://vocab.getty.edu/aat/300312355"

	// Name/title types
	aatPrimaryName = "http://vocab.getty.edu/aat/300404670"

	// Dimension types
	aatDimensionStatement = "http://vocab.getty.edu/aat/300435430"

	// Artist attribution
	aatArtistStatement = "http://vocab.getty.edu/aat/300435416"
)

// ExtractTitle extracts the English primary title from the identified_by field.
// Falls back to any English name, then any name at all.
func ExtractTitle(raw json.RawMessage) string {
	var items []IdentifiedByItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return ""
	}

	var primaryEN, anyEN, anyName string

	for _, item := range items {
		if item.Type != "Name" || item.Content == "" {
			continue
		}

		isEnglish := hasLanguage(item.Language, aatEnglish)
		isPrimary := hasClassification(item.ClassifiedAs, aatPrimaryName)

		if isPrimary && isEnglish {
			primaryEN = item.Content
		} else if isEnglish && anyEN == "" {
			anyEN = item.Content
		} else if anyName == "" {
			anyName = item.Content
		}
	}

	if primaryEN != "" {
		return primaryEN
	}
	if anyEN != "" {
		return anyEN
	}
	return anyName
}

// ExtractObjectNumber extracts the accession number (e.g., "SK-C-5") from identified_by.
func ExtractObjectNumber(raw json.RawMessage) string {
	var items []IdentifiedByItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return ""
	}

	for _, item := range items {
		if item.Type == "Identifier" && hasClassification(item.ClassifiedAs, aatAccessionNumber) {
			return item.Content
		}
	}
	return ""
}

// ExtractArtist extracts the artist name from the production event.
func ExtractArtist(p *Production) string {
	if p == nil {
		return ""
	}
	for _, ref := range p.ReferredToBy {
		if hasClassification(ref.ClassifiedAs, aatArtistStatement) {
			// Prefer English content
			if hasLanguage(ref.Language, aatEnglish) {
				return ref.Content
			}
		}
	}
	// Fallback: any referred_to_by content
	for _, ref := range p.ReferredToBy {
		if ref.Content != "" {
			return ref.Content
		}
	}
	return ""
}

// ExtractDimensions extracts height and width from the referred_to_by dimension statements.
// Returns (width, height) parsed from English text like "height 379.5 cm × width 453.5 cm".
func ExtractDimensions(raw json.RawMessage) (float64, float64) {
	var items []LinguisticObject
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, 0
	}

	for _, item := range items {
		if !hasClassification(item.ClassifiedAs, aatDimensionStatement) {
			continue
		}
		if !hasLanguage(item.Language, aatEnglish) {
			continue
		}

		return parseDimensionText(item.Content)
	}
	return 0, 0
}

// parseDimensionText parses strings like "height 379.5 cm × width 453.5 cm"
func parseDimensionText(text string) (float64, float64) {
	var height, width float64

	text = strings.ToLower(text)
	// Try "height X cm × width Y cm" format
	if _, err := fmt.Sscanf(text, "height %f cm × width %f cm", &height, &width); err == nil {
		return width, height
	}
	// Try "height X cm x width Y cm" format (lowercase x)
	if _, err := fmt.Sscanf(text, "height %f cm x width %f cm", &height, &width); err == nil {
		return width, height
	}
	return 0, 0
}

// BuildObjectURL constructs the public website URL for an object.
func BuildObjectURL(objectNumber string) string {
	if objectNumber == "" {
		return WebBaseURL
	}
	return fmt.Sprintf("%s/en/collection/%s", WebBaseURL, objectNumber)
}

// helper: check if a language list contains a specific AAT language URI
func hasLanguage(langs []TypedRef, aat string) bool {
	for _, l := range langs {
		if l.ID == aat {
			return true
		}
	}
	return false
}

// helper: check if a classification list contains a specific AAT type URI
func hasClassification(items []ClassifiedAsItem, aat string) bool {
	for _, c := range items {
		if c.ID == aat {
			return true
		}
	}
	return false
}
