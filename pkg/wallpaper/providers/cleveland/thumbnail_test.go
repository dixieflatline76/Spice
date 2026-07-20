package cleveland

import (
	"context"
	"net/http"
	"testing"
)

func TestFetchThumbnails_TDD(t *testing.T) {
	p := NewProvider(nil, http.DefaultClient)

	// Valid ID from CMA
	ids := []string{"141639"}
	thumbnails, err := p.FetchThumbnails(context.Background(), ids)
	if err != nil {
		t.Fatalf("FetchThumbnails error: %v", err)
	}

	if len(thumbnails) != 1 {
		t.Fatalf("Expected 1 thumbnail, got %d", len(thumbnails))
	}

	if thumbnails[0].ID != "141639" {
		t.Errorf("Expected ID '141639', got %s", thumbnails[0].ID)
	}
	if thumbnails[0].URL == "" {
		t.Error("Expected a valid URL, got empty string")
	}
	if thumbnails[0].ViewURL == "" {
		t.Errorf("Expected ViewURL to be populated, got empty string")
	}

	// Invalid ID
	_, err = p.FetchThumbnails(context.Background(), []string{"invalid"})
	if err != nil {
		t.Errorf("Expected no error for invalid ID (should skip), got %v", err)
	}
}
