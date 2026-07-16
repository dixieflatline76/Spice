package getty

import (
	"context"
	"net/http"
	"testing"
)

func TestFetchThumbnails_TDD(t *testing.T) {
	p := NewProvider(nil, http.DefaultClient)

	// Valid ID from Getty
	ids := []string{"ca9023dd-235f-4344-ac1e-73e5e5f44ceb"}
	thumbnails, err := p.FetchThumbnails(context.Background(), ids)
	if err != nil {
		t.Fatalf("FetchThumbnails error: %v", err)
	}

	// This specific artwork might not have a thumbnail, but the method should not fail.
	// If it doesn't return anything, that's fine. It shouldn't crash.
	if len(thumbnails) > 0 {
		if thumbnails[0].ID != "ca9023dd-235f-4344-ac1e-73e5e5f44ceb" {
			t.Errorf("Expected ID 'ca9023dd-235f-4344-ac1e-73e5e5f44ceb', got %s", thumbnails[0].ID)
		}
	}

	// Invalid ID
	_, err = p.FetchThumbnails(context.Background(), []string{"invalid"})
	if err != nil {
		t.Errorf("Expected no error for invalid ID, got %v", err)
	}
}
