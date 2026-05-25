# ENGINEERING BRIEF & FEASIBILITY REPORT: PROJECT NACHO CHILLI

> **Author**: Gemini 3.1 Pro  
> **Date**: 2026-05-25  
> **Context**: Initial feasibility analysis exploring CGO-linked ternary inference as the backend

## 1. Executive Summary
Project Nacho Chilli proposes embedding a 100% local, sub-1.5GB 1.58-bit ternary LLM (via `bitnet.cpp` or `llama.cpp`) into the Go-based Spice ecosystem to serve as an animated, low-latency desktop companion. The goal is to provide intelligent OS telemetry analysis and interactive technical support without bloat, GPU requirements, or cloud APIs.

**Feasibility Verdict:** **HIGHLY FEASIBLE** with specific architectural guardrails. The shift to ternary quantization (where floating-point multiplication is replaced by integer addition) unlocks the CPU performance needed for real-time (30-40 t/s) token generation. However, strict attention must be paid to CGO toolchain management, Fyne window manipulation, and thread contention.

---

## 2. Technical Feasibility Analysis

### A. Syscall Integration: Fyne Window Handles & Mouse Passthrough
Fyne abstracts native OS windows to maintain cross-platform purity, which traditionally blocks low-level hacks like click-through alpha masking. 

**Solution:** As of Fyne v2.5.0, the `driver.NativeWindow` interface was officially introduced. This allows us to safely bypass the abstraction and extract the raw OS window pointers.

**Implementation Path:**
1. Type-assert the Fyne window: `native, ok := win.(driver.NativeWindow)`
2. Invoke `native.RunNative(func(ctx any))` to enter the platform-specific thread context.
3. **Windows:** Extract `ctx.HWND` (as `uintptr`). Use Go's `syscall` package to call `SetWindowLongPtr` and append `WS_EX_LAYERED | WS_EX_TRANSPARENT`.
4. **macOS:** Extract `ctx.NSWindow`. Use CGO/Objective-C bindings to call `[window setIgnoresMouseEvents:YES]`.
5. **Hit-Testing:** Fyne's canvas can capture mouse motion. If the cursor enters the bounds of a non-transparent sprite pixel, we dynamically toggle the OS window flags back to solid to intercept the click.

### B. CGO Toolchain Constraints & Build Matrix
Statically linking a complex C/C++ engine into a pure Go binary breaks standard `GOOS/GOARCH` cross-compilation. `CGO_ENABLED=1` is mandatory.

**Solution:** Pre-compile the inference engine into static libraries (`.a` / `.lib`) per platform during a pre-build step, then link them via `#cgo` directives.

**Implementation Path:**
1. **Build Pipeline Update:** Update the Spice `Makefile` and GitHub Actions `ci.yml` to run CMake for `bitnet.cpp` (or `llama.cpp`) before running `go build`.
2. **Mac (ARM64):** Standard Clang toolchain builds `libllama.a` and `libggml.a`. The `#cgo` directive links `-lc++`.
3. **Windows (AMD64):** Use MSYS2/MinGW-w64 in CI to build static libraries. Linking requires `-lstdc++`. (Avoid MSVC to prevent ABI mismatches with CGO).
4. **Directory Structure:** Place prebuilt static libraries in `libs/darwin_arm64/` and `libs/windows_amd64/` and use build tags to load the correct `LDFLAGS`.
5. **Memory Management:** Ensure the C++ engine uses memory-mapped files (`mmap`) to load the `.gguf` weights directly from disk into RAM, completely bypassing the Go Garbage Collector.

### C. Thread Collision & Power Draw (Scheduler Starvation)
Go's runtime scheduler assumes it controls the CPU. When a CGO call invokes C++ code that spins up 8 OpenMP threads hitting 100% CPU utilization, the Go scheduler and the Fyne UI render loop will starve, causing the UI animation to stutter heavily.

**Solution:** Intentionally throttle the C++ engine and segregate thread domains.

**Implementation Path:**
1. **Thread Capping:** Configure the inference engine to use `runtime.NumCPU() - 2` threads. Leaving at least 1-2 physical cores entirely free guarantees the Go scheduler and Fyne's OpenGL render loop remain highly responsive.
2. **Yielding:** Go 1.21+ handles CGO thread blocking well (it parks the goroutine and spawns a new OS thread for Go execution), but the OS CPU scheduler still decides who gets cycles.
3. **OS Priority (Optional):** In the CGO bridging code, apply `SetThreadPriority(THREAD_PRIORITY_BELOW_NORMAL)` (Windows) or adjust `nice` values (Unix) to the C++ inference threads. This ensures Fyne UI repaints always pre-empt the LLM token generation.

---

## 3. Implementation Roadmap

### Phase 1: Engine Validation & CGO Bindings
*   **Goal:** Compile a minimal Go CLI binary that loads a 1.58B ternary `.gguf` file via a CGO wrapper and streams tokens to stdout.
*   **Steps:** 
    *   Fork/Submodule `llama.cpp` (with `TQ1_0`/`TQ2_0` support) or `bitnet.cpp`.
    *   Write a minimal `llm.go` with `#cgo CXXFLAGS` and `#cgo LDFLAGS`.
    *   Implement an asynchronous Go channel that receives `(string, error)` from the C++ callback.

### Phase 2: UI Transparent Canvas & OS Hooking
*   **Goal:** Create a transparent, click-through Fyne window that renders a test sprite.
*   **Steps:**
    *   Implement Fyne v2.5.0 `driver.NativeWindow`.
    *   Write Windows (`user32.dll`) and macOS (Objective-C) wrappers to toggle mouse passthrough.

### Phase 3: The Telemetry Loop & IPC Integration
*   **Goal:** Connect the LLM to OS telemetry without blocking.
*   **Steps:**
    *   Implement a background Go ticker polling `gopsutil` for thermal, disk, or memory spikes.
    *   When an anomaly is detected, format a strict JSON prompt and dispatch it to the CGO inference channel.
    *   Sync token output to Fyne sprite states (`STATE_SPEAKING`, `STATE_IDLE`).

### Phase 4: Action Sandboxing (Human-in-the-Loop)
*   **Goal:** Ensure the agent cannot execute arbitrary destructive commands.
*   **Steps:**
    *   The LLM prompt instructs the model to return remediations as structured payload: `{"thought": "...", "command": "ipconfig /release"}`.
    *   Fyne renders the text stream, then displays an "Execute" button containing the command payload.

## 4. Open Questions
> **Model Distribution:** A 1-1.5GB binary is too large for the current Spice Github Releases/Winget distribution model. Should the plugin download the `.gguf` model dynamically on first launch into `%APPDATA%\Spice\Models`, or should we ship a separate "Nacho Chilli Installer"?
>
> **Ternary Model Selection:** Standard ternary models excel at generic text generation, but do you have a specific fine-tuned 1.58B model in mind that is explicitly trained for OS telemetry and diagnostic logic? If not, we may need to build an adapter/prompt-tuning layer to force it to output reliable JSON.
