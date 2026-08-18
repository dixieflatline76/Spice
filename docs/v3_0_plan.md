# Spice v3.0 Plan & Roadmap

> **Status**: Approved Roadmap
> **Target Release Line**: v3.0.0 – v3.2.0

---

## 1. Executive Summary & Vision

Spice v3.0 is focused on making the app smarter, more respectful of your battery and gaming sessions, and adding highly requested features like dark mode for wallpapers and multi-monitor support. 

---

## 2. Pre-Requisite: Codebase Audit & Technical Debt Remediation

Before commencing work on the v3.0 features, the following technical debt and codebase inconsistencies must be resolved to ensure a clean foundation:

1. **Configuration Dead Code**: Delete the unused `GetHideStatusBar()` and its associated ghost preference keys.
2. **i18n & Dropdown Schemas**: Remove the unused and broken `ParseSmartFitMode()` string parser. Enforce strict integer-based index binding for all `schema.SelectItem` UI dropdowns to prevent localization round-tripping bugs.
3. **Goroutine Leaks**: Remove the permanently-idle `startEnrichmentWorker` stub and `enrichmentSignal` channel from the core `Plugin` struct.
4. **Documentation Drift**: Synchronize all architectural documentation (`architecture.md`, `creating_new_providers.md`, `creating_new_plugins.md`) to reflect the current declarative UI schemas and provider enum namespaces (`TypeCommunity`, `TypePersonal`, `TypeMuseum`). Remove duplicated blocks in the image pipeline deep dive.

---

## 3. Milestone 3.0: Engine Intelligence

The first major update focuses on making Spice completely invisible when you need your computer's full power.

- **Pause During Games & Presentations**: Stop rotating wallpapers or downloading images when you are running a full-screen game or giving a presentation.
- **Battery Saver**: Automatically pause rotation when your laptop drops below 20% battery.
- **Multi-Monitor Fingerprinting**: Let users set different wallpaper sources for different monitors (e.g., photos on the laptop screen, museums on the ultrawide), and remember those screens even if they get unplugged.
- **Dark Mode (Solar Filtering)**: Automatically stop showing bright, white wallpapers late at night to prevent eye strain.

---

## 4. Milestone 3.1: Aesthetics & Community

The second update focuses on making your desktop look incredible and letting you share your curation.

- **Art Walk Tours (.artwalk)**: Let users create, save, and share "guided tours" of museum art with their own sticky-note commentary attached to specific images.
- **Color Extraction (Desktop Ricing)**: Spice will figure out the primary colors of your current wallpaper and save them to a tiny JSON file. This lets Linux and Windows power-users automatically sync their window borders and terminal colors to match the wallpaper.

---

## 5. Milestone 3.2: Ecosystem Expansion

The final update of the v3.0 cycle focuses on supporting more obscure operating systems and third-party devs.

- **Linux Wayland Support**: Make Spice work natively with modern Linux compositors (swww, hyprpaper) without needing complex C-bindings.
- **Custom Scripts**: Let power-users write their own scrapers in Python or Bash to grab wallpapers from obscure websites, and feed them directly into Spice.

---

## 6. Milestone 4.0: Nacho Chilli (Desktop Companion)

The v4 release shifts Spice from a passive wallpaper manager into an active, ambient desktop ecosystem by introducing Nacho Chilli—a 90s Sega-style virtual assistant.

- **Ambient System Watchdog**: Nacho silently monitors your system resources (CPU, RAM, disk space) and pops up with sarcastic, helpful alerts when things go wrong.
- **Fine Art Critic**: When Spice sets a museum piece as your wallpaper, Nacho dons a painter's beret and offers sharp, cynical historical context about the artwork.
- **Decoupled Local AI**: Nacho is completely private and runs 100% locally. He connects seamlessly to your existing Ollama or LM Studio daemon via standard REST APIs, keeping Spice extremely lightweight.
- **Borderless 16-Bit UI**: Nacho floats on your desktop as a frameless, transparent pixel-art sprite that you can click right through when he's idle.

---

# 🚫 Explicit Anti-Goals (What we are NOT doing)

- **No Heavy AI Vision Models**: We will not embed massive AI models (like ONNX/MobileCLIP) to analyze images. It uses too much RAM and violates the app's promise of being lightweight.
- **No Mobile Apps**: We are staying 100% focused on desktop workstations. iOS and Android are too locked down for background wallpaper rotation anyway.
