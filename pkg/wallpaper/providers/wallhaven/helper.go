package wallhaven

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/dixieflatline76/Spice/v2/pkg/wallpaper"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// CheckWallhavenUsername verifies if the username exists and has accessible collections.
// Following the sequence:
// 1. Verify API Key works by fetching its owner's collections.
// 2. Fetch collections for the specific username to ensure it exists and is public.
func CheckWallhavenUsername(ctx context.Context, username, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("API key is required for username verification")
	}

	// 1. Verify API Key owner's collections
	req, err := http.NewRequestWithContext(ctx, "GET", "https://wallhaven.cc/api/v1/collections", nil)
	if err != nil {
		return fmt.Errorf("creating check request: %w", err)
	}

	q := req.URL.Query()
	q.Set("apikey", apiKey)
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("network error during API key verification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API key verification failed (status %d)", resp.StatusCode)
	}

	var colResp CollectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&colResp); err != nil {
		return fmt.Errorf("failed to decode collections list: %w", err)
	}

	if len(colResp.Data) == 0 {
		return fmt.Errorf("API key owner has no collections; favorites sync unavailable")
	}

	// 2. Verify existence of target username's collections
	usernameURL := fmt.Sprintf(WallhavenAPICollectionsRootURL, username)
	uReq, err := http.NewRequestWithContext(ctx, "GET", usernameURL, nil)
	if err != nil {
		return fmt.Errorf("creating username request: %w", err)
	}

	uq := uReq.URL.Query()
	uq.Set("apikey", apiKey)
	uReq.URL.RawQuery = uq.Encode()

	uResp, err := http.DefaultClient.Do(uReq)
	if err != nil {
		return fmt.Errorf("network error during username verification: %w", err)
	}
	defer uResp.Body.Close()

	if uResp.StatusCode != http.StatusOK {
		if uResp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("username '%s' not found or has no public collections on Wallhaven", username)
		}
		return fmt.Errorf("username verification failed (status %d)", uResp.StatusCode)
	}

	return nil
}

// CheckWallhavenAPIKeyWithContext checks if the given API key is valid using the provided context.
func CheckWallhavenAPIKeyWithContext(ctx context.Context, apiKey string) error {
	// 1. Make a Request to a Protected Endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, WallhavenTestAPIKeyURL+apiKey, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// 2. Execute the Request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	// 3. Check the Response Status Code
	if resp.StatusCode == http.StatusOK {
		return nil // Success!
	}
	return fmt.Errorf("API key is invalid")
}

// CheckWallhavenAPIKey checks if the given API key is valid (legacy wrapper).
func CheckWallhavenAPIKey(apiKey string) error {
	return CheckWallhavenAPIKeyWithContext(context.Background(), apiKey)
}

// CovertWebToAPIURL converts a web URL to an API URL. validates input, transforms to API format, determines type,
// cleans parameters, and returns the cleaned API URL suitable for saving.
func CovertWebToAPIURL(webURL string) (finalAPIURL string, queryType wallpaper.URLType, err error) {
	trimmedURL := strings.TrimSpace(webURL)
	var baseURL string
	queryType = wallpaper.Unknown // Default state

	// --- Transformation and Type Detection using constants ---
	switch {
	case UserFavoritesRegex.MatchString(trimmedURL):
		matches := UserFavoritesRegex.FindStringSubmatch(trimmedURL)
		// matches[0]=Full, matches[1]=Username, matches[2]=ID
		baseURL = fmt.Sprintf(WallhavenAPICollectionURL, matches[1], matches[2])
		queryType = wallpaper.Favorites

	case SearchRegex.MatchString(trimmedURL):
		matches := SearchRegex.FindStringSubmatch(trimmedURL)
		// matches[0]=Full match, matches[1]=Base ("https://wallhaven.cc/"), matches[2]=Query part ("?q=...") or empty string
		apiSearchBase := WallhavenAPISearchURL
		if len(matches) == 3 && matches[2] != "" { // Check if query part was captured
			baseURL = apiSearchBase + matches[2] // Append the captured query part
		} else {
			baseURL = apiSearchBase // No query part found
		}
		queryType = wallpaper.Search

	case APICollectionRegex.MatchString(trimmedURL):
		baseURL = trimmedURL // Already API format
		queryType = wallpaper.Favorites

	case APISearchRegex.MatchString(trimmedURL):
		baseURL = trimmedURL // Already API format
		queryType = wallpaper.Search

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
