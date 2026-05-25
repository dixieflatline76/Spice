# Nacho Chilli: Technical Decisions & Design Spec

> **Date**: 2026-05-25  
> **Status**: Approved decisions from brainstorming rounds 01–03

This document captures the finalized technology choices and design decisions for the Nacho Chilli MVP. It supersedes any conflicting guidance in earlier brainstorming docs.

---

## 1. Repository Strategy

**Decision: Monorepo (hardlocked to Spice)**

Nacho Chilli ships as a compiled-in plugin inside the main Spice binary. No separate repository, no dynamic loading.

**Rationale:**
- Go's `plugin` package only works on Linux — unusable for Windows/macOS
- Dynamic loading alternatives (shared libs, IPC) would require 2-3 months of infrastructure before any feature work
- Nacho Chilli needs tight access to Spice internals: `ImageStore` update channel, `provider.Image` metadata, the Fyne app instance, and the systray
- The Spice plugin architecture uses compile-time `init()` self-registration — Nacho Chilli follows the same pattern as the Wallpaper plugin

**Future extraction:** If Spice eventually builds a real dynamic plugin system (likely WebSocket/IPC-based), `pkg/nachochilli/` can be extracted into its own module with minimal surgery because the package boundaries are kept clean from day one.

**Coupling strategy:** Follows Spice's existing hexagonal pattern. `pkg/nachochilli/` is the **inner ring** — pure Go, zero Fyne imports, zero Spice imports. It declares ports (interfaces for image events, schema structs for settings). `pkg/nachochilli/ui/` is the **outer ring adapter** — it imports Fyne for rendering and wires Spice's concrete implementations to the inner ring's interfaces.

**Package layout:**
```
pkg/
  nachochilli/                        # INNER RING (pure Go — zero Fyne, zero Spice imports)
    nachochilli.go                    # Plugin entry point, lifecycle (init/activate/deactivate)
    interfaces.go                     # Ports: ImageChangeNotifier, ArtworkMetadata, etc.
    schema.go                         # PanelSchema declarations (pure Go structs)
    personality.go                    # System prompts, persona definitions, adult mode logic
    llm_client.go                     # HTTP client for llama-server + subprocess lifecycle
    art_context.go                    # Transforms artwork metadata into prompts
    sprite_state.go                   # Sprite state machine (IDLE, THINKING, SPEAKING)

  nachochilli/ui/                     # OUTER RING — Adapter (Fyne + Spice bridge)
    window.go                         # Fyne window: sprite + speech bubble + controls
    settings.go                       # Fyne rendering of PanelSchema → widgets
    bridge.go                         # Wires Spice ImageStore events → nachochilli interfaces
```

---

## 2. Inference Backend

**Decision: llama-server sidecar (self-contained, no CGO, no external deps)**

Nacho Chilli ships a pre-built `llama-server` binary (the official HTTP server from the llama.cpp project) alongside `spice.exe`. Spice manages it as a subprocess and communicates via OpenAI-compatible HTTP API on localhost.

**Why llama-server sidecar:**
- **Zero external dependencies.** User enables Nacho Chilli, it just works. No "install Ollama first."
- **No CGO, no purego FFI.** Standard Go `exec.Command` + HTTP client. Dead simple.
- **Process isolation.** If inference crashes, Spice keeps running and restarts the server.
- **Battle-tested.** `llama-server` is a first-class llama.cpp component, used by millions.
- **Same API as Ollama.** If a user already has Ollama, Nacho Chilli can optionally talk to that instead — the HTTP client code is identical.

**GPU support — ship the Vulkan build:**

| Binary Variant | GPU Coverage |
| :--- | :--- |
| **`llama-server-vulkan` (default)** | **AMD + NVIDIA + Intel — universal** |
| `llama-server-cuda` | NVIDIA only (marginally faster than Vulkan on NVIDIA) |
| `llama-server-rocm` | AMD only (native) |
| `llama-server-metal` | Apple Silicon only |
| `llama-server` (CPU) | Everything, no GPU acceleration |

MVP ships the **Vulkan build** — one binary covers AMD, NVIDIA, and Intel. macOS ships the **Metal build** for Apple Silicon.

**Licensing:** llama.cpp and llama-server are MIT. Models have their own licenses (Llama 3.2 requires "Built with Llama" attribution; Gemma and Phi-3 are MIT). Attribution displayed in Nacho Chilli's Preferences.

**Model distribution:**
- First time user enables Nacho Chilli → auto-download recommended model (~1.5-2GB) to `%APPDATA%/Spice/Models/`
- Progress bar in the Companion settings tab
- Model selection configurable (default: `llama-3.2-3b` Q4 quantized)

**Distribution layout:**
```
spice.exe
llama-server.exe                      # Vulkan build (Windows) / Metal build (macOS)
assets/models/                        # Downloaded on first launch
  llama-3.2-3b-Q4_K_M.gguf
```

---

## 3. UI & Output Strategy

**Decision: Docked Fyne window with speech bubble for MVP. Voice (TTS) in future phase.**

Nacho Chilli renders in a small docked Fyne window (bottom-right corner, always-on-top, ~300x200px). No transparent click-through overlay for Phase 1.

**Rationale:**
- Transparent click-through in Fyne is the single highest-risk item (Fyne's GLFW event loop conflicts with OS-level `WS_EX_TRANSPARENT`)
- A docked panel is still delightful — animated sprite + speech bubble
- Defer the Clippy-style floating overlay to a future phase, gated behind a standalone PoC

**Output modality roadmap:**

| Phase | Primary Output | Fallback |
| :--- | :--- | :--- |
| **MVP** | Speech bubble (streaming text) | — |
| **Phase 2** | Voice (TTS — OS-native or Piper) | Speech bubble (accessibility) |

The speech bubble is never removed — it remains the accessibility fallback when TTS is added. Voice becomes the primary output, speech bubble becomes the caption track. The inner ring generates text; the outer ring adapter decides how to present it (text, voice, or both).

---

## 4. Personality System

### 4.1 Base Persona

Nacho Chilli is a tiny animated pepper with a dry wit and genuine love of art history. His voice is:
- **Sarcastic but never mean-spirited** — he roasts the art, not the user
- **Historically accurate** — always includes a real fact about the artist or period
- **Self-aware** — occasionally acknowledges he's a cartoon pepper judging masterpieces
- **Concise** — responses stay under 60 words, no emoji, never breaks character

### 4.2 Spicy Mode 🌶️ (Adult Content)

**Decision: Opt-in adult personality mode, off by default**

When Spice is connected to sources that serve adult or risqué content (e.g., Wallhaven NSFW queries), Nacho Chilli can deliver cheeky, innuendo-laden commentary instead of his default G-rated persona. This mode is:

- **Off by default** — must be explicitly enabled in the Nacho Chilli preferences tab
- **Labeled clearly** — the toggle is named something like *"Spicy Mode 🌶️"* with a subtitle: *"Let Nacho Chilli make cheeky comments on adult content"*
- **Content-aware** — the personality switch is triggered by the image's content rating metadata (e.g., Wallhaven's `purity` flag), not globally. When Spicy Mode is on AND the current wallpaper is flagged as adult/sketchy, Nacho Chilli uses an alternate system prompt. For SFW images, he stays in his normal persona regardless.

**System prompt injection (conceptual):**
```
[Base personality prompt]

The current wallpaper has been flagged as adult content by its source.
You may now be playfully cheeky, use tasteful innuendo, and make sly
observations. Stay witty and clever — think "raised eyebrow" not
"locker room." Never be vulgar, crude, or explicit. You're a
sophisticated pepper with impeccable taste, even when the art isn't.
```

**Why this design:**
- No risk of inappropriate content leaking to users who haven't opted in
- Content-rating awareness means Nacho Chilli doesn't randomly turn cheeky on a Monet
- The "Spicy Mode" branding is on-theme with the Spice product family
- Wallhaven already provides `purity: sfw | sketchy | nsfw` metadata per image

---

## 5. Preferences Tab

**Decision: Nacho Chilli gets its own top-level tab in Spice Preferences**

Following the existing Spice pattern where the Wallpaper plugin injects its own settings tabs (Online, Local, Display, General), Nacho Chilli registers a **"Companion"** tab via the same `schema.PanelSchema` system.

**Planned settings:**

| Setting | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| **Enable Nacho Chilli** | Toggle | Off | Master on/off switch |
| **Model** | Dropdown | `llama-3.2-3b-Q4` | Which model Nacho Chilli uses for generation |
| **Spicy Mode 🌶️** | Toggle | Off | Enable cheeky commentary on adult content |
| **Auto-comment** | Toggle | On | Automatically comment when wallpaper changes (vs. manual trigger only) |
| **Comment on** | Multi-select | Museums only | Which provider categories trigger commentary (Museums / All / None) |

The tab follows Spice's existing **Hexagonal Architecture** — settings are declared as pure Go `schema.PanelSchema` structs in the inner ring (`pkg/nachochilli/schema.go`), and the outer ring adapter (`pkg/nachochilli/ui/settings.go`) renders them as Fyne widgets. The bridge file registers the tab with Spice's settings manager.

---

## 6. Agent Framework

**Decision: Direct Ollama API for Phase 1. PicoClaw for Phase 2+.**

Phase 1 (Art Critic) does not need tool calling, MCP, or agent routing. A single `client.Chat()` call with a system prompt is sufficient. Adding PicoClaw now would be premature complexity.

When Phase 2 (OS Diagnostics) begins, PicoClaw's Go packages will be imported as a library dependency to provide:
- Tool routing (file I/O, shell execution)
- Prompt chaining
- MCP support
- Provider fallback management

---

## 7. Summary of Decisions

| Decision | Choice | Alternatives Deferred |
| :--- | :--- | :--- |
| **Repo strategy** | Monorepo (compiled into Spice) | Separate repo + dynamic plugins |
| **Inference backend** | llama-server sidecar (self-contained, Vulkan) | Ollama, purego, CGO |
| **Agent framework** | Direct llama.cpp inference (Phase 1) | PicoClaw (Phase 2+) |
| **UI approach** | Docked Fyne window | Transparent overlay (future PoC) |
| **Adult content** | Opt-in Spicy Mode, content-aware | Always-on, always-off |
| **Settings** | Own "Companion" tab via PanelSchema | Inline in existing tabs |
| **Model default** | `llama3.2:3b` (user-configurable) | Hardcoded model |
