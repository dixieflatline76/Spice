//go:build linux
// +build linux

package sysinfo

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// GetScreenDimensions returns the desktop dimensions on Linux.
func GetScreenDimensions() (int, int, error) {
	// Use `xdpyinfo` to get screen resolution
	cmd := exec.Command("xdpyinfo")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get screen resolution: %w", err)
	}

	// Parse the output to extract the resolution
	// We look for "dimensions:    1920x1080 pixels (508x285 millimeters)"
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "dimensions:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				resolution := parts[1] // "1920x1080"
				dimensions := strings.Split(resolution, "x")
				if len(dimensions) == 2 {
					width, _ := strconv.Atoi(dimensions[0])
					height, _ := strconv.Atoi(dimensions[1])
					return width, height, nil
				}
			}
		}
	}

	return 0, 0, fmt.Errorf("failed to parse screen resolution")
}
