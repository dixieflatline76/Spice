# How to Create a New Museum Provider

This guide details how to implement a new "Museum Template" provider in Spice, meant specifically for cultural institutions and open access repositories (like the Metropolitan Museum of Art or the Art Institute of Chicago). Museum providers follow a distinct pattern compared to standard search-based providers (like Pexels or Wallhaven).

## 1. The Core Philosophy

Unlike generic image providers where users type arbitrary search queries, **Museum Providers are curated experiences.**

*   **No User Search Bars:** Users do not type search terms.
*   **Curated Tours:** Collections are presented as fixed "Tours" or "Highlights" (e.g., "Director's Cut", "Impressionist Vistas", "American Wing").
*   **Evangelist UI:** The UI is designed to promote the institution, with a large header, romance copy, and links to "Plan a Visit" and donate.
*   **Remote Curation (CDN):** The list of IDs that define a "Tour" is often driven by a remote JSON file on GitHub, allowing the developer to update the curated collections without shipping a new binary of Spice.

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
      "name": "Director's Cut",
      "ids": [ 1234, 5678, 91011 ]
    }
  }
}
```

## 5. Overriding `resolveQueryToIDs`

In `FetchImages`, queries are not search strings, but the map keys (like `highlights`). 

```go
func (p *Provider) resolveQueryToIDs(ctx context.Context, query string) ([]int, error) {
    // 1. Check ID Cache
    // 2. Look up `query` in p.curatedList.Tours[query].IDs
    // 3. Fallback to API search if query is an unexpected Custom Search or Object ID
    // 4. Stable Sort IDs
    // 5. Shuffle if p.cfg.GetImgShuffle() is true
    // 6. Store in Cache and return
}
```
*Note: Stable ID sorting is critical to ensure API pagination aligns safely with shuffle mechanics.*

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
