package getty

import (
	"encoding/json"
	"testing"
)

func TestEmbeddedJSON(t *testing.T) {
	var c Collection
	if err := json.Unmarshal(embeddedJSON, &c); err != nil {
		t.Fatalf("Failed to parse embedded getty.json: %v", err)
	}
	if c.Version == "" {
		t.Error("Missing version in embedded json")
	}
	if len(c.Entries) == 0 {
		t.Error("No collections defined in embedded json")
	}

	masterpieces := c.FindEntry("getty_masterpieces")
	if masterpieces == nil {
		t.Error("getty_masterpieces collection not found")
	}
}
