# Spice Internal Developer Context & Architecture Guide

> **Purpose**: This document serves as the authoritative "Mental Model" of the Spice codebase. It contains deep-dive technical details required to understand the system's constraints, concurrency model, and extension patterns. **Read this before making changes.**

## 1. Core Architecture: Hybrid Concurrency Model

Spice is designed to remain responsive (60fps UI) even while downloading 50MB 8K images or performing CPU-intensive face detection.

### 1.1 The Concurrency Model
**The hot path (image ingestion) is serialized through one goroutine. Admin operations use direct mutex access.**

*   **The Store (`pkg/wallpaper/store.go`)**: The source of truth.
    *   **Data Structure**: Uses `[]provider.Image` for sequential access (history) and `map[string]bool` (`idSet`) for O(1) lookups.
    *   **Locking**:
        *   `RLock()`: Used by the UI (Reader) for instant "Next/Prev" actions.
        *   `Lock()`: Used by the Pipeline's StateManager (hot path) and the Plugin (admin operations).
    *   **Persistence**: Uses a "Debounced Save" mechanism. Calling `scheduleSaveLocked()` starts a timer (2s). If called again, the timer resets. This batches 100+ rapid updates (bulk imports) into a single JSON write.
    *   **Synchronization Model**: The `Sync` method uses a **Policy Pattern** via `ImageSyncAction` (Keep, Delete, Invalidate). This ensures deterministic state transitions for every image based on active queries, avoid sets, and file availability, replacing complex ad-hoc logic.

*   **The Pipeline (`pkg/wallpaper/pipeline.go`)**: The workhorse.
    *   **Worker Pool**: N goroutines (default: NumCPU) that fetch and process images. They communicate results via `resultChan`.
    *   **State Manager Loop**: The serialized writer for the **hot path**. It `select`s on:
        1.  `resultChan`: New images from workers -> Calls `store.Add()`.
        2.  `cmdChan`: Commands from UI (`CmdMarkSeen`, `CmdRemove`) -> Calls `store.MarkSeen()`.
    *   **Yielding**: The loop calls `runtime.Gosched()` after every operation to ensure the UI reader thread is never starved during heavy batch processing.

*   **Admin Operations (Plugin / `wallpaper.go`)**: Infrequent, user-initiated mutations.
    *   `ToggleFavorite`: Calls `store.Remove()` or `store.Update()` directly.
    *   `reconcileFavorites`: Calls `store.Remove()` / `store.Update()` at startup.
    *   `onQueryRemoved` / `onQueryDisabled`: Calls `store.RemoveByQueryID()`.
    *   `ClearCache`: Calls `store.Wipe()`.
    *   These are all protected by the store's `sync.RWMutex` and do not contend with the hot path in practice.

### 1.2 pagination & Anti-Thrashing Logic
In `pkg/wallpaper/wallpaper.go`:
*   **The Trigger**: `applyWallpaper` checks `seenCount / totalCount`. If > 70%, it triggers a fetch.
*   **Protection**:
    *   **Atomic Flag**: `fetchingInProgress` (Atomic Bool) acts as a mutex for the *network trigger*, preventing 1000 clicks from spawning 1000 fetch threads.
    *   **Starvation Check**: If the provider returns 0 images (dry source), the system compares `currentTotal` vs `lastTriggerTotal`. If they haven't changed, it enforces a 60s cooldown to prevent API bans.

### 2.1 The Registry Pattern (v2.5)
 
The Settings UI (`pkg/ui/setting`) uses a **Deferred Save / Git-like Commit** model. Changes are staged, not saved, until "Apply" is clicked.

#### ❌ The Legacy "Closure Trap" (Resolved)
Previously, dynamic UI lists often captured stale state in closures, leading to "Apply" actions that did nothing.

#### ✅ The Registry Solution
`SettingsManager` now maintains a **Registry** (`map[string]interface{}`) of the last-known "Baseline" values.
1. **Baseline Seeding**: When a widget is created, its initial persisted value is mirrored into the registry.
2. **Live Comparison**: `OnChanged` handlers now compare the widget's current state directly against the registry's baseline.
3. **Advanced Props**:
   - `IsPassword`: Setting this to `true` on a `TextEntrySettingConfig` masks the UI and enables secure keychain storage.
   - `EnabledIf`: A dynamic function that determines if a widget is interactive (e.g., locking an API key field until it's cleared).
4. **Programmatic Updates**: `SetValue(name, val)` allows for cross-widget state updates (e.g., the "Clear" button resetting the API key text field).
5. **Atomic Commit**: On "Apply", the engine executes all queued changes and then synchronously promotes all "Live" values to "Baseline".

#### 2.2 The Transactional UI Pattern (v2.6)

For sensitive credentials (API Keys, Usernames), Spice bypasses the strictly deferred model in favor of **Intentional Transactions**.

*   **Rationale**: We never want to send an API key silently in the background while the user is typing (Auto-Validation), nor do we want to save a potentially invalid key that could break the background worker.
*   **The Flow**:
    1.  **Direct Interaction**: The user enters a key. The "Clear" button transforms into **"Verify & Connect"**.
    2.  **Explicit Action**: Verification only occurs when the button is clicked.
    3.  **Immediate Persistence**: If verification succeeds, the key is saved to the config AND the baseline is seeded **immediately**. The field locks (via `EnabledIf`) and the button becomes **"Clear API Key"**.
    4.  **Hanging Prevention**: All verification transactions MUST include a timeout (standard: 10s) via `context.WithTimeout` to prevent UI "stuckness".
*   **Implementation**: This is handled manually in the provider's `CreateSettingsPanel` by wiring the action button to `p.cfg.SetKey()`, `sm.SeedBaseline()`, and `sm.Refresh()`.

## 3. Deep Dive: Provider Implementations

Key logic patterns for the major providers in `pkg/wallpaper/providers/`.

### 3.1 Wallhaven (`wallhaven.go`)
*   **Regex Router**: It parses user-pasted URLs using rigorous Regex:
    *   `UserFavoritesRegex`: `wallhaven.cc/user/([^/]+)/favorites/(\d+)` -> Converts to API Collection Endpoint.
    *   `SearchRegex`: Extracts `?q=...` or `?categories=...`.
*   **API Key Hygiene**: The `ParseURL` function explicitly *strips* `apikey` params from saved URLs to prevent leaking keys in shared configs. Keys are stored in the secure OS keychain and are **masked** in the UI via the `IsPassword` property.

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

### 6.1 The Strategy Pattern (Refactored)
The decision tree is now implemented via the **Strategy Pattern** (`CropStrategy` interface).

1.  **Analysis Phase**: The processor first calculates **Entropy** (Energy) and scans for **Faces**.
2.  **Strategy Selection**:
    *   **`FaceCropStrategy`**: Selected if a face is found and Face Crop enabled.
    *   **`SmartPanStrategy`**: Selected for "Face Boost" or as a fallback for low-energy images in Flexibility Mode.
    *   **`EntropyCropStrategy`**: The default smart cropper. Contains the **Feet Guard**.
3.  **The "Feet Guard"**:
    *   If `smartcrop` suggests a crop starting very low (cutting heads?), we force a Center Crop.
    *   **Energy-Aware**: This threshold relaxes for "High Energy" (busy/detailed) and stays strict for "Low Energy" (sky/ground).

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

## 9. Internationalization & Localization (i18n)

Spice uses a custom i18n package (`pkg/i18n`) for runtime translations and synchronization with Fyne's internal state.

### 9.1 Translation Architecture
- **Language Selection**: Managed via `i18n.SetLanguage(code)`. It maps standard codes (e.g., "en", "de", "zh-Hant") to internal states and synchronizes with Fyne's `lang` package.
- **String Retrieval**:
    - `i18n.T("key")`: Standard translation retrieval.
    - `i18n.Tf("key", map)`: Templated translation using Go `text/template` syntax.
- **Embedded Files**: Translations are stored in `pkg/i18n/translations/*.json` and embedded into the binary using `//go:embed`.

### 9.2 Contributing New Translations
1.  **Reference English**: Use `pkg/i18n/translations/en.json` as the authoritative source for keys.
2.  **Create JSON**: Add `[lang-code].json` to the translations directory.
3.  **Register Language**: 
    - Add the language code and its display name to the `switch` statement in `pkg/i18n/i18n.go:SetLanguage`.
    - Update the `Language` dropdown in `ui/ui.go` within the Preferences window construction.
4.  **Verify**: Run `make test` to ensure no JSON parsing errors or missing key panics.

### 9.3 Tray Menu Constraints (Critical)
When translating strings specifically for the system tray menu:
- **Brevity**: Operating systems (especially Windows) calculate tray menu width based on the longest item. Keep translations as short as possible (e.g., German "Bild" vs "Hintergrundbild").
- **Mnemonic Safety**: On Windows, `&` acts as a mnemonic prefix and is hidden. Use `+` as a universal cross-platform separator instead of `&` or `&&`.
- **Sanitization**: All dynamic strings (like attributions) displayed in the tray MUST pass through `SanitizeMenuString` (in `pkg/wallpaper/helper.go`) to strip HTML and collapse excessive whitespace.
- **Rune-Aware Truncation**: Never truncate tray labels by bytes. Always cast to `[]rune` before slicing to avoid corrupting multi-byte localized characters.

