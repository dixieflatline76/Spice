package getty

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParseGettyJSONLD(t *testing.T) {
	b, err := os.ReadFile("../../../../scratch_getty_out.json")
	if err != nil {
		t.Fatalf("failed to read mock: %v", err)
	}
	var doc map[string]interface{}
	json.Unmarshal(b, &doc)

	p := NewProvider(nil, nil)
	img, err := p.parseGettyJSONLD(doc)
	if err != nil {
		t.Fatalf("parseGettyJSONLD failed: %v", err)
	}

	if img.Attribution != "Unknown - Zodiacal Sign of Virgo (Ms. Ludwig VIII 3 (83.MK.94), fol. 5)" {
		t.Errorf("Unexpected attribution: %s", img.Attribution)
	}
	if img.Path != "https://media.getty.edu/iiif/image/6d4d276f-14fc-49df-b3ff-6f363a9aa7f1/full/max/0/default.jpg" {
		t.Errorf("Unexpected URL: %s", img.Path)
	}
}
