# Spice Roadmap & Technical Debt

This document tracks planned architectural refactors and feature enhancements to improve systemic stability and user experience.

## 1. UI Framework: Clean State Registry (Closure Trap Prevention)
**Problem**: The `SettingsManager` currently relies on static `InitialValue` benchmarks captured at window creation. If a user toggles a setting and clicks "Apply", the benchmark becomes stale. Subsequent toggles are incorrectly treated as "reverts" to the original (stale) state, preventing further saves in the same window session.

**Refactor Plan**:
- **Internal Registry**: Modify `SettingsManager` (in `ui/settings_manager.go`) to maintain a `map[string]interface{}` of baseline values.
- **Automatic Hydration**: In `CreateBoolSetting`, `CreateSelectSetting`, and `CreateTextEntrySetting`, automatically seed the registry with the `InitialValue`.
- **Live Comparison**: Update `OnChanged` handlers to compare the current widget state against the registry's baseline instead of the ephemeral `Config` struct.
- **The "Commit" Phase**: In the `Apply` button callback, after successfully executing all queued `ApplyFunc` closures, iterate through the dirty settings and update the baseline registry to match the new "True" state.
- **Benefit**: Completely eliminates the "Closure Trap" and removes the need for manual state management in the `ui.go` setup logic.

## 2. Hotkey Engine: Targeted Shortcut Modifier Customization
**Problem**: The default `Alt + Arrow` chord for targeted navigation (Display Specific) conflicts with browser "Back/Forward" history. While dynamic unregistration allows users to disable them to resolve conflicts, power users may want to keep the feature but move the conflict.

**Refactor Plan**:
- **Preference Key**: Add `TargetedModifierPrefKey` to `pkg/wallpaper/const.go`.
- **UI Element**: Add a dropdown in the "Hotkeys" section of Preferences:
    - `Alt` (Default)
    - `Ctrl` (Caution: Word Jumping conflict)
    - `Win / Cmd` (Caution: Snap Assist conflict)
    - `Alt + Ctrl` (Safe but ergonomic-heavy)
- **Engine Logic**: Update `pkg/hotkey/hotkey_default.go` to read this preference before calling `hotkey.New()`.
- **Cross-Platform Mapping**: Ensure the selection maps correctly across platforms (e.g., `ModOpt` on macOS, `ModAlt` on Windows).

## 3. Concurrency & Performance
- **Store Batching**: Investigate moving `scheduleSaveLocked()` to a more granular debouncer for ultra-high-frequency updates.
- **Worker Telemetry**: Add internal metrics for the image processing pipeline to detect bottlenecks in face detection or resolution scaling.
