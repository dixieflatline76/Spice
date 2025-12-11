package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a semantic version with major, minor, and patch components.
type Version struct {
	Major  int
	Minor  int
	Patch  int
	Raw    string
	Prefix string // "v" or ""
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run version_util.go <bump-type>")
		fmt.Println("Where <bump-type> is one of: patch, minor, major")
		os.Exit(1)
	}

	bumpType := os.Args[1]

	// Safety Check: Ensure we are on 'main'
	currentBranch, err := getCurrentBranch()
	if err != nil {
		fmt.Println("Error determining current branch:", err)
		os.Exit(1)
	}
	if currentBranch != "main" {
		fmt.Printf("Error: Release bumps must be performed on 'main'. Current branch: '%s'\n", currentBranch)
		os.Exit(1)
	}

	version, err := readVersionFromFile("version.txt")
	if err != nil {
		fmt.Println("Error reading version from file:", err)
		os.Exit(1)
	}

	newVersion, err := bumpVersion(version, bumpType)
	if err != nil {
		fmt.Println("Error incrementing version:", err)
		os.Exit(1)
	}

	err = writeVersionToFile("version.txt", newVersion)
	if err != nil {
		fmt.Println("Error writing new version to file:", err)
		os.Exit(1)
	}

	// Commit the version change before tagging
	err = commitVersionFile("version.txt", newVersion.String())
	if err != nil {
		fmt.Println("Error committing version file:", err)
		os.Exit(1)
	}

	err = createGitTag(newVersion.String())
	if err != nil {
		fmt.Println("Error creating Git tag:", err)
		os.Exit(1)
	}
}

// readVersionFromFile reads the version string from the specified file.
func readVersionFromFile(filename string) (Version, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return Version{}, err
	}

	versionString := strings.TrimSpace(string(data))
	return parseVersion(versionString)
}

// parseVersion parses a version string into a Version struct.
func parseVersion(versionString string) (Version, error) {
	re := regexp.MustCompile(`^(v?)(\d+)\.(\d+)\.(\d+)$`)
	matches := re.FindStringSubmatch(versionString)

	if matches == nil {
		return Version{}, fmt.Errorf("invalid version format: %s", versionString)
	}

	major, _ := strconv.Atoi(matches[2]) // Error handling not needed due to regex
	minor, _ := strconv.Atoi(matches[3])
	patch, _ := strconv.Atoi(matches[4])

	return Version{
		Major:  major,
		Minor:  minor,
		Patch:  patch,
		Prefix: matches[1],
		Raw:    versionString,
	}, nil
}

// bumpVersion increments the version based on the bump type.
func bumpVersion(v Version, bumpType string) (Version, error) {
	switch bumpType {
	case "patch":
		v.Patch++
	case "minor":
		v.Minor++
		v.Patch = 0
	case "major":
		v.Major++
		v.Minor = 0
		v.Patch = 0
	default:
		return v, fmt.Errorf("invalid bump type: %s", bumpType)
	}

	v.Raw = v.String()
	return v, nil
}

// writeVersionToFile writes the new version string to the specified file.
func writeVersionToFile(filename string, v Version) error {
	return os.WriteFile(filename, []byte(v.String()), 0644)
}

// String returns the formatted version string (e.g., "v1.2.4").
func (v Version) String() string {
	return fmt.Sprintf("%s%d.%d.%d", v.Prefix, v.Major, v.Minor, v.Patch)
}

// createGitTag creates a Git tag with the given version string.
func createGitTag(versionString string) error {
	cmd := exec.Command("git", "tag", "-a", versionString, "-m", fmt.Sprintf("Release %s", versionString))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}

	cmd = exec.Command("git", "push", "origin", versionString)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// commitVersionFile commits the version file to git.
func commitVersionFile(filename, version string) error {
	cmd := exec.Command("git", "add", filename)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	commitMsg := fmt.Sprintf("Bump version to %s", version)
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// getCurrentBranch returns the name of the current git branch.
func getCurrentBranch() (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git branch failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
