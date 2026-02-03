//go:build darwin
// +build darwin

package sysinfo

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

var (
	// resolutionRegex matches strings like "3456 x 2234" or "2880 x 1864 Retina" or "1710 x 1107 @ 60.00Hz"
	resolutionRegex = regexp.MustCompile(`(\d+)\s*x\s*(\d+)`)
)

// systemProfilerOutput represents the nested structure of system_profiler -json
type systemProfilerOutput struct {
	Displays []gpuInfo `json:"SPDisplaysDataType"`
}

type gpuInfo struct {
	NDRVs []displayInfo `json:"spdisplays_ndrvs"`
}

type displayInfo struct {
	PixelResolution string `json:"spdisplays_pixelresolution"` // Physical pixels (e.g. "2880x1864Retina")
	Resolution      string `json:"_spdisplays_pixels"`         // Actual resolution (e.g. "3420 x 2214")
	Main            string `json:"spdisplays_main"`            // "spdisplays_yes"
}

// GetScreenDimensions returns the primary desktop dimensions on macOS.
func GetScreenDimensions() (int, int, error) {
	cmd := exec.Command("system_profiler", "SPDisplaysDataType", "-json")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to run system_profiler: %w", err)
	}

	return parseJSONResolution(out)
}

func parseJSONResolution(data []byte) (int, int, error) {
	var profiler systemProfilerOutput
	if err := json.Unmarshal(data, &profiler); err != nil {
		return 0, 0, fmt.Errorf("decoding system_profiler JSON: %w", err)
	}

	for _, gpu := range profiler.Displays {
		for _, display := range gpu.NDRVs {
			if display.Main == "spdisplays_yes" {
				// Use the actual pixels string (e.g. "3420 x 2214")
				return parseResolutionString(display.Resolution)
			}
		}
	}

	// Fallback: If no main display found, try the first display of the first GPU
	if len(profiler.Displays) > 0 && len(profiler.Displays[0].NDRVs) > 0 {
		return parseResolutionString(profiler.Displays[0].NDRVs[0].Resolution)
	}

	return 0, 0, fmt.Errorf("no displays found in system_profiler output")
}

func parseResolutionString(s string) (int, int, error) {
	matches := resolutionRegex.FindStringSubmatch(s)
	if len(matches) < 3 {
		return 0, 0, fmt.Errorf("failed to parse resolution from string: %s", s)
	}

	width, errW := strconv.Atoi(matches[1])
	height, errH := strconv.Atoi(matches[2])

	if errW != nil || errH != nil {
		return 0, 0, fmt.Errorf("failed to convert dimensions: %v, %v", errW, errH)
	}

	return width, height, nil
}
