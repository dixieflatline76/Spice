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

	// Check if we should skip package manager distribution (prerelease builds)
	if strings.EqualFold(os.Getenv("SKIP_DISTRIBUTION"), "true") {
		log.Println("SKIP_DISTRIBUTION=true — skipping Homebrew and Winget updates. Assets uploaded successfully! 🚀")
		return
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
	// Uses Git Trees API for a single atomic commit on a feature branch.
	// This keeps the fork's master clean and produces 1-commit PRs.

	log.Printf("Syncing %s/%s with upstream...", repoOwner, wingetRepo)
	repoObj, _, err := client.Repositories.Get(ctx, repoOwner, wingetRepo)
	if err != nil {
		log.Fatalf("Failed to get repo details for %s/%s: %v", repoOwner, wingetRepo, err)
	}
	defaultBranch := repoObj.GetDefaultBranch()
	if defaultBranch == "" {
		defaultBranch = "master"
	}

	_, _, syncErr := client.Repositories.MergeUpstream(ctx, repoOwner, wingetRepo, &github.RepoMergeUpstreamRequest{
		Branch: github.String(defaultBranch),
	})
	if syncErr != nil {
		log.Printf("Note: Syncing fork returned: %v (usually fine if already up to date)", syncErr)
	} else {
		log.Printf("Successfully synced fork %s branch with upstream.", defaultBranch)
	}

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

	baseManifestPath := fmt.Sprintf("manifests/d/dixieflatline76/Spice/%s", version)

	wingetData := struct {
		Version   string
		SetupHash string
	}{version, strings.ToUpper(setupHash)}

	// Render all 3 manifest templates
	wingetFiles := []struct {
		Path     string
		Template string
	}{
		{fmt.Sprintf("%s/dixieflatline76.Spice.yaml", baseManifestPath), wingetVersionTmpl},
		{fmt.Sprintf("%s/dixieflatline76.Spice.installer.yaml", baseManifestPath), wingetInstallerTmpl},
		{fmt.Sprintf("%s/dixieflatline76.Spice.locale.en-US.yaml", baseManifestPath), wingetLocaleTmpl},
	}

	pushWingetBranch(ctx, client, defaultBranch, version, wingetData, wingetFiles)

	log.Println("Release process completed successfully! 🚀")
}

// pushWingetBranch creates a feature branch on the winget-pkgs fork and pushes
// all manifest files as a single atomic commit using the Git Trees API.
// This produces clean 1-commit PRs and keeps the fork's default branch pristine.
func pushWingetBranch(ctx context.Context, client *github.Client, baseBranch, version string, data interface{}, files []struct {
	Path     string
	Template string
}) {
	branchName := fmt.Sprintf("spice-v%s", version)
	commitMsg := fmt.Sprintf("New version: dixieflatline76.Spice version %s", version)

	// 1. Get the SHA of the base branch HEAD
	baseRef, _, err := client.Git.GetRef(ctx, repoOwner, wingetRepo, "refs/heads/"+baseBranch)
	if err != nil {
		log.Fatalf("Failed to get ref for %s: %v", baseBranch, err)
	}
	baseSHA := baseRef.Object.GetSHA()
	log.Printf("Base branch %s is at %s", baseBranch, baseSHA[:12])

	// 2. Delete the feature branch if it already exists (idempotent reruns)
	_, resp, _ := client.Git.GetRef(ctx, repoOwner, wingetRepo, "refs/heads/"+branchName)
	if resp != nil && resp.StatusCode == 200 {
		log.Printf("Branch %s already exists, deleting for clean rerun...", branchName)
		_, delErr := client.Git.DeleteRef(ctx, repoOwner, wingetRepo, "refs/heads/"+branchName)
		if delErr != nil {
			log.Fatalf("Failed to delete existing branch %s: %v", branchName, delErr)
		}
	}

	// 3. Create the feature branch from base
	newRef := &github.Reference{
		Ref:    github.String("refs/heads/" + branchName),
		Object: &github.GitObject{SHA: github.String(baseSHA)},
	}
	_, _, err = client.Git.CreateRef(ctx, repoOwner, wingetRepo, newRef)
	if err != nil {
		log.Fatalf("Failed to create branch %s: %v", branchName, err)
	}
	log.Printf("Created feature branch: %s", branchName)

	// 4. Build tree entries for all manifest files
	var treeEntries []*github.TreeEntry
	for _, f := range files {
		t, err := template.New("tmpl").Parse(f.Template)
		if err != nil {
			log.Fatalf("Failed to parse template for %s: %v", f.Path, err)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, data); err != nil {
			log.Fatalf("Failed to execute template for %s: %v", f.Path, err)
		}
		content := buf.String()

		treeEntries = append(treeEntries, &github.TreeEntry{
			Path:    github.String(f.Path),
			Mode:    github.String("100644"),
			Type:    github.String("blob"),
			Content: github.String(content),
		})
		log.Printf("Prepared manifest: %s", f.Path)
	}

	// 5. Create a new tree with the manifest files layered on the base
	tree, _, err := client.Git.CreateTree(ctx, repoOwner, wingetRepo, baseSHA, treeEntries)
	if err != nil {
		log.Fatalf("Failed to create tree: %v", err)
	}

	// 6. Create a single commit pointing to the new tree
	commit := &github.Commit{
		Message: github.String(commitMsg),
		Tree:    tree,
		Parents: []*github.Commit{{SHA: github.String(baseSHA)}},
	}
	newCommit, _, err := client.Git.CreateCommit(ctx, repoOwner, wingetRepo, commit, nil)
	if err != nil {
		log.Fatalf("Failed to create commit: %v", err)
	}
	log.Printf("Created atomic commit: %s", newCommit.GetSHA()[:12])

	// 7. Update the feature branch ref to point to the new commit
	branchRef := &github.Reference{
		Ref:    github.String("refs/heads/" + branchName),
		Object: &github.GitObject{SHA: newCommit.SHA},
	}
	_, _, err = client.Git.UpdateRef(ctx, repoOwner, wingetRepo, branchRef, true)
	if err != nil {
		log.Fatalf("Failed to update branch %s: %v", branchName, err)
	}

	log.Printf("✅ Winget manifests pushed to branch '%s' as a single commit.", branchName)
	log.Printf("   → Open a PR from: https://github.com/%s/%s/compare/master...%s:%s", "microsoft", wingetRepo, repoOwner, branchName)
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

// updateRepoFile commits a single rendered template to a repo using the Contents API.
// Used for Homebrew cask updates where a single-file commit is appropriate.
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
