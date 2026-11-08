---
name: spice-settings-ui
description: >-
  Architecture and step-by-step workflow for adding or modifying settings,
  preferences, Config persistence, declarative UI schemas, and Fyne widgets in
  Spice. Activate this skill whenever working on pkg/wallpaper/config.go,
  pkg/ui/schema/schema.go, pkg/wallpaper/ui_builder.go, provider settings/query
  panels, or ui/settings_manager.go.
---

# Spice Settings & Declarative UI Skill

Spice uses a strict **Hexagonal (Ports & Adapters)** pattern for all configuration and UI. Follow this four-layer workflow whenever adding or modifying a setting or UI control.

---

## Layer 1: Domain & Persistence (`pkg/wallpaper/config.go` & `pkg/wallpaper/const.go`)

All configuration state, validation/normalization, and persistence belong in `pkg/wallpaper/config.go`.

1. **Define Preference Key (if applicable)** in [`pkg/wallpaper/const.go`](../../pkg/wallpaper/const.go):
   - Prefix with `pluginPrefix` (e.g., `MySettingPrefKey = pluginPrefix + "my_setting_key"`).
   - Add the key to `TestConfig_ActivityGatePrefKeysUnique` (or config key uniqueness tests) so collisions are caught.
2. **Persist in `Config` (`pkg/wallpaper/config.go`)**:
   - **Scalar settings (`bool`, `int`, `string`)**: Implement thread-safe `Get*` (`c.mu.RLock()`) and `Set*` (`c.mu.Lock()`) methods using `BoolWithFallback`/`SetBool`, `IntWithFallback`/`SetInt`, `StringWithFallback`/`SetString`.
   - **Structured settings / Lists / Maps (`[]ImageQuery`, `[]string`, `map[string]bool`, structs)**:
     - Add the field with a `json:"..."` tag to the `Config` struct in `pkg/wallpaper/config.go`.
     - Initialize it in `GetConfig()`.
     - Deep-copy the slice/map inside `c.save()` before `json.MarshalIndent(clone, ...)` to prevent data races.
     - Load/migrate it inside `loadFromPrefs()`.
     - Expose clean, thread-safe domain methods on `*Config` (e.g., `GetGameProcesses()`, `SetGameProcesses([]string)`, `AddGameProcess(string) error`, `RemoveGameProcess(string) error`) that normalize inputs, deduplicate, and call `c.save()`.
   - **Never** leak string-splitting, comma-joining, path normalization, or deduplication into `ui_builder.go` or `ui/settings_manager.go`.

---

## Layer 2: Pure-Go Schema Contract (`pkg/ui/schema/schema.go`)

- `pkg/ui/schema/schema.go` must remain **100% pure Go** (zero Fyne imports).
- Reuse existing schema types whenever possible:
  - `BoolItem`, `TextItem`, `SelectItem`, `SliderItem` — standard scalar settings with `InitialValue` and `ApplyFunc`.
  - `AppPickerItem` — native OS application/executable picker with a per-entry list and individual `Delete` buttons (`InitialValue []string`, `ApplyFunc`). Additions and removals are staged in UI memory and persisted only when `ApplyFunc` runs via **Apply Changes**.
  - `FolderPickerItem` + `QueryListItem` — native directory picker and scrollable query table with per-row `Active` toggle and `Delete` button.
  - `SecretItem` — transactional credential input with `OnVerify` and `OnClear`.
  - `ButtonItem`, `ConfirmButtonItem`, `AsyncButtonItem` — action buttons.

---

## Layer 3: Declarative UI Builder (`pkg/wallpaper/ui_builder.go` & `pkg/wallpaper/providers/*`)

- `pkg/wallpaper/ui_builder.go` and provider `CreateSettingsPanel` / `CreateQueryPanel` methods must be **purely declarative**.
- Wire `b.plugin.cfg` methods directly to schema fields:
  ```go
  schema.AppPickerItem{
      Name:         "gateGameProcesses",
      Label:        i18n.T("Pause for specific apps & games:"),
      Help:         i18n.T("Automatically pause wallpaper changes whenever one of these apps or games is running in the foreground."),
      ButtonText:   i18n.T("Add App or Game..."),
      InitialValue: b.plugin.cfg.GetGameProcesses(),
      ApplyFunc: func(val []string) {
          b.plugin.cfg.SetGameProcesses(val)
          if b.plugin.gate != nil {
              b.plugin.gate.Wakeup()
          }
      },
  }
  ```

---

## Layer 4: Fyne Adapter & Registry Pattern (`ui/settings_manager.go`)

`ui/settings_manager.go` is the sole Fyne rendering engine. Every rendered setting must integrate with the `SettingsManager` Registry Pattern:

1. **Seed Baseline**: Store the initial value in `sm.registry[v.Name]`.
2. **Register Getters & Apply Wrappers**:
   - `sm.valueGetters[v.Name]` returns the current live widget value.
   - `sm.applyFuncs[v.Name]` invokes `v.ApplyFunc`.
3. **Dirty Tracking (Staged vs. Immediate — Never Mix)**:
   - Any widget that participates in the **Apply Changes** workflow (`BoolItem`, `SelectItem`, `TextItem`, `SliderItem`, `AppPickerItem`) **must stage changes in UI memory** and **only** persist to `Config` when `v.ApplyFunc` runs on **Apply Changes**.
   - **Never** call `Config` persistence methods on intermediate widget clicks if the widget also marks the form dirty for **Apply Changes**.
   - When the user interacts with the widget, compare the live staged value with `sm.registry[v.Name]`.
   - If different from baseline: call `sm.SetSettingChangedCallback(v.Name, func() { ... })`.
   - If equal to baseline: call `sm.RemoveSettingChangedCallback(v.Name)`.
   - Call `sm.checkAndEnableApply()` to update the **Apply Changes** button state and evaluate `EnabledIf` / `VisibleIf` rules.
4. **One-Way Port Data Flow (`PluginManager` in `pkg/ui/plugin_manager.go` & `ui/ui.go`)**:
   - When the domain (`pkg/wallpaper/`) calls a `PluginManager` method (e.g. `ShowTuneImagePopup`), snapshot all required state (`displayLabel`, `monitorBounds`, `currentOpts`, `effectiveOpts`) under `mc.mu.RLock()` and pass it across the port.
   - Never have `ui/ui.go` reach back into `wallpaper.GetInstance()` to re-query state that the domain caller already held.
4. **Critical Gotchas**:
   - `sm.SetValue(name, val)` updates widget display state programmatically and **does not** trigger `OnChanged` or register a dirty callback. Do not use `sm.SetValue` expecting it to persist a setting.
   - Native OS file/folder pickers (`ui/picker_windows.go`, `ui/picker_darwin.go`, `ui/picker_others.go`) run on background goroutines; any Fyne widget updates in their callbacks **must** be wrapped in `fyne.Do(func() { ... })`.

---

## Verification Checklist

1. Add unit tests for the new `Config` methods and reload persistence (`ResetConfig()` + `GetConfig(prefs)`) in `pkg/wallpaper/`.
2. Update all translation files in `pkg/i18n/translations/*.json` for any new `i18n.T` / `i18n.Tf` keys.
3. Run:
   ```powershell
   go run ./cmd/util/gen_i18n/main.go --check
   go vet ./...
   go test ./pkg/wallpaper/... ./ui/...
   ```

