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
    *   **Dispatcher (Fair Bouncer)**: A dedicated goroutine that holds jobs and trickles them into the worker pool based on strict provider API limits.
    *   **Worker Pool**: 16 generic goroutines that fetch and process images immediately upon receiving a job. They communicate results via `resultChan`.
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

### 1.2 Pagination & Anti-Thrashing Logic
In `pkg/wallpaper/wallpaper.go`:
*   **The Trigger**: `applyWallpaper` checks `seenCount / totalCount`. If > 70%, it triggers a fetch.
*   **Protection**:
    *   **Atomic Flag**: `fetchingInProgress` (Atomic Bool) acts as a mutex for the *network trigger*, preventing 1000 clicks from spawning 1000 fetch threads.
    *   **Starvation Check**: If the provider returns 0 images (dry source), the system compares `currentTotal` vs `lastTriggerTotal`. If they haven't changed, it enforces a 60s cooldown to prevent API bans.

### 1.3 Persistent Architectural Constraints (Golden Rules)
⚠️ **Never remove or "refactor away" these mechanisms. They solve recurring production bugs.**

1.  **Safe Page Wrapping (fetch_logic.go)**: 
    *   **Rule**: When `FetchImages` returns 0 results for a `page > 1`, ALWAYS reset the query to `Page 1`.
    *   **Rationale**: Most providers (Wallhaven, Wikimedia) have finite pages. Without wrapping, the wallpaper flow dies permanently once the last page is reached.
    *   **Restriction**: Do NOT wrap if `err != nil`. This distinguishes "EndOfResults" from "NetworkError".
2.  **Resolution Probing & Persistence (downloader.go)**:
    *   **Rule**: Once an image is opened (or its header decoded), the discovered `Width` and `Height` MUST be saved back to the `Store`.
    *   **Rationale**: Fixes the "Ghost Dimension" ($0 \times 0$) bug where lack of persistence forces expensive full-file decodes on every refresh.
3.  **Deadlock-Free Deduplication (fetch_logic.go)**:
    *   **Rule**: The fetcher MUST NOT skip an image just because it exists in the store. It must also check if the image is missing a derivative for the *current* monitors.
    *   **Rationale**: Essential for "Backlog Healing." If a user adds a new monitor, the system must be able to "pull" existing cached originals into the processing pipeline.

### 2.1 Schema-Driven UI Architecture (Hexagonal / Ports & Adapters)

Spice implements a **Hexagonal Architecture** for its settings UI. Providers never import or create Fyne widgets directly. Instead, they declare their UI needs using pure Go structs, and a central rendering engine translates those structs into the actual framework widgets.

#### The Boundary

```
┌─────────────────────────────────────────────────────────────────┐
│  INNER RING (Pure Go — zero framework imports)                  │
│                                                                 │
│  pkg/provider/provider.go     → Domain interfaces               │
│  pkg/ui/schema/schema.go      → PORT: UI contract (ItemSchema)  │
│  pkg/ui/schema/museum.go      → Schema helpers (museum template)│
│  pkg/ui/setting/setting_mgr   → PORT: SettingsManager interface │
│  pkg/wallpaper/providers/*    → Domain logic (100% Fyne-free)   │
└─────────────────────────────────────────────────────────────────┘
                              ▲
                              │  Returns *schema.PanelSchema
                              │
┌─────────────────────────────────────────────────────────────────┐
│  OUTER RING (Framework-coupled — Fyne)                          │
│                                                                 │
│  ui/settings_manager.go       → ADAPTER: RenderSchema()         │
│  ui/ui.go                     → Application shell & tray        │
└─────────────────────────────────────────────────────────────────┘
```

**The key rule**: The dependency arrow always flows **inward**. Providers depend on `schema` and `setting` (ports), never on `ui/` (adapter). The engine (`ui/settings_manager.go`) depends on `schema` to know what to render, but providers never know about Fyne.

#### Schema Types (`pkg/ui/schema/schema.go`)

All provider UI is composed from these building blocks:

| Schema Type | Purpose |
|:---|:---|
| `BoolItem` | Checkbox / toggle with label, help, `ApplyFunc`, `EnabledIf`, `VisibleIf` |
| `TextItem` | Text entry with validation, debounce, `PostValidateCheck`, password masking |
| `SelectItem` | Dropdown with index-based value tracking |
| `SecretItem` | API key / credential with `OnVerify` and `OnClear` (Transactional pattern) |
| `ButtonItem` | Simple action button |
| `AsyncButtonItem` | Button with loading state and background task |
| `ConfirmButtonItem` | Button with confirmation dialog |
| `HyperlinkItem` | Clickable URL |
| `LabelItem` | Static text or description |
| `QueryListItem` | Full query management list (toggle, delete, display) |
| `OAuthPickerItem` | OAuth + Picker workflow (Google Photos) |
| `FolderPickerItem` | Native directory selector (Local Folders) |
| `HorizontalRowItem` | Groups items side-by-side |

#### The `RenderSchema()` Adapter

`SettingsManager.RenderSchema(p schema.PanelSchema)` is the **single entry point** that walks the schema tree and materializes Fyne widgets. For each schema item it:

1. Creates the native widget (e.g., `widget.NewCheck` for `BoolItem`)
2. Seeds the **Baseline** in the registry
3. Wires `OnChanged` → dirty detection → `ApplyFunc` queuing
4. Registers `EnabledIf` / `VisibleIf` for reactive state management
5. Returns a composed `fyne.CanvasObject`

Providers never call these render methods — they only build and return `*schema.PanelSchema`.

### 2.2 The Registry Pattern (Deferred Save)

The Settings UI uses a **Deferred Save / Git-like Commit** model. Changes are staged, not saved, until "Apply" is clicked.

`SettingsManager` maintains a **Registry** (`map[string]interface{}`) of the last-known "Baseline" values.

1. **Baseline Seeding**: When `RenderSchema` processes a schema item, its initial value is automatically mirrored into the registry.
2. **Live Comparison**: `OnChanged` handlers compare the widget's current state directly against the registry's baseline.
3. **Reactive State**: Schema items support:
   - `EnabledIf`: Dynamic function controlling widget interactivity (e.g., locking an API key field until cleared).
   - `VisibleIf`: Dynamic function controlling widget visibility.
4. **Programmatic Updates**: `SetValue(name, val)` allows for cross-widget state updates.
5. **Atomic Commit**: On "Apply", the engine executes all queued `ApplyFunc` callbacks and then promotes all "Live" values to "Baseline".

### 2.3 The Transactional UI Pattern (Credentials)

For sensitive credentials (API Keys, Usernames), Spice bypasses the deferred model in favor of **Intentional Transactions** via `schema.SecretItem`.

*   **Rationale**: We never want to send an API key silently in the background while the user is typing, nor save a potentially invalid key.
*   **The Flow**:
    1.  **Direct Interaction**: The user enters a key. The "Clear" button transforms into **"Verify & Connect"**.
    2.  **Explicit Action**: Verification only occurs when the button is clicked.
    3.  **Immediate Persistence**: If verification succeeds, the key is saved to the config AND the baseline is seeded **immediately**. The field locks (via `EnabledIf`) and the button becomes **"Clear API Key"**.
    4.  **Hanging Prevention**: All verification transactions MUST include a timeout (standard: 10s) via `context.WithTimeout`.
*   **Implementation**: Declare a `schema.SecretItem` with `OnVerify` and `OnClear` hooks. The engine handles the UI state machine automatically.

### 2.4 The Generic Action Pattern (Query Lists)

`schema.QueryListItem` uses an **Explicit Action Contract** for the list's primary action button (historically "Delete"):

*   **`DeleteLabel`**: Allows a provider to rename the button (e.g., "Clear") without the engine knowing the provider's identity.
*   **`ForceActionEnabled`**: Overrides the standard `Managed` check. Allows "Clearable" system queries (like Favorites) to remain interactive while preserving the "Disabled" guard for read-only synced sources.
*   **`DeleteConfirmMessage`**: Provider-specific warnings injected from the domain layer.
*   **`GetDisplayText` / `GetDisplayURL`**: Pure functions for custom rendering without Fyne imports.

### 2.5 UI Services (ShowError, ShowConfirm, ShowAddQueryDialog)

Providers that need to display dialogs (errors, confirmations, add-query modals) use the `SettingsManager` interface methods. These are **never** called via direct Fyne `dialog.*` imports:

*   `sm.ShowError(err)` — Modal error display
*   `sm.ShowConfirm(title, message, callback)` — Confirmation with boolean callback
*   `sm.ShowAddQueryDialog(cfg, url, desc, onAdded)` — Standardized "Add Query" modal using `schema.AddQueryConfig`
*   `sm.OpenURL(urlString)` — Browser URL opening

## 3. Deep Dive: Provider Implementations

Key logic patterns for the major providers in `pkg/wallpaper/providers/`. **All 8 providers are 100% Fyne-free** — they return `*schema.PanelSchema` from `CreateSettingsPanel()` and `CreateQueryPanel()`.

### 3.1 Wallhaven (`wallhaven.go`)
*   **Regex Router**: Parses user-pasted URLs using rigorous Regex:
    *   `UserFavoritesRegex`: `wallhaven.cc/user/([^/]+)/favorites/(\d+)` -> Converts to API Collection Endpoint.
    *   `SearchRegex`: Extracts `?q=...` or `?categories=...`.
*   **API Key Hygiene**: The `ParseURL` function explicitly *strips* `apikey` params from saved URLs. Keys are stored in the secure OS keychain and **masked** via `schema.SecretItem`.

### 3.2 The Met Museum (`metmuseum.go`)
*   **Schema Template**: Uses `schema.CreateMuseumSettingsPanel()` for a standardized "Institution" look with Plan a Visit, Website, and Donate buttons.
*   **Director's Cut Configuration**: Uses a **Fixed List** of curated queries rendered as `schema.BoolItem` logic groups (not a dynamic list).
*   **Parallel Fetching**: Uses `errgroup` with a concurrency limit (5) to "scan" for valid images.
*   **Filtering**: Rejects extreme aspect ratios (>3.0 or <0.33) but allows both landscape and portrait, since `SmartImageProcessor` handles orientation scaling.

### 3.3 Favorites (`favorites.go`)
*   **Worker Pattern**: File IO (copying 5MB images) is too slow for the main thread.
    *   Uses a `jobChan favJob` channel with a `runWorker` loop.
*   **FIFO Garbage Collection**: Enforces a hard limit (MaxFavoritesLimit). When full, deletes the oldest file (by `ModTime`) before writing.
*   **Metadata Sidecar**: Maintains `metadata.json` for Attribution and Product URL.
*   **UI Implementation**: Uses the **Generic Action Pattern** (§2.4) — sets `DeleteLabel: "Clear"` and `ForceActionEnabled: true` on its `schema.QueryListItem`.

### 3.4 Local Folders (`localfolder.go`)
*   **Cross-Platform Picker**: Uses `schema.FolderPickerItem` which the engine renders as a native directory selector (Windows uses `cfd` shell picker via `picker_windows.go` to avoid Fyne/Cgo deadlocks).
*   **Path Normalization**: Strict case-insensitive deduplication and trailing slash removal.
*   **Recursive Scanning**: Uses `filepath.WalkDir` with early-exit optimization.

## 4. Extension Guide

### 4.1 Adding a New Provider
1.  **Create Package**: `pkg/wallpaper/providers/myprovider`.
2.  **Implement Interface**: `provider.ImageProvider`.
    - `CreateSettingsPanel(sm) *schema.PanelSchema` — Return a declarative schema, not Fyne widgets.
    - `CreateQueryPanel(sm, pendingURL) *schema.PanelSchema` — Return the query list schema.
3.  **Register**:
    ```go
    func init() {
        wallpaper.RegisterProvider("MyProvider", func(cfg *Config, client *client) ImageProvider {
            return NewProvider(cfg, client)
        })
    }
    ```
4.  **Import**: Run `go generate ./...` (or `make gen`). The `gen_providers` tool will automatically discover your package and add it to `cmd/spice/zz_generated_providers.go`. To disable a provider, add a `.disabled` file to its directory.

### 4.2 UI Standardization
Use `schema.AddQueryConfig` for add-query modals and `sm.ShowAddQueryDialog()` to display them.
For museums, use `schema.CreateMuseumSettingsPanel()` to generate the standard header layout.

## 5. Critical Files Map

| File | Role |
|:---|:---|
| `pkg/ui/schema/schema.go` | **PORT**: Framework-agnostic UI contract. All schema types live here. |
| `pkg/ui/schema/museum.go` | Schema helper for standardized museum provider layouts. |
| `pkg/ui/setting/setting_manager.go` | **PORT**: `SettingsManager` interface. Providers depend on this. |
| `ui/settings_manager.go` | **ADAPTER**: Fyne implementation of `RenderSchema()` and all SM methods. |
| `pkg/wallpaper/store.go` | The DB. Thread-safety critical. |
| `pkg/wallpaper/pipeline.go` | The Async Engine. |
| `pkg/wallpaper/smart_image_processor.go` | Face Detection & Cropping logic. |
| `pkg/api/server.go` | Local HTTP server for Browser Extension communication. |


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

Spice uses a custom i18n package (`pkg/i18n`) for runtime translations and synchronization with Fyne's internal state. Translation integrity is enforced at CI time.

### 9.1 Translation Architecture
- **Language Selection**: Managed via `i18n.SetLanguage(code)`. It maps standard codes (e.g., "en", "de", "zh-Hant") to internal states and synchronizes with Fyne's `lang` package.
- **String Retrieval**:
    - `i18n.T("key")`: Standard translation retrieval.
    - `i18n.Tf("key", map)`: Templated translation using Go `text/template` syntax.
    - `i18n.N("key", count)`: Pluralized translation.
- **Embedded Files**: Translations are stored in `pkg/i18n/translations/*.json` and embedded into the binary using `//go:embed`.

### 9.2 The `gen-i18n` Tool (`cmd/util/gen_i18n/main.go`)

This is the single source of truth for translation management. It replaces all previous Python scripts.

**Generate mode** (`make gen-i18n`):
1. **Scans** all `.go` source files for `i18n.T()`, `i18n.Tf()`, and `i18n.N()` calls via regex.
2. **Merges** `dynamicI18nKeys` — keys used via variables (e.g., `attribution_by`) that can't be statically detected.
3. **Auto-adds** new keys to `en.json` (key = value, English fallback).
4. **Warns** about stale keys in `en.json` (keys no longer referenced in code).
5. **Propagates** missing keys into all language files (fills with English fallback).
6. **Generates** `pseudo.json` for layout testing (vowel doubling to test UI expansion).
7. **Generates** `zz_generated_languages.go` — the Go language registry used at runtime.

**Check mode** (`make check-i18n`):
- Read-only. Exits non-zero if:
  - **Stale keys** exist in `en.json` or any language file (keys not in code).
  - **Missing keys** in `en.json` (keys in code but not in translations).
  - **Untranslated strings** in any language file (value identical to English, unless allowlisted).
- This is gated into **release** build chains (`win-amd64`, `linux-amd64`, `darwin-*`) but not dev builds.

### 9.3 Allowlists and Dynamic Keys

Two mechanisms prevent false positives in `--check` mode:

**`allowIdenticalToEnglish`** — Keys where the translation is legitimately the same as English:
- Proper nouns: `"Art Institute of Chicago"`, `"The Metropolitan Museum of Art"`
- Loanwords: `"App"`, `"Online"`, `"System"`, `"General"`, etc.
- Brand names: `"Pexels"`, `"Wikimedia"`, `"Google Photos"`, `"wallhaven"`
- Locations: `"Chicago, IL, USA"`, `"New York City, USA"`

**`dynamicI18nKeys`** — Keys used via variables at runtime, invisible to the static regex scanner:
- `"attribution_by"`, `"attribution_in"` — selected dynamically based on `provider.AttributionType`
- `"Egyptian Art"`, `"European Paintings"` — museum collection names from curated JSON

Both maps live in `cmd/util/gen_i18n/main.go`.

### 9.4 Developer Workflow: Adding New Strings

```bash
# 1. Write Go code with i18n calls
i18n.T("My New String")
i18n.Tf("Download {{.Count}} images", map[string]any{"Count": n})

# 2. Run the generator (auto-adds to en.json, propagates to all languages)
make gen-i18n

# 3. Translate the new key in each language JSON file
#    (or leave as English fallback temporarily)

# 4. Before PR merge — CI runs this and blocks if anything is out of sync
make check-i18n
```

**If using dynamic keys** (variable-based `i18n.Tf(key, ...)` calls):
- Add the key to `dynamicI18nKeys` in `cmd/util/gen_i18n/main.go`.

### 9.5 Adding a New Language

1. Create `pkg/i18n/translations/[code].json` with all keys from `en.json`.
2. Add a `"_meta_name"` key with the language's native display name (e.g., `"日本語"`).
3. Run `make gen-i18n` — this auto-generates the language registry and fills any missing keys.
4. No manual code registration needed. The tool discovers languages from the filesystem.

### 9.6 Tray Menu Constraints (Critical)
When translating strings specifically for the system tray menu:
- **Brevity**: Operating systems (especially Windows) calculate tray menu width based on the longest item. Keep translations as short as possible (e.g., German "Bild" vs "Hintergrundbild").
- **Mnemonic Safety**: On Windows, `&` acts as a mnemonic prefix and is hidden. Use `+` as a universal cross-platform separator instead of `&` or `&&`.
- **Sanitization**: All dynamic strings (like attributions) displayed in the tray MUST pass through `SanitizeMenuString` (in `pkg/wallpaper/helper.go`) to strip HTML and collapse excessive whitespace.
- **Rune-Aware Truncation**: Never truncate tray labels by bytes. Always cast to `[]rune` before slicing to avoid corrupting multi-byte localized characters.

