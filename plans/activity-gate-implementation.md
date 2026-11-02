# Activity Gate — Scratch Implementation Plan (TDD, four-signal detection)

> **Branch**: `feature/activity-gate` (created off `main` @ `eef6d1b`)
> **Scope**: Milestone 3.0 — "Pause During Games & Presentations" + "Battery Saver" as one shared gate.
> **Design basis**: [`plans/activity-gate-design.md`](activity-gate-design.md) **v2** (four-signal detection)
> **Methodology**: **Test-Driven Development (RED → GREEN → REFACTOR)** for every unit of code.
> **Quality gate**: **Minimum 90% statement coverage on all NEW code** (`pkg/activity`, `pkg/wallpaper/activity_gate.go`, new config getters, new UI section) — verified with `go test -coverprofile` + `go tool cover -func` before every commit. Pre-existing engine file coverage must not regress.
> **Status**: DRAFT — implementation checklist, not yet started.

---

## 0. Delivery decisions locked in

1. **Ship on by default** (`pause_when_busy=true`, `battery_saver=true`).
2. **No-battery platforms silently skip** battery logic (Windows `BatteryLifePercent==255`, macOS `pmset` missing, Linux stub).
3. **One PR** for fullscreen + battery + allowlist (shared ticker + state machine), staged internally in 3 commits.
4. **Four-signal detection** (v2 revision — direct answer to "will this actually detect gaming?"):
   - **S1** Shell notification state (`SHQueryUserNotificationState` on Windows; `QUNS_BUSY/3/4/7` = busy; `NOT_PRESENT/QUIET_TIME` ignored)
   - **S2** Foreground fullscreen geometry heuristic (borderless games)
   - **S3** Foreground **process allowlist** (deterministic per-game guarantee via user-maintained list)
   - **S4** Manual **Game Mode** tray toggle (always-on override)
5. **TDD cycle per behavior**: write failing test → minimal implementation → refactor with coverage check. No production file committed without its test file in the same commit.
6. **Coverage verification** before every commit:
   ```bash
   go test -coverprofile=cover.out ./pkg/activity/... ./pkg/wallpaper/...
   go tool cover -func=cover.out   # every NEW function must show >= 90%
   ```

---

## TDD Ground Rules

- **RED**: Write the test first against the intended API. Must fail (compile or assert). Confirm RED by running.
- **GREEN**: Implement the minimum to pass. Confirm GREEN.
- **REFACTOR**: Clean up without coverage drop below 90% on new code.
- **Fake-first**: All gate logic tested through a `FakeDetector` (scripted `UserBusy`/`Battery`/`ForegroundProcess`); all detection logic extracted into **pure functions** so ≥90% is achievable **without mocking Win32/cgo**.
- **OS call shells** (the few `syscall`/`exec` lines) are the only accepted uncovered lines — isolated single-expression wrappers, documented in §7.
- **Table-driven tests** are the house style.

---

## 1. Commit 1 — `pkg/activity` port package (pure Go, no engine changes)

### New files
| File | Build tag | Contents |
|:---|:---|:---|
| `pkg/activity/activity.go` | none | `Detector` interface, `BusyState`/`BatteryState` enums + `String()`, and **all pure detection functions** (below) |
| `pkg/activity/detector_stub.go` | `//go:build !windows && !darwin` | No-op detector |
| `pkg/activity/windows.go` | `//go:build windows` | LazyDLL shells (`shell32!SHQueryUserNotificationState`, `user32!GetForegroundWindow/GetWindowRect/GetWindowLong/GetWindowThreadProcessId`, `kernel32!GetSystemPowerStatus`, `QueryFullProcessImageName`) — each delegating to pure parsers |
| `pkg/activity/darwin.go` | `//go:build darwin` | cgo `CGWindowListCopyWindowInfo` shell + `exec pmset` shell + `osascript` frontmost-app shell — delegating to pure parsers/classifier |
| `pkg/activity/activity_test.go` + per-platform `_test.go` | matching tags | Tables (see TDD steps) |

### Contract
```go
type Detector interface {
    UserBusy() BusyState          // BusyNone | BusyPresentation | BusyFullscreen
    Battery() BatteryState        // OnAC | OnBattery{Pct} | NoBattery
    ForegroundProcess() (name string, ok bool)  // for allowlist matching (S3)
}
```

### Pure functions (all in `activity.go`, all 100% table-testable)
| Function | Maps | Notes |
|:---|:---|:---|
| `ParseUserNotificationState(v int) BusyState` | Win32 S1 | busy: 1 (BUSY), 3 (D3D_FULL_SCREEN), 4 (PRESENTATION_MODE), 7 (APP); **ignore 2 (NOT_PRESENT) & 6 (QUIET_TIME)**; else BusyNone |
| `ParsePowerStatus(acLine, lifePercent int) BatteryState` | Win32 battery | `acLine==1`→OnAC; `255`→NoBattery; else OnBattery{pct} |
| `ParsePmsetOutput(out string) BatteryState` | macOS battery | `discharging N%`→OnBattery; `AC Power`→OnAC; missing→NoBattery |
| `IsFullscreenGeometry(winRect, monitorRects []Rect, styleFlags uint32) bool` | Win32 S2 | true iff covers ≥98% of any monitor AND `WS_CAPTION` bit absent (borderless). Table-test the 98% + style math |
| `MatchesProcessList(name, allowlist string) bool` | S3 | case-insensitive, trims spaces, comma-separated, matches basename (`eldenring.exe`) or contains-app (`Keynote`); empty list → false |
| `ClassifyWindow(owner, layer, frame, display, spiceOwner string) BusyState` | macOS S1 | layer 0, owner ≠ Spice, owner ∉ {Dock, WindowServer, SystemUIServer, Control Center}, frame covers ≥98% of a display → BusyFullscreen |
| `CombineBusy(s1, s2, s3 bool) BusyState` | decision rule | any true → BusyFullscreen (conservative severity), precedence: presentation if only S1 presentation |

### TDD sequence

| Step | RED (test first) | GREEN |
|:---|:---|:---|
| 1.1 | Contract test: `FakeDetector` satisfies `Detector`; enums distinct; `String()` values | `activity.go` interface + enums |
| 1.2 | Table: `ParseUserNotificationState` — all 8 QUNS values incl. ignored 2/6 + unknown | parser |
| 1.3 | Table: `ParsePowerStatus` — AC / 0% / 100% / 255 / edge | parser |
| 1.4 | Table: `ParsePmsetOutput` — `discharging 42%`, `AC Power`, `–1%`, malformed, empty | parser |
| 1.5 | Table: `IsFullscreenGeometry` — exact cover, 97.9% vs 98.1%, multi-monitor, captioned window (maximized w/ WS_CAPTION → false), popup-borderless → true | classifier |
| 1.6 | Table: `MatchesProcessList` — exact, case-insensitive, basename of full path, comma+space list, empty list, partial-name guard (no accidental substring: `notepad` must not match `csnotepad.exe`) | matcher |
| 1.7 | Table: `ClassifyWindow` — layer-0 full cover → busy; layer>0 (menus/Spotlight) → none; owner==Spice → none; system exclusions → none; partial cover → none | classifier |
| 1.8 | Table: `CombineBusy` — each combination incl. presentation-only | combiner |
| 1.9 | `detector_stub_test.go`: stub returns BusyNone/NoBattery/`ok=false` | stub |
| 1.10 | `windows_test.go` / `darwin_test.go`: composition tests with fake syscall results (structs passed to pure funcs) | platform detectors |

### Acceptance
- Every pure function + stub ≥ 90% (parsers target 100%); uncovered lines limited to §7 list.
- `go build ./...` green on Windows (dev box); `go vet` clean.

---

## 2. Commit 2 — Engine gate: `pkg/wallpaper/activity_gate.go`

### State machine (extends v1 with allowlist + manual toggle)
```go
type ActivityGate struct {
    plugin          *Plugin
    detector        activity.Detector
    stop            chan struct{}
    stopOnce        sync.Once
    busyStreak, clearStreak int
    engaged         bool
    manualEngaged   bool  // S4 Game Mode pref
    gatePausedIDs   map[int]bool
}
```

### Decision rules (each maps to ≥1 test)
| # | Rule |
|:---|:---|
| R1a | Engage after **2 consecutive** busy samples (busy := detector `UserBusy()!=BusyNone`) |
| R1b | Engage immediately when battery `!OnAC && pct <= threshold` |
| R1c | Engage immediately when `ForegroundProcess()` matches allowlist (S3) |
| R1d | Engaged whenever Game Mode pref is on (S4) — overrides all release rules |
| R2 | Release after **3 consecutive** clear samples **and** battery cleared (AC, or pct ≥ threshold+10) **and** Game Mode off |
| R3 | On engage: snapshot `IsMonitorPaused(id)` ∀ monitors → dispatch `CmdPause` all → record flipped IDs; set `gateSuppressed` |
| R4 | On release: re-dispatch `CmdPause` **only** to gate-owned IDs; clear suppression. Never touch user-paused monitors |
| R5 | Notify exactly once per transition via `wp.manager.NotifyUser` (message differs: game detected / battery / game mode on-off) |
| R6 | Grace period: ignore busy for `N` min after user-initiated unpause (default 2 min; grace does NOT apply to S3 allowlist or S4 manual — those are explicit intent) |
| R7 | Prefs off → gate inert for the corresponding trigger (busy prefs off ignores R1a/1c; battery off ignores R1b; Game Mode pref still honored as manual) |
| R8 | `Stop()` idempotent drain, like `stopNightlyRefresh` |
| R9 | `ResetIfIdle()` clears engaged state (nightly sweep for crash recovery) |

### TDD sequence — one RED test per step

| Step | RED test | GREEN |
|:---|:---|:---|
| 2.1 | `TestGate_Construction` — inert at rest | struct + ctor |
| 2.2 | `TestGate_EngagesAfterTwoBusySamples` | R1a |
| 2.3 | `TestGate_EngagesOnLowBattery` | R1b |
| 2.4 | `TestGate_DoesNotEngageWhileOnAC` | battery precondition |
| 2.5 | `TestGate_ReleasesAfterThreeClearSamples` | R2 |
| 2.6 | `TestGate_BatteryReleaseMargin` (20%→24% no release; →30% begins) | R2 margin |
| 2.7 | `TestGate_OwnsOnlyWhatItPaused` | R4 |
| 2.8 | `TestGate_DoesNotUnpauseUserPausedMonitors` | R4 negative |
| 2.9 | `TestGate_NotifiesOncePerTransition` | R5 |
| 2.10 | `TestGate_GracePeriodAfterManualUnpause` (busy suppressed during grace; allowlist + game-mode still engage) | R6 |
| 2.11 | `TestGate_DisabledByPrefs` | R7 |
| 2.12 | `TestGate_SuppressesAndRestoresFetch` (engage → `RequestFetch` no-op; release → works again) | suppression |
| 2.13 | `TestGate_StopDrains` + double-stop | R8 |
| 2.14 | `TestGate_ResetIfIdle` | R9 |
| 2.15 | `TestGate_AllowlistEngagesImmediately` — fake detector returns `("cs2.exe", true)` → engage on first tick even with `UserBusy()==BusyNone` (windowed game) | R1c |
| 2.16 | `TestGate_AllowlistMatchParsing` (delegates to `MatchesProcessList`; end-to-end through gate with pref) | R1c |
| 2.17 | `TestGate_GameModeToggleEngagesAndHolds` — pref on → engaged immediately; detector clear → still engaged; pref off → release countdown | R1d |
| 2.18 | `config_test.go`: round-trips + defaults for 6 new prefs | config.go |
| 2.19 | const uniqueness test for new keys | const.go |

### Wiring (surgical, additive)
| File | Change |
|:---|:---|
| [`wallpaper.go`](../../pkg/wallpaper/wallpaper.go) | `gate *ActivityGate`, `gateSuppressed *util.SafeFlag`; start in `Activate()`, stop in `Deactivate()` under `downloadMutex`; `RequestFetch` + `FetchNewImages` early-return on suppression |
| [`const.go`](../../pkg/wallpaper/const.go) | 6 pref keys + `ActivityGateTick`, engage/release samples, battery release margin, default game-mode off |
| `config.go` | `Get/SetPauseWhenBusy`, `Get/SetBatterySaver`, `Get/SetBatterySaverLevel`, `Get/SetGameMode`, `Get/SetGateGameProcesses`, `Get/SetGateGraceMinutes` |
| `ui_builder.go` | "Activity" section: `PauseWhenBusy` + `BatterySaver` BoolItems, battery `SelectItem` (10/15/20/25%), game-process `TextItem` (placeholder `cs2.exe, eldenring.exe`), grace `TextItem{IsNumeric:true}`. **No `NeedsRefresh`.** |
| `ui.go` / tray schema | Checkable "Game Mode" tray item (`MenuSchema{IsChecked}`) reading/writing `GameModePrefKey`, followed by tray rebuild via existing path (`RebuildTrayMenu`) — called through Fyne-safe path |

### Acceptance
- 19 RED/GREEN steps green; `activity_gate.go` ≥ 90% (pure logic vs fakes); full `go test ./pkg/wallpaper/...` green; pre-existing suites untouched-green.

---

## 3. Download suppression detail

- Dedicated `wp.gateSuppressed *util.SafeFlag` (NOT reusing `interrupt`, which belongs to shutdown semantics).
- `RequestFetch` gains one early-return: `if wp.gateSuppressed != nil && wp.gateSuppressed.Value() { return }` — placed before the Favorites exception (during a game, even cheap Favorites scans should be paused).
- In-flight downloads finish; only new fetches blocked. `FetchNewImages` entry: same check.

---

## 4. Commit 3 — i18n + docs + smoke util + verification matrix run

| Item | Detail |
|:---|:---|
| i18n | Gate notifications (game/battery/game-mode variants), section title, help texts, placeholders. `make gen-i18n` + `make check-i18n` green |
| Smoke util | `cmd/util/activity_probe`: `one-shot` + `watch` (1s loop printing busy state + foreground process + battery). Used to run the §5 detection matrix on real hardware |
| Docs | `docs/user_guide.md` "Automatic Pausing" section incl. detection matrix + allowlist how-to + common game exe names; `docs/v3_0_plan.md` roadmap tick; `docs/internal_developer_context.md` actor-table addition (gate goroutine + gateSuppressed flag) |
| Final QA | `go test ./...`, `golangci-lint run`, coverage report ≥90% new code, matrix results attached to PR |

---

## 5. Detection verification matrix (manual, Windows dev box + macOS)

| Scenario | Expected | Signal |
|:---|:---|:---|
| Exclusive-D3D game fullscreen | Busy | S1 |
| FSO "Fullscreen" game (Win10/11 default) | Busy | S1 or S2 |
| Manual borderless-windowed game | Busy | S2 or S3 |
| Windowed game, not allowlisted | Not busy — documented limitation; add exe to list | S3 |
| Browser F11 / YouTube fullscreen | Busy | S1 |
| PowerPoint presenter mode | Busy | S1 |
| Locked / screensaver | **Not** busy | S1 exclusion |
| Quiet hours | **Not** busy | S1 exclusion |
| Allowlisted game process (windowed) | Busy (immediate) | S3 |
| Game Mode toggle on | Busy always | S4 |
| On battery 15%, threshold 20% | Engaged | Battery |
| On AC 15% | Not engaged | Battery |
| Desktop PC (no battery) | Battery trigger inert | — |

---

## 6. Out of scope (explicit)

- Per-monitor targeted pause (game on monitor 2 pauses only monitor 2) — deferred to post-3.0.0.
- Monitor Fingerprinting, Solar Filtering — separate features.
- Linux detector implementations — stub now; sway/hypr/KWin IPC + UPower at v3.2.
- Tray UI additions beyond the Game Mode toggle.

---

## 7. Accepted uncovered lines (ONLY exceptions to 90%)

| File | Uncovered construct | Why |
|:---|:---|:---|
| `pkg/activity/windows.go` | `proc.Call(...)`, `proc.Find()` guards, `QueryFullProcessImageName` syscall, `GetForegroundWindow` chain | Requires live Win32 session; all mapping/matching is in 100%-covered pure funcs |
| `pkg/activity/darwin.go` | `exec.Command("pmset"...)/("osascript"...)`, cgo `CGWindowListCopyWindowInfo` block | Live-session only; parsing/classification fully covered |
| `cmd/util/activity_probe` | entire (dev tool) | Manual verification utility |
| Everything else | none | ≥90% mandatory |

---

## 8. Risk register

| Risk | Mitigation |
|:---|:---|
| FSO/borderless games missed by shell state alone | S2 geometry + S3 allowlist + S4 manual toggle close the gap; matrix in §5 verifies each path on hardware |
| Geometry heuristic false positives (any maximized-borderless app looks fullscreen) | 98%-cover + no-`WS_CAPTION` requirement; false-positive consequence is only a paused rotation (safe failure direction) — user can disable S2 via pref if ever annoying (`GateGeometryHeuristicPrefKey` optional; default on) |
| Allowlist false positives (substring matching) | Guarded matcher: basename compare, case-insensitive, no substring unless explicit; table test 1.6 |
| Gate pause persists across crash | Nightly `ResetIfIdle()` (test 2.14) |
| Flapping (loading screens, alt-tab) | 2-in/3-out hysteresis + grace (tests 2.2/2.5/2.10) |
| Fighting user pause intent | Ownership model (tests 2.7/2.8) |
| Coverage pressure → syscall mocking | Forbidden; pure-func extraction is the escape hatch (§7 documents the only exceptions) |

---

## 9. Implementation order (TDD checklist)

- [ ] 1.1 RED/GREEN — Detector contract + enums + String()
- [ ] 1.2 RED/GREEN — `ParseUserNotificationState`
- [ ] 1.3 RED/GREEN — `ParsePowerStatus`
- [ ] 1.4 RED/GREEN — `ParsePmsetOutput`
- [ ] 1.5 RED/GREEN — `IsFullscreenGeometry`
- [ ] 1.6 RED/GREEN — `MatchesProcessList`
- [ ] 1.7 RED/GREEN — `ClassifyWindow`
- [ ] 1.8 RED/GREEN — `CombineBusy`
- [ ] 1.9 RED/GREEN — stub detector + test
- [ ] 1.10 RED/GREEN — windows/darwin detectors (+ composition tests)
- [ ] Commit 1: coverage check ≥90% new code → commit
- [ ] 2.1–2.19 RED/GREEN — gate rules R1a–R9, one test per step
- [ ] Wiring: wallpaper.go / const.go / config.go / ui_builder.go / tray Game Mode toggle
- [ ] `TestRequestFetch_GateSuppressed` + existing suites green
- [ ] Commit 2: coverage check ≥90% → commit
- [ ] i18n → `make gen-i18n` → `make check-i18n`
- [ ] `cmd/util/activity_probe` (one-shot + watch)
- [ ] Docs (user_guide matrix, v3_0_plan, internal_developer_context)
- [ ] Run §5 matrix on hardware; record results
- [ ] Final: `go test ./...`, lint, coverage report → PR
