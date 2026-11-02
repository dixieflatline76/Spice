# Decision Memo: Should We Proceed with the Activity Gate?

> **Question**: Objectively, should we proceed?
> **Inputs**: [`activity-gate-design.md`](activity-gate-design.md) v2 (five signals), [`activity-gate-implementation.md`](activity-gate-implementation.md) (TDD, ≥90% coverage), [`activity-gate-gpu-load-addendum.md`](activity-gate-gpu-load-addendum.md) (S5), [`activity-gate-risk-assessment.md`](activity-gate-risk-assessment.md)

---

## Verdict: YES — proceed, with one sequencing change (probe-first).

This is the rare feature where every objective criterion points the same direction. The analysis:

### 1. Value-per-risk is the best on the entire roadmap
- It is **ranked #1** in the objective priority analysis ([`feature-priority.md`](feature-priority.md)): highest user value (the #1 complaint class for background wallpaper tools — rotation interrupting games/presentations, battery drain) with the **lowest engineering risk** of all v3.0 items.
- Unlike Monitor Fingerprinting (Config/Store/UI upheaval) or Solar Filtering (derivative-cache hot-path risk), the gate is **purely additive**: a leaf package + one flag read in an existing gatekeeper + schema items. No store, no pipeline, no cache invalidation.

### 2. The engineering uncertainty is near-zero
- Every complex piece (hysteresis, ownership, grace, allowlist matching, all parsers/classifiers) is pure Go, exhaustively table-testable **in CI** under the 90% coverage gate.
- It reuses production-proven engine verbs (`CmdPause` per-`DevicePath` persistence, `RequestFetch` flag no-op, `Notifier`). Nothing new is trusted in the hot path.
- Failure modes are one-directional: missed detection ⇒ feature absent (safe); false positive ⇒ rotation pauses with a notification (annoying but harmless, and user-fixable via prefs).

### 3. The only real uncertainty is quantified and bounded
- Two platform unknowns remain (Windows FSO long tail; macOS CGWindowList behavior across spaces) — both **measurable in week 1** via the probe-first sequencing, and both **bounded to near-zero for anything the user lists (S3) or toggles (S4)**. Combined pre-hardware confidence ~85%; trending ~95% after the matrix.

### 4. Sequencing change that makes "yes" even stronger
**Build `activity_probe` (one-shot/watch/--gpu) FIRST**, before the engine gate. This converts the two unknowns from arguments into data before we commit engineering, and the probe itself is ~150 lines of build-tagged Go reusing existing `sysinfo` patterns. If a signal proves flaky on real hardware, we cut it and ship the remaining four — the plan degrades gracefully at every point.

### 5. Cost is modest and known
- ~3 commits, ~1,500–2,000 LOC including tests; the largest single file (~250 LOC logic) is the gate itself. No new dependencies (`pdh.dll`, `shell32`, `pmset`/`osascript` are system-provided; `golang.org/x/sys` already in [`go.mod`](../../go.mod)).

### Counter-arguments considered and why they don't overturn "yes"
| Counter | Rebuttal |
|:---|:---|
| "Monitor Fingerprinting has more user value" | True, but ~4× the architectural surface and it touches Config schema + store filtering + settings UI. Build it **second**, on the stable gate foundation (shared auto-trigger→Actor-command pattern). |
| "Heuristics can't be 100%" | Correct — which is why the design makes the *deterministic* signals (allowlist S3, manual S4) first-class and demotes heuristics to votes. Detection confidence for user-named games is 100%. |
| "Ship solar filtering instead (same milestone)" | It requires ProcessingFlag + derivative-invalidation design on the 4K hot path — the riskiest change class in the codebase. Wrong first feature. |
| "Wait for hardware data before deciding" | That's what probe-first **is**: the decision point after the probe costs only ~150 LOC, and the probe is useful regardless (documented dev tool). |

### Conditions attached (already in the plan)
1. Probe-first sequencing (new todo #10, moved to the top of execution).
2. Release gate: the §5 hardware matrix must run before PR.
3. Graceful degradation: any signal that fails calibration is cut, never patched around.
4. macOS fallback stance is pre-agreed: if cgo enumeration is problematic, macOS v1 ships S3+S4+battery only.

**Recommendation: proceed immediately, starting with the probe.**