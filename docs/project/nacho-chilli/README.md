# 🌶️ Project Nacho Chilli

> **Status**: Early Design & Brainstorming  
> **Goal**: An animated, local-first AI desktop companion plugin for Spice

Nacho Chilli is a planned Spice plugin that brings a sarcastic, animated pepper character to your desktop. When Spice sets a museum masterpiece as your wallpaper, Nacho Chilli wakes up and delivers witty, historically-accurate art commentary — powered entirely by a local LLM via [Ollama](https://ollama.com).

## Documents

| Document | Description |
| :--- | :--- |
| [01-initial-feasibility.md](01-initial-feasibility.md) | Initial engineering brief & feasibility analysis covering CGO integration, Fyne syscall hooks, and ternary inference |
| [02-second-opinion.md](02-second-opinion.md) | Honest second-opinion review identifying risks the initial plan underestimated |
| [03-mvp-brainstorm.md](03-mvp-brainstorm.md) | Backend strategy comparison (Ollama vs PicoClaw vs CGO) and concrete MVP architecture for the Art Critic persona |
| [04-technical-decisions.md](04-technical-decisions.md) | **Finalized decisions** — monorepo strategy, Ollama backend, Spicy Mode, preferences tab, and phasing |
| [05-spikes-and-assets.md](05-spikes-and-assets.md) | Required spike investigations and character asset specifications for AI generation |

## MVP Scope (Phase 1: Art Critic)

- Sarcastic-but-informative art commentary on museum wallpapers
- Opt-in **Spicy Mode 🌶️** for cheeky commentary on adult content (off by default, content-aware)
- Ollama as inference backend (no CGO, no build changes)
- Standard docked Fyne window with sprite + speech bubble
- Own **"Companion"** preferences tab via Spice's PanelSchema system

## Key Design Decisions

- **Repo**: Monorepo — Nacho Chilli ships compiled into the Spice binary (no dynamic plugin loading)
- **Backend**: Ollama (external HTTP dependency) — zero CGO complexity, automatic GPU support
- **Agent Framework**: Direct Ollama API for Phase 1; [PicoClaw](https://picoclaw.io) for Phase 2+ (tool calling, OS diagnostics)
- **UI**: Docked mini-panel, not transparent overlay (defer click-through to future phase)
- **Model**: User's choice via Ollama (recommended: `llama3.2:3b`)
- **Personality**: Content-aware — Spicy Mode activates only on adult-flagged images from sources like Wallhaven

## Future Phases

- **Phase 2**: Technical HUD — keyboard-driven query overlay, PicoClaw integration, OS diagnostics
- **Phase 3**: Ambient OS Monitor — silent telemetry interception, automated remediation suggestions
