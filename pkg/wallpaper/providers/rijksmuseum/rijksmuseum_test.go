package rijksmuseum

import (
	"encoding/json"
	"testing"
)

func TestExtractTitle(t *testing.T) {
	raw := json.RawMessage(`[
		{"type": "Name", "content": "De Nachtwacht", "language": [{"id": "http://vocab.getty.edu/aat/300388256", "type": "Language"}]},
		{"type": "Name", "content": "The Night Watch", "classified_as": [{"id": "http://vocab.getty.edu/aat/300404670", "type": "Type"}], "language": [{"id": "http://vocab.getty.edu/aat/300388277", "type": "Language"}]},
		{"type": "Identifier", "content": "SK-C-5", "classified_as": [{"id": "http://vocab.getty.edu/aat/300312355", "type": "Type"}]}
	]`)

	title := ExtractTitle(raw)
	if title != "The Night Watch" {
		t.Errorf("expected 'The Night Watch', got %q", title)
	}
}

func TestExtractTitle_FallbackToAnyEnglish(t *testing.T) {
	raw := json.RawMessage(`[
		{"type": "Name", "content": "Some English Title", "language": [{"id": "http://vocab.getty.edu/aat/300388277", "type": "Language"}]}
	]`)

	title := ExtractTitle(raw)
	if title != "Some English Title" {
		t.Errorf("expected 'Some English Title', got %q", title)
	}
}

func TestExtractTitle_FallbackToAnyName(t *testing.T) {
	raw := json.RawMessage(`[
		{"type": "Name", "content": "Een Nederlandse Titel", "language": [{"id": "http://vocab.getty.edu/aat/300388256", "type": "Language"}]}
	]`)

	title := ExtractTitle(raw)
	if title != "Een Nederlandse Titel" {
		t.Errorf("expected 'Een Nederlandse Titel', got %q", title)
	}
}

func TestExtractObjectNumber(t *testing.T) {
	raw := json.RawMessage(`[
		{"type": "Name", "content": "The Night Watch", "language": []},
		{"type": "Identifier", "content": "SK-C-5", "classified_as": [{"id": "http://vocab.getty.edu/aat/300312355", "type": "Type"}]}
	]`)

	objNum := ExtractObjectNumber(raw)
	if objNum != "SK-C-5" {
		t.Errorf("expected 'SK-C-5', got %q", objNum)
	}
}

func TestExtractArtist(t *testing.T) {
	p := &Production{
		ReferredToBy: []LinguisticObject{
			{Content: "Rembrandt van Rijn", ClassifiedAs: []ClassifiedAsItem{{ID: aatArtistStatement}}, Language: []TypedRef{{ID: aatEnglish}}},
		},
	}

	artist := ExtractArtist(p)
	if artist != "Rembrandt van Rijn" {
		t.Errorf("expected 'Rembrandt van Rijn', got %q", artist)
	}
}

func TestExtractArtist_Nil(t *testing.T) {
	artist := ExtractArtist(nil)
	if artist != "" {
		t.Errorf("expected empty string, got %q", artist)
	}
}

func TestExtractDimensions(t *testing.T) {
	raw := json.RawMessage(`[
		{"type": "LinguisticObject", "content": "height 379.5 cm × width 453.5 cm", "classified_as": [{"id": "http://vocab.getty.edu/aat/300435430", "type": "Type"}], "language": [{"id": "http://vocab.getty.edu/aat/300388277", "type": "Language"}]}
	]`)

	width, height := ExtractDimensions(raw)
	if width != 453.5 || height != 379.5 {
		t.Errorf("expected 453.5x379.5, got %.1fx%.1f", width, height)
	}
}

func TestExtractDimensions_LowercaseX(t *testing.T) {
	raw := json.RawMessage(`[
		{"type": "LinguisticObject", "content": "height 11 cm x width 9.5 cm", "classified_as": [{"id": "http://vocab.getty.edu/aat/300435430", "type": "Type"}], "language": [{"id": "http://vocab.getty.edu/aat/300388277", "type": "Language"}]}
	]`)

	width, height := ExtractDimensions(raw)
	if width != 9.5 || height != 11 {
		t.Errorf("expected 9.5x11, got %.1fx%.1f", width, height)
	}
}

func TestBuildObjectURL(t *testing.T) {
	url := BuildObjectURL("SK-C-5")
	expected := "https://www.rijksmuseum.nl/en/collection/SK-C-5"
	if url != expected {
		t.Errorf("expected %q, got %q", expected, url)
	}
}

func TestBuildObjectURL_Empty(t *testing.T) {
	url := BuildObjectURL("")
	if url != WebBaseURL {
		t.Errorf("expected %q, got %q", WebBaseURL, url)
	}
}

func TestObjectURLRegex(t *testing.T) {
	tests := []struct {
		url    string
		match  bool
		number string
	}{
		{"https://www.rijksmuseum.nl/en/collection/SK-C-5", true, "SK-C-5"},
		{"https://www.rijksmuseum.nl/nl/collection/SK-A-2344", true, "SK-A-2344"},
		{"https://www.rijksmuseum.nl/en/collection/RP-P-1878-A-1234", true, "RP-P-1878-A-1234"},
		{"https://www.metmuseum.org/art/collection/search/437261", false, ""},
		{"https://example.com", false, ""},
	}

	for _, tt := range tests {
		matches := ObjectURLRegex.FindStringSubmatch(tt.url)
		if tt.match {
			if len(matches) < 2 {
				t.Errorf("expected match for %q, got none", tt.url)
			} else if matches[1] != tt.number {
				t.Errorf("expected %q for %q, got %q", tt.number, tt.url, matches[1])
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
			{Key: "masterpieces", Name: "Masterpieces"},
			{Key: "landscapes", Name: "Landscapes"},
		},
	}

	entry := col.FindEntry("landscapes")
	if entry == nil || entry.Name != "Landscapes" {
		t.Errorf("expected Landscapes entry, got %v", entry)
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

	if col.Version < 1 {
		t.Errorf("expected version >= 1, got %d", col.Version)
	}

	if len(col.Entries) == 0 {
		t.Error("expected at least one collection entry")
	}

	// Verify masterpieces have pre-resolved URLs
	masterpieces := col.FindEntry(CollectionMasterpieces)
	if masterpieces == nil {
		t.Fatal("expected masterpieces entry")
	}
	if masterpieces.Type != "curated" {
		t.Errorf("expected curated type, got %q", masterpieces.Type)
	}
	if len(masterpieces.Items) == 0 {
		t.Error("expected curated items")
	}
	for _, item := range masterpieces.Items {
		if item.ImageURL == "" {
			t.Errorf("curated item %q has no image URL", item.Title)
		}
	}
}
