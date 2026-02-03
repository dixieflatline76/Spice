package wallpaper

import (
	"fmt"
	"image"
	"path/filepath"
)

// Monitor represents a connected display
type Monitor struct {
	ID   int             // Internal ID (0, 1, 2)
	Name string          // OS-specific name (e.g. "DP-1")
	Rect image.Rectangle // Dimensions (X, Y, W, H)
}

// MonitorContext is a transient object passed through the pipeline
type MonitorContext struct {
	ID   int
	Rect image.Rectangle
	// Config *SessionConfig // Future: override settings
}

// Resolution represents a unique display resolution and the monitors that use it.
type Resolution struct {
	Width, Height int
	Monitors      []int // List of Monitor IDs using this resolution
}

// GetUniqueResolutions filters a list of monitors to return only unique resolutions.
// It groups monitors by resolution.
func GetUniqueResolutions(monitors []Monitor) []Resolution {
	grouped := make(map[string]*Resolution)
	var order []string // To preserve order or deterministic output

	for _, m := range monitors {
		w, h := m.Rect.Dx(), m.Rect.Dy()
		if w <= 0 || h <= 0 {
			continue // Skip invalid/empty monitors
		}
		key := fmt.Sprintf("%dx%d", w, h)

		if _, exists := grouped[key]; !exists {
			grouped[key] = &Resolution{
				Width:    w,
				Height:   h,
				Monitors: []int{},
			}
			order = append(order, key)
		}
		grouped[key].Monitors = append(grouped[key].Monitors, m.ID)
	}

	var unique []Resolution
	for _, key := range order {
		unique = append(unique, *grouped[key])
	}
	return unique
}

// GetDerivativePath returns the calculated path for a specific resolution.
// Format: .../fitted/{Width}x{Height}/{ID}.jpg
func (wp *Plugin) GetDerivativePath(id string, w, h int) string {
	resolutionDir := fmt.Sprintf("%dx%d", w, h)
	filename := id + ".jpg"

	// Using FittedRootDir from const.go
	return filepath.Join(wp.fm.GetDownloadDir(), FittedRootDir, resolutionDir, filename)
}
