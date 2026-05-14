package rijksmuseum

import (
	"regexp"
)

const (
	// ProviderName is the unique identifier for config
	ProviderName = "Rijksmuseum"

	// ProviderTitle is the user-facing name
	ProviderTitle = "Rijksmuseum"

	// SearchBaseURL is the Linked Art search endpoint (no auth required)
	SearchBaseURL = "https://data.rijksmuseum.nl/search/collection"

	// ObjectBaseURL is the resolver for individual objects
	ObjectBaseURL = "https://id.rijksmuseum.nl"

	// WebBaseURL is the public-facing website
	WebBaseURL = "https://www.rijksmuseum.nl"

	// Collection keys
	CollectionMasterpieces = "rijks_masterpieces"
	CollectionGoldenAge    = "rijks_golden_age"
	CollectionRembrandt    = "rijks_rembrandt"
	CollectionLandscapes   = "rijks_landscapes"
	CollectionVermeer      = "rijks_vermeer"
	CollectionMaritime     = "rijks_maritime"
)

// ObjectURLRegex matches Rijksmuseum object URLs.
// e.g. https://www.rijksmuseum.nl/en/collection/SK-A-3148
var ObjectURLRegex = regexp.MustCompile(`^https?://www\.rijksmuseum\.nl/[a-z]{2}/collection/([A-Z]{1,5}-[A-Za-z0-9\-]+)`)
