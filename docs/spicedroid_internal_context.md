# Spice Droid: Internal Developer Context

> **Purpose**: This file contains the architecture and implementation details for **Spice Droid** (Android Port). It defines the strategy for decoupling the Fyne UI from the Core Logic to enable Android Widget support.

## 1. The Strategy: "Interface Split" (No Rewrite)

We will **NOT** rewrite the existing Fyne UI. Instead, we will use Go's build system (`//go:build`) to segregate UI code from Core code.

### 1.1 The Problem
Android Widgets cannot run Fyne (OpenGL). However, our Desktop App *requires* Fyne.
The current `pkg/provider` package mixes Core Logic (HTTP) with UI Logic (Fyne Widgets), causing Android builds to fail.

### 1.2 The Solution: File-Level Segregation
We will split monolithic provider files into two:

1.  **`myprovider.go`** (Shared):
    *   Builds for **ALL** platforms.
    *   Contains: `FetchImages()`, `ParseURL()`, `Name()`.
    *   **NO** `import "fyne.io/..."`.
2.  **`myprovider_gui.go`** (Desktop Only):
    *   Builds for **!android**.
    *   Contains: `CreateSettingsPanel()`, `GetProviderIcon()`.
    *   **Can** `import "fyne.io/..."`.

### 1.3 The Mobile Interface Pattern
We define a "Core" interface that works everywhere, and a "GUI" interface that extends it for Desktop.

```go
// pkg/provider/interfaces.go

// CoreProvider is safe for Android
type CoreProvider interface {
    Name() string
    FetchImages(...)
    // ... logic methods only
}

// GUIProvider extends Core for Desktop
type GUIProvider interface {
    CoreProvider
    GetProviderIcon() fyne.Resource
    CreateSettingsPanel(...)
}
```

## 2. Shared Configuration (The "Brain")

Since the Android Widget runs in a separate process/context from the Main App, they cannot share memory. They must communicate via **Shared Storage**.

*   **Config Abstraction**: The `Config` struct currently embeds `fyne.Preferences`. We will abstract this behind a `Preferences` interface.
    *   **Desktop**: Wraps `fyne.Preferences` (Status Quo).
    *   **Android**: Implements `JSONPreferences` (Reads/Writes `config.json`).
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

## 5. Implementation Checklist

1.  [ ] **Refactor `pkg/provider`**: Split interfaces into `CoreProvider` and `GUIProvider`.
2.  [ ] **Split Files**: Rename/Move UI code in providers to `_gui.go` files with `//go:build !android`.
3.  [ ] **Abstract Config**: Create `Preferences` interface and `JSONPreferences` implementation.
4.  [ ] **Go Mobile**: Run `gomobile bind ./pkg/mobile` to generate the AAR.
