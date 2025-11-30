package util

import (
	"context"
	"fmt"
	"strings"

	"github.com/dixieflatline76/Spice/config"
	"github.com/google/go-github/v63/github"
	"golang.org/x/mod/semver"
)

const (
	githubOwner = "dixieflatline76"
	githubRepo  = "Spice"
)

// CheckForUpdatesResult holds the outcome of the update check.
type CheckForUpdatesResult struct {
	UpdateAvailable bool
	CurrentVersion  string
	LatestVersion   string
	ReleaseURL      string
	ReleaseNotes    string
}

// CheckForUpdates polls GitHub for the latest stable release.
// It automatically uses the global config.AppVersion.
func CheckForUpdates() (*CheckForUpdatesResult, error) {
	client := github.NewClient(nil)

	release, _, err := client.Repositories.GetLatestRelease(context.Background(), githubOwner, githubRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest GitHub release: %w", err)
	}

	// Use the global AppVersion from the config package.
	currentAppVersion := config.AppVersion
	latestVersionTag := release.GetTagName()

	// Prepare versions for semantic version comparison.
	if !strings.HasPrefix(currentAppVersion, "v") {
		currentAppVersion = "v" + currentAppVersion
	}
	if !strings.HasPrefix(latestVersionTag, "v") {
		latestVersionTag = "v" + latestVersionTag
	}

	result := &CheckForUpdatesResult{
		CurrentVersion: currentAppVersion,
		LatestVersion:  latestVersionTag,
		ReleaseURL:     release.GetHTMLURL(),
		ReleaseNotes:   release.GetBody(),
	}

	if semver.Compare(latestVersionTag, currentAppVersion) > 0 {
		result.UpdateAvailable = true
	}

	return result, nil
}
