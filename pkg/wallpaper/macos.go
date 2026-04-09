//go:build darwin
// +build darwin

package wallpaper

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation
#include "wallpaper_native.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"regexp"
	"strconv"
	"unsafe"

	"github.com/dixieflatline76/Spice/v2/pkg/sysinfo"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// macOSOS implements the OS interface for macOS.
type macOSOS struct{}

// ---- JSON types for system_profiler (kept for test mock support) ----

type spDisplay struct {
	Name       string `json:"_name"`
	Resolution string `json:"_spdisplays_pixels"`
}

type spGPU struct {
	Displays []spDisplay `json:"spdisplays_ndrvs"`
}

type spDataType struct {
	GPUs []spGPU `json:"SPDisplaysDataType"`
}

var (
	resolutionRegex = regexp.MustCompile(`(\d+)\s*x\s*(\d+)`)
)

// parseSystemProfiler parses system_profiler JSON output into Monitor structs.
// Retained for unit test mock support (see macos_test.go).
func (m *macOSOS) parseSystemProfiler(jsonOutput string) ([]Monitor, error) {
	var data spDataType
	if err := json.Unmarshal([]byte(jsonOutput), &data); err != nil {
		return nil, fmt.Errorf("parsing system_profiler: %w", err)
	}

	var monitors []Monitor
	monitorIdx := 0

	for _, gpu := range data.GPUs {
		for _, d := range gpu.Displays {
			matches := resolutionRegex.FindStringSubmatch(d.Resolution)
			if len(matches) < 3 {
				continue
			}

			w, _ := strconv.Atoi(matches[1])
			h, _ := strconv.Atoi(matches[2])

			monitors = append(monitors, Monitor{
				ID:         monitorIdx,
				Name:       d.Name,
				DevicePath: d.Name,
				Rect:       image.Rect(0, 0, w, h),
			})
			monitorIdx++
		}
	}

	if len(monitors) == 0 {
		return nil, fmt.Errorf("no displays found in output")
	}

	return monitors, nil
}

// ---- Production implementation using NSScreen via CGO ----

// GetMonitors returns information about all connected monitors on macOS.
// Uses NSScreen (via native CGO) so indices are guaranteed to match SetWallpaper.
func (m *macOSOS) GetMonitors() ([]Monitor, error) {
	// Mock support for cross-platform testing
	if output := os.Getenv("MOCK_MACOS_OUTPUT"); output != "" {
		return m.parseSystemProfiler(output)
	}

	count := int(C.nativeGetScreenCount())
	if count == 0 {
		return nil, fmt.Errorf("no displays found via NSScreen")
	}

	var monitors []Monitor
	for i := 0; i < count; i++ {
		var info C.NativeMonitorInfo
		if C.nativeGetScreenInfo(C.int(i), &info) != 0 {
			log.Printf("[macOS] Failed to get info for screen index %d, skipping", i)
			continue
		}
		monitors = append(monitors, Monitor{
			ID:         i,
			Name:       C.GoString(&info.name[0]),
			DevicePath: C.GoString(&info.name[0]),
			Rect:       image.Rect(0, 0, int(info.width), int(info.height)),
		})
	}

	if len(monitors) == 0 {
		return nil, fmt.Errorf("no displays found")
	}

	return monitors, nil
}

// SetWallpaper sets the desktop wallpaper on macOS.
// Uses native NSWorkspace via CGO for sandbox compliance.
// Monitor indices are guaranteed to match GetMonitors (both use NSScreen).
func (m *macOSOS) SetWallpaper(imagePath string, monitorID int) error {
	// Mock support
	if os.Getenv("MOCK_MACOS_OUTPUT") != "" {
		log.Printf("[MOCK] Setting Wallpaper for Monitor %d: %s", monitorID, imagePath)
		return nil
	}

	log.Debugf("[macOS] Setting wallpaper via NSWorkspace for monitor %d: %s", monitorID, imagePath)

	cPath := C.CString(imagePath)
	defer C.free(unsafe.Pointer(cPath))

	res := C.nativeSetWallpaper(cPath, C.int(monitorID))

	switch res {
	case 0:
		return nil
	case -1:
		return fmt.Errorf("monitor index %d out of bounds (NSScreen count mismatch)", monitorID)
	case -2:
		return fmt.Errorf("NSWorkspace failed to set desktop image (check Console.app for details)")
	default:
		return fmt.Errorf("unknown native wallpaper error: %d", res)
	}
}

// GetDesktopDimension returns the primary desktop dimensions on macOS.
func (m *macOSOS) GetDesktopDimension() (int, int, error) {
	return sysinfo.GetScreenDimensions()
}

// Stat returns file info for the given path on macOS.
func (m *macOSOS) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// getOS returns a new instance of the macOSOS struct.
func getOS() OS {
	return &macOSOS{}
}
