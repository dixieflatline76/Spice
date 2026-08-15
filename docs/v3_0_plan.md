# Spice v3.0 Feature Plan

> **Status**: Planning — Not yet started  
> **Branch**: TBD (create `feature/v3.0` when ready)

---

## Feature 1: Display Registry & Per-Query Monitor Routing

### Problem
Users with multi-monitor setups cannot control which queries/collections appear on which display. All queries rotate across all monitors equally.

### Design

#### Display Fingerprinting

Each platform already exposes a stable hardware identifier:

| Platform | Source | Implementation |
| :--- | :--- | :--- |
| **Windows** | `DevicePath` from `IDesktopWallpaper::GetMonitorDevicePathAt` (EDID-based, e.g. `\\?\DISPLAY#DEL41A8#...`) | `pkg/wallpaper/windows.go` |
| **macOS** | `NSScreen.localizedName` + pixel dimensions (via CGO) | `pkg/wallpaper/macos.go` |
| **Linux** | Xrandr connector name (e.g. `DP-1`, `HDMI-A-1`) | `pkg/wallpaper/linux.go` |

Fingerprint format: `Name + "_" + WxH` (e.g. `"Dell U2720Q_3840x2160"`).

#### Data Model

```go
// KnownDisplay represents a display that has been seen at least once.
type KnownDisplay struct {
    Fingerprint string    `json:"fingerprint"` // Stable key
    Name        string    `json:"name"`        // Human-readable name
    Resolution  string    `json:"resolution"`  // "3840x2160"
    LastSeen    time.Time `json:"last_seen"`
}
```

#### Query Routing

`ImageQuery` gains one new field:

```go
type ImageQuery struct {
    // ... existing fields ...
    TargetDisplays []string `json:"target_displays,omitempty"` // Empty = all displays
}
```

When a `MonitorController` pulses for a new image, the candidate pool filters to queries where `TargetDisplays` is empty (all monitors) or contains the current monitor's fingerprint.

### Proposed Changes

- `pkg/wallpaper/display_registry.go` [NEW]: `DisplayRegistry` with JSON persistence, register/lookup, deduplication
- `pkg/wallpaper/display_registry_test.go` [NEW]: Fingerprint, registration, persistence, query filtering tests
- `pkg/wallpaper/resolution.go`: Add `Fingerprint()` method to `Monitor`
- `pkg/wallpaper/config.go`: Add `TargetDisplays` to `ImageQuery`, add `DisplayRegistry` persistence
- `pkg/wallpaper/wallpaper.go`: Register monitors on `Activate()`, filter queries by display target
- `pkg/wallpaper/ui_builder.go`: Add "Target Displays" multi-select control per query item

---

## Feature 2: Art Walk (Shareable Museum Tours)

### Concept

Art Walks are curated, ordered collections of museum artworks with personal commentary. Users create guided tours through their favorite pieces across multiple world museums, with personal notes on each artwork. Tours are exportable as `.artwalk` files and shareable via Discord, Reddit, or email.

**Key design decisions:**
- **Museum-only providers** (Rijksmuseum, Met, AIC, SMK, NPM, Cleveland, Getty, Wikimedia) — all public domain, permanent URLs, zero authentication
- **Pure file export/import** — no in-app browse wall, no cloud gallery, no UGC moderation infrastructure required
- **Full rich content** — title, description, author, per-stop curator notes, tuning data all included in the shared file
- **App Store safe** — same model as opening a `.docx` or `.m3u` file. Apple/Microsoft have no grounds to classify local file import as UGC

### `.artwalk` YAML Schema

```yaml
version: "1.0"
walk:
  title: "Northern Renaissance: Light & Shadow"
  description: "A guided walk through masterpieces exploring how painters revolutionized light."
  author: "Karl"
  created_at: "2026-08-15"

stops:
  - provider: "Rijksmuseum"
    image_id: "SK-A-2344"
    title: "The Milkmaid"
    artist: "Johannes Vermeer"
    year: "1660"
    url: "https://lh3.googleusercontent.com/..."
    product_url: "https://www.rijksmuseum.nl/en/collection/SK-A-2344"
    note: "Notice how the light falls only on her hands and the bread."
    tuning:
      crop_anchor: "center"
      framing_mode: "virtual_frame"
      frame_size: 0.85

  - provider: "MetMuseum"
    image_id: "437133"
    title: "Self-Portrait with a Straw Hat"
    artist: "Vincent van Gogh"
    year: "1887"
    url: "https://images.metmuseum.org/..."
    product_url: "https://www.metmuseum.org/art/collection/search/437133"
    note: "Van Gogh painted this during his Paris period."
    tuning:
      crop_anchor: "face_center"
      framing_mode: "virtual_frame"
```

The word **"stops"** instead of "items" reinforces the guided tour metaphor.

### Architecture

Art Walk lives in a new `pkg/artwalk/` package (pure Go, no Fyne):

```
pkg/artwalk/
├── artwalk.go       # ArtWalk, Stop structs, YAML marshal/unmarshal
├── artwalk_test.go  # Parse/serialize round-trip tests
├── manager.go       # Load/save/list/delete walks from GetAppDir()/artwalks/
└── manager_test.go  # Manager tests
```

### Proposed Changes

- `pkg/artwalk/artwalk.go` [NEW]: `ArtWalk` + `Stop` structs, YAML parser/serializer, provider allowlist validation
- `pkg/artwalk/manager.go` [NEW]: Manages `GetAppDir()/artwalks/*.artwalk` files (List, Load, Save, Delete, Import, Export)
- `pkg/artwalk/artwalk_test.go` [NEW]: Round-trip parsing tests
- `pkg/artwalk/manager_test.go` [NEW]: Manager CRUD tests
- `pkg/wallpaper/ui_builder.go`: Add "Art Walk" tab schema (list walks, import/export/activate buttons)
- `pkg/wallpaper/wallpaper.go`: Inject active walk stops into rotation as virtual query (ordered, not shuffled)
- `pkg/ui/schema/schema.go`: Any new schema types needed for walk list/detail views

### Open Design Questions

1. **Hero Image**: Use the first stop's image as the thumbnail? Or auto-generate a mosaic collage from the first 4 stops?
2. **Tab Placement**: New top-level tab in Preferences? Or nested under a new "Gallery" section?
3. **Active Walk Behavior**: Should an active Art Walk fully replace normal rotation, or blend with other active queries?
4. **Walk Creation UX**: Create by picking images one-by-one from providers? Or by "saving" current Favorites to a walk?
5. **Sticky Note Overlay**: Desktop overlay (requires platform-specific transparent window), or system tray tooltip/menu only?

### Provider Shareability Matrix

| Provider | Shareable? | Auth Required | Content Gating | Notes |
| :--- | :--- | :--- | :--- | :--- |
| Rijksmuseum | ✅ Yes | Free API key | None | Permanent IIIF URLs |
| Met Museum | ✅ Yes | None | None | Fully open |
| Art Institute Chicago | ✅ Yes | None | None | IIIF standard |
| SMK / NPM / Cleveland / Getty | ✅ Yes | None or free | None | Public APIs |
| Wikimedia Commons | ✅ Yes | None | None | Fully public |
| Pexels | ⚠️ Partial | Free API key | None | URLs may expire; needs re-resolve |
| Wallhaven | ⚠️ Partial | API key + account for NSFW | NSFW locked | SFW only shareable |
| Google Photos | ❌ No | Full OAuth2 | Private | Not shareable |
| Local Folders | ❌ No | N/A | N/A | Absolute file paths |
| Favorites | ❌ No | N/A | N/A | Local copies only |

Art Walk v1 supports only the ✅ providers. Pexels/Wallhaven SFW support can be added in a follow-up.
