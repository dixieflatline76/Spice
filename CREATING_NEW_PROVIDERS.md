# How to Create a New Image Provider

This guide outlines the steps to add a new image source (Provider) to Spice. It incorporates best practices, UI consistency patterns, and resolution handling logic based on the internal architecture.

## 1. Preparation

Create a new file `pkg/wallpaper/const_<provider>.go` for your constants.

- **Base URL**: The API endpoint.
- **Regex**: For validating/parsing URLs.

```go
package wallpaper

const (
    MyProviderBaseURL = "https://api.myprovider.com/v1"
    MyProviderDomainRegexp = `^https?://(www\.)?myprovider\.com/.*$`
)
```

## 2. Implement the Interface

Create `pkg/wallpaper/<provider>.go` and implement the `ImageProvider` interface.

```go
type ImageProvider interface {
    Name() string       // Internal ID (e.g., "Wikimedia")
    Title() string      // UI Display Name (e.g., "Wikimedia Commons") - Short & Clean!
    ParseURL(webURL string) (string, error)
    FetchImages(ctx context.Context, apiURL string, page int) ([]Image, error)
    EnrichImage(ctx context.Context, img Image) (Image, error)
    
    // UI Methods
    GetProviderIcon() fyne.Resource // Tray/Menu Icon (64x64)
    CreateSettingsPanel(sm setting.SettingsManager) fyne.CanvasObject
    CreateQueryPanel(sm setting.SettingsManager) fyne.CanvasObject
}
```

### 2.1 Optional: Custom Headers

If your provider requires specific headers (e.g., a custom `User-Agent`), implement the `HeaderProvider` interface:

```go
func (p *MyProvider) GetDownloadHeaders() map[string]string {
    return map[string]string{
        "User-Agent": "Spice-App/1.0",
    }
}
```

### 2.2 Optional: Resolution Awareness

To support "Smart Fit" filtering (injecting desktop resolution into queries), implement `ResolutionAwareProvider`:

```go
func (p *MyProvider) WithResolution(apiURL string, width, height int) string {
    // Modify apiURL to include width/height params
    return apiURL + fmt.Sprintf("&min_width=%d", width)
}
```

## 3. Configuration Integration

*Note: This step currently requires modifying the central `Config` struct (see `pkg/wallpaper/config.go`).*

1. Add a **Getter** (`GetMyProviderQueries`).
2. Add an **Adder** (`AddMyProviderQuery`).

## 4. UI Implementation (Critical Patterns)

The settings UI determines how users interact with your provider. Follow these patterns strictly to ensure the "Apply Changes" button works correctly.

### 4.1 The "Apply" Button Logic

Do **NOT** commit changes immediately when a user checks a box. Instead, queue the change and notify the `SettingsManager`.

**Correct Pattern for List Items:**

```go
pendingState := make(map[string]bool) // Track pending toggles

// In your list item constructor:
check := widget.NewCheck("Active", nil)
check.OnChanged = func(b bool) {
    if b != initialActive {
        pendingState[queryID] = b
        
        // 1. Register a callback to execution when "Apply" is clicked
        sm.SetSettingChangedCallback(queryID, func() {
            if b {
                 p.cfg.EnableImageQuery(queryID)
            } else {
                 p.cfg.DisableImageQuery(queryID)
            }
            delete(pendingState, queryID)
        })
        
        // 2. set the refresh flag to enable the button
        sm.SetRefreshFlag(queryID)
    } else {
        // Reverted to original state
        sm.RemoveSettingChangedCallback(queryID)
        sm.UnsetRefreshFlag(queryID)
    }
    
    // 3. Trigger button state update
    sm.GetCheckAndEnableApplyFunc()()
}
```

## 5. Registration

In your `pkg/wallpaper/<provider>.go` file, add an `init()` function to auto-register your provider. **If you miss this, your provider will not show up.**

```go
func init() {
    RegisterProvider("MyProvider", func(cfg *Config, client *http.Client) ImageProvider {
        return NewMyProvider(cfg, client)
    })
}
```

## 6. Testing & Linting

We run strict linters (`golangci-lint`). Avoid these common errors:

- **Unchecked Errors**: Always check errors from config methods.

    ```go
    // Bad
    p.cfg.EnableImageQuery(id)
    // Good
    if err := p.cfg.EnableImageQuery(id); err != nil { log.Error(err) }
    ```

- **Unused Assignments**: Don't assign to `err` if you don't check it.

- **Deprecated Random**: Use `rand.New(rand.NewSource(...))` instead of `rand.Seed`.

## Checklist

- [ ] Constants defined?
- [ ] Interface implemented?
- [ ] `Title()` returns a short, clean name?
- [ ] `HeaderProvider` implemented (if needed)?
- [ ] `ResolutionAwareProvider` implemented (if possible)?
- [ ] UI uses `pendingState` and `SetSettingChangedCallback`?
- [ ] Input validation loop included in UI?
- [ ] `init()` registration added?
- [ ] Unit tests passing?
