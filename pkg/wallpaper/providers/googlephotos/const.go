package googlephotos

// GoogleClientID is the client ID for the Google Photos application.
var GoogleClientID = ""

// GoogleClientSecret is the client secret for the Google Photos application.
// Note: For Desktop apps, this is considered public.
var GoogleClientSecret = ""

const (
	// GooglePhotosRedirectURI is the redirect URI for the OAuth flow.
	GooglePhotosRedirectURI = "http://127.0.0.1:10999/callback"

	// GooglePhotosAuthURL is the URL to initiate the OAuth flow.
	GooglePhotosAuthURL = "https://accounts.google.com/o/oauth2/auth"

	// GooglePhotosTokenURL is the URL to exchange the authorization code for an access token.
	GooglePhotosTokenURL = "https://oauth2.googleapis.com/token"
)
