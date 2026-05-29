package cleveland

import (
	"encoding/json"
	"testing"

	"github.com/dixieflatline76/Spice/v2/pkg/provider"
)

func TestObjectURLRegex(t *testing.T) {
	tests := []struct {
		url    string
		match  bool
		accNum string
	}{
		{"https://www.clevelandart.org/art/1958.31", true, "1958.31"},
		{"https://www.clevelandart.org/art/1927.1984", true, "1927.1984"},
		{"https://www.clevelandart.org/art/2015.123.4", true, "2015.123.4"},
		{"https://www.metmuseum.org/art/collection/search/12345", false, ""},
		{"https://example.com", false, ""},
	}

	for _, tt := range tests {
		matches := ObjectURLRegex.FindStringSubmatch(tt.url)
		if tt.match {
			if len(matches) < 2 {
				t.Errorf("expected match for %q, got none", tt.url)
			} else if matches[1] != tt.accNum {
				t.Errorf("expected %q for %q, got %q", tt.accNum, tt.url, matches[1])
			}
		} else {
			if len(matches) > 0 {
				t.Errorf("expected no match for %q, got %v", tt.url, matches)
			}
		}
	}
}

func TestCollectionFindEntry(t *testing.T) {
	col := &Collection{
		Entries: []CollectionEntry{
			{Key: "cma_masterpieces", Name: "Masterpieces"},
			{Key: "cma_european", Name: "European"},
		},
	}

	entry := col.FindEntry("cma_european")
	if entry == nil || entry.Name != "European" {
		t.Errorf("expected European entry, got %v", entry)
	}

	entry = col.FindEntry("nonexistent")
	if entry != nil {
		t.Errorf("expected nil for nonexistent key, got %v", entry)
	}
}

func TestEmbeddedCollection(t *testing.T) {
	var col Collection
	if err := json.Unmarshal(embeddedJSON, &col); err != nil {
		t.Fatalf("failed to parse embedded collection: %v", err)
	}

	if col.Version == "" {
		t.Error("expected non-empty version string")
	}

	if len(col.Entries) == 0 {
		t.Error("expected at least one collection entry")
	}

	masterpieces := col.FindEntry(CollectionMasterpieces)
	if masterpieces == nil {
		t.Fatal("expected masterpieces entry")
	}
	if masterpieces.Type != "curated" {
		t.Errorf("expected curated type, got %q", masterpieces.Type)
	}
	if len(masterpieces.IDs) == 0 {
		t.Error("expected curated IDs")
	}
}

func TestArtworkToImage_Landscape(t *testing.T) {
	p := &Provider{
		poolCache: make(map[int]*provider.Image),
	}

	art := apiArtwork{
		ID:              141639,
		AccessionNumber: "1965.233",
		Title:           "Twilight in the Wilderness",
		URL:             "https://www.clevelandart.org/art/1965.233",
		Images: &apiImages{
			Print: &apiImageSize{
				URL:    "https://openaccess-cdn.clevelandart.org/1965.233/1965.233_print.jpg",
				Width:  "3400",
				Height: "2123",
			},
		},
		Creators: []apiCreator{
			{Description: "Frederic Edwin Church (American, 1826-1900)", Role: "artist"},
		},
	}

	img := p.artworkToImage(&art)
	if img == nil {
		t.Fatal("expected image for landscape artwork")
	}
	if img.Path != "https://openaccess-cdn.clevelandart.org/1965.233/1965.233_print.jpg" {
		t.Errorf("unexpected image path: %s", img.Path)
	}
	if img.Attribution != "Frederic Edwin Church (American, 1826-1900) - Twilight in the Wilderness" {
		t.Errorf("unexpected attribution: %s", img.Attribution)
	}
}

func TestArtworkToImage_Portrait(t *testing.T) {
	p := &Provider{
		poolCache: make(map[int]*provider.Image),
	}

	art := apiArtwork{
		ID:    12345,
		Title: "Portrait of Someone",
		Images: &apiImages{
			Print: &apiImageSize{
				URL:    "https://example.com/print.jpg",
				Width:  "2000",
				Height: "3000",
			},
		},
	}

	img := p.artworkToImage(&art)
	if img != nil {
		t.Error("expected nil for portrait artwork")
	}
}

func TestArtworkToImage_NoImages(t *testing.T) {
	p := &Provider{
		poolCache: make(map[int]*provider.Image),
	}

	art := apiArtwork{
		ID:    12345,
		Title: "No Image",
	}

	img := p.artworkToImage(&art)
	if img != nil {
		t.Error("expected nil for artwork with no images")
	}
}
