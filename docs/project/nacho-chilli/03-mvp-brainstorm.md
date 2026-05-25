# Nacho Chilli: Art Critic MVP — Backend Strategy & Architecture Brainstorm

> **Authors**: Claude Opus 4.6 + Gemini 3.1 Pro (collaborative)  
> **Date**: 2026-05-25  
> **Status**: Active brainstorm — decisions pending

## The Actual Vision (Corrected)

You want **new Clippy** — an animated desktop companion that lives inside the Spice ecosystem. The previous plan got lost in syscall details and forgot the soul of the project:

1. **PicoClaw** — an ultra-lightweight Go agent framework (<10MB, single binary, Ollama-native) — was the original routing engine idea. It handles tool calling, prompt routing, and LLM provider management. The previous plan barely mentioned it.
2. **The UI approach is flexible.** You want a character, not necessarily a transparent overlay. A docked panel, a sidebar, a popup bubble — all acceptable.
3. **The inference backend is flexible.** You're not married to CGO-linking bitnet.cpp. Ollama, AMD GAIA, or any local runtime that handles GPU/CPU dispatch automatically is fine.

With that corrected, here are three backend strategies ranked by engineering effort vs. payoff.

---

## Backend Strategy Comparison

### Option A: Ollama as External Dependency (RECOMMENDED for MVP)

```
┌─────────────────────────────────────────┐
│           SPICE (Go Binary)             │
│                                         │
│  ┌─────────────┐   ┌────────────────┐   │
│  │  Wallpaper   │   │  Nacho Chilli  │   │
│  │  Plugin      │──▶│  Plugin        │   │
│  │              │   │                │   │
│  │  (image      │   │  • Art context │   │
│  │   metadata)  │   │  • Personality │   │
│  └─────────────┘   │  • Sprite UI   │   │
│                     └───────┬────────┘   │
│                             │            │
│                     HTTP localhost:11434  │
│                             │            │
└─────────────────────────────┼────────────┘
                              ▼
                    ┌──────────────────┐
                    │     Ollama       │
                    │  (separate app)  │
                    │                  │
                    │  AMD / NVIDIA /  │
                    │  Intel / CPU     │
                    │  auto-detected   │
                    └──────────────────┘
```

**Why this wins for MVP:**
- **Zero CGO complexity.** Pure Go HTTP client. No build matrix changes. No static linking. Your CI doesn't change at all.
- **Hardware universality.** Ollama already handles AMD ROCm, NVIDIA CUDA, Intel oneAPI, Apple Metal, and CPU fallback. You don't write a single line of GPU dispatch code.
- **Model management is free.** `ollama pull llama3.2:3b` handles download, caching, and quantization selection. Users already know how to use it.
- **Streaming tokens over HTTP** adds ~5-10ms latency per token. For an art critic generating a 50-word quip, that's imperceptible. This is NOT a latency-sensitive chatbot.
- **The downside:** Users must install Ollama separately. But Ollama is a one-click installer on Windows/macOS and already has ~20M+ installs. This is not a hard ask.

**Go integration:**
```go
import "github.com/ollama/ollama/api"

client := api.ClientFromEnvironment() // auto-connects to localhost:11434
req := &api.ChatRequest{
    Model: "llama3.2:3b",
    Messages: []api.Message{
        {Role: "system", Content: nachoPersonality},
        {Role: "user", Content: artworkPrompt},
    },
    Stream: &streaming,
}
// Stream tokens directly into Fyne text widget
client.Chat(ctx, req, func(resp api.ChatResponse) error {
    nachoBubble.Append(resp.Message.Content)
    return nil
})
```

---

### Option B: PicoClaw as Middleware (Best for Future Phases)

```
┌──────────────────────────────────────────────────┐
│                SPICE (Go Binary)                 │
│                                                  │
│  ┌─────────────┐   ┌─────────────────────────┐   │
│  │  Wallpaper   │   │     Nacho Chilli        │   │
│  │  Plugin      │──▶│     Plugin              │   │
│  └─────────────┘   │                         │   │
│                     │  ┌───────────────────┐  │   │
│                     │  │    PicoClaw Core   │  │   │
│                     │  │  (embedded as lib) │  │   │
│                     │  │                   │  │   │
│                     │  │  • Tool routing   │  │   │
│                     │  │  • Prompt chains  │  │   │
│                     │  │  • MCP support    │  │   │
│                     │  │  • Provider mgmt  │  │   │
│                     │  └────────┬──────────┘  │   │
│                     └───────────┼─────────────┘   │
│                                 │                 │
└─────────────────────────────────┼─────────────────┘
                                  ▼
                        ┌──────────────────┐
                        │     Ollama       │
                        └──────────────────┘
```

**Why PicoClaw matters:**
- PicoClaw is written in Go. It's <10MB. It's designed exactly for this: a lightweight agent brain that routes prompts, manages tool calls, and talks to Ollama (or any OpenAI-compatible API).
- For the **art critic MVP**, PicoClaw is overkill — you don't need tool calling or MCP for generating witty commentary. A direct Ollama API call is simpler.
- For **Phase 2 (OS diagnostics)**, PicoClaw becomes essential. Its built-in file I/O, shell execution, and tool routing mean you don't have to build your own agent framework from scratch. You embed PicoClaw's core as a Go library and wire its tools to Nacho Chilli's UI.

**Recommendation:** Start with Option A (direct Ollama). When you reach Phase 2, import PicoClaw's routing logic as a library dependency. PicoClaw's architecture is designed to be embedded — it's just Go packages.

---

### Option C: Direct CGO (bitnet.cpp / llama.cpp)

The previous plan's approach. Skip Ollama entirely, statically link the inference engine.

**When this makes sense:**
- You want zero external dependencies (true single-binary distribution)
- You're targeting ternary-only models for absolute minimum resource usage
- You're okay with 2-3 months of CGO plumbing before any visible feature

**When it doesn't:**
- For an MVP art critic? Massive overkill. The CGO tax buys you nothing — Ollama already does CPU inference faster than you'll be able to hand-tune it.
- You lose GPU acceleration on NVIDIA/AMD hardware. `llama.cpp` via CGO *can* link CUDA/ROCm but the build matrix becomes nightmarish.

**Verdict:** Revisit in Phase 3 if and only if you need a fully self-contained binary with zero dependencies. For now, it's an engineering vanity project.

---

## MVP Architecture: Nacho Chilli Art Critic

### Scope: What Ships in Phase 1

| Feature | Description |
| :--- | :--- |
| **Nacho Chilli sprite** | Static or simple animated character rendered in a standard Fyne window (docked to bottom-right corner, always-on-top, small). No transparent overlay for MVP. |
| **Art commentary** | When the Wallpaper plugin sets a museum piece, Nacho Chilli receives the metadata (artist, title, year, museum, medium) via an in-memory Go channel and generates a 2-3 sentence sarcastic-but-educational commentary. |
| **Speech bubble** | Text streams into a styled Fyne text widget next to the sprite, token-by-token, synced with a simple mouth-open/mouth-closed animation. |
| **Ollama backend** | Connects to `localhost:11434`. If Ollama isn't running, Nacho Chilli displays "💤 Nacho Chilli is sleeping... Install Ollama to wake him up!" |
| **Personality system prompt** | A carefully crafted system prompt that defines Nacho Chilli's voice: sarcastic, informative, occasionally irreverent, always educational. |
| **Toggle in Settings** | A simple on/off toggle in Spice Preferences under a new "Companion" tab. |

### What Does NOT Ship in Phase 1

- ❌ Transparent click-through overlay
- ❌ OS telemetry / diagnostics
- ❌ Tool calling / shell execution
- ❌ PicoClaw integration
- ❌ CGO / embedded inference
- ❌ Multiple personas

### Proposed File Structure

```
pkg/
  nachochilli/                        # INNER RING (pure Go — zero Fyne, zero Spice imports)
    nachochilli.go                    # Plugin entry point, lifecycle (init/activate/deactivate)
    interfaces.go                     # Ports: ImageChangeNotifier, ArtworkMetadata, etc.
    schema.go                         # PanelSchema declarations (pure Go structs)
    personality.go                    # System prompts, persona definitions
    ollama_client.go                  # Ollama API wrapper, streaming, health check
    art_context.go                    # Transforms artwork metadata into prompts
    sprite_state.go                   # Sprite state machine (IDLE, THINKING, SPEAKING)

  nachochilli/ui/                     # OUTER RING — Adapter (Fyne + Spice bridge)
    window.go                         # Fyne window: sprite + speech bubble + controls
    settings.go                       # Fyne rendering of PanelSchema → widgets
    bridge.go                         # Wires Spice ImageStore events → nachochilli interfaces
```

### The Personality Prompt (Draft)

```
You are Nacho Chilli, a tiny animated pepper who lives on the user's desktop
inside a wallpaper app called Spice. You are a self-proclaimed art critic
with strong opinions, a dry wit, and a genuine love of art history.

When shown artwork metadata, you deliver a 2-3 sentence reaction that is:
- Sarcastic but never mean-spirited
- Historically accurate (mention real facts about the artist or period)
- Occasionally self-aware that you're a cartoon pepper judging masterpieces

Keep responses under 60 words. Never use emoji. Never break character.
```

### Example Interaction

**Input metadata:**
```json
{
  "title": "The Night Watch",
  "artist": "Rembrandt van Rijn",
  "year": "1642",
  "museum": "Rijksmuseum",
  "medium": "Oil on canvas"
}
```

**Nacho Chilli's response (streamed):**
> "Ah, Rembrandt's The Night Watch. The man painted 34 militiamen and somehow made it look like they're all late for the same dinner reservation. Fun fact: it was actually called 'The Company of Captain Frans Banning Cocq' but nobody could be bothered to remember that. Fair."

---

## Open Decisions

> **1. Ollama as hard dependency or optional enhancement?**
> - **Hard dependency:** Nacho Chilli only works with Ollama installed. Simpler to build, clearer messaging.
> - **Optional:** Nacho Chilli also supports a "canned responses" fallback mode with pre-written commentary for known artworks (no LLM needed). More work, but works for users who don't want to install Ollama.
>
> **2. Window style for MVP:**
> - **Docked mini-panel** (bottom-right corner, always-on-top, ~300x200px Fyne window)
> - **Inline in tray/notification** (speech bubble appears as a system notification)
> - **Settings panel widget** (Nacho Chilli lives inside the Preferences window only)
>
> **3. Model recommendation for users:**
> - `llama3.2:3b` — Best quality-to-speed ratio for art commentary. ~2GB download, runs on any hardware.
> - `phi3:mini` — Smaller (1.4GB), faster, slightly less creative.
> - `gemma2:2b` — Google's 2B model, good at structured creative text.
>
> **4. PicoClaw timing:**
> - Embed PicoClaw in Phase 1 to future-proof the architecture?
> - Or keep Phase 1 dead simple (direct Ollama calls) and add PicoClaw in Phase 2?
