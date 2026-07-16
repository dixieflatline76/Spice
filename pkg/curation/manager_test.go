package curation

import (
	"testing"
)

func TestGetEntry(t *testing.T) {
	mockJSON := []byte(`{
		"version": "v1.0.0",
		"description": "Test Collection",
		"collections": [
			{
				"name": "Test Curated",
				"key": "test_curated",
				"type": "curated",
				"ids": ["1", "2", "3"]
			},
			{
				"name": "Test Query",
				"key": "test_query",
				"type": "query",
				"ids": []
			}
		]
	}`)

	mgr := NewManager()
	mgr.embeddedData["test_provider"] = mockJSON

	entry := mgr.GetEntry("test_provider", "test_curated")
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}

	if entry.Name != "Test Curated" {
		t.Errorf("expected Test Curated, got %s", entry.Name)
	}

	if len(entry.IDs) != 3 {
		t.Fatalf("expected 3 ids, got %d", len(entry.IDs))
	}

	if entry.IDs[0] != "1" || entry.IDs[1] != "2" || entry.IDs[2] != "3" {
		t.Errorf("ids mismatch: %v", entry.IDs)
	}

	queryEntry := mgr.GetEntry("test_provider", "test_query")
	if queryEntry == nil {
		t.Fatal("expected query entry, got nil")
	}
	if queryEntry.Type != "query" {
		t.Errorf("expected query type, got %s", queryEntry.Type)
	}
}

func TestGetCollection(t *testing.T) {
	mockJSON := []byte(`{
		"version": "v2.0.0",
		"collections": [
			{
				"key": "test_1",
				"ids": ["a"]
			}
		]
	}`)
	mgr := NewManager()
	mgr.embeddedData["test_provider"] = mockJSON

	col := mgr.GetCollection("test_provider")
	if col == nil {
		t.Fatal("expected collection, got nil")
	}
	if col.Version != "v2.0.0" {
		t.Errorf("expected v2.0.0, got %s", col.Version)
	}
	if len(col.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(col.Entries))
	}
	if col.Entries[0].Key != "test_1" {
		t.Errorf("expected test_1, got %s", col.Entries[0].Key)
	}
}
