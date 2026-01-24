---
layout: default
title: Architecture
---

# Spice Architecture Documentation (Wallpaper Plugin)

> **Status**: Current as of v1.6.5
> **Focus**: Concurrency Model, Image Pipeline, and UI Synchronization

## 1. Executive Summary

Spice employs a **Single-Writer, Multiple-Reader (SWMR)** concurrency architecture to separate resource-intensive operations (image processing, I/O) from the user interface. This design eliminates UI main-thread blocking ("jank") and lock contention, ensuring a buttery-smooth user experience even during heavy background downloads.

## 2. System Architecture (Plugin System)

Spice is built as a modular application where core functionality is delivered via plugins.

```mermaid
graph TD
    classDef core fill:#e3f2fd,stroke:#1565c0,stroke-width:2px,color:#000,font-size:16px;
    classDef plug fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px,color:#000,font-size:16px;

    App[Spice Application]:::core
    PM[Plugin Manager]:::core
    
    subgraph Plugins ["Loaded Plugins"]
        WP[Wallpaper Plugin]:::plug
        Other[Other Plugins...]:::plug
    end

    App -->|Initializes| PM
    PM -->|Manages Lifecycle| Plugins
    
    WP -->|Injects| SettingsUI[Settings Tab]:::core
    WP -->|Injects| TrayUI[Tray Menu Items]:::core
```

## 3. Wallpaper Plugin Architecture

### 3.1 Core Concepts

#### 3.1.1 The Single-Writer Principle

To prevent race conditions and lock contention, **only one** goroutine is allowed to mutate the global state (Image Store). All other components, including the UI, are **Readers** or **Command Senders**.

#### 3.1.2 Decoupled UI Session

The User Interface maintains its own **local session state** (which image am I looking at right now?). It strictly reads from the shared store and sends asynchronous commands to request changes.

## 3.2 High-Level Architecture

The system is divided into two distinct execution contexts:

1. **The UI Context (Main Thread)**: Handles user input, rendering, and navigation. It is optimized for **Read Speed**.
2. **The Pipeline Context (Background)**: Handles downloading, processing, and state mutation. It is optimized for **Throughput**.

```mermaid
graph TD
    classDef ui fill:#e1f5fe,stroke:#01579b,stroke-width:2px,color:#000,font-size:16px;
    classDef pipe fill:#fff3e0,stroke:#e65100,stroke-width:2px,color:#000,font-size:16px;
    classDef store fill:#e8f5e9,stroke:#1b5e20,stroke-width:2px,color:#000,font-size:16px;

    subgraph UI_Context ["UI Context (Plugin)"]
        NextBtn[Next Button]:::ui
        PrevBtn[Delete Button]:::ui
        LocalState["Session State<br/>(Current Index, History)"]:::ui
    end

    subgraph Shared_Resource [Shared Resources]
        Store[(ImageStore)]:::store
    end

    subgraph Pipeline_Context ["Pipeline Context"]
        Workers[["Worker Pool<br/>(Download/Crop)"]]:::pipe
        LazyWorker(("Persistent Worker<br/>(Enrichment)")):::pipe
        Manager(("State Manager<br/>Loop")):::pipe
    end

    %% Reads
    NextBtn -- 1. RLock (Fast) --> Store
    LocalState -- Read Image Info --> Store

    %% Async Commands
    NextBtn -- 2. CmdMarkSeen (Async) --> Manager
    PrevBtn -- CmdDelete (Async) --> Manager
    Workers -- Result (New Image) --> Manager

    %% Writers
    Manager -- 3. Lock (Exclusive) --> Store

    %% Feedback
    Manager -- Yield (Gosched) --> Manager
```

## 3.3 Component Details

### 4.1 ImageStore (`pkg/wallpaper/store.go`)

A thread-safe, stateless container.

- **Role**: The "Database" of the application.
- **Locking**: Uses `sync.RWMutex`.
  - **Reads (Next Button)**: Uses `RLock()` (Non-blocking).
  - **Writes (Download/MarkSeen)**: Uses `Lock()` (Exclusive).
  - **Persistence**: Debounced (2s) and uses `RLock()` to save to disk without blocking readers.
- **Constraints**: Contains no iteration logic (no `currentIndex`).

### 4.2 Pipeline State Manager (`pkg/wallpaper/pipeline.go`)

The "Brain" of the backend.

- **Role**: Consumes results from workers and commands from the UI.
- **Behavior**:
  - Loops continuously selecting on `resultChan` and `cmdChan`.
  - Acquires Write Lock -> Mutates Store -> Releases Lock.
  - **Yields**: Calls `runtime.Gosched()` after every operation to prevent starving readers.

### 4.3 UI Plugin (`pkg/wallpaper/wallpaper.go`)

The "Controller".

- **Role**: Manages the user session and optimistic UI updates.
- **Key Logic**:
  - **Navigation**: Calculates next index locally, reads from store.
  - **Optimistic Updates**: Updates Tray Menu *before* calling blocking OS wallpaper functions.
  - **Rollback**: If OS call fails, reverts UI to previous state.

### 4.4 Lazy Enrichment Worker (`startEnrichmentWorker`)

A persistent, single-goroutine worker that "pivots" to the user's current location.

- **Role**: Fetches metadata (Enrichment) for upcoming images.
- **Behavior**:
  - Listens on `enrichmentSignal` (buffered channel).
  - When user navigates, instantly aborts current look-ahead and jumps to new index.
  - Ensures metadata is ready *just in time*, reducing API waste.

## 3.4 Interaction Flows

### 3.4.1 "Next Wallpaper" Flow (Zero Contention)

This flow demonstrates how the UI updates instantly without waiting for a write lock.

```mermaid
sequenceDiagram
    participant User
    participant UI as UI (Plugin)
    participant Store as ImageStore
    participant Mgr as State Manager

    User->>UI: Click "Next"
    activate UI
    Note right of UI: 1. Calculate new index (Local)
    UI->>Store: Get(newIndex) [RLock]
    Store-->>UI: Image
    
    Note right of UI: 2. Optimistic Update
    UI->>UI: Update Tray Menu & currentImage
    
    Note right of UI: 3. Async Command
    UI--)Mgr: CmdMarkSeen(ImageID)
    
    Note right of UI: 4. Apply Wallpaper
    UI->>OS: setWallpaper(path)
    
    deactivate UI
    
    activate Mgr
    Note left of Mgr: Process CmdMarkSeen
    Mgr->>Store: MarkSeen() [Lock]
    
    deactivate Mgr
```

### 3.4.2 "Delete Wallpaper" Flow

Deleting requires modifying the store, handled asynchronously.

```mermaid
sequenceDiagram
    participant User
    participant UI as UI (Plugin)
    participant Mgr as State Manager
    participant Store as ImageStore

    User->>UI: Click "Delete"
    activate UI
    
    UI--)Mgr: CmdRemove(ImageID)
    UI->>OS: Remove File (Disk)
    
    Note right of UI: Move to Next
    UI->>UI: setNextWallpaper()
    
    deactivate UI
    
    activate Mgr
    Note left of Mgr: Process CmdRemove
    Mgr->>Store: Remove(ID) [Lock]
    deactivate Mgr
```

## 3.5 Directory Structure & Key Files

| Component | File Path | Responsibility |
| :--- | :--- | :--- |
| **Store** | `pkg/wallpaper/store.go` | Data repository. RWMutex protected. |
| **Pipeline** | `pkg/wallpaper/pipeline.go` | Worker pool & State Manager Loop. |
| **Controller**| `pkg/wallpaper/wallpaper.go` | UI logic, Lifecycle, Optimistic Updates. |
| **Processor** | `pkg/wallpaper/smart_image_processor.go` | Face detection, cropping (Heavy CPU). |

## 3.5 Resource Management

### 3.5.1 Deep Cache Cleaning (Recursive Deletion)

Spice implements a strict "Zero Orphans" policy for resource management. When a wallpaper collection (Query) is deleted:

1.  **Configuration Callback**: The `Config` triggers a registered callback (`onQueryRemoved`).
2.  **Store Pruning**: The callback invokes `store.RemoveByQueryID(queryID)`.
3.  **Deep Delete**: The File Manager's `DeepDelete` function is called for every image ID:
    - Deletes the **Master Image**.
    - recursively deletes all **Derivatives** (Smart Fit, Face Crop, Face Boost images) in their respective subdirectories.

### 3.5.2 Provider Strategies

Spice supports two distinct provider interaction models:

*   **API Fetch (Standard)**:
    *   **Examples**: Wallhaven, Pexels.
    *   **Flow**: Query API -> Get URLs -> Download One-by-One on demand.
    *   **State**: Ephemeral. Images are only downloaded when viewed (or pre-fetched).

*   **Import / Pick (Google Photos)**:
    *   **Example**: Google Photos.
    *   **Flow**: Launch Picker -> Select N Items -> Bulk Download Immediately to `cache/google_photos/<GUID>`.
    *   **State**: Local. The provider acts as a local file scanner over the imported directory.
    *   **Cleanup**: Deleting the collection deletes the entire backing folder.

### 3.5.3 Provider Categorization

To manage the growing number of providers, Spice categorizes them into three distinct types (`provider.ProviderType`), which dictates their UI placement:

*   **TypeOnline**: Remote APIs (Unsplash, Pexels). Placed in the "Online" tab working via network fetch.
*   **TypeLocal**: Local filesystem interactions (Favorites, Local Files). Placed in the "Local" tab.
*   **TypeAI**: Generative or logical providers. Placed in the "AI" tab (Future).


- **Events**: The `cmdChan` pattern can be expanded to a full Event Bus if the application grows complexity (e.g., specific event subscribers).

## 3.7 Performance Strategies

To maintain responsiveness under load, the following optimizations are employed:

1. **O(1) Image Store**: The store uses a secondary `idSet map[string]bool` to perform existence checks in constant time (45ns) rather than linear scans (470ns+), ensuring that the Writer Loop never lags even with thousands of images.
2. **Synchronous Race Prevention**: The Controller synchronously anticipates background work (setting `isDownloading = true` under lock) *before* spawning goroutines. This prevents "job storms" and CPU saturation during rapid UI interactions.

## 3.8 Smart Fit Algorithm
Spice uses a "Holistic Imaging" approach that combines Face Detection (Pigo), Entropy Analysis (SmartCrop), and Composition Rules.

*   **Face Rescue**: High-quality images with incorrect aspect ratios are "Rescued" only if a dominant face is detected, ensuring we never crop heads.
*   **Feet Guard**: A heuristic that prevents the cropper from selecting the bottom 20% of an image (usually shoes/legs) unless the image has "High Energy" (complex texture).
*   **Tuning**: All heuristic parameters are externalized in `pkg/wallpaper/tuning.go` to separate logic from magic numbers.

## 3.9 The "Git-Driven" Content System
For verified providers (Museums), Spice treats `raw.githubusercontent.com` as a Content Delivery Network (CDN).
*   **Architecture**: `Remote > Cache > Embed > Hardcoded`.
*   **Benefit**: Allows instant curation updates (adding new artworks to "Director's Cut") without requiring users to download a binary update.

<!-- Mermaid JS Handling -->
<script>
  document.addEventListener("DOMContentLoaded", function() {
    // Find all code blocks with 'language-mermaid'
    const codeBlocks = document.querySelectorAll('code.language-mermaid');
    codeBlocks.forEach(code => {
      // Jekyll usually renders: <div class="highlighter-rouge"><div class="highlight"><pre class="highlight"><code>...</code></pre></div></div>
      // We need to allow the text to be processed by Mermaid.
      // Easiest is to create a new div.mermaid and replace the pre/code block.
      
      const div = document.createElement('div');
      div.className = 'mermaid';
      div.textContent = code.textContent;
      
      // Find the closest container that we want to replace (usually the pre or the div wrapper)
      const wrapper = code.closest('pre');
      if (wrapper) {
        wrapper.replaceWith(div);
      }
    });
  });
</script>

<script type="module">
  import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
  mermaid.initialize({ startOnLoad: true, theme: 'default' });
</script>
