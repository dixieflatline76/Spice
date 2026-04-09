package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: bump_msix <version> <build_number> <manifest_path>")
		os.Exit(1)
	}

	version := os.Args[1]
	manifestPath := os.Args[3]

	// Clean version string (e.g. from "v2.2.4" to "2.2.4")
	version = strings.TrimPrefix(version, "v")

	// Ensure we extract the major, minor, and build conceptually
	parts := strings.Split(version, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	if len(parts) > 3 {
		parts = parts[:3] // Cap to 3 segments
	}

	// Produce strict 4-part version for Windows (Major.Minor.Build.Revision)
	// NOTE: Microsoft Store requires the Revision (4th part) to be 0 for initial submissions.
	fullVersion := fmt.Sprintf("%s.%s.%s.0", parts[0], parts[1], parts[2])

	content, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Printf("Error reading manifest: %v\n", err)
		os.Exit(1)
	}

	// Replace the Version string safely using RegEx (matching space before Version to prevent matching MinVersion)
	re := regexp.MustCompile(`\sVersion="[0-9.]+"`)
	newContent := re.ReplaceAll(content, []byte(fmt.Sprintf(` Version="%s"`, fullVersion)))

	err = os.WriteFile(manifestPath, newContent, 0600)
	if err != nil {
		fmt.Printf("Error writing manifest: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully injected dynamic MSIX version: %s\n", fullVersion)
}
