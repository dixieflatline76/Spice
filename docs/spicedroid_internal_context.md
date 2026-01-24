# Spice Droid: Internal Developer Context

> **Purpose**: This file contains the implementation details and "Mental Model" for the **Spice Droid** project. It complements `spicedroid_feasibility.md` by providing the *How* to the feasibility's *What*. Use this to immediately start coding on the mobile port.

## 1. The "Clean Core" Refactoring Strategy

The primary blocker for Mobile is Fyne dependencies in the core logic. Fyne cannot run inside an Android Widget (RemoteViews).

### 1.1 The Dependency Rule
**Rule**: Packages in `pkg/core/...` must **NEVER** import `fyne.io/fyne/v2`.

### 1.2 Migration Map
We must move files from `pkg/wallpaper/` to a new `pkg/core/` structure.

| Current File | Destination | Action Required |
| :--- | :--- | :--- |
| `store.go` | `pkg/core/store/store.go` | Strip any Fyne-based event signaling. Use generic `func()` callbacks or Channels. |
| `pipeline.go` | `pkg/core/pipeline/pipeline.go` | Ensure `worker` logic doesn't touch UI. |
| `smart_image_processor.go` | `pkg/core/processor/processor.go` | **Pure**. This is already Fyne-free (mostly). Ensure `pigo` and `imaging` are the only deps. |
| `tuning.go` | `pkg/core/tuning/config.go` | Move `TuningConfig` here. |
| `config.go` | `pkg/core/config/config.go` | **Hard**: The current Config heavily relies on `fyne.Preferences`. **Solution**: Abstract this behind a `Preferences` interface so we can use `SharedPreferences` on Android or a JSON file. |

## 2. The Driver Interface Pattern

To support both Desktop (Linux/Windows) and Mobile (Android), we need an abstraction layer for OS interactions.

### 2.1 The Interface (`pkg/core/driver/interface.go`)
```go
type WallpaperDriver interface {
    // Desktop: Calls feh/SetWallpaper
    // Android: Calls WallpaperManager.setBitmap() via JNI
    SetWallpaper(imagePath string, imgData []byte) error

    // Desktop: Returns screen resolution
    // Android: Returns display metrics
    GetDisplaySize() (width, height int, err error)
    
    // Desktop: ~/.config/spice
    // Android: Context.getFilesDir()
    GetStorageDir() string
}
```

## 3. Android Integration (Go Mobile)

We will use `gomobile bind` to generate an `.aar` library.

### 3.1 The Bridge (`android/bindings.go`)
This package will be exported to Java/Kotlin.

```go
package spice_mobile

import "github.com/dixieflatline76/Spice/pkg/core/pipeline"

// WidgetHelper is called by the Kotlin WidgetProvider
type WidgetHelper struct {
    pipeline *pipeline.Pipeline
}

func NewWidgetHelper(storageDir string) *WidgetHelper {
    // 1. Initialize 'Headless' Core
    cfg := config.LoadFrom(storageDir)
    store := store.New(cfg)
    return &WidgetHelper{...}
}

// GetWidgetImage is the Money Shot
// Kotlin passes the EXACT size of the widget (e.g. 400x200)
// Go runs Smart Crop 2.0 and returns the perfect bitmap
func (w *WidgetHelper) GetWidgetImage(width, height int) ([]byte, error) {
    img, _ := w.pipeline.GetNextImage()
    
    // Inject Mobile Tuning (Aggressive Crop)
    processor := processor.New(tuning.MobileProfile())
    
    // Crop specifically for this widget instance
    finalImg, _ := processor.FitImage(img, width, height)
    
    return encodeBitmap(finalImg), nil
}
```

## 4. Mobile Tuning Profile

In `pkg/core/tuning/profiles.go`:

```go
func MobileProfile() TuningConfig {
    return TuningConfig{
        // Mobile screens/widgets are small. We need to be aggressive.
        SmartFitMode: SmartFitAggressive, 
        
        // 0.5 means: "If aspect ratio differs by > 0.5, CROP IT."
        // (Desktop is 0.9, much more conservative)
        AspectThreshold: 0.5, 
        
        // Faces are larger in mobile usage context (Selfies/Portraits).
        // 1.3 scale factor optimized for mobile screen composition.
        FaceScaleFactor: 1.3,
    }
}
```

## 5. Next Steps Checklist

1.  [ ] **Repo Restructure**: Create `pkg/core` and move `store.go` first.
2.  [ ] **Fyne Decoupling**: Refactor `Config` to not depend strictly on Fyne preferences.
3.  [ ] **Go Mobile Init**: Run `gomobile init`.
4.  [ ] **Hello World**: Create a simple Android App that calls `spice_mobile.Hello()`.
