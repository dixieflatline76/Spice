# How to Create a New Image Provider

A deep-dive technical guide for implementing new image sources in Spice.

### Prerequisites — Recommended Reading Order

Before starting, familiarize yourself with these documents in order:

1.  **This document** — The primary provider creation guide (interface, settings, Apply lifecycle).
2.  **`architecture.md` §3–4** — The pipeline, store, and actor model your provider plugs into.
3.  **`internal_developer_context.md` §1–4** — Concurrency model, existing provider deep-dives, and the extension guide.
4.  **`creating_new_museum_providers.md`** — Only if building a cultural institution provider.

## 1. Provider Architecture & Purpose

**The "Why"**: Spice is designed so that the core engine (`store.go`, `pipeline.go`, `monitor_controller.go`) never has to know about the specifics of an image API. The general purpose of a *Provider* is to act as a **dumb, stateless bridge** between a remote API (or local filesystem) and Spice's ingestion pipeline. 
A provider's only job is to translate user settings into a deterministic list of image metadata. It does not shuffle, it does not manage its own disk cache, and it does not draw its own UI. This strict separation of concerns is what allows Spice to run 15 different image sources simultaneously without turning the core logic into spaghetti.

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

### 2.2 Advanced Interfaces (Optional but Recommended)

* **`provider.PacedProvider`**:
  * **Purpose**: Prevents rate limits (429s) when processing many images concurrently.
  * **Methods**:
    * `GetAPIPacing() time.Duration`: Enforces a minimum delay between metadata/enrichment API calls.
    * `GetProcessPacing() time.Duration`: Enforces a minimum delay between actual media file downloads.
  * **Mechanics**: The overarching `downloader` pipeline runs 16 concurrent generic workers. If this interface is omitted, jobs are immediately piped to workers at maximum concurrency. If implemented, a central **Fair Bouncer Dispatcher** meticulously spaces out jobs *before* handing them to the workers. This eliminates head-of-line (HOL) blocking and guarantees the provider's API limits are respected without stalling the rest of the application.

* **`provider.CustomClientProvider`**:
  * **Purpose**: Inject a heavily customized HTTP client.
  * **Methods**: `GetClient() *http.Client`
  * **Mechanics**: Use this to enforce transport-layer restrictions that `PacedProvider` alone cannot handle. For example, building a custom `http.RoundTripper` that implements a **Global Circuit Breaker** to instantly halt all downloads across all workers if an HTTP 429 response is encountered (e.g., used by Wikimedia).

### 2.3 UI Integration (Schema-Based)

Spice uses a **Hexagonal Architecture** for settings UI. Providers never import Fyne directly. Instead, they return pure Go `*schema.PanelSchema` structs, and the rendering engine (`ui/settings_manager.go`) handles all framework-specific logic.

* **`CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema`**:
  * **Purpose**: Declares the "General" tab for your provider (API Keys, toggles, buttons).
  * **Returns**: A `*schema.PanelSchema` containing `SectionSchema` groups with `ItemSchema` elements.
  * **Critical**: Must NOT import `fyne.io/*`. Return schema structs only.

* **`CreateQueryPanel(sm setting.SettingsManager, pendingURL string) *schema.PanelSchema`**:
  * **Purpose**: Declares the image source management panel.
  * **Returns**: A `*schema.PanelSchema` typically containing a `QueryListItem` and optionally an `AddQueryConfig`.

* **`GetProviderIcon() interface{}`**:
  * **Purpose**: 64x64px icon for Tray Menu and Settings Headers.
  * **Implementation**: Use `fyne.NewStaticResource("Name", []byte{...})`. This is the **only** method that returns a Fyne type (via `interface{}`), since icons are inherently platform-specific resources.

## 3. Configuration & Settings Logic (Declarative Schema)

**The "Why" (Hexagonal Architecture)**: Spice's UI is built on Fyne, but Fyne is heavily tied to the host OS graphics layer. If providers imported Fyne directly to draw their settings tabs, the entire provider layer would become untestable without a running X11/Wayland/Windows graphics context. 
To solve this, Spice uses a **Hexagonal Architecture**. Providers are 100% UI-framework agnostic. They define what their settings should look like using pure Go structs (`schema.PanelSchema`), and a central rendering engine (`ui/settings_manager.go`) translates that schema into actual Fyne widgets. This makes provider code entirely unit-testable and decouples our core logic from our chosen GUI toolkit.

Do **NOT** modify the global `Config` struct. Use `fyne.Preferences` for storage and `schema.*` types for UI declaration.

### 3.1 Settings Panel (`CreateSettingsPanel`)

Build a `*schema.PanelSchema` composed of `SectionSchema` groups and `ItemSchema` elements:

**Available Schema Types**:

| Schema Type | Use For |
|:---|:---|
| `schema.SecretItem` | API Keys, Credentials (Transactional verify/clear pattern) |
| `schema.TextItem` | Text entries with validation and debounce |
| `schema.BoolItem` | Checkboxes / toggles |
| `schema.SelectItem` | Dropdowns |
| `schema.ButtonItem` | Action buttons |
| `schema.AsyncButtonItem` | Background task buttons with loading state |
| `schema.OAuthPickerItem` | OAuth Connect buttons with status |
| `schema.FolderPickerItem` | Directory selectors |

### 3.2 UI Refresh & Cache Invalidation

When building your schemas, pay attention to the `NeedsRefresh` boolean on `ItemSchema`:

* **What it does:** Setting `NeedsRefresh: true` tells the central UI manager that modifying this setting significantly alters the current wallpaper state.
* **The `ApplyFunc` Lifecycle:** When a user changes a setting, your `ApplyFunc` is called to save the new value. If `NeedsRefresh` is true, Spice subsequently fires a global `RefreshImagesAndPulse()` event.
* **Cache Invalidation:** During this refresh event, the `ImageStore` reconciles its derivative cache against a global **Processing Hash** (a map of boolean flags representing the current SmartFit, Cropping, and Framing settings). 
* **The Result:** Any cached derivative images (e.g., `1920x1080.jpg`) whose original processing hash no longer matches the active application settings will be **instantly invalidated and deleted**. The pipeline will then seamlessly rebuild fresh derivatives from the preserved master files using the new settings, and immediately redraw the monitor wallpapers.
* **Rule of Thumb:** If your setting changes how an image is *fetched* (e.g., changing search sort order) or how an image is *processed* (e.g., enabling Face Crop), you MUST set `NeedsRefresh: true`.
| `schema.ConfirmButtonItem` | Buttons requiring user confirmation |
| `schema.HyperlinkItem` | Clickable URLs |
| `schema.LabelItem` | Static text or descriptions |

> [!TIP]
> **Dynamic UI Dependencies**
> All schema items support `VisibleIf func() bool` and `EnabledIf func() bool` closures. Use these to dynamically hide or disable dependent settings based on the state of other config values (e.g., hiding a "Sub-setting" if the "Master Toggle" is disabled).

**Example** (Pexels API Key):
```go
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
    return &schema.PanelSchema{
        Sections: []schema.SectionSchema{
            {
                Title: i18n.T("Pexels Settings"),
                Items: []schema.ItemSchema{
                    schema.SecretItem{
                        Name:         "pexelsAPIKey",
                        Label:        i18n.T("pexels API Key:"),
                        Help:         i18n.T("Enter your Pexels API key."),
                        InitialValue: p.cfg.GetPexelsAPIKey(),
                        OnVerify: func(key string) error {
                            return p.verifyAPIKey(key)
                        },
                        OnClear: func() {
                            p.cfg.ClearPexelsAPIKey()
                        },
                    },
                },
            },
        },
    }
}
```

### 3.2 Query Panel (`CreateQueryPanel`)

Constructs the image source list. Use `schema.QueryListItem` for the interactive list, and `schema.AddQueryConfig` for the add-query modal.

**Pattern**:

1. Filter `p.cfg.Queries` by `q.Provider == p.ID()`.
2. Return a `schema.QueryListItem` with pure functions for enable/disable/remove.
3. For user-queryable providers, include a `schema.ButtonItem` that calls `sm.ShowAddQueryDialog()`.

```go
func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, pendingURL string) *schema.PanelSchema {
    return &schema.PanelSchema{
        Sections: []schema.SectionSchema{
            {
                Title: i18n.T("Queries"),
                Items: []schema.ItemSchema{
                    schema.QueryListItem{
                        ID: "myProviderQueries",
                        GetQueries: func() []schema.Query {
                            var queries []schema.Query
                            for _, q := range p.cfg.GetMyProviderQueries() {
                                queries = append(queries, schema.Query{
                                    ID: q.ID, URL: q.URL,
                                    Description: q.Description,
                                    Active: q.Active, Managed: q.Managed,
                                })
                            }
                            return queries
                        },
                        EnableQuery:  func(id string) error { return p.cfg.EnableQuery(id) },
                        DisableQuery: func(id string) error { return p.cfg.DisableQuery(id) },
                        RemoveQuery:  func(id string) error { return p.cfg.RemoveQuery(id) },
                    },
                    schema.ButtonItem{
                        Name:       "addQuery",
                        ButtonText: i18n.T("Add Query"),
                        OnPressed: func() {
                            sm.ShowAddQueryDialog(schema.AddQueryConfig{
                                Title:          i18n.T("Add Query"),
                                URLPlaceholder: "Search term or URL",
                                AddHandler: func(desc, url string, active bool) (string, error) {
                                    return p.cfg.AddMyProviderQuery(desc, url, active)
                                },
                            }, pendingURL, "", func() { sm.RefreshUI() })
                        },
                    },
                },
            },
        },
    }
}
```

## 4. The "Apply" Lifecycle (Schema-Driven)

Spice uses a **Strict Deferred-Save Model**. Changes made in the UI must NOT be saved immediately to disk. They are queued and only committed when the user clicks "Apply".

### 4.1 How It Works (Automatic)

When using schema types, the rendering engine (`RenderSchema`) handles the entire lifecycle for you:

1.  **Seed**: Automatically seeds the baseline value from `InitialValue`.
2.  **Monitor**: Automatically compares the live widget state against the baseline.
3.  **Queue**: When a value differs from baseline, your `ApplyFunc` is queued.
4.  **Apply**: Executes all queued `ApplyFunc` callbacks and promotes "Live" to "Baseline".

**You just declare the schema — the engine does the rest.** No manual `SeedBaseline`, `SetSettingChangedCallback`, or `GetCheckAndEnableApplyFunc` calls needed for standard settings.

### 4.2 Manual Registry (Advanced — Museum Collections)

For curated fixed lists (like museum collections rendered as `schema.BoolItem`), you declare them as normal `BoolItem` elements with `ApplyFunc`. The engine handles dirty tracking automatically:

```go
schema.BoolItem{
    Name:         "collection_asian_art",
    Label:        i18n.T("Arts of Asia"),
    InitialValue: isActive,
    ApplyFunc: func(on bool) {
        if on { p.cfg.EnableQuery(id) } else { p.cfg.DisableQuery(id) }
    },
    NeedsRefresh: true,
}
```

### 4.3 The Transactional Exception (Credentials)

**Sensitive Credentials** (API Keys, Usernames) use `schema.SecretItem`, which bypasses the deferred model:

1.  **Declare**: Set `OnVerify` (network check) and `OnClear` (reset) functions.
2.  **Engine Handles**: The rendering engine manages the full state machine — verify button, loading state, field locking, error display, and baseline seeding.
3.  **Timeouts**: `OnVerify` should use `context.WithTimeout` (10s) internally.
4.  **No Manual Wiring**: Unlike the old pattern, you don't need to call `sm.SeedBaseline()`, `sm.CommitSetting()`, or `sm.RefreshUI()` — the engine does it.

## 5. Pagination & The FIFO Cache Rule (ZERO SHUFFLING)

<AI_AGENT_INSTRUCTION>
**CRITICAL**: You MUST NEVER shuffle the array of IDs returned by an API inside the provider. The provider's ONLY job is to return a stable, deterministic, paginated list. Shuffling is strictly handled downstream by the `MonitorController` display actor.
</AI_AGENT_INSTRUCTION>

### 5.1 The `store.go` FIFO Queue
Spice manages its downloaded images using a global, user-configured cache limit (e.g., 50 images) implemented in `pkg/wallpaper/store.go`. 
The `Sync()` method acts as a strict **FIFO (First-In, First-Out)** queue:
1. As the provider returns pages, new images are downloaded and appended to the store's list.
2. During the nightly or periodic sync, if the total number of images exceeds the cache limit (e.g., `len(images) > 50`), `store.go` slices off the *oldest* excess images from the front of the array and permanently deletes them from disk.

### 5.2 Why Providers Must Never Shuffle
Because `store.go` relies on a deterministic FIFO queue, **if a provider shuffles its return array**, it breaks the queue. `store.go` will see a different list of active IDs every time it syncs, causing it to thrash—randomly pruning and deleting images from the user's hard drive because it thinks the "active" items have changed.

### 5.3 Memory Leak Warning (No Memory-Based Pagination)
Do not fetch the entire collection upfront, cram it into an unbounded `map[string][]int` cache in the provider, and then drip-feed it locally. This causes permanent memory leaks. You must use the remote API's native pagination parameters (e.g., `limit` and `offset` or `page`) directly inside `FetchImages()`.

## 6. ID Namespacing (Automatic)

To prevent ID collisions across providers (e.g., Pexels and Wallhaven both using numeric IDs), Spice applies a **transparent namespacing middleware**.

*   **How it works**: When `FetchImages` returns images with IDs like `123`, the pipeline automatically prefixes them as `YourProvider_123` before storing.
*   **Transparency**: Your provider code never sees the prefix. `EnrichImage` receives the raw ID (`123`), and the pipeline re-applies the namespace after enrichment.
*   **You do nothing**: This is handled entirely by the pipeline. Just return clean, provider-native IDs from your methods.

For full details, see `architecture.md` §3.12.

## 7. Registration (Automated)
 
 Spice uses a code generation tool (`cmd/util/gen_providers`) to automatically register all providers found in `pkg/wallpaper/providers/`.
 
 ### 7.1 The Logic
 
 1.  **Auto-Discovery**: The tool scans the `providers/` directory for subdirectories.
 2.  **Generation**: It creates `cmd/spice/zz_generated_providers.go`, which contains the necessary `_` imports to trigger the `init()` functions of your providers.
 3.  **Build Integration**: The generation runs automatically via `go generate` (called by `make build` or `make run`).
 
 ### 7.2 Disabling a Provider
 
 To temporarily disable a provider without deleting the code:
 
 1.  Create an empty file named `.disabled` inside the provider's directory (e.g., `pkg/wallpaper/providers/myprovider/.disabled`).
 2.  Run `go generate ./...` (or `make gen`).
 3.  The tool will skip this directory when generating `zz_generated_providers.go`, effectively compiling it out of the final binary.
 
 ### 7.3 Manual imports (Legacy/Debug)
 
 You do **not** need to manually edit `cmd/spice/main.go` anymore. The `//go:generate` directive at the top of `main.go` handles this.


## 8. Internationalization (i18n) Best Practices

All user-facing strings in your provider **must** use the `i18n` package. This includes settings labels, validation errors, descriptions, button text, and tray menu items.

### 8.1 Usage

```go
import "github.com/dixieflatline76/Spice/v2/pkg/i18n"

// Simple string
label := i18n.T("My Provider Settings")

// Templated string
status := i18n.Tf("Downloading {{.Count}} images", map[string]any{"Count": len(images)})
```

### 8.2 After Adding New Strings

Whenever you add new `i18n.T("...")` calls to your provider, you must run the extraction tool:

```bash
make gen-i18n
```

This command parses the Go AST, automatically extracts the new english strings, inserts them into `en.json`, and propagates them as placeholder keys across all other language files (`pkg/i18n/translations/*.json`). You (or the localization team) then must translate the new values in each respective JSON file.

### 8.3 Legitimately Identical Strings (Proper Nouns)

Release builds gate on `make check-i18n`, which will **block the CI/PR** if any strings in foreign language files are exactly identical to their English counterparts (as this usually implies an untranslated placeholder). 

However, many strings—especially Proper Nouns, museum names, and geographic locations—are legitimately identical across languages (e.g., "The Getty", "Los Angeles, CA, USA").

If you encounter CI failures for these strings:
1. Open `cmd/util/gen_i18n/main.go`.
2. Add your string to the `allowIdenticalToEnglish` map.
3. This bypasses the strict translation check for that specific key.

### 8.4 Dynamic Keys

If your provider selects translation keys at runtime via a variable (e.g. iterating over a JSON object), the AST parser will not see them. Add these keys to the `dynamicI18nKeys` list in `cmd/util/gen_i18n/main.go` to prevent the CI checker from flagging them as "stale" keys.

For full details, see `internal_developer_context.md` §9.

## 9. Testing

* **Unit**: Test `ParseURL` with table-driven tests.
* **Integration**: Mock the `http.Client` or usage `httptest.Server` to test `FetchImages` without real network calls.
* **UI**: UI testing is optional but recommended if complex.

## 10. Browser Extension Integration

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


## 11. Rate Limiting — Decision Tree

Spice has three distinct rate limiting mechanisms. Choose based on your API's behavior:

| Scenario | Mechanism | What to Implement |
| :--- | :--- | :--- |
| **Standard API limits** (e.g., 45 RPM) | **`PacedProvider`** | Return appropriate `time.Duration` from `GetAPIPacing()` and `GetProcessPacing()`. The Fair Bouncer Dispatcher spaces out jobs automatically. |
| **Aggressive 429 responses** (e.g., Wikimedia) | **`CustomClientProvider`** + Circuit Breaker | Return a custom `*http.Client` with a `RoundTripper` that implements a global circuit breaker. Halts all workers instantly on 429. |
| **Fragile / slow APIs** (e.g., Museum endpoints) | **`errgroup` + manual delays** | Use `errgroup.SetLimit(N)` for concurrency caps and/or `time.Sleep` between batches. Consider a `sync.Mutex`-based `RoundTripper` for strict serialization. |

**Rules of Thumb:**
*   If your API publishes a rate limit (e.g., "45 requests per minute"), use `PacedProvider`. It's the simplest.
*   If your API aggressively returns HTTP 429 and bans repeat offenders, use `CustomClientProvider` with a circuit breaker transport.
*   If your API is undocumented or fragile (common with museum/institutional APIs), be conservative — use `errgroup` with a low limit (3–5) and manual sleep between requests.
*   **Never** rely on the 16 generic pipeline workers for pacing — they execute immediately. All pacing must happen upstream.

## 12. The Museum Template (v1.6+)

For cultural institutions (Museums, Archives), Spice provides a standardized "Evangelist" UI template designed to drive engagement rather than just utility.

### 12.1 Core Components (`pkg/ui/schema/museum.go`)

*   **Header**: Use `schema.CreateMuseumSettingsPanel(cfg, openURL)`.
    *   **Config**: `schema.MuseumSettingsConfig{ ID, Title, Location, LicenseURL, Description, MapQuery, WebsiteURL, DonateURL }`
    *   **Features**:
        *   **"Plan a Visit" Button**: Automatically renders with a map pin icon, linking to Google Maps.
        *   **"Visit Website" / "Donate" Buttons**: Standard action buttons for engagement.
        *   **Clickable License**: Supports explicit licensing links (e.g., CC0) in the header metadata.
        *   **Romance Copy**: Supports long-form, evocative descriptions via `LabelItem` with `ImportanceLow`.

### 12.2 Collections as Tours
Instead of raw database categories, frame collections as curated experiences:
*   **Bad**: "Department 1", "Asian Art".
*   **Good**: "Director's Cut", "Arts of Asia", "The Impressionist Era".

### 12.3 Interaction Model
*   **Fixed List**: Use `schema.BoolItem` elements for collections. The engine renders them as checkboxes with automatic dirty tracking.
*   **No Delete**: Unlike generic queries, these are permanent fixtures of the provider.
*   **Toggle Logic**: Map `ApplyFunc` directly to `cfg.EnableQuery` / `cfg.DisableQuery`.

