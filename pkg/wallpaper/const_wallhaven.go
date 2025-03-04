package wallpaper

// serviceName is the name of the wallhaven image service
const serviceName = "wallhaven"

// Preference keys for wwallhaven.cc image service
const (
	servicePrefix          = serviceName + "_"               // servicePrefix is the prefix used for all wallhaven image service preference keys
	wallhavenConfigPrefKey = servicePrefix + "image_queries" // wallhavenConfigPrefKey is the string key use for saving and retrieving wallhaven image queries to fyne preferences
	WallhavenAPIKeyPrefKey = servicePrefix + "api_key"       // WallhavenAPIKeyPrefKey is used to set and retrieve the string wallhaven api key
)

// Default values for wallhaven.cc image service
const (
	WallhavenAPIKeyRegexp  = `^[a-zA-Z0-9]{32}$`                                                                              // WallhavenAPIKeyRegexp is the regular expression used to validate a wallhaven API key
	WallhavenURLRegexp     = `^https:\/\/wallhaven\.cc\/(?:search|api\/v1\/search)(?:\?[a-zA-Z0-9_\-.~!$&'()*+,;=:@\/?%]*|)$` // WallhavenURLRegexp is the regular expression used to validate a wallhaven URL
	WallhavenDescRegexp    = `^[^\x00-\x1F\x7F]{5,150}$`                                                                      // WallhavenDescRegexp is the regular expression used to validate an image query description
	WallhavenTestAPIKeyURL = "https://wallhaven.cc/api/v1/settings?apikey="                                                   // WallhavenTestAPIKeyURL is the URL used to test a wallhaven API key
)
