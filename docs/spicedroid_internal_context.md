# Spice Droid: Internal Developer Context

> **Purpose**: This file contains the architecture and implementation details for **Spice Droid** (Android Port). It defines the strategy for decoupling the Fyne UI from the Core Logic to enable Android Widget support.

## 1. The Strategy: "Hybrid Architecture" (Interface Split)

We will use a **Hybrid Approach**:
1.  **Main App**: Uses **Fyne** (Go) to render the full Settings UI. This is a standard Android Activity.
2.  **Widget & Controls**: Uses **Native Android** (Kotlin + Go Core) for the Home Screen Widget and Persistent Notification controls.

### 1.1 The Challenge
The Android Widget and Background Service (Notification) cannot load the Fyne (OpenGL) runtime. They must be lightweight.
However, our `pkg/provider` code currently mixes Core Logic with Fyne UI code, making it impossible to compile for the lightweight context.

### 1.2 The Solution: Interface Split & Build Tags (The "How")
We use Go build tags to keep the UI code but compile it *only* for the Main App, not the Widget/Service library.

#### A. File Refactoring Pattern
For each provider (e.g., `pkg/wallpaper/providers/wallhaven`), we split the code:
1.  **`wallhaven.go`** (Shared):
    *   **Content**: `FetchImages`, `ParseURL`, `EnrichImage`, `Name`, `Title`.
    *   **Build**: All platforms.
2.  **`wallhaven_gui.go`** (Desktop Only):
    *   **Content**: `CreateSettingsPanel`, `CreateQueryPanel`, `GetProviderIcon`.
    *   **Build**: `//go:build !android` (Ignored by Mobile).

#### B. The Interface Split
We segregate the monolithic `ImageProvider` into two tiers:
```go
// CoreProvider is safe for Android (Logic Only)
type CoreProvider interface {
    Name() string
    FetchImages(...)
}

// GUIProvider extends Core for Desktop (Logic + Fyne UI)
type GUIProvider interface {
    CoreProvider
    GetProviderIcon() fyne.Resource
    CreateSettingsPanel(...)
}
```
*   **Desktop App**: Uses `GUIProvider`.
*   **Android Widget**: Uses `CoreProvider`.

### 2.1 The "App" (Settings & Setup)
*   **Icon Click**: Opens the standard Fyne UI (just like Desktop).
*   **Function**: Users configure providers (API keys), manage image collections, and tweak tuning settings here.
*   **State**: Saves to `config.json`.

### 2.2 The "Daily Driver" (Widget & Notification)
Once configured, the user rarely opens the App. They interact via:

1.  **Home Screen Widget**:
    *   **Visual**: Displays the current wallpaper (or a specific "Frame").
    *   **Actions**: Tapping it usually opens the App or triggers a "Next Image" (configurable).
2.  **Persistent Notification** (The Control Center):
    *   **Context**: Always available in the notification shade.
    *   **Content**: Thumbnail of current image + Source Attribution (e.g. "Photo by @userid on Unsplash").
    *   **Controls**:
        *   `[Prev]` `[Next]`: Cycles images immediately.
        *   `[Fav]`: Adds to local favorites.
        *   `[Block/Skip]`: Bans the image/tag and skips.
    *   **Effect**: Updates **both** the System Wallpaper and the Widget immediately.

## 3. Shared Configuration (The "Brain")
Since the Android Widget runs in a separate process/context from the Main App, they cannot share memory. They must communicate via **Shared Storage**.

*   **Config Abstraction**: The `Config` struct currently embeds `fyne.Preferences`. We will abstract this behind a `Preferences` interface.
    *   **Desktop**: Wraps `fyne.Preferences` (Status Quo).
    *   **Android**: Implements `JSONPreferences` (Reads/Writes `config.json`).

### 2.1 The Control Plane (Tray vs. Notification)
*   **Desktop**: Uses `systray` (Fyne Tray Menu) for controls.
*   **Android**: Replaces the Tray Menu with a **Persistent Notification**.
    *   **Buttons**: "Next Image", "Previous Image", "Open Settings".
    *   **Implementation**: Native Kotlin `NotificationService` calling into Go Core via JNI to trigger fetches.

*   **Workflow**:
    1.  User updates settings in the Fyne App.
    2.  App writes to `config.json`.
    3.  Android Widget wakes up, reads `config.json`, and executes the fetch logic using the `CoreProvider`.

## 3. Android Integration (Go Mobile)

We will use `gomobile bind` to generate an `.aar` library.

### 3.1 The Bridge (`pkg/mobile/api.go`)
This package is the *only* entry point for Kotlin.

```go
package mobile

import "github.com/dixieflatline76/Spice/pkg/wallpaper/pipeline"

// WidgetHelper is called by the Kotlin WidgetProvider
type WidgetHelper struct {
    pipeline *pipeline.Pipeline
}

func NewWidgetHelper(storageDir string) *WidgetHelper {
    // 1. Initialize 'Headless' Config (JSON Backed)
    cfg := config.NewJSONConfig(storageDir)
    
    // 2. Initialize Store & Pipeline
    store := store.New(cfg)
    return &WidgetHelper{...}
}

// GetWidgetImage returns the cropped bitmap bytes
func (w *WidgetHelper) GetWidgetImage(width, height int) ([]byte, error) {
    img, _ := w.pipeline.GetNextImage()
    
    // Inject Mobile Tuning (Aggressive Crop 0.5)
    processor := processor.New(tuning.MobileProfile())
    
    // Smart Crop specifically for this widget instance
    finalImg, _ := processor.FitImage(img, width, height)
    
    return encodeBitmap(finalImg), nil
}
```

## 4. Mobile Tuning Profile

In `pkg/core/tuning/profiles.go`:

```go
func MobileProfile() TuningConfig {
    return TuningConfig{
        SmartFitMode: SmartFitAggressive, 
        AspectThreshold: 0.5, // Aggressive crop for small widgets
        FaceScaleFactor: 1.3, // Larger faces for small screens
    }
}
```

## 5. Distribution & Compliance

To ship this via the Google Play Store, we use the "**Eject Strategy**" rather than a simple `fyne package`.

### 5.1 Project Structure (Standard Android Studio)
We create a standard Gradle/Kotlin project that imports our Go code as a library.
*   **The App**: A standard Activity that launches the Fyne window (via Fyne's helper).
*   **The Widget**: A standard `AppWidgetProvider` (Kotlin) that calls our `WidgetHelper`.

### 5.2 The "Double Runtime" Safety Net
To prevent threading crashes between Fyne (OpenGL) and the Widget (Headless):
*   **Configuration**: We configure the Widget to run in a separate process in `AndroidManifest.xml`: `android:process=":widget"`.
*   **Result**: The OS isolates the Widget's memory and Go runtime from the Main App. If the widget crashes, the app survives (and vice-versa).

### 5.3 Play Store Compliance
*   **Foreground Service**: The "Persistent Notification" control panel uses a valid Android Foreground Service type (`specialUse` or `mediaPlayback`), explicitly allowed for this use case.
*   **Permissions**: We request standard `INTERNET` and `READ_EXTERNAL_STORAGE` (scoped), which creates no policy issues.

## 7. Project Timeline (Two Weeks / ~10 Days)

**Target**: A production-ready Android release.

### Phase 1: The Foundation (Days 1-4)
*Goal: Decouple Fyne from Core Logic without breaking the Desktop App.*
*   **Day 1**: Refactor `pkg/provider` interfaces (`CoreProvider` vs `GUIProvider`) and abstract `pkg/config`.
*   **Day 2**: Pilot the "Split File" strategy on complex providers (`Unsplash`, `GooglePhotos`) and simple ones (`Wallhaven`).
*   **Day 3**: Complete the split for all 8 providers. Verify Desktop build passes.
*   **Day 4**: Update `Store` and `Pipeline` to use the new `CoreProvider` interface.

### Phase 2: The Android Bridge (Days 5-7)
*Goal: Get Go code running inside an Android Studio project.*
*   **Day 5**: Create `pkg/mobile` API (`WidgetHelper`). Run `gomobile bind` to generate the `.aar`.
*   **Day 6**: Initialize Android Studio project. Configure "Double Runtime" (separate process for widget).
*   **Day 7**: Implement the "Persistent Notification" (Kotlin) that calls Go to fetch images.

### Phase 3: The Widget & Polish (Days 8-10)
*Goal: A shipping-quality Android experience.*
*   **Day 8**: Implement the Home Screen Widget (Kotlin) and its bitmap rendering loop.
*   **Day 9**: Tune `Smart Fit` for mobile aspect ratios (testing on emulator/device).
*   **Day 10**: Testing, Permissions cleanup, and Release build.

### 7.1 Key Risks
1.  **JNI Overhead**: Passing large bitmaps from Go to Kotlin can be slow (need efficient buffer handling).
2.  **Process Lifecycle**: Preventing Android form killing the "Persistent Notification" service.

## 8. Implementation Checklist

1.  [ ] **Refactor `pkg/provider`**: Split interfaces into `CoreProvider` and `GUIProvider`.
2.  [ ] **Split Files**: Rename/Move UI code in providers to `_gui.go` files with `//go:build !android`.
3.  [ ] **Abstract Config**: Create `Preferences` interface and `JSONPreferences` implementation.
4.  [ ] **Go Mobile**: Run `gomobile bind ./pkg/mobile` to generate the AAR.
5.  [ ] **Android Studio**: Create strict "Two Process" project structure.

