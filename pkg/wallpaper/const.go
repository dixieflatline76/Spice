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
	CacheSizePrefKey        = pluginPrefix + "cache_size_key"         // WallpaperCacheSizePrefKey is used to set and retrieve the int wallpaper cache size
	WallpaperChgFreqPrefKey = pluginPrefix + "wallpaper_chg_freq_key" // WallpaperChgFreqPrefKey is used to set and retrieve the int change frequency for wallpapers
	ImgShufflePrefKey       = pluginPrefix + "img_shuffle_key"        // ImgShufflePrefKey is used to set and retrieve the boolean flag for wallpaper image shuffle
	ChgImgOnStartPrefKey    = pluginPrefix + "chg_img_on_start_key"   // ChgImgOnStartPrefKey is used to set and retrieve the boolean flag for changing wallpaper on startup
)

// Default values for wallpaper
const (
	MinLocalImageBeforePulse = 2               // MinLocalImageBeforePulse is the minimum number of images to keep in local before pulsing
	MaxImageWaitRetry        = 3               // MaxImageWaitRetry is the maximum number of retries to wait for an image to be downloaded
	ImageWaitRetryDelay      = 2 * time.Second // ImageWaitRetryDelay is the delay between retries to wait for an image to be downloaded
	MaxURLLength             = 1024            // MaxURLLength is the maximum length of a URL
	MaxDescLength            = 105             // MaxDescLength is the maximum length of an image description
	MinSeenImagesForDownload = 3               // MinSeenImagesForDownload is the minimum number of images seen before downloading (3 / 4)
	PrcntSeenTillDownload    = 0.80            // PrcntSeenTillDownload is the percentage of images seen before downloading (50%)
	FittedImgDir             = "fit"           // FittedImgDir is the suffix used to identify a fitted image directory
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
