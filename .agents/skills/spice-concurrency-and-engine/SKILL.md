---
name: spice-concurrency-and-engine
description: >-
  Concurrency rules, lock hierarchy, MonitorController Actor model, ActivityGate
  invariants, and ImageProvider patterns for the Spice wallpaper engine.
  Activate this skill when modifying pkg/wallpaper/, pkg/activity/, or
  pkg/wallpaper/providers/.
---

# Spice Concurrency & Wallpaper Engine Skill

Use this guide whenever modifying the wallpaper engine (`pkg/wallpaper/`), OS activity detection (`pkg/activity/`), or image providers (`pkg/wallpaper/providers/`).

---

## 1. Lock Hierarchy & Deadlock Prevention

Always acquire locks in this exact order:
```text
wp.monMu -> mc.mu -> store.mu -> saveMu
```
Independent locks (never nest with `wp.monMu` or `mc.mu`):
- `wp.downloadMutex`
- `wp.globalFetchMu`
- `wp.queryCancelMu`
- `favProvider.mu`
- `gate.mu`

### Fyne Main-Thread Deadlock Rule
- **Never** hold `wp.monMu` while calling `fyne.Do`, updating Fyne widgets, or rebuilding the system tray menu (`CreateTrayMenuItems` / `RebuildTrayMenu` / `RebuildTrayMenuImmediate`).
- Always snapshot monitor/engine state under `wp.monMu.RLock()`, release `wp.monMu`, and then call into Fyne or build menu schemas lock-free.
- **Dual-Lock Invariant on `mc.Monitor`**: All writers of `mc.Monitor` (`SyncActionUpdate`, `SyncActionRefresh`) must hold **both** `wp.monMu.Lock()` and `mc.mu.Lock()`; readers may hold either `wp.monMu.RLock()` or `mc.mu.RLock()`.
- **Per-Screen Tray (`TrayCapableOS`)**: `HasPerScreenTray()` must be cached/cheap (`sync.Once`), `GetCursorPosition()` must be thread-safe and never dispatch to the main thread (`CGEventCreate`), and tray rebuilds triggered by cursor crossing must happen while the menu is closed (AppKit tracking mode blocks `systray` main-thread calls).

---

## 2. MonitorController Actor Model (`pkg/wallpaper/monitor_controller.go`)

Each physical display is managed by its own `MonitorController` actor goroutine:
- State mutations happen via commands sent to `mc.Commands` (`CmdNext`, `CmdNextAuto`, `CmdPrev`, `CmdPause`) or under `mc.mu`.
- **User Pause vs. System Suppression**:
  - `mc.State.Paused` (persisted in `cfg.MonitorPauseStates`) represents the **user's explicit pause intent** for that monitor.
  - Automatic suppression (`ActivityGate` during fullscreen games, presentations, or low battery) sets `wp.gateSuppressed` (`*util.SafeFlag`) and **must never mutate `mc.State.Paused`**.
  - `CmdNextAuto` checks `mc.IsSuppressed()` and skips auto-rotation while suppressed, whereas manual `CmdNext` / `CmdPrev` always execute.
  - When a user manually unpauses via the tray menu (`PauseAllMonitors` / `ToggleMonitorPause`), notify `wp.gate.NotifyUserUnpaused()` so the grace period starts and any active gate suppression releases immediately.

---

## 3. OS Activity Detection (`pkg/activity/`)

- Keep decision logic (`ClassifyWindowsState`, `IsFullscreenGeometry`, `ClassifyDarwinWindows`, `MatchesProcessList`, `ParseGpuEngineInstances`, `IsSustained3DLoad`) **100% pure and table-tested** in `pkg/activity/activity.go`.
- Keep OS syscall shells (`pkg/activity/windows.go`, `pkg/activity/darwin.go`) thin:
  - **Windows (`pkg/activity/windows.go`)**:
    - Use `CreateToolhelp32Snapshot` fallback (`processExeNameBySnapshot`) when `OpenProcess` is denied on elevated or anti-cheat protected game processes.
    - Check `(style & wsCaption) == wsCaption` (full bitmask equality, since `WS_CAPTION = WS_BORDER | WS_DLGFRAME`) when distinguishing titled windows from borderless fullscreen windows.
    - `PdhGetFormattedCounterArrayW` takes 5 arguments (`hCounter`, `dwFormat`, `lpdwBufferSize`, `lpdwBufferCount`, `ItemBuffer`).
  - **macOS (`pkg/activity/darwin.go`)**:
    - Require >=50% window or display area overlap before attributing a window to a display in `ClassifyDarwinWindows`.
    - When `CGWindowListCopyWindowInfo` indicates a BusyFullscreen window, only override with the allowlist exe name if `frontmostProcessName()` actually matches `cfg.GetGateGameProcesses()`.

