package wallpaper

import (
	"fmt"
	"image"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Monitor represents a connected display
type Monitor struct {
	ID         int             // Internal ID (0, 1, 2); 0 is the primary display on every OS
	Name       string          // OS-specific name (e.g. "DELL U2723QE", "DP-1")
	DevicePath string          // Stable OS path/identifier
	Rect       image.Rectangle // Physical pixel dimensions (image fitting & resolution-change detection)
	Bounds     image.Rectangle // Desktop bounds in native window/cursor units (window placement & cursor hit-testing)
	Primary    bool            // True if this is the OS primary display
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

var xrandrMonitorRegex = regexp.MustCompile(`(\d+):\s+\+(\*?)(\S+)\s+(\d+)/\d+x(\d+)/\d+\+(\d+)\+(\d+)`)

// ParseXRandrMonitors parses `xrandr --listmonitors` output into Monitor structs.
// If no monitor carries the '*' primary marker, the first monitor is marked Primary.
func ParseXRandrMonitors(output string) ([]Monitor, error) {
	var monitors []Monitor
	for _, line := range strings.Split(output, "\n") {
		matches := xrandrMonitorRegex.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) < 8 {
			continue
		}
		id, _ := strconv.Atoi(matches[1])
		w, _ := strconv.Atoi(matches[4])
		h, _ := strconv.Atoi(matches[5])
		x, _ := strconv.Atoi(matches[6])
		y, _ := strconv.Atoi(matches[7])
		monitors = append(monitors, Monitor{
			ID:      id,
			Name:    matches[3],
			Rect:    image.Rect(0, 0, w, h),
			Bounds:  image.Rect(x, y, x+w, y+h),
			Primary: matches[2] == "*",
		})
	}
	ensurePrimary(monitors)
	return monitors, nil
}

// ensurePrimary marks the first monitor Primary if none is.
func ensurePrimary(monitors []Monitor) {
	for _, m := range monitors {
		if m.Primary {
			return
		}
	}
	if len(monitors) > 0 {
		monitors[0].Primary = true
	}
}

// MonitorAtPoint returns the ID of the monitor whose Bounds (or Rect if Bounds is
// empty) contains pt, and hit=true. If no monitor contains pt it returns the
// Primary (or first) monitor ID and hit=false.
func MonitorAtPoint(pt image.Point, monitors []Monitor) (id int, hit bool) {
	if len(monitors) == 0 {
		return 0, false
	}
	for _, m := range monitors {
		b := m.Bounds
		if b.Empty() {
			b = m.Rect
		}
		if pt.In(b) {
			return m.ID, true
		}
	}
	for _, m := range monitors {
		if m.Primary {
			return m.ID, false
		}
	}
	return monitors[0].ID, false
}

// PlanTrayMonitors decides which monitor renders flat at the tray-menu root and
// which (if any) render as submenus.
//
//	perScreen=true  (macOS separate Spaces): activeID at root, no submenus.
//	perScreen=false (Windows/Linux):         Primary at root, others as submenus.
func PlanTrayMonitors(monitors []Monitor, perScreen bool, activeID int) (rootID int, submenus []Monitor) {
	if len(monitors) == 0 {
		return 0, nil
	}
	rootID = monitors[0].ID
	for _, m := range monitors {
		if m.Primary {
			rootID = m.ID
			break
		}
	}
	if perScreen {
		for _, m := range monitors {
			if m.ID == activeID {
				return activeID, nil
			}
		}
		return rootID, nil
	}
	for _, m := range monitors {
		if m.ID != rootID {
			submenus = append(submenus, m)
		}
	}
	return rootID, submenus
}

// sortAndIndexMonitors orders monitors Primary-first, then left-to-right,
// then top-to-bottom, and reassigns sequential IDs. Used by Windows so that
// "Display 1" is always the primary display.
func sortAndIndexMonitors(monitors []Monitor) []Monitor {
	sort.SliceStable(monitors, func(i, j int) bool {
		if monitors[i].Primary != monitors[j].Primary {
			return monitors[i].Primary
		}
		if monitors[i].Bounds.Min.X != monitors[j].Bounds.Min.X {
			return monitors[i].Bounds.Min.X < monitors[j].Bounds.Min.X
		}
		return monitors[i].Bounds.Min.Y < monitors[j].Bounds.Min.Y
	})
	for i := range monitors {
		monitors[i].ID = i
	}
	return monitors
}
