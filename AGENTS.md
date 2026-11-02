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

