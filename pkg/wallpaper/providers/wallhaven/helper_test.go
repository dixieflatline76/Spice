package wallhaven

import (
	"testing"

	"github.com/dixieflatline76/Spice/pkg/wallpaper"
	"github.com/stretchr/testify/assert"
)

func TestCovertWebToAPIURL(t *testing.T) {
	tests := []struct {
		name          string
		inputURL      string
		expectedURL   string
		expectedType  wallpaper.URLType
		expectedError bool
	}{
		{
			name:         "Search URL",
			inputURL:     "https://wallhaven.cc/search?q=anime",
			expectedURL:  "https://wallhaven.cc/api/v1/search?q=anime",
			expectedType: wallpaper.Search,
		},
		{
			name:         "Collection URL",
			inputURL:     "https://wallhaven.cc/user/username/favorites/12345",
			expectedURL:  "https://wallhaven.cc/api/v1/collections/username/12345",
			expectedType: wallpaper.Favorites,
		},
		{
			name:         "API Search URL",
			inputURL:     "https://wallhaven.cc/api/v1/search?q=anime",
			expectedURL:  "https://wallhaven.cc/api/v1/search?q=anime",
			expectedType: wallpaper.Search,
		},
		{
			name:          "Invalid URL",
			inputURL:      "https://google.com",
			expectedError: true,
		},
		{
			name:         "URL with API Key (should be removed)",
			inputURL:     "https://wallhaven.cc/api/v1/search?q=anime&apikey=123",
			expectedURL:  "https://wallhaven.cc/api/v1/search?q=anime",
			expectedType: wallpaper.Search,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, qType, err := CovertWebToAPIURL(tt.inputURL)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURL, url)
				assert.Equal(t, tt.expectedType, qType)
			}
		})
	}
}
