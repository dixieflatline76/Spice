# Spice Project Rules & Architectural Invariants

Before modifying code in Spice, follow these non-negotiable architectural boundaries. For deep reference, see [`docs/internal_developer_context.md`](docs/internal_developer_context.md) and [`docs/architecture.md`](docs/architecture.md).

---

## 1. Hexagonal UI & Settings Architecture (Strict Separation)

Spice strictly separates its Pure-Go Domain Core (Inner Ring) from its Fyne UI Adapter (Outer Ring):

1. **Domain Configuration & Persistence (`pkg/wallpaper/config.go` & `pkg/wallpaper/const.go`)**:
   - `Config` in `pkg/wallpaper/config.go` is the **sole owner** of domain state, normalization, deduplication, and persistence.
   - Structured settings (`Queries`, `AvoidSet`, `MonitorPauseStates`, `GameProcesses`, `MuseumFraming`, `Tuning`, etc.) are fields on `Config`, deep-copied in `c.save()` (serialized to `WallhavenConfigPrefKey` JSON), and loaded/migrated in `loadFromPrefs()`.
   - Scalar preferences use `Preferences` keys defined in `pkg/wallpaper/const.go`.
   - All domain mutation and list manipulation methods (`Add*`, `Remove*`, `Enable*`, `Disable*`, `Get*`, `Set*`) **must** live as thread-safe (`c.mu`) methods on `*Config` in `pkg/wallpaper/config.go` and call `c.save()`.

2. **Declarative Schema Port (`pkg/ui/schema/schema.go` & `pkg/ui/setting/setting_manager.go`)**:
   - `pkg/ui/schema/schema.go` defines pure-Go declarative structs (`BoolItem`, `TextItem`, `SelectItem`, `SliderItem`, `AppPickerItem`, `FolderPickerItem`, `QueryListItem`, `SecretItem`, etc.).
   - **Zero Fyne imports** are permitted in `pkg/ui/schema/`, `pkg/wallpaper/`, or `pkg/wallpaper/providers/`.

3. **Declarative Builders (`pkg/wallpaper/ui_builder.go` & `pkg/wallpaper/providers/*`)**:
   - Builders must be **100% declarative**. They only map `b.plugin.cfg` getters and domain methods (`InitialValue`, `ApplyFunc`, `OnAdd`, `OnRemove`, `GetQueries`, etc.) onto `schema.*` structs.
   - **Never** put string-splitting, parsing, deduplication, or business logic inside `ui_builder.go` or provider UI methods.

4. **Isolated Fyne Adapter (`ui/settings_manager.go`)**:
   - `ui/settings_manager.go` is the **only** place that translates `schema.*` structs into Fyne widgets and OS dialogs (`ui/picker_*.go`).
   - All settings widgets must participate in `SettingsManager`'s **Registry Pattern**:
     - Seed initial baseline into `sm.registry[name]`.
     - Register live getter in `sm.valueGetters[name]` and apply wrapper in `sm.applyFuncs[name]`.
     - Compare live value against baseline on user interaction; call `sm.SetSettingChangedCallback(name, ...)` when dirty or `sm.RemoveSettingChangedCallback(name)` when reverted, followed by `sm.checkAndEnableApply()`.
   - **Gotcha**: `sm.SetValue(name, val)` is strictly for programmatic UI synchronization and **does not** trigger `OnChanged` or mark a setting dirty for the "Apply Changes" button.

---

## 2. Concurrency & Lock Hierarchy

Acquire locks strictly in this order to prevent deadlocks:
```text
wp.monMu -> mc.mu -> store.mu -> saveMu
```
- `wp.downloadMutex`, `wp.globalFetchMu`, `wp.queryCancelMu`, `favProvider.mu`, and `gate.mu` are independent and must **never** be nested with `wp.monMu` or `mc.mu`.
- **Never hold `wp.monMu` when calling into Fyne (`fyne.Do`, widget updates, or tray menu updates)**. Snapshot state under `monMu`, release the lock, and then invoke Fyne.
- Window creation or UI mutation from background goroutines (e.g., HTTP API handlers or OS file picker goroutines) **must** use `fyne.Do(...)`.
- Each monitor is an independent Actor (`MonitorController` in `pkg/wallpaper/monitor_controller.go`). Temporary system suppression (`ActivityGate`) uses `wp.gateSuppressed` (`*util.SafeFlag`) and must **never** overwrite per-monitor user pause state (`mc.State.Paused` / `c.MonitorPauseStates`).

---

## 3. Internationalization (i18n)

- All user-visible strings in UI schemas, notifications, and tray menus must use `i18n.T("...")` or `i18n.Tf("...", map[string]any{...})`.
- Whenever adding or changing any `i18n.T` / `i18n.Tf` string, update all translation files in `pkg/i18n/translations/*.json` (note: non-English locales fail CI if a translation is identical to English unless allowlisted in `cmd/util/gen_i18n/main.go`).
- Always verify with:
  ```powershell
  go run ./cmd/util/gen_i18n/main.go --check
  ```

---

## 4. Golden Engine Rules (Never Remove)

1. **Safe Page Wrapping (`pkg/wallpaper/fetch_logic.go`)**: When `FetchImages` returns 0 results for `page > 1` with `err == nil`, always reset the query page counter to Page 1.
2. **Resolution Probing (`pkg/wallpaper/downloader.go`)**: Decoded image `Width` and `Height` must be persisted back to the `Store`.
3. **Backlog Healing (`pkg/wallpaper/fetch_logic.go`)**: Do not skip an existing store image if it is missing fitted derivatives for currently connected monitors.

---

## 5. CI/CD, Testing & Cross-Platform Guardrails

1. **Never Modify `.github/workflows/` or Build Infrastructure Without Asking**:
   - Do **not** edit `.github/workflows/*.yml` or `makefile` to diagnose or work around a failing CI build, linter error, or test failure. Always ask the user first before touching CI/CD or release configuration.
   - Note that `golangci/golangci-lint-action` runs `golangci-lint` under Node's `child_process.exec` with a 1 MB output buffer: if `golangci-lint` fails with `exit with code null` on `macos-latest` or `windows-latest`, it means the compiler (`clang`/`gcc`/`go build`) dumped >1 MB of compile errors to `stderr` (not an OOM). Audit the branch's platform-specific files (`git diff origin/main...HEAD`) directly instead of changing CI flags.

2. **Diagnose Locally First — Never Use CI as a Trial-and-Error Loop**:
   - Do not blindly rerun failed CI jobs (`gh run rerun`) without first reproducing or probing the failure locally.
   - **Darwin CGO Rules**: Any `.go` file with inline Objective-C in its CGO preamble (`#import <AppKit/AppKit.h>`, `@autoreleasepool`, `NSWorkspace`, etc.) **must** specify `#cgo CFLAGS: -x objective-c` (or place Objective-C in a separate `.m` file like `pkg/wallpaper/wallpaper_native_darwin.m`) and use standard C / Toll-Free Bridging casts (`(NSDictionary *)`, `(NSString *)`, `(CFDictionaryRef)`).
   - **Cross-Platform Path Normalization**: When extracting executable or file basenames from user strings that may originate on either Windows or macOS (e.g., `normalizeGameProcesses` in `pkg/wallpaper/config.go` or `normalizeProcessName` in `pkg/activity/activity.go`), split across **both** `/` and `\` (`strings.LastIndexAny(s, `/\`)`) instead of `filepath.Base`, which does not split on `\` when tests run on macOS/Linux.

3. **Hermetic Tests (Zero Un-Gated Live Network Calls in CI)**:
   - Unit tests must be deterministic and offline-safe using `httptest.NewServer` or a mock `http.RoundTripper` (see `pkg/wallpaper/providers/artic/artic_test.go`).
   - Any integration test that contacts a live external server **must** be gated at the start of the test:
     ```go
     if os.Getenv("SPICE_LIVE_TESTS") == "" {
         t.Skip("Skipping live network test in CI. Run with SPICE_LIVE_TESTS=1 to execute.")
     }
     ```
   - To verify locally that no test leaks live network calls, run with a dead proxy (`HTTP_PROXY=http://127.0.0.1:1 HTTPS_PROXY=http://127.0.0.1:1 NO_PROXY=127.0.0.1,localhost go test ./...`).

4. **Keep Scratch Plans & Private Roadmaps Out of Git (`plans/`)**:
   - Never commit AI task plans, risk assessments, brainstorming docs, or unreleased product roadmaps to the public repository. Store them in the conversation artifact/scratch directory or in the local gitignored `plans/` folder. Only polished user and contributor documentation for shipped features belongs in `docs/`.



