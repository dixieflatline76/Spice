# Release Process & Distribution

This document outlines the steps required to release a new version of Spice and update the official distribution manifests.

## 1. Version Bumping

Use the included Makefile targets to bump the version:

```bash
make bump-patch  # v2.3.0 -> v2.3.1
make bump-minor  # v2.3.0 -> v2.4.0
make bump-major  # v2.3.0 -> v3.0.0
```

This updates `version.txt`, commits, and creates a Git tag.

## 2. Creating a GitHub Release

1. Push the tag: `git push origin vX.Y.Z`
2. Go to **GitHub → Releases → Create a new release**.
3. Select the tag, write release notes, and publish.

Publishing the release triggers the CI pipeline (GitHub Actions) which builds:
*   `Spice-vX.Y.Z-macos-arm64.dmg` (macOS Cask)
*   `Spice-Setup-X.Y.Z-windows-amd64.exe` (Windows Installer via Inno Setup, code-signed via Azure Trusted Signing)
*   `Spice.msix` (Windows MSIX package)
*   Browser extensions (Chrome/Firefox `.zip` archives)

## 3. Updating Distribution Manifests

### Winget (Automated)

The CI pipeline includes a `spice_releaser` step that automatically:
1. Syncs your `dixieflatline76/winget-pkgs` fork with `microsoft/winget-pkgs`.
2. Generates the 3 manifest files (version, installer, locale) for the new release.
3. Pushes them to your fork's `master` branch.

**After CI completes:**
1. Go to your fork at `github.com/dixieflatline76/winget-pkgs`.
2. Click **"Contribute" → "Open pull request"** to `microsoft/winget-pkgs`.
3. Verify the diff shows only the 3 new files under `manifests/d/dixieflatline76/Spice/X.Y.Z/`.
4. Fill out the checklist template and submit.
5. Wait for the `wingetbot` validation pipeline to pass and a maintainer to approve.

### Homebrew (Cask)

1. Calculate the SHA256 of the new DMG:
    ```bash
    curl -L -O https://github.com/dixieflatline76/Spice/releases/download/vX.Y.Z/Spice-X.Y.Z-macos-arm64.dmg
    shasum -a 256 Spice-X.Y.Z-macos-arm64.dmg
    ```
2. Update the `version` and `sha256` fields in `Casks/spice.rb`.
3. Submit a PR to [homebrew-cask](https://github.com/Homebrew/homebrew-cask).

## 4. App Store Submission

Follow the standard Apple and Microsoft Store submission workflows using the artifacts generated in the `dist/` directory.

## 5. Post-Release Checklist

- [ ] GitHub Release published with release notes
- [ ] Winget PR submitted and passing validation
- [ ] Homebrew Cask PR submitted (if macOS release)
- [ ] Delete any superseded buggy releases from GitHub (if applicable)
- [ ] Update `docs/` version references if architecture changed
