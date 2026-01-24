# Spice Internal Developer Context & Architecture Guide

> **Purpose**: This document serves as the authoritative "Mental Model" of the Spice codebase. It contains deep-dive technical details required to understand the system's constraints, concurrency model, and extension patterns. **Read this before making changes.**

## 1. Core Architecture: The Single-Writer, Multiple-Reader (SWMR) Model

Spice is designed to remain responsive (60fps UI) even while downloading 50MB 8K images or performing CPU-intensive face detection.

### 1.1 The Golden Rule of Concurrency
**Only ONE goroutine is allowed to write to the global state.**

*   **The Store (`pkg/wallpaper/store.go`)**: The source of truth.
    *   **Data Structure**: Uses `[]provider.Image` for sequential access (history) and `map[string]bool` (`idSet`) for O(1) lookups.
    *   **Locking**:
        *   `RLock()`: Used by the UI (Reader) for instant "Next/Prev" actions.
        *   `Lock()`: Used **ONLY** by the Pipeline's StateManager.
    *   **Persistence**: Uses a "Debounced Save" mechanism. Calling `scheduleSaveLocked()` starts a timer (2s). If called again, the timer resets. This batches 100+ rapid updates (bulk imports) into a single JSON write.

*   **The Pipeline (`pkg/wallpaper/pipeline.go`)**: The workhorse.
    *   **Worker Pool**: N goroutines (default: NumCPU) that fetch and process images. They communicate results via `resultChan`.
    *   **State Manager Loop**: The **sole writer**. It `select`s on:
        1.  `resultChan`: New images from workers -> Calls `store.Add()`.
        2.  `cmdChan`: Commands from UI (`CmdMarkSeen`, `CmdRemove`) -> Calls `store.MarkSeen()`.
    *   **Yielding**: The loop calls `runtime.Gosched()` after every operation to ensure the UI reader thread is never starved during heavy batch processing.

### 1.2 pagination & Anti-Thrashing Logic
In `pkg/wallpaper/wallpaper.go`:
*   **The Trigger**: `applyWallpaper` checks `seenCount / totalCount`. If > 70%, it triggers a fetch.
*   **Protection**:
    *   **Atomic Flag**: `fetchingInProgress` (Atomic Bool) acts as a mutex for the *network trigger*, preventing 1000 clicks from spawning 1000 fetch threads.
    *   **Starvation Check**: If the provider returns 0 images (dry source), the system compares `currentTotal` vs `lastTriggerTotal`. If they haven't changed, it enforces a 60s cooldown to prevent API bans.

## 2. The Settings UI: A Minefield of Closures

The Settings UI (`pkg/ui/setting`) uses a **Deferred Save / Git-like Commit** model. Changes are staged, not saved, until "Apply" is clicked.

### 2.1 The "Closure Trap" (Common Bug)
When creating dynamic UI lists (like `CreateQueryPanel`), **NEVER** compare the new value against a variable captured at widget creation time.

#### ❌ The Buggy Way (Stale State)
```go
// check.Checked is set to 'true' initially
check.OnChanged = func(nowActive bool) {
   // BUG: This 'true' is captured forever.
   // If user clicks OFF -> Apply -> ON ... 'nowActive' is TRUE, 'capturedInitial' is TRUE.
   // The logic thinks nothing changed!
   if nowActive != capturedInitial {
       sm.SetRefreshFlag(...) 
   }
}
```

#### ✅ The Correct Way (Live State)
```go
check.OnChanged = func(nowActive bool) {
   // 1. Always look up the CURRENT configuration source of truth
   liveState := config.GetImageQuery(id).Active
   
   // 2. Define unique dirty keys
   dirtyKey := fmt.Sprintf("provider_query_%s", id)
   callbackKey := fmt.Sprintf("cb_%s", id)

   if nowActive != liveState {
       // 3. Queue the Save Action (Closure runs ONLY on Apply)
       sm.SetSettingChangedCallback(callbackKey, func() {
           config.SetQueryActive(id, nowActive)
       })
       // 4. Mark Dirty
       sm.SetRefreshFlag(dirtyKey)
   } else {
       // 5. Reverted? Clean up
       sm.RemoveSettingChangedCallback(callbackKey)
       sm.UnsetRefreshFlag(dirtyKey)
   }
   
   // 6. Update the "Apply" button state
   sm.GetCheckAndEnableApplyFunc()()
}
```

## 3. Deep Dive: Provider Implementations

Key logic patterns for the major providers in `pkg/wallpaper/providers/`.

### 3.1 Wallhaven (`wallhaven.go`)
*   **Regex Router**: It parses user-pasted URLs using rigorous Regex:
    *   `UserFavoritesRegex`: `wallhaven.cc/user/([^/]+)/favorites/(\d+)` -> Converts to API Collection Endpoint.
    *   `SearchRegex`: Extracts `?q=...` or `?categories=...`.
*   **API Key Hygiene**: The `ParseURL` function explicitly *strips* `apikey` params from saved URLs to preventing leaking keys in shared configs. Keys are injected *only* at request time from the secure config.

### 3.2 The Met Museum (`metmuseum.go`)
*   **The Template**: Uses `wallpaper.CreateMuseumHeader` for a standardized "Institution" look.
*   **Director's Cut Configuration**: Unlike generic search providers, The Met uses a **Fixed List** of curated `const` queries (e.g., "Arts of Asia"). These are rendered as `widget.NewCheck` logic groups, not a dynamic `widget.List`.
*   **Parallel Fetching**: Uses `errgroup` with a concurrency limit (5) to "scan" for valid images.
*   **Filtering**: Implementation strictly filters images:
    *   **Aspect Ratio**: Must be > 1.2 (Strict Landscape). Rejects portraits and squares.
    *   **Quality**: Must have high-res `primaryImage`.

### 3.3 Favorites (`favorites.go`)
*   **Local Bridge**: This provider is unique—it bridges the `chrome-extension://` world with the local filesystem.
*   **Worker Pattern**: File IO (copying 5MB images) is too slow for the main thread.
    *   It uses a `jobChan favJob` channel.
    *   `runWorker` loop consumes jobs to `Add` or `Remove` files from `/tmp/spice/favorites`.
*   **FIFO Garbage Collection**: It enforces a hard limit (MaxFavoritesLimit). When adding, if full, it finds the oldest file (by `ModTime`) and deletes it before writing the new one.
*   **Metadata Sidecar**: It maintains `metadata.json` in the same folder to store Attribution and Product URL, which cannot be stored in the image file itself reliably.

## 4. Extension Guide

### 4.1 Adding a New Provider
1.  **Create Package**: `pkg/wallpaper/providers/myprovider`.
2.  **Implement Interface**: `provider.ImageProvider`.
3.  **Register**:
    ```go
    func init() {
        wallpaper.RegisterProvider("MyProvider", func(cfg *Config, client *client) ImageProvider {
            return NewProvider(cfg, client)
        })
    }
    ```
4.  **Import**: You don't need to manually import it. Run `go generate ./...` (or `make gen`). The `gen_providers` tool will automatically discover your package and add it to `cmd/spice/zz_generated_providers.go`. To disable a provider, simply add a `.disabled` file to its directory.

### 4.2 UI Standardization
Use the helpers in `pkg/wallpaper/ui_add_query.go` for consistency:
*   `CreateAddQueryButton(...)` -> Handles the popup logic.
*   `AddQueryConfig` struct -> Define your Regex validators here.

## 5. Critical Files Map
*   **`pkg/wallpaper/store.go`**: The DB. Thread-safety critical.
*   **`pkg/wallpaper/pipeline.go`**: The Async Engine.
*   **`pkg/wallpaper/smart_image_processor.go`**: Face Detection & Cropping logic.
*   **`pkg/ui/setting/setting_manager.go`**: The "Apply" logic controller.
*   **`pkg/api/server.go`**: Local HTTP server for Browser Extension communication.

## 6. Smart Fit 2.0 & Imaging Logic

Spice's imaging engine (`pkg/wallpaper/smart_image_processor.go`) is more than just a cropper. It implements a decision tree to balance "Artistic Integrity" vs "Screen Filling".

### 6.1 The Logic Tree
1.  **Strict Resolution Floor**: Reject anything smaller than desktop dimensions.
2.  **Face Detection (Pigo)**:
    *   **Clustering**: Uses IoU (Intersection over Union) to merge overlapping detections.
    *   **Edge Safety**: Ignores low-confidence faces in the bottom 30% of the image (often noise/patterns).
    *   **Rescue**: In "Quality Mode", an image that fails the aspect ratio check (too wide/tall) can be *Rescued* if a high-confidence face is found.
3.  **Mode Switching**:
    *   **Quality Mode**: Enforces `AspectThreshold` (0.9). Rejects unless Rescued.
    *   **Flexibility Mode**: Calculates a `DynamicThreshold` based on resolution surplus. If you have 8K pixels for a 1080p screen, we allow more aggressive cropping.
4.  **The "Feet Guard" (Holistic Safety)**:
    *   If `smartcrop` suggests a crop starting very low (cutting heads?), we force a Center Crop.
    *   **Energy-Aware**: This threshold relaxes for "High Energy" (busy/detailed) images but stays strict for "Low Energy" (sky/ground) images.

### 6.2 Externalized Tuning (`pkg/wallpaper/tuning.go`)
All magic numbers are extracted into `TuningConfig`.
*   `AggressiveMultiplier`: Controls how much "extra" resolution allows for "extra" cropping.
*   `FaceRescueQThreshold`: Confidence score required to override aspect ratio bans.
*   **Goal**: These values are structured to be hot-swapped via a remote JSON config in the future.

## 7. The "Live Update" Architecture (Museums)

Providers like MetMuseum (`pkg/wallpaper/providers/metmuseum`) use a **4-Layer Fallback** system to allow content updates without app updates.

### 7.1 The Fallback Chain
`InitRemoteCollection()` logic:
1.  **Remote Fetch**: HTTP GET `raw.githubusercontent.com/.../met.json`. (Timeout: 3s).
2.  **Local Cache**: If remote fails, load `~/.config/spice/cache/met/met_cache.json`.
3.  **Embed**: If cache missing/corrupt, load `//go:embed met.json` (baked at build time).
4.  **Hardcoded**: If all hell breaks loose, load `SpiceMelangeIDs` (Go `const` slice).

### 7.2 Usage
This allows curators to update the "Director's Cut" collections (IDs, Descriptions) by simply committing to the GitHub repo. Spice clients enforce the new catalog on their next restart.

## 8. Development Environment & Secrets

When developing locally, especially when running the app directly via `go run` instead of `make`, you must manually inject API secrets into your environment.

### 8.1 The `.spice_secrets` File
Create a file named `.spice_secrets` in the project root. This file is git-ignored.
Format: `KEY=VALUE`

```bash
UNSPLASH_CLIENT_ID=abc...
GOOGLE_PHOTOS_CLIENT_ID=xyz...
```

### 8.2 The `load_secrets` Helper
The project includes helper scripts (`load_secrets.ps1` for PowerShell, `load_secrets.sh` for Bash) to read this file and export variables to your current shell session.

**Usage (PowerShell):**
```powershell
. .\load_secrets.ps1  # Note the dot-source syntax!
go run cmd/spice/main.go
```

**Usage (Bash):**
```bash
source ./load_secrets.sh
go run cmd/spice/main.go
```

**Note**: The `Makefile` automatically handles this injection via `cmd/util/load_secrets/main.go`, so you only need to manually source this script when bypassing the Makefile.

