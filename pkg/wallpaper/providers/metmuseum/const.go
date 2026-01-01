package metmuseum

import (
	"regexp"
)

const (
	// ProviderName is the unique identifier for config
	ProviderName = "MetMuseum"

	// ProviderTitle is the user-facing name
	ProviderTitle = "The Met"

	// APIBaseURL is the base for all API calls
	APIBaseURL = "https://collectionapi.metmuseum.org/public/collection/v1"

	// Department IDs
	DeptEuropeanPaintings = 11
	DeptModernArt         = 21
	DeptAsianArt          = 6
	DeptEgyptianArt       = 10

	// Collection IDs (Internal Query Strings)
	CollectionSpiceMelange = "spice_melange"
	CollectionEuropean     = "european_masterpieces"
	CollectionModern       = "modern_art"
	CollectionAsian        = "asian_art"
	CollectionEgyptian     = "egyptian_art"
	CollectionAmerican     = "american_art"
)

// Regex for direct URL pasting
// e.g. https://www.metmuseum.org/art/collection/search/437261
var ObjectURLRegex = regexp.MustCompile(`^https://www\.metmuseum\.org/art/collection/search/(\d+)`)
