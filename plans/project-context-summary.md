# Spice — Comprehensive Architecture & Codebase Context

> **Generated**: October 2026
> **Scope**: Architecture, component roles, concurrency constraints, key packages, and Go source files in preparation for feature implementation.

---

## 1. High-Level Overview & Product Model

[`README.md`](../README.md:1) identifies Spice as a high-concurrency desktop environment engine for **macOS (Apple Silicon)** and **Windows (x64/ARM)** written in Go with a Fyne UI. Distributed through the **Mac App Store** and **Microsoft Store**, it operates quietly in the macOS menu bar or Windows system tray.

### Core Capabilities
1. **13 Image Sources**:
   - Curated Open Access Museums: The Met, Art Institute of Chicago, Cleveland Museum of Art, Rijksmuseum, National Palace Museum (Taiwan), Statens Museum for Kunst (Denmark), J. Paul Getty Museum.
   - Community APIs: Wallhaven, Pexels, Wikimedia Commons.
   - Personal/Local: Google Photos, Local Directory scanning, Local Favorites.
2. **Browser Extension Companion**: Chrome, Firefox, and Safari extensions communicating via local REST API ([`pkg/api/server.go`](../pkg/api/server.go:1)).
3. **Smart Fit 2.0**: Cascade face detection (`pigo`) and luminance entropy analysis (`smartcrop`) that dynamically chooses between Face Crop, Face Boost (Smart Pan), Entropy Crop, or Centered fallback.
4. **Virtual Museum Frame**: Matting/framing fallback for extreme aspect ratio artworks ([`pkg/wallpaper/virtual_framer.go`](../pkg/wallpaper/virtual_framer.go:1)).
5. **Decoupled Hexagonal UI**: Providers and plugins declare UI using pure Go schema types ([`pkg/ui/schema/schema.go`](../pkg/ui/schema/schema.go:1)); Fyne adapters render them ([`ui/settings_manager.go`](../ui/settings_manager.go:1)).
6. **Autonomous Multi-Monitor Display Actors**: Each display is managed by an independent `MonitorController` actor loop with dedicated resolution buckets and shuffle decks ([`pkg/wallpaper/monitor_controller.go`](../pkg/wallpaper/monitor_controller.go:1)).

### Module & Dependencies ([`go.mod`](../go.mod:1))
- Module: `github.com/dixieflatline76/Spice/v2`, Go 1.25.0, Fyne v2.7.4.
- Notable deps: `esimov/pigo` (face detection), `muesli/smartcrop`, `disintegration/imaging`, `nfnt/resize`, `zalando/go-keyring` (credentials), `golang.design/x/hotkey`, `gorilla/websocket` + local REST, `go-locale` (i18n), `lumberjack` (log rotation), `google/go-github` (release/OTA checks), `piprate/json-gold` (JSON-LD, museum metadata).

---

## 2. Directory Structure & Key Packages

```text
Spice/
├── cmd/
│   ├── spice/                 # Main app entrypoint (probe_windows, mutex, zz_generated_providers.go)
│   ├── gallerygen/            # Offline salon masonry gallery generator
│   └── util/                  # Dev utilities: audit_met/audit_aic, debug_crop, gen_i18n, gen_providers,
│                              #   load_secrets, pack_extension, spice_releaser, stress_test, sync_regex,
│                              #   face_detect_tuning_report, smart_fit_report, check_dimensions
├── config/                    # App-wide config + distribution detection (App Store / MSIX / dev build)
├── docs/                      # Architecture, provider/plugin guides, roadmap, release process
├── extension/                 # Manifest V3 browser companion (Chrome/Edge/Firefox) + i18n locales
├── pkg/
│   ├── api/                   # Local REST API server for the browser extension (server.go, handlers.go, local_handlers.go)
│   ├── curation/              # Git-driven OTA museum collection loader (raw.githubusercontent.com CDN)
│   ├── gallery/               # Salon layout packing math (salon.go)
│   ├── hotkey/                # Global shortcut listeners (platform-specific)
│   ├── i18n/                  # 12-language pure-Go localization (embedded JSON + gen-i18n tooling)
│   ├── provider/              # Domain interfaces & Image model (provider.go)
│   ├── sysinfo/               # OS detection, monitor topology, Windows OpenGL probe
│   ├── ui/
│   │   ├── schema/            # Pure-Go declarative UI contracts (schema.go, museum.go)
│   │   └── setting/           # SettingsManager interface (port)
│   └── wallpaper/             # Core engine (store, pipeline, dispatcher, monitor_controller, processors…)
│       └── providers/         # 13 provider implementations (artic, cleveland, favorites, getty, googlephotos,
│                              #   localfolder, metmuseum, npm, pexels, rijksmuseum, smk, wallhaven, wikimedia)
├── ui/                        # Fyne adapter layer: app shell, tray, settings renderer, pickers, EULA flows
├── util/                      # util.SafeCounter/SafeFlag helpers, EULA store decoupling, logging package
└── plans/                     # Architectural plans & context briefs (this file)
```

---

## 3. Package Deep-Dive & Source File Analysis

### 3.1 `pkg/provider` ([`provider.go`](../pkg/provider/provider.go:1))
- **`ImageProvider` interface** (pure Go, Fyne-free):
  - Identity: `ID()` (stable, non-localized — changing breaks user settings), `Name()` (localized), `Title()` (short tray form), `GetProviderIcon() interface{}`.
  - Metadata: `Type() ProviderType` → `TypeCommunity` / `TypePersonal` / `TypeMuseum`; `AttributionType` → `AttributionBy` / `AttributionIn`.
  - Data: `ParseURL(webURL)` validation; `FetchImages(ctx, apiURL, page)` (deterministic pages — **no shuffling**); `EnrichImage(ctx, img)` lazy metadata completion.
  - UI ports: `CreateSettingsPanel(sm) *schema.PanelSchema`, `CreateQueryPanel(sm, pendingURL) *schema.PanelSchema`.
- **Optional capability interfaces**: `Favoriter` (`IsFavorited/AddFavorite/RemoveFavorite/GetSourceQueryID`), `PacedProvider` (`GetAPIPacing/GetProcessPacing`), `CustomClientProvider` (`GetClient`), `ThumbnailProvider` (`FetchThumbnails` for gallery generation).
- **`Image` struct** (the universal currency of the engine):
  - Core: `ID`, `Path` (download URL), `ViewURL`, `FilePath`, `Attribution`, `Title`, `Artist`, `Year`, `Provider`, `FileType`, `Width/Height` (probed & persisted).
  - Pipeline state: `ProcessingFlags map[string]bool` (Processing Hash incl. `incompatible:WxH` tags), `DerivativePaths map[string]string` (`"WxH"` → derivative file), `SourceQueryID` (smart cache clearing), `IsFavorited`, `Seen`.
  - Tuning: `Tuning map[string]TuningOptions` keyed by `"WxH"` — per-image/per-resolution overrides: `CropAnchor` (200–209, aligned with hotkey Command values for zero-conversion dispatch), `FrameOverrideMode`, `MattingOverrideMode`, `WallColorOverrideMode`, `FrameSize`, `TightCrop`.
  - Helpers: `GetTuning(resKey)` (defaults to `AnchorAuto`/Inherit), `MergeExistingMetadata()` (preserves probed dims + tuning during backlog healing), custom `UnmarshalJSON` migrating legacy `CropAnchors`.
- **`CropAnchor`** enum: `AnchorAuto=200 … AnchorBottomRight=209` with `Center()` normalized coordinates and `String()`.

### 3.2 `pkg/wallpaper` — Engine Core
- **`Plugin`** ([`wallpaper.go`](../pkg/wallpaper/wallpaper.go:1), ~1358 lines): singleton orchestrator (`wpOnce`).
  - Holds: `os OS`, `imgProcessor ImageProcessor` (VirtualFramer wrapping SmartImageProcessor), `cfg *Config`, `store StoreInterface`, `fm *FileManager`, `pipeline *Pipeline`, `jobSubmitter JobSubmitter`, `providers map[string]ImageProvider`, `favoriter provider.Favoriter`, `Monitors map[int]*MonitorController` guarded by `monMu`.
  - Concurrency primitives: `downloadMutex`, `globalFetchMu`, `queryCancelMu` + per-query context/cancel maps, rate `sync.Map` limiters per provider (API + process).
  - Robust shared `http.Client` with `UserAgentTransport` (Chrome UA), tuned dial/TLS timeouts.
  - `Activate()`: wires callbacks, starts nightly scheduler, optional startup pulse vs page-reset + `SyncProviders()` + `FetchNewImages`, `ChangeWallpaperFrequency`, `startMonitorWatcher`, `RebuildTrayMenu`.
  - `Deactivate()`: cancel fetch context FIRST → stop nightly/ticker/ctx → `interrupt` flag → `stopAllWorkers` → nil-out jobSubmitter/pipeline → `pipeline.Stop()` → stop all MC actors → `fm.Close()` (drain).
  - `RequestFetch(providerID...)`: gatekeeper — Favorites bypasses download debounce; global fetches use anti-loop cooldowns (60s starvation / 15s retry) tracking `lastTriggeredSeenCount/TotalCount`.
  - `SetNextWallpaper(-1, false)` staggering: monitor 0 immediate, others `time.AfterFunc` random 10–30% of interval (`CmdNextAuto`).
  - Command shims: `SetPreviousWallpaper`, `DeleteCurrentImage`, `TogglePauseMonitorAction` (global vs targeted), `IsMonitorPaused` (documented benign racy read), hotkey enable/disable setters.
  - Ticker loop: intentional lockless `wp.ticker != currentTicker` stale check; `resetTimerCh` drains concurrent ticks and `Reset(duration)` on user action.
- **`MonitorController`** ([`monitor_controller.go`](../pkg/wallpaper/monitor_controller.go:1), ~649 lines): the Actor.
  - Channels: `Commands chan Command` (buffer 50), `TuningChan chan TuningOptions` (buffer 50).
  - `Run(ctx)` loop: `ctx.Done` | `Store.GetUpdateChannel()` broadcast (refresh handle each event; retry `next(ManualRecovery)` if starving) | `Commands` → `handleCommand` under `mc.mu.Lock()` | `TuningChan` → `reprocessWithTuning`.
  - `Command` enum: `CmdNext/CmdPrev/CmdDelete/CmdBlock/CmdFavorite/CmdUpdateShuffle/CmdSyncState/CmdPause/CmdNextAuto`, tuning start/end, and legacy anchors 200–209 mapped directly from hotkeys.
  - `StoreInterface` (full contract the store must satisfy): read paths (`GetIDsForResolution`, `GetBucketSize`, `GetByID`, `GetUpdateChannel`, `WaitForImages`), mutations (`Add`, `Remove`, `MarkSeen`, `SetTuningOptions`, `SetDerivativePath`, `ClearDerivatives`, `SetFavorited`), admin (`Sync`, `RemoveByQueryID`, `Wipe`, `ResetFavorites`, `LoadCache`, `LoadAvoidSet`, debounce/save controls).
  - `MonitorState`: `CurrentID`, `History`, `RandomPos`, `ShuffleIDs` (per-resolution deck), `CurrentImage`, `WaitingForImages`, `Paused` (persisted per `DevicePath`), `TuningInProgress`, `ManualRecovery`.
  - `next(manual)` flow: bucket query → starvation check (< `BucketStarvationThreshold`) or cycle progress (> `PrcntSeenTillDownload` = 0.8) → `OnFetchRequest` → empty ⇒ `WaitingForImages=true` + `ManualRecovery=manual` → reconcile shuffle (`growShuffle` scatters new IDs into unplayed portion to avoid provider clustering; full rebuild on shrink/exhaustion) → pick → apply → `MarkSeen`.
- **Store/Pipeline/Scheduler/FileManaager**: as documented in [architecture.md](../docs/architecture.md) — resolution buckets, FIFO `Sync` eviction, debounced JSON saves, per-provider dispatcher pumps with `rate.Limiter`, `runtime.NumCPU()` workers, single serialized stateManager loop with `runtime.Gosched()`.

### 3.3 Schema System ([`pkg/ui/schema/schema.go`](../pkg/ui/schema/schema.go:1))
Full declarative item catalog (superset of the docs list):
- Controls: `BoolItem`, `TextItem` (`IsNumeric`, validators, `SkipApply`), `SelectItem` (index-bound), `SliderItem` (min/max/step/format), `SecretItem` (transactional verify/clear), `OAuthPickerItem` (auth status + picker stream + collection persistence hooks), `FolderPickerItem`.
- Actions: `ButtonItem`, `AsyncButtonItem` (loading state + `TargetStatusKey`), `ConfirmButtonItem` (with `Importance`), `HyperlinkItem`, `LabelItem` (`IsTitle`, `Importance`).
- Composites: `HorizontalRowItem` (+ `RowLayout` `hbox_spacer`), `QueryListItem` (explicit action contract), `AddQueryConfig`.
- Layout containers: `TabsSchema`/`TabItemSchema` (with `Panel` or `Accordion`), `AccordionSchema`/`AccordionItemSchema` (dynamic `TitleFunc`, `Open`, icons), `MenuSchema`/`MenuItemSchema` (tray menus, separators, checkmarks, submenus).
- Conventions: `NeedsRefresh` triggers global refresh + derivative invalidation; `EnabledIf`/`VisibleIf` closures everywhere; `ButtonStyle`/`Importance` pure-Go visual weights.

### 3.4 Plugin System ([`pkg/ui/plugin.go`](../pkg/ui/plugin.go:1))
Actual interface (schema-based — docs drift noted):
```go
type Plugin interface {
    ID() string
    Name() string
    Activate()
    Deactivate()
    CreateTrayMenu() *schema.MenuSchema          // declarative tray
    CreatePrefsSchema(setting.SettingsManager) *schema.TabsSchema // declarative prefs tabs
    Init(PluginManager)
}
```
`Notifier` type: `func(title, message string)`. The wallpaper engine ([`pkg/wallpaper/wallpaper.go`](../pkg/wallpaper/wallpaper.go:1)) is itself the flagship plugin.

### 3.5 Configuration Layer
- [`config/app_config.go`](../config/app_config.go:1): app-level prefs on `fyne.Preferences` — notifications, update checks, theme (`System` default), debug logging, language override (`System` | locale code).
- [`config/distribution.go`](../config/distribution.go:1): detects App Store / MSIX / standard distribution (affects EULA variant via [`util/eula.go`](../util/eula.go:1) and UI gating; see `ui/ui_eula_appstore.go` vs `ui_eula_standard.go`).
- [`pkg/wallpaper/config.go`](../pkg/wallpaper/config.go:1) (~1522 lines): engine `Config` + `Preferences` interface + Migration Chain (see [`config_migrations.go`](../pkg/wallpaper/config_migrations.go:1)).
- [`pkg/wallpaper/const.go`](../pkg/wallpaper/const.go:1): tuning thresholds (`BucketStarvationThreshold`, `PrcntSeenTillDownload`), HTTP timeouts, preference keys.

### 3.6 UI Adapter ([`ui/`](../ui:1))
- [`ui/settings_manager.go`](../ui/settings_manager.go:1): `RenderSchema()` engine — Registry Pattern (baseline seeding → dirty detection → atomic commit), renders all schema item types incl. Tabs/Accordion/Menus.
- [`ui/ui.go`](../ui/ui.go:1): app shell, tray build from `schema.MenuSchema`, monitor watcher integration.
- Platform files: `darwin.go`, `windows.go`, `linux.go`, pickers (`picker_*.go` via `go-common-file-dialog` on Windows), EULA flows, `layout.go`, `const.go`, `settings_helpers.go`.

### 3.7 API Server ([`pkg/api/`](../pkg/api:1))
- `server.go` + `handlers.go` + `local_handlers.go` + `local_listing_handler.go`: local REST surface consumed by the browser extension (send image → queue fetch; open preferences via `fyne.Do`; listing endpoints). Tests in `server_test.go`, `local_handlers_test.go`.

### 3.8 Support Systems
- [`pkg/hotkey/`](../pkg/hotkey:1): global + targeted chords (README table), modifier remap planned (see docs/todos.md).
- [`pkg/i18n/`](../pkg/i18n:1): `T/Tf/N`, embedded translations, `gen-i18n` codegen + CI check, allowlists.
- [`pkg/curation/`](../pkg/curation:1) + [`docs/collections/`](../docs/collections:1): embedded fallback JSONs per museum (`embed.go`).
- [`asset/`](../asset:1): embedded icons, splash, pigo `facefinder` model, default config, Salon gallery HTML.
- [`util/log/`](../util/log:1): debug vs release logging variants (lumberjack).

---

## 4. Architectural Rules & Invariants (Golden Constraints)

1. **Lock hierarchy**: `wp.monMu → mc.mu → store.mu → saveMu`; `downloadMutex`, `globalFetchMu`, `queryCancelMu`, `favProvider.mu` independent — never nested with `monMu`.
2. **Fyne main thread**: never hold engine locks across `fyne.Do`; snapshot under lock, then dispatch. `OpenPreferences` must marshal via `fyne.Do` from the API goroutine.
3. **Providers never shuffle** (FIFO store integrity); deterministic pagination only; no memory-based full-collection caches.
4. **Safe Page Wrapping**: 0 results at `page > 1` with `err == nil` → reset to page 1.
5. **Backlog Healing**: dedup must re-submit images missing derivatives for current monitors; `MergeExistingMetadata` preserves probed dims + user tuning.
6. **Resolution probing**: discovered `Width/Height` persisted back to store (Ghost Dimension fix).
7. **3rd Compatibility Gate**: re-evaluates aspect math with current settings at derivative time (stale `incompatible` tags are not authoritative).
8. **Windows OpenGL guard**: hidden `-probe-gl` subprocess probes GLFW; `SEM_NOGPFAULTERRORBOX` suppresses WER.
9. **Tune Image bridge**: temporary `OnWallpaperChanged` hook + channel + 10s timeout; never mutex across UI/Actor boundary.
10. **i18n discipline**: all user strings via `i18n.T/Tf/N`; `make gen-i18n` + `make check-i18n` CI gate; tray strings must be short, `SanitizeMenuString`, rune-safe truncation.

---

## 5. Roadmap & Open Work Signals

- **v3.0 Engine Intelligence**: pause during games/presentations; battery saver; multi-monitor fingerprinting (per-monitor sources); dark mode / solar filtering.
- **v3.1**: `.artwalk` shareable tours; color extraction (ricing JSON).
- **v3.2**: Linux Wayland (swww/hyprpaper); custom user scripts (Python/Bash scrapers).
- **v4.0**: Nacho Chilli companion (local AI via Ollama/LM Studio; docs in `docs/project/nacho-chilli/`).
- **Open todos**: `TargetedModifierPrefKey` hotkey modifier dropdown; store save debouncer batching; worker telemetry.
- **v3.0 pre-req debt**: remove `GetHideStatusBar()`, `ParseSmartFitMode()`, idle enrichment worker; docs drift sync (noted: `creating_new_plugins.md` still shows old fyne-based `CreateTrayMenuItems/CreatePrefsPanel` while code uses schema-based interface).

---

## 6. Feature-Work Entry Points (by concern)

| Concern | Primary files |
|:---|:---|
| New image source | `pkg/wallpaper/providers/<name>/` + `go generate` registration |
| Wallpaper pipeline behavior | `pkg/wallpaper/pipeline.go`, `dispatcher.go`, `fetch_logic.go`, `downloader.go` |
| Display/rotation logic | `pkg/wallpaper/monitor_controller.go`, `wallpaper.go` |
| Image processing/cropping | `pkg/wallpaper/smart_image_processor.go`, `crop_strategies.go`, `virtual_framer.go`, `tuning.go` |
| Persistence/cache | `pkg/wallpaper/store.go`, `sync.go`, `file_manager.go`, `scheduler.go` |
| Settings UI | `pkg/ui/schema/*` + `ui/settings_manager.go` (adapter) |
| Tray/menus | `schema.MenuSchema` + `ui/ui.go` |
| Hotkeys | `pkg/hotkey/*` + `pkg/wallpaper/const.go` prefs |
| Browser extension | `pkg/api/*` + `extension/` |
| App prefs/distribution | `config/*` |

The codebase context is established; ready to plan the requested feature in detail.
