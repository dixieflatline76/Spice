package wikimedia

const (
	// WikimediaBaseURL is the base URL for the Wikimedia Commons API
	WikimediaBaseURL = "https://commons.wikimedia.org/w/api.php"

	// WikimediaUserAgent is the required User-Agent header for Wikimedia API requests.
	// Policy: https://meta.wikimedia.org/wiki/User-Agent_policy
	WikimediaUserAgent = "Spice-Wallpaper-App/1.0 (https://github.com/dixieflatline76/Spice; contact@dixieflatline.com)"

	// WikimediaDomainRegexp matches commons.wikimedia.org or wikipedia.org URLs
	WikimediaDomainRegexp = `^https?://(commons\.wikimedia\.org|.*\.wikipedia\.org)/.*$`

	// WikimediaCategoryRegexp matches "Category:" pattern (case insensitive)
	WikimediaCategoryRegexp = `(?i)(Category:|Category%3A)`
)
