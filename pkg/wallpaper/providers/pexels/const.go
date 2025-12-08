package pexels

// pexelsServiceName is the name of the pexels image service, mostly used for prefixing preference keys
const pexelsServiceName = "pexels"

// Preference keys for Pexels image service
const (
	pexelsServicePrefix = pexelsServiceName + "_"         // pexelsServicePrefix is the prefix used for all pexels image service preference keys
	PexelsAPIKeyPrefKey = pexelsServicePrefix + "api_key" // PexelsAPIKeyPrefKey is used to set and retrieve the string pexels api key
)

// Pexels API URLs
const (
	PexelsAPISearchURL     = "https://api.pexels.com/v1/search"
	PexelsAPICollectionURL = "https://api.pexels.com/v1/collections/%s"

	// PexelsURLRegexp validates Pexels URLs (search, collections).
	// Matches: https://www.pexels.com/search/..., https://www.pexels.com/collections/...
	PexelsURLRegexp = `^https:\/\/(?:www\.)?pexels\.com\/(?:search\/|collections\/).*$`

	// PexelsAPIKeyRegexp validates Pexels API keys (typically 56 params).
	PexelsAPIKeyRegexp = `^[a-zA-Z0-9-]{56}$`

	// PexelsDescRegexp validates image descriptions.
	PexelsDescRegexp = `^[^\x00-\x1F\x7F]{5,150}$`
)
