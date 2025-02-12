package service

import "time"

// WallhavenAPIKeyPrefKey is used to set and retrieve the string wallhaven api key
const WallhavenAPIKeyPrefKey = "wallhaven_api_key"

// WallpaperChgFreqPrefKey is used to set and retrieve the int change frequency for wallpapers
const WallpaperChgFreqPrefKey = "wallpaper_chg_freq"

// SmartFitPrefKey is used to set and retrieve the boolean flag for wallpaper smart fit
const SmartFitPrefKey = "smart_fit"

// WallhavenAPIKeyRegexp is the regular expression used to validate a wallhaven API key
const WallhavenAPIKeyRegexp = `^[a-zA-Z0-9]{32}$`

// WallhavenURLRegexp is the regular expression used to validate a wallhaven URL
const WallhavenURLRegexp = `^https:\/\/wallhaven\.cc\/(search|api\/v1\/search)\?[a-zA-Z0-9\-\._\~\:\/\?\#\[\]\@\!\$\&\'\(\)\*\+\,\;\=]{0,775}$`

// WallhavenDescRegexp is the regular expression used to validate an image query description
const WallhavenDescRegexp = `^.{5,100}$`

// Service represents a service
type Service interface {
	Name() string
	Description() string
	Run()
	Stop()
	Frequency() Frequency
}

// Frequency represents the frequency of a service
type Frequency int

// Frequency constants
const (
	Frequency1Minutes Frequency = iota
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
	Frequency1Minutes:  1 * time.Minute,
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
	case Frequency1Minutes:
		return "Every Minute"
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

// GetFrequencies returns a list of all available frequencies
func GetFrequencies() []Frequency {
	return []Frequency{
		Frequency1Minutes,
		Frequency5Minutes,
		Frequency15Minutes,
		Frequency30Minutes,
		FrequencyHourly,
		Frequency3Hours,
		Frequency6Hours,
		FrequencyDaily,
	}
}
