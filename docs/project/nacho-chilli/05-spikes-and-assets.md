# Nacho Chilli: Spikes & Character Asset Requirements

> **Date**: 2026-05-25  
> **Status**: Pre-implementation preparation

---

## 1. Required Spikes

Spikes are time-boxed investigations (1-2 days max each) to de-risk unknowns before committing to the full build.

### Spike 1: Fyne Secondary Window (CRITICAL)

**Question:** Can Fyne create a secondary always-on-top window that runs alongside Spice's systray-driven headless app, without interfering with the main event loop or window lifecycle?

**Why it matters:** Spice today is a systray app with no persistent visible window (just the settings dialog). Nacho Chilli needs a small, always-visible window that:
- Stays on top of other apps
- Doesn't steal focus
- Doesn't block the systray or settings window
- Can be independently shown/hidden via toggle

**Test:** Create a minimal Fyne app with a systray icon + a small secondary window. Verify both coexist, window stays on top, and hiding/showing doesn't crash.

**Time box:** 1 day  
**Risk if it fails:** High — need to rethink the entire UI surface (notification-based? embedded in settings?)

---

### Spike 2: llama-server Subprocess + HTTP Streaming

**Question:** Can Spice reliably manage `llama-server` as a subprocess — start, health-check, stream tokens via HTTP, and kill on exit — without orphaned processes or port conflicts?

**Why it matters:** Spice needs to lifecycle-manage llama-server: start it when Nacho Chilli is enabled, stream tokens into a Fyne text widget, and ensure it dies when Spice exits (even on crash). Port allocation must avoid conflicts.

**Test:** 
- Start `llama-server` via `exec.Command` with a small model, stream a chat response via the OpenAI-compatible `/v1/chat/completions` endpoint
- Pipe streamed tokens into a Fyne `widget.Label` via `Refresh()`
- Kill Spice mid-stream — verify llama-server also terminates (no orphan)
- Start Spice with no model downloaded — verify graceful "downloading model..." UX

**Time box:** 1 day  
**Risk if it fails:** Low — subprocess management is well-understood Go; HTTP streaming is standard

---

### Spike 3: Sprite Sheet Animation in Fyne

**Question:** Can we animate a sprite sheet at 8-12 FPS in a Fyne `canvas.Image` without visible stutter or excessive CPU usage?

**Why it matters:** Fyne's canvas wasn't designed for game-style sprite animation. We need to measure whether swapping `canvas.Image.Resource` on a ticker causes flickering, memory churn, or dropped frames.

**Test:**
- Load a 4-frame sprite sheet
- Swap frames via `time.Ticker` at 100ms intervals
- Measure CPU usage and visual smoothness
- Test on both Windows and macOS if possible

**Time box:** 0.5 days  
**Risk if it fails:** Medium — fallback is static character with expression changes only (no smooth animation)

---

### Spike 4: ImageStore Event Subscription

**Question:** Can `pkg/nachochilli/` receive wallpaper change events from Spice without importing Spice packages, via the interface contract defined in `interfaces.go`?

**Why it matters:** Validates the hexagonal decoupling. The bridge file in `pkg/nachochilli/ui/bridge.go` needs to adapt `ImageStore.GetUpdateChannel()` to the `nachochilli.ImageChangeNotifier` interface and pass artwork metadata through cleanly.

**Test:**
- Define `ImageChangeNotifier` interface in `pkg/nachochilli/`
- Write a bridge adapter that wraps `ImageStore`
- Verify metadata (title, artist, year, museum, medium, content rating) arrives correctly
- Verify Nacho Chilli ignores non-museum images when "Museums only" is selected

**Time box:** 0.5 days  
**Risk if it fails:** Low — this is standard Go interface wiring

---

### Spike Summary

| Spike | Risk | Time | Blocks |
| :--- | :--- | :--- | :--- |
| Fyne secondary window | **High** | 1 day | Everything — no window, no MVP |
| llama-server subprocess | Low | 1 day | Commentary feature |
| Sprite animation | Medium | 0.5 days | Animation quality |
| ImageStore events | Low | 0.5 days | Event-driven commentary |

**Total spike budget: 3 days.** Run Spike 1 first — if it fails, we need a different UI approach before anything else matters.

---

## 2. Character Asset Requirements

### Overview

Nacho Chilli is an animated chilli pepper character. For the MVP, he needs sprite assets for three animation states. Assets should be created as **individual PNG frames** with transparent backgrounds.

### Technical Specifications

| Property | Requirement |
| :--- | :--- |
| **Format** | PNG with alpha transparency |
| **Resolution** | 256×256 pixels per frame (renders at ~128px in the window, 2x for HiDPI/Retina) |
| **Color mode** | RGBA, 32-bit |
| **Background** | Fully transparent (alpha = 0) |
| **Style** | Consistent across all frames — same line weight, color palette, proportions |

### Animation States (MVP)

#### State 1: IDLE (Breathing Loop)
The default resting state. Nacho Chilli is alive but not doing anything specific.

- **Frames:** 4–6
- **Animation:** Subtle breathing (slight body scale), occasional blink, maybe a small sway
- **Loop:** Yes, continuous
- **Mood:** Relaxed, slightly bored, waiting

#### State 2: THINKING (Processing)
Triggered when Ollama receives a prompt and is generating tokens. Shows Nacho Chilli is "working on it."

- **Frames:** 4–6
- **Animation:** Eyes looking up or to the side, maybe a thought bubble puff, slight head tilt, steam from the pepper top
- **Loop:** Yes, until first token arrives
- **Mood:** Contemplative, concentrating

#### State 3: SPEAKING (Delivering Commentary)
Active while text is streaming into the speech bubble.

- **Frames:** 4–6
- **Animation:** Mouth open/closed cycle (synced to token stream), slight body movement, expressive eyes
- **Loop:** Yes, while tokens are streaming
- **Mood:** Animated, opinionated, engaged

### Optional States (Post-MVP)

| State | Trigger | Description |
| :--- | :--- | :--- |
| **SLEEPING** | Ollama unavailable | Eyes closed, zzz particles, slumped posture |
| **LAUGHING** | Spicy Mode commentary | Head thrown back, tears of joy |
| **SHOCKED** | Adult content detected | Wide eyes, blush, dramatic gasp |
| **WAVING** | First launch / greeting | Friendly wave, introduction pose |

### File Naming Convention

```
assets/nachochilli/
  idle_01.png
  idle_02.png
  idle_03.png
  idle_04.png
  thinking_01.png
  thinking_02.png
  thinking_03.png
  thinking_04.png
  speaking_01.png
  speaking_02.png
  speaking_03.png
  speaking_04.png
```

### Character Design Notes

- **He's a chilli pepper** — red/green body with a stem/cap on top
- **Expressive face** — large eyes and a visible mouth are essential for the animation states to read clearly at small sizes
- **Arms optional** — small arms/hands can add expressiveness but aren't required at 128px render size
- **Consistent silhouette** — the character outline should stay roughly the same across all frames so he doesn't "jump" during animation
- **Keep it simple** — at 128px on screen, fine detail is invisible. Bold shapes, clear expressions, high contrast

### Google AI Studio Prompt Suggestions

When generating with AI, try prompting for consistency across a sheet:

> *"Create a sprite sheet of a cartoon chilli pepper character named Nacho Chilli. He has large expressive eyes, a small mouth, and a green stem cap on top. Flat 2D style, bold outlines, transparent background. Show 4 frames of an idle breathing animation in a horizontal strip. 256x256 pixels per frame, PNG."*

Generate each state separately to maintain consistency, then validate that the character proportions match across states before committing.
