# Spice Roadmap & Technical Debt

This document tracks planned architectural refactors and feature enhancements to improve systemic stability and user experience.

## 1. UI Framework: Clean State Registry

- [x] **Registry Pattern Implementation**: Successfully implemented in v2.3.0. The `SettingsManager` now maintains a baseline registry and uses a git-like commit model for atomic saves. Eliminates the "Closure Trap" where stale closures captured outdated widget references.

## 2. Hotkey Engine: Targeted Shortcut Modifier Customization

**Problem**: The default `Alt + Arrow` chord for targeted navigation (Display Specific) conflicts with browser "Back/Forward" history. While dynamic unregistration allows users to disable them to resolve conflicts, power users may want to keep the feature but remap the modifier.

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

- [ ] **Store Batching**: Investigate moving `scheduleSaveLocked()` to a more granular debouncer for ultra-high-frequency updates.
- [ ] **Worker Telemetry**: Add internal metrics for the image processing pipeline to detect bottlenecks in face detection or resolution scaling.
