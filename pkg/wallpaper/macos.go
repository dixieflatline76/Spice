//go:build darwin
// +build darwin

package wallpaper

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation

#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>

static int setWallpaperNative(const char* path, int screenIndex) {
    __block int result = 0;
    // NSWorkspace calls must be made on the main thread
    dispatch_sync(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            NSString *imagePath = [NSString stringWithUTF8String:path];
            NSURL *imageURL = [NSURL fileURLWithPath:imagePath];
            NSArray *screens = [NSScreen screens];

            if (screenIndex < 0 || screenIndex >= [screens count]) {
                result = -1; // Index out of bounds
                return;
            }

            NSScreen *screen = [screens objectAtIndex:screenIndex];
            NSError *error = nil;
            BOOL success = [[NSWorkspace sharedWorkspace] setDesktopImageURL:imageURL
                                                                  forScreen:screen
                                                                    options:@{}
                                                                      error:&error];
            if (!success) {
                NSLog(@"Spice: Native wallpaper set failed: %@", [error localizedDescription]);
                result = -2; // Execution failed
            }
        }
    });
    return result;
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"unsafe"

	"github.com/dixieflatline76/Spice/v2/pkg/sysinfo"
	"github.com/dixieflatline76/Spice/v2/util/log"
)

// macOSOS implements the OS interface for macOS.
type macOSOS struct{}

type spDisplay struct {
	Name       string `json:"_name"`
	Resolution string `json:"_spdisplays_pixels"` // Changed from _spdisplays_resolution to _spdisplays_pixels for actual resolution
}

type spGPU struct {
	Displays []spDisplay `json:"spdisplays_ndrvs"`
}

type spDataType struct {
	GPUs []spGPU `json:"SPDisplaysDataType"`
}

// GetMonitors returns information about all connected monitors on macOS.
func (m *macOSOS) GetMonitors() ([]Monitor, error) {
	// 1. Mock Support for Windows Testing
	if output := os.Getenv("MOCK_MACOS_OUTPUT"); output != "" {
		return m.parseSystemProfiler(output)
	}

	// 2. Real Implementation
	cmd := exec.Command("system_profiler", "SPDisplaysDataType", "-json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting display info: %w", err)
	}

	return m.parseSystemProfiler(string(out))
}

var (
	// resolutionRegex matches strings like "3456 x 2234"
	resolutionRegex = regexp.MustCompile(`(\d+)\s*x\s*(\d+)`)
)

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
				DevicePath: d.Name, // macOS stable identifier fallback
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

// SetWallpaper sets the desktop wallpaper on macOS.
// Uses native NSWorkspace via CGO for sandbox compliance.
func (m *macOSOS) SetWallpaper(imagePath string, monitorID int) error {
	// 1. Mock Support
	if os.Getenv("MOCK_MACOS_OUTPUT") != "" {
		log.Printf("[MOCK] Setting Wallpaper for Monitor %d: %s", monitorID, imagePath)
		return nil
	}

	// 2. Real Implementation
	log.Debugf("Executing native NSWorkspace to set wallpaper for monitor %d: %s", monitorID, imagePath)
	
	cPath := C.CString(imagePath)
	defer C.free(unsafe.Pointer(cPath))

	res := C.setWallpaperNative(cPath, C.int(monitorID))
	
	switch res {
	case 0:
		return nil
	case -1:
		return fmt.Errorf("monitor index %d out of bounds (NSScreen count mismatch)", monitorID)
	case -2:
		return fmt.Errorf("NSWorkspace failed to set desktop image (check system logs)")
	default:
		return fmt.Errorf("unknown error from native wallpaper engine: %d", res)
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
