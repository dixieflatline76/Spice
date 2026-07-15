---
layout: default
title: Image Pipeline Deep Dive
---

# Image Pipeline Deep Dive

> **Status**: Current as of v2.5.3
> **Scope**: Detailed walkthrough of how images flow from API fetch → store → monitor display.
> **Companion to**: [architecture.md](architecture.md) (system-level overview)

This document is a deep reference for understanding the complete image data flow in Spice.
It covers each system involved, the data structures they operate on, and how they interact —
including known failure modes and the invariants they rely on.

---

## Table of Contents

1. [The Image Store](#1-the-image-store)
2. [Resolution Buckets](#2-resolution-buckets)
3. [The Monitor Controller](#3-the-monitor-controller)
4. [The Pipeline](#4-the-pipeline)
5. [ProcessImageJob: Step-by-Step](#5-processimagejob-step-by-step)
6. [store.Update() vs store.Add()](#6-storeupdate-vs-storeadd)
7. [The Nightly Refresh Cycle](#7-the-nightly-refresh-cycle)
8. [Known Failure Modes](#8-known-failure-modes)

---

## 1. The Image Store

**File**: `pkg/wallpaper/store.go`

The **ImageStore** is Spice's central database of all known images. It lives in memory as a
Go slice (`[]provider.Image`) and is periodically saved to disk as JSON at:

```
<AppData>/Spice/wallpaper_downloads/image_cache_map.json
```

### 1.1 The Image Struct

Each image in the store has these critical fields:

```go
type Image struct {
    ID              string                 // e.g., "MetMuseum_436530"
    Path            string                 // Download URL from the museum API
    FilePath        string                 // Local path after download (set by pipeline)
    DerivativePaths map[string]string      // KEY FIELD: "3440x1440" → local fitted file path
    ProcessingFlags map[string]bool        // e.g., {"SmartFit": true, "FaceCrop": true, ...}
    SourceQueryID   string                 // Links image to the query that fetched it
    Width           int                    // Image dimensions (may be probed post-download)
    Height          int
    IsFavorited     bool
    Seen            bool
    CropAnchors     map[string]CropAnchor  // Per-resolution manual anchor overrides
    // ... other fields like Attribution, Provider, ViewURL, etc.
}
```

### 1.2 DerivativePaths — The Most Important Field

`DerivativePaths` is a map that stores the path to the **processed/cropped version** of the image
for each monitor resolution. For example:

```json
{
    "3440x1440": "C:\\...\\fitted\\flexibility\\facecrop\\3440x1440\\MetMuseum_436530.jpg",
    "primary": "C:\\...\\fitted\\flexibility\\facecrop\\3440x1440\\MetMuseum_436530.jpg"
}
```

**If `DerivativePaths` is `null` or empty, the image is invisible to the rotation system.**
This is because resolution buckets (see §2) are built from this field. No entries = no buckets = 
monitor never sees the image.

### 1.3 Internal Indexes

The store maintains several internal indexes for O(1) access:

- **`idSet`** (`map[string]bool`): Fast existence check by ID. Used by `Add()` to reject duplicates.
- **`pathSet`** (`map[string]bool`): Fast seen-check by file path. Used by `MarkSeen()`.
- **`resolutionBuckets`** (`map[string][]string`): Maps resolution strings to image ID lists (see §2).

### 1.4 Persistence

The store uses a **debounced save** mechanism:

1. Any mutation (`Add`, `Update`, `Remove`, `Sync`) calls `scheduleSaveLocked()`
2. This starts or resets a 2-second timer
3. When the timer fires, it takes a snapshot of `s.images` under `RLock` and writes JSON asynchronously
4. A separate `saveMu` mutex serializes writes to prevent interleaved disk I/O

---

## 2. Resolution Buckets

**File**: `pkg/wallpaper/store.go` — `rebuildBucketsLocked()`

Resolution buckets are a performance optimization that maps each monitor resolution to the list
of image IDs that have a fitted derivative for that resolution:

```
resolutionBuckets = {
    "3440x1440": ["MetMuseum_436530", "ClevelandMuseum_145473", ...],
    "1920x1080": ["MetMuseum_436530", "ClevelandMuseum_145473", ...],
}
```

### 2.1 How Buckets Are Built

The bucket-building logic is straightforward:

```go
func (s *ImageStore) rebuildBucketsLocked() {
    s.resolutionBuckets = make(map[string][]string)
    for _, img := range s.images {
        for res := range img.DerivativePaths {      // If DerivativePaths is nil, this loop
            s.resolutionBuckets[res] = append(...)  // body never executes. Image is invisible.
        }
    }
}
```

### 2.2 When Buckets Are Rebuilt

Buckets are rebuilt (from scratch) on every store mutation:
- `Add()` — new image added
- `Update()` — existing image modified
- `Remove()` — image deleted
- `Sync()` — batch grooming operation
- `LoadCache()` — cache loaded from disk at startup

### 2.3 Invariant

> **An image exists in a resolution bucket if and only if its `DerivativePaths` map contains
> that resolution key.** Any code path that sets `DerivativePaths` to nil/empty without intending
> to remove the image from rotation must be considered a bug.

---

## 3. The Monitor Controller

**File**: `pkg/wallpaper/monitor_controller.go`

Each physical monitor has its own **MonitorController** actor. When it's time to rotate the
wallpaper, the controller calls `next()`. Here's the critical flow:

### 3.1 Image Selection (`next()`)

```go
func (mc *MonitorController) next(manual bool) {
    // 1. Determine this monitor's resolution
    resKey := fmt.Sprintf("%dx%d", width, height)  // e.g., "3440x1440"

    // 2. Ask the store: "which image IDs have a fitted file for my resolution?"
    bucketIDs := mc.Store.GetIDsForResolution(resKey)

    // 3. Starvation check
    if len(bucketIDs) == 0 {
        mc.State.WaitingForImages = true  // Will retry when store signals update
        return
    }

    // 4. Reconcile shuffle deck with current bucket
    //    (handles new arrivals, removals, deck exhaustion)
    if len(mc.State.ShuffleIDs) != len(bucketIDs) {
        mc.rebuildShuffle(bucketIDs) // or mc.growShuffle(bucketIDs)
    }

    // 5. Pick next image from shuffle deck
    nextID := mc.State.ShuffleIDs[mc.State.RandomPos]
    mc.State.RandomPos++

    // 6. Look up and apply
    if img, ok := mc.Store.GetByID(nextID); ok {
        mc.applyImage(img)
    }
}
```

**Critical insight**: The monitor controller ONLY considers images that appear in the resolution
bucket for its resolution. An image could be in the store with a beautiful master file and even
a fitted file on disk — but if `DerivativePaths` is null in the store's metadata, the resolution
bucket won't contain it, and the monitor will never show it.

### 3.2 Shuffle Strategies

- **`rebuildShuffle(ids)`**: Full Fisher-Yates shuffle, position reset to 0. Used on initial build, 
  deck exhaustion, or when the pool shrinks.
- **`growShuffle(bucketIDs)`**: Incrementally inserts new arrivals into the unplayed portion of 
  the deck. Prevents provider clustering when images arrive in same-provider bursts.

### 3.3 Store Update Channel

The controller subscribes to the store's update channel (broadcast pattern via channel close):

```go
case <-updateCh:
    updateCh = mc.Store.GetUpdateChannel()  // Refresh for next event
    mc.pendingUpdate = true
    if mc.State.WaitingForImages {
        mc.next(mc.State.ManualRecovery)    // Retry image selection
    }
```

### 3.4 Stale File Detection (`applyImage`)

Before setting a wallpaper, the controller checks if the file physically exists:

```go
if _, err := mc.os.Stat(path); os.IsNotExist(err) {
    // File is missing — clear stale metadata
    img.DerivativePaths = make(map[string]string)
    img.ProcessingFlags = make(map[string]bool)
    mc.Store.Update(img)
    // Request re-fetch
    mc.OnFetchRequest()
    return
}
```

This is an important **intentional clearing** of `DerivativePaths` — the image's derivative was
deleted from disk (perhaps by an external cleanup or OS action), so the metadata should be cleared
to prevent the monitor from repeatedly trying to show a missing file.

---

## 4. The Pipeline

**File**: `pkg/wallpaper/pipeline.go`

The pipeline manages the worker pool that processes image downloads and fitting.

### 4.1 Components

```
FetchNewImages()
      ↓
  Dispatcher (per-provider rate limiting via pump goroutines)
      ↓
  jobChan (unbuffered — strict pacing guarantee)
      ↓
  Worker Pool (N goroutines, each calls ProcessImageJob)
      ↓
  resultChan (buffered: 100)
      ↓
  stateManagerLoop (SINGLE goroutine — serializes all store writes)
      ↓
  store.Add(result.Image)
```

### 4.2 The Dispatcher (Fair Scheduler)

Each provider gets its own `pump` goroutine that enforces API-specific rate limits. This prevents
a slow provider (e.g., Wallhaven with 1.5s pacing) from blocking fast providers (e.g., local
folder with 0ms pacing). Jobs are released to the shared `jobChan` only when fully ready.

### 4.3 The State Manager Loop

A single consumer goroutine that processes pipeline results and state commands:

```go
func (p *Pipeline) stateManagerLoop() {
    for {
        select {
        case res := <-p.resultChan:
            if res.Error != nil { continue }
            if !p.store.Add(res.Image) {
                // Image already exists (re-processed via backlog healing).
                // Update so the fully-processed result with DerivativePaths lands.
                p.store.Update(res.Image)
            }
        case cmd := <-p.cmdChan:
            // MarkSeen, Remove, Clear commands
        }
        runtime.Gosched()  // Yield to UI readers
    }
}
```

### 4.4 The Deduplication Gate (fetch_logic.go)

Before submitting a job to the pipeline, `fetchFromProvider` checks if the image already exists
in the store with all required derivatives:

```go
if existing, exists := wp.store.GetByID(img.ID); exists {
    if wp.allMonitorDerivativesExist(existing) {
        continue  // Skip — already fully processed
    }
    // "Backlog healing": Allow re-processing for images missing derivatives
    // Merge existing metadata (dimensions, flags) into the new img
    img.Width = existing.Width
    img.Height = existing.Height
    if img.ProcessingFlags == nil {
        img.ProcessingFlags = make(map[string]bool)
    }
    for k, v := range existing.ProcessingFlags {
        img.ProcessingFlags[k] = v
    }
    // NOTE: existing.DerivativePaths is NOT merged into img
}
```

---

## 5. ProcessImageJob: Step-by-Step

**File**: `pkg/wallpaper/downloader.go`

This is the most complex function in the pipeline. It implements the Source + Derivative
architecture: ensure the raw master file exists, then ensure fitted derivatives exist for each
monitor resolution.

### 5.1 Complete Flow

```
ProcessImageJob(ctx, job) → (provider.Image, error)

Input:
  job.Image    = {ID, Path, Attribution, Width?, Height?, ...}
                 (From API — has NO FilePath, NO DerivativePaths)
  job.Provider = The ImageProvider that sourced this image

Step 0: Early Filtering
  - Check image dimensions against all monitor resolutions
  - Skip if incompatible with every monitor

Step 1: Enrichment (Soft Fail)
  - Call provider.EnrichImage() for metadata (e.g., Wallhaven fetches uploader name)
  - Returns modified img struct (does NOT touch the store)
  - Failure is non-fatal — image proceeds with whatever metadata it has

Step 2: Ensure Master (Raw Image)
  - Check if master file exists on disk (by ID + extension)
  - If not: download from URL to <downloads>/<ID>.<ext>
  - Returns absolute path to master file

Step 2.5: Resolution Probing (Conditional)
  - If img.Width == 0 or img.Height == 0:
    - Probe the master file for dimensions
    - Set on local img variable (NOT persisted to store here)
    - Dimensions are persisted when the final result lands via stateManagerLoop

Step 2.7: Multi-Monitor Compatibility Tagging
  - For each monitor resolution, check if the image can be fitted
  - Tag incompatible resolutions: img.ProcessingFlags["incompatible:WxH"] = true
  - Tags are set on the local img variable (NOT persisted to store here)
  - Persisted with the final result via stateManagerLoop

Step 3: Ensure Derivatives
  - For each unique monitor resolution:
    - Check if fitted file already exists on disk
    - If not: load master, run FitImage (smart crop/resize), save to:
      <downloads>/fitted/<mode>/<type>/<WxH>/<ID>.<ext>
  - Returns: map[string]string{"3440x1440": "/path/to/fitted.jpg", "primary": "..."}

Step 4: Finalize
  - img.DerivativePaths = derivativePaths   ← SET HERE
  - img.FilePath = path                     ← SET HERE
  - Return img (fully populated)
  - stateManagerLoop persists via Add-or-Update fallback
```

### 5.2 Provider Enrichment Patterns

All providers implement `EnrichImage()`. Critically, **none of them call `store.Update()`** —
they return a modified local `img` struct:

| Provider | What EnrichImage does | Store impact |
|----------|----------------------|-------------|
| Wallhaven | 2nd API call to fetch uploader username (if missing) | None — local only |
| MetMuseum | Fetches object details (artist name) via API | None — local only |
| Cleveland | Sets attribution from existing data | None — local only |
| Rijksmuseum | No-op enrichment | None |
| Art Institute Chicago | Enrichment stub | None |
| Pexels | Fetches photographer info | None — local only |
| Wikimedia | Fetches image metadata | None — local only |
| Local Folder | No-op | None |
| Google Photos | Fetches photo metadata | None — local only |
| Favorites | Enrichment stub | None |

### 5.3 The Derivative Directory Structure

Derivatives are stored in a directory hierarchy based on the current processing settings:

```
wallpaper_downloads/
├── <ID>.jpg                          ← Master (raw downloaded file)
└── fitted/
    ├── flexibility/                  ← SmartFit Aggressive mode
    │   ├── facecrop/                 ← FaceCrop enabled
    │   │   ├── 3440x1440/           ← Resolution-specific
    │   │   │   ├── MetMuseum_436530.jpg
    │   │   │   └── ...
    │   │   └── 1920x1080/
    │   ├── faceboost/                ← FaceBoost enabled
    │   └── standard/                ← No face processing
    └── quality/                      ← SmartFit Normal mode
        ├── facecrop/
        ├── faceboost/
        └── standard/
```

When the user changes processing settings (e.g., toggles FaceCrop), the nightly `Sync()` detects
a flag mismatch and **invalidates** all derivatives — wiping `DerivativePaths` and
`ProcessingFlags` so images are re-processed with the new settings.

### 5.4 The Three Compatibility Gates (Multi-Monitor & Mode Resiliency)

The downloader employs three distinct `CheckCompatibility` gates throughout its lifecycle. While this might appear redundant at first glance, it is a highly resilient architecture designed to protect multi-monitor setups and handle dynamic user setting changes gracefully.

1. **Gate 1: The API Gate (`checkImageCompatibility`)**
   - **When:** Immediately after fetching the API JSON (before any disk I/O).
   - **Why:** Bandwidth optimization. If an image is mathematically incompatible with all monitors based on the API data, we reject it before downloading the 100MB master file. *(Note: Many museum APIs do not report dimensions in their JSON, so this gate is often bypassed).*
2. **Gate 2: The Multi-Monitor Tagging Loop (Step 2.7)**
   - **When:** After the master image is downloaded and its exact dimensions are probed from disk.
   - **Why:** It iterates through the unique resolution profile of **every single plugged-in monitor**. It permanently tags the image (e.g., `img.ProcessingFlags["incompatible:3440x1440"] = true`) if it fails a specific monitor's geometry. This ensures an image is kept if it perfectly fits your 16:9 laptop screen, but tells the routing system to never send it to your 21:9 ultrawide. If the image fits *zero* monitors, the job aborts and the master file is deep-deleted.
3. **Gate 3: The Generation Loop Safety Net (Step 3)**
   - **When:** Inside `generateMissingDerivatives`, immediately before generating the fitted file.
   - **Why:** Instead of just trusting the `incompatible` tag set by Gate 2, it dynamically re-evaluates the math against current global settings. This solves the **Mode Change Problem**. If an image was tagged incompatible yesterday under strict "Quality Mode", but the user switches to permissive "Flexibility Mode" today and opens the Tune Image UI, Gate 3 ignores the stale tag. It runs the math, sees it now passes under Flexibility Mode, and processes the crop.

---

## 6. store.Update() vs store.Add()

This distinction is critical for understanding data flow and avoiding corruption.

### 6.1 Add()

Adds a **new** image to the store. Rejects duplicates.

```go
func (s *ImageStore) Add(img provider.Image) bool {
    if _, exists := s.idSet[img.ID]; exists {
        // Special case: Favorites provider can displace existing entries
        if img.SourceQueryID == FavoritesQueryID {
            // In-place update preserving Seen state
            s.images[i] = img
            return true
        }
        return false  // Duplicate — reject
    }
    // Reject images from queries disabled mid-download
    if s.QueryActiveFunc != nil && !s.QueryActiveFunc(img.SourceQueryID) {
        return false
    }
    if s.avoidSet[img.ID] { return false }  // Blocked image
    s.images = append(s.images, img)
    s.idSet[img.ID] = true
    // Add to resolution buckets (incremental, not full rebuild)
    for res := range img.DerivativePaths {
        s.resolutionBuckets[res] = append(s.resolutionBuckets[res], img.ID)
    }
    s.scheduleSaveLocked()
    s.notifyUpdateLocked()
    return true
}
```

**Key behavior**: If the ID already exists, `Add()` returns `false` and does **nothing**
(unless the source is the Favorites provider, which is allowed to displace existing entries).
This means a re-processed image must use the Add-or-Update fallback pattern in
`stateManagerLoop` to ensure its results land in the store.

### 6.2 Update()

Replaces an **existing** image by ID. Full struct replacement.

```go
func (s *ImageStore) Update(img provider.Image) bool {
    for i, existing := range s.images {
        if existing.ID == img.ID {
            s.images[i] = img  // FULL REPLACEMENT. Every field overwritten.
            s.rebuildBucketsLocked()
            s.scheduleSaveLocked()
            return true
        }
    }
    return false
}
```

**Key behavior**: There is **no field-level merge**. If you call `Update()` with an image that
has `DerivativePaths = nil`, it will overwrite the existing image's `DerivativePaths` — even if
the existing one was perfectly good.

### 6.3 All Call Sites (as of v2.5.3)

| # | Location | Purpose | Has DerivativePaths? |
|---|----------|---------|---------------------|
| 1 | `pipeline.go:150` | Add-or-Update fallback in stateManagerLoop | Yes (fully processed) |
| 2 | `wallpaper.go:831` | Un-favorite | Yes (from store.List()) |
| 3 | `wallpaper.go:841` | Add favorite | Yes (from store.List()) |
| 4 | `wallpaper.go:905` | Reconcile favorite flags | Yes (from store.List()) |
| 5 | `monitor_controller.go:502` | Stale file → clear derivatives | Intentionally empty |
| 6 | `monitor_controller.go:545` | Anchor update | Yes (from CurrentImage) |

> **Note**: Prior to v2.5.3, `downloader.go` had two additional intermediate `Update()` calls
> at steps 2.5 (resolution probing) and 2.7 (incompatibility tagging) that clobbered
> `DerivativePaths`. These were removed as part of the fix described in §8.1.

Call site 5 is an intentional clearing — any guard on `Update()` must not block it.

---

## 7. The Nightly Refresh Cycle

**File**: `pkg/wallpaper/scheduler.go`

The nightly scheduler runs as a persistent goroutine and triggers maintenance on day changes.

### 7.1 Trigger Conditions

A nightly refresh runs when:
- The app starts and detects it's shortly after midnight (within 6 minutes)
- The day has changed since the last check (detected every 5 minutes)
- Network is available (checked via `https://connectivitycheck.gstatic.com/generate_204`)

### 7.2 Maintenance Sequence

```
1. syncStoreWithConfig()          ← Reconcile store with current settings
   → Builds targetFlags map from current config
   → Calls store.Sync(cacheSize, targetFlags, activeQueryIDs)
     → For each image: determineSyncAction()
       → Delete if: inactive query, blocked, missing master, or zombie
       → Invalidate if: processing flags mismatch
       → Keep if: all checks pass

2. fm.CleanupOrphans(knownIDs)    ← Delete files not in the store

3. updateCallback()               ← Check for app updates (if callback set)

4. SyncProviders()                ← Remote config sync (e.g., Wallhaven collections)

5. FetchNewImages(false)          ← Download new images (if nightly refresh enabled)
```

### 7.3 Zombie Recovery

The zombie check in `determineSyncAction()` catches images that have:
- A master file on disk ✅
- Matching processing flags ✅
- **Zero derivatives** ❌

These "zombie" images are invisible to monitors and block re-fetching (because `Add()` rejects
duplicates). The zombie check deletes them from the store so the next fetch cycle can re-download
and reprocess them.

---

## 8. Known Failure Modes

### 8.1 DerivativePaths Clobbering (Pipeline Race Condition) — FIXED in v2.5.3

**Root cause**: Prior to v2.5.3, `ProcessImageJob` called `wp.store.Update(img)` at steps
2.5 and 2.7, where the local `img` variable had `DerivativePaths = nil`. This overwrote the
existing image's `DerivativePaths` in the store. When the pipeline later called `store.Add()`
with the fully-processed image, `Add()` rejected it as a duplicate.

**Pre-fix timeline** (for historical reference):

```
T=0: img = job.Image  (from API — DerivativePaths = nil)

T=1: Step 2.5 probes dimensions
     wp.store.Update(img)  ← img.DerivativePaths is nil!
     Store now has: {ID: "X", DerivativePaths: nil, Width: 4000, Height: 3000}
     ↑ Clobbered! Store previously had DerivativePaths populated.

T=2: Step 2.7 tags incompatibilities
     wp.store.Update(img)  ← img.DerivativePaths is STILL nil!
     Store still has: {ID: "X", DerivativePaths: nil}

T=3: Step 3 generates derivatives
     img.DerivativePaths = {"3440x1440": "/path/to/fitted.jpg"}
     img.FilePath = "/path/to/fitted.jpg"

T=4: stateManagerLoop calls store.Add(img)
     Add() checks: idSet["X"] exists? → YES (from T=1's Update)
     Add() returns false. FULLY-PROCESSED IMAGE IS REJECTED.

Result: Image permanently stuck with DerivativePaths = nil.
        Resolution bucket is empty. Monitor never shows it.
        Files exist on disk but metadata is corrupted.
```

**Fix applied** (v2.5.3):
1. **Removed** the two intermediate `wp.store.Update(img)` calls in `downloader.go` (steps
   2.5 and 2.7). `ProcessImageJob` is now purely functional — it mutates its local `img`
   variable and returns the final result without touching the store.
2. **Added Add-or-Update fallback** in `stateManagerLoop`: if `store.Add()` rejects a
   fully-processed image as a duplicate (e.g., during backlog healing), it falls back to
   `store.Update()` to ensure the complete result always lands.
3. **Added diagnostic logging** in `store.Update()`: a debug-level warning when
   `DerivativePaths` are being cleared from a non-empty state (defense-in-depth canary).

### 8.2 Flag Mismatch Invalidation Cascade

If `syncStoreWithConfig()` builds a target flag map that doesn't match what the downloader
sets, **all images** will be invalidated on every nightly cycle. This was caught by the
regression guard tests (`TestNightlyGrooming_RegressionGuard_IncompleteFlags`).

Both flag maps must be kept in sync — see `makeDownloaderFlags()` and `makeGroomingTarget()`
in `store_test.go` for the consistency tests.

### 8.3 Zombie → Delete → Re-Fetch → Zombie Loop

- **Why:** Bandwidth optimization. If an image is mathematically incompatible with all monitors based on the API data, we reject it before downloading the 100MB master file. *(Note: Many museum APIs do not report dimensions in their JSON, so this gate is often bypassed).*
2. **Gate 2: The Multi-Monitor Tagging Loop (Step 2.7)**
   - **When:** After the master image is downloaded and its exact dimensions are probed from disk.
   - **Why:** It iterates through the unique resolution profile of **every single plugged-in monitor**. It permanently tags the image (e.g., `img.ProcessingFlags["incompatible:3440x1440"] = true`) if it fails a specific monitor's geometry. This ensures an image is kept if it perfectly fits your 16:9 laptop screen, but tells the routing system to never send it to your 21:9 ultrawide. If the image fits *zero* monitors, the job aborts and the master file is deep-deleted.
3. **Gate 3: The Generation Loop Safety Net (Step 3)**
   - **When:** Inside `generateMissingDerivatives`, immediately before generating the fitted file.
   - **Why:** Instead of just trusting the `incompatible` tag set by Gate 2, it dynamically re-evaluates the math against current global settings. This solves the **Mode Change Problem**. If an image was tagged incompatible yesterday under strict "Quality Mode", but the user switches to permissive "Flexibility Mode" today and opens the Tune Image UI, Gate 3 ignores the stale tag. It runs the math, sees it now passes under Flexibility Mode, and processes the crop.

---

## 6. store.Update() vs store.Add()

This distinction is critical for understanding data flow and avoiding corruption.

### 6.1 Add()

Adds a **new** image to the store. Rejects duplicates.

```go
func (s *ImageStore) Add(img provider.Image) bool {
    if _, exists := s.idSet[img.ID]; exists {
        // Special case: Favorites provider can displace existing entries
        if img.SourceQueryID == FavoritesQueryID {
            // In-place update preserving Seen state
            s.images[i] = img
            return true
        }
        return false  // Duplicate — reject
    }
    // Reject images from queries disabled mid-download
    if s.QueryActiveFunc != nil && !s.QueryActiveFunc(img.SourceQueryID) {
        return false
    }
    if s.avoidSet[img.ID] { return false }  // Blocked image
    s.images = append(s.images, img)
    s.idSet[img.ID] = true
    // Add to resolution buckets (incremental, not full rebuild)
    for res := range img.DerivativePaths {
        s.resolutionBuckets[res] = append(s.resolutionBuckets[res], img.ID)
    }
    s.scheduleSaveLocked()
    s.notifyUpdateLocked()
    return true
}
```

**Key behavior**: If the ID already exists, `Add()` returns `false` and does **nothing**
(unless the source is the Favorites provider, which is allowed to displace existing entries).
This means a re-processed image must use the Add-or-Update fallback pattern in
`stateManagerLoop` to ensure its results land in the store.

### 6.2 Update()

Replaces an **existing** image by ID. Full struct replacement.

```go
func (s *ImageStore) Update(img provider.Image) bool {
    for i, existing := range s.images {
        if existing.ID == img.ID {
            s.images[i] = img  // FULL REPLACEMENT. Every field overwritten.
            s.rebuildBucketsLocked()
            s.scheduleSaveLocked()
            return true
        }
    }
    return false
}
```

**Key behavior**: There is **no field-level merge**. If you call `Update()` with an image that
has `DerivativePaths = nil`, it will overwrite the existing image's `DerivativePaths` — even if
the existing one was perfectly good.

### 6.3 All Call Sites (as of v2.5.3)

| # | Location | Purpose | Has DerivativePaths? |
|---|----------|---------|---------------------|
| 1 | `pipeline.go:150` | Add-or-Update fallback in stateManagerLoop | Yes (fully processed) |
| 2 | `wallpaper.go:831` | Un-favorite | Yes (from store.List()) |
| 3 | `wallpaper.go:841` | Add favorite | Yes (from store.List()) |
| 4 | `wallpaper.go:905` | Reconcile favorite flags | Yes (from store.List()) |
| 5 | `monitor_controller.go:502` | Stale file → clear derivatives | Intentionally empty |
| 6 | `monitor_controller.go:545` | Anchor update | Yes (from CurrentImage) |

> **Note**: Prior to v2.5.3, `downloader.go` had two additional intermediate `Update()` calls
> at steps 2.5 (resolution probing) and 2.7 (incompatibility tagging) that clobbered
> `DerivativePaths`. These were removed as part of the fix described in §8.1.

Call site 5 is an intentional clearing — any guard on `Update()` must not block it.

---

## 7. The Nightly Refresh Cycle

**File**: `pkg/wallpaper/scheduler.go`

The nightly scheduler runs as a persistent goroutine and triggers maintenance on day changes.

### 7.1 Trigger Conditions

A nightly refresh runs when:
- The app starts and detects it's shortly after midnight (within 6 minutes)
- The day has changed since the last check (detected every 5 minutes)
- Network is available (checked via `https://connectivitycheck.gstatic.com/generate_204`)

### 7.2 Maintenance Sequence

```
1. syncStoreWithConfig()          ← Reconcile store with current settings
   → Builds targetFlags map from current config
   → Calls store.Sync(cacheSize, targetFlags, activeQueryIDs)
     → For each image: determineSyncAction()
       → Delete if: inactive query, blocked, missing master, or zombie
       → Invalidate if: processing flags mismatch
       → Keep if: all checks pass

2. fm.CleanupOrphans(knownIDs)    ← Delete files not in the store

3. updateCallback()               ← Check for app updates (if callback set)

4. SyncProviders()                ← Remote config sync (e.g., Wallhaven collections)

5. FetchNewImages(false)          ← Download new images (if nightly refresh enabled)
```

### 7.3 Zombie Recovery

The zombie check in `determineSyncAction()` catches images that have:
- A master file on disk ✅
- Matching processing flags ✅
- **Zero derivatives** ❌

These "zombie" images are invisible to monitors and block re-fetching (because `Add()` rejects
duplicates). The zombie check deletes them from the store so the next fetch cycle can re-download
and reprocess them.

---

## 8. Known Failure Modes

### 8.1 DerivativePaths Clobbering (Pipeline Race Condition) — FIXED in v2.5.3

**Root cause**: Prior to v2.5.3, `ProcessImageJob` called `wp.store.Update(img)` at steps
2.5 and 2.7, where the local `img` variable had `DerivativePaths = nil`. This overwrote the
existing image's `DerivativePaths` in the store. When the pipeline later called `store.Add()`
with the fully-processed image, `Add()` rejected it as a duplicate.

**Pre-fix timeline** (for historical reference):

```
T=0: img = job.Image  (from API — DerivativePaths = nil)

T=1: Step 2.5 probes dimensions
     wp.store.Update(img)  ← img.DerivativePaths is nil!
     Store now has: {ID: "X", DerivativePaths: nil, Width: 4000, Height: 3000}
     ↑ Clobbered! Store previously had DerivativePaths populated.

T=2: Step 2.7 tags incompatibilities
     wp.store.Update(img)  ← img.DerivativePaths is STILL nil!
     Store still has: {ID: "X", DerivativePaths: nil}

T=3: Step 3 generates derivatives
     img.DerivativePaths = {"3440x1440": "/path/to/fitted.jpg"}
     img.FilePath = "/path/to/fitted.jpg"

T=4: stateManagerLoop calls store.Add(img)
     Add() checks: idSet["X"] exists? → YES (from T=1's Update)
     Add() returns false. FULLY-PROCESSED IMAGE IS REJECTED.

Result: Image permanently stuck with DerivativePaths = nil.
        Resolution bucket is empty. Monitor never shows it.
        Files exist on disk but metadata is corrupted.
```

**Fix applied** (v2.5.3):
1. **Removed** the two intermediate `wp.store.Update(img)` calls in `downloader.go` (steps
   2.5 and 2.7). `ProcessImageJob` is now purely functional — it mutates its local `img`
   variable and returns the final result without touching the store.
2. **Added Add-or-Update fallback** in `stateManagerLoop`: if `store.Add()` rejects a
   fully-processed image as a duplicate (e.g., during backlog healing), it falls back to
   `store.Update()` to ensure the complete result always lands.
3. **Added diagnostic logging** in `store.Update()`: a debug-level warning when
   `DerivativePaths` are being cleared from a non-empty state (defense-in-depth canary).

### 8.2 Flag Mismatch Invalidation Cascade

If `syncStoreWithConfig()` builds a target flag map that doesn't match what the downloader
sets, **all images** will be invalidated on every nightly cycle. This was caught by the
regression guard tests (`TestNightlyGrooming_RegressionGuard_IncompleteFlags`).

Both flag maps must be kept in sync — see `makeDownloaderFlags()` and `makeGroomingTarget()`
in `store_test.go` for the consistency tests.

### 8.3 Zombie → Delete → Re-Fetch → Zombie Loop

If the zombie recovery deletes images but the fetch cycle re-adds them without generating
derivatives (e.g., due to a compatibility rejection), the same images will be re-added as
zombies and deleted again on the next nightly cycle. This is a benign loop (no data loss)
but wastes API calls and disk I/O.

---

## 9. Virtual Framer & Tune Image Overrides

The pipeline strictness described above is intentionally rigid to protect users from bad crops. However, Spice provides two massive override systems:

### 9.1 The Virtual Framer Rescue Loop (Sentinel Error Pattern)

When an image reaches Step 2.7 (Compatibility Check) and fails because its aspect ratio is too extreme (e.g., a tall portrait on an ultrawide monitor in Quality Mode), the pipeline normally rejects it and tags it as fundamentally incompatible.

However, if **Virtual Framing** is enabled (usually globally via Museum Mode settings), the `VirtualFramer` intercepts the mathematical failure and explicitly returns the Sentinel Error `ErrRequiresVirtualFraming` instead of returning a standard error or `nil`.

1. The three Compatibility Gates in `downloader.go` explicitly check for `!errors.Is(err, ErrRequiresVirtualFraming)`.
2. If the sentinel error is detected, the gates bypass the normal rejection flow and allow the image to proceed down the pipeline as if it were valid.
3. The image finally reaches `FitImage()`, where the `VirtualFramer` handles the actual composition: generating a blurred background and painting the uncropped image in the center.
4. It tags the image with `VirtualFramed:<resolution>` in `ProcessingFlags`.

This Sentinel Error pattern strictly decouples the *intent to rescue* (during the gate checks) from the *act of rescuing* (during `FitImage`), ensuring that no expensive processing occurs until absolutely necessary.

### 9.2 Tune Image Dynamic Lock

When the user opens the "Tune Image" dialog from the system tray, the UI queries the `SmartImageProcessor` *on the fly* to determine if the image is natively compatible with the current monitor and SmartFit mode.

- **If Incompatible (Quality Mode)**: The user is locked out of unchecking the "Enable Frame" box. They can adjust frame size, matting, and wall color, but the pipeline enforces the frame to prevent a catastrophic manual crop.
- **If Compatible (or Flexibility Mode)**: The user can freely uncheck the frame, and the system bypasses the normal `downloader.go` pipeline entirely, directly routing the raw master image through `FitImage` with the user's manual 9-slice anchor choices.
