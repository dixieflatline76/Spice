# Activity Gate — Objective Risk & Confidence Assessment

> **Question**: What is the objective risk and confidence of this plan working across macOS and Windows?
> **Scope of assessment**: [`plans/activity-gate-design.md`](activity-gate-design.md) v2 (four-signal) + [`plans/activity-gate-implementation.md`](activity-gate-implementation.md) (TDD)
> **Assessment date**: October 2026

---

## 1. Bottom line

| Dimension | Windows | macOS | Overall |
|:---|:---|:---|:---|
| **Confidence feature ships & works as designed** | ~85% | ~75% | ~80% |
| **Confidence gate logic itself is correct** (pause/resume/ownership/hysteresis) | ~98% | ~98% | ~98% |
| **Confidence game detection catches the user's games** | ~85% with S3+S4; ~60% heuristics-only | ~70% | ~80% combined |

**Statement in plain terms**: The *engineering* (gate correctness, wiring, persistence, safety) is near-certain — it's pure Go logic, fully TDD-able in CI, reusing battle-tested engine verbs, and every platform failure mode degrades to "no detection" rather than "wrong behavior". The *uncertainty lives exclusively in the two platform detectors*, and it is bounded by the process-allowlist signal (S3) and the manual toggle (S4), which convert "probabilistic detection" into "guaranteed detection for anything the user names". The plan's verification matrix on real hardware is what converts the residual risk from unknown to measured before release.

---

## 2. Confidence: what we're sure about (and why)

| # | High-confidence item | Basis |
|:---|:---|:---|
| 1 | **Gate state machine correctness** | Pure Go against `FakeDetector`; hysteresis/ownership/grace are exhaustively table-testable in CI (tests 2.1–2.19). No OS involvement in this logic. |
| 2 | **Pause plumbing works** | Reuses existing `CmdPause` → `togglePause()` → per-`DevicePath` persistence, already production-proven by the manual Pause/Play feature ([`monitor_controller.go`](../../pkg/wallpaper/monitor_controller.go)). |
| 3 | **Fetch suppression works** | `RequestFetch` already no-ops on a flag; adding `gateSuppressed` is one early-return. Existing fetch/deactivate test suites must stay green. |
| 4 | **No regression to core engine** | The gate holds no locks while dispatching, touches no store/pipeline code, no `NeedsRefresh` (no derivative invalidation). Blast radius is strictly additive. |
| 5 | **Windows battery detection** | `GetSystemPowerStatus` is a stable, 25-year-old API; parsing is pure and 100%-table-tested. Risk ≈ nil. |
| 6 | **macOS battery detection** | `pmset -g batt` parse mirrors the existing `system_profiler` shell-out pattern already shipping in [`pkg/sysinfo/macos.go`](../../pkg/sysinfo/macos.go). |
| 7 | **TDD + 90% coverage plan is achievable** | All detection logic extracted to pure functions; only literal syscall shells are uncovered (documented §7). |
| 8 | **Windows Game Mode + allowlist guarantee** | `GetForegroundWindow` → PID → `QueryFullProcessImageName` is a deterministic, documented API path. If the game is in the list, detection is 100%. |

## 3. Risk register — quantified

| # | Risk | Platform | Likelihood | Impact | Residual after mitigation | Mitigation in plan |
|:---|:---|:---|:---|:---|:---|:---|
| R1 | FSO/borderless games not flagged by shell state (S1 blind spot) | Windows | **High** (by design of modern Windows) | Medium | **Low** — S2 geometry + S3 allowlist + S4 toggle close it; worst case user adds 1 exe | Four-signal design; matrix row verified on hardware |
| R2 | Geometry heuristic (S2) false positive — some borderless non-game app looks fullscreen | Windows/macOS | Medium | Low | Low — safe failure direction (rotation pauses; user can disable or un-favorite); pref to disable S2 | 98%-cover + no-caption requirement; safe-direction argument |
| R3 | cgo `CGWindowListCopyWindowInfo` complexity/behaviour differences (Sonoma+ / Sequoia permission prompts, spaces) | macOS | Medium | Medium | Medium — this is the single largest platform unknown; mitigated by building S2 fallback + allowlist + toggle so the feature is usable even if S1 classification is imperfect | Layer-0 + owner exclusions; smoke `watch` mode on real hardware; frontmost-app osascript fallback path |
| R4 | macOS fullscreen classification misses borderless-windowed games (common on Mac for indies) | macOS | Medium-High | Medium | Medium-low — S3 allowlist (exe/app name) is deterministic; S4 always available | Allowlist + toggle; docs list known Mac game app names |
| R5 | App Store sandbox/review concerns for cgo window enumeration | macOS | Low | Medium | Low — Spice already ships cgo (hotkey) and holds Accessibility guidance; window *reading* (not posting events) is not privileged; still, if review pushes back, ship macOS v1 with allowlist+toggle only | Fallback: detector degrades to BusyNone; feature remains valuable via S3/S4 + battery |
| R6 | Gate pause persists after crash (stale paused state) | Both | Low | Low | Very low — nightly `ResetIfIdle()` (test 2.14); worst case one manual unpause | R9 + notification copy tells user state is auto |
| R7 | Flapping (loading screens, alt-tab, FSO transitions) | Windows | Medium | Low | Very low — 2-in/3-out hysteresis + grace period; consequence of flap is brief pause/resume with notification | Tests 2.2/2.5/2.10 |
| R8 | Gate fights user intent (user unpauses, gate re-pauses) | Both | Medium if unmitigated | Medium | Very low — ownership model (R3/R4) + grace (R6) + S4 manual override as trump card | Tests 2.7/2.8/2.10/2.17 |
| R9 | `SHQueryUserNotificationState` missing/changed on exotic Windows builds | Windows | Very low | Low | Nil — lazy-proc `.Find()` guard → BusyNone; other signals still active | §7 pattern from `GetDpiForSystem` |
| R10 | 90% coverage target slips due to unexpected OS coupling | Both | Low | Low | Low — discipline is "extract pure function"; escape hatch documented; worst case the pure logic still passes | §7 exceptions list is the only allowed uncovered code |
| R11 | Linux stub ships "do nothing" and users on Linux notice (currently unsupported platform anyway) | Linux | N/A | N/A | None — Linux is pre-v3.2 officially; stub mirrors `pkg/hotkey` precedent | Out-of-scope list |
| R12 | Multi-monitor games (spanning) classified inconsistently | Both | Low | Low | Low — S2 checks ≥98% of *any* single monitor; spanning covered by S1/S3/S4 fallbacks | Matrix row |

## 4. What would change the numbers

- **Better**: running the §5 verification matrix early on the dev box **before** building the engine gate would let us cut Commit 1 to only the signals that actually work on real hardware (e.g., if S2's borderless detection proves flaky on Windows 11 24H2, we'd know in week 1 and lean on S3/S4 sooner). Recommended: do step 1.2's smoke util *first*.
- **Worse**: if Apple's App Store review objects to cgo window enumeration (R5), macOS falls back to S3+S4+battery only — still a shippable, honest feature ("pause for your listed games or when you toggle Game Mode"), but automatic fullscreen detection on macOS becomes opt-in-via-list.
- **Neutral**: Linux stub changes nothing for these numbers.

## 5. Residual unknowns that only hardware can answer

1. Exact `QUNS_*` distribution across FSO games on Windows 11 24H2 (S1 breadth).
2. Whether `WS_CAPTION`-absence + 98%-cover reliably excludes maximized browser/IDE in real setups (S2 precision).
3. `CGWindowList` behaviour across macOS fullscreen spaces & Stage Manager (S1 macOS).
4. osascript frontmost-app latency (needs to stay <10ms for 60s polling; expected fine but measured only on hardware).

These are exactly the items the `activity_probe watch` mode + matrix (Commit 3, §5) exist to measure — and the plan gates release on running them.

## 6. Verdict

- **Engineering risk (breakage, regression, concurrency):** negligible. The design adds a leaf module + one flag read in an existing gatekeeper; all complex logic is CI-testable; failure modes are safe-direction (miss detection ⇒ feature absent; false positive ⇒ paused rotation with a notification).
- **Detection-efficacy risk (does it catch games?):** moderate on macOS heuristics, moderate-low on Windows heuristics, **but bounded to near-zero for anything the user explicitly lists (S3) or toggles (S4)**. Combined confidence that "the gate pauses during the user's gaming sessions" ≈ **80–85%** pre-hardware-verification, trending to **~95%** after the matrix run and (if needed) allowlist tuning.
- **Recommendation**: proceed, with one adjustment — pull the `activity_probe` smoke utility forward to the very first implementation step so detector calibration data is in hand before the engine gate is written.
