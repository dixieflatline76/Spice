package googlephotos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dixieflatline76/Spice/util/log"
)

// CreatePickerSession starts a new Picker session and returns the Picker URI.
func (p *Provider) CreatePickerSession(ctx context.Context) (*PickerSessionResponse, error) {
	if err := p.auth.EnsureValidToken(); err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}
	accessToken := p.cfg.GetGooglePhotosToken()

	url := "https://photospicker.googleapis.com/v1/sessions"

	reqBody := PickerSessionRequest{}

	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	log.Printf("[GooglePhotos] Creating session at %s", url)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[GooglePhotos] CreateSession Failed. Status: %d, Body: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("failed to create session: %d %s", resp.StatusCode, string(body))
	}

	var session PickerSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, err
	}

	log.Printf("[GooglePhotos] Session Created: ID=%s, URI=%s", session.ID, session.PickerURI)
	return &session, nil
}

// PollSession waits for the user to complete selection or for the session to timeout.
func (p *Provider) PollSession(ctx context.Context, sessionID string, intervalStr string) (*PickerSessionResponse, error) {
	// Parse interval
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		interval = 5 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Handle full resource name "sessions/<id>" or just "<id>"
	cleanID := strings.TrimPrefix(sessionID, "sessions/")
	url := "https://photospicker.googleapis.com/v1/sessions/" + cleanID

	log.Printf("[GooglePhotos] Start Polling Session: %s (clean: %s) using URL: %s", sessionID, cleanID, url)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			// Poll status
			if err := p.auth.EnsureValidToken(); err != nil {
				return nil, err
			}
			accessToken := p.cfg.GetGooglePhotosToken()

			req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
			req.Header.Set("Authorization", "Bearer "+accessToken)

			resp, err := p.httpClient.Do(req)
			if err != nil {
				log.Printf("Polling error: %v", err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				// If 404, maybe session expired?
				// Just log and retry or fail?
				log.Printf("[GooglePhotos] Polling Status %d for %s", resp.StatusCode, url)
				if resp.StatusCode == 404 {
					log.Printf("[GooglePhotos] Session not found (404) during polling")
				}
				continue
			}

			var session PickerSessionResponse
			if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
				continue
			}

			if session.MediaItemsSet {
				log.Printf("[GooglePhotos] Session Completed! Items Set: %v. New ID? %s", session.MediaItemsSet, session.ID)
				return &session, nil
			}
			// Keep waiting
		}
	}
}

// GetSessionItems retrieves the list of picked items.
func (p *Provider) GetSessionItems(ctx context.Context, sessionID string) ([]PickerMediaItem, error) {
	if err := p.auth.EnsureValidToken(); err != nil {
		return nil, err
	}
	accessToken := p.cfg.GetGooglePhotosToken()

	var allItems []PickerMediaItem
	pageToken := ""

	cleanID := strings.TrimPrefix(sessionID, "sessions/")
	log.Printf("[GooglePhotos] GetSessionItems for %s (clean: %s)", sessionID, cleanID)

	for {
		url := fmt.Sprintf("https://photospicker.googleapis.com/v1/mediaItems?sessionId=%s&pageSize=100", cleanID)
		if pageToken != "" {
			url += "&pageToken=" + pageToken
		}

		log.Printf("[GooglePhotos] Fetching Items: %s", url)

		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("[GooglePhotos] GetSessionItems Failed: %d. Body: %s", resp.StatusCode, string(body))
			return nil, fmt.Errorf("failed to list items: %d", resp.StatusCode)
		}

		var result PickerMediaItemResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		log.Printf("[GooglePhotos] Fetched %d items", len(result.MediaItems))
		allItems = append(allItems, result.MediaItems...)

		if result.NextPageToken == "" {
			break
		}
		pageToken = result.NextPageToken
	}

	return allItems, nil
}

// DownloadItems downloads a batch of items to the target directory.
// Returns a map of filename -> ProductUrl (for metadata) and error.
func (p *Provider) DownloadItems(ctx context.Context, items []PickerMediaItem, targetDir string) (map[string]string, error) {
	// Ensure we have a valid token for downloading
	if err := p.auth.EnsureValidToken(); err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create dir: %w", err)
	}

	log.Printf("[GooglePhotos] Downloading %d items to %s", len(items), targetDir)

	urlMap := make(map[string]string)

	for i, item := range items {
		// Basic filename: ID.jpg (or guess extension from mime?)
		// item.MediaFile.Filename might be available
		ext := filepath.Ext(item.MediaFile.Filename)
		if ext == "" {
			ext = ".jpg" // Default
		}
		// sanitize filename
		safeFilename := strings.ReplaceAll(item.MediaFile.Filename, "/", "_")
		safeFilename = strings.ReplaceAll(safeFilename, "\\", "_")

		filename := fmt.Sprintf("%03d_%s", i, safeFilename)
		if safeFilename == "" {
			filename = fmt.Sprintf("%03d_%s%s", i, item.ID, ext)
		}

		path := filepath.Join(targetDir, filename)
		urlMap[filename] = item.ProductUrl

		// Download
		// Use baseUrl=d (download)
		downloadUrl := item.MediaFile.BaseURL + "=d"

		// log.Printf("Downloading %s", downloadUrl) // verbose

		if err := p.downloadFile(ctx, downloadUrl, path); err != nil {
			log.Printf("Failed to download %s: %v", item.ID, err)
			continue // Partial success allowed?
		}
	}

	return urlMap, nil
}

func (p *Provider) downloadFile(ctx context.Context, url, path string) error {
	// Use a custom client with a very long timeout for large file downloads (Google Photos videos can be huge).
	// Standard p.httpClient might have a short timeout (e.g. 30s) suitable for API calls but not downloads.
	downloadClient := &http.Client{
		// 60 minutes should be enough for even large 4K videos on decent connections
		Timeout: 60 * time.Minute,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+p.cfg.GetGooglePhotosToken())

	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
