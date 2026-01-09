package artic

import (
	"regexp"
)

const (
	// ProviderName is the unique identifier for config
	ProviderName = "ArtInstituteChicago"

	// ProviderTitle is the user-facing name
	ProviderTitle = "Art Institute of Chicago"

	// APIBaseURL is the base for all API calls
	APIBaseURL = "https://api.artic.edu/api/v1"

	// Collection IDs (Internal Query Strings)
	CollectionHighlights = "artic_highlights"
	CollectionImpression = "artic_impressionism"
	CollectionModern     = "artic_modern"
	CollectionAsia       = "artic_asia"
)

// ObjectURLRegex matches direct AIC artwork URLs
// e.g. https://www.artic.edu/artworks/27992/a-sunday-on-la-grande-jatte-1884
var ObjectURLRegex = regexp.MustCompile(`^https://www\.artic\.edu/artworks/(\d+)`)
