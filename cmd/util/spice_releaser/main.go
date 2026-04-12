package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/google/go-github/v63/github"
)

const (
	repoOwner   = "dixieflatline76"
	repoName    = "Spice"
	homebrewTap = "homebrew-spice"
	wingetRepo  = "winget-pkgs"
)

func main() {
	log.SetFlags(log.Lshortfile | log.Ltime)

	token := os.Getenv("SPICE_RELEASER_TOKEN")
	if token == "" {
		log.Fatal("SPICE_RELEASER_TOKEN environment variable is not set")
	}

	tag := os.Getenv("GITHUB_REF_NAME")
	if tag == "" {
		log.Fatal("GITHUB_REF_NAME environment variable is not set")
	}
	if !strings.HasPrefix(tag, "v") {
		log.Fatalf("Tag %s does not start with 'v'", tag)
	}

	version := strings.TrimPrefix(tag, "v")
	log.Printf("Starting release process for version: %s (tag: %s)", version, tag)

	client := github.NewClient(nil).WithAuthToken(token)
	ctx := context.Background()

	// 1. Artifact Verification & Hashing
	artifacts := []struct {
		Path     string
		BaseName string
	}{
		{Path: fmt.Sprintf("bin/Spice-%s-windows-amd64.exe", version)},
		{Path: fmt.Sprintf("bin/Spice-Setup-%s-windows-amd64.exe", version)},
		{Path: fmt.Sprintf("dist/Spice-%s-macos-arm64.dmg", version)},
		{Path: fmt.Sprintf("dist/Spice-Extension-%s-chrome.zip", version)},
		{Path: fmt.Sprintf("dist/Spice-Extension-%s-firefox.zip", version)},
	}

	hashes := make(map[string]string)
	var checksumData bytes.Buffer

	for i, a := range artifacts {
		artifacts[i].BaseName = filepath.Base(a.Path)
		if _, err := os.Stat(a.Path); os.IsNotExist(err) {
			log.Fatalf("Required artifact missing: %s", a.Path)
		}

		hash, err := hashFile(a.Path)
		if err != nil {
			log.Fatalf("Failed to hash %s: %v", a.Path, err)
		}
		hashes[artifacts[i].BaseName] = hash
		checksumData.WriteString(fmt.Sprintf("%s  %s\n", hash, artifacts[i].BaseName))
		log.Printf("Hashed %s -> %s", artifacts[i].BaseName, hash)
	}

	checksumBytes := checksumData.Bytes()
	err := os.WriteFile("checksums.txt", checksumBytes, 0600)
	if err != nil {
		log.Fatalf("Failed to write checksums.txt: %v", err)
	}
	log.Println("Successfully generated checksums.txt")

	dmgHash := hashes[fmt.Sprintf("Spice-%s-macos-arm64.dmg", version)]
	setupHash := hashes[fmt.Sprintf("Spice-Setup-%s-windows-amd64.exe", version)]

	// 2. GitHub Release Management
	release, resp, err := client.Repositories.GetReleaseByTag(ctx, repoOwner, repoName, tag)
	if err != nil && resp != nil && resp.StatusCode == 404 {
		log.Println("Release not found, creating a new release...")
		newRelease := &github.RepositoryRelease{
			TagName: github.String(tag),
			Name:    github.String("Spice " + tag),
			Body:    github.String("Automated release for " + tag),
			Draft:   github.Bool(false),
		}
		release, _, err = client.Repositories.CreateRelease(ctx, repoOwner, repoName, newRelease)
		if err != nil {
			log.Fatalf("Failed to create release: %v", err)
		}
	} else if err != nil {
		log.Fatalf("Failed to fetch release: %v", err)
	}

	// Upload artifacts + checksums
	uploadFiles := []string{"checksums.txt"}
	for _, a := range artifacts {
		uploadFiles = append(uploadFiles, a.Path)
	}

	for _, file := range uploadFiles {
		uploadAsset(ctx, client, release.GetID(), file)
	}

	// 3. Homebrew Cask Automation
	caskTmpl := `cask "spice" do
  arch arm: "arm64", intel: "x86_64"

  version "{{.Version}}"
  sha256 "{{.DMGHash}}"

  url "https://github.com/dixieflatline76/Spice/releases/download/v#{version}/Spice-#{version}-macos-#{arch}.dmg"
  name "Spice"
  desc "Highly-concurrent, plugin-driven desktop environment engine"
  homepage "https://spicebox.dev"

  auto_updates true

  app "Spice.app"
  app "Spice Wallpaper Manager Extension.app"

  zap trash: [
    "~/Library/Application Support/Spice",
    "~/Library/Preferences/com.dixieflatline76.spice.plist",
    "~/Library/Saved Application State/com.dixieflatline76.spice.savedState",
  ]
end
`
	updateRepoFile(ctx, client, homebrewTap, "Casks/spice.rb", caskTmpl, struct {
		Version string
		DMGHash string
	}{version, dmgHash}, fmt.Sprintf("Bump spice to %s", version))

	// 4. Winget Manifest Automation (Multi-File Format)
	// Microsoft winget-pkgs pipeline dropped support for singleton manifests.
	// We must now generate a 3-part multi-file manifest (Version, Installer, DefaultLocale).
	
	wingetVersionTmpl := `PackageIdentifier: dixieflatline76.Spice
PackageVersion: {{.Version}}
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.5.0
`

	wingetInstallerTmpl := `PackageIdentifier: dixieflatline76.Spice
PackageVersion: {{.Version}}
Installers:
  - Architecture: x64
    InstallerType: exe
    InstallerUrl: https://github.com/dixieflatline76/Spice/releases/download/v{{.Version}}/Spice-Setup-{{.Version}}-windows-amd64.exe
    InstallerSha256: {{.SetupHash}}
    UpgradeBehavior: install
    InstallerSwitches:
      Silent: /VERYSILENT /SUPPRESSMSGBOXES /NORESTART
      SilentWithProgress: /SILENT /SUPPRESSMSGBOXES /NORESTART
ManifestType: installer
ManifestVersion: 1.5.0
`

	wingetLocaleTmpl := `PackageIdentifier: dixieflatline76.Spice
PackageVersion: {{.Version}}
PackageLocale: en-US
Publisher: dixieflatline76
PackageName: Spice
License: PolyForm Noncommercial 1.0.0
ShortDescription: A highly-concurrent, plugin-driven desktop environment engine.
Moniker: spice
Tags:
  - wallpaper
  - desktop
  - customization
  - go
  - engine
ManifestType: defaultLocale
ManifestVersion: 1.5.0
`

	// Define paths
	baseManifestPath := fmt.Sprintf("manifests/d/dixieflatline76/Spice/%s", version)
	
	// Create the data payload for the templates
	wingetData := struct {
		Version   string
		SetupHash string
	}{version, strings.ToUpper(setupHash)}
	
	// Push all three files dynamically
	updateRepoFile(ctx, client, wingetRepo, fmt.Sprintf("%s/dixieflatline76.Spice.yaml", baseManifestPath), wingetVersionTmpl, wingetData, fmt.Sprintf("Bump spice (version manifest) to %s", version))
	updateRepoFile(ctx, client, wingetRepo, fmt.Sprintf("%s/dixieflatline76.Spice.installer.yaml", baseManifestPath), wingetInstallerTmpl, wingetData, fmt.Sprintf("Bump spice (installer manifest) to %s", version))
	updateRepoFile(ctx, client, wingetRepo, fmt.Sprintf("%s/dixieflatline76.Spice.locale.en-US.yaml", baseManifestPath), wingetLocaleTmpl, wingetData, fmt.Sprintf("Bump spice (locale manifest) to %s", version))

	log.Println("Release process completed successfully! 🚀")
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func uploadAsset(ctx context.Context, client *github.Client, releaseID int64, path string) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("Failed to open %s for upload: %v", path, err)
	}
	defer file.Close()

	baseName := filepath.Base(path)

	assets, _, err := client.Repositories.ListReleaseAssets(ctx, repoOwner, repoName, releaseID, nil)
	if err != nil {
		log.Fatalf("Failed to list assets: %v", err)
	}
	for _, asset := range assets {
		if asset.GetName() == baseName {
			log.Printf("Asset %s already exists, deleting...", baseName)
			_, err = client.Repositories.DeleteReleaseAsset(ctx, repoOwner, repoName, asset.GetID())
			if err != nil {
				log.Fatalf("Failed to delete existing asset %s: %v", baseName, err)
			}
		}
	}

	opts := &github.UploadOptions{Name: baseName}
	log.Printf("Uploading %s...", baseName)
	_, _, err = client.Repositories.UploadReleaseAsset(ctx, repoOwner, repoName, releaseID, opts, file)
	if err != nil {
		log.Fatalf("Failed to upload %s: %v", baseName, err)
	}
	log.Printf("Successfully uploaded %s", baseName)
}

func updateRepoFile(ctx context.Context, client *github.Client, repo, path, tmpl string, data interface{}, commitMsg string) {
	t, err := template.New("tmpl").Parse(tmpl)
	if err != nil {
		log.Fatalf("Failed to parse template for %s: %v", path, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		log.Fatalf("Failed to execute template for %s: %v", path, err)
	}
	content := buf.String()

	var sha *string
	fileContent, _, resp, err := client.Repositories.GetContents(ctx, repoOwner, repo, path, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			log.Printf("File %s does not exist in %s, will create.", path, repo)
		} else {
			log.Fatalf("Failed to get contents of %s in %s: %v", path, repo, err)
		}
	} else {
		decoded, _ := fileContent.GetContent()
		if decoded == content {
			log.Printf("File %s is already up to date.", path)
			return
		}
		sha = fileContent.SHA
	}

	// For Winget we should try to figure out default branch. It is usually "master" or "main"
	// Setting nil for Branch uses the default branch structure.
	opts := &github.RepositoryContentFileOptions{
		Message: github.String(commitMsg),
		Content: []byte(content),
		SHA:     sha,
	}

	log.Printf("Committing %s to %s...", path, repo)
	var createErr error
	if sha == nil {
		_, _, createErr = client.Repositories.CreateFile(ctx, repoOwner, repo, path, opts)
	} else {
		_, _, createErr = client.Repositories.UpdateFile(ctx, repoOwner, repo, path, opts)
	}

	if createErr != nil {
		log.Fatalf("Failed to commit %s: %v", path, createErr)
	}
	log.Printf("Successfully committed %s to %s", path, repo)
}
