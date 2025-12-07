package wallpaper

const (
	// UnsplashTokenPrefKey is the key for storing the Unsplash access token in the keyring.
	UnsplashTokenPrefKey = "unsplash_access_token"
)

// UnsplashClientID is the client ID for the Unsplash application.
// This is injected at build time via -ldflags.
var UnsplashClientID = "YOUR_UNSPLASH_ACCESS_KEY"

// UnsplashClientSecret is the client secret for the Unsplash application.
// This is injected at build time via -ldflags.
var UnsplashClientSecret = "YOUR_UNSPLASH_SECRET_KEY"

const (

	// UnsplashRedirectURI is the redirect URI for the OAuth flow.
	UnsplashRedirectURI = "http://127.0.0.1:10999/callback"

	// UnsplashAuthURL is the URL to initiate the OAuth flow.
	UnsplashAuthURL = "https://unsplash.com/oauth/authorize"

	// UnsplashTokenURL is the URL to exchange the authorization code for an access token.
	UnsplashTokenURL = "https://unsplash.com/oauth/token"
)
