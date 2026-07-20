package getty

import (
	"context"
	"net/http"
	"testing"
)

func TestFetchThumbnails_TDD(t *testing.T) {
	p := NewProvider(nil, http.DefaultClient)

	// Valid ID from Getty (needs to have an image representation)
	ids := []string{"9a9c0a0c-bcbc-46bc-8a7a-d1dcb06085ed"}
	thumbnails, err := p.FetchThumbnails(context.Background(), ids)
	if err != nil {
		t.Fatalf("FetchThumbnails error: %v", err)
	}

	if len(thumbnails) == 0 {
		t.Fatalf("Expected thumbnails, got none")
	}

	if thumbnails[0].ID != "9a9c0a0c-bcbc-46bc-8a7a-d1dcb06085ed" {
		t.Errorf("Expected ID '9a9c0a0c-bcbc-46bc-8a7a-d1dcb06085ed', got %s", thumbnails[0].ID)
	}
	if thumbnails[0].ViewURL == "" {
		t.Errorf("Expected ViewURL to be populated, got empty string")
	}

	// Invalid ID
	_, err = p.FetchThumbnails(context.Background(), []string{"invalid"})
	if err != nil {
		t.Errorf("Expected no error for invalid ID, got %v", err)
	}
}
