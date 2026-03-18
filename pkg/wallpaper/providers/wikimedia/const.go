package wikimedia

// WikimediaClientID is the client ID for the Wikimedia OAuth application.
var WikimediaClientID = ""

// WikimediaClientSecret is the client secret for the Wikimedia OAuth application.
var WikimediaClientSecret = ""

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

	// WikimediaURLRegexp validates full Wikimedia Commons Category, Search, or Gallery URLs.
	// Matches: https://commons.wikimedia.org/wiki/... or /w/index.php?search=...
	WikimediaURLRegexp = `^(https://commons\.wikimedia\.org/(?:wiki/|w/index\.php\?)|category:|search:|file:|page:).*$`

	// Wikimedia OAuth 2.0 URLs
	// Docs: https://www.mediawiki.org/wiki/Extension:OAuth/Global_usage#OAuth_2.0
	WikimediaAuthURL  = "https://meta.wikimedia.org/w/rest.php/oauth2/authorize"
	WikimediaTokenURL = "https://meta.wikimedia.org/w/rest.php/oauth2/access_token" //nolint:gosec // Public URL

	// WikimediaRedirectURI is the local callback URL
	WikimediaRedirectURI = "http://127.0.0.1:10998/callback"
)
