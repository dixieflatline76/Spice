package wallpaper

import (
	"fmt"
	"math"
	"time"
)

// pluginName is the name of the wallpaper plugin
const pluginName = "wallpaper"

// Preference keys for wallpaper
const (
	pluginPrefix            = pluginName + "_"
	SmartFitPrefKey         = pluginPrefix + "smart_fit_key"          // SmartFitPrefKey is used to set and retrieve the boolean flag for wallpaper smart fit
	SmartFitModePrefKey     = pluginPrefix + "smart_fit_mode_key"     // SmartFitModePrefKey is used to set and retrieve the int smart fit mode
	CacheSizePrefKey        = pluginPrefix + "cache_size_key"         // WallpaperCacheSizePrefKey is used to set and retrieve the int wallpaper cache size
	WallpaperChgFreqPrefKey = pluginPrefix + "wallpaper_chg_freq_key" // WallpaperChgFreqPrefKey is used to set and retrieve the int change frequency for wallpapers
	ImgShufflePrefKey       = pluginPrefix + "img_shuffle_key"        // ImgShufflePrefKey is used to set and retrieve the boolean flag for wallpaper image shuffle
	ChgImgOnStartPrefKey    = pluginPrefix + "chg_img_on_start_key"   // ChgImgOnStartPrefKey is used to set and retrieve the boolean flag for changing wallpaper on startup
	NightlyRefreshPrefKey   = pluginPrefix + "nightly_refresh_key"    // NightlyRefreshPrefKey is used to set and retrieve the boolean flag for nightly refresh
	FaceBoostPrefKey        = pluginPrefix + "face_boost_key"         // FaceBoostPrefKey is used to set and retrieve the boolean flag for face boost
	FaceCropPrefKey         = pluginPrefix + "face_crop_key"          // FaceCropPrefKey is used to set and retrieve the boolean flag for face crop

	// Provider keys (Shared)
	UnsplashTokenPrefKey            = "unsplash_access_token"
	GooglePhotosTokenPrefKey        = "google_photos_access_token"
	GooglePhotosRefreshTokenPrefKey = "google_photos_refresh_token"
	GooglePhotosTokenExpiryPrefKey  = "google_photos_token_expiry"
	WallhavenAPIKeyPrefKey          = "wallhaven_api_key" //nolint:gosec // Preference key, not a secret
	wallhavenConfigPrefKey          = "wallhaven_image_queries"
	PexelsAPIKeyPrefKey             = "pexels_api_key" //nolint:gosec // Preference key, not a secret
)

// URLType indicates the type of image source (Search or Collection).
type URLType int

const (
	// Unknown type represents a web URL pattern not recognized nor supported by this application.
	Unknown URLType = iota
	// Search type represents a web search query url pattern.
	Search
	// Favorites type represents a web favorites url pattern.
	Favorites
)

func (qt URLType) String() string {
	switch qt {
	case Search:
		return "Search"
	case Favorites:
		return "Favorites"
	default:
		return "Unknown"
	}
}

// Internal constants
const (
	// Cache Directory Segments
	// Structure: FittedRootDir / [Quality|Flexibility] / [Standard|FaceBoost|FaceCrop]
	FittedRootDir = "fitted"

	// Mode Segments
	QualityDir     = "quality"
	FlexibilityDir = "flexibility"

	// Type Segments
	StandardDir              = "standard"
	FaceBoostDir             = "faceboost"
	FaceCropDir              = "facecrop"
	PrcntSeenTillDownload    = 0.8
	MinSeenImagesForDownload = 5
	MinLocalImageBeforePulse = 1
	MaxImageWaitRetry        = 10
	ImageWaitRetryDelay      = 1 * time.Second
	MaxDescLength            = 50
	MaxURLLength             = 255
	MaxFavoritesLimit        = 200
	FavoritesNamespace       = "favorites" // API Namespace
	FavoritesCollection      = "favorite_images"
	FavoritesQueryID         = "favorites://" + FavoritesCollection
)

// Frequency represents the frequency of a service
type Frequency int

// Frequency constants
const (
	FrequencyNever Frequency = iota
	Frequency5Minutes
	Frequency15Minutes
	Frequency30Minutes
	FrequencyHourly
	Frequency3Hours
	Frequency6Hours
	FrequencyDaily
	FrequencyInvalid
)

// FrequencyDurations maps a Frequency to its time.Duration
var FrequencyDurations = map[Frequency]time.Duration{
	FrequencyNever:     time.Duration(math.MaxInt64),
	Frequency5Minutes:  5 * time.Minute,
	Frequency15Minutes: 15 * time.Minute,
	Frequency30Minutes: 30 * time.Minute,
	FrequencyHourly:    time.Hour,
	Frequency3Hours:    3 * time.Hour,
	Frequency6Hours:    6 * time.Hour,
	FrequencyDaily:     24 * time.Hour,
}

// String returns the string representation of a Frequency
func (f Frequency) String() string {
	switch f {
	case FrequencyNever:
		return "Never"
	case Frequency5Minutes:
		return "Every 5 Minutes"
	case Frequency15Minutes:
		return "Every 15 Minutes"
	case Frequency30Minutes:
		return "Every 30 Minutes"
	case FrequencyHourly:
		return "Hourly"
	case Frequency3Hours:
		return "Every 3 Hours"
	case Frequency6Hours:
		return "Every 6 Hours"
	case FrequencyDaily:
		return "Daily"
	default:
		return "Unknown"
	}
}

// Duration returns the time.Duration of a Frequency
func (f Frequency) Duration() time.Duration {
	return FrequencyDurations[f]
}

// GetFrequencies returns a list of all available frequencies AS fmt.Stringer
func GetFrequencies() []fmt.Stringer {
	frequencies := []Frequency{
		FrequencyNever,
		Frequency5Minutes,
		Frequency15Minutes,
		Frequency30Minutes,
		FrequencyHourly,
		Frequency3Hours,
		Frequency6Hours,
		FrequencyDaily,
	}
	stringers := make([]fmt.Stringer, len(frequencies))
	for i, f := range frequencies {
		stringers[i] = f // This is the key: assign to the interface type
	}
	return stringers
}

// CacheSize represents the predefined cache sizes (in number of images).
type CacheSize int

// CacheSize constants
const (
	CacheNone CacheSize = iota
	Cache100Images
	Cache200Images
	Cache300Images
	Cache500Images
	Cache1000Images
)

// CacheSizeValues maps CacheSize to its integer representation.
var CacheSizeValues = map[CacheSize]int{
	CacheNone:       0,
	Cache100Images:  100,
	Cache200Images:  200,
	Cache300Images:  300,
	Cache500Images:  500,
	Cache1000Images: 1000,
}

// String returns the string representation of a CacheSize.
func (cs CacheSize) String() string {
	switch cs {
	case CacheNone:
		return "None"
	case Cache100Images:
		return "100 Images"
	case Cache200Images:
		return "200 Images"
	case Cache300Images:
		return "300 Images"
	case Cache500Images:
		return "500 Images"
	case Cache1000Images:
		return "1000 Images"
	default:
		return "Unknown"
	}
}

// Size returns the integer value of a CacheSize.
func (cs CacheSize) Size() int {
	return CacheSizeValues[cs]
}

// GetCacheSizes returns a list of all available cache sizes AS fmt.Stringer.
func GetCacheSizes() []fmt.Stringer {
	cacheSizes := []CacheSize{
		CacheNone,
		Cache100Images,
		Cache200Images,
		Cache300Images,
		Cache500Images,
		Cache1000Images,
	}
	stringers := make([]fmt.Stringer, len(cacheSizes))
	for i, cs := range cacheSizes {
		stringers[i] = cs // Assign to the interface type
	}
	return stringers
}

// NetworkTimeouts defines the standard durations for various network operations.
const (
	// HTTPClientRequestTimeout is the total time limit for a single HTTP request,
	// including connection, redirects, and reading the response body.
	HTTPClientRequestTimeout = 60 * time.Second

	// HTTPClientDialerTimeout is the timeout for establishing a TCP connection.
	// This is the most critical timeout for handling network issues after sleep.
	HTTPClientDialerTimeout = 15 * time.Second

	// HTTPClientTLSHandshakeTimeout is the time limit for the TLS handshake for HTTPS.
	// This is the time limit for the TLS handshake for HTTPS.
	HTTPClientTLSHandshakeTimeout = 10 * time.Second

	// HTTPClientResponseHeaderTimeout is the time limit for receiving response headers
	// from the server after the request has been successfully sent.
	HTTPClientResponseHeaderTimeout = 15 * time.Second

	// HTTPClientKeepAlive is the duration for TCP keep-alive probes. This helps
	// maintain long-lived connections efficiently.
	HTTPClientKeepAlive = 30 * time.Second

	// NetworkConnectivityCheckTimeout is a short timeout used specifically for
	// the initial, lightweight network availability check.
	NetworkConnectivityCheckTimeout = 4 * time.Second

	// URLValidationTimeout is used when checking if a user-provided URL is valid.
	URLValidationTimeout = 15 * time.Second
)
