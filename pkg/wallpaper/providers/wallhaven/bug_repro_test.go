package wallhaven

import (
	"testing"
)

func TestParseURL_ComplexUserQuery(t *testing.T) {
	// ParseUrl does not use the config for parsing, so nil is safe
	provider := NewWallhavenProvider(nil, nil)
	inputURL := "https://wallhaven.cc/search?q=id%3A37&categories=101&purity=100&atleast=3840x2160&sorting=random&order=desc&seed=ZjbLxi&page=2"

	parsed, err := provider.ParseURL(inputURL)
	if err != nil {
		t.Fatalf("ParseURL failed: %v", err)
	}

	t.Logf("Parsed URL: %s", parsed)

	// Check if key parameters are present
	if len(parsed) < 50 {
		t.Errorf("Parsed URL seems too short or empty: %s", parsed)
	}
}
