# How to Create a New Image Provider

A deep-dive technical guide for implementing new image sources in Spice (v1.1.0+).

## 1. Provider Architecture

Spice uses a **Registry Pattern** to decouple providers. Providers are standalone packages in `pkg/wallpaper/providers/<name>`.

### Directory Structure

```text
pkg/wallpaper/providers/bing/
├── bing.go         # Implementation & Registration
├── const.go        # Constants (API URL, Regex)
└── bing_test.go    # Unit Tests
```

## 2. Interface Contract (`pkg/provider.ImageProvider`)

You must implement the following 6 methods.

### 2.1 Core Logic

* **`Name() string`**:
  * **Purpose**: Internal ID used for config keys and logging.
  * **Format**: PascalCase, unique (e.g., "Bing").

* **`Title() string`**:
  * **Purpose**: User-facing display name.
  * **Format**: Short, Title Case (e.g., "Bing Daily").

* **`ParseURL(webURL string) (string, error)`**:
  * **Input**: A URL copied from the browser (e.g., `bing.com/images/search?q=foo`).
  * **Output**: A clean API-ready string (e.g., `search:foo`) or the input if it's already compliant.
  * **Validation**: Use your `const.go` regex here to reject invalid domains.

* **`FetchImages(ctx context.Context, apiURL string, page int) ([]Image, error)`**:
  * **Context**: Must respect `ctx.Done()` for cancellation.
  * **Pagination**: `page` is 1-indexed. If the API uses offsets, calculate `offset = (page-1) * limit`.
  * **Returns**: A slice of `provider.Image`.
  * **Critical**: Map the API response fields to `Image` struct fields (`Path`, `ID`, `Attribution`, `ViewURL`).

* **`EnrichImage(ctx, img) (Image, error)`**:
  * **Purpose**: Called *after* download if metadata is missing.
  * **Usage**: Some search APIs don't return high-res URLs or file types. Use this to perform a secondary fetch (e.g., HEAD request or scraping) to fill in `FileType`, `Path`, etc.
  * **Safe Default**: If your API provides everything in `FetchImages`, just return `img, nil`.

### 2.2 UI Integration

* **`GetProviderIcon() fyne.Resource`**:
  * **Purpose**: 64x64px icon for Tray Menu and Settings Headers.
  * **Implementation**: Use `fyne.NewStaticResource("Name", []byte{...})`. Embed the PNG bytes in code or use `//go:embed`.

## 3. Configuration & Settings Logic

Do **NOT** modify the global `Config` struct. Use `fyne.Preferences`.

### 3.1 Settings Panel (`CreateSettingsPanel`)

Constructs the "General" tab for your provider (e.g., API Keys).
**Input**: `sm setting.SettingsManager`.
**returns**: `fyne.CanvasObject` (usually a `container.NewVBox`).

**Widget Types**:

* **`CreateTextEntrySetting`**: For strings (API Keys).
  * **Validator**: Use `fyne.StringValidator` (e.g., `validator.NewRegexp(...)`).
  * **PostValidateCheck**: Optional function `func(s string) error` for logic validation (e.g., "Key must start with 'Bearer '").
* **`CreateBoolSetting`**: For toggles.
* **`CreateSelectSetting`**: For dropdowns.
* **`CreateButtonWithConfirmationSetting`**: For dangerous actions (Reset, Clear Cache).

### 3.2 Query Panel (`CreateQueryPanel`)

Constructs the image source list.
**Pattern**:

1. Iterate through `p.cfg.Preferences.QueryList("queries")`? **NO**.
2. Use `p.cfg.Queries` (the unified list). Filter by `q.Provider == p.Name()`.
3. Render a list of queries with "Active" toggles.
4. **Use Standardized Add Button**: Use `wallpaper.CreateAddQueryButton` (in `pkg/wallpaper/ui_add_query.go`) to create the "Add" button. This helper handles validation, modal creation, and the critical "Apply" button wiring for you.

   ```go
   addBtn := wallpaper.CreateAddQueryButton(
       "Add MyProvider Query",
       sm,
       wallpaper.AddQueryConfig{
           Title:           "New Query",
           URLPlaceholder:  "Search term or URL",
           DescPlaceholder: "Description",
           ValidateFunc: func(url, desc string) error {
               if len(url) == 0 {
                   return errors.New("URL cannot be empty")
               }
               // Add provider-specific validation here (e.g., regex check)
               return nil
           },
           AddHandler: func(desc, url string, active bool) (string, error) {
               return p.cfg.AddMyProviderQuery(desc, url, active)
           },
       },
       func() {
           queryList.Refresh()
           sm.SetRefreshFlag("queries")
       },
   )
   ```

## 4. The "Apply" Lifecycle (Critical)

Spice uses a deferred-save model. Changes are staged until "Apply" is clicked.
**You must implement this wiring:**

1. **Change Detected**: Inside `OnChanged`:

    ```go
    sm.SetRefreshFlag("setting.id") // Enables "Apply" button
    ```

2. **Queue Action**:

    ```go
    sm.SetSettingChangedCallback("setting.id", func() {
        // Logic to run ONLY when Apply is clicked
        prefs.SetString("key", newValue)
    })
    ```

3. **Revert**: If user cancels/reverts:

    ```go
    sm.UnsetRefreshFlag("setting.id")
    sm.RemoveSettingChangedCallback("setting.id")
    ```

4. **UI Update**: Trigger visual update:

    ```go
    sm.GetCheckAndEnableApplyFunc()()
    ```

## 5. Registration (The Hook)

In `myprovider.go`, add:

```go
func init() {
    // Key "Bing" must match Name() return value
    provider.RegisterProvider("Bing", func(cfg *wallpaper.Config, client *http.Client) provider.ImageProvider {
        return NewBingProvider(cfg, client)
    })
}
```

In `cmd/spice/main.go`:

```go
import _ "github.com/dixieflatline76/Spice/pkg/wallpaper/providers/bing"
```

## 6. Testing

* **Unit**: Test `ParseURL` with table-driven tests.
* **Integration**: Mock the `http.Client` or usage `httptest.Server` to test `FetchImages` without real network calls.
* **UI**: UI testing is optional but recommended if complex.

## 7. Browser Extension Integration

If your provider supports "copy-pasting" URLs from the browser (like Wallhaven or Pexels), you can integrate with the Spice Safari/Chrome extension.

1.  **Define Regex**: In your `pkg/wallpaper/providers/<name>/const.go`, define a constant for your URL pattern.
    *   Naming Convention: ` <Name>URLRegexp` (e.g., `BingURLRegexp`).
    *   Value: A regex string matching the URLs you want to intercept (e.g., `^https://bing.com/images/.*`).

2.  **Enable Discovery**:
    *   Ensure your provider is imported in `cmd/spice/main.go` (e.g., `_ "github.com/.../providers/bing"`).
    *   The build tool ` cmd/util/sync_regex` will automatically parse `main.go`, find your enabled provider, and extract the regex from your `const.go` to inject it into the extension's `background.js`.

3.  **Manual Sync**: If you need to force a sync during development, run:
    ```bash
    make sync-extension
    ```

## Reference

See `pkg/wallpaper/providers/wikimedia/wikimedia.go` or `pkg/wallpaper/providers/pexels/pexels.go` for the reference implementation of all these patterns.
