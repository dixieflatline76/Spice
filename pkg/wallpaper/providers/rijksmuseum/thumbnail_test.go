package rijksmuseum

import (
	"context"
	"testing"
)

func TestRijksmuseum_FetchThumbnails(t *testing.T) {
	p := &Provider{}

	jsonID := `{"artist":"Jan Steen","image_url":"https://iiif.micr.io/bRWvw/full/max/0/default.jpg","title":"The Merry Family","id":"200107229","accession_id":"SK-C-229"}`

	thumbnails, err := p.FetchThumbnails(context.Background(), []string{jsonID})
	if err != nil {
		t.Fatalf("FetchThumbnails returned an error: %v", err)
	}

	if len(thumbnails) != 1 {
		t.Fatalf("Expected 1 thumbnail, got %d", len(thumbnails))
	}

	if thumbnails[0].ID != "200107229" {
		t.Errorf("Expected ID '200107229', got %s", thumbnails[0].ID)
	}

	if thumbnails[0].URL != "https://iiif.micr.io/bRWvw/full/800,/0/default.jpg" {
		t.Errorf("Expected URL 'https://iiif.micr.io/bRWvw/full/800,/0/default.jpg', got %s", thumbnails[0].URL)
	}
}
