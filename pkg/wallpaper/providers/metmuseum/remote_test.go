package metmuseum

import (
	"encoding/json"
	"testing"
)

func TestCollectionEntryUnmarshal_WithTranslations(t *testing.T) {
	jsonData := `{
		"name": "Artistic Nudes",
		"name_translations": {
			"fr": "Nus Artistiques",
			"de": "Künstlerische Akte"
		},
		"key": "Artistic Nudes",
		"type": "curated",
		"ids": [1, 2, 3]
	}`

	var entry CollectionEntry
	err := json.Unmarshal([]byte(jsonData), &entry)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if entry.Name != "Artistic Nudes" {
		t.Errorf("Expected Name 'Artistic Nudes', got %q", entry.Name)
	}

	if len(entry.NameTranslations) != 2 {
		t.Errorf("Expected 2 translations, got %d", len(entry.NameTranslations))
	}

	if entry.NameTranslations["fr"] != "Nus Artistiques" {
		t.Errorf("Expected 'Nus Artistiques', got %q", entry.NameTranslations["fr"])
	}
}

func TestCollectionEntryUnmarshal_WithoutTranslations(t *testing.T) {
	// Simulating an old JSON file format without translations
	jsonData := `{
		"name": "Best of The Met",
		"key": "Best of The Met",
		"type": "curated",
		"ids": [4, 5, 6]
	}`

	var entry CollectionEntry
	err := json.Unmarshal([]byte(jsonData), &entry)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if entry.Name != "Best of The Met" {
		t.Errorf("Expected Name 'Best of The Met', got %q", entry.Name)
	}

	if entry.NameTranslations != nil {
		t.Errorf("Expected NameTranslations to be nil, got %v", entry.NameTranslations)
	}
}
