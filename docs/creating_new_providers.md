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

* **`Type() ProviderType`**:
  * **Purpose**: Categorizes the provider for the Tabbed UI.
  * **Returns**: One of `provider.TypeOnline` (Pexels, Wallhaven), `provider.TypeLocal` (Filesystem), or `provider.TypeAI` (Generative).

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

## 4. The "Apply" Lifecycle (Critical Pattern)

Spice uses a **Strict Deferred-Save Model**. Changes made in the UI must NOT be saved immediately to disk. They must be queued and only committed when the user clicks "Apply".

### 4.1 Correct Implementation Pattern

The `SettingsManager` now handles the "Closure Trap" and dirty detection automatically via the **Registry**.

#### For Standard Settings (API Keys, Toggles)
When using helpers like `CreateTextEntrySetting`, `CreateBoolSetting`, or `CreateSelectSetting`, the `SettingsManager` will:
1.  **Seed**: Automatically seed the baseline value in the registry.
2.  **Monitor**: Automatically compare the live widget state against the registry.
3.  **Apply**: Automatically execute your `ApplyFunc` only if the value has changed.

**Example**:
```go
pexelsAPIKeyConfig := setting.TextEntrySettingConfig{
    Name:         "pexelsAPIKey",
    InitialValue: p.cfg.GetPexelsAPIKey(),
    // ... other fields
    ApplyFunc: func(s string) {
        p.cfg.SetPexelsAPIKey(s)
    },
}
sm.CreateTextEntrySetting(&pexelsAPIKeyConfig, pexHeader)
```

#### For Custom Widgets (Manual Query Lists)
If you are building a custom list (like a museum collection or query list), you must manually wire into the Registry:

1.  **Seed the Baseline**: Call `sm.SeedBaseline(key, initialValue)` for each item.
2.  **Check in OnChanged**: Compare the new value against `sm.GetBaseline(key)`.
3.  **Queue the Callback**: Use `sm.SetSettingChangedCallback(key, callback)` and `sm.SetRefreshFlag(key)`.

```go
check.OnChanged = func(on bool) {
    baseline := sm.GetBaseline(queryKey).(bool)
    if on != baseline {
        sm.SetSettingChangedCallback(queryKey, func() {
            if on { p.cfg.EnableQuery(id) } else { p.cfg.DisableQuery(id) }
        })
        sm.SetRefreshFlag(queryKey)
    } else {
        sm.RemoveSettingChangedCallback(queryKey)
        sm.UnsetRefreshFlag(queryKey)
    }
    sm.GetCheckAndEnableApplyFunc()()
}
```

### 4.2 Common Pitfalls

*   **Immediate Saving**: `p.cfg.Save()` inside the `OnChanged` callback.
    *   *Result*: The "Apply" button becomes a "Pulse" button because the dirty state is instantly cleared (or never technically dirty).
*   **Captured Stale State**: Comparing `on != capturedInitialState`.
    *   *Result*: After clicking Apply, the UI thinks `capturedInitialState` is the truth, but the config has updated. Toggling back will mistakenly leave the "Apply" button enabled. **Use `sm.GetBaseline(key)` instead.**
*   **Sticky Global Flags**: `sm.SetRefreshFlag("queries")` on toggle.
    *   *Result*: Global flags are often not cleared by `UnsetRefreshFlag(uniqueKey)`. If a user reverts a change, the global flag remains, leaving the "Apply" button stuck on. Only use global flags for destructive actions (Delete) or inside the *Apply Callback* itself.

### 4.3 The Transactional Exception (Credentials)

While standard settings are deferred, **Sensitive Credentials** (API Keys, Usernames) must follow the **Transactional UI Pattern**.

1.  **Immediate Persistence**: In your `CreateSettingsPanel`, when the user clicks **"Verify & Connect"**, the network check is performed and the value is saved to the config *immediately* upon success.
2.  **Visual Locking**: You must call `sm.SeedBaseline(key, value)` and `sm.Refresh()` inside the success goroutine. This locks the field and resets the action button.
3.  **Timeouts**: Network checks must use `context.WithTimeout` (10s) to prevent the UI from hanging on "Verifying...".
4.  **Consistency**: Use standard labels like "Verify & Connect" (for keys) and "Verify Username".

## 5. Pagination & Randomization Stability

APIs often return results in inconsistent orders (e.g., "Page 2" might contain items from "Page 1").
If your provider supports **Pagination** AND **Shuffling**, you must implement the **"Cache-First Stable Shuffle"** pattern.

### 5.1 The Pattern (resolveQueryToIDs)

1.  **Cache First**: Check an internal `map[string][]int` for already resolved IDs.
    *   *Why*: Ensures Page 2 sees the exact same list as Page 1.
2.  **Fetch & Sort**: Download all IDs, then `sort.Ints(ids)`.
    *   *Why*: Creates a deterministic baseline, fixing API jitter.
3.  **Shuffle (If Enabled)**: If `cfg.GetImgShuffle()` is true, shuffle the sorted list using a session-stable seed.
    *   *Why*: Supports the user's "Shuffle" feature without breaking pagination.
4.  **Store**: Save the final list to the cache.

### 5.2 Example Implementation

```go
type Provider struct {
    // ...
    idCache   map[string][]int
    idCacheMu sync.RWMutex
}

func (p *Provider) resolveIDs(query string) ([]int, error) {
    p.idCacheMu.RLock()
    if cached, ok := p.idCache[query]; ok {
        p.idCacheMu.RUnlock()
        return cached, nil
    }
    p.idCacheMu.RUnlock()

    // 1. Fetch
    ids, _ := fetchFromAPI(query)

    // 2. Sort (Deterministic Baseline)
    sort.Ints(ids)

    // 3. Shuffle (If User Wants It)
    if p.cfg.GetImgShuffle() {
        r := rand.New(rand.NewSource(time.Now().UnixNano()))
        r.Shuffle(len(ids), func(i, j int) {
            ids[i], ids[j] = ids[j], ids[i]
        })
    }

    // 4. Cache
    p.idCacheMu.Lock()
    p.idCache[query] = ids
    p.idCacheMu.Unlock()

    return ids, nil
}
```

## 6. Registration (Automated)
 
 Spice uses a code generation tool (`cmd/util/gen_providers`) to automatically register all providers found in `pkg/wallpaper/providers/`.
 
 ### 6.1 The Logic
 
 1.  **Auto-Discovery**: The tool scans the `providers/` directory for subdirectories.
 2.  **Generation**: It creates `cmd/spice/zz_generated_providers.go`, which contains the necessary `_` imports to trigger the `init()` functions of your providers.
 3.  **Build Integration**: The generation runs automatically via `go generate` (called by `make build` or `make run`).
 
 ### 6.2 Disabling a Provider
 
 To temporarily disable a provider without deleting the code:
 
 1.  Create an empty file named `.disabled` inside the provider's directory (e.g., `pkg/wallpaper/providers/myprovider/.disabled`).
 2.  Run `go generate ./...` (or `make gen`).
 3.  The tool will skip this directory when generating `zz_generated_providers.go`, effectively compiling it out of the final binary.
 
 ### 6.3 Manual imports (Legacy/Debug)
 
 You do **not** need to manually edit `cmd/spice/main.go` anymore. The `//go:generate` directive at the top of `main.go` handles this.


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

## 8. The Museum Template (v1.6+)

For cultural institutions (Museums, Archives), Spice provides a standardized "Evangelist" UI template designed to drive engagement rather than just utility.

### 8.1 Core Components (`ui_museum.go`)

*   **Header**: Use `wallpaper.CreateMuseumHeader`.
    *   **Arguments**: Name, Location, Description, MapURL, WebURL, DonateURL, SettingsManager.
    *   **Features**:
        *   **"Plan a Visit" Button**: Automatically renders if `MapURL` is provided. Use this to drive foot traffic.
        *   **Clickable License**: Supports explicit licensing links (e.g., CC0) in the header metadata.
        *   **Romance Copy**: Supports long-form, evocative descriptions to "sell the magic" of the institution.

### 8.2 Collections as Tours
Instead of raw database categories, frame collections as curated experiences:
*   **Bad**: "Department 1", "Asian Art".
*   **Good**: "Director's Cut", "Arts of Asia", "The Impressionist Era".

### 8.3 Interaction Model
*   **Fixed List**: Use a fixed set of checkboxes (via `widget.NewCheck`) for collections.
*   **No Delete**: Unlike generic queries, these are permanent fixtures of the provider.
*   **Toggle Logic**: Map checkbox states directly to `cfg.Enable/DisableQuery`.
