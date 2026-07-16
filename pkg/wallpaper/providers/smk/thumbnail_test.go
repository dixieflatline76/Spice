package smk

import (
	"context"
	"net/http"
	"testing"
)

func TestFetchThumbnails_TDD(t *testing.T) {
	p := NewProvider(nil, http.DefaultClient)

	// Valid ID from SMK
	ids := []string{"KMS3418"}
	thumbnails, err := p.FetchThumbnails(context.Background(), ids)
	if err != nil {
		t.Fatalf("FetchThumbnails error: %v", err)
	}

	if len(thumbnails) != 1 {
		t.Fatalf("Expected 1 thumbnail, got %d", len(thumbnails))
	}

	if thumbnails[0].ID != "KMS3418" {
		t.Errorf("Expected ID 'KMS3418', got %s", thumbnails[0].ID)
	}
	if thumbnails[0].URL == "" {
		t.Error("Expected a valid URL, got empty string")
	}

	// Invalid ID
	_, err = p.FetchThumbnails(context.Background(), []string{"invalid"})
	if err != nil {
		t.Errorf("Expected no error for invalid ID, got %v", err)
	}
}
