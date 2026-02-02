package wallpaper

import (
	"net/http"
)

// UserAgentTransport wraps an http.RoundTripper and adds a User-Agent header.
type UserAgentTransport struct {
	http.RoundTripper
	UserAgent string
}

// RoundTrip executes a single HTTP transaction, adding the User-Agent header.
func (t *UserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid modifying the original
	clonedReq := req.Clone(req.Context())
	clonedReq.Header.Set("User-Agent", t.UserAgent)
	return t.RoundTripper.RoundTrip(clonedReq)
}
