package cleveland

import (
	"regexp"
)

const (
	// ProviderName is the unique identifier for config
	ProviderName = "ClevelandMuseum"

	// ProviderTitle is the user-facing name
	ProviderTitle = "CMA"

	// APIBaseURL is the base for all API calls (no auth required)
	APIBaseURL = "https://openaccess-api.clevelandart.org/api/artworks/"

	// WebBaseURL is the public-facing website
	WebBaseURL = "https://www.clevelandart.org"

	// Collection keys
	CollectionMasterpieces  = "cma_masterpieces"
	CollectionAmerican      = "cma_american"
	CollectionEuropean      = "cma_european"
	CollectionImpressionism = "cma_impressionism"
	CollectionAsian         = "cma_asian"
	CollectionPhotography   = "cma_photography"
)

// ObjectURLRegex matches Cleveland Museum of Art object URLs.
// e.g. https://www.clevelandart.org/art/1958.31
var ObjectURLRegex = regexp.MustCompile(`^https?://www\.clevelandart\.org/art/([0-9]+(?:\.[0-9]+)*)`)
