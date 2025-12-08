package wallpaper_test

import (
	"regexp"
	"testing"

	"github.com/dixieflatline76/Spice/pkg/wallpaper/providers/pexels"
	"github.com/dixieflatline76/Spice/pkg/wallpaper/providers/unsplash"
	"github.com/dixieflatline76/Spice/pkg/wallpaper/providers/wallhaven"
)

func TestValidationRegexes(t *testing.T) {
	tests := []struct {
		name        string
		regexStr    string
		inputs      []string
		shouldMatch bool
	}{
		// --- Wallhaven URLs ---
		{
			name:     "Wallhaven URL - Valid Search",
			regexStr: wallhaven.WallhavenURLRegexp,
			inputs: []string{
				"https://wallhaven.cc/search?q=anime",
				"https://wallhaven.cc/search?q=anime&categories=111",
				"https://wallhaven.cc/search",
			},
			shouldMatch: true,
		},
		{
			name:     "Wallhaven URL - Invalid",
			regexStr: wallhaven.WallhavenURLRegexp,
			inputs: []string{
				"http://wallhaven.cc/search",   // No HTTPS
				"https://wallhaven.com/search", // Wrong domain
				"random string",
			},
			shouldMatch: false,
		},

		// --- Unsplash URLs ---

		{
			name:     "Unsplash URL - Valid Collections",
			regexStr: unsplash.UnsplashURLRegexp,
			inputs: []string{
				"https://unsplash.com/s/photos/nature",
				"https://www.unsplash.com/s/photos/nature",
			},
			shouldMatch: true,
		},
		{
			name:     "Unsplash URL - Invalid Single Photo",
			regexStr: unsplash.UnsplashURLRegexp,
			inputs: []string{
				"https://unsplash.com/photos/xyz-123",
				"https://unsplash.com/invalid",
				"http://unsplash.com/s/photos/nature",
			},
			shouldMatch: false,
		},

		// --- Pexels URLs ---
		{
			name:     "Pexels URL - Valid Search",
			regexStr: pexels.PexelsURLRegexp,
			inputs: []string{
				"https://www.pexels.com/search/nature/",
				"https://pexels.com/search/test",
			},
			shouldMatch: true,
		},
		{
			name:     "Pexels URL - Valid Collections",
			regexStr: pexels.PexelsURLRegexp,
			inputs: []string{
				"https://www.pexels.com/collections/my-collection-12345/",
			},
			shouldMatch: true,
		},
		{
			name:     "Pexels URL - Invalid",
			regexStr: pexels.PexelsURLRegexp,
			inputs: []string{
				"https://www.pexels.com/",
				"https://www.pexels.com/license",
				"ftp://pexels.com/search/test",
			},
			shouldMatch: false,
		},

		// --- Pexels API Key ---
		{
			name:     "Pexels API Key - Valid",
			regexStr: pexels.PexelsAPIKeyRegexp,
			inputs: []string{
				"563492ad6f9170000100000100000000000000000000000000000000", // 56 chars
			},
			shouldMatch: true,
		},
		{
			name:     "Pexels API Key - Invalid",
			regexStr: pexels.PexelsAPIKeyRegexp,
			inputs: []string{
				"shortkey",
				"563492ad6f917000010000010000000000000000000000000000000",   // 55 chars
				"563492ad6f91700001000001000000000000000000000000000000000", // 57 chars
				"invalid-characters!@#$",
			},
			shouldMatch: false,
		},

		// --- Wallhaven API Key ---
		{
			name:     "Wallhaven API Key - Valid",
			regexStr: wallhaven.WallhavenAPIKeyRegexp,
			inputs: []string{
				"AbCdEfGb123456789012345678901234", // 32 alphanumeric
			},
			shouldMatch: true,
		},
		{
			name:     "Wallhaven API Key - Invalid",
			regexStr: wallhaven.WallhavenAPIKeyRegexp,
			inputs: []string{
				"short",
				"AbCdEfGb1234567890123456789012345", // 33 chars
				"AbCdEfGb12345678901234567890123!",  // Invalid char
			},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regex, err := regexp.Compile(tt.regexStr)
			if err != nil {
				t.Fatalf("Failed to compile regex %q: %v", tt.regexStr, err)
			}

			for _, input := range tt.inputs {
				matched := regex.MatchString(input)
				if matched != tt.shouldMatch {
					t.Errorf("Input %q: expected match=%v, got=%v (regex: %s)", input, tt.shouldMatch, matched, tt.regexStr)
				}
			}
		})
	}
}
