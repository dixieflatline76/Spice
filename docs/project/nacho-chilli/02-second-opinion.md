# Second Opinion: Project Nacho Chilli — Honest Feasibility Review

> **Author**: Claude Opus 4.6  
> **Date**: 2026-05-25  
> **Context**: Independent review of [01-initial-feasibility.md](01-initial-feasibility.md), cross-referenced against the Spice codebase architecture

## Verdict: FEASIBLE, BUT THE PLAN IS DANGEROUSLY OPTIMISTIC

The core *idea* is sound and forward-thinking. The ternary inference breakthrough is real, the performance numbers check out, and the alignment with Spice's zero-bloat philosophy is genuine. But the previous feasibility report reads like a sales pitch — it glosses over the hardest problems and stamps "HIGHLY FEASIBLE" on what is actually a **6-12 month R&D project with at least three hard unknowns**.

Here's the honest breakdown.

---

## 1. What the Previous Report Got Right

- **Ternary math is real.** The benchmarks are legitimate: 30-45 t/s on Apple Silicon M2 for sub-2B models, 55-82% energy reduction. This isn't hype.
- **CGO is the correct bridge.** HTTP IPC to a local llama.cpp server would add ~20ms latency per token and a process management nightmare. Static linking via CGO is the right call.
- **Thread capping (`NumCPU() - 2`) is correct.** This is the standard pattern and it works.
- **Fyne v2.5.0's `driver.NativeWindow` exists** and provides the HWND/NSWindow access you need.

---

## 2. What the Previous Report Got Wrong or Skipped

### A. The Click-Through Window Problem is Severely Underestimated

> **⚠️ CAUTION: This is the single highest-risk item in the entire project.**

The plan says: "Use `driver.NativeWindow` to extract HWND, apply `WS_EX_TRANSPARENT`, done."

**Reality:**
- Fyne's event loop **captures all mouse events** within the window bounds. Setting `WS_EX_TRANSPARENT` at the OS level creates a conflict: the OS says "this window is click-through" but Fyne's internal GLFW event loop still thinks it owns the window. You will get phantom events, broken hover states, and potentially deadlocked input queues.
- **Dynamic toggling** (transparent when idle, solid when clicking the sprite) requires you to strip and re-apply extended window styles *on the fly*. On Windows, this requires calling `SetWindowLongPtr` followed by `SetWindowPos` with `SWP_FRAMECHANGED` to force the window manager to re-evaluate. This can cause visible flicker.
- **On macOS**, `setIgnoresMouseEvents:YES` works cleanly, but you need to run the Objective-C call on the **AppKit main thread**, not from a random Go goroutine. Fyne's `RunNative` should handle this, but you need to verify it dispatches to the correct thread.
- **The hit-testing logic** ("if cursor is on a non-transparent sprite pixel, toggle back to solid") requires per-frame alpha sampling of the sprite at the cursor position. This is doable but it's a custom input pipeline you're building *on top of* Fyne's existing one. Fyne was not designed for this.

**Recommendation:** Build a standalone proof-of-concept for the transparent overlay window **before** touching the LLM integration. If this doesn't work cleanly on both Windows and macOS within 2 weeks, the entire "animated desktop companion" concept needs to pivot to a standard Fyne window (docked to screen edge, non-transparent). That's still a perfectly viable product — just not the Clippy/BonziBuddy aesthetic.

---

### B. The "1-2B Model" Intelligence Ceiling

The plan assumes a 1-2B ternary model can do reliable OS diagnostics, parse system metrics, generate correct shell commands, and output structured JSON.

**Reality:**
- A 1B ternary model is roughly equivalent in capability to **GPT-2 Large** (774M FP16). It can complete sentences, summarize short text, and do basic classification. It **cannot** reliably:
  - Generate correct, context-sensitive Windows PowerShell commands
  - Diagnose novel system failures from raw metrics
  - Maintain multi-turn conversational context beyond ~2K tokens
  - Produce consistently valid JSON without frequent schema violations
- A 2B ternary model is better (roughly Phi-2 tier), but still struggles with anything requiring genuine reasoning. It will hallucinate shell commands that look plausible but are subtly wrong — which is *worse* than no suggestion at all for a non-technical user.

**The "sarcastic art critic" use case is actually the best fit.** That's pure creative text generation with no correctness requirement. A 2B model can absolutely do witty commentary about a Rembrandt painting. The OS diagnostic assistant? That needs a 7B+ model minimum, which pushes you to 2-3GB RAM and 8-15 t/s on CPU — still viable but a very different resource profile.

> **ℹ️ Recommendation:** Start with the art critic persona *only*. It's the lowest-risk, highest-delight feature. The OS diagnostic agent should be Phase 2, gated behind benchmarking a 7B ternary model on minimum-spec hardware (8GB RAM laptops).

---

### C. CGO Build Complexity is Real but Manageable

The plan says: "Pre-compile static libraries, link via `#cgo` directives."

**What it doesn't say:**
- You already use CGO in Spice (Fyne requires it for OpenGL/GLFW). So you're not introducing CGO for the first time — you're adding a *second* C++ dependency to an already-complex build. Your `ci.yml` already fights with MinGW on Windows and Clang on macOS.
- `llama.cpp` alone pulls in ~150 C/C++ source files. Static linking it adds 30-60 seconds to every CI build.
- You need to decide: **submodule or vendored copy?** Submodule means tracking upstream `llama.cpp` breaking changes (they move fast — multiple breaking API changes per month). Vendored copy means you're maintaining a fork.
- The `bitnet.cpp` path is cleaner (smaller codebase, purpose-built for ternary) but has a much smaller community and fewer pre-trained models available.

**What I'd actually do:** Use `llama.cpp` with `TQ2_0` quantization. It has the largest ecosystem, the most hardware-optimized kernels, and the broadest model compatibility. Accept the build complexity tax. `bitnet.cpp` is technically superior for pure ternary but the model zoo is too thin right now.

---

### D. Scope Creep: This is Three Products, Not One

The brief defines three independent products:
1. **An animated desktop sprite** (transparent window, sprite sheets, state machine)
2. **A local LLM inference engine** (CGO, memory management, thread safety)
3. **An OS telemetry monitoring system** (gopsutil, threshold detection, remediation commands)

Each of these is individually a 2-3 month project for a solo developer. Together, with the integration glue, you're looking at 6-12 months of focused work. The previous report's four-phase roadmap makes it look like a weekend project.

---

## 3. Architecture Fit with Spice

Having reviewed the Spice architecture, here's the good news: **the plugin system is well-designed for this**.

| Aspect | Assessment |
| :--- | :--- |
| **Plugin registration** | Spice's plugin system uses `init()` self-registration. Nacho Chilli can register as a second plugin alongside Wallpaper. ✅ Clean. |
| **IPC with Wallpaper plugin** | The `ImageStore` already has an update channel (`GetUpdateChannel()`). Nacho Chilli can listen for wallpaper changes without coupling. ✅ Clean. |
| **Config management** | Flat-file YAML config fits the "no SQLite" constraint. ✅ Clean. |
| **Concurrency model** | Spice already manages 15+ goroutines with a documented lock hierarchy. Adding LLM inference threads is feasible *if* they stay on the CGO side and communicate via Go channels. ⚠️ Needs care. |
| **Binary size** | Current `spice.exe` is 65MB. Adding statically-linked `llama.cpp` will push it to ~80-90MB. The model weights (1-2GB) must be a separate download. ⚠️ Acceptable but changes distribution. |
| **CI complexity** | The CI already builds for Windows (MinGW) and macOS (Clang) with CGO. Adding a second C++ library doubles the C++ build matrix. ⚠️ Painful but not blocking. |

---

## 4. The Real Risk Matrix

| Risk | Severity | Mitigation |
| :--- | :--- | :--- |
| Transparent click-through window doesn't work cleanly in Fyne | **HIGH** | Build standalone PoC first. Have fallback to docked panel. |
| 1-2B model hallucinates dangerous shell commands | **HIGH** | Start with art critic only. Gate diagnostics behind 7B+ model validation. |
| CGO + llama.cpp doubles CI build time and complexity | **MEDIUM** | Use pre-built static libs cached in CI. Accept the tax. |
| Model distribution (1-2GB download on first launch) | **MEDIUM** | First-launch download with progress UI. Store in AppData. |
| Thread starvation during inference stutters Fyne UI | **LOW** | Thread capping + OS priority lowering. Well-understood pattern. |
| Users on 4GB RAM laptops can't run Nacho Chilli | **LOW** | Make Nacho Chilli an optional plugin that checks available RAM before loading. |

---

## 5. Honest Recommendation

> **ℹ️ Don't build all three products at once. Build the fun one first.**

### Phase 0: Transparent Window PoC (2 weeks)
Before writing any LLM code, prove the transparent animated sprite works on Windows and macOS. If it doesn't, pivot to a docked panel design early.

### Phase 1: Art Critic Companion (6-8 weeks)
- Integrate `llama.cpp` via CGO with a 2B ternary model
- Listen to the Wallpaper plugin's image change events
- When a museum piece appears, feed the artwork metadata into a "sarcastic art critic" system prompt
- Stream the response as animated text next to the sprite
- **This is the "wow" feature.** It's delightful, low-risk, and demonstrates the entire stack working end-to-end.

### Phase 2: Technical HUD (8-12 weeks, after Phase 1 ships)
- Add the keyboard-driven technical query overlay
- Benchmark 7B ternary models for OS diagnostic accuracy
- Implement the telemetry polling + remediation button system
- **Only build this if Phase 1 proves the engine is stable in production.**

### Phase 3: Ambient OS Monitor (future)
- The silent background telemetry interceptor
- Requires the most model intelligence and the most testing
- Ship this last, when you have real-world data on model reliability

---

## 6. Bottom Line

**Is it worth investing in?** Yes — but as a measured, phased R&D effort, not as a monolithic build. The ternary inference tech is genuinely transformative for local desktop AI. The art critic persona is a unique differentiator that no other wallpaper app has. The Spice plugin architecture can support it cleanly.

**Is the previous plan realistic?** No. It underestimates the transparent window challenge, overestimates sub-2B model intelligence for OS diagnostics, and bundles three independent R&D projects into one scope. Build the delightful thing first, prove it works, then expand.
