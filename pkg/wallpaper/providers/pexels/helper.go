package pexels

import (
	"context"
	"fmt"
	"net/http"
)

// CheckPexelsAPIKeyWithContext verifies if the given API key is valid using the provided context.
// Uses the /v1/collections endpoint which requires authentication (unlike /v1/curated which is public).
func CheckPexelsAPIKeyWithContext(ctx context.Context, apiKey string) error {
	if len(apiKey) < 10 {
		return fmt.Errorf("invalid Pexels API key (too short)")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.pexels.com/v1/collections?per_page=1", nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("invalid Pexels API key")
	}

	return fmt.Errorf("Pexels API verification failed (status %d)", resp.StatusCode)
}

// CheckPexelsAPIKey verifies if the given API key is valid (legacy wrapper).
func CheckPexelsAPIKey(apiKey string) error {
	return CheckPexelsAPIKeyWithContext(context.Background(), apiKey)
}
