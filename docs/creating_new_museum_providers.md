# How to Create a New Museum Provider

This guide details how to implement a new "Museum Template" provider in Spice, meant specifically for cultural institutions and open access repositories (like the Metropolitan Museum of Art or the Art Institute of Chicago). Museum providers follow a distinct pattern compared to standard search-based providers (like Pexels or Wallhaven).

## 1. The Core Philosophy

**The "Why"**: The general purpose of a Museum Provider is to surface high-quality, public domain fine art. However, museums are not generic image aggregators. If we simply presented a search bar for the Metropolitan Museum of Art, users might search for "dogs" and get pictures of ancient, broken clay dog figurines instead of beautiful wallpapers. 
To guarantee a premium aesthetic experience, Museum Providers are strictly **curated experiences**. We act as the museum curators for the user, doing the heavy lifting of finding the 50 best landscapes or portraits and bundling them into one-click toggles.

*   **No User Search Bars:** Users do not type search terms because raw museum API results are wildly unpredictable in quality.
*   **Curated Tours:** Collections are presented as fixed "Tours" or "Highlights" (e.g., "Director's Cut", "Impressionist Vistas", "American Wing").
*   **Evangelist UI:** The UI is designed to promote the institution, with a large header, romance copy, and links to "Plan a Visit" and donate. We use their open APIs for free, so we give them premium placement in our settings UI.
*   **Remote Curation (CDN):** The list of IDs that define a "Tour" is driven by a remote JSON file on GitHub. This allows developers/curators to update the curated collections (e.g., adding newly digitized masterpieces) without shipping a new binary of Spice.

## 2. Directory Structure

A museum provider requires at least three core files inside `pkg/wallpaper/providers/<name>/`:

```text
pkg/wallpaper/providers/<name>/
├── <name>.go         # Main provider logic and UI implementation
├── const.go          # Regex, endpoints, and collection keys
├── remote.go         # Remote JSON fetching, caching, and fallback logic
├── <name>.json       # Embedded fallback curation data
├── <name>.png        # 64x64px provider icon
```

> Ensure you generate `.png` assets for the museum logo to display in the UI.

## 3. Implementing the Museum UI (Schema-Based)

Museums use `schema.CreateMuseumSettingsPanel` for the rich header, and `schema.BoolItem` for the curated collection toggles. **No Fyne imports are needed** — everything is declared as pure Go schema structs.

### 3.1 Settings Panel (Header)

```go
func (p *Provider) CreateSettingsPanel(sm setting.SettingsManager) *schema.PanelSchema {
    return schema.CreateMuseumSettingsPanel(schema.MuseumSettingsConfig{
        ID:          "CMA",
        Title:       i18n.T("Cleveland Museum of Art"),
        Location:    i18n.T("Cleveland, OH, USA"),
        LicenseURL:  "https://www.clevelandart.org/open-access",
        Description: i18n.T("Discover thousands of masterpieces..."),
        MapQuery:    "Cleveland Museum of Art",
        WebsiteURL:  "https://www.clevelandart.org",
        DonateURL:   "https://give.clevelandart.org",
        SupportsFraming: true, // Enables the Virtual Museum Frame auto-salvage toggle
    }, sm.OpenURL)
}
```

### 3.2 Query Panel (Curated Tours)

```go
func (p *Provider) CreateQueryPanel(sm setting.SettingsManager, _ string) *schema.PanelSchema {
    tourItems := make([]schema.ItemSchema, 0)
    for key, tour := range p.curatedList.Tours {
        key := key // shadow for closure
        isActive, queryID := p.getQueryState(key)

        tourItems = append(tourItems, schema.BoolItem{
            Name:         "cma_tour_" + key,
            Label:        tour.Name,
            InitialValue: isActive,
            ApplyFunc: func(on bool) {
                if on {
                    p.cfg.EnableQuery(queryID)
                } else {
                    p.cfg.DisableQuery(queryID)
                }
            },
            NeedsRefresh: true,
        })
    }

    return &schema.PanelSchema{
        Sections: []schema.SectionSchema{
            {
                Title:   i18n.T("Curated Tours"),
                Compact: true,
                Items:   tourItems,
            },
        },
    }
}
```
The rendering engine handles all dirty tracking, Apply button state, and widget creation automatically.

> [!NOTE]
> **Cache Invalidation**
> Notice `NeedsRefresh: true` in the snippet above. If a user toggles a museum tour, this triggers the `RefreshImagesAndPulse()` event. The central Image Store will reconcile its derivative cache against the new configuration, automatically invalidating stale files and refreshing the monitor wallpapers. Always set this flag if your setting alters image processing or fetching logic.

## 4. Remote Curation (`remote.go`)

Museum APIs often have hundreds of thousands of items, many of which are boring (coins, broken pottery) or portraits. Spice solves this by maintaining a hardcoded list of "Good Wallpapers", managed via a `remote.go` pattern.

### 4.1 The Fetch Hierarchy
The provider must implement a hierarchy to get the curated IDs:
1.  **Remote GitHub JSON:** Attempt to fetch the latest `docs/collections/<name>.json` from the `main` branch.
2.  **Local Cache:** If offline or timed out, fall back to `cache/<name>/<name>_cache.json`.
3.  **Embedded Go Asset:** If cache is missing, use a `//go:embed` JSON file compiled into the binary.
4.  **Hardcoded Struct:** Ground zero fallback if parsing fails.

### 4.2 Curation File format
The curated lists map a logical string key to a slice of integer/string object IDs from the provider's API.

```json
{
  "version": 1,
  "description": "CMA Highlights",
  "tours": {
    "highlights": {
      "type": "curated",
      "name": "Director's Cut",
      "ids": [ 1234, 5678, 91011 ]
    },
    "european_paintings": {
      "type": "search",
      "name": "European Paintings",
      "query": "departmentId=11&q=painting"
    }
  }
}
```

### 4.3 Curated vs Dynamic Queries
Museum Providers typically support two types of collections, differentiated by the `"type"` field in the JSON:
1. **Curated (`"type": "curated"`)**: The most common type. A hardcoded array of `ids` that point directly to hand-picked masterpieces.
2. **Dynamic Search (`"type": "search"`)**: Used when a museum has an excellent API and you want to pull live results (e.g., all items in a specific department). Instead of `ids`, it provides a `query` string (like URL parameters). The provider code reads this `query` string and dynamically fetches a list of IDs from the museum's REST API search endpoint before paginating through them.

### 4.4 Providers Without Search/Listing APIs
Some modern museum APIs (especially strictly academic Linked Open Data APIs like the J. Paul Getty Museum) do not expose public, easily paginated REST `search` endpoints. Instead, they may only support complex academic query languages (like SPARQL) or require you to already know the exact Object UUID.

If you encounter a museum without a functional search/listing API:
* **Strictly Curated Only**: Do not attempt to reverse-engineer their frontend GraphQL or write brittle web scrapers to dynamically "search" the museum. The provider must be implemented as strictly `curated` only.
* **Hand-Curate the JSON**: You must manually gather the Object IDs (or write an offline script to scrape the Object IDs once) and embed them directly into the `"ids": []` array of your remote curation JSON.
* **Filter in Code**: In your provider's `FetchImages` method, if the requested collection type is not `"curated"`, simply return `nil, nil` to gracefully ignore unsupported dynamic queries.

## 5. Overriding `resolveQueryToIDs` (DEPRECATED)

<AI_AGENT_INSTRUCTION>
**CRITICAL**: You MUST NEVER shuffle IDs returned by an API inside the provider, and you MUST NEVER cache IDs in unbounded memory maps (`idCache` or `poolCache`). The provider's ONLY job is to return a stable, deterministic, paginated list using native API pagination parameters (like `offset` or `limit`).
</AI_AGENT_INSTRUCTION>

In the past, `resolveQueryToIDs` was used to fetch the entire collection upfront, cram it into an unbounded memory map, and shuffle it locally. **This is strictly banned.** It causes infinite memory leaks and breaks `store.go`'s FIFO cache queue limit.

If you are implementing a new museum provider, you must stream the results directly from the museum's REST API natively in `FetchImages` using their pagination parameters. For curated lists (like `npm.json`), simply read the `IDs` array and slice it dynamically using `pageSize`.

## 6. Resolution & Shape Filtering

Museum APIs usually don't categorize images by orientation. Since Spice now fully supports multi-monitor setups (including vertical/portrait monitors), we **no longer enforce strict landscape filtering**. The `SmartImageProcessor` gracefully handles aspect ratio scaling, cropping, and panning based on the destination monitor's true orientation.

However, you should still filter out extreme panoramas or extremely thin slivers (e.g., a ratio > 3.0 or < 0.33) as these do not make good wallpapers regardless of orientation.

Inside your `fetchObjectDetails` (or equivalent) loop, ensure the image is of a reasonable shape and high enough resolution:
```go
func isUsableShape(width, height float64) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	ratio := width / height
	
	// Allow both landscape and portrait, but reject extreme slivers or scrolls
	if ratio > 3.0 || ratio < 0.33 {
	    return false 
	}
	
	// Suggest returning true if the minimum resolution is high enough (e.g. 1080px)
	// minSide := math.Min(width, height)
	// return minSide >= 1080
	return true
}
```

## 7. Rate Limiting

Museum APIs are often fragile or strictly rate-limited.
*   **MET approach:** Uses `errgroup` with a concurrent limit of `5`, plus a manual `time.Sleep` between batches.
*   **AIC approach:** Uses a highly strict `http.RoundTripper` middleware containing a `sync.Mutex` and a mandatory 1.5s delay between every single HTTP request.

Determine your provider's limits and implement responsible scraping. If using IIIF for high-res images, construct standard IIIF URLs to offload processing to standard museum image servers.


## 7. Localization (i18n) for Museums

When implementing your UI panels, all user-facing strings must be wrapped in `i18n.T("...")`. 

However, Museum Providers frequently include **Proper Nouns** (like the museum's name, e.g., "The J. Paul Getty Museum") and geographic locations (e.g., "Los Angeles, CA, USA") that do not translate and are perfectly valid to remain as their English equivalents in other languages.

Because our CI pipelines enforce strict translation checks (`make check-i18n`) and fail if a translated string is identical to the English original, you must whitelist these specific proper nouns.

**To whitelist proper nouns:**
1. Open `cmd/util/gen_i18n/main.go`.
2. Locate the `allowIdenticalToEnglish` map.
3. Add your museum's proper nouns to this list:
   ```go
   "The J. Paul Getty Museum": true,
   "Los Angeles, CA, USA":     true,
   ```

For a full guide on extracting strings and handling dynamic keys, see `docs/creating_new_providers.md` **Section 8: Internationalization (i18n) Best Practices**.

## 8. Attribution Strings

When setting the `Attribution` field on the `provider.Image` struct, **DO NOT prepend the museum's name** (e.g., `"The Getty - [Title]"` or `"NPM - [Title]"`). 

Because museum providers typically return `provider.AttributionBy` from their `GetAttributionType()` method, the frontend UI automatically handles prepending the provider's Title to the image metadata (e.g., rendering as "Photo by The J. Paul Getty Museum").

**Correct Pattern:**
Use only the artwork title and the author (or artist).
```go
img := &provider.Image{
    Attribution: fmt.Sprintf("%s - %s", author, title),
}
```

If you hardcode the museum name into the attribution string, it will result in redundant UI text for the user (e.g., "By The Getty - The Getty - [Title]").
