# Release Process & Distribution

This document outlines the steps required to release a new version of Spice and update the official distribution manifests.

## 1. Version Bumping

Use the included Makefile targets to bump the version:

```bash
make bump-patch  # v2.2.5 -> v2.2.6
make bump-minor  # v2.2.5 -> v2.3.0
make bump-major  # v2.2.5 -> v3.0.0
```

## 2. Generating Release Artifacts

Pushing a new tag triggers the CI pipeline (GitHub Actions) to build the following:
*   `Spice-vX.Y.Z-macos-arm64.dmg` (macOS Cask)
*   `Spice-Setup-X.Y.Z-windows-amd64.exe` (Windows Installer)
*   `Spice.pkg` (App Store)

## 3. Updating Distribution Manifests

Once artifacts are available on the GitHub Releases page, update the manifests with the new version and file hashes.

### Homebrew (Cask)
1.  Navigate to `Casks/spice.rb`.
2.  Update the `version` field.
3.  Calculate the SHA256 of the new DMG:
    ```bash
    curl -L -O https://github.com/dixieflatline76/Spice/releases/download/vX.Y.Z/Spice-X.Y.Z-macos-arm64.dmg
    shasum -a 256 Spice-X.Y.Z-macos-arm64.dmg
    ```
4.  Update the `sha256` field.
5.  Submit to [homebrew-cask](https://github.com/Homebrew/homebrew-cask).

### Winget
1.  Ensure you have the [Winget Create](https://github.com/microsoft/winget-create) tool installed.
2.  Update and submit the manifest in a single command:
    ```bash
    wingetcreate update DixieFlatline76.Spice -v X.Y.Z -u https://github.com/dixieflatline76/Spice/releases/download/vX.Y.Z/Spice-Setup-X.Y.Z-windows-amd64.exe
    ```

## 4. App Store Submission

Follow the standard Apple and Microsoft Store submission workflows using the artifacts generated in the `dist/` directory.
