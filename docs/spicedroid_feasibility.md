# Spice Droid: The Unified Architecture

## 1. The Vision
**One Brain, Many Screens.**
The goal is to support **both** native Android Wallpaper changing (`WallpaperManager`) and Widgets (`RemoteViews`), dealing with the unique aspect ratios of mobile (9:16) vs desktop (16:9) using a tuned Smart Crop engine.

## 2. Refactoring for Reuse (`pkg/core` Strategy)
To achieve this without duplicating code, we must decouple the "Brain" (Logic) from the "Body" (UI/OS).

### Proposed Directory Structure

```text
/
├── pkg/
│   ├── core/              # [SHARED] Pure Go Logic. No Fyne. No GLib.
│   │   ├── store/         # The ImageStore
│   │   ├── pipeline/      # The Worker Pool & State Manager
│   │   ├── processor/     # Smart Crop 2.0 (Pigo + Imaging)
│   │   └── tuning/        # TuningConfig struct
│   │
│   ├── providers/         # [SHARED] Image Sources (Met, Wallhaven)
│   │
│   ├── drivers/           # [PLATFORM] OS Abstraction Layer
│   │   ├── desktop/       # Linux (feh), Windows (SystemParametersInfo)
│   │   └── android/       # Go Mobile bindings to call JVM methods
│   │
│   └── ui/                # [SHARED-ISH] Fyne Settings Panels
│       ├── settings/      # The shared Logic for creating Panels
│       └── ...
│
├── cmd/
│   ├── desktop/           # Desktop Entry Point (main.go)
│   └── mobile/            # Android Entry Point (Fyne App)
│
└── android/               # [NATIVE] The Android Studio Project
    └── app/src/main/
        ├── java/.../widget # Kotlin Widget Code calls pkg/core via Bindings
        ├── java/.../service# WallpaperService calls pkg/core via Bindings
        └── assets/         # Bundled tuning_mobile.json
```

## 3. The "Mobile Tuning" Strategy

You mentioned **"mobile only tuning set"**. This is easy with our `tuning.go` architecture.

### Implementation
1.  **Injection**: The `Config` struct will hold the `TuningConfig`.
2.  **Runtime Loading**:
    *   **Desktop**: Loads standard values (0.9 Aspect Ratio threshold).
    *   **Mobile**: Detects `runtime.GOOS == "android"` (or injected flag). Loads a stricter/different profile.
        *   `FaceScaleFactor`: 1.3 (Faces are closer/larger on phone selfies/portraits).
        *   `SmartFitMode`: **Aggressive** (Phones *need* to crop landscape images heavily to fit portrait screens).
        *   `AspectThreshold`: 0.5 (We expect heavy cropping).

## 4. Workflows

### A. The Wallpaper Service (Unified)
Instead of the desktop's "Timer Loop", Android uses a `WorkManager` or `AlarmManager`.
1.  **Wake Up**: Android wakes up the app process.
2.  **Fetch**: Go code runs `Pipeline.FetchNext()`.
3.  **Process**: `SmartImageProcessor` runs with **Mobile Tuning**.
    *   *Input*: 4K Landscape Image.
    *   *Target*: 1080x2400 (Portrait).
    *   *Logic*: Finds Face -> Centers -> Crops vertical slice.
4.  **Set**: Go calls `driver.SetWallpaper(bitmap)`.

### B. The Widget (The Snapshot)
1.  User taps Widget.
2.  Kotlin calls `GoLib.GetWidgetImage(widgetWidth, widgetHeight)`.
3.  Go runs `SmartImageProcessor` specifically for *that* widget's weird size (e.g. 400x200).
4.  Returns Bitmap.

## 5. Summary of Changes Needed
1.  **Extract**: Move `pkg/wallpaper/store.go` et al. into a clean `pkg/core` package that doesn't import `fyne`.
2.  **Interface**: Define a `WallpaperDriver` interface that both `desktop/linux.go` and `android/bindings.go` implement.
3.  **Bind**: Use `gomobile bind` to compile `pkg/core` into an `.aar` for the Android project.

**Verdict**: This architecture allows **90% code reuse**. Only the final "Draw to Screen" line of code differs.

## 6. Strategic Verdict: Is this a "Good" Idea?

**Yes.** But specifically because of **Smart Crop**.

### The "Killer Feature" Argument
The biggest pain point with Mobile Widgets is **Aspect Ratio Mismatch**.
*   **The Problem**: Users have a 4x2 Widget (Panoramic) but a 9:16 Photo (Portrait). Standard apps just show a blurry zoomed center or black bars.
*   **Your Solution**: Spice's **Smart Crop** (tuned for mobile) actively hunts for the "Subject" (Face/High Energy) and dynamically recomposes the image to fit the *specific widget instance*.
*   **Result**: A "Smart Frame" that always looks intentional, never broken.

### The "Unified" Win
By supporting both **Wallpaper** (System) and **Widgets** (Custom), you cover the entire customization market.
*   **Minimal Debt**: The `pkg/core` architectural split means new providers (e.g., ArtStation) added to Desktop automatically appear on Mobile.

**Recommendation**: **Green Light**. The engineering cost of the generic `pkg/core` refactor is worth it even for the Desktop app (cleaner testing), and it opens the door to Mobile for free.
