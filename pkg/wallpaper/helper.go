package wallpaper

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2"
)

// extractFilenameFromURL extracts the filename from a URL.
func extractFilenameFromURL(url string) string {
	lastSlashIndex := strings.LastIndex(url, "/")
	if lastSlashIndex == -1 || lastSlashIndex == len(url)-1 {
		return "" // Handle cases where there's no slash or it's at the end
	}
	return url[lastSlashIndex+1:]
}

// isImageFile checks if a file has a common image extension.
func isImageFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif"
}

// CheckWallhavenAPIKey checks if the given API key is valid.
func CheckWallhavenAPIKey(apiKey string) error {
	// 1. Configure the OAuth2 HTTP Client
	// Wallhaven uses API keys as Bearer tokens, which OAuth2 handles nicely.
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: apiKey},
	)
	client := oauth2.NewClient(context.Background(), ts)

	// 2. Make a Request to a Protected Endpoint
	// Choose an endpoint that requires authentication.  The 'account' endpoint is a good option.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, WallhavenTestAPIKeyURL+apiKey, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// 3. Execute the Request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	// 4. Check the Response Status Code
	if resp.StatusCode == http.StatusOK {
		return nil // Success!
	}
	return fmt.Errorf("API key is invalid")
}

// CovertToAPIURL converts the given URL to a Wallhaven API URL.
func CovertToAPIURL(queryURL string) string {

	// Convert to API URL
	queryURL = strings.Replace(queryURL, "https://wallhaven.cc/search?", "https://wallhaven.cc/api/v1/search?", 1)

	u, err := url.Parse(queryURL)
	if err != nil {
		// Not a valid URL
		return queryURL
	}

	q := u.Query()

	// Remove API key
	if q.Has("apikey") {
		q.Del("apikey")
	}

	// Remove page
	if q.Has("page") {
		q.Del("page")
	}

	u.RawQuery = q.Encode()
	return u.String()
}
