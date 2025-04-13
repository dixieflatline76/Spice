package wallpaper

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/dixieflatline76/Spice/util/log"
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

// CovertWebToAPIURL converts a web URL to an API URL. validates input, transforms to API format, determines type,
// cleans parameters, and returns the cleaned API URL suitable for saving.
func CovertWebToAPIURL(webURL string) (finalAPIURL string, queryType URLType, err error) {
	trimmedURL := strings.TrimSpace(webURL)
	var baseURL string
	queryType = Unknown // Default state

	// --- Transformation and Type Detection using constants ---
	switch {
	case UserFavoritesRegex.MatchString(trimmedURL):
		matches := UserFavoritesRegex.FindStringSubmatch(trimmedURL)
		// matches[0]=Full, matches[1]=Username, matches[2]=ID
		baseURL = fmt.Sprintf(WallhavenAPICollectionURL, matches[1], matches[2])
		queryType = Favorites

	case SearchRegex.MatchString(trimmedURL):
		matches := SearchRegex.FindStringSubmatch(trimmedURL)
		// matches[0]=Full match, matches[1]=Base ("https://wallhaven.cc/"), matches[2]=Query part ("?q=...") or empty string
		apiSearchBase := WallhavenAPISearchURL
		if len(matches) == 3 && matches[2] != "" { // Check if query part was captured
			baseURL = apiSearchBase + matches[2] // Append the captured query part
		} else {
			baseURL = apiSearchBase // No query part found
		}
		queryType = Search

	case APICollectionRegex.MatchString(trimmedURL):
		baseURL = trimmedURL // Already API format
		queryType = Favorites

	case APISearchRegex.MatchString(trimmedURL):
		baseURL = trimmedURL // Already API format
		queryType = Search

	default:
		// This path should ideally not be reached if the Fyne validator uses WallhavenURLRegexpStr
		err = fmt.Errorf("entered URL is currently not supported: %s", trimmedURL)
		return // Return early
	}

	// --- Parameter Cleaning (for the URL to be *saved*) ---
	parsedURL, parseErr := url.Parse(baseURL)
	if parseErr != nil {
		err = fmt.Errorf("internal error parsing transformed URL '%s': %w", baseURL, parseErr)
		return
	}
	q := parsedURL.Query()
	paramsChanged := false
	// Clean API key (don't store in saved query)
	if q.Has("apikey") {
		q.Del("apikey")
		paramsChanged = true
	}
	// Clean page (don't store pagination in base query)
	if q.Has("page") {
		q.Del("page")
		paramsChanged = true
	}

	if paramsChanged {
		parsedURL.RawQuery = q.Encode()
	}
	finalAPIURL = parsedURL.String() // This is the cleaned URL suitable for saving

	// Return the cleaned URL, the detected type, and nil error if successful so far
	log.Printf("Transformed URL to: %s (Type: %s)", finalAPIURL, queryType)
	return
}
