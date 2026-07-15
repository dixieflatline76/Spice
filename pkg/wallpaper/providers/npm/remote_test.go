package npm

import (
	"encoding/json"
	"testing"
)

func TestCollectionFindEntry(t *testing.T) {
	col := &Collection{
		Entries: []CollectionEntry{
			{Key: "npm_masterpieces", Name: "Masterpieces"},
			{Key: "npm_ceramics", Name: "Ceramics"},
		},
	}

	entry := col.FindEntry("npm_ceramics")
	if entry == nil || entry.Name != "Ceramics" {
		t.Errorf("expected Ceramics entry, got %v", entry)
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

	masterpieces := col.FindEntry("npm_masterpieces")
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
