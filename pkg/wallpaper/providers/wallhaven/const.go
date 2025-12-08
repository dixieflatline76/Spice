package wallhaven

import "regexp"

// serviceName is the name of the wallhaven image service
const serviceName = "wallhaven"

// Preference keys for wwallhaven.cc image service
const (
	servicePrefix = serviceName + "_" // servicePrefix is the prefix used for all wallhaven image service preference keys

	WallhavenAPIKeyPrefKey = servicePrefix + "api_key" // WallhavenAPIKeyPrefKey is used to set and retrieve the string wallhaven api key
)

// Default values for wallhaven.cc image service
const (
	WallhavenURLRegexp = `^https:\/\/wallhaven\.cc\/(?:search|api\/v1\/search|api\/v1\/collections\/[a-zA-Z0-9_]+\/[0-9]+|user\/[a-zA-Z0-9_]+\/favorites\/[0-9]+|favorites\/[0-9]+)(?:\?[a-zA-Z0-9_\-.~!$&'()*+,;=:@\/?%]*|)$` // WallhavenURLRegexp is the regular expression used to validate a wallhaven URL

	WallhavenAPIKeyRegexp     = `^[a-zA-Z0-9]{32}$`                             // WallhavenAPIKeyRegexp is the regular expression used to validate a wallhaven API key
	WallhavenDescRegexp       = `^[^\x00-\x1F\x7F]{5,150}$`                     // WallhavenDescRegexp is the regular expression used to validate an image query description
	WallhavenTestAPIKeyURL    = "https://wallhaven.cc/api/v1/settings?apikey="  // WallhavenTestAPIKeyURL is the URL used to test a wallhaven API key
	WallhavenAPISearchURL     = "https://wallhaven.cc/api/v1/search"            // WallhavenAPISearchURL is the URL used to search for images on wallhaven API
	WallhavenAPICollectionURL = "https://wallhaven.cc/api/v1/collections/%s/%s" // WallhavenAPICollectionURL is the URL used to access a collection on wallhaven API
)

// Compiled Regexps reused in transformation logic.
// MustCompile panics if the expression is invalid, which is okay here as they are static.
var (
	// UserFavoritesRegex matches /user/{user}/favorites/{id} and captures user & id.
	UserFavoritesRegex = regexp.MustCompile(`^https:\/\/wallhaven\.cc\/user\/([a-zA-Z0-9_]+)\/favorites\/([0-9]+)\/?(?:\?.*)?$`)

	// OwnFavoritesRegex matches /favorites/{id} and captures id.
	OwnFavoritesRegex = regexp.MustCompile(`^https:\/\/wallhaven\.cc\/favorites\/([0-9]+)\/?(?:\?.*)?$`)

	// SearchRegex matches /search (non-API) and captures the base URL and the optional query string part.
	SearchRegex = regexp.MustCompile(`^(https:\/\/wallhaven\.cc\/)search\/?(\?.*)?$`) // Allows optional trailing slash before query

	// APICollectionRegex checks if a URL starts with the API collection path prefix.
	APICollectionRegex = regexp.MustCompile(`^https:\/\/wallhaven\.cc\/api\/v1\/collections\/`)

	// APISearchRegex checks if a URL starts with the API search path prefix.
	APISearchRegex = regexp.MustCompile(`^https:\/\/wallhaven\.cc\/api\/v1\/search`)
)

// URLType indicates the type of Wallhaven source (Search or Collection).
type URLType int

const (
	// Unknown type represents a web URL pattern not recognized nor supported by this application.
	Unknown URLType = iota
	// Search type represents a web search query url pattern.
	Search
	// Favorites type represents a web favorites url pattern.
	Favorites
)

// String provides a human-readable representation for the QueryType.
// This is useful for logging, debugging, or potentially displaying in the UI.
func (qt URLType) String() string {
	switch qt {
	case Search:
		return "Search"
	case Favorites:
		return "Favorites"
	default:
		return "Unknown"
	}
}
